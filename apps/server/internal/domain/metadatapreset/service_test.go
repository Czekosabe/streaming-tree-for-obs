package metadatapreset

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/platform"
)

type fakeRepository struct {
	byID   map[string]Preset
	byName map[string]string // lowercase name -> id
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{byID: map[string]Preset{}, byName: map[string]string{}}
}

func (f *fakeRepository) List(ctx context.Context) ([]Preset, error) {
	out := make([]Preset, 0, len(f.byID))
	for _, p := range f.byID {
		out = append(out, p)
	}
	return out, nil
}

func (f *fakeRepository) Get(ctx context.Context, id string) (Preset, error) {
	p, ok := f.byID[id]
	if !ok {
		return Preset{}, ErrNotFound
	}
	return p, nil
}

func (f *fakeRepository) Count(ctx context.Context) (int, error) {
	return len(f.byID), nil
}

func lower(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			r += 'a' - 'A'
		}
		out = append(out, r)
	}
	return string(out)
}

func (f *fakeRepository) Create(ctx context.Context, p Preset) error {
	if _, taken := f.byName[lower(p.Name)]; taken {
		return ErrDuplicateName
	}
	f.byID[p.ID] = p
	f.byName[lower(p.Name)] = p.ID
	return nil
}

func (f *fakeRepository) Update(ctx context.Context, id string, input UpdateInput, updatedAt string) error {
	existing, ok := f.byID[id]
	if !ok {
		return ErrNotFound
	}
	if owner, taken := f.byName[lower(input.Name)]; taken && owner != id {
		return ErrDuplicateName
	}
	delete(f.byName, lower(existing.Name))
	updated, err := platform.ParseTimestamp(updatedAt)
	if err != nil {
		return err
	}
	existing.Name = input.Name
	existing.Note = input.Note
	existing.Common = input.Common
	existing.Providers = input.Providers
	existing.UpdatedAt = updated
	f.byID[id] = existing
	f.byName[lower(input.Name)] = id
	return nil
}

func (f *fakeRepository) Delete(ctx context.Context, id string) error {
	existing, ok := f.byID[id]
	if !ok {
		return ErrNotFound
	}
	delete(f.byName, lower(existing.Name))
	delete(f.byID, id)
	return nil
}

// fakePlatformStore is the in-memory PlatformMetadataStore used by
// every test in this package - CRUD tests never call Apply/
// ApplyPreview, so an empty one is enough for newTestService; apply_test.go
// seeds it with real platforms per test.
type fakePlatformStore struct {
	platforms map[string]platform.Platform
	saveErr   error
	lastSaved map[string]platform.Metadata
}

func newFakePlatformStore() *fakePlatformStore {
	return &fakePlatformStore{platforms: map[string]platform.Platform{}}
}

func (f *fakePlatformStore) GetMany(ctx context.Context, ids []string) (map[string]platform.Platform, error) {
	out := make(map[string]platform.Platform, len(ids))
	for _, id := range ids {
		p, ok := f.platforms[id]
		if !ok {
			return nil, fmt.Errorf("%w: platform %s", platform.ErrNotFound, id)
		}
		out[id] = p
	}
	return out, nil
}

func (f *fakePlatformStore) SaveMetadataBatch(
	ctx context.Context, updates map[string]platform.Metadata,
) (map[string]platform.Metadata, error) {
	if f.saveErr != nil {
		return nil, f.saveErr
	}
	f.lastSaved = updates
	out := make(map[string]platform.Metadata, len(updates))
	for id, m := range updates {
		p := f.platforms[id]
		p.Metadata = m
		f.platforms[id] = p
		out[id] = m
	}
	return out, nil
}

func newTestService() (*Service, *fakeRepository) {
	repo := newFakeRepository()
	counter := 0
	svc := NewService(repo, newFakePlatformStore(),
		WithIDGenerator(func() (string, error) {
			counter++
			return "mp_test_" + string(rune('a'+counter)), nil
		}),
		WithClock(func() time.Time { return time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC) }),
	)
	return svc, repo
}

