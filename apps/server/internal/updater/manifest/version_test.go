package manifest

import "testing"

func TestParseVersionAccepts(t *testing.T) {
	got, err := ParseVersion("0.2.0")
	if err != nil {
		t.Fatalf("ParseVersion() error = %v", err)
	}
	want := Version{Major: 0, Minor: 2, Patch: 0}
	if got != want {
		t.Fatalf("ParseVersion() = %+v, want %+v", got, want)
	}
}

func TestParseVersionRejects(t *testing.T) {
	cases := []string{
		"latest", "master", "main", "0.2", "0.2.0.0", "v0.2",
		"1.0-beta", "1.0.0-rc1", "", "1.2.03", "1.2.-1", "a.b.c",
		"1..0", "1.2.3.", ".1.2.3",
	}
	for _, raw := range cases {
		if _, err := ParseVersion(raw); err == nil {
			t.Errorf("ParseVersion(%q) accepted, want rejection", raw)
		}
	}
}

func TestParseTagStripsLeadingV(t *testing.T) {
	got, err := ParseTag("v0.2.0")
	if err != nil {
		t.Fatalf("ParseTag() error = %v", err)
	}
	if got.String() != "0.2.0" {
		t.Fatalf("ParseTag() = %s, want 0.2.0", got)
	}

	// A tag with no leading "v" is also accepted - comparison is always
	// against the parsed value, never the original string.
	got2, err := ParseTag("0.2.0")
	if err != nil {
		t.Fatalf("ParseTag() error = %v", err)
	}
	if got2 != got {
		t.Fatalf("ParseTag(\"0.2.0\") = %+v, want %+v", got2, got)
	}
}

func TestVersionCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.1.0", "0.1.0", 0},
		{"0.1.0", "0.2.0", -1},
		{"0.2.0", "0.1.0", 1},
		{"0.9.0", "0.10.0", -1}, // exact integer comparison, never lexicographic
		{"0.10.0", "0.9.0", 1},
		{"1.0.0", "0.99.99", 1},
		{"0.1.9", "0.1.10", -1},
	}
	for _, c := range cases {
		va, err := ParseVersion(c.a)
		if err != nil {
			t.Fatalf("ParseVersion(%q) error = %v", c.a, err)
		}
		vb, err := ParseVersion(c.b)
		if err != nil {
			t.Fatalf("ParseVersion(%q) error = %v", c.b, err)
		}
		if got := va.Compare(vb); got != c.want {
			t.Errorf("%s.Compare(%s) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestVersionString(t *testing.T) {
	v := Version{Major: 1, Minor: 2, Patch: 3}
	if v.String() != "1.2.3" {
		t.Fatalf("String() = %s, want 1.2.3", v.String())
	}
}
