package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/account"
	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/domain/remotetarget"
	"github.com/streaming-tree/server/internal/provider/twitch"
	"github.com/streaming-tree/server/internal/provider/youtube"
	"github.com/streaming-tree/server/internal/runtime/deviceflow"
	"github.com/streaming-tree/server/internal/runtime/youtubeauth"
	"github.com/streaming-tree/server/internal/secrets/secretstest"
	"github.com/streaming-tree/server/internal/storage/sqlite"
)

const testYouTubeScope = "https://www.googleapis.com/auth/youtube.force-ssl"

// newYouTubeTestServer wires the real router over real SQLite-backed
// platform/account/remote-target services and a real youtubeauth.Manager
// pointed at a local fake Google OAuth/YouTube API server - no real
// network call is ever made.
func newYouTubeTestServer(t *testing.T) (http.Handler, *account.Service, *youtube.MetadataService) {
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

	fakeOAuth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"access_token":"fake-access-token","refresh_token":"fake-refresh-token","expires_in":3600,"scope":"` + testYouTubeScope + `","token_type":"Bearer"}`))
	}))
	t.Cleanup(fakeOAuth.Close)
	fakeAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	t.Cleanup(fakeAPI.Close)

	youtubeClient := youtube.New(youtube.Options{OAuthBaseURL: fakeOAuth.URL, APIBaseURL: fakeAPI.URL})
	youtubeAdapter := youtube.NewAdapter(youtubeClient)

	accounts := account.NewService(account.Options{
		Repository:     sqlite.NewAccountRepository(db.DB),
		Secrets:        secretstest.New(),
		Providers:      map[account.ProviderID]account.Provider{account.ProviderYouTube: youtubeAdapter},
		RequiredScopes: map[account.ProviderID][]string{account.ProviderYouTube: {testYouTubeScope}},
		Logger:         logger,
	})
	if _, err := accounts.SetIntegrationClientID(context.Background(), account.ProviderYouTube, "test-client-id"); err != nil {
		t.Fatalf("SetIntegrationClientID() error = %v", err)
	}

	youtubeAuth := youtubeauth.NewManager(youtubeauth.Options{
		Accounts: accounts, Client: youtubeClient, RequiredScopes: []string{testYouTubeScope}, Logger: logger,
	})
	youtubeAuth.Start(context.Background())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		youtubeAuth.Shutdown(ctx)
	})

	youtubeRegions := sqlite.NewYouTubeRegionRepository(db.DB)
	youtubeMetadata := youtube.NewMetadataService(accounts, youtubeRegions, youtubeClient)
	remoteTargets := remotetarget.NewService(sqlite.NewRemoteTargetRepository(db.DB), nil)

	// Twitch is wired too (with a real, harmless client) purely because
	// registerAccountRoutes requires DeviceFlow/TwitchMetadata to be
	// non-nil for the shared publish endpoints to register at all.
	deviceFlow := deviceflow.NewManager(deviceflow.Options{
		Accounts: accounts, Providers: map[account.ProviderID]account.DeviceFlowProvider{},
		RequiredScopes: map[account.ProviderID][]string{}, Logger: logger,
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
		YouTubeAuth: youtubeAuth, YouTubeMetadata: youtubeMetadata, RemoteTargets: remoteTargets,
	})
	return handler, accounts, youtubeMetadata
}

// --- integration config ------------------------------------------------

func TestYouTubeIntegrationConfigRejectsAClientSecretField(t *testing.T) {
	handler, _, _ := newYouTubeTestServer(t)

	recorder := do(t, handler, http.MethodPut, "/api/integrations/youtube/config", map[string]any{
		"clientId": "real-client-id", "clientSecret": "should-be-rejected",
	})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for an unknown field", recorder.Code)
	}
}

func TestYouTubeIntegrationConfigRejectsACompleteCredentialsJSON(t *testing.T) {
	handler, _, _ := newYouTubeTestServer(t)

	// Google's downloadable OAuth client JSON has a nested "installed"
	// object with client_id/client_secret/redirect_uris - none of which
	// this application's request schema declares a field for.
	recorder := do(t, handler, http.MethodPut, "/api/integrations/youtube/config", map[string]any{
		"installed": map[string]any{"client_id": "x", "client_secret": "y"},
	})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a pasted credentials.json shape", recorder.Code)
	}
}

func TestYouTubeIntegrationConfigSavesAndReportsDatabaseSource(t *testing.T) {
	handler, _, _ := newYouTubeTestServer(t)

	recorder := do(t, handler, http.MethodGet, "/api/integrations/youtube/config", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var body struct {
		Configured bool   `json:"configured"`
		Source     string `json:"source"`
		ClientID   string `json:"clientId"`
	}
	decodeBody(t, recorder, &body)
	if !body.Configured || body.Source != "database" || body.ClientID != "test-client-id" {
		t.Errorf("body = %+v, want the client ID saved in newYouTubeTestServer's setup", body)
	}
}

// --- OAuth attempts ------------------------------------------------------

func TestYouTubeOAuthAttemptStartAndConflict(t *testing.T) {
	handler, _, _ := newYouTubeTestServer(t)

	first := do(t, handler, http.MethodPost, "/api/integrations/youtube/oauth-attempts", nil)
	if first.Code != http.StatusAccepted {
		t.Fatalf("first attempt status = %d, want 202", first.Code)
	}
	var body struct {
		AttemptID        string `json:"attemptId"`
		State            string `json:"state"`
		AuthorizationURL string `json:"authorizationUrl"`
		UserCode         string `json:"userCode"`
	}
	decodeBody(t, first, &body)
	if body.AttemptID == "" || body.AuthorizationURL == "" {
		t.Fatalf("body = %+v, want a non-empty attemptId and authorizationUrl", body)
	}
	if body.UserCode != "" {
		t.Error("response contains a userCode field - that is a device-flow concept, not YouTube's")
	}

	second := do(t, handler, http.MethodPost, "/api/integrations/youtube/oauth-attempts", nil)
	if second.Code != http.StatusConflict {
		t.Fatalf("second attempt status = %d, want 409", second.Code)
	}

	got := do(t, handler, http.MethodGet, "/api/integrations/youtube/oauth-attempts/"+body.AttemptID, nil)
	if got.Code != http.StatusOK {
		t.Fatalf("GET attempt status = %d, want 200", got.Code)
	}

	cancelled := do(t, handler, http.MethodDelete, "/api/integrations/youtube/oauth-attempts/"+body.AttemptID, nil)
	if cancelled.Code != http.StatusOK {
		t.Fatalf("DELETE attempt status = %d, want 200", cancelled.Code)
	}
}

func TestYouTubeOAuthAttemptNotFound(t *testing.T) {
	handler, _, _ := newYouTubeTestServer(t)

	recorder := do(t, handler, http.MethodGet, "/api/integrations/youtube/oauth-attempts/nonexistent", nil)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
}

func TestYouTubeOAuthAttemptStartRejectsANonEmptyBody(t *testing.T) {
	handler, _, _ := newYouTubeTestServer(t)

	recorder := do(t, handler, http.MethodPost, "/api/integrations/youtube/oauth-attempts", map[string]any{"unexpected": "field"})
	if recorder.Code == http.StatusAccepted {
		t.Fatal("status = 202, want a command endpoint to reject an unexpected request body")
	}
}

// --- remote target -----------------------------------------------------

func TestSetRemoteTargetRejectsANonYouTubePlatform(t *testing.T) {
	handler, _, _ := newYouTubeTestServer(t)

	recorder := do(t, handler, http.MethodPut, "/api/platforms/pf_seed_twitch/remote-target", map[string]any{"resourceId": "bcast1"})
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 for a non-YouTube destination", recorder.Code)
	}
}

func TestSetRemoteTargetRequiresALinkedAccount(t *testing.T) {
	handler, _, _ := newYouTubeTestServer(t)

	recorder := do(t, handler, http.MethodPut, "/api/platforms/pf_seed_youtube/remote-target", map[string]any{"resourceId": "bcast1"})
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (account_not_linked)", recorder.Code)
	}
}

func TestGetRemoteTargetReturnsNullWhenUnset(t *testing.T) {
	handler, _, _ := newYouTubeTestServer(t)

	recorder := do(t, handler, http.MethodGet, "/api/platforms/pf_seed_youtube/remote-target", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if body := strings.TrimSpace(recorder.Body.String()); body != "null" {
		t.Errorf("body = %q, want the literal null", body)
	}
}

func TestGetRemoteTargetFor404sOnAnUnknownPlatform(t *testing.T) {
	handler, _, _ := newYouTubeTestServer(t)

	recorder := do(t, handler, http.MethodGet, "/api/platforms/pf_does_not_exist/remote-target", nil)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
}

func TestDeleteRemoteTargetIsIdempotent(t *testing.T) {
	handler, _, _ := newYouTubeTestServer(t)

	recorder := do(t, handler, http.MethodDelete, "/api/platforms/pf_seed_youtube/remote-target", nil)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 even when no target was ever set", recorder.Code)
	}
}

// --- method routing -----------------------------------------------------

func TestYouTubeRoutesReject405WithAllowHeader(t *testing.T) {
	handler, _, _ := newYouTubeTestServer(t)

	recorder := do(t, handler, http.MethodDelete, "/api/integrations/youtube/config", nil)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", recorder.Code)
	}
	if allow := recorder.Header().Get("Allow"); allow == "" {
		t.Error("405 response missing an Allow header")
	}
}

// --- categories/region --------------------------------------------------

func TestYouTubeCategoriesFor404sOnAnUnknownAccount(t *testing.T) {
	handler, _, _ := newYouTubeTestServer(t)

	recorder := do(t, handler, http.MethodGet, "/api/connected-accounts/acct_missing/youtube/categories", nil)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
}

func TestYouTubeSetRegionRejectsAnInvalidCode(t *testing.T) {
	handler, accounts, _ := newYouTubeTestServer(t)

	acc, err := accounts.FinalizeConnection(context.Background(), account.ProviderYouTube,
		account.Identity{ProviderUserID: "UC1", Login: "Channel", DisplayName: "Channel"},
		account.TokenBundle{TokenType: "bearer", AccessToken: "at", RefreshToken: "rt", ExpiresAt: time.Now().Add(time.Hour)},
		[]string{testYouTubeScope}, "")
	if err != nil {
		t.Fatalf("FinalizeConnection() error = %v", err)
	}

	recorder := do(t, handler, http.MethodPut, "/api/connected-accounts/"+acc.ID+"/youtube/region", map[string]any{"region": "usa"})
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 for a non-two-letter region code", recorder.Code)
	}

	ok := do(t, handler, http.MethodPut, "/api/connected-accounts/"+acc.ID+"/youtube/region", map[string]any{"region": "us"})
	if ok.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for a valid two-letter code", ok.Code)
	}
	var body struct {
		Region string `json:"region"`
	}
	decodeBody(t, ok, &body)
	if body.Region != "US" {
		t.Errorf("Region = %q, want it upper-cased to US", body.Region)
	}
}
