// Package visualtemplate holds Stage 14A's reusable, portable visual
// design TEMPLATE: a named, described, licensed, provider-independent
// wrapper around a complete internal/domain/visualdesign.Document, one
// per alert-owner or chat-overlay target, either application-owned
// (built-in, immutable) or operator-owned (persisted, mutable
// metadata).
//
// A template is deliberately NOT a visual_designs row and never becomes
// one automatically: using a template loads its document into the
// Designer's own local, unsaved draft state (docs/visual-templates.md
// - "draft-first" application semantics); only the Designer's existing
// explicit Save persists a visual_designs row. There is no foreign key,
// no provenance link, and no live reference from a saved design back to
// the template it may once have come from.
//
// This package never imports internal/domain/alerts,
// internal/domain/chatoverlay, internal/domain/engagement,
// internal/provider/twitch, internal/alerts, internal/chatoverlay, or
// internal/operatorchat - owner-instance-specific compatibility (does
// this template suit THIS alert rule's own event type, or THIS chat
// overlay) is supplied by the caller as a narrow function value
// (compatibility.go's OwnerBindingCheck), never embedded here.
//
// See docs/visual-templates.md for the full format/library contract.
package visualtemplate

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/streaming-tree/server/internal/domain/visualdesign"
)

// Target is the closed set of owner categories a template may be built
// for - deliberately coarser than a specific alert rule or chat
// overlay (see compatibility.go for the finer, owner-instance check).
type Target string

const (
	TargetAlert Target = "alert"
	TargetChat  Target = "chat"
)

func (t Target) valid() bool {
	return t == TargetAlert || t == TargetChat
}

// Source distinguishes an application-owned, immutable built-in
// template from an operator-owned, persisted user template - a
// presentation/authorization concept only, never stored as its own
// database column (a built-in is never a database row at all; Source
// is derived from where a Template value came from).
type Source string

const (
	SourceBuiltin Source = "builtin"
	SourceUser    Source = "user"
)

// CurrentTemplateSchemaVersion is the only template-interchange schema
// version Stage 14A reads and writes - completely independent of
// visualdesign.CurrentVersion (docs/visual-templates.md: "Template
// schema v1 contains visual design document v2" - two different
// version counters for two different concerns).
const CurrentTemplateSchemaVersion = 1

// Format is the closed top-level discriminator every portable Stage 14A
// template file must carry, guarding against a client accidentally (or
// deliberately) importing an unrelated JSON file.
const Format = "streaming-tree-visual-template"

// Metadata bounds (Stage 14A task Part 9) - Unicode code points, not
// bytes, matching internal/domain/platform's own established
// utf8.RuneCountInString convention.
const (
	MinNameLen        = 1
	MaxNameLen        = 80
	MaxDescriptionLen = 400
	MaxAuthorLen      = 100
	MaxLicenseLen     = 120
)

// Template is the one shared Go value both a built-in and a persisted
// user template are represented as.
type Template struct {
	ID                    string
	Target                Target
	Source                Source
	Name                  string
	Description           string
	Author                string
	License               string
	TemplateSchemaVersion int
	Document              visualdesign.Document
	CreatedAt             time.Time
	UpdatedAt             time.Time

	// AlertAudio is Stage 17B's own template-level persistent-sound/TTS
	// preset (docs/alert-audio.md §10.5) - nil for every template that
	// never went through a package v2 import (every built-in, every
	// plain Stage 14A JSON create/import, and every existing template
	// migrated by this stage). Never legal for a TargetChat template
	// (validation.go).
	AlertAudio *RuleAudioPreset
}

// NewTemplateID returns a fresh, opaque, random local database id
// ("tpl_" + 16 random bytes hex-encoded) - never chosen by a client,
// never derived from an imported file's own metadata (Stage 14A task
// Part 10).
func NewTemplateID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate template id: %w", err)
	}
	return "tpl_" + hex.EncodeToString(buf), nil
}
