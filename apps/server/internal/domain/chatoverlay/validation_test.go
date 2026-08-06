package chatoverlay

import (
	"math"
	"testing"
)

func TestDefaultProfileValidates(t *testing.T) {
	p := Default("My Overlay")
	p.ID = "ov_1"
	p.PublicSlug = "slug1"
	if err := ValidateProfile(p); err != nil {
		t.Fatalf("Default() profile failed validation: %v", err)
	}
}

func TestValidateProfileRejectsEmptyName(t *testing.T) {
	p := Default("")
	p.ID = "ov_1"
	p.PublicSlug = "slug1"
	if err := ValidateProfile(p); err == nil {
		t.Fatal("expected an error for an empty name")
	}
}

func TestValidateProfileRejectsUnsupportedEnums(t *testing.T) {
	p := Default("x")
	p.LayoutMode = "diagonal"
	if err := ValidateProfile(p); err == nil {
		t.Fatal("expected an error for an unsupported layout mode")
	}
}

func TestValidateProfileRejectsOutOfRangeMaxVisibleItems(t *testing.T) {
	for _, value := range []int{0, -1, 101, 1000} {
		p := Default("x")
		p.MaxVisibleItems = value
		if err := ValidateProfile(p); err == nil {
			t.Errorf("maxVisibleItems=%d: expected an error", value)
		}
	}
}

func TestValidateProfileAcceptsBoundaryMaxVisibleItems(t *testing.T) {
	for _, value := range []int{1, 100} {
		p := Default("x")
		p.MaxVisibleItems = value
		if err := ValidateProfile(p); err != nil {
			t.Errorf("maxVisibleItems=%d: unexpected error: %v", value, err)
		}
	}
}

func TestValidateProfileMessageLifetimeZeroMeansNoExpiry(t *testing.T) {
	p := Default("x")
	p.MessageLifetimeSeconds = 0
	if err := ValidateProfile(p); err != nil {
		t.Fatalf("0 (no timed expiry) unexpectedly rejected: %v", err)
	}
}

func TestValidateProfileRejectsOutOfRangeMessageLifetime(t *testing.T) {
	for _, value := range []int{1, 2, 601, -5} {
		p := Default("x")
		p.MessageLifetimeSeconds = value
		if err := ValidateProfile(p); err == nil {
			t.Errorf("messageLifetimeSeconds=%d: expected an error", value)
		}
	}
}

func TestValidateProfileAcceptsBoundaryMessageLifetime(t *testing.T) {
	for _, value := range []int{3, 600} {
		p := Default("x")
		p.MessageLifetimeSeconds = value
		if err := ValidateProfile(p); err != nil {
			t.Errorf("messageLifetimeSeconds=%d: unexpected error: %v", value, err)
		}
	}
}

func TestValidateProfileRejectsInvalidColors(t *testing.T) {
	cases := []string{"red", "rgb(0,0,0)", "#GGGGGG", "#12345", "", "black"}
	for _, value := range cases {
		p := Default("x")
		p.TextColor = value
		if err := ValidateProfile(p); err == nil {
			t.Errorf("textColor=%q: expected an error", value)
		}
	}
}

func TestValidateProfileAcceptsRRGGBBAndRRGGBBAAColors(t *testing.T) {
	for _, value := range []string{"#FFFFFF", "#000000", "#AbCdEf", "#12345678"} {
		p := Default("x")
		p.TextColor = value
		p.BubbleColor = value
		if err := ValidateProfile(p); err != nil {
			t.Errorf("color=%q: unexpected error: %v", value, err)
		}
	}
}

func TestValidateProfileRejectsNaNAndInfiniteFloats(t *testing.T) {
	p := Default("x")
	p.LineHeight = math.Inf(1)
	if err := ValidateProfile(p); err == nil {
		t.Fatal("expected an error for an infinite line height")
	}
}

func TestValidateProfileRejectsOutOfRangeAnimationDuration(t *testing.T) {
	for _, value := range []int{-1, 2001} {
		p := Default("x")
		p.AnimationDurationMS = value
		if err := ValidateProfile(p); err == nil {
			t.Errorf("animationDurationMs=%d: expected an error", value)
		}
	}
}

func TestIsValidHexColor(t *testing.T) {
	valid := []string{"#000000", "#FFFFFF", "#aAbBcC", "#12345678"}
	for _, v := range valid {
		if !IsValidHexColor(v) {
			t.Errorf("expected %q to be valid", v)
		}
	}
	invalid := []string{"", "000000", "#12345", "#1234567", "rgb(0,0,0)", "red", "#GGGGGG"}
	for _, v := range invalid {
		if IsValidHexColor(v) {
			t.Errorf("expected %q to be invalid", v)
		}
	}
}

func TestValidateBlockedTermRejectsEmpty(t *testing.T) {
	if err := ValidateBlockedTerm("   ", MatchContains); err == nil {
		t.Fatal("expected an error for an empty/whitespace-only term")
	}
}

func TestValidateBlockedTermRejectsOversized(t *testing.T) {
	long := make([]byte, MaxBlockedTermLength+1)
	for i := range long {
		long[i] = 'a'
	}
	if err := ValidateBlockedTerm(string(long), MatchContains); err == nil {
		t.Fatal("expected an error for an oversized term")
	}
}

func TestValidateBlockedTermAcceptsBoundaryLength(t *testing.T) {
	exact := make([]byte, MaxBlockedTermLength)
	for i := range exact {
		exact[i] = 'a'
	}
	if err := ValidateBlockedTerm(string(exact), MatchContains); err != nil {
		t.Fatalf("unexpected error at the exact max length: %v", err)
	}
}

func TestValidateBlockedTermRejectsUnknownMatchMode(t *testing.T) {
	if err := ValidateBlockedTerm("spam", "regex"); err == nil {
		t.Fatal("expected an error for an unsupported match mode")
	}
}

func TestNormalizeTermFoldsCaseAndTrims(t *testing.T) {
	if got := NormalizeTerm("  SPAM  "); got != "spam" {
		t.Errorf("NormalizeTerm() = %q, want %q", got, "spam")
	}
}

func TestNormalizeTermUnicodeCaseFolding(t *testing.T) {
	if got := NormalizeTerm("ŚPAM"); got != "śpam" {
		t.Errorf("NormalizeTerm() = %q, want %q", got, "śpam")
	}
}
