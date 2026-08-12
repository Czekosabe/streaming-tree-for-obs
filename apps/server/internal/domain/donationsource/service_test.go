package donationsource

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/secrets/secretstest"
)

// fakeRepository is a minimal in-memory Repository for Service unit tests.
type fakeRepository struct {
	mu      sync.Mutex
	sources map[string]Source

	// failNextCreate, when true, makes the next CreateSource call fail
	// with ErrStorage and clears itself - exercises Service.Create's own
	// credential-rollback path without relying on an id collision.
	failNextCreate bool
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{sources: make(map[string]Source)}
}

func (f *fakeRepository) GetSource(ctx context.Context, id string) (Source, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	src, ok := f.sources[id]
	if !ok {
		return Source{}, ErrNotFound
	}
	return src, nil
}

func (f *fakeRepository) ListSources(ctx context.Context) ([]Source, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Source, 0, len(f.sources))
	for _, src := range f.sources {
		out = append(out, src)
	}
	return out, nil
}

func (f *fakeRepository) CreateSource(ctx context.Context, src Source) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failNextCreate {
		f.failNextCreate = false
		return ErrStorage
	}
	if _, exists := f.sources[src.ID]; exists {
		return ErrConflict
	}
	f.sources[src.ID] = src
	return nil
}

func (f *fakeRepository) UpdateSource(ctx context.Context, src Source) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.sources[src.ID]; !exists {
		return ErrNotFound
	}
	f.sources[src.ID] = src
	return nil
}

func (f *fakeRepository) DeleteSource(ctx context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.sources, id)
	return nil
}

func newTestService(t *testing.T) (*Service, *secretstest.Store) {
	t.Helper()
	store := secretstest.New()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	svc := NewService(Options{
		Repository: newFakeRepository(), Secrets: store,
		Now: func() time.Time { return now },
	})
	return svc, store
}

func validCreateInput() CreateInput {
	return CreateInput{
		ProviderID: ProviderStreamElements, Label: "Main channel donations",
		RemoteChannelID: "5ad23dcc18fff500d78c5348", Token: "fake-jwt-token",
	}
}

func TestCreateStoresCredentialAndSafeMetadataOnly(t *testing.T) {
	svc, store := newTestService(t)
	src, err := svc.Create(context.Background(), validCreateInput())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if src.ID == "" || src.Enabled {
		t.Fatalf("expected a non-empty id and Enabled=false by default, got %+v", src)
	}
	if src.Label != "Main channel donations" || src.RemoteChannelID != "5ad23dcc18fff500d78c5348" {
		t.Fatalf("unexpected saved metadata: %+v", src)
	}

	configured, err := svc.CredentialConfigured(context.Background(), src.ID)
	if err != nil || !configured {
		t.Fatalf("CredentialConfigured() = (%v, %v), want (true, nil)", configured, err)
	}

	raw, err := LoadCredential(context.Background(), store, src.ID)
	if err != nil || raw != "fake-jwt-token" {
		t.Fatalf("LoadCredential() = (%q, %v), want (\"fake-jwt-token\", nil)", raw, err)
	}
}

func TestCreateRejectsMissingCredential(t *testing.T) {
	svc, _ := newTestService(t)
	in := validCreateInput()
	in.Token = ""
	if _, err := svc.Create(context.Background(), in); !errors.Is(err, ErrCredentialRequired) {
		t.Fatalf("Create() error = %v, want ErrCredentialRequired", err)
	}
}

func TestCreateRejectsUnsupportedProvider(t *testing.T) {
	svc, _ := newTestService(t)
	in := validCreateInput()
	in.ProviderID = ProviderID("streamlabs")
	if _, err := svc.Create(context.Background(), in); !errors.Is(err, ErrInvalidProvider) {
		t.Fatalf("Create() error = %v, want ErrInvalidProvider", err)
	}
}

func TestCreateRejectsEmptyLabel(t *testing.T) {
	svc, _ := newTestService(t)
	in := validCreateInput()
	in.Label = ""
	if _, err := svc.Create(context.Background(), in); !errors.Is(err, ErrInvalidLabel) {
		t.Fatalf("Create() error = %v, want ErrInvalidLabel", err)
	}
}

func TestCreateRejectsEmptyRemoteChannelID(t *testing.T) {
	svc, _ := newTestService(t)
	in := validCreateInput()
	in.RemoteChannelID = ""
	if _, err := svc.Create(context.Background(), in); !errors.Is(err, ErrInvalidRemoteChannelID) {
		t.Fatalf("Create() error = %v, want ErrInvalidRemoteChannelID", err)
	}
}

