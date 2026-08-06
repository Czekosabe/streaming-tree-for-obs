package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/chatoverlay"
)

func newTestOverlayProfile(id, slug, name string) chatoverlay.Profile {
	p := chatoverlay.Default(name)
	p.ID = id
	p.PublicSlug = slug
	return p
}

func TestChatOverlayCreateThenGetRoundTrips(t *testing.T) {
	db := newTestDB(t)
	repo := NewChatOverlayRepository(db.DB)

	p := newTestOverlayProfile("ov_1", "slug_1", "My Overlay")
	saved, err := repo.CreateProfile(context.Background(), p)
	if err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}
	if saved.ID != "ov_1" || saved.PublicSlug != "slug_1" || saved.Name != "My Overlay" {
		t.Fatalf("saved = %+v, want the created identity fields", saved)
	}
	if !saved.Enabled || saved.MaxVisibleItems != 30 || saved.FontFamily != chatoverlay.FontSansSerif {
		t.Errorf("saved = %+v, want the documented defaults", saved)
	}

	got, found, err := repo.GetProfile(context.Background(), "ov_1")
	if err != nil {
		t.Fatalf("GetProfile() error = %v", err)
	}
	if !found {
		t.Fatal("GetProfile() found = false, want true")
	}
	if got.Name != "My Overlay" {
		t.Errorf("got.Name = %q, want %q", got.Name, "My Overlay")
	}
}

func TestChatOverlayGetReturnsNotFoundWhenAbsent(t *testing.T) {
	db := newTestDB(t)
	repo := NewChatOverlayRepository(db.DB)

	_, found, err := repo.GetProfile(context.Background(), "ov_missing")
	if err != nil {
		t.Fatalf("GetProfile() error = %v", err)
	}
	if found {
		t.Error("GetProfile() found = true, want false")
	}
}

func TestChatOverlayGetByPublicSlug(t *testing.T) {
	db := newTestDB(t)
	repo := NewChatOverlayRepository(db.DB)

	if _, err := repo.CreateProfile(context.Background(), newTestOverlayProfile("ov_1", "slug_1", "x")); err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}

	got, found, err := repo.GetProfileByPublicSlug(context.Background(), "slug_1")
	if err != nil {
		t.Fatalf("GetProfileByPublicSlug() error = %v", err)
	}
	if !found || got.ID != "ov_1" {
		t.Fatalf("got = %+v, found = %v, want ov_1", got, found)
	}

	_, found, err = repo.GetProfileByPublicSlug(context.Background(), "does-not-exist")
	if err != nil {
		t.Fatalf("GetProfileByPublicSlug() error = %v", err)
	}
	if found {
		t.Error("expected found = false for an unknown slug")
	}
}

func TestChatOverlayPublicSlugMustBeUnique(t *testing.T) {
	db := newTestDB(t)
	repo := NewChatOverlayRepository(db.DB)

	if _, err := repo.CreateProfile(context.Background(), newTestOverlayProfile("ov_1", "same-slug", "a")); err != nil {
		t.Fatalf("first CreateProfile() error = %v", err)
	}
	if _, err := repo.CreateProfile(context.Background(), newTestOverlayProfile("ov_2", "same-slug", "b")); err == nil {
		t.Fatal("expected an error creating a second overlay with the same public slug")
	}
}

