-- Stage 12A alerts: persisted alert-output profiles and alert-rule
-- DEFINITIONS only - user-authored configuration, exactly like a chat
-- overlay profile (migration 0011) or a chat-automation schedule/command
-- (migration 0012).
--
-- Scope note, matching every other table in this schema: no real matched
-- alert event is ever stored here, no triggering username, no queue/
-- playback history, no replay history, no OAuth token, no EventSub
-- payload. All of that is runtime-only state, kept in memory by
-- internal/alerts, and resets on every backend restart - see
-- docs/progress.md's Stage 12A entry and docs/engagement-architecture.md
-- §9/§10.
--
-- Two deliberate deviations from the task's own suggested schema, both
-- documented in docs/progress.md's Stage 12A persistence entry:
--   1. No "show_amount" column: the real normalized Twitch events this
--      stage supports never populate engagement.Event.Amount/Currency,
--      so a monetary-amount visibility toggle would control a field
--      that can never be non-empty. "show_quantity" is the real
--      equivalent (Bits amount, gift count, raid viewer count).
--   2. No "allow_grouping"/"group_window_ms" columns: alert grouping is
--      explicitly deferred to Stage 12B (see the Stage 12A task's own
--      Part 21, which permits deferring it), so no dead/unused column
--      is added now - Stage 12B's own migration will add it when the
--      runtime behavior actually exists.

-- One alert-output profile: a single independent OBS Browser Source
-- destination with its own public URL, its own bounded queue, and its
-- own small set of fixed presentation choices. An operator may create
-- more than one - see the Stage 12A task's own Part 4 ("one profile's
-- queue or slow Browser Source must not block another profile").
CREATE TABLE alert_profiles (
    id                          TEXT    PRIMARY KEY,
    public_slug                 TEXT    NOT NULL UNIQUE,
    name                        TEXT    NOT NULL,
    enabled                     INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),

    -- Language used for this profile's own built-in presentation strings
    -- (localized event-type labels etc. rendered server-side into
    -- rendered_text) - an explicit, stored choice, never inferred from
    -- whoever last edited the profile. See the Stage 12A task's own
    -- Part 44.
    language                    TEXT    NOT NULL DEFAULT 'en' CHECK (language IN ('en', 'pl')),

    -- Fixed presentation model only (Stage 12A task Part 24) - never
    -- arbitrary coordinates, layers, uploaded assets, or custom CSS.
    theme                       TEXT    NOT NULL DEFAULT 'minimal' CHECK (theme IN ('minimal', 'compact', 'large')),
    position                    TEXT    NOT NULL DEFAULT 'bottom' CHECK (position IN ('top', 'center', 'bottom')),
    text_align                  TEXT    NOT NULL DEFAULT 'center' CHECK (text_align IN ('left', 'center', 'right')),

    max_queue_items             INTEGER NOT NULL DEFAULT 100 CHECK (max_queue_items BETWEEN 1 AND 500),
    maximum_queue_age_seconds   INTEGER NOT NULL DEFAULT 120 CHECK (maximum_queue_age_seconds BETWEEN 5 AND 3600),

    created_at                  TEXT    NOT NULL,
    updated_at                  TEXT    NOT NULL
);

-- One alert rule, belonging to exactly one profile. event_type is
-- restricted to the real, currently-normalized Twitch alert-capable
-- types - see internal/alerts/capability.go for the per-type capability
-- table this list must stay in sync with. minimum_quantity/
-- maximum_quantity are nullable (NULL = unbounded on that side) and
-- inclusive, letting an operator build non-overlapping tiers (e.g. Bits
-- 1-99, 100-999, 1000+) - see the Stage 12A task's own Part 7.
--
-- required_role is reserved for a future event type that actually
-- supplies role data: none of the 8 real Twitch event types normalized
-- by this application populates engagement.User.Roles today (confirmed
-- against internal/provider/twitch/eventsub_normalize.go before writing
-- this migration), so internal/domain/alerts's own capability
-- validation currently rejects any value other than 'everyone' - the
-- column exists so a later connector/event type can use it without a
-- schema change.
CREATE TABLE alert_rules (
    id                      TEXT    PRIMARY KEY,
    profile_id              TEXT    NOT NULL REFERENCES alert_profiles (id) ON DELETE CASCADE,
    name                    TEXT    NOT NULL,
    enabled                 INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),

    event_type              TEXT    NOT NULL CHECK (event_type IN (
                                 'follow', 'subscription', 'resubscription',
                                 'gifted_subscription', 'subscription_gift_batch',
                                 'bits', 'raid', 'channel_point_redemption'
                             )),

    priority                INTEGER NOT NULL DEFAULT 50 CHECK (priority BETWEEN 0 AND 100),
    duration_ms             INTEGER NOT NULL DEFAULT 5000 CHECK (duration_ms BETWEEN 1000 AND 30000),

    minimum_quantity        INTEGER,
    maximum_quantity        INTEGER,

    required_role           TEXT    NOT NULL DEFAULT 'everyone'
                                     CHECK (required_role IN ('everyone', 'subscriber', 'vip', 'moderator', 'broadcaster')),

    show_platform           INTEGER NOT NULL DEFAULT 1 CHECK (show_platform IN (0, 1)),
    show_username           INTEGER NOT NULL DEFAULT 1 CHECK (show_username IN (0, 1)),
    show_message            INTEGER NOT NULL DEFAULT 1 CHECK (show_message IN (0, 1)),
    show_quantity           INTEGER NOT NULL DEFAULT 1 CHECK (show_quantity IN (0, 1)),

    text_template            TEXT    NOT NULL,

    entry_animation          TEXT    NOT NULL DEFAULT 'fade'
                                      CHECK (entry_animation IN ('none', 'fade', 'slide_up', 'slide_left', 'scale')),
    exit_animation           TEXT    NOT NULL DEFAULT 'fade'
                                      CHECK (exit_animation IN ('none', 'fade', 'slide_up', 'slide_left', 'scale')),
    animation_duration_ms    INTEGER NOT NULL DEFAULT 400 CHECK (animation_duration_ms BETWEEN 0 AND 2000),

    created_at                TEXT    NOT NULL,
    updated_at                TEXT    NOT NULL
);

CREATE INDEX idx_alert_rules_profile ON alert_rules (profile_id);

-- Provider filter: empty (no rows) means "any supported provider";
-- non-empty restricts matching to exactly the listed providers. See
-- internal/domain/alerts/service.go for how an empty list is
-- distinguished from "matches nothing".
CREATE TABLE alert_rule_providers (
    rule_id     TEXT NOT NULL REFERENCES alert_rules (id) ON DELETE CASCADE,
    provider_id TEXT NOT NULL,
    PRIMARY KEY (rule_id, provider_id)
);

-- Account filter: empty (no rows) means "any connected account";
-- non-empty restricts matching to exactly the listed accounts.
-- internal/domain/alerts's own Service validates that every configured
-- account id actually exists before a rule is saved (Part 5: "Validate
-- that explicitly configured accounts exist").
CREATE TABLE alert_rule_accounts (
    rule_id               TEXT NOT NULL REFERENCES alert_rules (id) ON DELETE CASCADE,
    connected_account_id  TEXT NOT NULL REFERENCES connected_accounts (id) ON DELETE CASCADE,
    PRIMARY KEY (rule_id, connected_account_id)
);
