//go:build !windows

// Non-Windows fallback. Stage 20B's actual install-handoff target is
// Windows-first, exactly like every other packaged-lifecycle piece
// since Stage 20A (docs/windows-packaging.md, docs/platform-support.md)
// - this exists only so a non-Windows developer build keeps compiling.
// It always reports unavailable; nothing here ever installs anything.
package updater

import (
	"context"
	"errors"
)

// UnsupportedHandoff is the non-Windows Handoff implementation - always
// unavailable.
type UnsupportedHandoff struct{}

// NewPlatformHandoff returns the non-Windows stub.
func NewPlatformHandoff() Handoff { return UnsupportedHandoff{} }

func (UnsupportedHandoff) Available() (bool, string) {
	return false, BlockerPlatformUnsupported
}

func (UnsupportedHandoff) Begin(ctx context.Context, candidatePath, expectedVersion string) error {
	return errors.New("update installation is not supported on this platform")
}
