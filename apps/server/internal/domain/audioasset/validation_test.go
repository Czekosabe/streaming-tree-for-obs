package audioasset

import (
	"encoding/binary"
	"errors"
	"testing"
)

// buildWAV assembles a minimal, well-formed 16-bit PCM WAV file for tests.
// numSamples is per channel.
func buildWAV(t *testing.T, sampleRate uint32, channels uint16, bitsPerSample uint16, numSamples int) []byte {
	t.Helper()
	blockAlign := int(channels) * int(bitsPerSample) / 8
	dataSize := numSamples * blockAlign
	byteRate := sampleRate * uint32(blockAlign)

	buf := make([]byte, 44+dataSize)
	copy(buf[0:4], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:8], uint32(36+dataSize))
	copy(buf[8:12], "WAVE")
	copy(buf[12:16], "fmt ")
	binary.LittleEndian.PutUint32(buf[16:20], 16)
	binary.LittleEndian.PutUint16(buf[20:22], 1) // PCM
	binary.LittleEndian.PutUint16(buf[22:24], channels)
	binary.LittleEndian.PutUint32(buf[24:28], sampleRate)
	binary.LittleEndian.PutUint32(buf[28:32], byteRate)
	binary.LittleEndian.PutUint16(buf[32:34], uint16(blockAlign))
	binary.LittleEndian.PutUint16(buf[34:36], bitsPerSample)
	copy(buf[36:40], "data")
	binary.LittleEndian.PutUint32(buf[40:44], uint32(dataSize))
	for i := 0; i < dataSize; i++ {
		buf[44+i] = byte(i % 200)
	}
	return buf
}

func testWAV(t *testing.T) []byte {
	t.Helper()
	return buildWAV(t, 44100, 2, 16, 4410) // 100ms stereo
}

func TestDetectSignature(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		ok   bool
	}{
		{"wav", testWAV(t), true},
		{"mp3", []byte{0xFF, 0xFB, 0x90, 0x00}, false},
		{"ogg", []byte("OggS\x00\x02"), false},
		{"webm-not-riff", []byte{0x1A, 0x45, 0xDF, 0xA3, 0, 0, 0, 0}, false},
		{"svg", []byte("<svg xmlns='http://www.w3.org/2000/svg'></svg>"), false},
		{"html", []byte("<html><body>hi</body></html>"), false},
		{"pe-executable", []byte("MZ\x90\x00\x03\x00\x00\x00"), false},
		{"riff-but-not-wave", append([]byte("RIFF\x00\x00\x00\x00"), []byte("WEBP")...), false},
		{"empty", []byte{}, false},
		{"truncated-riff", []byte("RIFF"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := DetectSignature(tc.data)
			if ok != tc.ok {
				t.Errorf("DetectSignature(%s) ok = %v, want %v", tc.name, ok, tc.ok)
			}
		})
	}
}

func TestVerifyTypeAgreementAcceptsWAV(t *testing.T) {
	data := testWAV(t)
	mt, err := VerifyTypeAgreement(data, "wav", "audio/wav")
	if err != nil {
		t.Fatalf("VerifyTypeAgreement() error = %v", err)
	}
	if mt != MediaWAV {
		t.Errorf("VerifyTypeAgreement() = %v, want %v", mt, MediaWAV)
	}
}

func TestVerifyTypeAgreementRejectsExtensionMismatch(t *testing.T) {
	data := testWAV(t)
	if _, err := VerifyTypeAgreement(data, "mp3", ""); !errors.Is(err, ErrUnsupported) {
		t.Errorf("VerifyTypeAgreement() error = %v, want ErrUnsupported", err)
	}
}

func TestVerifyTypeAgreementRejectsDeclaredMediaTypeMismatch(t *testing.T) {
	data := testWAV(t)
	if _, err := VerifyTypeAgreement(data, "", "audio/mpeg"); !errors.Is(err, ErrUnsupported) {
		t.Errorf("VerifyTypeAgreement() error = %v, want ErrUnsupported", err)
	}
}

