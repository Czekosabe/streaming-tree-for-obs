package httpapi

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/metadatapreset"
	"github.com/streaming-tree/server/internal/domain/platform"
)

// stubMetadataPresets is a controllable MetadataPresetService for
// handler tests - the domain service's own behaviour is covered by its
// package tests, so this is only about the HTTP contract.
type stubMetadataPresets struct {
	presets   map[string]metadatapreset.Preset
	listErr   error
	getErr    error
	createErr error
	updateErr error
	deleteErr error

	previewResult  []metadatapreset.DestinationPreview
	previewErr     error
	lastPreviewIDs []string

	applyResult  map[string]platform.Metadata
	applyErr     error
	lastApplyIDs []string
}

func newStubMetadataPresets() *stubMetadataPresets {
	return &stubMetadataPresets{presets: map[string]metadatapreset.Preset{}}
}

func (s *stubMetadataPresets) List(ctx context.Context) ([]metadatapreset.Preset, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	out := make([]metadatapreset.Preset, 0, len(s.presets))
	for _, p := range s.presets {
		out = append(out, p)
	}
	return out, nil
}

func (s *stubMetadataPresets) Get(ctx context.Context, id string) (metadatapreset.Preset, error) {
	if s.getErr != nil {
		return metadatapreset.Preset{}, s.getErr
	}
	p, ok := s.presets[id]
	if !ok {
		return metadatapreset.Preset{}, metadatapreset.ErrNotFound
	}
	return p, nil
}

func (s *stubMetadataPresets) Create(ctx context.Context, input metadatapreset.CreateInput) (metadatapreset.Preset, error) {
	if s.createErr != nil {
		return metadatapreset.Preset{}, s.createErr
	}
	p := metadatapreset.Preset{
		ID: "mp_stub", Name: input.Name, Note: input.Note, Common: input.Common, Providers: input.Providers,
		CreatedAt: time.Unix(0, 0).UTC(), UpdatedAt: time.Unix(0, 0).UTC(),
	}
	s.presets[p.ID] = p
	return p, nil
}

func (s *stubMetadataPresets) Update(ctx context.Context, id string, input metadatapreset.UpdateInput) (metadatapreset.Preset, error) {
	if s.updateErr != nil {
		return metadatapreset.Preset{}, s.updateErr
	}
	existing, ok := s.presets[id]
	if !ok {
		return metadatapreset.Preset{}, metadatapreset.ErrNotFound
	}
	existing.Name, existing.Note, existing.Common, existing.Providers = input.Name, input.Note, input.Common, input.Providers
	s.presets[id] = existing
	return existing, nil
}

func (s *stubMetadataPresets) Delete(ctx context.Context, id string) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	if _, ok := s.presets[id]; !ok {
		return metadatapreset.ErrNotFound
	}
	delete(s.presets, id)
	return nil
}

func (s *stubMetadataPresets) ApplyPreview(ctx context.Context, id string, platformIDs []string) ([]metadatapreset.DestinationPreview, error) {
	s.lastPreviewIDs = platformIDs
	if s.previewErr != nil {
		return nil, s.previewErr
	}
	return s.previewResult, nil
}

func (s *stubMetadataPresets) Apply(ctx context.Context, id string, platformIDs []string) (map[string]platform.Metadata, error) {
	s.lastApplyIDs = platformIDs
	if s.applyErr != nil {
		return nil, s.applyErr
	}
	return s.applyResult, nil
}

func newMetadataPresetServer(t *testing.T, service MetadataPresetService) http.Handler {
	t.Helper()
	return NewRouter(Options{
		Logger:          slog.New(slog.NewTextHandler(io.Discard, nil)),
		StartedAt:       time.Now(),
		MetadataPresets: service,
	})
}

func TestListPresetsEmptyByDefault(t *testing.T) {
	stub := newStubMetadataPresets()
	handler := newMetadataPresetServer(t, stub)

	recorder := do(t, handler, http.MethodGet, "/api/metadata-presets", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var body []presetResponse
	decodeBody(t, recorder, &body)
	if len(body) != 0 {
		t.Fatalf("body = %v, want an empty list", body)
	}
}

func TestCreatePresetThenGetIt(t *testing.T) {
	stub := newStubMetadataPresets()
	handler := newMetadataPresetServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/metadata-presets", map[string]any{
		"name":  "Just Chatting",
		"title": "Hanging out",
		"tags":  []string{"chat"},
		"providers": map[string]any{
			"twitch": map[string]string{"category": "Just Chatting", "categoryId": "509658"},
		},
	})
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body=%s", recorder.Code, recorder.Body.String())
	}
	var created presetResponse
	decodeBody(t, recorder, &created)
	if created.Name != "Just Chatting" || created.Title != "Hanging out" {
		t.Fatalf("created = %+v", created)
	}
	if created.Providers["twitch"].CategoryID != "509658" {
		t.Fatalf("providers.twitch.categoryId = %q, want 509658", created.Providers["twitch"].CategoryID)
	}

	getRecorder := do(t, handler, http.MethodGet, "/api/metadata-presets/"+created.ID, nil)
	if getRecorder.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", getRecorder.Code)
	}
}

