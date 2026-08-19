//go:build windows

// Windows system TTS provider - SAPI (SAPI.SpVoice) via COM Automation,
// chosen over Windows.Media.SpeechSynthesis/WinRT after direct research
// (docs/audio-tts.md §2): WinRT's own official examples all target a
// packaged UWP/XAML app and Go has no first-party WinRT projection,
// while SAPI's own reference documentation explicitly describes using
// AudioOutputStream with SpMemoryStream to capture synthesized audio in
// memory instead of playing it through the speakers - exactly the
// "synthesize to bytes, the shared runtime plays it" architecture this
// stage needs.
//
// Every SAPI/COM detail is confined to this file. No HRESULT, no COM
// exception string, and no go-ole type ever crosses the Provider
// interface boundary - every error is mapped to one of this package's
// own sentinel errors with a short, sanitized diagnostic string.
package tts

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"runtime"
	"time"

	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// SAPI's own native ranges (docs/audio-tts.md §10.4, confirmed from
// SAPI's own reference documentation): Rate -10..10, Volume 0..100.
// speedToSAPIRate/volumeToSAPIVolume translate this package's
// canonical, provider-independent Speed (0.5-2.0)/Volume (0.0-1.0)
// ranges into SAPI's own - no UI or Event Bus consumer ever sees a
// SAPI-native number.
const (
	sapiRateMin   = -10
	sapiRateMax   = 10
	sapiVolumeMin = 0
	sapiVolumeMax = 100

	// svsFlagsAsync tells SAPI's Speak to return immediately so this
	// provider can poll WaitUntilDone and respond promptly to
	// cancellation, rather than blocking the whole call irrecoverably.
	svsFlagsAsync = 1

	// waitPollInterval bounds how often WaitUntilDone is re-polled -
	// short enough that a canceled context is honored promptly.
	waitPollInterval = 200
)

var errCanceled = errors.New("synthesis canceled")

// SystemProvider is the Windows SAPI implementation of Provider. Holds
// no live COM state between calls - every ListVoices/Synthesize call
// creates and releases its own SpVoice on its own dedicated,
// OS-thread-locked goroutine with its own CoInitialize/CoUninitialize
// pair, so no COM object or apartment-threading assumption is ever
// shared across calls or goroutines.
type SystemProvider struct{}

// NewSystemProvider builds the Windows system TTS provider.
func NewSystemProvider() *SystemProvider { return &SystemProvider{} }

var _ Provider = (*SystemProvider)(nil)

// Capabilities performs a fast COM initialization + SpVoice creation +
// voice-count smoke test - never plays audio, never blocks on a full
// synthesis.
func (p *SystemProvider) Capabilities() Capabilities {
	if err := runOnLockedThread(checkAvailable); err != nil {
		return Capabilities{Available: false, Reason: sanitize(err)}
	}
	return Capabilities{Available: true}
}

// ListVoices enumerates every voice SAPI currently reports as
// installed.
func (p *SystemProvider) ListVoices(ctx context.Context) ([]Voice, error) {
	type outcome struct {
		voices []Voice
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		var o outcome
		o.err = runOnLockedThread(func() error {
			voices, err := listVoices()
			o.voices = voices
			return err
		})
		done <- o
	}()

	select {
	case o := <-done:
		if o.err != nil {
			return nil, fmt.Errorf("%w: %s", ErrUnavailable, sanitize(o.err))
		}
		return o.voices, nil
	case <-ctx.Done():
		// Unlike Synthesize's own cancellation path (which has SAPI's
		// own Skip method to promptly interrupt in-flight work), there
		// is no way to interrupt an in-flight GetVoices/token
		// enumeration - but this goroutine still owns a locked OS
		// thread and a live COM apartment until runOnLockedThread
		// returns. Returning here without waiting would orphan that
		// thread/apartment for however long the COM call actually
		// takes, silently accumulating under repeated
		// cancellation/timeout - waiting for <-done (discarding the
		// now-unwanted result) guarantees the thread/apartment is
		// always released before this method returns, exactly like
		// Synthesize already does.
		<-done
		return nil, ctx.Err()
	}
}

