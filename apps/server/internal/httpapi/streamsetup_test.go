package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/streamsetup"
)

type stubStreamSetupService struct {
	profiles    map[string]streamsetup.Profile
	previews    map[string]streamsetup.Preview
	applyErr    error
	applyResult streamsetup.ApplyResult

	createErr         error
	lastCreate        streamsetup.CreateInput
	lastUpdate        streamsetup.UpdateInput
	deletedID         string
	dupName           string
	saveCurrentCalled bool
}

func (s *stubStreamSetupService) List(context.Context) ([]streamsetup.Profile, error) {
	out := make([]streamsetup.Profile, 0, len(s.profiles))
	for _, p := range s.profiles {
		out = append(out, p)
	}
	return out, nil
}

func (s *stubStreamSetupService) Get(_ context.Context, id string) (streamsetup.Profile, error) {
	p, ok := s.profiles[id]
	if !ok {
		return streamsetup.Profile{}, streamsetup.ErrNotFound
	}
	return p, nil
}

func (s *stubStreamSetupService) Create(_ context.Context, input streamsetup.CreateInput) (streamsetup.Profile, error) {
	s.lastCreate = input
	if s.createErr != nil {
		return streamsetup.Profile{}, s.createErr
	}
	p := streamsetup.Profile{ID: "setup_new", Name: input.Name, Note: input.Note, MetadataPresetID: input.MetadataPresetID}
	if s.profiles == nil {
		s.profiles = map[string]streamsetup.Profile{}
	}
	s.profiles[p.ID] = p
	return p, nil
}

func (s *stubStreamSetupService) Update(_ context.Context, id string, input streamsetup.UpdateInput) (streamsetup.Profile, error) {
	s.lastUpdate = input
	p, ok := s.profiles[id]
	if !ok {
		return streamsetup.Profile{}, streamsetup.ErrNotFound
	}
	p.Name, p.Note = input.Name, input.Note
	s.profiles[id] = p
	return p, nil
}

func (s *stubStreamSetupService) Delete(_ context.Context, id string) error {
	if _, ok := s.profiles[id]; !ok {
		return streamsetup.ErrNotFound
	}
	s.deletedID = id
	delete(s.profiles, id)
	return nil
}

func (s *stubStreamSetupService) Duplicate(_ context.Context, id, newName string) (streamsetup.Profile, error) {
	original, ok := s.profiles[id]
	if !ok {
		return streamsetup.Profile{}, streamsetup.ErrNotFound
	}
	s.dupName = newName
	dup := original
	dup.ID = "setup_dup"
	dup.Name = newName
	return dup, nil
}

func (s *stubStreamSetupService) SaveCurrent(_ context.Context, name, note string, presetID *string) (streamsetup.Profile, error) {
	s.saveCurrentCalled = true
	p := streamsetup.Profile{ID: "setup_saved", Name: name, Note: note, MetadataPresetID: presetID}
	return p, nil
}

func (s *stubStreamSetupService) Preview(_ context.Context, id string) (streamsetup.Preview, error) {
	p, ok := s.previews[id]
	if !ok {
		return streamsetup.Preview{}, streamsetup.ErrNotFound
	}
	return p, nil
}

func (s *stubStreamSetupService) Apply(_ context.Context, id string) (streamsetup.ApplyResult, error) {
	if _, ok := s.profiles[id]; !ok && s.previews[id].Profile.ID == "" {
		return streamsetup.ApplyResult{}, streamsetup.ErrNotFound
	}
	if s.applyErr != nil {
		return streamsetup.ApplyResult{}, s.applyErr
	}
	return s.applyResult, nil
}

func newStreamSetupServer(t *testing.T, service StreamSetupService) http.Handler {
	t.Helper()
	return NewRouter(Options{
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		StartedAt:    time.Now(),
		StreamSetups: service,
	})
}

func TestListStreamSetupsReturnsEveryProfile(t *testing.T) {
	stub := &stubStreamSetupService{profiles: map[string]streamsetup.Profile{
		"setup_1": {ID: "setup_1", Name: "Gaming"},
	}}
	handler := newStreamSetupServer(t, stub)

	recorder := do(t, handler, http.MethodGet, "/api/stream-setups", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", recorder.Code, recorder.Body.String())
	}
	var body []streamSetupProfileResponse
	decodeBody(t, recorder, &body)
	if len(body) != 1 || body[0].ID != "setup_1" {
		t.Fatalf("body = %+v", body)
	}
}

func TestGetStreamSetupUnknownIDReturns404(t *testing.T) {
	stub := &stubStreamSetupService{profiles: map[string]streamsetup.Profile{}}
	handler := newStreamSetupServer(t, stub)

	recorder := do(t, handler, http.MethodGet, "/api/stream-setups/setup_missing", nil)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
}

