package alerts

// Capability declares, for one EventType, which conditions and
// presentation toggles are semantically meaningful - the answer to the
// Stage 12A task's own Part 6: "do not create one generic rule form
// whose every threshold appears valid for every event."
//
// This table was built by reading the real Twitch normalization code
// (internal/provider/twitch/eventsub_normalize.go) rather than the
// aspirational event list in docs/engagement-architecture.md §9 - see
// docs/progress.md's Stage 12A persistence entry for the exact
// per-function citation. It is deliberately conservative: a field is
// only marked available here if a real, already-shipped normalization
// path can populate it.
type Capability struct {
	// HasUser: the event carries a User (though the user may still be
	// Anonymous - see HasAnonymity). Every Stage 12A event type has this.
	HasUser bool
	// HasMessage: the event's Message can carry real text (a resub
	// message, a cheer message, a redemption's user-input text) - not
	// necessarily always populated, but a real field a template's
	// {message} placeholder can resolve.
	HasMessage bool
	// HasQuantity: the event's Quantity is a real, meaningful number
	// (bits amount, gift count, raid viewer count) - never a reward's
	// point cost or a subscription tier, which live only in
	// ProviderExtra and are not exposed as a threshold-able quantity in
	// Stage 12A.
	HasQuantity bool
	// HasAmount is false for every Stage 12A (Twitch) event type - no
	// Twitch normalization path populates a real Money value (Stage 12A
	// task Part 7: "do not invent a money representation"). Stage 15A's
	// YouTube Super Chat/Super Sticker are the first two event types with
	// HasAmount: true - a real, provider-reported monetary value a rule
	// may threshold on (ValidateMoneyThresholds) and a template may show
	// via {amount}/{currency} (internal/alerts/templates.go).
	HasAmount bool
	// HasMembershipLevel: the event carries a real provider-reported
	// membership/level name, available via the {membershipLevel}
	// placeholder - Stage 15A's YouTube membership-family events only.
	HasMembershipLevel bool
	// HasRoles is always false for every Stage 12A event type: none of
	// these 8 normalized activity events populates
	// engagement.User.Roles (only chat.message-adjacent code paths ever
	// would, and even those do not today) - see RequiredRole's own doc
	// comment on Rule.
	HasRoles bool
	// HasAnonymity: the event has a real anonymous-actor path (Twitch's
	// anonymous gifter/cheerer concept) where User.Anonymous may be
	// true and every other User field is empty.
	HasAnonymity bool
	// HasRewardTitle: the event carries a reward title, available only
	// via the {rewardTitle} placeholder (channel_point_redemption's
	// ProviderExtra["rewardTitle"]) - the one deliberately named
	// ProviderExtra exception the Stage 12A task's own Part 10 allows.
	HasRewardTitle bool
}

// Capabilities maps every valid EventType to its real capability. Keep
// in sync with ValidEventTypes and internal/alerts' own event-type to
// engagement.Type conversion.
var Capabilities = map[EventType]Capability{
	EventFollow:                 {HasUser: true},
	EventSubscription:           {HasUser: true},
	EventResubscription:         {HasUser: true, HasMessage: true},
	EventGiftedSubscription:     {HasUser: true},
	EventSubscriptionGiftBatch:  {HasUser: true, HasQuantity: true, HasAnonymity: true},
	EventBits:                   {HasUser: true, HasMessage: true, HasQuantity: true, HasAnonymity: true},
	EventRaid:                   {HasUser: true, HasQuantity: true},
	EventChannelPointRedemption: {HasUser: true, HasMessage: true, HasRewardTitle: true},

	// Stage 15A (YouTube).
	EventYouTubeMembership:          {HasUser: true, HasMembershipLevel: true},
	EventYouTubeMembershipMilestone: {HasUser: true, HasMessage: true, HasQuantity: true, HasMembershipLevel: true},
	EventYouTubeSuperChat:           {HasUser: true, HasMessage: true, HasAmount: true},
	EventYouTubeSuperSticker:        {HasUser: true, HasAmount: true},
}

// CapabilityFor returns t's capability, or the zero Capability (nothing
// supported) if t is not a recognized EventType - callers should check
// EventType validity separately before relying on this for anything but
// a safe default.
func CapabilityFor(t EventType) Capability {
	return Capabilities[t]
}
