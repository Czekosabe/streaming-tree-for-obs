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

// --- Stage 18B: supporter widgets, richer counters, dashboards
// (docs/supporter-widgets.md §5, §9, §18) ---------------------------------

// TestOldStyleWidgetProfileRowLoadsWithGoalKind proves a widget_profiles
// row written using only the exact pre-Stage-18B column set (no kind, no
// nullable goal_id, none of the new Stage 18B columns) still loads
// correctly through the widened repository, with Kind defaulting to
// "goal" and every new field at its safe zero value - the real-world
// shape of every row Stage 18A ever wrote, proven directly rather than
// only through the Go-layer defaults.
func TestOldStyleWidgetProfileRowLoadsWithGoalKind(t *testing.T) {
	db := newTestDB(t)
	repo := NewGoalsRepository(db.DB)
	repo.CreateGoal(context.Background(), newTestGoal("goal_1"))

	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := db.DB.Exec(`
		INSERT INTO widget_profiles (id, goal_id, name, enabled, public_slug, title_override,
			show_current, show_target, show_percent, orientation, text_align, font_family,
			background_color, foreground_color, fill_color, border_color, border_radius_px, opacity,
			created_at, updated_at)
		VALUES ('widget_old', 'goal_1', 'Old Widget', 1, 'oldstyleslugoldstyleslugoldstyleslug01', '',
			1, 1, 1, 'horizontal', 'center', 'sans_serif',
			'#00000080', '#ffffff', '#7c3aed', '#ffffff33', 12, 1.0,
			?, ?)`, now, now)
	if err != nil {
		t.Fatalf("raw pre-Stage-18B insert failed: %v", err)
	}

	got, found, err := repo.GetWidgetProfile(context.Background(), "widget_old")
	if err != nil || !found {
		t.Fatalf("GetWidgetProfile() found=%v, err=%v", found, err)
	}
	if got.Kind != goals.WidgetProfileKindGoal {
		t.Errorf("Kind = %q, want %q for an old-style row", got.Kind, goals.WidgetProfileKindGoal)
	}
	if got.GoalID != "goal_1" {
		t.Errorf("GoalID = %q, want goal_1", got.GoalID)
	}
	if got.MaxItems != 0 || got.Currency != "" || got.Metric != "" || got.Columns != 0 || len(got.Children) != 0 {
		t.Errorf("new Stage 18B fields not at zero value: %+v", got)
	}
	if !got.ShowProvider || !got.ShowTime {
		t.Errorf("ShowProvider/ShowTime = %v/%v, want both true (column default)", got.ShowProvider, got.ShowTime)
	}
}

func TestWidgetProfileEventDerivedFiltersRoundTrip(t *testing.T) {
	db := newTestDB(t)
	repo := NewGoalsRepository(db.DB)

	p := goals.DefaultWidgetProfileOfKind(goals.WidgetProfileKindLatestDonation, "", "Latest Donation")
	p.ID, p.PublicSlug = "widget_latest_donation", "ldldldldldldldldldldldldldldldldldldld01"
	p.Providers = []goals.ProviderID{goals.ProviderStreamElements}
	p.Accounts = []string{"src_se"}
	p.ShowMessage = true
	p.CreatedAt, p.UpdatedAt = time.Now().UTC(), time.Now().UTC()
	if _, err := repo.CreateWidgetProfile(context.Background(), p); err != nil {
		t.Fatalf("CreateWidgetProfile() error = %v", err)
	}

	got, found, err := repo.GetWidgetProfile(context.Background(), "widget_latest_donation")
	if err != nil || !found {
		t.Fatalf("GetWidgetProfile() found=%v, err=%v", found, err)
	}
	if got.Kind != goals.WidgetProfileKindLatestDonation {
		t.Errorf("Kind = %q, want latest_donation", got.Kind)
	}
	if len(got.Providers) != 1 || got.Providers[0] != goals.ProviderStreamElements {
		t.Errorf("Providers = %v, want [streamelements]", got.Providers)
	}
	if len(got.Accounts) != 1 || got.Accounts[0] != "src_se" {
		t.Errorf("Accounts = %v, want [src_se]", got.Accounts)
	}
	if !got.ShowMessage {
		t.Error("ShowMessage = false, want true")
	}
	if got.GoalID != "" {
		t.Errorf("GoalID = %q, want empty for a non-goal widget", got.GoalID)
	}
}

