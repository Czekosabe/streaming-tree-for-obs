//go:build integration

// Integration-build-only deterministic fake Provider (governing task
// §66/§67, docs/audio-tts.md §21). Never linked into the real
// cmd/server binary - this whole file requires the `integration` build
// tag, exactly like cmd/testserver/main.go itself. Generates a small,
// valid, fully deterministic WAV/PCM buffer in-process; never shells
// out to ffmpeg, never touches the network, never depends on any
// installed OS voice - the 19th integration script must never be
// flaky because of what happens to be installed on the machine
// running it.
package tts

import (
	"context"
	"encoding/binary"
	"sync"
	"sync/atomic"
	"time"
)

const (
	fakeSampleRate    = 8000
	fakeBitsPerSample = 16
	fakeChannels      = 1
	// fakeSamplesPerCodePoint keeps the generated duration deterministic
	// and directly derivable from the input text length - useful for a
	// test asserting "duration scales with text" without decoding audio.
	fakeSamplesPerCodePoint = 80
	fakeMinSamples          = 800
	// fakeOversizeSamples deliberately exceeds this stage's own 8 MiB
	// maxAudioBytes bound (internal/audio.defaultMaxAudioBytes) many
	// times over, for the "oversized synthesis output rejected" scenario.
	fakeOversizeSamples = 20_000_000
)

// FakeProvider is the deterministic Provider double the 19th
// integration script wires into cmd/testserver instead of the real
// Windows SAPI provider. Every knob is a plain method so the script can
// arrange a specific scenario (unavailable, a missing voice, a forced
// failure, an oversized result, an artificial delay for a cancellation
// test) before triggering a real Event Bus event or a Test Speak call.
type FakeProvider struct {
	mu               sync.Mutex
	available        bool
	reason           string
	voices           []Voice
	delay            time.Duration
	failNextCall     bool
	oversizeNextCall bool

	synthesizeCalls atomic.Int32
}

// NewFakeProvider builds a FakeProvider that starts available, with two
// fixed, deterministic voices.
func NewFakeProvider() *FakeProvider {
	return &FakeProvider{
		available: true,
		voices: []Voice{
			{ID: "fake-voice-default", Name: "Fake Default Voice", Language: "en-US", IsDefault: true},
			{ID: "fake-voice-alt", Name: "Fake Alternate Voice", Language: "en-GB"},
		},
	}
}

var _ Provider = (*FakeProvider)(nil)

func (p *FakeProvider) Capabilities() Capabilities {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.available {
		reason := p.reason
		if reason == "" {
			reason = "fake provider set unavailable for this scenario"
		}
		return Capabilities{Available: false, Reason: reason}
	}
	return Capabilities{Available: true}
}

// SetAvailable toggles Capabilities().Available for the "provider
// capability response"/"unavailable" integration scenarios.
func (p *FakeProvider) SetAvailable(available bool, reason string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.available = available
	p.reason = reason
}

func (p *FakeProvider) ListVoices(ctx context.Context) ([]Voice, error) {
	p.mu.Lock()
	available := p.available
	voices := make([]Voice, len(p.voices))
	copy(voices, p.voices)
	p.mu.Unlock()

	if !available {
		return nil, ErrUnavailable
	}
	return voices, nil
}

// FailNextSynthesis makes exactly the next Synthesize call return
// ErrSynthesisFailed - the "synthesis provider failure isolates one
// item" scenario.
func (p *FakeProvider) FailNextSynthesis() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failNextCall = true
}

// OversizeNextSynthesis makes exactly the next Synthesize call return
// audio far larger than any real bound - the "oversized synthesis
// output rejected" scenario.
func (p *FakeProvider) OversizeNextSynthesis() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.oversizeNextCall = true
}

// SetSynthesisDelay makes every subsequent Synthesize call block for d
// (respecting ctx cancellation) - the "synthesis cancellation"/"skip
// current cancels synthesis" scenarios.
func (p *FakeProvider) SetSynthesisDelay(d time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.delay = d
}

// SynthesizeCallCount reports how many times Synthesize has been
// called - lets a scenario assert a failure isolated exactly one item
// rather than stopping the runtime.
func (p *FakeProvider) SynthesizeCallCount() int {
	return int(p.synthesizeCalls.Load())
}

func (p *FakeProvider) Synthesize(ctx context.Context, in SynthesizeInput) (SynthesizeResult, error) {
	p.synthesizeCalls.Add(1)

	p.mu.Lock()
	available := p.available
	voices := p.voices
	delay := p.delay
	failNextCall := p.failNextCall
	oversizeNextCall := p.oversizeNextCall
	p.failNextCall = false
	p.oversizeNextCall = false
	p.mu.Unlock()

	if !available {
		return SynthesizeResult{}, ErrUnavailable
	}
	if in.VoiceID != "" {
		found := false
		for _, v := range voices {
			if v.ID == in.VoiceID {
				found = true
				break
			}
		}
		if !found {
			return SynthesizeResult{}, ErrVoiceNotFound
		}
	}

	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return SynthesizeResult{}, ctx.Err()
		}
	}

	if failNextCall {
		return SynthesizeResult{}, ErrSynthesisFailed
	}

	numSamples := len([]rune(in.Text)) * fakeSamplesPerCodePoint
	if numSamples < fakeMinSamples {
		numSamples = fakeMinSamples
	}
	if oversizeNextCall {
		numSamples = fakeOversizeSamples
	}

	pcm := generateFakePCM(numSamples)
	wav := wrapFakePCMAsWAV(pcm, fakeSampleRate, fakeBitsPerSample, fakeChannels)
	duration := time.Duration(numSamples) * time.Second / fakeSampleRate

	return SynthesizeResult{
		ContentType: "audio/wav",
		Audio:       wav,
		Duration:    duration,
	}, nil
}

// generateFakePCM produces a deterministic, non-silent 16-bit PCM
// buffer - a simple bounded repeating ramp, never real speech, never
// silence-only (so a test can distinguish real generated bytes from an
// accidentally-zeroed buffer without decoding audio).
func generateFakePCM(numSamples int) []byte {
	pcm := make([]byte, numSamples*2)
	for i := 0; i < numSamples; i++ {
		v := int16((i % 200) * 100)
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(v))
	}
	return pcm
}

// wrapFakePCMAsWAV mirrors windows.go's own wrapPCMAsWAV exactly (a
// standard 44-byte canonical RIFF/WAV header, PCM format tag 1) -
// duplicated under its own name rather than shared, since windows.go
// is `//go:build windows`-only and this file is `//go:build
// integration`-only; on a Windows integration build both are compiled
// into the same package, so a shared name would collide.
func wrapFakePCMAsWAV(pcm []byte, sampleRate, bitsPerSample, channels int) []byte {
	byteRate := sampleRate * channels * bitsPerSample / 8
	blockAlign := channels * bitsPerSample / 8

	header := make([]byte, 44)
	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], uint32(36+len(pcm)))
	copy(header[8:12], "WAVE")
	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], 1)
	binary.LittleEndian.PutUint16(header[22:24], uint16(channels))
	binary.LittleEndian.PutUint32(header[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(header[28:32], uint32(byteRate))
	binary.LittleEndian.PutUint16(header[32:34], uint16(blockAlign))
	binary.LittleEndian.PutUint16(header[34:36], uint16(bitsPerSample))
	copy(header[36:40], "data")
	binary.LittleEndian.PutUint32(header[40:44], uint32(len(pcm)))

	return append(header, pcm...)
}
