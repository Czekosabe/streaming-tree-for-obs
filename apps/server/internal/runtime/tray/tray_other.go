//go:build !windows

package tray

import "errors"

// ErrUnsupported is returned by Run on every platform other than
// Windows - there is no tray implementation here (see tray.go).
var ErrUnsupported = errors.New("tray: not supported on this platform")

// Run always fails on non-Windows platforms.
func Run(Options) (Handle, error) {
	return nil, ErrUnsupported
}
