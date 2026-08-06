// Package operatorchatprefs holds the Stage 9 unified-operator-chat
// preferences that survive a restart: presentation toggles, per-account
// chat visibility, and operator-maintained hidden-user/bot-user lists.
//
// Deliberately minimal, mirroring internal/domain/engagementsettings's own
// reasoning: everything about live chat content - message text, lifecycle
// state, badges, fragments - lives only in the in-memory operator-chat
// projection (internal/operatorchat) and is gone on restart. Nothing in
// this package ever holds a message, a display name treated as
// authoritative identity, a token, or a raw provider event.
package operatorchatprefs

import "time"

// ProviderID identifies which provider a hidden/bot user entry is scoped
// to. Deliberately its own type rather than reusing account.ProviderID or
// engagement.ProviderID - see those types' own doc comments for why no
// provider-id type is shared across domains in this codebase.
type ProviderID string

// ProviderTwitch is the only provider this stage's operator chat supports.
const ProviderTwitch ProviderID = "twitch"

// Preferences is the singleton set of operator-chat presentation toggles.
// See docs/progress.md's Stage 9 entry for the documented defaults.
type Preferences struct {
	ShowPlatformIcon    bool
	ShowPlatformName    bool
	ShowAccountLabel    bool
	ShowBadges          bool
	ShowTimestamps      bool
	ShowActivityEvents  bool
	ShowDeletedMessages bool
	HideCommandMessages bool
	CompactMode         bool
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// Default returns the documented out-of-the-box preferences: platform icon
// shown, textual platform name off (one provider today, so the icon alone
// is enough), account label shown (there may be more than one connected
// account), badges/timestamps/activity events/deleted messages shown,
// commands shown, compact mode off.
func Default() Preferences {
	return Preferences{
		ShowPlatformIcon:    true,
		ShowPlatformName:    false,
		ShowAccountLabel:    true,
		ShowBadges:          true,
		ShowTimestamps:      true,
		ShowActivityEvents:  true,
		ShowDeletedMessages: true,
		HideCommandMessages: false,
		CompactMode:         false,
	}
}

// AccountVisibility is one connected account's chat visibility. Absent
// means visible - a row is only written once an operator turns an account
// off, mirroring engagementsettings's absent-row-means-default convention.
type AccountVisibility struct {
	AccountID string
	Visible   bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// UserRef identifies one operator-maintained hidden-user or bot-user list
// entry, by the provider's own stable user id - never by display name or
// login, both of which a user can change.
type UserRef struct {
	ID                 string
	ProviderID         ProviderID
	ConnectedAccountID string
	ProviderUserID     string
	Label              string
	CreatedAt          time.Time
}
