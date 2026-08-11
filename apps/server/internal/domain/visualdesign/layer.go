package visualdesign

// LayerKind is the closed, bounded set of layer primitives (Stage 13A
// task Part 10; message_fragments/badge_list added in Stage 13B, see
// docs/visual-designs.md §21). Deliberately small and shared - never an
// arbitrary/custom-media kind (uploaded image/video/audio/font layers
// remain explicitly out of scope; see this package's own doc comment
// and docs/visual-designs.md).
type LayerKind string

const (
	LayerShape        LayerKind = "shape"
	LayerText         LayerKind = "text"
	LayerPlatformIcon LayerKind = "platform_icon"
	LayerAvatar       LayerKind = "avatar"
	// LayerMessageFragments renders an item's own already-normalized,
	// already-ordered message fragments (text/emote/mention) - Version2
	// only (Stage 13B, docs/visual-designs.md §21).
	LayerMessageFragments LayerKind = "message_fragments"
	// LayerBadgeList renders an item's own already-resolved public badge
	// image DTOs - Version2 only (Stage 13B, docs/visual-designs.md §21).
	LayerBadgeList LayerKind = "badge_list"
	// LayerImage renders a managed, operator-uploaded or package-imported
	// image asset - Version3 only (Stage 14B,
	// docs/visual-template-packages.md §12). Always an opaque managed
	// asset reference (ImageProps.AssetID), never a filesystem path or
	// URL of any kind - see validation.go's validAssetRef.
	LayerImage LayerKind = "image"
	// LayerVideo renders a managed, operator-uploaded or package-imported
	// video asset - Version3 only (Stage 14B,
	// docs/visual-template-packages.md §12). Always rendered muted, with
	// no controls and no audio output, regardless of what the underlying
	// container holds - sound/audio playback is explicitly out of scope
	// for this package (Stage 17 owns the application's one audio
	// subsystem).
	LayerVideo LayerKind = "video"
)

var validLayerKinds = []LayerKind{
	LayerShape, LayerText, LayerPlatformIcon, LayerAvatar,
	LayerMessageFragments, LayerBadgeList, LayerImage, LayerVideo,
}

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
// may be bound to (Stage 13A task Part 10/21; timestamp/account_label
// added in Stage 13B, docs/visual-designs.md §20) - deliberately never
// an arbitrary object path ("event.user.name", "payload.foo.bar") or an
// expression language. This is one shared enum reused by both owners
// (see docs/visual-designs.md §20's own binding-meaning table) - never
// duplicated per owner; which values are actually *available* for a
// given owner/event-type/item-kind is a capability check that lives
// beside that owner's own domain package, never here.
type TextBinding string

const (
	// BindingStatic: fixed operator-authored text, never provider/user
	// content - see StaticText on TextProps.
	BindingStatic TextBinding = "static"
	// BindingAlertRenderedText: the rule's own Stage 12 text template,
	// already rendered - Stage 13A task Part 21's "a text layer can bind
	// to alert_rendered_text to display its rendered result," reusing
	// the Stage 12 template parser rather than replacing it. Alert-only:
	// never legal for a chat-overlay design (docs/visual-designs.md §20).
	BindingAlertRenderedText TextBinding = "alert_rendered_text"
	// BindingUsername: an alert's actor, or a chat item's own user
	// display name.
	BindingUsername TextBinding = "username"
	// BindingPlatform: the alert's or chat item's own provider.
	BindingPlatform TextBinding = "platform"
	// BindingEventType: an alert rule's own event type, or - reusing the
	// identical vocabulary - a chat activity item's own activityType
	// (docs/visual-designs.md §20's own table).
	BindingEventType TextBinding = "event_type"
	// BindingMessage: an alert's rendered message placeholder, or a chat
	// message item's own plain message text.
	BindingMessage TextBinding = "message"
	// BindingQuantity: bits/gift-count/redemption quantity for an alert,
	// or a chat activity item's own Activity.Quantity.
	BindingQuantity TextBinding = "quantity"
	// BindingGroupCount: an alert's own grouped-alert count. Not
	// available to chat (chat items are never grouped).
	BindingGroupCount TextBinding = "group_count"
	// BindingTimestamp: a chat item's own OccurredAt, formatted by the
	// shared renderer using a fixed, safe format - never a user-suppliable
	// format string. Chat-only (Stage 13B, docs/visual-designs.md §20).
	BindingTimestamp TextBinding = "timestamp"
	// BindingAccountLabel: a chat item's own AccountLabel, when the
	// owning profile's account-label setting resolved one. Chat-only
	// (Stage 13B, docs/visual-designs.md §20).
	BindingAccountLabel TextBinding = "account_label"
)

