package visualtemplate

import (
	"fmt"
	"unicode/utf8"

	"github.com/streaming-tree/server/internal/domain/visualdesign"
)

func validationErr(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrValidation, fmt.Sprintf(format, args...))
}

// NormalizeAndValidateDocument migrates doc to visualdesign.CurrentVersion
// (a no-op if it is already there) and validates the result - the one
// shared entry point every template creation/import path uses (Stage
// 14A task Part 8): a supported older v1 document is transparently
// upgraded; a document at an unknown/future/malformed version is
// rejected outright, never silently reinterpreted.
func NormalizeAndValidateDocument(doc visualdesign.Document) (visualdesign.Document, error) {
	if doc.Version != visualdesign.Version1 && doc.Version != visualdesign.CurrentVersion {
		return visualdesign.Document{}, fmt.Errorf("%w: version %d", ErrUnsupportedDesignVersion, doc.Version)
	}
	migrated := visualdesign.MigrateToCurrentVersion(doc)
	if migrated.Version != visualdesign.CurrentVersion {
		return visualdesign.Document{}, fmt.Errorf("%w: version %d", ErrUnsupportedDesignVersion, migrated.Version)
	}
	if err := visualdesign.Validate(migrated); err != nil {
		return visualdesign.Document{}, fmt.Errorf("%w: %v", ErrValidation, err)
	}
	return migrated, nil
}

// Validate checks t's own template-level fields: target enum, bounded
// Unicode-code-point metadata, template schema version, and (via
// NormalizeAndValidateDocument) the embedded visual-design document.
// It does NOT check owner-instance compatibility (a specific alert
// rule's event type, or a specific chat overlay) - see compatibility.go.
//
// t.Document must already be the normalized, validated document
// NormalizeAndValidateDocument returned - Validate re-checks it defensively
// but callers should not skip that step, since Validate alone cannot
// perform the version migration.
func Validate(t Template) error {
	if !t.Target.valid() {
		return validationErr("target %q is not a recognized template target", string(t.Target))
	}
	if t.TemplateSchemaVersion != CurrentTemplateSchemaVersion {
		return fmt.Errorf("%w: template schema version %d (expected %d)", ErrUnsupportedTemplateVersion, t.TemplateSchemaVersion, CurrentTemplateSchemaVersion)
	}
	nameLen := utf8.RuneCountInString(t.Name)
	if nameLen < MinNameLen || nameLen > MaxNameLen {
		return validationErr("name must be %d-%d Unicode code points, got %d", MinNameLen, MaxNameLen, nameLen)
	}
	if utf8.RuneCountInString(t.Description) > MaxDescriptionLen {
		return validationErr("description must not exceed %d Unicode code points", MaxDescriptionLen)
	}
	if utf8.RuneCountInString(t.Author) > MaxAuthorLen {
		return validationErr("author must not exceed %d Unicode code points", MaxAuthorLen)
	}
	if utf8.RuneCountInString(t.License) > MaxLicenseLen {
		return validationErr("license must not exceed %d Unicode code points", MaxLicenseLen)
	}
	if _, err := NormalizeAndValidateDocument(t.Document); err != nil {
		return err
	}
	// A target-generic (not owner-instance-specific) hard rule: Stage
	// 12's own rendered-text template placeholder has no chat
	// equivalent and is never legal for a chat-overlay design at all,
	// regardless of which specific overlay (docs/visual-designs.md
	// §20). This is the one binding-legality check narrow enough to be
	// stable across every possible owner instance of a given target,
	// so it belongs here rather than in the per-owner compatibility
	// check (compatibility.go).
	if t.Target == TargetChat {
		for _, l := range t.Document.Layers {
			if l.Kind == visualdesign.LayerText && l.Text != nil && l.Text.Binding == visualdesign.BindingAlertRenderedText {
				return validationErr("text layer %q uses alert_rendered_text, which is never valid for a chat target", l.ID)
			}
		}
	}
	return nil
}
