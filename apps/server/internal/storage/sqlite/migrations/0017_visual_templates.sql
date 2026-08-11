-- Stage 14A: a reusable, portable visual-design TEMPLATE library -
-- distinct from visual_designs (migration 0015/0016), which persists
-- the one CURRENT design an alert rule or chat overlay actually renders
-- through. A template is a reusable, independently-owned starting
-- point: creating a design draft from a template copies its document
-- into the Designer's own local draft state and never links back here
-- (docs/visual-templates.md; docs/visual-designs.md's own "no live
-- reference from an owner design back to a template" rule).
--
-- Only USER-created templates (via "Save as template" or a completed
-- JSON import) live in this table. Built-in templates are
-- application-owned, immutable, reviewed constants
-- (internal/domain/visualtemplate/builtin.go) - never rows here, never
-- downloaded, never fetched at runtime.
--
-- document_json stores a normalized visualdesign.Document at
-- CurrentVersion, using the same "typed, validated JSON column" pattern
-- visual_designs already established (see that migration's own
-- comment for the general reasoning) - never raw imported file bytes,
-- never a file path, never export/preview history.
CREATE TABLE visual_templates (
    id TEXT PRIMARY KEY,
    target_kind TEXT NOT NULL CHECK (target_kind IN ('alert', 'chat')),
    name TEXT NOT NULL,
    description TEXT NOT NULL,
    author TEXT NOT NULL,
    license TEXT NOT NULL,
    template_schema_version INTEGER NOT NULL,
    document_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
