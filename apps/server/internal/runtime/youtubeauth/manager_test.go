package youtubeauth

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/account"
	"github.com/streaming-tree/server/internal/provider/youtube"
	"github.com/streaming-tree/server/internal/secrets/secretstest"
	"github.com/streaming-tree/server/internal/storage/sqlite"
)

const testScope = "https://www.googleapis.com/auth/youtube.force-ssl"

// fakeGoogle is a local httptest double reproducing only the response
// shapes this package's manager actually depends on - no real Google
// request is ever made by these tests.
type fakeGoogle struct {
	oauth *httptest.Server
	api   *httptest.Server

	channelsBody string
}

func newFakeGoogle(t *testing.T) *fakeGoogle {
	t.Helper()
	fg := &fakeGoogle{channelsBody: `{"items":[{"id":"UC1","snippet":{"title":"Solo Channel"}}]}`}

	fg.oauth = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"access_token":"fake-access-token","refresh_token":"fake-refresh-token","expires_in":3600,"scope":"` + testScope + `","token_type":"Bearer"}`))
	}))
	fg.api = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fg.channelsBody))
	}))
	t.Cleanup(fg.oauth.Close)
	t.Cleanup(fg.api.Close)
	return fg
}

func newTestManager(t *testing.T, fg *fakeGoogle) (*Manager, *account.Service) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sqlite.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("sqlite.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := sqlite.Migrate(context.Background(), db.DB); err != nil {
		t.Fatalf("sqlite.Migrate() error = %v", err)
	}

	client := youtube.New(youtube.Options{OAuthBaseURL: fg.oauth.URL, APIBaseURL: fg.api.URL})
	adapter := youtube.NewAdapter(client)

	accounts := account.NewService(account.Options{
		Repository:     sqlite.NewAccountRepository(db.DB),
		Secrets:        secretstest.New(),
		Providers:      map[account.ProviderID]account.Provider{account.ProviderYouTube: adapter},
		RequiredScopes: map[account.ProviderID][]string{account.ProviderYouTube: {testScope}},
		Logger:         logger,
	})
	if _, err := accounts.SetIntegrationClientID(context.Background(), account.ProviderYouTube, "test-client-id"); err != nil {
		t.Fatalf("SetIntegrationClientID() error = %v", err)
	}

	m := NewManager(Options{Accounts: accounts, Client: client, RequiredScopes: []string{testScope}, Logger: logger})
	m.Start(context.Background())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		m.Shutdown(ctx)
	})

	return m, accounts
}

// callbackURLAndState extracts the loopback callback URL and the CSRF
// state this manager generated, both embedded in the public authorization
// URL exactly as a real browser round-trip to Google would echo them back -
// nothing here reads any unexported field.
func callbackURLAndState(t *testing.T, authorizationURL string) (string, string) {
	t.Helper()
	parsed, err := url.Parse(authorizationURL)
	if err != nil {
		t.Fatalf("authorization URL did not parse: %v", err)
	}
	q := parsed.Query()
	redirectURI := q.Get("redirect_uri")
	state := q.Get("state")
	if redirectURI == "" || state == "" {
		t.Fatalf("authorization URL missing redirect_uri or state: %q", authorizationURL)
	}
	return redirectURI, state
}

func waitForState(t *testing.T, m *Manager, attemptID string, want State) Snapshot {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		snap, err := m.GetAttempt(attemptID)
		if err != nil {
			t.Fatalf("GetAttempt() error = %v", err)
		}
		if snap.State == want {
			return snap
		}
		if snap.State.terminal() && want != snap.State {
			t.Fatalf("attempt reached terminal state %q, want %q (error: %s / %s)", snap.State, want, snap.ErrorCode, snap.ErrorMessage)
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for state %q", want)
	return Snapshot{}
}

func TestStartAttemptReturnsWaitingForBrowserWithAnAuthorizationURL(t *testing.T) {
	m, _ := newTestManager(t, newFakeGoogle(t))

	snap, err := m.StartAttempt(context.Background(), "")
	if err != nil {
		t.Fatalf("StartAttempt() error = %v", err)
	}
	if snap.State != StateWaitingForBrowser {
		t.Errorf("State = %q, want %q", snap.State, StateWaitingForBrowser)
	}
	if snap.AuthorizationURL == "" {
		t.Error("AuthorizationURL is empty")
	}
	if snap.ConnectedAccountID != "" {
		t.Error("ConnectedAccountID should be empty before authorization completes")
	}
}

func TestStartAttemptRejectsASecondConcurrentAttempt(t *testing.T) {
	m, _ := newTestManager(t, newFakeGoogle(t))

	if _, err := m.StartAttempt(context.Background(), ""); err != nil {
		t.Fatalf("first StartAttempt() error = %v", err)
	}
	if _, err := m.StartAttempt(context.Background(), ""); err == nil {
		t.Fatal("second StartAttempt() error = nil, want ErrConflict")
	}
}

func TestStartAttemptRequiresAConfiguredClientID(t *testing.T) {
	fg := newFakeGoogle(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sqlite.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("sqlite.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := sqlite.Migrate(context.Background(), db.DB); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	client := youtube.New(youtube.Options{OAuthBaseURL: fg.oauth.URL, APIBaseURL: fg.api.URL})
	accounts := account.NewService(account.Options{
		Repository: sqlite.NewAccountRepository(db.DB), Secrets: secretstest.New(),
		Providers:      map[account.ProviderID]account.Provider{account.ProviderYouTube: youtube.NewAdapter(client)},
		RequiredScopes: map[account.ProviderID][]string{account.ProviderYouTube: {testScope}}, Logger: logger,
	})
	m := NewManager(Options{Accounts: accounts, Client: client, RequiredScopes: []string{testScope}, Logger: logger})
	m.Start(context.Background())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		m.Shutdown(ctx)
	})

	if _, err := m.StartAttempt(context.Background(), ""); err == nil {
		t.Fatal("StartAttempt() error = nil, want an integration-not-configured error")
	}
}

func TestCallbackWithWrongStateDoesNotAffectTheRealAttempt(t *testing.T) {
	m, accounts := newTestManager(t, newFakeGoogle(t))

	snap, err := m.StartAttempt(context.Background(), "")
	if err != nil {
		t.Fatalf("StartAttempt() error = %v", err)
	}
	callbackURL, realState := callbackURLAndState(t, snap.AuthorizationURL)

	// A stray/hostile request with the wrong state must not deny the real
	// attempt - see the task's constant-time-comparison requirement.
	resp, err := http.Get(callbackURL + "?code=irrelevant&state=totally-wrong-state")
	if err != nil {
		t.Fatalf("stray callback request failed: %v", err)
	}
	_ = resp.Body.Close()

	still, err := m.GetAttempt(snap.AttemptID)
	if err != nil {
		t.Fatalf("GetAttempt() error = %v", err)
	}
	if still.State != StateWaitingForBrowser {
		t.Fatalf("state after a wrong-state callback = %q, want unchanged %q", still.State, StateWaitingForBrowser)
	}

	// The real callback, with the correct state, still succeeds afterwards.
	resp2, err := http.Get(callbackURL + "?code=real-code&state=" + realState)
	if err != nil {
		t.Fatalf("real callback request failed: %v", err)
	}
	_ = resp2.Body.Close()

	final := waitForState(t, m, snap.AttemptID, StateAuthorized)
	if final.ConnectedAccountID == "" {
		t.Error("ConnectedAccountID is empty after successful authorization")
	}
	if _, err := accounts.GetAccount(context.Background(), final.ConnectedAccountID); err != nil {
		t.Errorf("the finalized account could not be loaded: %v", err)
	}
}

func TestSingleChannelFinalizesAutomatically(t *testing.T) {
	m, accounts := newTestManager(t, newFakeGoogle(t))

	snap, err := m.StartAttempt(context.Background(), "")
	if err != nil {
		t.Fatalf("StartAttempt() error = %v", err)
	}
	callbackURL, state := callbackURLAndState(t, snap.AuthorizationURL)

	resp, err := http.Get(callbackURL + "?code=real-code&state=" + state)
	if err != nil {
		t.Fatalf("callback request failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if string(body) == "" {
		t.Error("callback page response body is empty")
	}

	final := waitForState(t, m, snap.AttemptID, StateAuthorized)
	acc, err := accounts.GetAccount(context.Background(), final.ConnectedAccountID)
	if err != nil {
		t.Fatalf("GetAccount() error = %v", err)
	}
	if acc.ProviderUserID != "UC1" || acc.Login != "Solo Channel" {
		t.Errorf("account = %+v, want ProviderUserID=UC1 Login=\"Solo Channel\"", acc)
	}
}

func TestMultipleChannelsRequireExplicitSelection(t *testing.T) {
	fg := newFakeGoogle(t)
	fg.channelsBody = `{"items":[{"id":"UC1","snippet":{"title":"First"}},{"id":"UC2","snippet":{"title":"Second"}}]}`
	m, accounts := newTestManager(t, fg)

	snap, err := m.StartAttempt(context.Background(), "")
	if err != nil {
		t.Fatalf("StartAttempt() error = %v", err)
	}
	callbackURL, state := callbackURLAndState(t, snap.AuthorizationURL)

	resp, err := http.Get(callbackURL + "?code=real-code&state=" + state)
	if err != nil {
		t.Fatalf("callback request failed: %v", err)
	}
	_ = resp.Body.Close()

	waiting := waitForState(t, m, snap.AttemptID, StateAwaitingChannelSelect)
	if len(waiting.Channels) != 2 {
		t.Fatalf("Channels = %+v, want 2 offered channels", waiting.Channels)
	}

	final, err := m.SelectChannel(context.Background(), snap.AttemptID, "UC2")
	if err != nil {
		t.Fatalf("SelectChannel() error = %v", err)
	}
	if final.State != StateAuthorized {
		t.Fatalf("state after selection = %q, want %q", final.State, StateAuthorized)
	}
	acc, err := accounts.GetAccount(context.Background(), final.ConnectedAccountID)
	if err != nil {
		t.Fatalf("GetAccount() error = %v", err)
	}
	if acc.ProviderUserID != "UC2" {
		t.Errorf("finalized account = %+v, want the explicitly selected UC2, not the first channel", acc)
	}
}

func TestSelectChannelRejectsAChannelNotOffered(t *testing.T) {
	fg := newFakeGoogle(t)
	fg.channelsBody = `{"items":[{"id":"UC1","snippet":{"title":"First"}},{"id":"UC2","snippet":{"title":"Second"}}]}`
	m, _ := newTestManager(t, fg)

	snap, err := m.StartAttempt(context.Background(), "")
	if err != nil {
		t.Fatalf("StartAttempt() error = %v", err)
	}
	callbackURL, state := callbackURLAndState(t, snap.AuthorizationURL)
	resp, err := http.Get(callbackURL + "?code=real-code&state=" + state)
	if err != nil {
		t.Fatalf("callback request failed: %v", err)
	}
	_ = resp.Body.Close()
	waitForState(t, m, snap.AttemptID, StateAwaitingChannelSelect)

	if _, err := m.SelectChannel(context.Background(), snap.AttemptID, "UC-not-offered"); err == nil {
		t.Fatal("SelectChannel() error = nil, want ErrInvalidChannelSelection")
	}
}

func TestCallbackWithAccessDeniedSetsStateDenied(t *testing.T) {
	m, _ := newTestManager(t, newFakeGoogle(t))

	snap, err := m.StartAttempt(context.Background(), "")
	if err != nil {
		t.Fatalf("StartAttempt() error = %v", err)
	}
	callbackURL, state := callbackURLAndState(t, snap.AuthorizationURL)

	resp, err := http.Get(callbackURL + "?error=access_denied&state=" + state)
	if err != nil {
		t.Fatalf("callback request failed: %v", err)
	}
	_ = resp.Body.Close()

	final := waitForState(t, m, snap.AttemptID, StateDenied)
	if final.ErrorCode != "youtube_oauth_access_denied" {
		t.Errorf("ErrorCode = %q, want youtube_oauth_access_denied", final.ErrorCode)
	}
}

func TestMissingRequiredScopeFinishesWithScopeMissingError(t *testing.T) {
	fg := newFakeGoogle(t)
	fg.oauth = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"access_token":"at","refresh_token":"rt","expires_in":3600,"scope":"https://www.googleapis.com/auth/youtube.readonly","token_type":"Bearer"}`))
	}))
	m, _ := newTestManager(t, fg)

	snap, err := m.StartAttempt(context.Background(), "")
	if err != nil {
		t.Fatalf("StartAttempt() error = %v", err)
	}
	callbackURL, state := callbackURLAndState(t, snap.AuthorizationURL)
	resp, err := http.Get(callbackURL + "?code=real-code&state=" + state)
	if err != nil {
		t.Fatalf("callback request failed: %v", err)
	}
	_ = resp.Body.Close()

	final := waitForState(t, m, snap.AttemptID, StateError)
	if final.ErrorCode != "youtube_scope_missing" {
		t.Errorf("ErrorCode = %q, want youtube_scope_missing", final.ErrorCode)
	}
}

