package visualpackage

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"
)

// ExportAsset is one asset to embed while writing a package - the
// manifest entry plus its own already-resolved bytes (docs/visual-
// template-packages.md §20: "exactly the assets the document
// transitively references").
type ExportAsset struct {
	Manifest ManifestAsset
	Data     []byte
}

// ExportAudioAsset is ExportAsset's own audio-asset counterpart
// (docs/alert-audio.md §10.4) - at most one in practice today (a
// RuleAudioPreset carries a single SoundAssetID), written under
// "audio/<path>" rather than "assets/<path>".
type ExportAudioAsset struct {
	Manifest ManifestAudioAsset
	Data     []byte
}

// deterministicModTime is used for every archive entry's own timestamp,
// rather than wall-clock time, so that exporting the same template twice
// produces the same manifest/template bytes and the same per-entry
// metadata (docs/visual-template-packages.md §51: "stable archive
// timestamps rather than wall-clock where ZIP allows"). Compression
// itself is not guaranteed byte-identical across runs of the standard
// library's flate writer, so this package claims semantic determinism,
// not a byte-for-byte guarantee.
var deterministicModTime = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

// WriteArchive writes a complete, valid `.streaming-tree-template`
// package to w: manifest.json, template.json, assets/<path> for every
// visual asset, and audio/<path> for every audio asset, in a stable,
// sorted order (docs/visual-template-packages.md §4/§20/§51, docs/
// alert-audio.md §10.2/§10.4). manifest.Assets/manifest.AudioAssets are
// expected already sorted and already describing exactly the assets/
// audioAssets slices passed in - callers build both together (see
// service.go's ExportTemplate).
func WriteArchive(w io.Writer, manifest Manifest, templateJSON []byte, assets []ExportAsset, audioAssets []ExportAudioAsset) error {
	zw := zip.NewWriter(w)

	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	if err := writeZipEntry(zw, "manifest.json", manifestJSON); err != nil {
		return err
	}
	if err := writeZipEntry(zw, TemplatePath, templateJSON); err != nil {
		return err
	}

	sorted := make([]ExportAsset, len(assets))
	copy(sorted, assets)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Manifest.Path < sorted[j].Manifest.Path })

	for _, a := range sorted {
		if err := writeZipEntry(zw, a.Manifest.Path, a.Data); err != nil {
			return err
		}
	}

	sortedAudio := make([]ExportAudioAsset, len(audioAssets))
	copy(sortedAudio, audioAssets)
	sort.Slice(sortedAudio, func(i, j int) bool { return sortedAudio[i].Manifest.Path < sortedAudio[j].Manifest.Path })

	for _, a := range sortedAudio {
		if err := writeZipEntry(zw, a.Manifest.Path, a.Data); err != nil {
			return err
		}
	}

	return zw.Close()
}

func writeZipEntry(zw *zip.Writer, name string, data []byte) error {
	header := &zip.FileHeader{
		Name:     name,
		Method:   zip.Deflate,
		Modified: deterministicModTime,
	}
	fw, err := zw.CreateHeader(header)
	if err != nil {
		return fmt.Errorf("create archive entry %q: %w", name, err)
	}
	if _, err := fw.Write(data); err != nil {
		return fmt.Errorf("write archive entry %q: %w", name, err)
	}
	return nil
}
