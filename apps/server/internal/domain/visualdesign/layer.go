package visualdesign

// LayerKind is the closed, bounded set of Stage 13A layer primitives
// (Stage 13A task Part 10). Deliberately small and shared enough to be
// reused unchanged by the future Stage 13B Chat Overlay Designer -
// never an arbitrary/custom-media kind (uploaded image/video/audio/
// font layers are explicitly out of Stage 13A's scope; see this
// package's own doc comment and docs/visual-designs.md).
type LayerKind string

const (
	LayerShape        LayerKind = "shape"
	LayerText         LayerKind = "text"
	LayerPlatformIcon LayerKind = "platform_icon"
	LayerAvatar       LayerKind = "avatar"
)

var validLayerKinds = []LayerKind{LayerShape, LayerText, LayerPlatformIcon, LayerAvatar}

func (k LayerKind) valid() bool {
	for _, v := range validLayerKinds {
		if k == v {
			return true
		}
	}
	return false
}

// Animation is the closed, application-owned animation-class enum -
// deliberately its own type, never internal/domain/alerts.Animation
// (this package must never import that one-directional dependency in
// reverse - see the package doc comment), even though the two share the
// same 5 values today by design, mirroring the same "none/fade/
// slide_up/slide_left/scale" vocabulary already used for both alerts
// and the chat overlay (Stage 13A task Part 15: "reuse the existing
// application-owned safe animation vocabulary").
type Animation string

const (
	AnimationNone      Animation = "none"
	AnimationFade      Animation = "fade"
	AnimationSlideUp   Animation = "slide_up"
	AnimationSlideLeft Animation = "slide_left"
	AnimationScale     Animation = "scale"
)

var validAnimations = []Animation{AnimationNone, AnimationFade, AnimationSlideUp, AnimationSlideLeft, AnimationScale}

func (a Animation) valid() bool {
	for _, v := range validAnimations {
		if a == v {
			return true
		}
	}
	return false
}

// TextBinding is the closed, fixed vocabulary a text layer's content
// may be bound to (Stage 13A task Part 10/21) - deliberately never an
// arbitrary object path ("event.user.name", "payload.foo.bar") or an
// expression language. Every value except Static/AlertRenderedText
// mirrors one of internal/alerts's own KnownPlaceholders one-to-one, on
// purpose, so both systems describe the same underlying alert data.
type TextBinding string

const (
	// BindingStatic: fixed operator-authored text, never provider/user
	// content - see StaticText on TextProps.
	BindingStatic TextBinding = "static"
	// BindingAlertRenderedText: the rule's own Stage 12 text template,
	// already rendered - Stage 13A task Part 21's "a text layer can bind
	// to alert_rendered_text to display its rendered result," reusing
	// the Stage 12 template parser rather than replacing it.
	BindingAlertRenderedText TextBinding = "alert_rendered_text"
	BindingUsername          TextBinding = "username"
	BindingPlatform          TextBinding = "platform"
	BindingEventType         TextBinding = "event_type"
	BindingMessage           TextBinding = "message"
	BindingQuantity          TextBinding = "quantity"
	BindingGroupCount        TextBinding = "group_count"
)

var validTextBindings = []TextBinding{
	BindingStatic, BindingAlertRenderedText, BindingUsername, BindingPlatform,
	BindingEventType, BindingMessage, BindingQuantity, BindingGroupCount,
}

func (b TextBinding) valid() bool {
	for _, v := range validTextBindings {
		if b == v {
			return true
		}
	}
	return false
}

// MissingValueBehavior is the closed policy for what a data-bound layer
// does when its bound value is absent for a particular alert (Stage 13A
// task Part 12) - never a fabricated string like "undefined"/"null"/the
// literal placeholder token.
type MissingValueBehavior string

