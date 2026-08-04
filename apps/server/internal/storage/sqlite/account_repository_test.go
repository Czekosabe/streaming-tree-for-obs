package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/account"
)

func newTestAccount(id, providerUserID string) account.Account {
	now := time.Now().UTC()
	return account.Account{
		ID: id, ProviderID: account.ProviderTwitch, ProviderUserID: providerUserID,
		Login: "streamer", DisplayName: "Streamer", AvatarURL: "https://example.invalid/a.png",
		Status: account.StatusConnected, LastValidatedAt: &now, CreatedAt: now, UpdatedAt: now,
		Scopes: []string{"channel:manage:broadcast"},
	}
}

func TestCreateAndGetAccountRoundTrips(t *testing.T) {
	db := newTestDB(t)
	repo := NewAccountRepository(db.DB)
	ctx := context.Background()

	acc := newTestAccount("acct_1", "u1")
	if err := repo.CreateAccount(ctx, acc); err != nil {
		t.Fatalf("CreateAccount() error = %v", err)
	}

	got, err := repo.GetAccount(ctx, "acct_1")
	if err != nil {
		t.Fatalf("GetAccount() error = %v", err)
	}
	if got.Login != "streamer" || got.ProviderUserID != "u1" {
		t.Errorf("GetAccount() = %+v", got)
	}
	if len(got.Scopes) != 1 || got.Scopes[0] != "channel:manage:broadcast" {
		t.Errorf("Scopes = %v, want [channel:manage:broadcast]", got.Scopes)
	}
}

func TestGetAccountReturnsNotFoundForAnUnknownID(t *testing.T) {
	db := newTestDB(t)
	repo := NewAccountRepository(db.DB)

	_, err := repo.GetAccount(context.Background(), "acct_missing")
	if !errors.Is(err, account.ErrNotFound) {
		t.Errorf("GetAccount() error = %v, want ErrNotFound", err)
	}
}

func TestCreateAccountRejectsADuplicateProviderIdentity(t *testing.T) {
	db := newTestDB(t)
	repo := NewAccountRepository(db.DB)
	ctx := context.Background()

	if err := repo.CreateAccount(ctx, newTestAccount("acct_1", "u1")); err != nil {
		t.Fatalf("first CreateAccount() error = %v", err)
	}
	second := newTestAccount("acct_2", "u1")
	if err := repo.CreateAccount(ctx, second); !errors.Is(err, account.ErrConflict) {
		t.Errorf("CreateAccount() with a duplicate identity error = %v, want ErrConflict", err)
	}
}

func TestFindByProviderIdentityLocatesAnExistingAccount(t *testing.T) {
	db := newTestDB(t)
	repo := NewAccountRepository(db.DB)
	ctx := context.Background()
	_ = repo.CreateAccount(ctx, newTestAccount("acct_1", "u1"))

	found, ok, err := repo.FindByProviderIdentity(ctx, account.ProviderTwitch, "u1")
	if err != nil || !ok {
		t.Fatalf("FindByProviderIdentity() = %+v, %v, %v", found, ok, err)
	}
	if found.ID != "acct_1" {
		t.Errorf("ID = %q, want acct_1", found.ID)
	}

	_, ok, err = repo.FindByProviderIdentity(ctx, account.ProviderTwitch, "u-unknown")
	if err != nil || ok {
		t.Errorf("FindByProviderIdentity() for an unknown identity = %v, %v", ok, err)
	}
}

func TestUpdateAccountReplacesScopes(t *testing.T) {
	db := newTestDB(t)
	repo := NewAccountRepository(db.DB)
	ctx := context.Background()
	acc := newTestAccount("acct_1", "u1")
	_ = repo.CreateAccount(ctx, acc)

	acc.Scopes = []string{"channel:manage:broadcast", "user:read:email"}
	acc.DisplayName = "Renamed"
	if err := repo.UpdateAccount(ctx, acc); err != nil {
		t.Fatalf("UpdateAccount() error = %v", err)
	}

	got, err := repo.GetAccount(ctx, "acct_1")
	if err != nil {
		t.Fatalf("GetAccount() error = %v", err)
	}
	if got.DisplayName != "Renamed" {
		t.Errorf("DisplayName = %q, want Renamed", got.DisplayName)
	}
	if len(got.Scopes) != 2 {
		t.Errorf("Scopes = %v, want 2 entries", got.Scopes)
	}
}

func TestDeleteAccountCascadesLinks(t *testing.T) {
	db := newTestDB(t)
	repo := NewAccountRepository(db.DB)
	ctx := context.Background()
	_ = repo.CreateAccount(ctx, newTestAccount("acct_1", "u1"))
	if _, err := repo.SetLink(ctx, "pf_seed_twitch", "acct_1", time.Now().UTC()); err != nil {
		t.Fatalf("SetLink() error = %v", err)
	}

	if err := repo.DeleteAccount(ctx, "acct_1"); err != nil {
		t.Fatalf("DeleteAccount() error = %v", err)
	}
	if _, found, _ := repo.GetLink(ctx, "pf_seed_twitch"); found {
		t.Error("the link survived the account's deletion")
	}
}

