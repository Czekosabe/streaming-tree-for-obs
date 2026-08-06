// Package chatoverlay holds the Stage 10 persisted chat-overlay-profile
// model: presentation settings, the account selection, the overlay's own
// hidden-user list, and blocked terms - everything an operator configures
// once and expects to survive a restart.
//
// Deliberately does not hold anything about live chat content or runtime
// projection state - see internal/chatoverlay (a different package) for
// the in-memory public-overlay projection this profile drives. Mirrors
// internal/domain/operatorchatprefs's own reasoning: a persisted
// preference and the transient state it configures are never the same
// storage.
package chatoverlay

import (
	"strings"
	"time"
)

// LayoutMode is the overlay's stream-orientation layout.
type LayoutMode string

const (
	LayoutHorizontal LayoutMode = "horizontal"
	LayoutVertical   LayoutMode = "vertical"
)

// StackDirection is which end of the visible list new items enter from.
type StackDirection string

const (
	StackTopDown  StackDirection = "top_down"
	StackBottomUp StackDirection = "bottom_up"
)

// HorizontalAlignment is the text/item alignment within the overlay.
type HorizontalAlignment string

const (
	AlignLeft   HorizontalAlignment = "left"
	AlignCenter HorizontalAlignment = "center"
	AlignRight  HorizontalAlignment = "right"
)

// FontFamily is a fixed, safe system-font-stack allowlist - never an
// arbitrary font-family string, remote font URL, or uploaded font file
// (see the Stage 10 task's Part 11).
type FontFamily string

const (
	FontSansSerif FontFamily = "sans_serif"
	FontSerif     FontFamily = "serif"
	FontMonospace FontFamily = "monospace"
	FontRounded   FontFamily = "rounded"
)

// UsernameColorMode chooses between the chat provider's own reported user
// color and a single fixed color for every username.
type UsernameColorMode string

const (
	UsernameColorProvider UsernameColorMode = "provider"
	UsernameColorFixed    UsernameColorMode = "fixed"
)

// Animation is a fixed entry/exit animation choice - never an arbitrary
// CSS keyframe or easing expression.
type Animation string

const (
	AnimationNone      Animation = "none"
	AnimationFade      Animation = "fade"
	AnimationSlideUp   Animation = "slide_up"
	AnimationSlideLeft Animation = "slide_left"
	AnimationScale     Animation = "scale"
)

// Language is the profile's own documented canonical UI language for its
// generic strings (a deleted-message placeholder, and similar) - never
// derived silently from whoever last edited the profile. Chat text and
// usernames are always rendered verbatim regardless of this setting.
type Language string

const (
	LanguageEnglish Language = "en"
	LanguagePolish  Language = "pl"
)

// Profile is one persisted chat-overlay configuration.
type Profile struct {
	ID         string
	PublicSlug string
	Name       string
	Enabled    bool

	LayoutMode          LayoutMode
	StackDirection      StackDirection
	HorizontalAlignment HorizontalAlignment

	ShowPlatformIcon       bool
	ShowPlatformName       bool
	ShowAccountLabel       bool
	ShowAvatar             bool
	ShowBadges             bool
	ShowTimestamp          bool
	ShowActivityEvents     bool
	ShowDeletedPlaceholder bool
	HideCommands           bool
	HideBots               bool

	// MaxVisibleItems bounds how many items the public overlay ever shows
	// at once - see internal/chatoverlay's own expiry/capacity scheduler.
	MaxVisibleItems int
	// MessageLifetimeSeconds is how long an item stays visible before
	// timed expiry. 0 means no timed expiry - capacity and moderation
	// still remove items.
	MessageLifetimeSeconds int

	FontFamily        FontFamily
	FontSize          int
	FontWeight        int
	LineHeight        float64
	TextColor         string
	UsernameColorMode UsernameColorMode
	BubbleColor       string
	BubbleOpacity     float64
	BorderRadius      int
	ItemSpacing       int
	TextOutline       bool
	TextShadow        bool

	EntryAnimation      Animation
	ExitAnimation       Animation
	AnimationDurationMS int

	HighlightBroadcaster bool
	HighlightModerators  bool
	HighlightSubscribers bool
	HighlightVIPs        bool

	Language Language

	CreatedAt time.Time
	UpdatedAt time.Time
}

