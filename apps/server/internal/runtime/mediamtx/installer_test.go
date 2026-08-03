package mediamtx

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- archive fixtures -------------------------------------------------------

type archiveEntry struct {
	name     string
	body     string
	mode     int64
	typeflag byte
	linkname string
}

func regularEntry(name, body string) archiveEntry {
	return archiveEntry{name: name, body: body, mode: 0o755, typeflag: tar.TypeReg}
}

func buildTarGz(t *testing.T, entries []archiveEntry) []byte {
	t.Helper()

	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)

	for _, entry := range entries {
		header := &tar.Header{
			Name:     entry.name,
			Mode:     entry.mode,
			Size:     int64(len(entry.body)),
			Typeflag: entry.typeflag,
			Linkname: entry.linkname,
		}
		if entry.typeflag == tar.TypeSymlink || entry.typeflag == tar.TypeLink {
			header.Size = 0
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("write tar header: %v", err)
		}
		if header.Size > 0 {
			if _, err := tarWriter.Write([]byte(entry.body)); err != nil {
				t.Fatalf("write tar body: %v", err)
			}
		}
	}

	if err := tarWriter.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buffer.Bytes()
}

func buildZip(t *testing.T, entries []archiveEntry) []byte {
	t.Helper()

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)

	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name, Method: zip.Deflate}
		header.SetMode(os.FileMode(entry.mode))
		file, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatalf("create zip entry: %v", err)
		}
		if _, err := file.Write([]byte(entry.body)); err != nil {
			t.Fatalf("write zip body: %v", err)
		}
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buffer.Bytes()
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// --- release server ---------------------------------------------------------

type releaseServer struct {
	*httptest.Server
	assets map[string][]byte
	// checksums is served verbatim so tests can corrupt or omit entries.
	checksums string
	requests  map[string]int
}

func newReleaseServer(t *testing.T) *releaseServer {
	t.Helper()

	release := &releaseServer{
		assets:   map[string][]byte{},
		requests: map[string]int{},
	}

	release.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		release.requests[name]++

		if name == ChecksumFileName {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(release.checksums))
			return
		}

		body, ok := release.assets[name]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))

	t.Cleanup(release.Close)
	return release
}

// publish registers an asset and adds it to the checksum manifest.
func (s *releaseServer) publish(name string, body []byte) {
	s.assets[name] = body
	s.checksums += fmt.Sprintf("%s *%s\n", sha256Hex(body), name)
}

// linuxAsset is the platform used by most installer tests, so the fixtures do
// not depend on which machine the suite runs on.
func linuxAsset(t *testing.T) ReleaseAsset {
	t.Helper()
	asset, err := AssetFor("linux", "amd64")
	if err != nil {
		t.Fatalf("AssetFor(linux/amd64) returned an error: %v", err)
	}
	return asset
}

func newTestInstaller(t *testing.T, dataDir string, release *releaseServer, extra ...InstallerOption) *Installer {
	t.Helper()

	opts := []InstallerOption{
		WithReleaseBaseURL(release.URL),
		WithPlatform("linux", "amd64"),
		// The fixture archives contain no real executable, so the version probe
		// is injected. Nothing unverified is ever executed here.
		WithVersionProbe(staticProbe(SupportedVersion+"\n", nil)),
	}
	return NewInstaller(dataDir, append(opts, extra...)...)
}

func goodArchive(t *testing.T) []byte {
	return buildTarGz(t, []archiveEntry{
		regularEntry("mediamtx", "#!/bin/sh\n"),
		regularEntry("LICENSE", "MIT License\n"),
		regularEntry("mediamtx.yml", "logLevel: info\n"),
	})
}

// --- successful install -----------------------------------------------------

