package audio

import (
	"errors"
	"math"
	"strings"
	"testing"

	engagement "github.com/streaming-tree/server/internal/domain/engagement"
)

func validSettings() Settings {
	s := Default()
	s.ProviderMode = ProviderModeSystem
	return s
}

func TestValidateSettingsAcceptsDefault(t *testing.T) {
	if err := ValidateSettings(validSettings()); err != nil {
		t.Fatalf("ValidateSettings(default) = %v, want nil", err)
	}
}

func TestValidateSettingsRejectsUnknownProviderMode(t *testing.T) {
	s := validSettings()
	s.ProviderMode = ProviderMode("not-a-mode")
	requireValidationErr(t, ValidateSettings(s))
}

func TestValidateSettingsRejectsUnimplementedProviderModes(t *testing.T) {
	for _, mode := range []ProviderMode{ProviderModeLocal, ProviderModeCloud} {
		s := validSettings()
		s.ProviderMode = mode
		if err := ValidateSettings(s); !errors.Is(err, ErrValidation) {
			t.Errorf("ValidateSettings(mode=%s) = %v, want ErrValidation", mode, err)
		}
	}
}

func TestValidateSettingsRejectsUnknownEventType(t *testing.T) {
	s := validSettings()
	s.EnabledEventTypes = []engagement.Type{engagement.Type("not-a-real-type")}
	requireValidationErr(t, ValidateSettings(s))
}

func TestValidateSettingsRejectsDuplicateEventTypes(t *testing.T) {
	s := validSettings()
	s.EnabledEventTypes = []engagement.Type{engagement.TypeChatMessage, engagement.TypeChatMessage}
	requireValidationErr(t, ValidateSettings(s))
}

func TestValidateSettingsRejectsDuplicateProviderIDs(t *testing.T) {
	s := validSettings()
	s.EnabledProviderIDs = []engagement.ProviderID{engagement.ProviderTwitch, engagement.ProviderTwitch}
	requireValidationErr(t, ValidateSettings(s))
}

func TestValidateSettingsRejectsTooManySourceIDs(t *testing.T) {
	s := validSettings()
	ids := make([]string, MaxEnabledSourceIDs+1)
	for i := range ids {
		ids[i] = "src"
	}
	s.EnabledSourceIDs = ids
	requireValidationErr(t, ValidateSettings(s))
}

func TestValidateSettingsRejectsEmptySourceID(t *testing.T) {
	s := validSettings()
	s.EnabledSourceIDs = []string{""}
	requireValidationErr(t, ValidateSettings(s))
}

func TestValidateSettingsRejectsDuplicateSourceID(t *testing.T) {
	s := validSettings()
	s.EnabledSourceIDs = []string{"acct_1", "acct_1"}
	requireValidationErr(t, ValidateSettings(s))
}

func TestValidateSettingsRejectsOverlongSourceID(t *testing.T) {
	s := validSettings()
	s.EnabledSourceIDs = []string{strings.Repeat("a", MaxSourceIDLength+1)}
	requireValidationErr(t, ValidateSettings(s))
}

func TestValidateSettingsRejectsThresholdWithoutCurrency(t *testing.T) {
	s := validSettings()
	minimum := int64(1_000_000)
	s.ThresholdMinimumAmountMicros = &minimum
	s.ThresholdCurrency = ""
	requireValidationErr(t, ValidateSettings(s))
}

func TestValidateSettingsAcceptsThresholdWithCurrency(t *testing.T) {
	s := validSettings()
	minimum := int64(1_000_000)
	s.ThresholdMinimumAmountMicros = &minimum
	s.ThresholdCurrency = "USD"
	if err := ValidateSettings(s); err != nil {
		t.Fatalf("ValidateSettings() = %v, want nil", err)
	}
}

func TestValidateSettingsRejectsNegativeThresholdAmount(t *testing.T) {
	s := validSettings()
	minimum := int64(-1)
	s.ThresholdMinimumAmountMicros = &minimum
	s.ThresholdCurrency = "USD"
	requireValidationErr(t, ValidateSettings(s))
}

