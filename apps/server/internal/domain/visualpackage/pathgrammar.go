package visualpackage

import (
	"fmt"
	"regexp"
	"strings"
)

// packageAssetIDPattern matches a package-local logical asset id
// (docs/visual-template-packages.md §6/§11: "pkgasset_<short-random-or-
// deterministic-id>", bounded, ASCII, path-independent) - a completely
// different, disjoint-by-construction namespace from
// visualasset.AssetIDPrefix ("asset_..."), so a package id can never be
// mistaken for a local database id even if a caller forgets to remap it.
var packageAssetIDPattern = regexp.MustCompile(`^pkgasset_[A-Za-z0-9]{1,64}$`)

func validatePackageAssetID(id string) error {
	if !packageAssetIDPattern.MatchString(id) {
		return fmt.Errorf("%w: asset id %q is not a valid pkgasset_ identifier", ErrManifestInvalid, id)
	}
	return nil
}

// packageAudioAssetIDPattern is packageAssetIDPattern's own audio-asset
// counterpart (docs/alert-audio.md §10.2) - "pkgaudio_<short-random-or-
// deterministic-id>", disjoint by construction from pkgasset_, so a
// manifest-parsing bug can never accidentally cross-resolve a visual
// reference against an audio entry or vice versa.
var packageAudioAssetIDPattern = regexp.MustCompile(`^pkgaudio_[A-Za-z0-9]{1,64}$`)

func validatePackageAudioAssetID(id string) error {
	if !packageAudioAssetIDPattern.MatchString(id) {
		return fmt.Errorf("%w: audio asset id %q is not a valid pkgaudio_ identifier", ErrManifestInvalid, id)
	}
	return nil
}

// assetSegmentPattern is the bounded ASCII filename grammar an
// assets/<segment> entry's own segment must match (docs/visual-template-
// packages.md §7) - letters, digits, dot, underscore, hyphen only,
// 1-128 characters. Any backslash, any control character, and any
// non-ASCII byte are already impossible here since the pattern is a
// strict allowlist, not a denylist.
var assetSegmentPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

var reservedWindowsNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// validateEntryPath checks any raw archive entry name against the full
// path grammar (docs/visual-template-packages.md §7) before it is ever
// treated as manifest.json/template.json/an assets/ entry. Rejects
// everything the contract names: absolute paths, drive letters, UNC
// paths, any backslash, "."/".."/empty segments, control characters,
// and (for an assets/ entry) anything outside the bounded ASCII segment
// grammar, a trailing dot/space, or a Windows reserved device name.
func validateEntryPath(name string) error {
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
	segments := strings.Split(name, "/")
	for _, seg := range segments {
		if seg == "" {
			return fmt.Errorf("%w: entry %q has an empty path segment", ErrEntryInvalid, name)
		}
		if seg == "." || seg == ".." {
			return fmt.Errorf("%w: entry %q contains a %q segment", ErrEntryInvalid, name, seg)
		}
	}

	switch name {
	case "manifest.json", TemplatePath:
		return nil
	}

	if len(segments) != 2 {
		return fmt.Errorf("%w: entry %q is outside the allowed root structure", ErrEntryInvalid, name)
	}
	switch segments[0] {
	case "assets":
		return validateAssetPath(name)
	case "audio":
		return validateAudioAssetPath(name)
	default:
		return fmt.Errorf("%w: entry %q is outside the allowed root structure", ErrEntryInvalid, name)
	}
}

// validateSegmentPath is the shared bounded-ASCII-filename grammar check
// both validateAssetPath and validateAudioAssetPath apply to their own
// respective root ("assets" or "audio") - one filename grammar, reused
// under either legal root pattern (docs/alert-audio.md §10.2: "the
// identical validateAssetPath machinery already used for assets/
// <segment>, no new path-grammar code, one more accepted prefix").
func validateSegmentPath(name, root string) error {
	segments := strings.Split(name, "/")
	if len(segments) != 2 || segments[0] != root {
		return fmt.Errorf("%w: path %q must be of the form %s/<file>", ErrEntryInvalid, name, root)
	}
	seg := segments[1]
	if !assetSegmentPattern.MatchString(seg) {
		return fmt.Errorf("%w: filename %q is not a valid bounded ASCII filename", ErrEntryInvalid, seg)
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

// validateAssetPath additionally checks the manifest-declared path of
// one visual asset entry - reused both from validateEntryPath (real
// archive entries) and from manifest validation (a manifest asset's own
// "path" field, before any matching archive entry has even been
// located).
func validateAssetPath(name string) error {
	return validateSegmentPath(name, "assets")
}

// validateAudioAssetPath is validateAssetPath's own audio-asset
// counterpart (docs/alert-audio.md §10.2), for the "audio/<segment>"
// root pattern.
func validateAudioAssetPath(name string) error {
	return validateSegmentPath(name, "audio")
}

// normalizePath returns name's case-insensitive comparison key, used to
// detect a duplicate entry/asset path that differs only by case (docs/
// visual-template-packages.md §7/§12: "a case-insensitive duplicate
// path").
func normalizePath(name string) string {
	return strings.ToLower(name)
}