func TestVerifyTypeAgreementIgnoresOctetStream(t *testing.T) {
	data := testWAV(t)
	if _, err := VerifyTypeAgreement(data, "", "application/octet-stream"); err != nil {
		t.Errorf("VerifyTypeAgreement() error = %v, want nil (octet-stream is not a claim)", err)
	}
}

func TestValidateWAVAcceptsCanonicalMonoAndStereo(t *testing.T) {
	for _, channels := range []uint16{1, 2} {
		data := buildWAV(t, 48000, channels, 16, 4800) // 100ms
		durationMS, err := ValidateWAV(data)
		if err != nil {
			t.Fatalf("ValidateWAV(channels=%d) error = %v", channels, err)
		}
		if durationMS < 90 || durationMS > 110 {
			t.Errorf("ValidateWAV(channels=%d) duration = %dms, want ~100ms", channels, durationMS)
		}
	}
}

func TestValidateWAVDurationIsExact(t *testing.T) {
	data := buildWAV(t, 8000, 1, 16, 8000) // exactly 1 second, mono, 8kHz
	durationMS, err := ValidateWAV(data)
	if err != nil {
		t.Fatalf("ValidateWAV() error = %v", err)
	}
	if durationMS != 1000 {
		t.Errorf("ValidateWAV() duration = %dms, want exactly 1000ms", durationMS)
	}
}

func TestValidateWAVSkipsUnknownChunks(t *testing.T) {
	base := buildWAV(t, 44100, 2, 16, 100)
	// Insert a LIST metadata chunk (id + 4-byte LE size + 4-byte body)
	// between fmt and data.
	listChunk := make([]byte, 12)
	copy(listChunk[0:4], "LIST")
	binary.LittleEndian.PutUint32(listChunk[4:8], 4)
	copy(listChunk[8:12], "INFO")
	withList := append(append(append([]byte{}, base[:36]...), listChunk...), base[36:]...)
	copy(withList[0:4], "RIFF")
	binary.LittleEndian.PutUint32(withList[4:8], uint32(len(withList)-8))
	if _, err := ValidateWAV(withList); err != nil {
		t.Errorf("ValidateWAV() with an unknown LIST chunk error = %v, want nil", err)
	}
}

func TestValidateWAVRejectsMissingRIFF(t *testing.T) {
	if _, err := ValidateWAV([]byte("not a wav file at all")); !errors.Is(err, ErrUnsupported) {
		t.Errorf("ValidateWAV() error = %v, want ErrUnsupported", err)
	}
}

func TestValidateWAVRejectsMissingFmtChunk(t *testing.T) {
	data := testWAV(t)
	// Corrupt the "fmt " chunk ID so it's never recognized.
	copy(data[12:16], "xxxx")
	if _, err := ValidateWAV(data); !errors.Is(err, ErrUnsupported) {
		t.Errorf("ValidateWAV() error = %v, want ErrUnsupported", err)
	}
}

func TestValidateWAVRejectsMissingDataChunk(t *testing.T) {
	data := testWAV(t)
	copy(data[36:40], "xxxx")
	if _, err := ValidateWAV(data); !errors.Is(err, ErrUnsupported) {
		t.Errorf("ValidateWAV() error = %v, want ErrUnsupported", err)
	}
}

func TestValidateWAVRejectsNonPCMFormatTag(t *testing.T) {
	data := testWAV(t)
	binary.LittleEndian.PutUint16(data[20:22], 0xFFFE) // WAVE_FORMAT_EXTENSIBLE
	if _, err := ValidateWAV(data); !errors.Is(err, ErrUnsupported) {
		t.Errorf("ValidateWAV() error = %v, want ErrUnsupported", err)
	}
}

func TestValidateWAVRejectsNonCanonicalFmtChunkSize(t *testing.T) {
	// Build a WAV whose fmt chunk is 18 bytes (extended, non-canonical).
	data := testWAV(t)
	binary.LittleEndian.PutUint32(data[16:20], 18)
	extended := append(append([]byte{}, data[:36]...), 0, 0)
	extended = append(extended, data[36:]...)
	if _, err := ValidateWAV(extended); !errors.Is(err, ErrUnsupported) {
		t.Errorf("ValidateWAV() error = %v, want ErrUnsupported", err)
	}
}

