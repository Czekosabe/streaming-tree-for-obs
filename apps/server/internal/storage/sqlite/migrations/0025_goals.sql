-- Stage 18A: persistent goals/counters foundation (docs/goals-widgets.md
-- §8, §14, §25), backing internal/domain/goals.
--
-- goals holds one row per persistent, accumulating goal. current_value is
-- "observed contributions since baseline" (docs/goals-widgets.md §1),
-- never a provider-canonical total. config_revision guards only operator
-- configuration edits (PUT) via optimistic concurrency - contribution
-- application (see goal_applied_events below) never touches it
-- (docs/goals-widgets.md §8.1).
--
-- goal_providers/goal_accounts are plain filter child tables, mirroring
-- alert_rule_providers/alert_rule_accounts exactly (0013_alerts.sql).
-- goal_accounts carries no table-level foreign key on account_id, for
-- the same reason 0020_donation_sources.sql dropped alert_rule_accounts'
-- own single-table foreign key: an entry may reference either
-- connected_accounts or a donation source, and only application-layer
-- validation (internal/domain/goals.Service, via a combined lookup
-- adapter) can express that - never a single-table SQL foreign key.
--
-- goal_applied_events is the durable, per-goal contribution dedupe
-- ledger (docs/goals-widgets.md §11) - its primary key is exactly the
-- durable dedupe identity, so a second INSERT for an already-applied
-- (goal_id, provider_id, account_id, provider_event_key) fails with a
-- UNIQUE/PRIMARY KEY violation the repository translates into
-- "applied=false, no error" (docs/goals-widgets.md §12). Indexed by
-- applied_at for the bounded 30-day retention prune (§11.5).
--
-- widget_profiles holds one row per public presentation of exactly one
-- goal (docs/goals-widgets.md §18). goal_id references goals(id) with no
-- ON DELETE clause - the repository explicitly checks for referencing
-- widget profiles before deleting a goal and returns ErrGoalInUse itself
-- (never relying on a raw, opaque SQLite foreign-key-violation error to
-- communicate that to a caller).
CREATE TABLE goals (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    kind            TEXT NOT NULL CHECK (kind IN ('followers', 'subscriptions', 'donations', 'bits')),
    enabled         INTEGER NOT NULL DEFAULT 1,
    target          INTEGER NOT NULL CHECK (target > 0),
    current_value   INTEGER NOT NULL CHECK (current_value >= 0),
    baseline        INTEGER NOT NULL CHECK (baseline >= 0),
    currency        TEXT,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    started_at      TEXT NOT NULL,
    config_revision INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE goal_providers (
    goal_id     TEXT NOT NULL REFERENCES goals (id) ON DELETE CASCADE,
    provider_id TEXT NOT NULL,
    PRIMARY KEY (goal_id, provider_id)
);

CREATE TABLE goal_accounts (
    goal_id    TEXT NOT NULL REFERENCES goals (id) ON DELETE CASCADE,
    account_id TEXT NOT NULL,
    PRIMARY KEY (goal_id, account_id)
);

CREATE TABLE goal_applied_events (
    goal_id             TEXT NOT NULL REFERENCES goals (id) ON DELETE CASCADE,
    provider_id         TEXT NOT NULL,
    account_id          TEXT NOT NULL,
    provider_event_key  TEXT NOT NULL,
    applied_at          TEXT NOT NULL,
    PRIMARY KEY (goal_id, provider_id, account_id, provider_event_key)
);
CREATE INDEX idx_goal_applied_events_applied_at ON goal_applied_events (applied_at);

CREATE TABLE widget_profiles (
    id                TEXT PRIMARY KEY,
    goal_id           TEXT NOT NULL REFERENCES goals (id),
    name              TEXT NOT NULL,
    enabled           INTEGER NOT NULL DEFAULT 1,
    public_slug       TEXT NOT NULL UNIQUE,
    title_override    TEXT NOT NULL DEFAULT '',
    show_current      INTEGER NOT NULL DEFAULT 1,
    show_target       INTEGER NOT NULL DEFAULT 1,
    show_percent      INTEGER NOT NULL DEFAULT 1,
    orientation       TEXT NOT NULL CHECK (orientation IN ('horizontal', 'vertical')),
    text_align        TEXT NOT NULL CHECK (text_align IN ('left', 'center', 'right')),
    font_family       TEXT NOT NULL CHECK (font_family IN ('sans_serif', 'serif', 'monospace', 'rounded')),
    background_color  TEXT NOT NULL,
    foreground_color  TEXT NOT NULL,
    fill_color        TEXT NOT NULL,
    border_color      TEXT NOT NULL,
    border_radius_px  INTEGER NOT NULL DEFAULT 12,
    opacity           REAL NOT NULL DEFAULT 1.0,
    created_at        TEXT NOT NULL,
    updated_at        TEXT NOT NULL
);
CREATE INDEX idx_widget_profiles_goal_id ON widget_profiles (goal_id);
