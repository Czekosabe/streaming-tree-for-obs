//go:build !windows && !darwin

// Fallback for platforms with no real packaged single-instance mechanism
// yet (e.g. Linux, ahead of Stage 20D). Windows has a real implementation
// (docs/windows-packaging.md §9) and so does macOS
// (docs/macos-packaging.md §11, singleinstance_darwin.go). This exists
// only so a Linux/other developer build keeps compiling. It always reports
// success (never blocks a second launch) - a real mechanism for that
// platform is out of scope until its own package is designed.
package singleinstance

// Acquire always succeeds on non-Windows builds.
func Acquire() (ok bool, release func(), err error) {
	return true, func() {}, nil
}
