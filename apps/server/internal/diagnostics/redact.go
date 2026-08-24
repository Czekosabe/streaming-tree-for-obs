// Package diagnostics implements Stage 20E's own bounded, redacted
// operator diagnostics surface: a ring buffer capturing recent log
// records (never a second logging universe - it wraps the existing
// single slog.Logger, docs/final-hardening.md §A), the centralized
// redaction this stage's own audit found the pre-existing per-call-site
// redaction (internal/httpapi/middleware.go's redactLoggedPath) did not
// fully cover, and the support-bundle generator.
package diagnostics

import (
	"regexp"
	"strings"
)

// capabilityPathPrefix is one route prefix under which the path
// segment(s) immediately following are capability-shaped values (a
// public slug or a one-time token) that must never reach a log line,
// a panic report, or a support bundle. Each real route this project
// registers under /api/public/*, /overlay/*, or /api/remote-overlay/*
// is listed explicitly here - never inferred from a generic pattern,
// so a newly added route is a real, reviewable addition to this list,
// not something the redactor is expected to guess about.
type capabilityPathPrefix struct {
	prefix string
	// extraSegmentAfter, when non-empty, names a second path segment
	// (e.g. "bytes") after which yet another capability-shaped value
	// follows - apps/server/internal/httpapi/audio.go's own
	// /api/public/audio/{slug}/bytes/{token} route is the one real
	// case of this in the current route set.
	extraSegmentAfter string
}

// knownCapabilityPathPrefixes enumerates every real path prefix in
// apps/server/internal/httpapi whose next segment is a capability
// value. Sourced directly from the real mux.HandleFunc registrations
// (chatoverlay.go, alerts.go, audio.go, public_widgets.go,
// visualasset.go, remote_overlay.go, remote_overlay_management.go),
// not assumed.
var knownCapabilityPathPrefixes = []capabilityPathPrefix{
	{prefix: "/api/public/chat-overlays/"},
	{prefix: "/api/public/alert-profiles/"},
	{prefix: "/api/public/widgets/"},
	{prefix: "/api/public/visual-assets/"},
	{prefix: "/api/public/audio/", extraSegmentAfter: "bytes"},
	{prefix: "/overlay/chat/"},
	{prefix: "/overlay/alerts/"},
	{prefix: "/overlay/audio/"},
	{prefix: "/overlay/widgets/"},
	{prefix: "/api/remote-overlay/chat-overlay/"},
	{prefix: "/api/remote-overlay/alert-profile/"},
	{prefix: "/api/remote-overlay/audio/"},
	{prefix: "/api/remote-overlay/widget/"},
}

const redactedSegment = "{redacted}"

// RedactPath replaces every capability-shaped path segment in path
// with a fixed placeholder, for every known route shape - the
// centralized replacement for internal/httpapi/middleware.go's own
// redactLoggedPath, which this stage's audit found covered only the
// chat-overlay case, leaving visual-asset tokens, remote-overlay
// capability paths, and the alert-profile/audio/widget public slugs
// unredacted in both the access log and the panic-recovery log.
func RedactPath(path string) string {
	for _, known := range knownCapabilityPathPrefixes {
		if !strings.HasPrefix(path, known.prefix) {
			continue
		}
		rest := path[len(known.prefix):]
		slashIdx := strings.IndexByte(rest, '/')
		var redacted, tail string
		if slashIdx < 0 {
			redacted, tail = known.prefix+redactedSegment, ""
		} else {
			redacted, tail = known.prefix+redactedSegment, rest[slashIdx:]
		}
		if known.extraSegmentAfter != "" {
			tail = redactExtraSegment(tail, known.extraSegmentAfter)
		}
		return redacted + tail
	}
	return path
}

// redactExtraSegment redacts the path segment immediately following a
// literal "/"+name+"/" occurrence in tail - apps/server/internal/
// httpapi/audio.go's own /api/public/audio/{slug}/bytes/{token} is
// the one real route with two capability-shaped segments.
func redactExtraSegment(tail, name string) string {
	marker := "/" + name + "/"
	idx := strings.Index(tail, marker)
	if idx < 0 {
		return tail
	}
	after := tail[idx+len(marker):]
	if slashIdx := strings.IndexByte(after, '/'); slashIdx >= 0 {
		return tail[:idx+len(marker)] + redactedSegment + after[slashIdx:]
	}
	return tail[:idx+len(marker)] + redactedSegment
}

// secretShapedPattern is a defense-in-depth scan for values that look
// like a capability token, session identifier, or credential even
// when appearing in free-text (an error message, a panic value) that
// RedactPath never sees, because it is not a request path at all.
// docs/final-hardening.md §B: no real call site logs a raw secret
// value today (confirmed by direct audit and by the existing
// TestAccessLogNeverContainsOAuthSecrets regression test) - this
// exists so a future one that interpolates an error containing one
// does not silently leak it. Matches:
//   - a long hex run (32+ chars: sha256 verifiers, capability tokens);
//   - a long base64url run (32+ chars: session IDs, CSRF tokens,
//     remote-overlay/visual-asset tokens, all generated as base64url
//     random values elsewhere in this codebase);
//   - user=...&pass=... query fragments (the RTMPS credential shape).
var secretShapedPattern = regexp.MustCompile(
	`(?i)[0-9a-f]{32,}|[A-Za-z0-9_-]{32,}|([?&]user=[^&\s]*&pass=[^&\s]*)`,
)

// RedactText scans free-text for secret-shaped substrings and replaces
// each with a fixed placeholder. Never applied to structured fields
// that are already known-safe (status codes, durations, subsystem
// names) - only to message/detail text that could echo untrusted or
// credential-bearing content.
func RedactText(s string) string {
	return secretShapedPattern.ReplaceAllString(s, redactedSegment)
}
