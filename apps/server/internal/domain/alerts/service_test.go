package alerts

import (
	"context"
	"errors"
	"testing"
)

// fakeRepo is a minimal in-memory Repository for Service tests - the
// sqlite implementation's own behavior (transactions, FK/UNIQUE
// mapping) is covered separately in internal/storage/sqlite.
type fakeRepo struct {
	profiles map[string]Profile
	rules    map[string]Rule
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{profiles: map[string]Profile{}, rules: map[string]Rule{}}
}

func (r *fakeRepo) CreateProfile(_ context.Context, p Profile) (Profile, error) {
	if _, exists := r.profiles[p.ID]; exists {
		return Profile{}, errors.New("duplicate id")
	}
	for _, existing := range r.profiles {
		if existing.PublicSlug == p.PublicSlug {
			return Profile{}, errors.New("duplicate slug")
		}
	}
	r.profiles[p.ID] = p
	return p, nil
}
func (r *fakeRepo) GetProfile(_ context.Context, id string) (Profile, bool, error) {
	p, ok := r.profiles[id]
	return p, ok, nil
}
func (r *fakeRepo) GetProfileByPublicSlug(_ context.Context, slug string) (Profile, bool, error) {
	for _, p := range r.profiles {
		if p.PublicSlug == slug {
			return p, true, nil
		}
	}
	return Profile{}, false, nil
}
func (r *fakeRepo) ListProfiles(_ context.Context) ([]Profile, error) {
	out := make([]Profile, 0, len(r.profiles))
	for _, p := range r.profiles {
		out = append(out, p)
	}
	return out, nil
}
func (r *fakeRepo) UpdateProfile(_ context.Context, p Profile) (Profile, error) {
	if _, ok := r.profiles[p.ID]; !ok {
		return Profile{}, ErrProfileNotFound
	}
	r.profiles[p.ID] = p
	return p, nil
}
func (r *fakeRepo) RotatePublicSlug(_ context.Context, id, newSlug string) (Profile, error) {
	p, ok := r.profiles[id]
	if !ok {
		return Profile{}, ErrProfileNotFound
	}
	p.PublicSlug = newSlug
	r.profiles[id] = p
	return p, nil
}
func (r *fakeRepo) DeleteProfile(_ context.Context, id string) error {
	delete(r.profiles, id)
	for rid, rule := range r.rules {
		if rule.ProfileID == id {
			delete(r.rules, rid)
		}
	}
	return nil
}

func (r *fakeRepo) CreateRule(_ context.Context, ru Rule) (Rule, error) {
	if _, exists := r.rules[ru.ID]; exists {
		return Rule{}, errors.New("duplicate id")
	}
	r.rules[ru.ID] = ru
	return ru, nil
}
func (r *fakeRepo) GetRule(_ context.Context, id string) (Rule, bool, error) {
	ru, ok := r.rules[id]
	return ru, ok, nil
}
func (r *fakeRepo) ListRules(_ context.Context, profileID string) ([]Rule, error) {
	var out []Rule
	for _, ru := range r.rules {
		if ru.ProfileID == profileID {
			out = append(out, ru)
		}
	}
	return out, nil
}
func (r *fakeRepo) UpdateRule(_ context.Context, ru Rule) (Rule, error) {
	if _, ok := r.rules[ru.ID]; !ok {
		return Rule{}, ErrRuleNotFound
	}
	r.rules[ru.ID] = ru
	return ru, nil
}
func (r *fakeRepo) DeleteRule(_ context.Context, id string) error {
	delete(r.rules, id)
	return nil
}

type fakeAccountLookup map[string]bool

func (f fakeAccountLookup) AccountExists(_ context.Context, accountID string) (bool, error) {
	return f[accountID], nil
}

func newTestService() (*Service, *fakeRepo) {
	repo := newFakeRepo()
	accounts := fakeAccountLookup{"acct_1": true, "acct_2": true}
	return NewService(repo, accounts, nil), repo
}

func baseRuleInput() RuleInput {
	return RuleInput{
		Name: "Follow alert", Enabled: true, EventType: EventFollow,
		Priority: 50, DurationMS: 5000, RequiredRole: RoleEveryone,
		ShowPlatform: true, ShowUsername: true,
		TextTemplate:   "{username} just followed!",
		EntryAnimation: AnimationFade, ExitAnimation: AnimationFade, AnimationDurationMS: 400,
		GroupWindowMS: DefaultGroupWindowMS, InterruptMode: InterruptNever, Interruptible: true,
	}
}

