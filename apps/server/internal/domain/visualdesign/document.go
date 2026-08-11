// Package visualdesign holds the shared, provider-independent
// declarative visual-design document introduced in Stage 13A and
// extended in Stage 13B: a versioned, bounded tree of typed layers
// (shape/text/platform_icon/avatar/message_fragments/badge_list) an
// operator builds in either the Alert Overlay Designer or the Chat
// Overlay Designer, both of which reuse this exact same document/layer
// model and the one shared React renderer.
//
// Deliberately excludes anything owner-specific (an alert's or a chat
// overlay's own capability-driven text-binding availability, "generate
// a legacy-compatible draft from the current fixed presentation" logic,
// a chat design's own data-needs derivation) - all of that lives beside
// the owning domain (internal/domain/alerts's own visualdesign_binding.go
// / visualdesign_draft.go; internal/domain/chatoverlay's own
// visualdesign_binding.go / visualdesign_draft.go / visualdesign_dataneeds.go),
// which are each free to import this package one-directionally. This
// package never imports internal/domain/alerts, internal/domain/
// chatoverlay, internal/domain/engagement, internal/provider/twitch,
// internal/alerts, internal/chatoverlay, or internal/operatorchat -
// exactly like every other domain package in this project (see
// internal/domain/alerts/model.go's own doc comment for the same
// one-directional-dependency rule).
//
// See docs/visual-designs.md for the full document-format contract
// this package implements.
package visualdesign

// Version1 was Stage 13A's own original, and so far only, document
// schema version - four layer kinds (shape/text/platform_icon/avatar),
// eight text bindings. Version2 (Stage 13B) adds two shared layer kinds
// (message_fragments/badge_list, see layer.go) and two text bindings
// (timestamp/account_label) the Chat Overlay Designer needs - a
// version-1 document's own wire shape is unchanged in version 2 (see
// MigrateToCurrentVersion in migration.go), so every Stage 13A design
// loads and renders identically after migration.
const (
	Version1 = 1
	Version2 = 2
)

// CurrentVersion is the visual-design document schema version this
// package currently reads and writes. A document is rejected outright
// if its own Version does not equal CurrentVersion at the point it is
// validated - see MigrateToCurrentVersion (migration.go) for how an
// older *stored* row is transparently upgraded on read before it ever
// reaches Validate; a stale-version *write* is still always rejected.
const CurrentVersion = Version2

// OwnerKind is the closed, fixed set of entity kinds a visual design
// may belong to.
type OwnerKind string

// OwnerKindAlertRule: one visual design belongs to exactly one alert
// rule (Stage 13A task Part 18 - "in Stage 13A one saved visual design
// belongs to one alert rule," never many rules sharing one mutable
// design object).
const OwnerKindAlertRule OwnerKind = "alert_rule"

// OwnerKindChatOverlay: one visual design belongs to exactly one
// chat-overlay profile (Stage 13B, docs/visual-designs.md §18) - added
// alongside OwnerKindAlertRule in migration 0016, which widens
// visual_designs.owner_kind's own CHECK constraint (see that
// migration's own doc comment for why SQLite needs an explicit
// migration for this rather than accepting the value implicitly).
const OwnerKindChatOverlay OwnerKind = "chat_overlay"

// AcceptedOwnerKinds lists every OwnerKind this build of the service
// will actually persist a design for.
var AcceptedOwnerKinds = []OwnerKind{OwnerKindAlertRule, OwnerKindChatOverlay}

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

// CanvasChatItem is the Chat Overlay Designer's own canvas preset
// (Stage 13B, docs/visual-designs.md §17) - a chat visual design
// describes one repeated overlay item/card, never a full-screen
// presentation, so its own canvas is deliberately small and roughly
// horizontal: wide enough for a comfortable username+message
// combination at a normal font size, tall enough for a short wrapped
// message plus padding, chosen as the same order of magnitude as a
// real chat bubble's own today (see internal/chatoverlay's own
// intrinsically-sized item today - a Chat Overlay Designer card fixes
// that same rough shape into an absolute design-space size). Not
// owner-specific in the type system - a generic preset, like
// CanvasLandscape/CanvasVertical, that any owner could in principle
// use; it is simply the one the Chat Overlay Designer offers by
// default. Still governed by the same MinCanvasWidth/MaxCanvasWidth/
// MinCanvasHeight/MaxCanvasHeight bounds as every other canvas size.
var CanvasChatItem = Canvas{Width: 960, Height: 280, Transparent: true}

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
