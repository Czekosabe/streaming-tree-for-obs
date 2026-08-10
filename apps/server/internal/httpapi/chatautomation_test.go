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
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/chatautomation"
	"github.com/streaming-tree/server/internal/domain/account"
	"github.com/streaming-tree/server/internal/domain/operatorchatprefs"
	"github.com/streaming-tree/server/internal/domain/platform"
	bus "github.com/streaming-tree/server/internal/engagement"
	"github.com/streaming-tree/server/internal/outboundchat"
	"github.com/streaming-tree/server/internal/provider/twitch"
	"github.com/streaming-tree/server/internal/runtime/deviceflow"
	"github.com/streaming-tree/server/internal/secrets/secretstest"
	"github.com/streaming-tree/server/internal/storage/sqlite"
)

type fakeIngestChecker struct {
	receiving bool
}

func (f fakeIngestChecker) IsReceiving() bool { return f.receiving }
func (f fakeIngestChecker) ReceivingSince() (time.Time, bool) {
	if !f.receiving {
		return time.Time{}, false
	}
	return time.Now().UTC(), true
}

type chatAutomationTestServer struct {
	handler  http.Handler
	accounts *account.Service
	provider *fakeOutboundProvider
}

func newChatAutomationTestServer(t *testing.T, receiving bool) *chatAutomationTestServer {
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
	secretStore := secretstest.New()
	provider := fakeHTTPProvider{}
	accounts := account.NewService(account.Options{
		Repository: sqlite.NewAccountRepository(db.DB), Secrets: secretStore,
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

	outboundProvider := &fakeOutboundProvider{}
	outboundManager := outboundchat.NewManager(outboundchat.ManagerOptions{
		Accounts: accounts, Providers: []outboundchat.Provider{outboundProvider},
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = outboundManager.Shutdown(ctx)
	})

	prefsService := operatorchatprefs.NewService(sqlite.NewOperatorChatPrefsRepository(db.DB), nil, nil)

	eventBus := bus.New(bus.Options{Capacity: 100})
	t.Cleanup(eventBus.Shutdown)

	domainSvc := chatautomation.NewDomainService(sqlite.NewChatAutomationRepository(db.DB), accounts, platforms)
	automationManager := chatautomation.NewManager(chatautomation.ManagerOptions{
		DomainService: domainSvc, Outbound: outboundManager, Bus: eventBus,
		Ingest: fakeIngestChecker{receiving: receiving}, Accounts: accounts, Platforms: platforms,
		BotUsers: chatautomation.BotUserCheckerAdapter{Prefs: prefsService},
	})
	if err := automationManager.Start(context.Background()); err != nil {
		t.Fatalf("automationManager.Start() error = %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = automationManager.Shutdown(ctx)
	})

	handler := NewRouter(Options{
		Logger: logger, StartedAt: time.Now(), Platforms: platforms,
		Accounts: accounts, DeviceFlow: deviceFlow, TwitchMetadata: twitch.NewMetadataService(accounts, twitch.New(twitch.Options{})),
		OutboundChat: outboundManager, ChatAutomation: automationManager,
	})

	return &chatAutomationTestServer{handler: handler, accounts: accounts, provider: outboundProvider}
}

func (ts *chatAutomationTestServer) createAccount(t *testing.T, id string, extraScopes ...string) account.Account {
	t.Helper()
	scopes := append([]string{"channel:manage:broadcast"}, extraScopes...)
	acc, err := ts.accounts.FinalizeConnection(context.Background(), account.ProviderTwitch,
		account.Identity{ProviderUserID: id + "_provider_user", Login: "viewer_" + id, DisplayName: "Viewer " + id},
		account.TokenBundle{TokenType: "bearer", AccessToken: "fake-access", RefreshToken: "fake-refresh", ExpiresAt: time.Now().Add(time.Hour)},
		scopes, "")
	if err != nil {
		t.Fatalf("createAccount() error = %v", err)
	}
	return acc
}

func (ts *chatAutomationTestServer) do(t *testing.T, method, path string, body any) *http.Response {
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

func decodeChatAutomationBody(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	return out
}

func validScheduleBody(accountID string) map[string]any {
	return map[string]any{
		"name": "Reminder", "enabled": true, "intervalSeconds": 3600, "firstDelaySeconds": 0,
		"jitterSeconds": 0, "onlyWhileIngestReceiving": false, "minimumChatMessages": 0, "maximumSendsPerHour": 10,
		"targets": []map[string]any{{"accountId": accountID}}, "messages": []string{"hello {channelName}"},
	}
}

func validCommandBody(accountID string) map[string]any {
	return map[string]any{
		"name": "discord", "enabled": true, "responseTemplate": "join us", "requiredRole": "everyone",
		"globalCooldownSeconds": 0, "userCooldownSeconds": 0, "aliases": []string{},
		"targets": []map[string]any{{"accountId": accountID}},
	}
}

// --- schedules ------------------------------------------------------------

func TestCreateScheduleThenGet(t *testing.T) {
	ts := newChatAutomationTestServer(t, false)
	acc := ts.createAccount(t, "a1", "user:write:chat")

	resp := ts.do(t, http.MethodPost, "/api/chat-automation/schedules", validScheduleBody(acc.ID))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", resp.StatusCode)
	}
	if resp.Header.Get("Location") == "" {
		t.Error("Location header missing on create")
	}
	body := decodeChatAutomationBody(t, resp)
	id, _ := body["id"].(string)
	if id == "" {
		t.Fatal("created schedule has no id")
	}

	getResp := ts.do(t, http.MethodGet, "/api/chat-automation/schedules/"+id, nil)
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("get status = %d, want 200", getResp.StatusCode)
	}
	got := decodeChatAutomationBody(t, getResp)
	if got["name"] != "Reminder" {
		t.Errorf("name = %v, want Reminder", got["name"])
	}
	if got["state"] == "" || got["state"] == nil {
		t.Error("state missing from schedule response")
	}
}

func TestCreateScheduleRejectsUnknownField(t *testing.T) {
	ts := newChatAutomationTestServer(t, false)
	acc := ts.createAccount(t, "a1", "user:write:chat")
	body := validScheduleBody(acc.ID)
	body["bogusField"] = "x"

	resp := ts.do(t, http.MethodPost, "/api/chat-automation/schedules", body)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestCreateScheduleRejectsWrongContentType(t *testing.T) {
	ts := newChatAutomationTestServer(t, false)
	req := httptest.NewRequest(http.MethodPost, "/api/chat-automation/schedules", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	ts.handler.ServeHTTP(rec, req)
	if rec.Result().StatusCode != http.StatusUnsupportedMediaType {
		t.Errorf("status = %d, want 415", rec.Result().StatusCode)
	}
}

func TestCreateScheduleRejectsUnknownPlaceholder(t *testing.T) {
	ts := newChatAutomationTestServer(t, false)
	acc := ts.createAccount(t, "a1", "user:write:chat")
	body := validScheduleBody(acc.ID)
	body["messages"] = []string{"hi {viewerCount}"}

	resp := ts.do(t, http.MethodPost, "/api/chat-automation/schedules", body)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", resp.StatusCode)
	}
	got := decodeChatAutomationBody(t, resp)
	if got["error"] != "chat_automation_placeholder_invalid" {
		t.Errorf("error = %v, want chat_automation_placeholder_invalid", got["error"])
	}
}

func TestCreateScheduleRejectsMissingTarget(t *testing.T) {
	ts := newChatAutomationTestServer(t, false)
	body := map[string]any{
		"name": "x", "enabled": true, "intervalSeconds": 3600, "maximumSendsPerHour": 10,
		"targets": []map[string]any{}, "messages": []string{"hi"},
	}
	resp := ts.do(t, http.MethodPost, "/api/chat-automation/schedules", body)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", resp.StatusCode)
	}
	got := decodeChatAutomationBody(t, resp)
	if got["error"] != "chat_automation_target_required" {
		t.Errorf("error = %v, want chat_automation_target_required", got["error"])
	}
}

func TestCreateScheduleRejectsUnknownAccount(t *testing.T) {
	ts := newChatAutomationTestServer(t, false)
	resp := ts.do(t, http.MethodPost, "/api/chat-automation/schedules", validScheduleBody("acct_missing"))
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	got := decodeChatAutomationBody(t, resp)
	if got["error"] != "chat_automation_account_not_found" {
		t.Errorf("error = %v, want chat_automation_account_not_found", got["error"])
	}
}

func TestGetScheduleMissingReturns404(t *testing.T) {
	ts := newChatAutomationTestServer(t, false)
	resp := ts.do(t, http.MethodGet, "/api/chat-automation/schedules/sched_missing", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestUpdateAndDeleteSchedule(t *testing.T) {
	ts := newChatAutomationTestServer(t, false)
	acc := ts.createAccount(t, "a1", "user:write:chat")
	createResp := ts.do(t, http.MethodPost, "/api/chat-automation/schedules", validScheduleBody(acc.ID))
	id := decodeChatAutomationBody(t, createResp)["id"].(string)

	updateBody := validScheduleBody(acc.ID)
	updateBody["name"] = "Renamed"
	updResp := ts.do(t, http.MethodPut, "/api/chat-automation/schedules/"+id, updateBody)
	if updResp.StatusCode != http.StatusOK {
		t.Fatalf("update status = %d, want 200", updResp.StatusCode)
	}
	if got := decodeChatAutomationBody(t, updResp)["name"]; got != "Renamed" {
		t.Errorf("name after update = %v, want Renamed", got)
	}

	delResp := ts.do(t, http.MethodDelete, "/api/chat-automation/schedules/"+id, nil)
	if delResp.StatusCode != http.StatusNoContent {
		t.Errorf("delete status = %d, want 204", delResp.StatusCode)
	}
	getResp := ts.do(t, http.MethodGet, "/api/chat-automation/schedules/"+id, nil)
	if getResp.StatusCode != http.StatusNotFound {
		t.Errorf("get after delete status = %d, want 404", getResp.StatusCode)
	}
}

func TestListSchedulesMethodNotAllowed(t *testing.T) {
	ts := newChatAutomationTestServer(t, false)
	resp := ts.do(t, http.MethodPatch, "/api/chat-automation/schedules", nil)
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", resp.StatusCode)
	}
	if resp.Header.Get("Allow") == "" {
		t.Error("Allow header missing on 405")
	}
}

func TestSendNowRespectsIngestAndReturnsPerTargetResult(t *testing.T) {
	ts := newChatAutomationTestServer(t, false) // not receiving
	acc := ts.createAccount(t, "a1", "user:write:chat")
	body := validScheduleBody(acc.ID)
	body["onlyWhileIngestReceiving"] = true
	createResp := ts.do(t, http.MethodPost, "/api/chat-automation/schedules", body)
	id := decodeChatAutomationBody(t, createResp)["id"].(string)

	sendResp := ts.do(t, http.MethodPost, "/api/chat-automation/schedules/"+id+"/send-now", nil)
	if sendResp.StatusCode != http.StatusOK {
		t.Fatalf("send-now status = %d, want 200", sendResp.StatusCode)
	}
	got := decodeChatAutomationBody(t, sendResp)
	results, _ := got["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("results = %v, want 1 entry", results)
	}
	first, _ := results[0].(map[string]any)
	if first["sent"] != false || first["skipReason"] != "waiting_for_stream" {
		t.Errorf("first result = %+v, want sent=false skipReason=waiting_for_stream", first)
	}
}

func TestSendNowSendsWhenReceiving(t *testing.T) {
	ts := newChatAutomationTestServer(t, true) // receiving
	acc := ts.createAccount(t, "a1", "user:write:chat")
	ts.provider.setNext(outboundchat.SendMessageResult{Sent: true, ProviderMessageID: "m1"}, nil)
	createResp := ts.do(t, http.MethodPost, "/api/chat-automation/schedules", validScheduleBody(acc.ID))
	id := decodeChatAutomationBody(t, createResp)["id"].(string)

	sendResp := ts.do(t, http.MethodPost, "/api/chat-automation/schedules/"+id+"/send-now", nil)
	got := decodeChatAutomationBody(t, sendResp)
	results, _ := got["results"].([]any)
	first, _ := results[0].(map[string]any)
	if first["sent"] != true {
		t.Errorf("first result = %+v, want sent=true", first)
	}
	body, _ := json.Marshal(got)
	if bytes.Contains(body, []byte("hello")) {
		t.Error("send-now response must never echo the sent message text")
	}
}

// --- commands ---------------------------------------------------------

func TestCreateCommandThenGet(t *testing.T) {
	ts := newChatAutomationTestServer(t, false)
	acc := ts.createAccount(t, "a1", "user:write:chat")

	resp := ts.do(t, http.MethodPost, "/api/chat-automation/commands", validCommandBody(acc.ID))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", resp.StatusCode)
	}
	body := decodeChatAutomationBody(t, resp)
	id, _ := body["id"].(string)
	if id == "" {
		t.Fatal("created command has no id")
	}
	if body["name"] != "discord" {
		t.Errorf("name = %v, want discord", body["name"])
	}

	getResp := ts.do(t, http.MethodGet, "/api/chat-automation/commands/"+id, nil)
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("get status = %d, want 200", getResp.StatusCode)
	}
}

