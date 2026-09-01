package streamsession

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/runtime/branch"
	"github.com/streaming-tree/server/internal/runtime/mediamtx"
)

// DefaultPollInterval is how often the Manager polls ingest/branch
// state - there is no push/event mechanism for either today (docs/
// stream-session-history.md §1).
const DefaultPollInterval = 5 * time.Second

// DefaultGraceWindow is how long ingest may stay non-receiving before
// an open session is actually closed - long enough to absorb a normal
// OBS reconnect blip, short enough that a genuinely ended session
// closes within about a minute (docs/stream-session-history.md §1).
const DefaultGraceWindow = 60 * time.Second

// DefaultPruneInterval is how often the retention sweep runs - reuses
// the same poll-loop timer rather than a separate scheduled job with
// its own failure mode to reason about (docs/stream-session-history.md
// §6), gated to this coarser cadence so a fresh prune DELETE does not
// run on every single poll tick.
const DefaultPruneInterval = time.Hour

// NewSessionID returns a random, non-sequential session identifier -
// matching platform.NewID's own reasoning (sequential integers would
// leak how many sessions exist and invite enumeration).
func NewSessionID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate stream session id: %w", err)
	}
	return "sess_" + hex.EncodeToString(buf), nil
}

// NewDestinationID returns a random, non-sequential destination-
// participation-row identifier.
func NewDestinationID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate stream session destination id: %w", err)
	}
	return "sessdest_" + hex.EncodeToString(buf), nil
}

// Clock returns the current time; injected so tests are deterministic.
type Clock func() time.Time

// BranchSnapshotter is the narrow port onto branch.Manager this domain
// needs - never a business-mutating method, read-only.
type BranchSnapshotter interface {
	Snapshot(ctx context.Context) ([]branch.Snapshot, error)
}

// IngestSnapshotter is the narrow port onto mediamtx.Supervisor this
// domain needs.
type IngestSnapshotter interface {
	Snapshot() mediamtx.Snapshot
}

// PlatformLookup resolves a destination's provider/display name at the
// moment its participation row is created - never re-resolved later
// (docs/stream-session-history.md §3).
type PlatformLookup interface {
	Get(ctx context.Context, id string) (platform.Platform, error)
}

// Manager runs the Stage 24 poll loop: on each tick it reads real
// ingest/branch state and derives session/destination-participation
// rows from it (docs/stream-session-history.md §1). A failure here
// must never affect streaming itself (§10 of the contract) - every
// tick error is logged and swallowed, never propagated into a panic
// or a call into any branch-control method.
type Manager struct {
	repo      Repository
	branches  BranchSnapshotter
	ingest    IngestSnapshotter
	platforms PlatformLookup
	logger    *slog.Logger
	now       Clock

	newSessionID     func() (string, error)
	newDestinationID func() (string, error)

	pollInterval  time.Duration
	graceWindow   time.Duration
	pruneInterval time.Duration

	mu                         sync.Mutex
	openSession                *Session
	openDestinationsByPlatform map[string]*Destination
	lastPruneAt                time.Time

	stop chan struct{}
	done chan struct{}
}

// Option customises a Manager, mainly for tests.
type Option func(*Manager)

// WithClock overrides the time source.
func WithClock(clock Clock) Option {
	return func(m *Manager) { m.now = clock }
}

// WithPollInterval overrides the poll cadence.
func WithPollInterval(d time.Duration) Option {
	return func(m *Manager) { m.pollInterval = d }
}

// WithGraceWindow overrides the reconnect grace window.
func WithGraceWindow(d time.Duration) Option {
	return func(m *Manager) { m.graceWindow = d }
}

// WithPruneInterval overrides the retention-sweep cadence.
func WithPruneInterval(d time.Duration) Option {
	return func(m *Manager) { m.pruneInterval = d }
}

