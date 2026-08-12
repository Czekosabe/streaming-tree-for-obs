package alerts

import (
	"errors"
	"testing"

	domain "github.com/streaming-tree/server/internal/domain/alerts"
)

func ptr(s string) *string { return &s }

func TestRenderNormal(t *testing.T) {
	res, err := Render("{username} followed!", Context{Username: ptr("Ann")})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if res.Text != "Ann followed!" {
		t.Errorf("Text = %q, want %q", res.Text, "Ann followed!")
	}
	if len(res.Resolved) != 1 || res.Resolved[0] != "username" {
		t.Errorf("Resolved = %v, want [username]", res.Resolved)
	}
}

func TestRenderMultiplePlaceholders(t *testing.T) {
	q := int64(500)
	res, err := Render("{username} gave {quantity} bits on {platform}", Context{
		Username: ptr("Ann"), Quantity: &q, Platform: "Twitch",
	})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if res.Text != "Ann gave 500 bits on Twitch" {
		t.Errorf("Text = %q", res.Text)
	}
}

func TestRenderAdjacentPlaceholders(t *testing.T) {
	res, err := Render("{username}{platform}", Context{Username: ptr("Ann"), Platform: "Twitch"})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if res.Text != "AnnTwitch" {
		t.Errorf("Text = %q, want AnnTwitch", res.Text)
	}
}

func TestRenderEscapedBraces(t *testing.T) {
	res, err := Render("{{literal}} {username}", Context{Username: ptr("Ann")})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if res.Text != "{literal} Ann" {
		t.Errorf("Text = %q, want {literal} Ann", res.Text)
	}
}

func TestRenderUnknownPlaceholderUnresolvedNotFatal(t *testing.T) {
	res, err := Render("hello {bogus}", Context{})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if res.Text != "hello " {
		t.Errorf("Text = %q, want %q", res.Text, "hello ")
	}
	if len(res.Unresolved) != 1 || res.Unresolved[0] != "bogus" {
		t.Errorf("Unresolved = %v, want [bogus]", res.Unresolved)
	}
	if res.ValidForProvider {
		t.Error("ValidForProvider = true with an unresolved placeholder, want false")
	}
}

func TestRenderUnmatchedBraceIsFatal(t *testing.T) {
	if _, err := Render("hello {username", Context{}); !errors.Is(err, ErrPlaceholderInvalid) {
		t.Errorf("Render() error = %v, want ErrPlaceholderInvalid", err)
	}
	if _, err := Render("hello username}", Context{}); !errors.Is(err, ErrPlaceholderInvalid) {
		t.Errorf("Render() error = %v, want ErrPlaceholderInvalid", err)
	}
}

func TestRenderNestedLookingBraceRejected(t *testing.T) {
	if _, err := Render("{user{name}}", Context{}); !errors.Is(err, ErrPlaceholderInvalid) {
		t.Errorf("Render() error = %v, want ErrPlaceholderInvalid for a nested-looking brace", err)
	}
}

func TestRenderUnavailablePlaceholderResolvesEmpty(t *testing.T) {
	res, err := Render("{message}", Context{Message: nil})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if res.Text != "" || len(res.Unresolved) != 1 {
		t.Errorf("Render(nil Message) = %+v, want empty text and one unresolved placeholder", res)
	}
}

func TestRenderAnonymousUserUnresolved(t *testing.T) {
	res, err := Render("{username} cheered", Context{Username: nil})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if len(res.Unresolved) != 1 || res.Unresolved[0] != "username" {
		t.Errorf("Unresolved = %v, want [username] for an anonymous actor", res.Unresolved)
	}
}

func TestRenderUnicode(t *testing.T) {
	res, err := Render("👋 {username} 🎉", Context{Username: ptr("Zażółć")})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if res.Text != "👋 Zażółć 🎉" {
		t.Errorf("Text = %q", res.Text)
	}
}

func TestRenderNeverInterpretsHTML(t *testing.T) {
	res, err := Render("{username}", Context{Username: ptr("<b>Ann</b>")})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if res.Text != "<b>Ann</b>" {
		t.Errorf("Text = %q, want the literal tag text unmodified (never stripped/escaped here - the renderer's own responsibility)", res.Text)
	}
}

func TestRenderLongOutputCodePointCount(t *testing.T) {
	long := ""
	for i := 0; i < 300; i++ {
		long += "a"
	}
	res, err := Render(long, Context{})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if res.CodePointCount != 300 {
		t.Errorf("CodePointCount = %d, want 300", res.CodePointCount)
	}
	if res.ValidForProvider {
		t.Error("ValidForProvider = true for 300 code points, want false (over MaxRenderedCodePoints)")
	}
}

func TestValidateTemplatePlaceholdersRejectsUnknown(t *testing.T) {
	if err := ValidateTemplatePlaceholders("{bogus}"); !errors.Is(err, ErrPlaceholderInvalid) {
		t.Errorf("ValidateTemplatePlaceholders(bogus) error = %v, want ErrPlaceholderInvalid", err)
	}
	if err := ValidateTemplatePlaceholders("{username}"); err != nil {
		t.Errorf("ValidateTemplatePlaceholders(username) error = %v, want nil", err)
	}
}

