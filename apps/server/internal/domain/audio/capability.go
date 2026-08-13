package audio

import engagement "github.com/streaming-tree/server/internal/domain/engagement"

// Capability declares, for one engagement.Type, whether Stage 17A TTS may
// ever speak it and which fields the utterance builder may rely on -
// mirrors internal/domain/alerts/capability.go's own exact
// map[EventType]Capability/CapabilityFor pattern (docs/audio-tts.md §9).
//
// Built by reading the real normalized event model (this package's own
// engagement.Event/User/Message/Money), never by guessing from a
// provider name. A type absent from Capabilities is not speakable at
// all - CapabilityFor returns the zero value for it, exactly like the
// alerts package's own safe-default convention, so a future provider
// adding a new event type is never spoken until this table gets an
// explicit new entry.
type Capability struct {
	// Speakable gates everything else: false means TTS never reads this
	// event type aloud, regardless of any other setting.
	Speakable bool
	// HasUser: the event carries a *engagement.User (which may itself
	// still be Anonymous).
	HasUser bool
	// HasMessage: the event's Message can carry real spoken-worthy text.
	HasMessage bool
	// HasMoney: the event carries a real *engagement.Money value.
	HasMoney bool
	// HasQuantity: the event's Quantity is a real, meaningful number
	// (bits amount, gift count, raid viewer count, membership-milestone
	// month count).
	HasQuantity bool
	// SupporterFamily marks the closed set of "supporter" event types
	// docs/audio-tts.md §9 names explicitly - never inferred from a
	// provider name or from User.Roles. Governs Settings.SupporterOnlyMode.
	SupporterFamily bool
	// AnonymousPossible: this event type has a real anonymous-actor path
	// (an anonymous gifter/cheerer/donor) where User.Anonymous may be
	// true and the utterance builder must not fabricate a name.
	AnonymousPossible bool
}

// Capabilities maps every engagement.Type Stage 17A TTS may ever speak.
// engagement.TypeChatMessageDeleted, TypeChatCleared, TypeModeration,
// TypeStreamOnline, and TypeStreamOffline are deliberately absent - none
// carries spoken-worthy content, so each falls through to the zero
// Capability (not speakable) via CapabilityFor.
var Capabilities = map[engagement.Type]Capability{
	engagement.TypeChatMessage: {Speakable: true, HasUser: true, HasMessage: true},

	engagement.TypeFollow:                 {Speakable: true, HasUser: true},
	engagement.TypeSubscription:           {Speakable: true, HasUser: true, SupporterFamily: true},
	engagement.TypeResubscription:         {Speakable: true, HasUser: true, HasMessage: true, SupporterFamily: true},
	engagement.TypeGiftedSubscription:     {Speakable: true, HasUser: true, SupporterFamily: true},
	engagement.TypeSubscriptionGiftBatch:  {Speakable: true, HasUser: true, HasQuantity: true, SupporterFamily: true, AnonymousPossible: true},
	engagement.TypeBits:                   {Speakable: true, HasUser: true, HasMessage: true, HasQuantity: true, SupporterFamily: true, AnonymousPossible: true},
	engagement.TypeRaid:                   {Speakable: true, HasUser: true, HasQuantity: true},
	engagement.TypeChannelPointRedemption: {Speakable: true, HasUser: true, HasMessage: true},

	// Stage 15A (YouTube).
	engagement.TypeYouTubeMembership:          {Speakable: true, HasUser: true, SupporterFamily: true},
	engagement.TypeYouTubeMembershipMilestone: {Speakable: true, HasUser: true, HasMessage: true, HasQuantity: true, SupporterFamily: true},
	engagement.TypeYouTubeSuperChat:           {Speakable: true, HasUser: true, HasMessage: true, HasMoney: true, SupporterFamily: true},
	engagement.TypeYouTubeSuperSticker:        {Speakable: true, HasUser: true, HasMoney: true, SupporterFamily: true},

	// Stage 16A (external donations).
	engagement.TypeDonation: {Speakable: true, HasUser: true, HasMessage: true, HasMoney: true, SupporterFamily: true, AnonymousPossible: true},
}

// CapabilityFor returns t's capability, or the zero Capability (nothing
// speakable) if t is not a recognized entry in this table.
func CapabilityFor(t engagement.Type) Capability {
	return Capabilities[t]
}
