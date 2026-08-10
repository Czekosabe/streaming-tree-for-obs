package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/streaming-tree/server/internal/domain/visualdesign"
)

func testDesignDoc() visualdesign.Document {
	return visualdesign.Document{
		Version: visualdesign.CurrentVersion,
		Canvas:  visualdesign.CanvasLandscape,
		Layers: []visualdesign.Layer{
			{
				ID: "layer_1", Name: "Text", Kind: visualdesign.LayerText,
				Visible: true, Locked: false, Order: 0,
				Frame: visualdesign.Frame{X: 10, Y: 10, Width: 400, Height: 100}, Opacity: 1,
				Text: &visualdesign.TextProps{
					Binding: visualdesign.BindingAlertRenderedText, MissingValueBehavior: visualdesign.MissingHide,
					FontFamily: visualdesign.FontSystemUI, FontSize: 32, FontWeight: 700, LineHeight: 1.2,
					TextColor: "#FFFFFF", HorizontalAlign: visualdesign.HAlignCenter, VerticalAlign: visualdesign.VAlignMiddle,
					OutlineColor: "#000000", ShadowColor: "#000000",
				},
				EntryAnimation: visualdesign.AnimationFade, ExitAnimation: visualdesign.AnimationFade, AnimationDurationMS: 300,
			},
		},
	}
}

func fixedTestID() (string, error) { return "design_fixed_1", nil }

func TestVisualDesignRepositoryGetMissingReturnsNotFound(t *testing.T) {
	db := newTestDB(t)
	repo := NewVisualDesignRepository(db.DB)

	_, found, err := repo.Get(context.Background(), visualdesign.OwnerKindAlertRule, "alrule_missing")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if found {
		t.Fatal("Get() found = true, want false")
	}
}

func TestVisualDesignRepositorySaveThenGetRoundTrips(t *testing.T) {
	db := newTestDB(t)
	repo := NewVisualDesignRepository(db.DB)
	ctx := context.Background()

	saved, err := repo.Save(ctx, visualdesign.OwnerKindAlertRule, "alrule_1", testDesignDoc(), 0, fixedTestID)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if saved.Revision != 1 {
		t.Errorf("Revision = %d, want 1", saved.Revision)
	}
	if saved.ID != "design_fixed_1" {
		t.Errorf("ID = %q, want design_fixed_1", saved.ID)
	}

	got, found, err := repo.Get(ctx, visualdesign.OwnerKindAlertRule, "alrule_1")
	if err != nil || !found {
		t.Fatalf("Get() found = %v, err = %v", found, err)
	}
	if len(got.Document.Layers) != 1 || got.Document.Layers[0].ID != "layer_1" {
		t.Fatalf("Get().Document.Layers = %+v, want the one saved layer", got.Document.Layers)
	}
	if got.Document.Layers[0].Text.Binding != visualdesign.BindingAlertRenderedText {
		t.Errorf("round-tripped text binding = %q, want alert_rendered_text", got.Document.Layers[0].Text.Binding)
	}
	if got.Document.Canvas.Width != 1920 || got.Document.Canvas.Height != 1080 {
		t.Errorf("round-tripped canvas = %+v, want 1920x1080", got.Document.Canvas)
	}
}

func TestVisualDesignRepositorySaveIncrementsRevisionOnReplace(t *testing.T) {
	db := newTestDB(t)
	repo := NewVisualDesignRepository(db.DB)
	ctx := context.Background()

	first, err := repo.Save(ctx, visualdesign.OwnerKindAlertRule, "alrule_1", testDesignDoc(), 0, fixedTestID)
	if err != nil {
		t.Fatalf("first Save() error = %v", err)
	}
	second, err := repo.Save(ctx, visualdesign.OwnerKindAlertRule, "alrule_1", testDesignDoc(), first.Revision, fixedTestID)
	if err != nil {
		t.Fatalf("second Save() error = %v", err)
	}
	if second.Revision != 2 {
		t.Errorf("second.Revision = %d, want 2", second.Revision)
	}
	if second.ID != first.ID {
		t.Errorf("second.ID = %q, want unchanged %q (a replace, not a new row)", second.ID, first.ID)
	}
}

func TestVisualDesignRepositorySaveReturnsConflictOnStaleRevision(t *testing.T) {
	db := newTestDB(t)
	repo := NewVisualDesignRepository(db.DB)
	ctx := context.Background()

	if _, err := repo.Save(ctx, visualdesign.OwnerKindAlertRule, "alrule_1", testDesignDoc(), 0, fixedTestID); err != nil {
		t.Fatalf("first Save() error = %v", err)
	}
	_, err := repo.Save(ctx, visualdesign.OwnerKindAlertRule, "alrule_1", testDesignDoc(), 0, fixedTestID)
	if !errors.Is(err, visualdesign.ErrRevisionConflict) {
		t.Fatalf("Save() with stale expectedRevision=0 error = %v, want ErrRevisionConflict", err)
	}
}

func TestVisualDesignRepositoryOwnerUniqueness(t *testing.T) {
	db := newTestDB(t)
	repo := NewVisualDesignRepository(db.DB)
	ctx := context.Background()

	if _, err := repo.Save(ctx, visualdesign.OwnerKindAlertRule, "alrule_1", testDesignDoc(), 0, fixedTestID); err != nil {
		t.Fatalf("Save() alrule_1 error = %v", err)
	}
	_, found, err := repo.Get(ctx, visualdesign.OwnerKindAlertRule, "alrule_2")
	if err != nil {
		t.Fatalf("Get() alrule_2 error = %v", err)
	}
	if found {
		t.Fatal("Get() alrule_2 found = true, want false - each owner has its own independent design")
	}
}

func TestVisualDesignRepositoryDeleteIsIdempotent(t *testing.T) {
	db := newTestDB(t)
	repo := NewVisualDesignRepository(db.DB)
	ctx := context.Background()

	if err := repo.Delete(ctx, visualdesign.OwnerKindAlertRule, "alrule_never_saved"); err != nil {
		t.Fatalf("Delete() on an unsaved owner error = %v, want nil", err)
	}

	if _, err := repo.Save(ctx, visualdesign.OwnerKindAlertRule, "alrule_1", testDesignDoc(), 0, fixedTestID); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := repo.Delete(ctx, visualdesign.OwnerKindAlertRule, "alrule_1"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if err := repo.Delete(ctx, visualdesign.OwnerKindAlertRule, "alrule_1"); err != nil {
		t.Fatalf("second Delete() error = %v, want nil (idempotent)", err)
	}
	_, found, _ := repo.Get(ctx, visualdesign.OwnerKindAlertRule, "alrule_1")
	if found {
		t.Fatal("Get() found = true after Delete, want false")
	}
}

func TestVisualDesignRepositorySurvivesRestart(t *testing.T) {
	db := newTestDB(t)
	repo := NewVisualDesignRepository(db.DB)
	ctx := context.Background()

	if _, err := repo.Save(ctx, visualdesign.OwnerKindAlertRule, "alrule_1", testDesignDoc(), 0, fixedTestID); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	path := db.Path()
	if err := db.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	repo2 := NewVisualDesignRepository(reopened.DB)
	got, found, err := repo2.Get(ctx, visualdesign.OwnerKindAlertRule, "alrule_1")
	if err != nil || !found {
		t.Fatalf("Get() after restart found = %v, err = %v", found, err)
	}
	if got.Revision != 1 {
		t.Errorf("Revision after restart = %d, want 1", got.Revision)
	}
}
