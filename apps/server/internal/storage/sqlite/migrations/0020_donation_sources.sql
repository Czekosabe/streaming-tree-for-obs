-- Stage 16A: external donation sources (StreamElements first - see
-- docs/provider-integrations/external-donations.md) plus the schema
-- changes needed to make the generic "donation" event alert-capable.
--
-- donation_sources is a small, deliberately separate domain from
-- connected_accounts - see internal/domain/donationsource's own package
-- doc comment for the full reasoning (a StreamElements personal JWT has
-- no login, no OAuth scopes, no refresh token, and is not a streaming
-- destination). Like connected_account_engagement_settings, this table
-- holds only safe, non-secret metadata and the operator's explicit
-- enable/disable choice - no runtime connection state (no WebSocket
-- session, no reconnect token, no last error, no event data), and never
-- the credential itself, which lives only in the OS credential store
-- (internal/secrets, SecretTypeDonationSourceToken), addressed by this
-- table's own id.
CREATE TABLE donation_sources (
    id                  TEXT    PRIMARY KEY,
    provider_id         TEXT    NOT NULL CHECK (provider_id IN ('streamelements')),
    label               TEXT    NOT NULL,
    enabled             INTEGER NOT NULL DEFAULT 0 CHECK (enabled IN (0, 1)),

    -- remote_channel_id: the donation service's own channel/account
    -- identifier (StreamElements: the "Account ID" shown next to the
    -- JWT on the operator's own dashboard) - safe to persist, never a
    -- secret, and never sufficient on its own to authenticate as the
    -- operator.
    remote_channel_id   TEXT    NOT NULL DEFAULT '',

    created_at          TEXT    NOT NULL,
    updated_at          TEXT    NOT NULL
);

-- alert_rules.event_type is a literal CHECK list, so SQLite's lack of
-- `ALTER TABLE ... ALTER CONSTRAINT` means it can only be widened by the
-- same safe rebuild pattern migration 0019 already used to add the
-- YouTube event types: create the replacement table, copy every existing
-- row across unchanged, drop the old table, rename the new one into
-- place - all inside this migration's own transaction. Every existing
-- rule's own columns survive byte-for-byte; only the CHECK list widens,
-- adding the one new value 'donation'.
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
                                 'youtube_super_chat', 'youtube_super_sticker',
                                 'donation'
                             )),

    priority                INTEGER NOT NULL DEFAULT 50 CHECK (priority BETWEEN 0 AND 100),
    duration_ms             INTEGER NOT NULL DEFAULT 5000 CHECK (duration_ms BETWEEN 1000 AND 30000),

    minimum_quantity        INTEGER,
    maximum_quantity        INTEGER,

    currency                TEXT    NOT NULL DEFAULT '',
    minimum_amount_micros   INTEGER,
    maximum_amount_micros   INTEGER,

    required_role           TEXT NOT NULL DEFAULT 'everyone'
                             CHECK (required_role IN ('everyone', 'subscriber', 'vip', 'moderator', 'broadcaster')),

    show_platform           INTEGER NOT NULL DEFAULT 1 CHECK (show_platform IN (0, 1)),
    show_username           INTEGER NOT NULL DEFAULT 1 CHECK (show_username IN (0, 1)),
    show_message            INTEGER NOT NULL DEFAULT 1 CHECK (show_message IN (0, 1)),
    show_quantity            INTEGER NOT NULL DEFAULT 1 CHECK (show_quantity IN (0, 1)),
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
    minimum_quantity, maximum_quantity,
    currency, minimum_amount_micros, maximum_amount_micros,
    required_role,
    show_platform, show_username, show_message, show_quantity, show_amount,
    text_template, entry_animation, exit_animation, animation_duration_ms,
    created_at, updated_at,
    allow_grouping, group_window_ms, interrupt_mode, interruptible
)
    SELECT
        id, profile_id, name, enabled, event_type, priority, duration_ms,
        minimum_quantity, maximum_quantity,
        currency, minimum_amount_micros, maximum_amount_micros,
        required_role,
        show_platform, show_username, show_message, show_quantity, show_amount,
        text_template, entry_animation, exit_animation, animation_duration_ms,
        created_at, updated_at,
        allow_grouping, group_window_ms, interrupt_mode, interruptible
    FROM alert_rules;

DROP TABLE alert_rules;

ALTER TABLE alert_rules_new RENAME TO alert_rules;

CREATE INDEX idx_alert_rules_profile ON alert_rules (profile_id);

-- alert_rule_providers (no FOREIGN KEY on provider_id today) is untouched
-- - a new provider value ('streamelements') needs no schema change there,
-- see internal/domain/alerts.ValidateProviders for its own application-
-- level check.
--
-- alert_rule_accounts, by contrast, DOES need a schema change: its
-- connected_account_id column is a hard FOREIGN KEY into
-- connected_accounts. A donation source is deliberately not a row in
-- connected_accounts (see this migration's own top comment), so that
-- foreign key would make it impossible to ever select a donation source
-- in an alert rule's account/source filter. This rebuild drops the
-- foreign key constraint - existence is now validated in application
-- code only (internal/alerts/wiring.go's combined account-or-donation-
-- source lookup), the same looser pattern alert_rule_providers.
-- provider_id already used. The column itself is intentionally left
-- named connected_account_id (not renamed) to minimize unrelated schema
-- churn - internal/domain/alerts.Rule.Accounts is untyped ([]string) and
-- was already documented as "a connected account or, as of Stage 16A, a
-- donation source id" at the Go layer, not the SQL layer.
CREATE TABLE alert_rule_accounts_new (
    rule_id               TEXT NOT NULL REFERENCES alert_rules (id) ON DELETE CASCADE,
    connected_account_id  TEXT NOT NULL,
    PRIMARY KEY (rule_id, connected_account_id)
);

INSERT INTO alert_rule_accounts_new (rule_id, connected_account_id)
    SELECT rule_id, connected_account_id FROM alert_rule_accounts;

DROP TABLE alert_rule_accounts;

ALTER TABLE alert_rule_accounts_new RENAME TO alert_rule_accounts;
