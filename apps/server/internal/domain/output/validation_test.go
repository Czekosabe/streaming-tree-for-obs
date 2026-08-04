package output

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

func TestValidateServerURLAcceptsEmptyAsNotConfigured(t *testing.T) {
	got, err := ValidateServerURL("")
	if err != nil {
		t.Fatalf("ValidateServerURL(\"\") error = %v, want nil", err)
	}
	if got != "" {
		t.Errorf("got = %q, want empty", got)
	}
}

func TestValidateServerURLAcceptsWhitespaceOnlyAsNotConfigured(t *testing.T) {
	got, err := ValidateServerURL("   \t  ")
	if err != nil {
		t.Fatalf("ValidateServerURL(whitespace) error = %v, want nil", err)
	}
	if got != "" {
		t.Errorf("got = %q, want empty", got)
	}
}

func TestValidateServerURLAcceptsValidRTMP(t *testing.T) {
	got, err := ValidateServerURL("  rtmp://live.example.invalid/app  ")
	if err != nil {
		t.Fatalf("ValidateServerURL() error = %v", err)
	}
	if got != "rtmp://live.example.invalid/app" {
		t.Errorf("got = %q, want trimmed value", got)
	}
}

func TestValidateServerURLAcceptsValidRTMPS(t *testing.T) {
	_, err := ValidateServerURL("rtmps://live.example.invalid:443/app/instance")
	if err != nil {
		t.Fatalf("ValidateServerURL() error = %v", err)
	}
}

func TestValidateServerURLRejectsUnsupportedScheme(t *testing.T) {
	_, err := ValidateServerURL("https://example.invalid/app")
	if err == nil {
		t.Fatal("an https:// URL was accepted")
	}
	rules := violationRules(t, err, "serverUrl")
	if len(rules) != 1 || rules[0] != platform.RuleInvalid {
		t.Errorf("rules = %v, want [%s]", rules, platform.RuleInvalid)
	}
}

func TestValidateServerURLRejectsMissingHost(t *testing.T) {
	_, err := ValidateServerURL("rtmp:///app")
	if err == nil {
		t.Fatal("a URL with no host was accepted")
	}
	rules := violationRules(t, err, "serverUrl")
	if len(rules) != 1 || rules[0] != platform.RuleRequired {
		t.Errorf("rules = %v, want [%s]", rules, platform.RuleRequired)
	}
}

func TestValidateServerURLRejectsInvalidPort(t *testing.T) {
	_, err := ValidateServerURL("rtmp://example.invalid:999999/app")
	if err == nil {
		t.Fatal("an out-of-range port was accepted")
	}
}

func TestValidateServerURLRejectsUserinfo(t *testing.T) {
	_, err := ValidateServerURL("rtmp://user:pass@example.invalid/app")
	if err == nil {
		t.Fatal("userinfo in the URL was accepted")
	}
}

func TestValidateServerURLRejectsFragment(t *testing.T) {
	_, err := ValidateServerURL("rtmp://example.invalid/app#fragment")
	if err == nil {
		t.Fatal("a fragment was accepted")
	}
}

func TestValidateServerURLRejectsQueryString(t *testing.T) {
	_, err := ValidateServerURL("rtmp://example.invalid/app?token=abc")
	if err == nil {
		t.Fatal("a query string was accepted")
	}
}

func TestValidateServerURLRejectsControlCharacters(t *testing.T) {
	_, err := ValidateServerURL("rtmp://example.invalid/app\x07")
	if err == nil {
		t.Fatal("a control character was accepted")
	}
}

func TestValidateServerURLRejectsEmbeddedNewline(t *testing.T) {
	_, err := ValidateServerURL("rtmp://example.invalid/app\nEvil-Header: x")
	if err == nil {
		t.Fatal("an embedded newline was accepted")
	}
}

func TestValidateServerURLRejectsOversizedValue(t *testing.T) {
	huge := "rtmp://example.invalid/" + strings.Repeat("a", MaxServerURLBytes)
	_, err := ValidateServerURL(huge)
	if err == nil {
		t.Fatal("an oversized value was accepted")
	}
	rules := violationRules(t, err, "serverUrl")
	if len(rules) != 1 || rules[0] != platform.RuleTooLong {
		t.Errorf("rules = %v, want [%s]", rules, platform.RuleTooLong)
	}
}

func TestValidateServerURLAllowsAPath(t *testing.T) {
	// Providers commonly use an application path as part of the server
	// address; this must not be rejected.
	got, err := ValidateServerURL("rtmp://example.invalid/live/app-instance")
	if err != nil {
		t.Fatalf("ValidateServerURL() error = %v", err)
	}
	if got != "rtmp://example.invalid/live/app-instance" {
		t.Errorf("got = %q", got)
	}
}

func TestValidateServerURLAcceptsUnicodeHost(t *testing.T) {
	// An IDN host is legitimate input; it must not be rejected by the
	// control-character scan or the generic parser.
	_, err := ValidateServerURL("rtmp://例え.invalid/app")
	if err != nil {
		t.Fatalf("ValidateServerURL() error = %v, want nil for a Unicode host", err)
	}
}

func TestValidateServerURLErrorNeverContainsAStreamKeyLookingSuffix(t *testing.T) {
	// This field never receives a stream key, but the error message itself
	// must not echo back attacker-supplied content either.
	const suspicious = "rtmp://example.invalid/app?key=sk_live_should_not_appear"
	_, err := ValidateServerURL(suspicious)
	if err == nil {
		t.Fatal("expected a validation error")
	}
	if strings.Contains(err.Error(), "sk_live_should_not_appear") {
		t.Errorf("validation error echoed the rejected value: %v", err)
	}
}
