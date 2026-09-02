package streamsetup

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/metadatapreset"
	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/runtime/branch"
)

// --- fakes -----------------------------------------------------------

type fakeRepo struct {
	profiles map[string]Profile
	byName   map[string]string // lowercased name -> id, for duplicate detection
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{profiles: map[string]Profile{}, byName: map[string]string{}}
}

func (f *fakeRepo) List(context.Context) ([]Profile, error) {
	out := make([]Profile, 0, len(f.profiles))
	for _, p := range f.profiles {
		out = append(out, p)
	}
	return out, nil
}

func (f *fakeRepo) Get(_ context.Context, id string) (Profile, error) {
	p, ok := f.profiles[id]
	if !ok {
		return Profile{}, ErrNotFound
	}
	return p, nil
}

func (f *fakeRepo) Create(_ context.Context, p Profile) error {
	if _, taken := f.byName[lower(p.Name)]; taken {
		return ErrDuplicateName
	}
	f.profiles[p.ID] = p
	f.byName[lower(p.Name)] = p.ID
	return nil
}

func (f *fakeRepo) Update(_ context.Context, p Profile) error {
	existing, ok := f.profiles[p.ID]
	if !ok {
		return ErrNotFound
	}
	if lower(existing.Name) != lower(p.Name) {
		if _, taken := f.byName[lower(p.Name)]; taken {
			return ErrDuplicateName
		}
		delete(f.byName, lower(existing.Name))
		f.byName[lower(p.Name)] = p.ID
	}
	f.profiles[p.ID] = p
	return nil
}

func (f *fakeRepo) Count(context.Context) (int, error) {
	return len(f.profiles), nil
}

func (f *fakeRepo) Delete(_ context.Context, id string) error {
	p, ok := f.profiles[id]
	if !ok {
		return ErrNotFound
	}
	delete(f.profiles, id)
	delete(f.byName, lower(p.Name))
	return nil
}

func lower(s string) string {
	out := []rune(s)
	for i, r := range out {
		if r >= 'A' && r <= 'Z' {
			out[i] = r + ('a' - 'A')
		}
	}
	return string(out)
}

type fakePlatforms struct {
	byID map[string]platform.Platform
}

func newFakePlatforms() *fakePlatforms { return &fakePlatforms{byID: map[string]platform.Platform{}} }

func (f *fakePlatforms) List(context.Context) ([]platform.Platform, error) {
	out := make([]platform.Platform, 0, len(f.byID))
	for _, p := range f.byID {
		out = append(out, p)
	}
	return out, nil
}

func (f *fakePlatforms) Get(_ context.Context, id string) (platform.Platform, error) {
	p, ok := f.byID[id]
	if !ok {
		return platform.Platform{}, platform.ErrNotFound
	}
	return p, nil
}

func (f *fakePlatforms) SetEnabledBatch(_ context.Context, updates map[string]bool) error {
	for id, enabled := range updates {
		p, ok := f.byID[id]
		if !ok {
			return platform.ErrNotFound
		}
		p.Enabled = enabled
		f.byID[id] = p
	}
	return nil
}

type fakePresets struct {
	byID               map[string]metadatapreset.Preset
	applyPreview       []metadatapreset.DestinationPreview
	applyErr           error
	applyCalled        int
	applyPreviewCalled int
}

func newFakePresets() *fakePresets {
	return &fakePresets{byID: map[string]metadatapreset.Preset{}}
}

func (f *fakePresets) Get(_ context.Context, id string) (metadatapreset.Preset, error) {
	p, ok := f.byID[id]
	if !ok {
		return metadatapreset.Preset{}, metadatapreset.ErrNotFound
	}
	return p, nil
}

func (f *fakePresets) ApplyPreview(_ context.Context, _ string, _ []string) ([]metadatapreset.DestinationPreview, error) {
	f.applyPreviewCalled++
	return f.applyPreview, nil
}

func (f *fakePresets) Apply(_ context.Context, _ string, _ []string) (map[string]platform.Metadata, error) {
	f.applyCalled++
	if f.applyErr != nil {
		return nil, f.applyErr
	}
	return map[string]platform.Metadata{}, nil
}

type fakeBranches struct{ snapshots []branch.Snapshot }

func (f *fakeBranches) Snapshot(context.Context) ([]branch.Snapshot, error) { return f.snapshots, nil }

func testService(repo Repository, platforms *fakePlatforms, presets *fakePresets, branches *fakeBranches) *Service {
	svc := NewService(repo, platforms, presets, branches)
	svc.newID = func() (string, error) {
		testServiceIDCounter++
		return "setup_test_" + strconv.Itoa(testServiceIDCounter), nil
	}
	return svc
}

