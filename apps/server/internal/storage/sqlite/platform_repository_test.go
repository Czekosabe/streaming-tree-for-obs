package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/platform"
)

func newTestRepo(t *testing.T) (*PlatformRepository, *DB) {
	t.Helper()
	db := newTestDB(t)
	return NewPlatformRepository(db.DB), db
}

func TestListReturnsSeededPlatformsInSortOrder(t *testing.T) {
	repo, _ := newTestRepo(t)

	platforms, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("List() returned an error: %v", err)
	}
	if len(platforms) != 4 {
		t.Fatalf("List() returned %d platforms, want 4", len(platforms))
	}

	wantOrder := []platform.ProviderID{
		platform.ProviderTwitch, platform.ProviderYouTube,
		platform.ProviderKick, platform.ProviderTikTok,
	}
	for i, want := range wantOrder {
		if platforms[i].ProviderID != want {
			t.Errorf("platform %d is %q, want %q", i, platforms[i].ProviderID, want)
		}
		if platforms[i].SortOrder != i {
			t.Errorf("platform %d has sort order %d, want %d", i, platforms[i].SortOrder, i)
		}
	}
}

func TestListLoadsOrderedTagsWithoutPerPlatformQueries(t *testing.T) {
	repo, _ := newTestRepo(t)

	platforms, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("List() returned an error: %v", err)
	}

	byID := make(map[string]platform.Platform, len(platforms))
	for _, p := range platforms {
		byID[p.ID] = p
	}

	twitch := byID["pf_seed_twitch"]
	want := []string{"programming", "go", "react", "obs"}
	if len(twitch.Metadata.Tags) != len(want) {
		t.Fatalf("twitch has %d tags, want %d", len(twitch.Metadata.Tags), len(want))
	}
	for i := range want {
		if twitch.Metadata.Tags[i] != want[i] {
			t.Errorf("tag %d = %q, want %q", i, twitch.Metadata.Tags[i], want[i])
		}
	}

	// A provider without tag support must come back with an empty, non-nil slice.
	kick := byID["pf_seed_kick"]
	if kick.Metadata.Tags == nil {
		t.Error("kick tags are nil, want an empty slice")
	}
	if len(kick.Metadata.Tags) != 0 {
		t.Errorf("kick has %d tags, want 0", len(kick.Metadata.Tags))
	}
}

func TestGetReturnsNotFoundForUnknownID(t *testing.T) {
	repo, _ := newTestRepo(t)

	_, err := repo.Get(context.Background(), "pf_does_not_exist")
	if !errors.Is(err, platform.ErrNotFound) {
		t.Fatalf("Get() error = %v, want ErrNotFound", err)
	}
}

// newPlatform builds a valid platform value for repository-level tests.
func newPlatform(id string, provider platform.ProviderID, name string, sortOrder int) platform.Platform {
	now := time.Now().UTC()
	return platform.Platform{
		ID:          id,
		ProviderID:  provider,
		DisplayName: name,
		Enabled:     false,
		SortOrder:   sortOrder,
		CreatedAt:   now,
		UpdatedAt:   now,
		Metadata:    platform.Metadata{Tags: []string{}, UpdatedAt: now},
	}
}

func TestCreateAllowsASecondConfigurationForTheSameProvider(t *testing.T) {
	repo, _ := newTestRepo(t)
	ctx := context.Background()

	second := newPlatform("pf_second_twitch", platform.ProviderTwitch, "Second Twitch channel", 10)
	if err := repo.Create(ctx, second); err != nil {
		t.Fatalf("Create() returned an error: %v", err)
	}

	stored, err := repo.Get(ctx, second.ID)
	if err != nil {
		t.Fatalf("Get() returned an error: %v", err)
	}
	if stored.DisplayName != "Second Twitch channel" {
		t.Errorf("DisplayName = %q, want %q", stored.DisplayName, "Second Twitch channel")
	}

	platforms, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List() returned an error: %v", err)
	}
	twitchCount := 0
	for _, p := range platforms {
		if p.ProviderID == platform.ProviderTwitch {
			twitchCount++
		}
	}
	if twitchCount != 2 {
		t.Errorf("found %d twitch destinations, want 2", twitchCount)
	}
}

