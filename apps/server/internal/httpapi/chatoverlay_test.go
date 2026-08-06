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

	co "github.com/streaming-tree/server/internal/chatoverlay"
	"github.com/streaming-tree/server/internal/domain/account"
	chatoverlaydomain "github.com/streaming-tree/server/internal/domain/chatoverlay"
	engagement "github.com/streaming-tree/server/internal/domain/engagement"
	bus "github.com/streaming-tree/server/internal/engagement"
	oc "github.com/streaming-tree/server/internal/operatorchat"
	"github.com/streaming-tree/server/internal/secrets/secretstest"
	"github.com/streaming-tree/server/internal/storage/sqlite"
)

type chatOverlayTestServer struct {
	handler  http.Handler
	accounts *account.Service
	bus      *bus.Bus
	profiles *chatoverlaydomain.Service
	runtime  *co.Manager
}

func newChatOverlayTestServer(t *testing.T) *chatOverlayTestServer {
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

	accounts := account.NewService(account.Options{
		Repository: sqlite.NewAccountRepository(db.DB), Secrets: secretstest.New(), Logger: logger,
	})

	eventBus := bus.New(bus.Options{Capacity: 100})
	t.Cleanup(eventBus.Shutdown)

	projection := oc.New(oc.Options{Source: eventBus, Capacity: 100})
	if err := projection.Start(context.Background()); err != nil {
		t.Fatalf("operatorchat projection.Start() error = %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		projection.Shutdown(ctx)
	})

	profiles := chatoverlaydomain.NewService(sqlite.NewChatOverlayRepository(db.DB), nil)
	resolver := &co.DefaultSettingsResolver{Profiles: profiles, AccountLabel: func(string) (string, bool) { return "", false }}
	runtime := co.NewManager(co.WrapOperatorChatSource(projection), resolver, logger)
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("chat overlay runtime.Start() error = %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		runtime.Shutdown(ctx)
	})

	handler := NewRouter(Options{
		Logger: logger, StartedAt: time.Now(), Accounts: accounts,
		ChatOverlayProfiles: profiles, ChatOverlayRuntime: runtime,
	})

	return &chatOverlayTestServer{handler: handler, accounts: accounts, bus: eventBus, profiles: profiles, runtime: runtime}
}

