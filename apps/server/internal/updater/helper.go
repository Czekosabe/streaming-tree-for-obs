package updater

// HelperArgs is the closed, application-generated argument set the
// update helper accepts (docs/updater.md §22) - never an arbitrary
// executable, shell command, URL, or install flag. Populated only by
// WindowsHandoff.Begin and consumed only by RunHelper.
type HelperArgs struct {
	// ParentPID is the original application process's own process id,
	// captured before the helper does anything else so the parent-wait
	// (docs/updater.md §23) is race-free.
	ParentPID int
	// CandidatePath is the already-verified installer candidate's path
	// inside the update-staging subtree - re-validated by the helper,
	// never trusted at face value (docs/updater.md §22).
	CandidatePath string
	// TargetExePath is the installed application's own real executable
	// path, used both to re-derive the installed-context marker and to
	// restart the application after a successful install.
	TargetExePath string
	// ExpectedVersion is the version this update attempt targets - the
	// helper verifies the post-install executable actually reports this
	// exact version before declaring success (docs/updater.md §26).
	ExpectedVersion string
}

// Outcome codes for HelperResult.Outcome - the closed vocabulary from
// docs/updater.md §25.
const (
	OutcomeOK                 = "ok"
	OutcomeParentDidNotExit   = "parent_did_not_exit"
	OutcomeReverifyFailed     = "reverify_failed"
	OutcomeVersionMismatch    = "version_mismatch"
	OutcomeRestartFailed      = "restart_failed"
	outcomeInstallerFailedFmt = "installer_failed:%d"
)

// RunHelper executes the full external update-installer handoff
// (docs/updater.md §21): waits for the parent process to exit,
// re-verifies the staged candidate, runs the real Inno Setup installer
// silently, verifies the resulting installed version, restarts the
// application, and returns a result record. Implemented for real only
// on Windows (helper_windows.go) - see helper_other.go for the
// non-Windows stub, which is never reachable in practice since
// WindowsHandoff/UnsupportedHandoff never launches a helper on any
// other platform.
func RunHelper(args HelperArgs) HandoffResult {
	return runHelper(args)
}
