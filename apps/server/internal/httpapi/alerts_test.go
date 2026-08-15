package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/alerts"
	"github.com/streaming-tree/server/internal/domain/account"
	"github.com/streaming-tree/server/internal/domain/engagement"
	bus "github.com/streaming-tree/server/internal/engagement"
	"github.com/streaming-tree/server/internal/secrets/secretstest"
	"github.com/streaming-tree/server/internal/storage/sqlite"
)

type alertsTestServer struct {
	handler  http.Handler
	accounts *account.Service
	bus      *bus.Bus
	manager  *alerts.Manager
}

func newAlertsTestServer(t *testing.T) *alertsTestServer {
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

	provider := fakeHTTPProvider{}
	accounts := account.NewService(account.Options{
		Repository: sqlite.NewAccountRepository(db.DB), Secrets: secretstest.New(),
		Providers:      map[account.ProviderID]account.Provider{account.ProviderTwitch: provider},
		RequiredScopes: map[account.ProviderID][]string{account.ProviderTwitch: {"channel:manage:broadcast"}},
		Logger:         logger,
	})

	eventBus := bus.New(bus.Options{Capacity: 100})
	t.Cleanup(eventBus.Shutdown)

	domainSvc := alerts.NewDomainService(sqlite.NewAlertsRepository(db.DB), accounts, nil, nil)
	visualDesignSvc := alerts.NewVisualDesignService(sqlite.NewVisualDesignRepository(db.DB))
	manager := alerts.NewManager(alerts.ManagerOptions{DomainService: domainSvc, VisualDesignService: visualDesignSvc, Bus: eventBus})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("manager.Start() error = %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = manager.Shutdown(ctx)
	})

	handler := NewRouter(Options{
		Logger: logger, StartedAt: time.Now(), Accounts: accounts, Alerts: manager,
	})

	return &alertsTestServer{handler: handler, accounts: accounts, bus: eventBus, manager: manager}
}

func (ts *alertsTestServer) createAccount(t *testing.T, id string) account.Account {
	t.Helper()
	acc, err := ts.accounts.FinalizeConnection(context.Background(), account.ProviderTwitch,
		account.Identity{ProviderUserID: id + "_provider_user", Login: "viewer_" + id, DisplayName: "Viewer " + id},
		account.TokenBundle{TokenType: "bearer", AccessToken: "fake-access", RefreshToken: "fake-refresh", ExpiresAt: time.Now().Add(time.Hour)},
		[]string{"channel:manage:broadcast"}, "")
	if err != nil {
		t.Fatalf("createAccount() error = %v", err)
	}
	return acc
}

func (ts *alertsTestServer) do(t *testing.T, method, path string, body any) *http.Response {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}
		reader = bytes.NewReader(encoded)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	ts.handler.ServeHTTP(rec, req)
	return rec.Result()
}

func decodeAlertsBody(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	return out
}

func validFollowRuleBody() map[string]any {
	return map[string]any{
		"name": "Follow alert", "enabled": true, "eventType": "follow", "priority": 50, "durationMs": 5000,
		"requiredRole": "everyone", "showPlatform": true, "showUsername": true,
		"textTemplate": "{username} just followed!", "entryAnimation": "fade", "exitAnimation": "fade",
		"animationDurationMs": 400, "providers": []string{}, "accounts": []string{},
		"allowGrouping": false, "groupWindowMs": 5000, "interruptMode": "never", "interruptible": true,
	}
}

// --- Stage 17B: rule audio (docs/alert-audio.md §6/§7) -------------------

func TestCreateAlertRuleRoundTripsAudio(t *testing.T) {
	ts := newAlertsTestServer(t)
	profileID := createTestProfile(t, ts)

	body := validFollowRuleBody()
	body["audio"] = map[string]any{
		"soundEnabled": true, "soundAssetId": "audioasset_abc", "soundVolume": 0.75,
		"ttsEnabled": true, "ttsTemplate": "{username} just followed!", "ttsVolume": 0.5,
	}
	resp := ts.do(t, http.MethodPost, "/api/alert-profiles/"+profileID+"/rules", body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201, body = %+v", resp.StatusCode, decodeAlertsBody(t, resp))
	}
	created := decodeAlertsBody(t, resp)
	audio, ok := created["audio"].(map[string]any)
	if !ok {
		t.Fatalf("response has no audio object: %+v", created)
	}
	if audio["soundEnabled"] != true || audio["soundAssetId"] != "audioasset_abc" || audio["soundVolume"] != 0.75 {
		t.Errorf("audio sound fields = %+v, want soundEnabled=true soundAssetId=audioasset_abc soundVolume=0.75", audio)
	}
	if audio["ttsEnabled"] != true || audio["ttsTemplate"] != "{username} just followed!" || audio["ttsVolume"] != 0.5 {
		t.Errorf("audio TTS fields = %+v, want ttsEnabled=true ttsTemplate={username} just followed! ttsVolume=0.5", audio)
	}

	id := created["id"].(string)
	getResp := ts.do(t, http.MethodGet, "/api/alert-rules/"+id, nil)
	got := decodeAlertsBody(t, getResp)
	if gotAudio, _ := got["audio"].(map[string]any); gotAudio["soundAssetId"] != "audioasset_abc" {
		t.Errorf("GET audio = %+v, want soundAssetId=audioasset_abc", gotAudio)
	}
}