func TestValidateTemplateForEventTypeRejectsUnsupportedPlaceholder(t *testing.T) {
	if err := ValidateTemplateForEventType("{quantity}", domain.EventFollow); !errors.Is(err, ErrPlaceholderInvalid) {
		t.Errorf("ValidateTemplateForEventType({quantity}, follow) error = %v, want ErrPlaceholderInvalid", err)
	}
	if err := ValidateTemplateForEventType("{quantity}", domain.EventBits); err != nil {
		t.Errorf("ValidateTemplateForEventType({quantity}, bits) error = %v, want nil", err)
	}
	if err := ValidateTemplateForEventType("{rewardTitle}", domain.EventChannelPointRedemption); err != nil {
		t.Errorf("ValidateTemplateForEventType({rewardTitle}, channel_point_redemption) error = %v, want nil", err)
	}
}

func TestAvailablePlaceholdersMatchesCapability(t *testing.T) {
	avail := AvailablePlaceholders(domain.EventFollow)
	for _, p := range []string{"quantity", "message", "rewardTitle"} {
		for _, a := range avail {
			if a == p {
				t.Errorf("AvailablePlaceholders(follow) unexpectedly includes %q", p)
			}
		}
	}
	avail = AvailablePlaceholders(domain.EventBits)
	found := false
	for _, a := range avail {
		if a == "quantity" {
			found = true
		}
	}
	if !found {
		t.Error("AvailablePlaceholders(bits) does not include quantity")
	}
}

func TestEventTypeLabelLocalized(t *testing.T) {
	if got := EventTypeLabel(domain.EventFollow, domain.LanguageEnglish); got != "Follow" {
		t.Errorf("EventTypeLabel(follow, en) = %q, want Follow", got)
	}
	if got := EventTypeLabel(domain.EventFollow, domain.LanguagePolish); got != "Obserwacja" {
		t.Errorf("EventTypeLabel(follow, pl) = %q, want Obserwacja", got)
	}
}

func TestEventTypeLabelLocalizedYouTube(t *testing.T) {
	if got := EventTypeLabel(domain.EventYouTubeSuperChat, domain.LanguageEnglish); got != "Super Chat" {
		t.Errorf("EventTypeLabel(youtube_super_chat, en) = %q, want %q", got, "Super Chat")
	}
	if got := EventTypeLabel(domain.EventYouTubeSuperChat, domain.LanguagePolish); got != "Super Chat" {
		t.Errorf("EventTypeLabel(youtube_super_chat, pl) = %q, want %q", got, "Super Chat")
	}
	if got := EventTypeLabel(domain.EventYouTubeMembership, domain.LanguagePolish); got != "Członkostwo" {
		t.Errorf("EventTypeLabel(youtube_membership, pl) = %q, want %q", got, "Członkostwo")
	}
}

func TestPlatformDisplayNameNeverTranslated(t *testing.T) {
	if got := PlatformDisplayName(domain.ProviderTwitch); got != "Twitch" {
		t.Errorf("PlatformDisplayName(twitch) = %q, want Twitch", got)
	}
}

func TestPlatformDisplayNameYouTube(t *testing.T) {
	if got := PlatformDisplayName(domain.ProviderYouTube); got != "YouTube" {
		t.Errorf("PlatformDisplayName(youtube) = %q, want YouTube", got)
	}
}

func TestAvailablePlaceholdersYouTubeMoneyAndMembership(t *testing.T) {
	avail := AvailablePlaceholders(domain.EventYouTubeSuperChat)
	wantHas := map[string]bool{"amount": false, "currency": false, "membershipLevel": false}
	for _, a := range avail {
		if _, ok := wantHas[a]; ok {
			wantHas[a] = true
		}
	}
	if !wantHas["amount"] || !wantHas["currency"] {
		t.Errorf("AvailablePlaceholders(youtube_super_chat) = %v, want amount and currency present", avail)
	}
	if wantHas["membershipLevel"] {
		t.Errorf("AvailablePlaceholders(youtube_super_chat) unexpectedly includes membershipLevel")
	}

	avail = AvailablePlaceholders(domain.EventYouTubeMembership)
	found := false
	for _, a := range avail {
		if a == "membershipLevel" {
			found = true
		}
		if a == "amount" {
			t.Error("AvailablePlaceholders(youtube_membership) unexpectedly includes amount")
		}
	}
	if !found {
		t.Error("AvailablePlaceholders(youtube_membership) does not include membershipLevel")
	}
}

func TestValidateTemplateForEventTypeAcceptsMoneyPlaceholders(t *testing.T) {
	if err := ValidateTemplateForEventType("{amount} {currency}", domain.EventYouTubeSuperChat); err != nil {
		t.Errorf("ValidateTemplateForEventType({amount}{currency}, youtube_super_chat) error = %v, want nil", err)
	}
	if err := ValidateTemplateForEventType("{amount}", domain.EventFollow); !errors.Is(err, ErrPlaceholderInvalid) {
		t.Errorf("ValidateTemplateForEventType({amount}, follow) error = %v, want ErrPlaceholderInvalid", err)
	}
	if err := ValidateTemplateForEventType("{membershipLevel}", domain.EventYouTubeMembership); err != nil {
		t.Errorf("ValidateTemplateForEventType({membershipLevel}, youtube_membership) error = %v, want nil", err)
	}
}

func TestFormatAmountMicros(t *testing.T) {
	cases := []struct {
		micros int64
		want   string
	}{
		{5_000_000, "5.00"},
		{1_500_000, "1.50"},
		{0, "0.00"},
		{999, "0.000999"},
		{1_000_001, "1.000001"},
	}
	for _, c := range cases {
		if got := FormatAmountMicros(c.micros); got != c.want {
			t.Errorf("FormatAmountMicros(%d) = %q, want %q", c.micros, got, c.want)
		}
	}
}
