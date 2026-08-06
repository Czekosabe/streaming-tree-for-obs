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
	"github.com/streaming-tree/server/internal/domain/operatorchatprefs"
	bus "github.com/streaming-tree/server/internal/engagement"
	oc "github.com/streaming-tree/server/internal/operatorchat"
	"github.com/streaming-tree/server/internal/secrets/secretstest"
	"github.com/streaming-tree/server/internal/storage/sqlite"
)

type operatorChatTestServer struct {
	handler    http.Handler
	accounts   *account.Service
	bus        *bus.Bus
	projection *oc.Projection
	prefs      *operatorchatprefs.Service
}

func newOperatorChatTestServer(t *testing.T) *operatorChatTestServer {
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

	accountRepo := sqlite.NewAccountRepository(db.DB)
	accounts := account.NewService(account.Options{
		Repository: accountRepo, Secrets: secretstest.New(),
		Logger: logger,
	})

	eventBus := bus.New(bus.Options{Capacity: 100})
	t.Cleanup(eventBus.Shutdown)

	projection := oc.New(oc.Options{Source: eventBus, Capacity: 100})
	if err := projection.Start(context.Background()); err != nil {
		t.Fatalf("projection.Start() error = %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		projection.Shutdown(ctx)
	})

	prefsRepo := sqlite.NewOperatorChatPrefsRepository(db.DB)
	prefs := operatorchatprefs.NewService(prefsRepo, nil, nil)

	handler := NewRouter(Options{
		Logger: logger, StartedAt: time.Now(), Accounts: accounts,
		OperatorChatProjection: projection, OperatorChatPrefs: prefs,
	})

	return &operatorChatTestServer{handler: handler, accounts: accounts, bus: eventBus, projection: projection, prefs: prefs}
}

func (ts *operatorChatTestServer) createAccount(t *testing.T, providerUserID string) string {
	t.Helper()
	acc, err := ts.accounts.FinalizeConnection(context.Background(), account.ProviderTwitch,
		account.Identity{ProviderUserID: providerUserID, Login: "streamer", DisplayName: "Streamer"},
		account.TokenBundle{TokenType: "bearer", AccessToken: "fake-access", RefreshToken: "fake-refresh", ExpiresAt: time.Now().Add(time.Hour)},
		nil, "")
	if err != nil {
		t.Fatalf("FinalizeConnection() error = %v", err)
	}
	return acc.ID
}

func waitForOperatorChatItems(t *testing.T, ts *operatorChatTestServer, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		items, _ := ts.projection.ItemsAfter(0, 0)
		if len(items) >= want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d operator chat items, have %d", want, len(items))
		}
		time.Sleep(time.Millisecond)
	}
}

func TestOperatorChatStatusReportsCapacityAndCounts(t *testing.T) {
	ts := newOperatorChatTestServer(t)

	recorder := do(t, ts.handler, http.MethodGet, "/api/operator-chat/status", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", recorder.Code, recorder.Body.String())
	}
	var body operatorChatStatusResponse
	decodeBody(t, recorder, &body)
	if body.BufferCapacity != 100 {
		t.Errorf("BufferCapacity = %d, want 100", body.BufferCapacity)
	}
	if body.RetainedCount != 0 {
		t.Errorf("RetainedCount = %d, want 0 before any event", body.RetainedCount)
	}
}

