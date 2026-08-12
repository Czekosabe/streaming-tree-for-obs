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

	"github.com/coder/websocket"

	"github.com/streaming-tree/server/internal/domain/donationsource"
	bus "github.com/streaming-tree/server/internal/engagement"
	"github.com/streaming-tree/server/internal/provider/streamelements"
	"github.com/streaming-tree/server/internal/runtime/streamelementsengagement"
	"github.com/streaming-tree/server/internal/secrets/secretstest"
	"github.com/streaming-tree/server/internal/storage/sqlite"
)

// newFakeAstroTestServer is a minimal Astro-shaped WebSocket server that
// accepts a connection, sends welcome, and answers every subscribe
// request with success - enough for the HTTP-layer engagement tests
// below, which only assert on the HTTP contract (status codes, response
// shape), not the connector's own internal state machine (already
// thoroughly covered by internal/runtime/streamelementsengagement's own
// test suite).
func newFakeAstroTestServer(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		ctx := r.Context()

		welcome, _ := json.Marshal(streamelements.Envelope{Type: streamelements.MessageTypeWelcome})
		if err := conn.Write(ctx, websocket.MessageText, welcome); err != nil {
			return
		}
		for i := 0; i < 2; i++ {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var req streamelements.Envelope
			if err := json.Unmarshal(data, &req); err != nil {
				return
			}
			resp, _ := json.Marshal(streamelements.Envelope{Type: streamelements.MessageTypeResponse, Nonce: req.Nonce})
			if err := conn.Write(ctx, websocket.MessageText, resp); err != nil {
				return
			}
		}
		<-ctx.Done()
	}))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

// newDonationSourceTestServer wires the real router over a real
// donationsource.Service (SQLite in the test's own temporary directory, an
// in-memory fake SecretStore) and a real streamelementsengagement.Manager
// pointed at a local fake Astro server.
func newDonationSourceTestServer(t *testing.T) (http.Handler, *donationsource.Service) {
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

	astroURL := newFakeAstroTestServer(t)
	secretStore := secretstest.New()
	repo := sqlite.NewDonationSourceRepository(db.DB)

	var manager *streamelementsengagement.Manager
	sources := donationsource.NewService(donationsource.Options{
		Repository: repo, Secrets: secretStore,
		OnSourceRemoved: func(id string) {
			if manager != nil {
				manager.StopAndRemove(id)
			}
		},
	})

	eventBus := bus.New(bus.Options{Capacity: 100})
	t.Cleanup(eventBus.Shutdown)

	manager = streamelementsengagement.NewManager(streamelementsengagement.Options{
		Sources: sources, Secrets: secretStore, Bus: eventBus,
		Client: streamelements.New(streamelements.Options{WSBaseURL: astroURL}),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("manager.Start() error = %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		manager.Shutdown(ctx)
	})

	handler := NewRouter(Options{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), StartedAt: time.Now(),
		DonationSources: sources, DonationConnectors: manager,
	})

	return handler, sources
}

func createTestDonationSource(t *testing.T, handler http.Handler) string {
	t.Helper()
	recorder := do(t, handler, http.MethodPost, "/api/donation-sources", map[string]any{
		"providerId": "streamelements", "label": "Main channel", "remoteChannelId": "chan_1", "token": "jwt-token",
	})
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201, body: %s", recorder.Code, recorder.Body.String())
	}
	var body donationSourceResponse
	decodeBody(t, recorder, &body)
	return body.ID
}

// --- collection --------------------------------------------------------

func TestListDonationSourcesStartsEmpty(t *testing.T) {
	handler, _ := newDonationSourceTestServer(t)

	recorder := do(t, handler, http.MethodGet, "/api/donation-sources", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	assertJSONContentType(t, recorder)

	var body struct {
		Items []donationSourceResponse `json:"items"`
	}
	decodeBody(t, recorder, &body)
	if len(body.Items) != 0 {
		t.Fatalf("Items = %v, want empty", body.Items)
	}
}

func TestCreateDonationSourceNeverEchoesTheCredential(t *testing.T) {
	handler, _ := newDonationSourceTestServer(t)

	recorder := do(t, handler, http.MethodPost, "/api/donation-sources", map[string]any{
		"providerId": "streamelements", "label": "Main channel", "remoteChannelId": "chan_1", "token": "super-secret-jwt",
	})
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body: %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "super-secret-jwt") {
		t.Fatalf("response body leaked the credential: %s", recorder.Body.String())
	}

	var body donationSourceResponse
	decodeBody(t, recorder, &body)
	if body.ID == "" {
		t.Fatal("ID is empty")
	}
	if body.ProviderID != "streamelements" {
		t.Fatalf("ProviderID = %q, want streamelements", body.ProviderID)
	}
	if body.Enabled {
		t.Fatal("Enabled = true for a freshly created source, want false")
	}
	if !body.CredentialConfigured {
		t.Fatal("CredentialConfigured = false, want true immediately after create")
	}
	if loc := recorder.Header().Get("Location"); loc != "/api/donation-sources/"+body.ID {
		t.Fatalf("Location = %q, want /api/donation-sources/%s", loc, body.ID)
	}
}

