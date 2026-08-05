package youtubeauth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/streaming-tree/server/internal/domain/account"
	"github.com/streaming-tree/server/internal/provider/youtube"
)

// ErrConflict means a YouTube OAuth attempt is already active; only one may
// run at a time.
var ErrConflict = errors.New("an authorization attempt is already in progress")

// ErrNotFound means no attempt exists with the given ID.
var ErrNotFound = errors.New("authorization attempt not found")

// ErrChannelSelectionNotPending means SelectChannel was called for an
// attempt that is not currently awaiting a channel selection.
var ErrChannelSelectionNotPending = errors.New("this attempt is not awaiting a channel selection")

// ErrInvalidChannelSelection means the selected channel ID was not among
// the channels this attempt actually fetched.
var ErrInvalidChannelSelection = errors.New("the selected channel was not offered by this attempt")

// maxAttemptLifetime is a safety cap on how long an attempt (and its
// loopback listener) may stay open.
const maxAttemptLifetime = 15 * time.Minute

// retentionAfterTerminal is how long a finished attempt's snapshot stays
// readable before this Manager forgets it - see deviceflow.Manager's own
// identical reasoning.
const retentionAfterTerminal = 5 * time.Minute

type attempt struct {
	mu       sync.Mutex
	snapshot Snapshot

	// Secret-bearing / internal-only fields - never exposed via Snapshot.
	pkceVerifier       string
	oauthState         string
	redirectURI        string
	reconnectAccountID string
	codeConsumed       bool
	pendingBundle      account.TokenBundle
	pendingScopes      []string
	pendingChannels    []youtube.Channel

	cancel    context.CancelFunc
	listener  net.Listener
	server    *http.Server
	closeOnce sync.Once
}

func (a *attempt) snapshotCopy() Snapshot {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.snapshot
}

// closeListener stops the loopback HTTP server. Idempotent and safe to call
// from multiple goroutines (a successful callback, an expiry timer, and an
// explicit cancellation may all race to close the same attempt).
func (a *attempt) closeListener() {
	a.closeOnce.Do(func() {
		if a.server != nil {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = a.server.Shutdown(shutdownCtx)
		}
		if a.listener != nil {
			_ = a.listener.Close()
		}
	})
}

// Manager orchestrates YouTube's Authorization Code + PKCE + loopback-
// callback attempts.
//
// Exactly one attempt may be active (creating through processing_callback
// or awaiting_channel_selection) at a time - starting a second one is a
// conflict, not a queued or parallel attempt. Every attempt is bounded
// (maxAttemptLifetime), cancellable, and its loopback listener is closed on
// success, failure, cancellation, expiration, or backend shutdown.
type Manager struct {
	accounts       *account.Service
	client         *youtube.Client
	requiredScopes []string
	logger         *slog.Logger

	newAttemptID func() (string, error)
	now          func() time.Time

	mu       sync.Mutex
	attempts map[string]*attempt
	activeID string

	lifecycle context.Context
	cancelAll context.CancelFunc
	workers   sync.WaitGroup
}

// Options constructs a Manager.
type Options struct {
	Accounts       *account.Service
	Client         *youtube.Client
	RequiredScopes []string
	Logger         *slog.Logger
}

// NewManager builds a Manager. Call Start before Start-ing any attempt.
func NewManager(opts Options) *Manager {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		accounts: opts.Accounts, client: opts.Client, requiredScopes: opts.RequiredScopes, logger: logger,
		newAttemptID: newAttemptID, now: func() time.Time { return time.Now().UTC() },
		attempts: make(map[string]*attempt),
	}
}

func newAttemptID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate attempt id: %w", err)
	}
	return "ytauth_" + hex.EncodeToString(buf), nil
}

// Start begins the background lifecycle. Must be called once before any
// attempt is started.
func (m *Manager) Start(ctx context.Context) {
	m.lifecycle, m.cancelAll = context.WithCancel(context.Background())
	_ = ctx
}

// Shutdown cancels every active attempt, closes its loopback listener, and
// waits for its worker to exit.
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

