package updater

import (
	"fmt"
	"strings"
)

// githubDigestPrefix is the only digest algorithm prefix this project
// cross-checks - GitHub's own current documentation (docs/updater.md
// §2/§14) only describes "sha256:<hex>".
const githubDigestPrefix = "sha256:"

// CrossCheckDigest implements docs/updater.md §14: when asset.Digest is
// present and uses the documented "sha256:" prefix, it must agree with
// manifestSHA256 (already-validated lowercase hex, from
// manifest.Artifact.SHA256) or the release is refused outright. An
// absent digest, or one using a prefix this project does not recognize,
// is not an error - the manifest's own SHA-256 remains sufficient on
// its own, exactly as documented.
func CrossCheckDigest(asset Asset, manifestSHA256 string) error {
	if asset.Digest == "" {
		return nil
	}
	if !strings.HasPrefix(asset.Digest, githubDigestPrefix) {
		return nil
	}

	reported := strings.TrimPrefix(asset.Digest, githubDigestPrefix)
	if !strings.EqualFold(reported, manifestSHA256) {
		return fmt.Errorf("%w: github reports %s, manifest declares %s", ErrDigestMismatch, reported, manifestSHA256)
	}
	return nil
}
