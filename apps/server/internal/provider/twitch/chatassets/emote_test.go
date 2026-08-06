package chatassets

import "testing"

func TestEmoteImageURLBuildsTheDocumentedTemplate(t *testing.T) {
	got := EmoteImageURL("emotesv2_abc123")
	want := "https://static-cdn.jtvnw.net/emoticons/v2/emotesv2_abc123/static/dark/2.0"
	if got != want {
		t.Errorf("EmoteImageURL() = %q, want %q", got, want)
	}
}

func TestEmoteImageURLEscapesTheID(t *testing.T) {
	got := EmoteImageURL("weird id/with?chars")
	if got == "" {
		t.Fatal("EmoteImageURL() = \"\", want a URL")
	}
	want := "https://static-cdn.jtvnw.net/emoticons/v2/weird%20id%2Fwith%3Fchars/static/dark/2.0"
	if got != want {
		t.Errorf("EmoteImageURL() = %q, want %q", got, want)
	}
}

func TestEmoteImageURLEmptyIDReturnsEmpty(t *testing.T) {
	if got := EmoteImageURL(""); got != "" {
		t.Errorf("EmoteImageURL(\"\") = %q, want empty so callers fall back to fragment text", got)
	}
}