var testServiceIDCounter int

func seedPlatform(fp *fakePlatforms, id, provider, name string, enabled bool) {
	fp.byID[id] = platform.Platform{ID: id, ProviderID: platform.ProviderID(provider), DisplayName: name, Enabled: enabled}
}

// --- tests -------------------------------------------------------------

func TestListIsEmptyOnFreshRepo(t *testing.T) {
	svc := testService(newFakeRepo(), newFakePlatforms(), newFakePresets(), &fakeBranches{})
	list, err := svc.List(context.Background())
	if err != nil || len(list) != 0 {
		t.Fatalf("List() = %+v, %v, want empty", list, err)
	}
}

func TestCreateResolvesDestinationSnapshotsAndRejectsUnknownDestination(t *testing.T) {
	fp := newFakePlatforms()
	seedPlatform(fp, "pf_1", "twitch", "My Twitch", false)
	svc := testService(newFakeRepo(), fp, newFakePresets(), &fakeBranches{})
	ctx := context.Background()

	p, err := svc.Create(ctx, CreateInput{Name: "Gaming", DestinationIDs: []string{"pf_1"}})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if len(p.Destinations) != 1 || p.Destinations[0].DisplayName != "My Twitch" || p.Destinations[0].ProviderID != "twitch" {
		t.Fatalf("Destinations = %+v, want a resolved snapshot of pf_1", p.Destinations)
	}

	_, err = svc.Create(ctx, CreateInput{Name: "Broken", DestinationIDs: []string{"pf_does_not_exist"}})
	if !errors.Is(err, platform.ErrNotFound) {
		t.Fatalf("Create() with an unknown destination error = %v, want ErrNotFound", err)
	}
}

func TestCreateRejectsAnUnknownMetadataPreset(t *testing.T) {
	svc := testService(newFakeRepo(), newFakePlatforms(), newFakePresets(), &fakeBranches{})
	missing := "mp_missing"
	_, err := svc.Create(context.Background(), CreateInput{Name: "Gaming", MetadataPresetID: &missing})
	if !errors.Is(err, metadatapreset.ErrNotFound) {
		t.Fatalf("Create() with an unknown preset error = %v, want ErrNotFound", err)
	}
}

func TestCreateSnapshotsTheMetadataPresetName(t *testing.T) {
	fpr := newFakePresets()
	fpr.byID["mp_1"] = metadatapreset.Preset{ID: "mp_1", Name: "Gaming preset"}
	svc := testService(newFakeRepo(), newFakePlatforms(), fpr, &fakeBranches{})
	presetID := "mp_1"

	p, err := svc.Create(context.Background(), CreateInput{Name: "Gaming", MetadataPresetID: &presetID})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if p.MetadataPresetName != "Gaming preset" {
		t.Errorf("MetadataPresetName = %q, want %q", p.MetadataPresetName, "Gaming preset")
	}
}

func TestUpdateReplacesDestinationsAndPresetReference(t *testing.T) {
	fp := newFakePlatforms()
	seedPlatform(fp, "pf_1", "twitch", "A", false)
	seedPlatform(fp, "pf_2", "youtube", "B", false)
	repo := newFakeRepo()
	svc := testService(repo, fp, newFakePresets(), &fakeBranches{})
	ctx := context.Background()

	p, err := svc.Create(ctx, CreateInput{Name: "Gaming", DestinationIDs: []string{"pf_1"}})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	updated, err := svc.Update(ctx, p.ID, UpdateInput{Name: "Gaming 2", DestinationIDs: []string{"pf_2"}})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if len(updated.Destinations) != 1 || *updated.Destinations[0].PlatformID != "pf_2" {
		t.Fatalf("Destinations = %+v, want exactly pf_2", updated.Destinations)
	}
}

