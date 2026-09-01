package metadatapreset

import (
	"context"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/platform"
)

func newApplyTestService() (*Service, *fakeRepository, *fakePlatformStore) {
	repo := newFakeRepository()
	store := newFakePlatformStore()
	svc := NewService(repo, store,
		WithClock(func() time.Time { return time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC) }),
	)
	return svc, repo, store
}

func seedPreset(t *testing.T, repo *fakeRepository, p Preset) {
	t.Helper()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	}
	if p.UpdatedAt.IsZero() {
		p.UpdatedAt = p.CreatedAt
	}
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatalf("seedPreset: %v", err)
	}
}

func seedPlatform(store *fakePlatformStore, id string, provider platform.ProviderID, metadata platform.Metadata) {
	if metadata.Tags == nil {
		metadata.Tags = []string{}
	}
	store.platforms[id] = platform.Platform{
		ID: id, ProviderID: provider, DisplayName: id, Metadata: metadata,
	}
}

func TestApplyPreviewNeverBlendsCategoryAcrossProviders(t *testing.T) {
	svc, repo, store := newApplyTestService()

	// A real Twitch category ID and a real YouTube category ID, deliberately
	// distinct values from each other - the exact scenario
	// docs/metadata-presets.md §1 warns must never be conflated.
	seedPreset(t, repo, Preset{
		ID: "mp_1", Name: "Coding stream",
		Common: CommonMetadata{Title: "Live coding", Tags: []string{"go"}, Language: "en"},
		Providers: map[platform.ProviderID]ProviderMetadata{
			platform.ProviderTwitch:  {Category: "Software and Game Development", CategoryID: "1469308723"},
			platform.ProviderYouTube: {Category: "Science & Technology", CategoryID: "28"},
		},
	})
	seedPlatform(store, "pf_yt", platform.ProviderYouTube, platform.Metadata{})

	previews, err := svc.ApplyPreview(context.Background(), "mp_1", []string{"pf_yt"})
	if err != nil {
		t.Fatalf("ApplyPreview() error = %v", err)
	}
	if len(previews) != 1 {
		t.Fatalf("got %d previews, want 1", len(previews))
	}
	if !previews[0].Valid {
		t.Fatalf("preview invalid, errors = %+v", previews[0].Errors)
	}

	// Confirm indirectly via Apply: the written candidate's category must be
	// the YouTube one, never the Twitch one.
	updated, err := svc.Apply(context.Background(), "mp_1", []string{"pf_yt"})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	got := updated["pf_yt"]
	if got.Category != "Science & Technology" || got.CategoryID != "28" {
		t.Errorf("applied category = %q/%q, want the YouTube-scoped values only", got.Category, got.CategoryID)
	}
}

func TestApplyPreviewMarksUnsupportedFieldsNotSupported(t *testing.T) {
	svc, repo, store := newApplyTestService()

	// Twitch has no Description capability (docs/metadata-presets.md §1).
	seedPreset(t, repo, Preset{
		ID: "mp_1", Name: "With description",
		Common: CommonMetadata{Title: "Live coding", Description: "A longer description"},
	})
	seedPlatform(store, "pf_tw", platform.ProviderTwitch, platform.Metadata{})

	previews, err := svc.ApplyPreview(context.Background(), "mp_1", []string{"pf_tw"})
	if err != nil {
		t.Fatalf("ApplyPreview() error = %v", err)
	}

	var found bool
	for _, f := range previews[0].Fields {
		if f.Field == "description" {
			found = true
			if f.Status != FieldNotSupported {
				t.Errorf("description status = %q, want %q", f.Status, FieldNotSupported)
			}
		}
	}
	if !found {
		t.Fatal("no \"description\" entry in the preview fields")
	}
}

func TestApplyPreviewClassifiesWillChangeAndUnchanged(t *testing.T) {
	svc, repo, store := newApplyTestService()

	seedPreset(t, repo, Preset{
		ID: "mp_1", Name: "Ranked night",
		Common: CommonMetadata{Title: "Ranked climb", Tags: []string{"ranked"}, Language: "en"},
	})
	seedPlatform(store, "pf_tw", platform.ProviderTwitch, platform.Metadata{
		Title: "Old title", Tags: []string{"ranked"}, Language: "en",
	})

	previews, err := svc.ApplyPreview(context.Background(), "mp_1", []string{"pf_tw"})
	if err != nil {
		t.Fatalf("ApplyPreview() error = %v", err)
	}

	statuses := map[string]FieldStatus{}
	for _, f := range previews[0].Fields {
		statuses[f.Field] = f.Status
	}
	if statuses["title"] != FieldWillChange {
		t.Errorf("title status = %q, want %q", statuses["title"], FieldWillChange)
	}
	if statuses["tags"] != FieldUnchanged {
		t.Errorf("tags status = %q, want %q", statuses["tags"], FieldUnchanged)
	}
	if statuses["language"] != FieldUnchanged {
		t.Errorf("language status = %q, want %q", statuses["language"], FieldUnchanged)
	}
}

