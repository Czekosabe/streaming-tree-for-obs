package visualpackage

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"testing"

	"github.com/streaming-tree/server/internal/domain/visualdesign"
)

// validPNGBytes returns a tiny, well-formed 4x4 PNG - a real, valid
// signature/container, not a hand-rolled byte literal, so every test
// using it is exercising the real png.DecodeConfig path.
func validPNGBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 60), G: uint8(y * 60), B: 100, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode test PNG: %v", err)
	}
	return buf.Bytes()
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// zipEntry is one raw entry to write via buildZipCustom - deliberately
// exposes the exact same knobs a hostile archive builder would use
// (raw name, raw content, and a mode), so the security matrix tests
// exercise validateEntryPath/mode-checking directly rather than only
// exercising what archive/zip's own high-level Writer.Create would ever
// produce naturally.
type zipEntry struct {
	name string
	data []byte
	mode fs.FileMode // 0 = regular file; e.g. fs.ModeSymlink for a symlink entry
}

func buildZipCustom(t *testing.T, entries []zipEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		hdr := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		if e.mode != 0 {
			hdr.SetMode(e.mode)
		}
		fw, err := zw.CreateHeader(hdr)
		if err != nil {
			t.Fatalf("create entry %q: %v", e.name, err)
		}
		if _, err := fw.Write(e.data); err != nil {
			t.Fatalf("write entry %q: %v", e.name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	return buf.Bytes()
}

// validManifestAndTemplate returns a minimal, fully valid manifest.json/
// template.json/asset triple: one image asset referenced by exactly one
// image layer, everything within bounds - the "known good" baseline
// every mutation test starts from.
func validPackageParts(t *testing.T) (manifestJSON, templateJSON, pngData []byte) {
	t.Helper()
	png := validPNGBytes(t)
	hash := sha256Hex(png)

	manifest := `{
		"format": "streaming-tree-template-package",
		"schemaVersion": 1,
		"templatePath": "template.json",
		"assets": [{
			"id": "pkgasset_0001",
			"path": "assets/pkgasset_0001.png",
			"kind": "image",
			"mediaType": "image/png",
			"sha256": "` + hash + `",
			"sizeBytes": ` + itoa(len(png)) + `,
			"displayName": "Corner badge",
			"author": "",
			"license": "",
			"notice": ""
		}]
	}`

	template := `{
		"format": "streaming-tree-visual-template",
		"schemaVersion": 1,
		"target": "alert",
		"name": "Package test template",
		"description": "",
		"author": "",
		"license": "",
		"visualDesign": {
			"version": 3,
			"canvas": {"width": 1920, "height": 1080, "transparent": true},
			"layers": [{
				"id": "layer-1",
				"name": "Badge",
				"kind": "image",
				"visible": true,
				"locked": false,
				"order": 0,
				"frame": {"x": 0, "y": 0, "width": 200, "height": 200},
				"opacity": 1,
				"image": {"assetId": "pkgasset_0001", "fit": "contain", "alt": ""},
				"entryAnimation": "none",
				"exitAnimation": "none",
				"animationDurationMs": 0
			}]
		}
	}`
	return []byte(manifest), []byte(template), png
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}

// assetFreeDocument returns a minimal, valid Version3 document with no
// asset reference at all - used to prove package export/import still
// works for a template that carries no managed asset (docs/visual-
// template-packages.md §21's "an asset-free template exports as a
// valid package with zero assets").
func assetFreeDocument(t *testing.T) visualdesign.Document {
	t.Helper()
	return visualdesign.Document{
		Version: visualdesign.Version3,
		Canvas:  visualdesign.CanvasLandscape,
		Layers: []visualdesign.Layer{{
			ID: "layer-1", Name: "Rect", Kind: visualdesign.LayerShape,
			Visible: true, Order: 0,
			Frame: visualdesign.Frame{X: 0, Y: 0, Width: 100, Height: 100}, Opacity: 1,
			Shape: &visualdesign.ShapeProps{Kind: visualdesign.ShapeRectangle, Fill: "#112233"},
			EntryAnimation: visualdesign.AnimationNone, ExitAnimation: visualdesign.AnimationNone,
		}},
	}
}

func validPackageZip(t *testing.T) []byte {
	t.Helper()
	manifest, template, png := validPackageParts(t)
	return buildZipCustom(t, []zipEntry{
		{name: "manifest.json", data: manifest},
		{name: TemplatePath, data: template},
		{name: "assets/pkgasset_0001.png", data: png},
	})
}
