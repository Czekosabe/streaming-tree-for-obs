package archivesafety

import (
	"archive/zip"
	"bytes"
	"errors"
	"testing"
)

func TestValidateNoTraversalRejectsKnownAttacks(t *testing.T) {
	cases := []string{
		"../etc/passwd",
		"/etc/passwd",
		"a/../../b",
		"C:\\Windows\\system32",
		"a\\b",
		"a/./b",
		"a//b",
		"",
		"a\x00b",
	}
	for _, name := range cases {
		if err := ValidateNoTraversal(name); err == nil {
			t.Errorf("ValidateNoTraversal(%q) = nil, want an error", name)
		} else if !errors.Is(err, ErrEntryInvalid) {
			t.Errorf("ValidateNoTraversal(%q) error = %v, want ErrEntryInvalid", name, err)
		}
	}
}

func TestValidateNoTraversalAcceptsOrdinaryPaths(t *testing.T) {
	for _, name := range []string{"manifest.json", "assets/file.png", "a/b/c.txt"} {
		if err := ValidateNoTraversal(name); err != nil {
			t.Errorf("ValidateNoTraversal(%q) = %v, want nil", name, err)
		}
	}
}

func TestValidateBoundedASCIISegmentRejectsReservedAndOversized(t *testing.T) {
	if err := ValidateBoundedASCIISegment("CON"); err == nil {
		t.Error("expected rejection of reserved Windows device name")
	}
	if err := ValidateBoundedASCIISegment("CON.png"); err == nil {
		t.Error("expected rejection of reserved Windows device name with extension")
	}
	if err := ValidateBoundedASCIISegment("trailing."); err == nil {
		t.Error("expected rejection of trailing dot")
	}
	long := make([]byte, 129)
	for i := range long {
		long[i] = 'a'
	}
	if err := ValidateBoundedASCIISegment(string(long)); err == nil {
		t.Error("expected rejection of an oversized filename")
	}
	if err := ValidateBoundedASCIISegment("héllo.png"); err == nil {
		t.Error("expected rejection of a non-ASCII filename")
	}
}

func TestValidateBoundedASCIISegmentAcceptsOrdinaryNames(t *testing.T) {
	for _, name := range []string{"a.png", "a_b-c.WAV", "config.json"} {
		if err := ValidateBoundedASCIISegment(name); err != nil {
			t.Errorf("ValidateBoundedASCIISegment(%q) = %v, want nil", name, err)
		}
	}
}

func TestCheckDecompressionRatioRejectsBomb(t *testing.T) {
	if err := CheckDecompressionRatio(1_000_000, 100, 100.0); !errors.Is(err, ErrDecompressionLimit) {
		t.Fatalf("error = %v, want ErrDecompressionLimit", err)
	}
	if err := CheckDecompressionRatio(1000, 100, 100.0); err != nil {
		t.Fatalf("error = %v, want nil for a ratio within bounds", err)
	}
	if err := CheckDecompressionRatio(1000, 0, 100.0); err != nil {
		t.Fatalf("error = %v, want nil for zero compressed size (stored entry)", err)
	}
}

func TestReadEntryBoundedRejectsOversizedDeclaredSize(t *testing.T) {
	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	w, err := zw.Create("big.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(make([]byte, 100)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReadEntryBounded(zr.File[0], 50); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("error = %v, want ErrTooLarge", err)
	}
	data, err := ReadEntryBounded(zr.File[0], 200)
	if err != nil {
		t.Fatalf("ReadEntryBounded() error = %v, want nil", err)
	}
	if len(data) != 100 {
		t.Fatalf("len(data) = %d, want 100", len(data))
	}
}

func TestNormalizePathIsCaseInsensitive(t *testing.T) {
	if NormalizePath("Assets/Foo.PNG") != NormalizePath("assets/foo.png") {
		t.Fatal("NormalizePath is not case-insensitive")
	}
}
