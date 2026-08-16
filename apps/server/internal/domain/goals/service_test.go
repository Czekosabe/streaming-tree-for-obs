package goals

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeRepo is a minimal in-memory Repository for Service tests - the
// sqlite implementation's own behavior (transactions, unique-violation
// mapping) is covered separately in internal/storage/sqlite.
type fakeRepo struct {
	goals   map[string]Goal
	widgets map[string]WidgetProfile
	ledger  map[string]bool // "goalID|providerID|accountID|key"
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{goals: map[string]Goal{}, widgets: map[string]WidgetProfile{}, ledger: map[string]bool{}}
}

func (r *fakeRepo) CreateGoal(_ context.Context, g Goal) (Goal, error) {
	r.goals[g.ID] = g
	return g, nil
}
func (r *fakeRepo) GetGoal(_ context.Context, id string) (Goal, bool, error) {
	g, ok := r.goals[id]
	return g, ok, nil
}
func (r *fakeRepo) ListGoals(_ context.Context) ([]Goal, error) {
	out := make([]Goal, 0, len(r.goals))
	for _, g := range r.goals {
		out = append(out, g)
	}
	return out, nil
}
func (r *fakeRepo) UpdateGoal(_ context.Context, g Goal) (Goal, error) {
	existing, ok := r.goals[g.ID]
	if !ok {
		return Goal{}, ErrGoalNotFound
	}
	if existing.ConfigRevision != g.ConfigRevision {
		return Goal{}, ErrConfigConflict
	}
	g.ConfigRevision = existing.ConfigRevision + 1
	r.goals[g.ID] = g
	return g, nil
}
func (r *fakeRepo) DeleteGoal(_ context.Context, id string) error {
	if _, ok := r.goals[id]; !ok {
		return ErrGoalNotFound
	}
	for _, w := range r.widgets {
		if w.GoalID == id {
			return ErrGoalInUse
		}
	}
	delete(r.goals, id)
	return nil
}
func (r *fakeRepo) SetCurrent(_ context.Context, id string, current int64) (Goal, error) {
	g, ok := r.goals[id]
	if !ok {
		return Goal{}, ErrGoalNotFound
	}
	g.Current = current
	r.goals[id] = g
	return g, nil
}
func (r *fakeRepo) ResetProgress(_ context.Context, id string) (Goal, error) {
	g, ok := r.goals[id]
	if !ok {
		return Goal{}, ErrGoalNotFound
	}
	g.Current = g.Baseline
	r.goals[id] = g
	return g, nil
}
func ledgerKey(goalID string, key AppliedEventKey) string {
	return goalID + "|" + string(key.ProviderID) + "|" + key.AccountID + "|" + key.ProviderEventKey
}
func (r *fakeRepo) ApplyContribution(_ context.Context, goalID string, key AppliedEventKey, amount int64) (bool, Goal, error) {
	lk := ledgerKey(goalID, key)
	if r.ledger[lk] {
		return false, Goal{}, nil
	}
	g, ok := r.goals[goalID]
	if !ok {
		return false, Goal{}, ErrGoalNotFound
	}
	r.ledger[lk] = true
	g.Current += amount
	r.goals[goalID] = g
	return true, g, nil
}
func (r *fakeRepo) PruneAppliedEvents(_ context.Context, _ time.Time) (int64, error) { return 0, nil }

