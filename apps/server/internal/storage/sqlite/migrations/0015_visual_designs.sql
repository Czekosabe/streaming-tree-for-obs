-- Stage 13A: the shared, provider-independent visual-design document -
-- one persisted design per (owner_kind, owner_id) pair. See
-- docs/visual-designs.md for the full document-format contract and
-- internal/domain/visualdesign for the typed Go model this JSON column
-- is always parsed into/validated against before ever reaching a
-- renderer.
--
-- document_json is a versioned JSON document rather than a normalized
-- column-per-field table, deliberately: a layer tree is a dynamic,
-- ordered, variant-shaped structure (unlike alert_rules' own fixed set
-- of columns), and every write is fully re-validated against the typed
-- Go struct (internal/domain/visualdesign.Validate) before it is ever
-- persisted or read back - this is never an arbitrary CSS/HTML/JS
-- string a viewer's browser would render unsanitized (see
-- internal/domain/chatoverlay's own migration for the project's
-- general "never a settings blob" rule; visual_designs is the one
-- deliberate, narrow exception, justified by the shape of the data it
-- holds, not a departure from the underlying safety principle - the
-- JSON is still fully typed/bounded/validated, never interpreted as
-- markup or code).
--
-- owner_kind/owner_id are a deliberately polymorphic pair rather than a
-- single-table foreign key (Stage 13A task Part 6/18) - only
-- 'alert_rule' is accepted at the application layer today
-- (internal/domain/visualdesign.AcceptedOwnerKinds), enforced again
-- here defensively via CHECK, but the column shape is written so a
-- future owner kind (Stage 13B's chat overlays) can share this exact
-- table without a schema change. Cascade-on-owner-delete is therefore
-- an explicit application-level call
-- (internal/alerts.Manager.DeleteRule), never a SQL foreign key, since
-- a polymorphic owner_id cannot reference a single parent table.
CREATE TABLE visual_designs (
    id TEXT PRIMARY KEY,
    owner_kind TEXT NOT NULL CHECK (owner_kind IN ('alert_rule')),
    owner_id TEXT NOT NULL,
    schema_version INTEGER NOT NULL,
    document_json TEXT NOT NULL,
    revision INTEGER NOT NULL CHECK (revision >= 1),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (owner_kind, owner_id)
);
