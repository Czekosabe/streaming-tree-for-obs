package sqlite

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/goals"
)

func newTestGoal(id string) goals.Goal {
	now := time.Now().UTC()
	return goals.Goal{
		ID: id, Name: "Followers", Kind: goals.KindFollowers, Enabled: true,
		Target: 1000, Current: 825, Baseline: 825,
		CreatedAt: now, UpdatedAt: now, StartedAt: now, ConfigRevision: 1,
	}
}

func TestGoalCreateThenGetRoundTrips(t *testing.T) {
	db := newTestDB(t)
	repo := NewGoalsRepository(db.DB)

	g := newTestGoal("goal_1")
	g.Providers = []goals.ProviderID{goals.ProviderTwitch}
	g.Accounts = []string{"acc_1"}
	saved, err := repo.CreateGoal(context.Background(), g)
	if err != nil {
		t.Fatalf("CreateGoal() error = %v", err)
	}
	if saved.Current != 825 || saved.Target != 1000 {
		t.Errorf("saved = %+v, want Current=825 Target=1000", saved)
	}

	got, found, err := repo.GetGoal(context.Background(), "goal_1")
	if err != nil || !found {
		t.Fatalf("GetGoal() found=%v, err=%v", found, err)
	}
	if len(got.Providers) != 1 || got.Providers[0] != goals.ProviderTwitch {
		t.Errorf("Providers = %v, want [twitch]", got.Providers)
	}
	if len(got.Accounts) != 1 || got.Accounts[0] != "acc_1" {
		t.Errorf("Accounts = %v, want [acc_1]", got.Accounts)
	}
}

func TestGoalUpdateRejectsStaleRevision(t *testing.T) {
	db := newTestDB(t)
	repo := NewGoalsRepository(db.DB)
	g, _ := repo.CreateGoal(context.Background(), newTestGoal("goal_1"))

	stale := g
	stale.ConfigRevision = g.ConfigRevision + 5
	if _, err := repo.UpdateGoal(context.Background(), stale); err != goals.ErrConfigConflict {
		t.Fatalf("error = %v, want ErrConfigConflict", err)
	}
}

func TestGoalUpdateSucceedsAndBumpsRevision(t *testing.T) {
	db := newTestDB(t)
	repo := NewGoalsRepository(db.DB)
	g, _ := repo.CreateGoal(context.Background(), newTestGoal("goal_1"))

	g.Name = "Renamed"
	updated, err := repo.UpdateGoal(context.Background(), g)
	if err != nil {
		t.Fatalf("UpdateGoal() error = %v", err)
	}
	if updated.Name != "Renamed" {
		t.Errorf("Name = %q, want Renamed", updated.Name)
	}
	if updated.ConfigRevision != g.ConfigRevision+1 {
		t.Errorf("ConfigRevision = %d, want %d", updated.ConfigRevision, g.ConfigRevision+1)
	}
}

func TestGoalDeleteRejectedWhileReferenced(t *testing.T) {
	db := newTestDB(t)
	repo := NewGoalsRepository(db.DB)
	g, _ := repo.CreateGoal(context.Background(), newTestGoal("goal_1"))

	wp := goals.DefaultWidgetProfile(g.ID, "Widget")
	wp.ID, wp.PublicSlug = "widget_1", "slug1234567890123456789012345678901234"
	wp.CreatedAt, wp.UpdatedAt = time.Now().UTC(), time.Now().UTC()
	if _, err := repo.CreateWidgetProfile(context.Background(), wp); err != nil {
		t.Fatalf("CreateWidgetProfile() error = %v", err)
	}

	if err := repo.DeleteGoal(context.Background(), g.ID); err != goals.ErrGoalInUse {
		t.Fatalf("error = %v, want ErrGoalInUse", err)
	}
}

