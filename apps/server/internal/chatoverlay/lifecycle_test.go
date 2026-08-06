package chatoverlay

import (
	"testing"

	chatoverlaydomain "github.com/streaming-tree/server/internal/domain/chatoverlay"
	operatorchat "github.com/streaming-tree/server/internal/operatorchat"
)

func TestEvaluateRejectsItemThatFailsStaticFilters(t *testing.T) {
	cfg := testSettings(nil)
	visible, _ := evaluate(moderationItem("mod1", "acct_1"), cfg)
	if visible {
		t.Fatal("expected a moderation item to never be visible on the public overlay")
	}
}

func TestEvaluateDeletedMessageHiddenByDefault(t *testing.T) {
	cfg := testSettings(func(p *chatoverlaydomain.Profile) { p.ShowDeletedPlaceholder = false })
	visible, _ := evaluate(deletedMessageItem("m1", "acct_1", "u1", "viewer", "bad word"), cfg)
	if visible {
		t.Fatal("expected a deleted message to be removed from the public overlay when ShowDeletedPlaceholder is off")
	}
}

func TestEvaluateDeletedMessageShowsPlaceholderWhenEnabled(t *testing.T) {
	cfg := testSettings(func(p *chatoverlaydomain.Profile) { p.ShowDeletedPlaceholder = true })
	visible, out := evaluate(deletedMessageItem("m1", "acct_1", "u1", "viewer", "the original text"), cfg)
	if !visible {
		t.Fatal("expected a deleted message to remain visible as a placeholder when ShowDeletedPlaceholder is on")
	}
	if !out.Deleted {
		t.Error("expected Deleted to be true")
	}
	if out.Message != nil {
		t.Errorf("expected Message to be nil for a deleted placeholder, got %+v - the original text must never leak", out.Message)
	}
}

func TestBuildItemRespectsShowAvatarAndBadgesToggles(t *testing.T) {
	badges := []operatorchat.Badge{{SetID: "moderator", ID: "1"}}
	item := messageItemWithBadges("m1", "acct_1", "u1", "viewer", "hi", badges)
	item.User.AvatarURL = "https://static-cdn.jtvnw.net/avatar.png"

	off := testSettings(func(p *chatoverlaydomain.Profile) { p.ShowAvatar = false; p.ShowBadges = false })
	out := buildItem(item, off)
	if out.User.AvatarURL != "" {
		t.Error("expected AvatarURL to be empty when ShowAvatar is off")
	}
	if len(out.User.Badges) != 0 {
		t.Error("expected Badges to be empty when ShowBadges is off")
	}

	on := testSettings(func(p *chatoverlaydomain.Profile) { p.ShowAvatar = true; p.ShowBadges = true })
	out = buildItem(item, on)
	if out.User.AvatarURL == "" {
		t.Error("expected AvatarURL to be populated when ShowAvatar is on")
	}
	if len(out.User.Badges) != 1 {
		t.Error("expected Badges to be populated when ShowBadges is on")
	}
}

func TestBuildItemAccountLabelOnlyWhenSettingOnAndResolvable(t *testing.T) {
	item := messageItem("m1", "acct_1", "u1", "viewer", "hi")

	off := testSettings(func(p *chatoverlaydomain.Profile) { p.ShowAccountLabel = false })
	off.accountLabel = func(string) (string, bool) { return "Main Channel", true }
	if out := buildItem(item, off); out.AccountLabel != "" {
		t.Error("expected AccountLabel to be empty when ShowAccountLabel is off, even if resolvable")
	}

	on := testSettings(func(p *chatoverlaydomain.Profile) { p.ShowAccountLabel = true })
	on.accountLabel = func(string) (string, bool) { return "", false }
	if out := buildItem(item, on); out.AccountLabel != "" {
		t.Error("expected AccountLabel to stay empty when the lookup can't resolve a label")
	}

	on.accountLabel = func(string) (string, bool) { return "Main Channel", true }
	if out := buildItem(item, on); out.AccountLabel != "Main Channel" {
		t.Errorf("expected AccountLabel = 'Main Channel', got %q", out.AccountLabel)
	}
}

func TestBuildUserAnonymousOmitsIdentity(t *testing.T) {
	item := anonymousActivityItem("a1", "acct_1", "sub_gift")
	item.User = &operatorchat.User{Anonymous: true}
	out := buildUser(item.User, testSettings(nil))
	if !out.Anonymous {
		t.Error("expected Anonymous to be true")
	}
	if out.DisplayName != "" || out.ProviderUserID != "" {
		t.Errorf("expected no identity fields for an anonymous user, got %+v", out)
	}
}

func TestBuildUserRoleFlagsDerivedFromBadgesOnly(t *testing.T) {
	badges := []operatorchat.Badge{{SetID: "broadcaster"}, {SetID: "subscriber"}}
	item := messageItemWithBadges("m1", "acct_1", "u1", "streamer", "hi", badges)
	out := buildUser(item.User, testSettings(nil))

	if !out.IsBroadcaster || !out.IsSubscriber {
		t.Errorf("expected IsBroadcaster and IsSubscriber true from badges, got %+v", out)
	}
	if out.IsModerator || out.IsVIP {
		t.Errorf("expected IsModerator and IsVIP false when no such badge is present, got %+v", out)
	}
}

func TestBuildUserMissingRoleInfoNeverHighlights(t *testing.T) {
	item := messageItem("m1", "acct_1", "u1", "viewer", "hi") // no badges at all
	out := buildUser(item.User, testSettings(nil))
	if out.IsBroadcaster || out.IsModerator || out.IsSubscriber || out.IsVIP {
		t.Errorf("expected every role flag false when no badge info is present, got %+v", out)
	}
}

func TestBuildMessageFoldsCheermoteAndUnknownFragmentsToText(t *testing.T) {
	m := &operatorchat.Message{
		PlainText: "cheer100 gg",
		Fragments: []operatorchat.Fragment{
			{Type: operatorchat.FragmentCheermote, Text: "cheer100", CheermotePrefix: "cheer", CheermoteBits: 100},
			{Type: operatorchat.FragmentUnknown, Text: "??"},
			{Type: operatorchat.FragmentEmote, Text: "Kappa", EmoteID: "emote_1"},
			{Type: operatorchat.FragmentMention, Text: "@viewer"},
		},
	}
	out := buildMessage(m)
	if out.Fragments[0].Type != FragmentText || out.Fragments[0].Text != "cheer100" {
		t.Errorf("expected a cheermote fragment to fold to plain text, got %+v", out.Fragments[0])
	}
	if out.Fragments[1].Type != FragmentText {
		t.Errorf("expected an unknown fragment to fold to plain text, got %+v", out.Fragments[1])
	}
	if out.Fragments[2].Type != FragmentEmote || out.Fragments[2].EmoteID != "emote_1" {
		t.Errorf("expected the emote fragment to pass through with its EmoteID, got %+v", out.Fragments[2])
	}
	if out.Fragments[3].Type != FragmentMention {
		t.Errorf("expected the mention fragment to pass through, got %+v", out.Fragments[3])
	}
}
