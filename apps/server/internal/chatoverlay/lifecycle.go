package chatoverlay

import (
	operatorchat "github.com/streaming-tree/server/internal/operatorchat"
)

// evaluate decides whether item should currently be visible on this
// overlay and, if so, builds its public representation. It never
// consults or mutates a Projection's own prior state - see
// Projection.handleUpstreamItem in projection.go for how the result is
// turned into an upsert/remove/skip decision against what was visible
// before.
func evaluate(item operatorchat.Item, cfg resolvedSettings) (visible bool, out Item) {
	if !passesStaticFilters(item, cfg) {
		return false, Item{}
	}

	if item.Kind == operatorchat.KindMessage && item.Lifecycle.Deleted {
		if !cfg.profile.ShowDeletedPlaceholder {
			return false, Item{}
		}
		return true, buildDeletedPlaceholder(item, cfg)
	}

	return true, buildItem(item, cfg)
}

func buildItem(item operatorchat.Item, cfg resolvedSettings) Item {
	out := Item{
		ID:              item.ID,
		ProviderID:      item.ProviderID,
		SourceAccountID: item.ConnectedAccountID,
		OccurredAt:      item.OccurredAt,
		Synthetic:       item.Synthetic,
	}

	needsAccountLabel := cfg.profile.ShowAccountLabel || (cfg.designDataNeeds != nil && cfg.designDataNeeds.AccountLabel)
	if needsAccountLabel && cfg.accountLabel != nil {
		if label, ok := cfg.accountLabel(item.ConnectedAccountID); ok {
			out.AccountLabel = label
		}
	}

	if item.User != nil {
		out.User = buildUser(item.User, cfg)
	}

	switch item.Kind {
	case operatorchat.KindMessage:
		out.Kind = KindMessage
		if item.Message != nil {
			out.Message = buildMessage(item.Message)
		}
	case operatorchat.KindActivity:
		out.Kind = KindActivity
		if item.Activity != nil {
			out.Activity = &Activity{
				ActivityType:  item.Activity.ActivityType,
				AmountMicros:  item.Activity.AmountMicros,
				Currency:      item.Activity.Currency,
				DisplayAmount: item.Activity.DisplayAmount,
				Quantity:      item.Activity.Quantity,
			}
		}
	}

	return out
}

// buildDeletedPlaceholder builds the public item for a deleted message
// whose overlay shows a placeholder - never the original text, per the
// Stage 10 task's own hard requirement.
func buildDeletedPlaceholder(item operatorchat.Item, cfg resolvedSettings) Item {
	out := buildItem(item, cfg)
	out.Message = nil
	out.Deleted = true
	return out
}

// deletionRemoveReason maps operator-chat's own lifecycle deletion
// reason onto this package's public RemoveReason - used only when a
// previously-visible item stops passing evaluate() because its
// Lifecycle.Deleted flag just became true (see Projection.
// applyUpstreamItem's "wasVisible" branch). Every other field on an
// operator-chat Item is fixed at creation (see operatorchat.Item's own
// doc comment), so a live update to an already-visible item's
// visibility can only ever be this - never a filter/settings change,
// which always goes through Configure's full rebuild instead. Falls
// back to RemoveReasonUnknown (immediate, never cosmetic) for a
// deletion reason this package does not recognize, rather than
// guessing.
func deletionRemoveReason(item operatorchat.Item) RemoveReason {
	switch item.Lifecycle.DeletionReason {
	case operatorchat.DeletionReasonModeratorDeleted:
		return RemoveReasonMessageDeleted
	case operatorchat.DeletionReasonChatCleared:
		return RemoveReasonChatCleared
	case operatorchat.DeletionReasonUserMessagesCleared:
		return RemoveReasonUserMessagesCleared
	default:
		return RemoveReasonUnknown
	}
}

func buildUser(u *operatorchat.User, cfg resolvedSettings) *User {
	out := &User{Anonymous: u.Anonymous}
	if u.Anonymous {
		return out
	}

	out.ProviderUserID = u.ProviderUserID
	out.DisplayName = u.DisplayName
	if out.DisplayName == "" {
		out.DisplayName = u.Login
	}
	out.Color = u.Color

	needsAvatar := cfg.profile.ShowAvatar || (cfg.designDataNeeds != nil && cfg.designDataNeeds.Avatar)
	if needsAvatar {
		out.AvatarURL = u.AvatarURL
	}
	needsBadges := cfg.profile.ShowBadges || (cfg.designDataNeeds != nil && cfg.designDataNeeds.Badges)
	if needsBadges {
		out.Badges = make([]Badge, 0, len(u.Badges))
		for _, b := range u.Badges {
			out.Badges = append(out.Badges, Badge{SetID: b.SetID, ID: b.ID, Info: b.Info})
		}
	}

	out.IsBroadcaster = hasBadgeSet(u.Badges, badgeSetBroadcaster)
	out.IsModerator = hasBadgeSet(u.Badges, badgeSetModerator)
	out.IsSubscriber = hasBadgeSet(u.Badges, badgeSetSubscriber)
	out.IsVIP = hasBadgeSet(u.Badges, badgeSetVIP)

	return out
}

func buildMessage(m *operatorchat.Message) *Message {
	out := &Message{PlainText: m.PlainText, Fragments: make([]Fragment, 0, len(m.Fragments))}
	for _, f := range m.Fragments {
		switch f.Type {
		case operatorchat.FragmentEmote:
			out.Fragments = append(out.Fragments, Fragment{Type: FragmentEmote, Text: f.Text, EmoteID: f.EmoteID})
		case operatorchat.FragmentMention:
			out.Fragments = append(out.Fragments, Fragment{Type: FragmentMention, Text: f.Text})
		default:
			// Cheermote and unknown fragments fold to plain text - see
			// this package's own FragmentType doc comment.
			out.Fragments = append(out.Fragments, Fragment{Type: FragmentText, Text: f.Text})
		}
	}
	return out
}
