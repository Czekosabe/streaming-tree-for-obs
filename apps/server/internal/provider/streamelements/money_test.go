package streamelements

import (
	"encoding/json"
	"errors"
	"testing"
)

func num(s string) json.Number { return json.Number(s) }

func TestParseAmountMicrosIntegerAmount(t *testing.T) {
	got, err := ParseAmountMicros(num("5"))
	if err != nil || got != 5_000_000 {
		t.Fatalf("ParseAmountMicros(5) = (%d, %v), want (5000000, nil)", got, err)
	}
}

func TestParseAmountMicrosOneDecimal(t *testing.T) {
	got, err := ParseAmountMicros(num("4.2"))
	if err != nil || got != 4_200_000 {
		t.Fatalf("ParseAmountMicros(4.2) = (%d, %v), want (4200000, nil)", got, err)
	}
}

func TestParseAmountMicrosTwoDecimals(t *testing.T) {
	got, err := ParseAmountMicros(num("0.01"))
	if err != nil || got != 10_000 {
		t.Fatalf("ParseAmountMicros(0.01) = (%d, %v), want (10000, nil)", got, err)
	}
}

func TestParseAmountMicrosSixDecimals(t *testing.T) {
	got, err := ParseAmountMicros(num("1.000001"))
	if err != nil || got != 1_000_001 {
		t.Fatalf("ParseAmountMicros(1.000001) = (%d, %v), want (1000001, nil)", got, err)
	}
}

func TestParseAmountMicrosTrailingZeros(t *testing.T) {
	got, err := ParseAmountMicros(num("4.200000"))
	if err != nil || got != 4_200_000 {
		t.Fatalf("ParseAmountMicros(4.200000) = (%d, %v), want (4200000, nil)", got, err)
	}
}

func TestParseAmountMicrosExponentForm(t *testing.T) {
	// math/big.Rat.SetString accepts JSON-valid exponent notation
	// exactly (confirmed empirically before writing this parser) - "1e1"
	// major units is 10.000000, i.e. 10,000,000 micros.
	got, err := ParseAmountMicros(num("1e1"))
	if err != nil || got != 10_000_000 {
		t.Fatalf("ParseAmountMicros(1e1) = (%d, %v), want (10000000, nil)", got, err)
	}
}

func TestParseAmountMicrosZero(t *testing.T) {
	got, err := ParseAmountMicros(num("0"))
	if err != nil || got != 0 {
		t.Fatalf("ParseAmountMicros(0) = (%d, %v), want (0, nil)", got, err)
	}
}

func TestParseAmountMicrosNegativeIsRejected(t *testing.T) {
	if _, err := ParseAmountMicros(num("-1")); !errors.Is(err, ErrAmountNegative) {
		t.Fatalf("ParseAmountMicros(-1) error = %v, want ErrAmountNegative", err)
	}
}

func TestParseAmountMicrosMoreThanSixFractionalDigitsIsRejectedNotRounded(t *testing.T) {
	if _, err := ParseAmountMicros(num("1.1234567")); !errors.Is(err, ErrAmountPrecisionUnsupported) {
		t.Fatalf("ParseAmountMicros(1.1234567) error = %v, want ErrAmountPrecisionUnsupported", err)
	}
}

func TestParseAmountMicrosOverflowIsRejected(t *testing.T) {
	// Comfortably beyond maxAmountMicros (1,000,000,000,000) once
	// converted to micros.
	if _, err := ParseAmountMicros(num("999999999999999")); !errors.Is(err, ErrAmountOverflow) {
		t.Fatalf("ParseAmountMicros(999999999999999) error = %v, want ErrAmountOverflow", err)
	}
}

func TestParseAmountMicrosInt64OverflowIsRejected(t *testing.T) {
	// A value so large that even the numerator itself cannot fit an
	// int64 - exercises the numerator.IsInt64() guard specifically,
	// distinct from the maxAmountMicros bound check above.
	if _, err := ParseAmountMicros(num("99999999999999999999999999")); !errors.Is(err, ErrAmountOverflow) {
		t.Fatalf("ParseAmountMicros(huge) error = %v, want ErrAmountOverflow", err)
	}
}

