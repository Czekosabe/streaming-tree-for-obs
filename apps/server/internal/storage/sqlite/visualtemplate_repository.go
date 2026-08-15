package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/domain/visualdesign"
	"github.com/streaming-tree/server/internal/domain/visualtemplate"
)

// VisualTemplateRepository is the SQLite implementation of
// visualtemplate.Repository - USER templates only (migration
// 0017_visual_templates.sql). Built-in templates never pass through
// here.
type VisualTemplateRepository struct {
	db *sql.DB
}

// NewVisualTemplateRepository builds a repository over an open database.
func NewVisualTemplateRepository(db *sql.DB) *VisualTemplateRepository {
	return &VisualTemplateRepository{db: db}
}

var _ visualtemplate.Repository = (*VisualTemplateRepository)(nil)

func visualTemplateStorageErr(op string, err error) error {
	return fmt.Errorf("%w: %s: %v", visualtemplate.ErrStorage, op, err)
}

const visualTemplateColumns = `id, target_kind, name, description, author, license, template_schema_version, document_json, created_at, updated_at,
	audio_sound_enabled, audio_sound_asset_id, audio_sound_volume, audio_tts_enabled, audio_tts_template, audio_tts_volume`

func scanVisualTemplate(scanner interface{ Scan(...any) error }) (visualtemplate.Template, error) {
	var (
		t                    visualtemplate.Template
		targetKind           string
		documentJSON         string
		createdAt, updatedAt string
		soundEnabled         bool
		soundAssetID         string
		soundVolume          float64
		ttsEnabled           bool
		ttsTemplate          string
		ttsVolume            float64
	)
	if err := scanner.Scan(&t.ID, &targetKind, &t.Name, &t.Description, &t.Author, &t.License, &t.TemplateSchemaVersion, &documentJSON, &createdAt, &updatedAt,
		&soundEnabled, &soundAssetID, &soundVolume, &ttsEnabled, &ttsTemplate, &ttsVolume); err != nil {
		return visualtemplate.Template{}, err
	}
	t.Target = visualtemplate.Target(targetKind)
	t.Source = visualtemplate.SourceUser

	doc, err := visualdesign.UnmarshalDocumentJSON([]byte(documentJSON))
	if err != nil {
		return visualtemplate.Template{}, fmt.Errorf("%w: parse stored document_json: %v", visualtemplate.ErrStorage, err)
	}
	t.Document = visualdesign.MigrateToCurrentVersion(doc)

	if t.CreatedAt, err = platform.ParseTimestamp(createdAt); err != nil {
		return visualtemplate.Template{}, fmt.Errorf("parse created_at %q: %w", createdAt, err)
	}
	if t.UpdatedAt, err = platform.ParseTimestamp(updatedAt); err != nil {
		return visualtemplate.Template{}, fmt.Errorf("parse updated_at %q: %w", updatedAt, err)
	}

	// audio_sound_enabled/audio_tts_enabled both false and their string
	// fields empty is the safe "no preset" zero value every pre-Stage-
	// 17B row (and every template a plain JSON create/import ever
	// produces) migrates to - AlertAudio stays nil rather than a
	// populated-but-all-zero struct, mirroring Instance.Audio's own
	// "nil means no rule-owned audio configured" convention exactly.
	if soundEnabled || soundAssetID != "" || soundVolume != 0 || ttsEnabled || ttsTemplate != "" || ttsVolume != 0 {
		t.AlertAudio = &visualtemplate.RuleAudioPreset{
			SoundEnabled: soundEnabled, SoundAssetID: soundAssetID, SoundVolume: soundVolume,
			TTSEnabled: ttsEnabled, TTSTemplate: ttsTemplate, TTSVolume: ttsVolume,
		}
	}
	return t, nil
}

