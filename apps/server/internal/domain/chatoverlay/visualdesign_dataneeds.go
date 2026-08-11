package chatoverlay

import "github.com/streaming-tree/server/internal/domain/visualdesign"

// ChatDataNeeds is a typed, server-side assessment of which optional
// public item fields a saved chat-overlay design's own layers actually
// need to render correctly (Stage 13B, docs/visual-designs.md §22).
// Derived only from the saved design - never from a legacy show/hide
// toggle - so an active design's layers are never silently starved of
// data an unrelated legacy setting happens to have turned off.
//
// This can only ever *add* fields already governed by privacy/
// filtering upstream of it; it never bypasses a blocked term, a hidden
// user, moderation removal, or account selection, all of which are
// decided before an item ever reaches the point this assessment is
// consulted.
type ChatDataNeeds struct {
	Avatar       bool
	Badges       bool
	AccountLabel bool
}

// DeriveDataNeeds walks doc's own layers once and reports which
// optional public item fields they need. doc is assumed already
// Validate-d.
func DeriveDataNeeds(doc visualdesign.Document) ChatDataNeeds {
	var needs ChatDataNeeds
	for _, l := range doc.Layers {
		switch l.Kind {
		case visualdesign.LayerAvatar:
			needs.Avatar = true
		case visualdesign.LayerBadgeList:
			needs.Badges = true
		case visualdesign.LayerText:
			if l.Text != nil && l.Text.Binding == visualdesign.BindingAccountLabel {
				needs.AccountLabel = true
			}
		}
	}
	return needs
}