func TestOperatorChatItemsReflectsAPublishedMessage(t *testing.T) {
	ts := newOperatorChatTestServer(t)
	accountID := ts.createAccount(t, "twitch_user_1")

	msg := engagement.NewMessage([]engagement.Fragment{{Type: engagement.FragmentText, Text: "hello chat"}})
	evt := engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderTwitch,
		ConnectedAccountID: accountID, Type: engagement.TypeChatMessage, ProviderEventType: "channel.chat.message",
		ProviderEventID: "msg_1", PlatformTimestamp: time.Now().UTC(), DedupeKey: "dedupe_1",
		User: &engagement.User{ProviderUserID: "u1", Login: "viewer", DisplayName: "Viewer"}, Message: &msg,
	}
	if _, _, err := ts.bus.Publish(evt); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	waitForOperatorChatItems(t, ts, 1)

	recorder := do(t, ts.handler, http.MethodGet, "/api/operator-chat/items", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Items []operatorChatItemResponse `json:"items"`
		Gap   bool                       `json:"gap"`
	}
	decodeBody(t, recorder, &body)
	if len(body.Items) != 1 {
		t.Fatalf("Items = %d, want 1", len(body.Items))
	}
	item := body.Items[0]
	if item.Kind != "message" || item.Message == nil || item.Message.PlainText != "hello chat" {
		t.Errorf("item = %+v, want a message item with text 'hello chat'", item)
	}
	if item.ConnectedAccountID != accountID {
		t.Errorf("ConnectedAccountID = %q, want %q", item.ConnectedAccountID, accountID)
	}
}

func TestOperatorChatItemsFilterByKind(t *testing.T) {
	ts := newOperatorChatTestServer(t)
	accountID := ts.createAccount(t, "twitch_user_2")

	msg := engagement.NewMessage([]engagement.Fragment{{Type: engagement.FragmentText, Text: "hi"}})
	chatEvt := engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderTwitch,
		ConnectedAccountID: accountID, Type: engagement.TypeChatMessage, ProviderEventType: "channel.chat.message",
		ProviderEventID: "msg_2", PlatformTimestamp: time.Now().UTC(), DedupeKey: "dedupe_2",
		User: &engagement.User{ProviderUserID: "u2"}, Message: &msg,
	}
	followEvt := engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderTwitch,
		ConnectedAccountID: accountID, Type: engagement.TypeFollow, ProviderEventType: "channel.follow",
		PlatformTimestamp: time.Now().UTC(), DedupeKey: "dedupe_3", User: &engagement.User{ProviderUserID: "u3"},
	}
	ts.bus.Publish(chatEvt)
	ts.bus.Publish(followEvt)
	waitForOperatorChatItems(t, ts, 2)

	recorder := do(t, ts.handler, http.MethodGet, "/api/operator-chat/items?kinds=activity", nil)
	var body struct {
		Items []operatorChatItemResponse `json:"items"`
	}
	decodeBody(t, recorder, &body)
	if len(body.Items) != 1 || body.Items[0].Kind != "activity" {
		t.Errorf("Items = %+v, want exactly one activity item", body.Items)
	}
}

func TestOperatorChatItemsRejectsUnknownKind(t *testing.T) {
	ts := newOperatorChatTestServer(t)

	recorder := do(t, ts.handler, http.MethodGet, "/api/operator-chat/items?kinds=bogus", nil)
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", recorder.Code, recorder.Body.String())
	}
	var body ErrorBody
	decodeBody(t, recorder, &body)
	if body.Error != "operator_chat_invalid_filter" {
		t.Errorf("Error = %q, want operator_chat_invalid_filter", body.Error)
	}
}

func TestOperatorChatPreferencesDefaultsBeforeAnySave(t *testing.T) {
	ts := newOperatorChatTestServer(t)

	recorder := do(t, ts.handler, http.MethodGet, "/api/operator-chat/preferences", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", recorder.Code, recorder.Body.String())
	}
	var body operatorChatPreferencesResponse
	decodeBody(t, recorder, &body)
	if !body.ShowPlatformIcon || !body.ShowAccountLabel || body.CompactMode {
		t.Errorf("body = %+v, want the documented defaults", body)
	}
}

