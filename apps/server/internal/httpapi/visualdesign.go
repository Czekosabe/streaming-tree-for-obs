package httpapi

import (
	"log/slog"
	"net/http"

	domain "github.com/streaming-tree/server/internal/domain/alerts"
	"github.com/streaming-tree/server/internal/domain/visualdesign"
)

// maxVisualDesignBodyBytes caps a saved design's own JSON request body -
// generous relative to visualdesign.MaxDocumentBytes (64 KiB) so a
// well-formed, in-bounds document is never rejected purely by HTTP
// framing overhead, while still bounding what a malicious/broken client
// could send before validation even runs.
const maxVisualDesignBodyBytes = 96 * 1024

// registerVisualDesignRoutes wires Stage 13A's alert-rule visual-design
// management API - GET/PUT/DELETE on the one design a rule may own
// (Stage 13A task Part 42). Deliberately nested under /api/alert-rules/
// rather than a generic /api/visual-designs/{ownerKind}/{ownerId} route:
// the browser is never handed a raw polymorphic owner id to construct
// itself (Part 42's own "do not expose a generic arbitrary owner ID
// endpoint").
func registerVisualDesignRoutes(mux *http.ServeMux, logger *slog.Logger, svc AlertsService) {
	mux.HandleFunc("GET /api/alert-rules/{id}/visual-design", handleGetVisualDesign(logger, svc))
	mux.HandleFunc("PUT /api/alert-rules/{id}/visual-design", handleSaveVisualDesign(logger, svc))
	mux.HandleFunc("DELETE /api/alert-rules/{id}/visual-design", handleDeleteVisualDesign(logger, svc))
	mux.HandleFunc("/api/alert-rules/{id}/visual-design", methodNotAllowed(logger, http.MethodGet, http.MethodPut, http.MethodDelete))
}

// --- wire DTOs (Stage 13A task Part 6: a management-only DTO, never
// the raw domain struct, never the public one) -----------------------

type visualDesignFrameDTO struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

type visualDesignShapeDTO struct {
	Kind         string `json:"kind"`
	Fill         string `json:"fill"`
	BorderColor  string `json:"borderColor"`
	BorderWidth  int    `json:"borderWidth"`
	CornerRadius int    `json:"cornerRadius"`
}