func TestCreateDonationSourceRejectsMissingLabel(t *testing.T) {
	handler, _ := newDonationSourceTestServer(t)

	recorder := do(t, handler, http.MethodPost, "/api/donation-sources", map[string]any{
		"providerId": "streamelements", "label": "", "remoteChannelId": "chan_1", "token": "jwt-token",
	})
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body: %s", recorder.Code, recorder.Body.String())
	}
	var body ErrorBody
	decodeBody(t, recorder, &body)
	if body.Error != "invalid_label" {
		t.Errorf("error code = %q, want invalid_label", body.Error)
	}
}

func TestCreateDonationSourceRejectsMissingCredential(t *testing.T) {
	handler, _ := newDonationSourceTestServer(t)

	recorder := do(t, handler, http.MethodPost, "/api/donation-sources", map[string]any{
		"providerId": "streamelements", "label": "Main channel", "remoteChannelId": "chan_1", "token": "",
	})
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body: %s", recorder.Code, recorder.Body.String())
	}
	var body ErrorBody
	decodeBody(t, recorder, &body)
	if body.Error != "credential_required" {
		t.Errorf("error code = %q, want credential_required", body.Error)
	}
}

func TestCreateDonationSourceRejectsUnsupportedProvider(t *testing.T) {
	handler, _ := newDonationSourceTestServer(t)

	recorder := do(t, handler, http.MethodPost, "/api/donation-sources", map[string]any{
		"providerId": "streamlabs", "label": "Main channel", "remoteChannelId": "chan_1", "token": "jwt-token",
	})
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body: %s", recorder.Code, recorder.Body.String())
	}
	var body ErrorBody
	decodeBody(t, recorder, &body)
	if body.Error != "invalid_provider" {
		t.Errorf("error code = %q, want invalid_provider", body.Error)
	}
}

func TestCreateDonationSourceRejectsUnknownField(t *testing.T) {
	handler, _ := newDonationSourceTestServer(t)

	recorder := do(t, handler, http.MethodPost, "/api/donation-sources",
		`{"providerId":"streamelements","label":"Main","remoteChannelId":"chan_1","token":"jwt-token","email":"donor@example.com"}`)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body: %s", recorder.Code, recorder.Body.String())
	}
}

// --- single source -----------------------------------------------------

func TestGetDonationSourceRoundTrips(t *testing.T) {
	handler, _ := newDonationSourceTestServer(t)
	id := createTestDonationSource(t, handler)

	recorder := do(t, handler, http.MethodGet, "/api/donation-sources/"+id, nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var body donationSourceResponse
	decodeBody(t, recorder, &body)
	if body.ID != id || body.Label != "Main channel" || body.RemoteChannelID != "chan_1" {
		t.Fatalf("body = %+v, want the created source's own fields", body)
	}
}

func TestGetDonationSourceUnknownIsNotFound(t *testing.T) {
	handler, _ := newDonationSourceTestServer(t)

	recorder := do(t, handler, http.MethodGet, "/api/donation-sources/donsrc_missing", nil)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
	var body ErrorBody
	decodeBody(t, recorder, &body)
	if body.Error != "donation_source_not_found" {
		t.Errorf("error code = %q, want donation_source_not_found", body.Error)
	}
}

func TestUpdateDonationSourceReplacesSafeMetadataOnly(t *testing.T) {
	handler, sources := newDonationSourceTestServer(t)
	id := createTestDonationSource(t, handler)

	recorder := do(t, handler, http.MethodPut, "/api/donation-sources/"+id, map[string]any{
		"label": "Renamed", "remoteChannelId": "chan_2",
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", recorder.Code, recorder.Body.String())
	}
	var body donationSourceResponse
	decodeBody(t, recorder, &body)
	if body.Label != "Renamed" || body.RemoteChannelID != "chan_2" {
		t.Fatalf("body = %+v, want the updated label/remoteChannelId", body)
	}

	configured, err := sources.CredentialConfigured(context.Background(), id)
	if err != nil || !configured {
		t.Fatalf("CredentialConfigured() = %v, %v, want true, nil (metadata update must never touch the credential)", configured, err)
	}
}

func TestDeleteDonationSourceRemovesItAndItsCredential(t *testing.T) {
	handler, sources := newDonationSourceTestServer(t)
	id := createTestDonationSource(t, handler)

	recorder := do(t, handler, http.MethodDelete, "/api/donation-sources/"+id, nil)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", recorder.Code)
	}

	if _, found, err := sources.Get(context.Background(), id); err != nil || found {
		t.Fatalf("Get() after delete: found = %v, err = %v, want false, nil", found, err)
	}
	if configured, err := sources.CredentialConfigured(context.Background(), id); err != nil || configured {
		t.Fatalf("CredentialConfigured() after delete = %v, %v, want false, nil", configured, err)
	}
}

func TestMethodNotAllowedOnDonationSourceCollection(t *testing.T) {
	handler, _ := newDonationSourceTestServer(t)

	recorder := do(t, handler, http.MethodDelete, "/api/donation-sources", nil)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", recorder.Code)
	}
	if recorder.Header().Get("Allow") == "" {
		t.Error("Allow header is empty")
	}
}

