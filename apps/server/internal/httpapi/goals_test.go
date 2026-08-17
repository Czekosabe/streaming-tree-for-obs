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

	domain "github.com/streaming-tree/server/internal/domain/goals"
	"github.com/streaming-tree/server/internal/storage/sqlite"
)

type alwaysExistsLookup struct{}

func (alwaysExistsLookup) AccountExists(context.Context, string) (bool, error) { return true, nil }

type goalsTestServer struct {
	handler http.Handler
	svc     *domain.Service
}

func newGoalsTestServer(t *testing.T) *goalsTestServer {
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
	handler := NewRouter(Options{Logger: logger, StartedAt: time.Now(), Goals: svc})
	return &goalsTestServer{handler: handler, svc: svc}
}

func (ts *goalsTestServer) do(t *testing.T, method, path string, body any) *http.Response {
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

func decodeGoalsBody(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	return out
}

func validFollowerGoalBody() map[string]any {
	return map[string]any{
		"name": "Followers", "kind": "followers", "enabled": true,
		"target": 1000, "baseline": 825, "providers": []string{}, "accounts": []string{},
	}
}

func TestCreateGoalReturns201AndLocation(t *testing.T) {
	ts := newGoalsTestServer(t)
	resp := ts.do(t, http.MethodPost, "/api/goals", validFollowerGoalBody())
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	if resp.Header.Get("Location") == "" {
		t.Error("expected a Location header")
	}
	body := decodeGoalsBody(t, resp)
	if body["current"].(float64) != 825 {
		t.Errorf("current = %v, want 825 (the operator's own baseline)", body["current"])
	}
	if body["kind"] != "followers" {
		t.Errorf("kind = %v, want followers", body["kind"])
	}
}

func TestCreateGoalInvalidReturns422(t *testing.T) {
	ts := newGoalsTestServer(t)
	bad := validFollowerGoalBody()
	bad["target"] = 0
	resp := ts.do(t, http.MethodPost, "/api/goals", bad)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", resp.StatusCode)
	}
	body := decodeGoalsBody(t, resp)
	if body["error"] != "goal_invalid" {
		t.Errorf("error = %v, want goal_invalid", body["error"])
	}
}

