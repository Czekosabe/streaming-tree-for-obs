-- Stage 24: the one persisted stream-session-history preference -
-- retention, in days (docs/stream-session-history.md §6). Exactly the
-- same singleton-row pattern update_preferences (Stage 20B) already
-- uses.
--
-- Scope note: this table holds a retention preference only - no
-- session data, no destination participation data, no chat/donation/
-- engagement content of any kind.

CREATE TABLE stream_session_settings (
    id             INTEGER PRIMARY KEY CHECK (id = 1),
    retention_days INTEGER NOT NULL DEFAULT 90 CHECK (retention_days > 0),
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);
