package metadatapreset

import (
	"strings"
	"testing"

	"github.com/streaming-tree/server/internal/domain/platform"
)

func TestValidateCreateTrimsName(t *testing.T) {
	out, err := ValidateCreate(CreateInput{Name: "  Padded  "})
	if err != nil {
		t.Fatalf("ValidateCreate() error = %v", err)
	}
	if out.Name != "Padded" {
		t.Errorf("Name = %q, want %q", out.Name, "Padded")
	}
}

func TestValidateCreateRejectsOversizeNote(t *testing.T) {
	_, err := ValidateCreate(CreateInput{
		Name: "ok",
		Note: strings.Repeat("a", NoteMaxLength+1),
	})
	if _, ok := platform.AsValidationError(err); !ok {
		t.Fatalf("ValidateCreate() error = %v, want a ValidationError", err)
	}
}

func TestValidateCreateRejectsOversizeTitle(t *testing.T) {
	_, err := ValidateCreate(CreateInput{
		Name:   "ok",
		Common: CommonMetadata{Title: strings.Repeat("a", TitleMaxLength+1)},
	})
	if _, ok := platform.AsValidationError(err); !ok {
		t.Fatalf("ValidateCreate() error = %v, want a ValidationError", err)
	}
}

func TestValidateCreateRejectsOversizeDescription(t *testing.T) {
	_, err := ValidateCreate(CreateInput{
		Name:   "ok",
		Common: CommonMetadata{Description: strings.Repeat("a", DescriptionMaxLength+1)},
	})
	if _, ok := platform.AsValidationError(err); !ok {
		t.Fatalf("ValidateCreate() error = %v, want a ValidationError", err)
	}
}

func TestValidateCreateRejectsTooManyTags(t *testing.T) {
	tags := make([]string, MaxTags+1)
	for i := range tags {
		tags[i] = "tag"
	}
	_, err := ValidateCreate(CreateInput{Name: "ok", Common: CommonMetadata{Tags: tags}})
	if _, ok := platform.AsValidationError(err); !ok {
		t.Fatalf("ValidateCreate() error = %v, want a ValidationError", err)
	}
}

func TestValidateCreateDropsEmptyTags(t *testing.T) {
	out, err := ValidateCreate(CreateInput{Name: "ok", Common: CommonMetadata{Tags: []string{"real", "  ", ""}}})
	if err != nil {
		t.Fatalf("ValidateCreate() error = %v", err)
	}
	if len(out.Common.Tags) != 1 || out.Common.Tags[0] != "real" {
		t.Fatalf("Common.Tags = %v, want [real]", out.Common.Tags)
	}
}

func TestValidateCreateRejectsUnknownProvider(t *testing.T) {
	_, err := ValidateCreate(CreateInput{
		Name: "ok",
		Providers: map[platform.ProviderID]ProviderMetadata{
			"not-a-real-provider": {Category: "x"},
		},
	})
	if _, ok := platform.AsValidationError(err); !ok {
		t.Fatalf("ValidateCreate() error = %v, want a ValidationError", err)
	}
}

func TestValidateCreateKeepsProviderScopingSeparate(t *testing.T) {
	// The central Stage 22 invariant: a Twitch category/ID must never
	// leak into the YouTube entry, or vice versa, even when both are
	// present in the same preset.
	out, err := ValidateCreate(CreateInput{
		Name: "ok",
		Providers: map[platform.ProviderID]ProviderMetadata{
			platform.ProviderTwitch:  {Category: "Software and Game Development", CategoryID: "509658"},
			platform.ProviderYouTube: {Category: "Science & Technology", CategoryID: "28"},
		},
	})
	if err != nil {
		t.Fatalf("ValidateCreate() error = %v", err)
	}
	twitch := out.Providers[platform.ProviderTwitch]
	youtube := out.Providers[platform.ProviderYouTube]
	if twitch.CategoryID != "509658" {
		t.Errorf("Twitch CategoryID = %q, want 509658", twitch.CategoryID)
	}
	if youtube.CategoryID != "28" {
		t.Errorf("YouTube CategoryID = %q, want 28", youtube.CategoryID)
	}
	if twitch.CategoryID == youtube.CategoryID {
		t.Fatal("Twitch and YouTube category IDs must never be conflated")
	}
}

func TestValidateCreateOmitsEmptyProviderEntries(t *testing.T) {
	out, err := ValidateCreate(CreateInput{
		Name: "ok",
		Providers: map[platform.ProviderID]ProviderMetadata{
			platform.ProviderTwitch: {}, // both fields empty
		},
	})
	if err != nil {
		t.Fatalf("ValidateCreate() error = %v", err)
	}
	if _, present := out.Providers[platform.ProviderTwitch]; present {
		t.Fatal("an entirely empty provider entry should be omitted, not stored")
	}
}
