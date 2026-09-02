package streamsetup

import (
	"strings"
	"unicode/utf8"

	"github.com/streaming-tree/server/internal/domain/platform"
)

// Bounds mirror metadatapreset's own identical NameMaxLength/
// NoteMaxLength (docs/stream-setup-profiles.md §13) - the same
// generic "reusable local configuration" text-field shape, so the
// same limits apply for the same reason: immediate feedback and a
// storage ceiling, with the server remaining the real authority.
const (
	NameMaxLength = 100
	NoteMaxLength = 280

	// MaxProfiles bounds the number of stream setup profiles one
	// installation may hold - generous, not arbitrary: no legitimate
	// creator workflow needs more distinct shows configured at once,
	// and it prevents unbounded growth, mirroring
	// metadatapreset.MaxPresets.
	MaxProfiles = 200
)

// NormalizeName trims surrounding whitespace.
func NormalizeName(name string) string {
	return strings.TrimSpace(name)
}

func validateName(name string, v *platform.ValidationError) {
	if name == "" {
		v.Add("name", platform.RuleRequired, "Setup profile name is required.", nil)
		return
	}
	if utf8.RuneCountInString(name) > NameMaxLength {
		v.Addf("name", platform.RuleTooLong, map[string]any{"max": NameMaxLength},
			"Setup profile name cannot exceed %d characters.", NameMaxLength)
	}
}

func validateNote(note string, v *platform.ValidationError) {
	if utf8.RuneCountInString(note) > NoteMaxLength {
		v.Addf("note", platform.RuleTooLong, map[string]any{"max": NoteMaxLength},
			"Setup profile note cannot exceed %d characters.", NoteMaxLength)
	}
}

// ValidateCreate checks and normalizes a create request's own bounded
// text fields. Destination/metadata-preset reference validity is
// checked separately by the Service against real current state, not
// here.
func ValidateCreate(input CreateInput) (CreateInput, error) {
	v := &platform.ValidationError{}
	input.Name = NormalizeName(input.Name)
	validateName(input.Name, v)
	validateNote(input.Note, v)
	return input, v.OrNil()
}

// ValidateUpdate checks and normalizes a full-replacement update
// request's own bounded text fields.
func ValidateUpdate(input UpdateInput) (UpdateInput, error) {
	v := &platform.ValidationError{}
	input.Name = NormalizeName(input.Name)
	validateName(input.Name, v)
	validateNote(input.Note, v)
	return input, v.OrNil()
}