const (
	// MissingHide: the layer is not rendered at all for this alert - the
	// only behavior a real, public alert ever uses.
	MissingHide MissingValueBehavior = "hide"
	// MissingPlaceholder: the editor preview shows an obviously synthetic
	// missing-data indicator instead - EDITOR-ONLY, never sent to a real
	// broadcast (the public renderer always treats a data-bound layer
	// with an absent value as MissingHide, regardless of this saved
	// preference - see the shared React renderer, which is deliberately
	// the one place this distinction is actually enforced, since the
	// same document/component renders both the editor preview and the
	// public route).
	MissingPlaceholder MissingValueBehavior = "placeholder"
)

var validMissingValueBehaviors = []MissingValueBehavior{MissingHide, MissingPlaceholder}

func (m MissingValueBehavior) valid() bool {
	for _, v := range validMissingValueBehaviors {
		if m == v {
			return true
		}
	}
	return false
}

// FontFamily is the closed system-font allowlist (Stage 13A task Part
// 13) - never an arbitrary font-family string, never an uploaded or
// remote font.
type FontFamily string

const (
	FontSystemUI  FontFamily = "system-ui"
	FontSansSerif FontFamily = "sans-serif"
	FontSerif     FontFamily = "serif"
	FontMonospace FontFamily = "monospace"
)

var validFontFamilies = []FontFamily{FontSystemUI, FontSansSerif, FontSerif, FontMonospace}

func (f FontFamily) valid() bool {
	for _, v := range validFontFamilies {
		if f == v {
			return true
		}
	}
	return false
}

// HorizontalAlign / VerticalAlign are a text layer's closed alignment
// enums (Stage 13A task Part 13).
type HorizontalAlign string

const (
	HAlignLeft   HorizontalAlign = "left"
	HAlignCenter HorizontalAlign = "center"
	HAlignRight  HorizontalAlign = "right"
)

var validHAligns = []HorizontalAlign{HAlignLeft, HAlignCenter, HAlignRight}

func (a HorizontalAlign) valid() bool {
	for _, v := range validHAligns {
		if a == v {
			return true
		}
	}
	return false
}

type VerticalAlign string

const (
	VAlignTop    VerticalAlign = "top"
	VAlignMiddle VerticalAlign = "middle"
	VAlignBottom VerticalAlign = "bottom"
)

var validVAligns = []VerticalAlign{VAlignTop, VAlignMiddle, VAlignBottom}

func (a VerticalAlign) valid() bool {
	for _, v := range validVAligns {
		if a == v {
			return true
		}
	}
	return false
}

// ShapeKind is the closed shape-primitive enum. Only "rectangle" is
// implemented in Stage 13A (Stage 13A task Part 10) - kept as its own
// closed type rather than hardcoding, so a future additional shape
// (e.g. "ellipse") extends one allowlist rather than changing the
// LayerShape kind's own meaning.
type ShapeKind string

const ShapeRectangle ShapeKind = "rectangle"

var validShapeKinds = []ShapeKind{ShapeRectangle}

func (k ShapeKind) valid() bool {
	for _, v := range validShapeKinds {
		if k == v {
			return true
		}
	}
	return false
}

// Frame is a layer's bounded position and size in integer design units
// (Stage 13A task Part 8). Always required to remain fully inside its
// document's own Canvas, and no smaller than MinLayerSize on either
// axis - see validation.go.
type Frame struct {
	X      int
	Y      int
	Width  int
	Height int
}

// MinLayerSize is the minimum width/height a layer's Frame may have
// (Stage 13A task Part 8).
const MinLayerSize = 8

// Color is a normalized, validated hex color string - "#RRGGBB" or
// "#RRGGBBAA" only (Stage 13A task Part 13). Never a CSS color name,
// `rgb()`/`hsl()` function, or CSS variable.
type Color = string

// ShapeProps is LayerShape's own bounded, kind-specific payload (Stage
// 13A task Part 10). Deliberately solid-color only - no gradients (the
// task's own explicit "no gradients unless a closed, fully validated
// enum without CSS passthrough" was not attempted here; plain fill
// stays simplest and safest for Stage 13A).
type ShapeProps struct {
	Kind         ShapeKind
	Fill         Color
	BorderColor  Color
	BorderWidth  int
	CornerRadius int
}