func TestDeletePlatformCascadesItsLinkButNotTheAccount(t *testing.T) {
	db := newTestDB(t)
	platforms := NewPlatformRepository(db.DB)
	accounts := NewAccountRepository(db.DB)
	ctx := context.Background()

	_ = accounts.CreateAccount(ctx, newTestAccount("acct_1", "u1"))
	if _, err := accounts.SetLink(ctx, "pf_seed_twitch", "acct_1", time.Now().UTC()); err != nil {
		t.Fatalf("SetLink() error = %v", err)
	}

	if err := platforms.Delete(ctx, "pf_seed_twitch"); err != nil {
		t.Fatalf("Delete(platform) error = %v", err)
	}

	if _, found, _ := accounts.GetLink(ctx, "pf_seed_twitch"); found {
		t.Error("the link survived the platform's deletion")
	}
	if _, err := accounts.GetAccount(ctx, "acct_1"); err != nil {
		t.Error("the account was deleted along with the platform")
	}
}

func TestSetLinkReplacesAnExistingLink(t *testing.T) {
	db := newTestDB(t)
	repo := NewAccountRepository(db.DB)
	ctx := context.Background()
	_ = repo.CreateAccount(ctx, newTestAccount("acct_1", "u1"))
	_ = repo.CreateAccount(ctx, newTestAccount("acct_2", "u2"))

	if _, err := repo.SetLink(ctx, "pf_seed_twitch", "acct_1", time.Now().UTC()); err != nil {
		t.Fatalf("first SetLink() error = %v", err)
	}
	if _, err := repo.SetLink(ctx, "pf_seed_twitch", "acct_2", time.Now().UTC()); err != nil {
		t.Fatalf("second SetLink() error = %v", err)
	}

	link, found, err := repo.GetLink(ctx, "pf_seed_twitch")
	if err != nil || !found {
		t.Fatalf("GetLink() = %+v, %v, %v", link, found, err)
	}
	if link.AccountID != "acct_2" {
		t.Errorf("AccountID = %q, want acct_2 after replacing the link", link.AccountID)
	}
}

func TestSetLinkRejectsAnUnknownAccount(t *testing.T) {
	db := newTestDB(t)
	repo := NewAccountRepository(db.DB)

	_, err := repo.SetLink(context.Background(), "pf_seed_twitch", "acct_missing", time.Now().UTC())
	if !errors.Is(err, account.ErrConflict) {
		t.Errorf("SetLink() with an unknown account error = %v, want ErrConflict", err)
	}
}

func TestDeleteLinkIsIdempotent(t *testing.T) {
	db := newTestDB(t)
	repo := NewAccountRepository(db.DB)

	if err := repo.DeleteLink(context.Background(), "pf_never_linked"); err != nil {
		t.Errorf("DeleteLink() on an absent link error = %v, want nil", err)
	}
}

func TestIntegrationSettingsRoundTrip(t *testing.T) {
	db := newTestDB(t)
	repo := NewAccountRepository(db.DB)
	ctx := context.Background()

	if _, found, err := repo.GetIntegrationSettings(ctx, account.ProviderTwitch); err != nil || found {
		t.Fatalf("GetIntegrationSettings() before any save = %v, %v", found, err)
	}

	if _, err := repo.SetIntegrationSettings(ctx, account.ProviderTwitch, "client-1", time.Now().UTC()); err != nil {
		t.Fatalf("SetIntegrationSettings() error = %v", err)
	}
	settings, found, err := repo.GetIntegrationSettings(ctx, account.ProviderTwitch)
	if err != nil || !found || settings.ClientID != "client-1" {
		t.Fatalf("GetIntegrationSettings() = %+v, %v, %v", settings, found, err)
	}

	if _, err := repo.SetIntegrationSettings(ctx, account.ProviderTwitch, "client-2", time.Now().UTC()); err != nil {
		t.Fatalf("second SetIntegrationSettings() error = %v", err)
	}
	settings, _, _ = repo.GetIntegrationSettings(ctx, account.ProviderTwitch)
	if settings.ClientID != "client-2" {
		t.Errorf("ClientID = %q, want client-2 after replacing it", settings.ClientID)
	}
}

func TestListAccountsReturnsEveryAccountWithScopes(t *testing.T) {
	db := newTestDB(t)
	repo := NewAccountRepository(db.DB)
	ctx := context.Background()
	_ = repo.CreateAccount(ctx, newTestAccount("acct_1", "u1"))
	_ = repo.CreateAccount(ctx, newTestAccount("acct_2", "u2"))

	list, err := repo.ListAccounts(ctx)
	if err != nil {
		t.Fatalf("ListAccounts() error = %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("ListAccounts() returned %d accounts, want 2", len(list))
	}
	for _, a := range list {
		if len(a.Scopes) != 1 {
			t.Errorf("account %s Scopes = %v, want 1 entry", a.ID, a.Scopes)
		}
	}
}

func TestCountAccountsScopedByProvider(t *testing.T) {
	db := newTestDB(t)
	repo := NewAccountRepository(db.DB)
	ctx := context.Background()
	_ = repo.CreateAccount(ctx, newTestAccount("acct_1", "u1"))

	count, err := repo.CountAccounts(ctx, account.ProviderTwitch)
	if err != nil || count != 1 {
		t.Fatalf("CountAccounts() = %d, %v, want 1", count, err)
	}
}

func TestNoTokenLikeColumnExistsOnConnectedAccounts(t *testing.T) {
	db := newTestDB(t)

	rows, err := db.DB.Query(`PRAGMA table_info(connected_accounts)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info failed: %v", err)
	}
	defer rows.Close()

	forbidden := map[string]bool{"access_token": true, "refresh_token": true, "device_code": true, "client_secret": true, "token": true}
	for rows.Next() {
		var (
			cid       int
			name, typ string
			notnull   int
			dflt      any
			pk        int
		)
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan table_info row failed: %v", err)
		}
		if forbidden[name] {
			t.Errorf("connected_accounts has a token-like column %q", name)
		}
	}
}