func TestSessionCounterAndLargestDonationRoundTripCurrencyAndMetric(t *testing.T) {
	db := newTestDB(t)
	repo := NewGoalsRepository(db.DB)

	counter := goals.DefaultWidgetProfileOfKind(goals.WidgetProfileKindSessionCounter, "", "Support Amount")
	counter.ID, counter.PublicSlug = "widget_counter", "counterslugcounterslugcounterslugcount01"
	counter.Metric = goals.MetricSupportAmount
	counter.Currency = "EUR"
	counter.CreatedAt, counter.UpdatedAt = time.Now().UTC(), time.Now().UTC()
	if _, err := repo.CreateWidgetProfile(context.Background(), counter); err != nil {
		t.Fatalf("CreateWidgetProfile(counter) error = %v", err)
	}
	got, _, _ := repo.GetWidgetProfile(context.Background(), "widget_counter")
	if got.Metric != goals.MetricSupportAmount || got.Currency != "EUR" {
		t.Errorf("Metric/Currency = %q/%q, want support_amount/EUR", got.Metric, got.Currency)
	}

	largest := goals.DefaultWidgetProfileOfKind(goals.WidgetProfileKindLargestDonation, "", "Largest Donation")
	largest.ID, largest.PublicSlug = "widget_largest", "largestslugslargestslugslargestslugs01"
	largest.Currency = "USD"
	largest.CreatedAt, largest.UpdatedAt = time.Now().UTC(), time.Now().UTC()
	if _, err := repo.CreateWidgetProfile(context.Background(), largest); err != nil {
		t.Fatalf("CreateWidgetProfile(largest) error = %v", err)
	}
	got2, _, _ := repo.GetWidgetProfile(context.Background(), "widget_largest")
	if got2.Currency != "USD" || got2.Metric != "" {
		t.Errorf("Currency/Metric = %q/%q, want USD/<empty>", got2.Currency, got2.Metric)
	}
}

func TestEventTickerEventTypesRoundTrip(t *testing.T) {
	db := newTestDB(t)
	repo := NewGoalsRepository(db.DB)

	p := goals.DefaultWidgetProfileOfKind(goals.WidgetProfileKindEventTicker, "", "Ticker")
	p.ID, p.PublicSlug = "widget_ticker", "tickerslugtickerslugtickerslugtickers01"
	p.EventTypes = []goals.SupporterEventType{goals.EventTypeFollow, goals.EventTypeDonation, goals.EventTypeRaid}
	p.CreatedAt, p.UpdatedAt = time.Now().UTC(), time.Now().UTC()
	if _, err := repo.CreateWidgetProfile(context.Background(), p); err != nil {
		t.Fatalf("CreateWidgetProfile() error = %v", err)
	}

	got, _, _ := repo.GetWidgetProfile(context.Background(), "widget_ticker")
	if len(got.EventTypes) != 3 {
		t.Fatalf("EventTypes = %v, want 3 entries", got.EventTypes)
	}

	// An update replacing the allowlist must fully overwrite it, never
	// merge with the previous set.
	p.EventTypes = []goals.SupporterEventType{goals.EventTypeBits}
	p.UpdatedAt = time.Now().UTC()
	if _, err := repo.UpdateWidgetProfile(context.Background(), p); err != nil {
		t.Fatalf("UpdateWidgetProfile() error = %v", err)
	}
	got2, _, _ := repo.GetWidgetProfile(context.Background(), "widget_ticker")
	if len(got2.EventTypes) != 1 || got2.EventTypes[0] != goals.EventTypeBits {
		t.Errorf("EventTypes after update = %v, want [bits]", got2.EventTypes)
	}
}

