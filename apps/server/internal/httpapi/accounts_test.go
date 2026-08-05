package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/account"
	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/provider/twitch"
	"github.com/streaming-tree/server/internal/runtime/deviceflow"
	"github.com/streaming-tree/server/internal/secrets/secretstest"
	"github.com/streaming-tree/server/internal/storage/sqlite"
)

// fakeHTTPProvider is a no-op account.Provider: these tests never need a
// real device flow to complete, only that the endpoints wire correctly and
// never leak a secret.
type fakeHTTPProvider struct{}

func (fakeHTTPProvider) ProviderID() account.ProviderID { return account.ProviderTwitch }
func (fakeHTTPProvider) StartDeviceFlow(ctx context.Context, clientID string, scopes []string) (account.DeviceFlowStart, error) {
	return account.DeviceFlowStart{
		DeviceCode: "dc-should-never-leave-the-backend", UserCode: "ABCD-EFGH",
		VerificationURI: "https://www.twitch.tv/activate", ExpiresIn: time.Minute, Interval: time.Hour,
	}, nil
}
func (fakeHTTPProvider) PollDeviceFlow(ctx context.Context, clientID, deviceCode string) (account.PollOutcome, error) {
	return account.PollOutcome{Status: account.PollPending}, nil
}
func (fakeHTTPProvider) ValidateToken(ctx context.Context, accessToken string) (account.ValidationResult, error) {
	return account.ValidationResult{Valid: true, ClientID: "test-client-id", Scopes: []string{"channel:manage:broadcast"}}, nil
}
func (fakeHTTPProvider) RefreshToken(ctx context.Context, clientID, refreshToken string) (account.TokenBundle, error) {
	return account.TokenBundle{}, nil
}
func (fakeHTTPProvider) RevokeToken(ctx context.Context, clientID, accessToken string) error {
	return nil
}
func (fakeHTTPProvider) GetIdentity(ctx context.Context, accessToken, clientID string) (account.Identity, error) {
	return account.Identity{}, nil
}

// newAccountTestServer wires the real router over real SQLite-backed
// platform and account services, an in-memory fake credential store, and a
// no-op fake Twitch provider - no real network call is ever made.
func newAccountTestServer(t *testing.T) (http.Handler, *account.Service) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "test.db")
	db, err := sqlite.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("opening the test database failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := sqlite.Migrate(context.Background(), db.DB); err != nil {
		t.Fatalf("migrating the test database failed: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	platforms := platform.NewService(sqlite.NewPlatformRepository(db.DB))

	providers := map[account.ProviderID]account.Provider{account.ProviderTwitch: fakeHTTPProvider{}}
	deviceFlowProviders := map[account.ProviderID]account.DeviceFlowProvider{account.ProviderTwitch: fakeHTTPProvider{}}
	accounts := account.NewService(account.Options{
		Repository: sqlite.NewAccountRepository(db.DB), Secrets: secretstest.New(), Providers: providers,
		RequiredScopes: map[account.ProviderID][]string{account.ProviderTwitch: {"channel:manage:broadcast"}}, Logger: logger,
	})
	if _, err := accounts.SetIntegrationClientID(context.Background(), account.ProviderTwitch, "test-client-id"); err != nil {
		t.Fatalf("SetIntegrationClientID() error = %v", err)
	}

	deviceFlow := deviceflow.NewManager(deviceflow.Options{
		Accounts: accounts, Providers: deviceFlowProviders,
		RequiredScopes: map[account.ProviderID][]string{account.ProviderTwitch: {"channel:manage:broadcast"}}, Logger: logger,
	})
	deviceFlow.Start(context.Background())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		deviceFlow.Shutdown(ctx)
	})

	twitchMetadata := twitch.NewMetadataService(accounts, twitch.New(twitch.Options{}))

	handler := NewRouter(Options{
		Logger: logger, StartedAt: time.Now(), Platforms: platforms,
		Accounts: accounts, DeviceFlow: deviceFlow, TwitchMetadata: twitchMetadata,
	})
	return handler, accounts
}