// NewManager builds a Manager. Start must be called once to begin
// polling; Shutdown stops it.
func NewManager(repo Repository, branches BranchSnapshotter, ingest IngestSnapshotter, platforms PlatformLookup, logger *slog.Logger, opts ...Option) *Manager {
	m := &Manager{
		repo: repo, branches: branches, ingest: ingest, platforms: platforms, logger: logger,
		now:              time.Now,
		newSessionID:     NewSessionID,
		newDestinationID: NewDestinationID,
		pollInterval:     DefaultPollInterval,
		graceWindow:      DefaultGraceWindow,
		pruneInterval:    DefaultPruneInterval,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Start performs unclean-shutdown recovery (docs/stream-session-
// history.md §2) and then begins the poll loop.
func (m *Manager) Start(ctx context.Context) error {
	if err := m.recoverOpenSession(ctx); err != nil {
		return fmt.Errorf("recover open stream session: %w", err)
	}

	m.stop = make(chan struct{})
	m.done = make(chan struct{})
	go m.loop(ctx)
	return nil
}

// Shutdown stops the poll loop. A session left open at this point is
// left exactly as it is (EndedAt still nil) - a graceful shutdown
// while a session is genuinely still active is not meaningfully
// different from a crash from this feature's own point of view, and
// is recovered the same way on next Start (docs/stream-session-
// history.md §2).
func (m *Manager) Shutdown(ctx context.Context) error {
	if m.stop == nil {
		return nil
	}
	close(m.stop)
	select {
	case <-m.done:
	case <-ctx.Done():
	}
	return nil
}

func (m *Manager) recoverOpenSession(ctx context.Context) error {
	open, found, err := m.repo.OpenSession(ctx)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}

	now := m.now()
	endedAt := open.LastSeenAt

	openDests, err := m.repo.OpenDestinations(ctx, open.ID)
	if err != nil {
		return err
	}
	for _, d := range openDests {
		d.EndedAt = &endedAt
		d.Outcome = OutcomeSessionEnded
		d.UpdatedAt = now
		if err := m.repo.UpdateDestination(ctx, d); err != nil {
			return err
		}
	}

	open.EndedAt = &endedAt
	open.EndReason = EndReasonUncleanShutdown
	open.UpdatedAt = now
	if err := m.repo.UpdateSession(ctx, open); err != nil {
		return err
	}
	if m.logger != nil {
		m.logger.Info("recovered a stream session left open by a previous process",
			slog.String("sessionId", open.ID), slog.Time("endedAt", endedAt))
	}
	return nil
}

func (m *Manager) loop(ctx context.Context) {
	defer close(m.done)
	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.stop:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := m.tick(ctx); err != nil && m.logger != nil {
				m.logger.Error("stream session poll tick failed", slog.Any("error", err))
			}
		}
	}
}

