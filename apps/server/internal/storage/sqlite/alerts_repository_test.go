package sqlite

import (
	"context"
	"testing"

	"github.com/streaming-tree/server/internal/domain/alerts"
)

func newTestProfile(id, slug string) alerts.Profile {
	p := alerts.DefaultProfile("Main alerts")
	p.ID = id
	p.PublicSlug = slug
	return p
}

func TestAlertsProfileCreateThenGetRoundTrips(t *testing.T) {
	db := newTestDB(t)
	repo := NewAlertsRepository(db.DB)

	p := newTestProfile("alprof_1", "slugslugslugslug1")
	saved, err := repo.CreateProfile(context.Background(), p)
	if err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}
	if saved.Name != "Main alerts" || saved.PublicSlug != "slugslugslugslug1" || !saved.Enabled {
		t.Errorf("saved = %+v, want the created fields", saved)
	}
	if saved.Theme != alerts.ThemeMinimal || saved.Position != alerts.PositionBottom {
		t.Errorf("saved defaults = %+v, want minimal/bottom", saved)
	}

	got, found, err := repo.GetProfile(context.Background(), "alprof_1")
	if err != nil || !found {
		t.Fatalf("GetProfile() = %+v, found=%v, err=%v", got, found, err)
	}
	if got.ID != saved.ID {
		t.Errorf("GetProfile().ID = %q, want %q", got.ID, saved.ID)
	}
}

func TestAlertsProfileGetByPublicSlug(t *testing.T) {
	db := newTestDB(t)
	repo := NewAlertsRepository(db.DB)
	if _, err := repo.CreateProfile(context.Background(), newTestProfile("alprof_1", "abc123")); err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}

	got, found, err := repo.GetProfileByPublicSlug(context.Background(), "abc123")
	if err != nil || !found {
		t.Fatalf("GetProfileByPublicSlug() = %+v, found=%v, err=%v", got, found, err)
	}
	if got.ID != "alprof_1" {
		t.Errorf("got.ID = %q, want alprof_1", got.ID)
	}

	_, found, err = repo.GetProfileByPublicSlug(context.Background(), "does-not-exist")
	if err != nil {
		t.Fatalf("GetProfileByPublicSlug() error = %v", err)
	}
	if found {
		t.Error("GetProfileByPublicSlug() found = true for an unknown slug, want false")
	}
}

func TestAlertsProfilePublicSlugUnique(t *testing.T) {
	db := newTestDB(t)
	repo := NewAlertsRepository(db.DB)
	if _, err := repo.CreateProfile(context.Background(), newTestProfile("alprof_1", "same-slug")); err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}
	if _, err := repo.CreateProfile(context.Background(), newTestProfile("alprof_2", "same-slug")); err == nil {
		t.Error("CreateProfile() with a duplicate slug succeeded, want an error")
	}
}

func TestAlertsProfileRotatePublicSlugInvalidatesOld(t *testing.T) {
	db := newTestDB(t)
	repo := NewAlertsRepository(db.DB)
	if _, err := repo.CreateProfile(context.Background(), newTestProfile("alprof_1", "old-slug")); err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}

	updated, err := repo.RotatePublicSlug(context.Background(), "alprof_1", "new-slug")
	if err != nil {
		t.Fatalf("RotatePublicSlug() error = %v", err)
	}
	if updated.PublicSlug != "new-slug" {
		t.Errorf("updated.PublicSlug = %q, want new-slug", updated.PublicSlug)
	}

	if _, found, err := repo.GetProfileByPublicSlug(context.Background(), "old-slug"); err != nil || found {
		t.Errorf("old slug still resolves: found=%v err=%v, want found=false", found, err)
	}
	if _, found, err := repo.GetProfileByPublicSlug(context.Background(), "new-slug"); err != nil || !found {
		t.Errorf("new slug does not resolve: found=%v err=%v, want found=true", found, err)
	}
}

