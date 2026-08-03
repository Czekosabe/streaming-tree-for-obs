-- Initial schema for persistent platform configuration.
--
-- Scope note: these tables hold CONFIGURATION only. Runtime stream state
-- (offline/starting/live, viewer counts, FFmpeg process state) is deliberately
-- absent, because no streaming engine exists yet and runtime state must not be
-- persisted here.
--
-- No table stores a stream key, OAuth token or any other credential.
--
-- Timestamps are RFC 3339 with nanosecond precision in UTC. Stored as TEXT
-- because that format sorts lexicographically in the same order as it does
-- chronologically.

CREATE TABLE platforms (
    id           TEXT    PRIMARY KEY,
    -- References a built-in provider definition compiled into the binary.
    -- Deliberately not a foreign key: providers are code, not rows.
    provider_id  TEXT    NOT NULL,
    display_name TEXT    NOT NULL,
    enabled      INTEGER NOT NULL DEFAULT 0 CHECK (enabled IN (0, 1)),
    sort_order   INTEGER NOT NULL DEFAULT 0,
    created_at   TEXT    NOT NULL,
    updated_at   TEXT    NOT NULL
);

-- The dashboard always lists platforms in this order; created_at is the
-- deterministic tie-breaker for equal sort orders.
CREATE INDEX idx_platforms_sort_order ON platforms (sort_order, created_at, id);

-- Several destinations may share one provider, so this index is intentionally
-- non-unique.
CREATE INDEX idx_platforms_provider_id ON platforms (provider_id);

CREATE TABLE platform_metadata (
    platform_id    TEXT    PRIMARY KEY
                           REFERENCES platforms (id) ON DELETE CASCADE,
    -- NULL means "the provider does not support this field", which is a
    -- different statement from an empty string the user actually left blank.
    title          TEXT,
    description    TEXT,
    category       TEXT,
    language       TEXT,
    visibility     TEXT,
    mature_content INTEGER CHECK (mature_content IS NULL OR mature_content IN (0, 1)),
    dvr            INTEGER CHECK (dvr IS NULL OR dvr IN (0, 1)),
    latency_mode   TEXT,
    updated_at     TEXT    NOT NULL
);

-- Tags are separate ordered rows rather than a delimited string, so ordering is
-- explicit and a tag containing a comma cannot corrupt the list.
CREATE TABLE platform_metadata_tags (
    platform_id TEXT    NOT NULL
                        REFERENCES platforms (id) ON DELETE CASCADE,
    position    INTEGER NOT NULL,
    value       TEXT    NOT NULL,
    PRIMARY KEY (platform_id, position)
);

-- Case-insensitive uniqueness within one platform, enforced by the database as
-- well as by the domain layer.
CREATE UNIQUE INDEX idx_platform_tags_unique_value
    ON platform_metadata_tags (platform_id, lower(value));