func TestDeleteRemovesTheProfile(t *testing.T) {
	repo := newFakeRepo()
	svc := testService(repo, newFakePlatforms(), newFakePresets(), &fakeBranches{})
	ctx := context.Background()

	p, err := svc.Create(ctx, CreateInput{Name: "Gaming"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := svc.Delete(ctx, p.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := svc.Get(ctx, p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() after delete error = %v, want ErrNotFound", err)
	}
}

func TestDuplicateCopiesEverythingIncludingAMissingPresetSnapshot(t *testing.T) {
	repo := newFakeRepo()
	svc := testService(repo, newFakePlatforms(), newFakePresets(), &fakeBranches{})
	ctx := context.Background()

	original := Profile{
		ID: "setup_orig", Name: "Gaming", Note: "n",
		MetadataPresetID: nil, MetadataPresetName: "Deleted preset", // already "missing" going in
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := repo.Create(ctx, original); err != nil {
		t.Fatalf("seed original: %v", err)
	}

	dup, err := svc.Duplicate(ctx, "setup_orig", "Gaming (copy)")
	if err != nil {
		t.Fatalf("Duplicate() error = %v", err)
	}
	if dup.MetadataPresetName != "Deleted preset" || !dup.MetadataPresetMissing() {
		t.Errorf("duplicate did not carry over the missing-preset snapshot: %+v", dup)
	}
	if dup.ID == original.ID {
		t.Error("duplicate kept the original's own id")
	}
}

func TestCreateRejectsAnEmptyName(t *testing.T) {
	svc := testService(newFakeRepo(), newFakePlatforms(), newFakePresets(), &fakeBranches{})
	_, err := svc.Create(context.Background(), CreateInput{Name: "   "})
	if _, ok := platform.AsValidationError(err); !ok {
		t.Fatalf("Create() with a blank name error = %v, want a ValidationError", err)
	}
}

func TestCreateRejectsAnOverlongNameOrNote(t *testing.T) {
	svc := testService(newFakeRepo(), newFakePlatforms(), newFakePresets(), &fakeBranches{})
	ctx := context.Background()

	longName := make([]byte, NameMaxLength+1)
	for i := range longName {
		longName[i] = 'a'
	}
	_, nameErr := svc.Create(ctx, CreateInput{Name: string(longName)})
	if _, ok := platform.AsValidationError(nameErr); !ok {
		t.Errorf("Create() with an overlong name error = %v, want a ValidationError", nameErr)
	}

	longNote := make([]byte, NoteMaxLength+1)
	for i := range longNote {
		longNote[i] = 'a'
	}
	_, noteErr := svc.Create(ctx, CreateInput{Name: "Gaming", Note: string(longNote)})
	if _, ok := platform.AsValidationError(noteErr); !ok {
		t.Fatalf("Create() with an overlong note error = %v, want a ValidationError", noteErr)
	}
}

func TestCreateTrimsTheName(t *testing.T) {
	svc := testService(newFakeRepo(), newFakePlatforms(), newFakePresets(), &fakeBranches{})
	p, err := svc.Create(context.Background(), CreateInput{Name: "  Gaming  "})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if p.Name != "Gaming" {
		t.Errorf("Name = %q, want trimmed %q", p.Name, "Gaming")
	}
}

func TestCreateRejectsWhenAtMaxProfiles(t *testing.T) {
	repo := newFakeRepo()
	svc := testService(repo, newFakePlatforms(), newFakePresets(), &fakeBranches{})
	ctx := context.Background()

	for i := 0; i < MaxProfiles; i++ {
		if _, err := svc.Create(ctx, CreateInput{Name: "Setup " + strconv.Itoa(i)}); err != nil {
			t.Fatalf("seed Create() #%d error = %v", i, err)
		}
	}

	_, err := svc.Create(ctx, CreateInput{Name: "One too many"})
	if !errors.Is(err, ErrTooMany) {
		t.Fatalf("Create() at MaxProfiles error = %v, want ErrTooMany", err)
	}
}

func TestDuplicateRejectsAnEmptyNewName(t *testing.T) {
	repo := newFakeRepo()
	svc := testService(repo, newFakePlatforms(), newFakePresets(), &fakeBranches{})
	ctx := context.Background()

	p, err := svc.Create(ctx, CreateInput{Name: "Gaming"})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	_, err = svc.Duplicate(ctx, p.ID, "   ")
	if _, ok := platform.AsValidationError(err); !ok {
		t.Fatalf("Duplicate() with a blank name error = %v, want a ValidationError", err)
	}
}

func TestSaveCurrentCapturesOnlyEnabledDestinations(t *testing.T) {
	fp := newFakePlatforms()
	seedPlatform(fp, "pf_1", "twitch", "Enabled One", true)
	seedPlatform(fp, "pf_2", "youtube", "Disabled One", false)
	svc := testService(newFakeRepo(), fp, newFakePresets(), &fakeBranches{})

	p, err := svc.SaveCurrent(context.Background(), "Current setup", "", nil)
	if err != nil {
		t.Fatalf("SaveCurrent() error = %v", err)
	}
	if len(p.Destinations) != 1 || p.Destinations[0].DisplayName != "Enabled One" {
		t.Fatalf("Destinations = %+v, want exactly the one currently-enabled destination", p.Destinations)
	}
}
