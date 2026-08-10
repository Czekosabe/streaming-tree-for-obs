// Package visualdesign holds the Stage 13A shared, provider-independent
// declarative visual-design document: a versioned, bounded tree of
// typed layers (shape/text/platform_icon/avatar) an operator builds in
// the Alert Overlay Designer, meant to be reused unchanged by the
// future Stage 13B Chat Overlay Designer.
//
// Deliberately excludes anything alert-specific (an alert's own
// capability-driven text-binding availability, the "generate a legacy-
// compatible draft from a Stage 12 rule" logic) - that lives beside the
// alert domain in internal/domain/alerts (see its own
// visualdesign_binding.go and visualdesign_draft.go), which is free to
// import this package one-directionally. This package never imports
// internal/domain/alerts, internal/domain/engagement,
// internal/provider/twitch, internal/alerts, or internal/operatorchat -
// exactly like every other domain package in this project (see
// internal/domain/alerts/model.go's own doc comment for the same
// one-directional-dependency rule).
//
// See docs/visual-designs.md for the full document-format contract
// this package implements.
package visualdesign

// CurrentVersion is the visual-design document schema version this
// package currently reads and writes. A document is rejected outright
// if its own Version does not equal CurrentVersion - Stage 13A defines
// no migration path yet (there is only ever one version so far); a
// future version bump gets its own explicit migration function here,
// never a silent best-effort reinterpretation of an older shape.
const CurrentVersion = 1

// OwnerKind is the closed, fixed set of entity kinds a visual design
// may belong to. Stage 13A accepts only OwnerKindAlertRule - see this
// package's own doc comment for why a second (chat-overlay) kind is
// deliberately NOT defined yet: exposing an unimplemented owner kind
// merely because Stage 13B is planned would be presenting unfinished
// work as real. Adding OwnerKindChatOverlay in Stage 13B is expected to
// require no change to this package beyond adding the new constant and
// extending AcceptedOwnerKinds.
type OwnerKind string

// OwnerKindAlertRule is the only accepted OwnerKind in Stage 13A: one
// visual design belongs to exactly one alert rule (Stage 13A task Part
// 18 - "in Stage 13A one saved visual design belongs to one alert
// rule," never many rules sharing one mutable design object).
const OwnerKindAlertRule OwnerKind = "alert_rule"

// AcceptedOwnerKinds lists every OwnerKind this build of the service
// will actually persist a design for - deliberately just the one, for
// now.
var AcceptedOwnerKinds = []OwnerKind{OwnerKindAlertRule}

func (k OwnerKind) valid() bool {
	for _, v := range AcceptedOwnerKinds {
		if k == v {
			return true
		}
	}
	return false
}

// Canvas is a design's fixed, bounded drawing surface (Stage 13A task
// Part 8). Width/Height are integer design units - never CSS pixels,
// never dependent on the OBS Browser Source's own actual viewport size
// (see internal/domain/alerts's own renderer-scaling doc, and
// docs/visual-designs.md's "coordinate system" section).
type Canvas struct {
	Width  int
	Height int
	// Transparent: true means the canvas background outside/between
	// opaque layers stays transparent (the normal case for a Browser
	// Source overlay) - false is reserved for a future non-transparent
	// preview context and is never itself an arbitrary color; Stage 13A
	// always saves true.
	Transparent bool
}

// Canvas bounds (Stage 13A task Part 8). CanvasLandscape/CanvasVertical
// are the two required presets; a custom size is accepted only within
// these same bounds.
const (
	MinCanvasWidth  = 320
	MaxCanvasWidth  = 3840
	MinCanvasHeight = 240
	MaxCanvasHeight = 3840
)

// CanvasLandscape and CanvasVertical are the two built-in canvas
// presets the Alert Designer's own canvas-preset control offers (Stage
// 13A task Part 38).
var (
	CanvasLandscape = Canvas{Width: 1920, Height: 1080, Transparent: true}
	CanvasVertical  = Canvas{Width: 1080, Height: 1920, Transparent: true}
)

// Document is one complete, versioned visual design: a canvas plus a
// bounded, ordered list of layers. This is the typed shape every
// persisted design is parsed into - a malformed or semantically invalid
// JSON document can never reach this struct (see validation.go), and
// this struct itself can never reach a renderer without first being
// reduced to the smaller PublicDocument (see public.go).
//
// Deliberately excludes every editor-transient concern (Stage 13A task
// Part 5): no selected-layer id, hover state, zoom level, undo/redo
// history, open-inspector-section, or in-progress pointer-drag
// coordinates. All of that is frontend-only React state that never
// reaches this struct, let alone SQLite.
type Document struct {
	Version int
	Canvas  Canvas
	Layers  []Layer
}

// Document-wide bounds (Stage 13A task Part 16).
const (
	MaxLayers        = 50
	MaxDocumentBytes = 64 * 1024
)
