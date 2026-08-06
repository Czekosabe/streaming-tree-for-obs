package chatoverlay

import (
	"testing"

	chatoverlaydomain "github.com/streaming-tree/server/internal/domain/chatoverlay"
	operatorchat "github.com/streaming-tree/server/internal/operatorchat"
)

func TestPassesStaticFiltersDefaultAllowsAllAccounts(t *testing.T) {
	cfg := testSettings(nil)
	item := messageItem("m1", "acct_1", "u1", "viewer", "hello")
	if !passesStaticFilters(item, cfg) {
		t.Fatal("expected default settings to allow a message from any account")
	}
}

func TestPassesStaticFiltersRejectsModerationAndSystemKinds(t *testing.T) {
	cfg := testSettings(nil)
	if passesStaticFilters(moderationItem("mod1", "acct_1"), cfg) {
		t.Error("expected a moderation-kind item to never pass, even with permissive settings")
	}
	if passesStaticFilters(systemItem("sys1", "acct_1"), cfg) {
		t.Error("expected a system-kind item to never pass, even with permissive settings")
	}
}

func TestPassesStaticFiltersAccountSelectionRestrictsToChosenAccounts(t *testing.T) {
	cfg := testSettings(nil)
	cfg.accountIDs = toSet([]string{"acct_1"})

	if !passesStaticFilters(messageItem("m1", "acct_1", "u1", "viewer", "hello"), cfg) {
		t.Error("expected a message from a selected account to pass")
	}
	if passesStaticFilters(messageItem("m2", "acct_2", "u1", "viewer", "hello"), cfg) {
		t.Error("expected a message from a non-selected account to be rejected")
	}
}

func TestPassesStaticFiltersActivityRequiresShowActivityEvents(t *testing.T) {
	cfg := testSettings(func(p *chatoverlaydomain.Profile) { p.ShowActivityEvents = false })
	if passesStaticFilters(anonymousActivityItem("a1", "acct_1", "follow"), cfg) {
		t.Error("expected an activity item to be rejected when ShowActivityEvents is off")
	}
}

func TestPassesStaticFiltersActivityTypeSelectionRestricts(t *testing.T) {
	cfg := testSettings(nil)
	cfg.activityTypes = toSet([]string{"follow"})

	if !passesStaticFilters(anonymousActivityItem("a1", "acct_1", "follow"), cfg) {
		t.Error("expected a selected activity type to pass")
	}
	if passesStaticFilters(anonymousActivityItem("a2", "acct_1", "bits"), cfg) {
		t.Error("expected a non-selected activity type to be rejected")
	}
}

func TestPassesStaticFiltersHiddenUserRejected(t *testing.T) {
	cfg := testSettings(nil)
	cfg.hiddenUsers = toSet([]string{userKey("twitch", "acct_1", "u1")})

	if passesStaticFilters(messageItem("m1", "acct_1", "u1", "viewer", "hi"), cfg) {
		t.Error("expected a message from a hidden user to be rejected")
	}
	if !passesStaticFilters(messageItem("m2", "acct_1", "u2", "other", "hi"), cfg) {
		t.Error("expected a message from a different user to still pass")
	}
}

func TestPassesStaticFiltersHiddenUserOnOneOverlayDoesNotAffectAnother(t *testing.T) {
	hiddenOnA := testSettings(nil)
	hiddenOnA.hiddenUsers = toSet([]string{userKey("twitch", "acct_1", "u1")})
	visibleOnB := testSettings(nil) // its own, separate, empty hidden-user list

	item := messageItem("m1", "acct_1", "u1", "viewer", "hi")
	if passesStaticFilters(item, hiddenOnA) {
		t.Error("expected overlay A to reject the hidden user")
	}
	if !passesStaticFilters(item, visibleOnB) {
		t.Error("expected overlay B's own, separate hidden-user list to leave the user visible")
	}
}

func TestPassesStaticFiltersBotRejectedWhenHideBotsOn(t *testing.T) {
	cfg := testSettings(func(p *chatoverlaydomain.Profile) { p.HideBots = true })
	cfg.botUsers = toSet([]string{userKey("twitch", "acct_1", "u1")})

	if passesStaticFilters(messageItem("m1", "acct_1", "u1", "botaccount", "hi"), cfg) {
		t.Error("expected a classified bot user's message to be rejected when HideBots is on")
	}
}

func TestPassesStaticFiltersBotKeptWhenHideBotsOff(t *testing.T) {
	cfg := testSettings(func(p *chatoverlaydomain.Profile) { p.HideBots = false })
	cfg.botUsers = toSet([]string{userKey("twitch", "acct_1", "u1")})

	if !passesStaticFilters(messageItem("m1", "acct_1", "u1", "botaccount", "hi"), cfg) {
		t.Error("expected a classified bot user's message to pass when HideBots is off")
	}
}

