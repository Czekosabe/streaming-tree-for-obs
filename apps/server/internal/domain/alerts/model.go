// Package alerts holds the Stage 12A persisted alert-profile and
// alert-rule definitions: an operator-configured OBS Browser Source
// destination (profile) and the rules that decide which normalized
// engagement events become an alert on it - user-authored configuration,
// mirroring internal/domain/chatoverlay's and internal/domain/chatautomation's
// own split between "what is configured" (this package) and "what is
// happening right now" (the sibling runtime package internal/alerts,
// which never persists a queued alert, a current-alert snapshot, a
// replay slot, or a counter).
//
// Deliberately does not hold anything about a real matched event, a
// rendered alert instance, or playback state - see internal/alerts for
// the in-memory matcher and queue that read these definitions and
// consume the normalized Engagement Event Bus. This package never
// imports internal/domain/engagement, internal/provider/twitch, or any
// other domain package's concrete types - it declares its own narrow,
// primitive-typed ProviderID/EventType, exactly like every other domain
// package in this project (see internal/domain/operatorchatprefs's own
// ProviderID doc comment for the same reasoning: no provider-id or
// event-type type is shared across domain packages here).
package alerts

import "time"

// ProviderID identifies which provider a rule's provider filter or an
// account's provider is. Deliberately its own type, never
// engagement.ProviderID/account.ProviderID/platform.ProviderID.
type ProviderID string

// ProviderTwitch and ProviderYouTube are the providers alerts support
// (Stage 12A and Stage 15A respectively).
const (
	ProviderTwitch  ProviderID = "twitch"
	ProviderYouTube ProviderID = "youtube"
)

// EventType is the closed set of normalized engagement event types an
// alert rule may match. Deliberately mirrors, as plain string literals,
// the real internal/domain/engagement.Type constants this application's
// Twitch connector actually produces today - not the larger,
// aspirational list in docs/engagement-architecture.md §9.1. Adding a
// new EventType here requires adding a matching entry to
// internal/alerts/capability.go's own capability table.
type EventType string

const (
	EventFollow                 EventType = "follow"
	EventSubscription           EventType = "subscription"
	EventResubscription         EventType = "resubscription"
	EventGiftedSubscription     EventType = "gifted_subscription"
	EventSubscriptionGiftBatch  EventType = "subscription_gift_batch"
	EventBits                   EventType = "bits"
	EventRaid                   EventType = "raid"
	EventChannelPointRedemption EventType = "channel_point_redemption"

	// Stage 15A (YouTube). EventGiftedSubscription/EventSubscriptionGiftBatch
	// above are deliberately reused for YouTube's giftMembershipReceived/
	// membershipGifting events too - see internal/domain/engagement's own
	// mapping (docs/provider-integrations/youtube-engagement.md §5) for
	// the reasoning; only the two genuinely new concepts get new types
	// here.
	EventYouTubeMembership          EventType = "youtube_membership"
	EventYouTubeMembershipMilestone EventType = "youtube_membership_milestone"
	EventYouTubeSuperChat           EventType = "youtube_super_chat"
	EventYouTubeSuperSticker        EventType = "youtube_super_sticker"
)

// ValidEventTypes lists every accepted EventType, in the order shown by
// the frontend's event-type selector.
var ValidEventTypes = []EventType{
	EventFollow, EventSubscription, EventResubscription,
	EventGiftedSubscription, EventSubscriptionGiftBatch,
	EventBits, EventRaid, EventChannelPointRedemption,
	EventYouTubeMembership, EventYouTubeMembershipMilestone,
	EventYouTubeSuperChat, EventYouTubeSuperSticker,
}

func (t EventType) valid() bool {
	for _, v := range ValidEventTypes {
		if t == v {
			return true
		}
	}
	return false
}

// Role is the closed, fixed role-condition enum. Reserved: no Stage 12A
// event type currently supplies role data (see
// internal/alerts/capability.go), so a rule's RequiredRole is currently
// only ever accepted as RoleEveryone - see ValidateRuleConditions.
type Role string

const (
	RoleEveryone    Role = "everyone"
	RoleSubscriber  Role = "subscriber"
	RoleVIP         Role = "vip"
	RoleModerator   Role = "moderator"
	RoleBroadcaster Role = "broadcaster"
)

// ValidRoles lists every accepted Role value.
var ValidRoles = []Role{RoleEveryone, RoleSubscriber, RoleVIP, RoleModerator, RoleBroadcaster}

func (r Role) valid() bool {
	for _, v := range ValidRoles {
		if r == v {
			return true
		}
	}
	return false
}

// Theme is a profile's fixed visual family - never arbitrary CSS or a
// designer output (Stage 13's own concern).
type Theme string

const (
	ThemeMinimal Theme = "minimal"
	ThemeCompact Theme = "compact"
	ThemeLarge   Theme = "large"
)

// Position is where inside the Browser Source viewport an alert appears.
type Position string

const (
	PositionTop    Position = "top"
	PositionCenter Position = "center"
	PositionBottom Position = "bottom"
)

// TextAlign is the alert text's horizontal alignment.
type TextAlign string

const (
	AlignLeft   TextAlign = "left"
	AlignCenter TextAlign = "center"
	AlignRight  TextAlign = "right"
)

// Animation is the closed, application-owned animation-class enum -
// never an arbitrary CSS class or easing string (Stage 12A task Part 25).
type Animation string

const (
	AnimationNone      Animation = "none"
	AnimationFade      Animation = "fade"
	AnimationSlideUp   Animation = "slide_up"
	AnimationSlideLeft Animation = "slide_left"
	AnimationScale     Animation = "scale"
)

