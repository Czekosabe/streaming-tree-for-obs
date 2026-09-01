package backup

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// Stage 23F: fills the remaining gaps in archive_test.go's own
// malicious-package coverage against the governing task's full list
// (docs/backup-restore.md §12) - decompression bomb, unknown format
// version, truncated archive, an asset whose content does not match
// its declared hash (distinct from TestReadArchiveRejectsConfigHash
// Mismatch, which only exercises config.json's own hash), malformed
// (unparseable, not just schema-mismatched) JSON, duplicate archive
// entries, and a config.json carrying a secret-shaped property name
// DisallowUnknownFields must reject regardless of what it is named.

func TestReadArchiveRejectsUnknownFormatVersion(t *testing.T) {
	cfg := Config{FormatVersion: FormatVersion}
	data, err := WriteArchive(cfg, "0.1.0-test", "windows", time.Now(), memBlobSource{}, memBlobSource{})
	if err != nil {
		t.Fatalf("WriteArchive() error = %v", err)
	}

	tampered := tamperManifestField(t, data, `"formatVersion":1`, `"formatVersion":99`)
	if _, err := ReadArchive(tampered); !errors.Is(err, ErrVersionUnsupported) {
		t.Fatalf("ReadArchive() error = %v, want ErrVersionUnsupported", err)
	}
}

func TestReadArchiveRejectsTruncatedArchive(t *testing.T) {
	cfg, visualBlobs := fixtureConfigWithOneAsset()
	data, err := WriteArchive(cfg, "0.1.0-test", "windows", time.Now(), visualBlobs, memBlobSource{})
	if err != nil {
		t.Fatalf("WriteArchive() error = %v", err)
	}

	truncated := data[:len(data)/2]
	if _, err := ReadArchive(truncated); !errors.Is(err, ErrInvalidArchive) {
		t.Fatalf("ReadArchive() error = %v, want ErrInvalidArchive", err)
	}
}

// Distinct from TestReadArchiveRejectsConfigHashMismatch: that test
// tampers config.json itself. This tampers an ASSET's own content
// while leaving its content-addressed archive path (and the
// manifest's declared hash for it) unchanged - a different code path
// (reader.go's per-asset hash check, not the config-wide one).
func TestReadArchiveRejectsAssetContentHashMismatch(t *testing.T) {
	cfg, visualBlobs := fixtureConfigWithOneAsset()
	data, err := WriteArchive(cfg, "0.1.0-test", "windows", time.Now(), visualBlobs, memBlobSource{})
	if err != nil {
		t.Fatalf("WriteArchive() error = %v", err)
	}

	// Same length as the original "fake-png-bytes" (14 bytes) so this
	// exercises the hash check specifically, not the separate declared-
	// size check the reader also performs.
	assetPath := "assets/" + cfg.VisualAssets[0].Blob.SHA256
	tampered := tamperEntry(t, data, assetPath, []byte("XXXXXXXXXXXXXX"))
	if _, err := ReadArchive(tampered); !errors.Is(err, ErrAssetHashMismatch) {
		t.Fatalf("ReadArchive() error = %v, want ErrAssetHashMismatch", err)
	}
}

func TestReadArchiveRejectsMalformedConfigJSON(t *testing.T) {
	cfg := Config{FormatVersion: FormatVersion}
	data, err := WriteArchive(cfg, "0.1.0-test", "windows", time.Now(), memBlobSource{}, memBlobSource{})
	if err != nil {
		t.Fatalf("WriteArchive() error = %v", err)
	}

	malformed := []byte(`{"formatVersion": this is not valid JSON`)
	tampered := tamperConfigAndItsDeclaredHash(t, data, malformed)
	if _, err := ReadArchive(tampered); !errors.Is(err, ErrConfigInvalid) {
		t.Fatalf("ReadArchive() error = %v, want ErrConfigInvalid", err)
	}
}

// The structural reflection scan in security_test.go proves the real
// Config type has no secret-shaped field. This proves the OTHER half
// of that guarantee: a crafted config.json that adds one anyway is
// rejected outright by DisallowUnknownFields, regardless of what the
// extra property is named - it never silently passes through as
// ignored/extra data.
func TestReadArchiveRejectsConfigWithASecretShapedUnknownField(t *testing.T) {
	cfg := Config{FormatVersion: FormatVersion}
	data, err := WriteArchive(cfg, "0.1.0-test", "windows", time.Now(), memBlobSource{}, memBlobSource{})
	if err != nil {
		t.Fatalf("WriteArchive() error = %v", err)
	}

	malicious := []byte(`{"formatVersion":1,"streamKey":"sk_live_should_never_be_accepted"}`)
	tampered := tamperConfigAndItsDeclaredHash(t, data, malicious)
	if _, err := ReadArchive(tampered); !errors.Is(err, ErrConfigInvalid) {
		t.Fatalf("ReadArchive() error = %v, want ErrConfigInvalid", err)
	}
}

