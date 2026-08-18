//go:build !windows && !darwin && !linux

// Fallback for platforms with no real packaged single-instance mechanism
// (e.g. the BSDs). Windows, macOS (docs/macos-packaging.md §11,
// singleinstance_darwin.go), and Linux (docs/linux-desktop-packaging.md
// §11, singleinstance_linux.go) all have real implementations. This exists
// only so an unusual developer build keeps compiling. It always reports
// success (never blocks a second launch) - a real mechanism for that
// platform is out of scope until its own package is designed.
package singleinstance

// Acquire always succeeds on non-Windows builds.
func Acquire() (ok bool, release func(), err error) {
	return true, func() {}, nil
}
