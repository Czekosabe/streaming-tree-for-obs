//go:build windows

package tray

import (
	"syscall"
	"testing"
)

// A real bug, found by re-reading this package's own NOTIFYICONDATAW
// construction against the documented NIF_* flag table: adding the
// icon under NOTIFYICON_VERSION_4 without NIF_SHOWTIP silently
// suppresses the standard hover tooltip entirely, so the tray showed
// no "Streaming Tree for OBS" identification on hover at all. This
// locks the fix.
func TestAddIconFlagsIncludesShowTip(t *testing.T) {
	flags := addIconFlags()
	if flags&nifShowTip == 0 {
		t.Fatalf("addIconFlags() = %#x, missing NIF_SHOWTIP (%#x) - the hover tooltip would be suppressed under NOTIFYICON_VERSION_4", flags, uint32(nifShowTip))
	}
	if flags&nifTip == 0 {
		t.Fatalf("addIconFlags() = %#x, missing NIF_TIP (%#x) - szTip would never be read at all", flags, uint32(nifTip))
	}
}

func TestLoadIconFromICOBytesRealAsset(t *testing.T) {
	hIcon, err := loadIconFromICOBytes(IconICO)
	if err != nil {
		t.Fatalf("loadIconFromICOBytes(IconICO) error = %v", err)
	}
	if hIcon == 0 {
		t.Fatal("loadIconFromICOBytes(IconICO) returned a zero HICON")
	}
	if ret, _, _ := procDestroyIcon.Call(uintptr(hIcon)); ret == 0 {
		t.Fatal("DestroyIcon failed on the icon this test created")
	}
}

func TestLoadIconFromICOBytesRejectsGarbage(t *testing.T) {
	cases := map[string][]byte{
		"empty":              nil,
		"too short":          {0, 0, 1, 0},
		"zero-length header": {0, 0, 1, 0, 0, 0},
		"truncated directory": func() []byte {
			b := make([]byte, 6+16)
			b[4], b[5] = 1, 0 // count = 1, but no image bytes follow
			return b[:10]     // shorter than 6+16
		}(),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := loadIconFromICOBytes(data); err == nil {
				t.Fatalf("loadIconFromICOBytes(%s) succeeded, want an error", name)
			}
		})
	}
}

func TestCopyUTF16TruncatesAndNulTerminates(t *testing.T) {
	dst := make([]uint16, 4)
	copyUTF16(dst, "hello")

	decoded := syscall.UTF16ToString(dst)
	if decoded != "hel" {
		t.Fatalf("copyUTF16 truncated result = %q, want %q", decoded, "hel")
	}
	if dst[3] != 0 {
		t.Fatalf("copyUTF16 did not NUL-terminate a fully-filled buffer: dst = %v", dst)
	}
}

func TestCopyUTF16FitsWithoutTruncation(t *testing.T) {
	dst := make([]uint16, 8)
	copyUTF16(dst, "hi")

	decoded := syscall.UTF16ToString(dst)
	if decoded != "hi" {
		t.Fatalf("copyUTF16 result = %q, want %q", decoded, "hi")
	}
}
