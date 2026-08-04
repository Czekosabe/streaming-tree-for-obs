package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/output"
	"github.com/streaming-tree/server/internal/domain/platform"
)

func TestMigrationSeedsOutputSettingsForExistingPlatforms(t *testing.T) {
	db := newTestDB(t)

	var count int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM platform_output_settings`).Scan(&count); err != nil {
		t.Fatalf("counting output settings rows failed: %v", err)
	}
	if count != 4 {
		t.Fatalf("output settings rows = %d, want 4 (one per seeded platform)", count)
	}
}

func TestSeededOutputSettingsAreUnconfiguredByDefault(t *testing.T) {
	db := newTestDB(t)
	repo := NewOutputRepository(db.DB)

	settings, err := repo.Get(context.Background(), "pf_seed_twitch")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if settings.ServerURL != "" {
		t.Errorf("ServerURL = %q, want empty - no seeded destination should be silently ready to stream", settings.ServerURL)
	}
	if !settings.AutoRestart {
		t.Error("AutoRestart = false, want the default true")
	}
}

func TestGetReturnsNotFoundForAnUnknownPlatform(t *testing.T) {
	db := newTestDB(t)
	repo := NewOutputRepository(db.DB)

	_, err := repo.Get(context.Background(), "pf_does_not_exist")
	if err != output.ErrNotFound {
		t.Errorf("Get() error = %v, want ErrNotFound", err)
	}
}

func TestUpdateReplacesServerURLAndAutoRestart(t *testing.T) {
	db := newTestDB(t)
	repo := NewOutputRepository(db.DB)
	ctx := context.Background()

	updated, err := repo.Update(ctx, "pf_seed_twitch", output.UpdateInput{
		ServerURL:   "rtmps://live.example.invalid/app",
		AutoRestart: false,
	}, platform.FormatTimestamp(time.Now()))
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if updated.ServerURL != "rtmps://live.example.invalid/app" {
		t.Errorf("ServerURL = %q, want the new value", updated.ServerURL)
	}
	if updated.AutoRestart {
		t.Error("AutoRestart = true, want false")
	}

	reread, err := repo.Get(ctx, "pf_seed_twitch")
	if err != nil {
		t.Fatalf("Get() after update error = %v", err)
	}
	if reread.ServerURL != "rtmps://live.example.invalid/app" {
		t.Error("the update did not persist")
	}
}

func TestUpdateOnAnUnknownPlatformReturnsNotFound(t *testing.T) {
	db := newTestDB(t)
	repo := NewOutputRepository(db.DB)

	_, err := repo.Update(context.Background(), "pf_does_not_exist", output.UpdateInput{
		ServerURL: "rtmp://example.invalid/app",
	}, platform.FormatTimestamp(time.Now()))
	if err != output.ErrNotFound {
		t.Errorf("Update() error = %v, want ErrNotFound", err)
	}
}

func TestDeletingAPlatformCascadesToItsOutputSettings(t *testing.T) {
	db := newTestDB(t)
	platforms := NewPlatformRepository(db.DB)
	outputs := NewOutputRepository(db.DB)
	ctx := context.Background()

	if err := platforms.Delete(ctx, "pf_seed_twitch"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if _, err := outputs.Get(ctx, "pf_seed_twitch"); err != output.ErrNotFound {
		t.Errorf("Get() after platform delete error = %v, want ErrNotFound", err)
	}
}

func TestCreatingAPlatformSeedsOutputSettingsInTheSameTransaction(t *testing.T) {
	db := newTestDB(t)
	platforms := NewPlatformRepository(db.DB)
	outputs := NewOutputRepository(db.DB)
	ctx := context.Background()

	now := time.Now()
	newPlatform := platform.Platform{
		ID:          "pf_new_output_test",
		ProviderID:  platform.ProviderKick,
		DisplayName: "New destination",
		Enabled:     false,
		SortOrder:   99,
		CreatedAt:   now,
		UpdatedAt:   now,
		Metadata:    platform.Metadata{Tags: []string{}, UpdatedAt: now},
	}

	if err := platforms.Create(ctx, newPlatform); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	settings, err := outputs.Get(ctx, "pf_new_output_test")
	if err != nil {
		t.Fatalf("Get() for the newly created platform error = %v", err)
	}
	if settings.ServerURL != "" {
		t.Errorf("ServerURL = %q, want empty for a brand-new destination", settings.ServerURL)
	}
}