func TestCreateStreamSetupPassesDestinationsAndPresetThrough(t *testing.T) {
	stub := &stubStreamSetupService{profiles: map[string]streamsetup.Profile{}}
	handler := newStreamSetupServer(t, stub)

	presetID := "mp_1"
	recorder := do(t, handler, http.MethodPost, "/api/stream-setups", map[string]any{
		"name": "Gaming", "note": "n", "destinationIds": []string{"pf_1"}, "metadataPresetId": presetID,
	})
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body=%s", recorder.Code, recorder.Body.String())
	}
	if len(stub.lastCreate.DestinationIDs) != 1 || stub.lastCreate.DestinationIDs[0] != "pf_1" {
		t.Errorf("DestinationIDs = %+v, want [pf_1]", stub.lastCreate.DestinationIDs)
	}
	if stub.lastCreate.MetadataPresetID == nil || *stub.lastCreate.MetadataPresetID != "mp_1" {
		t.Errorf("MetadataPresetID = %v, want mp_1", stub.lastCreate.MetadataPresetID)
	}
}

func TestCreateStreamSetupDuplicateNameReturns409(t *testing.T) {
	stub := &stubStreamSetupService{profiles: map[string]streamsetup.Profile{}, createErr: streamsetup.ErrDuplicateName}
	handler := newStreamSetupServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/stream-setups", map[string]any{"name": "Gaming"})
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestDeleteStreamSetupRemovesIt(t *testing.T) {
	stub := &stubStreamSetupService{profiles: map[string]streamsetup.Profile{"setup_1": {ID: "setup_1", Name: "Gaming"}}}
	handler := newStreamSetupServer(t, stub)

	recorder := do(t, handler, http.MethodDelete, "/api/stream-setups/setup_1", nil)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body=%s", recorder.Code, recorder.Body.String())
	}
	if stub.deletedID != "setup_1" {
		t.Errorf("deletedID = %q, want setup_1", stub.deletedID)
	}
}

func TestDuplicateStreamSetupUsesTheProvidedName(t *testing.T) {
	stub := &stubStreamSetupService{profiles: map[string]streamsetup.Profile{"setup_1": {ID: "setup_1", Name: "Gaming"}}}
	handler := newStreamSetupServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/stream-setups/setup_1/duplicate", map[string]any{"name": "Gaming copy"})
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body=%s", recorder.Code, recorder.Body.String())
	}
	if stub.dupName != "Gaming copy" {
		t.Errorf("dupName = %q, want %q", stub.dupName, "Gaming copy")
	}
}

func TestSaveCurrentStreamSetupCallsTheService(t *testing.T) {
	stub := &stubStreamSetupService{}
	handler := newStreamSetupServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/stream-setups/save-current", map[string]any{"name": "Current"})
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body=%s", recorder.Code, recorder.Body.String())
	}
	if !stub.saveCurrentCalled {
		t.Error("SaveCurrent was not called")
	}
}

func TestPreviewStreamSetupReturnsClassifiedDestinations(t *testing.T) {
	stub := &stubStreamSetupService{previews: map[string]streamsetup.Preview{
		"setup_1": {
			Profile: streamsetup.Profile{ID: "setup_1", Name: "Gaming"},
			Destinations: []streamsetup.DestinationPreviewItem{
				{PlatformID: "pf_1", ProviderID: "twitch", DisplayName: "Twitch", Change: streamsetup.ChangeWillEnable},
			},
		},
	}}
	handler := newStreamSetupServer(t, stub)

	recorder := do(t, handler, http.MethodGet, "/api/stream-setups/setup_1/preview", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", recorder.Code, recorder.Body.String())
	}
	var body streamSetupPreviewResponse
	decodeBody(t, recorder, &body)
	if len(body.Destinations) != 1 || body.Destinations[0].Change != "will_enable" {
		t.Fatalf("body.Destinations = %+v", body.Destinations)
	}
}

func TestApplyStreamSetupReturnsTheResult(t *testing.T) {
	stub := &stubStreamSetupService{
		profiles:    map[string]streamsetup.Profile{"setup_1": {ID: "setup_1", Name: "Gaming"}},
		applyResult: streamsetup.ApplyResult{DestinationsChanged: 2, MetadataApplied: true},
	}
	handler := newStreamSetupServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/stream-setups/setup_1/apply", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", recorder.Code, recorder.Body.String())
	}
	var body applyStreamSetupResponse
	decodeBody(t, recorder, &body)
	if body.DestinationsChanged != 2 || !body.MetadataApplied {
		t.Fatalf("body = %+v", body)
	}
}

func TestApplyStreamSetupBlockedByActiveStreamReturns409(t *testing.T) {
	stub := &stubStreamSetupService{
		profiles: map[string]streamsetup.Profile{"setup_1": {ID: "setup_1", Name: "Gaming"}},
		applyErr: streamsetup.ErrActiveStreamBlocksApply,
	}
	handler := newStreamSetupServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/stream-setups/setup_1/apply", nil)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body=%s", recorder.Code, recorder.Body.String())
	}
}