func (r *fakeRepo) CreateWidgetProfile(_ context.Context, p WidgetProfile) (WidgetProfile, error) {
	r.widgets[p.ID] = p
	return p, nil
}
func (r *fakeRepo) GetWidgetProfile(_ context.Context, id string) (WidgetProfile, bool, error) {
	p, ok := r.widgets[id]
	return p, ok, nil
}
func (r *fakeRepo) GetWidgetProfileByPublicSlug(_ context.Context, slug string) (WidgetProfile, bool, error) {
	for _, p := range r.widgets {
		if p.PublicSlug == slug {
			return p, true, nil
		}
	}
	return WidgetProfile{}, false, nil
}
func (r *fakeRepo) ListWidgetProfiles(_ context.Context, goalID string) ([]WidgetProfile, error) {
	out := []WidgetProfile{}
	for _, p := range r.widgets {
		if goalID == "" || p.GoalID == goalID {
			out = append(out, p)
		}
	}
	return out, nil
}
func (r *fakeRepo) UpdateWidgetProfile(_ context.Context, p WidgetProfile) (WidgetProfile, error) {
	if _, ok := r.widgets[p.ID]; !ok {
		return WidgetProfile{}, ErrWidgetProfileNotFound
	}
	r.widgets[p.ID] = p
	return p, nil
}
func (r *fakeRepo) RotatePublicSlug(_ context.Context, id, newSlug string) (WidgetProfile, error) {
	p, ok := r.widgets[id]
	if !ok {
		return WidgetProfile{}, ErrWidgetProfileNotFound
	}
	p.PublicSlug = newSlug
	r.widgets[id] = p
	return p, nil
}
func (r *fakeRepo) DeleteWidgetProfile(_ context.Context, id string) error {
	if _, ok := r.widgets[id]; !ok {
		return ErrWidgetProfileNotFound
	}
	delete(r.widgets, id)
	return nil
}

var _ Repository = (*fakeRepo)(nil)

type fakeAccountLookup struct{ existing map[string]bool }

func (f fakeAccountLookup) AccountExists(_ context.Context, id string) (bool, error) {
	return f.existing[id], nil
}

func newTestService(t *testing.T) (*Service, *fakeRepo) {
	t.Helper()
	repo := newFakeRepo()
	fixed := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	svc := NewService(repo, fakeAccountLookup{existing: map[string]bool{"acc_twitch": true, "src_se": true}}, func() time.Time { return fixed })
	return svc, repo
}

func TestCreateGoalSetsCurrentToBaseline(t *testing.T) {
	svc, _ := newTestService(t)
	g, err := svc.CreateGoal(context.Background(), Goal{Name: "Followers", Kind: KindFollowers, Target: 1000, Baseline: 825})
	if err != nil {
		t.Fatalf("CreateGoal() error: %v", err)
	}
	if g.Current != 825 {
		t.Errorf("Current = %d, want 825 (the operator's own baseline, never fabricated - docs/goals-widgets.md §1)", g.Current)
	}
	if g.ConfigRevision != 1 {
		t.Errorf("ConfigRevision = %d, want 1", g.ConfigRevision)
	}
}

func TestCreateGoalRejectsUnknownAccount(t *testing.T) {
	svc, _ := newTestService(t)
	_, err := svc.CreateGoal(context.Background(), Goal{Name: "Followers", Kind: KindFollowers, Target: 100, Accounts: []string{"nope"}})
	if !errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("error = %v, want ErrAccountNotFound", err)
	}
}

func TestCreateGoalAcceptsDonationSourceAccount(t *testing.T) {
	svc, _ := newTestService(t)
	g, err := svc.CreateGoal(context.Background(), Goal{
		Name: "Fund", Kind: KindDonations, Target: 100000000, Currency: "USD", Accounts: []string{"src_se"},
	})
	if err != nil {
		t.Fatalf("CreateGoal() error: %v", err)
	}
	if len(g.Accounts) != 1 || g.Accounts[0] != "src_se" {
		t.Errorf("Accounts = %v, want [src_se]", g.Accounts)
	}
}

func TestUpdateGoalRejectsStaleRevision(t *testing.T) {
	svc, _ := newTestService(t)
	g, _ := svc.CreateGoal(context.Background(), Goal{Name: "Followers", Kind: KindFollowers, Target: 100})

	g.ConfigRevision = g.ConfigRevision + 999
	if _, err := svc.UpdateGoal(context.Background(), g); !errors.Is(err, ErrConfigConflict) {
		t.Fatalf("error = %v, want ErrConfigConflict", err)
	}
}

