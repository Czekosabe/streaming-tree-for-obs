package visualdesign

import "sort"

// PublicDocument is the safe, bounded subset of a Document a public
// Browser Source renderer (or the Designer's own preview, which uses
// the identical shared React renderer - Stage 13A task Part 24) ever
// receives (Stage 13A task Part 17/44). Deliberately excludes:
//   - the design's own database id, owner kind/id, revision-management
//     metadata - none of it is presentation, all of it is
//     management-only,
//   - every layer's editor Name and Locked state,
//   - hidden layers entirely (never sent at all, not merely marked
//     hidden - Stage 13A task Part 33),
//   - internal validation metadata.
//
// Never constructed by hand outside ToPublic - always derived from a
// validated Document, so a caller can never accidentally leak an
// editor-only field by adding one to Document without also updating
// ToPublic.
type PublicDocument struct {
	SchemaVersion int
	Canvas        PublicCanvas
	Layers        []PublicLayer
}

type PublicCanvas struct {
	Width       int
	Height      int
	Transparent bool
}

type PublicLayer struct {
	// ID is kept public (unlike Name/Locked) - it is an opaque, never-
	// reused-for-anything-sensitive stable identifier the frontend
	// renderer uses as a React list key, not a secret or management
	// reference.
	ID   string
	Kind LayerKind

	Frame   Frame
	Opacity float64

	Shape            *ShapeProps
	Text             *TextProps
	PlatformIcon     *PlatformIconProps
	Avatar           *AvatarProps
	MessageFragments *MessageFragmentsProps
	BadgeList        *BadgeListProps

	EntryAnimation      Animation
	ExitAnimation       Animation
	AnimationDurationMS int
}

// ToPublic reduces doc to its safe PublicDocument projection, ordering
// layers by their own Order (ascending: back to front) and dropping
// every layer with Visible=false entirely. doc is assumed already
// Validate-d; ToPublic performs no validation of its own.
func ToPublic(doc Document) PublicDocument {
	visible := make([]Layer, 0, len(doc.Layers))
	for _, l := range doc.Layers {
		if l.Visible {
			visible = append(visible, l)
		}
	}
	sort.SliceStable(visible, func(i, j int) bool { return visible[i].Order < visible[j].Order })

	layers := make([]PublicLayer, 0, len(visible))
	for _, l := range visible {
		layers = append(layers, PublicLayer{
			ID: l.ID, Kind: l.Kind, Frame: l.Frame, Opacity: l.Opacity,
			Shape: l.Shape, Text: l.Text, PlatformIcon: l.PlatformIcon, Avatar: l.Avatar,
			MessageFragments: l.MessageFragments, BadgeList: l.BadgeList,
			EntryAnimation: l.EntryAnimation, ExitAnimation: l.ExitAnimation, AnimationDurationMS: l.AnimationDurationMS,
		})
	}
	return PublicDocument{
		SchemaVersion: doc.Version,
		Canvas:        PublicCanvas{Width: doc.Canvas.Width, Height: doc.Canvas.Height, Transparent: doc.Canvas.Transparent},
		Layers:        layers,
	}
}
