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

	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/storage/sqlite"
)

// newTestServer wires the real router over a real service backed by a database
// in the test's own temporary directory. Handler tests therefore exercise the
// full path down to SQL without ever touching the user's database.
func newTestServer(t *testing.T) http.Handler {
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

	service := platform.NewService(sqlite.NewPlatformRepository(db.DB))

	return NewRouter(Options{
		// Discard log output so a deliberate error case does not spam the run.
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		StartedAt: time.Now(),
		Platforms: service,
	})
}

func do(t *testing.T, handler http.Handler, method, target string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var reader io.Reader
	if body != nil {
		switch typed := body.(type) {
		case string:
			reader = strings.NewReader(typed)
		default:
			encoded, err := json.Marshal(typed)
			if err != nil {
				t.Fatalf("encoding the request body failed: %v", err)
			}
			reader = bytes.NewReader(encoded)
		}
	}

	request := httptest.NewRequest(method, target, reader)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func decodeBody(t *testing.T, recorder *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(recorder.Body.Bytes(), target); err != nil {
		t.Fatalf("decoding the response failed: %v\nbody: %s", err, recorder.Body.String())
	}
}

func assertJSONContentType(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	contentType := recorder.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", contentType)
	}
}

// --- definitions ------------------------------------------------------------

func TestGetPlatformDefinitions(t *testing.T) {
	handler := newTestServer(t)

	recorder := do(t, handler, http.MethodGet, "/api/platform-definitions", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	assertJSONContentType(t, recorder)

	var body struct {
		Definitions []platform.ProviderDefinition `json:"definitions"`
	}
	decodeBody(t, recorder, &body)

	if len(body.Definitions) != 4 {
		t.Fatalf("returned %d definitions, want 4", len(body.Definitions))
	}
	if body.Definitions[0].ID != platform.ProviderTwitch {
		t.Errorf("first definition = %q, want twitch", body.Definitions[0].ID)
	}
	if !body.Definitions[0].Capabilities.Tags {
		t.Error("twitch definition lost tag support over the wire")
	}
}

// --- list -------------------------------------------------------------------

func TestGetPlatformsReturnsSeededConfigurations(t *testing.T) {
	handler := newTestServer(t)

	recorder := do(t, handler, http.MethodGet, "/api/platforms", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}

	var body struct {
		Platforms []platformResponse `json:"platforms"`
	}
	decodeBody(t, recorder, &body)

	if len(body.Platforms) != 4 {
		t.Fatalf("returned %d platforms, want 4", len(body.Platforms))
	}

	first := body.Platforms[0]
	if first.Provider == nil {
		t.Fatal("the response carries no provider definition, so the card cannot render")
	}
	if first.Enabled {
		t.Error("a seeded platform is enabled, want all disabled")
	}
	if len(first.Metadata.Tags) != 4 {
		t.Errorf("twitch returned %d tags, want the 4 seeded ones", len(first.Metadata.Tags))
	}
}

// --- create -----------------------------------------------------------------

func TestPostPlatformCreatesAConfiguration(t *testing.T) {
	handler := newTestServer(t)

	recorder := do(t, handler, http.MethodPost, "/api/platforms", map[string]any{
		"providerId":  "twitch",
		"displayName": "  Second Twitch channel  ",
		"enabled":     true,
	})
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201\nbody: %s", recorder.Code, recorder.Body.String())
	}

	var created platformResponse
	decodeBody(t, recorder, &created)

	if created.ID == "" {
		t.Fatal("the created platform has no backend-generated id")
	}
	if created.DisplayName != "Second Twitch channel" {
		t.Errorf("DisplayName = %q, want it trimmed", created.DisplayName)
	}
	if !created.Enabled {
		t.Error("Enabled = false, want true")
	}

	location := recorder.Header().Get("Location")
	if location != "/api/platforms/"+created.ID {
		t.Errorf("Location = %q, want /api/platforms/%s", location, created.ID)
	}

	// A second destination for the same provider must be allowed.
	if again := do(t, handler, http.MethodPost, "/api/platforms", map[string]any{
		"providerId": "twitch", "displayName": "Third Twitch channel", "enabled": false,
	}); again.Code != http.StatusCreated {
		t.Errorf("a second twitch destination returned %d, want 201", again.Code)
	}
}

