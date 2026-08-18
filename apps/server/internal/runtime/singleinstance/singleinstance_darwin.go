//go:build darwin

// Package singleinstance detects whether another packaged instance of
// Streaming Tree for OBS is already running (docs/macos-packaging.md §11).
//
// macOS implementation: an exclusive, non-blocking advisory flock(2) on a
// fixed, per-user, application-owned lock file inside the same per-user
// data directory internal/config.resolveDataDir already resolves
// (~/Library/Application Support/StreamingTree/.instance.lock by default).
// flock's defining property - the same one Windows's own CreateMutexW
// mechanism relies on - is that the kernel releases the lock automatically
// the instant the owning process exits, for any reason including a crash,
// so there is no stale-lock state that can ever permanently block a future
// launch. A plaintext PID file alone is deliberately not used as the proof
// of another instance: a PID number cannot prove the process holding it is
// actually Streaming Tree, only that some process has that number, whereas
// a live kernel-held flock can only be held by a live, actual owner.
package singleinstance

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// appDirName mirrors internal/config.AppDirName - see that constant's own
// doc comment for why it is duplicated here rather than imported (this
// low-level runtime package must not depend upward on internal/config).
const appDirName = "StreamingTree"

// lockFileName is fixed and application-specific, never derived from user
// input.
const lockFileName = ".instance.lock"

// dataDirOverrideEnv mirrors the STREAMING_TREE_DATA_DIR override
// internal/config.resolveDataDir understands, so a hermetic test build
// (which points that variable at an isolated temporary directory) gets an
// equally isolated lock file instead of colliding with a real installed
// instance's lock.
const dataDirOverrideEnv = "STREAMING_TREE_DATA_DIR"

// Acquire attempts to become the one running packaged instance. ok is true
// when this process holds the lock (and therefore owns the application);
// ok is false when another instance already holds it - the caller must not
// start a second backend in that case. The returned release func should be
// deferred for the lifetime of the process when ok is true; it is a no-op
// otherwise.
func Acquire() (ok bool, release func(), err error) {
	path, pathErr := lockFilePath()
	if pathErr != nil {
		return false, func() {}, pathErr
	}

	if mkdirErr := os.MkdirAll(filepath.Dir(path), 0o700); mkdirErr != nil {
		return false, func() {}, fmt.Errorf("singleinstance: %w", mkdirErr)
	}

	file, openErr := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if openErr != nil {
		return false, func() {}, fmt.Errorf("singleinstance: %w", openErr)
	}

	if flockErr := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); flockErr != nil {
		_ = file.Close()
		if flockErr == unix.EWOULDBLOCK {
			// Another live process already holds the lock.
			return false, func() {}, nil
		}
		return false, func() {}, fmt.Errorf("singleinstance: %w", flockErr)
	}

	// Closing the fd releases the flock, including if the process later
	// crashes - the kernel does this unconditionally, so no explicit
	// unlock call is needed here beyond the eventual Close.
	return true, func() { _ = file.Close() }, nil
}

// lockFilePath resolves the lock file's location using the same
// override/default rule as internal/config.resolveDataDir.
func lockFilePath() (string, error) {
	if dir := os.Getenv(dataDirOverrideEnv); dir != "" {
		return filepath.Join(dir, lockFileName), nil
	}

	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf(
			"singleinstance: cannot determine the per-user configuration directory: %w", err)
	}
	return filepath.Join(base, appDirName, lockFileName), nil
}
