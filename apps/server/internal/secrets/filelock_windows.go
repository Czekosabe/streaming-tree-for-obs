//go:build windows

package secrets

import "os"

// Windows never runs Stage 20D2A headless mode (docs/linux-headless-
// server.md is a Linux/systemd-specific design) - this exists only so
// the cross-platform secrets package keeps compiling. HeadlessStore's
// own in-process mutex is the only synchronization that could ever
// matter here.
func flockFile(f *os.File) error   { return nil }
func unflockFile(f *os.File) error { return nil }