func TestCancelAttemptClosesTheListenerAndFreesTheSlot(t *testing.T) {
	m, _ := newTestManager(t, newFakeGoogle(t))

	snap, err := m.StartAttempt(context.Background(), "")
	if err != nil {
		t.Fatalf("StartAttempt() error = %v", err)
	}
	callbackURL, _ := callbackURLAndState(t, snap.AuthorizationURL)

	cancelled, err := m.CancelAttempt(snap.AttemptID)
	if err != nil {
		t.Fatalf("CancelAttempt() error = %v", err)
	}
	if cancelled.State != StateCancelled {
		t.Errorf("State = %q, want %q", cancelled.State, StateCancelled)
	}

	// The slot is free immediately: a new attempt can start without waiting.
	if _, err := m.StartAttempt(context.Background(), ""); err != nil {
		t.Errorf("StartAttempt() after cancellation error = %v, want the slot to be free", err)
	}

	// The listener is actually closed, not just marked cancelled.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_, err := http.Get(callbackURL)
		if err != nil {
			return // connection refused - the listener is closed, as expected
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Error("the cancelled attempt's loopback listener is still accepting connections")
}

func TestCancellingAnAlreadyTerminalAttemptIsANoOp(t *testing.T) {
	m, _ := newTestManager(t, newFakeGoogle(t))
	snap, err := m.StartAttempt(context.Background(), "")
	if err != nil {
		t.Fatalf("StartAttempt() error = %v", err)
	}
	if _, err := m.CancelAttempt(snap.AttemptID); err != nil {
		t.Fatalf("first CancelAttempt() error = %v", err)
	}
	again, err := m.CancelAttempt(snap.AttemptID)
	if err != nil {
		t.Fatalf("second CancelAttempt() error = %v, want a no-op success", err)
	}
	if again.State != StateCancelled {
		t.Errorf("State = %q, want %q", again.State, StateCancelled)
	}
}

func TestGetAttemptForUnknownIDReturnsNotFound(t *testing.T) {
	m, _ := newTestManager(t, newFakeGoogle(t))
	if _, err := m.GetAttempt("nonexistent"); err == nil {
		t.Fatal("GetAttempt() error = nil, want ErrNotFound")
	}
}

func TestSnapshotNeverCarriesAnAuthorizationURLOnceTerminal(t *testing.T) {
	m, _ := newTestManager(t, newFakeGoogle(t))
	snap, err := m.StartAttempt(context.Background(), "")
	if err != nil {
		t.Fatalf("StartAttempt() error = %v", err)
	}
	cancelled, err := m.CancelAttempt(snap.AttemptID)
	if err != nil {
		t.Fatalf("CancelAttempt() error = %v", err)
	}
	if cancelled.AuthorizationURL != "" {
		t.Error("a terminal snapshot still carries an AuthorizationURL - it must be cleared once the attempt is no longer waiting")
	}
}

func TestShutdownStopsAllActiveAttemptsPromptly(t *testing.T) {
	m, _ := newTestManager(t, newFakeGoogle(t))
	if _, err := m.StartAttempt(context.Background(), ""); err != nil {
		t.Fatalf("StartAttempt() error = %v", err)
	}

	done := make(chan struct{})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		m.Shutdown(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Shutdown() did not return promptly with an active attempt")
	}
}