func TestChatOverlayListProfiles(t *testing.T) {
	db := newTestDB(t)
	repo := NewChatOverlayRepository(db.DB)

	repo.CreateProfile(context.Background(), newTestOverlayProfile("ov_1", "slug_1", "a"))
	repo.CreateProfile(context.Background(), newTestOverlayProfile("ov_2", "slug_2", "b"))

	list, err := repo.ListProfiles(context.Background())
	if err != nil {
		t.Fatalf("ListProfiles() error = %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("ListProfiles() = %d items, want 2", len(list))
	}
}

func TestChatOverlayUpdateReplacesEditableFieldsInPlace(t *testing.T) {
	db := newTestDB(t)
	repo := NewChatOverlayRepository(db.DB)

	saved, err := repo.CreateProfile(context.Background(), newTestOverlayProfile("ov_1", "slug_1", "Original"))
	if err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}

	updated := saved
	updated.Name = "Renamed"
	updated.MaxVisibleItems = 50
	updated.LayoutMode = chatoverlay.LayoutVertical
	updated.PublicSlug = "should-be-ignored" // UpdateProfile never changes the slug

	result, err := repo.UpdateProfile(context.Background(), updated)
	if err != nil {
		t.Fatalf("UpdateProfile() error = %v", err)
	}
	if result.Name != "Renamed" || result.MaxVisibleItems != 50 || result.LayoutMode != chatoverlay.LayoutVertical {
		t.Errorf("result = %+v, want the updated fields", result)
	}
	if result.PublicSlug != "slug_1" {
		t.Errorf("PublicSlug = %q, want unchanged %q (UpdateProfile must never touch it)", result.PublicSlug, "slug_1")
	}
	if result.ID != "ov_1" || !result.CreatedAt.Equal(saved.CreatedAt) {
		t.Errorf("id/createdAt must be unchanged, got %+v", result)
	}
}

func TestChatOverlayUpdateUnknownReturnsNotFound(t *testing.T) {
	db := newTestDB(t)
	repo := NewChatOverlayRepository(db.DB)

	p := newTestOverlayProfile("ov_missing", "slug_x", "x")
	if _, err := repo.UpdateProfile(context.Background(), p); err != chatoverlay.ErrNotFound {
		t.Errorf("UpdateProfile() error = %v, want ErrNotFound", err)
	}
}

func TestChatOverlayDelete(t *testing.T) {
	db := newTestDB(t)
	repo := NewChatOverlayRepository(db.DB)

	repo.CreateProfile(context.Background(), newTestOverlayProfile("ov_1", "slug_1", "x"))
	if err := repo.DeleteProfile(context.Background(), "ov_1"); err != nil {
		t.Fatalf("DeleteProfile() error = %v", err)
	}
	_, found, err := repo.GetProfile(context.Background(), "ov_1")
	if err != nil {
		t.Fatalf("GetProfile() error = %v", err)
	}
	if found {
		t.Error("profile still found after DeleteProfile()")
	}
}

func TestChatOverlayRotatePublicSlug(t *testing.T) {
	db := newTestDB(t)
	repo := NewChatOverlayRepository(db.DB)

	repo.CreateProfile(context.Background(), newTestOverlayProfile("ov_1", "old-slug", "x"))

	rotated, err := repo.RotatePublicSlug(context.Background(), "ov_1", "new-slug", time.Now().UTC())
	if err != nil {
		t.Fatalf("RotatePublicSlug() error = %v", err)
	}
	if rotated.PublicSlug != "new-slug" {
		t.Errorf("PublicSlug = %q, want new-slug", rotated.PublicSlug)
	}

	_, found, err := repo.GetProfileByPublicSlug(context.Background(), "old-slug")
	if err != nil {
		t.Fatalf("GetProfileByPublicSlug(old) error = %v", err)
	}
	if found {
		t.Error("the old public slug must stop resolving immediately after rotation")
	}

	got, found, err := repo.GetProfileByPublicSlug(context.Background(), "new-slug")
	if err != nil || !found || got.ID != "ov_1" {
		t.Fatalf("GetProfileByPublicSlug(new) = %+v, found=%v, err=%v", got, found, err)
	}
}

