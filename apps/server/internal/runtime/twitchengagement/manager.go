package twitchengagement

import (
	"context"
	"log/slog"
	"sync"
	"time"

	bus "github.com/streaming-tree/server/internal/engagement"

	"github.com/streaming-tree/server/internal/domain/account"
	"github.com/streaming-tree/server/internal/domain/engagementsettings"
	"github.com/streaming-tree/server/internal/provider/twitch"
)

// Manager supervises one Twitch EventSub connector per enabled connected
// Twitch account. See the package doc comment (state.go) for the full
// design.
type Manager struct {
	accounts *account.Service
	settings *engagementsettings.Service
	bus      *bus.Bus
	client   *twitch.Client
	logger   *slog.Logger
	now      func() time.Time

	// allowedReconnectHosts is checked against every session_reconnect URL
	// before it is ever dialed - the real Twitch host in production, plus a
	// test host only when explicitly configured (see cmd/testserver).
	allowedReconnectHosts []string

	// destinationLookup resolves a connected account's linked destination
	// id, when unambiguous - optional; nil means DestinationID is always
	// left empty on published events.
	destinationLookup func(accountID string) (string, bool)

	mu         sync.Mutex
	connectors map[string]*connector
	started    bool

	lifecycle context.Context
	cancelAll context.CancelFunc
	workers   sync.WaitGroup
}

// Options constructs a Manager.
type Options struct {
	Accounts              *account.Service
	Settings              *engagementsettings.Service
	Bus                   *bus.Bus
	Client                *twitch.Client
	Logger                *slog.Logger
	AllowedReconnectHosts []string
	DestinationLookup     func(accountID string) (string, bool)
}

// NewManager builds a Manager. Call Start before using it.
func NewManager(opts Options) *Manager {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	hosts := opts.AllowedReconnectHosts
	if len(hosts) == 0 {
		hosts = []string{"eventsub.wss.twitch.tv"}
	}
	return &Manager{
		accounts:              opts.Accounts,
		settings:              opts.Settings,
		bus:                   opts.Bus,
		client:                opts.Client,
		logger:                logger,
		now:                   func() time.Time { return time.Now().UTC() },
		allowedReconnectHosts: hosts,
		destinationLookup:     opts.DestinationLookup,
		connectors:            make(map[string]*connector),
	}
}

// Start loads every enabled account's engagement setting and starts an
// eligible connector for each, asynchronously - it never blocks the HTTP
// server on a slow or unreachable Twitch.
func (m *Manager) Start(ctx context.Context) error {
	m.lifecycle, m.cancelAll = context.WithCancel(context.Background())

	m.mu.Lock()
	m.started = true
	m.mu.Unlock()

	enabled, err := m.settings.ListEnabled(ctx)
	if err != nil {
		return err
	}
	for _, s := range enabled {
		acc, err := m.accounts.GetAccount(ctx, s.AccountID)
		if err != nil {
			continue // account no longer exists; its settings row will be cleaned up separately
		}
		if acc.ProviderID != account.ProviderTwitch {
			continue
		}
		m.reconcile(acc)
	}
	return nil
}

// Shutdown stops every running connector and waits for their goroutines to
// exit, bounded by ctx.
func (m *Manager) Shutdown(ctx context.Context) {
	if m.cancelAll != nil {
		m.cancelAll()
	}
	done := make(chan struct{})
	go func() {
		m.workers.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

// reconcile starts a connector for acc if its capability assessment allows
// it, or records a blocked snapshot otherwise. Used both at Start() and
// after Enable().
func (m *Manager) reconcile(acc account.Account) Snapshot {
	assessment := twitch.AssessEngagementCapability(acc.Scopes)

	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.connectors[acc.ID]; ok {
		return existing.getSnapshot()
	}

	c := newConnector(m, acc.ID)
	m.connectors[acc.ID] = c

	if !assessment.Available {
		c.setBlocked([]string{BlockerScopeUpgradeRequired}, assessment.Missing)
		return c.getSnapshot()
	}

	ctx, cancel := context.WithCancel(m.lifecycle)
	c.cancel = cancel
	m.workers.Add(1)
	go func() {
		defer m.workers.Done()
		c.run(ctx)
	}()

	return c.getSnapshot()
}

// Enable persists accountID's engagement setting as enabled and starts (or
// reports blocked for) its connector.
func (m *Manager) Enable(ctx context.Context, accountID string) (Snapshot, error) {
	acc, err := m.accounts.GetAccount(ctx, accountID)
	if err != nil {
		return Snapshot{}, err
	}
	if acc.ProviderID != account.ProviderTwitch {
		return Snapshot{}, ErrUnsupportedProvider
	}
	if _, err := m.settings.SetEnabled(ctx, accountID, true); err != nil {
		return Snapshot{}, err
	}
	return m.reconcile(acc), nil
}

// Disable persists accountID's engagement setting as disabled and stops its
// connector.
func (m *Manager) Disable(ctx context.Context, accountID string) (Snapshot, error) {
	if _, err := m.settings.SetEnabled(ctx, accountID, false); err != nil {
		return Snapshot{}, err
	}
	m.stop(accountID)
	return Snapshot{AccountID: accountID, Enabled: false, State: StateDisabled}, nil
}

// stop cancels and removes accountID's connector, if any. Used by Disable
// and by the account-disconnect cleanup hook.
func (m *Manager) stop(accountID string) {
	m.mu.Lock()
	c, ok := m.connectors[accountID]
	if ok {
		delete(m.connectors, accountID)
	}
	m.mu.Unlock()
	if !ok {
		return
	}
	c.setState(StateStopping)
	if c.cancel != nil {
		c.cancel()
	}
}

// StopAndRemove stops accountID's connector and forgets it entirely - used
// when the underlying connected account is disconnected, so no lingering
// snapshot or goroutine survives with a now-invalid token.
func (m *Manager) StopAndRemove(accountID string) {
	m.stop(accountID)
}

// Restart cancels and restarts accountID's connector without changing its
// persisted enabled setting - an operational recovery action.
func (m *Manager) Restart(ctx context.Context, accountID string) (Snapshot, error) {
	acc, err := m.accounts.GetAccount(ctx, accountID)
	if err != nil {
		return Snapshot{}, err
	}
	settings, found, err := m.settings.Get(ctx, accountID)
	if err != nil {
		return Snapshot{}, err
	}
	if !found || !settings.Enabled {
		return Snapshot{}, ErrNotFound
	}
	m.stop(accountID)
	return m.reconcile(acc), nil
}

// Snapshot returns one account's current connector status.
func (m *Manager) Snapshot(accountID string) (Snapshot, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.connectors[accountID]
	if !ok {
		return Snapshot{}, false
	}
	return c.getSnapshot(), true
}

// Snapshots returns every currently-tracked connector's status - used for
// GET /api/engagement/status's connector summaries.
func (m *Manager) Snapshots() []Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Snapshot, 0, len(m.connectors))
	for _, c := range m.connectors {
		out = append(out, c.getSnapshot())
	}
	return out
}
