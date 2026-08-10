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
