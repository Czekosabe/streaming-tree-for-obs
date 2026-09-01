package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/metadatapreset"
	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/domain/streamsetup"
)

func TestStreamSetupProfileListIsEmptyOnFreshDatabase(t *testing.T) {
	db := newTestDB(t)
	repo := NewStreamSetupProfileRepository(db.DB)

	list, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("List() = %d profiles, want 0 - a fresh database must start with zero real profiles, no seed data", len(list))
	}
}

func seedStreamSetupPlatform(t *testing.T, repo *PlatformRepository, id, name string) {
	t.Helper()
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	if err := repo.Create(context.Background(), platform.Platform{
		ID: id, ProviderID: platform.ProviderTwitch, DisplayName: name,
		CreatedAt: now, UpdatedAt: now, Metadata: platform.Metadata{Tags: []string{}, UpdatedAt: now},
	}); err != nil {
		t.Fatalf("seed platform %q: %v", id, err)
	}
}

func TestStreamSetupProfileCreateThenGetRoundTrips(t *testing.T) {
	db := newTestDB(t)
	repo := NewStreamSetupProfileRepository(db.DB)
	platformRepo := NewPlatformRepository(db.DB)
	ctx := context.Background()
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	seedStreamSetupPlatform(t, platformRepo, "pf_1", "Main Twitch")
	pid := "pf_1"

	p := streamsetup.Profile{
		ID: "setup_1", Name: "Gaming", Note: "for gaming streams",
		Destinations: []streamsetup.Destination{{PlatformID: &pid, ProviderID: "twitch", DisplayName: "Main Twitch"}},
		CreatedAt:    now, UpdatedAt: now,
	}
	if err := repo.Create(ctx, p); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := repo.Get(ctx, "setup_1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Name != "Gaming" || got.Note != "for gaming streams" {
		t.Errorf("Name/Note = %q/%q, want Gaming/for gaming streams", got.Name, got.Note)
	}
	if len(got.Destinations) != 1 || *got.Destinations[0].PlatformID != "pf_1" || got.Destinations[0].DisplayName != "Main Twitch" {
		t.Fatalf("Destinations = %+v, want exactly 1 matching pf_1/Main Twitch", got.Destinations)
	}
	if got.MetadataPresetID != nil {
		t.Errorf("MetadataPresetID = %v, want nil", *got.MetadataPresetID)
	}
}

