// Package goals holds the Stage 18A persisted goal/counter and public
// widget-profile definitions: an operator-configured, persistent
// accumulation target (goal) and the public OBS Browser Source
// presentations of it (widget profile) - user-authored configuration and
// accumulated state, mirroring internal/domain/alerts's own split between
// "what is configured/accumulated" (this package) and "what is happening
// right now" (the sibling runtime package internal/goals, which owns the
// Event Bus subscription and never itself persists anything).
//
// Deliberately does not hold anything about a real matched
// engagement.Event or live SSE projection state - see internal/goals for
// that. This package never imports internal/domain/engagement,
// internal/provider/twitch, or any other domain package's concrete
// types - it declares its own narrow, primitive-typed ProviderID/Type,
// exactly like internal/domain/alerts, internal/domain/chatoverlay, and
// internal/domain/chatautomation already do (see docs/goals-widgets.md
// §3 for the full reasoning, quoting internal/domain/alerts/model.go's
// own identical rule). See docs/goals-widgets.md for the full Stage 18A
// contract this package implements.
package goals

import "time"

// ProviderID identifies which provider a goal's provider filter or an
// account/source belongs to. Deliberately its own type, never
// engagement.ProviderID/alerts.ProviderID/account.ProviderID.
type ProviderID string

const (
	ProviderTwitch         ProviderID = "twitch"
	ProviderYouTube        ProviderID = "youtube"
	ProviderStreamElements ProviderID = "streamelements"
)

// ValidProviderIDs lists every accepted ProviderID.
var ValidProviderIDs = []ProviderID{ProviderTwitch, ProviderYouTube, ProviderStreamElements}

func (p ProviderID) valid() bool {
	for _, v := range ValidProviderIDs {
		if v == p {
			return true
		}
	}
	return false
}

// Type is the closed set of normalized engagement event types the
// contribution table (capability.go) recognizes. Deliberately mirrors,
// as plain string literals, the real internal/domain/engagement.Type
// constants this application's connectors actually produce today - see
// docs/goals-widgets.md §3. Adding a new Type here requires adding a
// matching entry to capability.go's own ContributionFor table.
type Type string

const (
	TypeFollow                     Type = "follow"
	TypeSubscription               Type = "subscription"
	TypeResubscription             Type = "resubscription"
	TypeGiftedSubscription         Type = "gifted_subscription"
	TypeSubscriptionGiftBatch      Type = "subscription_gift_batch"
	TypeBits                       Type = "bits"
	TypeYouTubeMembership          Type = "youtube.membership"
	TypeYouTubeMembershipMilestone Type = "youtube.membership_milestone"
	TypeYouTubeSuperChat           Type = "youtube.super_chat"
	TypeYouTubeSuperSticker        Type = "youtube.super_sticker"
	TypeDonation                   Type = "donation"
)

// Kind is the closed set of goal kinds Stage 18A supports (docs/
// goals-widgets.md §2). No arbitrary formula language, no expression
// parser, no user scripting - a future kind is rejected by validation
// until explicitly added here.
type Kind string

const (
	KindFollowers     Kind = "followers"
	KindSubscriptions Kind = "subscriptions"
	KindDonations     Kind = "donations"
	KindBits          Kind = "bits"
)

// ValidKinds lists every accepted Kind, in the order shown by validation
// error messages and any UI that enumerates the whole set.
var ValidKinds = []Kind{KindFollowers, KindSubscriptions, KindDonations, KindBits}

func (k Kind) valid() bool {
	for _, v := range ValidKinds {
		if v == k {
			return true
		}
	}
	return false
}

// RequiresCurrency reports whether k is a monetary goal kind (docs/
// goals-widgets.md §6: exactly one configured currency, required).
func (k Kind) RequiresCurrency() bool {
	return k == KindDonations
}

// FontFamily is a fixed, safe system-font-stack allowlist - never an
// arbitrary font-family string, remote font URL, or uploaded font file.
// Deliberately this package's own small closed type, mirroring
// internal/domain/chatoverlay.FontFamily's identical closed-list
// precedent, rather than importing that unrelated domain (docs/
// goals-widgets.md §18).
type FontFamily string

const (
	FontSansSerif FontFamily = "sans_serif"
	FontSerif     FontFamily = "serif"
	FontMonospace FontFamily = "monospace"
	FontRounded   FontFamily = "rounded"
)

var validFontFamilies = []FontFamily{FontSansSerif, FontSerif, FontMonospace, FontRounded}

func (f FontFamily) valid() bool {
	for _, v := range validFontFamilies {
		if v == f {
			return true
		}
	}
	return false
}

// Orientation is the widget's stream-orientation layout.
type Orientation string

const (
	OrientationHorizontal Orientation = "horizontal"
	OrientationVertical   Orientation = "vertical"
)

func (o Orientation) valid() bool {
	return o == OrientationHorizontal || o == OrientationVertical
}

// TextAlign is the widget's text/item alignment.
type TextAlign string

const (
	AlignLeft   TextAlign = "left"
	AlignCenter TextAlign = "center"
	AlignRight  TextAlign = "right"
)

func (a TextAlign) valid() bool {
	return a == AlignLeft || a == AlignCenter || a == AlignRight
}

// clock returns the current time; injected so tests are deterministic.
type Clock func() time.Time
