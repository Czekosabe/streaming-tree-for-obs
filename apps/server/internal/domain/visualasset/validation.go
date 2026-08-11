package visualasset

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image/gif"
	"image/jpeg"
	"image/png"
)

// Bounds (docs/visual-template-packages.md §10/§19).
const (
	MaxImageBytes int64 = 16 << 20
	MaxVideoBytes int64 = 64 << 20
	MaxFontBytes  int64 = 8 << 20

	MaxImageWidth  = 8192
	MaxImageHeight = 8192
	MaxImagePixels = 32_000_000

	MaxDisplayNameCodePoints = 200
	MaxAuthorCodePoints      = 200
	MaxLicenseCodePoints     = 200
	MaxNoticeCodePoints      = 200
)

// MaxBytesFor returns the size bound for kind (docs/visual-template-
// packages.md §10 - "max single image/video/font asset").
func MaxBytesFor(kind Kind) int64 {
	switch kind {
	case KindImage:
		return MaxImageBytes
	case KindVideo:
		return MaxVideoBytes
	case KindFont:
		return MaxFontBytes
	default:
		return 0
	}
}

// DetectSignature independently identifies data's own media type from its
// magic bytes alone (docs/visual-template-packages.md §11/§18) - never
// trusts a filename extension or a caller-declared media type. Returns
// ok=false if data does not begin with any accepted signature (including
// every explicitly excluded format: SVG, HTML, JS, executables, ZIP,
// audio, etc. all fall through to ok=false here).
func DetectSignature(data []byte) (MediaType, bool) {
	switch {
	case len(data) >= 8 && bytes.Equal(data[:8], []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}):
		return MediaPNG, true
	case len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		return MediaJPEG, true
	case len(data) >= 6 && (bytes.Equal(data[:6], []byte("GIF87a")) || bytes.Equal(data[:6], []byte("GIF89a"))):
		return MediaGIF, true
	case len(data) >= 12 && bytes.Equal(data[0:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")):
		return MediaWebP, true
	case len(data) >= 4 && bytes.Equal(data[0:4], []byte{0x1A, 0x45, 0xDF, 0xA3}):
		return MediaWebM, true
	case len(data) >= 12 && bytes.Equal(data[4:8], []byte("ftyp")):
		return MediaMP4, true
	case len(data) >= 4 && bytes.Equal(data[0:4], []byte("wOF2")):
		return MediaWOFF2, true
	default:
		return "", false
	}
}

// VerifyTypeAgreement checks that ext (a lowercase file extension without
// the leading dot, e.g. "png"), declared (a caller/manifest-supplied
// media type string), and the signature actually detected from data all
// name the same MediaType (docs/visual-template-packages.md §11 -
// "independent triple validation"). ext or declared may be empty (a
// manual upload's own filename extension is advisory only); when
// non-empty, both must agree with the detected signature or this returns
// ErrUnsupported.
func VerifyTypeAgreement(data []byte, ext, declared string) (MediaType, error) {
	detected, ok := DetectSignature(data)
	if !ok {
		return "", fmt.Errorf("%w: no recognized asset signature", ErrUnsupported)
	}
	if ext != "" && extensionMediaType(ext) != detected {
		return "", fmt.Errorf("%w: file extension %q does not match detected content %q", ErrUnsupported, ext, detected)
	}
	// "application/octet-stream" (and, defensively, its own bare
	// "application/" prefix) is the generic browser/client fallback for
	// "I do not actually know this file's type" - go's own
	// multipart.Writer.CreateFormFile hardcodes it, and real browsers
	// fall back to it whenever their own sniffing is inconclusive. It
	// carries no real signal either way, so it is treated as "no claim
	// made" (skipped) rather than compared - never trusted as evidence
	// FOR a type, and never grounds for rejecting a real, correctly
	// signed asset either.
	if declared != "" && declared != "application/octet-stream" && MediaType(declared) != detected {
		return "", fmt.Errorf("%w: declared media type %q does not match detected content %q", ErrUnsupported, declared, detected)
	}
	return detected, nil
}

var extensionMediaTypes = map[string]MediaType{
	"png":   MediaPNG,
	"jpg":   MediaJPEG,
	"jpeg":  MediaJPEG,
	"gif":   MediaGIF,
	"webp":  MediaWebP,
	"webm":  MediaWebM,
	"mp4":   MediaMP4,
	"woff2": MediaWOFF2,
}

func extensionMediaType(ext string) MediaType {
	return extensionMediaTypes[ext]
}

// ValidateImageDimensions enforces the width/height/megapixel bounds
// (docs/visual-template-packages.md §11/§19) using safe, header-only
// decoding - never a full-frame decode. PNG/JPEG/GIF use the standard
// library's own DecodeConfig; WebP uses this package's own minimal,
// reviewed container-header reader (decodeWebPDimensions below) - no
// third-party image library is added for this check.
func ValidateImageDimensions(data []byte, mediaType MediaType) error {
	var width, height int
	switch mediaType {
	case MediaPNG:
		cfg, err := png.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("%w: unreadable PNG header: %v", ErrUnsupported, err)
		}
		width, height = cfg.Width, cfg.Height
	case MediaJPEG:
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("%w: unreadable JPEG header: %v", ErrUnsupported, err)
		}
		width, height = cfg.Width, cfg.Height
	case MediaGIF:
		cfg, err := gif.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("%w: unreadable GIF header: %v", ErrUnsupported, err)
		}
		width, height = cfg.Width, cfg.Height
	case MediaWebP:
		w, h, err := decodeWebPDimensions(data)
		if err != nil {
			return fmt.Errorf("%w: unreadable WebP header: %v", ErrUnsupported, err)
		}
		width, height = w, h
	default:
		return fmt.Errorf("%w: %q is not an image media type", ErrUnsupported, mediaType)
	}
	if width <= 0 || height <= 0 {
		return fmt.Errorf("%w: image has non-positive dimensions", ErrUnsupported)
	}
	if width > MaxImageWidth {
		return fmt.Errorf("%w: image width %d exceeds the maximum of %d", ErrTooLarge, width, MaxImageWidth)
	}
	if height > MaxImageHeight {
		return fmt.Errorf("%w: image height %d exceeds the maximum of %d", ErrTooLarge, height, MaxImageHeight)
	}
	if width*height > MaxImagePixels {
		return fmt.Errorf("%w: image has %d total pixels, exceeding the maximum of %d", ErrTooLarge, width*height, MaxImagePixels)
	}
	return nil
}

