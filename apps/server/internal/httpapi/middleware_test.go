package httpapi

import "testing"

func TestRedactLoggedPathHidesThePublicOverlaySlug(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"config route", "/api/public/chat-overlays/abc123def456/config", "/api/public/chat-overlays/{slug}/config"},
		{"items route", "/api/public/chat-overlays/abc123def456/items", "/api/public/chat-overlays/{slug}/items"},
		{"stream route", "/api/public/chat-overlays/abc123def456/stream", "/api/public/chat-overlays/{slug}/stream"},
		{"slug only, no trailing segment", "/api/public/chat-overlays/abc123def456", "/api/public/chat-overlays/{slug}"},
		{"unrelated path is unchanged", "/api/chat-overlays/ov_1", "/api/chat-overlays/ov_1"},
		{"unrelated public-looking path is unchanged", "/api/public/other-thing/abc", "/api/public/other-thing/abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := redactLoggedPath(tt.path); got != tt.want {
				t.Errorf("redactLoggedPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
