package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/visualdesign"
	"github.com/streaming-tree/server/internal/domain/visualtemplate"
)

func testTemplateDoc() visualdesign.Document {
	return visualdesign.Document{
		Version: visualdesign.CurrentVersion,
		Canvas:  visualdesign.CanvasChatItem,
		Layers: []visualdesign.Layer{
			{
				ID: "layer_1", Name: "Text", Kind: visualdesign.LayerText,
				Visible: true, Locked: false, Order: 0,
				Frame: visualdesign.Frame{X: 10, Y: 10, Width: 400, Height: 100}, Opacity: 1,
				Text: &visualdesign.TextProps{
					Binding: visualdesign.BindingUsername, MissingValueBehavior: visualdesign.MissingHide,
					FontFamily: visualdesign.FontSystemUI, FontSize: 32, FontWeight: 700, LineHeight: 1.2,
					TextColor: "#FFFFFF", HorizontalAlign: visualdesign.HAlignCenter, VerticalAlign: visualdesign.VAlignMiddle,
					OutlineColor: "#000000", ShadowColor: "#000000",
				},
				EntryAnimation: visualdesign.AnimationFade, ExitAnimation: visualdesign.AnimationFade, AnimationDurationMS: 300,
			},
		},
	}
}

func testTemplate(id string) visualtemplate.Template {
	now := time.Now().UTC()
	return visualtemplate.Template{
		ID: id, Target: visualtemplate.TargetChat, Source: visualtemplate.SourceUser,
		Name: "My Template", Description: "desc", Author: "me", License: "MIT",
		TemplateSchemaVersion: visualtemplate.CurrentTemplateSchemaVersion,
		Document:              testTemplateDoc(),
		CreatedAt:             now, UpdatedAt: now,
	}
}

func TestVisualTemplateRepositoryGetMissingReturnsErrNotFound(t *testing.T) {
	db := newTestDB(t)
	repo := NewVisualTemplateRepository(db.DB)

	_, err := repo.Get(context.Background(), "tpl_missing")
	if !errors.Is(err, visualtemplate.ErrNotFound) {
		t.Fatalf("Get() error = %v, want ErrNotFound", err)
	}
}

func TestVisualTemplateRepositoryCreateAndGet(t *testing.T) {
	db := newTestDB(t)
	repo := NewVisualTemplateRepository(db.DB)

	created, err := repo.Create(context.Background(), testTemplate("tpl_1"))
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.ID != "tpl_1" || created.Name != "My Template" {
		t.Errorf("unexpected created template: %+v", created)
	}

	got, err := repo.Get(context.Background(), "tpl_1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Document.Version != visualdesign.CurrentVersion {
		t.Errorf("Document.Version = %d, want %d", got.Document.Version, visualdesign.CurrentVersion)
	}
	if len(got.Document.Layers) != 1 || got.Document.Layers[0].ID != "layer_1" {
		t.Errorf("document round-trip mismatch: %+v", got.Document)
	}
}

func TestVisualTemplateRepositoryList(t *testing.T) {
	db := newTestDB(t)
	repo := NewVisualTemplateRepository(db.DB)
	ctx := context.Background()

	if _, err := repo.Create(ctx, testTemplate("tpl_a")); err != nil {
		t.Fatalf("Create(a) error = %v", err)
	}
	if _, err := repo.Create(ctx, testTemplate("tpl_b")); err != nil {
		t.Fatalf("Create(b) error = %v", err)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("List() len = %d, want 2", len(list))
	}
}

func TestVisualTemplateRepositoryUpdateMetadata(t *testing.T) {
	db := newTestDB(t)
	repo := NewVisualTemplateRepository(db.DB)
	ctx := context.Background()

	if _, err := repo.Create(ctx, testTemplate("tpl_1")); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	updated, err := repo.UpdateMetadata(ctx, "tpl_1", "New Name", "New Desc", "New Author", "New License")
	if err != nil {
		t.Fatalf("UpdateMetadata() error = %v", err)
	}
	if updated.Name != "New Name" || updated.Description != "New Desc" || updated.Author != "New Author" || updated.License != "New License" {
		t.Errorf("unexpected updated metadata: %+v", updated)
	}
	if len(updated.Document.Layers) != 1 {
		t.Errorf("UpdateMetadata must not touch the document, got %+v", updated.Document)
	}
}