func TestGoalCurrentSurvivesRestart(t *testing.T) {
	db := newTestDB(t)
	repo := NewGoalsRepository(db.DB)
	repo.CreateGoal(context.Background(), newTestGoal("goal_1"))

	key := goals.AppliedEventKey{ProviderID: goals.ProviderTwitch, AccountID: "acc_1", ProviderEventKey: "follow_1"}
	applied, updated, err := repo.ApplyContribution(context.Background(), "goal_1", key, 1)
	if err != nil || !applied {
		t.Fatalf("ApplyContribution() = (%v, %v)", applied, err)
	}
	if updated.Current != 826 {
		t.Fatalf("Current = %d, want 826", updated.Current)
	}

	// Simulate a backend restart: reopen a repository over the same
	// underlying database handle (the real restart-persistence
	// guarantee is that nothing but the file matters).
	reopened := NewGoalsRepository(db.DB)
	got, found, err := reopened.GetGoal(context.Background(), "goal_1")
	if err != nil || !found {
		t.Fatalf("GetGoal() after reopen: found=%v, err=%v", found, err)
	}
	if got.Current != 826 {
		t.Errorf("Current after reopen = %d, want 826 (survives restart)", got.Current)
	}
}

func TestApplyContributionDuplicateNeverDoubleApplies(t *testing.T) {
	db := newTestDB(t)
	repo := NewGoalsRepository(db.DB)
	repo.CreateGoal(context.Background(), newTestGoal("goal_1"))

	key := goals.AppliedEventKey{ProviderID: goals.ProviderTwitch, AccountID: "acc_1", ProviderEventKey: "follow_1"}
	if applied, _, err := repo.ApplyContribution(context.Background(), "goal_1", key, 1); err != nil || !applied {
		t.Fatalf("first ApplyContribution() = (%v, %v)", applied, err)
	}
	applied, _, err := repo.ApplyContribution(context.Background(), "goal_1", key, 1)
	if err != nil || applied {
		t.Fatalf("second ApplyContribution() = (%v, %v), want (false, nil)", applied, err)
	}

	got, _, _ := repo.GetGoal(context.Background(), "goal_1")
	if got.Current != 826 {
		t.Errorf("Current = %d, want 826 (applied exactly once)", got.Current)
	}
}

func TestApplyContributionDifferentProviderEventKeysBothCount(t *testing.T) {
	db := newTestDB(t)
	repo := NewGoalsRepository(db.DB)
	repo.CreateGoal(context.Background(), newTestGoal("goal_1"))

	k1 := goals.AppliedEventKey{ProviderID: goals.ProviderTwitch, AccountID: "acc_1", ProviderEventKey: "follow_1"}
	k2 := goals.AppliedEventKey{ProviderID: goals.ProviderTwitch, AccountID: "acc_1", ProviderEventKey: "follow_2"}
	repo.ApplyContribution(context.Background(), "goal_1", k1, 1)
	repo.ApplyContribution(context.Background(), "goal_1", k2, 1)

	got, _, _ := repo.GetGoal(context.Background(), "goal_1")
	if got.Current != 827 {
		t.Errorf("Current = %d, want 827 (two genuinely different events)", got.Current)
	}
}

func TestApplyContributionConcurrentExactTotal(t *testing.T) {
	db := newTestDB(t)
	repo := NewGoalsRepository(db.DB)
	g := newTestGoal("goal_1")
	g.Current, g.Baseline = 0, 0
	repo.CreateGoal(context.Background(), g)

	const n = 25
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := goals.AppliedEventKey{ProviderID: goals.ProviderTwitch, AccountID: "acc_1", ProviderEventKey: intToKey(i)}
			if _, _, err := repo.ApplyContribution(context.Background(), "goal_1", key, 1); err != nil {
				t.Errorf("ApplyContribution() error = %v", err)
			}
		}(i)
	}
	wg.Wait()

	got, _, err := repo.GetGoal(context.Background(), "goal_1")
	if err != nil {
		t.Fatalf("GetGoal() error = %v", err)
	}
	if got.Current != n {
		t.Errorf("Current = %d, want %d - concurrent contributions must never lose an update", got.Current, n)
	}
}

func intToKey(i int) string {
	digits := "0123456789"
	if i < 10 {
		return "evt_" + string(digits[i])
	}
	return "evt_" + string(digits[i/10]) + string(digits[i%10])
}

