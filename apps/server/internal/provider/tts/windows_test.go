//go:build windows

package tts

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// --- deterministic unit tests (no COM/SAPI required) --------------------

func TestSpeedToSAPIRateBounds(t *testing.T) {
	if got := speedToSAPIRate(1.0); got != 0 {
		t.Errorf("speedToSAPIRate(1.0) = %d, want 0", got)
	}
	if got := speedToSAPIRate(0.5); got < sapiRateMin || got > sapiRateMax {
		t.Errorf("speedToSAPIRate(0.5) = %d, want within [%d,%d]", got, sapiRateMin, sapiRateMax)
	}
	if got := speedToSAPIRate(2.0); got < sapiRateMin || got > sapiRateMax {
		t.Errorf("speedToSAPIRate(2.0) = %d, want within [%d,%d]", got, sapiRateMin, sapiRateMax)
	}
}

func TestVolumeToSAPIVolumeBounds(t *testing.T) {
	if got := volumeToSAPIVolume(0.0); got != sapiVolumeMin {
		t.Errorf("volumeToSAPIVolume(0.0) = %d, want %d", got, sapiVolumeMin)
	}
	if got := volumeToSAPIVolume(1.0); got != sapiVolumeMax {
		t.Errorf("volumeToSAPIVolume(1.0) = %d, want %d", got, sapiVolumeMax)
	}
	if got := volumeToSAPIVolume(-1); got != sapiVolumeMin {
		t.Errorf("volumeToSAPIVolume(-1) = %d, want clamped to %d", got, sapiVolumeMin)
	}
	if got := volumeToSAPIVolume(2); got != sapiVolumeMax {
		t.Errorf("volumeToSAPIVolume(2) = %d, want clamped to %d", got, sapiVolumeMax)
	}
}

func TestSanitizeTruncatesLongMessages(t *testing.T) {
	longErr := errors.New(strings.Repeat("x", 500))
	got := sanitize(longErr)
	if len(got) > 200 {
		t.Errorf("sanitize() returned %d characters, want <= 200", len(got))
	}
}

func TestSanitizeNilError(t *testing.T) {
	if got := sanitize(nil); got != "" {
		t.Errorf("sanitize(nil) = %q, want empty", got)
	}
}

// PRE-20D2B.1: the zero-installed-voices contract (Capabilities()
// reports unavailable, never a crash or a silently-empty "available"
// provider) is deterministic product behavior and belongs in the hard
// gate - unlike the SAPI-dependent tests below, it needs no real COM/
// SAPI engine at all.
func TestVoiceCountAvailableZero(t *testing.T) {
	if err := voiceCountAvailable(0); err == nil {
		t.Error("voiceCountAvailable(0) = nil, want a non-nil error")
	}
}

func TestVoiceCountAvailableNegative(t *testing.T) {
	// SAPI's own Count property is never observed negative in
	// practice, but the check is a plain "<= 0" specifically so a
	// negative value (a malformed/unexpected COM response) is treated
	// the same as zero rather than as "available".
	if err := voiceCountAvailable(-1); err == nil {
		t.Error("voiceCountAvailable(-1) = nil, want a non-nil error")
	}
}

func TestVoiceCountAvailablePositive(t *testing.T) {
	if err := voiceCountAvailable(1); err != nil {
		t.Errorf("voiceCountAvailable(1) = %v, want nil", err)
	}
}

// --- best-effort real-host SAPI smoke tests -------------------------------
//
// PRE-20D2B.1 classification (docs/ci-reliability.md's own Windows CI
// investigation): these are host-capability integration smoke tests,
// not deterministic provider-correctness tests - they prove "this
// particular Windows host currently has a usable SAPI voice engine",
// which is real, meaningful integration evidence, but is not something
// this package's own code can guarantee about every machine/CI runner
// it happens to execute on. The deterministic parts of this package's
// own contract (bounds conversion above, the zero-voice decision
// above, and internal/audio's own tests against a fake Provider) never
// depend on real SAPI and always run as part of the hard gate. These
// never send audio to speakers (AudioOutputStream is always an
// in-memory SpMemoryStream) and never depend on one particular
// installed voice name (governing task §71). If SAPI reports itself
// unavailable in this environment, every test below skips rather than
// fails.

func skipIfSAPIUnavailable(t *testing.T, p *SystemProvider) {
	t.Helper()
	caps := p.Capabilities()
	t.Logf("SAPI capabilities: available=%v reason=%q", caps.Available, caps.Reason)
	if !caps.Available {
		t.Skip("SAPI not available in this environment - best-effort smoke test only")
	}
}