func TestUpdateGoalResetsCurrentWhenBaselineChanges(t *testing.T) {
	svc, _ := newTestService(t)
	g, _ := svc.CreateGoal(context.Background(), Goal{Name: "Followers", Kind: KindFollowers, Target: 100, Baseline: 10})

	// Simulate real accumulated contributions moving Current away from Baseline.
	if _, _, err := svc.ApplyContribution(context.Background(), g.ID, AppliedEventKey{ProviderID: ProviderTwitch, AccountID: "acc_twitch", ProviderEventKey: "evt1"}, 5); err != nil {
		t.Fatalf("ApplyContribution() error: %v", err)
	}
	g, _ = svc.GetGoal(context.Background(), g.ID)
	if g.Current != 15 {
		t.Fatalf("Current = %d, want 15 before reconfigure", g.Current)
	}

	g.Baseline = 50
	updated, err := svc.UpdateGoal(context.Background(), g)
	if err != nil {
		t.Fatalf("UpdateGoal() error: %v", err)
	}
	if updated.Current != 50 {
		t.Errorf("Current = %d, want 50 (reset to the new baseline - docs/goals-widgets.md §9.3)", updated.Current)
	}
}

func TestUpdateGoalLeavesCurrentUntouchedWhenBaselineUnchanged(t *testing.T) {
	svc, _ := newTestService(t)
	g, _ := svc.CreateGoal(context.Background(), Goal{Name: "Followers", Kind: KindFollowers, Target: 100, Baseline: 10})
	svc.ApplyContribution(context.Background(), g.ID, AppliedEventKey{ProviderID: ProviderTwitch, AccountID: "acc_twitch", ProviderEventKey: "evt1"}, 5)
	g, _ = svc.GetGoal(context.Background(), g.ID)

	g.Name = "Renamed"
	updated, err := svc.UpdateGoal(context.Background(), g)
	if err != nil {
		t.Fatalf("UpdateGoal() error: %v", err)
	}
	if updated.Current != 15 {
		t.Errorf("Current = %d, want 15 (untouched by an unrelated rename)", updated.Current)
	}
}

func TestDeleteGoalRejectedWhileReferencedByWidgetProfile(t *testing.T) {
	svc, _ := newTestService(t)
	g, _ := svc.CreateGoal(context.Background(), Goal{Name: "Followers", Kind: KindFollowers, Target: 100})
	if _, err := svc.CreateWidgetProfile(context.Background(), DefaultWidgetProfile(g.ID, "Widget")); err != nil {
		t.Fatalf("CreateWidgetProfile() error: %v", err)
	}
	if err := svc.DeleteGoal(context.Background(), g.ID); !errors.Is(err, ErrGoalInUse) {
		t.Fatalf("error = %v, want ErrGoalInUse", err)
	}
}

func TestDeleteGoalSucceedsOnceWidgetProfilesRemoved(t *testing.T) {
	svc, _ := newTestService(t)
	g, _ := svc.CreateGoal(context.Background(), Goal{Name: "Followers", Kind: KindFollowers, Target: 100})
	w, _ := svc.CreateWidgetProfile(context.Background(), DefaultWidgetProfile(g.ID, "Widget"))
	if err := svc.DeleteWidgetProfile(context.Background(), w.ID); err != nil {
		t.Fatalf("DeleteWidgetProfile() error: %v", err)
	}
	if err := svc.DeleteGoal(context.Background(), g.ID); err != nil {
		t.Fatalf("DeleteGoal() error: %v", err)
	}
}

func TestSetCurrentPersistsAndValidatesBounds(t *testing.T) {
	svc, _ := newTestService(t)
	g, _ := svc.CreateGoal(context.Background(), Goal{Name: "Followers", Kind: KindFollowers, Target: 1000})

	updated, err := svc.SetCurrent(context.Background(), g.ID, 500)
	if err != nil {
		t.Fatalf("SetCurrent() error: %v", err)
	}
	if updated.Current != 500 {
		t.Errorf("Current = %d, want 500", updated.Current)
	}

	if _, err := svc.SetCurrent(context.Background(), g.ID, -1); err == nil {
		t.Fatal("expected an error for a negative current")
	}
}

