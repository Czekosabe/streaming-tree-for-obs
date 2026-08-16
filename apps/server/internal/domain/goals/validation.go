package goals

import "fmt"

// Conservative bounds - see docs/goals-widgets.md §8.2 for the exact
// JS-safe-integer reasoning behind the two Max* constants.
const (
	MaxNameCodePoints = 80

	// MaxGoalCountValue bounds Target/Current/Baseline for
	// KindFollowers/KindSubscriptions/KindBits goals.
	MaxGoalCountValue int64 = 100_000_000

	// MaxGoalAmountMicros bounds Target/Current/Baseline (in integer
	// micros) for KindDonations goals - mirrors engagement.
	// maxAmountMicros's own reasoning, scaled up for an accumulated
	// total rather than a single event.
	MaxGoalAmountMicros int64 = 100_000_000_000_000

	DefaultBorderRadiusPx = 12
	MaxBorderRadiusPx     = 32

	MaxHexColorLength = 16
)

func validationErr(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrValidation, fmt.Sprintf(format, args...))
}

func codePointLen(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

// ValidateName checks a goal or widget profile's display name.
func ValidateName(name string) error {
	n := codePointLen(name)
	if n < 1 || n > MaxNameCodePoints {
		return validationErr("name must be 1-%d characters", MaxNameCodePoints)
	}
	return nil
}

func maxValueFor(k Kind) int64 {
	if k.RequiresCurrency() {
		return MaxGoalAmountMicros
	}
	return MaxGoalCountValue
}

// ValidateGoalFields checks every bound/enum/required-field rule on g,
// except provider/account existence (see Service.validateAccounts,
// which needs an AccountLookup) and ConfigRevision (checked by the
// repository at write time).
func ValidateGoalFields(g Goal) error {
	if err := ValidateName(g.Name); err != nil {
		return err
	}
	if !g.Kind.valid() {
		return validationErr("kind %q is not supported", string(g.Kind))
	}
	max := maxValueFor(g.Kind)
	if g.Target <= 0 || g.Target > max {
		return validationErr("target must be 1-%d", max)
	}
	if g.Current < 0 || g.Current > max {
		return validationErr("current must be 0-%d", max)
	}
	if g.Baseline < 0 || g.Baseline > max {
		return validationErr("baseline must be 0-%d", max)
	}
	if g.Kind.RequiresCurrency() {
		if !validCurrencyCode(g.Currency) {
			return validationErr("a monetary goal requires a valid currency code")
		}
	} else if g.Currency != "" {
		return validationErr("currency must be empty for a %s goal", string(g.Kind))
	}
	if err := ValidateProviders(g.Providers); err != nil {
		return err
	}
	if err := validateNoDuplicateStrings("accounts", g.Accounts); err != nil {
		return err
	}
	return nil
}

// ValidateProviders checks a goal's provider-filter list: every entry
// must be a recognized ProviderID, and the list must not repeat one -
// mirrors alerts.ValidateProviders exactly.
func ValidateProviders(providerIDs []ProviderID) error {
	seen := make(map[ProviderID]bool, len(providerIDs))
	for _, p := range providerIDs {
		if !p.valid() {
			return validationErr("provider %q is not supported", string(p))
		}
		if seen[p] {
			return validationErr("provider %q is listed more than once", string(p))
		}
		seen[p] = true
	}
	return nil
}

func validateNoDuplicateStrings(field string, values []string) error {
	seen := make(map[string]bool, len(values))
	for _, v := range values {
		if v == "" {
			return validationErr("%s entries must not be empty", field)
		}
		if seen[v] {
			return validationErr("%s entry %q is listed more than once", field, v)
		}
		seen[v] = true
	}
	return nil
}

// validCurrencyCode checks a plain uppercase-ASCII currency code shape
// (3-8 characters, A-Z only) - never validated against an external
// ISO-4217 registry, mirroring engagement.Money's own documented
// "the provider is trusted to report a real code" stance, applied here
// to an operator-typed code instead.
func validCurrencyCode(code string) bool {
	if len(code) < 3 || len(code) > 8 {
		return false
	}
	for _, c := range code {
		if c < 'A' || c > 'Z' {
			return false
		}
	}
	return true
}

// ValidateWidgetProfileFields checks every bound/enum/required-field
// rule on p, except GoalID existence (checked by the Service against
// the Goal repository).
func ValidateWidgetProfileFields(p WidgetProfile) error {
	if err := ValidateName(p.Name); err != nil {
		return err
	}
	if codePointLen(p.TitleOverride) > MaxNameCodePoints {
		return validationErr("title override must be at most %d characters", MaxNameCodePoints)
	}
	if !p.Orientation.valid() {
		return validationErr("orientation %q is not supported", string(p.Orientation))
	}
	if !p.TextAlign.valid() {
		return validationErr("text align %q is not supported", string(p.TextAlign))
	}
	if !p.FontFamily.valid() {
		return validationErr("font family %q is not supported", string(p.FontFamily))
	}
	for _, hex := range []struct {
		field string
		value string
	}{
		{"background color", p.BackgroundColor},
		{"foreground color", p.ForegroundColor},
		{"fill color", p.FillColor},
		{"border color", p.BorderColor},
	} {
		if !validHexColor(hex.value) {
			return validationErr("%s must be a valid hex color", hex.field)
		}
	}
	if p.BorderRadiusPx < 0 || p.BorderRadiusPx > MaxBorderRadiusPx {
		return validationErr("border radius must be 0-%d", MaxBorderRadiusPx)
	}
	if p.Opacity < 0.0 || p.Opacity > 1.0 {
		return validationErr("opacity must be between 0.0 and 1.0")
	}
	return nil
}

// validHexColor checks a bounded "#RRGGBB" or "#RRGGBBAA" shape - never
// an arbitrary CSS color function/keyword, never a remote reference.
func validHexColor(s string) bool {
	if len(s) == 0 || len(s) > MaxHexColorLength {
		return false
	}
	if s[0] != '#' {
		return false
	}
	hexDigits := s[1:]
	if len(hexDigits) != 6 && len(hexDigits) != 8 {
		return false
	}
	for _, c := range hexDigits {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}
