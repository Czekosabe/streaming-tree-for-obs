-- Stage 24: stream session / operational history (docs/stream-session-
-- history.md). A session is a contiguous period during which the local
-- MediaMTX ingest was actually receiving a publish from OBS - never a
-- proxy for destination-branch state (see the contract's own §1).
--
-- Scope note: this records ONLY the application's own operational
-- timeline - when a session started/ended, and which destinations
-- participated with what coarse, closed-enum outcome. It never stores
-- chat messages, chatter names, donation messages, donor names or
-- amounts, membership/Super Chat content, alert payload content, TTS
-- text, or any other viewer/engagement content - structurally, not
-- merely by convention: no column exists that could hold any of that.
--
-- No seed data: a fresh database starts with zero sessions.

CREATE TABLE stream_sessions (
    id           TEXT NOT NULL PRIMARY KEY,
    started_at   TEXT NOT NULL,
    -- NULL while the session is still open - see the contract's §2 for
    -- the unclean-shutdown recovery path that always eventually closes
    -- it.
    ended_at     TEXT,
    -- Heartbeat, updated on every poll tick while the session is open -
    -- the source of a recovered session's own ended_at, never
    -- time.Now() at recovery time.
    last_seen_at TEXT NOT NULL,
    -- '' while open; 'ingest_stopped' or 'unclean_shutdown' once closed
    -- (contract §5) - a closed, bounded set, never a raw error message.
    end_reason   TEXT NOT NULL DEFAULT '',
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);

CREATE INDEX idx_stream_sessions_started_at ON stream_sessions (started_at);

CREATE TABLE stream_session_destinations (
    id            TEXT NOT NULL PRIMARY KEY,
    session_id    TEXT NOT NULL
                       REFERENCES stream_sessions (id) ON DELETE CASCADE,
    -- SET NULL, never CASCADE: deleting a destination must never delete
    -- its own participation history (contract §3).
    platform_id   TEXT
                       REFERENCES platforms (id) ON DELETE SET NULL,
    -- provider_id/display_name are a SNAPSHOT taken when this row is
    -- created, never re-resolved from the live platform row, so a
    -- later rename or deletion never rewrites what history already
    -- says happened at the time.
    provider_id   TEXT NOT NULL,
    display_name  TEXT NOT NULL,
    started_at    TEXT,
    ended_at      TEXT,
    -- '' while open; 'completed', 'error', or 'session_ended' once
    -- closed (contract §4) - a closed, bounded set, never a raw FFmpeg
    -- stderr line or provider HTTP response body.
    outcome       TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);

CREATE INDEX idx_stream_session_destinations_session_id ON stream_session_destinations (session_id);
