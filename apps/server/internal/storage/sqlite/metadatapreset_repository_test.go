package sqlite

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/metadatapreset"
	"github.com/streaming-tree/server/internal/domain/platform"
)

func TestMetadataPresetListIsEmptyOnFreshDatabase(t *testing.T) {
	db := newTestDB(t)
	repo := NewMetadataPresetRepository(db.DB)

	list, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("List() = %d presets, want 0 - a fresh database must start with zero real presets, no seed data", len(list))
	}

	count, err := repo.Count(context.Background())
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("Count() = %d, want 0", count)
	}
}

func newTestPreset(id, name string) metadatapreset.Preset {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	return metadatapreset.Preset{
		ID: id, Name: name, Note: "a note",
		Common: metadatapreset.CommonMetadata{
			Title: "Test title", Description: "Test description",
			Tags: []string{"one", "two"}, Language: "en", Visibility: "public",
			MatureContent: true, DVR: false, LatencyMode: "",
		},
		Providers: map[platform.ProviderID]metadatapreset.ProviderMetadata{
			platform.ProviderTwitch:  {Category: "Software and Game Development", CategoryID: "509658"},
			platform.ProviderYouTube: {Category: "Science & Technology", CategoryID: "28"},
		},
		CreatedAt: now, UpdatedAt: now,
	}
}

func TestMetadataPresetCreateThenGetRoundTrips(t *testing.T) {
	db := newTestDB(t)
	repo := NewMetadataPresetRepository(db.DB)
	ctx := context.Background()

	p := newTestPreset("mp_1", "My Preset")
	if err := repo.Create(ctx, p); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := repo.Get(ctx, "mp_1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Name != "My Preset" || got.Common.Title != "Test title" {
		t.Fatalf("Get() = %+v, want the created values", got)
	}
	if len(got.Common.Tags) != 2 || got.Common.Tags[0] != "one" || got.Common.Tags[1] != "two" {
		t.Fatalf("Common.Tags = %v, want [one two] in order", got.Common.Tags)
	}
	twitch := got.Providers[platform.ProviderTwitch]
	youtube := got.Providers[platform.ProviderYouTube]
	if twitch.CategoryID != "509658" {
		t.Errorf("Twitch CategoryID = %q, want 509658", twitch.CategoryID)
	}
	if youtube.CategoryID != "28" {
		t.Errorf("YouTube CategoryID = %q, want 28", youtube.CategoryID)
	}
	if twitch.CategoryID == youtube.CategoryID {
		t.Fatal("Twitch and YouTube category IDs must round-trip independently, never conflated")
	}
}

func TestMetadataPresetGetUnknownIDReturnsNotFound(t *testing.T) {
	db := newTestDB(t)
	repo := NewMetadataPresetRepository(db.DB)

	_, err := repo.Get(context.Background(), "mp_does_not_exist")
	if !errors.Is(err, metadatapreset.ErrNotFound) {
		t.Fatalf("Get() error = %v, want ErrNotFound", err)
	}
}

func TestMetadataPresetCreateRejectsDuplicateNameCaseInsensitive(t *testing.T) {
	db := newTestDB(t)
	repo := NewMetadataPresetRepository(db.DB)
	ctx := context.Background()

	if err := repo.Create(ctx, newTestPreset("mp_1", "Same Name")); err != nil {
		t.Fatalf("first Create() error = %v", err)
	}
	err := repo.Create(ctx, newTestPreset("mp_2", "same name"))
	if !errors.Is(err, metadatapreset.ErrDuplicateName) {
		t.Fatalf("second Create() error = %v, want ErrDuplicateName", err)
	}

	// The failed create must not have left a partial row behind.
	if _, getErr := repo.Get(ctx, "mp_2"); !errors.Is(getErr, metadatapreset.ErrNotFound) {
		t.Fatalf("Get(mp_2) error = %v, want ErrNotFound - a rejected duplicate create must not persist anything", getErr)
	}
}

func TestMetadataPresetUpdateReplacesTagsAndProviderOverrides(t *testing.T) {
	db := newTestDB(t)
	repo := NewMetadataPresetRepository(db.DB)
	ctx := context.Background()

	if err := repo.Create(ctx, newTestPreset("mp_1", "Original")); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	err := repo.Update(ctx, "mp_1", metadatapreset.UpdateInput{
		Name: "Renamed", Note: "updated note",
		Common: metadatapreset.CommonMetadata{Title: "new title", Tags: []string{"only-one"}},
		Providers: map[platform.ProviderID]metadatapreset.ProviderMetadata{
			platform.ProviderKick: {Category: "Just Chatting"},
		},
	}, platform.FormatTimestamp(time.Now()))
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	got, err := repo.Get(ctx, "mp_1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Name != "Renamed" || got.Common.Title != "new title" {
		t.Fatalf("Get() = %+v, want the updated values", got)
	}
	if len(got.Common.Tags) != 1 || got.Common.Tags[0] != "only-one" {
		t.Fatalf("Common.Tags = %v, want [only-one] - the old tags must be fully replaced", got.Common.Tags)
	}
	if _, stillHasTwitch := got.Providers[platform.ProviderTwitch]; stillHasTwitch {
		t.Fatal("the old Twitch provider override must be fully replaced, not merged with the new one")
	}
	if got.Providers[platform.ProviderKick].Category != "Just Chatting" {
		t.Fatalf("Kick category = %q, want %q", got.Providers[platform.ProviderKick].Category, "Just Chatting")
	}
}

