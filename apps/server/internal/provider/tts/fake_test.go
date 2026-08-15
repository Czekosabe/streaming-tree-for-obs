//go:build integration

package tts

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestFakeProviderCapabilitiesDefaultsAvailable(t *testing.T) {
	p := NewFakeProvider()
	if !p.Capabilities().Available {
		t.Error("Capabilities().Available = false, want true by default")
	}
}

func TestFakeProviderSetAvailableFalse(t *testing.T) {
	p := NewFakeProvider()
	p.SetAvailable(false, "test reason")
	caps := p.Capabilities()
	if caps.Available {
		t.Error("Capabilities().Available = true after SetAvailable(false, ...), want false")
	}
	if caps.Reason != "test reason" {
		t.Errorf("Capabilities().Reason = %q, want %q", caps.Reason, "test reason")
	}
}

func TestFakeProviderListVoicesReturnsTwoDeterministicVoices(t *testing.T) {
	p := NewFakeProvider()
	voices, err := p.ListVoices(context.Background())
	if err != nil {
		t.Fatalf("ListVoices() error = %v", err)
	}
	if len(voices) != 2 {
		t.Fatalf("ListVoices() returned %d voices, want 2", len(voices))
	}
	if voices[0].ID != "fake-voice-default" || !voices[0].IsDefault {
		t.Errorf("voices[0] = %+v, want the default fake voice", voices[0])
	}
}

func TestFakeProviderListVoicesUnavailableReturnsError(t *testing.T) {
	p := NewFakeProvider()
	p.SetAvailable(false, "")
	if _, err := p.ListVoices(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Errorf("ListVoices() error = %v, want ErrUnavailable", err)
	}
}

func TestFakeProviderSynthesizeProducesValidDeterministicWAV(t *testing.T) {
	p := NewFakeProvider()
	result, err := p.Synthesize(context.Background(), SynthesizeInput{Text: "hello"})
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	if result.ContentType != "audio/wav" {
		t.Errorf("ContentType = %q, want audio/wav", result.ContentType)
	}
	if len(result.Audio) < 44 {
		t.Fatalf("Audio is %d bytes, want at least a 44-byte WAV header", len(result.Audio))
	}
	if string(result.Audio[0:4]) != "RIFF" || string(result.Audio[8:12]) != "WAVE" {
		t.Errorf("Audio does not start with a valid RIFF/WAVE header: % x", result.Audio[0:12])
	}
	if result.Duration <= 0 {
		t.Error("Duration <= 0, want a positive deterministic duration")
	}
}

func TestFakeProviderSynthesizeIsDeterministicForTheSameInput(t *testing.T) {
	p := NewFakeProvider()
	a, err := p.Synthesize(context.Background(), SynthesizeInput{Text: "same text"})
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	b, err := p.Synthesize(context.Background(), SynthesizeInput{Text: "same text"})
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	if len(a.Audio) != len(b.Audio) {
		t.Errorf("len(a.Audio)=%d != len(b.Audio)=%d, want identical for identical input", len(a.Audio), len(b.Audio))
	}
	if a.Duration != b.Duration {
		t.Errorf("a.Duration=%v != b.Duration=%v, want identical for identical input", a.Duration, b.Duration)
	}
}

func TestFakeProviderSynthesizeDurationScalesWithTextLength(t *testing.T) {
	p := NewFakeProvider()
	short, err := p.Synthesize(context.Background(), SynthesizeInput{Text: "hi"})
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	long, err := p.Synthesize(context.Background(), SynthesizeInput{Text: "this is a much longer utterance than the short one above"})
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	if long.Duration <= short.Duration {
		t.Errorf("long.Duration=%v, want strictly greater than short.Duration=%v", long.Duration, short.Duration)
	}
}

func TestFakeProviderSynthesizeUnknownVoiceRejected(t *testing.T) {
	p := NewFakeProvider()
	_, err := p.Synthesize(context.Background(), SynthesizeInput{Text: "hi", VoiceID: "not-a-real-voice"})
	if !errors.Is(err, ErrVoiceNotFound) {
		t.Errorf("Synthesize() error = %v, want ErrVoiceNotFound", err)
	}
}

func TestFakeProviderFailNextSynthesisAffectsOnlyOneCall(t *testing.T) {
	p := NewFakeProvider()
	p.FailNextSynthesis()

	_, err := p.Synthesize(context.Background(), SynthesizeInput{Text: "hi"})
	if !errors.Is(err, ErrSynthesisFailed) {
		t.Errorf("first Synthesize() error = %v, want ErrSynthesisFailed", err)
	}

	if _, err := p.Synthesize(context.Background(), SynthesizeInput{Text: "hi"}); err != nil {
		t.Errorf("second Synthesize() error = %v, want nil (failure isolated to one call)", err)
	}
}

func TestFakeProviderOversizeNextSynthesisAffectsOnlyOneCall(t *testing.T) {
	p := NewFakeProvider()
	p.OversizeNextSynthesis()

	big, err := p.Synthesize(context.Background(), SynthesizeInput{Text: "hi"})
	if err != nil {
		t.Fatalf("first Synthesize() error = %v", err)
	}
	const eightMiB = 8 * 1024 * 1024
	if len(big.Audio) <= eightMiB {
		t.Errorf("first Synthesize() produced %d bytes, want > %d (deliberately oversized)", len(big.Audio), eightMiB)
	}

	normal, err := p.Synthesize(context.Background(), SynthesizeInput{Text: "hi"})
	if err != nil {
		t.Fatalf("second Synthesize() error = %v", err)
	}
	if len(normal.Audio) >= eightMiB {
		t.Errorf("second Synthesize() produced %d bytes, want a normal small size (oversize isolated to one call)", len(normal.Audio))
	}
}

func TestFakeProviderSynthesisDelayRespectsCancellation(t *testing.T) {
	p := NewFakeProvider()
	p.SetSynthesisDelay(time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := p.Synthesize(ctx, SynthesizeInput{Text: "hi"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Synthesize() error = %v, want context.DeadlineExceeded", err)
	}
}

func TestFakeProviderSynthesizeCallCount(t *testing.T) {
	p := NewFakeProvider()
	if p.SynthesizeCallCount() != 0 {
		t.Fatalf("SynthesizeCallCount() = %d, want 0 before any call", p.SynthesizeCallCount())
	}
	_, _ = p.Synthesize(context.Background(), SynthesizeInput{Text: "hi"})
	_, _ = p.Synthesize(context.Background(), SynthesizeInput{Text: "there"})
	if p.SynthesizeCallCount() != 2 {
		t.Errorf("SynthesizeCallCount() = %d, want 2", p.SynthesizeCallCount())
	}
}
