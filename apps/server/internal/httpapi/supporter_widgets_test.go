package httpapi

import (
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

	engagement "github.com/streaming-tree/server/internal/domain/engagement"
	domain "github.com/streaming-tree/server/internal/domain/goals"
	bus "github.com/streaming-tree/server/internal/engagement"
	"github.com/streaming-tree/server/internal/storage/sqlite"
	"github.com/streaming-tree/server/internal/supporterwidgets"
)

// supporterWidgetsTestServer wires a real Bus and a real
// supporterwidgets.Manager alongside the same domain.Service every
// other goals test uses - needed only by the tests below that publish a
// real event and observe it flow through the public/runtime-status API,
// mirroring how cmd/server/main.go actually wires the two together.
type supporterWidgetsTestServer struct {
	handler http.Handler
	svc     *domain.Service
	bus     *bus.Bus
	runtime *supporterwidgets.Manager
}

func newSupporterWidgetsTestServer(t *testing.T) *supporterWidgetsTestServer {
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

	svc := domain.NewService(sqlite.NewGoalsRepository(db.DB), alwaysExistsLookup{}, nil)
	b := bus.New(bus.Options{})
	t.Cleanup(b.Shutdown)
	runtime := supporterwidgets.NewManager(supporterwidgets.ManagerOptions{Profiles: svc, Bus: b})
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("runtime.Start() error = %v", err)
	}
	waitUntilTrue(t, time.Second, runtime.Subscribed)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = runtime.Shutdown(ctx)
	})

	handler := NewRouter(Options{Logger: logger, StartedAt: time.Now(), Goals: svc, SupporterWidgets: runtime})
	return &supporterWidgetsTestServer{handler: handler, svc: svc, bus: b, runtime: runtime}
}

func waitUntilTrue(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition never became true within timeout")
}

func (ts *supporterWidgetsTestServer) do(t *testing.T, method, path string, body any) *http.Response {
	t.Helper()
	tsSrv := httptest.NewServer(ts.handler)
	t.Cleanup(tsSrv.Close)
	return doRequest(t, tsSrv.URL, method, path, body)
}

func doRequest(t *testing.T, base, method, path string, body any) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}
		reader = strings.NewReader(string(encoded))
	}
	req, err := http.NewRequest(method, base+path, reader)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s error = %v", method, path, err)
	}
	return resp
}

func decodeSupporterWidgetsBody(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	return out
}

func latestFollowerWidgetBody() map[string]any {
	return map[string]any{
		"kind": "latest_follower", "name": "Latest Follower", "enabled": true,
		"showProvider": true, "showTime": true,
		"orientation": "horizontal", "textAlign": "center", "fontFamily": "sans_serif",
		"backgroundColor": "#00000080", "foregroundColor": "#ffffff",
		"fillColor": "#7c3aed", "borderColor": "#ffffff33", "borderRadiusPx": 12, "opacity": 1.0,
	}
}

func followEventFixture(accountID, dedupeKey, login string) engagement.Event {
	return engagement.Event{
		SchemaVersion: engagement.CurrentSchemaVersion, ProviderID: engagement.ProviderTwitch,
		ConnectedAccountID: accountID, Type: engagement.TypeFollow, PlatformTimestamp: time.Now().UTC(),
		DedupeKey: dedupeKey, User: &engagement.User{ProviderUserID: "u_" + login, Login: login, DisplayName: login},
	}
}

// --- management API: Stage 18B kinds -------------------------------------

func TestCreateLatestFollowerWidgetReturns201(t *testing.T) {
	ts := newSupporterWidgetsTestServer(t)
	resp := ts.do(t, http.MethodPost, "/api/widget-profiles", latestFollowerWidgetBody())
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	body := decodeSupporterWidgetsBody(t, resp)
	if body["kind"] != "latest_follower" {
		t.Errorf("kind = %v, want latest_follower", body["kind"])
	}
	if body["goalId"] != nil && body["goalId"] != "" {
		t.Errorf("goalId = %v, want empty for a non-goal widget", body["goalId"])
	}
}