// StartAttempt begins a new YouTube OAuth attempt.
//
// reconnectAccountID, when non-empty, means this attempt must resolve to
// exactly that existing account - see account.Service.FinalizeConnection.
func (m *Manager) StartAttempt(ctx context.Context, reconnectAccountID string) (Snapshot, error) {
	m.mu.Lock()
	if m.activeID != "" {
		existing := m.activeID
		m.mu.Unlock()
		return Snapshot{}, fmt.Errorf("%w: attempt %s is already active", ErrConflict, existing)
	}
	m.mu.Unlock()

	clientID, err := m.accounts.IntegrationConfig(ctx, account.ProviderYouTube)
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
	verifier, err := youtube.GeneratePKCEVerifier()
	if err != nil {
		return Snapshot{}, err
	}
	state, err := youtube.GenerateState()
	if err != nil {
		return Snapshot{}, err
	}

	// Loopback IP, dynamic OS-assigned port - never 0.0.0.0, never a
	// user-supplied address (docs/provider-integrations/youtube.md).
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return Snapshot{}, fmt.Errorf("bind loopback callback listener: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	authURL := m.client.BuildAuthorizationURL(youtube.AuthorizationURLInput{
		ClientID: clientID.ClientID, RedirectURI: redirectURI, Scopes: m.requiredScopes,
		State: state, CodeChallenge: youtube.DeriveS256Challenge(verifier),
	})

	now := m.now()
	expiresAt := now.Add(maxAttemptLifetime)

	a := &attempt{
		snapshot: Snapshot{
			AttemptID: attemptID, ProviderID: string(account.ProviderYouTube), State: StateWaitingForBrowser,
			AuthorizationURL: authURL, CreatedAt: now, ExpiresAt: expiresAt,
		},
		pkceVerifier: verifier, oauthState: state, redirectURI: redirectURI,
		reconnectAccountID: reconnectAccountID, listener: listener,
	}

	m.mu.Lock()
	m.attempts[attemptID] = a
	m.activeID = attemptID
	m.mu.Unlock()

	attemptCtx, cancel := context.WithDeadline(m.lifecycle, expiresAt)
	a.cancel = cancel

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", m.handleCallback(attemptCtx, a))
	// Deliberately no logging middleware of any kind: the task requires
	// this listener never log a callback query string, and the simplest
	// way to guarantee that is to never wrap it with any general-purpose
	// access logger in the first place - see docs/provider-integrations/
	// youtube.md's "Loopback callback design" section.
	a.server = &http.Server{Handler: mux}

	m.workers.Add(1)
	go func() {
		defer m.workers.Done()
		_ = a.server.Serve(listener)
	}()

	m.workers.Add(1)
	go func() {
		defer m.workers.Done()
		<-attemptCtx.Done()
		a.closeListener()
		m.finalizeNonTerminal(a, attemptCtx.Err())
	}()

	return a.snapshotCopy(), nil
}

func (m *Manager) finalizeNonTerminal(a *attempt, ctxErr error) {
	if errors.Is(ctxErr, context.Canceled) {
		a.mu.Lock()
		alreadyTerminal := a.snapshot.State.terminal()
		a.mu.Unlock()
		if alreadyTerminal {
			return
		}
	}
	m.setTerminal(a, StateExpired, "", "")
}

func (m *Manager) setTerminal(a *attempt, state State, errorCode, errorMessage string) {
	a.mu.Lock()
	if a.snapshot.State.terminal() {
		a.mu.Unlock()
		return
	}
	a.snapshot.State = state
	a.snapshot.AuthorizationURL = "" // ephemeral; not retained past the waiting stage
	a.snapshot.ErrorCode = errorCode
	a.snapshot.ErrorMessage = errorMessage
	attemptID := a.snapshot.AttemptID
	a.mu.Unlock()
	a.closeListener()
	m.clearActiveAndSchedule(attemptID)
}

