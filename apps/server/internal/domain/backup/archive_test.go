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

	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/domain/visualasset"
)

// memBlobSource is an in-memory AssetBlobSource fixture, keyed by
// sha256 hex - a fake for internal/domain/visualasset.FileStore/
// internal/domain/audioasset's own identical FileStore instance.
type memBlobSource map[string][]byte

func (m memBlobSource) Open(sha256Hex string) (io.ReadCloser, error) {
	data, ok := m[sha256Hex]
	if !ok {
		return nil, errors.New("blob not found")
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func fixtureConfigWithOneAsset() (Config, memBlobSource) {
	imageBytes := []byte("fake-png-bytes")
	sum := sha256Hex(imageBytes)

	cfg := Config{
		FormatVersion: FormatVersion,
		Platforms: []PlatformExport{
			{Platform: platform.Platform{ID: "pf_1", DisplayName: "Main Twitch"}},
		},
		VisualAssets: []visualasset.Asset{
			{
				ID: "asset_1", DisplayName: "logo.png",
				Blob: &visualasset.Blob{SHA256: sum, ByteSize: int64(len(imageBytes)), PublicToken: "tok_abc"},
			},
		},
	}
	store := memBlobSource{sum: imageBytes}
	return cfg, store
}

func TestWriteThenReadArchiveRoundTripsConfigAndAssets(t *testing.T) {
	cfg, visualBlobs := fixtureConfigWithOneAsset()

	data, err := WriteArchive(cfg, "0.1.0-test", "windows", time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), visualBlobs, memBlobSource{})
	if err != nil {
		t.Fatalf("WriteArchive() error = %v", err)
	}

	got, err := ReadArchive(data)
	if err != nil {
		t.Fatalf("ReadArchive() error = %v", err)
	}
	if got.Manifest.Product != Product {
		t.Errorf("Product = %q, want %q", got.Manifest.Product, Product)
	}
	if len(got.Config.Platforms) != 1 || got.Config.Platforms[0].Platform.ID != "pf_1" {
		t.Errorf("config did not round-trip: %+v", got.Config)
	}
	if len(got.Assets) != 1 {
		t.Fatalf("got %d assets, want 1", len(got.Assets))
	}
	if string(got.Assets[0].Data) != "fake-png-bytes" {
		t.Errorf("asset data = %q, want the original bytes", got.Assets[0].Data)
	}
}

func TestWriteArchiveDeduplicatesSharedBlobs(t *testing.T) {
	imageBytes := []byte("shared-bytes")
	sum := sha256Hex(imageBytes)
	cfg := Config{
		FormatVersion: FormatVersion,
		VisualAssets: []visualasset.Asset{
			{ID: "asset_1", Blob: &visualasset.Blob{SHA256: sum, ByteSize: int64(len(imageBytes))}},
			{ID: "asset_2", Blob: &visualasset.Blob{SHA256: sum, ByteSize: int64(len(imageBytes))}},
		},
	}
	store := memBlobSource{sum: imageBytes}

	data, err := WriteArchive(cfg, "0.1.0-test", "windows", time.Now(), store, memBlobSource{})
	if err != nil {
		t.Fatalf("WriteArchive() error = %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	assetEntries := 0
	for _, f := range zr.File {
		if strings.HasPrefix(f.Name, "assets/") {
			assetEntries++
		}
	}
	if assetEntries != 1 {
		t.Errorf("got %d assets/ archive entries, want exactly 1 (content-deduplicated)", assetEntries)
	}
}

func TestReadArchiveRejectsWrongProduct(t *testing.T) {
	cfg := Config{FormatVersion: FormatVersion}
	data, err := WriteArchive(cfg, "0.1.0-test", "windows", time.Now(), memBlobSource{}, memBlobSource{})
	if err != nil {
		t.Fatalf("WriteArchive() error = %v", err)
	}

	tampered := tamperManifestProduct(t, data, "some-other-app-backup")
	if _, err := ReadArchive(tampered); !errors.Is(err, ErrProductMismatch) {
		t.Fatalf("ReadArchive() error = %v, want ErrProductMismatch", err)
	}
}

func TestReadArchiveRejectsConfigHashMismatch(t *testing.T) {
	cfg := Config{FormatVersion: FormatVersion}
	data, err := WriteArchive(cfg, "0.1.0-test", "windows", time.Now(), memBlobSource{}, memBlobSource{})
	if err != nil {
		t.Fatalf("WriteArchive() error = %v", err)
	}

	tampered := tamperEntry(t, data, ConfigPath, []byte(`{"formatVersion":1,"platforms":[{"tampered":true}]}`))
	if _, err := ReadArchive(tampered); !errors.Is(err, ErrAssetHashMismatch) {
		t.Fatalf("ReadArchive() error = %v, want ErrAssetHashMismatch", err)
	}
}

func TestReadArchiveRejectsZipSlipAssetPath(t *testing.T) {
	cfg := Config{FormatVersion: FormatVersion}
	data, err := WriteArchive(cfg, "0.1.0-test", "windows", time.Now(), memBlobSource{}, memBlobSource{})
	if err != nil {
		t.Fatalf("WriteArchive() error = %v", err)
	}
	malicious := addEntry(t, data, "../../evil.txt", []byte("pwned"))
	if _, err := ReadArchive(malicious); !errors.Is(err, ErrEntryInvalid) {
		t.Fatalf("ReadArchive() error = %v, want ErrEntryInvalid", err)
	}
}

func TestReadArchiveRejectsUnreferencedEntry(t *testing.T) {
	cfg := Config{FormatVersion: FormatVersion}
	data, err := WriteArchive(cfg, "0.1.0-test", "windows", time.Now(), memBlobSource{}, memBlobSource{})
	if err != nil {
		t.Fatalf("WriteArchive() error = %v", err)
	}
	// A syntactically valid content-addressed path, but never declared
	// in the manifest - a hidden payload the manifest does not know
	// about.
	hidden := strings.Repeat("a", 64)
	malicious := addEntry(t, data, "assets/"+hidden, []byte("hidden"))
	if _, err := ReadArchive(malicious); !errors.Is(err, ErrAssetUnreferenced) {
		t.Fatalf("ReadArchive() error = %v, want ErrAssetUnreferenced", err)
	}
}

func TestReadArchiveRejectsMissingManifestOrConfig(t *testing.T) {
	// A path-grammar-valid entry (a real-shaped content-addressed asset
	// path) but no manifest.json/config.json at all.
	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	w, _ := zw.Create("assets/" + strings.Repeat("a", 64))
	w.Write([]byte("x"))
	zw.Close()

	if _, err := ReadArchive(buf.Bytes()); !errors.Is(err, ErrManifestInvalid) {
		t.Fatalf("ReadArchive() error = %v, want ErrManifestInvalid", err)
	}
}

func TestReadArchiveRejectsOversizedPackage(t *testing.T) {
	huge := make([]byte, MaxPackageBytes+1)
	if _, err := ReadArchive(huge); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("ReadArchive() error = %v, want ErrTooLarge", err)
	}
}

func TestReadArchiveRejectsInvalidAssetSegmentName(t *testing.T) {
	cfg := Config{FormatVersion: FormatVersion}
	data, err := WriteArchive(cfg, "0.1.0-test", "windows", time.Now(), memBlobSource{}, memBlobSource{})
	if err != nil {
		t.Fatalf("WriteArchive() error = %v", err)
	}
	malicious := addEntry(t, data, "assets/not-a-sha256.png", []byte("x"))
	if _, err := ReadArchive(malicious); !errors.Is(err, ErrEntryInvalid) {
		t.Fatalf("ReadArchive() error = %v, want ErrEntryInvalid", err)
	}
}

// --- test helpers: tamper with an already-built archive -------------

func tamperEntry(t *testing.T, data []byte, name string, newContent []byte) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		if f.Name == name {
			content = newContent
		}
		w, err := zw.Create(f.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func tamperManifestProduct(t *testing.T, data []byte, newProduct string) []byte {
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
	tampered := strings.Replace(string(manifestBytes), Product, newProduct, 1)
	return tamperEntry(t, data, ManifestPath, []byte(tampered))
}

func addEntry(t *testing.T, data []byte, name string, content []byte) []byte {
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
	w, err := zw.CreateHeader(&zip.FileHeader{Name: name})
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