func TestPostPlatformRejectsUnknownProvider(t *testing.T) {
	handler := newTestServer(t)

	recorder := do(t, handler, http.MethodPost, "/api/platforms", map[string]any{
		"providerId": "myspace", "displayName": "Nope", "enabled": false,
	})
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", recorder.Code)
	}

	var body ErrorBody
	decodeBody(t, recorder, &body)
	if body.Error != "unknown_provider" {
		t.Errorf("error = %q, want unknown_provider", body.Error)
	}
}

func TestPostPlatformReturnsFieldDetailsForValidationFailure(t *testing.T) {
	handler := newTestServer(t)

	recorder := do(t, handler, http.MethodPost, "/api/platforms", map[string]any{
		"providerId": "twitch", "displayName": "   ", "enabled": false,
	})
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", recorder.Code)
	}

	var body ErrorBody
	decodeBody(t, recorder, &body)

	if body.Error != "validation_failed" {
		t.Errorf("error = %q, want validation_failed", body.Error)
	}
	if _, ok := body.Fields["displayName"]; !ok {
		t.Errorf("fields = %v, want an entry for displayName", body.Fields)
	}
	detail, ok := body.Details["displayName"]
	if !ok {
		t.Fatalf("details = %v, want an entry for displayName", body.Details)
	}
	if detail.Rule != platform.RuleRequired {
		t.Errorf("rule = %q, want %q", detail.Rule, platform.RuleRequired)
	}
}

func TestPostPlatformRejectsMalformedJSON(t *testing.T) {
	handler := newTestServer(t)

	recorder := do(t, handler, http.MethodPost, "/api/platforms", `{"providerId": `)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}

	var body ErrorBody
	decodeBody(t, recorder, &body)
	if body.Error != "malformed_json" {
		t.Errorf("error = %q, want malformed_json", body.Error)
	}
}

func TestPostPlatformRejectsUnknownJSONField(t *testing.T) {
	handler := newTestServer(t)

	// A stream key must never be silently accepted and dropped.
	recorder := do(t, handler, http.MethodPost, "/api/platforms",
		`{"providerId":"twitch","displayName":"X","enabled":false,"streamKey":"secret"}`)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}

	var body ErrorBody
	decodeBody(t, recorder, &body)
	if body.Error != "unknown_field" {
		t.Errorf("error = %q, want unknown_field", body.Error)
	}
	if strings.Contains(strings.ToLower(body.Message), "secret") {
		t.Error("the error message echoes the rejected value back to the client")
	}
}

func TestPostPlatformRejectsOversizedBody(t *testing.T) {
	handler := newTestServer(t)

	huge := `{"providerId":"twitch","enabled":false,"displayName":"` +
		strings.Repeat("a", maxRequestBodyBytes+1024) + `"}`

	recorder := do(t, handler, http.MethodPost, "/api/platforms", huge)
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", recorder.Code)
	}

	var body ErrorBody
	decodeBody(t, recorder, &body)
	if body.Error != "request_too_large" {
		t.Errorf("error = %q, want request_too_large", body.Error)
	}
}

// --- single platform --------------------------------------------------------

func TestGetPlatformByID(t *testing.T) {
	handler := newTestServer(t)

	recorder := do(t, handler, http.MethodGet, "/api/platforms/pf_seed_youtube", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}

	var body platformResponse
	decodeBody(t, recorder, &body)
	if body.ProviderID != platform.ProviderYouTube {
		t.Errorf("providerId = %q, want youtube", body.ProviderID)
	}
}

func TestGetPlatformReturnsNotFound(t *testing.T) {
	handler := newTestServer(t)

	recorder := do(t, handler, http.MethodGet, "/api/platforms/pf_missing", nil)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
	assertJSONContentType(t, recorder)

	var body ErrorBody
	decodeBody(t, recorder, &body)
	if body.Error != "not_found" {
		t.Errorf("error = %q, want not_found", body.Error)
	}
}