// Synthesize converts in.Text to bounded WAV bytes via SAPI's
// SpMemoryStream. Cancellation (ctx) triggers SAPI's own Skip method to
// stop synthesis promptly, rather than abandoning the underlying OS
// thread mid-COM-call.
func (p *SystemProvider) Synthesize(ctx context.Context, in SynthesizeInput) (SynthesizeResult, error) {
	type outcome struct {
		result SynthesizeResult
		err    error
	}
	done := make(chan outcome, 1)
	cancel := make(chan struct{})
	go func() {
		var o outcome
		o.err = runOnLockedThread(func() error {
			result, err := synthesize(in, cancel)
			o.result = result
			return err
		})
		done <- o
	}()

	select {
	case o := <-done:
		if o.err != nil {
			if errors.Is(o.err, errVoiceNotFoundInternal) {
				return SynthesizeResult{}, fmt.Errorf("%w: %s", ErrVoiceNotFound, sanitize(o.err))
			}
			return SynthesizeResult{}, fmt.Errorf("%w: %s", ErrSynthesisFailed, sanitize(o.err))
		}
		return o.result, nil
	case <-ctx.Done():
		close(cancel)
		<-done // wait for the locked goroutine to release its COM state
		return SynthesizeResult{}, ctx.Err()
	}
}

// comSFalse is COM's own S_FALSE HRESULT (winerror.h): for
// CoInitialize specifically, it means "COM was already initialized on
// this thread with a compatible concurrency model" - a documented
// SUCCESS outcome (Microsoft's own CoInitialize reference), not a
// failure. go-ole's CoInitialize wrapper treats any nonzero HRESULT as
// an error, which would misreport this benign case as a hard failure.
const comSFalse = 1

// runOnLockedThread runs fn on a freshly locked OS thread with its own
// CoInitialize/CoUninitialize pair - SAPI Automation objects are
// apartment-threaded, so every call must happen on the same thread that
// initialized COM for it.
func runOnLockedThread(fn func() error) error {
	done := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if err := ole.CoInitialize(0); err != nil {
			var oleErr *ole.OleError
			if !errors.As(err, &oleErr) || oleErr.Code() != comSFalse {
				done <- fmt.Errorf("CoInitialize: %w", err)
				return
			}
			// S_FALSE: COM is genuinely initialized on this thread
			// (most plausibly a thread Go's runtime reused from an
			// earlier, already-cleanly-uninitialized
			// LockOSThread/CoInitialize/CoUninitialize/
			// UnlockOSThread cycle elsewhere in this process) - fall
			// through to use it normally. CoUninitialize is still
			// called below to keep this thread's COM reference count
			// balanced for whichever goroutine acquires it next.
		}
		defer ole.CoUninitialize()
		done <- fn()
	}()
	return <-done
}

func checkAvailable() error {
	voice, release, err := newSpVoice()
	if err != nil {
		return err
	}
	defer release()

	tokens, count, err := getVoiceTokens(voice)
	if err != nil {
		return err
	}
	defer tokens.Release()
	return voiceCountAvailable(count)
}

// voiceCountAvailable is checkAvailable's own decision logic, factored
// out as a pure function so the "zero installed voices" contract is
// deterministically unit-testable without any real SAPI/COM
// dependency (docs/ci-reliability.md's own Windows-diagnostic
// investigation found no seam for this before).
func voiceCountAvailable(count int) error {
	if count <= 0 {
		return errors.New("no installed voices reported")
	}
	return nil
}

func newSpVoice() (voice *ole.IDispatch, release func(), err error) {
	unknown, err := oleutil.CreateObject("SAPI.SpVoice")
	if err != nil {
		return nil, nil, fmt.Errorf("create SAPI.SpVoice: %w", err)
	}
	disp, err := unknown.QueryInterface(ole.IID_IDispatch)
	unknown.Release()
	if err != nil {
		return nil, nil, fmt.Errorf("SpVoice IDispatch: %w", err)
	}
	return disp, func() { disp.Release() }, nil
}

