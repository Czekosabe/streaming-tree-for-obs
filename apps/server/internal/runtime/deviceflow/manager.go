package deviceflow

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/streaming-tree/server/internal/domain/account"
)

// ErrConflict means a Twitch device-flow attempt is already active; only
// one may run at a time (see Manager's doc comment).
var ErrConflict = errors.New("a device authorization attempt is already in progress")

// ErrNotFound means no attempt exists with the given ID.
var ErrNotFound = errors.New("device authorization attempt not found")

// maxAttemptLifetime is a safety cap independent of whatever expiry a
// provider reports, so a misbehaving provider response can never leave a
// polling goroutine running forever.
const maxAttemptLifetime = 30 * time.Minute

// retentionAfterTerminal is how long a finished attempt's snapshot stays
// readable before this Manager forgets it, so a client that was mid-poll
// when the attempt finished still sees the final state at least once, while
// memory does not grow without bound across a long backend session.
const retentionAfterTerminal = 5 * time.Minute

// slowDownIncrement is added to the polling interval every time Twitch
// answers slow_down, per RFC 8628's device-flow guidance (Twitch's own
// documentation does not specify a increment, so this follows the
// generally-recommended device-flow practice of a fixed backoff step).
const slowDownIncrement = 5 * time.Second

type attempt struct {
	mu       sync.Mutex
	snapshot Snapshot
	cancel   context.CancelFunc
}

// Manager orchestrates device-authorization attempts.
//
// Exactly one attempt per provider may be active (requesting_code through
// polling) at a time: starting a second one while the first has not reached
// a terminal state is a conflict, not a queued or parallel attempt. Every
// attempt is bounded (maxAttemptLifetime), pollable only at the interval
// the provider returned (never faster, and slower after slow_down), and
// cancellable - cancellation stops polling immediately rather than letting
// an in-flight request finish and then discarding its result.
type Manager struct {
	accounts       *account.Service
	providers      map[account.ProviderID]account.DeviceFlowProvider
	requiredScopes map[account.ProviderID][]string
	logger         *slog.Logger

	newAttemptID func() (string, error)
	now          func() time.Time

	mu               sync.Mutex
	attempts         map[string]*attempt
	activeByProvider map[account.ProviderID]string

	lifecycle context.Context
	cancelAll context.CancelFunc
	workers   sync.WaitGroup
}

// Options constructs a Manager.
type Options struct {
	Accounts       *account.Service
	Providers      map[account.ProviderID]account.DeviceFlowProvider
	RequiredScopes map[account.ProviderID][]string
	Logger         *slog.Logger
}

// NewManager builds a Manager. Call Start before Start-ing any attempt.
func NewManager(opts Options) *Manager {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		accounts:         opts.Accounts,
		providers:        opts.Providers,
		requiredScopes:   opts.RequiredScopes,
		logger:           logger,
		newAttemptID:     newAttemptID,
		now:              func() time.Time { return time.Now().UTC() },
		attempts:         make(map[string]*attempt),
		activeByProvider: make(map[account.ProviderID]string),
	}
}

func newAttemptID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate attempt id: %w", err)
	}
	return "devflow_" + hex.EncodeToString(buf), nil
}

// Start begins the background lifecycle. Must be called once before any
// attempt is started.
func (m *Manager) Start(ctx context.Context) {
	m.lifecycle, m.cancelAll = context.WithCancel(context.Background())
	_ = ctx // parent context is not retained; Shutdown provides an explicit stop
}

// Shutdown cancels every active attempt and waits for its worker to exit.
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

// StartAttempt begins a new device-flow attempt for a provider, requesting
// the Manager's own configured scope set for that provider (RequiredScopes).
//
// reconnectAccountID, when non-empty, means this attempt must resolve to
// exactly that existing account - see account.Service.FinalizeConnection.
func (m *Manager) StartAttempt(ctx context.Context, providerID account.ProviderID, reconnectAccountID string) (Snapshot, error) {
	return m.startAttempt(ctx, providerID, reconnectAccountID, m.requiredScopes[providerID])
}

// StartAttemptWithScopes begins a new device-flow attempt requesting an
// explicit scope set instead of the Manager's configured default for that
// provider.
//
// Used for a capability permission upgrade on an existing account (see
// internal/runtime/twitchengagement), where scopes must be the union of the
// account's currently granted scopes and a capability-specific profile -
// never the Manager's one fixed metadata-only default, and never a smaller
// set that could look like a downgrade. reconnectAccountID should always be
// set for an upgrade (identity-bound), exactly like StartAttempt's own
// reconnect case.
func (m *Manager) StartAttemptWithScopes(ctx context.Context, providerID account.ProviderID, reconnectAccountID string, scopes []string) (Snapshot, error) {
	return m.startAttempt(ctx, providerID, reconnectAccountID, scopes)
}

