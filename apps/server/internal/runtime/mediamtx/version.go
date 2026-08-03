// Package mediamtx manages the local MediaMTX ingest service: locating or
// installing the binary, generating its configuration, supervising the child
// process and reading ingest state from its Control API.
//
// Everything this package tracks is RUNTIME state and lives only in memory. It
// is never written to SQLite, because it describes what is happening right now
// rather than what the user configured.
package mediamtx

import (
	"fmt"
	"regexp"
	"strings"
)

// SupportedVersion is the single MediaMTX release this application is built and
// tested against.
//
// It is pinned deliberately: the generated configuration and the Control API
// client target this exact schema, and MediaMTX rejects unknown configuration
// keys outright, so an unpinned "latest" would break the application whenever
// upstream renamed a field. Raising it is a deliberate commit that also updates
// the configuration golden file and the API fixtures.
const SupportedVersion = "v1.19.3"

// versionPattern matches the output of `mediamtx --version`, which is a bare
// semantic version prefixed with "v", e.g. "v1.19.3".
var versionPattern = regexp.MustCompile(`^v\d+\.\d+\.\d+(?:-[0-9A-Za-z.\-]+)?$`)

// ParseVersionOutput extracts the version from `mediamtx --version` output.
//
// The binary prints just the version and a newline. Anything else - an error
// message, a shell wrapper's banner, an empty string - is rejected rather than
// guessed at, because accepting it would mean starting an unknown executable.
func ParseVersionOutput(output string) (string, error) {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return "", fmt.Errorf("%w: the executable printed no version", ErrVersionUnreadable)
	}

	// Take the first non-empty line: some environments prepend a warning.
	lines := strings.Split(trimmed, "\n")
	first := ""
	for _, line := range lines {
		if candidate := strings.TrimSpace(line); candidate != "" {
			first = candidate
			break
		}
	}

	if !versionPattern.MatchString(first) {
		return "", fmt.Errorf(
			"%w: %q does not look like a MediaMTX version", ErrVersionUnreadable, truncate(first, 64))
	}

	return first, nil
}

// IsSupportedVersion reports whether a version string is the pinned release.
func IsSupportedVersion(version string) bool {
	return version == SupportedVersion
}

// truncate bounds untrusted text before it reaches an error message or a log.
func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max] + "..."
}