func TestUpdateMetadataNeverTouchesCredential(t *testing.T) {
	svc, store := newTestService(t)
	src, err := svc.Create(context.Background(), validCreateInput())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	updated, err := svc.UpdateMetadata(context.Background(), src.ID, UpdateInput{Label: "Renamed", RemoteChannelID: "new_channel"})
	if err != nil {
		t.Fatalf("UpdateMetadata() error = %v", err)
	}
	if updated.Label != "Renamed" || updated.RemoteChannelID != "new_channel" {
		t.Fatalf("unexpected metadata after update: %+v", updated)
	}
	raw, err := LoadCredential(context.Background(), store, src.ID)
	if err != nil || raw != "fake-jwt-token" {
		t.Fatalf("credential changed unexpectedly: (%q, %v)", raw, err)
	}
}

func TestReplaceCredentialAtomicallyRotatesTheStoredValue(t *testing.T) {
	svc, store := newTestService(t)
	src, err := svc.Create(context.Background(), validCreateInput())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := svc.ReplaceCredential(context.Background(), src.ID, "rotated-jwt-token"); err != nil {
		t.Fatalf("ReplaceCredential() error = %v", err)
	}
	raw, err := LoadCredential(context.Background(), store, src.ID)
	if err != nil || raw != "rotated-jwt-token" {
		t.Fatalf("LoadCredential() = (%q, %v), want (\"rotated-jwt-token\", nil)", raw, err)
	}
}

func TestSetEnabledPersistsOnlyTheEnabledFlag(t *testing.T) {
	svc, _ := newTestService(t)
	src, err := svc.Create(context.Background(), validCreateInput())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if src.Enabled {
		t.Fatal("expected a newly created source to start disabled")
	}
	enabled, err := svc.SetEnabled(context.Background(), src.ID, true)
	if err != nil {
		t.Fatalf("SetEnabled() error = %v", err)
	}
	if !enabled.Enabled || enabled.Label != src.Label {
		t.Fatalf("unexpected source after SetEnabled: %+v", enabled)
	}
}

func TestDeleteRemovesSourceAndCredentialAndNotifies(t *testing.T) {
	svc, _ := newTestService(t)
	var notified string
	svc.onSourceRemoved = func(id string) { notified = id }

	src, err := svc.Create(context.Background(), validCreateInput())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := svc.Delete(context.Background(), src.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if notified != src.ID {
		t.Fatalf("OnSourceRemoved called with %q, want %q", notified, src.ID)
	}
	if _, found, err := svc.Get(context.Background(), src.ID); err != nil || found {
		t.Fatalf("Get() after delete = (found=%v, err=%v), want (false, nil)", found, err)
	}
	configured, err := svc.CredentialConfigured(context.Background(), src.ID)
	if err != nil || configured {
		t.Fatalf("CredentialConfigured() after delete = (%v, %v), want (false, nil)", configured, err)
	}
}

func TestSourceExistsMatchesGet(t *testing.T) {
	svc, _ := newTestService(t)
	src, err := svc.Create(context.Background(), validCreateInput())
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	exists, err := svc.SourceExists(context.Background(), src.ID)
	if err != nil || !exists {
		t.Fatalf("SourceExists() = (%v, %v), want (true, nil)", exists, err)
	}
	exists, err = svc.SourceExists(context.Background(), "donsrc_missing")
	if err != nil || exists {
		t.Fatalf("SourceExists() for a missing id = (%v, %v), want (false, nil)", exists, err)
	}
}

func TestCreateRollsBackCredentialWhenRepositoryCreateFails(t *testing.T) {
	store := secretstest.New()
	repo := newFakeRepository()
	repo.failNextCreate = true
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	svc := NewService(Options{Repository: repo, Secrets: store, Now: func() time.Time { return now }})
	svc.newID = func() (string, error) { return "donsrc_fixed", nil }

	if _, err := svc.Create(context.Background(), validCreateInput()); err == nil {
		t.Fatal("expected Create() to fail when the repository rejects the row")
	}
	// The row never became visible - its just-stored credential must not
	// be left behind as an orphaned secret no row references.
	configured, err := CredentialConfigured(context.Background(), store, "donsrc_fixed")
	if err != nil || configured {
		t.Fatalf("CredentialConfigured() after a rolled-back Create() = (%v, %v), want (false, nil)", configured, err)
	}
}