// Bounds for ShapeProps (Stage 13A task Part 14).
const (
	MinBorderWidth  = 0
	MaxBorderWidth  = 32
	MinCornerRadius = 0
	MaxCornerRadius = 500
)

// TextProps is LayerText's own bounded, kind-specific payload (Stage
// 13A task Part 10/13/21). StaticText is only meaningful (and only
// validated as non-empty) when Binding is BindingStatic - it is
// otherwise ignored and should be saved empty by a well-behaved client,
// though a non-empty value is harmless and never rendered when Binding
// is not BindingStatic.
type TextProps struct {
	Binding              TextBinding
	StaticText           string
	MissingValueBehavior MissingValueBehavior

	FontFamily      FontFamily
	FontSize        int
	FontWeight      int
	LineHeight      float64
	LetterSpacing   float64
	TextColor       Color
	HorizontalAlign HorizontalAlign
	VerticalAlign   VerticalAlign

	OutlineWidth int
	OutlineColor Color

	ShadowEnabled bool
	ShadowOffsetX int
	ShadowOffsetY int
	ShadowBlur    int
	ShadowColor   Color
}

// Bounds for TextProps (Stage 13A task Part 14/16).
const (
	MaxStaticTextCodePoints = 500

	MinFontSize = 8
	MaxFontSize = 300

	MinFontWeight  = 100
	MaxFontWeight  = 900
	FontWeightStep = 100

	MinLineHeight = 0.8
	MaxLineHeight = 3.0

	MinLetterSpacing = -2.0
	MaxLetterSpacing = 20.0

	MinOutlineWidth = 0
	MaxOutlineWidth = 16

	MinShadowOffset = -32
	MaxShadowOffset = 32
	MinShadowBlur   = 0
	MaxShadowBlur   = 64
)

// PlatformIconProps is LayerPlatformIcon's own payload - deliberately
// empty: it uses only the application-owned provider glyph mapping
// (Stage 13A task Part 47), never an arbitrary icon URL. Frame/Opacity
// (on Layer itself) fully determine its placement and size.
type PlatformIconProps struct{}

// AvatarProps is LayerAvatar's own bounded, kind-specific payload
// (Stage 13A task Part 46) - placement/style only, never an arbitrary
// URL (the safe normalized avatar URL already present on the public
// alert item is the only image source, resolved entirely at render
// time from the alert's own data, never stored in the design itself).
type AvatarProps struct {
	CornerRadius int
	BorderColor  Color
	BorderWidth  int
}

// Layer is one bounded, ordered element of a Document (Stage 13A task
// Part 5/10). Exactly one of Shape/Text/PlatformIcon/Avatar is non-nil,
// matching Kind - see validation.go.
type Layer struct {
	ID   string
	Name string
	Kind LayerKind

	Visible bool
	Locked  bool
	// Order is this layer's explicit z-order position - ascending, back
	// to front (Stage 13A task Part 32). Persisted as ordinary integers;
	// never inferred from array/DOM position at render time. The
	// service normalizes Order to a dense 0..N-1 sequence on every save
	// (see service.go), so callers never need to reason about gaps.
	Order int

	Frame   Frame
	Opacity float64

	Shape        *ShapeProps
	Text         *TextProps
	PlatformIcon *PlatformIconProps
	Avatar       *AvatarProps

	EntryAnimation      Animation
	ExitAnimation       Animation
	AnimationDurationMS int
}

// Bounds shared across every layer kind (Stage 13A task Part 14/15/16).
const (
	MaxLayerNameCodePoints = 80

	MinOpacity = 0.0
	MaxOpacity = 1.0

	MinLayerAnimationDurationMS = 0
	MaxLayerAnimationDurationMS = 2000
)
