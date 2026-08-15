-- Stage 17B: managed persistent audio asset persistence
-- (docs/alert-audio.md §5), backing internal/domain/audioasset. Mirrors
-- 0018_visual_assets.sql's own four-table blob/metadata/reference-tracking
-- shape exactly, applied to audio instead of images/video/fonts - a
-- deliberately separate table family, never merged into visual_assets
-- (docs/alert-audio.md §5.1: audioasset reuses visualasset's own generic
-- *FileStore* blob-storage primitive directly, but keeps its own
-- independent metadata/reference-tracking schema).
--
-- audioasset_blobs holds one row per immutable, content-addressed binary
-- payload (keyed by its own SHA-256) - the actual bytes live only in the
-- filesystem-backed blob store (a second visualasset.FileStore instance
-- rooted at a sibling directory), never in a SQLite column.
-- audioasset_assets holds one row per logical, operator-facing asset -
-- metadata only, pointing at exactly one blob. Two audioasset_assets rows
-- may point at the same blob_sha256 (content dedup) while carrying
-- different display_name - they are never merged just because their
-- bytes match, mirroring 0018_visual_assets.sql's own identical
-- reasoning.
--
-- alert_rule_audio_asset_refs/alert_template_audio_asset_refs are plain
-- reference-tracking join tables, rebuilt as a full replacement on every
-- rule/template save - used only for the "in use" delete guard
-- (docs/alert-audio.md §5.6) and for identifying an unreferenced blob
-- during startup reconciliation (§5.7). rule_id/template_id reference
-- alert_rules.id/visual_templates.id, each an already-existing real
-- primary key column.
CREATE TABLE audioasset_blobs (
    sha256 TEXT PRIMARY KEY,
    media_type TEXT NOT NULL,
    byte_size INTEGER NOT NULL CHECK (byte_size >= 0),
    duration_ms INTEGER NOT NULL CHECK (duration_ms >= 0),
    storage_name TEXT NOT NULL,
    public_token TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL
);

CREATE TABLE audioasset_assets (
    id TEXT PRIMARY KEY,
    blob_sha256 TEXT NOT NULL REFERENCES audioasset_blobs (sha256),
    kind TEXT NOT NULL CHECK (kind IN ('sound')),
    display_name TEXT NOT NULL,
    source TEXT NOT NULL CHECK (source IN ('upload', 'package')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX idx_audioasset_assets_blob_sha256 ON audioasset_assets (blob_sha256);

CREATE TABLE alert_rule_audio_asset_refs (
    rule_id TEXT NOT NULL REFERENCES alert_rules (id) ON DELETE CASCADE,
    asset_id TEXT NOT NULL REFERENCES audioasset_assets (id),
    PRIMARY KEY (rule_id, asset_id)
);
CREATE INDEX idx_alert_rule_audio_asset_refs_asset_id ON alert_rule_audio_asset_refs (asset_id);

CREATE TABLE alert_template_audio_asset_refs (
    template_id TEXT NOT NULL REFERENCES visual_templates (id) ON DELETE CASCADE,
    asset_id TEXT NOT NULL REFERENCES audioasset_assets (id),
    PRIMARY KEY (template_id, asset_id)
);
CREATE INDEX idx_alert_template_audio_asset_refs_asset_id ON alert_template_audio_asset_refs (asset_id);