// clearActiveAndSchedule frees the "active attempt" slot immediately and
// schedules the finished attempt's removal from the readable map after
// retentionAfterTerminal - deliberately an untracked, lifecycle-independent
// timer, for exactly the reason documented on deviceflow.Manager.
// clearActiveAndSchedule's own identical comment (see manager.go there):
// using m.lifecycle here would delete every just-finished attempt
// immediately on Shutdown instead of actually retaining it.
func (m *Manager) clearActiveAndSchedule(attemptID string) {
	m.mu.Lock()
	if m.activeID == attemptID {
		m.activeID = ""
	}
	m.mu.Unlock()

	go func() {
		time.Sleep(retentionAfterTerminal)
		m.mu.Lock()
		delete(m.attempts, attemptID)
		m.mu.Unlock()
	}()
}

// callbackPage is a static HTML page with no interpolation of any request
// data - the task's requirement that the callback response never echo an
// authorization code, token, state, PKCE verifier, or raw provider error.
const callbackPage = `<!doctype html><html><head><meta charset="utf-8"><title>Streaming Tree</title></head>` +
	`<body style="font-family:sans-serif;text-align:center;padding:3rem"><p>Authorization received. You may return to Streaming Tree.</p></body></html>`

func writeCallbackPage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(callbackPage))
}

// handleCallback processes Google's redirect back to the loopback listener.
//
// Never logs r.URL.String() or r.URL.RawQuery anywhere, on any path - see
// the task's "never log callback query parameters" requirement.
func (m *Manager) handleCallback(ctx context.Context, a *attempt) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()

		a.mu.Lock()
		if a.codeConsumed || a.snapshot.State.terminal() {
			a.mu.Unlock()
			writeCallbackPage(w)
			return
		}

		// Constant-time comparison, per the task's explicit requirement.
		providedState := query.Get("state")
		match := subtle.ConstantTimeCompare([]byte(providedState), []byte(a.oauthState)) == 1
		if !match {
			a.mu.Unlock()
			// A state mismatch may be a stray or hostile request to this
			// loopback port; it must not be able to deny a legitimate
			// concurrent attempt, so the attempt's own state is left
			// untouched and polling for a correct callback continues.
			writeCallbackPage(w)
			return
		}

		if errCode := query.Get("error"); errCode != "" {
			a.codeConsumed = true
			a.mu.Unlock()
			writeCallbackPage(w)
			// setTerminal closes the loopback server via a graceful
			// Shutdown(), which blocks until this very request's
			// connection goes idle - it cannot go idle until this handler
			// returns, so the transition must happen in a separate
			// goroutine rather than synchronously here, or Shutdown()
			// would deadlock against its own caller until it times out.
			m.workers.Add(1)
			go func() {
				defer m.workers.Done()
				m.setTerminal(a, StateDenied, "youtube_oauth_access_denied", "Authorization was denied.")
			}()
			return
		}

		code := query.Get("code")
		if code == "" {
			a.mu.Unlock()
			writeCallbackPage(w)
			return
		}

		a.codeConsumed = true
		a.snapshot.State = StateProcessingCallback
		a.mu.Unlock()

		writeCallbackPage(w)

		m.workers.Add(1)
		go func() {
			defer m.workers.Done()
			m.finishAuthorization(ctx, a, code)
		}()
	}
}

func (m *Manager) finishAuthorization(ctx context.Context, a *attempt, code string) {
	defer a.closeListener()

	clientID, err := m.accounts.EffectiveClientID(ctx, account.ProviderYouTube)
	if err != nil {
		m.setTerminal(a, StateError, "youtube_oauth_callback_failed", "The authorization attempt could not be completed.")
		return
	}

	bundle, scopes, err := m.client.ExchangeCode(ctx, clientID, code, a.pkceVerifier, a.redirectURI)
	if err != nil {
		m.setTerminal(a, StateError, "youtube_oauth_callback_failed", "The authorization attempt could not be completed.")
		return
	}

	for _, required := range m.requiredScopes {
		if !containsScope(scopes, required) {
			m.setTerminal(a, StateError, "youtube_scope_missing", "The authorization did not grant every required permission.")
			return
		}
	}

	accountBundle := account.TokenBundle{
		TokenType: bundle.TokenType, AccessToken: bundle.AccessToken,
		RefreshToken: bundle.RefreshToken, ExpiresAt: m.now().Add(bundle.ExpiresIn),
	}

	channels, err := m.client.ListMyChannels(ctx, bundle.AccessToken)
	if err != nil {
		m.setTerminal(a, StateError, "youtube_channel_not_found", "The authorized channels could not be retrieved.")
		return
	}
	if len(channels) == 0 {
		m.setTerminal(a, StateError, "youtube_channel_not_found", "No YouTube channel was found for this Google account.")
		return
	}
	if len(channels) == 1 {
		m.finalizeChannel(ctx, a, channels[0], accountBundle, scopes)
		return
	}

	summaries := make([]ChannelSummary, 0, len(channels))
	for _, ch := range channels {
		summaries = append(summaries, ChannelSummary{ChannelID: ch.ID, Title: ch.Title, ThumbnailURL: ch.ThumbnailURL})
	}

	a.mu.Lock()
	a.pendingBundle = accountBundle
	a.pendingScopes = scopes
	a.pendingChannels = channels
	a.snapshot.State = StateAwaitingChannelSelect
	a.snapshot.Channels = summaries
	a.mu.Unlock()
}

