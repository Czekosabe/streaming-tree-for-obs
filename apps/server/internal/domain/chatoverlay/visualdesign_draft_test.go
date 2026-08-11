package chatoverlay

import (
	"testing"

	"github.com/streaming-tree/server/internal/domain/visualdesign"
)

func TestGenerateLegacyDraftIsValid(t *testing.T) {
	profile := Default("Test overlay")
	doc := GenerateLegacyDraft(profile)
	if err := visualdesign.Validate(doc); err != nil {
		t.Fatalf("GenerateLegacyDraft() produced an invalid document: %v", err)
	}
	if err := ValidateDesignBindingsForChatOverlay(doc); err != nil {
		t.Fatalf("GenerateLegacyDraft() produced a document with an unavailable chat binding: %v", err)
	}
}

func TestGenerateLegacyDraftIsDeterministic(t *testing.T) {
	profile := Default("Test overlay")
	first := GenerateLegacyDraft(profile)
	second := GenerateLegacyDraft(profile)
	if len(first.Layers) != len(second.Layers) {
		t.Fatalf("layer counts differ: %d vs %d", len(first.Layers), len(second.Layers))
	}
	for i := range first.Layers {
		if first.Layers[i].ID != second.Layers[i].ID {
			t.Errorf("layer %d id differs between calls: %q vs %q", i, first.Layers[i].ID, second.Layers[i].ID)
		}
	}
}

func TestGenerateLegacyDraftAlwaysIncludesUsernameAndMessage(t *testing.T) {
	profile := Default("Test overlay")
	profile.ShowAvatar, profile.ShowBadges, profile.ShowTimestamp, profile.ShowAccountLabel = false, false, false, false
	doc := GenerateLegacyDraft(profile)

	var hasUsername, hasMessage bool
	for _, l := range doc.Layers {
		if l.Kind == visualdesign.LayerText && l.Text != nil && l.Text.Binding == visualdesign.BindingUsername {
			hasUsername = true
		}
		if l.Kind == visualdesign.LayerMessageFragments {
			hasMessage = true
		}
	}
	if !hasUsername || !hasMessage {
		t.Errorf("draft with every optional field off still needs username+message layers, got hasUsername=%v hasMessage=%v", hasUsername, hasMessage)
	}
}

func TestGenerateLegacyDraftIncludesOptionalLayersOnlyWhenEnabled(t *testing.T) {
	profile := Default("Test overlay")
	profile.ShowAvatar, profile.ShowBadges, profile.ShowTimestamp, profile.ShowAccountLabel = true, true, true, true
	full := GenerateLegacyDraft(profile)

	kinds := map[visualdesign.LayerKind]int{}
	for _, l := range full.Layers {
		kinds[l.Kind]++
	}
	if kinds[visualdesign.LayerAvatar] != 1 {
		t.Error("expected exactly one avatar layer when ShowAvatar is true")
	}
	if kinds[visualdesign.LayerBadgeList] != 1 {
		t.Error("expected exactly one badge_list layer when ShowBadges is true")
	}

	profile.ShowAvatar, profile.ShowBadges, profile.ShowTimestamp, profile.ShowAccountLabel = false, false, false, false
	minimal := GenerateLegacyDraft(profile)
	if len(minimal.Layers) >= len(full.Layers) {
		t.Errorf("minimal draft (%d layers) should have fewer layers than the full one (%d)", len(minimal.Layers), len(full.Layers))
	}
}

func TestGenerateLegacyDraftNeverPersists(t *testing.T) {
	// A structural/documentation test: GenerateLegacyDraft takes no
	// context.Context and no Repository/Service, so it cannot perform
	// I/O - this is a compile-time guarantee, exercised here only to
	// document the intent alongside the other draft tests.
	profile := Default("Test overlay")
	_ = GenerateLegacyDraft(profile)
}