func TestServiceCreateProfileDefaults(t *testing.T) {
	svc, _ := newTestService()
	p, err := svc.CreateProfile(context.Background(), "Main")
	if err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}
	if p.ID == "" || p.PublicSlug == "" {
		t.Fatalf("CreateProfile() = %+v, want a generated id and slug", p)
	}
	if p.Theme != ThemeMinimal || p.Position != PositionBottom || p.TextAlign != AlignCenter {
		t.Errorf("CreateProfile() defaults = %+v, want minimal/bottom/center", p)
	}
	if p.MaxQueueItems != DefaultMaxQueueItems || p.MaximumQueueAgeSeconds != DefaultMaximumQueueAgeSeconds {
		t.Errorf("CreateProfile() queue bounds = %+v, want the documented defaults", p)
	}
}

func TestServiceCreateProfileRejectsEmptyName(t *testing.T) {
	svc, _ := newTestService()
	if _, err := svc.CreateProfile(context.Background(), ""); !errors.Is(err, ErrValidation) {
		t.Errorf("CreateProfile(\"\") error = %v, want ErrValidation", err)
	}
}

func TestServiceRotatePublicSlugChangesValue(t *testing.T) {
	svc, _ := newTestService()
	p, err := svc.CreateProfile(context.Background(), "Main")
	if err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}
	rotated, err := svc.RotatePublicSlug(context.Background(), p.ID)
	if err != nil {
		t.Fatalf("RotatePublicSlug() error = %v", err)
	}
	if rotated.PublicSlug == p.PublicSlug {
		t.Error("RotatePublicSlug() did not change the slug")
	}
}

func TestServiceCreateRuleRequiresExistingProfile(t *testing.T) {
	svc, _ := newTestService()
	if _, err := svc.CreateRule(context.Background(), "missing", baseRuleInput()); !errors.Is(err, ErrProfileNotFound) {
		t.Errorf("CreateRule() error = %v, want ErrProfileNotFound", err)
	}
}

func TestServiceCreateRuleFollowRejectsQuantity(t *testing.T) {
	svc, _ := newTestService()
	p, _ := svc.CreateProfile(context.Background(), "Main")
	in := baseRuleInput()
	minQty := int64(10)
	in.MinimumQuantity = &minQty
	if _, err := svc.CreateRule(context.Background(), p.ID, in); !errors.Is(err, ErrConditionUnsupported) {
		t.Errorf("CreateRule() with a quantity threshold on 'follow' error = %v, want ErrConditionUnsupported", err)
	}
}

func TestServiceCreateRuleBitsAcceptsQuantity(t *testing.T) {
	svc, _ := newTestService()
	p, _ := svc.CreateProfile(context.Background(), "Main")
	in := baseRuleInput()
	in.EventType = EventBits
	minQty, maxQty := int64(100), int64(999)
	in.MinimumQuantity = &minQty
	in.MaximumQuantity = &maxQty
	in.ShowQuantity = true
	r, err := svc.CreateRule(context.Background(), p.ID, in)
	if err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}
	if r.MinimumQuantity == nil || *r.MinimumQuantity != 100 {
		t.Errorf("r.MinimumQuantity = %v, want 100", r.MinimumQuantity)
	}
}

func TestServiceCreateRuleRejectsInvertedThreshold(t *testing.T) {
	svc, _ := newTestService()
	p, _ := svc.CreateProfile(context.Background(), "Main")
	in := baseRuleInput()
	in.EventType = EventBits
	minQty, maxQty := int64(999), int64(100)
	in.MinimumQuantity = &minQty
	in.MaximumQuantity = &maxQty
	if _, err := svc.CreateRule(context.Background(), p.ID, in); !errors.Is(err, ErrThresholdInvalid) {
		t.Errorf("CreateRule() with minimum > maximum error = %v, want ErrThresholdInvalid", err)
	}
}

func TestServiceCreateRuleRejectsUnsupportedRole(t *testing.T) {
	svc, _ := newTestService()
	p, _ := svc.CreateProfile(context.Background(), "Main")
	in := baseRuleInput()
	in.RequiredRole = RoleSubscriber
	if _, err := svc.CreateRule(context.Background(), p.ID, in); !errors.Is(err, ErrConditionUnsupported) {
		t.Errorf("CreateRule() with a subscriber role on 'follow' error = %v, want ErrConditionUnsupported", err)
	}
}

func TestServiceCreateRuleRejectsMessageOnFollow(t *testing.T) {
	svc, _ := newTestService()
	p, _ := svc.CreateProfile(context.Background(), "Main")
	in := baseRuleInput()
	in.ShowMessage = true
	if _, err := svc.CreateRule(context.Background(), p.ID, in); !errors.Is(err, ErrConditionUnsupported) {
		t.Errorf("CreateRule() with show-message on 'follow' error = %v, want ErrConditionUnsupported", err)
	}
}

