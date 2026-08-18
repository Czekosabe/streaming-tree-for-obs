package manifest

import (
	"fmt"
	"strconv"
	"strings"
)

// Version is a parsed major.minor.patch version - see docs/updater.md
// §4. Comparison is always exact integer comparison of the three
// components, never a string/lexicographic comparison.
type Version struct {
	Major, Minor, Patch int
}

// ParseVersion parses a strict "major.minor.patch" string: three
// non-negative integers separated by '.', no leading zeros beyond a
// bare "0", no pre-release or build-metadata suffix, no leading "v".
// Reject everything else outright (docs/updater.md §4's explicit
// rejection list: "latest", "master", "main", "0.2", "0.2.0.0", "v0.2",
// "1.0-beta", "1.0.0-rc1", empty string).
func ParseVersion(raw string) (Version, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("%w: version %q must have exactly three dot-separated components", ErrInvalid, raw)
	}

	values := make([]int, 3)
	for i, part := range parts {
		if part == "" {
			return Version{}, fmt.Errorf("%w: version %q has an empty component", ErrInvalid, raw)
		}
		if part != "0" && strings.HasPrefix(part, "0") {
			return Version{}, fmt.Errorf("%w: version %q has a leading zero", ErrInvalid, raw)
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return Version{}, fmt.Errorf("%w: version %q is not numeric", ErrInvalid, raw)
			}
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			return Version{}, fmt.Errorf("%w: version %q is not numeric", ErrInvalid, raw)
		}
		values[i] = n
	}

	return Version{Major: values[0], Minor: values[1], Patch: values[2]}, nil
}

// ParseTag parses a canonical release tag ("v0.2.0"), stripping exactly
// one leading "v" before delegating to ParseVersion. A tag with no
// leading "v" is also accepted, since docs/updater.md §4 only requires
// the *canonical* tag format to carry one - comparison itself is always
// against the parsed numeric value, never the original string.
func ParseTag(tag string) (Version, error) {
	return ParseVersion(strings.TrimPrefix(tag, "v"))
}

// String renders "major.minor.patch", with no "v" prefix - the same
// unprefixed form the manifest's own "version" field and
// buildinfo.EffectiveVersion() use.
func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// Compare returns -1, 0 or 1 as v is less than, equal to or greater than
// other - exact integer comparison of (Major, Minor, Patch) in that
// order.
func (v Version) Compare(other Version) int {
	if v.Major != other.Major {
		return compareInt(v.Major, other.Major)
	}
	if v.Minor != other.Minor {
		return compareInt(v.Minor, other.Minor)
	}
	return compareInt(v.Patch, other.Patch)
}

func compareInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