// tick reads real ingest/branch state and advances the session/
// destination state machine by exactly one step (docs/stream-session-
// history.md §1/§3).
func (m *Manager) tick(ctx context.Context) error {
	now := m.now()
	ingestSnap := m.ingest.Snapshot()
	receiving := ingestSnap.Ingest.State == mediamtx.IngestReceiving

	branches, err := m.branches.Snapshot(ctx)
	if err != nil {
		return fmt.Errorf("snapshot branches: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if receiving {
		if m.openSession == nil {
			if err := m.openSessionLocked(ctx, now); err != nil {
				return err
			}
		} else {
			m.openSession.LastSeenAt = now
			m.openSession.UpdatedAt = now
			if err := m.repo.UpdateSession(ctx, *m.openSession); err != nil {
				return err
			}
		}
		if err := m.reconcileDestinationsLocked(ctx, branches, now); err != nil {
			return err
		}
	} else if m.openSession != nil && now.Sub(m.openSession.LastSeenAt) >= m.graceWindow {
		if err := m.closeSessionLocked(ctx, now, EndReasonIngestStopped); err != nil {
			return err
		}
	}

	return m.pruneIfDueLocked(ctx, now)
}

// pruneIfDueLocked runs the retention sweep (docs/stream-session-
// history.md §6) at most once per pruneInterval, reusing this same
// poll-loop timer rather than a separate scheduled job.
func (m *Manager) pruneIfDueLocked(ctx context.Context, now time.Time) error {
	if !m.lastPruneAt.IsZero() && now.Sub(m.lastPruneAt) < m.pruneInterval {
		return nil
	}
	m.lastPruneAt = now

	days, found, err := m.repo.GetRetentionDays(ctx)
	if err != nil {
		return fmt.Errorf("get retention days: %w", err)
	}
	if !found {
		days = DefaultRetentionDays
	}
	cutoff := now.AddDate(0, 0, -days)
	n, err := m.repo.PruneSessionsBefore(ctx, cutoff)
	if err != nil {
		return fmt.Errorf("prune sessions before %v: %w", cutoff, err)
	}
	if n > 0 && m.logger != nil {
		m.logger.Info("pruned expired stream session history", slog.Int("count", n), slog.Time("cutoff", cutoff))
	}
	return nil
}

func (m *Manager) openSessionLocked(ctx context.Context, now time.Time) error {
	id, err := m.newSessionID()
	if err != nil {
		return err
	}
	s := Session{ID: id, StartedAt: now, LastSeenAt: now, CreatedAt: now, UpdatedAt: now}
	if err := m.repo.CreateSession(ctx, s); err != nil {
		return err
	}
	m.openSession = &s
	m.openDestinationsByPlatform = map[string]*Destination{}
	return nil
}

func (m *Manager) closeSessionLocked(ctx context.Context, now time.Time, reason EndReason) error {
	// The session's own end time is the last real moment ingest was
	// receiving, never `now` (the moment the grace window happened to
	// expire) - docs/stream-session-history.md §1.
	endedAt := m.openSession.LastSeenAt

	for _, dest := range m.openDestinationsByPlatform {
		dest.EndedAt = &endedAt
		dest.Outcome = OutcomeSessionEnded
		dest.UpdatedAt = now
		if err := m.repo.UpdateDestination(ctx, *dest); err != nil {
			return err
		}
	}

	m.openSession.EndedAt = &endedAt
	m.openSession.EndReason = reason
	m.openSession.UpdatedAt = now
	if err := m.repo.UpdateSession(ctx, *m.openSession); err != nil {
		return err
	}

	m.openSession = nil
	m.openDestinationsByPlatform = nil
	return nil
}

func (m *Manager) reconcileDestinationsLocked(ctx context.Context, branches []branch.Snapshot, now time.Time) error {
	liveByPlatform := make(map[string]branch.Snapshot, len(branches))
	errorByPlatform := make(map[string]bool, len(branches))
	for _, b := range branches {
		if b.State == branch.StateLive {
			liveByPlatform[b.PlatformID] = b
		}
		if b.State == branch.StateError {
			errorByPlatform[b.PlatformID] = true
		}
	}

	// Close participation for any destination that stopped being live.
	for platformID, dest := range m.openDestinationsByPlatform {
		if _, stillLive := liveByPlatform[platformID]; stillLive {
			continue
		}
		outcome := OutcomeCompleted
		if errorByPlatform[platformID] {
			outcome = OutcomeError
		}
		dest.EndedAt = &now
		dest.Outcome = outcome
		dest.UpdatedAt = now
		if err := m.repo.UpdateDestination(ctx, *dest); err != nil {
			return err
		}
		delete(m.openDestinationsByPlatform, platformID)
	}

	// Open participation for any destination that just became live.
	for platformID := range liveByPlatform {
		if _, alreadyOpen := m.openDestinationsByPlatform[platformID]; alreadyOpen {
			continue
		}
		p, err := m.platforms.Get(ctx, platformID)
		if err != nil {
			// The platform vanished between the branch snapshot and
			// this lookup (a genuine race with a real delete) - skip
			// it this tick rather than fail the whole tick; it will
			// either resolve or stop appearing as live next tick.
			if m.logger != nil {
				m.logger.Warn("could not resolve a live destination's platform for session history",
					slog.String("platformId", platformID), slog.Any("error", err))
			}
			continue
		}
		id, err := m.newDestinationID()
		if err != nil {
			return err
		}
		pid := platformID
		d := Destination{
			ID: id, SessionID: m.openSession.ID, PlatformID: &pid,
			ProviderID: string(p.ProviderID), DisplayName: p.DisplayName,
			StartedAt: now, CreatedAt: now, UpdatedAt: now,
		}
		if err := m.repo.CreateDestination(ctx, d); err != nil {
			return err
		}
		m.openDestinationsByPlatform[platformID] = &d
	}
	return nil
}
