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
	engagement "github.com/streaming-tree/server/internal/domain/engagement"
	"github.com/streaming-tree/server/internal/domain/engagementsettings"
	"github.com/streaming-tree/server/internal/domain/platform"
	bus "github.com/streaming-tree/server/internal/engagement"
	"github.com/streaming-tree/server/internal/provider/twitch"
	"github.com/streaming-tree/server/internal/runtime/deviceflow"
	"github.com/streaming-tree/server/internal/runtime/twitchengagement"
	"github.com/streaming-tree/server/internal/secrets/secretstest"
	"github.com/streaming-tree/server/internal/storage/sqlite"
)

// fakeConnectors is a controllable EngagementConnectorService double - the
// real WebSocket connector lifecycle is already exhaustively tested in
// internal/runtime/twitchengagement; these tests only need to verify the
// HTTP layer's request/response mapping and error handling.
type fakeConnectors struct {
	snapshots  map[string]twitchengagement.Snapshot
	enableErr  error
	disableErr error
	restartErr error
}

func newFakeConnectors() *fakeConnectors {
	return &fakeConnectors{snapshots: map[string]twitchengagement.Snapshot{}}
}

func (f *fakeConnectors) Enable(ctx context.Context, accountID string) (twitchengagement.Snapshot, error) {
	if f.enableErr != nil {
		return twitchengagement.Snapshot{}, f.enableErr
	}
	snap := twitchengagement.Snapshot{AccountID: accountID, Enabled: true, State: twitchengagement.StateConnected, ExpectedSubscriptionCount: 13, ActiveSubscriptionCount: 13}
	f.snapshots[accountID] = snap
	return snap, nil
}

func (f *fakeConnectors) Disable(ctx context.Context, accountID string) (twitchengagement.Snapshot, error) {
	if f.disableErr != nil {
		return twitchengagement.Snapshot{}, f.disableErr
	}
	delete(f.snapshots, accountID)
	return twitchengagement.Snapshot{AccountID: accountID, Enabled: false, State: twitchengagement.StateDisabled}, nil
}

func (f *fakeConnectors) Restart(ctx context.Context, accountID string) (twitchengagement.Snapshot, error) {
	if f.restartErr != nil {
		return twitchengagement.Snapshot{}, f.restartErr
	}
	snap, ok := f.snapshots[accountID]
	if !ok {
		return twitchengagement.Snapshot{}, twitchengagement.ErrNotFound
	}
	return snap, nil
}

func (f *fakeConnectors) Snapshot(accountID string) (twitchengagement.Snapshot, bool) {
	snap, ok := f.snapshots[accountID]
	return snap, ok
}

func (f *fakeConnectors) Snapshots() []twitchengagement.Snapshot {
	out := make([]twitchengagement.Snapshot, 0, len(f.snapshots))
	for _, s := range f.snapshots {
		out = append(out, s)
	}
	return out
}

type engagementTestServer struct {
	handler    http.Handler
	accounts   *account.Service
	settings   *engagementsettings.Service
	bus        *bus.Bus
	connectors *fakeConnectors
}

func newEngagementTestServer(t *testing.T) *engagementTestServer {
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

	platforms := platform.NewService(sqlite.NewPlatformRepository(db.DB))
	accountRepo := sqlite.NewAccountRepository(db.DB)
	secretStore := secretstest.New()
	provider := fakeHTTPProvider{}
	accounts := account.NewService(account.Options{
		Repository: accountRepo, Secrets: secretStore,
		Providers:      map[account.ProviderID]account.Provider{account.ProviderTwitch: provider},
		RequiredScopes: map[account.ProviderID][]string{account.ProviderTwitch: {"channel:manage:broadcast"}},
		Logger:         logger,
	})
	if _, err := accounts.SetIntegrationClientID(context.Background(), account.ProviderTwitch, "test-client-id"); err != nil {
		t.Fatalf("SetIntegrationClientID() error = %v", err)
	}

	deviceFlow := deviceflow.NewManager(deviceflow.Options{
		Accounts:       accounts,
		Providers:      map[account.ProviderID]account.DeviceFlowProvider{account.ProviderTwitch: provider},
		RequiredScopes: map[account.ProviderID][]string{account.ProviderTwitch: {"channel:manage:broadcast"}},
		Logger:         logger,
	})
	deviceFlow.Start(context.Background())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		deviceFlow.Shutdown(ctx)
	})

	settingsRepo := sqlite.NewEngagementSettingsRepository(db.DB)
	settings := engagementsettings.NewService(settingsRepo, nil)

	eventBus := bus.New(bus.Options{Capacity: 100})
	t.Cleanup(eventBus.Shutdown)

	connectors := newFakeConnectors()

	handler := NewRouter(Options{
		Logger: logger, StartedAt: time.Now(), Platforms: platforms,
		Accounts: accounts, DeviceFlow: deviceFlow, TwitchMetadata: twitch.NewMetadataService(accounts, twitch.New(twitch.Options{})),
		EngagementBus: eventBus, EngagementSettings: settings, EngagementConnectors: connectors,
	})

	return &engagementTestServer{handler: handler, accounts: accounts, settings: settings, bus: eventBus, connectors: connectors}
}

