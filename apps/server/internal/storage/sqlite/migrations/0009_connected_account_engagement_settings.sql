-- Per-account engagement connector configuration (Stage 8A).
--
-- Scope note: this table holds only the user's enable/disable preference
-- for one connected account's inbound engagement connector (EventSub, for
-- Twitch). It deliberately holds nothing about runtime state - no session
-- id, no subscription id, no WebSocket URL, no reconnect count, no last
-- error, no event data - all of that lives in memory only, exactly like
-- MediaMTX and FFmpeg branch runtime state elsewhere in this schema. It
-- resets to the in-memory default on every backend restart; only the
-- enabled/disabled preference itself survives.

CREATE TABLE connected_account_engagement_settings (
    account_id TEXT PRIMARY KEY
                    REFERENCES connected_accounts (id) ON DELETE CASCADE,
    enabled    INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