func TestCreateAlertRuleOmittedAudioDefaultsToNoAudio(t *testing.T) {
	ts := newAlertsTestServer(t)
	profileID := createTestProfile(t, ts)

	// validFollowRuleBody() never sets "audio" at all - the pre-Stage-17B
	// request shape - and must still succeed with the safe zero value
	// (docs/alert-audio.md §6.5).
	resp := ts.do(t, http.MethodPost, "/api/alert-profiles/"+profileID+"/rules", validFollowRuleBody())
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201, body = %+v", resp.StatusCode, decodeAlertsBody(t, resp))
	}
	audio, _ := decodeAlertsBody(t, resp)["audio"].(map[string]any)
	if audio["soundEnabled"] != false || audio["ttsEnabled"] != false {
		t.Errorf("audio = %+v, want soundEnabled=false ttsEnabled=false", audio)
	}
}

func TestCreateAlertRuleRejectsSoundEnabledWithoutAsset(t *testing.T) {
	ts := newAlertsTestServer(t)
	profileID := createTestProfile(t, ts)

	body := validFollowRuleBody()
	body["audio"] = map[string]any{"soundEnabled": true, "soundVolume": 1.0}
	resp := ts.do(t, http.MethodPost, "/api/alert-profiles/"+profileID+"/rules", body)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %+v", resp.StatusCode, decodeAlertsBody(t, resp))
	}
}

func TestCreateAlertRuleRejectsGroupCountInTTSTemplate(t *testing.T) {
	ts := newAlertsTestServer(t)
	profileID := createTestProfile(t, ts)

	body := validFollowRuleBody()
	body["audio"] = map[string]any{
		"ttsEnabled": true, "ttsTemplate": "{username} followed, group of {groupCount}!", "ttsVolume": 1.0,
	}
	resp := ts.do(t, http.MethodPost, "/api/alert-profiles/"+profileID+"/rules", body)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %+v", resp.StatusCode, decodeAlertsBody(t, resp))
	}
}

func TestCreateAlertRuleRejectsUnknownTTSPlaceholder(t *testing.T) {
	ts := newAlertsTestServer(t)
	profileID := createTestProfile(t, ts)

	body := validFollowRuleBody()
	body["audio"] = map[string]any{"ttsEnabled": true, "ttsTemplate": "{notARealPlaceholder}", "ttsVolume": 1.0}
	resp := ts.do(t, http.MethodPost, "/api/alert-profiles/"+profileID+"/rules", body)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %+v", resp.StatusCode, decodeAlertsBody(t, resp))
	}
}

// --- profiles ---------------------------------------------------------

func TestCreateAlertProfileThenGet(t *testing.T) {
	ts := newAlertsTestServer(t)

	resp := ts.do(t, http.MethodPost, "/api/alert-profiles", map[string]any{"name": "Main"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", resp.StatusCode)
	}
	if resp.Header.Get("Location") == "" {
		t.Error("Location header missing on create")
	}
	body := decodeAlertsBody(t, resp)
	id, _ := body["id"].(string)
	slug, _ := body["publicSlug"].(string)
	if id == "" || slug == "" {
		t.Fatalf("created profile missing id/publicSlug: %+v", body)
	}

	getResp := ts.do(t, http.MethodGet, "/api/alert-profiles/"+id, nil)
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("get status = %d, want 200", getResp.StatusCode)
	}
}

