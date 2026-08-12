package streamelementsengagement

import (
	"context"
	"log/slog"
	"sync"
	"time"

	bus "github.com/streaming-tree/server/internal/engagement"

	"github.com/streaming-tree/server/internal/domain/donationsource"
	"github.com/streaming-tree/server/internal/provider/streamelements"
	"github.com/streaming-tree/server/internal/secrets"
)

// Manager supervises one StreamElements Astro connector per enabled
// donation source. See state.go's package doc comment for the full design.
type Manager struct {
	sources *donationsource.Service
	secrets secrets.SecretStore
	bus     *bus.Bus
	client  *streamelements.Client
	logger  *slog.Logger
	now     func() time.Time

	mu         sync.Mutex
	connectors map[string]*connector
	started    bool

	lifecycle context.Context
	cancelAll context.CancelFunc
	workers   sync.WaitGroup
}

// Options constructs a Manager.
type Options struct {
	Sources *donationsource.Service
	Secrets secrets.SecretStore
	Bus     *bus.Bus
	Client  *streamelements.Client
	Logger  *slog.Logger
}

// NewManager builds a Manager. Call Start before using it.
func NewManager(opts Options) *Manager {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	client := opts.Client
	if client == nil {
		client = streamelements.New(streamelements.Options{})
	}
	return &Manager{
		sources: opts.Sources, secrets: opts.Secrets, bus: opts.Bus, client: client,
		logger: logger, now: func() time.Time { return time.Now().UTC() },
		connectors: make(map[string]*connector),
	}
}

// Start loads every donation source and starts a connector for each one
// currently enabled, asynchronously.
func (m *Manager) Start(ctx context.Context) error {
	m.lifecycle, m.cancelAll = context.WithCancel(context.Background())

	m.mu.Lock()
	m.started = true
	m.mu.Unlock()

	all, err := m.sources.List(ctx)
	if err != nil {
		return err
	}
	for _, src := range all {
		if !src.Enabled {
			continue
		}
		m.reconcile(src)
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

// reconcile starts a connector for src if one is not already running.
func (m *Manager) reconcile(src donationsource.Source) Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.connectors[src.ID]; ok {
		return existing.getSnapshot()
	}

	c := newConnector(m, src.ID)
	m.connectors[src.ID] = c

	ctx, cancel := context.WithCancel(m.lifecycle)
	c.cancel = cancel
	m.workers.Add(1)
	go func() {
		defer m.workers.Done()
		c.run(ctx)
	}()

	return c.getSnapshot()
}

// Enable persists sourceID's Enabled flag and starts its connector.
func (m *Manager) Enable(ctx context.Context, sourceID string) (Snapshot, error) {
	src, err := m.sources.SetEnabled(ctx, sourceID, true)
	if err != nil {
		return Snapshot{}, err
	}
	return m.reconcile(src), nil
}

// Disable persists sourceID's Enabled flag as false and stops its
// connector - never deletes the source or its stored credential, only
// clears this connector's own runtime state.
func (m *Manager) Disable(ctx context.Context, sourceID string) (Snapshot, error) {
	if _, err := m.sources.SetEnabled(ctx, sourceID, false); err != nil {
		return Snapshot{}, err
	}
	m.stop(sourceID)
	return Snapshot{SourceID: sourceID, Enabled: false, State: StateDisabled}, nil
}

// stop cancels and removes sourceID's connector, if any.
func (m *Manager) stop(sourceID string) {
	m.mu.Lock()
	c, ok := m.connectors[sourceID]
	if ok {
		delete(m.connectors, sourceID)
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

// StopAndRemove stops sourceID's connector and forgets it entirely - the
// donationsource.Options.OnSourceRemoved callback target, used when the
// underlying donation source is deleted.
func (m *Manager) StopAndRemove(sourceID string) {
	m.stop(sourceID)
}

// Restart cancels and restarts sourceID's connector without changing its
// persisted Enabled setting - used after a credential replacement so a
// stale/rejected connection is fully discarded before the new credential
// is tried.
func (m *Manager) Restart(ctx context.Context, sourceID string) (Snapshot, error) {
	src, found, err := m.sources.Get(ctx, sourceID)
	if err != nil {
		return Snapshot{}, err
	}
	if !found || !src.Enabled {
		return Snapshot{}, ErrNotFound
	}
	m.stop(sourceID)
	return m.reconcile(src), nil
}

// Snapshot returns one source's current connector status.
func (m *Manager) Snapshot(sourceID string) (Snapshot, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.connectors[sourceID]
	if !ok {
		return Snapshot{}, false
	}
	return c.getSnapshot(), true
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

// getSource re-reads sourceID's current metadata (in particular
// RemoteChannelID) - read fresh on every connector attempt rather than
// captured once, so an operator's metadata edit takes effect on the next
// reconnect without requiring an explicit Restart.
func (m *Manager) getSource(ctx context.Context, sourceID string) (donationsource.Source, bool) {
	src, found, err := m.sources.Get(ctx, sourceID)
	if err != nil || !found {
		return donationsource.Source{}, false
	}
	return src, true
}