func TestPutPlatformUpdatesConfiguration(t *testing.T) {
	handler := newTestServer(t)

	recorder := do(t, handler, http.MethodPut, "/api/platforms/pf_seed_kick", map[string]any{
		"displayName": "Kick main", "enabled": true, "sortOrder": 9,
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200\nbody: %s", recorder.Code, recorder.Body.String())
	}

	var body platformResponse
	decodeBody(t, recorder, &body)
	if body.DisplayName != "Kick main" || !body.Enabled || body.SortOrder != 9 {
		t.Errorf("update did not apply: %+v", body)
	}
}

func TestPutPlatformReturnsNotFound(t *testing.T) {
	handler := newTestServer(t)

	recorder := do(t, handler, http.MethodPut, "/api/platforms/pf_missing", map[string]any{
		"displayName": "X", "enabled": false, "sortOrder": 0,
	})
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
}

func TestDeletePlatform(t *testing.T) {
	handler := newTestServer(t)

	recorder := do(t, handler, http.MethodDelete, "/api/platforms/pf_seed_tiktok", nil)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", recorder.Code)
	}
	if recorder.Body.Len() != 0 {
		t.Errorf("204 response carries a body: %q", recorder.Body.String())
	}

	if again := do(t, handler, http.MethodGet, "/api/platforms/pf_seed_tiktok", nil); again.Code != http.StatusNotFound {
		t.Errorf("after deletion GET returned %d, want 404", again.Code)
	}
}

func TestDeletePlatformReturnsNotFound(t *testing.T) {
	handler := newTestServer(t)

	recorder := do(t, handler, http.MethodDelete, "/api/platforms/pf_missing", nil)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
}

// --- metadata ---------------------------------------------------------------

func TestGetPlatformMetadata(t *testing.T) {
	handler := newTestServer(t)

	recorder := do(t, handler, http.MethodGet, "/api/platforms/pf_seed_twitch/metadata", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}

	var body metadataResponse
	decodeBody(t, recorder, &body)
	if len(body.Tags) != 4 || body.Tags[0] != "programming" {
		t.Errorf("tags = %v, want the seeded ordered list", body.Tags)
	}
}

func TestPutPlatformMetadataSavesTagsInOrder(t *testing.T) {
	handler := newTestServer(t)

	recorder := do(t, handler, http.MethodPut, "/api/platforms/pf_seed_twitch/metadata", map[string]any{
		"title": "Live coding", "description": "", "category": "Software and Game Development",
		"tags": []string{"zebra", "alpha"}, "language": "pl", "visibility": "",
		"matureContent": false, "dvr": false, "latencyMode": "low",
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200\nbody: %s", recorder.Code, recorder.Body.String())
	}

	var body metadataResponse
	decodeBody(t, recorder, &body)
	if len(body.Tags) != 2 || body.Tags[0] != "zebra" || body.Tags[1] != "alpha" {
		t.Errorf("tags = %v, want [zebra alpha] in that order", body.Tags)
	}

	// Read back through a separate request to prove it was persisted.
	reread := do(t, handler, http.MethodGet, "/api/platforms/pf_seed_twitch/metadata", nil)
	var stored metadataResponse
	decodeBody(t, reread, &stored)
	if stored.Title != "Live coding" {
		t.Errorf("stored title = %q, want %q", stored.Title, "Live coding")
	}
}

func TestPutPlatformMetadataRejectsTagsForUnsupportedProvider(t *testing.T) {
	handler := newTestServer(t)

	recorder := do(t, handler, http.MethodPut, "/api/platforms/pf_seed_youtube/metadata", map[string]any{
		"title": "Fine", "description": "", "category": "Science & Technology",
		"tags": []string{"nope"}, "language": "pl", "visibility": "public",
		"matureContent": false, "dvr": false, "latencyMode": "low",
	})
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", recorder.Code)
	}

	var body ErrorBody
	decodeBody(t, recorder, &body)
	if _, ok := body.Fields["tags"]; !ok {
		t.Errorf("fields = %v, want an entry for tags", body.Fields)
	}
}

