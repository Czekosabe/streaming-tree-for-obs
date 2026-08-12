package engagement

import "fmt"

// Money is a provider-independent monetary value attached to an Event that
// genuinely carries real money or a provider-native paid-support amount
// (Stage 15A: YouTube Super Chat/Super Sticker). Deliberately integer-only -
// AmountMicros is the value in millionths of the major currency unit
// (1,000,000 = 1.00), exactly matching the unit YouTube's own API already
// uses, so no floating-point money arithmetic or rounding ever happens
// anywhere in this application. See docs/provider-integrations/
// youtube-engagement.md §6.
//
// This application never converts between currencies - a Money value's
// Currency is carried exactly as the provider reported it, uppercased, and
// compared for equality only (see internal/domain/alerts' monetary
// condition matching). There is no exchange-rate table anywhere in this
// codebase, and none should ever be added.
type Money struct {
	// AmountMicros is the authoritative value used for every threshold
	// comparison. Never negative.
	AmountMicros int64
	// Currency is an uppercase, provider-reported currency code (e.g.
	// "USD", "EUR") - normalized to uppercase by NewMoney, never converted
	// or validated against an external ISO-4217 registry (the provider is
	// trusted to report a real code; this application only normalizes
	// case).
	Currency string
	// DisplayAmount is an optional, provider-formatted string (e.g.
	// "$1.00") kept for display purposes only. It is never the
	// authoritative value for a threshold comparison and is never parsed
	// back into a number by this application.
	DisplayAmount string
}

// maxAmountMicros bounds AmountMicros defensively so a malformed or
// maliciously large provider value can never be carried into a persisted
// alert-rule comparison or an int64 overflow - well above any realistic
// Super Chat/Super Sticker amount (YouTube's own UI caps a single Super
// Chat at a four-figure amount in major units), while still leaving vast
// headroom before int64's own limit.
const maxAmountMicros int64 = 1_000_000_000_000 // 1,000,000.000000 in major units

// NewMoney builds a Money value, normalizing currency to uppercase and
// rejecting a negative or implausibly large amount or an empty currency.
// A connector must call this rather than constructing Money directly, so
// every Money value ever attached to an Event has already passed these
// checks before Event.Validate ever runs.
func NewMoney(amountMicros int64, currency string, displayAmount string) (Money, error) {
	if amountMicros < 0 {
		return Money{}, fmt.Errorf("%w: money amount must not be negative", ErrInvalidEvent)
	}
	if amountMicros > maxAmountMicros {
		return Money{}, fmt.Errorf("%w: money amount exceeds the supported bound", ErrInvalidEvent)
	}
	upper := toUpperASCII(currency)
	if upper == "" {
		return Money{}, fmt.Errorf("%w: money requires a currency", ErrInvalidEvent)
	}
	return Money{AmountMicros: amountMicros, Currency: upper, DisplayAmount: displayAmount}, nil
}

// toUpperASCII avoids importing strings.ToUpper's full Unicode case-folding
// for what is always an ASCII currency code in practice - any non-ASCII
// input is left as-is rather than mangled, then still validated as
// non-empty by the caller.
func toUpperASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			b[i] = c - ('a' - 'A')
		}
	}
	return string(b)
}
