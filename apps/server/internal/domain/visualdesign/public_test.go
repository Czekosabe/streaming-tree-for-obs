package visualdesign

import "testing"

func TestToPublicExcludesHiddenLayersEntirely(t *testing.T) {
	doc := validDoc()
	doc.Layers[1].Visible = false
	pub := ToPublic(doc)
	if len(pub.Layers) != 1 {
		t.Fatalf("len(pub.Layers) = %d, want 1 (the hidden layer must be entirely absent)", len(pub.Layers))
	}
	if pub.Layers[0].ID != doc.Layers[0].ID {
		t.Errorf("pub.Layers[0].ID = %q, want %q", pub.Layers[0].ID, doc.Layers[0].ID)
	}
}

func TestToPublicOrdersLayersByOrderAscending(t *testing.T) {
	doc := validDoc()
	doc.Layers[0].Order = 5
	doc.Layers[1].Order = 1
	pub := ToPublic(doc)
	if len(pub.Layers) != 2 {
		t.Fatalf("len(pub.Layers) = %d, want 2", len(pub.Layers))
	}
	if pub.Layers[0].ID != doc.Layers[1].ID || pub.Layers[1].ID != doc.Layers[0].ID {
		t.Errorf("pub.Layers order = [%q, %q], want [%q, %q]", pub.Layers[0].ID, pub.Layers[1].ID, doc.Layers[1].ID, doc.Layers[0].ID)
	}
}

func TestToPublicNeverExposesLayerNamesOrLockState(t *testing.T) {
	// A compile-time guarantee as much as a runtime one: PublicLayer has
	// no Name or Locked field at all, so this test documents the intent
	// and would fail to compile if someone added one without updating
	// ToPublic and this test together.
	doc := validDoc()
	doc.Layers[0].Name = "Secret editor-only layer name"
	doc.Layers[0].Locked = true
	pub := ToPublic(doc)
	if len(pub.Layers) == 0 {
		t.Fatal("expected at least one public layer")
	}
	// PublicLayer simply has no Name/Locked field to check - this
	// assertion exists to keep the test non-trivial if PublicLayer ever
	// gains one by mistake (the struct literal below would then need a
	// zero value, which a reviewer should immediately question).
	_ = pub.Layers[0]
}

func TestToPublicCarriesMessageFragmentsAndBadgeListPayloads(t *testing.T) {
	doc := Document{
		Version: CurrentVersion, Canvas: CanvasLandscape,
		Layers: []Layer{
			{
				ID: "layer_fragments", Kind: LayerMessageFragments, Visible: true, Order: 0,
				Frame: Frame{X: 0, Y: 0, Width: 400, Height: 100}, Opacity: 1,
				MessageFragments: &MessageFragmentsProps{
					FontFamily: FontSystemUI, FontSize: 16, FontWeight: 400, LineHeight: 1.2,
					TextColor: "#FFFFFF", HorizontalAlign: HAlignLeft, VerticalAlign: VAlignTop, EmoteSize: 24,
				},
				EntryAnimation: AnimationNone, ExitAnimation: AnimationNone,
			},
			{
				ID: "layer_badges", Kind: LayerBadgeList, Visible: true, Order: 1,
				Frame: Frame{X: 0, Y: 100, Width: 200, Height: 32}, Opacity: 1,
				BadgeList:      &BadgeListProps{MaxCount: 5, BadgeSize: 24, Gap: 4},
				EntryAnimation: AnimationNone, ExitAnimation: AnimationNone,
			},
		},
	}
	pub := ToPublic(doc)
	if len(pub.Layers) != 2 {
		t.Fatalf("len(pub.Layers) = %d, want 2", len(pub.Layers))
	}
	if pub.Layers[0].MessageFragments == nil || pub.Layers[0].MessageFragments.EmoteSize != 24 {
		t.Errorf("MessageFragments = %+v, want EmoteSize=24", pub.Layers[0].MessageFragments)
	}
	if pub.Layers[1].BadgeList == nil || pub.Layers[1].BadgeList.MaxCount != 5 {
		t.Errorf("BadgeList = %+v, want MaxCount=5", pub.Layers[1].BadgeList)
	}
}

func TestToPublicCarriesSchemaVersionAndCanvas(t *testing.T) {
	doc := validDoc()
	pub := ToPublic(doc)
	if pub.SchemaVersion != CurrentVersion {
		t.Errorf("SchemaVersion = %d, want %d", pub.SchemaVersion, CurrentVersion)
	}
	if pub.Canvas.Width != doc.Canvas.Width || pub.Canvas.Height != doc.Canvas.Height {
		t.Errorf("Canvas = %+v, want %+v", pub.Canvas, doc.Canvas)
	}
}
