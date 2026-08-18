// Package webassets embeds the production frontend build and the canonical
// legal documents into the Go binary, so a packaged Stage 20A release is a
// single executable with no sidecar files (see docs/windows-packaging.md
// §2/§16).
//
// On a clean checkout, `embedded/` and `legal/` contain only a tracked
// `.gitkeep` placeholder each - go build/go test/go vet never require Node,
// npm, or a Vite build to succeed. Only the release build script
// (scripts/build-release.ps1) overwrites these directories with the real
// `apps/web/dist` output and the repository-root legal files immediately
// before building the release binary, so the embedded content is never
// committed and ordinary development is completely unaffected.
package webassets

import (
	"embed"
	"io/fs"
)

//go:embed all:embedded
var frontendFS embed.FS

//go:embed all:legal
var legalFS embed.FS

// Frontend returns the embedded production frontend build, rooted at its own
// files (index.html, assets/...) rather than at "embedded/...". On a clean
// checkout (no release build has run) it contains only the placeholder file.
func Frontend() fs.FS {
	sub, err := fs.Sub(frontendFS, "embedded")
	if err != nil {
		// fs.Sub only fails for a malformed argument, never a missing
		// directory - "embedded" is a compile-time constant matching the
		// //go:embed directive above, so this can never actually happen.
		panic(err)
	}
	return sub
}

// Legal returns the embedded canonical legal documents (LICENSE,
// THIRD_PARTY_NOTICES.md, LEGAL.md, PRIVACY.md), rooted at "legal/...".
func Legal() fs.FS {
	sub, err := fs.Sub(legalFS, "legal")
	if err != nil {
		panic(err)
	}
	return sub
}
