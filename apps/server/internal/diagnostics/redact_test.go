package diagnostics

import (
	"strings"
	"testing"
)

func TestRedactPath(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "chat overlay config",
			in:   "/api/public/chat-overlays/ab12cd34ef56/config",
			want: "/api/public/chat-overlays/{redacted}/config",
		},
		{
			name: "chat overlay stream, no trailing segment",
			in:   "/api/public/chat-overlays/ab12cd34ef56",
			want: "/api/public/chat-overlays/{redacted}",
		},
		{
			name: "alert profile config",
			in:   "/api/public/alert-profiles/some-secret-slug/config",
			want: "/api/public/alert-profiles/{redacted}/config",
		},
		{
			name: "widget stream",
			in:   "/api/public/widgets/widget-slug-value/stream",
			want: "/api/public/widgets/{redacted}/stream",
		},
		{
			name: "visual asset token",
			in:   "/api/public/visual-assets/tok_abcdef0123456789",
			want: "/api/public/visual-assets/{redacted}",
		},
		{
			name: "audio stream (single capability segment)",
			in:   "/api/public/audio/audio-slug/stream",
			want: "/api/public/audio/{redacted}/stream",
		},
		{
			name: "audio bytes (two capability segments)",
			in:   "/api/public/audio/audio-slug/bytes/one-time-token-value",
			want: "/api/public/audio/{redacted}/bytes/{redacted}",
		},
		{
			name: "audio ack",
			in:   "/api/public/audio/audio-slug/ack",
			want: "/api/public/audio/{redacted}/ack",
		},
		{
			name: "browser source overlay route",
			in:   "/overlay/chat/some-capability-slug",
			want: "/overlay/chat/{redacted}",
		},
		{
			name: "remote overlay management enable",
			in:   "/api/remote-overlay/chat-overlay/some-slug/enable",
			want: "/api/remote-overlay/chat-overlay/{redacted}/enable",
		},
		{
			name: "remote overlay management rotate",
			in:   "/api/remote-overlay/widget/some-slug/rotate",
			want: "/api/remote-overlay/widget/{redacted}/rotate",
		},
		{
			name: "unrelated path is untouched",
			in:   "/api/branches/123/start",
			want: "/api/branches/123/start",
		},
		{
			name: "root path is untouched",
			in:   "/",
			want: "/",
		},
		{
			name: "management api is untouched",
			in:   "/api/session/login",
			want: "/api/session/login",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RedactPath(tc.in)
			if got != tc.want {
				t.Errorf("RedactPath(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if strings.Contains(tc.name, "capability") || strings.Contains(tc.in, "slug") || strings.Contains(tc.in, "token") {
				if got == tc.in && tc.in != tc.want {
					t.Errorf("RedactPath(%q) did not redact the capability segment", tc.in)
				}
			}
		})
	}
}

func TestRedactText(t *testing.T) {
	cases := []struct {
		name           string
		in             string
		wantRedacted   bool
		mustNotContain string
	}{
		{
			name:           "long hex token is redacted",
			in:             "upstream rejected token 9f8e7d6c5b4a3928170615243342516278899aabbccddeeff0011223344",
			wantRedacted:   true,
			mustNotContain: "9f8e7d6c5b4a3928170615243342516278899aabbccddeeff0011223344",
		},
		{
			name:           "long base64url-shaped value is redacted",
			in:             "session lookup failed for cookie value AbCdEfGhIjKlMnOpQrStUvWxYz0123456789_-ABCDEFGH",
			wantRedacted:   true,
			mustNotContain: "AbCdEfGhIjKlMnOpQrStUvWxYz0123456789_-ABCDEFGH",
		},
		{
			name:           "rtmps user/pass query fragment is redacted",
			in:             "publish rejected for rtmps://ingest.example/live?user=abcd&pass=secretvalue",
			wantRedacted:   true,
			mustNotContain: "pass=secretvalue",
		},
		{
			name:         "short ordinary message is untouched",
			in:           "database ready",
			wantRedacted: false,
		},
		{
			name:         "short identifier is untouched",
			in:           "branch 123 started",
			wantRedacted: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RedactText(tc.in)
			if tc.mustNotContain != "" && strings.Contains(got, tc.mustNotContain) {
				t.Errorf("RedactText(%q) = %q, still contains %q", tc.in, got, tc.mustNotContain)
			}
			redacted := got != tc.in
			if redacted != tc.wantRedacted {
				t.Errorf("RedactText(%q) = %q, redacted=%v, want redacted=%v", tc.in, got, redacted, tc.wantRedacted)
			}
		})
	}
}
