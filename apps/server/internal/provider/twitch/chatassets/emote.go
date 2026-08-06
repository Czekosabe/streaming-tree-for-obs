// Package chatassets resolves Twitch chat badge and emote references to
// safe, renderable image URLs for the Stage 9 operator-chat presentation
// layer. See docs/provider-integrations/twitch-engagement.md's Stage 9
// addendum for the full research and design rationale this package
// implements.
//
// Deliberately a presentation-layer concern, not part of
// internal/operatorchat: that package stays provider-independent and never
// imports this one - see internal/operatorchat's own package doc comment.
package chatassets

import "net/url"

// emoteCDNBase is Twitch's documented emote image template with format,
// theme_mode, and scale fixed to safe, universal values - see the Stage 9
// addendum's "Emote-resolution strategy" section for why no catalog fetch,
// cache, or per-emote metadata is needed to build this URL.
const emoteCDNBase = "https://static-cdn.jtvnw.net/emoticons/v2/"

// EmoteImageURL builds a chat emote's CDN image URL directly from its
// emote id (internal/domain/engagement.Fragment.EmoteID) - a pure,
// request-free function, never a cache lookup. Returns "" for an empty id,
// so a caller can safely fall back to the fragment's own text.
func EmoteImageURL(emoteID string) string {
	if emoteID == "" {
		return ""
	}
	return emoteCDNBase + url.PathEscape(emoteID) + "/static/dark/2.0"
}
