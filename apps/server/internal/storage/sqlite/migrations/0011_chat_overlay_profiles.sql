-- Stage 10 OBS Browser Source chat overlay: persisted profile settings
-- only.
--
-- Scope note: exactly like operator_chat_preferences (Stage 9), this
-- schema holds nothing about live chat content. No message text, no
-- username treated as authoritative identity, no OAuth token, no
-- EventSub session data, no raw provider event. The public overlay
-- projection itself (internal/chatoverlay) is in-memory only and is gone
-- on every backend restart - see docs/progress.md's Stage 10 entry.
--
-- Explicit columns, not a settings JSON blob: every field below is a
-- fixed, individually validated setting (see internal/domain/chatoverlay's
-- own validation), never an arbitrary CSS/HTML/JS string a viewer's
-- browser would render unsanitized.

CREATE TABLE chat_overlays (
    id                        TEXT    PRIMARY KEY,
    -- A separate, high-entropy, rotatable value - never the same as id.
    -- See internal/domain/chatoverlay's own doc comment for why this is
    -- an unguessable locator, not a credential.
    public_slug               TEXT    NOT NULL UNIQUE,
    name                      TEXT    NOT NULL,
    enabled                   INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),

    layout_mode               TEXT    NOT NULL DEFAULT 'horizontal'
                                       CHECK (layout_mode IN ('horizontal', 'vertical')),
    stack_direction            TEXT    NOT NULL DEFAULT 'bottom_up'
                                       CHECK (stack_direction IN ('top_down', 'bottom_up')),
    horizontal_alignment      TEXT    NOT NULL DEFAULT 'left'
                                       CHECK (horizontal_alignment IN ('left', 'center', 'right')),

    show_platform_icon        INTEGER NOT NULL DEFAULT 1 CHECK (show_platform_icon IN (0, 1)),
    show_platform_name        INTEGER NOT NULL DEFAULT 0 CHECK (show_platform_name IN (0, 1)),
    show_account_label        INTEGER NOT NULL DEFAULT 0 CHECK (show_account_label IN (0, 1)),
    show_avatar                INTEGER NOT NULL DEFAULT 0 CHECK (show_avatar IN (0, 1)),
    show_badges                INTEGER NOT NULL DEFAULT 1 CHECK (show_badges IN (0, 1)),
    show_timestamp             INTEGER NOT NULL DEFAULT 0 CHECK (show_timestamp IN (0, 1)),
    show_activity_events      INTEGER NOT NULL DEFAULT 1 CHECK (show_activity_events IN (0, 1)),
    show_deleted_placeholder  INTEGER NOT NULL DEFAULT 0 CHECK (show_deleted_placeholder IN (0, 1)),
    hide_commands              INTEGER NOT NULL DEFAULT 1 CHECK (hide_commands IN (0, 1)),
    hide_bots                  INTEGER NOT NULL DEFAULT 1 CHECK (hide_bots IN (0, 1)),

    max_visible_items          INTEGER NOT NULL DEFAULT 30,
    message_lifetime_seconds  INTEGER NOT NULL DEFAULT 0,

    font_family                TEXT    NOT NULL DEFAULT 'sans_serif'
                                       CHECK (font_family IN ('sans_serif', 'serif', 'monospace', 'rounded')),
    font_size                  INTEGER NOT NULL DEFAULT 16,
    font_weight                INTEGER NOT NULL DEFAULT 400,
    line_height                REAL    NOT NULL DEFAULT 1.4,
    text_color                 TEXT    NOT NULL DEFAULT '#FFFFFF',
    username_color_mode       TEXT    NOT NULL DEFAULT 'provider'
                                       CHECK (username_color_mode IN ('provider', 'fixed')),
    bubble_color                TEXT    NOT NULL DEFAULT '#000000',
    bubble_opacity              REAL    NOT NULL DEFAULT 0.45,
    border_radius                INTEGER NOT NULL DEFAULT 8,
    item_spacing                  INTEGER NOT NULL DEFAULT 6,
    text_outline                  INTEGER NOT NULL DEFAULT 1 CHECK (text_outline IN (0, 1)),
    text_shadow                   INTEGER NOT NULL DEFAULT 0 CHECK (text_shadow IN (0, 1)),

    entry_animation              TEXT    NOT NULL DEFAULT 'fade'
                                       CHECK (entry_animation IN ('none', 'fade', 'slide_up', 'slide_left', 'scale')),
    exit_animation                TEXT    NOT NULL DEFAULT 'fade'
                                       CHECK (exit_animation IN ('none', 'fade', 'slide_up', 'slide_left', 'scale')),
    animation_duration_ms        INTEGER NOT NULL DEFAULT 250,

    highlight_broadcaster        INTEGER NOT NULL DEFAULT 1 CHECK (highlight_broadcaster IN (0, 1)),
    highlight_moderators          INTEGER NOT NULL DEFAULT 1 CHECK (highlight_moderators IN (0, 1)),
    highlight_subscribers          INTEGER NOT NULL DEFAULT 0 CHECK (highlight_subscribers IN (0, 1)),
    highlight_vips                  INTEGER NOT NULL DEFAULT 0 CHECK (highlight_vips IN (0, 1)),

    -- Documented canonical UI language for this profile's own generic
    -- strings (a deleted-message placeholder, and similar) - never
    -- derived silently from whoever last edited it. Chat text/usernames
    -- themselves are always rendered verbatim, untranslated, regardless
    -- of this setting.
    language                       TEXT    NOT NULL DEFAULT 'en' CHECK (language IN ('en', 'pl')),

    created_at                     TEXT    NOT NULL,
    updated_at                     TEXT    NOT NULL
);

