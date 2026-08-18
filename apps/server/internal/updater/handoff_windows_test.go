//go:build windows

package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestCheckInstalledMarkersAbsent(t *testing.T) {
	dir := t.TempDir()
	ok, code := checkInstalledMarkers(dir)
	if ok || code != BlockerNotInstalledCtx {
		t.Fatalf("checkInstalledMarkers() = (%v, %q), want (false, %q) with no marker files", ok, code, BlockerNotInstalledCtx)
	}
}

func TestCheckInstalledMarkersPresent(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "unins000.exe"), []byte("fake"))
	mustWriteFile(t, filepath.Join(dir, "unins000.dat"), []byte("fake"))

	ok, code := checkInstalledMarkers(dir)
	if !ok || code != "" {
		t.Fatalf("checkInstalledMarkers() = (%v, %q), want (true, \"\") with both marker files present", ok, code)
	}
}

func TestCheckInstalledMarkersPartial(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "unins000.exe"), []byte("fake"))
	// unins000.dat deliberately missing.

	ok, _ := checkInstalledMarkers(dir)
	if ok {
		t.Fatal("checkInstalledMarkers() = true with only one of the two marker files present, want false")
	}
}

func TestParseVersionOutputExtractsLastToken(t *testing.T) {
	got, err := parseVersionOutput("Streaming Tree for OBS 0.2.0\ncommit abc123\nlicence GPL-3.0-or-later\n")
	if err != nil {
		t.Fatalf("parseVersionOutput() error = %v", err)
	}
	if got != "0.2.0" {
		t.Fatalf("parseVersionOutput() = %q, want 0.2.0", got)
	}
}

func TestParseVersionOutputEmpty(t *testing.T) {
	if _, err := parseVersionOutput(""); err == nil {
		t.Fatal("parseVersionOutput() accepted empty output, want an error")
	}
}

func TestReverifyCandidateAccepted(t *testing.T) {
	dir := t.TempDir()
	content := []byte("fake installer bytes")
	sum := sha256.Sum256(content)
	sha := hex.EncodeToString(sum[:])
	path := filepath.Join(dir, "verified-0.2.0-"+sha[:12]+".exe")
	mustWriteFile(t, path, content)

	if err := reverifyCandidate(path); err != nil {
		t.Fatalf("reverifyCandidate() error = %v, want nil for a correctly-named, unmodified file", err)
	}
}

func TestReverifyCandidateDetectsTampering(t *testing.T) {
	dir := t.TempDir()
	original := []byte("fake installer bytes")
	sum := sha256.Sum256(original)
	sha := hex.EncodeToString(sum[:])
	path := filepath.Join(dir, "verified-0.2.0-"+sha[:12]+".exe")
	mustWriteFile(t, path, original)

	// Tamper with the file after it was "verified" and named.
	mustWriteFile(t, path, []byte("tampered bytes, different content entirely"))

	if err := reverifyCandidate(path); err == nil {
		t.Fatal("reverifyCandidate() accepted a tampered file, want rejection")
	}
}

func TestReverifyCandidateRejectsUnrecognizedName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "not-a-verified-candidate.exe")
	mustWriteFile(t, path, []byte("anything"))

	if err := reverifyCandidate(path); err == nil {
		t.Fatal("reverifyCandidate() accepted a non-conforming file name, want rejection")
	}
}

func TestValidateHelperArgsRequiresAllFields(t *testing.T) {
	if _, err := ValidateHelperArgs(0, "", "", ""); err == nil {
		t.Fatal("ValidateHelperArgs() accepted all-empty input, want an error")
	}
}

func TestValidateHelperArgsRejectsCandidateOutsideUpdatesDir(t *testing.T) {
	dir := t.TempDir()
	badCandidate := filepath.Join(dir, "not-updates-subdir", "verified-0.2.0-abcdef012345.exe")

	if _, err := ValidateHelperArgs(1234, badCandidate, filepath.Join(dir, "app.exe"), "0.2.0"); err == nil {
		t.Fatal("ValidateHelperArgs() accepted a candidate path outside the updates/ subdirectory, want rejection")
	}
}

func TestValidateHelperArgsAcceptsCandidateInsideUpdatesDir(t *testing.T) {
	dir := t.TempDir()
	goodCandidate := filepath.Join(dir, "updates", "verified-0.2.0-abcdef012345.exe")

	args, err := ValidateHelperArgs(1234, goodCandidate, filepath.Join(dir, "app.exe"), "0.2.0")
	if err != nil {
		t.Fatalf("ValidateHelperArgs() error = %v", err)
	}
	if args.ParentPID != 1234 || args.ExpectedVersion != "0.2.0" {
		t.Fatalf("ValidateHelperArgs() = %+v, unexpected field values", args)
	}
}

func mustWriteFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
