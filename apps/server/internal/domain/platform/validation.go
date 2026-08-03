package platform

import (
	"strings"
	"unicode/utf8"
)

const (
	// DisplayNameMaxLength bounds the user-chosen name of a destination.
	DisplayNameMaxLength = 80

	// CategoryMaxLength bounds the category-like field for every provider.
	CategoryMaxLength = 100

	// TagMinLength is the shortest accepted tag, matching the editor's rule.
	TagMinLength = 2

	// SortOrderMax keeps ordering values in a sane range.
	SortOrderMax = 10_000
)

// NormalizeDisplayName trims surrounding whitespace. Inner spacing and any
// Unicode the user typed are preserved exactly.
func NormalizeDisplayName(name string) string {
	return strings.TrimSpace(name)
}

// validateDisplayName checks a trimmed display name.
func validateDisplayName(name string, v *ValidationError) {
	if name == "" {
		v.Add("displayName", RuleRequired, "Display name is required.", nil)
		return
	}
	if utf8.RuneCountInString(name) > DisplayNameMaxLength {
		v.Addf("displayName", RuleTooLong, map[string]any{"max": DisplayNameMaxLength},
			"Display name cannot exceed %d characters.", DisplayNameMaxLength)
	}
}

func validateSortOrder(sortOrder int, v *ValidationError) {
	if sortOrder < 0 {
		v.Add("sortOrder", RuleInvalid, "Sort order cannot be negative.", nil)
		return
	}
	if sortOrder > SortOrderMax {
		v.Addf("sortOrder", RuleInvalid, map[string]any{"max": SortOrderMax},
			"Sort order cannot exceed %d.", SortOrderMax)
	}
}

// ValidateCreate checks a create request against the provider registry.
func ValidateCreate(input CreateInput) (CreateInput, error) {
	v := &ValidationError{}

	if !KnownProvider(input.ProviderID) {
		// Reported as a field violation so the client can highlight the select,
		// while the service also surfaces ErrUnknownProvider for the status code.
		v.Addf("providerId", RuleUnsupported, nil,
			"Unknown provider %q.", string(input.ProviderID))
	}

	input.DisplayName = NormalizeDisplayName(input.DisplayName)
	validateDisplayName(input.DisplayName, v)

	if input.SortOrder != nil {
		validateSortOrder(*input.SortOrder, v)
	}

	return input, v.OrNil()
}

// ValidateUpdate checks a full-replacement update request.
func ValidateUpdate(input UpdateInput) (UpdateInput, error) {
	v := &ValidationError{}

	input.DisplayName = NormalizeDisplayName(input.DisplayName)
	validateDisplayName(input.DisplayName, v)
	validateSortOrder(input.SortOrder, v)

	return input, v.OrNil()
}

// tagPattern mirrors the editor's rule: letters, digits, spaces, "-" and "_".
func isAllowedTagRune(r rune) bool {
	switch {
	case r == ' ' || r == '-' || r == '_':
		return true
	case r >= '0' && r <= '9':
		return true
	default:
		// Unicode letters and digits from any script are accepted, so a Polish
		// or Japanese tag is as valid as an ASCII one.
		return isLetterOrDigit(r)
	}
}