func TestInstallSucceedsAndIsAtomic(t *testing.T) {
	dataDir := t.TempDir()
	release := newReleaseServer(t)
	asset := linuxAsset(t)
	archive := goodArchive(t)
	release.publish(asset.FileName, archive)

	metadata, err := newTestInstaller(t, dataDir, release).Install(context.Background())
	if err != nil {
		t.Fatalf("Install() returned an error: %v", err)
	}

	if metadata.Version != SupportedVersion {
		t.Errorf("metadata version = %q, want %q", metadata.Version, SupportedVersion)
	}
	if metadata.SHA256 != sha256Hex(archive) {
		t.Errorf("metadata records the wrong checksum")
	}

	installDir := InstallDir(dataDir, asset.PlatformDir)
	for _, name := range []string{asset.ExecutableName, LicenseFileName, MetadataFileName} {
		if _, err := os.Stat(filepath.Join(installDir, name)); err != nil {
			t.Errorf("%s is missing from the installation: %v", name, err)
		}
	}

	// The staging area must not be left behind.
	staging := filepath.Join(RuntimeDir(dataDir), "tmp")
	entries, err := os.ReadDir(staging)
	if err == nil && len(entries) != 0 {
		t.Errorf("staging directory still holds %d entries", len(entries))
	}
}

func TestInstallSelectsTheAssetForTheTargetPlatform(t *testing.T) {
	cases := map[string]struct{ goos, goarch, wantName string }{
		"windows": {"windows", "amd64", "mediamtx_v1.19.3_windows_amd64.zip"},
		"linux":   {"linux", "arm64", "mediamtx_v1.19.3_linux_arm64.tar.gz"},
		"macos":   {"darwin", "arm64", "mediamtx_v1.19.3_darwin_arm64.tar.gz"},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			dataDir := t.TempDir()
			release := newReleaseServer(t)

			asset, err := AssetFor(testCase.goos, testCase.goarch)
			if err != nil {
				t.Fatalf("AssetFor() returned an error: %v", err)
			}
			if asset.FileName != testCase.wantName {
				t.Fatalf("asset = %q, want %q", asset.FileName, testCase.wantName)
			}

			var archive []byte
			if asset.Format == FormatZip {
				archive = buildZip(t, []archiveEntry{
					{name: "mediamtx.exe", body: "MZ", mode: 0o755},
					{name: "LICENSE", body: "MIT", mode: 0o644},
				})
			} else {
				archive = goodArchive(t)
			}
			release.publish(asset.FileName, archive)

			installer := NewInstaller(dataDir,
				WithReleaseBaseURL(release.URL),
				WithPlatform(testCase.goos, testCase.goarch),
				WithVersionProbe(staticProbe(SupportedVersion+"\n", nil)),
			)

			if _, err := installer.Install(context.Background()); err != nil {
				t.Fatalf("Install() returned an error: %v", err)
			}
			if release.requests[asset.FileName] != 1 {
				t.Errorf("asset %q was requested %d times, want 1",
					asset.FileName, release.requests[asset.FileName])
			}
		})
	}
}

func TestInstallRejectsAnUnsupportedPlatform(t *testing.T) {
	release := newReleaseServer(t)
	installer := NewInstaller(t.TempDir(),
		WithReleaseBaseURL(release.URL),
		WithPlatform("plan9", "mips"),
	)

	_, err := installer.Install(context.Background())
	if !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("Install() error = %v, want ErrUnsupportedPlatform", err)
	}
	if len(release.requests) != 0 {
		t.Error("an unsupported platform still triggered a download")
	}
}

// --- checksum verification --------------------------------------------------

func TestInstallRejectsAChecksumMismatch(t *testing.T) {
	dataDir := t.TempDir()
	release := newReleaseServer(t)
	asset := linuxAsset(t)

	// Advertise one checksum but serve different bytes.
	release.assets[asset.FileName] = goodArchive(t)
	release.checksums = fmt.Sprintf("%s *%s\n", strings.Repeat("a", 64), asset.FileName)

	_, err := newTestInstaller(t, dataDir, release).Install(context.Background())
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("Install() error = %v, want ErrChecksumMismatch", err)
	}

	if _, statErr := os.Stat(InstallDir(dataDir, asset.PlatformDir)); statErr == nil {
		t.Error("a mismatched download was installed anyway")
	}
}

func TestInstallRejectsAMissingChecksumEntry(t *testing.T) {
	dataDir := t.TempDir()
	release := newReleaseServer(t)
	asset := linuxAsset(t)

	release.assets[asset.FileName] = goodArchive(t)
	// The manifest lists a different file only.
	release.checksums = fmt.Sprintf("%s *some_other_asset.tar.gz\n", strings.Repeat("b", 64))

	_, err := newTestInstaller(t, dataDir, release).Install(context.Background())
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("Install() error = %v, want ErrChecksumMismatch", err)
	}
	// The archive must not even be fetched without an entry vouching for it.
	if release.requests[asset.FileName] != 0 {
		t.Error("the archive was downloaded despite having no checksum entry")
	}
}