var validTextBindings = []TextBinding{
	BindingStatic, BindingAlertRenderedText, BindingUsername, BindingPlatform,
	BindingEventType, BindingMessage, BindingQuantity, BindingGroupCount,
	BindingTimestamp, BindingAccountLabel,
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

	FontFamily FontFamily
	// FontAssetID is an optional managed WOFF2 font asset reference
	// (Stage 14B, docs/visual-template-packages.md §11/§12) - empty means
	// "use FontFamily's own system fallback only", exactly Stage 13A's
	// existing behavior. When non-empty, the renderer still keeps
	// FontFamily as the safe fallback used the moment the custom font
	// asset fails to load - never persisted as, and never accepting,
	// arbitrary CSS `font-family` text (see validation.go's
	// validAssetRef).
	FontAssetID     string
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

// MessageFragmentsProps is LayerMessageFragments's own bounded payload
// (Stage 13B, docs/visual-designs.md §21) - renders an item's own
// already-normalized, already-ordered message fragments (plain text,
// resolved emote image, mention). No binding field: there is exactly
// one thing this kind can ever show. Never re-parses raw provider
// payload, never a provider request at render time, never
// dangerouslySetInnerHTML.
type MessageFragmentsProps struct {
	FontFamily FontFamily
	// FontAssetID: see TextProps.FontAssetID - identical optional managed
	// WOFF2 font asset reference, same fallback behavior.
	FontAssetID     string
	FontSize        int
	FontWeight      int
	LineHeight      float64
	LetterSpacing   float64
	TextColor       Color
	HorizontalAlign HorizontalAlign
	VerticalAlign   VerticalAlign
	// EmoteSize is the bounded rendered size (in design units) of a
	// resolved emote image fragment.
	EmoteSize int
}

// Bounds for MessageFragmentsProps (Stage 13B).
const (
	MinEmoteSize = 8
	MaxEmoteSize = 128
)

// BadgeListProps is LayerBadgeList's own bounded payload (Stage 13B,
// docs/visual-designs.md §21) - renders an item's own already-resolved
// public badge image DTOs. No binding field, and never an arbitrary URL
// stored in the design itself.
type BadgeListProps struct {
	// MaxCount bounds how many badges are ever rendered, even if the
	// item itself carries more.
	MaxCount int
	// BadgeSize is the bounded rendered size (in design units) of one
	// badge image.
	BadgeSize int
	// Gap is the bounded spacing (in design units) between badges.
	Gap int
}

// Bounds for BadgeListProps (Stage 13B).
const (
	MinBadgeCount = 1
	MaxBadgeCount = 20
	MinBadgeSize  = 8
	MaxBadgeSize  = 128
	MinBadgeGap   = 0
	MaxBadgeGap   = 32
)

// ImageFit is the closed fit enum shared by LayerImage and LayerVideo
// (Stage 14B, docs/visual-template-packages.md §12) - deliberately never
// an arbitrary CSS `object-fit` string.
type ImageFit string

const (
	FitContain ImageFit = "contain"
	FitCover   ImageFit = "cover"
)

var validImageFits = []ImageFit{FitContain, FitCover}

func (f ImageFit) valid() bool {
	for _, v := range validImageFits {
		if f == v {
			return true
		}
	}
	return false
}

// ImageProps is LayerImage's own bounded payload (Stage 14B,
// docs/visual-template-packages.md §12). AssetID is an opaque managed
// asset reference - never a filesystem path, http(s)/file/blob/data URL
// (see validation.go's validAssetRef). Alt is a short, plain-text
// accessibility label - never markup.
type ImageProps struct {
	AssetID string
	Fit     ImageFit
	Alt     string
}

// VideoProps is LayerVideo's own bounded payload (Stage 14B,
// docs/visual-template-packages.md §12/§20). AssetID is an opaque managed
// asset reference, exactly like ImageProps.AssetID. Loop is the only
// playback field an operator controls - the renderer otherwise always
// fixes autoplay-subject-to-reduced-motion/muted/no-controls/no-volume,
// regardless of what this struct contains; there is deliberately no
// volume, poster URL, track URL, or subtitle field.
type VideoProps struct {
	AssetID string
	Fit     ImageFit
	Loop    bool
}

// Bounds for ImageProps/VideoProps (Stage 14B).
const (
	MaxAltCodePoints = 200
)

// Layer is one bounded, ordered element of a Document (Stage 13A task
// Part 5/10; message_fragments/badge_list added in Stage 13B; image/video
// added in Stage 14B). Exactly one of
// Shape/Text/PlatformIcon/Avatar/MessageFragments/BadgeList/Image/Video is
// non-nil, matching Kind - see validation.go.
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

	Shape            *ShapeProps
	Text             *TextProps
	PlatformIcon     *PlatformIconProps
	Avatar           *AvatarProps
	MessageFragments *MessageFragmentsProps
	BadgeList        *BadgeListProps
	Image            *ImageProps
	Video            *VideoProps

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