func TestAlertsProfileDeleteCascadesRules(t *testing.T) {
	db := newTestDB(t)
	repo := NewAlertsRepository(db.DB)
	if _, err := repo.CreateProfile(context.Background(), newTestProfile("alprof_1", "slug-x")); err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}
	r := testRule("alrule_1", "alprof_1")
	if _, err := repo.CreateRule(context.Background(), r); err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}

	if err := repo.DeleteProfile(context.Background(), "alprof_1"); err != nil {
		t.Fatalf("DeleteProfile() error = %v", err)
	}
	if _, found, err := repo.GetRule(context.Background(), "alrule_1"); err != nil || found {
		t.Errorf("rule survived profile deletion: found=%v err=%v", found, err)
	}
}

func testRule(id, profileID string) alerts.Rule {
	return alerts.Rule{
		ID: id, ProfileID: profileID, Name: "Follow alert", Enabled: true,
		EventType: alerts.EventFollow, Priority: 50, DurationMS: 5000,
		RequiredRole: alerts.RoleEveryone,
		ShowPlatform: true, ShowUsername: true,
		TextTemplate:   "{username} just followed!",
		EntryAnimation: alerts.AnimationFade, ExitAnimation: alerts.AnimationFade, AnimationDurationMS: 400,
		GroupWindowMS: alerts.DefaultGroupWindowMS, InterruptMode: alerts.InterruptNever, Interruptible: true,
	}
}

func TestAlertsRuleCreateThenGetRoundTrips(t *testing.T) {
	db := newTestDB(t)
	accounts := NewAccountRepository(db.DB)
	repo := NewAlertsRepository(db.DB)
	createTestAccount(t, accounts, "acct_1")
	if _, err := repo.CreateProfile(context.Background(), newTestProfile("alprof_1", "slug-x")); err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}

	r := testRule("alrule_1", "alprof_1")
	r.Providers = []alerts.ProviderID{alerts.ProviderTwitch}
	r.Accounts = []string{"acct_1"}
	minQty := int64(100)
	maxQty := int64(999)
	r.EventType = alerts.EventBits
	r.MinimumQuantity = &minQty
	r.MaximumQuantity = &maxQty
	r.ShowQuantity = true

	saved, err := repo.CreateRule(context.Background(), r)
	if err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}
	if saved.Name != "Follow alert" || saved.EventType != alerts.EventBits {
		t.Errorf("saved = %+v, want the created fields", saved)
	}
	if len(saved.Providers) != 1 || saved.Providers[0] != alerts.ProviderTwitch {
		t.Errorf("saved.Providers = %+v, want [twitch]", saved.Providers)
	}
	if len(saved.Accounts) != 1 || saved.Accounts[0] != "acct_1" {
		t.Errorf("saved.Accounts = %+v, want [acct_1]", saved.Accounts)
	}
	if saved.MinimumQuantity == nil || *saved.MinimumQuantity != 100 {
		t.Errorf("saved.MinimumQuantity = %v, want 100", saved.MinimumQuantity)
	}
	if saved.MaximumQuantity == nil || *saved.MaximumQuantity != 999 {
		t.Errorf("saved.MaximumQuantity = %v, want 999", saved.MaximumQuantity)
	}

	got, found, err := repo.GetRule(context.Background(), "alrule_1")
	if err != nil || !found {
		t.Fatalf("GetRule() = %+v, found=%v, err=%v", got, found, err)
	}
}

