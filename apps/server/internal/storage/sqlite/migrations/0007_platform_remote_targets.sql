-- Remote broadcast/target association for a configured destination.
--
-- Scope note: a connected account (internal/domain/account) is not enough to
-- know which remote resource (a YouTube live broadcast, today) a configured
-- destination's metadata should be read from and published to - that is a
-- separate fact from the account link itself, and from the destination's
-- stream key or output-server settings. This table holds only a reference to
-- that remote resource: no token, no stream key, no ingestion/stream-name
-- data, no raw provider response.

CREATE TABLE platform_remote_targets (
    platform_id   TEXT PRIMARY KEY
                       REFERENCES platforms (id) ON DELETE CASCADE,
    provider_id   TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id   TEXT NOT NULL,
    display_name  TEXT NOT NULL,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);
