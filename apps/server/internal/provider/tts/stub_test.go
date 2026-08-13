//go:build !windows

package tts

import (
	"context"
	"errors"
	"testing"
)

func TestStubCapabilitiesAlwaysUnavailable(t *testing.T) {
	p := NewSystemProvider()
	caps := p.Capabilities()
	if caps.Available {
		t.Error("Capabilities().Available = true on a non-Windows build, want false")
	}
	if caps.Reason == "" {
		t.Error("Capabilities().Reason is empty, want an honest explanation")
	}
}

func TestStubListVoicesReturnsErrUnavailable(t *testing.T) {
	p := NewSystemProvider()
	_, err := p.ListVoices(context.Background())
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("ListVoices() error = %v, want ErrUnavailable", err)
	}
}

func TestStubSynthesizeReturnsErrUnavailable(t *testing.T) {
	p := NewSystemProvider()
	_, err := p.Synthesize(context.Background(), SynthesizeInput{Text: "hello"})
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("Synthesize() error = %v, want ErrUnavailable", err)
	}
}
