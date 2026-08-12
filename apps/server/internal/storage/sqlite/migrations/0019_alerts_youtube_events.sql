-- Stage 15A: four new YouTube-capable alert event types
-- (youtube_membership, youtube_membership_milestone, youtube_super_chat,
-- youtube_super_sticker - see internal/domain/alerts/model.go and
-- capability.go) plus a currency-aware monetary threshold condition for
-- the two monetary ones. See docs/progress.md's Stage 15A alerts entry
-- for the full capability matrix.
--
-- alert_rules.event_type is a literal CHECK list, so SQLite's lack of
-- `ALTER TABLE ... ALTER CONSTRAINT` means it can only be widened by the
-- same safe rebuild pattern migration 0016 already used for
-- visual_designs.owner_kind: create the replacement table, copy every
-- existing row across unchanged, drop the old table, rename the new one
-- into place - all inside this migration's own transaction. Every
-- existing rule's own columns survive byte-for-byte; only the CHECK list
-- widens and four new columns are added, all with safe defaults so an
-- existing rule's behavior is completely unchanged until an operator
-- explicitly sets a monetary condition on a newly-created or edited rule.
--
-- alert_rule_providers/alert_rule_accounts (both FOREIGN KEY REFERENCES
-- alert_rules (id) ON DELETE CASCADE) are untouched by this migration -
-- SQLite's DROP TABLE does not fire ON DELETE triggers, so their rows
-- survive intact and are valid again the moment the renamed table exists
-- under the original alert_rules name with the same id values.

CREATE TABLE alert_rules_new (
    id                      TEXT    PRIMARY KEY,
    profile_id              TEXT    NOT NULL REFERENCES alert_profiles (id) ON DELETE CASCADE,
    name                    TEXT    NOT NULL,
    enabled                 INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),

    event_type              TEXT    NOT NULL CHECK (event_type IN (
                                 'follow', 'subscription', 'resubscription',
                                 'gifted_subscription', 'subscription_gift_batch',
                                 'bits', 'raid', 'channel_point_redemption',
                                 'youtube_membership', 'youtube_membership_milestone',
                                 'youtube_super_chat', 'youtube_super_sticker'
                             )),

    priority                INTEGER NOT NULL DEFAULT 50 CHECK (priority BETWEEN 0 AND 100),
    duration_ms             INTEGER NOT NULL DEFAULT 5000 CHECK (duration_ms BETWEEN 1000 AND 30000),

    minimum_quantity        INTEGER,
    maximum_quantity        INTEGER,

    -- currency: an uppercase provider-reported currency code, required
    -- whenever either amount threshold below is set (enforced at the
    -- application layer - internal/domain/alerts.ValidateMoneyThresholds
    -- - never converted, never compared across currencies). Empty string
    -- (never NULL) when no monetary condition is set, matching this
    -- table's existing text_template convention of NOT NULL with an
    -- empty-string "unset" value rather than a nullable column.
    currency                TEXT    NOT NULL DEFAULT '',
    -- minimum_amount_micros/maximum_amount_micros: integer micros
    -- (1,000,000 = 1.00 major unit) - never a float, matching
    -- internal/domain/engagement.Money's own representation exactly.
    minimum_amount_micros   INTEGER,
    maximum_amount_micros   INTEGER,

    required_role           TEXT NOT NULL DEFAULT 'everyone'
                             CHECK (required_role IN ('everyone', 'subscriber', 'vip', 'moderator', 'broadcaster')),

    show_platform           INTEGER NOT NULL DEFAULT 1 CHECK (show_platform IN (0, 1)),
    show_username           INTEGER NOT NULL DEFAULT 1 CHECK (show_username IN (0, 1)),
    show_message            INTEGER NOT NULL DEFAULT 1 CHECK (show_message IN (0, 1)),
    show_quantity           INTEGER NOT NULL DEFAULT 1 CHECK (show_quantity IN (0, 1)),
    -- show_amount: only meaningful (and only accepted by the application
    -- layer) for an event type whose Capability.HasAmount is true -
    -- defaults to 0 so every pre-Stage-15A rule is completely unaffected.
    show_amount             INTEGER NOT NULL DEFAULT 0 CHECK (show_amount IN (0, 1)),

    text_template            TEXT    NOT NULL,

    entry_animation          TEXT NOT NULL DEFAULT 'fade' CHECK (entry_animation IN ('none','fade','slide_up','slide_left','scale')),
    exit_animation           TEXT NOT NULL DEFAULT 'fade' CHECK (exit_animation  IN ('none','fade','slide_up','slide_left','scale')),
    animation_duration_ms    INTEGER NOT NULL DEFAULT 400 CHECK (animation_duration_ms BETWEEN 0 AND 2000),

    created_at                TEXT NOT NULL,
    updated_at                TEXT NOT NULL,

    allow_grouping  INTEGER NOT NULL DEFAULT 0 CHECK (allow_grouping IN (0,1)),
    group_window_ms  INTEGER NOT NULL DEFAULT 5000 CHECK (group_window_ms BETWEEN 1000 AND 30000),
    interrupt_mode  TEXT NOT NULL DEFAULT 'never' CHECK (interrupt_mode IN ('never','lower_priority')),
    interruptible   INTEGER NOT NULL DEFAULT 1 CHECK (interruptible IN (0,1))
);

INSERT INTO alert_rules_new (
    id, profile_id, name, enabled, event_type, priority, duration_ms,
    minimum_quantity, maximum_quantity, required_role,
    show_platform, show_username, show_message, show_quantity,
    text_template, entry_animation, exit_animation, animation_duration_ms,
    created_at, updated_at,
    allow_grouping, group_window_ms, interrupt_mode, interruptible
)
    SELECT
        id, profile_id, name, enabled, event_type, priority, duration_ms,
        minimum_quantity, maximum_quantity, required_role,
        show_platform, show_username, show_message, show_quantity,
        text_template, entry_animation, exit_animation, animation_duration_ms,
        created_at, updated_at,
        allow_grouping, group_window_ms, interrupt_mode, interruptible
    FROM alert_rules;

DROP TABLE alert_rules;

ALTER TABLE alert_rules_new RENAME TO alert_rules;

CREATE INDEX idx_alert_rules_profile ON alert_rules (profile_id);
