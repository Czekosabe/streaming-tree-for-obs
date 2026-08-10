package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/domain/visualdesign"
)

// VisualDesignRepository is the SQLite implementation of
// visualdesign.Repository.
type VisualDesignRepository struct {
	db *sql.DB
}

// NewVisualDesignRepository builds a repository over an open database.
func NewVisualDesignRepository(db *sql.DB) *VisualDesignRepository {
	return &VisualDesignRepository{db: db}
}

var _ visualdesign.Repository = (*VisualDesignRepository)(nil)

func visualDesignStorageErr(op string, err error) error {
	return fmt.Errorf("%w: %s: %v", visualdesign.ErrStorage, op, err)
}

// jsonDocument is document_json's own wire shape - a plain, exhaustive
// mirror of visualdesign.Document/Layer, so encoding/json's default
// struct tags (lowerCamelCase via explicit `json:` tags) never leak Go
// field-name casing into the stored JSON and so this file is the single
// place that shape can ever drift from the typed domain struct.
type jsonDocument struct {
	Version int         `json:"version"`
	Canvas  jsonCanvas  `json:"canvas"`
	Layers  []jsonLayer `json:"layers"`
}

type jsonCanvas struct {
	Width       int  `json:"width"`
	Height      int  `json:"height"`
	Transparent bool `json:"transparent"`
}

type jsonFrame struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

type jsonShapeProps struct {
	Kind         string `json:"kind"`
	Fill         string `json:"fill"`
	BorderColor  string `json:"borderColor"`
	BorderWidth  int    `json:"borderWidth"`
	CornerRadius int    `json:"cornerRadius"`
}

type jsonTextProps struct {
	Binding              string  `json:"binding"`
	StaticText           string  `json:"staticText"`
	MissingValueBehavior string  `json:"missingValueBehavior"`
	FontFamily           string  `json:"fontFamily"`
	FontSize             int     `json:"fontSize"`
	FontWeight           int     `json:"fontWeight"`
	LineHeight           float64 `json:"lineHeight"`
	LetterSpacing        float64 `json:"letterSpacing"`
	TextColor            string  `json:"textColor"`
	HorizontalAlign      string  `json:"horizontalAlign"`
	VerticalAlign        string  `json:"verticalAlign"`
	OutlineWidth         int     `json:"outlineWidth"`
	OutlineColor         string  `json:"outlineColor"`
	ShadowEnabled        bool    `json:"shadowEnabled"`
	ShadowOffsetX        int     `json:"shadowOffsetX"`
	ShadowOffsetY        int     `json:"shadowOffsetY"`
	ShadowBlur           int     `json:"shadowBlur"`
	ShadowColor          string  `json:"shadowColor"`
}

type jsonAvatarProps struct {
	CornerRadius int    `json:"cornerRadius"`
	BorderColor  string `json:"borderColor"`
	BorderWidth  int    `json:"borderWidth"`
}

type jsonLayer struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Kind    string    `json:"kind"`
	Visible bool      `json:"visible"`
	Locked  bool      `json:"locked"`
	Order   int       `json:"order"`
	Frame   jsonFrame `json:"frame"`
	Opacity float64   `json:"opacity"`

	Shape        *jsonShapeProps  `json:"shape,omitempty"`
	Text         *jsonTextProps   `json:"text,omitempty"`
	PlatformIcon *struct{}        `json:"platformIcon,omitempty"`
	Avatar       *jsonAvatarProps `json:"avatar,omitempty"`

	EntryAnimation      string `json:"entryAnimation"`
	ExitAnimation       string `json:"exitAnimation"`
	AnimationDurationMS int    `json:"animationDurationMs"`
}