func TestCreateAlsoCreatesAMetadataRow(t *testing.T) {
	repo, db := newTestRepo(t)

	created := newPlatform("pf_new", platform.ProviderKick, "Kick backup", 7)
	if err := repo.Create(context.Background(), created); err != nil {
		t.Fatalf("Create() returned an error: %v", err)
	}

	var count int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM platform_metadata WHERE platform_id = ?`, created.ID).Scan(&count); err != nil {
		t.Fatalf("counting metadata rows failed: %v", err)
	}
	if count != 1 {
		t.Errorf("created %d metadata rows, want exactly 1", count)
	}
}

func TestCreateRejectsDuplicateID(t *testing.T) {
	repo, _ := newTestRepo(t)
	ctx := context.Background()

	duplicate := newPlatform("pf_seed_twitch", platform.ProviderTwitch, "Clashing id", 9)
	err := repo.Create(ctx, duplicate)
	if !errors.Is(err, platform.ErrConflict) {
		t.Fatalf("Create() error = %v, want ErrConflict", err)
	}
}

func TestUpdateReplacesMutableFields(t *testing.T) {
	repo, _ := newTestRepo(t)
	ctx := context.Background()

	updatedAt := platform.FormatTimestamp(time.Now().UTC())
	err := repo.Update(ctx, "pf_seed_kick", platform.UpdateInput{
		DisplayName: "Kick main",
		Enabled:     true,
		SortOrder:   42,
	}, updatedAt)
	if err != nil {
		t.Fatalf("Update() returned an error: %v", err)
	}

	stored, err := repo.Get(ctx, "pf_seed_kick")
	if err != nil {
		t.Fatalf("Get() returned an error: %v", err)
	}
	if stored.DisplayName != "Kick main" {
		t.Errorf("DisplayName = %q, want %q", stored.DisplayName, "Kick main")
	}
	if !stored.Enabled {
		t.Error("Enabled = false, want true")
	}
	if stored.SortOrder != 42 {
		t.Errorf("SortOrder = %d, want 42", stored.SortOrder)
	}
}

func TestUpdateReturnsNotFoundForUnknownID(t *testing.T) {
	repo, _ := newTestRepo(t)

	err := repo.Update(context.Background(), "pf_missing",
		platform.UpdateInput{DisplayName: "x", SortOrder: 1},
		platform.FormatTimestamp(time.Now()))
	if !errors.Is(err, platform.ErrNotFound) {
		t.Fatalf("Update() error = %v, want ErrNotFound", err)
	}
}

func TestDeleteCascadesMetadataAndTags(t *testing.T) {
	repo, db := newTestRepo(t)
	ctx := context.Background()

	if err := repo.Delete(ctx, "pf_seed_twitch"); err != nil {
		t.Fatalf("Delete() returned an error: %v", err)
	}

	var metadataCount int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM platform_metadata WHERE platform_id = 'pf_seed_twitch'`,
	).Scan(&metadataCount); err != nil {
		t.Fatalf("counting metadata failed: %v", err)
	}
	if metadataCount != 0 {
		t.Errorf("%d metadata rows survived the delete, want 0", metadataCount)
	}

	var tagCount int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM platform_metadata_tags WHERE platform_id = 'pf_seed_twitch'`,
	).Scan(&tagCount); err != nil {
		t.Fatalf("counting tags failed: %v", err)
	}
	if tagCount != 0 {
		t.Errorf("%d tag rows survived the delete, want 0", tagCount)
	}
}

func TestDeleteReturnsNotFoundForUnknownID(t *testing.T) {
	repo, _ := newTestRepo(t)

	if err := repo.Delete(context.Background(), "pf_missing"); !errors.Is(err, platform.ErrNotFound) {
		t.Fatalf("Delete() error = %v, want ErrNotFound", err)
	}
}

func TestSaveMetadataStoresOrderedTags(t *testing.T) {
	repo, _ := newTestRepo(t)
	ctx := context.Background()

	tags := []string{"zebra", "alpha", "middle"}
	err := repo.SaveMetadata(ctx, "pf_seed_twitch", platform.Metadata{
		Title:     "New title",
		Category:  "Just Chatting",
		Language:  "en",
		Tags:      tags,
		UpdatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("SaveMetadata() returned an error: %v", err)
	}

	stored, err := repo.Get(ctx, "pf_seed_twitch")
	if err != nil {
		t.Fatalf("Get() returned an error: %v", err)
	}

	if len(stored.Metadata.Tags) != len(tags) {
		t.Fatalf("stored %d tags, want %d", len(stored.Metadata.Tags), len(tags))
	}
	for i := range tags {
		// Insertion order must survive; it is not re-sorted alphabetically.
		if stored.Metadata.Tags[i] != tags[i] {
			t.Errorf("tag %d = %q, want %q", i, stored.Metadata.Tags[i], tags[i])
		}
	}
	if stored.Metadata.Title != "New title" {
		t.Errorf("Title = %q, want %q", stored.Metadata.Title, "New title")
	}
}

func TestSaveMetadataReplacesPreviousTags(t *testing.T) {
	repo, _ := newTestRepo(t)
	ctx := context.Background()

	err := repo.SaveMetadata(ctx, "pf_seed_twitch", platform.Metadata{
		Title:     "Only one tag now",
		Tags:      []string{"solo"},
		UpdatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("SaveMetadata() returned an error: %v", err)
	}

	stored, err := repo.Get(ctx, "pf_seed_twitch")
	if err != nil {
		t.Fatalf("Get() returned an error: %v", err)
	}
	if len(stored.Metadata.Tags) != 1 || stored.Metadata.Tags[0] != "solo" {
		t.Errorf("tags = %v, want [solo] - the previous list must be replaced", stored.Metadata.Tags)
	}
}

func TestSaveMetadataRollsBackWhenATagViolatesTheUniqueIndex(t *testing.T) {
	repo, db := newTestRepo(t)
	ctx := context.Background()

	// The repository trusts the domain layer for uniqueness, so this bypasses
	// it deliberately to prove the transaction rolls back the metadata write
	// too, rather than leaving a half-applied save behind.
	err := repo.SaveMetadata(ctx, "pf_seed_twitch", platform.Metadata{
		Title:     "Should not persist",
		Tags:      []string{"duplicate", "DUPLICATE"},
		UpdatedAt: time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("SaveMetadata() accepted case-insensitively duplicated tags")
	}
	if !errors.Is(err, platform.ErrConflict) {
		t.Errorf("SaveMetadata() error = %v, want ErrConflict", err)
	}

	var title string
	if err := db.QueryRow(
		`SELECT title FROM platform_metadata WHERE platform_id = 'pf_seed_twitch'`).Scan(&title); err != nil {
		t.Fatalf("reading metadata failed: %v", err)
	}
	if title == "Should not persist" {
		t.Error("the metadata write was committed even though the tag insert failed")
	}

	var tagCount int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM platform_metadata_tags WHERE platform_id = 'pf_seed_twitch'`,
	).Scan(&tagCount); err != nil {
		t.Fatalf("counting tags failed: %v", err)
	}
	if tagCount != 4 {
		t.Errorf("tag count = %d, want the original 4 after rollback", tagCount)
	}
}

