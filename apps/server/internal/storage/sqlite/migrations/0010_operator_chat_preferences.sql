-- Stage 9 unified operator chat: persisted presentation preferences only.
--
-- Scope note: exactly like connected_account_engagement_settings (Stage
-- 8A), this schema holds nothing about live chat content. No message text,
-- no display name treated as authoritative identity, no OAuth token, no
-- EventSub session data, no raw provider event, no deleted-message content,
-- no search history. Chat items themselves live only in the in-memory
-- operator-chat projection (internal/operatorchat) and are gone on
-- restart - see docs/provider-integrations/twitch-engagement.md's Stage 9
-- addendum and docs/progress.md's Stage 9 entry for the full rationale.
--
-- operator_chat_preferences is a singleton row (id fixed at 1): this stage
-- has one operator, not per-profile preferences.

CREATE TABLE operator_chat_preferences (
    id                     INTEGER PRIMARY KEY CHECK (id = 1),
    show_platform_icon     INTEGER NOT NULL DEFAULT 1 CHECK (show_platform_icon IN (0, 1)),
    show_platform_name     INTEGER NOT NULL DEFAULT 0 CHECK (show_platform_name IN (0, 1)),
    show_account_label     INTEGER NOT NULL DEFAULT 1 CHECK (show_account_label IN (0, 1)),
    show_badges            INTEGER NOT NULL DEFAULT 1 CHECK (show_badges IN (0, 1)),
    show_timestamps        INTEGER NOT NULL DEFAULT 1 CHECK (show_timestamps IN (0, 1)),
    show_activity_events   INTEGER NOT NULL DEFAULT 1 CHECK (show_activity_events IN (0, 1)),
    show_deleted_messages  INTEGER NOT NULL DEFAULT 1 CHECK (show_deleted_messages IN (0, 1)),
    hide_command_messages  INTEGER NOT NULL DEFAULT 0 CHECK (hide_command_messages IN (0, 1)),
    compact_mode           INTEGER NOT NULL DEFAULT 0 CHECK (compact_mode IN (0, 1)),
    created_at             TEXT NOT NULL,
    updated_at             TEXT NOT NULL
);

-- Per-connected-account chat visibility (default visible; a row exists
-- only once an operator has explicitly set a preference for that account,
-- mirroring connected_account_engagement_settings's absent-row-means-
-- default convention - an absent row is treated identically to visible).
CREATE TABLE operator_chat_account_visibility (
    account_id TEXT PRIMARY KEY
                    REFERENCES connected_accounts (id) ON DELETE CASCADE,
    visible    INTEGER NOT NULL CHECK (visible IN (0, 1)),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- Operator-maintained lists identifying users by the provider's own stable
-- user id, never by display name or login (both of which a user can
-- change) - scoped to one provider + connected account, since the same
-- provider user id on a different account context is not necessarily the
-- same operator decision.
CREATE TABLE operator_chat_hidden_users (
    id                   TEXT PRIMARY KEY,
    provider_id          TEXT NOT NULL,
    connected_account_id TEXT NOT NULL
                              REFERENCES connected_accounts (id) ON DELETE CASCADE,
    provider_user_id     TEXT NOT NULL,
    label                TEXT,
    created_at           TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_operator_chat_hidden_users_identity
    ON operator_chat_hidden_users (provider_id, connected_account_id, provider_user_id);

CREATE TABLE operator_chat_bot_users (
    id                   TEXT PRIMARY KEY,
    provider_id          TEXT NOT NULL,
    connected_account_id TEXT NOT NULL
                              REFERENCES connected_accounts (id) ON DELETE CASCADE,
    provider_user_id     TEXT NOT NULL,
    label                TEXT,
    created_at           TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_operator_chat_bot_users_identity
    ON operator_chat_bot_users (provider_id, connected_account_id, provider_user_id);
