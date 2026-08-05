package engagement

import (
	"testing"
	"time"
)

func fixedTime() time.Time {
	return time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
}

func baseEvent() Event {
	return Event{
		SchemaVersion:      CurrentSchemaVersion,
		ProviderID:         ProviderTwitch,
		ConnectedAccountID: "acct_1",
		Type:               TypeFollow,
		PlatformTimestamp:  fixedTime(),
		DedupeKey:          "dedupe-1",
	}
}

func TestValidateAcceptsMinimalFollowEvent(t *testing.T) {
	if err := baseEvent().Validate(); err != nil {
		t.Fatalf("expected valid event, got %v", err)
	}
}

func TestValidateRejectsUnknownSchemaVersion(t *testing.T) {
	e := baseEvent()
	e.SchemaVersion = 99
	if err := e.Validate(); err == nil {
		t.Fatal("expected error for unknown schema version")
	}
}

func TestValidateRejectsMissingProviderID(t *testing.T) {
	e := baseEvent()
	e.ProviderID = ""
	if err := e.Validate(); err == nil {
		t.Fatal("expected error for missing providerId")
	}
}

func TestValidateRejectsUnknownType(t *testing.T) {
	e := baseEvent()
	e.Type = "not.a.real.type"
	if err := e.Validate(); err == nil {
		t.Fatal("expected error for unknown type")
	}
}

func TestValidateRequiresMessageAndUserForChatMessage(t *testing.T) {
	e := baseEvent()
	e.Type = TypeChatMessage
	if err := e.Validate(); err == nil {
		t.Fatal("expected error: chat.message with no message/user")
	}

	msg := NewMessage([]Fragment{{Type: FragmentText, Text: "hi"}})
	e.Message = &msg
	if err := e.Validate(); err == nil {
		t.Fatal("expected error: chat.message with message but no user")
	}

	e.User = &User{ProviderUserID: "u1"}
	if err := e.Validate(); err != nil {
		t.Fatalf("expected valid chat.message, got %v", err)
	}
}

func TestValidateRequiresModerationRefForDeletion(t *testing.T) {
	e := baseEvent()
	e.Type = TypeChatMessageDeleted
	if err := e.Validate(); err == nil {
		t.Fatal("expected error: message_deleted with no moderationRef")
	}
	e.ModerationRef = "evt_original"
	if err := e.Validate(); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

func TestValidateRequiresModerationRefOrActionForModeration(t *testing.T) {
	e := baseEvent()
	e.Type = TypeModeration
	if err := e.Validate(); err == nil {
		t.Fatal("expected error: moderation with neither a moderationRef nor a moderationAction")
	}
	e.ModerationAction = "clear_user_messages"
	if err := e.Validate(); err != nil {
		t.Fatalf("expected valid moderation event with only a moderationAction (e.g. clear_user_messages, which targets a user, not a specific prior message), got %v", err)
	}
}

func TestValidateRequiresQuantityForBits(t *testing.T) {
	e := baseEvent()
	e.Type = TypeBits
	if err := e.Validate(); err == nil {
		t.Fatal("expected error: bits with no quantity")
	}
	q := int64(100)
	e.Quantity = &q
	if err := e.Validate(); err != nil {
		t.Fatalf("expected valid, got %v", err)
	}
}

func TestValidateRejectsMismatchedMessageText(t *testing.T) {
	e := baseEvent()
	e.Type = TypeChatMessage
	e.User = &User{ProviderUserID: "u1"}
	e.Message = &Message{
		Fragments: []Fragment{{Type: FragmentText, Text: "hello"}},
		Text:      "not hello", // deliberately inconsistent with fragments
	}
	if err := e.Validate(); err == nil {
		t.Fatal("expected error: message.Text inconsistent with fragments")
	}
}

func TestValidateRejectsOversizedProviderExtra(t *testing.T) {
	e := baseEvent()
	e.ProviderExtra = map[string]string{}
	for i := 0; i < maxProviderExtraEntries+1; i++ {
		e.ProviderExtra[string(rune('a'+i))] = "v"
	}
	if err := e.Validate(); err == nil {
		t.Fatal("expected error for too many providerExtra entries")
	}
}
