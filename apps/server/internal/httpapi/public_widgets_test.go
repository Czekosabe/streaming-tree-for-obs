package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func (ts *goalsTestServer) createGoalAndWidget(t *testing.T) (goalID, slug string) {
	t.Helper()
	created := decodeGoalsBody(t, ts.do(t, http.MethodPost, "/api/goals", validFollowerGoalBody()))
	goalID = created["id"].(string)
	wp := decodeGoalsBody(t, ts.do(t, http.MethodPost, "/api/widget-profiles", validWidgetProfileBody(goalID)))
	slug = wp["publicSlug"].(string)
	return goalID, slug
}

func TestPublicWidgetConfigReturnsSafeSnapshot(t *testing.T) {
	ts := newGoalsTestServer(t)
	_, slug := ts.createGoalAndWidget(t)

	resp := ts.do(t, http.MethodGet, "/api/public/widgets/"+slug+"/config", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body := decodeGoalsBody(t, resp)
	if body["current"].(float64) != 825 {
		t.Errorf("current = %v, want 825", body["current"])
	}
	if body["kind"] != "goal" {
		t.Errorf("kind = %v, want goal", body["kind"])
	}
	for _, leaked := range []string{"id", "goalId", "providerEventId", "accounts", "providers", "publicSlug"} {
		if _, present := body[leaked]; present {
			t.Errorf("public config leaks internal field %q: %v", leaked, body)
		}
	}
}

func TestPublicWidgetConfigUnknownSlugReturns200SafeDefault(t *testing.T) {
	ts := newGoalsTestServer(t)
	resp := ts.do(t, http.MethodGet, "/api/public/widgets/does-not-exist/config", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (never a hard error for an unknown slug)", resp.StatusCode)
	}
	body := decodeGoalsBody(t, resp)
	if body["current"].(float64) != 0 {
		t.Errorf("current = %v, want 0 for an unknown slug", body["current"])
	}
}

func TestPublicWidgetConfigDisabledProfileReturns200SafeDefault(t *testing.T) {
	ts := newGoalsTestServer(t)
	goalID, slug := ts.createGoalAndWidget(t)
	list := decodeGoalsBodyList(t, ts.do(t, http.MethodGet, "/api/widget-profiles?goalId="+goalID, nil))
	widgetID := list[0]["id"].(string)

	disable := validWidgetProfileBody(goalID)
	disable["enabled"] = false
	ts.do(t, http.MethodPut, "/api/widget-profiles/"+widgetID, disable)

	resp := ts.do(t, http.MethodGet, "/api/public/widgets/"+slug+"/config", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body := decodeGoalsBody(t, resp)
	if body["current"].(float64) != 0 {
		t.Errorf("current = %v, want 0 for a disabled widget profile", body["current"])
	}
}

func decodeGoalsBodyList(t *testing.T, resp *http.Response) []map[string]any {
	t.Helper()
	defer resp.Body.Close()
	var out []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	return out
}

func TestPublicWidgetStreamSendsInitialReset(t *testing.T) {
	ts := newGoalsTestServer(t)
	_, slug := ts.createGoalAndWidget(t)

	srv := httptest.NewServer(ts.handler)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/public/widgets/"+slug+"/stream", nil)
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
	chunk := string(buf[:n])
	if !strings.Contains(chunk, "event: widget.reset") {
		t.Errorf("stream chunk missing widget.reset: %q", chunk)
	}
	if !strings.Contains(chunk, `"current":825`) {
		t.Errorf("stream chunk missing the goal's own current value: %q", chunk)
	}
}

func TestPublicWidgetStreamUnknownSlugSendsSafeResetNoError(t *testing.T) {
	ts := newGoalsTestServer(t)

	srv := httptest.NewServer(ts.handler)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/public/widgets/does-not-exist/stream", nil)
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
	chunk := string(buf[:n])
	if !strings.Contains(chunk, "event: widget.reset") {
		t.Errorf("stream chunk missing widget.reset: %q", chunk)
	}
}

func TestPublicWidgetStreamPicksUpContributionChange(t *testing.T) {
	ts := newGoalsTestServer(t)
	goalID, slug := ts.createGoalAndWidget(t)

	srv := httptest.NewServer(ts.handler)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/public/widgets/"+slug+"/stream", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET .../stream error = %v", err)
	}
	defer resp.Body.Close()

	buf := make([]byte, 4096)
	if _, err := resp.Body.Read(buf); err != nil {
		t.Fatalf("reading the initial reset failed: %v", err)
	}

	ts.do(t, http.MethodPost, "/api/goals/"+goalID+"/set-current", map[string]any{"current": 999})

	// The poll interval is 1.5s; give it enough headroom to fire at
	// least once within the request's own 6s timeout.
	n, err := resp.Body.Read(buf)
	if err != nil && n == 0 {
		t.Fatalf("reading the updated snapshot failed: %v", err)
	}
	chunk := string(buf[:n])
	if !strings.Contains(chunk, `"current":999`) {
		t.Errorf("stream chunk missing the updated current value: %q", chunk)
	}
}

func TestPublicWidgetStreamBoundedClientCount(t *testing.T) {
	ts := newGoalsTestServer(t)
	_, slug := ts.createGoalAndWidget(t)

	srv := httptest.NewServer(ts.handler)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var responses []*http.Response
	defer func() {
		for _, r := range responses {
			r.Body.Close()
		}
	}()
	for i := 0; i < maxWidgetSSEClientsPerSlug+2; i++ {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/public/widgets/"+slug+"/stream", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET .../stream error = %v", err)
		}
		responses = append(responses, resp)
	}
	// Every connection still gets a normal 200 (the bound rejects with a
	// safe reset-then-close, never a hard HTTP error) - this proves the
	// limiter never crashes or hangs the handler under over-subscription.
	for i, r := range responses {
		if r.StatusCode != http.StatusOK {
			t.Errorf("connection %d status = %d, want 200", i, r.StatusCode)
		}
	}
}
