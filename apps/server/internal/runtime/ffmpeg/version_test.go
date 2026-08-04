package ffmpeg

import "testing"

func TestParseVersionExtractsTheLeadingToken(t *testing.T) {
	cases := map[string]string{
		"ffmpeg version 8.1-full_build-www.gyan.dev Copyright (c) 2000-2026\n": "8.1-full_build-www.gyan.dev",
		"ffmpeg version 6.1.6 Copyright\n":                                     "6.1.6",
		"ffmpeg version n7.0\nlibavutil ...":                                   "n7.0",
	}

	for input, want := range cases {
		got, ok := ParseVersion(input)
		if !ok {
			t.Errorf("ParseVersion(%q) reported not ok", input)
			continue
		}
		if got != want {
			t.Errorf("ParseVersion(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestParseVersionFailsForAGitBuild(t *testing.T) {
	_, ok := ParseVersion("this is not a version line\n")
	if ok {
		t.Error("unparseable output was reported parseable")
	}
}

func TestAtLeastMinimumAcceptsTheFloorExactly(t *testing.T) {
	if !AtLeastMinimum(MinimumVersion) {
		t.Errorf("MinimumVersion %q was not accepted as at least itself", MinimumVersion)
	}
}

func TestAtLeastMinimumRejectsBelowTheFloor(t *testing.T) {
	if AtLeastMinimum("3.4") {
		t.Error("3.4 was accepted as at least the minimum")
	}
	if AtLeastMinimum("4.3.9") {
		t.Error("4.3.9 was accepted as at least the minimum")
	}
}

func TestAtLeastMinimumAcceptsNewer(t *testing.T) {
	for _, v := range []string{"5.0", "6.1.6", "8.1", "9.0", "100.0"} {
		if !AtLeastMinimum(v) {
			t.Errorf("AtLeastMinimum(%q) = false, want true", v)
		}
	}
}

func TestAtLeastMinimumAcceptsAnUnparseableVersion(t *testing.T) {
	// A git/dev build reports something with no leading digit; there is
	// nothing to compare, so it must not be rejected on this basis alone -
	// capability probing is what actually gates it.
	if !AtLeastMinimum("N-112233-gabcdef1234") {
		t.Error("an unparseable version was rejected by the version floor")
	}
}

func TestNumericComponentsHandlesPartialVersions(t *testing.T) {
	major, minor, patch, ok := numericComponents("8")
	if !ok || major != 8 || minor != 0 || patch != 0 {
		t.Errorf("numericComponents(8) = (%d,%d,%d,%v)", major, minor, patch, ok)
	}
}