func toJSONDocument(doc visualdesign.Document) jsonDocument {
	layers := make([]jsonLayer, 0, len(doc.Layers))
	for _, l := range doc.Layers {
		jl := jsonLayer{
			ID: l.ID, Name: l.Name, Kind: string(l.Kind), Visible: l.Visible, Locked: l.Locked, Order: l.Order,
			Frame:          jsonFrame{X: l.Frame.X, Y: l.Frame.Y, Width: l.Frame.Width, Height: l.Frame.Height},
			Opacity:        l.Opacity,
			EntryAnimation: string(l.EntryAnimation), ExitAnimation: string(l.ExitAnimation), AnimationDurationMS: l.AnimationDurationMS,
		}
		if l.Shape != nil {
			jl.Shape = &jsonShapeProps{
				Kind: string(l.Shape.Kind), Fill: l.Shape.Fill, BorderColor: l.Shape.BorderColor,
				BorderWidth: l.Shape.BorderWidth, CornerRadius: l.Shape.CornerRadius,
			}
		}
		if l.Text != nil {
			jl.Text = &jsonTextProps{
				Binding: string(l.Text.Binding), StaticText: l.Text.StaticText, MissingValueBehavior: string(l.Text.MissingValueBehavior),
				FontFamily: string(l.Text.FontFamily), FontSize: l.Text.FontSize, FontWeight: l.Text.FontWeight,
				LineHeight: l.Text.LineHeight, LetterSpacing: l.Text.LetterSpacing, TextColor: l.Text.TextColor,
				HorizontalAlign: string(l.Text.HorizontalAlign), VerticalAlign: string(l.Text.VerticalAlign),
				OutlineWidth: l.Text.OutlineWidth, OutlineColor: l.Text.OutlineColor,
				ShadowEnabled: l.Text.ShadowEnabled, ShadowOffsetX: l.Text.ShadowOffsetX, ShadowOffsetY: l.Text.ShadowOffsetY,
				ShadowBlur: l.Text.ShadowBlur, ShadowColor: l.Text.ShadowColor,
			}
		}
		if l.PlatformIcon != nil {
			jl.PlatformIcon = &struct{}{}
		}
		if l.Avatar != nil {
			jl.Avatar = &jsonAvatarProps{CornerRadius: l.Avatar.CornerRadius, BorderColor: l.Avatar.BorderColor, BorderWidth: l.Avatar.BorderWidth}
		}
		layers = append(layers, jl)
	}
	return jsonDocument{
		Version: doc.Version,
		Canvas:  jsonCanvas{Width: doc.Canvas.Width, Height: doc.Canvas.Height, Transparent: doc.Canvas.Transparent},
		Layers:  layers,
	}
}

func fromJSONDocument(jd jsonDocument) visualdesign.Document {
	layers := make([]visualdesign.Layer, 0, len(jd.Layers))
	for _, jl := range jd.Layers {
		l := visualdesign.Layer{
			ID: jl.ID, Name: jl.Name, Kind: visualdesign.LayerKind(jl.Kind), Visible: jl.Visible, Locked: jl.Locked, Order: jl.Order,
			Frame:          visualdesign.Frame{X: jl.Frame.X, Y: jl.Frame.Y, Width: jl.Frame.Width, Height: jl.Frame.Height},
			Opacity:        jl.Opacity,
			EntryAnimation: visualdesign.Animation(jl.EntryAnimation), ExitAnimation: visualdesign.Animation(jl.ExitAnimation),
			AnimationDurationMS: jl.AnimationDurationMS,
		}
		if jl.Shape != nil {
			l.Shape = &visualdesign.ShapeProps{
				Kind: visualdesign.ShapeKind(jl.Shape.Kind), Fill: jl.Shape.Fill, BorderColor: jl.Shape.BorderColor,
				BorderWidth: jl.Shape.BorderWidth, CornerRadius: jl.Shape.CornerRadius,
			}
		}
		if jl.Text != nil {
			l.Text = &visualdesign.TextProps{
				Binding: visualdesign.TextBinding(jl.Text.Binding), StaticText: jl.Text.StaticText,
				MissingValueBehavior: visualdesign.MissingValueBehavior(jl.Text.MissingValueBehavior),
				FontFamily:           visualdesign.FontFamily(jl.Text.FontFamily), FontSize: jl.Text.FontSize, FontWeight: jl.Text.FontWeight,
				LineHeight: jl.Text.LineHeight, LetterSpacing: jl.Text.LetterSpacing, TextColor: jl.Text.TextColor,
				HorizontalAlign: visualdesign.HorizontalAlign(jl.Text.HorizontalAlign), VerticalAlign: visualdesign.VerticalAlign(jl.Text.VerticalAlign),
				OutlineWidth: jl.Text.OutlineWidth, OutlineColor: jl.Text.OutlineColor,
				ShadowEnabled: jl.Text.ShadowEnabled, ShadowOffsetX: jl.Text.ShadowOffsetX, ShadowOffsetY: jl.Text.ShadowOffsetY,
				ShadowBlur: jl.Text.ShadowBlur, ShadowColor: jl.Text.ShadowColor,
			}
		}
		if jl.PlatformIcon != nil {
			l.PlatformIcon = &visualdesign.PlatformIconProps{}
		}
		if jl.Avatar != nil {
			l.Avatar = &visualdesign.AvatarProps{CornerRadius: jl.Avatar.CornerRadius, BorderColor: jl.Avatar.BorderColor, BorderWidth: jl.Avatar.BorderWidth}
		}
		layers = append(layers, l)
	}
	return visualdesign.Document{
		Version: jd.Version,
		Canvas:  visualdesign.Canvas{Width: jd.Canvas.Width, Height: jd.Canvas.Height, Transparent: jd.Canvas.Transparent},
		Layers:  layers,
	}
}