// --- integration config ------------------------------------------------

func TestGetIntegrationConfigReportsDatabaseSource(t *testing.T) {
	handler, _ := newAccountTestServer(t)

	recorder := do(t, handler, http.MethodGet, "/api/integrations/twitch/config", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var body integrationConfigResponse
	decodeBody(t, recorder, &body)
	if !body.Configured || body.Source != "database" || body.ClientID != "test-client-id" {
		t.Errorf("body = %+v", body)
	}
}

func TestSetIntegrationConfigRejectsAClientSecretField(t *testing.T) {
	handler, _ := newAccountTestServer(t)

	recorder := do(t, handler, http.MethodPut, "/api/integrations/twitch/config",
		map[string]any{"clientId": "new-id", "clientSecret": "should-be-rejected"})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for an unknown field", recorder.Code)
	}
}

func TestSetIntegrationConfigRejectsATokenShapedField(t *testing.T) {
	handler, _ := newAccountTestServer(t)

	for _, field := range []string{"accessToken", "refreshToken", "deviceCode"} {
		recorder := do(t, handler, http.MethodPut, "/api/integrations/twitch/config",
			map[string]any{"clientId": "new-id", field: "leak"})
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("field %q: status = %d, want 400", field, recorder.Code)
		}
	}
}

func TestSetIntegrationConfigRejectsAnEmptyValue(t *testing.T) {
	handler, _ := newAccountTestServer(t)

	recorder := do(t, handler, http.MethodPut, "/api/integrations/twitch/config", map[string]any{"clientId": ""})
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", recorder.Code)
	}
}

// --- device flow -----------------------------------------------------------

func TestDeviceFlowLifecycleThroughHTTP(t *testing.T) {
	handler, _ := newAccountTestServer(t)

	start := do(t, handler, http.MethodPost, "/api/integrations/twitch/device-flow", nil)
	if start.Code != http.StatusAccepted {
		t.Fatalf("start status = %d, want 202\nbody: %s", start.Code, start.Body.String())
	}
	var snap deviceFlowResponse
	decodeBody(t, start, &snap)
	if snap.UserCode != "ABCD-EFGH" {
		t.Errorf("UserCode = %q", snap.UserCode)
	}
	if start.Body.String() == "" || containsAny(start.Body.String(), "dc-should-never-leave-the-backend") {
		t.Error("the device code leaked into the HTTP response")
	}

	get := do(t, handler, http.MethodGet, "/api/integrations/twitch/device-flow/"+snap.AttemptID, nil)
	if get.Code != http.StatusOK {
		t.Fatalf("get status = %d, want 200", get.Code)
	}
	if containsAny(get.Body.String(), "dc-should-never-leave-the-backend") {
		t.Error("the device code leaked into the GET response")
	}

	cancel := do(t, handler, http.MethodDelete, "/api/integrations/twitch/device-flow/"+snap.AttemptID, nil)
	if cancel.Code != http.StatusOK {
		t.Fatalf("cancel status = %d, want 200", cancel.Code)
	}
	var cancelled deviceFlowResponse
	decodeBody(t, cancel, &cancelled)
	if cancelled.State != "cancelled" {
		t.Errorf("State = %q, want cancelled", cancelled.State)
	}
}

func TestStartDeviceFlowWithABodyIsRejected(t *testing.T) {
	handler, _ := newAccountTestServer(t)

	recorder := do(t, handler, http.MethodPost, "/api/integrations/twitch/device-flow", map[string]any{"unexpected": true})
	if recorder.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", recorder.Code)
	}
}

