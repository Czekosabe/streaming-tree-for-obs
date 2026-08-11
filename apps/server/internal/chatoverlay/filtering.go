package chatoverlay

import (
	"strings"
	"unicode"

	chatoverlaydomain "github.com/streaming-tree/server/internal/domain/chatoverlay"
	operatorchat "github.com/streaming-tree/server/internal/operatorchat"
)

// DefaultCommandPrefix mirrors Stage 9's own fixed command prefix
// (operator-chat-presentation.ts's DEFAULT_COMMAND_PREFIX) - a trimmed
// message starting with this is a command, never an arbitrary substring
// match.
const DefaultCommandPrefix = "!"

// userKey builds the same composite identity key used for both hidden-
// user and bot-user lookups: provider + connected account + provider
// user id. Two different overlays, or Stage 9's own separate operator
// list, never share a lookup map - each Projection builds its own from
// its own settings.
func userKey(providerID, connectedAccountID, providerUserID string) string {
	return providerID + "|" + connectedAccountID + "|" + providerUserID
}

// AccountLabelLookup resolves a connected account's display label, when
// known - mirrors operatorchat.DestinationLookup's own reasoning: a plain
// callback so this package never imports internal/domain/account.
type AccountLabelLookup func(connectedAccountID string) (label string, ok bool)

// resolvedSettings is everything one overlay's filtering and item
// construction needs, computed once per rebuild (profile save, account-
// selection change, hidden-user/blocked-term/activity-type change, or the
// shared Stage 9 bot-user list changing) rather than re-read per item.
type resolvedSettings struct {
	profile chatoverlaydomain.Profile

	// accountIDs is the resolved selection: nil/empty means "all
	// currently available accounts" (Part 4's own documented default).
	accountIDs map[string]struct{}
	// hiddenUsers is this overlay's own list (chat_overlay_hidden_users) -
	// deliberately separate from Stage 9's operator list.
	hiddenUsers map[string]struct{}
	// botUsers is Stage 9's shared, explicit bot-user classification
	// (operatorchatprefs) - never inferred from a username, and never a
	// second, overlay-specific bot list.
	botUsers map[string]struct{}
	// activityTypes is the resolved selection: nil/empty means "every
	// activity type."
	activityTypes map[string]struct{}
	blockedTerms  []chatoverlaydomain.BlockedTerm

	accountLabel AccountLabelLookup

	// designDataNeeds is non-nil only when this overlay currently has a
	// saved visual design (Stage 13B, docs/visual-designs.md §22) -
	// derived once per rebuild from that design's own layers, never
	// from a legacy show/hide toggle. buildUser/buildItem in
	// lifecycle.go populate an optional field whenever *either* the
	// legacy toggle *or* this assessment says to, so an active design's
	// layers are never silently starved of data. Never used to bypass
	// filtering - passesStaticFilters above always runs first.
	designDataNeeds *chatoverlaydomain.ChatDataNeeds
}

func toSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(values))
	for _, v := range values {
		out[v] = struct{}{}
	}
	return out
}

// passesStaticFilters reports whether item would ever be shown by this
// overlay right now, ignoring the specific deleted/placeholder text
// question (handled by the caller in lifecycle.go) - account selection,
// item kind, activity-type selection, hidden-user, bot, and command
// filtering, and blocked terms.
func passesStaticFilters(item operatorchat.Item, cfg resolvedSettings) bool {
	if item.Kind != operatorchat.KindMessage && item.Kind != operatorchat.KindActivity {
		// Moderation/system rows are operator-only diagnostic content -
		// never reach a public overlay, in any configuration.
		return false
	}

	if len(cfg.accountIDs) > 0 {
		if _, ok := cfg.accountIDs[item.ConnectedAccountID]; !ok {
			return false
		}
	}

	if item.Kind == operatorchat.KindActivity {
		if !cfg.profile.ShowActivityEvents {
			return false
		}
		if item.Activity != nil && len(cfg.activityTypes) > 0 {
			if _, ok := cfg.activityTypes[item.Activity.ActivityType]; !ok {
				return false
			}
		}
	}

	if item.User != nil && !item.User.Anonymous && item.User.ProviderUserID != "" {
		key := userKey(item.ProviderID, item.ConnectedAccountID, item.User.ProviderUserID)
		if _, hidden := cfg.hiddenUsers[key]; hidden {
			return false
		}
		if cfg.profile.HideBots {
			if _, isBot := cfg.botUsers[key]; isBot {
				return false
			}
		}
	}

	if item.Kind == operatorchat.KindMessage && item.Message != nil {
		if cfg.profile.HideCommands && isCommandMessage(item.Message.PlainText) {
			return false
		}
		if matchesAnyBlockedTerm(item.Message.PlainText, cfg.blockedTerms) {
			return false
		}
	}

	return true
}

// isCommandMessage mirrors the frontend's own isCommandMessage exactly:
// the trimmed text begins with the fixed command prefix - never an
// arbitrary substring search.
func isCommandMessage(plainText string) bool {
	return strings.HasPrefix(strings.TrimSpace(plainText), DefaultCommandPrefix)
}

// matchesAnyBlockedTerm reports whether text should be hidden in full -
// see the Stage 10 task's Part 7: literal matching only, Unicode-aware
// case folding, no regular expression, no partial censoring.
func matchesAnyBlockedTerm(text string, terms []chatoverlaydomain.BlockedTerm) bool {
	if len(terms) == 0 {
		return false
	}
	normalizedText := chatoverlaydomain.NormalizeTerm(text)
	for _, term := range terms {
		normalizedTerm := chatoverlaydomain.NormalizeTerm(term.Value)
		if normalizedTerm == "" {
			continue
		}
		switch term.MatchMode {
		case chatoverlaydomain.MatchContains:
			if strings.Contains(normalizedText, normalizedTerm) {
				return true
			}
		case chatoverlaydomain.MatchWholeWord:
			if containsWholeWord(normalizedText, normalizedTerm) {
				return true
			}
		}
	}
	return false
}

// containsWholeWord reports whether term appears in text bounded by
// non-letter/digit runes (or the string's own edges) on both sides - a
// simple, documented Unicode word-boundary approximation (unicode.IsLetter
// / unicode.IsDigit), not a full Unicode text-segmentation algorithm. Both
// text and term are expected to already be normalized by the caller.
func containsWholeWord(text, term string) bool {
	if term == "" {
		return false
	}
	runes := []rune(text)
	termRunes := []rune(term)
	for start := 0; start+len(termRunes) <= len(runes); start++ {
		match := true
		for i, r := range termRunes {
			if runes[start+i] != r {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		beforeOK := start == 0 || !isWordRune(runes[start-1])
		afterIdx := start + len(termRunes)
		afterOK := afterIdx >= len(runes) || !isWordRune(runes[afterIdx])
		if beforeOK && afterOK {
			return true
		}
	}
	return false
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// Twitch's own documented chat-badge set-id vocabulary (see
// docs/provider-integrations/twitch-engagement.md's Stage 9 addendum) -
// used only to derive a role-highlight flag from a badge already present
// on the normalized item, never to infer a role from a username. A
// future second provider would need its own equivalent mapping; this
// package still never imports internal/provider/twitch itself.
const (
	badgeSetBroadcaster = "broadcaster"
	badgeSetModerator   = "moderator"
	badgeSetSubscriber  = "subscriber"
	badgeSetVIP         = "vip"
)

func hasBadgeSet(badges []operatorchat.Badge, setID string) bool {
	for _, b := range badges {
		if b.SetID == setID {
			return true
		}
	}
	return false
}
