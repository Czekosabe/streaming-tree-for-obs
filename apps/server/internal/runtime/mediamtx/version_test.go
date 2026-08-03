package mediamtx

import "testing"

func TestSupportedVersionIsPinned(t *testing.T) {
	// The generated configuration and the API fixtures target this exact
	// release; changing it is a deliberate commit, not an incidental edit.
	if SupportedVersion != "v1.19.3" {
		t.Fatalf("SupportedVersion = %q, want v1.19.3", SupportedVersion)
	}
}

func TestParseVersionOutputAcceptsTheRealFormat(t *testing.T) {
	// This is exactly what `mediamtx --version` prints.
	version, err := ParseVersionOutput("v1.19.3\n")
	if err != nil {
		t.Fatalf("ParseVersionOutput() returned an error: %v", err)
	}
	if version != "v1.19.3" {
		t.Errorf("version = %q, want v1.19.3", version)
	}
}

func TestParseVersionOutputTolerarespaceAndLeadingNoise(t *testing.T) {
	version, err := ParseVersionOutput("\n\n  v1.19.3  \n")
	if err != nil {
		t.Fatalf("ParseVersionOutput() returned an error: %v", err)
	}
	if version != "v1.19.3" {
		t.Errorf("version = %q, want v1.19.3", version)
	}
}

func TestParseVersionOutputRejectsMalformedOutput(t *testing.T) {
	cases := map[string]string{
		"empty":            "",
		"whitespace only":  "   \n\t",
		"missing v prefix": "1.19.3",
		"not a version":    "command not found",
		"partial":          "v1.19",
		"prose":            "MediaMTX version v1.19.3",
	}

	for name, output := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseVersionOutput(output); err == nil {
				t.Fatalf("ParseVersionOutput(%q) succeeded, want an error", output)
			}
		})
	}
}

func TestParseVersionOutputTruncatesHostileText(t *testing.T) {
	// A broken binary could print a great deal; the error message must stay
	// bounded rather than embedding the whole output.
	huge := make([]byte, 4096)
	for i := range huge {
		huge[i] = 'x'
	}

	_, err := ParseVersionOutput(string(huge))
	if err == nil {
		t.Fatal("ParseVersionOutput() accepted 4 KB of noise")
	}
	if len(err.Error()) > 200 {
		t.Errorf("error message is %d characters, want it truncated", len(err.Error()))
	}
}

func TestIsSupportedVersion(t *testing.T) {
	if !IsSupportedVersion("v1.19.3") {
		t.Error("the pinned version was rejected")
	}
	for _, other := range []string{"v1.19.2", "v1.20.0", "v2.0.0", "", "v1.19.3-rc1"} {
		if IsSupportedVersion(other) {
			t.Errorf("version %q was accepted, want only the pinned one", other)
		}
	}
}

func TestAssetMatrixCoversTheRequiredPlatforms(t *testing.T) {
	required := map[string]string{
		"windows/amd64": "mediamtx_v1.19.3_windows_amd64.zip",
		"linux/amd64":   "mediamtx_v1.19.3_linux_amd64.tar.gz",
		"linux/arm64":   "mediamtx_v1.19.3_linux_arm64.tar.gz",
		"darwin/amd64":  "mediamtx_v1.19.3_darwin_amd64.tar.gz",
		"darwin/arm64":  "mediamtx_v1.19.3_darwin_arm64.tar.gz",
	}

	for platform, wantName := range required {
		goos, goarch, ok := splitPlatform(platform)
		if !ok {
			t.Fatalf("bad test fixture %q", platform)
		}

		asset, err := AssetFor(goos, goarch)
		if err != nil {
			t.Fatalf("AssetFor(%s) returned an error: %v", platform, err)
		}
		// These names were verified against the published checksums.sha256.
		if asset.FileName != wantName {
			t.Errorf("%s asset = %q, want %q", platform, asset.FileName, wantName)
		}
	}
}

func TestAssetForRejectsUnsupportedPlatforms(t *testing.T) {
	for _, platform := range []string{"windows/arm64", "linux/386", "freebsd/amd64", "plan9/amd64"} {
		goos, goarch, _ := splitPlatform(platform)

		_, err := AssetFor(goos, goarch)
		if err == nil {
			t.Errorf("AssetFor(%s) succeeded, want an unsupported-platform error", platform)
		}
	}
}

func TestExecutableNameFor(t *testing.T) {
	if got := ExecutableNameFor("windows"); got != "mediamtx.exe" {
		t.Errorf("windows executable = %q, want mediamtx.exe", got)
	}
	for _, goos := range []string{"linux", "darwin"} {
		if got := ExecutableNameFor(goos); got != "mediamtx" {
			t.Errorf("%s executable = %q, want mediamtx", goos, got)
		}
	}
}

func splitPlatform(value string) (string, string, bool) {
	for i := 0; i < len(value); i++ {
		if value[i] == '/' {
			return value[:i], value[i+1:], true
		}
	}
	return "", "", false
}