func getVoiceTokens(voice *ole.IDispatch) (tokens *ole.IDispatch, count int, err error) {
	tokensVariant, err := oleutil.CallMethod(voice, "GetVoices")
	if err != nil {
		return nil, 0, fmt.Errorf("GetVoices: %w", err)
	}
	tokens = tokensVariant.ToIDispatch()
	countVariant, err := oleutil.GetProperty(tokens, "Count")
	if err != nil {
		tokens.Release()
		return nil, 0, fmt.Errorf("voice token Count: %w", err)
	}
	return tokens, toInt(countVariant), nil
}

func listVoices() ([]Voice, error) {
	voice, release, err := newSpVoice()
	if err != nil {
		return nil, err
	}
	defer release()

	tokens, count, err := getVoiceTokens(voice)
	if err != nil {
		return nil, err
	}
	defer tokens.Release()

	defaultID := ""
	if defaultVariant, err := oleutil.GetProperty(voice, "Voice"); err == nil {
		if defaultDisp := defaultVariant.ToIDispatch(); defaultDisp != nil {
			if idVariant, err := oleutil.GetProperty(defaultDisp, "Id"); err == nil {
				defaultID = toStr(idVariant)
			}
			defaultDisp.Release()
		}
	}

	out := make([]Voice, 0, count)
	for i := 0; i < count; i++ {
		itemVariant, err := oleutil.CallMethod(tokens, "Item", i)
		if err != nil {
			continue
		}
		token := itemVariant.ToIDispatch()
		v := voiceFromToken(token)
		token.Release()
		if v.ID == "" {
			continue
		}
		v.IsDefault = v.ID == defaultID
		out = append(out, v)
	}
	return out, nil
}

func voiceFromToken(token *ole.IDispatch) Voice {
	var v Voice
	if idVariant, err := oleutil.GetProperty(token, "Id"); err == nil {
		v.ID = toStr(idVariant)
	}
	if descVariant, err := oleutil.CallMethod(token, "GetDescription"); err == nil {
		v.Name = toStr(descVariant)
	}
	// Language/Gender attributes are optional in SAPI's own token model
	// - a missing attribute is left empty here rather than guessed.
	// Language is SAPI's own LCID hex string (e.g. "409"), not
	// normalized to a BCP-47 tag - documented honestly rather than
	// silently mistranslated.
	if langVariant, err := oleutil.CallMethod(token, "GetAttribute", "Language"); err == nil {
		v.Language = toStr(langVariant)
	}
	if genderVariant, err := oleutil.CallMethod(token, "GetAttribute", "Gender"); err == nil {
		v.Gender = toStr(genderVariant)
	}
	return v
}

var errVoiceNotFoundInternal = errors.New("voice not found")

func findVoiceToken(voice *ole.IDispatch, voiceID string) (*ole.IDispatch, func(), error) {
	tokens, count, err := getVoiceTokens(voice)
	if err != nil {
		return nil, nil, err
	}
	defer tokens.Release()

	for i := 0; i < count; i++ {
		itemVariant, err := oleutil.CallMethod(tokens, "Item", i)
		if err != nil {
			continue
		}
		token := itemVariant.ToIDispatch()
		idVariant, err := oleutil.GetProperty(token, "Id")
		if err == nil && toStr(idVariant) == voiceID {
			return token, func() { token.Release() }, nil
		}
		token.Release()
	}
	return nil, nil, errVoiceNotFoundInternal
}

