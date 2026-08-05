-- Per-connected-account YouTube category region override.
--
-- Scope note: videoCategories.list requires an explicit ISO 3166-1 alpha-2
-- region (docs/provider-integrations/youtube.md, "Category region"). A
-- channel's own country, when the API provides one, is used as the default;
-- this table holds only an explicit operator override, keyed by the
-- connected account it applies to. Not a secret, not a token, not a general
-- settings bag - one column, one purpose.

CREATE TABLE youtube_channel_settings (
    account_id TEXT PRIMARY KEY
                    REFERENCES connected_accounts (id) ON DELETE CASCADE,
    region     TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
