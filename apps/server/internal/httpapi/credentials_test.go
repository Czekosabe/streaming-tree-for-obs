package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/credential"
	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/secrets/secretstest"
	"github.com/streaming-tree/server/internal/storage/sqlite"
)

// errFakeStoreFailure simulates an unexpected secrets.SecretStore failure
// via secretstest.Store.FailNext, distinct from Unavailable: it exercises
// the credential_store_failure path the real store cannot currently trigger.
var errFakeStoreFailure = errors.New("simulated store failure")

// newCredentialTestServer wires the real router over a real platform service
// (SQLite, in the test's own temporary directory) and a credential service
// backed by an in-memory fake store, so tests never touch a real OS
// credential store. It returns a seeded platform ID that tests can use.
func newCredentialTestServer(t *testing.T) (http.Handler, *secretstest.Store, string) {
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
	store := secretstest.New()
	credentials := credential.NewService(store)

	handler := NewRouter(Options{
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		StartedAt:   time.Now(),
		Platforms:   platforms,
		Credentials: credentials,
	})

	return handler, store, "pf_seed_twitch"
}

// --- GET status --------------------------------------------------------

func TestGetCredentialsReportsNotConfiguredForAFreshPlatform(t *testing.T) {
	handler, _, id := newCredentialTestServer(t)

	recorder := do(t, handler, http.MethodGet, "/api/platforms/"+id+"/credentials", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", recorder.Code, recorder.Body.String())
	}
	assertJSONContentType(t, recorder)

	var body struct {
		StreamKey struct {
			Configured bool `json:"configured"`
		} `json:"streamKey"`
		Store struct {
			Available bool `json:"available"`
		} `json:"store"`
	}
	decodeBody(t, recorder, &body)

	if body.StreamKey.Configured {
		t.Error("Configured = true for a platform that was never given a key")
	}
	if !body.Store.Available {
		t.Error("Store.Available = false, want true (the fake store is available)")
	}
}

func TestGetCredentialsForAnUnknownPlatformIsPlatformNotFound(t *testing.T) {
	handler, _, _ := newCredentialTestServer(t)

	recorder := do(t, handler, http.MethodGet, "/api/platforms/pf_does_not_exist/credentials", nil)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}

	var body ErrorBody
	decodeBody(t, recorder, &body)
	if body.Error != "platform_not_found" {
		t.Errorf("error code = %q, want platform_not_found", body.Error)
	}
}

func TestGetCredentialsReportsStoreUnavailable(t *testing.T) {
	handler, store, id := newCredentialTestServer(t)
	store.Unavailable = true

	recorder := do(t, handler, http.MethodGet, "/api/platforms/"+id+"/credentials", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (unavailability is a stable status, not an error)", recorder.Code)
	}

	var body struct {
		StreamKey struct {
			Configured bool `json:"configured"`
		} `json:"streamKey"`
		Store struct {
			Available bool `json:"available"`
		} `json:"store"`
	}
	decodeBody(t, recorder, &body)

	if body.StreamKey.Configured {
		t.Error("Configured = true while the store is unavailable")
	}
	if body.Store.Available {
		t.Error("Store.Available = true, want false")
	}
}

// --- PUT stream key ----------------------------------------------------

func TestPutStreamKeyStoresAndReportsConfigured(t *testing.T) {
	handler, store, id := newCredentialTestServer(t)

	recorder := do(t, handler, http.MethodPut, "/api/platforms/"+id+"/credentials/stream-key",
		map[string]string{"streamKey": "sk_live_abc123"})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", recorder.Code, recorder.Body.String())
	}

	var body struct {
		StreamKey struct {
			Configured bool `json:"configured"`
		} `json:"streamKey"`
	}
	decodeBody(t, recorder, &body)
	if !body.StreamKey.Configured {
		t.Error("Configured = false after a successful PUT")
	}

	if !store.Has("destination-stream-key:" + id) {
		t.Error("the value was not written to the store under the expected key")
	}
}