func (ts *chatOverlayTestServer) createAccount(t *testing.T, providerUserID string) string {
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

// toPutRequest builds the editable-fields-only PUT body from a full
// profile response - a PUT request body must never carry id/publicSlug/
// createdAt/updatedAt (decodeJSON rejects unknown fields), matching
// putChatOverlayProfileRequest's own documented contract.
func toPutRequest(p chatOverlayProfileResponse) putChatOverlayProfileRequest {
	return putChatOverlayProfileRequest{
		Name: p.Name, Enabled: p.Enabled,
		LayoutMode: p.LayoutMode, StackDirection: p.StackDirection, HorizontalAlignment: p.HorizontalAlignment,
		ShowPlatformIcon: p.ShowPlatformIcon, ShowPlatformName: p.ShowPlatformName, ShowAccountLabel: p.ShowAccountLabel,
		ShowAvatar: p.ShowAvatar, ShowBadges: p.ShowBadges, ShowTimestamp: p.ShowTimestamp,
		ShowActivityEvents: p.ShowActivityEvents, ShowDeletedPlaceholder: p.ShowDeletedPlaceholder,
		HideCommands: p.HideCommands, HideBots: p.HideBots,
		MaxVisibleItems: p.MaxVisibleItems, MessageLifetimeSeconds: p.MessageLifetimeSeconds,
		FontFamily: p.FontFamily, FontSize: p.FontSize, FontWeight: p.FontWeight, LineHeight: p.LineHeight,
		TextColor: p.TextColor, UsernameColorMode: p.UsernameColorMode, BubbleColor: p.BubbleColor,
		BubbleOpacity: p.BubbleOpacity, BorderRadius: p.BorderRadius, ItemSpacing: p.ItemSpacing,
		TextOutline: p.TextOutline, TextShadow: p.TextShadow,
		EntryAnimation: p.EntryAnimation, ExitAnimation: p.ExitAnimation, AnimationDurationMS: p.AnimationDurationMS,
		HighlightBroadcaster: p.HighlightBroadcaster, HighlightModerators: p.HighlightModerators,
		HighlightSubscribers: p.HighlightSubscribers, HighlightVIPs: p.HighlightVIPs,
		Language: p.Language,
	}
}

func (ts *chatOverlayTestServer) createOverlay(t *testing.T, name string) chatOverlayProfileResponse {
	t.Helper()
	recorder := do(t, ts.handler, http.MethodPost, "/api/chat-overlays", map[string]string{"name": name})
	if recorder.Code != http.StatusOK {
		t.Fatalf("create overlay status = %d, want 200, body = %s", recorder.Code, recorder.Body.String())
	}
	var body chatOverlayProfileResponse
	decodeBody(t, recorder, &body)
	return body
}

func (ts *chatOverlayTestServer) publishMessage(t *testing.T, accountID, providerEventID, text string) {
	t.Helper()
	msg := engagement.NewMessage([]engagement.Fragment{{Type: engagement.FragmentText, Text: text}})
	evt := engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderTwitch,
		ConnectedAccountID: accountID, Type: engagement.TypeChatMessage, ProviderEventType: "channel.chat.message",
		ProviderEventID: providerEventID, PlatformTimestamp: time.Now().UTC(), DedupeKey: "dedupe_" + providerEventID,
		User: &engagement.User{ProviderUserID: "u1", Login: "viewer", DisplayName: "Viewer"}, Message: &msg,
	}
	if _, _, err := ts.bus.Publish(evt); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
}

func (ts *chatOverlayTestServer) deleteMessage(t *testing.T, accountID, deletedProviderEventID string) {
	t.Helper()
	evt := engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderTwitch,
		ConnectedAccountID: accountID, Type: engagement.TypeChatMessageDeleted, ProviderEventType: "channel.chat.message_delete",
		ProviderEventID: "del_" + deletedProviderEventID, PlatformTimestamp: time.Now().UTC(), DedupeKey: "dedupe_del_" + deletedProviderEventID,
		ModerationRef: deletedProviderEventID,
	}
	if _, _, err := ts.bus.Publish(evt); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
}

