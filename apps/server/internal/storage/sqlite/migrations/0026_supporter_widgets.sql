-- Stage 18B: supporter/activity widgets, richer counters, and bounded
-- multi-widget dashboards (docs/supporter-widgets.md §5, §9, §18),
-- widening internal/domain/goals.WidgetProfile.
--
-- widget_profiles.kind was implicitly always "goal" through Stage 18A;
-- this migration adds a real kind column plus every field the eight new
-- kinds need, and makes goal_id nullable (required only for kind='goal').
-- SQLite cannot widen a CHECK list or drop a NOT NULL via plain
-- ALTER TABLE, so this follows the exact "create the replacement table,
-- copy every existing row across unchanged, drop the old table, rename
-- the new one into place" rebuild pattern 0020_donation_sources.sql's own
-- alert_rules_new rebuild already used - every Stage 18A row survives
-- byte-for-byte, with kind defaulting to 'goal' and every new column at
-- its safe zero value.
--
-- No column added anywhere in this migration ever stores event-derived
-- content (a display name, a donation message, a ticker/recent-supporter
-- row, a providerEventId) - docs/supporter-widgets.md §3's privacy
-- boundary. Every new column is operator configuration only.
CREATE TABLE widget_profiles_new (
    id                TEXT PRIMARY KEY,
    kind              TEXT NOT NULL DEFAULT 'goal' CHECK (kind IN (
                          'goal', 'latest_follower', 'latest_subscriber',
                          'latest_donation', 'largest_donation', 'recent_supporters',
                          'event_ticker', 'session_counter', 'dashboard'
                      )),
    goal_id           TEXT REFERENCES goals (id),
    name              TEXT NOT NULL,
    enabled           INTEGER NOT NULL DEFAULT 1,
    public_slug       TEXT NOT NULL UNIQUE,
    title_override    TEXT NOT NULL DEFAULT '',
    show_current      INTEGER NOT NULL DEFAULT 1,
    show_target       INTEGER NOT NULL DEFAULT 1,
    show_percent      INTEGER NOT NULL DEFAULT 1,
    show_provider     INTEGER NOT NULL DEFAULT 1,
    show_time         INTEGER NOT NULL DEFAULT 1,
    show_message      INTEGER NOT NULL DEFAULT 0,
    max_items         INTEGER NOT NULL DEFAULT 0,
    currency          TEXT,
    metric            TEXT CHECK (metric IS NULL OR metric IN (
                          'follows', 'new_subscriptions', 'resubscriptions',
                          'gifted_subscriptions', 'raids', 'bits_quantity',
                          'support_event_count', 'support_amount'
                      )),
    columns           INTEGER NOT NULL DEFAULT 0,
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

INSERT INTO widget_profiles_new (
    id, kind, goal_id, name, enabled, public_slug, title_override,
    show_current, show_target, show_percent,
    orientation, text_align, font_family,
    background_color, foreground_color, fill_color, border_color,
    border_radius_px, opacity, created_at, updated_at
)
    SELECT
        id, 'goal', goal_id, name, enabled, public_slug, title_override,
        show_current, show_target, show_percent,
        orientation, text_align, font_family,
        background_color, foreground_color, fill_color, border_color,
        border_radius_px, opacity, created_at, updated_at
    FROM widget_profiles;

DROP TABLE widget_profiles;

ALTER TABLE widget_profiles_new RENAME TO widget_profiles;

CREATE INDEX idx_widget_profiles_goal_id ON widget_profiles (goal_id);
CREATE INDEX idx_widget_profiles_kind ON widget_profiles (kind);

-- widget_profile_providers/widget_profile_accounts: an event-derived
-- widget's own provider/account filters (docs/supporter-widgets.md §15) -
-- meaningless for kind='goal' (which defers to its own goal's filters)
-- and kind='dashboard' (which has none), enforced at the application
-- layer, mirroring goal_providers/goal_accounts exactly, including
-- widget_profile_accounts carrying no FOREIGN KEY on account_id (an
-- entry may reference either connected_accounts or a donation source -
-- see 0025_goals.sql's own identical goal_accounts reasoning).
CREATE TABLE widget_profile_providers (
    widget_profile_id TEXT NOT NULL REFERENCES widget_profiles (id) ON DELETE CASCADE,
    provider_id       TEXT NOT NULL,
    PRIMARY KEY (widget_profile_id, provider_id)
);

CREATE TABLE widget_profile_accounts (
    widget_profile_id TEXT NOT NULL REFERENCES widget_profiles (id) ON DELETE CASCADE,
    account_id        TEXT NOT NULL,
    PRIMARY KEY (widget_profile_id, account_id)
);

-- widget_profile_event_types: an event_ticker widget's own closed
-- presentation-type allowlist subset (docs/supporter-widgets.md §8).
CREATE TABLE widget_profile_event_types (
    widget_profile_id TEXT NOT NULL REFERENCES widget_profiles (id) ON DELETE CASCADE,
    event_type        TEXT NOT NULL,
    PRIMARY KEY (widget_profile_id, event_type)
);

-- widget_profile_dashboard_children: a dashboard's own bounded grid
-- composition (docs/supporter-widgets.md §9) - references an existing
-- widget profile by id, never copies its state. child_id carries no
-- ON DELETE clause: internal/domain/goals.Service explicitly checks for
-- a referencing dashboard before allowing a child's own deletion and
-- returns ErrWidgetProfileInUse itself, exactly mirroring how goal
-- deletion already checks for a referencing widget profile rather than
-- relying on a raw, opaque SQLite foreign-key-violation error.
CREATE TABLE widget_profile_dashboard_children (
    dashboard_id  TEXT NOT NULL REFERENCES widget_profiles (id) ON DELETE CASCADE,
    child_id      TEXT NOT NULL REFERENCES widget_profiles (id),
    position      INTEGER NOT NULL,
    column_start  INTEGER NOT NULL,
    column_span   INTEGER NOT NULL,
    row_start     INTEGER NOT NULL,
    row_span      INTEGER NOT NULL,
    PRIMARY KEY (dashboard_id, child_id)
);
CREATE INDEX idx_widget_profile_dashboard_children_child ON widget_profile_dashboard_children (child_id);
