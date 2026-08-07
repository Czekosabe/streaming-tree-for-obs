package outboundchat

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/streaming-tree/server/internal/domain/account"
)

// AccountAccessor is the narrow subset of account.Service the dispatcher
// needs: resolving an account record, running one provider call under the
// existing single-flight-refresh-then-retry-once-on-401 semantics, and
// resolving a provider's currently-effective Client ID. Reusing
// WithFreshToken here, unchanged, is what gives outbound chat its "exactly
// one refresh/retry on a clear 401" behavior for free.
type AccountAccessor interface {
	GetAccount(ctx context.Context, id string) (account.Account, error)
	WithFreshToken(ctx context.Context, accountID string, fn func(accessToken string) (unauthorized bool, err error)) error
	EffectiveClientID(ctx context.Context, providerID account.ProviderID) (string, error)
}

// DispatcherState is one account dispatcher's own runtime activity state -
// distinct from Capability, which is about permission, not activity.
type DispatcherState string

const (
	DispatcherIdle        DispatcherState = "idle"
	DispatcherQueued      DispatcherState = "queued"
	DispatcherSending     DispatcherState = "sending"
	DispatcherRateLimited DispatcherState = "rate_limited"
	DispatcherStopping    DispatcherState = "stopping"
	DispatcherError       DispatcherState = "error"
)

const (
	// MaxQueueDepth bounds how many pending sends one account's dispatcher
	// ever retains at once - beyond this, Enqueue/Send returns ErrQueueFull
	// immediately rather than growing memory without bound.
	MaxQueueDepth = 20

	// Conservative local rate-limiting ceilings - safety ceilings, not a
	// claim about any account's real Twitch privilege class. See
	// docs/provider-integrations/twitch-outbound-chat.md's own reasoning.
	minDispatchInterval = 1 * time.Second
	rollingWindow       = 30 * time.Second
	maxStartsInWindow   = 20

	// providerCallTimeout bounds one dequeued send's own provider call,
	// independent of the original caller's context - see accountDispatcher
	// process's own doc comment on why a started send is never abandoned
	// just because the original HTTP request's context ended.
	providerCallTimeout = 20 * time.Second
)

// Snapshot is one account's outbound-chat runtime state - in memory only,
// reset on every backend restart. Never carries message text, a token, a
// raw provider response, or process/environment data.
type Snapshot struct {
	AccountID string
	// ProviderSupported reports whether any Provider is registered for this
	// account's own provider - false means "unsupported", independent of
	// Capability, which is only meaningful when this is true.
	ProviderSupported bool
	Capability        Capability
	State             DispatcherState
	QueueDepth        int
	QueueCapacity     int
	LastAttemptAt     *time.Time
	LastSuccessAt     *time.Time
	LastErrorCode     string
	RetryAt           *time.Time
}

type queuedSend struct {
	ctx    context.Context
	req    SendMessageRequest
	result chan<- sendOutcome
}

type sendOutcome struct {
	result SendMessageResult
	err    error
}

// Manager holds one accountDispatcher per connected account that has ever
// sent, fanning outbound send requests out to whichever Provider is
// registered for that account's own provider ID. Entirely in-memory - a
// fresh Manager (and therefore fresh per-account runtime state) is
// constructed on every backend start; nothing here is a SQLite column.
type Manager struct {
	accounts  AccountAccessor
	providers map[account.ProviderID]Provider
	now       func() time.Time

	mu          sync.Mutex
	dispatchers map[string]*accountDispatcher
	closed      bool
	workers     sync.WaitGroup
}

// ManagerOptions constructs a Manager. Now is a test-only fake-clock
// override; production code leaves it nil so time.Now is used.
type ManagerOptions struct {
	Accounts  AccountAccessor
	Providers []Provider
	Now       func() time.Time
}

// NewManager builds a Manager. Registering zero providers is valid (every
// account then reports ProviderSupported: false) - callers register
// whichever outbound-capable adapters exist, exactly like
// internal/runtime/twitchengagement's own Manager is constructed with a
// concrete provider already wired in.
func NewManager(opts ManagerOptions) *Manager {
	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	providers := make(map[account.ProviderID]Provider, len(opts.Providers))
	for _, p := range opts.Providers {
		providers[p.ProviderID()] = p
	}
	return &Manager{
		accounts: opts.Accounts, providers: providers, now: now,
		dispatchers: make(map[string]*accountDispatcher),
	}
}

