package chatoverlay

import (
	"fmt"

	"github.com/streaming-tree/server/internal/domain/visualdesign"
)

// ItemKind mirrors internal/chatoverlay.Kind (message vs activity) at
// the domain layer, without this package importing that runtime
// package (see this file's own reasoning: capability checks belong
// beside the owning domain, never inside the shared visualdesign
// package - docs/visual-designs.md §20).
type ItemKind string

const (
	ItemKindMessage  ItemKind = "message"
	ItemKindActivity ItemKind = "activity"
)

// AvailableTextBindings returns the subset of visualdesign's own closed
// TextBinding vocabulary that actually makes sense for a chat-overlay
// design, given itemKind (Stage 13B, docs/visual-designs.md §20/§20.1).
// A saved chat design commonly targets *both* item kinds at once (a
// message layer and an activity-only layer coexisting in one document),
// so this always returns the union available to itemKind - which
// specific layer actually renders for a specific item at request time
// is governed by missing-value/hide behavior, not validation-time
// rejection.
//
// BindingAlertRenderedText is never returned for any item kind - it is
// alert-only (docs/visual-designs.md §20).
func AvailableTextBindings(itemKind ItemKind) []visualdesign.TextBinding {
	out := []visualdesign.TextBinding{
		visualdesign.BindingStatic, visualdesign.BindingUsername, visualdesign.BindingPlatform,
		visualdesign.BindingTimestamp, visualdesign.BindingAccountLabel,
	}
	switch itemKind {
	case ItemKindMessage:
		out = append(out, visualdesign.BindingMessage)
	case ItemKindActivity:
		out = append(out, visualdesign.BindingEventType, visualdesign.BindingQuantity)
	}
	return out
}

// availableTextBindingsAnyItemKind is the union across every item kind -
// used by ValidateDesignBindingsForChatOverlay, since one saved design
// may legitimately contain layers meant for different item kinds (Stage
// 13B, docs/visual-designs.md §20.1: "Stage 13B does not create a
// separate persisted design per item kind").
func availableTextBindingsAnyItemKind() []visualdesign.TextBinding {
	seen := make(map[visualdesign.TextBinding]bool)
	var out []visualdesign.TextBinding
	for _, kind := range []ItemKind{ItemKindMessage, ItemKindActivity} {
		for _, b := range AvailableTextBindings(kind) {
			if !seen[b] {
				seen[b] = true
				out = append(out, b)
			}
		}
	}
	return out
}

// ValidateDesignBindingsForChatOverlay rejects a saved chat-overlay
// design containing any text layer bound to a value that could never be
// meaningful for a chat item of any kind - most importantly,
// BindingAlertRenderedText and BindingGroupCount, which are alert-only
// (Stage 13B, docs/visual-designs.md §20).
func ValidateDesignBindingsForChatOverlay(doc visualdesign.Document) error {
	available := availableTextBindingsAnyItemKind()
	for _, l := range doc.Layers {
		if l.Kind != visualdesign.LayerText || l.Text == nil {
			continue
		}
		ok := false
		for _, a := range available {
			if a == l.Text.Binding {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("%w: text layer %q binding %q is not available for a chat-overlay design", visualdesign.ErrValidation, l.ID, string(l.Text.Binding))
		}
	}
	return nil
}
