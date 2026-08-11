package visualtemplate

import (
	"fmt"

	"github.com/streaming-tree/server/internal/domain/visualdesign"
)

// Built-in templates are application-owned, immutable, reviewed
// constants (Stage 14A task Part 12) - never downloaded, never fetched
// from a remote registry, never a SQLite row. Each uses only existing
// safe visual-design primitives (no external asset, no arbitrary URL,
// no font beyond the closed allowlist) and must pass the exact same
// visualdesign.Validate every user template does - enforced once, for
// every built-in, by ValidateBuiltins below.
//
// Stable id namespace: "builtin_" + target + "_" + a short slug, never
// overlapping the "tpl_"-prefixed local database id space used for
// user templates (Stage 14A task Part 10).

func builtinShapeLayer(id, name string, frame visualdesign.Frame, order int, fill string) visualdesign.Layer {
	return visualdesign.Layer{
		ID: id, Name: name, Kind: visualdesign.LayerShape, Visible: true, Locked: false, Order: order,
		Frame: frame, Opacity: 1,
		Shape:          &visualdesign.ShapeProps{Kind: visualdesign.ShapeRectangle, Fill: fill, BorderColor: "#000000", BorderWidth: 0, CornerRadius: 8},
		EntryAnimation: visualdesign.AnimationFade, ExitAnimation: visualdesign.AnimationFade, AnimationDurationMS: 250,
	}
}

func builtinTextLayer(id, name string, frame visualdesign.Frame, order int, binding visualdesign.TextBinding, fontSize int, color string, weight int) visualdesign.Layer {
	return visualdesign.Layer{
		ID: id, Name: name, Kind: visualdesign.LayerText, Visible: true, Locked: false, Order: order,
		Frame: frame, Opacity: 1,
		Text: &visualdesign.TextProps{
			Binding: binding, MissingValueBehavior: visualdesign.MissingHide,
			FontFamily: visualdesign.FontSystemUI, FontSize: fontSize, FontWeight: weight, LineHeight: 1.2, LetterSpacing: 0,
			TextColor: color, HorizontalAlign: visualdesign.HAlignLeft, VerticalAlign: visualdesign.VAlignMiddle,
			OutlineWidth: 0, OutlineColor: "#000000",
		},
		EntryAnimation: visualdesign.AnimationFade, ExitAnimation: visualdesign.AnimationFade, AnimationDurationMS: 250,
	}
}

func builtinAvatarLayer(id, name string, frame visualdesign.Frame, order int) visualdesign.Layer {
	return visualdesign.Layer{
		ID: id, Name: name, Kind: visualdesign.LayerAvatar, Visible: true, Locked: false, Order: order,
		Frame: frame, Opacity: 1,
		Avatar:         &visualdesign.AvatarProps{CornerRadius: frame.Width / 2, BorderColor: "#FFFFFF", BorderWidth: 2},
		EntryAnimation: visualdesign.AnimationNone, ExitAnimation: visualdesign.AnimationNone,
	}
}

func builtinAlertTemplate(id, name, description string, fill, textColor, accent string) Template {
	doc := visualdesign.Document{
		Version: visualdesign.CurrentVersion,
		Canvas:  visualdesign.CanvasLandscape,
		Layers: []visualdesign.Layer{
			builtinShapeLayer(id+"_bg", "Background", visualdesign.Frame{X: 0, Y: 860, Width: 1920, Height: 220}, 0, fill),
			builtinShapeLayer(id+"_accent", "Accent", visualdesign.Frame{X: 0, Y: 860, Width: 12, Height: 220}, 1, accent),
			builtinAvatarLayer(id+"_avatar", "Avatar", visualdesign.Frame{X: 60, Y: 900, Width: 140, Height: 140}, 2),
			builtinTextLayer(id+"_username", "Username", visualdesign.Frame{X: 240, Y: 900, Width: 1600, Height: 60}, 3, visualdesign.BindingUsername, 44, textColor, 700),
			builtinTextLayer(id+"_message", "Message", visualdesign.Frame{X: 240, Y: 970, Width: 1600, Height: 90}, 4, visualdesign.BindingAlertRenderedText, 32, textColor, 400),
		},
	}
	return Template{
		ID: id, Target: TargetAlert, Source: SourceBuiltin,
		Name: name, Description: description, Author: "Streaming Tree", License: "CC0-1.0",
		TemplateSchemaVersion: CurrentTemplateSchemaVersion, Document: doc,
	}
}

