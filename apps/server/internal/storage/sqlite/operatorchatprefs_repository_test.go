package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/operatorchatprefs"
)

func TestOperatorChatPrefsGetReturnsNotFoundWhenAbsent(t *testing.T) {
	db := newTestDB(t)
	repo := NewOperatorChatPrefsRepository(db.DB)

	_, found, err := repo.GetPreferences(context.Background())
	if err != nil {
		t.Fatalf("GetPreferences() error = %v", err)
	}
	if found {
		t.Error("GetPreferences() found = true, want false before anything is set")
	}
}

func TestOperatorChatPrefsSetThenGetRoundTrips(t *testing.T) {
	db := newTestDB(t)
	repo := NewOperatorChatPrefsRepository(db.DB)
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)

	want := operatorchatprefs.Preferences{
		ShowPlatformIcon: true, ShowPlatformName: true, ShowAccountLabel: false,
		ShowBadges: false, ShowTimestamps: true, ShowActivityEvents: false,
		ShowDeletedMessages: false, HideCommandMessages: true, CompactMode: true,
	}
	saved, err := repo.SetPreferences(context.Background(), want, now)
	if err != nil {
		t.Fatalf("SetPreferences() error = %v", err)
	}
	if saved.ShowPlatformIcon != want.ShowPlatformIcon || saved.ShowPlatformName != want.ShowPlatformName ||
		saved.ShowAccountLabel != want.ShowAccountLabel || saved.ShowBadges != want.ShowBadges ||
		saved.ShowTimestamps != want.ShowTimestamps || saved.ShowActivityEvents != want.ShowActivityEvents ||
		saved.ShowDeletedMessages != want.ShowDeletedMessages || saved.HideCommandMessages != want.HideCommandMessages ||
		saved.CompactMode != want.CompactMode {
		t.Errorf("SetPreferences() = %+v, want the fields from %+v", saved, want)
	}

	got, found, err := repo.GetPreferences(context.Background())
	if err != nil {
		t.Fatalf("GetPreferences() error = %v", err)
	}
	if !found {
		t.Fatal("GetPreferences() found = false, want true after Set")
	}
	if got.ShowPlatformName != true || got.CompactMode != true || got.ShowAccountLabel != false {
		t.Errorf("GetPreferences() = %+v, want the round-tripped values", got)
	}
}

