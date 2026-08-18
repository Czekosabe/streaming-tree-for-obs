package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestHashFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.bin")
	content := []byte("some release artifact content")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	size, sha, err := hashFile(path)
	if err != nil {
		t.Fatalf("hashFile() error = %v", err)
	}
	if size != int64(len(content)) {
		t.Errorf("size = %d, want %d", size, len(content))
	}

	want := sha256.Sum256(content)
	if sha != hex.EncodeToString(want[:]) {
		t.Errorf("sha256 = %s, want %s", sha, hex.EncodeToString(want[:]))
	}
}

func TestHashFileMissing(t *testing.T) {
	if _, _, err := hashFile(filepath.Join(t.TempDir(), "does-not-exist.bin")); err == nil {
		t.Fatal("hashFile() accepted a missing file, want an error")
	}
}
