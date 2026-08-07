package outboundchat

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateMessageAcceptsOrdinaryText(t *testing.T) {
	if err := ValidateMessage("hello chat, how is everyone?"); err != nil {
		t.Fatalf("ValidateMessage() error = %v", err)
	}
}

func TestValidateMessageAcceptsEmojiAndCombiningCharacters(t *testing.T) {
	// A flag emoji (multiple code points) and a combining-accent sequence -
	// both must pass through untouched, counted by code point.
	if err := ValidateMessage("great stream! 🎉🇵🇱 café"); err != nil {
		t.Fatalf("ValidateMessage() error = %v", err)
	}
}

func TestValidateMessageAcceptsTwitchEmoteNamesAsPlainText(t *testing.T) {
	if err := ValidateMessage("Kappa PogChamp that was great Kappa"); err != nil {
		t.Fatalf("ValidateMessage() error = %v", err)
	}
}

func TestValidateMessageRejectsEmpty(t *testing.T) {
	if err := ValidateMessage(""); !errors.Is(err, ErrMessageEmpty) {
		t.Fatalf("error = %v, want ErrMessageEmpty", err)
	}
}

func TestValidateMessageRejectsWhitespaceOnly(t *testing.T) {
	if err := ValidateMessage("   \t  "); !errors.Is(err, ErrMessageEmpty) {
		t.Fatalf("error = %v, want ErrMessageEmpty for whitespace-only input", err)
	}
}

func TestValidateMessageRejectsInvalidUTF8(t *testing.T) {
	if err := ValidateMessage("hello \xff\xfe"); !errors.Is(err, ErrMessageInvalidUTF8) {
		t.Fatalf("error = %v, want ErrMessageInvalidUTF8", err)
	}
}

func TestValidateMessageRejectsNUL(t *testing.T) {
	if err := ValidateMessage("hello\x00world"); !errors.Is(err, ErrMessageControlCharacter) {
		t.Fatalf("error = %v, want ErrMessageControlCharacter for NUL", err)
	}
}

func TestValidateMessageRejectsCRLF(t *testing.T) {
	if err := ValidateMessage("line one\r\nline two"); !errors.Is(err, ErrMessageControlCharacter) {
		t.Fatalf("error = %v, want ErrMessageControlCharacter for CRLF", err)
	}
}

func TestValidateMessageRejectsLoneLF(t *testing.T) {
	if err := ValidateMessage("line one\nline two"); !errors.Is(err, ErrMessageControlCharacter) {
		t.Fatalf("error = %v, want ErrMessageControlCharacter for a lone LF", err)
	}
}

func TestValidateMessageRejectsOtherC0Controls(t *testing.T) {
	if err := ValidateMessage("bell\x07sound"); !errors.Is(err, ErrMessageControlCharacter) {
		t.Fatalf("error = %v, want ErrMessageControlCharacter", err)
	}
}

func TestValidateMessageAcceptsExactly500CodePoints(t *testing.T) {
	msg := strings.Repeat("a", MaxMessageCodePoints)
	if err := ValidateMessage(msg); err != nil {
		t.Fatalf("ValidateMessage() error = %v, want nil at exactly the limit", err)
	}
}

func TestValidateMessageRejects501CodePoints(t *testing.T) {
	msg := strings.Repeat("a", MaxMessageCodePoints+1)
	if err := ValidateMessage(msg); !errors.Is(err, ErrMessageTooLong) {
		t.Fatalf("error = %v, want ErrMessageTooLong just above the limit", err)
	}
}

func TestValidateMessageNeverTruncatesItJustRejects(t *testing.T) {
	msg := strings.Repeat("a", MaxMessageCodePoints+50)
	err := ValidateMessage(msg)
	if !errors.Is(err, ErrMessageTooLong) {
		t.Fatalf("error = %v, want ErrMessageTooLong (never a silently truncated success)", err)
	}
}

func TestValidateReplyParentMessageIDAcceptsEmpty(t *testing.T) {
	if err := ValidateReplyParentMessageID(""); err != nil {
		t.Fatalf("ValidateReplyParentMessageID() error = %v, want nil for no reply", err)
	}
}

func TestValidateReplyParentMessageIDAcceptsAnOrdinaryID(t *testing.T) {
	if err := ValidateReplyParentMessageID("msg_abc-123"); err != nil {
		t.Fatalf("ValidateReplyParentMessageID() error = %v", err)
	}
}

func TestValidateReplyParentMessageIDRejectsControlCharacters(t *testing.T) {
	if err := ValidateReplyParentMessageID("msg\x00abc"); !errors.Is(err, ErrReplyParentMessageIDInvalid) {
		t.Fatalf("error = %v, want ErrReplyParentMessageIDInvalid", err)
	}
}

func TestValidateReplyParentMessageIDRejectsOversized(t *testing.T) {
	id := strings.Repeat("a", MaxReplyParentMessageIDBytes+1)
	if err := ValidateReplyParentMessageID(id); !errors.Is(err, ErrReplyParentMessageIDInvalid) {
		t.Fatalf("error = %v, want ErrReplyParentMessageIDInvalid for an oversized id", err)
	}
}

func TestValidateReplyParentMessageIDRejectsInvalidUTF8(t *testing.T) {
	if err := ValidateReplyParentMessageID("msg\xff\xfe"); !errors.Is(err, ErrReplyParentMessageIDInvalid) {
		t.Fatalf("error = %v, want ErrReplyParentMessageIDInvalid", err)
	}
}
