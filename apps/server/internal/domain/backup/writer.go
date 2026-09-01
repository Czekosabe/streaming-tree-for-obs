package backup

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"
)

// AssetBlobSource opens one content-addressed blob by its own SHA-256 -
// satisfied directly by *visualasset.FileStore and *audioasset's own
// identical FileStore instance (docs/backup-restore.md §5). Backup
// never constructs its own copy of a blob's bytes independently of
// this - the same store every manual upload and package import
// already reads and writes through.
type AssetBlobSource interface {
	Open(sha256Hex string) (io.ReadCloser, error)
}

// blobRef is one distinct blob a Config references, resolved once
// regardless of how many logical Asset rows point at it (content
// dedup - docs/backup-restore.md §1).
type blobRef struct {
	sha256   string
	byteSize int64
	root     string // "assets" or "audio"
	source   AssetBlobSource
}

func collectBlobRefs(cfg Config, visualBlobs, audioBlobs AssetBlobSource) []blobRef {
	seen := make(map[string]bool)
	var refs []blobRef

	for _, a := range cfg.VisualAssets {
		if a.Blob == nil || seen[a.Blob.SHA256] {
			continue
		}
		seen[a.Blob.SHA256] = true
		refs = append(refs, blobRef{sha256: a.Blob.SHA256, byteSize: a.Blob.ByteSize, root: "assets", source: visualBlobs})
	}
	for _, a := range cfg.AudioAssets {
		if a.Blob == nil || seen[a.Blob.SHA256] {
			continue
		}
		seen[a.Blob.SHA256] = true
		refs = append(refs, blobRef{sha256: a.Blob.SHA256, byteSize: a.Blob.ByteSize, root: "audio", source: audioBlobs})
	}

	// Deterministic archive entry order, independent of map iteration
	// order or the order Config's own slices happened to be built in.
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].root != refs[j].root {
			return refs[i].root < refs[j].root
		}
		return refs[i].sha256 < refs[j].sha256
	})
	return refs
}

// WriteArchive builds one complete, self-contained backup package from
// cfg and returns its full bytes - manifest, config.json, and every
// managed asset blob cfg's visual/audio assets actually reference
// (docs/backup-restore.md §5/§6). Never partial: an error here means
// nothing should be written to disk by the caller.
func WriteArchive(cfg Config, sourceAppVersion, sourcePlatform string, now time.Time, visualBlobs, audioBlobs AssetBlobSource) ([]byte, error) {
	configBytes, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal config: %v", ErrConfigInvalid, err)
	}
	if int64(len(configBytes)) > MaxConfigBytes {
		return nil, fmt.Errorf("%w: config payload is %d bytes, exceeding the %d byte limit", ErrTooLarge, len(configBytes), MaxConfigBytes)
	}
	configSum := sha256.Sum256(configBytes)

	refs := collectBlobRefs(cfg, visualBlobs, audioBlobs)

	manifest := Manifest{
		FormatVersion:    FormatVersion,
		Product:          Product,
		CreatedAt:        now.UTC(),
		SourceAppVersion: sourceAppVersion,
		SourcePlatform:   sourcePlatform,
		ConfigSHA256:     hex.EncodeToString(configSum[:]),
		ConfigSizeBytes:  int64(len(configBytes)),
	}
	for _, ref := range refs {
		manifest.Assets = append(manifest.Assets, ManifestAsset{
			SHA256: ref.sha256, Path: ref.root + "/" + ref.sha256, SizeBytes: ref.byteSize,
		})
	}

	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal manifest: %v", ErrManifestInvalid, err)
	}
	if int64(len(manifestBytes)) > MaxManifestBytes {
		return nil, fmt.Errorf("%w: manifest is %d bytes, exceeding the %d byte limit", ErrTooLarge, len(manifestBytes), MaxManifestBytes)
	}

	if 2+len(refs) > MaxArchiveEntries {
		return nil, fmt.Errorf("%w: backup has %d entries, exceeding the maximum of %d", ErrTooManyEntries, 2+len(refs), MaxArchiveEntries)
	}

	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)

	if err := writeZipEntry(zw, ManifestPath, manifestBytes); err != nil {
		return nil, err
	}
	if err := writeZipEntry(zw, ConfigPath, configBytes); err != nil {
		return nil, err
	}

	var totalUncompressed int64 = int64(len(manifestBytes)) + int64(len(configBytes))
	for _, ref := range refs {
		totalUncompressed += ref.byteSize
		if totalUncompressed > MaxTotalUncompressedBytes {
			return nil, fmt.Errorf("%w: total backup size exceeds %d bytes", ErrTooLarge, MaxTotalUncompressedBytes)
		}

		rc, err := ref.source.Open(ref.sha256)
		if err != nil {
			return nil, fmt.Errorf("%w: open blob %s: %v", ErrAssetMissing, ref.sha256, err)
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: ref.root + "/" + ref.sha256, Method: zip.Deflate})
		if err != nil {
			rc.Close()
			return nil, fmt.Errorf("%w: create archive entry for blob %s: %v", ErrInvalidArchive, ref.sha256, err)
		}
		_, copyErr := io.Copy(w, rc)
		rc.Close()
		if copyErr != nil {
			return nil, fmt.Errorf("%w: write blob %s: %v", ErrInvalidArchive, ref.sha256, copyErr)
		}
	}

	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("%w: finalize archive: %v", ErrInvalidArchive, err)
	}
	if int64(buf.Len()) > MaxPackageBytes {
		return nil, fmt.Errorf("%w: package is %d bytes, exceeding the %d byte limit", ErrTooLarge, buf.Len(), MaxPackageBytes)
	}
	return buf.Bytes(), nil
}

func writeZipEntry(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
	if err != nil {
		return fmt.Errorf("%w: create archive entry %q: %v", ErrInvalidArchive, name, err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("%w: write archive entry %q: %v", ErrInvalidArchive, name, err)
	}
	return nil
}