func waitForPublicItems(t *testing.T, ts *chatOverlayTestServer, slug string, want int) []publicChatOverlayItemResponse {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		recorder := do(t, ts.handler, http.MethodGet, "/api/public/chat-overlays/"+slug+"/items", nil)
		if recorder.Code == http.StatusOK {
			var body struct {
				Items []publicChatOverlayItemResponse `json:"items"`
			}
			decodeBody(t, recorder, &body)
			if len(body.Items) >= want {
				return body.Items
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d public items for slug %q", want, slug)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// --- profile CRUD -----------------------------------------------------------

func TestChatOverlayCreateReturnsDefaultsAndAWorkingPublicSlug(t *testing.T) {
	ts := newChatOverlayTestServer(t)
	overlay := ts.createOverlay(t, "Main Overlay")

	if overlay.ID == "" || overlay.PublicSlug == "" {
		t.Fatalf("expected a non-empty id and publicSlug, got %+v", overlay)
	}
	if overlay.Name != "Main Overlay" || !overlay.Enabled {
		t.Errorf("expected the new overlay to carry the given name and be enabled by default, got %+v", overlay)
	}
	if overlay.MaxVisibleItems != 30 {
		t.Errorf("MaxVisibleItems = %d, want the documented default 30", overlay.MaxVisibleItems)
	}
}

func TestChatOverlayCreateRejectsEmptyName(t *testing.T) {
	ts := newChatOverlayTestServer(t)
	recorder := do(t, ts.handler, http.MethodPost, "/api/chat-overlays", map[string]string{"name": ""})
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", recorder.Code, recorder.Body.String())
	}
	var body ErrorBody
	decodeBody(t, recorder, &body)
	if body.Error != "validation_failed" {
		t.Errorf("error = %q, want validation_failed", body.Error)
	}
}

func TestChatOverlayCreateRejectsUnknownField(t *testing.T) {
	ts := newChatOverlayTestServer(t)
	recorder := do(t, ts.handler, http.MethodPost, "/api/chat-overlays", map[string]any{"name": "x", "unexpectedField": true})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestChatOverlayCreateRejectsMalformedJSON(t *testing.T) {
	ts := newChatOverlayTestServer(t)
	recorder := do(t, ts.handler, http.MethodPost, "/api/chat-overlays", "{not json")
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestChatOverlayListReturnsCreatedOverlays(t *testing.T) {
	ts := newChatOverlayTestServer(t)
	ts.createOverlay(t, "One")
	ts.createOverlay(t, "Two")

	recorder := do(t, ts.handler, http.MethodGet, "/api/chat-overlays", nil)
	var body struct {
		Items []chatOverlayProfileResponse `json:"items"`
	}
	decodeBody(t, recorder, &body)
	if len(body.Items) != 2 {
		t.Fatalf("expected 2 overlays, got %d", len(body.Items))
	}
}

func TestChatOverlayGetUnknownIDReturnsNotFound(t *testing.T) {
	ts := newChatOverlayTestServer(t)
	recorder := do(t, ts.handler, http.MethodGet, "/api/chat-overlays/ov_missing", nil)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", recorder.Code, recorder.Body.String())
	}
	var body ErrorBody
	decodeBody(t, recorder, &body)
	if body.Error != "chat_overlay_not_found" {
		t.Errorf("error = %q, want chat_overlay_not_found", body.Error)
	}
}

func TestChatOverlayPutReplacesSettingsAndPreservesIdentity(t *testing.T) {
	ts := newChatOverlayTestServer(t)
	overlay := ts.createOverlay(t, "Original")

	overlay.Name = "Renamed"
	overlay.MaxVisibleItems = 10
	overlay.FontSize = 20
	recorder := do(t, ts.handler, http.MethodPut, "/api/chat-overlays/"+overlay.ID, toPutRequest(overlay))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", recorder.Code, recorder.Body.String())
	}
	var saved chatOverlayProfileResponse
	decodeBody(t, recorder, &saved)
	if saved.Name != "Renamed" || saved.MaxVisibleItems != 10 || saved.FontSize != 20 {
		t.Errorf("expected the update to apply, got %+v", saved)
	}
	if saved.ID != overlay.ID || saved.PublicSlug != overlay.PublicSlug {
		t.Errorf("expected id/publicSlug to survive a PUT unchanged, got id=%q slug=%q", saved.ID, saved.PublicSlug)
	}
}

func TestChatOverlayPutRejectsOutOfRangeSettings(t *testing.T) {
	ts := newChatOverlayTestServer(t)
	overlay := ts.createOverlay(t, "Original")
	overlay.MaxVisibleItems = 99999

	recorder := do(t, ts.handler, http.MethodPut, "/api/chat-overlays/"+overlay.ID, toPutRequest(overlay))
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestChatOverlayDeleteThenGetReturnsNotFound(t *testing.T) {
	ts := newChatOverlayTestServer(t)
	overlay := ts.createOverlay(t, "Gone Soon")

	recorder := do(t, ts.handler, http.MethodDelete, "/api/chat-overlays/"+overlay.ID, nil)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204, body = %s", recorder.Code, recorder.Body.String())
	}

	recorder = do(t, ts.handler, http.MethodGet, "/api/chat-overlays/"+overlay.ID, nil)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status after delete = %d, want 404", recorder.Code)
	}
}

func TestChatOverlayRotatePublicSlugInvalidatesTheOldURL(t *testing.T) {
	ts := newChatOverlayTestServer(t)
	overlay := ts.createOverlay(t, "Rotates")
	oldSlug := overlay.PublicSlug

	recorder := do(t, ts.handler, http.MethodPost, "/api/chat-overlays/"+overlay.ID+"/rotate-public-slug", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", recorder.Code, recorder.Body.String())
	}
	var rotated chatOverlayProfileResponse
	decodeBody(t, recorder, &rotated)
	if rotated.PublicSlug == oldSlug {
		t.Fatal("expected a different public slug after rotation")
	}

	oldRecorder := do(t, ts.handler, http.MethodGet, "/api/public/chat-overlays/"+oldSlug+"/config", nil)
	if oldRecorder.Code != http.StatusNotFound {
		t.Errorf("old slug status = %d, want 404 after rotation", oldRecorder.Code)
	}
	newRecorder := do(t, ts.handler, http.MethodGet, "/api/public/chat-overlays/"+rotated.PublicSlug+"/config", nil)
	if newRecorder.Code != http.StatusOK {
		t.Errorf("new slug status = %d, want 200", newRecorder.Code)
	}
}

// --- method-not-allowed ------------------------------------------------------

func TestChatOverlayWrongMethodReturns405WithAllowHeader(t *testing.T) {
	ts := newChatOverlayTestServer(t)
	recorder := do(t, ts.handler, http.MethodPatch, "/api/chat-overlays", nil)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", recorder.Code)
	}
	if allow := recorder.Header().Get("Allow"); !strings.Contains(allow, "GET") || !strings.Contains(allow, "POST") {
		t.Errorf("Allow header = %q, want to contain GET and POST", allow)
	}
}