func TestConcurrentDeviceFlowStartIsAConflict(t *testing.T) {
	handler, _ := newAccountTestServer(t)

	first := do(t, handler, http.MethodPost, "/api/integrations/twitch/device-flow", nil)
	if first.Code != http.StatusAccepted {
		t.Fatalf("first start status = %d, want 202", first.Code)
	}
	second := do(t, handler, http.MethodPost, "/api/integrations/twitch/device-flow", nil)
	if second.Code != http.StatusConflict {
		t.Errorf("second start status = %d, want 409", second.Code)
	}
}

func TestGetUnknownDeviceFlowAttemptReturns404(t *testing.T) {
	handler, _ := newAccountTestServer(t)

	recorder := do(t, handler, http.MethodGet, "/api/integrations/twitch/device-flow/devflow_missing", nil)
	if recorder.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", recorder.Code)
	}
}

// --- connected accounts ------------------------------------------------

func TestListAccountsIsEmptyInitially(t *testing.T) {
	handler, _ := newAccountTestServer(t)

	recorder := do(t, handler, http.MethodGet, "/api/connected-accounts", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var body struct {
		Accounts []accountResponse `json:"accounts"`
	}
	decodeBody(t, recorder, &body)
	if len(body.Accounts) != 0 {
		t.Errorf("Accounts = %v, want empty", body.Accounts)
	}
}

func TestGetUnknownAccountReturns404(t *testing.T) {
	handler, _ := newAccountTestServer(t)

	recorder := do(t, handler, http.MethodGet, "/api/connected-accounts/acct_missing", nil)
	if recorder.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", recorder.Code)
	}
}

func TestAccountResponseNeverContainsATokenField(t *testing.T) {
	handler, svc := newAccountTestServer(t)
	acc, err := svc.FinalizeConnection(context.Background(), account.ProviderTwitch,
		account.Identity{ProviderUserID: "u1", Login: "streamer", DisplayName: "Streamer"},
		account.TokenBundle{TokenType: "bearer", AccessToken: "secret-access-token", RefreshToken: "secret-refresh-token", ExpiresAt: time.Now().Add(time.Hour)},
		[]string{"channel:manage:broadcast"}, "")
	if err != nil {
		t.Fatalf("FinalizeConnection() error = %v", err)
	}

	recorder := do(t, handler, http.MethodGet, "/api/connected-accounts/"+acc.ID, nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if containsAny(recorder.Body.String(), "secret-access-token", "secret-refresh-token") {
		t.Error("the account response leaked a token value")
	}
}

func TestDisconnectRemovesTheAccount(t *testing.T) {
	handler, svc := newAccountTestServer(t)
	acc, err := svc.FinalizeConnection(context.Background(), account.ProviderTwitch,
		account.Identity{ProviderUserID: "u1", Login: "streamer"},
		account.TokenBundle{TokenType: "bearer", AccessToken: "a", RefreshToken: "r", ExpiresAt: time.Now().Add(time.Hour)},
		[]string{"channel:manage:broadcast"}, "")
	if err != nil {
		t.Fatalf("FinalizeConnection() error = %v", err)
	}

	recorder := do(t, handler, http.MethodDelete, "/api/connected-accounts/"+acc.ID, nil)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", recorder.Code)
	}

	get := do(t, handler, http.MethodGet, "/api/connected-accounts/"+acc.ID, nil)
	if get.Code != http.StatusNotFound {
		t.Errorf("status after disconnect = %d, want 404", get.Code)
	}
}

// --- platform links --------------------------------------------------------

