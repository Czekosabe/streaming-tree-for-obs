package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/domain/remotetarget"
)

func TestRemoteTargetGetReturnsNotFoundWhenAbsent(t *testing.T) {
	db := newTestDB(t)
	repo := NewRemoteTargetRepository(db.DB)

	_, found, err := repo.Get(context.Background(), "pf_seed_youtube")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if found {
		t.Error("Get() found = true, want false before any target is set")
	}
}

func TestRemoteTargetSetThenGetRoundTrips(t *testing.T) {
	db := newTestDB(t)
	repo := NewRemoteTargetRepository(db.DB)
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)

	target := remotetarget.Target{
		PlatformID: "pf_seed_youtube", ProviderID: "youtube",
		ResourceType: remotetarget.ResourceTypeLiveBroadcast, ResourceID: "bcast1", DisplayName: "My Stream",
	}
	saved, err := repo.Set(context.Background(), target, now)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if saved.ResourceID != "bcast1" || saved.DisplayName != "My Stream" {
		t.Errorf("saved = %+v, want the submitted fields round-tripped", saved)
	}

	got, found, err := repo.Get(context.Background(), "pf_seed_youtube")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !found {
		t.Fatal("Get() found = false after Set()")
	}
	if got.ResourceID != "bcast1" || got.ProviderID != "youtube" || got.ResourceType != remotetarget.ResourceTypeLiveBroadcast {
		t.Errorf("got = %+v, want it to match what was set", got)
	}
}

func TestRemoteTargetSetReplacesExistingTargetInOneRow(t *testing.T) {
	db := newTestDB(t)
	repo := NewRemoteTargetRepository(db.DB)
	now := time.Now().UTC()

	first := remotetarget.Target{PlatformID: "pf_seed_youtube", ProviderID: "youtube", ResourceType: remotetarget.ResourceTypeLiveBroadcast, ResourceID: "bcast1", DisplayName: "First"}
	if _, err := repo.Set(context.Background(), first, now); err != nil {
		t.Fatalf("first Set() error = %v", err)
	}
	second := remotetarget.Target{PlatformID: "pf_seed_youtube", ProviderID: "youtube", ResourceType: remotetarget.ResourceTypeLiveBroadcast, ResourceID: "bcast2", DisplayName: "Second"}
	if _, err := repo.Set(context.Background(), second, now); err != nil {
		t.Fatalf("second Set() error = %v", err)
	}

	got, found, err := repo.Get(context.Background(), "pf_seed_youtube")
	if err != nil || !found {
		t.Fatalf("Get() error = %v, found = %v", err, found)
	}
	if got.ResourceID != "bcast2" {
		t.Errorf("ResourceID = %q, want the replaced value bcast2, not a second row", got.ResourceID)
	}

	var count int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM platform_remote_targets WHERE platform_id = ?`, "pf_seed_youtube").Scan(&count); err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	if count != 1 {
		t.Errorf("row count = %d, want exactly 1 (replace, not a second row)", count)
	}
}

func TestRemoteTargetSetRejectsAnUnknownPlatform(t *testing.T) {
	db := newTestDB(t)
	repo := NewRemoteTargetRepository(db.DB)

	target := remotetarget.Target{PlatformID: "pf_does_not_exist", ProviderID: "youtube", ResourceType: remotetarget.ResourceTypeLiveBroadcast, ResourceID: "bcast1", DisplayName: "X"}
	if _, err := repo.Set(context.Background(), target, time.Now().UTC()); err == nil {
		t.Fatal("Set() error = nil, want a foreign-key failure for an unknown platform")
	}
}

func TestRemoteTargetDeleteIsIdempotent(t *testing.T) {
	db := newTestDB(t)
	repo := NewRemoteTargetRepository(db.DB)

	if err := repo.Delete(context.Background(), "pf_seed_youtube"); err != nil {
		t.Fatalf("Delete() on an absent target error = %v, want nil", err)
	}

	target := remotetarget.Target{PlatformID: "pf_seed_youtube", ProviderID: "youtube", ResourceType: remotetarget.ResourceTypeLiveBroadcast, ResourceID: "bcast1", DisplayName: "X"}
	if _, err := repo.Set(context.Background(), target, time.Now().UTC()); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if err := repo.Delete(context.Background(), "pf_seed_youtube"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	_, found, err := repo.Get(context.Background(), "pf_seed_youtube")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if found {
		t.Error("target still found after Delete()")
	}
}

func TestRemoteTargetCascadesWhenPlatformIsDeleted(t *testing.T) {
	db := newTestDB(t)
	platforms := NewPlatformRepository(db.DB)
	targets := NewRemoteTargetRepository(db.DB)

	id, err := platform.NewID()
	if err != nil {
		t.Fatalf("NewID() error = %v", err)
	}
	now := time.Now().UTC()
	newPlatform := platform.Platform{
		ID: id, ProviderID: platform.ProviderYouTube, DisplayName: "Cascade Test",
		CreatedAt: now, UpdatedAt: now, Metadata: platform.Metadata{UpdatedAt: now},
	}
	if err := platforms.Create(context.Background(), newPlatform); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	target := remotetarget.Target{PlatformID: id, ProviderID: "youtube", ResourceType: remotetarget.ResourceTypeLiveBroadcast, ResourceID: "bcast1", DisplayName: "X"}
	if _, err := targets.Set(context.Background(), target, time.Now().UTC()); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	if err := platforms.Delete(context.Background(), id); err != nil {
		t.Fatalf("Delete() platform error = %v", err)
	}

	_, found, err := targets.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("Get() after cascade error = %v", err)
	}
	if found {
		t.Error("remote target survived its platform's deletion - it should cascade")
	}
}

func TestRemoteTargetPersistsAcrossRepositoryInstances(t *testing.T) {
	db := newTestDB(t)
	first := NewRemoteTargetRepository(db.DB)
	target := remotetarget.Target{PlatformID: "pf_seed_youtube", ProviderID: "youtube", ResourceType: remotetarget.ResourceTypeLiveBroadcast, ResourceID: "bcast1", DisplayName: "X"}
	if _, err := first.Set(context.Background(), target, time.Now().UTC()); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	second := NewRemoteTargetRepository(db.DB)
	got, found, err := second.Get(context.Background(), "pf_seed_youtube")
	if err != nil || !found {
		t.Fatalf("Get() from a new repository instance error = %v, found = %v", err, found)
	}
	if got.ResourceID != "bcast1" {
		t.Errorf("ResourceID = %q, want bcast1", got.ResourceID)
	}
}
