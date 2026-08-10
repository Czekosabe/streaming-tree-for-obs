package visualdesign

import (
	"fmt"
	"math"
	"regexp"
)

var hexColorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}([0-9A-Fa-f]{2})?$`)

// IsValidColor reports whether c is a normalized "#RRGGBB" or
// "#RRGGBBAA" hex color string - the only color shape Stage 13A ever
// accepts (Stage 13A task Part 13), on both backend and frontend.
func IsValidColor(c string) bool {
	return hexColorPattern.MatchString(c)
}

func codePointLen(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

func validationErr(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrValidation, fmt.Sprintf(format, args...))
}

// Validate checks doc structurally and semantically: version, canvas
// bounds, layer count, per-layer bounds, unique layer IDs, and that
// every layer carries exactly the kind-specific payload matching its
// own Kind. It does NOT check alert-specific text-binding availability
// (e.g. {quantity} on a "follow" rule's design) - that capability check
// lives beside the alert domain, since it needs to know the owning
// rule's own event type (see internal/domain/alerts's own
// visualdesign_binding.go).
func Validate(doc Document) error {
	if doc.Version != CurrentVersion {
		return validationErr("unsupported document version %d (expected %d)", doc.Version, CurrentVersion)
	}
	if err := validateCanvas(doc.Canvas); err != nil {
		return err
	}
	if len(doc.Layers) > MaxLayers {
		return validationErr("a design may have at most %d layers, got %d", MaxLayers, len(doc.Layers))
	}
	seenIDs := make(map[string]bool, len(doc.Layers))
	for i, l := range doc.Layers {
		if l.ID == "" {
			return validationErr("layer %d: id must not be empty", i)
		}
		if seenIDs[l.ID] {
			return validationErr("layer %d: duplicate layer id %q", i, l.ID)
		}
		seenIDs[l.ID] = true
		if err := validateLayer(l, doc.Canvas); err != nil {
			return fmt.Errorf("layer %q: %w", l.ID, err)
		}
	}
	return nil
}

func validateCanvas(c Canvas) error {
	if c.Width < MinCanvasWidth || c.Width > MaxCanvasWidth {
		return validationErr("canvas width must be between %d and %d", MinCanvasWidth, MaxCanvasWidth)
	}
	if c.Height < MinCanvasHeight || c.Height > MaxCanvasHeight {
		return validationErr("canvas height must be between %d and %d", MinCanvasHeight, MaxCanvasHeight)
	}
	if math.IsNaN(float64(c.Width)) || math.IsNaN(float64(c.Height)) {
		return validationErr("canvas dimensions must not be NaN")
	}
	return nil
}

func validateLayer(l Layer, canvas Canvas) error {
	if n := codePointLen(l.Name); n > MaxLayerNameCodePoints {
		return validationErr("name must be at most %d characters", MaxLayerNameCodePoints)
	}
	if !l.Kind.valid() {
		return validationErr("kind %q is not a recognized layer kind", string(l.Kind))
	}
	if err := validateFrame(l.Frame, canvas); err != nil {
		return err
	}
	if math.IsNaN(l.Opacity) || math.IsInf(l.Opacity, 0) || l.Opacity < MinOpacity || l.Opacity > MaxOpacity {
		return validationErr("opacity must be between %v and %v", MinOpacity, MaxOpacity)
	}
	if !l.EntryAnimation.valid() {
		return validationErr("entry animation %q is not recognized", string(l.EntryAnimation))
	}
	if !l.ExitAnimation.valid() {
		return validationErr("exit animation %q is not recognized", string(l.ExitAnimation))
	}
	if l.AnimationDurationMS < MinLayerAnimationDurationMS || l.AnimationDurationMS > MaxLayerAnimationDurationMS {
		return validationErr("animation duration must be between %d and %d milliseconds", MinLayerAnimationDurationMS, MaxLayerAnimationDurationMS)
	}

	present := 0
	if l.Shape != nil {
		present++
	}
	if l.Text != nil {
		present++
	}
	if l.PlatformIcon != nil {
		present++
	}
	if l.Avatar != nil {
		present++
	}
	if present != 1 {
		return validationErr("exactly one kind-specific payload must be present, got %d", present)
	}

	switch l.Kind {
	case LayerShape:
		if l.Shape == nil {
			return validationErr("kind %q requires a shape payload", l.Kind)
		}
		return validateShape(*l.Shape)
	case LayerText:
		if l.Text == nil {
			return validationErr("kind %q requires a text payload", l.Kind)
		}
		return validateText(*l.Text)
	case LayerPlatformIcon:
		if l.PlatformIcon == nil {
			return validationErr("kind %q requires a platform_icon payload", l.Kind)
		}
		return nil
	case LayerAvatar:
		if l.Avatar == nil {
			return validationErr("kind %q requires an avatar payload", l.Kind)
		}
		return validateAvatar(*l.Avatar)
	default:
		return validationErr("kind %q is not a recognized layer kind", string(l.Kind))
	}
}

func validateFrame(f Frame, canvas Canvas) error {
	if f.Width < MinLayerSize || f.Height < MinLayerSize {
		return validationErr("frame width/height must be at least %d design units", MinLayerSize)
	}
	if f.X < 0 || f.Y < 0 {
		return validationErr("frame position must not be negative")
	}
	if f.X+f.Width > canvas.Width || f.Y+f.Height > canvas.Height {
		return validationErr("frame must remain fully inside the canvas (%dx%d)", canvas.Width, canvas.Height)
	}
	return nil
}

func validateShape(s ShapeProps) error {
	if !s.Kind.valid() {
		return validationErr("shape kind %q is not recognized", string(s.Kind))
	}
	if !IsValidColor(s.Fill) {
		return validationErr("fill %q is not a valid color", s.Fill)
	}
	if s.BorderWidth > 0 && !IsValidColor(s.BorderColor) {
		return validationErr("border color %q is not a valid color", s.BorderColor)
	}
	if s.BorderWidth < MinBorderWidth || s.BorderWidth > MaxBorderWidth {
		return validationErr("border width must be between %d and %d", MinBorderWidth, MaxBorderWidth)
	}
	if s.CornerRadius < MinCornerRadius || s.CornerRadius > MaxCornerRadius {
		return validationErr("corner radius must be between %d and %d", MinCornerRadius, MaxCornerRadius)
	}
	return nil
}

func validateAvatar(a AvatarProps) error {
	if a.CornerRadius < MinCornerRadius || a.CornerRadius > MaxCornerRadius {
		return validationErr("corner radius must be between %d and %d", MinCornerRadius, MaxCornerRadius)
	}
	if a.BorderWidth < MinBorderWidth || a.BorderWidth > MaxBorderWidth {
		return validationErr("border width must be between %d and %d", MinBorderWidth, MaxBorderWidth)
	}
	if a.BorderWidth > 0 && !IsValidColor(a.BorderColor) {
		return validationErr("border color %q is not a valid color", a.BorderColor)
	}
	return nil
}

func validateText(t TextProps) error {
	if !t.Binding.valid() {
		return validationErr("text binding %q is not recognized", string(t.Binding))
	}
	if t.Binding == BindingStatic {
		n := codePointLen(t.StaticText)
		if n == 0 {
			return validationErr("static text must not be empty")
		}
		if n > MaxStaticTextCodePoints {
			return validationErr("static text must be at most %d characters", MaxStaticTextCodePoints)
		}
	}
	if !t.MissingValueBehavior.valid() {
		return validationErr("missing-value behavior %q is not recognized", string(t.MissingValueBehavior))
	}
	if !t.FontFamily.valid() {
		return validationErr("font family %q is not in the allowed system-font list", string(t.FontFamily))
	}
	if t.FontSize < MinFontSize || t.FontSize > MaxFontSize {
		return validationErr("font size must be between %d and %d", MinFontSize, MaxFontSize)
	}
	if t.FontWeight < MinFontWeight || t.FontWeight > MaxFontWeight || t.FontWeight%FontWeightStep != 0 {
		return validationErr("font weight must be between %d and %d in steps of %d", MinFontWeight, MaxFontWeight, FontWeightStep)
	}
	if t.LineHeight < MinLineHeight || t.LineHeight > MaxLineHeight {
		return validationErr("line height must be between %v and %v", MinLineHeight, MaxLineHeight)
	}
	if t.LetterSpacing < MinLetterSpacing || t.LetterSpacing > MaxLetterSpacing {
		return validationErr("letter spacing must be between %v and %v", MinLetterSpacing, MaxLetterSpacing)
	}
	if !IsValidColor(t.TextColor) {
		return validationErr("text color %q is not a valid color", t.TextColor)
	}
	if !t.HorizontalAlign.valid() {
		return validationErr("horizontal align %q is not recognized", string(t.HorizontalAlign))
	}
	if !t.VerticalAlign.valid() {
		return validationErr("vertical align %q is not recognized", string(t.VerticalAlign))
	}
	if t.OutlineWidth < MinOutlineWidth || t.OutlineWidth > MaxOutlineWidth {
		return validationErr("outline width must be between %d and %d", MinOutlineWidth, MaxOutlineWidth)
	}
	if t.OutlineWidth > 0 && !IsValidColor(t.OutlineColor) {
		return validationErr("outline color %q is not a valid color", t.OutlineColor)
	}
	if t.ShadowEnabled {
		if t.ShadowOffsetX < MinShadowOffset || t.ShadowOffsetX > MaxShadowOffset {
			return validationErr("shadow offset x must be between %d and %d", MinShadowOffset, MaxShadowOffset)
		}
		if t.ShadowOffsetY < MinShadowOffset || t.ShadowOffsetY > MaxShadowOffset {
			return validationErr("shadow offset y must be between %d and %d", MinShadowOffset, MaxShadowOffset)
		}
		if t.ShadowBlur < MinShadowBlur || t.ShadowBlur > MaxShadowBlur {
			return validationErr("shadow blur must be between %d and %d", MinShadowBlur, MaxShadowBlur)
		}
		if !IsValidColor(t.ShadowColor) {
			return validationErr("shadow color %q is not a valid color", t.ShadowColor)
		}
	}
	return nil
}
