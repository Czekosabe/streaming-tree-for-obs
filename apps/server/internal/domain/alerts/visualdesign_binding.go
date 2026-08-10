package alerts

import (
	"fmt"

	"github.com/streaming-tree/server/internal/domain/visualdesign"
)

// AvailableTextBindings returns the subset of visualdesign's own closed
// TextBinding vocabulary that actually makes sense for t (Stage 13A
// task Part 11) - the visual-design equivalent of AvailablePlaceholders
// in internal/alerts/templates.go, driven by the exact same
// CapabilityFor/GroupingCapabilityFor tables so the two systems can
// never silently drift apart. BindingStatic and BindingAlertRenderedText
// are always available (a static string and the rule's own Stage 12
// template are meaningful for every event type).
func AvailableTextBindings(t EventType) []visualdesign.TextBinding {
	capability := CapabilityFor(t)
	out := []visualdesign.TextBinding{
		visualdesign.BindingStatic, visualdesign.BindingAlertRenderedText,
		visualdesign.BindingPlatform, visualdesign.BindingEventType,
	}
	if capability.HasUser {
		out = append(out, visualdesign.BindingUsername)
	}
	if capability.HasQuantity {
		out = append(out, visualdesign.BindingQuantity)
	}
	if capability.HasMessage {
		out = append(out, visualdesign.BindingMessage)
	}
	if GroupingCapabilityFor(t).Groupable {
		out = append(out, visualdesign.BindingGroupCount)
	}
	return out
}

// ValidateDesignBindingsForEventType rejects a saved design containing
// any text layer bound to a value not available for t (Stage 13A task
// Part 11: "the designer must preview missing-data behavior honestly"
// - enforced here as "a binding that could NEVER resolve for this rule
// is rejected outright at save time", the design-document analogue of
// internal/alerts.ValidateTemplateForEventType).
func ValidateDesignBindingsForEventType(doc visualdesign.Document, t EventType) error {
	available := AvailableTextBindings(t)
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
			return fmt.Errorf("%w: text layer %q binding %q is not available for event type %q", ErrConditionUnsupported, l.ID, string(l.Text.Binding), string(t))
		}
	}
	return nil
}