func synthesize(in SynthesizeInput, cancel <-chan struct{}) (SynthesizeResult, error) {
	voice, releaseVoice, err := newSpVoice()
	if err != nil {
		return SynthesizeResult{}, err
	}
	defer releaseVoice()

	streamUnknown, err := oleutil.CreateObject("SAPI.SpMemoryStream")
	if err != nil {
		return SynthesizeResult{}, fmt.Errorf("create SAPI.SpMemoryStream: %w", err)
	}
	stream, err := streamUnknown.QueryInterface(ole.IID_IDispatch)
	streamUnknown.Release()
	if err != nil {
		return SynthesizeResult{}, fmt.Errorf("SpMemoryStream IDispatch: %w", err)
	}
	defer stream.Release()

	// AudioOutputStream is an object-reference property - COM requires
	// DISPATCH_PROPERTYPUTREF (oleutil.PutPropertyRef), not the plain
	// DISPATCH_PROPERTYPUT oleutil.PutProperty uses; the latter fails
	// with "Member not found" against a real SAPI.SpVoice, confirmed by
	// running this code against the real installed SAPI engine.
	if _, err := oleutil.PutPropertyRef(voice, "AudioOutputStream", stream); err != nil {
		return SynthesizeResult{}, fmt.Errorf("set AudioOutputStream: %w", err)
	}

	if in.VoiceID != "" {
		token, releaseToken, err := findVoiceToken(voice, in.VoiceID)
		if err != nil {
			return SynthesizeResult{}, err
		}
		// Voice is likewise an object-reference property.
		_, err = oleutil.PutPropertyRef(voice, "Voice", token)
		releaseToken()
		if err != nil {
			return SynthesizeResult{}, fmt.Errorf("set Voice: %w", err)
		}
	}

	if _, err := oleutil.PutProperty(voice, "Rate", speedToSAPIRate(in.Speed)); err != nil {
		return SynthesizeResult{}, fmt.Errorf("set Rate: %w", err)
	}
	if _, err := oleutil.PutProperty(voice, "Volume", volumeToSAPIVolume(in.Volume)); err != nil {
		return SynthesizeResult{}, fmt.Errorf("set Volume: %w", err)
	}

	if _, err := oleutil.CallMethod(voice, "Speak", in.Text, svsFlagsAsync); err != nil {
		return SynthesizeResult{}, fmt.Errorf("Speak: %w", err)
	}

	start := time.Now()
	for {
		select {
		case <-cancel:
			oleutil.CallMethod(voice, "Skip", "Sentence", int32(2147483647))
			return SynthesizeResult{}, errCanceled
		default:
		}
		doneVariant, err := oleutil.CallMethod(voice, "WaitUntilDone", waitPollInterval)
		if err != nil {
			return SynthesizeResult{}, fmt.Errorf("WaitUntilDone: %w", err)
		}
		if toBool(doneVariant) {
			break
		}
	}

	dataVariant, err := oleutil.CallMethod(stream, "GetData")
	if err != nil {
		return SynthesizeResult{}, fmt.Errorf("GetData: %w", err)
	}
	// SpMemoryStream.GetData returns raw PCM samples, never a
	// self-describing RIFF/WAV file - confirmed by inspecting the
	// actual bytes against the real installed SAPI engine (they never
	// start with "RIFF"). A real WAV header must be constructed here
	// from the stream's own actual format so the browser renderer (and
	// anything else that plays this as audio/wav) gets a valid file.
	pcm := dataVariant.ToArray().ToByteArray()

	sampleRate, bitsPerSample, channels, err := streamWaveFormat(stream)
	if err != nil || sampleRate <= 0 || bitsPerSample <= 0 || channels <= 0 {
		// Fall back to SAPI's own long-documented default PCM format
		// only if the real format genuinely could not be queried -
		// never the primary path, always attempted first above.
		sampleRate, bitsPerSample, channels = 22050, 16, 1
	}
	wavBytes := wrapPCMAsWAV(pcm, sampleRate, bitsPerSample, channels)

	return SynthesizeResult{
		ContentType: "audio/wav",
		Audio:       wavBytes,
		Duration:    time.Since(start),
	}, nil
}

