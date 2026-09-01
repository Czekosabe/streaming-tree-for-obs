// Package archivesafety holds the archive-entry safety primitives every
// zip-based package format in this codebase needs, extracted from
// internal/domain/visualpackage's own proven implementation
// (docs/visual-template-packages.md §7/§10) rather than re-implemented
// per feature. Bound VALUES (max sizes, max entries, max ratio) stay
// local to each caller - a `.streaming-tree-template` package and a
// `.streaming-tree-backup` package operate at genuinely different
// scales - but the safety LOGIC itself (path-traversal rejection,
// bounded-ASCII filename grammar, ratio-bomb detection, streamed
// bounded reads) is one implementation, reused everywhere a zip
// archive from an untrusted source is read in this application.
package archivesafety

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ErrEntryInvalid is returned for any archive entry that fails path,
// filename, or file-mode validation.
var ErrEntryInvalid = errors.New("archive contains an invalid entry")

// ErrTooLarge is returned when a bounded read would exceed its caller-
// supplied limit.
var ErrTooLarge = errors.New("archive entry exceeds its size limit")

// ErrDecompressionLimit is returned when an entry's compressed-to-
// uncompressed ratio exceeds the caller-supplied bound - the
// decompression-bomb defense.
var ErrDecompressionLimit = errors.New("archive entry exceeds its decompression bound")

var reservedWindowsNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// ValidateNoTraversal rejects everything a zip-slip or path-confusion
// attack needs: an absolute path, a Windows drive/UNC path, any
// backslash, any control character, and any "."/".."/empty path
// segment. It says nothing about what the path IS allowed to be -
// callers apply their own allowed-root/filename grammar on top (see
// ValidateBoundedASCIISegment) - only what it must never be.
func ValidateNoTraversal(name string) error {
	if name == "" {
		return fmt.Errorf("%w: empty archive entry name", ErrEntryInvalid)
	}
	if strings.ContainsRune(name, '\\') {
		return fmt.Errorf("%w: entry %q contains a backslash", ErrEntryInvalid, name)
	}
	if strings.HasPrefix(name, "/") {
		return fmt.Errorf("%w: entry %q is an absolute path", ErrEntryInvalid, name)
	}
	if len(name) >= 2 && name[1] == ':' {
		return fmt.Errorf("%w: entry %q looks like a Windows drive path", ErrEntryInvalid, name)
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7F {
			return fmt.Errorf("%w: entry %q contains a control character", ErrEntryInvalid, name)
		}
	}
	for _, seg := range strings.Split(name, "/") {
		if seg == "" {
			return fmt.Errorf("%w: entry %q has an empty path segment", ErrEntryInvalid, name)
		}
		if seg == "." || seg == ".." {
			return fmt.Errorf("%w: entry %q contains a %q segment", ErrEntryInvalid, name, seg)
		}
	}
	return nil
}

// boundedASCIISegmentPattern-equivalent check, spelled out without
// regexp so this package has no extra dependency: letters, digits,
// dot, underscore, hyphen only, 1-128 characters.
func ValidateBoundedASCIISegment(seg string) error {
	if len(seg) == 0 || len(seg) > 128 {
		return fmt.Errorf("%w: filename %q must be 1-128 characters", ErrEntryInvalid, seg)
	}
	for _, r := range seg {
		ok := (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') ||
			r == '.' || r == '_' || r == '-'
		if !ok {
			return fmt.Errorf("%w: filename %q is not a valid bounded ASCII filename", ErrEntryInvalid, seg)
		}
	}
	if strings.HasSuffix(seg, ".") || strings.HasSuffix(seg, " ") {
		return fmt.Errorf("%w: filename %q has an ambiguous trailing dot/space", ErrEntryInvalid, seg)
	}
	base := seg
	if i := strings.IndexByte(seg, '.'); i >= 0 {
		base = seg[:i]
	}
	if reservedWindowsNames[strings.ToUpper(base)] {
		return fmt.Errorf("%w: filename %q uses a reserved Windows device name", ErrEntryInvalid, seg)
	}
	return nil
}

// NormalizePath returns name's case-insensitive comparison key, used
// to detect a duplicate entry that differs only by case.
func NormalizePath(name string) string {
	return strings.ToLower(name)
}

// CheckDecompressionRatio rejects an entry whose declared uncompressed
// size is disproportionate to its compressed size - the standard
// decompression-bomb defense, checked from the zip central directory's
// own declared sizes, before a single byte of the entry is read.
func CheckDecompressionRatio(uncompressed, compressed uint64, maxRatio float64) error {
	if compressed == 0 {
		return nil
	}
	ratio := float64(uncompressed) / float64(compressed)
	if ratio > maxRatio {
		return fmt.Errorf("%w: decompression ratio %.1f exceeds the limit of %.1f", ErrDecompressionLimit, ratio, maxRatio)
	}
	return nil
}

// ReadEntryBounded opens and fully reads one zip entry, enforcing max
// as a hard streaming bound - the entry's own declared
// UncompressedSize64 is never trusted alone: both the declared size
// AND the actually-streamed byte count are checked.
func ReadEntryBounded(f *zip.File, max int64) ([]byte, error) {
	if int64(f.UncompressedSize64) > max {
		return nil, fmt.Errorf("%w: entry %q declares %d bytes, exceeding the %d byte limit", ErrTooLarge, f.Name, f.UncompressedSize64, max)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("%w: open entry %q: %v", ErrEntryInvalid, f.Name, err)
	}
	defer rc.Close()

	limited := io.LimitReader(rc, max+1)
	buf, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("%w: read entry %q: %v", ErrEntryInvalid, f.Name, err)
	}
	if int64(len(buf)) > max {
		return nil, fmt.Errorf("%w: entry %q streamed more than its declared/allowed %d bytes", ErrDecompressionLimit, f.Name, max)
	}
	return buf, nil
}
