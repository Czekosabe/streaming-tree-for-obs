-- Stage 22: reusable stream metadata presets (docs/metadata-presets.md).
--
-- Scope note: presets hold stream CONTENT metadata only - the exact
-- same shared, capability-gated fields platform_metadata already
-- carries (title/description/tags/language/visibility/mature_content/
-- dvr/latency_mode), plus a provider-scoped category, keyed per
-- provider so a Twitch category ID is never confused with a YouTube
-- one. No stream key, token, credential, destination transport
-- configuration (server URL, enabled state), or runtime state is ever
-- stored here - structurally, not merely by convention: no column
-- exists that could hold one.
--
-- No seed data: a fresh database starts with zero presets.

CREATE TABLE metadata_presets (
    id             TEXT    PRIMARY KEY,
    name           TEXT    NOT NULL,
    note           TEXT    NOT NULL DEFAULT '',
    title          TEXT    NOT NULL DEFAULT '',
    description    TEXT    NOT NULL DEFAULT '',
    language       TEXT    NOT NULL DEFAULT '',
    visibility     TEXT    NOT NULL DEFAULT '',
    mature_content INTEGER NOT NULL DEFAULT 0 CHECK (mature_content IN (0, 1)),
    dvr            INTEGER NOT NULL DEFAULT 0 CHECK (dvr IN (0, 1)),
    latency_mode   TEXT    NOT NULL DEFAULT '',
    created_at     TEXT    NOT NULL,
    updated_at     TEXT    NOT NULL
);

-- Duplicate-name policy: reject an exact (case-insensitive) collision -
-- simple and predictable (docs/metadata-presets.md §2).
CREATE UNIQUE INDEX idx_metadata_presets_name ON metadata_presets (name COLLATE NOCASE);

CREATE TABLE metadata_preset_tags (
    preset_id TEXT    NOT NULL
                       REFERENCES metadata_presets (id) ON DELETE CASCADE,
    position  INTEGER NOT NULL,
    value     TEXT    NOT NULL,
    PRIMARY KEY (preset_id, position)
);

-- The one genuinely provider-scoped concept (docs/metadata-presets.md
-- §1): a category is never applied outside the exact provider it was
-- captured under. provider_id is a plain text identifier here, exactly
-- like platforms.provider_id already is - never validated against the
-- built-in registry at the SQL layer, since that registry can change
-- independently of this schema.
CREATE TABLE metadata_preset_provider_overrides (
    preset_id   TEXT NOT NULL
                     REFERENCES metadata_presets (id) ON DELETE CASCADE,
    provider_id TEXT NOT NULL,
    category    TEXT NOT NULL DEFAULT '',
    category_id TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (preset_id, provider_id)
);