var validAnimations = []Animation{AnimationNone, AnimationFade, AnimationSlideUp, AnimationSlideLeft, AnimationScale}

func (a Animation) valid() bool {
	for _, v := range validAnimations {
		if a == v {
			return true
		}
	}
	return false
}

// Language is a profile's explicit, stored built-in-presentation
// language - never inferred from whoever last edited it (Stage 12A task
// Part 44).
type Language string

const (
	LanguageEnglish Language = "en"
	LanguagePolish  Language = "pl"
)

func (l Language) valid() bool { return l == LanguageEnglish || l == LanguagePolish }

// InterruptMode is the closed, fixed enum for Stage 12B mid-alert
// preemption (Stage 12B task Part 16): whether THIS rule's own newly
// matched alert may interrupt whatever is currently playing. Deliberately
// not a priority formula or a condition expression - only ever "never" or
// "a strictly higher priority may interrupt." Default InterruptNever
// preserves every Stage 12A rule's non-preempting behavior exactly.
type InterruptMode string

const (
	InterruptNever         InterruptMode = "never"
	InterruptLowerPriority InterruptMode = "lower_priority"
)

var validInterruptModes = []InterruptMode{InterruptNever, InterruptLowerPriority}

func (m InterruptMode) valid() bool {
	for _, v := range validInterruptModes {
		if m == v {
			return true
		}
	}
	return false
}

var validThemes = []Theme{ThemeMinimal, ThemeCompact, ThemeLarge}
var validPositions = []Position{PositionTop, PositionCenter, PositionBottom}
var validTextAligns = []TextAlign{AlignLeft, AlignCenter, AlignRight}

func (t Theme) valid() bool {
	for _, v := range validThemes {
		if t == v {
			return true
		}
	}
	return false
}

func (p Position) valid() bool {
	for _, v := range validPositions {
		if p == v {
			return true
		}
	}
	return false
}

func (a TextAlign) valid() bool {
	for _, v := range validTextAligns {
		if a == v {
			return true
		}
	}
	return false
}

// Profile is one persisted alert-output profile: an independent OBS
// Browser Source destination with its own public URL, queue bounds, and
// fixed presentation choices.
type Profile struct {
	ID         string
	PublicSlug string
	Name       string
	Enabled    bool

	Language Language

	Theme     Theme
	Position  Position
	TextAlign TextAlign

	MaxQueueItems          int
	MaximumQueueAgeSeconds int

	CreatedAt time.Time
	UpdatedAt time.Time
}

// Rule is one persisted alert-rule definition, belonging to exactly one
// profile.
type Rule struct {
	ID        string
	ProfileID string
	Name      string
	Enabled   bool

	EventType  EventType
	Priority   int
	DurationMS int

	// MinimumQuantity/MaximumQuantity are nil when unbounded on that
	// side. Both inclusive when set - see ValidateThresholds.
	MinimumQuantity *int64
	MaximumQuantity *int64

	// Currency/MinimumAmountMicros/MaximumAmountMicros: Stage 15A's
	// monetary condition (Super Chat/Super Sticker). Currency is required
	// whenever either threshold is set - an amount threshold with no
	// currency is meaningless, since this application never converts
	// between currencies (see ValidateMoneyThresholds). Both thresholds
	// are nil when unbounded on that side, both inclusive when set,
	// always integer micros - never a float.
	Currency            string
	MinimumAmountMicros *int64
	MaximumAmountMicros *int64

	RequiredRole Role

	ShowPlatform bool
	ShowUsername bool
	ShowMessage  bool
	ShowQuantity bool
	// ShowAmount: Stage 15A - only meaningful when Capability.HasAmount is
	// true for this rule's own EventType (Super Chat/Super Sticker).
	ShowAmount bool

	TextTemplate string

	EntryAnimation      Animation
	ExitAnimation       Animation
	AnimationDurationMS int

	// Providers/Accounts filters: empty means "any" - see
	// ValidateRuleConditions and the Stage 12A task's own Part 5.
	Providers []ProviderID
	Accounts  []string

	// AllowGrouping/GroupWindowMS: Stage 12B bounded alert grouping
	// (Stage 12B task Part 3-13). AllowGrouping may only be true when
	// GroupingCapabilityFor(EventType).Groupable is true - see
	// ValidateRuleConditions. GroupWindowMS is always validated within
	// [MinGroupWindowMS, MaxGroupWindowMS] regardless of AllowGrouping
	// (a single unconditional bound is simpler and safer than a
	// cross-column SQLite CHECK - the value is simply inert while
	// grouping is disabled).
	AllowGrouping bool
	GroupWindowMS int

	// InterruptMode/Interruptible: Stage 12B deterministic mid-alert
	// preemption (Stage 12B task Part 16-17). InterruptMode governs this
	// rule's own alert when it is the *incoming* candidate; Interruptible
	// governs this rule's own alert when it is the one *currently
	// playing*. Both are independent - a rule may be interruptible
	// without itself ever interrupting anything, and vice versa.
	InterruptMode InterruptMode
	Interruptible bool

	CreatedAt time.Time
	UpdatedAt time.Time
}

// DefaultProfile returns a new profile's safe, validated defaults plus
// the caller's chosen name. ID and PublicSlug are filled by the Service.
func DefaultProfile(name string) Profile {
	return Profile{
		Name:                   name,
		Enabled:                true,
		Language:               LanguageEnglish,
		Theme:                  ThemeMinimal,
		Position:               PositionBottom,
		TextAlign:              AlignCenter,
		MaxQueueItems:          DefaultMaxQueueItems,
		MaximumQueueAgeSeconds: DefaultMaximumQueueAgeSeconds,
	}
}
