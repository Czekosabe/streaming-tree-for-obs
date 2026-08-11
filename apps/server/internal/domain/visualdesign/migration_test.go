package visualdesign

import "testing"

func TestMigrateToCurrentVersionUpgradesVersion1(t *testing.T) {
	doc := Document{
		Version: Version1,
		Canvas:  CanvasLandscape,
		Layers: []Layer{{
			ID: "layer_1", Name: "Text", Kind: LayerText, Visible: true, Order: 0,
			Frame: Frame{X: 0, Y: 0, Width: 400, Height: 100}, Opacity: 1,
			Text: &TextProps{
				Binding: BindingAlertRenderedText, MissingValueBehavior: MissingHide,
				FontFamily: FontSystemUI, FontSize: 32, FontWeight: 700, LineHeight: 1.2,
				TextColor: "#FFFFFF", HorizontalAlign: HAlignCenter, VerticalAlign: VAlignMiddle,
			},
			EntryAnimation: AnimationFade, ExitAnimation: AnimationFade, AnimationDurationMS: 300,
		}},
	}

	migrated := MigrateToCurrentVersion(doc)

	if migrated.Version != CurrentVersion {
		t.Errorf("Version = %d, want CurrentVersion (%d)", migrated.Version, CurrentVersion)
	}
	if len(migrated.Layers) != 1 || migrated.Layers[0].ID != "layer_1" {
		t.Fatalf("Layers = %+v, want the one original layer unchanged", migrated.Layers)
	}
	if migrated.Layers[0].Text.Binding != BindingAlertRenderedText || migrated.Layers[0].Text.FontSize != 32 {
		t.Errorf("Text = %+v, want unchanged binding/fontSize", migrated.Layers[0].Text)
	}
	if migrated.Canvas != CanvasLandscape {
		t.Errorf("Canvas = %+v, want unchanged CanvasLandscape", migrated.Canvas)
	}
	if err := Validate(migrated); err != nil {
		t.Errorf("migrated document fails Validate: %v", err)
	}
}

func TestMigrateToCurrentVersionIsANoOpAtCurrentVersion(t *testing.T) {
	doc := validDoc()
	migrated := MigrateToCurrentVersion(doc)
	if migrated.Version != doc.Version {
		t.Errorf("Version changed from %d to %d for an already-current document", doc.Version, migrated.Version)
	}
}

func TestMigrateToCurrentVersionIsIdempotent(t *testing.T) {
	doc := Document{Version: Version1, Canvas: CanvasLandscape}
	once := MigrateToCurrentVersion(doc)
	twice := MigrateToCurrentVersion(once)
	if once.Version != twice.Version {
		t.Errorf("migrating twice changed the version again: %d -> %d", once.Version, twice.Version)
	}
}