func TestCreateLargestDonationWithoutCurrencyReturns422(t *testing.T) {
	ts := newSupporterWidgetsTestServer(t)
	body := map[string]any{
		"kind": "largest_donation", "name": "Largest", "enabled": true,
		"orientation": "horizontal", "textAlign": "center", "fontFamily": "sans_serif",
		"backgroundColor": "#00000080", "foregroundColor": "#ffffff",
		"fillColor": "#7c3aed", "borderColor": "#ffffff33", "borderRadiusPx": 12, "opacity": 1.0,
	}
	resp := ts.do(t, http.MethodPost, "/api/widget-profiles", body)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 (largest_donation requires a currency)", resp.StatusCode)
	}
}

func TestCreateDashboardReferencingUnknownChildReturns404(t *testing.T) {
	ts := newSupporterWidgetsTestServer(t)
	body := map[string]any{
		"kind": "dashboard", "name": "Dashboard", "enabled": true, "columns": 2,
		"children":    []map[string]any{{"widgetProfileId": "widget_nope", "column": 1, "columnSpan": 1, "row": 1, "rowSpan": 1}},
		"orientation": "horizontal", "textAlign": "center", "fontFamily": "sans_serif",
		"backgroundColor": "#00000080", "foregroundColor": "#ffffff",
		"fillColor": "#7c3aed", "borderColor": "#ffffff33", "borderRadiusPx": 12, "opacity": 1.0,
	}
	resp := ts.do(t, http.MethodPost, "/api/widget-profiles", body)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestDeleteWidgetReferencedByDashboardReturns409(t *testing.T) {
	ts := newSupporterWidgetsTestServer(t)
	leaf := decodeSupporterWidgetsBody(t, ts.do(t, http.MethodPost, "/api/widget-profiles", latestFollowerWidgetBody()))
	leafID := leaf["id"].(string)

	dashBody := map[string]any{
		"kind": "dashboard", "name": "Dashboard", "enabled": true, "columns": 2,
		"children":    []map[string]any{{"widgetProfileId": leafID, "column": 1, "columnSpan": 1, "row": 1, "rowSpan": 1}},
		"orientation": "horizontal", "textAlign": "center", "fontFamily": "sans_serif",
		"backgroundColor": "#00000080", "foregroundColor": "#ffffff",
		"fillColor": "#7c3aed", "borderColor": "#ffffff33", "borderRadiusPx": 12, "opacity": 1.0,
	}
	ts.do(t, http.MethodPost, "/api/widget-profiles", dashBody)

	resp := ts.do(t, http.MethodDelete, "/api/widget-profiles/"+leafID, nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
}

// --- reset-runtime / runtime-status ---------------------------------------

func TestResetRuntimeRejectedForGoalKind(t *testing.T) {
	ts := newSupporterWidgetsTestServer(t)
	goal := decodeSupporterWidgetsBody(t, ts.do(t, http.MethodPost, "/api/goals", validFollowerGoalBody()))
	wp := decodeSupporterWidgetsBody(t, ts.do(t, http.MethodPost, "/api/widget-profiles", validWidgetProfileBody(goal["id"].(string))))

	resp := ts.do(t, http.MethodPost, "/api/widget-profiles/"+wp["id"].(string)+"/reset-runtime", nil)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 (a goal widget has no runtime state to reset)", resp.StatusCode)
	}
}

func TestResetRuntimeClearsLatestFollower(t *testing.T) {
	ts := newSupporterWidgetsTestServer(t)
	wp := decodeSupporterWidgetsBody(t, ts.do(t, http.MethodPost, "/api/widget-profiles", latestFollowerWidgetBody()))
	id := wp["id"].(string)

	if _, _, err := ts.bus.Publish(followEventFixture("acct_1", "dk_1", "ada")); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	waitUntilTrue(t, 5*time.Second, func() bool { return ts.runtime.Snapshot(id).Revision >= 1 })

	resp := ts.do(t, http.MethodPost, "/api/widget-profiles/"+id+"/reset-runtime", nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	if got := ts.runtime.Snapshot(id); got.Revision != 0 {
		t.Errorf("Snapshot() after reset = %+v, want the zero value", got)
	}
}

func TestRuntimeStatusReportsLatestFollower(t *testing.T) {
	ts := newSupporterWidgetsTestServer(t)
	wp := decodeSupporterWidgetsBody(t, ts.do(t, http.MethodPost, "/api/widget-profiles", latestFollowerWidgetBody()))
	id := wp["id"].(string)

	ts.bus.Publish(followEventFixture("acct_1", "dk_1", "ada"))
	waitUntilTrue(t, 5*time.Second, func() bool { return ts.runtime.Snapshot(id).Revision >= 1 })

	resp := ts.do(t, http.MethodGet, "/api/widget-profiles/"+id+"/runtime-status", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body := decodeSupporterWidgetsBody(t, resp)
	latest, _ := body["latest"].(map[string]any)
	if latest == nil || latest["displayName"] != "ada" {
		t.Fatalf("latest = %v, want displayName ada", body["latest"])
	}
	if _, present := latest["itemId"]; !present {
		t.Error("expected an itemId presentation key")
	}
}

// --- public config/stream for a Stage 18B kind ----------------------------

func TestPublicConfigLatestFollowerReflectsRealEvent(t *testing.T) {
	ts := newSupporterWidgetsTestServer(t)
	wp := decodeSupporterWidgetsBody(t, ts.do(t, http.MethodPost, "/api/widget-profiles", latestFollowerWidgetBody()))
	id := wp["id"].(string)
	slug := wp["publicSlug"].(string)

	resp := ts.do(t, http.MethodGet, "/api/public/widgets/"+slug+"/config", nil)
	body := decodeSupporterWidgetsBody(t, resp)
	if body["kind"] != "latest_follower" {
		t.Fatalf("kind = %v, want latest_follower", body["kind"])
	}
	if body["latest"] != nil {
		t.Errorf("latest = %v, want nil before any event", body["latest"])
	}

	ts.bus.Publish(followEventFixture("acct_1", "dk_1", "ada"))
	waitUntilTrue(t, 5*time.Second, func() bool { return ts.runtime.Snapshot(id).Revision >= 1 })

	resp2 := ts.do(t, http.MethodGet, "/api/public/widgets/"+slug+"/config", nil)
	body2 := decodeSupporterWidgetsBody(t, resp2)
	latest, _ := body2["latest"].(map[string]any)
	if latest == nil || latest["displayName"] != "ada" {
		t.Fatalf("latest = %v, want displayName ada", body2["latest"])
	}
	for _, leaked := range []string{"id", "goalId", "providerEventId", "accounts", "providers"} {
		if _, present := body2[leaked]; present {
			t.Errorf("public config leaks internal field %q: %v", leaked, body2)
		}
	}
}

func TestPublicConfigDashboardComposesChildren(t *testing.T) {
	ts := newSupporterWidgetsTestServer(t)
	leaf := decodeSupporterWidgetsBody(t, ts.do(t, http.MethodPost, "/api/widget-profiles", latestFollowerWidgetBody()))
	leafID := leaf["id"].(string)

	dashBody := map[string]any{
		"kind": "dashboard", "name": "Dashboard", "enabled": true, "columns": 2,
		"children":    []map[string]any{{"widgetProfileId": leafID, "column": 1, "columnSpan": 1, "row": 1, "rowSpan": 1}},
		"orientation": "horizontal", "textAlign": "center", "fontFamily": "sans_serif",
		"backgroundColor": "#00000080", "foregroundColor": "#ffffff",
		"fillColor": "#7c3aed", "borderColor": "#ffffff33", "borderRadiusPx": 12, "opacity": 1.0,
	}
	dash := decodeSupporterWidgetsBody(t, ts.do(t, http.MethodPost, "/api/widget-profiles", dashBody))
	dashSlug := dash["publicSlug"].(string)

	resp := ts.do(t, http.MethodGet, "/api/public/widgets/"+dashSlug+"/config", nil)
	body := decodeSupporterWidgetsBody(t, resp)
	if body["kind"] != "dashboard" {
		t.Fatalf("kind = %v, want dashboard", body["kind"])
	}
	children, _ := body["dashboard"].([]any)
	if len(children) != 1 {
		t.Fatalf("dashboard children = %v, want 1", body["dashboard"])
	}
	child := children[0].(map[string]any)
	if _, present := child["widgetProfileId"]; present {
		t.Error("public dashboard child must never expose the internal widgetProfileId")
	}
	childSnapshot, _ := child["snapshot"].(map[string]any)
	if childSnapshot == nil || childSnapshot["kind"] != "latest_follower" {
		t.Fatalf("child snapshot = %v, want kind latest_follower", child["snapshot"])
	}
}