func TestGetUnknownAlertProfileReturns404(t *testing.T) {
	ts := newAlertsTestServer(t)
	resp := ts.do(t, http.MethodGet, "/api/alert-profiles/alprof_missing", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	body := decodeAlertsBody(t, resp)
	if body["error"] != "alert_profile_not_found" {
		t.Errorf("error = %v, want alert_profile_not_found", body["error"])
	}
}

func TestListAlertProfilesNeverReturnsNull(t *testing.T) {
	ts := newAlertsTestServer(t)
	resp := ts.do(t, http.MethodGet, "/api/alert-profiles", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	defer resp.Body.Close()
	var raw json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if string(raw) != "[]" {
		t.Errorf("body = %s, want []", raw)
	}
}

func TestUpdateAlertProfile(t *testing.T) {
	ts := newAlertsTestServer(t)
	created := decodeAlertsBody(t, ts.do(t, http.MethodPost, "/api/alert-profiles", map[string]any{"name": "Main"}))
	id := created["id"].(string)

	update := map[string]any{
		"name": "Renamed", "enabled": false, "language": "pl", "theme": "compact", "position": "top",
		"textAlign": "left", "maxQueueItems": 50, "maximumQueueAgeSeconds": 60,
	}
	resp := ts.do(t, http.MethodPut, "/api/alert-profiles/"+id, update)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update status = %d, want 200", resp.StatusCode)
	}
	body := decodeAlertsBody(t, resp)
	if body["name"] != "Renamed" || body["enabled"] != false || body["language"] != "pl" {
		t.Errorf("updated profile = %+v", body)
	}
}

func TestDeleteAlertProfile(t *testing.T) {
	ts := newAlertsTestServer(t)
	created := decodeAlertsBody(t, ts.do(t, http.MethodPost, "/api/alert-profiles", map[string]any{"name": "Main"}))
	id := created["id"].(string)

	resp := ts.do(t, http.MethodDelete, "/api/alert-profiles/"+id, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", resp.StatusCode)
	}
	getResp := ts.do(t, http.MethodGet, "/api/alert-profiles/"+id, nil)
	if getResp.StatusCode != http.StatusNotFound {
		t.Fatalf("get after delete status = %d, want 404", getResp.StatusCode)
	}
}

func TestRotateAlertProfilePublicSlugInvalidatesOld(t *testing.T) {
	ts := newAlertsTestServer(t)
	created := decodeAlertsBody(t, ts.do(t, http.MethodPost, "/api/alert-profiles", map[string]any{"name": "Main"}))
	id := created["id"].(string)
	oldSlug := created["publicSlug"].(string)

	resp := ts.do(t, http.MethodPost, "/api/alert-profiles/"+id+"/rotate-public-slug", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rotate status = %d, want 200", resp.StatusCode)
	}
	newBody := decodeAlertsBody(t, resp)
	newSlug := newBody["publicSlug"].(string)
	if newSlug == oldSlug {
		t.Error("public slug unchanged after rotation")
	}

	oldConfig := ts.do(t, http.MethodGet, "/api/public/alert-profiles/"+oldSlug+"/config", nil)
	oldBody := decodeAlertsBody(t, oldConfig)
	// The old slug never resolves to a hard error (Part 40) - it falls
	// back to the same safe default config an unknown slug gets.
	if oldBody["theme"] != string("minimal") {
		t.Errorf("old slug config = %+v, want the safe default fallback", oldBody)
	}

	newConfig := ts.do(t, http.MethodGet, "/api/public/alert-profiles/"+newSlug+"/config", nil)
	if newConfig.StatusCode != http.StatusOK {
		t.Fatalf("new slug config status = %d, want 200", newConfig.StatusCode)
	}
}

func TestRotatePublicSlugRejectsBody(t *testing.T) {
	ts := newAlertsTestServer(t)
	created := decodeAlertsBody(t, ts.do(t, http.MethodPost, "/api/alert-profiles", map[string]any{"name": "Main"}))
	id := created["id"].(string)

	resp := ts.do(t, http.MethodPost, "/api/alert-profiles/"+id+"/rotate-public-slug", map[string]any{"unexpected": true})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("rotate with a body status = %d, want 400", resp.StatusCode)
	}
}

// --- rules --------------------------------------------------------------

func createTestProfile(t *testing.T, ts *alertsTestServer) string {
	t.Helper()
	created := decodeAlertsBody(t, ts.do(t, http.MethodPost, "/api/alert-profiles", map[string]any{"name": "Main"}))
	return created["id"].(string)
}

func TestCreateAlertRuleThenGet(t *testing.T) {
	ts := newAlertsTestServer(t)
	profileID := createTestProfile(t, ts)

	resp := ts.do(t, http.MethodPost, "/api/alert-profiles/"+profileID+"/rules", validFollowRuleBody())
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201, body = %+v", resp.StatusCode, decodeAlertsBody(t, resp))
	}
	body := decodeAlertsBody(t, resp)
	id := body["id"].(string)

	getResp := ts.do(t, http.MethodGet, "/api/alert-rules/"+id, nil)
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("get status = %d, want 200", getResp.StatusCode)
	}
}

