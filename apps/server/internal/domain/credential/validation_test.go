package credential

import (
	"strings"
	"testing"

	"github.com/streaming-tree/server/internal/domain/platform"
)

func violationRules(t *testing.T, err error, field string) []string {
	t.Helper()

	verr, ok := platform.AsValidationError(err)
	if !ok {
		t.Fatalf("error %v is not a *platform.ValidationError", err)
	}

	var rules []string
	for _, violation := range verr.Violations {
		if violation.Field == field {
			rules = append(rules, violation.Rule)
		}
	}
	return rules
}

func TestValidateStreamKeyTrimsSurroundingWhitespace(t *testing.T) {
	got, err := ValidateStreamKey("  sk_live_abc123  \t")
	if err != nil {
		t.Fatalf("ValidateStreamKey() error = %v, want nil", err)
	}
	if got != "sk_live_abc123" {
		t.Errorf("ValidateStreamKey() = %q, want %q", got, "sk_live_abc123")
	}
}

func TestValidateStreamKeyPreservesInternalWhitespace(t *testing.T) {
	// Trimming is for accidental surrounding whitespace only; an internal
	// space is not a control character and must survive untouched.
	got, err := ValidateStreamKey("sk live key")
	if err != nil {
		t.Fatalf("ValidateStreamKey() error = %v, want nil", err)
	}
	if got != "sk live key" {
		t.Errorf("ValidateStreamKey() = %q, want %q", got, "sk live key")
	}
}

func TestValidateStreamKeyRejectsEmpty(t *testing.T) {
	_, err := ValidateStreamKey("")
	if err == nil {
		t.Fatal("ValidateStreamKey(\"\") returned nil error")
	}
	rules := violationRules(t, err, "streamKey")
	if len(rules) != 1 || rules[0] != platform.RuleRequired {
		t.Errorf("rules = %v, want [%s]", rules, platform.RuleRequired)
	}
}

func TestValidateStreamKeyRejectsWhitespaceOnly(t *testing.T) {
	_, err := ValidateStreamKey("   \t  ")
	if err == nil {
		t.Fatal("ValidateStreamKey(whitespace) returned nil error")
	}
	rules := violationRules(t, err, "streamKey")
	if len(rules) != 1 || rules[0] != platform.RuleRequired {
		t.Errorf("rules = %v, want [%s]", rules, platform.RuleRequired)
	}
}

func TestValidateStreamKeyRejectsControlCharacters(t *testing.T) {
	_, err := ValidateStreamKey("sk_live_\x07abc")
	if err == nil {
		t.Fatal("ValidateStreamKey(control char) returned nil error")
	}
	rules := violationRules(t, err, "streamKey")
	if len(rules) != 1 || rules[0] != platform.RuleInvalid {
		t.Errorf("rules = %v, want [%s]", rules, platform.RuleInvalid)
	}
}

func TestValidateStreamKeyRejectsEmbeddedNewline(t *testing.T) {
	_, err := ValidateStreamKey("sk_live_abc\ndef")
	if err == nil {
		t.Fatal("ValidateStreamKey(embedded newline) returned nil error")
	}
	rules := violationRules(t, err, "streamKey")
	if len(rules) != 1 || rules[0] != platform.RuleInvalid {
		t.Errorf("rules = %v, want [%s]", rules, platform.RuleInvalid)
	}
}

func TestValidateStreamKeyRejectsEmbeddedCarriageReturn(t *testing.T) {
	_, err := ValidateStreamKey("sk_live_abc\rdef")
	if err == nil {
		t.Fatal("ValidateStreamKey(embedded CR) returned nil error")
	}
	rules := violationRules(t, err, "streamKey")
	if len(rules) != 1 || rules[0] != platform.RuleInvalid {
		t.Errorf("rules = %v, want [%s]", rules, platform.RuleInvalid)
	}
}

func TestValidateStreamKeyRejectsOversizedValue(t *testing.T) {
	oversized := strings.Repeat("a", MaxStreamKeyBytes+1)
	_, err := ValidateStreamKey(oversized)
	if err == nil {
		t.Fatal("ValidateStreamKey(oversized) returned nil error")
	}
	rules := violationRules(t, err, "streamKey")
	if len(rules) != 1 || rules[0] != platform.RuleTooLong {
		t.Errorf("rules = %v, want [%s]", rules, platform.RuleTooLong)
	}
}

func TestValidateStreamKeyAcceptsValueAtTheLimit(t *testing.T) {
	exact := strings.Repeat("a", MaxStreamKeyBytes)
	got, err := ValidateStreamKey(exact)
	if err != nil {
		t.Fatalf("ValidateStreamKey(exact limit) error = %v, want nil", err)
	}
	if got != exact {
		t.Error("ValidateStreamKey(exact limit) did not return the value unchanged")
	}
}

func TestValidateStreamKeyRejectsInvalidUTF8(t *testing.T) {
	_, err := ValidateStreamKey("sk_live_\xff\xfe")
	if err == nil {
		t.Fatal("ValidateStreamKey(invalid UTF-8) returned nil error")
	}
	rules := violationRules(t, err, "streamKey")
	if len(rules) != 1 || rules[0] != platform.RuleInvalid {
		t.Errorf("rules = %v, want [%s]", rules, platform.RuleInvalid)
	}
}

func TestValidateStreamKeyAcceptsPrintableUnicode(t *testing.T) {
	got, err := ValidateStreamKey("sk_ключ_密钥_🔑")
	if err != nil {
		t.Fatalf("ValidateStreamKey(unicode) error = %v, want nil", err)
	}
	if got != "sk_ключ_密钥_🔑" {
		t.Errorf("ValidateStreamKey(unicode) = %q, want unchanged", got)
	}
}

func TestValidateStreamKeyErrorNeverContainsTheRejectedValue(t *testing.T) {
	const secretLookingValue = "sk_live_super_secret_value_should_never_appear\x07"

	_, err := ValidateStreamKey(secretLookingValue)
	if err == nil {
		t.Fatal("expected a validation error")
	}
	if strings.Contains(err.Error(), "sk_live_super_secret_value_should_never_appear") {
		t.Fatalf("validation error leaked the rejected value: %v", err)
	}
}