func TestSaveMetadataReturnsNotFoundForUnknownPlatform(t *testing.T) {
	repo, _ := newTestRepo(t)

	err := repo.SaveMetadata(context.Background(), "pf_missing", platform.Metadata{
		Tags: []string{}, UpdatedAt: time.Now().UTC(),
	})
	if !errors.Is(err, platform.ErrNotFound) {
		t.Fatalf("SaveMetadata() error = %v, want ErrNotFound", err)
	}
}

func TestSaveMetadataPreservesUnicodeExactly(t *testing.T) {
	repo, _ := newTestRepo(t)
	ctx := context.Background()

	title := "Zażółć gęślą jaźń — 日本語 ✨"
	err := repo.SaveMetadata(ctx, "pf_seed_twitch", platform.Metadata{
		Title:     title,
		Tags:      []string{"zażółć", "日本語"},
		UpdatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("SaveMetadata() returned an error: %v", err)
	}

	stored, err := repo.Get(ctx, "pf_seed_twitch")
	if err != nil {
		t.Fatalf("Get() returned an error: %v", err)
	}
	if stored.Metadata.Title != title {
		t.Errorf("Title = %q, want %q - user content must be stored verbatim", stored.Metadata.Title, title)
	}
	if stored.Metadata.Tags[0] != "zażółć" || stored.Metadata.Tags[1] != "日本語" {
		t.Errorf("tags = %v, want the exact Unicode values", stored.Metadata.Tags)
	}
}

func TestNextSortOrderAppends(t *testing.T) {
	repo, _ := newTestRepo(t)

	next, err := repo.NextSortOrder(context.Background())
	if err != nil {
		t.Fatalf("NextSortOrder() returned an error: %v", err)
	}
	// The seed occupies 0..3.
	if next != 4 {
		t.Errorf("NextSortOrder() = %d, want 4", next)
	}
}
