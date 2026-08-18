// Command releasemanifest generates the Stage 20B release manifest
// (docs/updater.md §5/§39) from a real, already-built release artifact.
//
// It computes the artifact's exact size and SHA-256 directly from the
// file on disk - nothing here is hand-typed or duplicated from another
// source - and validates its own output with the exact same
// internal/updater/manifest.Validate the runtime updater uses, so the
// release pipeline and the runtime can never disagree about what a
// valid manifest looks like. Invoked only by scripts/build-release.ps1;
// never run against arbitrary paths.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/streaming-tree/server/internal/updater/manifest"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "releasemanifest: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	version := flag.String("version", "", "release version, e.g. 0.2.0 (required)")
	tag := flag.String("tag", "", "release tag, e.g. v0.2.0 (defaults to \"v\"+version)")
	artifactPath := flag.String("artifact", "", "path to the real, already-built artifact (required)")
	artifactName := flag.String("artifact-name", "", "the artifact's own release-asset name (defaults to its file name)")
	osName := flag.String("os", "windows", "artifact OS")
	arch := flag.String("arch", "amd64", "artifact architecture")
	kind := flag.String("kind", "installer", "artifact package kind")
	out := flag.String("out", "", "output path for the generated manifest JSON (required)")
	flag.Parse()

	if *version == "" {
		return fmt.Errorf("-version is required")
	}
	if *artifactPath == "" {
		return fmt.Errorf("-artifact is required")
	}
	if *out == "" {
		return fmt.Errorf("-out is required")
	}
	effectiveTag := *tag
	if effectiveTag == "" {
		effectiveTag = "v" + *version
	}
	name := *artifactName
	if name == "" {
		name = filepath.Base(*artifactPath)
	}

	size, sha, err := hashFile(*artifactPath)
	if err != nil {
		return fmt.Errorf("hash artifact: %w", err)
	}

	m := manifest.Manifest{
		Format:        manifest.Format,
		SchemaVersion: manifest.SchemaVersion,
		Version:       *version,
		Channel:       manifest.ChannelStable,
		Artifacts: []manifest.Artifact{
			{
				OS:        manifest.OS(*osName),
				Arch:      manifest.Arch(*arch),
				Kind:      manifest.Kind(*kind),
				Name:      name,
				SizeBytes: size,
				SHA256:    sha,
			},
		},
	}

	// Self-validate before ever writing the file: the pipeline must fail
	// loudly here rather than produce a manifest the runtime updater
	// would itself reject (docs/updater.md §39).
	if err := manifest.Validate(m, effectiveTag); err != nil {
		return fmt.Errorf("generated manifest failed self-validation: %w", err)
	}

	raw := manifest.MustMarshal(m)
	if err := os.WriteFile(*out, raw, 0o644); err != nil { //nolint:gosec // release-build output, not a secret.
		return fmt.Errorf("write manifest: %w", err)
	}

	fmt.Printf("wrote %s (version=%s os=%s arch=%s kind=%s size=%d sha256=%s)\n",
		*out, *version, *osName, *arch, *kind, size, sha)
	return nil
}

func hashFile(path string) (int64, string, error) {
	f, err := os.Open(path) // #nosec G304 -- path is an explicit, operator-supplied build-script argument.
	if err != nil {
		return 0, "", err
	}
	defer f.Close()

	hasher := sha256.New()
	size, err := io.Copy(hasher, f)
	if err != nil {
		return 0, "", err
	}
	return size, hex.EncodeToString(hasher.Sum(nil)), nil
}
