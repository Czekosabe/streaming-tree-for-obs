package alerts

import "github.com/streaming-tree/server/internal/domain/visualdesign"

// draftTextWidth/draftTextHeight/draftMargin are the fixed layout
// constants GenerateLegacyDraft uses to place its one text layer -
// deliberately simple constants, not a real replication of every
// theme's exact CSS (Stage 13A task Part 19: "as closely as
// practical", not pixel-perfect).
const (
	draftTextWidth  = 1600
	draftTextHeight = 160
	draftMargin     = 40
)

// draftFontSizeForTheme gives each Stage 12A Theme a reasonably
// corresponding font size, so a draft opened for a "large" theme's rule
// starts out visibly larger than a "minimal" one's - an honest best
// effort, not a claim of pixel-perfect equivalence.
func draftFontSizeForTheme(t Theme) int {
	switch t {
	case ThemeCompact:
		return 32
	case ThemeLarge:
		return 64
	default: // ThemeMinimal and any future default
		return 44
	}
}

func draftFrameForPosition(canvas visualdesign.Canvas, pos Position) visualdesign.Frame {
	x := (canvas.Width - draftTextWidth) / 2
	if x < 0 {
		x = 0
	}
	switch pos {
	case PositionTop:
		return visualdesign.Frame{X: x, Y: draftMargin, Width: draftTextWidth, Height: draftTextHeight}
	case PositionCenter:
		y := (canvas.Height - draftTextHeight) / 2
		return visualdesign.Frame{X: x, Y: y, Width: draftTextWidth, Height: draftTextHeight}
	default: // PositionBottom and any future default
		y := canvas.Height - draftTextHeight - draftMargin
		return visualdesign.Frame{X: x, Y: y, Width: draftTextWidth, Height: draftTextHeight}
	}
}

func draftHorizontalAlign(a TextAlign) visualdesign.HorizontalAlign {
	switch a {
	case AlignLeft:
		return visualdesign.HAlignLeft
	case AlignRight:
		return visualdesign.HAlignRight
	default:
		return visualdesign.HAlignCenter
	}
}

// draftAnimation converts this domain package's own Animation enum to
// visualdesign's separate-but-value-identical one (see
// visualdesign.Animation's own doc comment for why the two types are
// deliberately never unified).
func draftAnimation(a Animation) visualdesign.Animation {
	switch a {
	case AnimationFade:
		return visualdesign.AnimationFade
	case AnimationSlideUp:
		return visualdesign.AnimationSlideUp
	case AnimationSlideLeft:
		return visualdesign.AnimationSlideLeft
	case AnimationScale:
		return visualdesign.AnimationScale
	default:
		return visualdesign.AnimationNone
	}
}

// GenerateLegacyDraft builds a deterministic, never-persisted-by-itself
// visual-design document representing rule's current Stage 12 fixed
// presentation as closely as practical (Stage 13A task Part 19): one
// text layer bound to alert_rendered_text (reusing the rule's own
// already-validated template, exactly as it renders today), positioned
// and aligned per profile's fixed position/text-align, sized per
// profile's theme, using the rule's own entry/exit animation. Calling
// this performs no I/O and has no persistence side effect whatsoever -
// the caller (internal/httpapi's GET visual-design handler) decides
// whether/when to ever save the result, per Part 19's own explicit
// "generate... do not persist it merely by opening the page."
func GenerateLegacyDraft(profile Profile, rule Rule) visualdesign.Document {
	canvas := visualdesign.CanvasLandscape
	frame := draftFrameForPosition(canvas, profile.Position)

	layerID := "layer_legacy_draft_text"
	textLayer := visualdesign.Layer{
		ID: layerID, Name: "Alert text", Kind: visualdesign.LayerText,
		Visible: true, Locked: false, Order: 0,
		Frame: frame, Opacity: 1,
		Text: &visualdesign.TextProps{
			Binding:              visualdesign.BindingAlertRenderedText,
			MissingValueBehavior: visualdesign.MissingHide,
			FontFamily:           visualdesign.FontSystemUI,
			FontSize:             draftFontSizeForTheme(profile.Theme),
			FontWeight:           700,
			LineHeight:           1.2,
			LetterSpacing:        0,
			TextColor:            "#FFFFFF",
			HorizontalAlign:      draftHorizontalAlign(profile.TextAlign),
			VerticalAlign:        visualdesign.VAlignMiddle,
			OutlineWidth:         0,
			OutlineColor:         "#000000",
			ShadowEnabled:        true,
			ShadowOffsetX:        0,
			ShadowOffsetY:        2,
			ShadowBlur:           8,
			ShadowColor:          "#000000CC",
		},
		EntryAnimation: draftAnimation(rule.EntryAnimation), ExitAnimation: draftAnimation(rule.ExitAnimation),
		AnimationDurationMS: rule.AnimationDurationMS,
	}

	return visualdesign.Document{
		Version: visualdesign.CurrentVersion,
		Canvas:  canvas,
		Layers:  []visualdesign.Layer{textLayer},
	}
}
