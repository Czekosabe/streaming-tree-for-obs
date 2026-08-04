package branch

import "testing"

func TestRedactorReplacesTheExactSecret(t *testing.T) {
	r := newRedactor("sk_live_super_secret")
	got := r.Redact("Opening 'rtmp://example.invalid/app/sk_live_super_secret' for writing")
	if contains(got, "sk_live_super_secret") {
		t.Errorf("secret survived redaction: %q", got)
	}
	if !contains(got, redactedPlaceholder) {
		t.Errorf("redacted text %q does not contain the placeholder", got)
	}
}

func TestRedactorReplacesTheFullDestinationURL(t *testing.T) {
	r := newRedactor("sk_live_abc", "rtmp://example.invalid/app/sk_live_abc")
	got := r.Redact("failed to connect to rtmp://example.invalid/app/sk_live_abc: timeout")
	if contains(got, "sk_live_abc") {
		t.Errorf("secret survived redaction: %q", got)
	}
}

func TestRedactorHandlesURLEscapedVariants(t *testing.T) {
	// A key containing characters a URL encoder would escape must still be
	// caught in its escaped form, since FFmpeg's own log lines sometimes
	// percent-encode a URL it prints.
	r := newRedactor("sk live/key")
	got := r.Redact("using key sk%20live%2Fkey now")
	if contains(got, "sk%20live%2Fkey") {
		t.Errorf("escaped secret survived redaction: %q", got)
	}
}

func TestRedactorWithNoSecretsIsANoOp(t *testing.T) {
	r := newRedactor()
	got := r.Redact("ordinary log line")
	if got != "ordinary log line" {
		t.Errorf("got %q", got)
	}
}

func TestRedactorOnANilReceiverIsANoOp(t *testing.T) {
	var r *Redactor
	got := r.Redact("ordinary log line")
	if got != "ordinary log line" {
		t.Errorf("got %q", got)
	}
}

func TestRedactorIgnoresEmptySecrets(t *testing.T) {
	r := newRedactor("", "sk_live_abc", "")
	got := r.Redact("key is sk_live_abc")
	if contains(got, "sk_live_abc") {
		t.Errorf("secret survived redaction: %q", got)
	}
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
