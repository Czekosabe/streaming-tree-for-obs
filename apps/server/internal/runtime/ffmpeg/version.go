package ffmpeg

import (
	"regexp"
	"strconv"
)

// MinimumVersion is the oldest release branch this application is written
// against.
//
// Chosen from FFmpeg's own list of currently maintained release branches
// (release/4.4 through release/9.0, per https://ffmpeg.org/download.html,
// checked while implementing this stage - see docs/progress.md) rather than
// from a remembered number: 4.4 is the oldest branch that page still lists as
// maintained.
//
// This is a floor, not the real gate. A binary at or above this version can
// still fail Capabilities probing; a binary below it is rejected without
// probing further, since a release the FFmpeg project itself no longer lists
// receives no attention from them at all, maintained or not.
const MinimumVersion = "4.4"

// versionPattern extracts a leading dotted numeric version from FFmpeg's
// often decorated version string, e.g. "8.1-full_build-www.gyan.dev" -> "8.1",
// "6.1.6" -> "6.1.6". A git/dev build such as "N-112233-gabcdef1234" has no
// leading digit and simply does not match - see ParseVersion.
var versionPattern = regexp.MustCompile(`^ffmpeg version (\S+)`)

var numericPrefixPattern = regexp.MustCompile(`^(\d+)(?:\.(\d+))?(?:\.(\d+))?`)

// ParseVersion extracts the version token from the first line of
// `ffmpeg -version` output, whatever it looks like - a git/dev build such as
// "N-112233-gabcdef1234" is a valid token here and is still worth displaying,
// even though it has no numeric meaning; see AtLeastMinimum for the separate
// question of whether a token can be compared to MinimumVersion at all. This
// returns ("", false) only when the output has no recognisable
// "ffmpeg version <token>" line at all - malformed output, not a build with
// an unusual version scheme.
func ParseVersion(versionOutput string) (string, bool) {
	match := versionPattern.FindStringSubmatch(versionOutput)
	if match == nil {
		return "", false
	}
	return match[1], true
}

// numericComponents pulls up to three leading dot-separated integers out of a
// version token, defaulting missing ones to 0. Returns ok=false when the
// token does not start with a digit at all (a git/dev build).
func numericComponents(version string) (major, minor, patch int, ok bool) {
	match := numericPrefixPattern.FindStringSubmatch(version)
	if match == nil || match[1] == "" {
		return 0, 0, 0, false
	}
	major, _ = strconv.Atoi(match[1])
	if match[2] != "" {
		minor, _ = strconv.Atoi(match[2])
	}
	if match[3] != "" {
		patch, _ = strconv.Atoi(match[3])
	}
	return major, minor, patch, true
}

// AtLeastMinimum reports whether version is >= MinimumVersion.
//
// Returns true when version cannot be parsed as numeric at all (a git/dev
// build reporting something like "N-112233-gabcdef1234"): there is nothing to
// compare, and rejecting an unrecognised-but-possibly-current build on that
// basis alone would contradict the "do not reject a newer, passing version
// just because of its string" rule. Capability probing still applies.
func AtLeastMinimum(version string) bool {
	major, minor, patch, ok := numericComponents(version)
	if !ok {
		return true
	}
	minMajor, minMinor, minPatch, _ := numericComponents(MinimumVersion)

	if major != minMajor {
		return major > minMajor
	}
	if minor != minMinor {
		return minor > minMinor
	}
	return patch >= minPatch
}