func TestChatOverlayAccountsRoundTripAndReplace(t *testing.T) {
	db := newTestDB(t)
	accounts := NewAccountRepository(db.DB)
	repo := NewChatOverlayRepository(db.DB)
	createTestAccount(t, accounts, "acct_1")
	createTestAccount(t, accounts, "acct_2")
	repo.CreateProfile(context.Background(), newTestOverlayProfile("ov_1", "slug_1", "x"))

	if err := repo.SetAccounts(context.Background(), "ov_1", []string{"acct_1", "acct_2"}); err != nil {
		t.Fatalf("SetAccounts() error = %v", err)
	}
	list, err := repo.ListAccounts(context.Background(), "ov_1")
	if err != nil {
		t.Fatalf("ListAccounts() error = %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("ListAccounts() = %v, want 2 entries", list)
	}

	// Replacing with a smaller set removes the rest.
	if err := repo.SetAccounts(context.Background(), "ov_1", []string{"acct_1"}); err != nil {
		t.Fatalf("second SetAccounts() error = %v", err)
	}
	list, err = repo.ListAccounts(context.Background(), "ov_1")
	if err != nil {
		t.Fatalf("ListAccounts() error = %v", err)
	}
	if len(list) != 1 || list[0] != "acct_1" {
		t.Fatalf("ListAccounts() = %v, want exactly [acct_1]", list)
	}
}

func TestChatOverlaySetAccountsRejectsUnknownAccount(t *testing.T) {
	db := newTestDB(t)
	repo := NewChatOverlayRepository(db.DB)
	repo.CreateProfile(context.Background(), newTestOverlayProfile("ov_1", "slug_1", "x"))

	err := repo.SetAccounts(context.Background(), "ov_1", []string{"acct_missing"})
	if err != chatoverlay.ErrAccountNotFound {
		t.Errorf("SetAccounts() error = %v, want ErrAccountNotFound", err)
	}
}

func TestChatOverlayAccountsCascadeOnProfileDelete(t *testing.T) {
	db := newTestDB(t)
	accounts := NewAccountRepository(db.DB)
	repo := NewChatOverlayRepository(db.DB)
	createTestAccount(t, accounts, "acct_1")
	repo.CreateProfile(context.Background(), newTestOverlayProfile("ov_1", "slug_1", "x"))
	repo.SetAccounts(context.Background(), "ov_1", []string{"acct_1"})

	if err := repo.DeleteProfile(context.Background(), "ov_1"); err != nil {
		t.Fatalf("DeleteProfile() error = %v", err)
	}

	var count int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM chat_overlay_accounts WHERE overlay_id = ?`, "ov_1").Scan(&count); err != nil {
		t.Fatalf("count query failed: %v", err)
	}
	if count != 0 {
		t.Errorf("chat_overlay_accounts rows survived profile deletion: %d", count)
	}
}

func TestChatOverlayHiddenUsersAddIsIdempotent(t *testing.T) {
	db := newTestDB(t)
	accounts := NewAccountRepository(db.DB)
	repo := NewChatOverlayRepository(db.DB)
	createTestAccount(t, accounts, "acct_1")
	repo.CreateProfile(context.Background(), newTestOverlayProfile("ov_1", "slug_1", "x"))
	now := time.Now().UTC()

	ref := chatoverlay.HiddenUser{OverlayID: "ov_1", ProviderID: chatoverlay.ProviderTwitch, ConnectedAccountID: "acct_1", ProviderUserID: "u1", Label: "spammer"}
	if _, err := repo.AddHiddenUser(context.Background(), ref, now); err != nil {
		t.Fatalf("first AddHiddenUser() error = %v", err)
	}
	if _, err := repo.AddHiddenUser(context.Background(), ref, now); err != nil {
		t.Fatalf("second AddHiddenUser() error = %v", err)
	}

	list, err := repo.ListHiddenUsers(context.Background(), "ov_1")
	if err != nil {
		t.Fatalf("ListHiddenUsers() error = %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListHiddenUsers() = %+v, want exactly one entry", list)
	}
}

func TestChatOverlayHiddenUserOnOneOverlayDoesNotAffectAnother(t *testing.T) {
	db := newTestDB(t)
	accounts := NewAccountRepository(db.DB)
	repo := NewChatOverlayRepository(db.DB)
	createTestAccount(t, accounts, "acct_1")
	repo.CreateProfile(context.Background(), newTestOverlayProfile("ov_1", "slug_1", "x"))
	repo.CreateProfile(context.Background(), newTestOverlayProfile("ov_2", "slug_2", "y"))

	ref := chatoverlay.HiddenUser{OverlayID: "ov_1", ProviderID: chatoverlay.ProviderTwitch, ConnectedAccountID: "acct_1", ProviderUserID: "u1"}
	if _, err := repo.AddHiddenUser(context.Background(), ref, time.Now().UTC()); err != nil {
		t.Fatalf("AddHiddenUser() error = %v", err)
	}

	list2, err := repo.ListHiddenUsers(context.Background(), "ov_2")
	if err != nil {
		t.Fatalf("ListHiddenUsers(ov_2) error = %v", err)
	}
	if len(list2) != 0 {
		t.Errorf("ov_2's hidden-user list = %+v, want empty - hiding on ov_1 must not affect ov_2", list2)
	}
}

func TestChatOverlayRemoveHiddenUser(t *testing.T) {
	db := newTestDB(t)
	accounts := NewAccountRepository(db.DB)
	repo := NewChatOverlayRepository(db.DB)
	createTestAccount(t, accounts, "acct_1")
	repo.CreateProfile(context.Background(), newTestOverlayProfile("ov_1", "slug_1", "x"))

	ref := chatoverlay.HiddenUser{OverlayID: "ov_1", ProviderID: chatoverlay.ProviderTwitch, ConnectedAccountID: "acct_1", ProviderUserID: "u1"}
	repo.AddHiddenUser(context.Background(), ref, time.Now().UTC())

	if err := repo.RemoveHiddenUser(context.Background(), "ov_1", chatoverlay.ProviderTwitch, "acct_1", "u1"); err != nil {
		t.Fatalf("RemoveHiddenUser() error = %v", err)
	}
	list, _ := repo.ListHiddenUsers(context.Background(), "ov_1")
	if len(list) != 0 {
		t.Errorf("ListHiddenUsers() after remove = %+v, want empty", list)
	}
}

func TestChatOverlayRemoveHiddenUserAbsentReturnsNotFound(t *testing.T) {
	db := newTestDB(t)
	repo := NewChatOverlayRepository(db.DB)
	repo.CreateProfile(context.Background(), newTestOverlayProfile("ov_1", "slug_1", "x"))

	err := repo.RemoveHiddenUser(context.Background(), "ov_1", chatoverlay.ProviderTwitch, "acct_1", "u_missing")
	if err != chatoverlay.ErrUserNotFound {
		t.Errorf("RemoveHiddenUser() error = %v, want ErrUserNotFound", err)
	}
}

func TestChatOverlayBlockedTermsAddIsIdempotentByNormalizedValue(t *testing.T) {
	db := newTestDB(t)
	repo := NewChatOverlayRepository(db.DB)
	repo.CreateProfile(context.Background(), newTestOverlayProfile("ov_1", "slug_1", "x"))
	now := time.Now().UTC()

	first, err := repo.AddBlockedTerm(context.Background(), chatoverlay.BlockedTerm{ID: "term_1", OverlayID: "ov_1", Value: "Spam", MatchMode: chatoverlay.MatchContains}, now)
	if err != nil {
		t.Fatalf("first AddBlockedTerm() error = %v", err)
	}
	second, err := repo.AddBlockedTerm(context.Background(), chatoverlay.BlockedTerm{ID: "term_2", OverlayID: "ov_1", Value: "  SPAM  ", MatchMode: chatoverlay.MatchWholeWord}, now)
	if err != nil {
		t.Fatalf("second AddBlockedTerm() error = %v", err)
	}
	if first.ID != second.ID {
		t.Errorf("AddBlockedTerm() was not idempotent by normalized value: %q vs %q", first.ID, second.ID)
	}

	list, err := repo.ListBlockedTerms(context.Background(), "ov_1")
	if err != nil {
		t.Fatalf("ListBlockedTerms() error = %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListBlockedTerms() = %+v, want exactly one entry", list)
	}
}

func TestChatOverlayBlockedTermsIndependentAcrossOverlays(t *testing.T) {
	db := newTestDB(t)
	repo := NewChatOverlayRepository(db.DB)
	repo.CreateProfile(context.Background(), newTestOverlayProfile("ov_1", "slug_1", "x"))
	repo.CreateProfile(context.Background(), newTestOverlayProfile("ov_2", "slug_2", "y"))

	if _, err := repo.AddBlockedTerm(context.Background(), chatoverlay.BlockedTerm{ID: "term_1", OverlayID: "ov_1", Value: "spam", MatchMode: chatoverlay.MatchContains}, time.Now().UTC()); err != nil {
		t.Fatalf("AddBlockedTerm() error = %v", err)
	}
	// The exact same term text is allowed on a different overlay - the
	// uniqueness constraint is scoped per overlay, not global.
	if _, err := repo.AddBlockedTerm(context.Background(), chatoverlay.BlockedTerm{ID: "term_2", OverlayID: "ov_2", Value: "spam", MatchMode: chatoverlay.MatchContains}, time.Now().UTC()); err != nil {
		t.Fatalf("AddBlockedTerm() on a different overlay unexpectedly failed: %v", err)
	}

	list1, _ := repo.ListBlockedTerms(context.Background(), "ov_1")
	list2, _ := repo.ListBlockedTerms(context.Background(), "ov_2")
	if len(list1) != 1 || len(list2) != 1 {
		t.Errorf("expected one term on each overlay independently, got %d and %d", len(list1), len(list2))
	}
}

func TestChatOverlayRemoveBlockedTerm(t *testing.T) {
	db := newTestDB(t)
	repo := NewChatOverlayRepository(db.DB)
	repo.CreateProfile(context.Background(), newTestOverlayProfile("ov_1", "slug_1", "x"))

	saved, err := repo.AddBlockedTerm(context.Background(), chatoverlay.BlockedTerm{ID: "term_1", OverlayID: "ov_1", Value: "spam", MatchMode: chatoverlay.MatchContains}, time.Now().UTC())
	if err != nil {
		t.Fatalf("AddBlockedTerm() error = %v", err)
	}
	if err := repo.RemoveBlockedTerm(context.Background(), "ov_1", saved.ID); err != nil {
		t.Fatalf("RemoveBlockedTerm() error = %v", err)
	}
	list, _ := repo.ListBlockedTerms(context.Background(), "ov_1")
	if len(list) != 0 {
		t.Errorf("ListBlockedTerms() after remove = %+v, want empty", list)
	}
}

func TestChatOverlayRemoveBlockedTermAbsentReturnsNotFound(t *testing.T) {
	db := newTestDB(t)
	repo := NewChatOverlayRepository(db.DB)
	repo.CreateProfile(context.Background(), newTestOverlayProfile("ov_1", "slug_1", "x"))

	err := repo.RemoveBlockedTerm(context.Background(), "ov_1", "term_missing")
	if err != chatoverlay.ErrTermNotFound {
		t.Errorf("RemoveBlockedTerm() error = %v, want ErrTermNotFound", err)
	}
}

func TestChatOverlayActivityTypesRoundTripAndReplace(t *testing.T) {
	db := newTestDB(t)
	repo := NewChatOverlayRepository(db.DB)
	repo.CreateProfile(context.Background(), newTestOverlayProfile("ov_1", "slug_1", "x"))

	if err := repo.SetActivityTypes(context.Background(), "ov_1", []string{"follow", "bits"}); err != nil {
		t.Fatalf("SetActivityTypes() error = %v", err)
	}
	list, err := repo.ListActivityTypes(context.Background(), "ov_1")
	if err != nil {
		t.Fatalf("ListActivityTypes() error = %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("ListActivityTypes() = %v, want 2 entries", list)
	}

	if err := repo.SetActivityTypes(context.Background(), "ov_1", []string{"raid"}); err != nil {
		t.Fatalf("second SetActivityTypes() error = %v", err)
	}
	list, err = repo.ListActivityTypes(context.Background(), "ov_1")
	if err != nil {
		t.Fatalf("ListActivityTypes() error = %v", err)
	}
	if len(list) != 1 || list[0] != "raid" {
		t.Fatalf("ListActivityTypes() = %v, want exactly [raid]", list)
	}
}

func TestChatOverlayNoMessageContentOrTokenColumnExists(t *testing.T) {
	db := newTestDB(t)

	rows, err := db.DB.Query(`SELECT name FROM pragma_table_info('chat_overlays')`)
	if err != nil {
		t.Fatalf("pragma_table_info query failed: %v", err)
	}
	defer rows.Close()

	forbidden := map[string]bool{
		"message": true, "text": true, "content": true, "token": true,
		"access_token": true, "refresh_token": true, "session_id": true,
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan failed: %v", err)
		}
		if forbidden[name] {
			t.Errorf("chat_overlays unexpectedly has a column named %q", name)
		}
	}
}