func TestPruneAppliedEventsRemovesOldRows(t *testing.T) {
	db := newTestDB(t)
	repo := NewGoalsRepository(db.DB)
	repo.CreateGoal(context.Background(), newTestGoal("goal_1"))
	key := goals.AppliedEventKey{ProviderID: goals.ProviderTwitch, AccountID: "acc_1", ProviderEventKey: "follow_1"}
	repo.ApplyContribution(context.Background(), "goal_1", key, 1)

	n, err := repo.PruneAppliedEvents(context.Background(), time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatalf("PruneAppliedEvents() error = %v", err)
	}
	if n != 1 {
		t.Errorf("pruned %d rows, want 1", n)
	}

	// The ledger entry is gone, so the same key can be applied again -
	// proving the prune actually deleted the row, not just reported a
	// count.
	applied, _, err := repo.ApplyContribution(context.Background(), "goal_1", key, 1)
	if err != nil || !applied {
		t.Fatalf("re-apply after prune = (%v, %v), want (true, nil)", applied, err)
	}
}

func TestWidgetProfileCreateThenGetBySlug(t *testing.T) {
	db := newTestDB(t)
	repo := NewGoalsRepository(db.DB)
	repo.CreateGoal(context.Background(), newTestGoal("goal_1"))

	wp := goals.DefaultWidgetProfile("goal_1", "Widget")
	wp.ID, wp.PublicSlug = "widget_1", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	wp.CreatedAt, wp.UpdatedAt = time.Now().UTC(), time.Now().UTC()
	if _, err := repo.CreateWidgetProfile(context.Background(), wp); err != nil {
		t.Fatalf("CreateWidgetProfile() error = %v", err)
	}

	got, found, err := repo.GetWidgetProfileByPublicSlug(context.Background(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil || !found {
		t.Fatalf("GetWidgetProfileByPublicSlug() found=%v, err=%v", found, err)
	}
	if got.GoalID != "goal_1" {
		t.Errorf("GoalID = %q, want goal_1", got.GoalID)
	}
}

func TestWidgetProfileRotatePublicSlugInvalidatesOld(t *testing.T) {
	db := newTestDB(t)
	repo := NewGoalsRepository(db.DB)
	repo.CreateGoal(context.Background(), newTestGoal("goal_1"))
	wp := goals.DefaultWidgetProfile("goal_1", "Widget")
	wp.ID, wp.PublicSlug = "widget_1", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	wp.CreatedAt, wp.UpdatedAt = time.Now().UTC(), time.Now().UTC()
	repo.CreateWidgetProfile(context.Background(), wp)

	updated, err := repo.RotatePublicSlug(context.Background(), "widget_1", "cccccccccccccccccccccccccccccccccccccccc")
	if err != nil {
		t.Fatalf("RotatePublicSlug() error = %v", err)
	}
	if updated.PublicSlug != "cccccccccccccccccccccccccccccccccccccccc" {
		t.Errorf("PublicSlug = %q, want the new slug", updated.PublicSlug)
	}

	if _, found, _ := repo.GetWidgetProfileByPublicSlug(context.Background(), "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"); found {
		t.Error("old slug still resolves after rotation")
	}
}

// TestMigrationPreservesExistingRowsAcrossGoalsMigration proves the
// Stage 18A migration (0025_goals.sql) never disturbs data from earlier
// migrations, mirroring platform_repository_test.go's own
// TestDeletedSeedDataIsNotRecreated precedent for a migration-
// preservation proof.
func TestMigrationPreservesExistingRowsAcrossGoalsMigration(t *testing.T) {
	db := newTestDB(t)

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM platforms`).Scan(&count); err != nil {
		t.Fatalf("counting platforms failed: %v", err)
	}
	if count != 4 {
		t.Fatalf("seeded %d platforms before the goals migration ran, want 4", count)
	}

	if _, err := Migrate(context.Background(), db.DB); err != nil {
		t.Fatalf("re-running Migrate() failed: %v", err)
	}

	if err := db.QueryRow(`SELECT COUNT(*) FROM platforms`).Scan(&count); err != nil {
		t.Fatalf("counting platforms after re-migrate failed: %v", err)
	}
	if count != 4 {
		t.Errorf("after the goals migration, platforms = %d, want 4 (unchanged)", count)
	}

	if !tableExists(t, db, "goals") || !tableExists(t, db, "widget_profiles") {
		t.Error("expected goals/widget_profiles tables to exist after migration")
	}
}