func TestPassesStaticFiltersNeverInfersBotFromUsername(t *testing.T) {
	cfg := testSettings(func(p *chatoverlaydomain.Profile) { p.HideBots = true })
	// No entry in botUsers at all - even a username that looks bot-like must
	// still pass, since classification is explicit-only (Part 7).
	if !passesStaticFilters(messageItem("m1", "acct_1", "u1", "nightbot", "hi"), cfg) {
		t.Error("expected an unclassified user to pass regardless of how its username looks")
	}
}

func TestPassesStaticFiltersCommandRejectedWhenHideCommandsOn(t *testing.T) {
	cfg := testSettings(func(p *chatoverlaydomain.Profile) { p.HideCommands = true })
	if passesStaticFilters(messageItem("m1", "acct_1", "u1", "viewer", "!uptime"), cfg) {
		t.Error("expected a command message to be rejected when HideCommands is on")
	}
	if !passesStaticFilters(messageItem("m2", "acct_1", "u1", "viewer", "not a command"), cfg) {
		t.Error("expected an ordinary message to still pass when HideCommands is on")
	}
}

func TestPassesStaticFiltersCommandKeptWhenHideCommandsOff(t *testing.T) {
	cfg := testSettings(func(p *chatoverlaydomain.Profile) { p.HideCommands = false })
	if !passesStaticFilters(messageItem("m1", "acct_1", "u1", "viewer", "!uptime"), cfg) {
		t.Error("expected a command message to pass when HideCommands is off")
	}
}

func TestIsCommandMessageRequiresFixedPrefix(t *testing.T) {
	if !isCommandMessage("!uptime") {
		t.Error("expected a leading '!' to be a command")
	}
	if isCommandMessage("hello !uptime") {
		t.Error("expected the prefix to only match at the start of the (trimmed) message")
	}
	if !isCommandMessage("  !uptime") {
		t.Error("expected leading whitespace to be trimmed before checking the prefix")
	}
}

func TestMatchesAnyBlockedTermContains(t *testing.T) {
	terms := []chatoverlaydomain.BlockedTerm{{Value: "spam", MatchMode: chatoverlaydomain.MatchContains}}
	if !matchesAnyBlockedTerm("this is SPAM content", terms) {
		t.Error("expected a case-insensitive substring match")
	}
	if matchesAnyBlockedTerm("clean message", terms) {
		t.Error("expected no match on unrelated text")
	}
}

func TestMatchesAnyBlockedTermWholeWordDoesNotMatchSubstring(t *testing.T) {
	terms := []chatoverlaydomain.BlockedTerm{{Value: "cat", MatchMode: chatoverlaydomain.MatchWholeWord}}
	if matchesAnyBlockedTerm("concatenate this", terms) {
		t.Error("expected whole-word matching to not match 'cat' inside 'concatenate'")
	}
	if !matchesAnyBlockedTerm("I have a cat", terms) {
		t.Error("expected whole-word matching to match a standalone word")
	}
	if !matchesAnyBlockedTerm("cat!", terms) {
		t.Error("expected whole-word matching to match at a punctuation boundary")
	}
}

func TestMatchesAnyBlockedTermUnicodeCaseFolding(t *testing.T) {
	terms := []chatoverlaydomain.BlockedTerm{{Value: "śpam", MatchMode: chatoverlaydomain.MatchContains}}
	if !matchesAnyBlockedTerm("to jest ŚPAM", terms) {
		t.Error("expected Unicode-aware case folding to match Polish ŚPAM against śpam")
	}
}

func TestMatchesAnyBlockedTermSimilarButNonMatchingTextIsRetained(t *testing.T) {
	terms := []chatoverlaydomain.BlockedTerm{{Value: "spammer", MatchMode: chatoverlaydomain.MatchContains}}
	if matchesAnyBlockedTerm("spam", terms) {
		t.Error("expected a shorter, non-matching similar word to be retained (term is the substring being searched, not vice versa)")
	}
}

func TestMatchesAnyBlockedTermEmptyListNeverMatches(t *testing.T) {
	if matchesAnyBlockedTerm("anything at all", nil) {
		t.Error("expected no blocked terms to mean nothing is ever hidden")
	}
}

func TestHasBadgeSetMissingReturnsFalse(t *testing.T) {
	if hasBadgeSet(nil, badgeSetModerator) {
		t.Error("expected no badges to mean no role match")
	}
	badges := []operatorchat.Badge{{SetID: "subscriber", ID: "12"}}
	if hasBadgeSet(badges, badgeSetModerator) {
		t.Error("expected an unrelated badge to not match a different set id")
	}
}

func TestHasBadgeSetPresent(t *testing.T) {
	badges := []operatorchat.Badge{{SetID: "moderator", ID: "1"}}
	if !hasBadgeSet(badges, badgeSetModerator) {
		t.Error("expected a matching badge set id to be found")
	}
}
