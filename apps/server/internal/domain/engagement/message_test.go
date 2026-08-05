package engagement

import "testing"

func TestBuildTextConcatenatesFragmentsDeterministically(t *testing.T) {
	fragments := []Fragment{
		{Type: FragmentText, Text: "Hello "},
		{Type: FragmentMention, Text: "@friend", MentionLogin: "friend"},
		{Type: FragmentText, Text: " check this out "},
		{Type: FragmentEmote, Text: "Kappa", EmoteID: "25"},
	}
	got := BuildText(fragments)
	want := "Hello @friend check this out Kappa"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestNewMessageDerivesTextFromFragments(t *testing.T) {
	msg := NewMessage([]Fragment{{Type: FragmentText, Text: "abc"}})
	if msg.Text != "abc" {
		t.Fatalf("expected derived text %q, got %q", "abc", msg.Text)
	}
}

func TestBuildTextHandlesUnknownFragmentTypeGracefully(t *testing.T) {
	fragments := []Fragment{
		{Type: FragmentUnknown, Text: "???"},
	}
	if got := BuildText(fragments); got != "???" {
		t.Fatalf("expected unknown fragment text preserved, got %q", got)
	}
}

func TestBuildTextEmptyFragmentsProducesEmptyString(t *testing.T) {
	if got := BuildText(nil); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}