func TestPutStreamKeyResponseNeverEchoesTheValue(t *testing.T) {
	handler, _, id := newCredentialTestServer(t)

	const secretValue = "sk_live_should_never_appear_in_the_response"
	recorder := do(t, handler, http.MethodPut, "/api/platforms/"+id+"/credentials/stream-key",
		map[string]string{"streamKey": secretValue})

	if strings.Contains(recorder.Body.String(), secretValue) {
		t.Fatalf("the response echoed the stream key: %s", recorder.Body.String())
	}
}

func TestPutStreamKeyRejectsAnEmptyValue(t *testing.T) {
	handler, _, id := newCredentialTestServer(t)

	recorder := do(t, handler, http.MethodPut, "/api/platforms/"+id+"/credentials/stream-key",
		map[string]string{"streamKey": ""})
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body: %s", recorder.Code, recorder.Body.String())
	}

	var body ErrorBody
	decodeBody(t, recorder, &body)
	if body.Error != "validation_failed" {
		t.Errorf("error code = %q, want validation_failed", body.Error)
	}
	if _, ok := body.Fields["streamKey"]; !ok {
		t.Error("response is missing a streamKey field message")
	}
}

func TestPutStreamKeyValidationErrorNeverEchoesTheRejectedValue(t *testing.T) {
	handler, _, id := newCredentialTestServer(t)

	const secretLookingValue = "sk_live_rejected_value_should_not_appear\x07"
	recorder := do(t, handler, http.MethodPut, "/api/platforms/"+id+"/credentials/stream-key",
		map[string]string{"streamKey": secretLookingValue})

	if strings.Contains(recorder.Body.String(), "sk_live_rejected_value_should_not_appear") {
		t.Fatalf("the 422 response echoed the rejected value: %s", recorder.Body.String())
	}
}

func TestPutStreamKeyForAnUnknownPlatformIsPlatformNotFound(t *testing.T) {
	handler, _, _ := newCredentialTestServer(t)

	recorder := do(t, handler, http.MethodPut, "/api/platforms/pf_does_not_exist/credentials/stream-key",
		map[string]string{"streamKey": "sk_live_abc"})
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}

	var body ErrorBody
	decodeBody(t, recorder, &body)
	if body.Error != "platform_not_found" {
		t.Errorf("error code = %q, want platform_not_found", body.Error)
	}
}

func TestPutStreamKeyReplacesAnExistingValue(t *testing.T) {
	handler, store, id := newCredentialTestServer(t)

	do(t, handler, http.MethodPut, "/api/platforms/"+id+"/credentials/stream-key",
		map[string]string{"streamKey": "sk_live_first"})
	do(t, handler, http.MethodPut, "/api/platforms/"+id+"/credentials/stream-key",
		map[string]string{"streamKey": "sk_live_second"})

	got, err := store.Get(context.Background(), "destination-stream-key:"+id)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if string(got) != "sk_live_second" {
		t.Errorf("stored value = %q, want %q", got, "sk_live_second")
	}
}

func TestPutStreamKeyRejectsMalformedJSON(t *testing.T) {
	handler, _, id := newCredentialTestServer(t)

	recorder := do(t, handler, http.MethodPut, "/api/platforms/"+id+"/credentials/stream-key", "{not json")
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

func TestPutStreamKeyRejectsUnknownFields(t *testing.T) {
	handler, _, id := newCredentialTestServer(t)

	recorder := do(t, handler, http.MethodPut, "/api/platforms/"+id+"/credentials/stream-key",
		map[string]string{"streamKey": "sk_live_abc", "unexpected": "value"})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

func TestPutStreamKeyRejectsWrongContentType(t *testing.T) {
	handler, _, id := newCredentialTestServer(t)

	request := httptest.NewRequest(http.MethodPut, "/api/platforms/"+id+"/credentials/stream-key",
		strings.NewReader(`{"streamKey":"sk_live_abc"}`))
	request.Header.Set("Content-Type", "text/plain")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415", recorder.Code)
	}
}

func TestPutStreamKeyRejectsAnOversizedBody(t *testing.T) {
	handler, _, id := newCredentialTestServer(t)

	huge := strings.Repeat("a", maxCredentialRequestBodyBytes+1)
	recorder := do(t, handler, http.MethodPut, "/api/platforms/"+id+"/credentials/stream-key",
		map[string]string{"streamKey": huge})
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", recorder.Code)
	}
}