// --- accounts ----------------------------------------------------------------

func TestChatOverlayAccountsRoundTrip(t *testing.T) {
	ts := newChatOverlayTestServer(t)
	overlay := ts.createOverlay(t, "Filtered")
	accountID := ts.createAccount(t, "acct_1")

	recorder := do(t, ts.handler, http.MethodPut, "/api/chat-overlays/"+overlay.ID+"/accounts", map[string]any{"accountIds": []string{accountID}})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", recorder.Code, recorder.Body.String())
	}

	recorder = do(t, ts.handler, http.MethodGet, "/api/chat-overlays/"+overlay.ID+"/accounts", nil)
	var body struct {
		AccountIDs []string `json:"accountIds"`
	}
	decodeBody(t, recorder, &body)
	if len(body.AccountIDs) != 1 || body.AccountIDs[0] != accountID {
		t.Errorf("accountIds = %v, want [%s]", body.AccountIDs, accountID)
	}
}

func TestChatOverlaySetAccountsRejectsUnknownAccount(t *testing.T) {
	ts := newChatOverlayTestServer(t)
	overlay := ts.createOverlay(t, "Filtered")

	recorder := do(t, ts.handler, http.MethodPut, "/api/chat-overlays/"+overlay.ID+"/accounts", map[string]any{"accountIds": []string{"acct_missing"}})
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", recorder.Code, recorder.Body.String())
	}
	var body ErrorBody
	decodeBody(t, recorder, &body)
	if body.Error != "chat_overlay_account_not_found" {
		t.Errorf("error = %q, want chat_overlay_account_not_found", body.Error)
	}
}

// --- hidden users --------------------------------------------------------------

