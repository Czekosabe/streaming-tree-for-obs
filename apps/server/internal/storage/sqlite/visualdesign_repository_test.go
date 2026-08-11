package sqlite

import (
	"context"
	"errors"
	"fmt"
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

// TestVisualDesignRepositoryAcceptsChatOverlayOwnerKind proves migration
// 0016's widened CHECK constraint actually works end to end, and that
// the same owner_id under a different owner_kind is a genuinely
// independent row (Stage 13B, docs/visual-designs.md §18).
func TestVisualDesignRepositoryAcceptsChatOverlayOwnerKind(t *testing.T) {
	db := newTestDB(t)
	repo := NewVisualDesignRepository(db.DB)
	ctx := context.Background()

	saved, err := repo.Save(ctx, visualdesign.OwnerKindChatOverlay, "co_1", testDesignDoc(), 0, fixedTestID)
	if err != nil {
		t.Fatalf("Save() with owner_kind=chat_overlay error = %v", err)
	}
	if saved.OwnerKind != visualdesign.OwnerKindChatOverlay {
		t.Errorf("OwnerKind = %q, want chat_overlay", saved.OwnerKind)
	}

	got, found, err := repo.Get(ctx, visualdesign.OwnerKindChatOverlay, "co_1")
	if err != nil || !found {
		t.Fatalf("Get() found = %v, err = %v", found, err)
	}
	if got.Document.Layers[0].ID != "layer_1" {
		t.Errorf("round-tripped chat_overlay design layers = %+v", got.Document.Layers)
	}
}

func TestVisualDesignRepositoryRejectsUnknownOwnerKind(t *testing.T) {
	db := newTestDB(t)
	repo := NewVisualDesignRepository(db.DB)
	ctx := context.Background()

	_, err := repo.Save(ctx, visualdesign.OwnerKind("widget"), "widget_1", testDesignDoc(), 0, fixedTestID)
	if err == nil {
		t.Fatal("Save() with an unknown owner_kind succeeded, want the database CHECK constraint to reject it")
	}
}

// TestVisualDesignRepositoryOwnerKindIsIndependentPerKind proves the
// same owner_id string under two different owner_kind values is two
// genuinely independent designs, never accidentally aliased by
// UNIQUE(owner_kind, owner_id) (Stage 13B).
func TestVisualDesignRepositoryOwnerKindIsIndependentPerKind(t *testing.T) {
	db := newTestDB(t)
	repo := NewVisualDesignRepository(db.DB)
	ctx := context.Background()

	nextID := func() func() (string, error) {
		n := 0
		return func() (string, error) {
			n++
			return fmt.Sprintf("design_shared_id_fixture_%d", n), nil
		}
	}()

	if _, err := repo.Save(ctx, visualdesign.OwnerKindAlertRule, "shared_id", testDesignDoc(), 0, nextID); err != nil {
		t.Fatalf("Save() alert_rule error = %v", err)
	}
	if _, err := repo.Save(ctx, visualdesign.OwnerKindChatOverlay, "shared_id", testDesignDoc(), 0, nextID); err != nil {
		t.Fatalf("Save() chat_overlay error = %v", err)
	}

	alertRec, found, err := repo.Get(ctx, visualdesign.OwnerKindAlertRule, "shared_id")
	if err != nil || !found {
		t.Fatalf("Get() alert_rule found = %v, err = %v", found, err)
	}
	chatRec, found, err := repo.Get(ctx, visualdesign.OwnerKindChatOverlay, "shared_id")
	if err != nil || !found {
		t.Fatalf("Get() chat_overlay found = %v, err = %v", found, err)
	}
	if alertRec.ID == chatRec.ID {
		t.Errorf("alert_rule and chat_overlay designs share the same row id %q for the same owner_id - they must be independent", alertRec.ID)
	}

	if err := repo.Delete(ctx, visualdesign.OwnerKindAlertRule, "shared_id"); err != nil {
		t.Fatalf("Delete() alert_rule error = %v", err)
	}
	_, stillFound, err := repo.Get(ctx, visualdesign.OwnerKindChatOverlay, "shared_id")
	if err != nil || !stillFound {
		t.Fatalf("Get() chat_overlay after deleting the alert_rule sibling found = %v, err = %v - deleting one owner's design must never affect the other's", stillFound, err)
	}
}

// TestVisualDesignRepositoryMigratesStoredVersion1Document proves the
// Stage 13A -> Stage 13B migration is genuinely lossless: a row written
// directly at schema_version=1 (bypassing the Go API, exactly as a
// real pre-Stage-13B row on disk would be) is transparently upgraded to
// CurrentVersion on read, with every layer/style value preserved
// exactly (docs/visual-designs.md §19).
func TestVisualDesignRepositoryMigratesStoredVersion1Document(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	const v1JSON = `{
		"version": 1,
		"canvas": {"width": 1920, "height": 1080, "transparent": true},
		"layers": [{
			"id": "layer_1", "name": "Text", "kind": "text", "visible": true, "locked": false, "order": 0,
			"frame": {"x": 10, "y": 10, "width": 400, "height": 100}, "opacity": 1,
			"text": {
				"binding": "alert_rendered_text", "missingValueBehavior": "hide",
				"fontFamily": "system-ui", "fontSize": 32, "fontWeight": 700, "lineHeight": 1.2, "letterSpacing": 0,
				"textColor": "#FFFFFF", "horizontalAlign": "center", "verticalAlign": "middle",
				"outlineWidth": 0, "outlineColor": "#000000",
				"shadowEnabled": false, "shadowOffsetX": 0, "shadowOffsetY": 0, "shadowBlur": 0, "shadowColor": "#000000"
			},
			"entryAnimation": "fade", "exitAnimation": "fade", "animationDurationMs": 300
		}]
	}`

	if _, err := db.DB.ExecContext(ctx, `
		INSERT INTO visual_designs (id, owner_kind, owner_id, schema_version, document_json, revision, created_at, updated_at)
		VALUES ('design_v1_fixture', 'alert_rule', 'alrule_legacy', 1, ?, 1, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
		v1JSON,
	); err != nil {
		t.Fatalf("insert raw version-1 fixture row: %v", err)
	}

	repo := NewVisualDesignRepository(db.DB)
	got, found, err := repo.Get(ctx, visualdesign.OwnerKindAlertRule, "alrule_legacy")
	if err != nil || !found {
		t.Fatalf("Get() found = %v, err = %v", found, err)
	}

	if got.Document.Version != visualdesign.CurrentVersion {
		t.Errorf("Document.Version after migration = %d, want CurrentVersion (%d)", got.Document.Version, visualdesign.CurrentVersion)
	}
	if got.ID != "design_v1_fixture" {
		t.Errorf("ID = %q, want unchanged design_v1_fixture", got.ID)
	}
	if got.Revision != 1 {
		t.Errorf("Revision = %d, want unchanged 1", got.Revision)
	}
	if len(got.Document.Layers) != 1 {
		t.Fatalf("Layers = %+v, want exactly 1 unchanged layer", got.Document.Layers)
	}
	layer := got.Document.Layers[0]
	if layer.ID != "layer_1" || layer.Kind != visualdesign.LayerText {
		t.Errorf("migrated layer = %+v, want id=layer_1 kind=text unchanged", layer)
	}
	if layer.Text == nil || layer.Text.Binding != visualdesign.BindingAlertRenderedText || layer.Text.FontSize != 32 {
		t.Errorf("migrated text props = %+v, want unchanged binding=alert_rendered_text fontSize=32", layer.Text)
	}
	if err := visualdesign.Validate(got.Document); err != nil {
		t.Errorf("migrated document fails Validate: %v", err)
	}

	// A save after migration always persists at CurrentVersion, never
	// re-downgrading to the stored row's original version.
	resaved, err := repo.Save(ctx, visualdesign.OwnerKindAlertRule, "alrule_legacy", got.Document, got.Revision, fixedTestID)
	if err != nil {
		t.Fatalf("Save() the migrated document error = %v", err)
	}
	if resaved.Document.Version != visualdesign.CurrentVersion {
		t.Errorf("resaved Document.Version = %d, want CurrentVersion (%d)", resaved.Document.Version, visualdesign.CurrentVersion)
	}
}

// TestVisualDesignRepositoryRoundTripsImageVideoFontLayers guards
// exactly the regression a Stage 14B httpapi-level test first caught:
// this repository keeps its own private JSON mirror of Document/Layer
// (predating internal/domain/visualdesign/json.go's later shared
// mirror, see this file's own doc comment), so adding a new layer kind
// or field to the domain package does not automatically reach storage -
// each mirror must be updated by hand, and only a real round-trip
// through this repository can prove it was.
func TestVisualDesignRepositoryRoundTripsImageVideoFontLayers(t *testing.T) {
	db := newTestDB(t)
	repo := NewVisualDesignRepository(db.DB)
	ctx := context.Background()

	doc := visualdesign.Document{
		Version: visualdesign.CurrentVersion,
		Canvas:  visualdesign.CanvasLandscape,
		Layers: []visualdesign.Layer{
			{
				ID: "layer_image", Name: "Badge", Kind: visualdesign.LayerImage,
				Visible: true, Order: 0,
				Frame: visualdesign.Frame{X: 0, Y: 0, Width: 100, Height: 100}, Opacity: 1,
				Image:          &visualdesign.ImageProps{AssetID: "asset_image1", Fit: visualdesign.FitContain, Alt: "Badge"},
				EntryAnimation: visualdesign.AnimationNone, ExitAnimation: visualdesign.AnimationNone,
			},
			{
				ID: "layer_video", Name: "Clip", Kind: visualdesign.LayerVideo,
				Visible: true, Order: 1,
				Frame: visualdesign.Frame{X: 100, Y: 100, Width: 200, Height: 200}, Opacity: 1,
				Video:          &visualdesign.VideoProps{AssetID: "asset_video1", Fit: visualdesign.FitCover, Loop: true},
				EntryAnimation: visualdesign.AnimationNone, ExitAnimation: visualdesign.AnimationNone,
			},
			{
				ID: "layer_text", Name: "Custom font text", Kind: visualdesign.LayerText,
				Visible: true, Order: 2,
				Frame: visualdesign.Frame{X: 0, Y: 300, Width: 400, Height: 100}, Opacity: 1,
				Text: &visualdesign.TextProps{
					Binding: visualdesign.BindingStatic, StaticText: "hi", MissingValueBehavior: visualdesign.MissingHide,
					FontFamily: visualdesign.FontSystemUI, FontAssetID: "asset_font1", FontSize: 32, FontWeight: 700, LineHeight: 1.2,
					TextColor: "#FFFFFF", HorizontalAlign: visualdesign.HAlignCenter, VerticalAlign: visualdesign.VAlignMiddle,
					OutlineColor: "#000000", ShadowColor: "#000000",
				},
				EntryAnimation: visualdesign.AnimationNone, ExitAnimation: visualdesign.AnimationNone,
			},
		},
	}
	if err := visualdesign.Validate(doc); err != nil {
		t.Fatalf("fixture document fails Validate: %v", err)
	}

	if _, err := repo.Save(ctx, visualdesign.OwnerKindAlertRule, "alrule_assets", doc, 0, fixedTestID); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, found, err := repo.Get(ctx, visualdesign.OwnerKindAlertRule, "alrule_assets")
	if err != nil || !found {
		t.Fatalf("Get() found = %v, err = %v", found, err)
	}
	if len(got.Document.Layers) != 3 {
		t.Fatalf("Layers = %+v, want 3", got.Document.Layers)
	}
	imgLayer := got.Document.Layers[0]
	if imgLayer.Image == nil || imgLayer.Image.AssetID != "asset_image1" || imgLayer.Image.Fit != visualdesign.FitContain {
		t.Errorf("image layer round-trip = %+v, want AssetID=asset_image1 Fit=contain", imgLayer.Image)
	}
	vidLayer := got.Document.Layers[1]
	if vidLayer.Video == nil || vidLayer.Video.AssetID != "asset_video1" || !vidLayer.Video.Loop {
		t.Errorf("video layer round-trip = %+v, want AssetID=asset_video1 Loop=true", vidLayer.Video)
	}
	textLayer := got.Document.Layers[2]
	if textLayer.Text == nil || textLayer.Text.FontAssetID != "asset_font1" {
		t.Errorf("text layer FontAssetID round-trip = %+v, want asset_font1", textLayer.Text)
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