// Default returns the documented, safe out-of-the-box settings for a
// newly created profile - matching migration 0011's own column defaults
// exactly, kept in sync by a dedicated test.
func Default(name string) Profile {
	return Profile{
		Name:                name,
		Enabled:             true,
		LayoutMode:          LayoutHorizontal,
		StackDirection:      StackBottomUp,
		HorizontalAlignment: AlignLeft,

		ShowPlatformIcon:       true,
		ShowPlatformName:       false,
		ShowAccountLabel:       false,
		ShowAvatar:             false,
		ShowBadges:             true,
		ShowTimestamp:          false,
		ShowActivityEvents:     true,
		ShowDeletedPlaceholder: false,
		HideCommands:           true,
		HideBots:               true,

		MaxVisibleItems:        30,
		MessageLifetimeSeconds: 0,

		FontFamily:        FontSansSerif,
		FontSize:          16,
		FontWeight:        400,
		LineHeight:        1.4,
		TextColor:         "#FFFFFF",
		UsernameColorMode: UsernameColorProvider,
		BubbleColor:       "#000000",
		BubbleOpacity:     0.45,
		BorderRadius:      8,
		ItemSpacing:       6,
		TextOutline:       true,
		TextShadow:        false,

		EntryAnimation:      AnimationFade,
		ExitAnimation:       AnimationFade,
		AnimationDurationMS: 250,

		HighlightBroadcaster: true,
		HighlightModerators:  true,
		HighlightSubscribers: false,
		HighlightVIPs:        false,

		Language: LanguageEnglish,
	}
}

// ProviderID identifies which provider a hidden-user entry is scoped to -
// deliberately its own type, mirroring operatorchatprefs.ProviderID's own
// doc comment on why no provider-id type is shared across domains here.
type ProviderID string

const ProviderTwitch ProviderID = "twitch"

// HiddenUser identifies one user hidden from this overlay's public
// output, by the provider's own stable user id - never a display name or
// login. Deliberately separate from Stage 9's operator hidden-user list:
// a user may remain visible to the operator while being hidden from the
// public overlay, and vice versa.
type HiddenUser struct {
	OverlayID          string
	ProviderID         ProviderID
	ConnectedAccountID string
	ProviderUserID     string
	Label              string
	CreatedAt          time.Time
}

// MatchMode is how a blocked term is matched against message text - safe
// literal matching only, never a regular expression, glob, or executable
// expression.
type MatchMode string

const (
	MatchContains  MatchMode = "contains"
	MatchWholeWord MatchMode = "whole_word"
)

// BlockedTerm is one literal term that hides a matching public message in
// full - see internal/chatoverlay/filtering.go for the matching semantics.
type BlockedTerm struct {
	ID        string
	OverlayID string
	Value     string
	MatchMode MatchMode
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Maximum bounds for blocked terms - see the Stage 10 task's Part 7:
// "conservative maximum length, bounded terms per overlay."
const (
	MaxBlockedTermLength      = 64
	MaxBlockedTermsPerOverlay = 100
)

// NormalizeTerm is the Unicode-aware case-folding + whitespace-trimming
// used both for blocked-term storage uniqueness (this package) and for
// matching a term against message text at filtering time
// (internal/chatoverlay/filtering.go) - the same function in both places
// so "what is stored" and "what is matched" can never silently disagree.
// Uses Go's own rune-level case folding (strings.ToLower, itself
// Unicode-aware via unicode.ToLower) rather than a full Unicode
// case-folding table, which needs no additional dependency and is
// documented here as the deliberate, good-enough choice for this stage.
func NormalizeTerm(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