func TestGetGoalNotFoundReturns404(t *testing.T) {
	ts := newGoalsTestServer(t)
	resp := ts.do(t, http.MethodGet, "/api/goals/goal_nope", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	body := decodeGoalsBody(t, resp)
	if body["error"] != "goal_not_found" {
		t.Errorf("error = %v, want goal_not_found", body["error"])
	}
}

func TestListGoalsReturnsCreated(t *testing.T) {
	ts := newGoalsTestServer(t)
	ts.do(t, http.MethodPost, "/api/goals", validFollowerGoalBody())

	resp := ts.do(t, http.MethodGet, "/api/goals", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	defer resp.Body.Close()
	var list []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("len(list) = %d, want 1", len(list))
	}
}

func TestUpdateGoalRoundTripsAndBumpsRevision(t *testing.T) {
	ts := newGoalsTestServer(t)
	created := decodeGoalsBody(t, ts.do(t, http.MethodPost, "/api/goals", validFollowerGoalBody()))
	id := created["id"].(string)

	update := validFollowerGoalBody()
	update["name"] = "Renamed"
	update["configRevision"] = created["configRevision"]
	resp := ts.do(t, http.MethodPut, "/api/goals/"+id, update)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body := decodeGoalsBody(t, resp)
	if body["name"] != "Renamed" {
		t.Errorf("name = %v, want Renamed", body["name"])
	}
	if body["configRevision"] == created["configRevision"] {
		t.Error("configRevision must bump on a successful update")
	}
}

func TestUpdateGoalStaleRevisionReturns409(t *testing.T) {
	ts := newGoalsTestServer(t)
	created := decodeGoalsBody(t, ts.do(t, http.MethodPost, "/api/goals", validFollowerGoalBody()))
	id := created["id"].(string)

	update := validFollowerGoalBody()
	update["configRevision"] = float64(999)
	resp := ts.do(t, http.MethodPut, "/api/goals/"+id, update)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
	body := decodeGoalsBody(t, resp)
	if body["error"] != "goal_config_conflict" {
		t.Errorf("error = %v, want goal_config_conflict", body["error"])
	}
}

func TestDeleteGoalReturns204(t *testing.T) {
	ts := newGoalsTestServer(t)
	created := decodeGoalsBody(t, ts.do(t, http.MethodPost, "/api/goals", validFollowerGoalBody()))
	id := created["id"].(string)

	resp := ts.do(t, http.MethodDelete, "/api/goals/"+id, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
}

func TestDeleteGoalInUseReturns409(t *testing.T) {
	ts := newGoalsTestServer(t)
	created := decodeGoalsBody(t, ts.do(t, http.MethodPost, "/api/goals", validFollowerGoalBody()))
	id := created["id"].(string)
	ts.do(t, http.MethodPost, "/api/widget-profiles", validWidgetProfileBody(id))

	resp := ts.do(t, http.MethodDelete, "/api/goals/"+id, nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
	body := decodeGoalsBody(t, resp)
	if body["error"] != "goal_in_use" {
		t.Errorf("error = %v, want goal_in_use", body["error"])
	}
}

func TestSetGoalCurrent(t *testing.T) {
	ts := newGoalsTestServer(t)
	created := decodeGoalsBody(t, ts.do(t, http.MethodPost, "/api/goals", validFollowerGoalBody()))
	id := created["id"].(string)

	resp := ts.do(t, http.MethodPost, "/api/goals/"+id+"/set-current", map[string]any{"current": 500})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body := decodeGoalsBody(t, resp)
	if body["current"].(float64) != 500 {
		t.Errorf("current = %v, want 500", body["current"])
	}
}

func TestResetGoal(t *testing.T) {
	ts := newGoalsTestServer(t)
	created := decodeGoalsBody(t, ts.do(t, http.MethodPost, "/api/goals", validFollowerGoalBody()))
	id := created["id"].(string)
	ts.do(t, http.MethodPost, "/api/goals/"+id+"/set-current", map[string]any{"current": 500})

	resp := ts.do(t, http.MethodPost, "/api/goals/"+id+"/reset", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body := decodeGoalsBody(t, resp)
	if body["current"].(float64) != 825 {
		t.Errorf("current = %v, want 825 (the goal's own baseline)", body["current"])
	}
}

func TestGoalsMethodNotAllowed(t *testing.T) {
	ts := newGoalsTestServer(t)
	resp := ts.do(t, http.MethodPatch, "/api/goals", nil)
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
	if resp.Header.Get("Allow") == "" {
		t.Error("expected an Allow header")
	}
}

// --- widget profiles -----------------------------------------------------

func validWidgetProfileBody(goalID string) map[string]any {
	return map[string]any{
		"kind": "goal", "goalId": goalID, "name": "Widget", "enabled": true,
		"showCurrent": true, "showTarget": true, "showPercent": true,
		"showProvider": true, "showTime": true,
		"orientation": "horizontal", "textAlign": "center", "fontFamily": "sans_serif",
		"backgroundColor": "#00000080", "foregroundColor": "#ffffff",
		"fillColor": "#7c3aed", "borderColor": "#ffffff33", "borderRadiusPx": 12, "opacity": 1.0,
	}
}

func TestCreateWidgetProfileReturns201WithPublicSlug(t *testing.T) {
	ts := newGoalsTestServer(t)
	created := decodeGoalsBody(t, ts.do(t, http.MethodPost, "/api/goals", validFollowerGoalBody()))
	id := created["id"].(string)

	resp := ts.do(t, http.MethodPost, "/api/widget-profiles", validWidgetProfileBody(id))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	body := decodeGoalsBody(t, resp)
	slug, _ := body["publicSlug"].(string)
	if len(slug) < 20 {
		t.Errorf("publicSlug = %q, want a high-entropy slug", slug)
	}
}

func TestCreateWidgetProfileUnknownGoalReturns404(t *testing.T) {
	ts := newGoalsTestServer(t)
	resp := ts.do(t, http.MethodPost, "/api/widget-profiles", validWidgetProfileBody("goal_nope"))
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestListWidgetProfilesFiltersByGoalID(t *testing.T) {
	ts := newGoalsTestServer(t)
	g1 := decodeGoalsBody(t, ts.do(t, http.MethodPost, "/api/goals", validFollowerGoalBody()))["id"].(string)
	g2Body := validFollowerGoalBody()
	g2Body["name"] = "Other"
	g2 := decodeGoalsBody(t, ts.do(t, http.MethodPost, "/api/goals", g2Body))["id"].(string)
	ts.do(t, http.MethodPost, "/api/widget-profiles", validWidgetProfileBody(g1))
	ts.do(t, http.MethodPost, "/api/widget-profiles", validWidgetProfileBody(g2))

	resp := ts.do(t, http.MethodGet, "/api/widget-profiles?goalId="+g1, nil)
	defer resp.Body.Close()
	var list []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("len(list) = %d, want 1", len(list))
	}
	if list[0]["goalId"] != g1 {
		t.Errorf("goalId = %v, want %v", list[0]["goalId"], g1)
	}
}

func TestRotateWidgetProfilePublicSlugChangesIt(t *testing.T) {
	ts := newGoalsTestServer(t)
	g := decodeGoalsBody(t, ts.do(t, http.MethodPost, "/api/goals", validFollowerGoalBody()))["id"].(string)
	created := decodeGoalsBody(t, ts.do(t, http.MethodPost, "/api/widget-profiles", validWidgetProfileBody(g)))
	id := created["id"].(string)
	oldSlug := created["publicSlug"].(string)

	resp := ts.do(t, http.MethodPost, "/api/widget-profiles/"+id+"/rotate-public-slug", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body := decodeGoalsBody(t, resp)
	if body["publicSlug"] == oldSlug {
		t.Error("publicSlug must change after rotation")
	}
}

func TestDeleteWidgetProfileReturns204(t *testing.T) {
	ts := newGoalsTestServer(t)
	g := decodeGoalsBody(t, ts.do(t, http.MethodPost, "/api/goals", validFollowerGoalBody()))["id"].(string)
	created := decodeGoalsBody(t, ts.do(t, http.MethodPost, "/api/widget-profiles", validWidgetProfileBody(g)))
	id := created["id"].(string)

	resp := ts.do(t, http.MethodDelete, "/api/widget-profiles/"+id, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
}

func TestDonationGoalRequiresCurrency(t *testing.T) {
	ts := newGoalsTestServer(t)
	body := map[string]any{
		"name": "Fund", "kind": "donations", "enabled": true,
		"target": 100000000, "baseline": 0, "providers": []string{}, "accounts": []string{},
	}
	resp := ts.do(t, http.MethodPost, "/api/goals", body)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", resp.StatusCode)
	}
}

func TestDonationGoalWithCurrencySucceeds(t *testing.T) {
	ts := newGoalsTestServer(t)
	body := map[string]any{
		"name": "Fund", "kind": "donations", "enabled": true,
		"target": 100000000, "baseline": 0, "currency": "USD", "providers": []string{}, "accounts": []string{},
	}
	resp := ts.do(t, http.MethodPost, "/api/goals", body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	respBody := decodeGoalsBody(t, resp)
	if respBody["currency"] != "USD" {
		t.Errorf("currency = %v, want USD", respBody["currency"])
	}
}