func TestCreateThenGet(t *testing.T) {
	svc, _ := newTestService()
	created, err := svc.Create(context.Background(), CreateInput{
		Name: "Just Chatting", Common: CommonMetadata{Title: "Hanging out", Tags: []string{"chat"}},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.Name != "Just Chatting" {
		t.Errorf("Name = %q, want %q", created.Name, "Just Chatting")
	}

	got, err := svc.Get(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Common.Title != "Hanging out" {
		t.Errorf("Common.Title = %q, want %q", got.Common.Title, "Hanging out")
	}
}

func TestCreateRejectsBlankName(t *testing.T) {
	svc, _ := newTestService()
	_, err := svc.Create(context.Background(), CreateInput{Name: "   "})
	if _, ok := platform.AsValidationError(err); !ok {
		t.Fatalf("Create() error = %v, want a ValidationError", err)
	}
}

func TestCreateRejectsDuplicateNameCaseInsensitive(t *testing.T) {
	svc, _ := newTestService()
	if _, err := svc.Create(context.Background(), CreateInput{Name: "My Setup"}); err != nil {
		t.Fatalf("first Create() error = %v", err)
	}
	_, err := svc.Create(context.Background(), CreateInput{Name: "my setup"})
	if !errors.Is(err, ErrDuplicateName) {
		t.Fatalf("second Create() error = %v, want ErrDuplicateName", err)
	}
}

func TestCreateRejectsOversizeName(t *testing.T) {
	svc, _ := newTestService()
	long := make([]byte, NameMaxLength+1)
	for i := range long {
		long[i] = 'a'
	}
	_, err := svc.Create(context.Background(), CreateInput{Name: string(long)})
	if _, ok := platform.AsValidationError(err); !ok {
		t.Fatalf("Create() error = %v, want a ValidationError", err)
	}
}

func TestUpdateRenamesAndReplacesContent(t *testing.T) {
	svc, _ := newTestService()
	created, err := svc.Create(context.Background(), CreateInput{Name: "Draft", Common: CommonMetadata{Title: "old"}})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	updated, err := svc.Update(context.Background(), created.ID, UpdateInput{
		Name: "Final", Common: CommonMetadata{Title: "new"},
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Name != "Final" || updated.Common.Title != "new" {
		t.Fatalf("Update() = %+v, want Name=Final Title=new", updated)
	}
}

func TestUpdateUnknownIDReturnsNotFound(t *testing.T) {
	svc, _ := newTestService()
	_, err := svc.Update(context.Background(), "mp_does_not_exist", UpdateInput{Name: "x"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Update() error = %v, want ErrNotFound", err)
	}
}

func TestDeleteRemovesPreset(t *testing.T) {
	svc, _ := newTestService()
	created, err := svc.Create(context.Background(), CreateInput{Name: "Temp"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := svc.Delete(context.Background(), created.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := svc.Get(context.Background(), created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() after delete error = %v, want ErrNotFound", err)
	}
}

func TestDeleteUnknownIDReturnsNotFound(t *testing.T) {
	svc, _ := newTestService()
	if err := svc.Delete(context.Background(), "mp_does_not_exist"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete() error = %v, want ErrNotFound", err)
	}
}

func TestCreateEnforcesMaxPresets(t *testing.T) {
	svc, repo := newTestService()
	// Pre-fill the fake repository directly, bypassing Create's own ID
	// churn, so this test stays fast and only exercises the bound.
	for i := 0; i < MaxPresets; i++ {
		id := "mp_prefill_" + string(rune('a'+i%26)) + string(rune('a'+i/26))
		repo.byID[id] = Preset{ID: id, Name: id}
		repo.byName[id] = id
	}

	_, err := svc.Create(context.Background(), CreateInput{Name: "one too many"})
	if !errors.Is(err, ErrTooMany) {
		t.Fatalf("Create() error = %v, want ErrTooMany", err)
	}
}

func TestListReturnsEveryPreset(t *testing.T) {
	svc, _ := newTestService()
	if _, err := svc.Create(context.Background(), CreateInput{Name: "A"}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := svc.Create(context.Background(), CreateInput{Name: "B"}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	list, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("List() returned %d presets, want 2", len(list))
	}
}