func TestMetadataPresetUpdateUnknownIDReturnsNotFound(t *testing.T) {
	db := newTestDB(t)
	repo := NewMetadataPresetRepository(db.DB)

	err := repo.Update(context.Background(), "mp_does_not_exist", metadatapreset.UpdateInput{Name: "x"}, "2026-09-01T00:00:00Z")
	if !errors.Is(err, metadatapreset.ErrNotFound) {
		t.Fatalf("Update() error = %v, want ErrNotFound", err)
	}
}

func TestMetadataPresetDeleteRemovesTagsAndProviderOverridesToo(t *testing.T) {
	db := newTestDB(t)
	repo := NewMetadataPresetRepository(db.DB)
	ctx := context.Background()

	if err := repo.Create(ctx, newTestPreset("mp_1", "To delete")); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := repo.Delete(ctx, "mp_1"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if _, err := repo.Get(ctx, "mp_1"); !errors.Is(err, metadatapreset.ErrNotFound) {
		t.Fatalf("Get() after delete error = %v, want ErrNotFound", err)
	}

	var tagCount, overrideCount int
	if err := db.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM metadata_preset_tags WHERE preset_id = 'mp_1'`).Scan(&tagCount); err != nil {
		t.Fatalf("count tags error = %v", err)
	}
	if err := db.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM metadata_preset_provider_overrides WHERE preset_id = 'mp_1'`).Scan(&overrideCount); err != nil {
		t.Fatalf("count provider overrides error = %v", err)
	}
	if tagCount != 0 || overrideCount != 0 {
		t.Fatalf("tagCount=%d overrideCount=%d, want 0 and 0 - ON DELETE CASCADE must remove child rows", tagCount, overrideCount)
	}
}

func TestMetadataPresetDeleteUnknownIDReturnsNotFound(t *testing.T) {
	db := newTestDB(t)
	repo := NewMetadataPresetRepository(db.DB)

	err := repo.Delete(context.Background(), "mp_does_not_exist")
	if !errors.Is(err, metadatapreset.ErrNotFound) {
		t.Fatalf("Delete() error = %v, want ErrNotFound", err)
	}
}

// A real, evidence-relevant test: deleting a preset must never touch a
// real destination's own metadata (docs/metadata-presets.md §6).
func TestMetadataPresetDeleteNeverTouchesPlatformMetadata(t *testing.T) {
	db := newTestDB(t)
	presetRepo := NewMetadataPresetRepository(db.DB)
	platformRepo := NewPlatformRepository(db.DB)
	ctx := context.Background()

	// pf_seed_twitch already exists from the seed migration with its
	// own real metadata row - use it directly rather than creating a
	// new platform, to keep this test's own setup minimal.
	before, err := platformRepo.Get(ctx, "pf_seed_twitch")
	if err != nil {
		t.Fatalf("Get(pf_seed_twitch) error = %v", err)
	}

	if err := presetRepo.Create(ctx, newTestPreset("mp_1", "Unrelated preset")); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := presetRepo.Delete(ctx, "mp_1"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	after, err := platformRepo.Get(ctx, "pf_seed_twitch")
	if err != nil {
		t.Fatalf("Get(pf_seed_twitch) after delete error = %v", err)
	}
	if !reflect.DeepEqual(after.Metadata, before.Metadata) {
		t.Fatalf("platform metadata changed after an unrelated preset delete: before=%+v after=%+v", before.Metadata, after.Metadata)
	}
}

func TestMetadataPresetListOrdersMostRecentlyUpdatedFirst(t *testing.T) {
	db := newTestDB(t)
	repo := NewMetadataPresetRepository(db.DB)
	ctx := context.Background()

	older := newTestPreset("mp_older", "Older")
	older.UpdatedAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := newTestPreset("mp_newer", "Newer")
	newer.UpdatedAt = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	if err := repo.Create(ctx, older); err != nil {
		t.Fatalf("Create(older) error = %v", err)
	}
	if err := repo.Create(ctx, newer); err != nil {
		t.Fatalf("Create(newer) error = %v", err)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 2 || list[0].ID != "mp_newer" || list[1].ID != "mp_older" {
		t.Fatalf("List() order = %v, want [mp_newer mp_older]", []string{list[0].ID, list[1].ID})
	}
}