func scanVisualDesign(scanner interface{ Scan(...any) error }) (visualdesign.Record, error) {
	var (
		rec                  visualdesign.Record
		ownerKind            string
		schemaVersion        int
		documentJSON         string
		createdAt, updatedAt string
	)
	if err := scanner.Scan(&rec.ID, &ownerKind, &rec.OwnerID, &schemaVersion, &documentJSON, &rec.Revision, &createdAt, &updatedAt); err != nil {
		return visualdesign.Record{}, err
	}
	rec.OwnerKind = visualdesign.OwnerKind(ownerKind)

	var jd jsonDocument
	if err := json.Unmarshal([]byte(documentJSON), &jd); err != nil {
		return visualdesign.Record{}, fmt.Errorf("%w: parse stored document_json: %v", visualdesign.ErrStorage, err)
	}
	rec.Document = fromJSONDocument(jd)

	var err error
	if rec.CreatedAt, err = platform.ParseTimestamp(createdAt); err != nil {
		return visualdesign.Record{}, fmt.Errorf("parse created_at %q: %w", createdAt, err)
	}
	if rec.UpdatedAt, err = platform.ParseTimestamp(updatedAt); err != nil {
		return visualdesign.Record{}, fmt.Errorf("parse updated_at %q: %w", updatedAt, err)
	}
	return rec, nil
}

const visualDesignColumns = `id, owner_kind, owner_id, schema_version, document_json, revision, created_at, updated_at`

// Get returns the design saved for (ownerKind, ownerID).
func (r *VisualDesignRepository) Get(ctx context.Context, ownerKind visualdesign.OwnerKind, ownerID string) (visualdesign.Record, bool, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+visualDesignColumns+` FROM visual_designs WHERE owner_kind = ? AND owner_id = ?`,
		string(ownerKind), ownerID)
	rec, err := scanVisualDesign(row)
	if errors.Is(err, sql.ErrNoRows) {
		return visualdesign.Record{}, false, nil
	}
	if err != nil {
		return visualdesign.Record{}, false, visualDesignStorageErr("get visual design", err)
	}
	return rec, true, nil
}

// Save performs the atomic optimistic-concurrency full replacement
// described on visualdesign.Repository.Save.
func (r *VisualDesignRepository) Save(ctx context.Context, ownerKind visualdesign.OwnerKind, ownerID string, doc visualdesign.Document, expectedRevision int, newID func() (string, error)) (visualdesign.Record, error) {
	raw, err := json.Marshal(toJSONDocument(doc))
	if err != nil {
		return visualdesign.Record{}, visualDesignStorageErr("marshal document", err)
	}
	if len(raw) > visualdesign.MaxDocumentBytes {
		return visualdesign.Record{}, visualdesign.ErrTooLarge
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return visualdesign.Record{}, visualDesignStorageErr("begin save", err)
	}
	defer func() { _ = tx.Rollback() }()

	var currentID string
	var currentRevision int
	row := tx.QueryRowContext(ctx, `SELECT id, revision FROM visual_designs WHERE owner_kind = ? AND owner_id = ?`, string(ownerKind), ownerID)
	scanErr := row.Scan(&currentID, &currentRevision)

	now := time.Now().UTC()
	nowText := platform.FormatTimestamp(now)

	switch {
	case errors.Is(scanErr, sql.ErrNoRows):
		if expectedRevision != 0 {
			return visualdesign.Record{}, visualdesign.ErrRevisionConflict
		}
		id, err := newID()
		if err != nil {
			return visualdesign.Record{}, visualDesignStorageErr("generate design id", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO visual_designs (id, owner_kind, owner_id, schema_version, document_json, revision, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, 1, ?, ?)`,
			id, string(ownerKind), ownerID, doc.Version, string(raw), nowText, nowText,
		); err != nil {
			return visualdesign.Record{}, visualDesignStorageErr("create visual design", err)
		}
	case scanErr != nil:
		return visualdesign.Record{}, visualDesignStorageErr("save visual design", scanErr)
	default:
		if currentRevision != expectedRevision {
			return visualdesign.Record{}, visualdesign.ErrRevisionConflict
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE visual_designs SET schema_version = ?, document_json = ?, revision = revision + 1, updated_at = ?
			WHERE id = ?`,
			doc.Version, string(raw), nowText, currentID,
		); err != nil {
			return visualdesign.Record{}, visualDesignStorageErr("save visual design", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return visualdesign.Record{}, visualDesignStorageErr("commit save", err)
	}

	saved, found, err := r.Get(ctx, ownerKind, ownerID)
	if err != nil {
		return visualdesign.Record{}, err
	}
	if !found {
		return visualdesign.Record{}, visualDesignStorageErr("save visual design", errors.New("design missing immediately after write"))
	}
	return saved, nil
}

// Delete removes the design saved for (ownerKind, ownerID), if any.
func (r *VisualDesignRepository) Delete(ctx context.Context, ownerKind visualdesign.OwnerKind, ownerID string) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM visual_designs WHERE owner_kind = ? AND owner_id = ?`, string(ownerKind), ownerID); err != nil {
		return visualDesignStorageErr("delete visual design", err)
	}
	return nil
}
