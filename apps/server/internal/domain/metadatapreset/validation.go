package metadatapreset

import (
	"strings"
	"unicode/utf8"

	"github.com/streaming-tree/server/internal/domain/platform"
)

// Bounds (docs/metadata-presets.md §2's own table). A preset is not
// tied to one provider, so it stores at the most generous real bound
// across every provider (never truncating meaningful content at save
// time); the existing, unchanged platform.ValidateMetadata is what
// enforces a specific target provider's real, tighter limits at apply
// time (docs/metadata-presets.md §9).
const (
	NameMaxLength = 100
	NoteMaxLength = 280

	// TitleMaxLength is the most generous real per-provider
	// TitleMaxLength (Twitch, 140) as of this stage's provider
	// registry (internal/domain/platform/definitions.go).
	TitleMaxLength = 140
	// DescriptionMaxLength is the most generous real per-provider
	// description limit (YouTube, 5000 UTF-8 bytes).
	DescriptionMaxLength = 5000
	// MaxTags/TagMaxLength/TagsCombinedMaxLength are the most generous
	// real per-provider tag limits (YouTube).
	MaxTags               = 500
	TagMaxLength          = 100
	TagsCombinedMaxLength = 500

	// MaxPresets bounds the number of presets one installation may
	// hold - generous, not arbitrary: no legitimate creator workflow
	// needs more, and it prevents unbounded growth.
	MaxPresets = 200
)

// Tag character rules are deliberately NOT re-validated here: a preset
// is not tied to one provider, and platform.ValidateMetadata already
// re-validates every real character/format rule (provider-specific tag
// rules included) at apply time against the real target provider - see
// docs/metadata-presets.md §9. This function only enforces the
// preset's own generic storage bounds.

// NormalizeName trims surrounding whitespace.
func NormalizeName(name string) string {
	return strings.TrimSpace(name)
}

func validateName(name string, v *platform.ValidationError) {
	if name == "" {
		v.Add("name", platform.RuleRequired, "Preset name is required.", nil)
		return
	}
	if utf8.RuneCountInString(name) > NameMaxLength {
		v.Addf("name", platform.RuleTooLong, map[string]any{"max": NameMaxLength},
			"Preset name cannot exceed %d characters.", NameMaxLength)
	}
}

func validateNote(note string, v *platform.ValidationError) {
	if utf8.RuneCountInString(note) > NoteMaxLength {
		v.Addf("note", platform.RuleTooLong, map[string]any{"max": NoteMaxLength},
			"Preset note cannot exceed %d characters.", NoteMaxLength)
	}
}

func validateCommon(common CommonMetadata, v *platform.ValidationError) CommonMetadata {
	out := CommonMetadata{
		Title:         strings.TrimSpace(common.Title),
		Description:   common.Description,
		Language:      strings.TrimSpace(common.Language),
		Visibility:    strings.TrimSpace(common.Visibility),
		MatureContent: common.MatureContent,
		DVR:           common.DVR,
		LatencyMode:   strings.TrimSpace(common.LatencyMode),
		Tags:          []string{},
	}

	if utf8.RuneCountInString(out.Title) > TitleMaxLength {
		v.Addf("common.title", platform.RuleTooLong, map[string]any{"max": TitleMaxLength},
			"Title cannot exceed %d characters.", TitleMaxLength)
	}
	if len(out.Description) > DescriptionMaxLength {
		v.Addf("common.description", platform.RuleTooLong, map[string]any{"max": DescriptionMaxLength},
			"Description cannot exceed %d bytes.", DescriptionMaxLength)
	}

	combined := 0
	for i, raw := range common.Tags {
		tag := strings.TrimSpace(raw)
		if tag == "" {
			continue
		}
		if utf8.RuneCountInString(tag) > TagMaxLength {
			v.Addf("common.tags", platform.RuleTooLong, map[string]any{"max": TagMaxLength},
				"A tag cannot exceed %d characters.", TagMaxLength)
			continue
		}
		out.Tags = append(out.Tags, tag)
		length := len(tag)
		if i > 0 {
			length++
		}
		combined += length
	}
	if len(out.Tags) > MaxTags {
		v.Addf("common.tags", platform.RuleTooMany, map[string]any{"max": MaxTags},
			"At most %d tags are allowed.", MaxTags)
	}
	if combined > TagsCombinedMaxLength {
		v.Addf("common.tags", platform.RuleTooLong, map[string]any{"max": TagsCombinedMaxLength},
			"Tags cannot exceed %d combined characters.", TagsCombinedMaxLength)
	}

	return out
}

func validateProviders(providers map[platform.ProviderID]ProviderMetadata, v *platform.ValidationError) map[platform.ProviderID]ProviderMetadata {
	out := make(map[platform.ProviderID]ProviderMetadata, len(providers))
	for id, pm := range providers {
		if !platform.KnownProvider(id) {
			v.Addf("providers", platform.RuleUnsupported, nil, "Unknown provider %q.", string(id))
			continue
		}

		category := strings.TrimSpace(pm.Category)
		categoryID := strings.TrimSpace(pm.CategoryID)
		if utf8.RuneCountInString(category) > platform.CategoryMaxLength {
			v.Addf("providers", platform.RuleTooLong, map[string]any{"max": platform.CategoryMaxLength},
				"Category cannot exceed %d characters.", platform.CategoryMaxLength)
			continue
		}
		if utf8.RuneCountInString(categoryID) > platform.CategoryIDMaxLength {
			v.Addf("providers", platform.RuleTooLong, map[string]any{"max": platform.CategoryIDMaxLength},
				"Category ID cannot exceed %d characters.", platform.CategoryIDMaxLength)
			continue
		}
		if category == "" && categoryID == "" {
			// Nothing to store for this provider - omit rather than
			// keep an empty entry.
			continue
		}
		out[id] = ProviderMetadata{Category: category, CategoryID: categoryID}
	}
	return out
}

// ValidateCreate checks and normalizes a create request.
func ValidateCreate(input CreateInput) (CreateInput, error) {
	v := &platform.ValidationError{}

	input.Name = NormalizeName(input.Name)
	validateName(input.Name, v)
	validateNote(input.Note, v)
	input.Common = validateCommon(input.Common, v)
	input.Providers = validateProviders(input.Providers, v)

	return input, v.OrNil()
}

// ValidateUpdate checks and normalizes a full-replacement update request.
func ValidateUpdate(input UpdateInput) (UpdateInput, error) {
	v := &platform.ValidationError{}

	input.Name = NormalizeName(input.Name)
	validateName(input.Name, v)
	validateNote(input.Note, v)
	input.Common = validateCommon(input.Common, v)
	input.Providers = validateProviders(input.Providers, v)

	return input, v.OrNil()
}