// A real decompression bomb: a large, maximally-compressible payload
// (all zeros) stored with genuine DEFLATE compression, so its
// compressed:uncompressed ratio blows past MaxDecompressionRatio. The
// shared addEntry test helper deliberately stores its new entry
// uncompressed (zip.FileHeader's zero Method is Store), so this test
// builds its own deflated entry instead - the ratio check has nothing
// to catch otherwise.
func TestReadArchiveRejectsDecompressionBomb(t *testing.T) {
	cfg := Config{FormatVersion: FormatVersion}
	data, err := WriteArchive(cfg, "0.1.0-test", "windows", time.Now(), memBlobSource{}, memBlobSource{})
	if err != nil {
		t.Fatalf("WriteArchive() error = %v", err)
	}

	zeros := make([]byte, 10<<20) // 10 MiB of zeros - deflates far past a 100:1 ratio.
	bombName := "assets/" + strings.Repeat("b", 64)
	malicious := addDeflatedEntry(t, data, bombName, zeros)
	if _, err := ReadArchive(malicious); !errors.Is(err, ErrDecompressionLimit) {
		t.Fatalf("ReadArchive() error = %v, want ErrDecompressionLimit", err)
	}
}

func TestReadArchiveRejectsDuplicateArchiveEntries(t *testing.T) {
	cfg := Config{FormatVersion: FormatVersion}
	data, err := WriteArchive(cfg, "0.1.0-test", "windows", time.Now(), memBlobSource{}, memBlobSource{})
	if err != nil {
		t.Fatalf("WriteArchive() error = %v", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	for _, f := range zr.File {
		rc, _ := f.Open()
		content, _ := io.ReadAll(rc)
		rc.Close()
		w, _ := zw.Create(f.Name)
		w.Write(content)
	}
	// manifest.json a second time - archive-level ambiguity about
	// which entry is authoritative, rejected before either is even
	// parsed.
	w, err := zw.Create(ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("a-second-manifest-entry"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := ReadArchive(buf.Bytes()); !errors.Is(err, ErrEntryInvalid) {
		t.Fatalf("ReadArchive() error = %v, want ErrEntryInvalid", err)
	}
}

// --- additional tamper helpers ---------------------------------------

// tamperManifestField does a literal string replacement inside
// manifest.json - the same trick tamperManifestProduct already uses,
// generalised to any "key":value pair so this file does not need one
// helper per field.
func tamperManifestField(t *testing.T, data []byte, oldField, newField string) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var manifestBytes []byte
	for _, f := range zr.File {
		if f.Name == ManifestPath {
			rc, _ := f.Open()
			manifestBytes, _ = io.ReadAll(rc)
			rc.Close()
		}
	}
	if !bytes.Contains(manifestBytes, []byte(oldField)) {
		t.Fatalf("manifest.json does not contain %q to tamper", oldField)
	}
	tampered := strings.Replace(string(manifestBytes), oldField, newField, 1)
	return tamperEntry(t, data, ManifestPath, []byte(tampered))
}

// tamperConfigAndItsDeclaredHash replaces config.json's content AND
// the manifest's own declared configSha256 together, so a test can
// reach config.json's actual JSON-decode step instead of being
// stopped earlier by the config-hash-agreement check.
func tamperConfigAndItsDeclaredHash(t *testing.T, data []byte, newConfig []byte) []byte {
	t.Helper()
	sum := sha256.Sum256(newConfig)
	tampered := tamperEntry(t, data, ConfigPath, newConfig)

	zr, err := zip.NewReader(bytes.NewReader(tampered), int64(len(tampered)))
	if err != nil {
		t.Fatal(err)
	}
	var manifestBytes []byte
	for _, f := range zr.File {
		if f.Name == ManifestPath {
			rc, _ := f.Open()
			manifestBytes, _ = io.ReadAll(rc)
			rc.Close()
		}
	}
	oldHashField := `"configSha256":"`
	idx := bytes.Index(manifestBytes, []byte(oldHashField))
	if idx < 0 {
		t.Fatal("manifest.json does not contain a configSha256 field to tamper")
	}
	start := idx + len(oldHashField)
	end := bytes.IndexByte(manifestBytes[start:], '"')
	if end < 0 {
		t.Fatal("malformed configSha256 field in manifest.json fixture")
	}
	newManifest := append(append(append([]byte{}, manifestBytes[:start]...), []byte(hex.EncodeToString(sum[:]))...), manifestBytes[start+end:]...)
	return tamperEntry(t, tampered, ManifestPath, newManifest)
}

// addDeflatedEntry mirrors the shared addEntry helper but forces real
// DEFLATE compression on the new entry - addEntry's zero-value
// zip.FileHeader stores it uncompressed, which a decompression-ratio
// test has nothing to trip on.
func addDeflatedEntry(t *testing.T, data []byte, name string, content []byte) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	for _, f := range zr.File {
		rc, _ := f.Open()
		fileContent, _ := io.ReadAll(rc)
		rc.Close()
		w, _ := zw.Create(f.Name)
		w.Write(fileContent)
	}
	w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
