//go:build !windows

package secrets

import (
	"os"

	"golang.org/x/sys/unix"
)

// flockFile takes a real, blocking, exclusive flock(2) on f - released
// automatically by the kernel if the holding process dies, exactly
// like every other flock-based lock this codebase already uses
// (internal/runtime/singleinstance).
func flockFile(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_EX)
}

func unflockFile(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_UN)
}
