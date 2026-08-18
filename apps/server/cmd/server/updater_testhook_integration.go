//go:build integration

// Integration-test-only hook (docs/updater.md §1/§15/§38/§41): lets
// scripts/verify-updater.mjs point this build's updater at a local
// fake GitHub API server, so the real end-to-end check/download/
// verify/install/restart cycle can be exercised without ever
// contacting the real GitHub API.
//
// Invisible to `go build ./cmd/server` (no tags) - the exact command
// scripts/build-release.ps1 runs, and the only way Inno Setup's own
// installed content is ever produced. A build made this way is
// otherwise identical to the real production binary in every other
// respect (real SQLite, real MediaMTX, real credential store, real
// routing) - only this one escape hatch, and only when the operator
// explicitly builds with `-tags integration` AND sets the env var
// below, exists at all.
package main

import (
	"os"

	"github.com/streaming-tree/server/internal/updater"
)

// testUpdateAPIBaseURLEnv is read only by an integration-tagged build,
// and only ever set by scripts/verify-updater.mjs in its own isolated
// test process environment - never a documented, supported, or
// production-facing configuration variable.
const testUpdateAPIBaseURLEnv = "STREAMING_TREE_TEST_UPDATE_API_BASE_URL"

func init() {
	baseURL := os.Getenv(testUpdateAPIBaseURLEnv)
	if baseURL == "" {
		return
	}
	newUpdaterClient = func(installedVersion string) *updater.Client {
		return updater.NewTestClient(baseURL, installedVersion)
	}
}
