//go:build linux

// Package singleinstance detects whether another packaged instance of
// Streaming Tree for OBS is already running (docs/linux-desktop-packaging.md
// §11).
//
// Linux implementation: an exclusive, non-blocking advisory flock(2) on a
// fixed, per-user, application-owned lock file - the same kernel primitive
// and the same "closing the fd releases the lock automatically, including
// on a crash" property singleinstance_darwin.go already relies on. Location
// policy: $XDG_RUNTIME_DIR/StreamingTree/.instance.lock when
// XDG_RUNTIME_DIR is set (session-scoped, mode 0700, automatically cleared
// on logout per the XDG Base Directory Specification - a natural fit for a
// live-process lock), falling back to the same per-user data directory
// internal/config.resolveDataDir already resolves
// (~/.config/StreamingTree/.instance.lock by default) when it is not. A
// plaintext PID file is deliberately not used as the proof of another
// instance: a PID number cannot prove the process holding it is actually
// Streaming Tree, only that some process has that number, whereas a live
// kernel-held flock can only be held by a live, actual owner.
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
// gets an equally isolated lock file instead of colliding with a real
// installed instance's lock.
const dataDirOverrideEnv = "STREAMING_TREE_DATA_DIR"

// runtimeDirEnv is the XDG Base Directory Specification's own environment
// variable for the current session's runtime directory.
const runtimeDirEnv = "XDG_RUNTIME_DIR"

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

// lockFilePath resolves the lock file's location: STREAMING_TREE_DATA_DIR
// (hermetic test isolation) wins if set; otherwise XDG_RUNTIME_DIR when
// present (session-scoped, self-cleaning); otherwise the same per-user data
// directory internal/config.resolveDataDir resolves.
func lockFilePath() (string, error) {
	if dir := os.Getenv(dataDirOverrideEnv); dir != "" {
		return filepath.Join(dir, lockFileName), nil
	}

	if runtimeDir := os.Getenv(runtimeDirEnv); runtimeDir != "" {
		return filepath.Join(runtimeDir, appDirName, lockFileName), nil
	}

	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf(
			"singleinstance: cannot determine the per-user configuration directory: %w", err)
	}
	return filepath.Join(base, appDirName, lockFileName), nil
}