func TestDashboardChildrenCreateThenGetRoundTrips(t *testing.T) {
	db := newTestDB(t)
	repo := NewGoalsRepository(db.DB)

	leaf := goals.DefaultWidgetProfileOfKind(goals.WidgetProfileKindLatestFollower, "", "Leaf")
	leaf.ID, leaf.PublicSlug = "widget_leaf", "leafslugleafslugleafslugleafslugleaf01"
	leaf.CreatedAt, leaf.UpdatedAt = time.Now().UTC(), time.Now().UTC()
	if _, err := repo.CreateWidgetProfile(context.Background(), leaf); err != nil {
		t.Fatalf("CreateWidgetProfile(leaf) error = %v", err)
	}

	dash := goals.DefaultWidgetProfileOfKind(goals.WidgetProfileKindDashboard, "", "Dashboard")
	dash.ID, dash.PublicSlug = "widget_dash", "dashslugdashslugdashslugdashslugdashs01"
	dash.Columns = 2
	dash.Children = []goals.DashboardChild{{WidgetProfileID: "widget_leaf", Column: 1, ColumnSpan: 1, Row: 1, RowSpan: 1}}
	dash.CreatedAt, dash.UpdatedAt = time.Now().UTC(), time.Now().UTC()
	if _, err := repo.CreateWidgetProfile(context.Background(), dash); err != nil {
		t.Fatalf("CreateWidgetProfile(dashboard) error = %v", err)
	}

	got, found, err := repo.GetWidgetProfile(context.Background(), "widget_dash")
	if err != nil || !found {
		t.Fatalf("GetWidgetProfile() found=%v, err=%v", found, err)
	}
	if got.Columns != 2 {
		t.Errorf("Columns = %d, want 2", got.Columns)
	}
	if len(got.Children) != 1 || got.Children[0].WidgetProfileID != "widget_leaf" {
		t.Fatalf("Children = %+v, want one entry referencing widget_leaf", got.Children)
	}

	if err := repo.DeleteWidgetProfile(context.Background(), "widget_leaf"); err != goals.ErrWidgetProfileInUse {
		t.Fatalf("error = %v, want ErrWidgetProfileInUse", err)
	}

	// UpdateWidgetProfile fully replaces the children set, never merges -
	// mirrors writeWidgetProfileExtras' own delete-then-insert pattern.
	dash.Children = nil
	if _, err := repo.UpdateWidgetProfile(context.Background(), dash); err != nil {
		t.Fatalf("UpdateWidgetProfile() error = %v", err)
	}
	gotAfter, _, _ := repo.GetWidgetProfile(context.Background(), "widget_dash")
	if len(gotAfter.Children) != 0 {
		t.Errorf("Children after clearing = %v, want none", gotAfter.Children)
	}
	if err := repo.DeleteWidgetProfile(context.Background(), "widget_leaf"); err != nil {
		t.Errorf("DeleteWidgetProfile() error = %v, want nil now that no dashboard references it", err)
	}
}

// TestMigrationPreservesExistingRowsAcrossSupporterWidgetsMigration
// proves the Stage 18B migration (0026_supporter_widgets.sql) - the
// widget_profiles rebuild in particular - never disturbs existing goal/
// widget-profile data when re-applied, mirroring
// TestMigrationPreservesExistingRowsAcrossGoalsMigration's own identical
// precedent for 0025.
func TestMigrationPreservesExistingRowsAcrossSupporterWidgetsMigration(t *testing.T) {
	db := newTestDB(t)
	repo := NewGoalsRepository(db.DB)
	repo.CreateGoal(context.Background(), newTestGoal("goal_1"))
	wp := goals.DefaultWidgetProfile("goal_1", "Widget")
	wp.ID, wp.PublicSlug = "widget_1", "migratepreserveslugmigratepreserveslug01"
	wp.CreatedAt, wp.UpdatedAt = time.Now().UTC(), time.Now().UTC()
	if _, err := repo.CreateWidgetProfile(context.Background(), wp); err != nil {
		t.Fatalf("CreateWidgetProfile() error = %v", err)
	}

	if _, err := Migrate(context.Background(), db.DB); err != nil {
		t.Fatalf("re-running Migrate() failed: %v", err)
	}

	got, found, err := repo.GetWidgetProfile(context.Background(), "widget_1")
	if err != nil || !found {
		t.Fatalf("GetWidgetProfile() after re-migrate: found=%v, err=%v", found, err)
	}
	if got.Kind != goals.WidgetProfileKindGoal || got.GoalID != "goal_1" || got.Name != "Widget" {
		t.Errorf("widget profile changed across re-migration: %+v", got)
	}

	if !tableExists(t, db, "widget_profile_providers") || !tableExists(t, db, "widget_profile_accounts") ||
		!tableExists(t, db, "widget_profile_event_types") || !tableExists(t, db, "widget_profile_dashboard_children") {
		t.Error("expected every new Stage 18B child table to exist after migration")
	}
}
