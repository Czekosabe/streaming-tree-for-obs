//go:build ignore

// Generates apps/server/internal/runtime/tray/assets/tray.ico - the
// Windows system-tray icon (Stage 20E §4).
//
// No final branding art exists for this project (see
// apps/web/src/components/layout/BrandMark.tsx's own doc comment: "No
// third-party logo or artwork is used anywhere in the application").
// This program reuses BrandMark's own existing neutral first-party
// visual identity - the rounded-square accent-to-accent-deep gradient
// (apps/web/src/index.css: --color-accent #8b5cf6, --color-accent-deep
// #6d28d9) - rather than inventing new artwork, and renders a plain
// minimal glyph rather than attempting to reproduce BrandMark's Lucide
// "Network" icon pixel-for-pixel: a faithful reproduction of a
// multi-path vector glyph is illegible at real tray sizes (16x16/20x20
// on a real Windows notification area), and an approximation would be
// dishonest as "the app's icon". This is recorded here, and in
// docs/windows-tray.md, as a real limitation - the tray icon should be
// replaced with real, designed branding art once that exists, at which
// point this generator is no longer needed.
//
// Run with: go run scripts/generate-tray-icon.go
package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
)

const size = 32

// accent/accentDeep mirror apps/web/src/index.css's own
// --color-accent/--color-accent-deep exactly.
var (
	accent     = color.RGBA{0x8b, 0x5c, 0xf6, 0xff}
	accentDeep = color.RGBA{0x6d, 0x28, 0xd9, 0xff}
	white      = color.RGBA{0xff, 0xff, 0xff, 0xff}
)

func lerp(a, b uint8, t float64) uint8 {
	return uint8(float64(a) + (float64(b)-float64(a))*t)
}

func main() {
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	const radius = 7.0 // rounded-square corner radius, matching BrandMark's rounded-lg
	center := float64(size-1) / 2

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			fx, fy := float64(x)+0.5, float64(y)+0.5

			// Rounded-square mask: outside the rounded corners is transparent,
			// so the tray's own background shows through instead of a hard
			// square edge.
			if !insideRoundedSquare(fx, fy, size, radius) {
				img.Set(x, y, color.RGBA{})
				continue
			}

			// Diagonal (top-left to bottom-right) gradient, matching
			// BrandMark's `bg-linear-to-br from-accent to-accent-deep`.
			t := ((fx + fy) / 2) / float64(size)
			bg := color.RGBA{
				R: lerp(accent.R, accentDeep.R, t),
				G: lerp(accent.G, accentDeep.G, t),
				B: lerp(accent.B, accentDeep.B, t),
				A: 0xff,
			}

			// A simple white ring-and-dot mark in the center - a minimal,
			// legible placeholder glyph rather than an attempted
			// reproduction of BrandMark's Lucide "Network" icon (see the
			// package doc comment above).
			dx, dy := fx-center, fy-center
			dist := dx*dx + dy*dy
			const outerR, innerR, dotR = 10.5, 8.0, 2.6
			switch {
			case dist <= dotR*dotR:
				img.Set(x, y, white)
			case dist <= outerR*outerR && dist >= innerR*innerR:
				img.Set(x, y, white)
			default:
				img.Set(x, y, bg)
			}
		}
	}

	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		log.Fatalf("encode png: %v", err)
	}

	icoBuf := wrapAsICO(pngBuf.Bytes(), size)

	outPath := "apps/server/internal/runtime/tray/assets/tray.ico"
	if err := os.WriteFile(outPath, icoBuf, 0o644); err != nil {
		log.Fatalf("write %s: %v", outPath, err)
	}
	log.Printf("wrote %s (%d bytes)", outPath, len(icoBuf))
}

// insideRoundedSquare reports whether (x, y) falls inside a size x size
// square with corner radius r.
func insideRoundedSquare(x, y float64, sizeF int, r float64) bool {
	s := float64(sizeF)
	cx, cy := x, y
	// Clamp to the nearest corner-circle center, then test distance -
	// the standard rounded-rect signed-distance shortcut.
	nx := clamp(cx, r, s-r)
	ny := clamp(cy, r, s-r)
	if cx >= r && cx <= s-r || cy >= r && cy <= s-r {
		return cx >= 0 && cx <= s && cy >= 0 && cy <= s
	}
	dx, dy := cx-nx, cy-ny
	return dx*dx+dy*dy <= r*r
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// wrapAsICO wraps one PNG image as a single-frame Windows .ico file -
// the PNG-frame ICO format Windows has supported since Vista
// (documented at https://learn.microsoft.com/windows/win32/ee416%28v=vs.85%29
// via https://learn.microsoft.com/openspecs/windows_protocols/ms-icoa/
// -- ICONDIR is 6 bytes, followed by one 16-byte ICONDIRENTRY per
// frame, followed by each frame's raw image bytes).
func wrapAsICO(pngBytes []byte, dim int) []byte {
	var buf bytes.Buffer

	// ICONDIR: reserved(2)=0, type(2)=1 (icon), count(2)=1.
	binary.Write(&buf, binary.LittleEndian, uint16(0))
	binary.Write(&buf, binary.LittleEndian, uint16(1))
	binary.Write(&buf, binary.LittleEndian, uint16(1))

	widthByte := byte(dim)  // 0 means 256, not needed at 32x32
	heightByte := byte(dim)
	const headerSize = 6
	const entrySize = 16

	// ICONDIRENTRY.
	buf.WriteByte(widthByte)
	buf.WriteByte(heightByte)
	buf.WriteByte(0) // color palette: none (true color)
	buf.WriteByte(0) // reserved
	binary.Write(&buf, binary.LittleEndian, uint16(1))  // color planes
	binary.Write(&buf, binary.LittleEndian, uint16(32)) // bits per pixel
	binary.Write(&buf, binary.LittleEndian, uint32(len(pngBytes)))
	binary.Write(&buf, binary.LittleEndian, uint32(headerSize+entrySize))

	buf.Write(pngBytes)
	return buf.Bytes()
}
