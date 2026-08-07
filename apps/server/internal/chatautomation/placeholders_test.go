package chatautomation

import (
	"errors"
	"testing"
)

func strPtr(s string) *string { return &s }

func TestParseTemplateLiteralAndPlaceholder(t *testing.T) {
	segs, err := ParseTemplate("Hello {channelName}!")
	if err != nil {
		t.Fatalf("ParseTemplate() error = %v", err)
	}
	if len(segs) != 3 || segs[0].text != "Hello " || segs[1].text != "channelName" || segs[2].text != "!" {
		t.Fatalf("segs = %+v", segs)
	}
}

func TestParseTemplateAdjacentPlaceholders(t *testing.T) {
	segs, err := ParseTemplate("{channelName}{platform}")
	if err != nil {
		t.Fatalf("ParseTemplate() error = %v", err)
	}
	if len(segs) != 2 || segs[0].text != "channelName" || segs[1].text != "platform" {
		t.Fatalf("segs = %+v", segs)
	}
}

func TestParseTemplateEscapedBraces(t *testing.T) {
	segs, err := ParseTemplate("Visit {{example}}")
	if err != nil {
		t.Fatalf("ParseTemplate() error = %v", err)
	}
	if len(segs) != 1 || segs[0].text != "Visit {example}" {
		t.Fatalf("segs = %+v, want a single literal 'Visit {example}'", segs)
	}
}

func TestParseTemplateUnmatchedBraceRejected(t *testing.T) {
	cases := []string{"hello {name", "hello }name", "{"}
	for _, c := range cases {
		if _, err := ParseTemplate(c); !errors.Is(err, ErrPlaceholderInvalid) {
			t.Errorf("ParseTemplate(%q) error = %v, want ErrPlaceholderInvalid", c, err)
		}
	}
}

func TestParseTemplateNestedLookingSyntaxRejected(t *testing.T) {
	if _, err := ParseTemplate("{foo{bar}}"); !errors.Is(err, ErrPlaceholderInvalid) {
		t.Errorf("ParseTemplate(nested) error = %v, want ErrPlaceholderInvalid", err)
	}
}

func TestParseTemplateEmptyPlaceholderRejected(t *testing.T) {
	if _, err := ParseTemplate("{}"); !errors.Is(err, ErrPlaceholderInvalid) {
		t.Errorf("ParseTemplate(empty) error = %v, want ErrPlaceholderInvalid", err)
	}
}

func TestParseTemplateUnicodeSurroundingText(t *testing.T) {
	segs, err := ParseTemplate("Witaj {channelName}! 🎉")
	if err != nil {
		t.Fatalf("ParseTemplate() error = %v", err)
	}
	if len(segs) != 3 || segs[2].text != "! 🎉" {
		t.Fatalf("segs = %+v", segs)
	}
}

func TestValidateTemplatePlaceholdersRejectsUnknownName(t *testing.T) {
	if err := ValidateTemplatePlaceholders("hi {viewerCount}"); !errors.Is(err, ErrPlaceholderInvalid) {
		t.Errorf("ValidateTemplatePlaceholders(unknown) error = %v, want ErrPlaceholderInvalid", err)
	}
}

func TestValidateTemplatePlaceholdersAcceptsKnownNames(t *testing.T) {
	for _, name := range KnownPlaceholders {
		if err := ValidateTemplatePlaceholders("hi {" + name + "}"); err != nil {
			t.Errorf("ValidateTemplatePlaceholders(%q) error = %v", name, err)
		}
	}
}

func TestRenderResolvesChannelPlatformURL(t *testing.T) {
	ctx := Context{ChannelName: "Streamer", Platform: "Twitch", ChannelURL: "https://www.twitch.tv/streamer"}
	result, err := Render("Hi from {channelName} on {platform}: {channelUrl}", ctx)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	want := "Hi from Streamer on Twitch: https://www.twitch.tv/streamer"
	if result.Text != want {
		t.Errorf("Text = %q, want %q", result.Text, want)
	}
	if len(result.Unresolved) != 0 {
		t.Errorf("Unresolved = %v, want none", result.Unresolved)
	}
	if !result.ValidForProvider {
		t.Error("ValidForProvider = false, want true")
	}
}

func TestRenderStreamTitleUnresolved(t *testing.T) {
	ctx := Context{ChannelName: "Streamer", Platform: "Twitch"}
	result, err := Render("Now playing: {streamTitle}", ctx)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if result.Text != "Now playing: " {
		t.Errorf("Text = %q, want empty substitution", result.Text)
	}
	if len(result.Unresolved) != 1 || result.Unresolved[0] != "streamTitle" {
		t.Errorf("Unresolved = %v, want [streamTitle]", result.Unresolved)
	}
	if result.ValidForProvider {
		t.Error("ValidForProvider = true, want false (unresolved placeholder)")
	}
}

func TestRenderStreamTitleResolved(t *testing.T) {
	ctx := Context{ChannelName: "Streamer", Platform: "Twitch", StreamTitle: strPtr("Ranked grind")}
	result, err := Render("{streamTitle}", ctx)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if result.Text != "Ranked grind" {
		t.Errorf("Text = %q, want %q", result.Text, "Ranked grind")
	}
}

func TestRenderUnknownPlaceholderTreatedAsUnresolved(t *testing.T) {
	result, err := Render("hi {mystery}", Context{})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if len(result.Unresolved) != 1 || result.Unresolved[0] != "mystery" {
		t.Errorf("Unresolved = %v, want [mystery]", result.Unresolved)
	}
}

func TestRenderRejectedForMalformedSyntax(t *testing.T) {
	if _, err := Render("hi {name", Context{}); !errors.Is(err, ErrPlaceholderInvalid) {
		t.Errorf("Render(malformed) error = %v, want ErrPlaceholderInvalid", err)
	}
}

func TestRenderCodePointBoundary(t *testing.T) {
	exact := make([]byte, 500)
	for i := range exact {
		exact[i] = 'a'
	}
	result, err := Render(string(exact), Context{})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if result.CodePointCount != 500 || !result.ValidForProvider {
		t.Errorf("500-char render: count=%d validForProvider=%v, want 500/true", result.CodePointCount, result.ValidForProvider)
	}

	over := append(exact, 'a')
	result, err = Render(string(over), Context{})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if result.CodePointCount != 501 || result.ValidForProvider {
		t.Errorf("501-char render: count=%d validForProvider=%v, want 501/false", result.CodePointCount, result.ValidForProvider)
	}
}

func TestPlatformDisplayNameNeverTranslated(t *testing.T) {
	if got := PlatformDisplayName("twitch"); got != "Twitch" {
		t.Errorf("PlatformDisplayName(twitch) = %q, want Twitch", got)
	}
}

func TestChannelURLTwitch(t *testing.T) {
	url, ok := ChannelURL("twitch", "streamer")
	if !ok || url != "https://www.twitch.tv/streamer" {
		t.Errorf("ChannelURL() = %q, %v, want https://www.twitch.tv/streamer, true", url, ok)
	}
}
