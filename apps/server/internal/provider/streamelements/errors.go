package streamelements

import "errors"

// Sentinel errors this package returns. internal/runtime/
// streamelementsengagement maps these onto its own connector state/error
// codes; the frontend never sees one of these directly.
var (
	// ErrAmountMalformed means a tip's amount field could not be parsed
	// as a number at all.
	ErrAmountMalformed = errors.New("streamelements: malformed donation amount")

	// ErrAmountNegative means a tip's amount was negative - never a real
	// donation.
	ErrAmountNegative = errors.New("streamelements: donation amount must not be negative")

	// ErrAmountPrecisionUnsupported means a tip's amount carries more
	// fractional precision than integer micros can represent exactly -
	// rejected rather than silently rounded.
	ErrAmountPrecisionUnsupported = errors.New("streamelements: donation amount has unsupported precision")

	// ErrAmountOverflow means a tip's amount, once converted to micros,
	// exceeds the supported bound (engagement.NewMoney's own maximum) or
	// does not fit an int64 at all.
	ErrAmountOverflow = errors.New("streamelements: donation amount exceeds the supported bound")

	// ErrInvalidCurrency means a tip's currency code is empty or could
	// not be normalized to a usable code.
	ErrInvalidCurrency = errors.New("streamelements: invalid donation currency")

	// ErrMissingTipID means a tip payload had no data._id - the stable
	// identity this application deduplicates on; without it, a tip can
	// never be safely normalized (docs/provider-integrations/
	// external-donations.md §7/§19).
	ErrMissingTipID = errors.New("streamelements: tip is missing its stable id")

	// ErrTipNotPublishable means a tip's own approved/status fields do
	// not represent "a completed, moderator-approved donation" - the
	// caller must not publish it (pending, rejected, or a non-"success"
	// status - see docs/provider-integrations/external-donations.md §7).
	// Not itself an unexpected failure.
	ErrTipNotPublishable = errors.New("streamelements: tip is not currently publishable")

	// ErrMalformedPayload means a tip's own JSON payload was structurally
	// invalid (missing donation object, wrong field types) - a real
	// protocol/provider surprise, logged as a bounded diagnostic, never
	// crashing the connector.
	ErrMalformedPayload = errors.New("streamelements: malformed tip payload")

	// ErrUnexpectedMessageType means an Astro envelope's own `type` field
	// was not one this client recognizes - handled as a bounded
	// diagnostic, never a fatal error.
	ErrUnexpectedMessageType = errors.New("streamelements: unexpected message type")

	// ErrSubscribeFailed means the server's own `response` envelope for a
	// `subscribe` request carried a non-empty `error` field.
	ErrSubscribeFailed = errors.New("streamelements: subscribe request failed")

	// ErrConnectionClosed means the WebSocket connection ended (either
	// party) - the caller's own reconnect logic decides what to do next;
	// this is not itself a claim about why.
	ErrConnectionClosed = errors.New("streamelements: connection closed")

	// ErrFrameTooLarge means an inbound WebSocket frame exceeded this
	// client's own bounded read limit - a defensive bound against an
	// unbounded/hostile payload, never a real documented message size.
	ErrFrameTooLarge = errors.New("streamelements: frame exceeds the bounded read limit")
)