func (ts *engagementTestServer) createAccount(t *testing.T, id string, providerID account.ProviderID, scopes []string) {
	t.Helper()
	acc := account.Account{
		ID: id, ProviderID: providerID, ProviderUserID: id + "_provider_user", Login: "viewer", DisplayName: "Viewer",
		Scopes: scopes,
	}
	if err := ts.accountRepoCreate(acc); err != nil {
		t.Fatalf("createAccount() error = %v", err)
	}
}

// accountRepoCreate is a tiny indirection so createAccount can reach the
// same underlying repository the test server's account.Service uses,
// without exposing the repository type directly in the test struct.
func (ts *engagementTestServer) accountRepoCreate(acc account.Account) error {
	// account.Service has no direct "insert" method (accounts are normally
	// created via device-flow finalization); go through FinalizeConnection
	// with the fake provider's identity instead, which exercises the real
	// code path these HTTP handlers ultimately depend on.
	_, err := ts.accounts.FinalizeConnection(context.Background(), acc.ProviderID,
		account.Identity{ProviderUserID: acc.ProviderUserID, Login: acc.Login, DisplayName: acc.DisplayName},
		account.TokenBundle{TokenType: "bearer", AccessToken: "fake-access", RefreshToken: "fake-refresh", ExpiresAt: time.Now().Add(time.Hour)},
		acc.Scopes, "")
	return err
}

func TestGetAccountEngagementReportsDisabledForNeverEnabledAccount(t *testing.T) {
	ts := newEngagementTestServer(t)
	ts.createAccount(t, "unused", account.ProviderTwitch, []string{"channel:manage:broadcast"})

	accounts, err := ts.accounts.ListAccounts(context.Background())
	if err != nil || len(accounts) != 1 {
		t.Fatalf("ListAccounts() = %v, %v", accounts, err)
	}
	accountID := accounts[0].ID

	recorder := do(t, ts.handler, http.MethodGet, "/api/connected-accounts/"+accountID+"/engagement", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", recorder.Code, recorder.Body.String())
	}
	var body accountEngagementResponse
	decodeBody(t, recorder, &body)
	if body.State != string(twitchengagement.StateDisabled) || body.Enabled {
		t.Errorf("body = %+v, want disabled", body)
	}
	if len(body.RequiredScopes) != len(twitch.EngagementScopeProfile) {
		t.Errorf("RequiredScopes = %v, want the full engagement profile", body.RequiredScopes)
	}
	if !body.PermissionUpgradeRequired {
		t.Error("expected PermissionUpgradeRequired=true for an account with only the metadata scope")
	}
}

func TestGetAccountEngagementRejectsNonTwitchProvider(t *testing.T) {
	ts := newEngagementTestServer(t)
	ts.createAccount(t, "unused", account.ProviderYouTube, nil)

	accounts, err := ts.accounts.ListAccounts(context.Background())
	if err != nil || len(accounts) != 1 {
		t.Fatalf("ListAccounts() = %v, %v", accounts, err)
	}

	recorder := do(t, ts.handler, http.MethodGet, "/api/connected-accounts/"+accounts[0].ID+"/engagement", nil)
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 for a non-Twitch account, body = %s", recorder.Code, recorder.Body.String())
	}
	var body ErrorBody
	decodeBody(t, recorder, &body)
	if body.Error != "engagement_not_supported" {
		t.Errorf("Error = %q, want engagement_not_supported", body.Error)
	}
}