// TestAlertsRuleRoundTripsYouTubeMoneyFields pins Stage 15A's own
// migration (0019_alerts_youtube_events.sql) - currency/minimum_amount_
// micros/maximum_amount_micros/show_amount, plus a YouTube event_type
// value, all round-trip through the rebuilt alert_rules table exactly.
func TestAlertsRuleRoundTripsYouTubeMoneyFields(t *testing.T) {
	db := newTestDB(t)
	repo := NewAlertsRepository(db.DB)
	if _, err := repo.CreateProfile(context.Background(), newTestProfile("alprof_1", "slug-x")); err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}

	r := testRule("alrule_1", "alprof_1")
	r.EventType = alerts.EventYouTubeSuperChat
	r.Providers = []alerts.ProviderID{alerts.ProviderYouTube}
	minAmount := int64(1_000_000)
	maxAmount := int64(50_000_000)
	r.Currency = "USD"
	r.MinimumAmountMicros = &minAmount
	r.MaximumAmountMicros = &maxAmount
	r.ShowAmount = true

	saved, err := repo.CreateRule(context.Background(), r)
	if err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}
	if saved.EventType != alerts.EventYouTubeSuperChat {
		t.Errorf("saved.EventType = %q, want youtube_super_chat", saved.EventType)
	}
	if len(saved.Providers) != 1 || saved.Providers[0] != alerts.ProviderYouTube {
		t.Errorf("saved.Providers = %+v, want [youtube]", saved.Providers)
	}
	if saved.Currency != "USD" {
		t.Errorf("saved.Currency = %q, want USD", saved.Currency)
	}
	if saved.MinimumAmountMicros == nil || *saved.MinimumAmountMicros != 1_000_000 {
		t.Errorf("saved.MinimumAmountMicros = %v, want 1000000", saved.MinimumAmountMicros)
	}
	if saved.MaximumAmountMicros == nil || *saved.MaximumAmountMicros != 50_000_000 {
		t.Errorf("saved.MaximumAmountMicros = %v, want 50000000", saved.MaximumAmountMicros)
	}
	if !saved.ShowAmount {
		t.Error("saved.ShowAmount = false, want true")
	}

	got, found, err := repo.GetRule(context.Background(), "alrule_1")
	if err != nil || !found {
		t.Fatalf("GetRule() = %+v, found=%v, err=%v", got, found, err)
	}
	if got.Currency != "USD" || got.MinimumAmountMicros == nil || *got.MinimumAmountMicros != 1_000_000 {
		t.Errorf("GetRule() = %+v, want currency/amount to survive a fresh read", got)
	}
}

// TestAlertsRuleDefaultsToEmptyCurrencyAndNoAmount pins the safe-migration
// contract: an existing (or new non-monetary) rule that never sets a
// monetary condition stores an empty currency and NULL amount bounds,
// never a fabricated default currency.
func TestAlertsRuleDefaultsToEmptyCurrencyAndNoAmount(t *testing.T) {
	db := newTestDB(t)
	repo := NewAlertsRepository(db.DB)
	if _, err := repo.CreateProfile(context.Background(), newTestProfile("alprof_1", "slug-x")); err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}
	r := testRule("alrule_1", "alprof_1")

	saved, err := repo.CreateRule(context.Background(), r)
	if err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}
	if saved.Currency != "" {
		t.Errorf("saved.Currency = %q, want empty", saved.Currency)
	}
	if saved.MinimumAmountMicros != nil || saved.MaximumAmountMicros != nil {
		t.Errorf("saved amount bounds = %v/%v, want both nil", saved.MinimumAmountMicros, saved.MaximumAmountMicros)
	}
	if saved.ShowAmount {
		t.Error("saved.ShowAmount = true, want false by default")
	}
}

// TestAlertsRuleCreateAcceptsAnyAccountIDAtTheRepositoryLayer documents a
// deliberate Stage 16A architecture change: alert_rule_accounts.
// connected_account_id no longer carries a SQL foreign key (migration
// 0020), since an alert rule's account/source filter may name either a
// connected_accounts row or a donation_sources row, and SQLite cannot
// express a foreign key against two tables. Existence is validated at
// the domain.Service layer only now (see
// TestCreateRuleRejectsUnknownAccount in
// internal/domain/alerts/service_test.go, which proves the real
// rejection via a fake AccountLookup) - the repository itself, like
// alert_rule_providers.provider_id before it, persists whatever id it is
// given.
func TestAlertsRuleCreateAcceptsAnyAccountIDAtTheRepositoryLayer(t *testing.T) {
	db := newTestDB(t)
	repo := NewAlertsRepository(db.DB)
	if _, err := repo.CreateProfile(context.Background(), newTestProfile("alprof_1", "slug-x")); err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}
	r := testRule("alrule_1", "alprof_1")
	r.Accounts = []string{"donsrc_missing_or_real_either_way"}
	if _, err := repo.CreateRule(context.Background(), r); err != nil {
		t.Errorf("CreateRule() error = %v, want nil (no repository-level account/source existence check)", err)
	}
}