func TestCreateAlertRuleRoundTripsMoneyFields(t *testing.T) {
	ts := newAlertsTestServer(t)
	profileID := createTestProfile(t, ts)

	body := validFollowRuleBody()
	body["eventType"] = "youtube_super_chat"
	body["textTemplate"] = "{username} sent {amount} {currency}"
	body["showAmount"] = true
	body["currency"] = "usd"
	body["minimumAmountMicros"] = 1_000_000
	body["maximumAmountMicros"] = 50_000_000

	resp := ts.do(t, http.MethodPost, "/api/alert-profiles/"+profileID+"/rules", body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201, body = %+v", resp.StatusCode, decodeAlertsBody(t, resp))
	}
	created := decodeAlertsBody(t, resp)
	if created["showAmount"] != true {
		t.Errorf("created showAmount = %v, want true", created["showAmount"])
	}
	if created["currency"] != "USD" {
		t.Errorf("created currency = %v, want USD (normalized uppercase)", created["currency"])
	}
	if created["minimumAmountMicros"] != float64(1_000_000) {
		t.Errorf("created minimumAmountMicros = %v, want 1000000", created["minimumAmountMicros"])
	}
	if created["maximumAmountMicros"] != float64(50_000_000) {
		t.Errorf("created maximumAmountMicros = %v, want 50000000", created["maximumAmountMicros"])
	}

	id := created["id"].(string)
	getResp := ts.do(t, http.MethodGet, "/api/alert-rules/"+id, nil)
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("get status = %d, want 200", getResp.StatusCode)
	}
	fetched := decodeAlertsBody(t, getResp)
	if fetched["currency"] != "USD" || fetched["minimumAmountMicros"] != float64(1_000_000) {
		t.Errorf("fetched rule money fields = %+v, want currency=USD minimumAmountMicros=1000000", fetched)
	}
}

func TestCreateAlertRuleRejectsAmountThresholdWithoutCurrency(t *testing.T) {
	ts := newAlertsTestServer(t)
	profileID := createTestProfile(t, ts)

	body := validFollowRuleBody()
	body["eventType"] = "youtube_super_chat"
	body["minimumAmountMicros"] = 1_000_000
	resp := ts.do(t, http.MethodPost, "/api/alert-profiles/"+profileID+"/rules", body)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 (amount threshold requires a currency), body = %+v", resp.StatusCode, decodeAlertsBody(t, resp))
	}
	respBody := decodeAlertsBody(t, resp)
	if respBody["error"] != "alert_rule_amount_invalid" {
		t.Errorf("error = %v, want alert_rule_amount_invalid", respBody["error"])
	}
}

func TestCreateAlertRuleRejectsUnsupportedCondition(t *testing.T) {
	ts := newAlertsTestServer(t)
	profileID := createTestProfile(t, ts)

	body := validFollowRuleBody()
	body["minimumQuantity"] = 10
	resp := ts.do(t, http.MethodPost, "/api/alert-profiles/"+profileID+"/rules", body)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", resp.StatusCode)
	}
	respBody := decodeAlertsBody(t, resp)
	if respBody["error"] != "alert_rule_condition_unsupported" {
		t.Errorf("error = %v, want alert_rule_condition_unsupported", respBody["error"])
	}
}

func TestCreateAlertRuleRejectsUnknownPlaceholder(t *testing.T) {
	ts := newAlertsTestServer(t)
	profileID := createTestProfile(t, ts)

	body := validFollowRuleBody()
	body["textTemplate"] = "{bogus}"
	resp := ts.do(t, http.MethodPost, "/api/alert-profiles/"+profileID+"/rules", body)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", resp.StatusCode)
	}
	respBody := decodeAlertsBody(t, resp)
	if respBody["error"] != "alert_template_invalid" {
		t.Errorf("error = %v, want alert_template_invalid", respBody["error"])
	}
}

func TestCreateAlertRuleRejectsUnknownAccount(t *testing.T) {
	ts := newAlertsTestServer(t)
	profileID := createTestProfile(t, ts)

	body := validFollowRuleBody()
	body["accounts"] = []string{"acct_missing"}
	resp := ts.do(t, http.MethodPost, "/api/alert-profiles/"+profileID+"/rules", body)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	respBody := decodeAlertsBody(t, resp)
	if respBody["error"] != "alert_rule_account_not_found" {
		t.Errorf("error = %v, want alert_rule_account_not_found", respBody["error"])
	}
}

func TestCreateAlertRuleRejectsGroupingForUngroupableEventType(t *testing.T) {
	ts := newAlertsTestServer(t)
	profileID := createTestProfile(t, ts)

	body := validFollowRuleBody() // follow is not groupable
	body["allowGrouping"] = true
	resp := ts.do(t, http.MethodPost, "/api/alert-profiles/"+profileID+"/rules", body)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %+v", resp.StatusCode, decodeAlertsBody(t, resp))
	}
	respBody := decodeAlertsBody(t, resp)
	if respBody["error"] != "alert_rule_condition_unsupported" {
		t.Errorf("error = %v, want alert_rule_condition_unsupported", respBody["error"])
	}
}

func TestCreateAlertRuleRejectsGroupingWithMessageShown(t *testing.T) {
	ts := newAlertsTestServer(t)
	profileID := createTestProfile(t, ts)

	body := validFollowRuleBody()
	body["eventType"] = "bits"
	body["showQuantity"] = true
	body["showMessage"] = true
	body["allowGrouping"] = true
	resp := ts.do(t, http.MethodPost, "/api/alert-profiles/"+profileID+"/rules", body)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %+v", resp.StatusCode, decodeAlertsBody(t, resp))
	}
}