func TestOperatorChatPrefsSetReplacesTheSingletonRowInPlace(t *testing.T) {
	db := newTestDB(t)
	repo := NewOperatorChatPrefsRepository(db.DB)
	now := time.Now().UTC()

	if _, err := repo.SetPreferences(context.Background(), operatorchatprefs.Preferences{CompactMode: false}, now); err != nil {
		t.Fatalf("first SetPreferences() error = %v", err)
	}
	if _, err := repo.SetPreferences(context.Background(), operatorchatprefs.Preferences{CompactMode: true}, now); err != nil {
		t.Fatalf("second SetPreferences() error = %v", err)
	}

	got, found, err := repo.GetPreferences(context.Background())
	if err != nil || !found {
		t.Fatalf("GetPreferences() error = %v, found = %v", err, found)
	}
	if !got.CompactMode {
		t.Error("CompactMode = false, want the replaced value true")
	}

	var count int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM operator_chat_preferences`).Scan(&count); err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	if count != 1 {
		t.Errorf("row count = %d, want exactly 1 (a singleton, replaced not duplicated)", count)
	}
}

func TestOperatorChatAccountVisibilitySetRejectsAnUnknownAccount(t *testing.T) {
	db := newTestDB(t)
	repo := NewOperatorChatPrefsRepository(db.DB)

	_, err := repo.SetAccountVisibility(context.Background(), "acct_does_not_exist", false, time.Now().UTC())
	if err == nil {
		t.Fatal("SetAccountVisibility() error = nil, want ErrAccountNotFound for an unknown account")
	}
	if err != operatorchatprefs.ErrAccountNotFound {
		t.Errorf("SetAccountVisibility() error = %v, want ErrAccountNotFound", err)
	}
}

func TestOperatorChatAccountVisibilityRoundTripsAndLists(t *testing.T) {
	db := newTestDB(t)
	accounts := NewAccountRepository(db.DB)
	repo := NewOperatorChatPrefsRepository(db.DB)
	createTestAccount(t, accounts, "acct_ocv_1")
	createTestAccount(t, accounts, "acct_ocv_2")
	now := time.Now().UTC()

	if _, err := repo.SetAccountVisibility(context.Background(), "acct_ocv_1", false, now); err != nil {
		t.Fatalf("SetAccountVisibility() error = %v", err)
	}

	list, err := repo.ListAccountVisibility(context.Background())
	if err != nil {
		t.Fatalf("ListAccountVisibility() error = %v", err)
	}
	if len(list) != 1 || list[0].AccountID != "acct_ocv_1" || list[0].Visible {
		t.Errorf("ListAccountVisibility() = %+v, want exactly one hidden acct_ocv_1", list)
	}
}

func TestOperatorChatAccountVisibilityCascadesWhenAccountIsDeleted(t *testing.T) {
	db := newTestDB(t)
	accounts := NewAccountRepository(db.DB)
	repo := NewOperatorChatPrefsRepository(db.DB)
	createTestAccount(t, accounts, "acct_ocv_3")

	if _, err := repo.SetAccountVisibility(context.Background(), "acct_ocv_3", false, time.Now().UTC()); err != nil {
		t.Fatalf("SetAccountVisibility() error = %v", err)
	}
	if err := accounts.DeleteAccount(context.Background(), "acct_ocv_3"); err != nil {
		t.Fatalf("DeleteAccount() error = %v", err)
	}

	list, err := repo.ListAccountVisibility(context.Background())
	if err != nil {
		t.Fatalf("ListAccountVisibility() error = %v", err)
	}
	if len(list) != 0 {
		t.Errorf("ListAccountVisibility() = %+v, want empty after the account cascaded away", list)
	}
}

func TestOperatorChatHiddenUsersAddIsIdempotent(t *testing.T) {
	db := newTestDB(t)
	accounts := NewAccountRepository(db.DB)
	repo := NewOperatorChatPrefsRepository(db.DB)
	createTestAccount(t, accounts, "acct_hu_1")
	now := time.Now().UTC()

	ref := operatorchatprefs.UserRef{
		ID: "ocu_1", ProviderID: operatorchatprefs.ProviderTwitch,
		ConnectedAccountID: "acct_hu_1", ProviderUserID: "u1", Label: "spammer",
	}
	first, err := repo.AddHiddenUser(context.Background(), ref, now)
	if err != nil {
		t.Fatalf("first AddHiddenUser() error = %v", err)
	}

	// A second add with a different generated id but the same identity
	// tuple must return the existing entry, not create a duplicate.
	second, err := repo.AddHiddenUser(context.Background(), operatorchatprefs.UserRef{
		ID: "ocu_2", ProviderID: operatorchatprefs.ProviderTwitch,
		ConnectedAccountID: "acct_hu_1", ProviderUserID: "u1", Label: "different label",
	}, now)
	if err != nil {
		t.Fatalf("second AddHiddenUser() error = %v", err)
	}
	if first.ID != second.ID {
		t.Errorf("AddHiddenUser() was not idempotent: first ID %q, second ID %q", first.ID, second.ID)
	}

	list, err := repo.ListHiddenUsers(context.Background())
	if err != nil {
		t.Fatalf("ListHiddenUsers() error = %v", err)
	}
	if len(list) != 1 {
		t.Errorf("ListHiddenUsers() = %+v, want exactly one entry", list)
	}
}

func TestOperatorChatHiddenUsersRemove(t *testing.T) {
	db := newTestDB(t)
	accounts := NewAccountRepository(db.DB)
	repo := NewOperatorChatPrefsRepository(db.DB)
	createTestAccount(t, accounts, "acct_hu_2")

	saved, err := repo.AddHiddenUser(context.Background(), operatorchatprefs.UserRef{
		ID: "ocu_3", ProviderID: operatorchatprefs.ProviderTwitch,
		ConnectedAccountID: "acct_hu_2", ProviderUserID: "u2",
	}, time.Now().UTC())
	if err != nil {
		t.Fatalf("AddHiddenUser() error = %v", err)
	}

	if err := repo.RemoveHiddenUser(context.Background(), saved.ID); err != nil {
		t.Fatalf("RemoveHiddenUser() error = %v", err)
	}

	list, err := repo.ListHiddenUsers(context.Background())
	if err != nil {
		t.Fatalf("ListHiddenUsers() error = %v", err)
	}
	if len(list) != 0 {
		t.Errorf("ListHiddenUsers() after remove = %+v, want empty", list)
	}
}

func TestOperatorChatHiddenUsersRemoveAbsentReturnsNotFound(t *testing.T) {
	db := newTestDB(t)
	repo := NewOperatorChatPrefsRepository(db.DB)

	err := repo.RemoveHiddenUser(context.Background(), "ocu_does_not_exist")
	if err != operatorchatprefs.ErrUserNotFound {
		t.Errorf("RemoveHiddenUser() error = %v, want ErrUserNotFound", err)
	}
}

func TestOperatorChatHiddenUsersCascadeWhenAccountIsDeleted(t *testing.T) {
	db := newTestDB(t)
	accounts := NewAccountRepository(db.DB)
	repo := NewOperatorChatPrefsRepository(db.DB)
	createTestAccount(t, accounts, "acct_hu_3")

	if _, err := repo.AddHiddenUser(context.Background(), operatorchatprefs.UserRef{
		ID: "ocu_4", ProviderID: operatorchatprefs.ProviderTwitch,
		ConnectedAccountID: "acct_hu_3", ProviderUserID: "u3",
	}, time.Now().UTC()); err != nil {
		t.Fatalf("AddHiddenUser() error = %v", err)
	}

	if err := accounts.DeleteAccount(context.Background(), "acct_hu_3"); err != nil {
		t.Fatalf("DeleteAccount() error = %v", err)
	}

	list, err := repo.ListHiddenUsers(context.Background())
	if err != nil {
		t.Fatalf("ListHiddenUsers() error = %v", err)
	}
	if len(list) != 0 {
		t.Errorf("ListHiddenUsers() = %+v, want empty after the account cascaded away", list)
	}
}

func TestOperatorChatBotUsersRoundTrip(t *testing.T) {
	db := newTestDB(t)
	accounts := NewAccountRepository(db.DB)
	repo := NewOperatorChatPrefsRepository(db.DB)
	createTestAccount(t, accounts, "acct_bu_1")

	saved, err := repo.AddBotUser(context.Background(), operatorchatprefs.UserRef{
		ID: "ocu_5", ProviderID: operatorchatprefs.ProviderTwitch,
		ConnectedAccountID: "acct_bu_1", ProviderUserID: "u4", Label: "StreamElements",
	}, time.Now().UTC())
	if err != nil {
		t.Fatalf("AddBotUser() error = %v", err)
	}
	if saved.Label != "StreamElements" {
		t.Errorf("Label = %q, want StreamElements", saved.Label)
	}

	list, err := repo.ListBotUsers(context.Background())
	if err != nil {
		t.Fatalf("ListBotUsers() error = %v", err)
	}
	if len(list) != 1 || list[0].ProviderUserID != "u4" {
		t.Errorf("ListBotUsers() = %+v, want exactly one entry for u4", list)
	}

	if err := repo.RemoveBotUser(context.Background(), saved.ID); err != nil {
		t.Fatalf("RemoveBotUser() error = %v", err)
	}
	list, err = repo.ListBotUsers(context.Background())
	if err != nil {
		t.Fatalf("ListBotUsers() error = %v", err)
	}
	if len(list) != 0 {
		t.Errorf("ListBotUsers() after remove = %+v, want empty", list)
	}
}

func TestOperatorChatHiddenAndBotListsAreIndependent(t *testing.T) {
	db := newTestDB(t)
	accounts := NewAccountRepository(db.DB)
	repo := NewOperatorChatPrefsRepository(db.DB)
	createTestAccount(t, accounts, "acct_indep")
	now := time.Now().UTC()

	ref := operatorchatprefs.UserRef{
		ID: "ocu_6", ProviderID: operatorchatprefs.ProviderTwitch,
		ConnectedAccountID: "acct_indep", ProviderUserID: "u5",
	}
	if _, err := repo.AddHiddenUser(context.Background(), ref, now); err != nil {
		t.Fatalf("AddHiddenUser() error = %v", err)
	}

	hidden, err := repo.ListHiddenUsers(context.Background())
	if err != nil {
		t.Fatalf("ListHiddenUsers() error = %v", err)
	}
	bots, err := repo.ListBotUsers(context.Background())
	if err != nil {
		t.Fatalf("ListBotUsers() error = %v", err)
	}
	if len(hidden) != 1 {
		t.Errorf("ListHiddenUsers() = %+v, want one entry", hidden)
	}
	if len(bots) != 0 {
		t.Errorf("ListBotUsers() = %+v, want empty - hiding a user must not mark them a bot", bots)
	}
}
