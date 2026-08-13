package audio

import (
	"fmt"
	"unicode/utf8"

	engagement "github.com/streaming-tree/server/internal/domain/engagement"
)

// Bounds - see docs/audio-tts.md §7 for the reasoning behind each
// default/range.
const (
	MinQueueCapacity     = 10
	MaxQueueCapacity     = 500
	DefaultQueueCapacity = 100

	MinMaxTextLengthCodePoints     = 50
	MaxMaxTextLengthCodePoints     = 2000
	DefaultMaxTextLengthCodePoints = 500

	MinPerUserCooldownSeconds     = 0
	MaxPerUserCooldownSeconds     = 3600
	DefaultPerUserCooldownSeconds = 30

	MinGlobalCooldownSeconds     = 0
	MaxGlobalCooldownSeconds     = 300
	DefaultGlobalCooldownSeconds = 3

	MinSpeed = 0.5
	MaxSpeed = 2.0

	MinVolume = 0.0
	MaxVolume = 1.0

	MaxBlockedWords      = 200
	MaxBlockedWordLength = 100
	MaxVoiceIDLength     = 200
	MaxLanguageLength    = 35
	MaxCurrencyLength    = 8
	MaxEnabledSourceIDs  = 200
	MaxSourceIDLength    = 128

	// maxAmountMicros mirrors internal/domain/alerts's own bound exactly
	// - a threshold this application will never need to compare a real
	// provider amount against beyond.
	maxAmountMicros int64 = 1_000_000_000_000
)

// ValidateSettings checks every bound and enum in s, returning
// ErrValidation-wrapped errors. Never mutates s.
func ValidateSettings(s Settings) error {
	if !s.ProviderMode.known() {
		return fmt.Errorf("%w: unknown provider mode %q", ErrValidation, s.ProviderMode)
	}
	if !s.ProviderMode.implemented() {
		return fmt.Errorf("%w: provider mode %q is not implemented in this stage", ErrValidation, s.ProviderMode)
	}

	for _, t := range s.EnabledEventTypes {
		if !t.Known() {
			return fmt.Errorf("%w: unknown event type %q", ErrValidation, t)
		}
	}
	if err := requireNoDuplicateStrings("event type", typesToStrings(s.EnabledEventTypes)); err != nil {
		return err
	}
	if err := requireNoDuplicateStrings("provider id", providerIDsToStrings(s.EnabledProviderIDs)); err != nil {
		return err
	}

	if len(s.EnabledSourceIDs) > MaxEnabledSourceIDs {
		return fmt.Errorf("%w: too many enabled source ids (max %d)", ErrValidation, MaxEnabledSourceIDs)
	}
	seen := make(map[string]bool, len(s.EnabledSourceIDs))
	for _, id := range s.EnabledSourceIDs {
		if id == "" {
			return fmt.Errorf("%w: source id must not be empty", ErrValidation)
		}
		if utf8.RuneCountInString(id) > MaxSourceIDLength {
			return fmt.Errorf("%w: source id exceeds %d characters", ErrValidation, MaxSourceIDLength)
		}
		if seen[id] {
			return fmt.Errorf("%w: duplicate source id %q", ErrValidation, id)
		}
		seen[id] = true
	}

	if err := validateMoneyThreshold(s.ThresholdCurrency, s.ThresholdMinimumAmountMicros); err != nil {
		return err
	}
	if s.MinimumBits != nil && *s.MinimumBits < 0 {
		return fmt.Errorf("%w: minimum bits must not be negative", ErrValidation)
	}

	if s.MaxTextLengthCodePoints < MinMaxTextLengthCodePoints || s.MaxTextLengthCodePoints > MaxMaxTextLengthCodePoints {
		return fmt.Errorf("%w: max text length must be between %d and %d code points", ErrValidation, MinMaxTextLengthCodePoints, MaxMaxTextLengthCodePoints)
	}
	if s.PerUserCooldownSeconds < MinPerUserCooldownSeconds || s.PerUserCooldownSeconds > MaxPerUserCooldownSeconds {
		return fmt.Errorf("%w: per-user cooldown must be between %d and %d seconds", ErrValidation, MinPerUserCooldownSeconds, MaxPerUserCooldownSeconds)
	}
	if s.GlobalCooldownSeconds < MinGlobalCooldownSeconds || s.GlobalCooldownSeconds > MaxGlobalCooldownSeconds {
		return fmt.Errorf("%w: global cooldown must be between %d and %d seconds", ErrValidation, MinGlobalCooldownSeconds, MaxGlobalCooldownSeconds)
	}

	if len(s.BlockedWords) > MaxBlockedWords {
		return fmt.Errorf("%w: too many blocked words (max %d)", ErrValidation, MaxBlockedWords)
	}
	for _, w := range s.BlockedWords {
		if w == "" {
			return fmt.Errorf("%w: a blocked word must not be empty", ErrValidation)
		}
		if utf8.RuneCountInString(w) > MaxBlockedWordLength {
			return fmt.Errorf("%w: a blocked word exceeds %d characters", ErrValidation, MaxBlockedWordLength)
		}
	}

	if s.QueueCapacity < MinQueueCapacity || s.QueueCapacity > MaxQueueCapacity {
		return fmt.Errorf("%w: queue capacity must be between %d and %d", ErrValidation, MinQueueCapacity, MaxQueueCapacity)
	}

	if utf8.RuneCountInString(s.VoiceID) > MaxVoiceIDLength {
		return fmt.Errorf("%w: voice id exceeds %d characters", ErrValidation, MaxVoiceIDLength)
	}
	if utf8.RuneCountInString(s.Language) > MaxLanguageLength {
		return fmt.Errorf("%w: language exceeds %d characters", ErrValidation, MaxLanguageLength)
	}

	if isNaNOrInf(s.Speed) || s.Speed < MinSpeed || s.Speed > MaxSpeed {
		return fmt.Errorf("%w: speed must be between %g and %g", ErrValidation, MinSpeed, MaxSpeed)
	}
	if isNaNOrInf(s.Volume) || s.Volume < MinVolume || s.Volume > MaxVolume {
		return fmt.Errorf("%w: volume must be between %g and %g", ErrValidation, MinVolume, MaxVolume)
	}

	return nil
}