func TestChatOverlayHiddenUserAddListRemove(t *testing.T) {
	ts := newChatOverlayTestServer(t)
	overlay := ts.createOverlay(t, "Hides")
	accountID := ts.createAccount(t, "acct_1")

	addReq := map[string]string{"providerId": "twitch", "connectedAccountId": accountID, "providerUserId": "u1", "label": "Annoying"}
	recorder := do(t, ts.handler, http.MethodPost, "/api/chat-overlays/"+overlay.ID+"/hidden-users", addReq)
	if recorder.Code != http.StatusOK {
		t.Fatalf("add status = %d, want 200, body = %s", recorder.Code, recorder.Body.String())
	}

	recorder = do(t, ts.handler, http.MethodGet, "/api/chat-overlays/"+overlay.ID+"/hidden-users", nil)
	var list struct {
		Items []chatOverlayHiddenUserResponse `json:"items"`
	}
	decodeBody(t, recorder, &list)
	if len(list.Items) != 1 || list.Items[0].ProviderUserID != "u1" {
		t.Fatalf("expected one hidden user u1, got %+v", list.Items)
	}

	target := "/api/chat-overlays/" + overlay.ID + "/hidden-users?providerId=twitch&connectedAccountId=" + accountID + "&providerUserId=u1"
	recorder = do(t, ts.handler, http.MethodDelete, target, nil)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("remove status = %d, want 204, body = %s", recorder.Code, recorder.Body.String())
	}

	recorder = do(t, ts.handler, http.MethodGet, "/api/chat-overlays/"+overlay.ID+"/hidden-users", nil)
	decodeBody(t, recorder, &list)
	if len(list.Items) != 0 {
		t.Errorf("expected no hidden users after removal, got %+v", list.Items)
	}
}

func TestChatOverlayHiddenUserAddRejectsMissingFields(t *testing.T) {
	ts := newChatOverlayTestServer(t)
	overlay := ts.createOverlay(t, "Hides")

	recorder := do(t, ts.handler, http.MethodPost, "/api/chat-overlays/"+overlay.ID+"/hidden-users", map[string]string{"providerId": "twitch"})
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", recorder.Code, recorder.Body.String())
	}
}

// --- blocked terms ---------------------------------------------------------

func TestChatOverlayBlockedTermAddListRemove(t *testing.T) {
	ts := newChatOverlayTestServer(t)
	overlay := ts.createOverlay(t, "Blocks")

	recorder := do(t, ts.handler, http.MethodPost, "/api/chat-overlays/"+overlay.ID+"/blocked-terms",
		map[string]string{"value": "spam", "matchMode": "contains"})
	if recorder.Code != http.StatusOK {
		t.Fatalf("add status = %d, want 200, body = %s", recorder.Code, recorder.Body.String())
	}
	var term chatOverlayBlockedTermResponse
	decodeBody(t, recorder, &term)
	if term.ID == "" || term.Value != "spam" || term.MatchMode != "contains" {
		t.Fatalf("unexpected saved term %+v", term)
	}

	recorder = do(t, ts.handler, http.MethodGet, "/api/chat-overlays/"+overlay.ID+"/blocked-terms", nil)
	var list struct {
		Items []chatOverlayBlockedTermResponse `json:"items"`
	}
	decodeBody(t, recorder, &list)
	if len(list.Items) != 1 {
		t.Fatalf("expected one blocked term, got %+v", list.Items)
	}

	recorder = do(t, ts.handler, http.MethodDelete, "/api/chat-overlays/"+overlay.ID+"/blocked-terms/"+term.ID, nil)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("remove status = %d, want 204, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestChatOverlayBlockedTermRejectsUnknownMatchMode(t *testing.T) {
	ts := newChatOverlayTestServer(t)
	overlay := ts.createOverlay(t, "Blocks")

	recorder := do(t, ts.handler, http.MethodPost, "/api/chat-overlays/"+overlay.ID+"/blocked-terms",
		map[string]string{"value": "spam", "matchMode": "regex"})
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", recorder.Code, recorder.Body.String())
	}
}

// --- activity types ----------------------------------------------------------

func TestChatOverlayActivityTypesRoundTrip(t *testing.T) {
	ts := newChatOverlayTestServer(t)
	overlay := ts.createOverlay(t, "Activity")

	recorder := do(t, ts.handler, http.MethodPut, "/api/chat-overlays/"+overlay.ID+"/activity-types", map[string]any{"activityTypes": []string{"follow", "bits"}})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", recorder.Code, recorder.Body.String())
	}

	recorder = do(t, ts.handler, http.MethodGet, "/api/chat-overlays/"+overlay.ID+"/activity-types", nil)
	var body struct {
		ActivityTypes []string `json:"activityTypes"`
	}
	decodeBody(t, recorder, &body)
	if len(body.ActivityTypes) != 2 {
		t.Errorf("activityTypes = %v, want 2 entries", body.ActivityTypes)
	}
}