-- One connected account may be selected by several overlay profiles, and
-- one overlay may select several accounts - a genuine many-to-many.
CREATE TABLE chat_overlay_accounts (
    overlay_id TEXT NOT NULL REFERENCES chat_overlays (id) ON DELETE CASCADE,
    account_id TEXT NOT NULL REFERENCES connected_accounts (id) ON DELETE CASCADE,
    PRIMARY KEY (overlay_id, account_id)
);

-- Deliberately separate from Stage 9's operator_chat_hidden_users: a user
-- may remain visible to the operator while being hidden from the public
-- overlay, and vice versa - see internal/domain/chatoverlay's own doc
-- comment.
CREATE TABLE chat_overlay_hidden_users (
    overlay_id           TEXT NOT NULL REFERENCES chat_overlays (id) ON DELETE CASCADE,
    provider_id          TEXT NOT NULL,
    connected_account_id TEXT NOT NULL REFERENCES connected_accounts (id) ON DELETE CASCADE,
    provider_user_id     TEXT NOT NULL,
    label                TEXT,
    created_at           TEXT NOT NULL,
    PRIMARY KEY (overlay_id, provider_id, connected_account_id, provider_user_id)
);

-- Stage 10 supports safe literal matching only - never a regular
-- expression, glob, or executable expression. See
-- internal/chatoverlay/filtering.go for the matching semantics
-- themselves.
CREATE TABLE chat_overlay_blocked_terms (
    id            TEXT NOT NULL PRIMARY KEY,
    overlay_id    TEXT NOT NULL REFERENCES chat_overlays (id) ON DELETE CASCADE,
    value         TEXT NOT NULL,
    -- The normalized (case-folded, trimmed) form of value, used only for
    -- the uniqueness index below - value itself stays exactly as entered
    -- for display in the management UI.
    normalized_value TEXT NOT NULL,
    match_mode    TEXT NOT NULL CHECK (match_mode IN ('contains', 'whole_word')),
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);

CREATE UNIQUE INDEX idx_chat_overlay_blocked_terms_unique
    ON chat_overlay_blocked_terms (overlay_id, normalized_value);

-- One row per normalized activity type this overlay shows inline
-- (follow, subscription, ...). Absent overlay_id rows mean "show every
-- activity type" - see internal/domain/chatoverlay's own default policy.
CREATE TABLE chat_overlay_activity_types (
    overlay_id    TEXT NOT NULL REFERENCES chat_overlays (id) ON DELETE CASCADE,
    activity_type TEXT NOT NULL,
    PRIMARY KEY (overlay_id, activity_type)
);
