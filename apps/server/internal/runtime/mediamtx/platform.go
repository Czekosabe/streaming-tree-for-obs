package mediamtx

import (
	"fmt"
	"runtime"
)

// ArchiveFormat distinguishes the two packaging formats upstream publishes.
type ArchiveFormat string

const (
	FormatTarGz ArchiveFormat = "tar.gz"
	FormatZip   ArchiveFormat = "zip"
)

// ReleaseAsset describes the official release file for one platform.
type ReleaseAsset struct {
	// FileName is the exact asset name, which is also the key used to find the
	// entry inside checksums.sha256.
	FileName string
	Format   ArchiveFormat
	// ExecutableName is the file expected inside the archive.
	ExecutableName string
	// PlatformDir is the "<os>-<arch>" folder the installation lives in.
	PlatformDir string
}

// assetMatrix maps GOOS/GOARCH onto the official v1.19.3 release assets.
//
// Only combinations verified against the published checksums.sha256 are listed.
// Anything absent is an explicit "unsupported platform" error rather than a
// guessed file name, because guessing would produce a confusing 404 at download
// time instead of an actionable message.
var assetMatrix = map[string]ReleaseAsset{
	"windows/amd64": {
		FileName:       "mediamtx_" + SupportedVersion + "_windows_amd64.zip",
		Format:         FormatZip,
		ExecutableName: "mediamtx.exe",
		PlatformDir:    "windows-amd64",
	},
	"linux/amd64": {
		FileName:       "mediamtx_" + SupportedVersion + "_linux_amd64.tar.gz",
		Format:         FormatTarGz,
		ExecutableName: "mediamtx",
		PlatformDir:    "linux-amd64",
	},
	"linux/arm64": {
		FileName:       "mediamtx_" + SupportedVersion + "_linux_arm64.tar.gz",
		Format:         FormatTarGz,
		ExecutableName: "mediamtx",
		PlatformDir:    "linux-arm64",
	},
	"darwin/amd64": {
		FileName:       "mediamtx_" + SupportedVersion + "_darwin_amd64.tar.gz",
		Format:         FormatTarGz,
		ExecutableName: "mediamtx",
		PlatformDir:    "darwin-amd64",
	},
	"darwin/arm64": {
		FileName:       "mediamtx_" + SupportedVersion + "_darwin_arm64.tar.gz",
		Format:         FormatTarGz,
		ExecutableName: "mediamtx",
		PlatformDir:    "darwin-arm64",
	},
}

// AssetFor returns the release asset for one GOOS/GOARCH pair.
func AssetFor(goos, goarch string) (ReleaseAsset, error) {
	asset, ok := assetMatrix[goos+"/"+goarch]
	if !ok {
		return ReleaseAsset{}, fmt.Errorf(
			"%w: %s/%s has no official MediaMTX %s release asset",
			ErrUnsupportedPlatform, goos, goarch, SupportedVersion)
	}
	return asset, nil
}

// CurrentAsset returns the release asset for the running platform.
func CurrentAsset() (ReleaseAsset, error) {
	return AssetFor(runtime.GOOS, runtime.GOARCH)
}

// ExecutableNameFor returns the expected executable file name for a platform.
func ExecutableNameFor(goos string) string {
	if goos == "windows" {
		return "mediamtx.exe"
	}
	return "mediamtx"
}
