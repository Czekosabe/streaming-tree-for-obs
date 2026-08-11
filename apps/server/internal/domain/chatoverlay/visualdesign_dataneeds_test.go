package chatoverlay

import (
	"testing"

	"github.com/streaming-tree/server/internal/domain/visualdesign"
)

func TestDeriveDataNeedsFromEmptyDocument(t *testing.T) {
	needs := DeriveDataNeeds(visualdesign.Document{Version: visualdesign.CurrentVersion, Canvas: visualdesign.CanvasChatItem})
	if needs.Avatar || needs.Badges || needs.AccountLabel {
		t.Errorf("DeriveDataNeeds(empty) = %+v, want all false", needs)
	}
}

func TestDeriveDataNeedsDetectsAvatarLayer(t *testing.T) {
	doc := visualdesign.Document{
		Version: visualdesign.CurrentVersion, Canvas: visualdesign.CanvasChatItem,
		Layers: []visualdesign.Layer{{
			ID: "layer_avatar", Kind: visualdesign.LayerAvatar, Visible: true, Order: 0,
			Frame: visualdesign.Frame{X: 0, Y: 0, Width: 32, Height: 32}, Opacity: 1,
			Avatar: &visualdesign.AvatarProps{},
		}},
	}
	needs := DeriveDataNeeds(doc)
	if !needs.Avatar {
		t.Error("DeriveDataNeeds().Avatar = false, want true")
	}
	if needs.Badges || needs.AccountLabel {
		t.Errorf("DeriveDataNeeds() = %+v, want only Avatar true", needs)
	}
}

func TestDeriveDataNeedsDetectsBadgeListLayer(t *testing.T) {
	doc := visualdesign.Document{
		Version: visualdesign.CurrentVersion, Canvas: visualdesign.CanvasChatItem,
		Layers: []visualdesign.Layer{{
			ID: "layer_badges", Kind: visualdesign.LayerBadgeList, Visible: true, Order: 0,
			Frame: visualdesign.Frame{X: 0, Y: 0, Width: 100, Height: 20}, Opacity: 1,
			BadgeList: &visualdesign.BadgeListProps{MaxCount: 5, BadgeSize: 16, Gap: 2},
		}},
	}
	if !DeriveDataNeeds(doc).Badges {
		t.Error("DeriveDataNeeds().Badges = false, want true")
	}
}

func TestDeriveDataNeedsDetectsAccountLabelBinding(t *testing.T) {
	doc := chatDesignWithBinding(visualdesign.BindingAccountLabel)
	needs := DeriveDataNeeds(doc)
	if !needs.AccountLabel {
		t.Error("DeriveDataNeeds().AccountLabel = false, want true")
	}
	if needs.Avatar || needs.Badges {
		t.Errorf("DeriveDataNeeds() = %+v, want only AccountLabel true", needs)
	}
}

func TestDeriveDataNeedsIgnoresUnrelatedTextBindings(t *testing.T) {
	doc := chatDesignWithBinding(visualdesign.BindingUsername)
	needs := DeriveDataNeeds(doc)
	if needs.Avatar || needs.Badges || needs.AccountLabel {
		t.Errorf("DeriveDataNeeds() for a username binding = %+v, want all false", needs)
	}
}
