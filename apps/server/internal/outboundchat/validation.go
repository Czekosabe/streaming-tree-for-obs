package outboundchat

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxMessageCodePoints is Twitch's own documented Send Chat Message limit -
// see docs/provider-integrations/twitch-outbound-chat.md. Enforced here so
// an over-length message never reaches a provider call at all.
const MaxMessageCodePoints = 500

// MaxReplyParentMessageIDBytes bounds a reply-parent identifier - a
// generous ceiling for an opaque provider message ID, never meant to be
// reached by a real one.
const MaxReplyParentMessageIDBytes = 256

// Message validation errors.
var (
	ErrMessageInvalidUTF8          = errors.New("message is not valid UTF-8")
	ErrMessageEmpty                = errors.New("message is empty")
	ErrMessageTooLong              = errors.New("message exceeds the maximum length")
	ErrMessageControlCharacter     = errors.New("message contains a disallowed control character")
	ErrReplyParentMessageIDInvalid = errors.New("reply parent message id is invalid")
)

// isDisallowedControl reports whether r is a C0 control character this
// application never accepts in a chat message or a reply-parent id - NUL,
// CR, LF, every other C0 control (0x00-0x1F), and DEL (0x7F). Ordinary
// spaces, emoji, combining characters and Twitch emote names (plain text)
// are all outside this range and pass through unchanged.
func isDisallowedControl(r rune) bool {
	return r < 0x20 || r == 0x7f
}

// ValidateMessage enforces Stage 11A's backend-authoritative message rules:
// valid UTF-8, non-empty after trimming Unicode whitespace, a maximum of
// MaxMessageCodePoints Unicode code points, and no C0 control character
// (which also rejects NUL and CR/LF multiline content, both C0 controls).
// Never truncates, rewrites a URL, parses Markdown, or renders HTML - the
// message this validates is sent to the provider exactly as accepted.
func ValidateMessage(message string) error {
	if !utf8.ValidString(message) {
		return ErrMessageInvalidUTF8
	}
	if strings.TrimFunc(message, unicode.IsSpace) == "" {
		return ErrMessageEmpty
	}
	count := 0
	for _, r := range message {
		if isDisallowedControl(r) {
			return ErrMessageControlCharacter
		}
		count++
		if count > MaxMessageCodePoints {
			return ErrMessageTooLong
		}
	}
	return nil
}

// ValidateReplyParentMessageID validates an optional reply-parent
// identifier: empty is valid (no reply), otherwise it must be valid UTF-8,
// bounded in length, and free of control characters. This is a pure shape
// check only - it says nothing about whether the id actually belongs to a
// real message the sending account may reference; the HTTP layer enforces
// that separately (see internal/httpapi's own reply-selection handling).
func ValidateReplyParentMessageID(id string) error {
	if id == "" {
		return nil
	}
	if len(id) > MaxReplyParentMessageIDBytes {
		return ErrReplyParentMessageIDInvalid
	}
	if !utf8.ValidString(id) {
		return ErrReplyParentMessageIDInvalid
	}
	for _, r := range id {
		if isDisallowedControl(r) {
			return ErrReplyParentMessageIDInvalid
		}
	}
	return nil
}
