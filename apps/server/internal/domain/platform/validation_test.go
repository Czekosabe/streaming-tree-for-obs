package platform

import (
	"strings"
	"testing"
)

// violationRules returns the rule identifiers reported for one field.
func violationRules(t *testing.T, err error, field string) []string {
	t.Helper()

	verr, ok := AsValidationError(err)
	if !ok {
		t.Fatalf("error %v is not a *ValidationError", err)
	}

	var rules []string
	for _, violation := range verr.Violations {
		if violation.Field == field {
			rules = append(rules, violation.Rule)
		}
	}
	return rules
}

func mustDefinition(t *testing.T, id ProviderID) ProviderDefinition {
	t.Helper()
	def, ok := Definition(id)
	if !ok {
		t.Fatalf("provider %q is not registered", id)
	}
	return def
}

// --- provider registry ------------------------------------------------------

func TestRegistryContainsTheFourBuiltInProviders(t *testing.T) {
	definitions := Definitions()
	if len(definitions) != 4 {
		t.Fatalf("Definitions() returned %d providers, want 4", len(definitions))
	}

	wantOrder := []ProviderID{ProviderTwitch, ProviderYouTube, ProviderKick, ProviderTikTok}
	for i, want := range wantOrder {
		if definitions[i].ID != want {
			t.Errorf("provider %d = %q, want %q", i, definitions[i].ID, want)
		}
	}
}

func TestTwitchKeepsTagSupport(t *testing.T) {
	def := mustDefinition(t, ProviderTwitch)

	if !def.Capabilities.Tags {
		t.Error("Twitch lost tag support")
	}
	if def.Limits.MaxTags <= 0 {
		t.Errorf("Twitch MaxTags = %d, want a positive limit", def.Limits.MaxTags)
	}
}

func TestOnlyTwitchSupportsTags(t *testing.T) {
	for _, id := range []ProviderID{ProviderYouTube, ProviderKick, ProviderTikTok} {
		if mustDefinition(t, id).Capabilities.Tags {
			t.Errorf("provider %q unexpectedly supports tags", id)
		}
	}
}

func TestUnknownProviderIsRejected(t *testing.T) {
	if KnownProvider("myspace") {
		t.Error("KnownProvider() accepted an unknown provider")
	}
	if _, ok := Definition("myspace"); ok {
		t.Error("Definition() returned a definition for an unknown provider")
	}
}

func TestDefinitionsExposeNoLocalizedProse(t *testing.T) {
	// Every option is a stable semantic identifier: lowercase ASCII with no
	// spaces. A localized label such as "Publiczny" would fail this.
	for _, def := range Definitions() {
		options := append(append([]string{}, def.VisibilityOptions...), def.LatencyOptions...)
		options = append(options, def.LanguageOptions...)
		options = append(options, string(def.CategoryFieldType))

		for _, option := range options {
			if option != strings.ToLower(option) || strings.Contains(option, " ") {
				t.Errorf("provider %q exposes %q, want a lowercase identifier without spaces",
					def.ID, option)
			}
		}
	}
}

// --- create / update validation ---------------------------------------------

func TestValidateCreateRejectsUnknownProvider(t *testing.T) {
	_, err := ValidateCreate(CreateInput{ProviderID: "myspace", DisplayName: "Anything"})
	if err == nil {
		t.Fatal("ValidateCreate() accepted an unknown provider")
	}
	if rules := violationRules(t, err, "providerId"); len(rules) == 0 {
		t.Error("no violation was reported on providerId")
	}
}

func TestValidateCreateRejectsEmptyDisplayName(t *testing.T) {
	for _, name := range []string{"", "   ", "\t\n"} {
		_, err := ValidateCreate(CreateInput{ProviderID: ProviderTwitch, DisplayName: name})
		if err == nil {
			t.Fatalf("ValidateCreate() accepted the blank display name %q", name)
		}
		if rules := violationRules(t, err, "displayName"); len(rules) == 0 || rules[0] != RuleRequired {
			t.Errorf("display name %q reported %v, want [%s]", name, rules, RuleRequired)
		}
	}
}