func (m *Manager) startAttempt(ctx context.Context, providerID account.ProviderID, reconnectAccountID string, scopes []string) (Snapshot, error) {
	provider, ok := m.providers[providerID]
	if !ok {
		return Snapshot{}, fmt.Errorf("%w: no provider adapter for %q", ErrNotFound, providerID)
	}

	m.mu.Lock()
	if existingID, active := m.activeByProvider[providerID]; active {
		m.mu.Unlock()
		return Snapshot{}, fmt.Errorf("%w: attempt %s is already active", ErrConflict, existingID)
	}
	m.mu.Unlock()

	clientID, err := m.accounts.IntegrationConfig(ctx, providerID)
	if err != nil {
		return Snapshot{}, err
	}
	if !clientID.Configured {
		return Snapshot{}, account.ErrIntegrationNotConfigured
	}

	attemptID, err := m.newAttemptID()
	if err != nil {
		return Snapshot{}, err
	}

	now := m.now()
	a := &attempt{snapshot: Snapshot{
		AttemptID: attemptID, ProviderID: providerID, State: StateRequestingCode, CreatedAt: now,
	}}

	m.mu.Lock()
	m.attempts[attemptID] = a
	m.activeByProvider[providerID] = attemptID
	m.mu.Unlock()

	start, err := provider.StartDeviceFlow(ctx, clientID.ClientID, scopes)
	if err != nil {
		m.finishWithError(a, providerID, "device_flow_start_failed", "Could not start the authorization attempt.")
		return a.snapshotCopy(), err
	}

	a.mu.Lock()
	a.snapshot.State = StateWaitingForUser
	a.snapshot.UserCode = start.UserCode
	a.snapshot.VerificationURI = start.VerificationURI
	a.snapshot.ExpiresAt = now.Add(minDuration(start.ExpiresIn, maxAttemptLifetime))
	a.snapshot.Interval = start.Interval
	snapshot := a.snapshot
	a.mu.Unlock()

	attemptCtx, cancel := context.WithDeadline(m.lifecycle, snapshot.ExpiresAt)
	a.mu.Lock()
	a.cancel = cancel
	a.mu.Unlock()

	m.workers.Add(1)
	go func() {
		defer m.workers.Done()
		m.pollLoop(attemptCtx, a, provider, clientID.ClientID, start.DeviceCode, reconnectAccountID)
	}()

	return snapshot, nil
}

func minDuration(a, b time.Duration) time.Duration {
	if a <= 0 || a > b {
		return b
	}
	return a
}

func (a *attempt) snapshotCopy() Snapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.snapshot
}

func (m *Manager) finishWithError(a *attempt, providerID account.ProviderID, code, message string) {
	a.mu.Lock()
	a.snapshot.State = StateError
	a.snapshot.ErrorCode = code
	a.snapshot.ErrorMessage = message
	a.mu.Unlock()
	m.clearActiveAndSchedule(providerID, a.snapshot.AttemptID)
}

// clearActiveAndSchedule frees the provider's "active attempt" slot
// immediately (so a new attempt can start right away) and schedules the
// finished attempt's removal from the readable map after
// retentionAfterTerminal.
func (m *Manager) clearActiveAndSchedule(providerID account.ProviderID, attemptID string) {
	m.mu.Lock()
	if m.activeByProvider[providerID] == attemptID {
		delete(m.activeByProvider, providerID)
	}
	m.mu.Unlock()

	// Deliberately NOT tracked by m.workers and NOT tied to m.lifecycle:
	// this timer's only job is to let a finished attempt's snapshot stay
	// readable for a while. Using m.lifecycle here would make it fire
	// immediately on Shutdown - the same signal that cancels active
	// polling - deleting every just-finished attempt before Shutdown even
	// returns, rather than actually retaining it. The process either keeps
	// running (the timer fires normally in a few minutes) or exits
	// entirely (the goroutine dies with it) - both are fine outcomes for a
	// bare cleanup timer with no external resource to release.
	go func() {
		time.Sleep(retentionAfterTerminal)
		m.mu.Lock()
		delete(m.attempts, attemptID)
		m.mu.Unlock()
	}()
}

