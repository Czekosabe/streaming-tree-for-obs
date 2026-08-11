package chatoverlay

import "github.com/streaming-tree/server/internal/domain/visualdesign"

// draftAnimation converts this domain package's own Animation enum to
// visualdesign's separate-but-value-identical one (mirrors
// internal/domain/alerts's own draftAnimation - see that function's own
// doc comment for why the two Animation types are deliberately never
// unified).
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

func draftFontFamily(f FontFamily) visualdesign.FontFamily {
	switch f {
	case FontSerif:
		return visualdesign.FontSerif
	case FontMonospace:
		return visualdesign.FontMonospace
	default: // FontSansSerif, FontRounded (no visualdesign equivalent) both fall back to system-ui.
		return visualdesign.FontSystemUI
	}
}

func draftHorizontalAlign(a HorizontalAlignment) visualdesign.HorizontalAlign {
	switch a {
	case AlignCenter:
		return visualdesign.HAlignCenter
	case AlignRight:
		return visualdesign.HAlignRight
	default:
		return visualdesign.HAlignLeft
	}
}

// GenerateLegacyDraft builds a deterministic, never-persisted-by-itself
// visual-design document representing profile's current Stage 10 fixed
// item presentation as closely as practical (Stage 13B,
// docs/visual-designs.md §23) - an honest best effort, not a
// pixel-perfect reproduction, mirroring internal/domain/alerts's own
// GenerateLegacyDraft. Calling this performs no I/O and has no
// persistence side effect whatsoever.
func GenerateLegacyDraft(profile Profile) visualdesign.Document {
	canvas := visualdesign.CanvasChatItem
	const margin = 12

	layers := []visualdesign.Layer{
		{
			ID: "layer_legacy_draft_background", Name: "Background", Kind: visualdesign.LayerShape,
			Visible: true, Locked: false, Order: 0,
			Frame:   visualdesign.Frame{X: 0, Y: 0, Width: canvas.Width, Height: canvas.Height},
			Opacity: 1,
			Shape: &visualdesign.ShapeProps{
				Kind: visualdesign.ShapeRectangle, Fill: normalizedDraftColor(profile.BubbleColor),
				BorderWidth: 0, CornerRadius: clampInt(profile.BorderRadius, visualdesign.MinCornerRadius, visualdesign.MaxCornerRadius),
			},
			EntryAnimation: draftAnimation(profile.EntryAnimation), ExitAnimation: draftAnimation(profile.ExitAnimation),
			AnimationDurationMS: profile.AnimationDurationMS,
		},
	}

	nextY := margin
	usernameHeight := 28
	layers = append(layers, visualdesign.Layer{
		ID: "layer_legacy_draft_username", Name: "Username", Kind: visualdesign.LayerText,
		Visible: true, Locked: false, Order: 1,
		Frame:   visualdesign.Frame{X: margin, Y: nextY, Width: canvas.Width - 2*margin, Height: usernameHeight},
		Opacity: 1,
		Text: &visualdesign.TextProps{
			Binding: visualdesign.BindingUsername, MissingValueBehavior: visualdesign.MissingHide,
			FontFamily: draftFontFamily(profile.FontFamily), FontSize: clampInt(profile.FontSize+4, visualdesign.MinFontSize, visualdesign.MaxFontSize),
			FontWeight: 700, LineHeight: profile.LineHeight, TextColor: normalizedDraftColor(profile.TextColor),
			HorizontalAlign: draftHorizontalAlign(profile.HorizontalAlignment), VerticalAlign: visualdesign.VAlignMiddle,
		},
		EntryAnimation: visualdesign.AnimationNone, ExitAnimation: visualdesign.AnimationNone,
	})
	nextY += usernameHeight + 4

	messageHeight := canvas.Height - nextY - margin
	if messageHeight < visualdesign.MinLayerSize {
		messageHeight = visualdesign.MinLayerSize
	}
	layers = append(layers, visualdesign.Layer{
		ID: "layer_legacy_draft_message", Name: "Message", Kind: visualdesign.LayerMessageFragments,
		Visible: true, Locked: false, Order: 2,
		Frame:   visualdesign.Frame{X: margin, Y: nextY, Width: canvas.Width - 2*margin, Height: messageHeight},
		Opacity: 1,
		MessageFragments: &visualdesign.MessageFragmentsProps{
			FontFamily: draftFontFamily(profile.FontFamily), FontSize: clampInt(profile.FontSize, visualdesign.MinFontSize, visualdesign.MaxFontSize),
			FontWeight: clampInt(profile.FontWeight, visualdesign.MinFontWeight, visualdesign.MaxFontWeight), LineHeight: profile.LineHeight,
			TextColor: normalizedDraftColor(profile.TextColor), HorizontalAlign: draftHorizontalAlign(profile.HorizontalAlignment),
			VerticalAlign: visualdesign.VAlignTop, EmoteSize: clampInt(profile.FontSize+4, visualdesign.MinEmoteSize, visualdesign.MaxEmoteSize),
		},
		EntryAnimation: visualdesign.AnimationNone, ExitAnimation: visualdesign.AnimationNone,
	})

	order := len(layers)
	if profile.ShowAvatar {
		const avatarSize = 32
		layers = append(layers, visualdesign.Layer{
			ID: "layer_legacy_draft_avatar", Name: "Avatar", Kind: visualdesign.LayerAvatar,
			Visible: true, Locked: false, Order: order,
			Frame:          visualdesign.Frame{X: canvas.Width - avatarSize - margin, Y: margin, Width: avatarSize, Height: avatarSize},
			Opacity:        1,
			Avatar:         &visualdesign.AvatarProps{CornerRadius: avatarSize / 2, BorderWidth: 0, BorderColor: "#000000"},
			EntryAnimation: visualdesign.AnimationNone, ExitAnimation: visualdesign.AnimationNone,
		})
		order++
	}
	if profile.ShowBadges {
		const badgeRowHeight = 20
		layers = append(layers, visualdesign.Layer{
			ID: "layer_legacy_draft_badges", Name: "Badges", Kind: visualdesign.LayerBadgeList,
			Visible: true, Locked: false, Order: order,
			Frame:          visualdesign.Frame{X: margin, Y: 2, Width: 200, Height: badgeRowHeight},
			Opacity:        1,
			BadgeList:      &visualdesign.BadgeListProps{MaxCount: 5, BadgeSize: 18, Gap: 4},
			EntryAnimation: visualdesign.AnimationNone, ExitAnimation: visualdesign.AnimationNone,
		})
		order++
	}
	if profile.ShowTimestamp {
		layers = append(layers, visualdesign.Layer{
			ID: "layer_legacy_draft_timestamp", Name: "Timestamp", Kind: visualdesign.LayerText,
			Visible: true, Locked: false, Order: order,
			Frame:   visualdesign.Frame{X: canvas.Width - 120 - margin, Y: canvas.Height - 24 - margin, Width: 120, Height: 20},
			Opacity: 0.75,
			Text: &visualdesign.TextProps{
				Binding: visualdesign.BindingTimestamp, MissingValueBehavior: visualdesign.MissingHide,
				FontFamily: draftFontFamily(profile.FontFamily), FontSize: clampInt(profile.FontSize-4, visualdesign.MinFontSize, visualdesign.MaxFontSize),
				FontWeight: 400, LineHeight: 1.2, TextColor: normalizedDraftColor(profile.TextColor),
				HorizontalAlign: visualdesign.HAlignRight, VerticalAlign: visualdesign.VAlignMiddle,
			},
			EntryAnimation: visualdesign.AnimationNone, ExitAnimation: visualdesign.AnimationNone,
		})
		order++
	}
	if profile.ShowAccountLabel {
		layers = append(layers, visualdesign.Layer{
			ID: "layer_legacy_draft_account_label", Name: "Account label", Kind: visualdesign.LayerText,
			Visible: true, Locked: false, Order: order,
			Frame:   visualdesign.Frame{X: margin, Y: canvas.Height - 24 - margin, Width: 160, Height: 20},
			Opacity: 0.75,
			Text: &visualdesign.TextProps{
				Binding: visualdesign.BindingAccountLabel, MissingValueBehavior: visualdesign.MissingHide,
				FontFamily: draftFontFamily(profile.FontFamily), FontSize: clampInt(profile.FontSize-4, visualdesign.MinFontSize, visualdesign.MaxFontSize),
				FontWeight: 400, LineHeight: 1.2, TextColor: normalizedDraftColor(profile.TextColor),
				HorizontalAlign: visualdesign.HAlignLeft, VerticalAlign: visualdesign.VAlignMiddle,
			},
			EntryAnimation: visualdesign.AnimationNone, ExitAnimation: visualdesign.AnimationNone,
		})
	}

	return visualdesign.Document{Version: visualdesign.CurrentVersion, Canvas: canvas, Layers: layers}
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// normalizedDraftColor falls back to a safe opaque color for anything
// that is not already a validated #RRGGBB/#RRGGBBAA string - the same
// defensive posture the frontend's own overlay-style.ts already applies
// to legacy profile colors.
func normalizedDraftColor(c string) string {
	if visualdesign.IsValidColor(c) {
		return c
	}
	return "#FFFFFF"
}
