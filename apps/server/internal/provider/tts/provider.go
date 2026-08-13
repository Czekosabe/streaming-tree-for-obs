// Package tts holds the Stage 17A provider-independent text-to-speech
// abstraction (docs/audio-tts.md §5) - the one interface every real
// synthesis backend implements, and the only thing internal/audio's
// runtime manager is allowed to depend on for actually producing
// speech.
//
// No SAPI/COM/WinRT/HRESULT detail, no Windows-only type, and no
// platform-specific error string crosses this package boundary - a
// concrete implementation (this stage: a Windows-only SAPI provider
// behind a //go:build windows file, plus a //go:build !windows stub
// that always reports Capabilities.Available == false) translates
// everything into the shapes defined here before returning.
package tts

import (
	"context"
	"errors"
	"time"
)

// Sentinel errors every Provider implementation returns instead of a
// provider-specific error type - internal/audio's manager maps these to
// safe, stable outcomes without ever inspecting a raw underlying error.
var (
	// ErrUnavailable means this provider cannot synthesize anything at
	// all right now (wrong platform, initialization failed) - a whole-
	// provider condition, checked via Capabilities before ever calling
	// Synthesize.
	ErrUnavailable = errors.New("tts provider unavailable")
	// ErrVoiceNotFound means the requested VoiceID does not exist among
	// this provider's current ListVoices result.
	ErrVoiceNotFound = errors.New("tts voice not found")
	// ErrSynthesisFailed is the safe, generic outcome for any provider-
	// internal synthesis failure (COM/WinRT error, malformed/empty
	// engine output) - isolates one queue item, never the process.
	ErrSynthesisFailed = errors.New("tts synthesis failed")
	// ErrOutputTooLarge means the synthesized audio exceeded the
	// configured maximum byte bound (docs/audio-tts.md §7).
	ErrOutputTooLarge = errors.New("tts synthesized audio exceeds the maximum size")
)

// Capabilities reports whether a Provider can synthesize anything right
// now, and if not, why - a short, already-sanitized string safe to
// surface to an operator (never a raw COM/HRESULT value or Windows
// error string).
type Capabilities struct {
	Available bool
	Reason    string
}

// Voice is one provider-reported installed/available voice. ID is the
// provider's own stable identifier - the only thing ever persisted
// (internal/domain/audio.Settings.VoiceID); Name/Language/Gender are
// display-only and never persisted as if they were the identifier.
type Voice struct {
	ID        string
	Name      string
	Language  string
	Gender    string
	IsDefault bool
}

// SynthesizeInput carries only provider-independent, already-validated
// fields - plain text only (docs/audio-tts.md §10.5: never SSML), a
// selected voice id (empty means "system default"), an optional
// language hint, and the canonical app-level Speed/Volume ranges
// (internal/domain/audio's own MinSpeed/MaxSpeed/MinVolume/MaxVolume) -
// a concrete provider adapter translates these into its own native
// numeric ranges internally; this struct never carries one.
type SynthesizeInput struct {
	Text     string
	VoiceID  string
	Language string
	Speed    float64
	Volume   float64
}

// SynthesizeResult exposes only what the shared audio runtime needs to
// serve and play one item - never a raw COM/HRESULT value, never a
// filesystem path.
type SynthesizeResult struct {
	ContentType string
	Audio       []byte
	// Duration is the audio's real playback length when the provider
	// can report it reliably; zero when unknown (the browser renderer
	// itself remains authoritative for actual playback completion via
	// its own ended/error events - see docs/audio-tts.md §14).
	Duration time.Duration
	// Diagnostics is a small, already-sanitized string safe to log or
	// surface - never a raw provider exception/error string.
	Diagnostics string
}

// Provider is the one interface every real TTS backend implements - see
// the package doc comment. No caller outside this package's own
// concrete implementations may ever type-assert to a concrete type;
// internal/audio's manager holds only this interface.
type Provider interface {
	// Capabilities reports whether this provider can synthesize
	// anything right now - checked before every ListVoices/Synthesize
	// call so an unavailable provider fails fast with a clear reason
	// rather than a confusing downstream error.
	Capabilities() Capabilities

	// ListVoices returns every voice this provider currently reports as
	// installed/available - never hardcoded, never assumed.
	ListVoices(ctx context.Context) ([]Voice, error)

	// Synthesize converts in.Text (already fully preprocessed, plain
	// text only) into bounded audio bytes. ctx carries the per-item
	// synthesis timeout (docs/audio-tts.md §7); a provider must respect
	// cancellation promptly rather than blocking past it.
	Synthesize(ctx context.Context, in SynthesizeInput) (SynthesizeResult, error)
}