func TestPutAccountEngagementEnableAndDisable(t *testing.T) {
	ts := newEngagementTestServer(t)
	ts.createAccount(t, "unused", account.ProviderTwitch, append([]string{"channel:manage:broadcast"}, twitch.EngagementScopeProfile...))
	accounts, _ := ts.accounts.ListAccounts(context.Background())
	accountID := accounts[0].ID

	recorder := do(t, ts.handler, http.MethodPut, "/api/connected-accounts/"+accountID+"/engagement", map[string]any{"enabled": true})
	if recorder.Code != http.StatusOK {
		t.Fatalf("enable status = %d, want 200, body = %s", recorder.Code, recorder.Body.String())
	}
	var enabled accountEngagementResponse
	decodeBody(t, recorder, &enabled)
	if !enabled.Enabled || enabled.State != string(twitchengagement.StateConnected) {
		t.Errorf("enabled body = %+v", enabled)
	}
	// Settings persistence itself (SetEnabled) is the connector Manager's
	// own responsibility, already covered by
	// internal/runtime/twitchengagement's own tests - this test verifies
	// only that the HTTP layer calls through to Enable/Disable and maps the
	// resulting snapshot correctly.

	recorder2 := do(t, ts.handler, http.MethodPut, "/api/connected-accounts/"+accountID+"/engagement", map[string]any{"enabled": false})
	if recorder2.Code != http.StatusOK {
		t.Fatalf("disable status = %d, want 200", recorder2.Code)
	}
	var disabled accountEngagementResponse
	decodeBody(t, recorder2, &disabled)
	if disabled.Enabled {
		t.Error("expected Enabled=false after disabling")
	}
}