func TestPutStreamKeyReportsStoreUnavailable(t *testing.T) {
	handler, store, id := newCredentialTestServer(t)
	store.Unavailable = true

	recorder := do(t, handler, http.MethodPut, "/api/platforms/"+id+"/credentials/stream-key",
		map[string]string{"streamKey": "sk_live_abc"})
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}

	var body ErrorBody
	decodeBody(t, recorder, &body)
	if body.Error != "credential_store_unavailable" {
		t.Errorf("error code = %q, want credential_store_unavailable", body.Error)
	}
}

func TestPutStreamKeyReportsStoreFailure(t *testing.T) {
	handler, store, id := newCredentialTestServer(t)
	store.FailNext = errFakeStoreFailure

	recorder := do(t, handler, http.MethodPut, "/api/platforms/"+id+"/credentials/stream-key",
		map[string]string{"streamKey": "sk_live_abc"})
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}

	var body ErrorBody
	decodeBody(t, recorder, &body)
	if body.Error != "credential_store_failure" {
		t.Errorf("error code = %q, want credential_store_failure", body.Error)
	}
	if strings.Contains(body.Message, errFakeStoreFailure.Error()) {
		t.Errorf("the raw store error leaked into the response message: %q", body.Message)
	}
}

// --- DELETE stream key ---------------------------------------------------

func TestDeleteStreamKeyRemovesAConfiguredValue(t *testing.T) {
	handler, store, id := newCredentialTestServer(t)

	do(t, handler, http.MethodPut, "/api/platforms/"+id+"/credentials/stream-key",
		map[string]string{"streamKey": "sk_live_abc"})

	recorder := do(t, handler, http.MethodDelete, "/api/platforms/"+id+"/credentials/stream-key", nil)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", recorder.Code)
	}
	if recorder.Body.Len() != 0 {
		t.Errorf("DELETE response body = %q, want empty", recorder.Body.String())
	}

	if store.Has("destination-stream-key:" + id) {
		t.Error("value still present in the store after DELETE")
	}
}

func TestDeleteStreamKeyOnAnAbsentValueIsStillNoContent(t *testing.T) {
	handler, _, id := newCredentialTestServer(t)

	recorder := do(t, handler, http.MethodDelete, "/api/platforms/"+id+"/credentials/stream-key", nil)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (delete is idempotent)", recorder.Code)
	}
}

func TestDeleteStreamKeyForAnUnknownPlatformIsPlatformNotFound(t *testing.T) {
	handler, _, _ := newCredentialTestServer(t)

	recorder := do(t, handler, http.MethodDelete, "/api/platforms/pf_does_not_exist/credentials/stream-key", nil)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
}

// --- method / path contract ------------------------------------------------

func TestCredentialsEndpointsRejectWrongMethodsWithAllow(t *testing.T) {
	handler, _, id := newCredentialTestServer(t)

	cases := []struct {
		path   string
		method string
		allow  string
	}{
		{"/api/platforms/" + id + "/credentials", http.MethodPost, "GET"},
		{"/api/platforms/" + id + "/credentials/stream-key", http.MethodGet, "PUT, DELETE"},
		{"/api/platforms/" + id + "/credentials/stream-key", http.MethodPost, "PUT, DELETE"},
	}

	for _, tc := range cases {
		recorder := do(t, handler, tc.method, tc.path, nil)
		if recorder.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s: status = %d, want 405", tc.method, tc.path, recorder.Code)
		}
		if got := recorder.Header().Get("Allow"); got != tc.allow {
			t.Errorf("%s %s: Allow = %q, want %q", tc.method, tc.path, got, tc.allow)
		}
	}
}