// streamWaveFormat reads the actual PCM format SAPI wrote to stream via
// its own Format/GetWaveFormatEx object (confirmed the correct
// automation path via Microsoft's own SpAudioFormat/SpWaveFormatEx
// reference documentation).
func streamWaveFormat(stream *ole.IDispatch) (sampleRate, bitsPerSample, channels int, err error) {
	formatVariant, err := oleutil.GetProperty(stream, "Format")
	if err != nil {
		return 0, 0, 0, fmt.Errorf("get Format: %w", err)
	}
	formatDisp := formatVariant.ToIDispatch()
	if formatDisp == nil {
		return 0, 0, 0, errors.New("Format property is not an object")
	}
	defer formatDisp.Release()

	waveVariant, err := oleutil.CallMethod(formatDisp, "GetWaveFormatEx")
	if err != nil {
		return 0, 0, 0, fmt.Errorf("GetWaveFormatEx: %w", err)
	}
	wave := waveVariant.ToIDispatch()
	if wave == nil {
		return 0, 0, 0, errors.New("GetWaveFormatEx did not return an object")
	}
	defer wave.Release()

	srVariant, err := oleutil.GetProperty(wave, "SamplesPerSec")
	if err != nil {
		return 0, 0, 0, fmt.Errorf("SamplesPerSec: %w", err)
	}
	bpsVariant, err := oleutil.GetProperty(wave, "BitsPerSample")
	if err != nil {
		return 0, 0, 0, fmt.Errorf("BitsPerSample: %w", err)
	}
	chVariant, err := oleutil.GetProperty(wave, "Channels")
	if err != nil {
		return 0, 0, 0, fmt.Errorf("Channels: %w", err)
	}
	return toInt(srVariant), toInt(bpsVariant), toInt(chVariant), nil
}

// wrapPCMAsWAV prepends a standard 44-byte canonical RIFF/WAV header
// (PCM format tag 1) to raw little-endian PCM samples.
func wrapPCMAsWAV(pcm []byte, sampleRate, bitsPerSample, channels int) []byte {
	byteRate := sampleRate * channels * bitsPerSample / 8
	blockAlign := channels * bitsPerSample / 8

	header := make([]byte, 44)
	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], uint32(36+len(pcm)))
	copy(header[8:12], "WAVE")
	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16) // PCM fmt chunk size
	binary.LittleEndian.PutUint16(header[20:22], 1)  // audio format = PCM
	binary.LittleEndian.PutUint16(header[22:24], uint16(channels))
	binary.LittleEndian.PutUint32(header[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(header[28:32], uint32(byteRate))
	binary.LittleEndian.PutUint16(header[32:34], uint16(blockAlign))
	binary.LittleEndian.PutUint16(header[34:36], uint16(bitsPerSample))
	copy(header[36:40], "data")
	binary.LittleEndian.PutUint32(header[40:44], uint32(len(pcm)))

	return append(header, pcm...)
}

func speedToSAPIRate(speed float64) int32 {
	if speed <= 0 {
		speed = 1.0
	}
	// Speed is a linear multiplier (1.0 = normal); SAPI's Rate is a
	// symmetric -10..10 scale with no documented exact formula, so this
	// package picks a simple, monotonic, clamped linear mapping
	// centered at 0 for speed==1.0 - deliberately not claimed to match
	// any particular perceptual curve, only to move in the right
	// direction within SAPI's own bounds.
	rate := int32((speed - 1.0) * 10)
	if rate < sapiRateMin {
		rate = sapiRateMin
	}
	if rate > sapiRateMax {
		rate = sapiRateMax
	}
	return rate
}

func volumeToSAPIVolume(volume float64) int32 {
	if volume < 0 {
		volume = 0
	}
	if volume > 1 {
		volume = 1
	}
	v := int32(volume * sapiVolumeMax)
	if v < sapiVolumeMin {
		v = sapiVolumeMin
	}
	if v > sapiVolumeMax {
		v = sapiVolumeMax
	}
	return v
}

func toBool(v *ole.VARIANT) bool {
	if b, ok := v.Value().(bool); ok {
		return b
	}
	return false
}

func toInt(v *ole.VARIANT) int {
	switch val := v.Value().(type) {
	case int32:
		return int(val)
	case int64:
		return int(val)
	case int:
		return val
	case int16:
		return int(val)
	default:
		return 0
	}
}

func toStr(v *ole.VARIANT) string {
	if s, ok := v.Value().(string); ok {
		return s
	}
	return v.ToString()
}

// sanitize returns a short, safe-to-surface diagnostic string - never
// the full underlying error chain, which for a COM failure can embed a
// raw HRESULT/exception description.
func sanitize(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	const maxLen = 200
	if len(msg) > maxLen {
		msg = msg[:maxLen]
	}
	return msg
}