func TestValidateCreateRejectsOversizedDisplayName(t *testing.T) {
	_, err := ValidateCreate(CreateInput{
		ProviderID:  ProviderTwitch,
		DisplayName: strings.Repeat("a", DisplayNameMaxLength+1),
	})
	if err == nil {
		t.Fatal("ValidateCreate() accepted an oversized display name")
	}
	if rules := violationRules(t, err, "displayName"); len(rules) == 0 || rules[0] != RuleTooLong {
		t.Errorf("reported %v, want [%s]", rules, RuleTooLong)
	}
}

func TestValidateCreateTrimsDisplayName(t *testing.T) {
	input, err := ValidateCreate(CreateInput{
		ProviderID:  ProviderTwitch,
		DisplayName: "  Main channel  ",
	})
	if err != nil {
		t.Fatalf("ValidateCreate() returned an error: %v", err)
	}
	if input.DisplayName != "Main channel" {
		t.Errorf("DisplayName = %q, want %q", input.DisplayName, "Main channel")
	}
}

func TestValidateCreateAcceptsMultipleConfigurationsPerProvider(t *testing.T) {
	// Nothing in validation ties a provider to a single configuration; the
	// uniqueness is on the generated platform id, not on providerId.
	for _, name := range []string{"Main channel", "Backup channel"} {
		if _, err := ValidateCreate(CreateInput{
			ProviderID: ProviderTwitch, DisplayName: name,
		}); err != nil {
			t.Fatalf("ValidateCreate() rejected %q: %v", name, err)
		}
	}
}

func TestValidateUpdateRejectsNegativeSortOrder(t *testing.T) {
	_, err := ValidateUpdate(UpdateInput{DisplayName: "Fine", SortOrder: -1})
	if err == nil {
		t.Fatal("ValidateUpdate() accepted a negative sort order")
	}
	if rules := violationRules(t, err, "sortOrder"); len(rules) == 0 {
		t.Error("no violation was reported on sortOrder")
	}
}

// --- metadata validation ----------------------------------------------------

func TestValidateMetadataAcceptsValidTwitchMetadata(t *testing.T) {
	def := mustDefinition(t, ProviderTwitch)

	out, err := ValidateMetadata(def, Metadata{
		Title:      "  Building a multistream tool  ",
		Category:   "Software and Game Development",
		CategoryID: "1469308723",
		Tags:       []string{"programming", "go", "obs"},
		Language:   "pl",
	})
	if err != nil {
		t.Fatalf("ValidateMetadata() returned an error: %v", err)
	}

	if out.Title != "Building a multistream tool" {
		t.Errorf("Title = %q, want it trimmed", out.Title)
	}
	if len(out.Tags) != 3 {
		t.Errorf("kept %d tags, want 3", len(out.Tags))
	}
	if out.CategoryID != "1469308723" {
		t.Errorf("CategoryID = %q, want it preserved", out.CategoryID)
	}
}

// TestValidateMetadataRejectsLatencyAndMatureContentForTwitch locks in the
// Stage 7A correction (docs/provider-integrations/twitch.md): Twitch's real
// Modify Channel Information endpoint has no latency-mode field and no
// single boolean equivalent to "mature content", so both are rejected as
// unsupported rather than accepted the way the earlier, unverified
// definition did.
func TestValidateMetadataRejectsLatencyAndMatureContentForTwitch(t *testing.T) {
	def := mustDefinition(t, ProviderTwitch)

	_, err := ValidateMetadata(def, Metadata{Title: "Fine", LatencyMode: LatencyLow})
	if rules := violationRules(t, err, "latencyMode"); len(rules) == 0 || rules[0] != RuleNotSupported {
		t.Errorf("latencyMode reported %v, want [%s]", rules, RuleNotSupported)
	}

	_, err = ValidateMetadata(def, Metadata{Title: "Fine", MatureContent: true})
	if rules := violationRules(t, err, "matureContent"); len(rules) == 0 || rules[0] != RuleNotSupported {
		t.Errorf("matureContent reported %v, want [%s]", rules, RuleNotSupported)
	}
}