func TestOperatorChatPreferencesRoundTrip(t *testing.T) {
	ts := newOperatorChatTestServer(t)

	put := do(t, ts.handler, http.MethodPut, "/api/operator-chat/preferences", operatorChatPreferencesResponse{
		ShowPlatformIcon: false, ShowPlatformName: true, ShowAccountLabel: true, ShowBadges: true,
		ShowTimestamps: false, ShowActivityEvents: true, ShowDeletedMessages: false, HideCommandMessages: true, CompactMode: true,
	})
	if put.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want 200, body = %s", put.Code, put.Body.String())
	}

	get := do(t, ts.handler, http.MethodGet, "/api/operator-chat/preferences", nil)
	var body operatorChatPreferencesResponse
	decodeBody(t, get, &body)
	if body.ShowPlatformIcon || !body.CompactMode || !body.HideCommandMessages {
		t.Errorf("body = %+v, want the round-tripped values", body)
	}
}

func TestOperatorChatPreferencesRejectsUnknownField(t *testing.T) {
	ts := newOperatorChatTestServer(t)

	recorder := do(t, ts.handler, http.MethodPut, "/api/operator-chat/preferences", map[string]any{"showPlatformIcon": true, "bogusField": 1})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for an unknown field, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestOperatorChatAccountVisibilityUnknownAccountReturnsNotFound(t *testing.T) {
	ts := newOperatorChatTestServer(t)

	recorder := do(t, ts.handler, http.MethodPut, "/api/operator-chat/account-visibility/acct_missing", map[string]any{"visible": false})
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestOperatorChatAccountVisibilityRoundTrip(t *testing.T) {
	ts := newOperatorChatTestServer(t)
	accountID := ts.createAccount(t, "twitch_user_3")

	put := do(t, ts.handler, http.MethodPut, "/api/operator-chat/account-visibility/"+accountID, map[string]any{"visible": false})
	if put.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want 200, body = %s", put.Code, put.Body.String())
	}

	get := do(t, ts.handler, http.MethodGet, "/api/operator-chat/account-visibility", nil)
	var body struct {
		Items []operatorChatAccountVisibilityResponse `json:"items"`
	}
	decodeBody(t, get, &body)
	if len(body.Items) != 1 || body.Items[0].AccountID != accountID || body.Items[0].Visible {
		t.Errorf("Items = %+v, want exactly one hidden account", body.Items)
	}
}

func TestOperatorChatHiddenUsersRequiresIdentityFields(t *testing.T) {
	ts := newOperatorChatTestServer(t)

	recorder := do(t, ts.handler, http.MethodPost, "/api/operator-chat/hidden-users", map[string]any{"providerId": "twitch"})
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", recorder.Code, recorder.Body.String())
	}
	var body ErrorBody
	decodeBody(t, recorder, &body)
	if body.Error != "operator_chat_user_invalid" {
		t.Errorf("Error = %q, want operator_chat_user_invalid", body.Error)
	}
}

