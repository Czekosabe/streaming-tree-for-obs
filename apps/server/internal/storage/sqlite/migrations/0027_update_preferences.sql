-- Stage 20B: the one persisted application-updater preference.
--
-- Scope note: exactly like operator_chat_preferences (Stage 9), this
-- table holds configuration only - no identity, no machine/installation
-- id, no check/download history. See docs/updater.md §27.

CREATE TABLE update_preferences (
    id          INTEGER PRIMARY KEY CHECK (id = 1),
    auto_check  INTEGER NOT NULL DEFAULT 1 CHECK (auto_check IN (0, 1)),
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);