// Create inserts t as a new row.
func (r *VisualTemplateRepository) Create(ctx context.Context, t visualtemplate.Template) (visualtemplate.Template, error) {
	raw, err := visualdesign.MarshalDocumentJSON(t.Document)
	if err != nil {
		return visualtemplate.Template{}, visualTemplateStorageErr("marshal document", err)
	}
	nowText := platform.FormatTimestamp(t.CreatedAt)
	updatedText := platform.FormatTimestamp(t.UpdatedAt)
	var soundEnabled, ttsEnabled bool
	var soundAssetID, ttsTemplate string
	var soundVolume, ttsVolume float64
	if t.AlertAudio != nil {
		soundEnabled, soundAssetID, soundVolume = t.AlertAudio.SoundEnabled, t.AlertAudio.SoundAssetID, t.AlertAudio.SoundVolume
		ttsEnabled, ttsTemplate, ttsVolume = t.AlertAudio.TTSEnabled, t.AlertAudio.TTSTemplate, t.AlertAudio.TTSVolume
	}
	if _, err := r.db.ExecContext(ctx, `
		INSERT INTO visual_templates (id, target_kind, name, description, author, license, template_schema_version, document_json, created_at, updated_at,
			audio_sound_enabled, audio_sound_asset_id, audio_sound_volume, audio_tts_enabled, audio_tts_template, audio_tts_volume)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, string(t.Target), t.Name, t.Description, t.Author, t.License, t.TemplateSchemaVersion, string(raw), nowText, updatedText,
		soundEnabled, soundAssetID, soundVolume, ttsEnabled, ttsTemplate, ttsVolume,
	); err != nil {
		return visualtemplate.Template{}, visualTemplateStorageErr("create visual template", err)
	}
	return r.Get(ctx, t.ID)
}

// Get returns the user template with id.
func (r *VisualTemplateRepository) Get(ctx context.Context, id string) (visualtemplate.Template, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+visualTemplateColumns+` FROM visual_templates WHERE id = ?`, id)
	t, err := scanVisualTemplate(row)
	if errors.Is(err, sql.ErrNoRows) {
		return visualtemplate.Template{}, visualtemplate.ErrNotFound
	}
	if err != nil {
		return visualtemplate.Template{}, visualTemplateStorageErr("get visual template", err)
	}
	return t, nil
}

// List returns every user template, newest first.
func (r *VisualTemplateRepository) List(ctx context.Context) ([]visualtemplate.Template, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+visualTemplateColumns+` FROM visual_templates ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, visualTemplateStorageErr("list visual templates", err)
	}
	defer rows.Close()

	var out []visualtemplate.Template
	for rows.Next() {
		t, err := scanVisualTemplate(rows)
		if err != nil {
			return nil, visualTemplateStorageErr("list visual templates", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, visualTemplateStorageErr("list visual templates", err)
	}
	return out, nil
}

// UpdateMetadata replaces name/description/author/license for id.
func (r *VisualTemplateRepository) UpdateMetadata(ctx context.Context, id, name, description, author, license string) (visualtemplate.Template, error) {
	now := platform.FormatTimestamp(time.Now().UTC())
	res, err := r.db.ExecContext(ctx, `
		UPDATE visual_templates SET name = ?, description = ?, author = ?, license = ?, updated_at = ?
		WHERE id = ?`,
		name, description, author, license, now, id,
	)
	if err != nil {
		return visualtemplate.Template{}, visualTemplateStorageErr("update visual template metadata", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return visualtemplate.Template{}, visualTemplateStorageErr("update visual template metadata", err)
	}
	if affected == 0 {
		return visualtemplate.Template{}, visualtemplate.ErrNotFound
	}
	return r.Get(ctx, id)
}

// Delete removes id if it exists; idempotent.
func (r *VisualTemplateRepository) Delete(ctx context.Context, id string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM visual_templates WHERE id = ?`, id); err != nil {
		return visualTemplateStorageErr("delete visual template", err)
	}
	return nil
}