// --- public config/items -----------------------------------------------------

func TestPublicChatOverlayConfigExposesOnlyRendererSettings(t *testing.T) {
	ts := newChatOverlayTestServer(t)
	overlay := ts.createOverlay(t, "Public")

	recorder := do(t, ts.handler, http.MethodGet, "/api/public/chat-overlays/"+overlay.PublicSlug+"/config", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), overlay.ID) {
		t.Errorf("public config body must never contain the management id %q: %s", overlay.ID, recorder.Body.String())
	}

	var body publicChatOverlayConfigResponse
	decodeBody(t, recorder, &body)
	if body.SchemaVersion != co.CurrentVersion {
		t.Errorf("schemaVersion = %d, want %d", body.SchemaVersion, co.CurrentVersion)
	}
}

func TestPublicChatOverlayConfigUnknownSlugReturnsNotFound(t *testing.T) {
	ts := newChatOverlayTestServer(t)
	recorder := do(t, ts.handler, http.MethodGet, "/api/public/chat-overlays/does-not-exist/config", nil)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestPublicChatOverlayConfigDisabledOverlayReturnsDisabledError(t *testing.T) {
	ts := newChatOverlayTestServer(t)
	overlay := ts.createOverlay(t, "Disabled Soon")
	overlay.Enabled = false
	recorder := do(t, ts.handler, http.MethodPut, "/api/chat-overlays/"+overlay.ID, toPutRequest(overlay))
	if recorder.Code != http.StatusOK {
		t.Fatalf("disable status = %d, want 200", recorder.Code)
	}

	recorder = do(t, ts.handler, http.MethodGet, "/api/public/chat-overlays/"+overlay.PublicSlug+"/config", nil)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", recorder.Code, recorder.Body.String())
	}
	var body ErrorBody
	decodeBody(t, recorder, &body)
	if body.Error != "chat_overlay_disabled" {
		t.Errorf("error = %q, want chat_overlay_disabled", body.Error)
	}
}

func TestPublicChatOverlayItemsReflectAPublishedMessageAfterFiltering(t *testing.T) {
	ts := newChatOverlayTestServer(t)
	overlay := ts.createOverlay(t, "Live")
	accountID := ts.createAccount(t, "acct_1")

	ts.publishMessage(t, accountID, "msg_1", "hello from twitch")
	items := waitForPublicItems(t, ts, overlay.PublicSlug, 1)

	if items[0].Message == nil || items[0].Message.PlainText != "hello from twitch" {
		t.Fatalf("unexpected item %+v", items[0])
	}
	if items[0].ProviderID != "twitch" {
		t.Errorf("providerId = %q, want twitch", items[0].ProviderID)
	}
}

func TestPublicChatOverlayItemsNeverExposeHiddenUserOrBlockedTermLists(t *testing.T) {
	ts := newChatOverlayTestServer(t)
	overlay := ts.createOverlay(t, "Live")
	accountID := ts.createAccount(t, "acct_1")

	do(t, ts.handler, http.MethodPost, "/api/chat-overlays/"+overlay.ID+"/blocked-terms",
		map[string]string{"value": "secretterm", "matchMode": "contains"})

	ts.publishMessage(t, accountID, "msg_1", "a perfectly normal message")
	items := waitForPublicItems(t, ts, overlay.PublicSlug, 1)

	recorder := do(t, ts.handler, http.MethodGet, "/api/public/chat-overlays/"+overlay.PublicSlug+"/items", nil)
	if strings.Contains(recorder.Body.String(), "secretterm") {
		t.Errorf("public items response must never contain a configured blocked term: %s", recorder.Body.String())
	}
	_ = items
}

