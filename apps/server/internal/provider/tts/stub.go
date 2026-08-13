//go:build !windows

// Non-Windows stub: the repository builds and tests on every platform
// (docs/audio-tts.md §3/governing task §8), but Stage 17A's only real
// system-TTS provider is Windows SAPI. This file honestly reports
// unavailability rather than faking a macOS `say`/Linux shell-command
// provider as false parity.
package tts

import (
	"context"
	"fmt"
)

const unavailableReason = "system text-to-speech is only implemented on Windows"

// SystemProvider is the non-Windows stand-in for the Windows SAPI
// provider - always reports Capabilities.Available == false, and every
// other method fails with ErrUnavailable. Never shells out to an
// external command; never fakes a working provider.
type SystemProvider struct{}

// NewSystemProvider builds the non-Windows stub provider.
func NewSystemProvider() *SystemProvider { return &SystemProvider{} }

var _ Provider = (*SystemProvider)(nil)

func (p *SystemProvider) Capabilities() Capabilities {
	return Capabilities{Available: false, Reason: unavailableReason}
}

func (p *SystemProvider) ListVoices(ctx context.Context) ([]Voice, error) {
	return nil, fmt.Errorf("%w: %s", ErrUnavailable, unavailableReason)
}

func (p *SystemProvider) Synthesize(ctx context.Context, in SynthesizeInput) (SynthesizeResult, error) {
	return SynthesizeResult{}, fmt.Errorf("%w: %s", ErrUnavailable, unavailableReason)
}