func TestFindChecksumParsesTheOfficialFormat(t *testing.T) {
	// This is the real shape of the published manifest.
	manifest := "" +
		"21989f4b046e4b4619d5c88835445c52bd766c7bc26199886e480d6304863211 *mediamtx_v1.19.3_darwin_amd64.tar.gz\n" +
		"5d82148d1032a6a190d9909a2997d9989457aaadf49af87dd02cd4512d31bebe *mediamtx_v1.19.3_windows_amd64.zip\n"

	sum, err := findChecksum(manifest, "mediamtx_v1.19.3_windows_amd64.zip")
	if err != nil {
		t.Fatalf("findChecksum() returned an error: %v", err)
	}
	if sum != "5d82148d1032a6a190d9909a2997d9989457aaadf49af87dd02cd4512d31bebe" {
		t.Errorf("sum = %q, want the windows entry", sum)
	}
}

func TestFindChecksumRejectsMalformedEntries(t *testing.T) {
	cases := map[string]string{
		"too short":       "abc *asset.tar.gz\n",
		"not hexadecimal": strings.Repeat("z", 64) + " *asset.tar.gz\n",
		"no entry":        "",
	}

	for name, manifest := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := findChecksum(manifest, "asset.tar.gz"); err == nil {
				t.Fatal("findChecksum() accepted a malformed manifest")
			}
		})
	}
}

func TestInstallRejectsAnOversizedChecksumResponse(t *testing.T) {
	dataDir := t.TempDir()
	release := newReleaseServer(t)
	release.checksums = strings.Repeat("x", maxChecksumBytes+1024)

	_, err := newTestInstaller(t, dataDir, release).Install(context.Background())
	if err == nil {
		t.Fatal("Install() accepted an oversized checksum response")
	}
}

// --- archive security -------------------------------------------------------

func TestInstallRejectsUnsafeArchives(t *testing.T) {
	cases := map[string][]archiveEntry{
		"path traversal": {
			{name: "../escaped", body: "x", mode: 0o755, typeflag: tar.TypeReg},
			regularEntry("LICENSE", "MIT"),
		},
		"nested traversal": {
			{name: "nested/../../escaped", body: "x", mode: 0o755, typeflag: tar.TypeReg},
			regularEntry("LICENSE", "MIT"),
		},
		"absolute path": {
			{name: "/etc/passwd", body: "x", mode: 0o755, typeflag: tar.TypeReg},
			regularEntry("LICENSE", "MIT"),
		},
		"symlink": {
			regularEntry("mediamtx", "#!/bin/sh"),
			{name: "evil", mode: 0o777, typeflag: tar.TypeSymlink, linkname: "/etc/passwd"},
			regularEntry("LICENSE", "MIT"),
		},
		"hard link": {
			regularEntry("mediamtx", "#!/bin/sh"),
			{name: "evil", mode: 0o777, typeflag: tar.TypeLink, linkname: "/etc/passwd"},
			regularEntry("LICENSE", "MIT"),
		},
		"backslash traversal": {
			{name: `..\escaped`, body: "x", mode: 0o755, typeflag: tar.TypeReg},
			regularEntry("LICENSE", "MIT"),
		},
	}

	for name, entries := range cases {
		t.Run(name, func(t *testing.T) {
			dataDir := t.TempDir()
			release := newReleaseServer(t)
			asset := linuxAsset(t)
			release.publish(asset.FileName, buildTarGz(t, entries))

			_, err := newTestInstaller(t, dataDir, release).Install(context.Background())
			if !errors.Is(err, ErrArchiveInvalid) {
				t.Fatalf("Install() error = %v, want ErrArchiveInvalid", err)
			}

			// Nothing may be written outside the staging area.
			if _, statErr := os.Stat(filepath.Join(dataDir, "escaped")); statErr == nil {
				t.Error("an entry escaped the extraction directory")
			}
			if _, statErr := os.Stat(InstallDir(dataDir, asset.PlatformDir)); statErr == nil {
				t.Error("an unsafe archive was installed")
			}
		})
	}
}

