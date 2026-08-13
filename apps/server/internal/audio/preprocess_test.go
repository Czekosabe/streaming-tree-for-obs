package audio

import (
	"strings"
	"testing"
)

func baseCfg() PreprocessConfig {
	return PreprocessConfig{
		SuppressCommands:       true,
		RemoveURLs:             true,
		NormalizeRepeatedChars: true,
		MaxLengthCodePoints:    500,
	}
}

func TestPreprocessPassesThroughPlainText(t *testing.T) {
	got, ok := Preprocess("hello world", baseCfg())
	if !ok || got != "hello world" {
		t.Errorf("Preprocess() = %q, %v, want %q, true", got, ok, "hello world")
	}
}

func TestPreprocessCommandSuppression(t *testing.T) {
	cfg := baseCfg()
	cfg.IsCommand = true
	if _, ok := Preprocess("!uptime", cfg); ok {
		t.Error("Preprocess() ok = true for a suppressed command, want false")
	}

	cfg.SuppressCommands = false
	got, ok := Preprocess("!uptime", cfg)
	if !ok || got != "!uptime" {
		t.Errorf("Preprocess() = %q, %v, want command text preserved when suppression disabled", got, ok)
	}
}

func TestPreprocessRemovesURLs(t *testing.T) {
	got, ok := Preprocess("check this out http://example.com/path?x=1 cool right", baseCfg())
	want := "check this out cool right"
	if !ok || got != want {
		t.Errorf("Preprocess() = %q, %v, want %q, true", got, ok, want)
	}
}

func TestPreprocessRemovesHTTPSAndBareWWW(t *testing.T) {
	got, ok := Preprocess("visit https://a.example and www.b.example now", baseCfg())
	want := "visit and now"
	if !ok || got != want {
		t.Errorf("Preprocess() = %q, %v, want %q, true", got, ok, want)
	}
}

func TestPreprocessDoesNotRemoveURLsWhenDisabled(t *testing.T) {
	cfg := baseCfg()
	cfg.RemoveURLs = false
	got, ok := Preprocess("go to http://example.com now", cfg)
	want := "go to http://example.com now"
	if !ok || got != want {
		t.Errorf("Preprocess() = %q, %v, want %q, true", got, ok, want)
	}
}

func TestPreprocessHostileVeryLongURLIsRemoved(t *testing.T) {
	longURL := "http://example.com/" + strings.Repeat("a", 5000)
	got, ok := Preprocess("look "+longURL+" wow", baseCfg())
	want := "look wow"
	if !ok || got != want {
		t.Errorf("Preprocess() = %q, %v, want %q, true", got, ok, want)
	}
}

func TestPreprocessBlockedWordsWholeTokenOnly(t *testing.T) {
	cfg := baseCfg()
	cfg.BlockedWords = []string{"ass"}
	got, ok := Preprocess("that class is ass", cfg)
	want := "that class is"
	if !ok || got != want {
		t.Errorf("Preprocess() = %q, %v, want %q, true (never a substring match inside 'class')", got, ok, want)
	}
}

func TestPreprocessBlockedWordsCaseInsensitive(t *testing.T) {
	cfg := baseCfg()
	cfg.BlockedWords = []string{"spam"}
	got, ok := Preprocess("no SPAM here", cfg)
	want := "no here"
	if !ok || got != want {
		t.Errorf("Preprocess() = %q, %v, want %q, true", got, ok, want)
	}
}

func TestPreprocessBlockedWordsWithPunctuation(t *testing.T) {
	cfg := baseCfg()
	cfg.BlockedWords = []string{"spam"}
	got, ok := Preprocess("no spam! here", cfg)
	want := "no here"
	if !ok || got != want {
		t.Errorf("Preprocess() = %q, %v, want %q, true", got, ok, want)
	}
}

func TestPreprocessRepeatedCharacterNormalization(t *testing.T) {
	got, ok := Preprocess("sooooooooo cool", baseCfg())
	want := "sooo cool"
	if !ok || got != want {
		t.Errorf("Preprocess() = %q, %v, want %q, true", got, ok, want)
	}
}

func TestPreprocessRepeatedCharacterNormalizationNeverTouchesDoubledLetter(t *testing.T) {
	got, ok := Preprocess("good stuff", baseCfg())
	want := "good stuff"
	if !ok || got != want {
		t.Errorf("Preprocess() = %q, %v, want doubled letters preserved", got, ok)
	}
}

func TestPreprocessRepeatedCharacterNormalizationHugeInputIsBounded(t *testing.T) {
	huge := strings.Repeat("a", 100000)
	got, ok := Preprocess(huge, baseCfg())
	if !ok || got != "aaa" {
		t.Errorf("Preprocess(huge repeat) = %q, %v, want aaa, true", got, ok)
	}
}

func TestPreprocessRepeatedCharacterNormalizationUnicodeAndEmoji(t *testing.T) {
	got, ok := Preprocess("😀😀😀😀😀 nice", baseCfg())
	want := "😀😀😀 nice"
	if !ok || got != want {
		t.Errorf("Preprocess() = %q, %v, want %q, true", got, ok, want)
	}
}

func TestPreprocessWhitespaceNormalization(t *testing.T) {
	got, ok := Preprocess("  hello   world  ", baseCfg())
	want := "hello world"
	if !ok || got != want {
		t.Errorf("Preprocess() = %q, %v, want %q, true", got, ok, want)
	}
}

func TestPreprocessMaxCodePointLength(t *testing.T) {
	cfg := baseCfg()
	cfg.MaxLengthCodePoints = 10
	got, ok := Preprocess("this is a longer sentence than allowed", cfg)
	if !ok {
		t.Fatal("Preprocess() ok = false, want true")
	}
	if len([]rune(got)) > 10 {
		t.Errorf("Preprocess() result has %d code points, want <= 10", len([]rune(got)))
	}
}

func TestPreprocessMaxCodePointLengthNeverSplitsRune(t *testing.T) {
	cfg := baseCfg()
	cfg.MaxLengthCodePoints = 3
	got, ok := Preprocess("héllo", cfg)
	if !ok {
		t.Fatal("Preprocess() ok = false, want true")
	}
	for _, r := range got {
		if r == 0xFFFD {
			t.Errorf("Preprocess() produced a replacement character, rune was split: %q", got)
		}
	}
}

func TestPreprocessRejectsEmptyResult(t *testing.T) {
	cfg := baseCfg()
	cfg.BlockedWords = []string{"hello"}
	if _, ok := Preprocess("hello", cfg); ok {
		t.Error("Preprocess() ok = true for an entirely-blocked message, want false")
	}
	if _, ok := Preprocess("   ", baseCfg()); ok {
		t.Error("Preprocess() ok = true for whitespace-only input, want false")
	}
	if _, ok := Preprocess("", baseCfg()); ok {
		t.Error("Preprocess() ok = true for empty input, want false")
	}
}
