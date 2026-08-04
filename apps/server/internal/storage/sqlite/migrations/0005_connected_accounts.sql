-- Provider-independent connected-account foundation.
--
-- Scope note: these tables hold identity and status only. No access token,
-- refresh token, device code, client secret, or raw provider response is
-- ever stored here - the complete OAuth token bundle lives in the OS
-- credential store (internal/secrets), one atomically-replaced secret per
-- connected account, exactly like a destination stream key. See
-- internal/domain/account for the full design.

CREATE TABLE connected_accounts (
    id                 TEXT    PRIMARY KEY,
    provider_id        TEXT    NOT NULL,
    -- The provider's own stable user identifier (Twitch: Get Users "id").
    -- Never the login or display name, both of which a user can change.
    provider_user_id   TEXT    NOT NULL,
    login              TEXT    NOT NULL,
    display_name       TEXT    NOT NULL,
    avatar_url         TEXT,
    -- Stable status identifiers, not booleans: "connected",
    -- "reconnect_required" - see account.Status in the Go domain.
    status             TEXT    NOT NULL,
    last_validated_at  TEXT,
    created_at         TEXT    NOT NULL,
    updated_at         TEXT    NOT NULL
);

-- One provider identity is never represented by two connected-account rows:
-- a repeated device-flow authorization for the same Twitch user is a
-- reconnect (rotate the token, update this row), never a duplicate insert.
CREATE UNIQUE INDEX idx_connected_accounts_provider_identity
    ON connected_accounts (provider_id, provider_user_id);

CREATE TABLE connected_account_scopes (
    account_id TEXT NOT NULL
                    REFERENCES connected_accounts (id) ON DELETE CASCADE,
    scope      TEXT NOT NULL,
    PRIMARY KEY (account_id, scope)
);

CREATE TABLE platform_account_links (
    -- One configured destination links to at most one connected account -
    -- platform_id is the primary key, not part of a composite one.
    platform_id TEXT PRIMARY KEY
                     REFERENCES platforms (id) ON DELETE CASCADE,
    account_id  TEXT NOT NULL
                     REFERENCES connected_accounts (id) ON DELETE CASCADE,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);

-- Not unique: one connected account may back several destinations for the
-- same provider (two Twitch channel configurations under one Twitch login,
-- for instance), so this index exists for lookup speed only.
CREATE INDEX idx_platform_account_links_account_id
    ON platform_account_links (account_id);
