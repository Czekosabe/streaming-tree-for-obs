package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/output"
	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/storage/sqlite"
)

// newOutputTestServer wires the real router over a real platform service and
// a real output-settings service, both backed by SQLite in the test's own
// temporary directory. Returns a seeded platform ID that tests can use.
func newOutputTestServer(t *testing.T) (http.Handler, string) {
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

	platforms := platform.NewService(sqlite.NewPlatformRepository(db.DB))
	outputs := output.NewService(sqlite.NewOutputRepository(db.DB))

	handler := NewRouter(Options{
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		StartedAt: time.Now(),
		Platforms: platforms,
		Outputs:   outputs,
	})

	return handler, "pf_seed_twitch"
}

func TestGetOutputSettingsReturnsTheDefaultUnconfiguredRow(t *testing.T) {
	handler, id := newOutputTestServer(t)

	recorder := do(t, handler, http.MethodGet, "/api/platforms/"+id+"/output", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", recorder.Code, recorder.Body.String())
	}
	assertJSONContentType(t, recorder)

	var body struct {
		ServerURL   string `json:"serverUrl"`
		AutoRestart bool   `json:"autoRestart"`
	}
	decodeBody(t, recorder, &body)

	if body.ServerURL != "" {
		t.Errorf("ServerURL = %q, want empty", body.ServerURL)
	}
	if !body.AutoRestart {
		t.Error("AutoRestart = false, want the default true")
	}
}

func TestGetOutputSettingsForAnUnknownPlatformIsPlatformNotFound(t *testing.T) {
	handler, _ := newOutputTestServer(t)

	recorder := do(t, handler, http.MethodGet, "/api/platforms/pf_does_not_exist/output", nil)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}

	var body ErrorBody
	decodeBody(t, recorder, &body)
	if body.Error != "platform_not_found" {
		t.Errorf("error code = %q, want platform_not_found", body.Error)
	}
}

func TestPutOutputSettingsStoresAValidRTMPAddress(t *testing.T) {
	handler, id := newOutputTestServer(t)

	recorder := do(t, handler, http.MethodPut, "/api/platforms/"+id+"/output", map[string]any{
		"serverUrl":   "rtmp://live.example.invalid/app",
		"autoRestart": true,
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", recorder.Code, recorder.Body.String())
	}

	var body struct {
		ServerURL string `json:"serverUrl"`
	}
	decodeBody(t, recorder, &body)
	if body.ServerURL != "rtmp://live.example.invalid/app" {
		t.Errorf("ServerURL = %q", body.ServerURL)
	}
}

func TestPutOutputSettingsRejectsAnInvalidURL(t *testing.T) {
	handler, id := newOutputTestServer(t)

	recorder := do(t, handler, http.MethodPut, "/api/platforms/"+id+"/output", map[string]any{
		"serverUrl":   "not a url",
		"autoRestart": true,
	})
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body: %s", recorder.Code, recorder.Body.String())
	}

	var body ErrorBody
	decodeBody(t, recorder, &body)
	if body.Error != "validation_failed" {
		t.Errorf("error code = %q, want validation_failed", body.Error)
	}
	if _, ok := body.Fields["serverUrl"]; !ok {
		t.Error("response is missing a serverUrl field message")
	}
}

func TestPutOutputSettingsForAnUnknownPlatformIsPlatformNotFound(t *testing.T) {
	handler, _ := newOutputTestServer(t)

	recorder := do(t, handler, http.MethodPut, "/api/platforms/pf_does_not_exist/output", map[string]any{
		"serverUrl":   "rtmp://example.invalid/app",
		"autoRestart": true,
	})
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
}

func TestPutOutputSettingsRejectsMalformedJSON(t *testing.T) {
	handler, id := newOutputTestServer(t)

	recorder := do(t, handler, http.MethodPut, "/api/platforms/"+id+"/output", "{not json")
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

func TestPutOutputSettingsRejectsUnknownFields(t *testing.T) {
	handler, id := newOutputTestServer(t)

	recorder := do(t, handler, http.MethodPut, "/api/platforms/"+id+"/output", map[string]any{
		"serverUrl":   "rtmp://example.invalid/app",
		"autoRestart": true,
		"streamKey":   "sk_live_should_be_rejected",
	})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

func TestPutOutputSettingsRejectsOversizedBody(t *testing.T) {
	handler, id := newOutputTestServer(t)

	huge := make([]byte, maxRequestBodyBytes+1)
	for i := range huge {
		huge[i] = 'a'
	}
	recorder := do(t, handler, http.MethodPut, "/api/platforms/"+id+"/output", map[string]any{
		"serverUrl":   "rtmp://example.invalid/" + string(huge),
		"autoRestart": true,
	})
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", recorder.Code)
	}
}

func TestOutputSettingsEndpointsRejectWrongMethodsWithAllow(t *testing.T) {
	handler, id := newOutputTestServer(t)

	recorder := do(t, handler, http.MethodPost, "/api/platforms/"+id+"/output", nil)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", recorder.Code)
	}
	if got := recorder.Header().Get("Allow"); got != "GET, PUT" {
		t.Errorf("Allow = %q, want %q", got, "GET, PUT")
	}
}

func TestOutputSettingsPersistAcrossRequests(t *testing.T) {
	handler, id := newOutputTestServer(t)

	do(t, handler, http.MethodPut, "/api/platforms/"+id+"/output", map[string]any{
		"serverUrl":   "rtmps://live.example.invalid/app",
		"autoRestart": false,
	})

	recorder := do(t, handler, http.MethodGet, "/api/platforms/"+id+"/output", nil)
	var body struct {
		ServerURL   string `json:"serverUrl"`
		AutoRestart bool   `json:"autoRestart"`
	}
	decodeBody(t, recorder, &body)

	if body.ServerURL != "rtmps://live.example.invalid/app" {
		t.Errorf("ServerURL = %q", body.ServerURL)
	}
	if body.AutoRestart {
		t.Error("AutoRestart = true, want the stored false")
	}
}

func TestExistingPlatformCRUDStillWorksWithOutputsWired(t *testing.T) {
	handler, id := newOutputTestServer(t)

	recorder := do(t, handler, http.MethodGet, "/api/platforms/"+id, nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /api/platforms/{id} status = %d, want 200", recorder.Code)
	}
}