// --- public SSE stream -----------------------------------------------------

func TestPublicChatOverlayStreamSendsUpsertForALiveMessage(t *testing.T) {
	ts := newChatOverlayTestServer(t)
	overlay := ts.createOverlay(t, "Streamed")
	accountID := ts.createAccount(t, "acct_1")

	srv := httptest.NewServer(ts.handler)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/public/chat-overlays/"+overlay.PublicSlug+"/stream", nil)
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

	// Read the initial (empty) reset before publishing, so the next read
	// only needs to contain the live upsert.
	buf := make([]byte, 4096)
	if _, err := resp.Body.Read(buf); err != nil {
		t.Fatalf("reading the initial reset failed: %v", err)
	}

	ts.publishMessage(t, accountID, "msg_1", "streamed hello")

	n, err := resp.Body.Read(buf)
	if err != nil && n == 0 {
		t.Fatalf("reading the stream failed: %v", err)
	}
	chunk := string(buf[:n])
	if !strings.Contains(chunk, "event: chat-overlay.upsert") {
		t.Errorf("stream chunk missing chat-overlay.upsert: %q", chunk)
	}
	if !strings.Contains(chunk, "streamed hello") {
		t.Errorf("stream chunk missing the live message text: %q", chunk)
	}
}

// TestPublicChatOverlayStreamRemoveEventCarriesReasonAndNoContent covers
// the corrective-pass requirement that a remove payload is exactly
// {id, reason} - never the removed message's own text, and the reason
// for a moderator deletion is the immediate, never-cosmetic
// "message_deleted".
func TestPublicChatOverlayStreamRemoveEventCarriesReasonAndNoContent(t *testing.T) {
	ts := newChatOverlayTestServer(t)
	overlay := ts.createOverlay(t, "Streamed")
	accountID := ts.createAccount(t, "acct_1")

	srv := httptest.NewServer(ts.handler)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/public/chat-overlays/"+overlay.PublicSlug+"/stream", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET .../stream error = %v", err)
	}
	defer resp.Body.Close()

	buf := make([]byte, 4096)
	if _, err := resp.Body.Read(buf); err != nil { // initial reset
		t.Fatalf("reading the initial reset failed: %v", err)
	}

	const secretText = "this message text must never appear in the remove event"
	ts.publishMessage(t, accountID, "msg_del_1", secretText)
	if _, err := resp.Body.Read(buf); err != nil { // the upsert
		t.Fatalf("reading the upsert failed: %v", err)
	}

	ts.deleteMessage(t, accountID, "msg_del_1")
	n, err := resp.Body.Read(buf)
	if err != nil && n == 0 {
		t.Fatalf("reading the remove event failed: %v", err)
	}
	chunk := string(buf[:n])

	if !strings.Contains(chunk, "event: chat-overlay.remove") {
		t.Fatalf("expected a chat-overlay.remove event, got: %q", chunk)
	}
	if !strings.Contains(chunk, `"reason":"message_deleted"`) {
		t.Errorf("expected reason \"message_deleted\", got: %q", chunk)
	}
	if strings.Contains(chunk, secretText) {
		t.Errorf("the remove event must never carry the deleted message's own text: %q", chunk)
	}
}

func TestPublicChatOverlayStreamUnknownSlugRendersEmptyNotError(t *testing.T) {
	ts := newChatOverlayTestServer(t)

	srv := httptest.NewServer(ts.handler)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/public/chat-overlays/does-not-exist/stream", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET .../stream error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (an unavailable overlay must never surface as a hard HTTP error on the live stream)", resp.StatusCode)
	}

	buf := make([]byte, 4096)
	n, err := resp.Body.Read(buf)
	if err != nil && n == 0 {
		t.Fatalf("reading the stream failed: %v", err)
	}
	chunk := string(buf[:n])
	if !strings.Contains(chunk, "event: chat-overlay.reset") {
		t.Errorf("expected an empty reset event, got: %q", chunk)
	}
}