func TestCreateCommandConflictReturns409(t *testing.T) {
	ts := newChatAutomationTestServer(t, false)
	acc := ts.createAccount(t, "a1", "user:write:chat")
	if resp := ts.do(t, http.MethodPost, "/api/chat-automation/commands", validCommandBody(acc.ID)); resp.StatusCode != http.StatusCreated {
		t.Fatalf("first create status = %d, want 201", resp.StatusCode)
	}
	resp := ts.do(t, http.MethodPost, "/api/chat-automation/commands", validCommandBody(acc.ID))
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("second create status = %d, want 409", resp.StatusCode)
	}
	got := decodeChatAutomationBody(t, resp)
	if got["error"] != "chat_automation_command_conflict" {
		t.Errorf("error = %v, want chat_automation_command_conflict", got["error"])
	}
}

func TestCreateCommandRejectsInvalidName(t *testing.T) {
	ts := newChatAutomationTestServer(t, false)
	acc := ts.createAccount(t, "a1", "user:write:chat")
	body := validCommandBody(acc.ID)
	body["name"] = "!discord"
	resp := ts.do(t, http.MethodPost, "/api/chat-automation/commands", body)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", resp.StatusCode)
	}
}

func TestDeleteCommandThen404(t *testing.T) {
	ts := newChatAutomationTestServer(t, false)
	acc := ts.createAccount(t, "a1", "user:write:chat")
	createResp := ts.do(t, http.MethodPost, "/api/chat-automation/commands", validCommandBody(acc.ID))
	id := decodeChatAutomationBody(t, createResp)["id"].(string)

	delResp := ts.do(t, http.MethodDelete, "/api/chat-automation/commands/"+id, nil)
	if delResp.StatusCode != http.StatusNoContent {
		t.Errorf("delete status = %d, want 204", delResp.StatusCode)
	}
	getResp := ts.do(t, http.MethodGet, "/api/chat-automation/commands/"+id, nil)
	if getResp.StatusCode != http.StatusNotFound {
		t.Errorf("get after delete = %d, want 404", getResp.StatusCode)
	}
}