func (m *Manager) pollLoop(ctx context.Context, a *attempt, provider account.DeviceFlowProvider, clientID, deviceCode, reconnectAccountID string) {
	a.mu.Lock()
	a.snapshot.State = StatePolling
	providerID := a.snapshot.ProviderID
	interval := a.snapshot.Interval
	a.mu.Unlock()

	if interval <= 0 {
		interval = 5 * time.Second
	}

	for {
		select {
		case <-ctx.Done():
			m.finalizeNonAuthorized(a, providerID, ctx.Err())
			return
		case <-time.After(interval):
		}

		outcome, err := provider.PollDeviceFlow(ctx, clientID, deviceCode)
		if err != nil {
			if ctx.Err() != nil {
				m.finalizeNonAuthorized(a, providerID, ctx.Err())
				return
			}
			m.finishWithError(a, providerID, "device_flow_poll_failed", "The authorization attempt could not be completed.")
			return
		}

		switch outcome.Status {
		case account.PollPending:
			continue
		case account.PollSlowDown:
			interval += slowDownIncrement
			continue
		case account.PollDenied:
			m.setTerminal(a, providerID, StateDenied)
			return
		case account.PollExpired:
			m.setTerminal(a, providerID, StateExpired)
			return
		case account.PollComplete:
			m.finalizeAuthorized(ctx, a, providerID, provider, clientID, outcome, reconnectAccountID)
			return
		default:
			m.finishWithError(a, providerID, "device_flow_poll_failed", "The authorization attempt returned an unrecognized result.")
			return
		}
	}
}

func (m *Manager) finalizeNonAuthorized(a *attempt, providerID account.ProviderID, ctxErr error) {
	if errors.Is(ctxErr, context.Canceled) {
		// Distinguish an explicit Cancel (see CancelAttempt) from the
		// deadline expiring: CancelAttempt already sets StateCancelled
		// itself before cancelling the context, so if the state is still
		// non-terminal here, the deadline is what fired.
		a.mu.Lock()
		alreadyTerminal := a.snapshot.State.terminal()
		a.mu.Unlock()
		if alreadyTerminal {
			return
		}
	}
	m.setTerminal(a, providerID, StateExpired)
}

func (m *Manager) setTerminal(a *attempt, providerID account.ProviderID, state State) {
	a.mu.Lock()
	if a.snapshot.State.terminal() {
		a.mu.Unlock()
		return
	}
	a.snapshot.State = state
	attemptID := a.snapshot.AttemptID
	a.mu.Unlock()
	m.clearActiveAndSchedule(providerID, attemptID)
}

func (m *Manager) finalizeAuthorized(
	ctx context.Context, a *attempt, providerID account.ProviderID, provider account.DeviceFlowProvider,
	clientID string, outcome account.PollOutcome, reconnectAccountID string,
) {
	identity, err := provider.GetIdentity(ctx, outcome.Bundle.AccessToken, clientID)
	if err != nil {
		m.finishWithError(a, providerID, "device_flow_identity_failed", "The authorized account's identity could not be resolved.")
		return
	}

	acc, err := m.accounts.FinalizeConnection(ctx, providerID, identity, outcome.Bundle, outcome.Scopes, reconnectAccountID)
	if err != nil {
		code := "device_flow_finalize_failed"
		message := "The authorization could not be completed."
		if errors.Is(err, account.ErrIdentityMismatch) {
			code = "oauth_identity_mismatch"
			message = "The authorized account does not match the account being reconnected."
		} else if errors.Is(err, account.ErrMissingScope) {
			code = "oauth_scope_missing"
			message = "The authorization did not grant every required permission."
		}
		m.finishWithError(a, providerID, code, message)
		return
	}

	a.mu.Lock()
	a.snapshot.State = StateAuthorized
	a.snapshot.ConnectedAccountID = acc.ID
	a.mu.Unlock()
	m.clearActiveAndSchedule(providerID, a.snapshot.AttemptID)
}

// GetAttempt returns the current snapshot of one attempt.
func (m *Manager) GetAttempt(attemptID string) (Snapshot, error) {
	m.mu.Lock()
	a, ok := m.attempts[attemptID]
	m.mu.Unlock()
	if !ok {
		return Snapshot{}, ErrNotFound
	}
	return a.snapshotCopy(), nil
}

// CancelAttempt stops an active attempt's polling immediately.
//
// Cancelling an already-terminal attempt is a no-op success, not an error -
// matching this project's other idempotent-cancel/delete conventions.
func (m *Manager) CancelAttempt(attemptID string) (Snapshot, error) {
	m.mu.Lock()
	a, ok := m.attempts[attemptID]
	m.mu.Unlock()
	if !ok {
		return Snapshot{}, ErrNotFound
	}

	a.mu.Lock()
	if a.snapshot.State.terminal() {
		snapshot := a.snapshot
		a.mu.Unlock()
		return snapshot, nil
	}
	a.snapshot.State = StateCancelled
	providerID := a.snapshot.ProviderID
	attemptCancel := a.cancel
	snapshot := a.snapshot
	a.mu.Unlock()

	if attemptCancel != nil {
		attemptCancel()
	}
	m.clearActiveAndSchedule(providerID, attemptID)

	return snapshot, nil
}
