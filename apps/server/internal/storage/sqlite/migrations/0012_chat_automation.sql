-- Stage 11B chat automation: persisted scheduled-message and chat-command
-- DEFINITIONS only - user-authored configuration, exactly like a chat
-- overlay profile (migration 0011) or a destination's output settings.
--
-- Scope note, matching every other table in this schema: no inbound chat
-- message is ever stored here, no triggering username, no outbound
-- delivery history, no OAuth token, no runtime cooldown/activity/next-run
-- state. Runtime state (next run time, rolling send-per-hour counters,
-- per-user cooldown timestamps, activity-since-last-send counters) lives
-- only in memory in internal/chatautomation and resets on every backend
-- restart - see docs/progress.md's Stage 11B entry and
-- docs/engagement-architecture.md §8.

-- One scheduled-message definition. A schedule always has at least one
-- target (chat_schedule_targets) and at least one message alternative
-- (chat_schedule_messages), enforced by internal/domain/chatautomation's
-- own validation, not by SQL (SQLite cannot express "at least one child
-- row exists" as a table constraint).
CREATE TABLE chat_schedules (
    id                            TEXT    PRIMARY KEY,
    name                          TEXT    NOT NULL,
    enabled                       INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),

    interval_seconds              INTEGER NOT NULL,
    first_delay_seconds           INTEGER NOT NULL DEFAULT 0,
    jitter_seconds                INTEGER NOT NULL DEFAULT 0,

    -- Local Streaming Tree ingest ("MediaMTX local path state ==
    -- receiving") only - never Twitch stream.online, never FFmpeg branch
    -- state, never viewer presence. See internal/chatautomation's own
    -- scheduler doc comment.
    only_while_ingest_receiving   INTEGER NOT NULL DEFAULT 0 CHECK (only_while_ingest_receiving IN (0, 1)),
    -- Minimum eligible human chat.message events received on a target
    -- account since that schedule/account's previous successful send.
    minimum_chat_messages         INTEGER NOT NULL DEFAULT 0,
    -- Rolling successful-scheduled-send ceiling per schedule/account -
    -- an automation-behavior control, not a provider-rate-limit
    -- guarantee (internal/outboundchat's dispatcher remains the
    -- authoritative safety ceiling).
    maximum_sends_per_hour        INTEGER NOT NULL DEFAULT 60,

    created_at                    TEXT    NOT NULL,
    updated_at                    TEXT    NOT NULL
);

-- Explicit per-account targets. platform_id is optional: when set it
-- provides deterministic local metadata context for placeholders such as
-- {streamTitle} (see internal/chatautomation/placeholders.go); when
-- absent, placeholders requiring destination metadata simply become
-- unresolved rather than this application silently guessing a linked
-- platform. ON DELETE SET NULL for platform_id (not CASCADE): deleting a
-- destination must not silently delete an operator's schedule target,
-- only drop its metadata context.
CREATE TABLE chat_schedule_targets (
    schedule_id TEXT NOT NULL REFERENCES chat_schedules (id) ON DELETE CASCADE,
    account_id  TEXT NOT NULL REFERENCES connected_accounts (id) ON DELETE CASCADE,
    platform_id TEXT REFERENCES platforms (id) ON DELETE SET NULL,
    PRIMARY KEY (schedule_id, account_id)
);

-- One or more message alternatives per schedule ("message groups with
-- random selection" - see the Stage 11B task's Part 4). position is a
-- stable display/authoring order; random selection at execution time is
-- runtime-only state, never persisted (see the Stage 11B task's own
-- "do not persist the previous selection merely for randomness").
CREATE TABLE chat_schedule_messages (
    id                TEXT    NOT NULL PRIMARY KEY,
    schedule_id       TEXT    NOT NULL REFERENCES chat_schedules (id) ON DELETE CASCADE,
    message_template  TEXT    NOT NULL,
    position          INTEGER NOT NULL,
    created_at        TEXT    NOT NULL,
    updated_at        TEXT    NOT NULL
);

CREATE INDEX idx_chat_schedule_messages_schedule
    ON chat_schedule_messages (schedule_id, position);

-- One safe chat-command definition. name is the canonical command name
-- without its leading "!" (the prefix itself is a fixed Stage 11B
-- constant, never stored - see internal/chatautomation/commands.go),
-- stored lowercase. required_role is one of the fixed, closed role enum
-- - never a free-text role or a per-command custom role.
--
-- Stage 11B requires every command name AND alias to be globally unique
-- across the whole local application (simpler and deterministic than
-- per-provider/per-account scoping - see the Stage 11B task's own Part
-- 12 "document that provider/account-specific duplicate names may be
-- supported later if needed"). The UNIQUE constraint below enforces
-- uniqueness within this table; cross-table uniqueness against
-- chat_command_aliases.alias is enforced in
-- internal/domain/chatautomation's own Service, inside one transaction,
-- since SQLite cannot express a UNIQUE constraint spanning two tables.
CREATE TABLE chat_commands (
    id                        TEXT    PRIMARY KEY,
    name                      TEXT    NOT NULL UNIQUE,
    enabled                   INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    response_template         TEXT    NOT NULL,
    required_role             TEXT    NOT NULL DEFAULT 'everyone'
                                       CHECK (required_role IN ('everyone', 'subscriber', 'vip', 'moderator', 'broadcaster')),
    global_cooldown_seconds   INTEGER NOT NULL DEFAULT 0,
    user_cooldown_seconds     INTEGER NOT NULL DEFAULT 0,
    created_at                TEXT    NOT NULL,
    updated_at                TEXT    NOT NULL
);

-- See chat_commands' own doc comment on cross-table global uniqueness.
CREATE TABLE chat_command_aliases (
    command_id TEXT NOT NULL REFERENCES chat_commands (id) ON DELETE CASCADE,
    alias      TEXT NOT NULL UNIQUE,
    PRIMARY KEY (command_id, alias)
);

-- Same deterministic platform-context semantics as chat_schedule_targets
-- above. A command with no target accounts is invalid (enforced by
-- internal/domain/chatautomation's own validation, not SQL).
CREATE TABLE chat_command_targets (
    command_id  TEXT NOT NULL REFERENCES chat_commands (id) ON DELETE CASCADE,
    account_id  TEXT NOT NULL REFERENCES connected_accounts (id) ON DELETE CASCADE,
    platform_id TEXT REFERENCES platforms (id) ON DELETE SET NULL,
    PRIMARY KEY (command_id, account_id)
);
