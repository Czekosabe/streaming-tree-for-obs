package sqlite

import (
	"context"
	"testing"
)

func tableExists(t *testing.T, db *DB, name string) bool {
	t.Helper()

	var found string
	err := db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&found)
	if err != nil {
		return false
	}
	return found == name
}

func TestMigrateCreatesSchemaOnEmptyDatabase(t *testing.T) {
	db := newEmptyTestDB(t)

	applied, err := Migrate(context.Background(), db.DB)
	if err != nil {
		t.Fatalf("Migrate() returned an error: %v", err)
	}
	if len(applied) == 0 {
		t.Fatal("Migrate() applied nothing on an empty database")
	}

	for _, table := range []string{
		"schema_migrations", "platforms", "platform_metadata", "platform_metadata_tags",
	} {
		if !tableExists(t, db, table) {
			t.Errorf("table %q was not created", table)
		}
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	db := newEmptyTestDB(t)
	ctx := context.Background()

	first, err := Migrate(ctx, db.DB)
	if err != nil {
		t.Fatalf("first Migrate() returned an error: %v", err)
	}

	second, err := Migrate(ctx, db.DB)
	if err != nil {
		t.Fatalf("second Migrate() returned an error: %v", err)
	}
	if len(second) != 0 {
		t.Errorf("second Migrate() applied %v, want nothing", second)
	}

	applied, err := AppliedMigrations(ctx, db.DB)
	if err != nil {
		t.Fatalf("AppliedMigrations() returned an error: %v", err)
	}
	if len(applied) != len(first) {
		t.Errorf("recorded %d migrations, want %d", len(applied), len(first))
	}
}

func TestMigrationsAreRecordedWithVersionNameAndTimestamp(t *testing.T) {
	db := newTestDB(t)

	applied, err := AppliedMigrations(context.Background(), db.DB)
	if err != nil {
		t.Fatalf("AppliedMigrations() returned an error: %v", err)
	}
	if len(applied) < 2 {
		t.Fatalf("recorded %d migrations, want at least 2", len(applied))
	}

	for i, record := range applied {
		if record.Version <= 0 {
			t.Errorf("migration %d has version %d, want a positive value", i, record.Version)
		}
		if record.Name == "" {
			t.Errorf("migration %d has an empty name", i)
		}
		if record.AppliedAt.IsZero() {
			t.Errorf("migration %d has no applied_at timestamp", i)
		}
		if i > 0 && applied[i-1].Version >= record.Version {
			t.Errorf("migrations are not ordered: %d then %d", applied[i-1].Version, record.Version)
		}
	}
}

func TestFailedMigrationIsNotRecordedAsApplied(t *testing.T) {
	db := newEmptyTestDB(t)
	ctx := context.Background()

	// A deliberately broken migration must roll back both the schema change and
	// its bookkeeping row, so the next start retries it.
	broken := Migration{Version: 9001, Name: "broken", SQL: `CREATE TABLE ok (id TEXT); THIS IS NOT SQL;`}

	if _, err := AppliedMigrations(ctx, db.DB); err != nil {
		t.Fatalf("AppliedMigrations() returned an error: %v", err)
	}

	if err := applyMigration(ctx, db.DB, broken); err == nil {
		t.Fatal("applyMigration() succeeded for invalid SQL, want an error")
	}

	applied, err := AppliedMigrations(ctx, db.DB)
	if err != nil {
		t.Fatalf("AppliedMigrations() returned an error: %v", err)
	}
	for _, record := range applied {
		if record.Version == broken.Version {
			t.Fatal("the failed migration was recorded as applied")
		}
	}

	if tableExists(t, db, "ok") {
		t.Error("the failed migration left a partially created table behind")
	}
}

func TestLoadMigrationsIsOrderedAndWellFormed(t *testing.T) {
	migrations, err := LoadMigrations()
	if err != nil {
		t.Fatalf("LoadMigrations() returned an error: %v", err)
	}
	if len(migrations) < 2 {
		t.Fatalf("loaded %d migrations, want at least 2", len(migrations))
	}

	for i, migration := range migrations {
		if migration.SQL == "" {
			t.Errorf("migration %d has empty SQL", migration.Version)
		}
		if i > 0 && migrations[i-1].Version >= migration.Version {
			t.Errorf("migrations are not sorted: %d before %d",
				migrations[i-1].Version, migration.Version)
		}
	}
}

func TestSeedInsertsFourDisabledPlatformsOnce(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM platforms`).Scan(&count); err != nil {
		t.Fatalf("counting platforms failed: %v", err)
	}
	if count != 4 {
		t.Fatalf("seeded %d platforms, want 4", count)
	}

	var enabledCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM platforms WHERE enabled = 1`).Scan(&enabledCount); err != nil {
		t.Fatalf("counting enabled platforms failed: %v", err)
	}
	if enabledCount != 0 {
		t.Errorf("%d seeded platforms are enabled, want all disabled", enabledCount)
	}

	// Running migrations again must not duplicate the seed.
	if _, err := Migrate(ctx, db.DB); err != nil {
		t.Fatalf("second Migrate() returned an error: %v", err)
	}

	if err := db.QueryRow(`SELECT COUNT(*) FROM platforms`).Scan(&count); err != nil {
		t.Fatalf("counting platforms failed: %v", err)
	}
	if count != 4 {
		t.Errorf("after re-running migrations there are %d platforms, want 4", count)
	}
}

func TestDeletedSeedDataIsNotRecreated(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	if _, err := db.Exec(`DELETE FROM platforms WHERE id = 'pf_seed_kick'`); err != nil {
		t.Fatalf("deleting the seeded platform failed: %v", err)
	}

	// Simulates an application restart: migrations run again on the same file.
	if _, err := Migrate(ctx, db.DB); err != nil {
		t.Fatalf("Migrate() returned an error: %v", err)
	}

	var count int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM platforms WHERE id = 'pf_seed_kick'`).Scan(&count); err != nil {
		t.Fatalf("counting the deleted platform failed: %v", err)
	}
	if count != 0 {
		t.Error("a deleted seeded platform came back after restart")
	}
}

func TestSeedGivesTwitchOrderedTags(t *testing.T) {
	db := newTestDB(t)

	rows, err := db.Query(
		`SELECT value FROM platform_metadata_tags WHERE platform_id = 'pf_seed_twitch' ORDER BY position`)
	if err != nil {
		t.Fatalf("reading seeded tags failed: %v", err)
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			t.Fatalf("scanning a tag failed: %v", err)
		}
		tags = append(tags, value)
	}

	want := []string{"programming", "go", "react", "obs"}
	if len(tags) != len(want) {
		t.Fatalf("seeded %d tags, want %d", len(tags), len(want))
	}
	for i := range want {
		if tags[i] != want[i] {
			t.Errorf("tag %d = %q, want %q - order must be preserved", i, tags[i], want[i])
		}
	}
}

func TestSeedStoresNullForUnsupportedFields(t *testing.T) {
	db := newTestDB(t)

	// TikTok supports neither language nor DVR, so both must be NULL rather
	// than an empty string or 0.
	var language, dvr any
	if err := db.QueryRow(
		`SELECT language, dvr FROM platform_metadata WHERE platform_id = 'pf_seed_tiktok'`,
	).Scan(&language, &dvr); err != nil {
		t.Fatalf("reading seeded metadata failed: %v", err)
	}

	if language != nil {
		t.Errorf("tiktok language = %v, want NULL", language)
	}
	if dvr != nil {
		t.Errorf("tiktok dvr = %v, want NULL", dvr)
	}
}