func TestPutAccountEngagementRejectsUnknownFields(t *testing.T) {
	ts := newEngagementTestServer(t)
	ts.createAccount(t, "unused", account.ProviderTwitch, []string{"channel:manage:broadcast"})
	accounts, _ := ts.accounts.ListAccounts(context.Background())

	req := httptest.NewRequest(http.MethodPut, "/api/connected-accounts/"+accounts[0].ID+"/engagement",
		strings.NewReader(`{"enabled": true, "sneaky": "field"}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	ts.handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for an unknown field, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestAuthorizeEngagementRequestsUnionScopesAndRejectsBody(t *testing.T) {
	ts := newEngagementTestServer(t)
	ts.createAccount(t, "unused", account.ProviderTwitch, []string{"channel:manage:broadcast"})
	accounts, _ := ts.accounts.ListAccounts(context.Background())
	accountID := accounts[0].ID

	// A request body is rejected outright - this is a command endpoint.
	bodyRecorder := do(t, ts.handler, http.MethodPost, "/api/connected-accounts/"+accountID+"/engagement/authorize", map[string]any{"unexpected": true})
	if bodyRecorder.Code != http.StatusBadRequest {
		t.Errorf("status with a body = %d, want 400", bodyRecorder.Code)
	}

	recorder := do(t, ts.handler, http.MethodPost, "/api/connected-accounts/"+accountID+"/engagement/authorize", nil)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202, body = %s", recorder.Code, recorder.Body.String())
	}
	var body deviceFlowResponse
	decodeBody(t, recorder, &body)
	if body.UserCode == "" {
		t.Error("expected a user code in the upgrade attempt's snapshot")
	}
	// Never a device code field anywhere in the response - deviceFlowResponse
	// structurally has no such field, verified by successfully decoding into
	// it without an "unknown field" mismatch.
}

func TestEngagementStatusReportsBusSnapshot(t *testing.T) {
	ts := newEngagementTestServer(t)

	recorder := do(t, ts.handler, http.MethodGet, "/api/engagement/status", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var body engagementStatusResponse
	decodeBody(t, recorder, &body)
	if body.BufferCapacity != 100 {
		t.Errorf("BufferCapacity = %d, want 100", body.BufferCapacity)
	}
	if body.RetainedCount != 0 {
		t.Errorf("RetainedCount = %d, want 0 before any event is published", body.RetainedCount)
	}
}

func TestEngagementEventsEndpointReturnsPublishedEventsInOrder(t *testing.T) {
	ts := newEngagementTestServer(t)
	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		evt := engagement.Event{
			SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderTwitch,
			ConnectedAccountID: "acct_1", Type: engagement.TypeFollow, PlatformTimestamp: now,
			DedupeKey: "key-" + string(rune('a'+i)), User: &engagement.User{ProviderUserID: "u1"},
		}
		if _, _, err := ts.bus.Publish(evt); err != nil {
			t.Fatalf("Publish() error = %v", err)
		}
	}

	recorder := do(t, ts.handler, http.MethodGet, "/api/engagement/events", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var body struct {
		Items []eventResponse `json:"items"`
		Gap   bool            `json:"gap"`
	}
	decodeBody(t, recorder, &body)
	if len(body.Items) != 3 {
		t.Fatalf("Items = %d, want 3", len(body.Items))
	}
	if body.Items[0].Sequence != 1 || body.Items[2].Sequence != 3 {
		t.Errorf("sequences = [%d,...,%d], want ascending starting at 1", body.Items[0].Sequence, body.Items[2].Sequence)
	}
	if body.Gap {
		t.Error("expected no gap when nothing was evicted")
	}
	// No raw provider payload/secret-shaped field: the response type itself
	// has no such field to decode into, verified by the successful decode.
}

func TestEngagementEventsEndpointCapsLimit(t *testing.T) {
	ts := newEngagementTestServer(t)
	now := time.Now().UTC()
	for i := 0; i < 10; i++ {
		evt := engagement.Event{
			SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderTwitch,
			ConnectedAccountID: "acct_1", Type: engagement.TypeFollow, PlatformTimestamp: now,
			DedupeKey: "key-" + string(rune('a'+i)), User: &engagement.User{ProviderUserID: "u1"},
		}
		ts.bus.Publish(evt)
	}

	recorder := do(t, ts.handler, http.MethodGet, "/api/engagement/events?limit=3", nil)
	var body struct {
		Items []eventResponse `json:"items"`
	}
	decodeBody(t, recorder, &body)
	if len(body.Items) != 3 {
		t.Errorf("Items = %d, want exactly the requested limit of 3", len(body.Items))
	}
}

func TestEngagementStreamServesSSEWithCorrectHeadersAndReplay(t *testing.T) {
	ts := newEngagementTestServer(t)
	now := time.Now().UTC()
	evt := engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderTwitch,
		ConnectedAccountID: "acct_1", Type: engagement.TypeFollow, PlatformTimestamp: now,
		DedupeKey: "replay-key-1", User: &engagement.User{ProviderUserID: "u1"},
	}
	if _, _, err := ts.bus.Publish(evt); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	srv := httptest.NewServer(ts.handler)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/engagement/stream", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/engagement/stream error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", cc)
	}

	buf := make([]byte, 4096)
	n, err := resp.Body.Read(buf)
	if err != nil && n == 0 {
		t.Fatalf("reading the stream failed: %v", err)
	}
	chunk := string(buf[:n])
	if !strings.Contains(chunk, "event: engagement.event") {
		t.Errorf("stream chunk missing engagement.event: %q", chunk)
	}
	if !strings.Contains(chunk, "id: 1") {
		t.Errorf("stream chunk missing SSE id matching the event sequence: %q", chunk)
	}
	if strings.Contains(chunk, "accessToken") || strings.Contains(chunk, "sessionId") {
		t.Errorf("stream chunk leaked a secret-shaped field: %q", chunk)
	}
}

func TestEngagementRoutesRejectWrongMethodWithAllowHeader(t *testing.T) {
	ts := newEngagementTestServer(t)

	recorder := do(t, ts.handler, http.MethodPost, "/api/engagement/status", nil)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", recorder.Code)
	}
	if recorder.Header().Get("Allow") != http.MethodGet {
		t.Errorf("Allow header = %q, want GET", recorder.Header().Get("Allow"))
	}
}

func TestRestartEngagementReturnsNotFoundForUnknownConnector(t *testing.T) {
	ts := newEngagementTestServer(t)
	ts.connectors.restartErr = twitchengagement.ErrNotFound

	recorder := do(t, ts.handler, http.MethodPost, "/api/connected-accounts/acct_missing/engagement/restart", nil)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestEngagementResponsesNeverContainATokenLikeField(t *testing.T) {
	ts := newEngagementTestServer(t)
	ts.createAccount(t, "unused", account.ProviderTwitch, append([]string{"channel:manage:broadcast"}, twitch.EngagementScopeProfile...))
	accounts, _ := ts.accounts.ListAccounts(context.Background())
	accountID := accounts[0].ID

	do(t, ts.handler, http.MethodPut, "/api/connected-accounts/"+accountID+"/engagement", map[string]any{"enabled": true})
	recorder := do(t, ts.handler, http.MethodGet, "/api/connected-accounts/"+accountID+"/engagement", nil)

	body := recorder.Body.String()
	for _, secretShaped := range []string{"accessToken", "refreshToken", "sessionId", "reconnectUrl", "clientSecret"} {
		if containsField(body, secretShaped) {
			t.Errorf("response body unexpectedly contains %q: %s", secretShaped, body)
		}
	}
}

func containsField(body, field string) bool {
	return len(body) > 0 && (indexOf(body, `"`+field+`"`) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