// --- platform deletion cascade ----------------------------------------------

func TestDeletingAPlatformDeletesItsCredential(t *testing.T) {
	handler, store, id := newCredentialTestServer(t)

	do(t, handler, http.MethodPut, "/api/platforms/"+id+"/credentials/stream-key",
		map[string]string{"streamKey": "sk_live_abc"})

	recorder := do(t, handler, http.MethodDelete, "/api/platforms/"+id, nil)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body: %s", recorder.Code, recorder.Body.String())
	}

	if store.Has("destination-stream-key:" + id) {
		t.Error("the platform's credential survived platform deletion")
	}
}

func TestDeletingAPlatformWithNoCredentialStillSucceeds(t *testing.T) {
	handler, _, id := newCredentialTestServer(t)

	recorder := do(t, handler, http.MethodDelete, "/api/platforms/"+id, nil)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", recorder.Code)
	}
}

func TestDeletingOnePlatformDoesNotRemoveAnothersCredential(t *testing.T) {
	handler, store, id := newCredentialTestServer(t)
	const otherID = "pf_seed_kick"

	do(t, handler, http.MethodPut, "/api/platforms/"+id+"/credentials/stream-key",
		map[string]string{"streamKey": "sk_live_this_one"})
	do(t, handler, http.MethodPut, "/api/platforms/"+otherID+"/credentials/stream-key",
		map[string]string{"streamKey": "sk_live_the_other_one"})

	recorder := do(t, handler, http.MethodDelete, "/api/platforms/"+id, nil)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", recorder.Code)
	}

	if !store.Has("destination-stream-key:" + otherID) {
		t.Error("deleting one platform removed a different platform's credential")
	}
}

func TestDeletingAPlatformFailsWhenCredentialCleanupFails(t *testing.T) {
	handler, store, id := newCredentialTestServer(t)

	do(t, handler, http.MethodPut, "/api/platforms/"+id+"/credentials/stream-key",
		map[string]string{"streamKey": "sk_live_abc"})
	store.FailNext = errFakeStoreFailure

	recorder := do(t, handler, http.MethodDelete, "/api/platforms/"+id, nil)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500, body: %s", recorder.Code, recorder.Body.String())
	}

	// The platform must still exist: deletion must not proceed while
	// credential cleanup is in a confirmed-failed state.
	getRecorder := do(t, handler, http.MethodGet, "/api/platforms/"+id, nil)
	if getRecorder.Code != http.StatusOK {
		t.Errorf("platform GET after failed cleanup = %d, want 200 (platform must survive)", getRecorder.Code)
	}
}

func TestDeletingAPlatformProceedsWhenTheCredentialStoreIsUnavailable(t *testing.T) {
	handler, store, id := newCredentialTestServer(t)
	store.Unavailable = true

	recorder := do(t, handler, http.MethodDelete, "/api/platforms/"+id, nil)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (an unavailable store must not block ordinary platform deletion)", recorder.Code)
	}
}

// --- existing platform CRUD stays intact ------------------------------------

func TestPlatformCRUDStillWorksWithCredentialsWired(t *testing.T) {
	handler, _, id := newCredentialTestServer(t)

	recorder := do(t, handler, http.MethodGet, "/api/platforms/"+id, nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /api/platforms/{id} status = %d, want 200", recorder.Code)
	}

	recorder = do(t, handler, http.MethodPut, "/api/platforms/"+id, map[string]any{
		"displayName": "Renamed",
		"enabled":     true,
		"sortOrder":   0,
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("PUT /api/platforms/{id} status = %d, want 200, body: %s", recorder.Code, recorder.Body.String())
	}
}