func TestValidateWAVRejectsWrongBitDepth(t *testing.T) {
	for _, bits := range []uint16{8, 24, 32} {
		data := buildWAV(t, 44100, 2, bits, 100)
		if _, err := ValidateWAV(data); !errors.Is(err, ErrUnsupported) {
			t.Errorf("ValidateWAV(bits=%d) error = %v, want ErrUnsupported", bits, err)
		}
	}
}

func TestValidateWAVRejectsWrongChannelCount(t *testing.T) {
	for _, channels := range []uint16{0, 3, 6} {
		data := testWAV(t)
		binary.LittleEndian.PutUint16(data[22:24], channels)
		if _, err := ValidateWAV(data); !errors.Is(err, ErrUnsupported) {
			t.Errorf("ValidateWAV(channels=%d) error = %v, want ErrUnsupported", channels, err)
		}
	}
}

func TestValidateWAVRejectsSampleRateOutOfRange(t *testing.T) {
	for _, rate := range []uint32{4000, 500000} {
		data := testWAV(t)
		binary.LittleEndian.PutUint32(data[24:28], rate)
		if _, err := ValidateWAV(data); !errors.Is(err, ErrUnsupported) {
			t.Errorf("ValidateWAV(rate=%d) error = %v, want ErrUnsupported", rate, err)
		}
	}
}

func TestValidateWAVRejectsEmptyDataChunk(t *testing.T) {
	data := buildWAV(t, 44100, 2, 16, 0)
	if _, err := ValidateWAV(data); !errors.Is(err, ErrUnsupported) {
		t.Errorf("ValidateWAV() error = %v, want ErrUnsupported", err)
	}
}

func TestValidateWAVRejectsDataSizePastEndOfFile(t *testing.T) {
	data := testWAV(t)
	// Declare a data chunk size far larger than the bytes actually
	// present - the declared size is never trusted alone.
	binary.LittleEndian.PutUint32(data[40:44], 0x7FFFFFFF)
	if _, err := ValidateWAV(data); !errors.Is(err, ErrUnsupported) {
		t.Errorf("ValidateWAV() error = %v, want ErrUnsupported", err)
	}
}

func TestValidateWAVRejectsTruncatedFile(t *testing.T) {
	data := testWAV(t)
	if _, err := ValidateWAV(data[:20]); !errors.Is(err, ErrUnsupported) {
		t.Errorf("ValidateWAV() error = %v, want ErrUnsupported", err)
	}
}

func TestValidateWAVRejectsDuplicateFmtChunk(t *testing.T) {
	data := testWAV(t)
	// Duplicate the fmt chunk (bytes 12..36) right after itself.
	dup := append(append([]byte{}, data[:36]...), data[12:36]...)
	dup = append(dup, data[36:]...)
	copy(dup[0:4], "RIFF")
	binary.LittleEndian.PutUint32(dup[4:8], uint32(len(dup)-8))
	if _, err := ValidateWAV(dup); !errors.Is(err, ErrUnsupported) {
		t.Errorf("ValidateWAV() error = %v, want ErrUnsupported", err)
	}
}

func TestValidateWAVRejectsOverDurationLimit(t *testing.T) {
	// 31 seconds at 8kHz mono 16-bit - exceeds MaxSoundDurationMS (30s).
	data := buildWAV(t, 8000, 1, 16, 8000*31)
	if _, err := ValidateWAV(data); !errors.Is(err, ErrTooLarge) {
		t.Errorf("ValidateWAV() error = %v, want ErrTooLarge", err)
	}
}

func TestValidateWAVDoesNotPanicOnRandomBytes(t *testing.T) {
	// Fuzz-style smoke test: a handful of adversarial-shaped byte
	// sequences must never panic, only ever return an error.
	cases := [][]byte{
		nil,
		{},
		{0},
		[]byte("RIFF\xff\xff\xff\xffWAVE"),
		[]byte("RIFFxxxxWAVEfmt \xff\xff\xff\xff"),
		append([]byte("RIFF\x00\x00\x00\x00WAVE"), make([]byte, 4)...),
	}
	for i, data := range cases {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("case %d: ValidateWAV panicked: %v", i, r)
				}
			}()
			_, _ = ValidateWAV(data)
		}()
	}
}