type visualDesignTextDTO struct {
	Binding              string  `json:"binding"`
	StaticText           string  `json:"staticText,omitempty"`
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

type visualDesignAvatarDTO struct {
	CornerRadius int    `json:"cornerRadius"`
	BorderColor  string `json:"borderColor"`
	BorderWidth  int    `json:"borderWidth"`
}

type visualDesignLayerDTO struct {
	ID      string               `json:"id"`
	Name    string               `json:"name"`
	Kind    string               `json:"kind"`
	Visible bool                 `json:"visible"`
	Locked  bool                 `json:"locked"`
	Order   int                  `json:"order"`
	Frame   visualDesignFrameDTO `json:"frame"`
	Opacity float64              `json:"opacity"`

	Shape        *visualDesignShapeDTO  `json:"shape,omitempty"`
	Text         *visualDesignTextDTO   `json:"text,omitempty"`
	PlatformIcon *struct{}              `json:"platformIcon,omitempty"`
	Avatar       *visualDesignAvatarDTO `json:"avatar,omitempty"`

	EntryAnimation      string `json:"entryAnimation"`
	ExitAnimation       string `json:"exitAnimation"`
	AnimationDurationMS int    `json:"animationDurationMs"`
}

type visualDesignCanvasDTO struct {
	Width       int  `json:"width"`
	Height      int  `json:"height"`
	Transparent bool `json:"transparent"`
}

type visualDesignDocumentDTO struct {
	Version int                    `json:"version"`
	Canvas  visualDesignCanvasDTO  `json:"canvas"`
	Layers  []visualDesignLayerDTO `json:"layers"`
}

type visualDesignResponse struct {
	// Persisted: false means document below is a freshly generated,
	// never-saved legacy-compatible draft (Stage 13A task Part 19) -
	// Revision is always 0 in that case.
	Persisted bool                    `json:"persisted"`
	Revision  int                     `json:"revision"`
	Document  visualDesignDocumentDTO `json:"document"`
}

type visualDesignSaveRequest struct {
	ExpectedRevision int                     `json:"expectedRevision"`
	Document         visualDesignDocumentDTO `json:"document"`
}

func documentToDTO(doc visualdesign.Document) visualDesignDocumentDTO {
	layers := make([]visualDesignLayerDTO, 0, len(doc.Layers))
	for _, l := range doc.Layers {
		dto := visualDesignLayerDTO{
			ID: l.ID, Name: l.Name, Kind: string(l.Kind), Visible: l.Visible, Locked: l.Locked, Order: l.Order,
			Frame:          visualDesignFrameDTO{X: l.Frame.X, Y: l.Frame.Y, Width: l.Frame.Width, Height: l.Frame.Height},
			Opacity:        l.Opacity,
			EntryAnimation: string(l.EntryAnimation), ExitAnimation: string(l.ExitAnimation), AnimationDurationMS: l.AnimationDurationMS,
		}
		if l.Shape != nil {
			dto.Shape = &visualDesignShapeDTO{
				Kind: string(l.Shape.Kind), Fill: l.Shape.Fill, BorderColor: l.Shape.BorderColor,
				BorderWidth: l.Shape.BorderWidth, CornerRadius: l.Shape.CornerRadius,
			}
		}
		if l.Text != nil {
			dto.Text = &visualDesignTextDTO{
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
			dto.PlatformIcon = &struct{}{}
		}
		if l.Avatar != nil {
			dto.Avatar = &visualDesignAvatarDTO{CornerRadius: l.Avatar.CornerRadius, BorderColor: l.Avatar.BorderColor, BorderWidth: l.Avatar.BorderWidth}
		}
		layers = append(layers, dto)
	}
	return visualDesignDocumentDTO{
		Version: doc.Version,
		Canvas:  visualDesignCanvasDTO{Width: doc.Canvas.Width, Height: doc.Canvas.Height, Transparent: doc.Canvas.Transparent},
		Layers:  layers,
	}
}

func documentFromDTO(dto visualDesignDocumentDTO) visualdesign.Document {
	layers := make([]visualdesign.Layer, 0, len(dto.Layers))
	for _, l := range dto.Layers {
		layer := visualdesign.Layer{
			ID: l.ID, Name: l.Name, Kind: visualdesign.LayerKind(l.Kind), Visible: l.Visible, Locked: l.Locked, Order: l.Order,
			Frame:          visualdesign.Frame{X: l.Frame.X, Y: l.Frame.Y, Width: l.Frame.Width, Height: l.Frame.Height},
			Opacity:        l.Opacity,
			EntryAnimation: visualdesign.Animation(l.EntryAnimation), ExitAnimation: visualdesign.Animation(l.ExitAnimation),
			AnimationDurationMS: l.AnimationDurationMS,
		}
		if l.Shape != nil {
			layer.Shape = &visualdesign.ShapeProps{
				Kind: visualdesign.ShapeKind(l.Shape.Kind), Fill: l.Shape.Fill, BorderColor: l.Shape.BorderColor,
				BorderWidth: l.Shape.BorderWidth, CornerRadius: l.Shape.CornerRadius,
			}
		}
		if l.Text != nil {
			layer.Text = &visualdesign.TextProps{
				Binding: visualdesign.TextBinding(l.Text.Binding), StaticText: l.Text.StaticText,
				MissingValueBehavior: visualdesign.MissingValueBehavior(l.Text.MissingValueBehavior),
				FontFamily:           visualdesign.FontFamily(l.Text.FontFamily), FontSize: l.Text.FontSize, FontWeight: l.Text.FontWeight,
				LineHeight: l.Text.LineHeight, LetterSpacing: l.Text.LetterSpacing, TextColor: l.Text.TextColor,
				HorizontalAlign: visualdesign.HorizontalAlign(l.Text.HorizontalAlign), VerticalAlign: visualdesign.VerticalAlign(l.Text.VerticalAlign),
				OutlineWidth: l.Text.OutlineWidth, OutlineColor: l.Text.OutlineColor,
				ShadowEnabled: l.Text.ShadowEnabled, ShadowOffsetX: l.Text.ShadowOffsetX, ShadowOffsetY: l.Text.ShadowOffsetY,
				ShadowBlur: l.Text.ShadowBlur, ShadowColor: l.Text.ShadowColor,
			}
		}
		if l.PlatformIcon != nil {
			layer.PlatformIcon = &visualdesign.PlatformIconProps{}
		}
		if l.Avatar != nil {
			layer.Avatar = &visualdesign.AvatarProps{CornerRadius: l.Avatar.CornerRadius, BorderColor: l.Avatar.BorderColor, BorderWidth: l.Avatar.BorderWidth}
		}
		layers = append(layers, layer)
	}
	return visualdesign.Document{
		Version: dto.Version,
		Canvas:  visualdesign.Canvas{Width: dto.Canvas.Width, Height: dto.Canvas.Height, Transparent: dto.Canvas.Transparent},
		Layers:  layers,
	}
}

// toPublicVisualDesignDTO serializes doc (already the safe
// visualdesign.PublicDocument shape - no names, no locked state) for
// embedding inside a public alert.show payload (Stage 13A task Part
// 17/23).
func toPublicVisualDesignDTO(doc *visualdesign.PublicDocument) map[string]any {
	if doc == nil {
		return nil
	}
	layers := make([]map[string]any, 0, len(doc.Layers))
	for _, l := range doc.Layers {
		layer := map[string]any{
			"id": l.ID, "kind": l.Kind,
			"frame":          map[string]any{"x": l.Frame.X, "y": l.Frame.Y, "width": l.Frame.Width, "height": l.Frame.Height},
			"opacity":        l.Opacity,
			"entryAnimation": l.EntryAnimation, "exitAnimation": l.ExitAnimation, "animationDurationMs": l.AnimationDurationMS,
		}
		if l.Shape != nil {
			layer["shape"] = map[string]any{
				"kind": l.Shape.Kind, "fill": l.Shape.Fill, "borderColor": l.Shape.BorderColor,
				"borderWidth": l.Shape.BorderWidth, "cornerRadius": l.Shape.CornerRadius,
			}
		}
		if l.Text != nil {
			layer["text"] = map[string]any{
				"binding": l.Text.Binding, "staticText": l.Text.StaticText, "missingValueBehavior": l.Text.MissingValueBehavior,
				"fontFamily": l.Text.FontFamily, "fontSize": l.Text.FontSize, "fontWeight": l.Text.FontWeight,
				"lineHeight": l.Text.LineHeight, "letterSpacing": l.Text.LetterSpacing, "textColor": l.Text.TextColor,
				"horizontalAlign": l.Text.HorizontalAlign, "verticalAlign": l.Text.VerticalAlign,
				"outlineWidth": l.Text.OutlineWidth, "outlineColor": l.Text.OutlineColor,
				"shadowEnabled": l.Text.ShadowEnabled, "shadowOffsetX": l.Text.ShadowOffsetX, "shadowOffsetY": l.Text.ShadowOffsetY,
				"shadowBlur": l.Text.ShadowBlur, "shadowColor": l.Text.ShadowColor,
			}
		}
		if l.PlatformIcon != nil {
			layer["platformIcon"] = map[string]any{}
		}
		if l.Avatar != nil {
			layer["avatar"] = map[string]any{"cornerRadius": l.Avatar.CornerRadius, "borderColor": l.Avatar.BorderColor, "borderWidth": l.Avatar.BorderWidth}
		}
		layers = append(layers, layer)
	}
	return map[string]any{
		"schemaVersion": doc.SchemaVersion,
		"canvas":        map[string]any{"width": doc.Canvas.Width, "height": doc.Canvas.Height, "transparent": doc.Canvas.Transparent},
		"layers":        layers,
	}
}

// --- handlers -------------------------------------------------------------

// handleGetVisualDesign returns ruleID's own saved design, or a freshly
// generated (never persisted by this call) legacy-compatible draft when
// none has been saved yet (Stage 13A task Part 19/42).
func handleGetVisualDesign(logger *slog.Logger, svc AlertsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ruleID := r.PathValue("id")
		rule, err := svc.GetRule(r.Context(), ruleID)
		if err != nil {
			writeAlertsError(w, logger, err)
			return
		}
		profile, err := svc.GetProfile(r.Context(), rule.ProfileID)
		if err != nil {
			writeAlertsError(w, logger, err)
			return
		}

		rec, found, err := svc.GetVisualDesign(r.Context(), ruleID)
		if err != nil {
			writeAlertsError(w, logger, err)
			return
		}
		if !found {
			draft := domain.GenerateLegacyDraft(profile, rule)
			writeJSON(w, logger, http.StatusOK, visualDesignResponse{Persisted: false, Revision: 0, Document: documentToDTO(draft)})
			return
		}
		writeJSON(w, logger, http.StatusOK, visualDesignResponse{Persisted: true, Revision: rec.Revision, Document: documentToDTO(rec.Document)})
	}
}

// handleSaveVisualDesign validates and persists a full-replacement save
// of ruleID's own visual design (Stage 13A task Part 41) - 409 on a
// stale expectedRevision, 422 on any structural/semantic/binding-
// capability validation failure.
func handleSaveVisualDesign(logger *slog.Logger, svc AlertsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ruleID := r.PathValue("id")
		var body visualDesignSaveRequest
		if err := decodeJSONWithLimit(w, r, &body, maxVisualDesignBodyBytes); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		doc := documentFromDTO(body.Document)
		rec, err := svc.SaveVisualDesign(r.Context(), ruleID, doc, body.ExpectedRevision)
		if err != nil {
			writeAlertsError(w, logger, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, visualDesignResponse{Persisted: true, Revision: rec.Revision, Document: documentToDTO(rec.Document)})
	}
}

// handleDeleteVisualDesign implements "Reset to legacy presentation"
// (Stage 13A task Part 19) - idempotent, no request body, never deletes
// the rule itself.
func handleDeleteVisualDesign(logger *slog.Logger, svc AlertsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := svc.DeleteVisualDesign(r.Context(), r.PathValue("id")); err != nil {
			writeAlertsError(w, logger, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