// ValidateMetadata checks metadata against the capability table, the limits and
// the option lists of the platform's provider, and returns the normalized value.
//
// A field the provider does not support is rejected when it carries a
// meaningful value, and silently reset when it is empty - so a client that
// always sends the whole object does not have to know the capability table.
func ValidateMetadata(def ProviderDefinition, in Metadata) (Metadata, error) {
	v := &ValidationError{}
	caps := def.Capabilities
	limits := def.Limits

	out := Metadata{}

	// --- title -------------------------------------------------------------
	title := strings.TrimSpace(in.Title)
	if caps.Title {
		if utf8.RuneCountInString(title) > limits.TitleMaxLength {
			v.Addf("title", RuleTooLong, map[string]any{"max": limits.TitleMaxLength},
				"Title cannot exceed %d characters.", limits.TitleMaxLength)
		}
		out.Title = title
	} else if title != "" {
		v.Add("title", RuleNotSupported, "This provider does not support a title.", nil)
	}

	// --- description -------------------------------------------------------
	description := in.Description
	if caps.Description {
		if utf8.RuneCountInString(description) > limits.DescriptionMaxLength {
			v.Addf("description", RuleTooLong, map[string]any{"max": limits.DescriptionMaxLength},
				"Description cannot exceed %d characters.", limits.DescriptionMaxLength)
		}
		out.Description = description
	} else if strings.TrimSpace(description) != "" {
		v.Add("description", RuleNotSupported, "This provider does not support a description.", nil)
	}

	// --- category ----------------------------------------------------------
	category := strings.TrimSpace(in.Category)
	if caps.Category {
		if utf8.RuneCountInString(category) > CategoryMaxLength {
			v.Addf("category", RuleTooLong, map[string]any{"max": CategoryMaxLength},
				"Category cannot exceed %d characters.", CategoryMaxLength)
		}
		out.Category = category
	} else if category != "" {
		v.Add("category", RuleNotSupported, "This provider does not support a category.", nil)
	}

	// --- tags --------------------------------------------------------------
	out.Tags = []string{}
	if caps.Tags {
		seen := make(map[string]struct{}, len(in.Tags))
		for _, raw := range in.Tags {
			tag := strings.TrimSpace(raw)
			if tag == "" {
				continue
			}

			if utf8.RuneCountInString(tag) < TagMinLength {
				v.Addf("tags", RuleTooShort, map[string]any{"min": TagMinLength},
					"A tag needs at least %d characters.", TagMinLength)
				continue
			}
			if utf8.RuneCountInString(tag) > limits.TagMaxLength {
				v.Addf("tags", RuleTooLong, map[string]any{"max": limits.TagMaxLength},
					"A tag cannot exceed %d characters.", limits.TagMaxLength)
				continue
			}
			if !allRunesAllowed(tag) {
				v.Add("tags", RuleInvalid,
					"Tags may only contain letters, digits, spaces, - and _.", nil)
				continue
			}

			// Case-insensitive uniqueness: "Go" and "go" are the same tag.
			key := strings.ToLower(tag)
			if _, duplicate := seen[key]; duplicate {
				v.Addf("tags", RuleDuplicate, nil, "Tag %q is duplicated.", tag)
				continue
			}
			seen[key] = struct{}{}

			out.Tags = append(out.Tags, tag)
		}

		if len(out.Tags) > limits.MaxTags {
			v.Addf("tags", RuleTooMany, map[string]any{"max": limits.MaxTags},
				"At most %d tags are allowed.", limits.MaxTags)
		}
	} else if hasNonEmpty(in.Tags) {
		v.Add("tags", RuleNotSupported, "This provider does not support tags.", nil)
	}

	// --- language ----------------------------------------------------------
	language := strings.TrimSpace(in.Language)
	if caps.Language {
		if language != "" && !contains(def.LanguageOptions, language) {
			v.Addf("language", RuleUnsupported, nil,
				"Language %q is not supported by this provider.", language)
		}
		out.Language = language
	} else if language != "" {
		v.Add("language", RuleNotSupported, "This provider does not support a language.", nil)
	}

	// --- visibility --------------------------------------------------------
	visibility := strings.TrimSpace(in.Visibility)
	if caps.Visibility {
		if visibility != "" && !contains(def.VisibilityOptions, visibility) {
			v.Addf("visibility", RuleUnsupported, nil,
				"Visibility %q is not supported by this provider.", visibility)
		}
		out.Visibility = visibility
	} else if visibility != "" {
		v.Add("visibility", RuleNotSupported, "This provider does not support visibility.", nil)
	}

	// --- latency mode ------------------------------------------------------
	latency := strings.TrimSpace(in.LatencyMode)
	if caps.LatencyMode {
		if latency != "" && !contains(def.LatencyOptions, latency) {
			v.Addf("latencyMode", RuleUnsupported, nil,
				"Latency mode %q is not supported by this provider.", latency)
		}
		out.LatencyMode = latency
	} else if latency != "" {
		v.Add("latencyMode", RuleNotSupported, "This provider does not support a latency mode.", nil)
	}

	// --- booleans ----------------------------------------------------------
	if caps.MatureContent {
		out.MatureContent = in.MatureContent
	} else if in.MatureContent {
		v.Add("matureContent", RuleNotSupported,
			"This provider does not support a mature content flag.", nil)
	}

	if caps.DVR {
		out.DVR = in.DVR
	} else if in.DVR {
		v.Add("dvr", RuleNotSupported, "This provider does not support DVR.", nil)
	}

	return out, v.OrNil()
}

func hasNonEmpty(values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func allRunesAllowed(s string) bool {
	for _, r := range s {
		if !isAllowedTagRune(r) {
			return false
		}
	}
	return true
}
