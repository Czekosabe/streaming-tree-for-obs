package visualtemplate

import "github.com/streaming-tree/server/internal/domain/visualdesign"

// Stable compatibility blocker codes (Stage 14A task Part 13/44) - a
// frontend must treat these as opaque, translatable codes, never parse
// prose.
const (
	BlockerTargetMismatch          = "template_target_mismatch"
	BlockerAlertBindingUnavailable = "alert_binding_unavailable"
	BlockerChatBindingUnavailable  = "chat_binding_unavailable"
	BlockerUnsupportedDocument     = "unsupported_visual_document"
	BlockerInvalidDocument         = "visual_document_invalid"
)

// Compatibility is the one provider-independent assessment result
// returned for "can this template be used as a draft for this specific
// owner" (Stage 14A task Part 13) - authoritative on the backend;
// the frontend never re-derives it.
type Compatibility struct {
	Compatible bool
	Blockers   []string
}

// OwnerBindingCheck validates whether doc's own text-layer bindings are
// legal for one specific owner instance - an alert rule's own event
// type, or a chat overlay. Supplied by the caller (internal/httpapi,
// which already has authenticated access to both
// internal/domain/alerts and internal/domain/chatoverlay); this
// package stays independent of both (see this package's own doc
// comment). A nil check means "no specific owner instance was given -
// assess target-level compatibility only".
type OwnerBindingCheck func(doc visualdesign.Document) error

// AssessCompatibility checks tpl against forTarget (and, if check is
// non-nil, against one specific owner instance's own binding
// availability). Never mutates tpl.
func AssessCompatibility(tpl Template, forTarget Target, check OwnerBindingCheck) Compatibility {
	if tpl.Target != forTarget {
		return Compatibility{Compatible: false, Blockers: []string{BlockerTargetMismatch}}
	}
	if tpl.Document.Version != visualdesign.CurrentVersion {
		return Compatibility{Compatible: false, Blockers: []string{BlockerUnsupportedDocument}}
	}
	if err := visualdesign.Validate(tpl.Document); err != nil {
		return Compatibility{Compatible: false, Blockers: []string{BlockerInvalidDocument}}
	}
	if check != nil {
		if err := check(tpl.Document); err != nil {
			blocker := BlockerAlertBindingUnavailable
			if forTarget == TargetChat {
				blocker = BlockerChatBindingUnavailable
			}
			return Compatibility{Compatible: false, Blockers: []string{blocker}}
		}
	}
	return Compatibility{Compatible: true}
}
