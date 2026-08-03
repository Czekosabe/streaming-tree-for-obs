package platform

import "sort"

// Built-in provider registry.
//
// This table was moved here from the frontend so the backend is the single
// source of truth for provider capabilities.
//
// IMPORTANT: these definitions are approximate and were NOT verified against
// the real Twitch, YouTube, Kick or TikTok APIs. They preserve the behaviour
// the dashboard had before persistence was introduced and exist to drive the
// capability-based metadata editor. They must be re-checked when real API
// integrations are implemented.

// supportedLanguages lists the stream language identifiers offered for
// providers that expose a language field. They are BCP 47 primary subtags; the
// frontend renders each as an endonym.
var supportedLanguages = []string{"en", "pl", "de", "es", "fr"}

var providerDefinitions = map[ProviderID]ProviderDefinition{
	ProviderTwitch: {
		ID:                ProviderTwitch,
		BrandName:         "Twitch",
		ShortLabel:        "TW",
		CategoryFieldType: CategoryFieldCategory,
		Capabilities: Capabilities{
			Title:         true,
			Description:   false,
			Category:      true,
			Tags:          true,
			Language:      true,
			Visibility:    false,
			MatureContent: true,
			DVR:           false,
			LatencyMode:   true,
		},
		Limits: Limits{
			TitleMaxLength:       140,
			DescriptionMaxLength: 0,
			MaxTags:              10,
			TagMaxLength:         25,
		},
		VisibilityOptions: []string{},
		LatencyOptions:    []string{LatencyLow, LatencyNormal},
		LanguageOptions:   supportedLanguages,
	},
	ProviderYouTube: {
		ID:                ProviderYouTube,
		BrandName:         "YouTube Live",
		ShortLabel:        "YT",
		CategoryFieldType: CategoryFieldCategory,
		Capabilities: Capabilities{
			Title:         true,
			Description:   true,
			Category:      true,
			Tags:          false,
			Language:      true,
			Visibility:    true,
			MatureContent: true,
			DVR:           true,
			LatencyMode:   true,
		},
		Limits: Limits{
			TitleMaxLength:       100,
			DescriptionMaxLength: 5000,
			MaxTags:              0,
			TagMaxLength:         0,
		},
		VisibilityOptions: []string{VisibilityPublic, VisibilityUnlisted, VisibilityPrivate},
		LatencyOptions:    []string{LatencyNormal, LatencyLow, LatencyUltraLow},
		LanguageOptions:   supportedLanguages,
	},
	ProviderKick: {
		ID:                ProviderKick,
		BrandName:         "Kick",
		ShortLabel:        "KI",
		CategoryFieldType: CategoryFieldCategory,
		Capabilities: Capabilities{
			Title:         true,
			Description:   false,
			Category:      true,
			Tags:          false,
			Language:      true,
			Visibility:    false,
			MatureContent: true,
			DVR:           false,
			LatencyMode:   false,
		},
		Limits: Limits{
			TitleMaxLength:       100,
			DescriptionMaxLength: 0,
			MaxTags:              0,
			TagMaxLength:         0,
		},
		VisibilityOptions: []string{},
		LatencyOptions:    []string{},
		LanguageOptions:   supportedLanguages,
	},
	ProviderTikTok: {
		ID:                ProviderTikTok,
		BrandName:         "TikTok Live",
		ShortLabel:        "TT",
		CategoryFieldType: CategoryFieldTopic,
		Capabilities: Capabilities{
			Title:         true,
			Description:   false,
			Category:      true,
			Tags:          false,
			Language:      false,
			Visibility:    false,
			MatureContent: false,
			DVR:           false,
			LatencyMode:   false,
		},
		Limits: Limits{
			TitleMaxLength:       60,
			DescriptionMaxLength: 0,
			MaxTags:              0,
			TagMaxLength:         0,
		},
		VisibilityOptions: []string{},
		LatencyOptions:    []string{},
		LanguageOptions:   []string{},
	},
}

// definitionOrder fixes the order provider definitions are listed in, so the
// API response is deterministic.
var definitionOrder = []ProviderID{
	ProviderTwitch,
	ProviderYouTube,
	ProviderKick,
	ProviderTikTok,
}

// Definition returns the built-in definition for a provider.
//
// The second result is false for any unknown provider; callers must treat that
// as a validation failure rather than substituting a default.
func Definition(id ProviderID) (ProviderDefinition, bool) {
	def, ok := providerDefinitions[id]
	return def, ok
}

// Definitions returns every built-in provider definition in a stable order.
func Definitions() []ProviderDefinition {
	out := make([]ProviderDefinition, 0, len(definitionOrder))
	for _, id := range definitionOrder {
		if def, ok := providerDefinitions[id]; ok {
			out = append(out, def)
		}
	}

	// Defensive: surface any provider added to the map but missing from the
	// explicit order rather than silently dropping it from the API.
	if len(out) != len(providerDefinitions) {
		seen := make(map[ProviderID]struct{}, len(out))
		for _, def := range out {
			seen[def.ID] = struct{}{}
		}
		var missing []ProviderDefinition
		for id, def := range providerDefinitions {
			if _, ok := seen[id]; !ok {
				missing = append(missing, def)
			}
		}
		sort.Slice(missing, func(i, j int) bool { return missing[i].ID < missing[j].ID })
		out = append(out, missing...)
	}

	return out
}

// KnownProvider reports whether the identifier maps to a built-in provider.
func KnownProvider(id ProviderID) bool {
	_, ok := providerDefinitions[id]
	return ok
}