func TestResetProgressRestoresBaseline(t *testing.T) {
	svc, _ := newTestService(t)
	g, _ := svc.CreateGoal(context.Background(), Goal{Name: "Followers", Kind: KindFollowers, Target: 1000, Baseline: 100})
	svc.SetCurrent(context.Background(), g.ID, 700)

	updated, err := svc.ResetProgress(context.Background(), g.ID)
	if err != nil {
		t.Fatalf("ResetProgress() error: %v", err)
	}
	if updated.Current != 100 {
		t.Errorf("Current = %d, want 100 (the goal's own baseline)", updated.Current)
	}
}

func TestManualActionsNeverTouchConfigRevision(t *testing.T) {
	svc, _ := newTestService(t)
	g, _ := svc.CreateGoal(context.Background(), Goal{Name: "Followers", Kind: KindFollowers, Target: 1000})
	before := g.ConfigRevision

	svc.SetCurrent(context.Background(), g.ID, 5)
	svc.ResetProgress(context.Background(), g.ID)
	svc.ApplyContribution(context.Background(), g.ID, AppliedEventKey{ProviderID: ProviderTwitch, AccountID: "acc_twitch", ProviderEventKey: "e1"}, 1)

	after, _ := svc.GetGoal(context.Background(), g.ID)
	if after.ConfigRevision != before {
		t.Errorf("ConfigRevision changed from %d to %d after manual/contribution actions - docs/goals-widgets.md §8.1 requires it stay untouched", before, after.ConfigRevision)
	}
}

func TestApplyContributionIsIdempotentForTheSameKey(t *testing.T) {
	svc, _ := newTestService(t)
	g, _ := svc.CreateGoal(context.Background(), Goal{Name: "Bits", Kind: KindBits, Target: 1000})
	key := AppliedEventKey{ProviderID: ProviderTwitch, AccountID: "acc_twitch", ProviderEventKey: "cheer_1"}

	applied1, _, err := svc.ApplyContribution(context.Background(), g.ID, key, 50)
	if err != nil || !applied1 {
		t.Fatalf("first ApplyContribution() = (%v, %v), want (true, nil)", applied1, err)
	}
	applied2, _, err := svc.ApplyContribution(context.Background(), g.ID, key, 50)
	if err != nil || applied2 {
		t.Fatalf("second ApplyContribution() = (%v, %v), want (false, nil) - duplicate must not double-apply", applied2, err)
	}

	final, _ := svc.GetGoal(context.Background(), g.ID)
	if final.Current != 50 {
		t.Errorf("Current = %d, want 50 (applied exactly once)", final.Current)
	}
}

func TestApplyContributionIsPerGoalNeverGlobal(t *testing.T) {
	svc, _ := newTestService(t)
	g1, _ := svc.CreateGoal(context.Background(), Goal{Name: "Fund A", Kind: KindDonations, Target: 100000000, Currency: "USD"})
	g2, _ := svc.CreateGoal(context.Background(), Goal{Name: "Fund B", Kind: KindDonations, Target: 200000000, Currency: "USD"})
	key := AppliedEventKey{ProviderID: ProviderStreamElements, AccountID: "src_se", ProviderEventKey: "tip_1"}

	if applied, _, err := svc.ApplyContribution(context.Background(), g1.ID, key, 5000000); err != nil || !applied {
		t.Fatalf("goal 1 ApplyContribution() = (%v, %v)", applied, err)
	}
	if applied, _, err := svc.ApplyContribution(context.Background(), g2.ID, key, 5000000); err != nil || !applied {
		t.Fatalf("goal 2 ApplyContribution() = (%v, %v), want applied=true - dedupe is per-goal, never global (docs/goals-widgets.md §11.4)", applied, err)
	}
}
