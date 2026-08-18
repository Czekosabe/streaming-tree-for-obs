//go:build !windows

// Non-Windows fallback - never actually reachable, since
// UnsupportedHandoff (handoff_other.go) never launches a helper on any
// platform other than Windows. Exists only so this package keeps
// compiling on every platform (docs/platform-support.md).
package updater

// runHelper always fails on non-Windows - update-helper mode is never
// invoked here in practice.
func runHelper(args HelperArgs) HandoffResult {
	return HandoffResult{Outcome: "platform_unsupported", ToVersion: args.ExpectedVersion}
}

// ValidateHelperArgs is a portable no-op stub matching the real
// Windows implementation's signature, so cmd/server/main.go's helper-
// mode detection compiles unconditionally on every platform.
func ValidateHelperArgs(parentPID int, candidate, targetExe, expectedVersion string) (HelperArgs, error) {
	return HelperArgs{
		ParentPID: parentPID, CandidatePath: candidate,
		TargetExePath: targetExe, ExpectedVersion: expectedVersion,
	}, nil
}

// Flag names, duplicated here (not in helper.go) only because the real
// Windows values live in helper_windows.go and a portable build needs
// them too - kept identical on purpose.
const (
	FlagUpdateHelper    = "update-helper"
	FlagParentPID       = "parent-pid"
	FlagCandidate       = "candidate"
	FlagTargetExe       = "target-exe"
	FlagExpectedVersion = "expected-version"
)