func TestCreateAlertRuleRejectsGroupingWithMessagePlaceholder(t *testing.T) {
	ts := newAlertsTestServer(t)
	profileID := createTestProfile(t, ts)

	body := validFollowRuleBody()
	body["eventType"] = "bits"
	body["showQuantity"] = true
	body["showMessage"] = false
	body["allowGrouping"] = true
	body["textTemplate"] = "{username} cheered: {message}"
	resp := ts.do(t, http.MethodPost, "/api/alert-profiles/"+profileID+"/rules", body)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %+v", resp.StatusCode, decodeAlertsBody(t, resp))
	}
	respBody := decodeAlertsBody(t, resp)
	if respBody["error"] != "alert_template_invalid" {
		t.Errorf("error = %v, want alert_template_invalid (a {message} reference is unsafe once grouping is enabled)", respBody["error"])
	}
}

func TestCreateAlertRuleAcceptsGroupingAndInterruptFields(t *testing.T) {
	ts := newAlertsTestServer(t)
	profileID := createTestProfile(t, ts)

	body := validFollowRuleBody()
	body["eventType"] = "bits"
	body["showQuantity"] = true
	body["showMessage"] = false
	body["allowGrouping"] = true
	body["groupWindowMs"] = 8000
	body["interruptMode"] = "lower_priority"
	body["interruptible"] = false
	body["textTemplate"] = "{username} cheered {quantity} bits (x{groupCount})"
	resp := ts.do(t, http.MethodPost, "/api/alert-profiles/"+profileID+"/rules", body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %+v", resp.StatusCode, decodeAlertsBody(t, resp))
	}
	respBody := decodeAlertsBody(t, resp)
	if respBody["allowGrouping"] != true {
		t.Errorf("allowGrouping = %v, want true", respBody["allowGrouping"])
	}
	if respBody["groupWindowMs"] != float64(8000) {
		t.Errorf("groupWindowMs = %v, want 8000", respBody["groupWindowMs"])
	}
	if respBody["interruptMode"] != "lower_priority" {
		t.Errorf("interruptMode = %v, want lower_priority", respBody["interruptMode"])
	}
	if respBody["interruptible"] != false {
		t.Errorf("interruptible = %v, want false", respBody["interruptible"])
	}
}

// TestCreateAlertRuleDefaultsGroupingAndInterruptFieldsWhenOmitted
// guards a real Stage 12A/12B backward-compatibility bug found by
// scripts/verify-alerts.mjs's own unchanged ruleBody(), which - being
// written before Stage 12B existed - never sends allowGrouping/
// groupWindowMs/interruptMode/interruptible at all. Before this was
// fixed, an omitted groupWindowMs unmarshaled to Go's int zero value
// (0), which then failed GroupWindowMS's own unconditional [1000,
// 30000] bound, and an omitted interruptible unmarshaled to false
// rather than the documented safe default (true) - silently making
// every rule created by a pre-Stage-12B client both un-creatable and,
// had the bound not caught it first, non-interruptible by default.
func TestCreateAlertRuleDefaultsGroupingAndInterruptFieldsWhenOmitted(t *testing.T) {
	ts := newAlertsTestServer(t)
	profileID := createTestProfile(t, ts)

	body := map[string]any{
		"name": "Follow alert", "enabled": true, "eventType": "follow", "priority": 50, "durationMs": 5000,
		"requiredRole": "everyone", "showPlatform": true, "showUsername": true,
		"textTemplate": "{username} just followed!", "entryAnimation": "fade", "exitAnimation": "fade",
		"animationDurationMs": 400, "providers": []string{}, "accounts": []string{},
		// allowGrouping/groupWindowMs/interruptMode/interruptible deliberately omitted.
	}
	resp := ts.do(t, http.MethodPost, "/api/alert-profiles/"+profileID+"/rules", body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %+v", resp.StatusCode, decodeAlertsBody(t, resp))
	}
	respBody := decodeAlertsBody(t, resp)
	if respBody["allowGrouping"] != false {
		t.Errorf("allowGrouping = %v, want false", respBody["allowGrouping"])
	}
	if respBody["groupWindowMs"] != float64(5000) {
		t.Errorf("groupWindowMs = %v, want the default 5000", respBody["groupWindowMs"])
	}
	if respBody["interruptMode"] != "never" {
		t.Errorf("interruptMode = %v, want never", respBody["interruptMode"])
	}
	if respBody["interruptible"] != true {
		t.Errorf("interruptible = %v, want true (the Stage-12A-preserving safe default)", respBody["interruptible"])
	}
}

