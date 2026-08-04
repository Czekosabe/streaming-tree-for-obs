-- Non-secret FFmpeg output configuration for each configured platform.
--
-- Scope note: this table holds the destination RTMP/RTMPS SERVER address
-- only - never a stream key, never a full destination URL containing one,
-- and never runtime state (process id, restart count, live/error status).
-- Runtime state stays in memory, exactly as MediaMTX runtime state does; see
-- docs/project-overview.md section 8.1. The stream key itself lives in the
-- OS credential store (internal/secrets), never in this database.

CREATE TABLE platform_output_settings (
    platform_id  TEXT    PRIMARY KEY
                         REFERENCES platforms (id) ON DELETE CASCADE,
    -- Empty string means "not configured yet", not NULL: every platform gets
    -- a row the moment it is created, so a row's mere existence never implies
    -- readiness to stream.
    server_url   TEXT    NOT NULL DEFAULT '',
    auto_restart INTEGER NOT NULL DEFAULT 1 CHECK (auto_restart IN (0, 1)),
    created_at   TEXT    NOT NULL,
    updated_at   TEXT    NOT NULL
);

-- Every platform that existed before this migration gets a default,
-- unconfigured settings row - see internal/domain/output for why an empty
-- server_url is a safe default rather than an invented one: no seeded
-- destination becomes silently ready to stream as a side effect of running
-- this migration.
INSERT INTO platform_output_settings (platform_id, server_url, auto_restart, created_at, updated_at)
SELECT id, '', 1, created_at, updated_at
FROM platforms;
