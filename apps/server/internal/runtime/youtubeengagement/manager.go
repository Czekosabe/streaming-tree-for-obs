package youtubeengagement

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	bus "github.com/streaming-tree/server/internal/engagement"

	"github.com/streaming-tree/server/internal/domain/account"
	"github.com/streaming-tree/server/internal/domain/engagementsettings"
	"github.com/streaming-tree/server/internal/provider/youtube"
)

// Manager supervises one YouTube Live Chat gRPC streamList connector per
// enabled connected YouTube account. See state.go's package doc comment
// for the full design and how it deliberately differs from
// internal/runtime/twitchengagement.
type Manager struct {
	accounts *account.Service
	settings *engagementsettings.Service
	bus      *bus.Bus
	client   *youtube.Client
	logger   *slog.Logger
	now      func() time.Time

	// destinationLookup resolves a connected account's linked destination
	// (platform) id, when unambiguous - reused from the exact same
	// pattern internal/runtime/twitchengagement already uses.
	destinationLookup func(accountID string) (string, bool)
	// broadcastLookup resolves a destination (platform) id's currently
	// selected YouTube live-broadcast id, when one is set - wraps
	// internal/domain/remotetarget.Service.GetTarget, reusing Stage 7B's
	// existing selected-broadcast persistence rather than inventing a
	// second, account-scoped selector (per this stage's own explicit
	// instruction).
	broadcastLookup func(platformID string) (broadcastID string, ok bool)

	mu         sync.Mutex
	connectors map[string]*connector
	started    bool

	lifecycle context.Context
	cancelAll context.CancelFunc
	workers   sync.WaitGroup
}

// Options constructs a Manager.
type Options struct {
	Accounts          *account.Service
	Settings          *engagementsettings.Service
	Bus               *bus.Bus
	Client            *youtube.Client
	Logger            *slog.Logger
	DestinationLookup func(accountID string) (string, bool)
	BroadcastLookup   func(platformID string) (broadcastID string, ok bool)
}

// NewManager builds a Manager. Call Start before using it.
func NewManager(opts Options) *Manager {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		accounts: opts.Accounts, settings: opts.Settings, bus: opts.Bus, client: opts.Client,
		logger: logger, now: func() time.Time { return time.Now().UTC() },
		destinationLookup: opts.DestinationLookup, broadcastLookup: opts.BroadcastLookup,
		connectors: make(map[string]*connector),
	}
}

// Start loads every enabled account's engagement setting and starts an
// eligible connector for each, asynchronously.
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
		if acc.ProviderID != account.ProviderYouTube {
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

// reconcile starts a connector for acc, or records a blocked snapshot if
// the account itself is unhealthy. Unlike Twitch, no separate engagement
// scope assessment is needed - the existing youtube.RequiredScope already
// covers every Stage 15A operation (docs/provider-integrations/
// youtube-engagement.md §1/§3.6), so there is no scope-upgrade blocker
// here.
func (m *Manager) reconcile(acc account.Account) Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.connectors[acc.ID]; ok {
		return existing.getSnapshot()
	}

	c := newConnector(m, acc.ID)
	m.connectors[acc.ID] = c

	if acc.Status == account.StatusReconnectRequired {
		c.setBlocked([]string{BlockerAccountUnhealthy})
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
	if acc.ProviderID != account.ProviderYouTube {
		return Snapshot{}, ErrUnsupportedProvider
	}
	if _, err := m.settings.SetEnabled(ctx, accountID, true); err != nil {
		return Snapshot{}, err
	}
	return m.reconcile(acc), nil
}

// Disable persists accountID's engagement setting as disabled and stops
// its connector - never deletes the account, never revokes the OAuth
// token, only clears this connector's own runtime state (per this task's
// explicit "disable" requirements).
func (m *Manager) Disable(ctx context.Context, accountID string) (Snapshot, error) {
	if _, err := m.settings.SetEnabled(ctx, accountID, false); err != nil {
		return Snapshot{}, err
	}
	m.stop(accountID)
	return Snapshot{AccountID: accountID, Enabled: false, State: StateDisabled}, nil
}

// stop cancels and removes accountID's connector, if any.
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
// when the underlying connected account is disconnected.
func (m *Manager) StopAndRemove(accountID string) {
	m.stop(accountID)
}

// Restart cancels and restarts accountID's connector without changing its
// persisted enabled setting - also the mechanism used when the operator
// changes the selected broadcast, so the old chat's runtime state (and any
// in-flight poll) is fully discarded before the new one is resolved,
// exactly as this stage's own "never leak old-chat events into the
// newly-selected broadcast" requirement demands.
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

// openStream opens one streamList gRPC stream for accountID/liveChatID/
// pageToken, reusing account.Service.WithFreshToken's existing single-
// flight-refresh-then-retry-once pattern exactly the way every other
// YouTube call in this codebase already does (docs/provider-integrations/
// youtube-engagement.md §2) - the gRPC transport's own initial
// authentication failure (UNAUTHENTICATED, mapped to youtube.ErrUnauthorized
// by classifyStreamError) is treated identically to a REST 401 here. Once
// the stream is open, its own Recv() calls are not individually wrapped in
// WithFreshToken - see connector.go's classifyPollError doc comment for why
// a mid-stream auth failure is instead treated as a retryable reconnect
// (the next attempt's resolveBroadcast/resolveLiveChatID/openStream calls
// each go through WithFreshToken again and pick up a refreshed token
// naturally).
func (m *Manager) openStream(ctx context.Context, accountID, liveChatID, pageToken string) (*youtube.LiveChatStream, error) {
	var stream *youtube.LiveChatStream
	err := m.accounts.WithFreshToken(ctx, accountID, func(accessToken string) (bool, error) {
		s, err := m.client.OpenLiveChatStream(ctx, liveChatID, pageToken, accessToken)
		if err != nil {
			if errors.Is(err, youtube.ErrUnauthorized) {
				return true, err
			}
			return false, err
		}
		stream = s
		return false, nil
	})
	if err != nil {
		return nil, err
	}
	return stream, nil
}

// Snapshots returns every currently-tracked connector's status.
func (m *Manager) Snapshots() []Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Snapshot, 0, len(m.connectors))
	for _, c := range m.connectors {
		out = append(out, c.getSnapshot())
	}
	return out
}