// --- status and preview ------------------------------------------------

func TestChatAutomationStatus(t *testing.T) {
	ts := newChatAutomationTestServer(t, false)
	acc := ts.createAccount(t, "a1", "user:write:chat")
	ts.do(t, http.MethodPost, "/api/chat-automation/schedules", validScheduleBody(acc.ID))
	ts.do(t, http.MethodPost, "/api/chat-automation/commands", validCommandBody(acc.ID))

	resp := ts.do(t, http.MethodGet, "/api/chat-automation/status", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	got := decodeChatAutomationBody(t, resp)
	engine, _ := got["engine"].(map[string]any)
	if engine["running"] != true {
		t.Errorf("engine.running = %v, want true", engine["running"])
	}
	schedules, _ := got["schedules"].([]any)
	commands, _ := got["commands"].([]any)
	if len(schedules) != 1 || len(commands) != 1 {
		t.Errorf("schedules=%d commands=%d, want 1 and 1", len(schedules), len(commands))
	}
}

func TestPreviewRendersWithoutSendingOrPersisting(t *testing.T) {
	ts := newChatAutomationTestServer(t, false)
	acc := ts.createAccount(t, "a1", "user:write:chat")

	resp := ts.do(t, http.MethodPost, "/api/chat-automation/preview", map[string]any{
		"template": "Hi from {channelName} on {platform}", "accountId": acc.ID,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	got := decodeChatAutomationBody(t, resp)
	if got["renderedText"] != "Hi from Viewer a1 on Twitch" {
		t.Errorf("renderedText = %v", got["renderedText"])
	}
	if got["validForProvider"] != true {
		t.Errorf("validForProvider = %v, want true", got["validForProvider"])
	}
	if ts.provider.lastReq.Message != "" {
		t.Error("preview must never call the outbound provider")
	}
}

func TestPreviewUnresolvedPlaceholderWarns(t *testing.T) {
	ts := newChatAutomationTestServer(t, false)
	acc := ts.createAccount(t, "a1", "user:write:chat")

	resp := ts.do(t, http.MethodPost, "/api/chat-automation/preview", map[string]any{
		"template": "Now playing: {streamTitle}", "accountId": acc.ID,
	})
	got := decodeChatAutomationBody(t, resp)
	unresolved, _ := got["unresolvedPlaceholders"].([]any)
	if len(unresolved) != 1 || unresolved[0] != "streamTitle" {
		t.Errorf("unresolvedPlaceholders = %v, want [streamTitle]", unresolved)
	}
	if got["validForProvider"] != false {
		t.Errorf("validForProvider = %v, want false", got["validForProvider"])
	}
}

func TestNoTokenLeaksInAnyChatAutomationResponse(t *testing.T) {
	ts := newChatAutomationTestServer(t, true)
	acc := ts.createAccount(t, "a1", "user:write:chat")
	ts.provider.setNext(outboundchat.SendMessageResult{Sent: true, ProviderMessageID: "m1"}, nil)

	createResp := ts.do(t, http.MethodPost, "/api/chat-automation/schedules", validScheduleBody(acc.ID))
	created := decodeChatAutomationBody(t, createResp)
	id := created["id"].(string)
	sendResp := ts.do(t, http.MethodPost, "/api/chat-automation/schedules/"+id+"/send-now", nil)
	body, _ := io.ReadAll(sendResp.Body)
	if bytes.Contains(body, []byte("fake-access")) || bytes.Contains(body, []byte("fake-refresh")) {
		t.Error("send-now response leaked a token")
	}
}
