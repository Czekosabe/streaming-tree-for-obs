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

// --- best-effort local SAPI smoke tests ----------------------------------
//
// These never send audio to speakers (AudioOutputStream is always an
// in-memory SpMemoryStream) and never depend on one particular
// installed voice name (governing task §71). If SAPI reports itself
// unavailable in this environment, every test below skips rather than
// fails - this package's own deterministic contract is covered by the
// bounds tests above and by internal/audio's own tests against a fake
// Provider; this file is a best-effort local verification only, never
// a required CI dependency.

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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