func TestSystemProviderListVoicesSmoke(t *testing.T) {
	p := NewSystemProvider()
	skipIfSAPIUnavailable(t, p)

	// PRE-20D2B.1: ListVoices does strictly more COM work per call than
	// Capabilities' own fast availability check (an extra
	// GetProperty("Voice") plus, per installed voice token, an
	// Item/GetDescription/GetAttribute×2 round trip) - a real,
	// audited asymmetry, not an arbitrary number. 20s (up from 5s)
	// gives that proportionally larger amount of real COM/OS work
	// genuine headroom under CI-runner virtualization/contention
	// variance that this package cannot control, rather than assuming
	// this call is exactly as fast as the simpler Capabilities check.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	voices, err := p.ListVoices(ctx)
	if err != nil {
		t.Fatalf("ListVoices() error = %v", err)
	}
	if len(voices) == 0 {
		t.Fatal("ListVoices() returned no voices despite Capabilities().Available == true")
	}
	defaultCount := 0
	for _, v := range voices {
		if v.ID == "" {
			t.Error("a voice has an empty ID")
		}
		if v.IsDefault {
			defaultCount++
		}
	}
	if defaultCount == 0 {
		t.Log("no voice was marked IsDefault (non-fatal - depends on the local SAPI configuration)")
	}
}

func TestSystemProviderSynthesizeToMemorySmoke(t *testing.T) {
	p := NewSystemProvider()
	skipIfSAPIUnavailable(t, p)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := p.Synthesize(ctx, SynthesizeInput{Text: "test", Speed: 1.0, Volume: 1.0})
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	if len(result.Audio) == 0 {
		t.Error("Synthesize() returned empty audio bytes")
	}
	if result.ContentType != "audio/wav" {
		t.Errorf("ContentType = %q, want audio/wav", result.ContentType)
	}
	// SpMemoryStream.GetData returns raw PCM with no self-describing
	// container - this asserts the RIFF/WAVE header this provider
	// prepends is actually present, guarding against the exact bug
	// found and fixed while implementing this file (a plain byte count
	// check alone would not have caught unplayable raw-PCM output).
	if len(result.Audio) < 44 {
		t.Fatalf("Synthesize() returned %d bytes, want at least a 44-byte WAV header", len(result.Audio))
	}
	if string(result.Audio[0:4]) != "RIFF" {
		t.Errorf("Synthesize() audio does not start with RIFF: %q", result.Audio[0:4])
	}
	if string(result.Audio[8:12]) != "WAVE" {
		t.Errorf("Synthesize() audio missing WAVE marker: %q", result.Audio[8:12])
	}
	if string(result.Audio[36:40]) != "data" {
		t.Errorf("Synthesize() audio missing data chunk marker: %q", result.Audio[36:40])
	}
}

func TestSystemProviderSynthesizeUnknownVoiceRejected(t *testing.T) {
	p := NewSystemProvider()
	skipIfSAPIUnavailable(t, p)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := p.Synthesize(ctx, SynthesizeInput{Text: "test", VoiceID: "not-a-real-voice-id", Speed: 1.0, Volume: 1.0})
	if !errors.Is(err, ErrVoiceNotFound) {
		t.Errorf("Synthesize() error = %v, want ErrVoiceNotFound", err)
	}
}

func TestSystemProviderSynthesizeCancellation(t *testing.T) {
	p := NewSystemProvider()
	skipIfSAPIUnavailable(t, p)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	longText := strings.Repeat("testing cancellation behavior. ", 200)
	_, err := p.Synthesize(ctx, SynthesizeInput{Text: longText, Speed: 1.0, Volume: 1.0})
	if err == nil {
		t.Fatal("Synthesize() error = nil for an immediately-expired context, want a context/cancellation error")
	}
}

// TestSystemProviderListVoicesCancellation exercises the fix in this
// same PRE-20D2B.1 milestone: ListVoices' ctx.Done() branch now waits
// for its locked-thread goroutine before returning, exactly like
// Synthesize's own cancellation path already did - this guards against
// silently orphaning a locked OS thread/COM apartment on every timed-
// out or canceled call, regardless of whether that was CI's own root
// cause. The test only proves the call returns promptly and with a
// context error, not that this specific run is what CI ever hit.
func TestSystemProviderListVoicesCancellation(t *testing.T) {
	p := NewSystemProvider()
	skipIfSAPIUnavailable(t, p)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	_, err := p.ListVoices(ctx)
	if err == nil {
		t.Fatal("ListVoices() error = nil for an immediately-expired context, want a context/cancellation error")
	}
}