func TestVisualTemplateRepositoryUpdateMetadataMissingReturnsErrNotFound(t *testing.T) {
	db := newTestDB(t)
	repo := NewVisualTemplateRepository(db.DB)

	_, err := repo.UpdateMetadata(context.Background(), "tpl_missing", "n", "d", "a", "l")
	if !errors.Is(err, visualtemplate.ErrNotFound) {
		t.Fatalf("UpdateMetadata() error = %v, want ErrNotFound", err)
	}
}

func TestVisualTemplateRepositoryDeleteIsIdempotent(t *testing.T) {
	db := newTestDB(t)
	repo := NewVisualTemplateRepository(db.DB)
	ctx := context.Background()

	if _, err := repo.Create(ctx, testTemplate("tpl_1")); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := repo.Delete(ctx, "tpl_1"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if err := repo.Delete(ctx, "tpl_1"); err != nil {
		t.Fatalf("second Delete() error = %v, want nil (idempotent)", err)
	}
	if _, err := repo.Get(ctx, "tpl_1"); !errors.Is(err, visualtemplate.ErrNotFound) {
		t.Fatalf("Get() after delete error = %v, want ErrNotFound", err)
	}
}

func TestVisualTemplateRepositoryNoAudioScansAsNil(t *testing.T) {
	db := newTestDB(t)
	repo := NewVisualTemplateRepository(db.DB)
	ctx := context.Background()

	if _, err := repo.Create(ctx, testTemplate("tpl_1")); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	got, err := repo.Get(ctx, "tpl_1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.AlertAudio != nil {
		t.Errorf("AlertAudio = %+v, want nil for a template with no preset", got.AlertAudio)
	}
}

func TestVisualTemplateRepositoryAlertAudioRoundTrips(t *testing.T) {
	db := newTestDB(t)
	repo := NewVisualTemplateRepository(db.DB)
	ctx := context.Background()

	tpl := testTemplate("tpl_audio")
	tpl.AlertAudio = &visualtemplate.RuleAudioPreset{
		SoundEnabled: true, SoundAssetID: "audioasset_1", SoundVolume: 0.75,
		TTSEnabled: true, TTSTemplate: "{username} says hi", TTSVolume: 0.4,
	}
	if _, err := repo.Create(ctx, tpl); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	got, err := repo.Get(ctx, "tpl_audio")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.AlertAudio == nil {
		t.Fatal("AlertAudio = nil, want the persisted preset")
	}
	if !got.AlertAudio.SoundEnabled || got.AlertAudio.SoundAssetID != "audioasset_1" || got.AlertAudio.SoundVolume != 0.75 {
		t.Errorf("sound fields = %+v, want round-tripped verbatim", got.AlertAudio)
	}
	if !got.AlertAudio.TTSEnabled || got.AlertAudio.TTSTemplate != "{username} says hi" || got.AlertAudio.TTSVolume != 0.4 {
		t.Errorf("TTS fields = %+v, want round-tripped verbatim", got.AlertAudio)
	}
}

func TestVisualTemplateRepositoryMigratesStoredVersion1Document(t *testing.T) {
	db := newTestDB(t)
	repo := NewVisualTemplateRepository(db.DB)
	ctx := context.Background()

	tpl := testTemplate("tpl_v1")
	tpl.Document.Version = visualdesign.Version1
	raw, err := visualdesign.MarshalDocumentJSON(tpl.Document)
	if err != nil {
		t.Fatalf("MarshalDocumentJSON() error = %v", err)
	}
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	if _, err := db.DB.ExecContext(ctx, `
		INSERT INTO visual_templates (id, target_kind, name, description, author, license, template_schema_version, document_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"tpl_v1", "chat", "Old", "d", "a", "l", 1, string(raw), now, now,
	); err != nil {
		t.Fatalf("raw insert error = %v", err)
	}

	got, err := repo.Get(ctx, "tpl_v1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Document.Version != visualdesign.CurrentVersion {
		t.Errorf("Document.Version = %d, want migrated to %d", got.Document.Version, visualdesign.CurrentVersion)
	}
	if len(got.Document.Layers) != 1 || got.Document.Layers[0].ID != "layer_1" {
		t.Errorf("migrated document content mismatch: %+v", got.Document)
	}
}