func TestApplyRejectsAllDestinationsWhenOneFailsValidation(t *testing.T) {
	svc, repo, store := newApplyTestService()

	// "This tag name is far too long for Twitch" is well within YouTube's
	// generous tag length, but exceeds Twitch's real 25-character limit.
	seedPreset(t, repo, Preset{
		ID: "mp_1", Name: "Mixed compatibility",
		Common: CommonMetadata{
			Title: "Fine everywhere",
			Tags:  []string{"this-tag-name-is-far-too-long-for-twitch"},
		},
	})
	seedPlatform(store, "pf_tw", platform.ProviderTwitch, platform.Metadata{Title: "Existing"})
	seedPlatform(store, "pf_yt", platform.ProviderYouTube, platform.Metadata{Title: "Existing"})

	_, err := svc.Apply(context.Background(), "mp_1", []string{"pf_tw", "pf_yt"})
	if err == nil {
		t.Fatal("Apply() succeeded, want a validation error for the Twitch destination")
	}
	applyErr, ok := AsApplyValidationError(err)
	if !ok {
		t.Fatalf("error = %v (%T), want *ApplyValidationError", err, err)
	}
	if _, failed := applyErr.Destinations["pf_tw"]; !failed {
		t.Errorf("Destinations = %+v, want pf_tw to have failed", applyErr.Destinations)
	}
	if _, failed := applyErr.Destinations["pf_yt"]; failed {
		t.Errorf("Destinations = %+v, want pf_yt to NOT have failed on its own", applyErr.Destinations)
	}

	// Nothing at all must have been written - not even the YouTube
	// destination that would have validated fine on its own.
	if store.lastSaved != nil {
		t.Errorf("SaveMetadataBatch was called (lastSaved = %+v), want no write at all", store.lastSaved)
	}
	if store.platforms["pf_yt"].Metadata.Title != "Existing" {
		t.Error("the YouTube destination's metadata changed despite the all-or-nothing rejection")
	}
}

func TestApplySucceedsWritesEveryDestinationInOneBatch(t *testing.T) {
	svc, repo, store := newApplyTestService()

	seedPreset(t, repo, Preset{
		ID: "mp_1", Name: "Compatible everywhere",
		Common: CommonMetadata{Title: "Shared title", Language: "en"},
	})
	seedPlatform(store, "pf_tw", platform.ProviderTwitch, platform.Metadata{Title: "Old"})
	seedPlatform(store, "pf_yt", platform.ProviderYouTube, platform.Metadata{Title: "Old"})

	updated, err := svc.Apply(context.Background(), "mp_1", []string{"pf_tw", "pf_yt"})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(updated) != 2 {
		t.Fatalf("got %d updated destinations, want 2", len(updated))
	}
	if updated["pf_tw"].Title != "Shared title" || updated["pf_yt"].Title != "Shared title" {
		t.Errorf("updated = %+v, want both titles set to the preset's value", updated)
	}
	if len(store.lastSaved) != 2 {
		t.Errorf("SaveMetadataBatch received %d entries, want 2 (one atomic batch)", len(store.lastSaved))
	}
}

func TestApplyPreviewRequiresAtLeastOneDestination(t *testing.T) {
	svc, repo, _ := newApplyTestService()
	seedPreset(t, repo, Preset{ID: "mp_1", Name: "Any preset"})

	_, err := svc.ApplyPreview(context.Background(), "mp_1", nil)
	if _, ok := platform.AsValidationError(err); !ok {
		t.Fatalf("error = %v (%T), want a *platform.ValidationError", err, err)
	}
}

func TestApplyPreviewUnknownPresetReturnsNotFound(t *testing.T) {
	svc, _, store := newApplyTestService()
	seedPlatform(store, "pf_tw", platform.ProviderTwitch, platform.Metadata{})

	_, err := svc.ApplyPreview(context.Background(), "mp_missing", []string{"pf_tw"})
	if err != ErrNotFound {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestApplyPreviewUnknownPlatformReturnsNotFound(t *testing.T) {
	svc, repo, _ := newApplyTestService()
	seedPreset(t, repo, Preset{ID: "mp_1", Name: "Any preset"})

	_, err := svc.ApplyPreview(context.Background(), "mp_1", []string{"pf_missing"})
	if err == nil {
		t.Fatal("ApplyPreview() succeeded for an unknown platform id")
	}
}

func TestApplyIndependenceFromPresetAfterwards(t *testing.T) {
	svc, repo, store := newApplyTestService()

	seedPreset(t, repo, Preset{
		ID: "mp_1", Name: "One-time apply",
		Common: CommonMetadata{Title: "Applied title"},
	})
	seedPlatform(store, "pf_tw", platform.ProviderTwitch, platform.Metadata{Title: "Old"})

	if _, err := svc.Apply(context.Background(), "mp_1", []string{"pf_tw"}); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if err := svc.Delete(context.Background(), "mp_1"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// The destination's own metadata must be entirely unaffected by the
	// preset it came from being deleted afterwards (docs/metadata-presets.md §6).
	if store.platforms["pf_tw"].Metadata.Title != "Applied title" {
		t.Error("deleting the preset changed the destination's already-applied metadata")
	}
}