func TestOperatorChatHiddenUsersUnknownAccountReturnsNotFound(t *testing.T) {
	ts := newOperatorChatTestServer(t)

	recorder := do(t, ts.handler, http.MethodPost, "/api/operator-chat/hidden-users", map[string]any{
		"providerId": "twitch", "connectedAccountId": "acct_missing", "providerUserId": "u1",
	})
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestOperatorChatHiddenUsersAddListRemove(t *testing.T) {
	ts := newOperatorChatTestServer(t)
	accountID := ts.createAccount(t, "twitch_user_4")

	add := do(t, ts.handler, http.MethodPost, "/api/operator-chat/hidden-users", map[string]any{
		"providerId": "twitch", "connectedAccountId": accountID, "providerUserId": "u1", "label": "spammer",
	})
	if add.Code != http.StatusOK {
		t.Fatalf("POST status = %d, want 200, body = %s", add.Code, add.Body.String())
	}
	var added operatorChatUserRefResponse
	decodeBody(t, add, &added)
	if added.Label != "spammer" || added.ID == "" {
		t.Fatalf("added = %+v, want a saved entry with label spammer", added)
	}

	list := do(t, ts.handler, http.MethodGet, "/api/operator-chat/hidden-users", nil)
	var listBody struct {
		Items []operatorChatUserRefResponse `json:"items"`
	}
	decodeBody(t, list, &listBody)
	if len(listBody.Items) != 1 {
		t.Fatalf("Items = %+v, want exactly one", listBody.Items)
	}

	del := do(t, ts.handler, http.MethodDelete, "/api/operator-chat/hidden-users/"+added.ID, nil)
	if del.Code != http.StatusNoContent {
		t.Fatalf("DELETE status = %d, want 204, body = %s", del.Code, del.Body.String())
	}

	listAfter := do(t, ts.handler, http.MethodGet, "/api/operator-chat/hidden-users", nil)
	var afterBody struct {
		Items []operatorChatUserRefResponse `json:"items"`
	}
	decodeBody(t, listAfter, &afterBody)
	if len(afterBody.Items) != 0 {
		t.Errorf("Items after delete = %+v, want empty", afterBody.Items)
	}
}

func TestOperatorChatHiddenUsersRemoveAbsentReturnsNotFound(t *testing.T) {
	ts := newOperatorChatTestServer(t)

	recorder := do(t, ts.handler, http.MethodDelete, "/api/operator-chat/hidden-users/does_not_exist", nil)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestOperatorChatBotUsersAreIndependentOfHiddenUsers(t *testing.T) {
	ts := newOperatorChatTestServer(t)
	accountID := ts.createAccount(t, "twitch_user_5")

	do(t, ts.handler, http.MethodPost, "/api/operator-chat/hidden-users", map[string]any{
		"providerId": "twitch", "connectedAccountId": accountID, "providerUserId": "u1",
	})

	bots := do(t, ts.handler, http.MethodGet, "/api/operator-chat/bot-users", nil)
	var body struct {
		Items []operatorChatUserRefResponse `json:"items"`
	}
	decodeBody(t, bots, &body)
	if len(body.Items) != 0 {
		t.Errorf("bot-users = %+v, want empty - hiding a user must not mark them a bot", body.Items)
	}
}

func TestOperatorChatRoutesRejectWrongMethodWithAllowHeader(t *testing.T) {
	ts := newOperatorChatTestServer(t)

	recorder := do(t, ts.handler, http.MethodPost, "/api/operator-chat/status", nil)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", recorder.Code)
	}
	if recorder.Header().Get("Allow") != http.MethodGet {
		t.Errorf("Allow header = %q, want GET", recorder.Header().Get("Allow"))
	}
}

func TestOperatorChatStreamServesSSEWithReplayAndNoSecrets(t *testing.T) {
	ts := newOperatorChatTestServer(t)
	accountID := ts.createAccount(t, "twitch_user_6")

	msg := engagement.NewMessage([]engagement.Fragment{{Type: engagement.FragmentText, Text: "streamed"}})
	evt := engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderTwitch,
		ConnectedAccountID: accountID, Type: engagement.TypeChatMessage, ProviderEventType: "channel.chat.message",
		ProviderEventID: "msg_stream_1", PlatformTimestamp: time.Now().UTC(), DedupeKey: "dedupe_stream_1",
		User: &engagement.User{ProviderUserID: "u1"}, Message: &msg,
	}
	if _, _, err := ts.bus.Publish(evt); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	waitForOperatorChatItems(t, ts, 1)

	srv := httptest.NewServer(ts.handler)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/operator-chat/stream", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/operator-chat/stream error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}

	buf := make([]byte, 4096)
	n, err := resp.Body.Read(buf)
	if err != nil && n == 0 {
		t.Fatalf("reading the stream failed: %v", err)
	}
	chunk := string(buf[:n])
	if !strings.Contains(chunk, "event: operator-chat.item") {
		t.Errorf("stream chunk missing operator-chat.item: %q", chunk)
	}
	if !strings.Contains(chunk, "streamed") {
		t.Errorf("stream chunk missing the replayed message text: %q", chunk)
	}
	if strings.Contains(chunk, "accessToken") || strings.Contains(chunk, "sessionId") {
		t.Errorf("stream chunk leaked a secret-shaped field: %q", chunk)
	}
}
