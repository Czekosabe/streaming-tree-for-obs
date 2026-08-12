package streamelements

import (
	"encoding/json"
	"fmt"
)

// TipUser is one tip's donor information, exactly as StreamElements sends
// it - including the privacy-sensitive fields (Geo, Email) this
// application never normalizes, persists, exposes, or logs (see
// normalize.go's own NormalizeTip and docs/provider-integrations/
// external-donations.md §22).
type TipUser struct {
	Username string `json:"username"`
	Geo      string `json:"geo"`
	Email    string `json:"email"`
	Channel  string `json:"channel"`
}

// TipDonation is one tip's own `data.donation` object.
type TipDonation struct {
	User    TipUser `json:"user"`
	Message string  `json:"message"`
	// Amount is deliberately json.Number, never float64 - see money.go's
	// own doc comment for why.
	Amount json.Number `json:"amount"`
	// Currency is the provider-reported currency code, normalized
	// (uppercased, validated) by BuildMoney - never trusted as-is.
	Currency string `json:"currency"`
	// PaymentMethod is discarded during normalization - never exposed
	// (docs/provider-integrations/external-donations.md §22).
	PaymentMethod string `json:"paymentMethod"`
}

// Tip is one complete `channel.tips`/`channel.tips.moderation` message's
// own `data` object - extracted byte-for-byte from the official examples
// (docs/provider-integrations/external-donations.md §7).
type Tip struct {
	Donation TipDonation `json:"donation"`
	// ID is StreamElements' own stable tip identifier (`data._id`) -
	// this application's providerEventId/dedupe key. Confirmed to stay
	// identical across the entire moderation lifecycle (docs/provider-
	// integrations/external-donations.md §7) - never the envelope's own
	// `id` (a per-message ULID), never TransactionID.
	ID string `json:"_id"`
	// Channel mirrors the envelope's own `room` - unused (Room is
	// already known from the subscription itself).
	Channel string `json:"channel"`
	// Provider is the payment rail (e.g. "paypal") - a payment
	// processor, never an engagement.ProviderID, and discarded during
	// normalization (docs/provider-integrations/
	// external-donations.md §23).
	Provider string `json:"provider"`
	// Approved is the moderation state: "pending", "allowed", or
	// "rejected" - see Publishable.
	Approved string `json:"approved"`
	// Status is the payment/transaction outcome. Only "success" is
	// documented as representing a completed payment - any other value
	// is treated conservatively as unsupported/not-publishable, never
	// guessed at (docs/provider-integrations/
	// external-donations.md §7).
	Status        string `json:"status"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
	TransactionID string `json:"transactionId"`
	ApprovedBy    string `json:"approvedBy,omitempty"`
}

// Documented values for Tip.Approved and the one documented value for
// Tip.Status representing a completed payment - see
// docs/provider-integrations/external-donations.md §7.
const (
	ApprovedPending  = "pending"
	ApprovedAllowed  = "allowed"
	ApprovedRejected = "rejected"

	StatusSuccess = "success"
)

// ParseTip decodes one channel.tips/channel.tips.moderation message's
// `data` object. Rejects a tip with no _id outright - this application's
// entire dedup/identity model depends on it (docs/provider-integrations/
// external-donations.md §19).
func ParseTip(raw json.RawMessage) (Tip, error) {
	var tip Tip
	if err := json.Unmarshal(raw, &tip); err != nil {
		return Tip{}, fmt.Errorf("%w: %s", ErrMalformedPayload, err)
	}
	if tip.ID == "" {
		return Tip{}, ErrMissingTipID
	}
	return tip, nil
}

// Publishable reports whether t currently represents a real, completed,
// moderator-approved donation - the only condition under which this
// application ever publishes a donation event. See
// docs/provider-integrations/external-donations.md §7 for the full
// pending/allowed/rejected policy this implements.
func (t Tip) Publishable() bool {
	return t.Approved == ApprovedAllowed && t.Status == StatusSuccess
}