func TestCreatePresetRejectsBlankName(t *testing.T) {
	stub := newStubMetadataPresets()
	stub.createErr = &platform.ValidationError{}
	stub.createErr.(*platform.ValidationError).Add("name", platform.RuleRequired, "Preset name is required.", nil)
	handler := newMetadataPresetServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/metadata-presets", map[string]any{"name": ""})
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestCreatePresetRejectsUnknownField(t *testing.T) {
	stub := newStubMetadataPresets()
	handler := newMetadataPresetServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/metadata-presets", map[string]any{
		"name": "ok", "streamKey": "should-never-be-accepted",
	})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestCreatePresetDuplicateNameIsConflict(t *testing.T) {
	stub := newStubMetadataPresets()
	stub.createErr = metadatapreset.ErrDuplicateName
	handler := newMetadataPresetServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/metadata-presets", map[string]any{"name": "dup"})
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", recorder.Code)
	}
}

func TestGetPresetNotFound(t *testing.T) {
	stub := newStubMetadataPresets()
	handler := newMetadataPresetServer(t, stub)

	recorder := do(t, handler, http.MethodGet, "/api/metadata-presets/mp_missing", nil)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
}

func TestUpdatePreset(t *testing.T) {
	stub := newStubMetadataPresets()
	stub.presets["mp_1"] = metadatapreset.Preset{ID: "mp_1", Name: "Old"}
	handler := newMetadataPresetServer(t, stub)

	recorder := do(t, handler, http.MethodPut, "/api/metadata-presets/mp_1", map[string]any{"name": "New"})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", recorder.Code, recorder.Body.String())
	}
	var updated presetResponse
	decodeBody(t, recorder, &updated)
	if updated.Name != "New" {
		t.Fatalf("Name = %q, want New", updated.Name)
	}
}

func TestDeletePreset(t *testing.T) {
	stub := newStubMetadataPresets()
	stub.presets["mp_1"] = metadatapreset.Preset{ID: "mp_1", Name: "To delete"}
	handler := newMetadataPresetServer(t, stub)

	recorder := do(t, handler, http.MethodDelete, "/api/metadata-presets/mp_1", nil)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", recorder.Code)
	}
	if _, stillThere := stub.presets["mp_1"]; stillThere {
		t.Fatal("preset should have been deleted")
	}
}

func TestDeletePresetRejectsBody(t *testing.T) {
	stub := newStubMetadataPresets()
	stub.presets["mp_1"] = metadatapreset.Preset{ID: "mp_1", Name: "x"}
	handler := newMetadataPresetServer(t, stub)

	recorder := do(t, handler, http.MethodDelete, "/api/metadata-presets/mp_1", map[string]string{"unexpected": "body"})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

func TestMetadataPresetTooManyIsUnprocessable(t *testing.T) {
	stub := newStubMetadataPresets()
	stub.createErr = metadatapreset.ErrTooMany
	handler := newMetadataPresetServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/metadata-presets", map[string]any{"name": "one too many"})
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", recorder.Code)
	}
}

func TestMetadataPresetWrongMethodIsRejected(t *testing.T) {
	stub := newStubMetadataPresets()
	handler := newMetadataPresetServer(t, stub)

	recorder := do(t, handler, http.MethodPatch, "/api/metadata-presets", nil)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", recorder.Code)
	}
}

func TestApplyPreviewParsesCommaSeparatedPlatformIDs(t *testing.T) {
	stub := newStubMetadataPresets()
	stub.previewResult = []metadatapreset.DestinationPreview{
		{
			PlatformID: "pf_1", ProviderID: platform.ProviderTwitch, Valid: true,
			Fields: []metadatapreset.FieldPreview{{Field: "title", Status: metadatapreset.FieldWillChange}},
		},
	}
	handler := newMetadataPresetServer(t, stub)

	recorder := do(t, handler, http.MethodGet, "/api/metadata-presets/mp_1/apply-preview?platformIds=pf_1,pf_2", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", recorder.Code, recorder.Body.String())
	}
	if got := stub.lastPreviewIDs; len(got) != 2 || got[0] != "pf_1" || got[1] != "pf_2" {
		t.Fatalf("lastPreviewIDs = %v, want [pf_1 pf_2]", got)
	}

	var body []applyDestinationResponse
	decodeBody(t, recorder, &body)
	if len(body) != 1 || body[0].PlatformID != "pf_1" || !body[0].Valid {
		t.Fatalf("body = %+v", body)
	}
	if body[0].Fields[0].Status != "will_change" {
		t.Errorf("field status = %q, want will_change", body[0].Fields[0].Status)
	}
}