func TestStreamSetupProfileCreateRejectsDuplicateName(t *testing.T) {
	db := newTestDB(t)
	repo := NewStreamSetupProfileRepository(db.DB)
	ctx := context.Background()
	now := time.Now()

	if err := repo.Create(ctx, streamsetup.Profile{ID: "setup_1", Name: "Gaming", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	err := repo.Create(ctx, streamsetup.Profile{ID: "setup_2", Name: "gaming", CreatedAt: now, UpdatedAt: now})
	if err != streamsetup.ErrDuplicateName {
		t.Fatalf("Create() (case-insensitive duplicate) error = %v, want ErrDuplicateName", err)
	}
}

func TestStreamSetupProfileUpdateReplacesDestinationsInFull(t *testing.T) {
	db := newTestDB(t)
	repo := NewStreamSetupProfileRepository(db.DB)
	platformRepo := NewPlatformRepository(db.DB)
	ctx := context.Background()
	now := time.Now()

	seedStreamSetupPlatform(t, platformRepo, "pf_1", "A")
	seedStreamSetupPlatform(t, platformRepo, "pf_2", "B")
	pid1, pid2 := "pf_1", "pf_2"

	if err := repo.Create(ctx, streamsetup.Profile{
		ID: "setup_1", Name: "Gaming",
		Destinations: []streamsetup.Destination{{PlatformID: &pid1, ProviderID: "twitch", DisplayName: "A"}},
		CreatedAt:    now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := repo.Update(ctx, streamsetup.Profile{
		ID: "setup_1", Name: "Gaming Updated", Note: "new note",
		Destinations: []streamsetup.Destination{{PlatformID: &pid2, ProviderID: "twitch", DisplayName: "B"}},
		UpdatedAt:    now,
	}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	got, err := repo.Get(ctx, "setup_1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Name != "Gaming Updated" || got.Note != "new note" {
		t.Errorf("Name/Note = %q/%q after update", got.Name, got.Note)
	}
	if len(got.Destinations) != 1 || *got.Destinations[0].PlatformID != "pf_2" {
		t.Fatalf("Destinations = %+v, want exactly pf_2 (the old pf_1 row must be fully replaced)", got.Destinations)
	}
}

func TestStreamSetupProfileUpdateUnknownIDReturnsNotFound(t *testing.T) {
	db := newTestDB(t)
	repo := NewStreamSetupProfileRepository(db.DB)

	err := repo.Update(context.Background(), streamsetup.Profile{ID: "setup_missing", Name: "X", UpdatedAt: time.Now()})
	if err != streamsetup.ErrNotFound {
		t.Fatalf("Update() error = %v, want ErrNotFound", err)
	}
}

func TestStreamSetupProfileDestinationSurvivesPlatformDeletionAsMissing(t *testing.T) {
	db := newTestDB(t)
	repo := NewStreamSetupProfileRepository(db.DB)
	platformRepo := NewPlatformRepository(db.DB)
	ctx := context.Background()
	now := time.Now()

	seedStreamSetupPlatform(t, platformRepo, "pf_1", "Doomed")
	pid := "pf_1"
	if err := repo.Create(ctx, streamsetup.Profile{
		ID: "setup_1", Name: "Gaming",
		Destinations: []streamsetup.Destination{{PlatformID: &pid, ProviderID: "twitch", DisplayName: "Doomed"}},
		CreatedAt:    now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := platformRepo.Delete(ctx, "pf_1"); err != nil {
		t.Fatalf("delete platform: %v", err)
	}

	got, err := repo.Get(ctx, "setup_1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if len(got.Destinations) != 1 {
		t.Fatalf("Destinations = %+v, want the row to survive", got.Destinations)
	}
	if got.Destinations[0].PlatformID != nil {
		t.Errorf("PlatformID = %v after platform deletion, want nil (SET NULL, never CASCADE)", *got.Destinations[0].PlatformID)
	}
	if got.Destinations[0].DisplayName != "Doomed" {
		t.Errorf("DisplayName = %q, want the snapshot to survive unchanged", got.Destinations[0].DisplayName)
	}
}

func TestStreamSetupProfileSurvivesMetadataPresetDeletionAsMissing(t *testing.T) {
	db := newTestDB(t)
	repo := NewStreamSetupProfileRepository(db.DB)
	presetRepo := NewMetadataPresetRepository(db.DB)
	ctx := context.Background()
	now := time.Now()

	if err := presetRepo.Create(ctx, metadatapreset.Preset{
		ID: "mp_1", Name: "Gaming preset", CreatedAt: now, UpdatedAt: now,
		Providers: map[platform.ProviderID]metadatapreset.ProviderMetadata{},
	}); err != nil {
		t.Fatalf("seed metadata preset: %v", err)
	}
	presetID := "mp_1"
	if err := repo.Create(ctx, streamsetup.Profile{
		ID: "setup_1", Name: "Gaming", MetadataPresetID: &presetID, MetadataPresetName: "Gaming preset",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := presetRepo.Delete(ctx, "mp_1"); err != nil {
		t.Fatalf("delete metadata preset: %v", err)
	}

	got, err := repo.Get(ctx, "setup_1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.MetadataPresetID != nil {
		t.Errorf("MetadataPresetID = %v after preset deletion, want nil", *got.MetadataPresetID)
	}
	if got.MetadataPresetName != "Gaming preset" {
		t.Errorf("MetadataPresetName = %q, want the snapshot %q to survive deletion", got.MetadataPresetName, "Gaming preset")
	}
	if !got.MetadataPresetMissing() {
		t.Error("MetadataPresetMissing() = false, want true")
	}
}

func TestStreamSetupProfileDeleteRemovesDestinationsToo(t *testing.T) {
	db := newTestDB(t)
	repo := NewStreamSetupProfileRepository(db.DB)
	platformRepo := NewPlatformRepository(db.DB)
	ctx := context.Background()
	now := time.Now()

	seedStreamSetupPlatform(t, platformRepo, "pf_1", "A")
	pid := "pf_1"
	if err := repo.Create(ctx, streamsetup.Profile{
		ID: "setup_1", Name: "Gaming",
		Destinations: []streamsetup.Destination{{PlatformID: &pid, ProviderID: "twitch", DisplayName: "A"}},
		CreatedAt:    now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := repo.Delete(ctx, "setup_1"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := repo.Get(ctx, "setup_1"); err != streamsetup.ErrNotFound {
		t.Fatalf("Get() after delete error = %v, want ErrNotFound", err)
	}

	var count int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM stream_setup_profile_destinations WHERE profile_id = ?`, "setup_1").Scan(&count); err != nil {
		t.Fatalf("count destinations: %v", err)
	}
	if count != 0 {
		t.Errorf("stream_setup_profile_destinations still has %d rows after profile deletion, want 0 (CASCADE)", count)
	}
}

func TestStreamSetupProfileDeleteUnknownIDReturnsNotFound(t *testing.T) {
	db := newTestDB(t)
	repo := NewStreamSetupProfileRepository(db.DB)

	if err := repo.Delete(context.Background(), "setup_missing"); err != streamsetup.ErrNotFound {
		t.Fatalf("Delete() error = %v, want ErrNotFound", err)
	}
}
