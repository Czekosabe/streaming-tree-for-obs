package streamelements

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/streaming-tree/server/internal/domain/engagement"
)

// microsPerUnit converts a major currency unit (e.g. one whole dollar)
// into integer micros - engagement.Money's own representation
// (internal/domain/engagement.Money.AmountMicros), never a float.
const microsPerUnit = 1_000_000

// maxAmountMicros mirrors engagement.NewMoney's own bound exactly - a
// value this application will never need to represent beyond.
const maxAmountMicros int64 = 1_000_000_000_000

// ParseAmountMicros converts a StreamElements tip's `data.donation.amount`
// JSON number field into exact integer micros - never through float64.
//
// raw must come from a json.Decoder configured with UseNumber() (see
// envelope.go), so the JSON number's original lexical text is preserved
// exactly rather than rounded through float64 during decoding itself -
// the float64 imprecision this function exists to avoid would otherwise
// already have happened one step earlier, silently, in the JSON decoder.
//
// Uses math/big.Rat throughout: an exact rational number, never a
// floating-point approximation. A value is accepted only if it converts
// to a whole number of micros with zero remainder - StreamElements' own
// example amount (4.2) needs six digits of fractional precision at most
// to be exact (4,200,000 micros); a value carrying more fractional
// precision than integer micros can represent exactly is rejected outright,
// never silently rounded (docs/provider-integrations/
// external-donations.md §20's own explicit instruction).
func ParseAmountMicros(raw json.Number) (int64, error) {
	s := string(raw)
	if s == "" {
		return 0, fmt.Errorf("%w: empty amount", ErrAmountMalformed)
	}

	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return 0, fmt.Errorf("%w: %q", ErrAmountMalformed, s)
	}
	if r.Sign() < 0 {
		return 0, fmt.Errorf("%w: %q", ErrAmountNegative, s)
	}

	micros := new(big.Rat).Mul(r, big.NewRat(microsPerUnit, 1))
	if !micros.IsInt() {
		return 0, fmt.Errorf("%w: %q cannot be represented exactly as integer micros", ErrAmountPrecisionUnsupported, s)
	}

	numerator := micros.Num() // exact, since micros.IsInt() means the denominator reduced to 1
	if !numerator.IsInt64() {
		return 0, fmt.Errorf("%w: %q", ErrAmountOverflow, s)
	}
	v := numerator.Int64()
	if v > maxAmountMicros {
		return 0, fmt.Errorf("%w: %q", ErrAmountOverflow, s)
	}
	return v, nil
}

// BuildMoney parses a tip's amount and currency into an
// engagement.Money, combining ParseAmountMicros' exact conversion with
// engagement.NewMoney's own currency uppercasing/validation - the single
// entry point normalize.go uses, and the one this package's own tests
// exercise both halves through.
//
// If displayAmount is empty, one is generated with formatDisplayAmount -
// unlike YouTube, a StreamElements tip carries no provider-formatted
// display string of its own (docs/provider-integrations/
// external-donations.md §7's own verbatim payload has no such field), and
// leaving DisplayAmount empty would mean a real donation's amount never
// renders anywhere that only reads DisplayAmount (operator chat, the
// Engagement diagnostic feed) despite AmountMicros/Currency being exactly
// known - see internal/operatorchat/activity.go's own buildActivityItem.
func BuildMoney(amountRaw json.Number, currency, displayAmount string) (engagement.Money, error) {
	micros, err := ParseAmountMicros(amountRaw)
	if err != nil {
		return engagement.Money{}, err
	}
	money, err := engagement.NewMoney(micros, currency, displayAmount)
	if err != nil {
		return engagement.Money{}, fmt.Errorf("%w: %s", ErrInvalidCurrency, err)
	}
	if money.DisplayAmount == "" {
		money.DisplayAmount = formatDisplayAmount(money.AmountMicros, money.Currency)
	}
	return money, nil
}

// formatDisplayAmount renders exact integer micros as a decimal string
// with a trailing currency code, entirely through integer arithmetic
// (division/modulo, string padding/trimming) - never float64, so it can
// never introduce the imprecision ParseAmountMicros itself exists to
// avoid. Always at least 2 fractional digits (conventional for currency);
// more are shown, never rounded away, when the exact value actually has
// them (e.g. 1,000,001 micros -> "1.000001 USD", never "1.00 USD").
func formatDisplayAmount(amountMicros int64, currency string) string {
	whole := amountMicros / microsPerUnit
	frac := amountMicros % microsPerUnit
	if frac < 0 {
		frac = -frac
	}
	fracStr := strings.TrimRight(fmt.Sprintf("%06d", frac), "0")
	if len(fracStr) < 2 {
		fracStr += strings.Repeat("0", 2-len(fracStr))
	}
	return strconv.FormatInt(whole, 10) + "." + fracStr + " " + currency
}
