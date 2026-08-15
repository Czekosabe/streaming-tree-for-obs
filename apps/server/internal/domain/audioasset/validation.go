package audioasset

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// Bounds (docs/alert-audio.md §5.3).
const (
	MaxSoundBytes      int64 = 8 << 20
	MaxSoundDurationMS int64 = 30_000

	MaxDisplayNameCodePoints = 200

	minSampleRate = 8000
	maxSampleRate = 192000
)

// MaxBytesFor returns the size bound for kind - trivial today (one kind),
// kept as its own function for the same shape visualasset.MaxBytesFor
// already has.
func MaxBytesFor(kind Kind) int64 {
	if kind == KindSound {
		return MaxSoundBytes
	}
	return 0
}

// DetectSignature independently identifies data's own media type from its
// magic bytes alone - never trusts a filename extension or a
// caller-declared media type. Returns ok=false for anything that is not a
// RIFF/WAVE container (including every explicitly excluded format: MP3,
// Ogg, WebM/Opus, FLAC, archives, executables, etc. all fall through to
// ok=false here - see docs/alert-audio.md §2.4).
func DetectSignature(data []byte) (MediaType, bool) {
	if len(data) >= 12 && bytes.Equal(data[0:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WAVE")) {
		return MediaWAV, true
	}
	return "", false
}

var extensionMediaTypes = map[string]MediaType{
	"wav": MediaWAV,
}

func extensionMediaType(ext string) MediaType {
	return extensionMediaTypes[ext]
}

// VerifyTypeAgreement checks that ext (a lowercase file extension without
// the leading dot, e.g. "wav"), declared (a caller-supplied media type
// string), and the signature actually detected from data all name the
// same MediaType (docs/alert-audio.md §5.3 - "independent triple
// validation," mirroring visualasset.VerifyTypeAgreement's own
// discipline). ext or declared may be empty (a manual upload's own
// filename extension is advisory only); when non-empty, both must agree
// with the detected signature or this returns ErrUnsupported.
func VerifyTypeAgreement(data []byte, ext, declared string) (MediaType, error) {
	detected, ok := DetectSignature(data)
	if !ok {
		return "", fmt.Errorf("%w: no recognized audio signature", ErrUnsupported)
	}
	if ext != "" && extensionMediaType(ext) != detected {
		return "", fmt.Errorf("%w: file extension %q does not match detected content %q", ErrUnsupported, ext, detected)
	}
	// "application/octet-stream" is the generic browser/client fallback
	// for "I do not actually know this file's type" - treated as "no
	// claim made" rather than compared, mirroring
	// visualasset.VerifyTypeAgreement's own identical exception.
	if declared != "" && declared != "application/octet-stream" && MediaType(declared) != detected {
		return "", fmt.Errorf("%w: declared media type %q does not match detected content %q", ErrUnsupported, declared, detected)
	}
	return detected, nil
}

// wavFormat is the parsed, validated contents of a canonical 16-byte PCM
// `fmt ` chunk.
type wavFormat struct {
	numChannels   uint16
	sampleRate    uint32
	bitsPerSample uint16
}

// ValidateWAV independently parses and validates data as a canonical
// 16-bit PCM WAV file (docs/alert-audio.md §5.3/§2.3) - the one closed
// shape this package accepts. It never decodes audio samples, only walks
// RIFF chunk headers, so it stays safe and cheap regardless of file
// content. Returns the exact duration in milliseconds, computed from the
// validated data chunk's own byte count divided by the header's own byte
// rate - never estimated, never requiring FFmpeg/ffprobe.
//
// Rejected outright, each with ErrUnsupported unless noted: a missing
// RIFF/WAVE magic; a missing, duplicated, truncated, or non-16-byte `fmt `
// chunk; any non-1 (non-PCM) format tag, including WAVE_FORMAT_EXTENSIBLE
// (0xFFFE); any bit depth other than 16; a channel count other than 1 or
// 2; a sample rate outside [8000, 192000]; a missing or duplicated `data`
// chunk; a `data` chunk whose declared size disagrees with the bytes
// actually present (the declared size is never trusted alone); a chunk
// whose declared size would run past the end of the buffer (rejected
// safely, never causing an out-of-bounds read/panic). ErrTooLarge is
// returned separately if the computed duration exceeds
// MaxSoundDurationMS.
func ValidateWAV(data []byte) (durationMS int64, err error) {
	if len(data) < 12 || !bytes.Equal(data[0:4], []byte("RIFF")) || !bytes.Equal(data[8:12], []byte("WAVE")) {
		return 0, fmt.Errorf("%w: not a RIFF/WAVE container", ErrUnsupported)
	}

	var format *wavFormat
	var dataBytes []byte
	sawFormat, sawData := false, false

	offset := 12
	for offset+8 <= len(data) {
		chunkID := data[offset : offset+4]
		chunkSize := binary.LittleEndian.Uint32(data[offset+4 : offset+8])
		chunkStart := offset + 8
		// A declared chunk size that would run past the buffer end is
		// rejected outright - the declared size is never trusted alone,
		// and this bound check is what keeps every subsequent slice
		// operation in this function panic-free.
		if uint64(chunkStart)+uint64(chunkSize) > uint64(len(data)) {
			return 0, fmt.Errorf("%w: chunk %q declares a size past the end of the file", ErrUnsupported, chunkID)
		}
		chunkData := data[chunkStart : chunkStart+int(chunkSize)]

		switch string(chunkID) {
		case "fmt ":
			if sawFormat {
				return 0, fmt.Errorf("%w: duplicate fmt chunk", ErrUnsupported)
			}
			sawFormat = true
			f, ferr := parseFormatChunk(chunkData)
			if ferr != nil {
				return 0, ferr
			}
			format = f
		case "data":
			if sawData {
				return 0, fmt.Errorf("%w: duplicate data chunk", ErrUnsupported)
			}
			sawData = true
			dataBytes = chunkData
		default:
			// Any other chunk (LIST metadata, fact, etc.) is safely
			// skipped by its own declared size - never parsed, never
			// trusted for anything.
		}

		// Chunks are padded to an even byte boundary.
		advance := int(chunkSize)
		if advance%2 == 1 {
			advance++
		}
		offset = chunkStart + advance
	}

	if !sawFormat {
		return 0, fmt.Errorf("%w: missing fmt chunk", ErrUnsupported)
	}
	if !sawData {
		return 0, fmt.Errorf("%w: missing data chunk", ErrUnsupported)
	}
	if len(dataBytes) == 0 {
		return 0, fmt.Errorf("%w: data chunk is empty", ErrUnsupported)
	}

	blockAlign := int(format.numChannels) * int(format.bitsPerSample) / 8
	if blockAlign <= 0 || len(dataBytes)%blockAlign != 0 {
		return 0, fmt.Errorf("%w: data chunk size is not a whole number of sample frames", ErrUnsupported)
	}
	byteRate := int64(format.sampleRate) * int64(blockAlign)
	if byteRate <= 0 {
		return 0, fmt.Errorf("%w: invalid computed byte rate", ErrUnsupported)
	}
	durationMS = int64(len(dataBytes)) * 1000 / byteRate
	if durationMS > MaxSoundDurationMS {
		return 0, fmt.Errorf("%w: audio duration %dms exceeds the maximum of %dms", ErrTooLarge, durationMS, MaxSoundDurationMS)
	}
	return durationMS, nil
}

// parseFormatChunk validates a `fmt ` chunk's own contents - canonical
// PCM only (16 bytes, format tag 1), rejecting WAVE_FORMAT_EXTENSIBLE and
// every other codec/subformat outright, per docs/alert-audio.md §2.3's
// own closed-format decision.
func parseFormatChunk(chunkData []byte) (*wavFormat, error) {
	if len(chunkData) != 16 {
		return nil, fmt.Errorf("%w: fmt chunk is %d bytes, want the canonical 16-byte PCM form", ErrUnsupported, len(chunkData))
	}
	audioFormat := binary.LittleEndian.Uint16(chunkData[0:2])
	if audioFormat != 1 {
		return nil, fmt.Errorf("%w: audio format tag %d is not PCM (only format tag 1 is accepted)", ErrUnsupported, audioFormat)
	}
	numChannels := binary.LittleEndian.Uint16(chunkData[2:4])
	if numChannels != 1 && numChannels != 2 {
		return nil, fmt.Errorf("%w: %d channels is not accepted (only mono or stereo)", ErrUnsupported, numChannels)
	}
	sampleRate := binary.LittleEndian.Uint32(chunkData[4:8])
	if sampleRate < minSampleRate || sampleRate > maxSampleRate {
		return nil, fmt.Errorf("%w: sample rate %d Hz is outside the accepted %d-%d Hz range", ErrUnsupported, sampleRate, minSampleRate, maxSampleRate)
	}
	bitsPerSample := binary.LittleEndian.Uint16(chunkData[14:16])
	if bitsPerSample != 16 {
		return nil, fmt.Errorf("%w: %d-bit samples are not accepted (only 16-bit PCM)", ErrUnsupported, bitsPerSample)
	}
	return &wavFormat{numChannels: numChannels, sampleRate: sampleRate, bitsPerSample: bitsPerSample}, nil
}