func TestPlatformAccountLinkLifecycle(t *testing.T) {
	handler, svc := newAccountTestServer(t)
	acc, err := svc.FinalizeConnection(context.Background(), account.ProviderTwitch,
		account.Identity{ProviderUserID: "u1", Login: "streamer"},
		account.TokenBundle{TokenType: "bearer", AccessToken: "a", RefreshToken: "r", ExpiresAt: time.Now().Add(time.Hour)},
		[]string{"channel:manage:broadcast"}, "")
	if err != nil {
		t.Fatalf("FinalizeConnection() error = %v", err)
	}

	initial := do(t, handler, http.MethodGet, "/api/platforms/pf_seed_twitch/connected-account", nil)
	if initial.Code != http.StatusOK || initial.Body.String() != "null\n" {
		t.Fatalf("initial link = %d %q, want 200 null", initial.Code, initial.Body.String())
	}

	set := do(t, handler, http.MethodPut, "/api/platforms/pf_seed_twitch/connected-account", map[string]any{"accountId": acc.ID})
	if set.Code != http.StatusOK {
		t.Fatalf("PUT link status = %d, want 200\nbody: %s", set.Code, set.Body.String())
	}
	var link linkResponse
	decodeBody(t, set, &link)
	if link.AccountID != acc.ID {
		t.Errorf("AccountID = %q, want %q", link.AccountID, acc.ID)
	}

	del := do(t, handler, http.MethodDelete, "/api/platforms/pf_seed_twitch/connected-account", nil)
	if del.Code != http.StatusNoContent {
		t.Fatalf("DELETE link status = %d, want 204", del.Code)
	}
	after := do(t, handler, http.MethodGet, "/api/platforms/pf_seed_twitch/connected-account", nil)
	if after.Body.String() != "null\n" {
		t.Errorf("link after delete = %q, want null", after.Body.String())
	}
}

func TestPlatformAccountLinkRejectsAProviderMismatch(t *testing.T) {
	handler, svc := newAccountTestServer(t)
	acc, err := svc.FinalizeConnection(context.Background(), account.ProviderTwitch,
		account.Identity{ProviderUserID: "u1", Login: "streamer"},
		account.TokenBundle{TokenType: "bearer", AccessToken: "a", RefreshToken: "r", ExpiresAt: time.Now().Add(time.Hour)},
		[]string{"channel:manage:broadcast"}, "")
	if err != nil {
		t.Fatalf("FinalizeConnection() error = %v", err)
	}

	// pf_seed_youtube is a YouTube destination; linking it to a Twitch
	// account must be rejected.
	recorder := do(t, handler, http.MethodPut, "/api/platforms/pf_seed_youtube/connected-account", map[string]any{"accountId": acc.ID})
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", recorder.Code)
	}
}

// --- category search ---------------------------------------------------

func TestSearchCategoriesValidatesQueryLength(t *testing.T) {
	handler, svc := newAccountTestServer(t)
	acc, err := svc.FinalizeConnection(context.Background(), account.ProviderTwitch,
		account.Identity{ProviderUserID: "u1", Login: "streamer"},
		account.TokenBundle{TokenType: "bearer", AccessToken: "a", RefreshToken: "r", ExpiresAt: time.Now().Add(time.Hour)},
		[]string{"channel:manage:broadcast"}, "")
	if err != nil {
		t.Fatalf("FinalizeConnection() error = %v", err)
	}

	tooShort := do(t, handler, http.MethodGet, "/api/connected-accounts/"+acc.ID+"/twitch/categories?query=a", nil)
	if tooShort.Code != http.StatusUnprocessableEntity {
		t.Errorf("too-short query status = %d, want 422", tooShort.Code)
	}
}

// --- method/route hygiene, preserved from earlier stages --------------------

func TestAccountRoutesRejectUnsupportedMethods(t *testing.T) {
	handler, _ := newAccountTestServer(t)

	recorder := do(t, handler, http.MethodPatch, "/api/connected-accounts", nil)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", recorder.Code)
	}
	if recorder.Header().Get("Allow") == "" {
		t.Error("405 response is missing the Allow header")
	}
}

func TestExistingPlatformAPIStillWorksWithAccountsWired(t *testing.T) {
	handler, _ := newAccountTestServer(t)

	recorder := do(t, handler, http.MethodGet, "/api/platforms", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
}

func containsAny(haystack string, needles ...string) bool {
	for _, n := range needles {
		if len(n) > 0 && strings.Contains(haystack, n) {
			return true
		}
	}
	return false
}