func TestValidateSettingsRejectsThresholdAmountAboveMax(t *testing.T) {
	s := validSettings()
	minimum := maxAmountMicros + 1
	s.ThresholdMinimumAmountMicros = &minimum
	s.ThresholdCurrency = "USD"
	requireValidationErr(t, ValidateSettings(s))
}

func TestValidateSettingsRejectsOverlongCurrency(t *testing.T) {
	s := validSettings()
	minimum := int64(1)
	s.ThresholdMinimumAmountMicros = &minimum
	s.ThresholdCurrency = strings.Repeat("X", MaxCurrencyLength+1)
	requireValidationErr(t, ValidateSettings(s))
}

func TestValidateSettingsRejectsNegativeBits(t *testing.T) {
	s := validSettings()
	bits := int64(-1)
	s.MinimumBits = &bits
	requireValidationErr(t, ValidateSettings(s))
}

func TestValidateSettingsMaxTextLengthBounds(t *testing.T) {
	s := validSettings()
	s.MaxTextLengthCodePoints = MinMaxTextLengthCodePoints - 1
	requireValidationErr(t, ValidateSettings(s))

	s = validSettings()
	s.MaxTextLengthCodePoints = MaxMaxTextLengthCodePoints + 1
	requireValidationErr(t, ValidateSettings(s))
}

func TestValidateSettingsCooldownBounds(t *testing.T) {
	s := validSettings()
	s.PerUserCooldownSeconds = MaxPerUserCooldownSeconds + 1
	requireValidationErr(t, ValidateSettings(s))

	s = validSettings()
	s.GlobalCooldownSeconds = MaxGlobalCooldownSeconds + 1
	requireValidationErr(t, ValidateSettings(s))

	s = validSettings()
	s.PerUserCooldownSeconds = -1
	requireValidationErr(t, ValidateSettings(s))
}

func TestValidateSettingsBlockedWordsBounds(t *testing.T) {
	s := validSettings()
	words := make([]string, MaxBlockedWords+1)
	for i := range words {
		words[i] = "word"
	}
	s.BlockedWords = words
	requireValidationErr(t, ValidateSettings(s))

	s = validSettings()
	s.BlockedWords = []string{""}
	requireValidationErr(t, ValidateSettings(s))

	s = validSettings()
	s.BlockedWords = []string{strings.Repeat("a", MaxBlockedWordLength+1)}
	requireValidationErr(t, ValidateSettings(s))
}

func TestValidateSettingsQueueCapacityBounds(t *testing.T) {
	s := validSettings()
	s.QueueCapacity = MinQueueCapacity - 1
	requireValidationErr(t, ValidateSettings(s))

	s = validSettings()
	s.QueueCapacity = MaxQueueCapacity + 1
	requireValidationErr(t, ValidateSettings(s))
}

func TestValidateSettingsVoiceIDAndLanguageBounds(t *testing.T) {
	s := validSettings()
	s.VoiceID = strings.Repeat("a", MaxVoiceIDLength+1)
	requireValidationErr(t, ValidateSettings(s))

	s = validSettings()
	s.Language = strings.Repeat("a", MaxLanguageLength+1)
	requireValidationErr(t, ValidateSettings(s))
}

func TestValidateSettingsSpeedBounds(t *testing.T) {
	for _, speed := range []float64{MinSpeed - 0.01, MaxSpeed + 0.01, math.NaN(), math.Inf(1), math.Inf(-1)} {
		s := validSettings()
		s.Speed = speed
		if err := ValidateSettings(s); !errors.Is(err, ErrValidation) {
			t.Errorf("ValidateSettings(speed=%v) = %v, want ErrValidation", speed, err)
		}
	}
}

func TestValidateSettingsVolumeBounds(t *testing.T) {
	for _, volume := range []float64{MinVolume - 0.01, MaxVolume + 0.01, math.NaN(), math.Inf(1)} {
		s := validSettings()
		s.Volume = volume
		if err := ValidateSettings(s); !errors.Is(err, ErrValidation) {
			t.Errorf("ValidateSettings(volume=%v) = %v, want ErrValidation", volume, err)
		}
	}
}

func TestNormalizeCurrencyUppercases(t *testing.T) {
	if got := NormalizeCurrency("usd"); got != "USD" {
		t.Errorf("NormalizeCurrency(usd) = %q, want USD", got)
	}
}

func requireValidationErr(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
}