func TestApplyPresetWritesAndReturnsUpdatedDestinations(t *testing.T) {
	stub := newStubMetadataPresets()
	stub.applyResult = map[string]platform.Metadata{
		"pf_1": {Title: "Applied title", Tags: []string{}},
	}
	handler := newMetadataPresetServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/metadata-presets/mp_1/apply",
		map[string]any{"platformIds": []string{"pf_1"}})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", recorder.Code, recorder.Body.String())
	}
	if got := stub.lastApplyIDs; len(got) != 1 || got[0] != "pf_1" {
		t.Fatalf("lastApplyIDs = %v, want [pf_1]", got)
	}

	var body applyPresetResponse
	decodeBody(t, recorder, &body)
	if body.Platforms["pf_1"].Title != "Applied title" {
		t.Fatalf("body = %+v", body)
	}
}

func TestApplyPresetRejectsAllOrNothingValidationFailure(t *testing.T) {
	stub := newStubMetadataPresets()
	stub.applyErr = &metadatapreset.ApplyValidationError{
		Destinations: map[string][]platform.FieldViolation{
			"pf_1": {{Field: "title", Rule: platform.RuleTooLong, Message: "Title cannot exceed 25 characters."}},
		},
	}
	handler := newMetadataPresetServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/metadata-presets/mp_1/apply",
		map[string]any{"platformIds": []string{"pf_1", "pf_2"}})
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body=%s", recorder.Code, recorder.Body.String())
	}

	var body ErrorBody
	decodeBody(t, recorder, &body)
	if _, ok := body.Fields["pf_1.title"]; !ok {
		t.Fatalf("Fields = %+v, want a \"pf_1.title\" entry (platform-prefixed)", body.Fields)
	}
}

// Regression: PlatformMetadataStore.GetMany's own platform.ErrNotFound was
// falling through to the generic 500 branch, because it is never wrapped
// into a metadatapreset sentinel first - found by
// scripts/verify-metadata-presets.mjs against the real server, not by an
// earlier, too-narrowly-mocked unit test.
func TestApplyPresetUnknownPlatformIsNotFound(t *testing.T) {
	stub := newStubMetadataPresets()
	stub.applyErr = fmt.Errorf("%w: platform pf_missing", platform.ErrNotFound)
	handler := newMetadataPresetServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/metadata-presets/mp_1/apply",
		map[string]any{"platformIds": []string{"pf_missing"}})
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestApplyPresetRejectsBodyWithUnknownField(t *testing.T) {
	stub := newStubMetadataPresets()
	handler := newMetadataPresetServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/metadata-presets/mp_1/apply",
		map[string]any{"platformIds": []string{"pf_1"}, "streamKey": "nope"})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestApplyPreviewWrongMethodIsRejected(t *testing.T) {
	stub := newStubMetadataPresets()
	handler := newMetadataPresetServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/metadata-presets/mp_1/apply-preview", nil)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", recorder.Code)
	}
}

func TestApplyWrongMethodIsRejected(t *testing.T) {
	stub := newStubMetadataPresets()
	handler := newMetadataPresetServer(t, stub)

	recorder := do(t, handler, http.MethodGet, "/api/metadata-presets/mp_1/apply", nil)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", recorder.Code)
	}
}

// A structural proof, not merely a UI-omission claim: a create request
// containing a plausible secret-shaped field is rejected as unknown,
// never silently accepted or stored.
func TestCreatePresetRejectsSecretShapedFields(t *testing.T) {
	stub := newStubMetadataPresets()
	handler := newMetadataPresetServer(t, stub)

	for _, field := range []string{"streamKey", "accessToken", "refreshToken", "clientSecret", "password"} {
		recorder := do(t, handler, http.MethodPost, "/api/metadata-presets", map[string]any{
			"name": "ok", field: "should-never-be-accepted",
		})
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("field %q: status = %d, want 400 (unknown field rejected)", field, recorder.Code)
		}
	}
}
