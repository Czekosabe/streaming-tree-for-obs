package streamelements

import (
	"fmt"
	"time"

	engagement "github.com/streaming-tree/server/internal/domain/engagement"
)

// NormalizeTip converts one publishable StreamElements tip into the
// provider-independent engagement.Event shape, or reports
// ErrTipNotPublishable if it is not currently safe to publish (pending or
// rejected - see Tip.Publishable). sourceID is the local donation
// source's own id (internal/domain/donationsource.Source.ID) - populated
// into Event.ConnectedAccountID exactly the way a connected account's id
// is, since alert account/source filtering and operator-chat both key on
// that field regardless of which domain actually owns the id (see
// docs/provider-integrations/external-donations.md's own persistence
// section).
//
// Deliberately never normalizes, and this function's own return value
// can never carry: tip.Donation.User.Email, tip.Donation.User.Geo,
// tip.Donation.PaymentMethod, tip.Provider (the payment rail), or
// tip.TransactionID - see docs/provider-integrations/
// external-donations.md §22/§23 for the full privacy-boundary reasoning.
// Only tip.Donation.User.Username, tip.Donation.Message,
// tip.Donation.Amount/Currency, and tip.ID/tip.CreatedAt ever reach the
// returned Event.
func NormalizeTip(sourceID string, tip Tip) (engagement.Event, error) {
	if !tip.Publishable() {
		return engagement.Event{}, ErrTipNotPublishable
	}

	ts, err := time.Parse(time.RFC3339, tip.CreatedAt)
	if err != nil {
		return engagement.Event{}, fmt.Errorf("%w: invalid createdAt: %s", ErrMalformedPayload, err)
	}

	money, err := BuildMoney(tip.Donation.Amount, tip.Donation.Currency, "")
	if err != nil {
		return engagement.Event{}, err
	}

	evt := engagement.Event{
		SchemaVersion:      engagement.CurrentSchemaVersion,
		ProviderID:         engagement.ProviderStreamElements,
		ConnectedAccountID: sourceID,
		ProviderEventID:    tip.ID,
		ProviderEventType:  "channel.tips",
		Type:               engagement.TypeDonation,
		PlatformTimestamp:  ts,
		DedupeKey:          tip.ID,
		Money:              &money,
	}

	// Donor identity: preserve the display name if one was given, never
	// fabricate a stable id, never use email/a hash of it, never use the
	// transaction id as if it were a donor identity (docs/provider-
	// integrations/external-donations.md §24).
	username := tip.Donation.User.Username
	if username != "" {
		evt.User = &engagement.User{DisplayName: username}
	} else {
		evt.User = &engagement.User{Anonymous: true}
	}

	// Message: plain text only, exactly as StreamElements sent it - no
	// HTML/Markdown execution, no linkification (docs/provider-
	// integrations/external-donations.md §25). An empty message is
	// normal (StreamElements' own example carries "") and is left as a
	// nil Message, mirroring how every other normalizer in this
	// codebase treats an absent optional message.
	if tip.Donation.Message != "" {
		message := engagement.NewMessage([]engagement.Fragment{{Type: engagement.FragmentText, Text: tip.Donation.Message}})
		evt.Message = &message
	}

	return evt, nil
}