func (m *Manager) finalizeChannel(ctx context.Context, a *attempt, ch youtube.Channel, bundle account.TokenBundle, scopes []string) {
	identity := account.Identity{ProviderUserID: ch.ID, Login: ch.Title, DisplayName: ch.Title, AvatarURL: ch.ThumbnailURL}

	a.mu.Lock()
	reconnectAccountID := a.reconnectAccountID
	a.mu.Unlock()

	acc, err := m.accounts.FinalizeConnection(ctx, account.ProviderYouTube, identity, bundle, scopes, reconnectAccountID)
	if err != nil {
		code, message := "youtube_oauth_callback_failed", "The authorization could not be completed."
		if errors.Is(err, account.ErrIdentityMismatch) {
			code, message = "youtube_channel_identity_mismatch", "The authorized channel does not match the account being reconnected."
		} else if errors.Is(err, account.ErrMissingScope) {
			code, message = "youtube_scope_missing", "The authorization did not grant every required permission."
		}
		m.setTerminal(a, StateError, code, message)
		return
	}

	a.mu.Lock()
	a.snapshot.State = StateAuthorized
	a.snapshot.ConnectedAccountID = acc.ID
	attemptID := a.snapshot.AttemptID
	a.mu.Unlock()
	a.closeListener()
	m.clearActiveAndSchedule(attemptID)
}

// SelectChannel finalizes an attempt that is awaiting an explicit channel
// selection. channelID must be one of the channels this attempt actually
// fetched - never accepted blindly.
func (m *Manager) SelectChannel(ctx context.Context, attemptID, channelID string) (Snapshot, error) {
	m.mu.Lock()
	a, ok := m.attempts[attemptID]
	m.mu.Unlock()
	if !ok {
		return Snapshot{}, ErrNotFound
	}

	a.mu.Lock()
	if a.snapshot.State != StateAwaitingChannelSelect {
		a.mu.Unlock()
		return Snapshot{}, ErrChannelSelectionNotPending
	}
	var selected *youtube.Channel
	for i := range a.pendingChannels {
		if a.pendingChannels[i].ID == channelID {
			selected = &a.pendingChannels[i]
			break
		}
	}
	if selected == nil {
		a.mu.Unlock()
		return Snapshot{}, ErrInvalidChannelSelection
	}
	bundle, scopes := a.pendingBundle, a.pendingScopes
	a.mu.Unlock()

	m.finalizeChannel(ctx, a, *selected, bundle, scopes)
	return a.snapshotCopy(), nil
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

// CancelAttempt stops an active attempt and closes its loopback listener.
//
// Cancelling an already-terminal attempt is a no-op success, matching this
// project's other idempotent-cancel/delete conventions.
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
	a.snapshot.AuthorizationURL = ""
	attemptCancel := a.cancel
	snapshot := a.snapshot
	a.mu.Unlock()

	if attemptCancel != nil {
		attemptCancel()
	}
	a.closeListener()
	m.clearActiveAndSchedule(attemptID)

	return snapshot, nil
}

func containsScope(scopes []string, target string) bool {
	for _, s := range scopes {
		if s == target {
			return true
		}
	}
	return false
}
