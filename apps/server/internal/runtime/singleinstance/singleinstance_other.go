//go:build !windows

// Non-Windows fallback. Stage 20A's actual packaging target is
// Windows-first (docs/windows-packaging.md §9); this exists only so a
// non-Windows developer build keeps compiling. It always reports success
// (never blocks a second launch) - a real cross-platform single-instance
// mechanism is out of scope until a non-Windows package is designed.
package singleinstance

// Acquire always succeeds on non-Windows builds.
func Acquire() (ok bool, release func(), err error) {
	return true, func() {}, nil
}
