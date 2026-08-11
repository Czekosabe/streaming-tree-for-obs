-- Stage 14B: managed visual asset persistence - the local-only
-- implementation of docs/visual-template-packages.md §13's four-table
-- model, backing internal/domain/visualasset. This is schema D from
-- that document's own "four independently versioned schemas" table -
-- never a portable file format, never shipped inside a
-- .streaming-tree-template package, purely a local SQLite detail.
--
-- Blob/metadata separation (docs/visual-template-packages.md §13):
-- visual_asset_blobs holds one row per immutable, content-addressed
-- binary payload (keyed by its own SHA-256, never by an application id)
-- - the actual bytes live only in the filesystem-backed blob store
-- (internal/domain/visualasset.FileStore), never in a SQLite column.
-- visual_assets holds one row per logical, operator-facing asset -
-- metadata only, pointing at exactly one blob. Two visual_assets rows
-- may point at the same blob_sha256 (content dedup: a package may
-- repeat an asset, a design and a template may reference identical
-- bytes) while carrying entirely different display_name/author/
-- license/notice - they are never merged just because their bytes
-- match (docs/visual-template-packages.md §27).
--
-- visual_design_asset_refs/visual_template_asset_refs are plain
-- reference-tracking join tables, rebuilt as a full replacement on
-- every design/template save (mirroring visual_designs' own
-- full-replacement Save semantics) - used only for the "in use" delete
-- guard (docs/visual-template-packages.md §15) and for identifying an
-- unreferenced blob during startup reconciliation (§16). design_id/
-- template_id reference visual_designs.id/visual_templates.id, each an
-- already-existing real primary key column (not the polymorphic
-- owner_kind/owner_id pair) - a design row is deleted and recreated by
-- id on every Save today, so ON DELETE CASCADE keeps a stale reference
-- row from ever outliving its own owner.
CREATE TABLE visual_asset_blobs (
    sha256 TEXT PRIMARY KEY,
    media_type TEXT NOT NULL,
    byte_size INTEGER NOT NULL CHECK (byte_size >= 0),
    storage_name TEXT NOT NULL,
    public_token TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL
);

CREATE TABLE visual_assets (
    id TEXT PRIMARY KEY,
    blob_sha256 TEXT NOT NULL REFERENCES visual_asset_blobs (sha256),
    kind TEXT NOT NULL CHECK (kind IN ('image', 'video', 'font')),
    display_name TEXT NOT NULL,
    author TEXT NOT NULL,
    license TEXT NOT NULL,
    notice TEXT NOT NULL,
    source TEXT NOT NULL CHECK (source IN ('upload', 'package')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX idx_visual_assets_blob_sha256 ON visual_assets (blob_sha256);

CREATE TABLE visual_design_asset_refs (
    design_id TEXT NOT NULL REFERENCES visual_designs (id) ON DELETE CASCADE,
    asset_id TEXT NOT NULL REFERENCES visual_assets (id),
    PRIMARY KEY (design_id, asset_id)
);
CREATE INDEX idx_visual_design_asset_refs_asset_id ON visual_design_asset_refs (asset_id);

CREATE TABLE visual_template_asset_refs (
    template_id TEXT NOT NULL REFERENCES visual_templates (id) ON DELETE CASCADE,
    asset_id TEXT NOT NULL REFERENCES visual_assets (id),
    PRIMARY KEY (template_id, asset_id)
);
CREATE INDEX idx_visual_template_asset_refs_asset_id ON visual_template_asset_refs (asset_id);
