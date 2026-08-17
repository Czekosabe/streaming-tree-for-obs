package supporterwidgets

import "time"

// SupporterItem is one presentation-safe, runtime-only row built from a
// real Engagement Event (docs/supporter-widgets.md §11, §12) - never
// persisted, never carrying a providerEventId or any other provider-
// private identifier. ItemID is a fresh, uncorrelated presentation id.
type SupporterItem struct {
	ItemID string

	// DisplayName is empty when the provider reported no display name or
	// marked the event anonymous - rendered as a generic label
	// client-side, never fabricated here (docs/supporter-widgets.md
	// §11).
	DisplayName string
	// Provider is the raw engagement.ProviderID string (e.g. "twitch") -
	// the frontend already owns provider-label localization for goals
	// (docs/goals-widgets.md), reused unchanged here.
	Provider string

	HasAmount    bool
	AmountMicros int64
	Currency     string

	HasQuantity bool
	Quantity    int64

	// Message is empty unless the source event carried one; the caller
	// (applyEventToProjection) clears it for latest_donation unless the
	// profile's own ShowMessage is true (docs/supporter-widgets.md §9).
	Message string

	ObservedAt time.Time
}

// TickerItem is one event_ticker row - a SupporterItem plus the closed
// presentation EventType it was built from (docs/supporter-widgets.md
// §8), so the client can render a gift-batch summary differently from an
// individual recipient without guessing from the other fields.
type TickerItem struct {
	SupporterItem
	EventType string
}

// Projection is one widget profile's own current runtime presentation
// state (docs/supporter-widgets.md §9) - the zero value is every kind's
// own well-defined "nothing observed yet" empty state. Exactly one of
// Latest/Largest/Recent/Ticker/Counter is ever meaningful for a given
// profile, decided entirely by that profile's own Kind - Manager never
// writes to a field its Kind does not own.
type Projection struct {
	Revision  uint64
	UpdatedAt time.Time

	Latest  *SupporterItem // latest_follower, latest_subscriber, latest_donation
	Largest *SupporterItem // largest_donation

	Recent []SupporterItem // recent_supporters, newest first, bounded by MaxItems
	Ticker []TickerItem    // event_ticker, newest first, bounded by MaxItems

	Counter int64 // session_counter
}

// clone returns a deep-enough copy so a caller can never mutate
// Manager's own internal state through a returned Snapshot.
func (p Projection) clone() Projection {
	out := p
	if p.Latest != nil {
		v := *p.Latest
		out.Latest = &v
	}
	if p.Largest != nil {
		v := *p.Largest
		out.Largest = &v
	}
	if p.Recent != nil {
		out.Recent = append([]SupporterItem(nil), p.Recent...)
	}
	if p.Ticker != nil {
		out.Ticker = append([]TickerItem(nil), p.Ticker...)
	}
	return out
}
