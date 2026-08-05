package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/account"
	"github.com/streaming-tree/server/internal/domain/engagementsettings"
)

func createTestAccount(t *testing.T, db *AccountRepository, id string) {
	t.Helper()
	now := time.Now().UTC()
	acc := account.Account{
		ID: id, ProviderID: account.ProviderTwitch, ProviderUserID: id + "_provider_user",
		Login: "viewer", DisplayName: "Viewer", Status: account.StatusConnected,
		CreatedAt: now, UpdatedAt: now, Scopes: []string{"channel:manage:broadcast"},
	}
	if err := db.CreateAccount(context.Background(), acc); err != nil {
		t.Fatalf("CreateAccount() error = %v", err)
	}
}

func TestEngagementSettingsGetReturnsNotFoundWhenAbsent(t *testing.T) {
	db := newTestDB(t)
	accounts := NewAccountRepository(db.DB)
	settings := NewEngagementSettingsRepository(db.DB)
	createTestAccount(t, accounts, "acct_es_1")

	_, found, err := settings.Get(context.Background(), "acct_es_1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if found {
		t.Error("Get() found = true, want false before any settings are set")
	}
}

func TestEngagementSettingsSetThenGetRoundTrips(t *testing.T) {
	db := newTestDB(t)
	accounts := NewAccountRepository(db.DB)
	settings := NewEngagementSettingsRepository(db.DB)
	createTestAccount(t, accounts, "acct_es_2")
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)

	saved, err := settings.Set(context.Background(), engagementsettings.Settings{AccountID: "acct_es_2", Enabled: true}, now)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if !saved.Enabled {
		t.Error("saved.Enabled = false, want true")
	}

	got, found, err := settings.Get(context.Background(), "acct_es_2")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !found || !got.Enabled {
		t.Errorf("got = %+v, found = %v, want enabled settings round-tripped", got, found)
	}
}

func TestEngagementSettingsSetReplacesExistingRowInPlace(t *testing.T) {
	db := newTestDB(t)
	accounts := NewAccountRepository(db.DB)
	settings := NewEngagementSettingsRepository(db.DB)
	createTestAccount(t, accounts, "acct_es_3")
	now := time.Now().UTC()

	if _, err := settings.Set(context.Background(), engagementsettings.Settings{AccountID: "acct_es_3", Enabled: true}, now); err != nil {
		t.Fatalf("first Set() error = %v", err)
	}
	if _, err := settings.Set(context.Background(), engagementsettings.Settings{AccountID: "acct_es_3", Enabled: false}, now); err != nil {
		t.Fatalf("second Set() error = %v", err)
	}

	got, found, err := settings.Get(context.Background(), "acct_es_3")
	if err != nil || !found {
		t.Fatalf("Get() error = %v, found = %v", err, found)
	}
	if got.Enabled {
		t.Error("Enabled = true, want the replaced value false")
	}

	var count int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM connected_account_engagement_settings WHERE account_id = ?`, "acct_es_3").Scan(&count); err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	if count != 1 {
		t.Errorf("row count = %d, want exactly 1 (replace, not a second row)", count)
	}
}

func TestEngagementSettingsSetRejectsAnUnknownAccount(t *testing.T) {
	db := newTestDB(t)
	settings := NewEngagementSettingsRepository(db.DB)

	if _, err := settings.Set(context.Background(), engagementsettings.Settings{AccountID: "acct_does_not_exist", Enabled: true}, time.Now().UTC()); err == nil {
		t.Fatal("Set() error = nil, want a foreign-key failure for an unknown account")
	}
}

func TestEngagementSettingsDeleteIsIdempotent(t *testing.T) {
	db := newTestDB(t)
	accounts := NewAccountRepository(db.DB)
	settings := NewEngagementSettingsRepository(db.DB)
	createTestAccount(t, accounts, "acct_es_4")

	if err := settings.Delete(context.Background(), "acct_es_4"); err != nil {
		t.Fatalf("Delete() on absent settings error = %v, want nil", err)
	}

	if _, err := settings.Set(context.Background(), engagementsettings.Settings{AccountID: "acct_es_4", Enabled: true}, time.Now().UTC()); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if err := settings.Delete(context.Background(), "acct_es_4"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	_, found, err := settings.Get(context.Background(), "acct_es_4")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if found {
		t.Error("settings still found after Delete()")
	}
}

func TestEngagementSettingsCascadesWhenAccountIsDeleted(t *testing.T) {
	db := newTestDB(t)
	accounts := NewAccountRepository(db.DB)
	settings := NewEngagementSettingsRepository(db.DB)
	createTestAccount(t, accounts, "acct_es_5")

	if _, err := settings.Set(context.Background(), engagementsettings.Settings{AccountID: "acct_es_5", Enabled: true}, time.Now().UTC()); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	if err := accounts.DeleteAccount(context.Background(), "acct_es_5"); err != nil {
		t.Fatalf("DeleteAccount() error = %v", err)
	}

	_, found, err := settings.Get(context.Background(), "acct_es_5")
	if err != nil {
		t.Fatalf("Get() after cascade error = %v", err)
	}
	if found {
		t.Error("engagement settings survived their account's deletion - it should cascade")
	}
}

func TestEngagementSettingsListEnabledReturnsOnlyEnabledAccounts(t *testing.T) {
	db := newTestDB(t)
	accounts := NewAccountRepository(db.DB)
	settings := NewEngagementSettingsRepository(db.DB)
	createTestAccount(t, accounts, "acct_es_enabled")
	createTestAccount(t, accounts, "acct_es_disabled")

	if _, err := settings.Set(context.Background(), engagementsettings.Settings{AccountID: "acct_es_enabled", Enabled: true}, time.Now().UTC()); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if _, err := settings.Set(context.Background(), engagementsettings.Settings{AccountID: "acct_es_disabled", Enabled: false}, time.Now().UTC()); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	enabled, err := settings.ListEnabled(context.Background())
	if err != nil {
		t.Fatalf("ListEnabled() error = %v", err)
	}
	if len(enabled) != 1 || enabled[0].AccountID != "acct_es_enabled" {
		t.Errorf("ListEnabled() = %+v, want exactly [acct_es_enabled]", enabled)
	}
}