func TestServiceCreateRuleRejectsUnknownAccountFilter(t *testing.T) {
	svc, _ := newTestService()
	p, _ := svc.CreateProfile(context.Background(), "Main")
	in := baseRuleInput()
	in.Accounts = []string{"acct_missing"}
	if _, err := svc.CreateRule(context.Background(), p.ID, in); !errors.Is(err, ErrAccountNotFound) {
		t.Errorf("CreateRule() with an unknown account filter error = %v, want ErrAccountNotFound", err)
	}
}

func TestServiceCreateRuleAcceptsKnownAccountFilter(t *testing.T) {
	svc, _ := newTestService()
	p, _ := svc.CreateProfile(context.Background(), "Main")
	in := baseRuleInput()
	in.Accounts = []string{"acct_1", "acct_2"}
	r, err := svc.CreateRule(context.Background(), p.ID, in)
	if err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}
	if len(r.Accounts) != 2 {
		t.Errorf("r.Accounts = %+v, want 2 entries", r.Accounts)
	}
}

func TestServiceOverlapWarningsDetectsOverlappingTiers(t *testing.T) {
	svc, _ := newTestService()
	p, _ := svc.CreateProfile(context.Background(), "Main")

	low := baseRuleInput()
	low.EventType = EventBits
	lowMin, lowMax := int64(1), int64(500)
	low.MinimumQuantity, low.MaximumQuantity = &lowMin, &lowMax

	high := baseRuleInput()
	high.EventType = EventBits
	highMin := int64(400)
	high.MinimumQuantity = &highMin // unbounded max, overlaps [1,500] at 400-500

	if _, err := svc.CreateRule(context.Background(), p.ID, low); err != nil {
		t.Fatalf("CreateRule(low) error = %v", err)
	}
	if _, err := svc.CreateRule(context.Background(), p.ID, high); err != nil {
		t.Fatalf("CreateRule(high) error = %v", err)
	}

	warnings, err := svc.OverlapWarnings(context.Background(), p.ID)
	if err != nil {
		t.Fatalf("OverlapWarnings() error = %v", err)
	}
	if len(warnings) != 1 {
		t.Fatalf("OverlapWarnings() = %+v, want exactly one warning", warnings)
	}
}

func TestServiceOverlapWarningsIgnoresNonOverlappingTiers(t *testing.T) {
	svc, _ := newTestService()
	p, _ := svc.CreateProfile(context.Background(), "Main")

	low := baseRuleInput()
	low.EventType = EventBits
	lowMin, lowMax := int64(1), int64(99)
	low.MinimumQuantity, low.MaximumQuantity = &lowMin, &lowMax

	high := baseRuleInput()
	high.EventType = EventBits
	highMin := int64(100)
	high.MinimumQuantity = &highMin

	if _, err := svc.CreateRule(context.Background(), p.ID, low); err != nil {
		t.Fatalf("CreateRule(low) error = %v", err)
	}
	if _, err := svc.CreateRule(context.Background(), p.ID, high); err != nil {
		t.Fatalf("CreateRule(high) error = %v", err)
	}

	warnings, err := svc.OverlapWarnings(context.Background(), p.ID)
	if err != nil {
		t.Fatalf("OverlapWarnings() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("OverlapWarnings() = %+v, want no warnings for non-overlapping tiers", warnings)
	}
}

func TestServiceOverlapWarningsIgnoresDifferentEventTypes(t *testing.T) {
	svc, _ := newTestService()
	p, _ := svc.CreateProfile(context.Background(), "Main")

	bits := baseRuleInput()
	bits.EventType = EventBits

	raid := baseRuleInput()
	raid.EventType = EventRaid

	if _, err := svc.CreateRule(context.Background(), p.ID, bits); err != nil {
		t.Fatalf("CreateRule(bits) error = %v", err)
	}
	if _, err := svc.CreateRule(context.Background(), p.ID, raid); err != nil {
		t.Fatalf("CreateRule(raid) error = %v", err)
	}
	warnings, err := svc.OverlapWarnings(context.Background(), p.ID)
	if err != nil {
		t.Fatalf("OverlapWarnings() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("OverlapWarnings() = %+v, want no warnings across different event types", warnings)
	}
}

func TestServiceDeleteProfileCascadesRulesInFakeRepo(t *testing.T) {
	svc, repo := newTestService()
	p, _ := svc.CreateProfile(context.Background(), "Main")
	r, err := svc.CreateRule(context.Background(), p.ID, baseRuleInput())
	if err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}
	if err := svc.DeleteProfile(context.Background(), p.ID); err != nil {
		t.Fatalf("DeleteProfile() error = %v", err)
	}
	if _, ok := repo.rules[r.ID]; ok {
		t.Error("rule survived DeleteProfile() in the fake repo")
	}
}
