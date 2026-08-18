//go:build integration

// Integration-test-only escape hatch (docs/updater.md §1/§15/§38/§41).
//
// NewTestClient exists ONLY in a binary built with `-tags integration` -
// exactly like cmd/testserver's own reasoning for existing only that way.
// A normal `go build ./cmd/server` (no tags), the exact command
// scripts/build-release.ps1 runs and the only way Inno Setup's own
// installed content is ever produced, never sees this file at all, so
// `updater.NewTestClient` is not merely unused in production - it does
// not exist as a compiled symbol. This is the structural guarantee
// backing docs/updater.md §1's promise that no production code path can
// ever redirect the updater's GitHub API traffic.
package updater

// NewTestClient builds a Client pointed at an arbitrary base URL -
// used only by scripts/verify-updater.mjs, via a companion
// integration-only hook in cmd/server, to point a real, otherwise-
// unmodified server build at a local fake GitHub API server for that
// one hermetic test.
func NewTestClient(baseURL, installedVersion string) *Client {
	return newClient(baseURL, installedVersion)
}