// TestValidateMetadataRejectsStaleCategoryIDForProviderWithoutCategory
// mirrors the existing category behaviour: a categoryId sent for a provider
// with no category support at all is rejected, not silently dropped.
func TestValidateMetadataRejectsStaleCategoryIDForProviderWithoutCategory(t *testing.T) {
	def, ok := Definition(ProviderTikTok)
	if !ok {
		t.Fatal("TikTok definition not found")
	}
	def.Capabilities.Category = false // isolate this assertion from TikTok's own capability table

	_, err := ValidateMetadata(def, Metadata{Title: "Fine", CategoryID: "123"})
	if rules := violationRules(t, err, "categoryId"); len(rules) == 0 || rules[0] != RuleNotSupported {
		t.Errorf("reported %v, want [%s]", rules, RuleNotSupported)
	}
}

func TestValidateMetadataRejectsOversizedTitle(t *testing.T) {
	def := mustDefinition(t, ProviderTwitch)

	_, err := ValidateMetadata(def, Metadata{
		Title: strings.Repeat("x", def.Limits.TitleMaxLength+1),
	})
	if err == nil {
		t.Fatal("ValidateMetadata() accepted an oversized title")
	}
	if rules := violationRules(t, err, "title"); len(rules) == 0 || rules[0] != RuleTooLong {
		t.Errorf("reported %v, want [%s]", rules, RuleTooLong)
	}
}

func TestValidateMetadataRejectsTagsForProviderWithoutTagSupport(t *testing.T) {
	def := mustDefinition(t, ProviderYouTube)

	_, err := ValidateMetadata(def, Metadata{Title: "Fine", Tags: []string{"nope"}})
	if err == nil {
		t.Fatal("ValidateMetadata() accepted tags for a provider without tag support")
	}
	if rules := violationRules(t, err, "tags"); len(rules) == 0 || rules[0] != RuleNotSupported {
		t.Errorf("reported %v, want [%s]", rules, RuleNotSupported)
	}
}

func TestValidateMetadataIgnoresEmptyValuesForUnsupportedFields(t *testing.T) {
	def := mustDefinition(t, ProviderTikTok)

	// A client that always sends the whole object must not be punished for
	// sending empty values it cannot omit.
	out, err := ValidateMetadata(def, Metadata{
		Title:      "Live now",
		Category:   "Gaming",
		Tags:       []string{},
		Language:   "",
		Visibility: "",
	})
	if err != nil {
		t.Fatalf("ValidateMetadata() returned an error: %v", err)
	}
	if out.Language != "" || out.Visibility != "" {
		t.Errorf("unsupported fields were kept: language=%q visibility=%q", out.Language, out.Visibility)
	}
}

func TestValidateMetadataRejectsDuplicateTagsCaseInsensitively(t *testing.T) {
	def := mustDefinition(t, ProviderTwitch)

	_, err := ValidateMetadata(def, Metadata{
		Title: "Fine",
		Tags:  []string{"Programming", "programming"},
	})
	if err == nil {
		t.Fatal("ValidateMetadata() accepted tags differing only by case")
	}
	if rules := violationRules(t, err, "tags"); len(rules) == 0 || rules[0] != RuleDuplicate {
		t.Errorf("reported %v, want [%s]", rules, RuleDuplicate)
	}
}

func TestValidateMetadataRejectsTooManyTags(t *testing.T) {
	def := mustDefinition(t, ProviderTwitch)

	tags := make([]string, 0, def.Limits.MaxTags+1)
	for i := 0; i <= def.Limits.MaxTags; i++ {
		tags = append(tags, string(rune('a'+i))+"tag")
	}

	_, err := ValidateMetadata(def, Metadata{Title: "Fine", Tags: tags})
	if err == nil {
		t.Fatal("ValidateMetadata() accepted more tags than the limit")
	}
	if rules := violationRules(t, err, "tags"); len(rules) == 0 || rules[0] != RuleTooMany {
		t.Errorf("reported %v, want [%s]", rules, RuleTooMany)
	}
}

func TestValidateMetadataRejectsOversizedTag(t *testing.T) {
	def := mustDefinition(t, ProviderTwitch)

	_, err := ValidateMetadata(def, Metadata{
		Title: "Fine",
		Tags:  []string{strings.Repeat("x", def.Limits.TagMaxLength+1)},
	})
	if err == nil {
		t.Fatal("ValidateMetadata() accepted an oversized tag")
	}
}

