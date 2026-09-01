package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/onboarding"
)

// The migration itself always seeds a row (docs/onboarding.md §4.3), so a
// freshly migrated database is found = true, unlike update_preferences'
// own "absent until Set" convention.
func TestOnboardingGetReturnsSeededPendingRowOnFreshDatabase(t *testing.T) {
	db := newTestDB(t)
	repo := NewOnboardingRepository(db.DB)

	got, found, err := repo.GetState(context.Background())
	if err != nil {
		t.Fatalf("GetState() error = %v", err)
	}
	if !found {
		t.Fatal("GetState() found = false, want true - the migration always seeds a row")
	}
	if got.Status != onboarding.StatusPending {
		t.Fatalf("GetState() Status = %v, want %v on an untouched fresh database", got.Status, onboarding.StatusPending)
	}
	if got.SchemaVersion != onboarding.CurrentSchemaVersion {
		t.Fatalf("GetState() SchemaVersion = %v, want %v", got.SchemaVersion, onboarding.CurrentSchemaVersion)
	}
}

func TestOnboardingSetThenGetRoundTrips(t *testing.T) {
	db := newTestDB(t)
	repo := NewOnboardingRepository(db.DB)
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	saved, err := repo.SetStatus(context.Background(), onboarding.StatusCompleted, onboarding.CurrentSchemaVersion, now)
	if err != nil {
		t.Fatalf("SetStatus() error = %v", err)
	}
	if saved.Status != onboarding.StatusCompleted {
		t.Errorf("SetStatus() Status = %v, want %v", saved.Status, onboarding.StatusCompleted)
	}

	got, found, err := repo.GetState(context.Background())
	if err != nil {
		t.Fatalf("GetState() error = %v", err)
	}
	if !found || got.Status != onboarding.StatusCompleted {
		t.Fatalf("GetState() = (%+v, %v), want the written status", got, found)
	}
}

func TestOnboardingSetReplacesTheSingletonRowInPlace(t *testing.T) {
	db := newTestDB(t)
	repo := NewOnboardingRepository(db.DB)
	ctx := context.Background()

	if _, err := repo.SetStatus(ctx, onboarding.StatusCompleted, onboarding.CurrentSchemaVersion, time.Now().UTC()); err != nil {
		t.Fatalf("first SetStatus() error = %v", err)
	}
	if _, err := repo.SetStatus(ctx, onboarding.StatusDismissed, onboarding.CurrentSchemaVersion, time.Now().UTC()); err != nil {
		t.Fatalf("second SetStatus() error = %v", err)
	}

	var count int
	if err := db.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM onboarding_state`).Scan(&count); err != nil {
		t.Fatalf("count query error = %v", err)
	}
	if count != 1 {
		t.Fatalf("row count = %d, want exactly 1 (singleton)", count)
	}

	got, found, err := repo.GetState(ctx)
	if err != nil {
		t.Fatalf("GetState() error = %v", err)
	}
	if !found || got.Status != onboarding.StatusDismissed {
		t.Fatalf("GetState() = (%+v, %v), want the second write's status", got, found)
	}
}

// onboardingExistingUserCase re-derives the migration's own existing-user
// CASE WHEN EXISTS(...) expression (0029_onboarding_state.sql), executed
// directly against the given database state - the migration itself only
// ever runs once per real database (tracked in schema_migrations), so this
// is the way to exercise its decision logic for more than the one state a
// freshly migrated newTestDB(t) happens to start in.
func onboardingExistingUserCase(t *testing.T, db *DB) onboarding.Status {
	t.Helper()
	var status string
	err := db.DB.QueryRowContext(context.Background(), `
        SELECT CASE WHEN EXISTS (
            SELECT 1 FROM connected_accounts
            UNION ALL
            SELECT 1 FROM platform_output_settings WHERE server_url <> ''
            UNION ALL
            SELECT 1 FROM platforms WHERE enabled = 1
            UNION ALL
            SELECT 1 FROM platforms WHERE id NOT IN
                ('pf_seed_twitch', 'pf_seed_youtube', 'pf_seed_kick', 'pf_seed_tiktok')
        ) THEN 'dismissed' ELSE 'pending' END`).Scan(&status)
	if err != nil {
		t.Fatalf("existing-user case query error = %v", err)
	}
	return onboarding.Status(status)
}

func TestOnboardingMigrationRuleUntouchedSeedIsPending(t *testing.T) {
	db := newTestDB(t)
	if got := onboardingExistingUserCase(t, db); got != onboarding.StatusPending {
		t.Fatalf("existing-user rule on an untouched seeded database = %v, want %v", got, onboarding.StatusPending)
	}
}

func TestOnboardingMigrationRuleConnectedAccountIsDismissed(t *testing.T) {
	db := newTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.DB.ExecContext(context.Background(), `
        INSERT INTO connected_accounts (id, provider_id, provider_user_id, login, display_name, status, created_at, updated_at)
        VALUES ('acc_test', 'twitch', 'pu_1', 'tester', 'Tester', 'connected', ?, ?)`, now, now); err != nil {
		t.Fatalf("insert connected_accounts error = %v", err)
	}
	if got := onboardingExistingUserCase(t, db); got != onboarding.StatusDismissed {
		t.Fatalf("existing-user rule with a connected account = %v, want %v", got, onboarding.StatusDismissed)
	}
}

// A real finding during implementation: 0003_platform_output_settings.sql
// gives every platform - seeded or not - a default settings row with an
// empty server_url the moment the platform exists, specifically so a
// row's mere existence never implies configuration (that migration's own
// comment). This confirms the rule correctly ignores that always-present
// empty row and only reacts to a real, non-empty server_url.
func TestOnboardingMigrationRuleConfiguredOutputServerIsDismissed(t *testing.T) {
	db := newTestDB(t)
	if _, err := db.DB.ExecContext(context.Background(),
		`UPDATE platform_output_settings SET server_url = 'rtmp://live.twitch.tv/app' WHERE platform_id = 'pf_seed_twitch'`); err != nil {
		t.Fatalf("configure output server error = %v", err)
	}
	if got := onboardingExistingUserCase(t, db); got != onboarding.StatusDismissed {
		t.Fatalf("existing-user rule with a configured output server = %v, want %v", got, onboarding.StatusDismissed)
	}
}

func TestOnboardingMigrationRuleEnabledSeedPlatformIsDismissed(t *testing.T) {
	db := newTestDB(t)
	if _, err := db.DB.ExecContext(context.Background(),
		`UPDATE platforms SET enabled = 1 WHERE id = 'pf_seed_twitch'`); err != nil {
		t.Fatalf("enable seed platform error = %v", err)
	}
	if got := onboardingExistingUserCase(t, db); got != onboarding.StatusDismissed {
		t.Fatalf("existing-user rule with an enabled seed platform = %v, want %v", got, onboarding.StatusDismissed)
	}
}

func TestOnboardingMigrationRuleUserCreatedPlatformIsDismissed(t *testing.T) {
	db := newTestDB(t)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.DB.ExecContext(context.Background(), `
        INSERT INTO platforms (id, provider_id, display_name, enabled, sort_order, created_at, updated_at)
        VALUES ('pf_real_1', 'twitch', 'My Channel', 0, 4, ?, ?)`, now, now); err != nil {
		t.Fatalf("insert user platform error = %v", err)
	}
	if got := onboardingExistingUserCase(t, db); got != onboarding.StatusDismissed {
		t.Fatalf("existing-user rule with a user-created platform = %v, want %v", got, onboarding.StatusDismissed)
	}
}
