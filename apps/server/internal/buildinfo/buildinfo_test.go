package buildinfo

import "testing"

// TestIsStrictProductionVersion locks the exact grammar this project's
// updater eligibility gate relies on - deliberately identical to
// scripts/build-release.ps1's own `-Version` regex (`^\d+\.\d+\.\d+$`),
// since the two must never drift (see IsStrictProductionVersion's own
// doc comment).
func TestIsStrictProductionVersion(t *testing.T) {
	t.Cleanup(func() { releaseVersion = "" })

	cases := []struct {
		version string
		want    bool
	}{
		{"", false}, // no release build at all
		{"0.1.0", true},
		{"1.2.3", true},
		{"10.20.30", true},
		{"0.1.0-manualtest+a0e2fb8", false},
		{"0.1.0-dev+local", false},
		{"0.1.0+build.1", false},
		{"v0.1.0", false}, // a leading "v" is a tag format, never a version
		{"0.1", false},
		{"0.1.0.0", false},
		{"latest", false},
		{"1.0-beta", false},
	}

	for _, tc := range cases {
		releaseVersion = tc.version
		if got := IsStrictProductionVersion(); got != tc.want {
			t.Errorf("IsStrictProductionVersion() for releaseVersion=%q = %v, want %v", tc.version, got, tc.want)
		}
	}
}

// TestIsStrictProductionVersionNarrowerThanIsReleaseBuild locks the
// deliberate gap between the two: a manual/test build is still a real
// release build (IsReleaseBuild() stays true, since About/CommitInfo
// need that to stay honest) but is not eligible for production update
// checking (IsStrictProductionVersion() is false).
func TestIsStrictProductionVersionNarrowerThanIsReleaseBuild(t *testing.T) {
	t.Cleanup(func() { releaseVersion = "" })

	releaseVersion = "0.1.0-manualtest+a0e2fb8"
	if !IsReleaseBuild() {
		t.Fatal("IsReleaseBuild() = false for a manual/test build, want true")
	}
	if IsStrictProductionVersion() {
		t.Fatal("IsStrictProductionVersion() = true for a manual/test build, want false")
	}
}