// --- credential ----------------------------------------------------------

func TestReplaceDonationSourceCredentialNeverEchoesTheValue(t *testing.T) {
	handler, _ := newDonationSourceTestServer(t)
	id := createTestDonationSource(t, handler)

	recorder := do(t, handler, http.MethodPut, "/api/donation-sources/"+id+"/credential", map[string]any{
		"token": "rotated-secret-jwt",
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "rotated-secret-jwt") {
		t.Fatalf("response body leaked the rotated credential: %s", recorder.Body.String())
	}

	var body map[string]bool
	decodeBody(t, recorder, &body)
	if !body["configured"] {
		t.Fatal(`body["configured"] = false, want true`)
	}
}

func TestReplaceDonationSourceCredentialUnknownSourceIsNotFound(t *testing.T) {
	handler, _ := newDonationSourceTestServer(t)

	recorder := do(t, handler, http.MethodPut, "/api/donation-sources/donsrc_missing/credential", map[string]any{"token": "x"})
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
}

// --- engagement connector ------------------------------------------------

func TestGetDonationSourceEngagementDefaultsToDisabled(t *testing.T) {
	handler, _ := newDonationSourceTestServer(t)
	id := createTestDonationSource(t, handler)

	recorder := do(t, handler, http.MethodGet, "/api/donation-sources/"+id+"/engagement", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body: %s", recorder.Code, recorder.Body.String())
	}
	var body donationConnectorResponse
	decodeBody(t, recorder, &body)
	if body.Enabled {
		t.Fatal("Enabled = true for a freshly created source, want false")
	}
	if body.State != string(streamelementsengagement.StateDisabled) {
		t.Fatalf("State = %q, want disabled", body.State)
	}
}

func TestPutDonationSourceEngagementEnablesAndDisables(t *testing.T) {
	handler, _ := newDonationSourceTestServer(t)
	id := createTestDonationSource(t, handler)

	recorder := do(t, handler, http.MethodPut, "/api/donation-sources/"+id+"/engagement", map[string]any{"enabled": true})
	if recorder.Code != http.StatusOK {
		t.Fatalf("enable status = %d, want 200, body: %s", recorder.Code, recorder.Body.String())
	}
	var enabled donationConnectorResponse
	decodeBody(t, recorder, &enabled)
	if !enabled.Enabled {
		t.Fatal("Enabled = false immediately after enabling, want true")
	}

	recorder = do(t, handler, http.MethodPut, "/api/donation-sources/"+id+"/engagement", map[string]any{"enabled": false})
	if recorder.Code != http.StatusOK {
		t.Fatalf("disable status = %d, want 200, body: %s", recorder.Code, recorder.Body.String())
	}
	var disabled donationConnectorResponse
	decodeBody(t, recorder, &disabled)
	if disabled.Enabled || disabled.State != string(streamelementsengagement.StateDisabled) {
		t.Fatalf("body after disable = %+v, want Enabled=false, State=disabled", disabled)
	}
}

func TestPutDonationSourceEngagementUnknownSourceIsNotFound(t *testing.T) {
	handler, _ := newDonationSourceTestServer(t)

	recorder := do(t, handler, http.MethodPut, "/api/donation-sources/donsrc_missing/engagement", map[string]any{"enabled": true})
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
}

func TestRestartDonationSourceEngagementRequiresEmptyBody(t *testing.T) {
	handler, _ := newDonationSourceTestServer(t)
	id := createTestDonationSource(t, handler)

	recorder := do(t, handler, http.MethodPost, "/api/donation-sources/"+id+"/engagement/restart", `{"unexpected":true}`)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for an unexpected body, body: %s", recorder.Code, recorder.Body.String())
	}
}