func TestValidateMetadataRejectsUnsupportedVisibility(t *testing.T) {
	def := mustDefinition(t, ProviderYouTube)

	_, err := ValidateMetadata(def, Metadata{Title: "Fine", Visibility: "secret"})
	if err == nil {
		t.Fatal("ValidateMetadata() accepted an unsupported visibility")
	}
	if rules := violationRules(t, err, "visibility"); len(rules) == 0 || rules[0] != RuleUnsupported {
		t.Errorf("reported %v, want [%s]", rules, RuleUnsupported)
	}
}

func TestValidateMetadataRejectsUnsupportedLatency(t *testing.T) {
	// YouTube supports LatencyMode but only its own three documented
	// options; Twitch has no LatencyMode capability at all as of Stage 7A
	// (see TestValidateMetadataRejectsLatencyAndMatureContentForTwitch), so
	// this "supported field, disallowed value" case now needs a provider
	// that still has the capability.
	def := mustDefinition(t, ProviderYouTube)

	_, err := ValidateMetadata(def, Metadata{Title: "Fine", LatencyMode: "turbo"})
	if err == nil {
		t.Fatal("ValidateMetadata() accepted a latency mode the provider does not offer")
	}
	if rules := violationRules(t, err, "latencyMode"); len(rules) == 0 || rules[0] != RuleUnsupported {
		t.Errorf("reported %v, want [%s]", rules, RuleUnsupported)
	}
}

func TestValidateMetadataRejectsUnsupportedLanguage(t *testing.T) {
	def := mustDefinition(t, ProviderTwitch)

	_, err := ValidateMetadata(def, Metadata{Title: "Fine", Language: "kl"})
	if err == nil {
		t.Fatal("ValidateMetadata() accepted an unsupported language")
	}
}

func TestValidateMetadataRejectsDvrForProviderWithoutDvr(t *testing.T) {
	def := mustDefinition(t, ProviderTwitch)

	_, err := ValidateMetadata(def, Metadata{Title: "Fine", DVR: true})
	if err == nil {
		t.Fatal("ValidateMetadata() accepted DVR for a provider without DVR support")
	}
	if rules := violationRules(t, err, "dvr"); len(rules) == 0 || rules[0] != RuleNotSupported {
		t.Errorf("reported %v, want [%s]", rules, RuleNotSupported)
	}
}

func TestValidateMetadataPreservesUnicodeUserContent(t *testing.T) {
	def := mustDefinition(t, ProviderTwitch)

	title := "Zażółć gęślą jaźń — 日本語 ✨"
	out, err := ValidateMetadata(def, Metadata{
		Title: title,
		Tags:  []string{"zażółć", "日本語"},
	})
	if err != nil {
		t.Fatalf("ValidateMetadata() returned an error: %v", err)
	}
	if out.Title != title {
		t.Errorf("Title = %q, want %q unchanged", out.Title, title)
	}
	if out.Tags[0] != "zażółć" || out.Tags[1] != "日本語" {
		t.Errorf("tags = %v, want the exact Unicode values", out.Tags)
	}
}

func TestValidateMetadataRejectsTagWithForbiddenPunctuation(t *testing.T) {
	def := mustDefinition(t, ProviderTwitch)

	_, err := ValidateMetadata(def, Metadata{Title: "Fine", Tags: []string{"bad!tag"}})
	if err == nil {
		t.Fatal("ValidateMetadata() accepted a tag with forbidden punctuation")
	}
	if rules := violationRules(t, err, "tags"); len(rules) == 0 || rules[0] != RuleInvalid {
		t.Errorf("reported %v, want [%s]", rules, RuleInvalid)
	}
}

func TestValidateMetadataDropsBlankTags(t *testing.T) {
	def := mustDefinition(t, ProviderTwitch)

	out, err := ValidateMetadata(def, Metadata{
		Title: "Fine",
		Tags:  []string{"keep", "   ", ""},
	})
	if err != nil {
		t.Fatalf("ValidateMetadata() returned an error: %v", err)
	}
	if len(out.Tags) != 1 || out.Tags[0] != "keep" {
		t.Errorf("tags = %v, want [keep]", out.Tags)
	}
}
