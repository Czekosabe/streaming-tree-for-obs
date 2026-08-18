// Package manifest defines the Streaming Tree release manifest: a small,
// project-controlled JSON document published as one asset
// ("streaming-tree-release.json") on every Stable GitHub Release,
// describing exactly which platform artifacts that release contains and
// how to verify them.
//
// See docs/updater.md §5 for the full contract. This package has no I/O
// and no dependency beyond the standard library, so the exact same
// Validate logic runs both in the release-build pipeline (cmd/
// releasemanifest) and in the runtime updater - the two can never
// disagree about what a valid manifest looks like.
package manifest

import "errors"

// ErrInvalid wraps every manifest validation failure.
var ErrInvalid = errors.New("invalid release manifest")

// Format is the fixed, required value of Manifest.Format.
const Format = "streaming-tree-release"

// SchemaVersion is the one schema version this build understands. A
// manifest declaring any other value is rejected outright rather than
// guessed at - see docs/updater.md §5.
const SchemaVersion = 1

// Channel is the release channel a manifest declares. Stage 20B
// implements Stable only (docs/updater.md §3).
type Channel string

// ChannelStable is the only channel Stage 20B accepts.
const ChannelStable Channel = "stable"

// OS is a target operating system, using the same vocabulary as
// docs/platform-support.md and Go's own runtime.GOOS.
type OS string

const (
	OSWindows OS = "windows"
	OSDarwin  OS = "darwin"
	OSLinux   OS = "linux"
)

func (o OS) known() bool {
	switch o {
	case OSWindows, OSDarwin, OSLinux:
		return true
	default:
		return false
	}
}

// Arch is a target CPU architecture, using the same vocabulary as Go's
// own runtime.GOARCH.
type Arch string

const (
	ArchAMD64 Arch = "amd64"
	ArchARM64 Arch = "arm64"
)

func (a Arch) known() bool {
	switch a {
	case ArchAMD64, ArchARM64:
		return true
	default:
		return false
	}
}

// Kind is the package format one artifact ships as. The full enum
// already names every package kind docs/platform-support.md records as
// a future candidate (docs/updater.md §7) - only KindInstaller is ever
// actually installable by Stage 20B.
type Kind string

const (
	KindInstaller Kind = "installer"
	KindDMG       Kind = "dmg"
	KindPKG       Kind = "pkg"
	KindAppImage  Kind = "appimage"
	KindDeb       Kind = "deb"
	KindRPM       Kind = "rpm"
)

func (k Kind) known() bool {
	switch k {
	case KindInstaller, KindDMG, KindPKG, KindAppImage, KindDeb, KindRPM:
		return true
	default:
		return false
	}
}

// Identity is the (OS, Arch, Kind) tuple that distinguishes one artifact
// from another within a single release - see docs/updater.md §7.
type Identity struct {
	OS   OS
	Arch Arch
	Kind Kind
}

// Artifact is one downloadable release asset described by the manifest.
//
// Deliberately no download URL field - see docs/updater.md §5. The
// downloadable location always comes from matching Name against the
// same GitHub Release's own assets array.
type Artifact struct {
	OS        OS     `json:"os"`
	Arch      Arch   `json:"arch"`
	Kind      Kind   `json:"kind"`
	Name      string `json:"name"`
	SizeBytes int64  `json:"sizeBytes"`
	SHA256    string `json:"sha256"`
}

// Identity returns this artifact's (OS, Arch, Kind) tuple.
func (a Artifact) Identity() Identity {
	return Identity{OS: a.OS, Arch: a.Arch, Kind: a.Kind}
}

// Manifest is the full parsed, unvalidated release manifest document.
// Callers must run Validate before trusting any field.
type Manifest struct {
	Format        string     `json:"format"`
	SchemaVersion int        `json:"schemaVersion"`
	Version       string     `json:"version"`
	Channel       Channel    `json:"channel"`
	Artifacts     []Artifact `json:"artifacts"`
}

// ArtifactFor returns the manifest's artifact matching id, if any.
func (m Manifest) ArtifactFor(id Identity) (Artifact, bool) {
	for _, a := range m.Artifacts {
		if a.Identity() == id {
			return a, true
		}
	}
	return Artifact{}, false
}
