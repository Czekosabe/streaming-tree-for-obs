package audio

import (
	"fmt"
	"strings"

	engagement "github.com/streaming-tree/server/internal/domain/engagement"
)

// BuildUtterance converts a normalized engagement.Event into plain
// spoken text, using only fields evt actually carries (docs/audio-tts.md
// §10.1) - never inventing data, never letting a provider generate its
// own sentence. ok is false when evt.Type is not speakable at all
// (CapabilityFor), or when a field the sentence structurally requires
// (a display name, a monetary amount) is missing. Output is always
// plain text - no SSML, HTML, Markdown, or embedded markup is ever
// constructed here.
func BuildUtterance(evt engagement.Event) (text string, ok bool) {
	capa := CapabilityFor(evt.Type)
	if !capa.Speakable {
		return "", false
	}

	name := displayName(evt.User)
	message := messageText(evt.Message)

	switch evt.Type {
	case engagement.TypeChatMessage:
		if name == "" || message == "" {
			return "", false
		}
		return name + " says " + message, true

	case engagement.TypeFollow:
		if name == "" {
			return "", false
		}
		return name + " followed", true

	case engagement.TypeSubscription:
		if name == "" {
			return "", false
		}
		return name + " subscribed", true

	case engagement.TypeResubscription:
		if name == "" {
			return "", false
		}
		return appendMessage(name+" resubscribed", message), true

	case engagement.TypeGiftedSubscription:
		if name == "" {
			return "", false
		}
		return name + " gifted a subscription", true

	case engagement.TypeSubscriptionGiftBatch:
		gifter := anonymousAware(name, evt.User, "An anonymous gifter")
		return fmt.Sprintf("%s gifted %d subscriptions", gifter, quantityOrOne(evt.Quantity)), true

	case engagement.TypeBits:
		who := anonymousAware(name, evt.User, "Someone")
		base := fmt.Sprintf("%s cheered %d bits", who, quantityOrOne(evt.Quantity))
		return appendMessage(base, message), true

	case engagement.TypeRaid:
		if name == "" {
			return "", false
		}
		if evt.Quantity != nil {
			return fmt.Sprintf("%s raided with %d viewers", name, *evt.Quantity), true
		}
		return name + " raided", true

	case engagement.TypeChannelPointRedemption:
		if name == "" {
			return "", false
		}
		return appendMessage(name+" redeemed a reward", message), true

	case engagement.TypeYouTubeMembership:
		if name == "" {
			return "", false
		}
		if level := membershipLevel(evt); level != "" {
			return fmt.Sprintf("%s became a %s member", name, level), true
		}
		return name + " became a member", true

	case engagement.TypeYouTubeMembershipMilestone:
		if name == "" {
			return "", false
		}
		base := name
		if level := membershipLevel(evt); level != "" {
			base += fmt.Sprintf(" (%s)", level)
		}
		if evt.Quantity != nil {
			base += fmt.Sprintf(" celebrated %d months as a member", *evt.Quantity)
		} else {
			base += " celebrated a membership milestone"
		}
		return appendMessage(base, message), true

	case engagement.TypeYouTubeSuperChat:
		amount := moneyDisplay(evt.Money)
		if name == "" || amount == "" {
			return "", false
		}
		return appendMessage(fmt.Sprintf("%s sent a super chat of %s", name, amount), message), true

	case engagement.TypeYouTubeSuperSticker:
		amount := moneyDisplay(evt.Money)
		if name == "" || amount == "" {
			return "", false
		}
		return fmt.Sprintf("%s sent a super sticker of %s", name, amount), true

	case engagement.TypeDonation:
		amount := moneyDisplay(evt.Money)
		if amount == "" {
			return "", false
		}
		donor := anonymousAware(name, evt.User, "An anonymous donor")
		return appendMessage(fmt.Sprintf("%s donated %s", donor, amount), message), true

	default:
		return "", false
	}
}

func displayName(u *engagement.User) string {
	if u == nil || u.Anonymous {
		return ""
	}
	if u.DisplayName != "" {
		return u.DisplayName
	}
	return u.Login
}

// anonymousAware returns fallback whenever u is nil, u.Anonymous is
// true, or no name could be resolved - so an anonymous gifter/cheerer/
// donor is announced honestly as anonymous rather than silently dropped
// or given a fabricated name.
func anonymousAware(name string, u *engagement.User, fallback string) string {
	if name == "" || u == nil || u.Anonymous {
		return fallback
	}
	return name
}

func messageText(m *engagement.Message) string {
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m.Text)
}

func appendMessage(base, message string) string {
	if message == "" {
		return base
	}
	return base + ": " + message
}

// moneyDisplay prefers the provider-formatted DisplayAmount (never
// parsed back into a number by this application); falls back to a
// deterministic integer-only rendering of AmountMicros when
// DisplayAmount was left empty, never using floating-point arithmetic.
func moneyDisplay(m *engagement.Money) string {
	if m == nil {
		return ""
	}
	if m.DisplayAmount != "" {
		return m.DisplayAmount
	}
	if m.Currency == "" {
		return ""
	}
	return fmt.Sprintf("%s %s", formatMicros(m.AmountMicros), m.Currency)
}

// formatMicros renders AmountMicros (millionths of the major unit) as a
// plain "whole.cents" string using only integer arithmetic.
func formatMicros(micros int64) string {
	if micros < 0 {
		micros = 0
	}
	whole := micros / 1_000_000
	cents := (micros % 1_000_000) / 10_000
	return fmt.Sprintf("%d.%02d", whole, cents)
}

func quantityOrOne(q *int64) int64 {
	if q == nil || *q < 1 {
		return 1
	}
	return *q
}

// membershipLevel reads the one deliberately-named ProviderExtra
// exception the YouTube normalizer populates for membership events -
// see internal/alerts/matcher.go's own identical "memberLevelName" read
// for the alerts engine's equivalent {membershipLevel} placeholder.
func membershipLevel(evt engagement.Event) string {
	if evt.ProviderExtra == nil {
		return ""
	}
	return evt.ProviderExtra["memberLevelName"]
}