func TestAlertsRuleCreateRejectsUnknownProfile(t *testing.T) {
	db := newTestDB(t)
	repo := NewAlertsRepository(db.DB)
	r := testRule("alrule_1", "alprof_missing")
	if _, err := repo.CreateRule(context.Background(), r); err != alerts.ErrProfileNotFound {
		t.Errorf("CreateRule() error = %v, want ErrProfileNotFound", err)
	}
}

func TestAlertsRuleUpdateReplacesFilters(t *testing.T) {
	db := newTestDB(t)
	accounts := NewAccountRepository(db.DB)
	repo := NewAlertsRepository(db.DB)
	createTestAccount(t, accounts, "acct_1")
	createTestAccount(t, accounts, "acct_2")
	if _, err := repo.CreateProfile(context.Background(), newTestProfile("alprof_1", "slug-x")); err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}
	r := testRule("alrule_1", "alprof_1")
	r.Accounts = []string{"acct_1"}
	if _, err := repo.CreateRule(context.Background(), r); err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}

	r.Name = "Updated"
	r.Accounts = []string{"acct_2"}
	r.Providers = []alerts.ProviderID{alerts.ProviderTwitch}
	updated, err := repo.UpdateRule(context.Background(), r)
	if err != nil {
		t.Fatalf("UpdateRule() error = %v", err)
	}
	if updated.Name != "Updated" {
		t.Errorf("updated.Name = %q, want Updated", updated.Name)
	}
	if len(updated.Accounts) != 1 || updated.Accounts[0] != "acct_2" {
		t.Errorf("updated.Accounts = %+v, want [acct_2]", updated.Accounts)
	}
}

func TestAlertsRuleDeleteRemovesFilters(t *testing.T) {
	db := newTestDB(t)
	accounts := NewAccountRepository(db.DB)
	repo := NewAlertsRepository(db.DB)
	createTestAccount(t, accounts, "acct_1")
	if _, err := repo.CreateProfile(context.Background(), newTestProfile("alprof_1", "slug-x")); err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}
	r := testRule("alrule_1", "alprof_1")
	r.Accounts = []string{"acct_1"}
	if _, err := repo.CreateRule(context.Background(), r); err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}
	if err := repo.DeleteRule(context.Background(), "alrule_1"); err != nil {
		t.Fatalf("DeleteRule() error = %v", err)
	}
	var count int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM alert_rule_accounts WHERE rule_id = ?`, "alrule_1").Scan(&count); err != nil {
		t.Fatalf("count accounts: %v", err)
	}
	if count != 0 {
		t.Errorf("alert_rule_accounts count = %d after delete, want 0", count)
	}
}

func TestAlertsRuleListOrderedByCreation(t *testing.T) {
	db := newTestDB(t)
	repo := NewAlertsRepository(db.DB)
	if _, err := repo.CreateProfile(context.Background(), newTestProfile("alprof_1", "slug-x")); err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}
	if _, err := repo.CreateRule(context.Background(), testRule("alrule_1", "alprof_1")); err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}
	if _, err := repo.CreateRule(context.Background(), testRule("alrule_2", "alprof_1")); err != nil {
		t.Fatalf("CreateRule() error = %v", err)
	}
	list, err := repo.ListRules(context.Background(), "alprof_1")
	if err != nil {
		t.Fatalf("ListRules() error = %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("ListRules() len = %d, want 2", len(list))
	}
}

func TestAlertsRuleNoRuntimeColumns(t *testing.T) {
	db := newTestDB(t)
	var cols []string
	rows, err := db.DB.Query(`PRAGMA table_info(alert_rules)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan pragma row: %v", err)
		}
		cols = append(cols, name)
	}
	forbidden := []string{"queue", "current", "next_run", "cooldown", "counter", "token", "raw_event"}
	for _, c := range cols {
		for _, f := range forbidden {
			if c == f {
				t.Errorf("alert_rules has an unexpected runtime-looking column %q", c)
			}
		}
	}
}