// Send validates req, queues it behind any already-pending sends for the
// same account, and blocks until either a trustworthy result is obtained,
// the queue is full, the account/provider is not ready to send, or ctx is
// cancelled while the job is still waiting to start (never after a send
// has actually begun - see accountDispatcher.process's own doc comment).
func (m *Manager) Send(ctx context.Context, req SendMessageRequest) (SendMessageResult, error) {
	if err := ValidateMessage(req.Message); err != nil {
		return SendMessageResult{}, err
	}
	if err := ValidateReplyParentMessageID(req.ReplyParentMessageID); err != nil {
		return SendMessageResult{}, err
	}

	acc, err := m.accounts.GetAccount(ctx, req.AccountID)
	if err != nil {
		return SendMessageResult{}, err
	}
	provider, ok := m.providers[acc.ProviderID]
	if !ok {
		return SendMessageResult{}, ErrUnsupportedProvider
	}
	if acc.Status == account.StatusReconnectRequired {
		return SendMessageResult{}, account.ErrReconnectRequired
	}
	if capability := provider.AssessCapability(acc); capability.PermissionUpgradeRequired {
		return SendMessageResult{}, ErrPermissionRequired
	}

	d, err := m.dispatcherFor(req.AccountID, provider)
	if err != nil {
		return SendMessageResult{}, err
	}
	return d.enqueueAndWait(ctx, req)
}

// dispatcherFor returns the existing dispatcher for accountID or creates
// and starts a new one - lazy, so an account that never sends never gets a
// goroutine at all.
func (m *Manager) dispatcherFor(accountID string, provider Provider) (*accountDispatcher, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, ErrCancelled
	}
	if d, ok := m.dispatchers[accountID]; ok {
		return d, nil
	}
	d := newAccountDispatcher(accountID, provider, m)
	m.dispatchers[accountID] = d
	m.workers.Add(1)
	go func() {
		defer m.workers.Done()
		d.run()
	}()
	return d, nil
}

// Status reports accountID's current outbound-chat capability and runtime
// state, merging a fresh capability assessment with the dispatcher's own
// cached runtime snapshot (empty/idle if the account has never sent).
func (m *Manager) Status(ctx context.Context, accountID string) (Snapshot, error) {
	acc, err := m.accounts.GetAccount(ctx, accountID)
	if err != nil {
		return Snapshot{}, err
	}

	snap := Snapshot{AccountID: accountID, State: DispatcherIdle, QueueCapacity: MaxQueueDepth}
	provider, ok := m.providers[acc.ProviderID]
	if ok {
		snap.ProviderSupported = true
		snap.Capability = provider.AssessCapability(acc)
	}

	m.mu.Lock()
	d, exists := m.dispatchers[accountID]
	m.mu.Unlock()
	if exists {
		runtime := d.snapshot()
		snap.State, snap.QueueDepth = runtime.State, runtime.QueueDepth
		snap.LastAttemptAt, snap.LastSuccessAt = runtime.LastAttemptAt, runtime.LastSuccessAt
		snap.LastErrorCode, snap.RetryAt = runtime.LastErrorCode, runtime.RetryAt
	}
	return snap, nil
}