func TestParseAmountMicrosMalformedLexicalNumberIsRejected(t *testing.T) {
	// Real json.Number values only ever carry text a json.Decoder itself
	// already accepted as a valid JSON number token - these are not
	// valid JSON number tokens, so a real decode would never actually
	// hand this parser any of them; tested anyway as a defensive
	// boundary in case a caller ever constructs a json.Number by hand.
	for _, s := range []string{"abc", "", "NaN", "1,000", "$5.00", "Infinity"} {
		if _, err := ParseAmountMicros(num(s)); err == nil {
			t.Errorf("ParseAmountMicros(%q) error = nil, want a parse error", s)
		}
	}
}

func TestBuildMoneyNormalizesLowercaseCurrency(t *testing.T) {
	money, err := BuildMoney(num("5"), "usd", "$5.00")
	if err != nil {
		t.Fatalf("BuildMoney() error = %v", err)
	}
	if money.Currency != "USD" {
		t.Fatalf("Currency = %q, want USD", money.Currency)
	}
	if money.AmountMicros != 5_000_000 {
		t.Fatalf("AmountMicros = %d, want 5000000", money.AmountMicros)
	}
	if money.DisplayAmount != "$5.00" {
		t.Fatalf("DisplayAmount = %q, want $5.00", money.DisplayAmount)
	}
}

func TestBuildMoneyRejectsEmptyCurrency(t *testing.T) {
	if _, err := BuildMoney(num("5"), "", "$5.00"); !errors.Is(err, ErrInvalidCurrency) {
		t.Fatalf("BuildMoney() with empty currency error = %v, want ErrInvalidCurrency", err)
	}
}

func TestBuildMoneyPropagatesAmountErrors(t *testing.T) {
	if _, err := BuildMoney(num("not-a-number"), "USD", ""); !errors.Is(err, ErrAmountMalformed) {
		t.Fatalf("BuildMoney() with a malformed amount error = %v, want ErrAmountMalformed", err)
	}
}

func TestBuildMoneyGeneratesADisplayAmountWhenNoneIsSupplied(t *testing.T) {
	cases := []struct {
		amount, currency, want string
	}{
		{"4.2", "USD", "4.20 USD"},
		{"10", "USD", "10.00 USD"},
		{"1.000001", "USD", "1.000001 USD"},
		{"0.01", "eur", "0.01 EUR"},
		{"0", "USD", "0.00 USD"},
	}
	for _, c := range cases {
		money, err := BuildMoney(num(c.amount), c.currency, "")
		if err != nil {
			t.Fatalf("BuildMoney(%q, %q, \"\") error = %v", c.amount, c.currency, err)
		}
		if money.DisplayAmount != c.want {
			t.Errorf("BuildMoney(%q, %q, \"\").DisplayAmount = %q, want %q", c.amount, c.currency, money.DisplayAmount, c.want)
		}
	}
}

func TestBuildMoneyNeverOverwritesAProviderSuppliedDisplayAmount(t *testing.T) {
	money, err := BuildMoney(num("4.2"), "USD", "$4.20")
	if err != nil {
		t.Fatalf("BuildMoney() error = %v", err)
	}
	if money.DisplayAmount != "$4.20" {
		t.Fatalf("DisplayAmount = %q, want the supplied \"$4.20\" unmodified", money.DisplayAmount)
	}
}

func TestParseAmountMicrosNoFloatingPointArithmeticRegression(t *testing.T) {
	// A value that is exactly representable in decimal but famously NOT
	// exactly representable in binary float64 (0.1 + 0.2 != 0.3 in
	// float64 arithmetic) - proves this parser never routes through
	// float64 at any point.
	got, err := ParseAmountMicros(num("0.1"))
	if err != nil || got != 100_000 {
		t.Fatalf("ParseAmountMicros(0.1) = (%d, %v), want (100000, nil) - exact, not a float64 approximation", got, err)
	}
	got, err = ParseAmountMicros(num("0.3"))
	if err != nil || got != 300_000 {
		t.Fatalf("ParseAmountMicros(0.3) = (%d, %v), want (300000, nil) - exact, not a float64 approximation", got, err)
	}
}
