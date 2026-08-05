package engagement

import "strings"

// FragmentType is the kind of one ordered piece of a chat-shaped message.
type FragmentType string

const (
	FragmentText      FragmentType = "text"
	FragmentEmote     FragmentType = "emote"
	FragmentCheermote FragmentType = "cheermote"
	FragmentMention   FragmentType = "mention"
	// FragmentUnknown is used when a connector receives a fragment type it
	// does not recognize (for example, a new Twitch fragment type added
	// after this code was written). The fragment's Text is still carried,
	// so a consumer can still render something reasonable instead of
	// silently dropping part of the message.
	FragmentUnknown FragmentType = "unknown"
)

// Fragment is one ordered piece of a chat-shaped message. A message is a
// list of these, never a single pre-rendered string, so a consumer (an
// overlay, an operator-chat view) can style an emote or a mention without
// re-parsing plain text.
type Fragment struct {
	Type FragmentType
	// Text is always populated for every fragment type - the literal text a
	// plain-text rendering should show for this fragment (an emote's own
	// name/code, a cheermote's literal token, a mention's "@login").
	Text string

	// EmoteID is set only when Type == FragmentEmote.
	EmoteID string

	// CheermotePrefix and CheermoteBits are set only when
	// Type == FragmentCheermote.
	CheermotePrefix string
	CheermoteBits   int

	// MentionUserID, MentionLogin and MentionDisplayName are set only when
	// Type == FragmentMention. The provider does not always resolve all
	// three; each is left empty rather than guessed when absent.
	MentionUserID      string
	MentionLogin       string
	MentionDisplayName string
}

// Message is a chat-shaped event's content.
type Message struct {
	// Fragments is the ordered, authoritative content.
	Fragments []Fragment
	// Text is a convenience field only - see BuildText. It is always the
	// deterministic concatenation of every fragment's Text, never an
	// independent value a connector could set inconsistently.
	Text string
}

// NewMessage builds a Message from fragments, deriving Text deterministically
// so the two can never disagree.
func NewMessage(fragments []Fragment) Message {
	return Message{Fragments: fragments, Text: BuildText(fragments)}
}

// BuildText deterministically concatenates every fragment's Text field with
// no separator, mirroring how Twitch's own message.text field is built from
// message.fragments. Kept as a standalone, independently testable function
// so Message's Text field's derivation is provably deterministic rather
// than merely documented as such.
func BuildText(fragments []Fragment) string {
	var b strings.Builder
	for _, f := range fragments {
		b.WriteString(f.Text)
	}
	return b.String()
}
