package updater

import "context"

// HandoffResult is the outcome of a completed install attempt, surfaced
// once after restart (docs/updater.md §25/§26). Outcome is one of the
// closed vocabulary that section defines: "ok",
// "parent_did_not_exit", "reverify_failed", "installer_failed:<exit
// code>", "version_mismatch", "restart_failed".
type HandoffResult struct {
	Outcome     string
	FromVersion string
	ToVersion   string
}

// Handoff begins the external update-installer handoff (docs/updater.md
// §19/§21/§22) - implemented for real only on Windows
// (handoff_windows.go, added in a later commit); the non-Windows stub
// (handoff_other.go) always reports unavailable, since Stage 20B never
// installs anything outside Windows x64 (docs/platform-support.md).
type Handoff interface {
	// Available reports whether this platform/build can actually
	// perform an install handoff right now, and if not, the stable
	// blocker code explaining why (BlockerNotInstalledCtx or
	// BlockerPlatformUnsupported).
	Available() (ok bool, blockerCode string)

	// Begin starts the handoff: copies the helper, launches it with the
	// closed, application-generated argument set from docs/updater.md
	// §22, and returns once the helper has been launched successfully -
	// not once installation finishes. The caller is expected to begin
	// application shutdown immediately after this returns nil.
	Begin(ctx context.Context, candidatePath, expectedVersion string) error
}