func TestListAlertEventTypesExposesGroupingCapability(t *testing.T) {
	ts := newAlertsTestServer(t)
	resp := ts.do(t, http.MethodGet, "/api/alert-event-types", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var out []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	found := map[string]bool{}
	for _, entry := range out {
		found[entry["eventType"].(string)] = entry["groupable"].(bool)
	}
	want := map[string]bool{
		"follow": false, "subscription": false, "resubscription": false, "gifted_subscription": false,
		"subscription_gift_batch": true, "bits": true, "raid": false, "channel_point_redemption": true,
	}
	for eventType, groupable := range want {
		if found[eventType] != groupable {
			t.Errorf("eventType %q groupable = %v, want %v", eventType, found[eventType], groupable)
		}
	}
}

func TestCreateAlertRuleAcceptsKnownAccount(t *testing.T) {
	ts := newAlertsTestServer(t)
	acc := ts.createAccount(t, "a1")
	profileID := createTestProfile(t, ts)

	body := validFollowRuleBody()
	body["accounts"] = []string{acc.ID}
	resp := ts.do(t, http.MethodPost, "/api/alert-profiles/"+profileID+"/rules", body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %+v", resp.StatusCode, decodeAlertsBody(t, resp))
	}
}

func TestListAlertRulesIncludesOverlapWarning(t *testing.T) {
	ts := newAlertsTestServer(t)
	profileID := createTestProfile(t, ts)

	bits := func(minQty any) map[string]any {
		b := validFollowRuleBody()
		b["eventType"] = "bits"
		b["textTemplate"] = "{username} cheered {quantity}"
		b["showQuantity"] = true
		if minQty != nil {
			b["minimumQuantity"] = minQty
		}
		return b
	}
	ts.do(t, http.MethodPost, "/api/alert-profiles/"+profileID+"/rules", bits(nil))
	ts.do(t, http.MethodPost, "/api/alert-profiles/"+profileID+"/rules", bits(1))

	resp := ts.do(t, http.MethodGet, "/api/alert-profiles/"+profileID+"/rules", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body := decodeAlertsBody(t, resp)
	warnings, _ := body["overlapWarnings"].([]any)
	if len(warnings) != 1 {
		t.Fatalf("overlapWarnings = %+v, want exactly one warning", body["overlapWarnings"])
	}
}

func TestDeleteAlertRule(t *testing.T) {
	ts := newAlertsTestServer(t)
	profileID := createTestProfile(t, ts)
	created := decodeAlertsBody(t, ts.do(t, http.MethodPost, "/api/alert-profiles/"+profileID+"/rules", validFollowRuleBody()))
	id := created["id"].(string)

	resp := ts.do(t, http.MethodDelete, "/api/alert-rules/"+id, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", resp.StatusCode)
	}
	getResp := ts.do(t, http.MethodGet, "/api/alert-rules/"+id, nil)
	if getResp.StatusCode != http.StatusNotFound {
		t.Fatalf("get after delete status = %d, want 404", getResp.StatusCode)
	}
}

// --- rule test / preview -------------------------------------------------

func TestTestAlertRuleEndpoint(t *testing.T) {
	ts := newAlertsTestServer(t)
	profileID := createTestProfile(t, ts)
	created := decodeAlertsBody(t, ts.do(t, http.MethodPost, "/api/alert-profiles/"+profileID+"/rules", validFollowRuleBody()))
	id := created["id"].(string)

	resp := ts.do(t, http.MethodPost, "/api/alert-rules/"+id+"/test", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %+v", resp.StatusCode, decodeAlertsBody(t, resp))
	}
	body := decodeAlertsBody(t, resp)
	if body["synthetic"] != true {
		t.Errorf("synthetic = %v, want true", body["synthetic"])
	}

	queueResp := ts.do(t, http.MethodGet, "/api/alert-profiles/"+profileID+"/queue", nil)
	queueBody := decodeAlertsBody(t, queueResp)
	if queueBody["totalSynthetic"].(float64) != 1 {
		t.Errorf("totalSynthetic = %v, want 1", queueBody["totalSynthetic"])
	}
}

func TestTestAlertRuleRejectsDisabledProfile(t *testing.T) {
	ts := newAlertsTestServer(t)
	profileID := createTestProfile(t, ts)
	created := decodeAlertsBody(t, ts.do(t, http.MethodPost, "/api/alert-profiles/"+profileID+"/rules", validFollowRuleBody()))
	ruleID := created["id"].(string)

	ts.do(t, http.MethodPut, "/api/alert-profiles/"+profileID, map[string]any{
		"name": "Main", "enabled": false, "language": "en", "theme": "minimal", "position": "bottom",
		"textAlign": "center", "maxQueueItems": 100, "maximumQueueAgeSeconds": 120,
	})

	resp := ts.do(t, http.MethodPost, "/api/alert-rules/"+ruleID+"/test", nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
	body := decodeAlertsBody(t, resp)
	if body["error"] != "alert_profile_disabled" {
		t.Errorf("error = %v, want alert_profile_disabled", body["error"])
	}
}

func TestAlertRulePreviewNeverTouchesQueue(t *testing.T) {
	ts := newAlertsTestServer(t)
	profileID := createTestProfile(t, ts)
	created := decodeAlertsBody(t, ts.do(t, http.MethodPost, "/api/alert-profiles/"+profileID+"/rules", validFollowRuleBody()))
	_ = created

	resp := ts.do(t, http.MethodPost, "/api/alert-rule-preview", map[string]any{
		"eventType": "follow", "template": "{username} just followed on {platform}!",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body := decodeAlertsBody(t, resp)
	if body["renderedText"] == "" {
		t.Error("renderedText is empty")
	}

	queueResp := ts.do(t, http.MethodGet, "/api/alert-profiles/"+profileID+"/queue", nil)
	queueBody := decodeAlertsBody(t, queueResp)
	if queueBody["totalEnqueued"].(float64) != 0 {
		t.Errorf("totalEnqueued = %v after a preview, want 0 (preview never touches the queue)", queueBody["totalEnqueued"])
	}
}

func TestAlertRulePreviewRejectsUnsupportedPlaceholder(t *testing.T) {
	ts := newAlertsTestServer(t)
	resp := ts.do(t, http.MethodPost, "/api/alert-rule-preview", map[string]any{
		"eventType": "follow", "template": "{quantity}",
	})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", resp.StatusCode)
	}
}

// --- queue commands -------------------------------------------------------

func TestAlertQueuePauseResume(t *testing.T) {
	ts := newAlertsTestServer(t)
	profileID := createTestProfile(t, ts)

	resp := ts.do(t, http.MethodPost, "/api/alert-profiles/"+profileID+"/queue/pause", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pause status = %d, want 200", resp.StatusCode)
	}
	body := decodeAlertsBody(t, resp)
	if body["paused"] != true {
		t.Errorf("paused = %v after pause, want true", body["paused"])
	}

	resp = ts.do(t, http.MethodPost, "/api/alert-profiles/"+profileID+"/queue/resume", nil)
	body = decodeAlertsBody(t, resp)
	if body["paused"] != false {
		t.Errorf("paused = %v after resume, want false", body["paused"])
	}
}

func TestAlertQueueSkipCurrentEmptyReturns409(t *testing.T) {
	ts := newAlertsTestServer(t)
	profileID := createTestProfile(t, ts)

	resp := ts.do(t, http.MethodPost, "/api/alert-profiles/"+profileID+"/queue/skip-current", nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
	body := decodeAlertsBody(t, resp)
	if body["error"] != "alert_queue_empty" {
		t.Errorf("error = %v, want alert_queue_empty", body["error"])
	}
}

func TestAlertQueueReplayPreviousEmptyReturns409(t *testing.T) {
	ts := newAlertsTestServer(t)
	profileID := createTestProfile(t, ts)

	resp := ts.do(t, http.MethodPost, "/api/alert-profiles/"+profileID+"/queue/replay-previous", nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
}

func TestAlertQueueClear(t *testing.T) {
	ts := newAlertsTestServer(t)
	profileID := createTestProfile(t, ts)

	resp := ts.do(t, http.MethodPost, "/api/alert-profiles/"+profileID+"/queue/clear", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestAlertQueueCommandsRejectBody(t *testing.T) {
	ts := newAlertsTestServer(t)
	profileID := createTestProfile(t, ts)

	resp := ts.do(t, http.MethodPost, "/api/alert-profiles/"+profileID+"/queue/pause", map[string]any{"x": true})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

// --- method/content-type/body validation ------------------------------

func TestAlertProfilesMethodNotAllowed(t *testing.T) {
	ts := newAlertsTestServer(t)
	resp := ts.do(t, http.MethodPatch, "/api/alert-profiles", nil)
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
	if resp.Header.Get("Allow") == "" {
		t.Error("Allow header missing on 405")
	}
}

func TestCreateAlertProfileMalformedJSON(t *testing.T) {
	ts := newAlertsTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/alert-profiles", bytes.NewReader([]byte("{not json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	ts.handler.ServeHTTP(rec, req)
	if rec.Result().StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Result().StatusCode)
	}
}

func TestCreateAlertProfileUnknownField(t *testing.T) {
	ts := newAlertsTestServer(t)
	resp := ts.do(t, http.MethodPost, "/api/alert-profiles", map[string]any{"name": "Main", "bogus": 1})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestCreateAlertProfileWrongContentType(t *testing.T) {
	ts := newAlertsTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/alert-profiles", bytes.NewReader([]byte(`{"name":"Main"}`)))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	ts.handler.ServeHTTP(rec, req)
	if rec.Result().StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415", rec.Result().StatusCode)
	}
}

// --- event types ----------------------------------------------------------

func TestListAlertEventTypesCapabilityDriven(t *testing.T) {
	ts := newAlertsTestServer(t)
	resp := ts.do(t, http.MethodGet, "/api/alert-event-types", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	defer resp.Body.Close()
	var list []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 13 {
		t.Fatalf("len(list) = %d, want 13", len(list))
	}
	for _, entry := range list {
		if entry["eventType"] == "follow" {
			if entry["hasQuantity"] != false {
				t.Errorf("follow hasQuantity = %v, want false", entry["hasQuantity"])
			}
		}
		if entry["eventType"] == "bits" {
			if entry["hasQuantity"] != true || entry["hasAnonymity"] != true {
				t.Errorf("bits capability = %+v, want hasQuantity/hasAnonymity true", entry)
			}
		}
		if entry["eventType"] == "youtube_super_chat" {
			if entry["hasAmount"] != true {
				t.Errorf("youtube_super_chat capability = %+v, want hasAmount true", entry)
			}
		}
		if entry["eventType"] == "youtube_membership" {
			if entry["hasMembershipLevel"] != true {
				t.Errorf("youtube_membership capability = %+v, want hasMembershipLevel true", entry)
			}
		}
	}
}

// --- public API -------------------------------------------------------

func TestPublicAlertConfigUnknownSlugNeverErrors(t *testing.T) {
	ts := newAlertsTestServer(t)
	resp := ts.do(t, http.MethodGet, "/api/public/alert-profiles/does-not-exist/config", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (never a hard error for an unknown slug)", resp.StatusCode)
	}
}

func TestPublicAlertConfigNoManagementData(t *testing.T) {
	ts := newAlertsTestServer(t)
	created := decodeAlertsBody(t, ts.do(t, http.MethodPost, "/api/alert-profiles", map[string]any{"name": "Main"}))
	slug := created["publicSlug"].(string)

	resp := ts.do(t, http.MethodGet, "/api/public/alert-profiles/"+slug+"/config", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body := decodeAlertsBody(t, resp)
	for _, forbidden := range []string{"id", "name", "publicSlug", "maxQueueItems", "maximumQueueAgeSeconds"} {
		if _, ok := body[forbidden]; ok {
			t.Errorf("public config leaked management-only field %q: %+v", forbidden, body)
		}
	}
}

// --- public SSE stream -------------------------------------------------

func waitUntilAlertsHTTP(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition never became true within timeout")
}

func TestPublicAlertStreamInitialResetThenShow(t *testing.T) {
	ts := newAlertsTestServer(t)
	acc := ts.createAccount(t, "a1")
	created := decodeAlertsBody(t, ts.do(t, http.MethodPost, "/api/alert-profiles", map[string]any{"name": "Main"}))
	profileID := created["id"].(string)
	slug := created["publicSlug"].(string)
	ruleBody := validFollowRuleBody()
	ruleBody["accounts"] = []string{acc.ID}
	ts.do(t, http.MethodPost, "/api/alert-profiles/"+profileID+"/rules", ruleBody)

	waitUntilAlertsHTTP(t, ts.manager.Subscribed)

	srv := httptest.NewServer(ts.handler)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/public/alert-profiles/"+slug+"/stream", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET .../stream error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	buf := make([]byte, 4096)
	n, err := resp.Body.Read(buf)
	if err != nil && n == 0 {
		t.Fatalf("reading the initial reset failed: %v", err)
	}
	if !strings.Contains(string(buf[:n]), "event: alert.reset") {
		t.Fatalf("first chunk missing alert.reset: %q", string(buf[:n]))
	}

	evt := engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderTwitch,
		ConnectedAccountID: acc.ID, Type: engagement.TypeFollow, PlatformTimestamp: time.Now().UTC(),
		DedupeKey: "dk_stream_test", User: &engagement.User{ProviderUserID: "u1", Login: "viewer", DisplayName: "Viewer"},
	}
	if _, _, err := ts.bus.Publish(evt); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}

	n, err = resp.Body.Read(buf)
	if err != nil && n == 0 {
		t.Fatalf("reading the live show failed: %v", err)
	}
	chunk := string(buf[:n])
	if !strings.Contains(chunk, "event: alert.show") {
		t.Errorf("stream chunk missing alert.show: %q", chunk)
	}
	if !strings.Contains(chunk, "Viewer just followed") {
		t.Errorf("stream chunk missing the rendered text: %q", chunk)
	}
}

func TestPublicAlertStreamUnknownSlugRendersEmptyNotError(t *testing.T) {
	ts := newAlertsTestServer(t)
	srv := httptest.NewServer(ts.handler)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/public/alert-profiles/does-not-exist/stream", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET .../stream error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (never a hard error for an unknown slug)", resp.StatusCode)
	}
	buf := make([]byte, 4096)
	n, err := resp.Body.Read(buf)
	if err != nil && n == 0 {
		t.Fatalf("reading the reset failed: %v", err)
	}
	if !strings.Contains(string(buf[:n]), "event: alert.reset") {
		t.Fatalf("chunk missing alert.reset: %q", string(buf[:n]))
	}
}