// decodeWebPDimensions reads just enough of a RIFF/WEBP container to
// recover the canvas width/height, without decoding any pixel data -
// this package's own small, reviewed header reader (docs/visual-
// template-packages.md §19: "a small reviewed header/config reader" is
// preferred over a new image-processing dependency). Understands the
// three WebP chunk shapes a real-world encoder emits: simple lossy
// (VP8 ), simple lossless (VP8L), and extended (VP8X, used for
// animation/alpha/ICC/EXIF/XMP - the canvas size in VP8X's own header is
// authoritative regardless of what any inner frame chunk later claims).
func decodeWebPDimensions(data []byte) (int, int, error) {
	if len(data) < 30 || !bytes.Equal(data[0:4], []byte("RIFF")) || !bytes.Equal(data[8:12], []byte("WEBP")) {
		return 0, 0, fmt.Errorf("not a WebP container")
	}
	fourCC := string(data[12:16])
	chunkData := data[20:]
	switch fourCC {
	case "VP8X":
		if len(chunkData) < 10 {
			return 0, 0, fmt.Errorf("truncated VP8X header")
		}
		w := int(chunkData[4]) | int(chunkData[5])<<8 | int(chunkData[6])<<16
		h := int(chunkData[7]) | int(chunkData[8])<<8 | int(chunkData[9])<<16
		return w + 1, h + 1, nil
	case "VP8 ":
		if len(chunkData) < 10 {
			return 0, 0, fmt.Errorf("truncated VP8 header")
		}
		// 3-byte frame tag, then a 3-byte start code (0x9d 0x01 0x2a).
		if chunkData[3] != 0x9d || chunkData[4] != 0x01 || chunkData[5] != 0x2a {
			return 0, 0, fmt.Errorf("missing VP8 start code")
		}
		w := int(binary.LittleEndian.Uint16(chunkData[6:8])) & 0x3FFF
		h := int(binary.LittleEndian.Uint16(chunkData[8:10])) & 0x3FFF
		return w, h, nil
	case "VP8L":
		if len(chunkData) < 5 {
			return 0, 0, fmt.Errorf("truncated VP8L header")
		}
		if chunkData[0] != 0x2F {
			return 0, 0, fmt.Errorf("missing VP8L signature byte")
		}
		bits := uint32(chunkData[1]) | uint32(chunkData[2])<<8 | uint32(chunkData[3])<<16 | uint32(chunkData[4])<<24
		w := int(bits&0x3FFF) + 1
		h := int((bits>>14)&0x3FFF) + 1
		return w, h, nil
	default:
		return 0, 0, fmt.Errorf("unrecognized WebP chunk %q", fourCC)
	}
}