func TestPutPlatformMetadataRejectsOversizedTitle(t *testing.T) {
	handler := newTestServer(t)

	recorder := do(t, handler, http.MethodPut, "/api/platforms/pf_seed_tiktok/metadata", map[string]any{
		"title": strings.Repeat("x", 61), "description": "", "category": "Gaming",
		"tags": []string{}, "language": "", "visibility": "",
		"matureContent": false, "dvr": false, "latencyMode": "",
	})
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", recorder.Code)
	}

	var body ErrorBody
	decodeBody(t, recorder, &body)
	detail, ok := body.Details["title"]
	if !ok {
		t.Fatalf("details = %v, want an entry for title", body.Details)
	}
	if detail.Rule != platform.RuleTooLong {
		t.Errorf("rule = %q, want %q", detail.Rule, platform.RuleTooLong)
	}
	if max, ok := detail.Params["max"]; !ok || max != float64(60) {
		t.Errorf("params = %v, want max=60 so the client can localize the message", detail.Params)
	}
}

func TestPutPlatformMetadataReturnsNotFound(t *testing.T) {
	handler := newTestServer(t)

	recorder := do(t, handler, http.MethodPut, "/api/platforms/pf_missing/metadata", map[string]any{
		"title": "X", "description": "", "category": "", "tags": []string{},
		"language": "", "visibility": "", "matureContent": false, "dvr": false, "latencyMode": "",
	})
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
}

// --- protocol behaviour -----------------------------------------------------

func TestUnsupportedMethodReturns405WithAllow(t *testing.T) {
	handler := newTestServer(t)

	cases := map[string]struct {
		target string
		method string
		allow  string
	}{
		"definitions": {"/api/platform-definitions", http.MethodPost, "GET"},
		"collection":  {"/api/platforms", http.MethodDelete, "GET, POST"},
		"item":        {"/api/platforms/pf_seed_twitch", http.MethodPatch, "GET, PUT, DELETE"},
		"metadata":    {"/api/platforms/pf_seed_twitch/metadata", http.MethodPost, "GET, PUT"},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			recorder := do(t, handler, testCase.method, testCase.target, nil)
			if recorder.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status = %d, want 405", recorder.Code)
			}
			if allow := recorder.Header().Get("Allow"); allow != testCase.allow {
				t.Errorf("Allow = %q, want %q", allow, testCase.allow)
			}
			assertJSONContentType(t, recorder)
		})
	}
}

func TestUnknownAPIPathStillReturns404(t *testing.T) {
	handler := newTestServer(t)

	recorder := do(t, handler, http.MethodGet, "/api/nope", nil)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}

	var body ErrorBody
	decodeBody(t, recorder, &body)
	if body.Error != "not_found" {
		t.Errorf("error = %q, want not_found", body.Error)
	}
}

func TestHealthEndpointStillWorks(t *testing.T) {
	handler := newTestServer(t)

	recorder := do(t, handler, http.MethodGet, "/api/health", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
}

func TestErrorsNeverLeakSQLiteDetail(t *testing.T) {
	handler := newTestServer(t)

	// Probe the error paths a client can reach and confirm none of them
	// mentions the driver, the schema or the database file.
	responses := []*httptest.ResponseRecorder{
		do(t, handler, http.MethodGet, "/api/platforms/pf_missing", nil),
		do(t, handler, http.MethodDelete, "/api/platforms/pf_missing", nil),
		do(t, handler, http.MethodPost, "/api/platforms", `{"providerId":"nope","displayName":"x","enabled":false}`),
		do(t, handler, http.MethodPost, "/api/platforms", `{`),
	}

	forbidden := []string{"sqlite", "sql:", "SELECT", "INSERT", "constraint", ".db"}
	for i, recorder := range responses {
		body := recorder.Body.String()
		for _, needle := range forbidden {
			if strings.Contains(strings.ToLower(body), strings.ToLower(needle)) {
				t.Errorf("response %d leaks internal detail %q: %s", i, needle, body)
			}
		}
	}
}

func TestResponsesCarryNoCredentialFields(t *testing.T) {
	handler := newTestServer(t)

	recorder := do(t, handler, http.MethodGet, "/api/platforms", nil)
	body := strings.ToLower(recorder.Body.String())

	for _, forbidden := range []string{"streamkey", "stream_key", "token", "secret", "password", "credential"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("the platform payload contains %q, which must never be stored or returned", forbidden)
		}
	}
}