// Shutdown cancels every account dispatcher and waits for their worker
// goroutines to exit, bounded by ctx - no goroutine leak across a backend
// restart.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	dispatchers := make([]*accountDispatcher, 0, len(m.dispatchers))
	for _, d := range m.dispatchers {
		dispatchers = append(dispatchers, d)
	}
	m.mu.Unlock()

	for _, d := range dispatchers {
		d.cancel()
	}

	done := make(chan struct{})
	go func() {
		m.workers.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// accountDispatcher serializes every send for one account through a single
// buffered queue and one worker goroutine, so accounts never contend with
// each other and sends within one account never reorder.
type accountDispatcher struct {
	accountID string
	provider  Provider
	manager   *Manager

	queue chan queuedSend

	mu            sync.Mutex
	state         DispatcherState
	lastAttemptAt *time.Time
	lastSuccessAt *time.Time
	lastErrorCode string
	retryAt       *time.Time
	recentStarts  []time.Time
	// providerNotBefore is set from a RateLimitedError's own RetryAt hint,
	// so the next queued send also honors the provider's own pacing, not
	// just this application's local floor/window - "respect a provider
	// 429/reset time".
	providerNotBefore time.Time

	ctx    context.Context
	cancel context.CancelFunc
}

func newAccountDispatcher(accountID string, provider Provider, m *Manager) *accountDispatcher {
	ctx, cancel := context.WithCancel(context.Background())
	return &accountDispatcher{
		accountID: accountID, provider: provider, manager: m,
		queue: make(chan queuedSend, MaxQueueDepth),
		state: DispatcherIdle, ctx: ctx, cancel: cancel,
	}
}

// enqueueAndWait queues req (returning ErrQueueFull immediately if the
// bounded queue is already full - never blocking here) and waits for its
// outcome, or for ctx to end while the job is still queued.
func (d *accountDispatcher) enqueueAndWait(ctx context.Context, req SendMessageRequest) (SendMessageResult, error) {
	resultCh := make(chan sendOutcome, 1)
	job := queuedSend{ctx: ctx, req: req, result: resultCh}

	select {
	case d.queue <- job:
	default:
		return SendMessageResult{}, ErrQueueFull
	}
	d.setState(DispatcherQueued)

	select {
	case outcome := <-resultCh:
		return outcome.result, outcome.err
	case <-ctx.Done():
		return SendMessageResult{}, ErrCancelled
	case <-d.ctx.Done():
		return SendMessageResult{}, ErrCancelled
	}
}

func (d *accountDispatcher) run() {
	for {
		select {
		case <-d.ctx.Done():
			d.setState(DispatcherStopping)
			return
		case job := <-d.queue:
			d.process(job)
		}
	}
}

// rateLimitPollInterval bounds how often process re-checks whether a rate-
// limit wait is over. Deliberately a poll rather than one big timer sized
// from the (possibly fake, test-controlled) clock's own idea of the wait
// duration: a real time.Timer's fire time is fixed at construction and
// cannot be pulled earlier by a test later advancing a fake clock, so a
// short, cheap, real poll is what actually lets tests drive this
// deterministically via Manager's Now override - see dispatcher_test.go.
const rateLimitPollInterval = 10 * time.Millisecond

// process sends one queued job. Once a send genuinely begins (the provider
// call is made), it is never abandoned due to the original caller's
// context ending - the call runs under its own decoupled, bounded timeout
// instead, because a request that may have already reached the provider
// must never be treated as if it simply never happened (see
// docs/provider-integrations/twitch-outbound-chat.md's retry policy).
func (d *accountDispatcher) process(job queuedSend) {
	for {
		waitUntil := d.nextAllowedStart()
		if waitUntil.IsZero() {
			break
		}
		d.setRateLimited(waitUntil)
		select {
		case <-time.After(rateLimitPollInterval):
		case <-d.ctx.Done():
			job.result <- sendOutcome{err: ErrCancelled}
			return
		}
	}

	d.setState(DispatcherSending)
	d.recordStart(d.manager.now())

	sendCtx, cancel := context.WithTimeout(context.WithoutCancel(job.ctx), providerCallTimeout)
	result, err := d.sendOnce(sendCtx, job.req)
	cancel()

	d.recordOutcome(result, err)
	job.result <- sendOutcome{result: result, err: err}
}

// sendOnce resolves the account and its effective Client ID, then makes
// exactly one provider call under account.Service.WithFreshToken's
// existing refresh-and-retry-once-on-401 policy.
func (d *accountDispatcher) sendOnce(ctx context.Context, req SendMessageRequest) (SendMessageResult, error) {
	acc, err := d.manager.accounts.GetAccount(ctx, req.AccountID)
	if err != nil {
		return SendMessageResult{}, err
	}
	if acc.Status == account.StatusReconnectRequired {
		return SendMessageResult{}, account.ErrReconnectRequired
	}
	clientID, err := d.manager.accounts.EffectiveClientID(ctx, acc.ProviderID)
	if err != nil {
		return SendMessageResult{}, err
	}

	var outcome sendOutcome
	err = d.manager.accounts.WithFreshToken(ctx, req.AccountID, func(accessToken string) (bool, error) {
		result, sendErr := d.provider.SendChatMessage(ctx, acc, account.TokenBundle{AccessToken: accessToken}, clientID, req)
		if sendErr != nil {
			if errors.Is(sendErr, ErrUnauthorized) {
				return true, nil
			}
			outcome = sendOutcome{err: sendErr}
			return false, nil
		}
		outcome = sendOutcome{result: result}
		return false, nil
	})
	if err != nil {
		return SendMessageResult{}, err
	}
	return outcome.result, outcome.err
}

// nextAllowedStart reports the earliest time a new send may begin under
// this account's own local rate-limit ceilings (a floor of
// minDispatchInterval since the last start, and at most maxStartsInWindow
// starts in any rollingWindow), or the zero time if a send may begin
// immediately.
func (d *accountDispatcher) nextAllowedStart() time.Time {
	d.mu.Lock()
	defer d.mu.Unlock()
	now := d.manager.now()

	cutoff := now.Add(-rollingWindow)
	kept := d.recentStarts[:0]
	for _, t := range d.recentStarts {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	d.recentStarts = kept

	var earliest time.Time
	if n := len(d.recentStarts); n > 0 {
		if candidate := d.recentStarts[n-1].Add(minDispatchInterval); candidate.After(now) {
			earliest = candidate
		}
	}
	if len(d.recentStarts) >= maxStartsInWindow {
		if candidate := d.recentStarts[0].Add(rollingWindow); candidate.After(earliest) {
			earliest = candidate
		}
	}
	if !d.providerNotBefore.IsZero() && d.providerNotBefore.After(earliest) {
		earliest = d.providerNotBefore
	}
	if earliest.IsZero() || !earliest.After(now) {
		return time.Time{}
	}
	return earliest
}

func (d *accountDispatcher) recordStart(t time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.recentStarts = append(d.recentStarts, t)
	if len(d.recentStarts) > maxStartsInWindow {
		d.recentStarts = d.recentStarts[len(d.recentStarts)-maxStartsInWindow:]
	}
}

func (d *accountDispatcher) setState(s DispatcherState) {
	d.mu.Lock()
	d.state = s
	d.mu.Unlock()
}

func (d *accountDispatcher) setRateLimited(retryAt time.Time) {
	d.mu.Lock()
	d.state = DispatcherRateLimited
	d.retryAt = &retryAt
	d.mu.Unlock()
}

func (d *accountDispatcher) recordOutcome(result SendMessageResult, err error) {
	now := d.manager.now()
	d.mu.Lock()
	defer d.mu.Unlock()
	d.lastAttemptAt = &now

	var rateLimitErr *RateLimitedError
	switch {
	case err == nil && result.Sent:
		d.lastSuccessAt = &now
		d.state = DispatcherIdle
		d.lastErrorCode = ""
		d.retryAt = nil
	case err == nil:
		d.state = DispatcherIdle
		d.lastErrorCode = result.Code
		d.retryAt = nil
	case errors.As(err, &rateLimitErr):
		d.state = DispatcherRateLimited
		d.lastErrorCode = "rate_limited"
		if !rateLimitErr.RetryAt.IsZero() {
			retryAt := rateLimitErr.RetryAt
			d.retryAt = &retryAt
			// Also pace the next queued send behind the provider's own
			// hint - "respect a provider 429/reset time" - not just report
			// it in the snapshot.
			if retryAt.After(d.providerNotBefore) {
				d.providerNotBefore = retryAt
			}
		} else {
			d.retryAt = nil
		}
	case errors.Is(err, ErrCancelled):
		d.state = DispatcherIdle
		d.lastErrorCode = "cancelled"
		d.retryAt = nil
	default:
		d.state = DispatcherError
		d.lastErrorCode = errorCode(err)
		d.retryAt = nil
	}
}

func (d *accountDispatcher) snapshot() Snapshot {
	d.mu.Lock()
	defer d.mu.Unlock()
	return Snapshot{
		AccountID: d.accountID, State: d.state, QueueDepth: len(d.queue), QueueCapacity: MaxQueueDepth,
		LastAttemptAt: d.lastAttemptAt, LastSuccessAt: d.lastSuccessAt,
		LastErrorCode: d.lastErrorCode, RetryAt: d.retryAt,
	}
}

// errorCode maps a Go error from a provider call to a short, stable,
// message-free identifier suitable for a runtime snapshot - never the
// error's own free-text Error() string, which could in principle embed
// provider response detail.
func errorCode(err error) string {
	switch {
	case errors.Is(err, ErrForbidden):
		return "forbidden"
	case errors.Is(err, ErrProviderFailure):
		return "provider_failure"
	case errors.Is(err, ErrDeliveryUnknown):
		return "delivery_unknown"
	case errors.Is(err, ErrUnauthorized):
		return "unauthorized"
	case errors.Is(err, account.ErrReconnectRequired):
		return "reconnect_required"
	case errors.Is(err, ErrUnsupportedProvider):
		return "unsupported_provider"
	case errors.Is(err, ErrPermissionRequired):
		return "permission_required"
	default:
		return "error"
	}
}