func builtinChatTemplate(id, name, description string, fill, textColor, accent string) Template {
	doc := visualdesign.Document{
		Version: visualdesign.CurrentVersion,
		Canvas:  visualdesign.CanvasChatItem,
		Layers: []visualdesign.Layer{
			builtinShapeLayer(id+"_bg", "Background", visualdesign.Frame{X: 0, Y: 0, Width: 960, Height: 280}, 0, fill),
			builtinShapeLayer(id+"_accent", "Accent", visualdesign.Frame{X: 0, Y: 0, Width: 960, Height: 8}, 1, accent),
			builtinAvatarLayer(id+"_avatar", "Avatar", visualdesign.Frame{X: 16, Y: 20, Width: 48, Height: 48}, 2),
			builtinTextLayer(id+"_username", "Username", visualdesign.Frame{X: 76, Y: 20, Width: 860, Height: 32}, 3, visualdesign.BindingUsername, 22, textColor, 700),
			{
				ID: id + "_message", Name: "Message", Kind: visualdesign.LayerMessageFragments, Visible: true, Locked: false, Order: 4,
				Frame: visualdesign.Frame{X: 16, Y: 70, Width: 928, Height: 190}, Opacity: 1,
				MessageFragments: &visualdesign.MessageFragmentsProps{
					FontFamily: visualdesign.FontSystemUI, FontSize: 20, FontWeight: 400, LineHeight: 1.3, LetterSpacing: 0,
					TextColor: textColor, HorizontalAlign: visualdesign.HAlignLeft, VerticalAlign: visualdesign.VAlignTop, EmoteSize: 28,
				},
				EntryAnimation: visualdesign.AnimationFade, ExitAnimation: visualdesign.AnimationFade, AnimationDurationMS: 200,
			},
		},
	}
	return Template{
		ID: id, Target: TargetChat, Source: SourceBuiltin,
		Name: name, Description: description, Author: "Streaming Tree", License: "CC0-1.0",
		TemplateSchemaVersion: CurrentTemplateSchemaVersion, Document: doc,
	}
}

// DefaultBuiltins returns the reviewed built-in template set (Stage
// 14A task Part 12): three alert templates, three chat templates.
func DefaultBuiltins() []Template {
	return []Template{
		builtinAlertTemplate("builtin_alert_minimal_dark", "Minimal Dark", "A clean, low-contrast dark alert with a subtle accent bar.", "#141414CC", "#FFFFFF", "#6366F1"),
		builtinAlertTemplate("builtin_alert_clean_modern", "Clean Modern", "A bright, high-contrast alert with a bold accent bar.", "#FFFFFFE6", "#111827", "#2563EB"),
		builtinAlertTemplate("builtin_alert_neon_accent", "Neon Accent", "A dark alert with a vivid neon accent bar.", "#0B0B0FE6", "#F5F5F5", "#39FF88"),
		builtinChatTemplate("builtin_chat_minimal_dark", "Minimal Dark", "A clean, low-contrast dark chat item with a subtle accent bar.", "#141414CC", "#FFFFFF", "#6366F1"),
		builtinChatTemplate("builtin_chat_compact", "Compact", "A bright, compact chat item for dense chat activity.", "#FFFFFFE6", "#111827", "#2563EB"),
		builtinChatTemplate("builtin_chat_neon_accent", "Neon Accent", "A dark chat item with a vivid neon accent bar.", "#0B0B0FE6", "#F5F5F5", "#39FF88"),
	}
}

// ValidateBuiltins fails loudly (returning an error, never silently
// dropping an entry) if any built-in is malformed: not a
// structurally/semantically valid template, a duplicate id, or a "tpl_"
// -prefixed id colliding with the user id namespace (Stage 14A task
// Part 25/10). Intended to run at application startup and in a
// dedicated unit test, so a broken built-in can never reach a user.
func ValidateBuiltins(builtins []Template) error {
	seen := make(map[string]bool, len(builtins))
	for _, b := range builtins {
		if b.Source != SourceBuiltin {
			return fmt.Errorf("built-in template %q: Source must be SourceBuiltin", b.ID)
		}
		if seen[b.ID] {
			return fmt.Errorf("built-in template %q: duplicate id", b.ID)
		}
		seen[b.ID] = true
		if len(b.ID) >= 4 && b.ID[:4] == "tpl_" {
			return fmt.Errorf("built-in template %q: id must not use the \"tpl_\" user-template namespace", b.ID)
		}
		if err := Validate(b); err != nil {
			return fmt.Errorf("built-in template %q: %w", b.ID, err)
		}
	}
	return nil
}