func isNaNOrInf(v float64) bool {
	return v != v || v > 1e308 || v < -1e308
}

// validateMoneyThreshold mirrors internal/domain/alerts.ValidateMoneyThresholds's
// own exact reasoning: a threshold is never meaningful without knowing
// which currency it bounds, and this application never compares an
// amount across currencies.
func validateMoneyThreshold(currency string, minimum *int64) error {
	if minimum == nil {
		return nil
	}
	if currency == "" {
		return fmt.Errorf("%w: a monetary threshold requires a currency", ErrValidation)
	}
	if utf8.RuneCountInString(currency) > MaxCurrencyLength {
		return fmt.Errorf("%w: currency exceeds %d characters", ErrValidation, MaxCurrencyLength)
	}
	if *minimum < 0 || *minimum > maxAmountMicros {
		return fmt.Errorf("%w: minimum amount is out of range", ErrValidation)
	}
	return nil
}

// NormalizeCurrency uppercases a currency code exactly like
// engagement.NewMoney/internal/domain/alerts.NormalizeCurrency do, so a
// stored threshold's currency always matches the case a normalized
// Money.Currency will actually carry.
func NormalizeCurrency(currency string) string {
	b := []byte(currency)
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			b[i] = c - ('a' - 'A')
		}
	}
	return string(b)
}

func typesToStrings(types []engagement.Type) []string {
	out := make([]string, len(types))
	for i, t := range types {
		out[i] = string(t)
	}
	return out
}

func providerIDsToStrings(ids []engagement.ProviderID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = string(id)
	}
	return out
}

func requireNoDuplicateStrings(label string, values []string) error {
	seen := make(map[string]bool, len(values))
	for _, v := range values {
		if seen[v] {
			return fmt.Errorf("%w: duplicate %s %q", ErrValidation, label, v)
		}
		seen[v] = true
	}
	return nil
}
