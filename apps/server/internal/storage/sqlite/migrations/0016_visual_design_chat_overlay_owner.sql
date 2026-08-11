-- Stage 13B: widen visual_designs.owner_kind to also accept
-- 'chat_overlay', alongside the existing 'alert_rule' (Stage 13A,
-- migration 0015). See docs/visual-designs.md §10/§18 for why this
-- needs its own migration even though the table shape and
-- document_json are already owner-agnostic: SQLite has no
-- `ALTER TABLE ... ALTER CONSTRAINT`, so a literal `CHECK (owner_kind
-- IN (...))` list can only be widened by the standard safe rebuild
-- pattern - create the replacement table, copy every existing row
-- across unchanged, drop the old table, rename the new one into
-- place, all inside this migration's own transaction (see
-- internal/storage/sqlite's own migration runner for the
-- transaction boundary).
--
-- Every existing row's id, owner_kind, owner_id, schema_version,
-- document_json, revision, created_at and updated_at survives this
-- migration byte-for-byte - this statement only copies columns, it
-- never re-parses or rewrites document_json.

CREATE TABLE visual_designs_new (
    id TEXT PRIMARY KEY,
    owner_kind TEXT NOT NULL CHECK (owner_kind IN ('alert_rule', 'chat_overlay')),
    owner_id TEXT NOT NULL,
    schema_version INTEGER NOT NULL,
    document_json TEXT NOT NULL,
    revision INTEGER NOT NULL CHECK (revision >= 1),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (owner_kind, owner_id)
);

INSERT INTO visual_designs_new (id, owner_kind, owner_id, schema_version, document_json, revision, created_at, updated_at)
    SELECT id, owner_kind, owner_id, schema_version, document_json, revision, created_at, updated_at
    FROM visual_designs;

DROP TABLE visual_designs;

ALTER TABLE visual_designs_new RENAME TO visual_designs;
