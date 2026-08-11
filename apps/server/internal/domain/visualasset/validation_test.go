package visualasset

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 1, G: 2, B: 3, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode test png: %v", err)
	}
	return buf.Bytes()
}

func TestDetectSignature(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want MediaType
		ok   bool
	}{
		{"png", testPNG(t, 2, 2), MediaPNG, true},
		{"jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0, 'J', 'F', 'I', 'F'}, MediaJPEG, true},
		{"gif87a", append([]byte("GIF87a"), make([]byte, 10)...), MediaGIF, true},
		{"gif89a", append([]byte("GIF89a"), make([]byte, 10)...), MediaGIF, true},
		{"webm", []byte{0x1A, 0x45, 0xDF, 0xA3, 0, 0, 0, 0}, MediaWebM, true},
		{"woff2", []byte("wOF2" + "restofheader"), MediaWOFF2, true},
		{"svg", []byte("<svg xmlns='http://www.w3.org/2000/svg'></svg>"), "", false},
		{"html", []byte("<html><body>hi</body></html>"), "", false},
		{"js", []byte("alert(1)"), "", false},
		{"pe-executable", []byte("MZ\x90\x00\x03\x00\x00\x00"), "", false},
		{"empty", []byte{}, "", false},
		{"zip", []byte("PK\x03\x04"), "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := DetectSignature(tc.data)
			if ok != tc.ok {
				t.Fatalf("DetectSignature() ok = %v, want %v", ok, tc.ok)
			}
			if ok && got != tc.want {
				t.Errorf("DetectSignature() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestVerifyTypeAgreement_MismatchRejected(t *testing.T) {
	png := testPNG(t, 2, 2)
	if _, err := VerifyTypeAgreement(png, "jpg", ""); !errors.Is(err, ErrUnsupported) {
		t.Errorf("expected ErrUnsupported for extension mismatch, got %v", err)
	}
	if _, err := VerifyTypeAgreement(png, "", "image/jpeg"); !errors.Is(err, ErrUnsupported) {
		t.Errorf("expected ErrUnsupported for declared media type mismatch, got %v", err)
	}
	if _, err := VerifyTypeAgreement([]byte("not an image at all"), "", ""); !errors.Is(err, ErrUnsupported) {
		t.Errorf("expected ErrUnsupported for unrecognized content, got %v", err)
	}
	mt, err := VerifyTypeAgreement(png, "png", "image/png")
	if err != nil {
		t.Fatalf("expected agreement to succeed, got %v", err)
	}
	if mt != MediaPNG {
		t.Errorf("got %q, want image/png", mt)
	}
}

func TestValidateImageDimensions_Bounds(t *testing.T) {
	ok := testPNG(t, 100, 100)
	if err := ValidateImageDimensions(ok, MediaPNG); err != nil {
		t.Errorf("expected a normal-sized image to pass, got %v", err)
	}

	tooWide := testPNG(t, MaxImageWidth+1, 10)
	if err := ValidateImageDimensions(tooWide, MediaPNG); !errors.Is(err, ErrTooLarge) {
		t.Errorf("expected ErrTooLarge for width over the bound, got %v", err)
	}

	tooTall := testPNG(t, 10, MaxImageHeight+1)
	if err := ValidateImageDimensions(tooTall, MediaPNG); !errors.Is(err, ErrTooLarge) {
		t.Errorf("expected ErrTooLarge for height over the bound, got %v", err)
	}
}

func TestDecodeWebPDimensions_VP8X(t *testing.T) {
	// A minimal, hand-built VP8X-shaped RIFF/WEBP header declaring a
	// 100x50 canvas (width-1=99, height-1=49, 24-bit little-endian).
	data := []byte{
		'R', 'I', 'F', 'F', 0, 0, 0, 0, 'W', 'E', 'B', 'P',
		'V', 'P', '8', 'X', 10, 0, 0, 0, // chunk size
		0, 0, 0, 0, // flags + reserved
		99, 0, 0, // width-1 = 99
		49, 0, 0, // height-1 = 49
	}
	w, h, err := decodeWebPDimensions(data)
	if err != nil {
		t.Fatalf("decodeWebPDimensions() returned an error: %v", err)
	}
	if w != 100 || h != 50 {
		t.Errorf("got %dx%d, want 100x50", w, h)
	}
}

func TestMaxBytesFor(t *testing.T) {
	if MaxBytesFor(KindImage) != MaxImageBytes {
		t.Errorf("image bound mismatch")
	}
	if MaxBytesFor(KindVideo) != MaxVideoBytes {
		t.Errorf("video bound mismatch")
	}
	if MaxBytesFor(KindFont) != MaxFontBytes {
		t.Errorf("font bound mismatch")
	}
	if MaxBytesFor("bogus") != 0 {
		t.Errorf("unrecognized kind should bound to 0")
	}
}