func TestInstallRejectsAnArchiveWithoutTheExecutable(t *testing.T) {
	dataDir := t.TempDir()
	release := newReleaseServer(t)
	asset := linuxAsset(t)
	release.publish(asset.FileName, buildTarGz(t, []archiveEntry{
		regularEntry("LICENSE", "MIT"),
		regularEntry("readme.txt", "hello"),
	}))

	_, err := newTestInstaller(t, dataDir, release).Install(context.Background())
	if !errors.Is(err, ErrArchiveInvalid) {
		t.Fatalf("Install() error = %v, want ErrArchiveInvalid", err)
	}
}

func TestInstallRejectsAnArchiveWithoutTheLicense(t *testing.T) {
	dataDir := t.TempDir()
	release := newReleaseServer(t)
	asset := linuxAsset(t)
	release.publish(asset.FileName, buildTarGz(t, []archiveEntry{
		regularEntry("mediamtx", "#!/bin/sh"),
	}))

	_, err := newTestInstaller(t, dataDir, release).Install(context.Background())
	if !errors.Is(err, ErrArchiveInvalid) {
		t.Fatalf("Install() error = %v, want ErrArchiveInvalid - the license must be preserved", err)
	}
}

// --- version verification ---------------------------------------------------

func TestInstallRejectsAnArchiveReportingTheWrongVersion(t *testing.T) {
	dataDir := t.TempDir()
	release := newReleaseServer(t)
	asset := linuxAsset(t)
	release.publish(asset.FileName, goodArchive(t))

	installer := newTestInstaller(t, dataDir, release,
		WithVersionProbe(staticProbe("v1.18.0\n", nil)))

	_, err := installer.Install(context.Background())
	if !errors.Is(err, ErrIncompatibleVersion) {
		t.Fatalf("Install() error = %v, want ErrIncompatibleVersion", err)
	}
	if _, statErr := os.Stat(InstallDir(dataDir, asset.PlatformDir)); statErr == nil {
		t.Error("a wrong-version archive was installed")
	}
}

// --- existing installation safety -------------------------------------------

func TestFailedReinstallLeavesTheExistingInstallationIntact(t *testing.T) {
	dataDir := t.TempDir()
	release := newReleaseServer(t)
	asset := linuxAsset(t)
	release.publish(asset.FileName, goodArchive(t))

	// First install succeeds.
	if _, err := newTestInstaller(t, dataDir, release).Install(context.Background()); err != nil {
		t.Fatalf("first Install() returned an error: %v", err)
	}

	installDir := InstallDir(dataDir, asset.PlatformDir)
	marker := filepath.Join(installDir, asset.ExecutableName)
	original, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read the installed executable: %v", err)
	}

	// Second install fails at the checksum stage.
	release.checksums = fmt.Sprintf("%s *%s\n", strings.Repeat("c", 64), asset.FileName)
	if _, err := newTestInstaller(t, dataDir, release).Install(context.Background()); err == nil {
		t.Fatal("the second Install() succeeded, want a checksum failure")
	}

	after, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("the working installation disappeared after a failed reinstall: %v", err)
	}
	if !bytes.Equal(original, after) {
		t.Error("a failed reinstall modified the working installation")
	}
}

func TestClassifyInstallErrorMapsStableCodes(t *testing.T) {
	cases := map[string]struct {
		err  error
		want string
	}{
		"unsupported platform": {ErrUnsupportedPlatform, CodeUnsupportedPlatform},
		"checksum":             {ErrChecksumMismatch, CodeChecksumMismatch},
		"archive":              {ErrArchiveInvalid, CodeArchiveInvalid},
		"version":              {ErrIncompatibleVersion, CodeIncompatibleVersion},
		"in progress":          {ErrInstallInProgress, CodeInstallInProgress},
		"network":              {errors.New("connection refused"), CodeDownloadFailed},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			runtimeErr := ClassifyInstallError(fmt.Errorf("wrapped: %w", testCase.err))
			if runtimeErr.Code != testCase.want {
				t.Errorf("code = %q, want %q", runtimeErr.Code, testCase.want)
			}
			if runtimeErr.Message == "" {
				t.Error("the English fallback message is empty")
			}
		})
	}
}
