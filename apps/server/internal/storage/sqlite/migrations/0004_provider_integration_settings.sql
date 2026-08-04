-- Non-secret, provider-scoped integration configuration.
--
-- Scope note: this table holds application-level configuration for talking
-- to a provider's API at all - today, only a Twitch Client ID. A Client ID
-- is not a secret (Twitch's own public clients embed it in every device-flow
-- request), but it is still managed consistently: this row exists only when
-- no STREAMING_TREE_TWITCH_CLIENT_ID environment override is set, and it
-- never holds a client secret, an access token, or any other credential -
-- see internal/domain/account/validation.go for the fields explicitly
-- rejected at the API boundary.

CREATE TABLE provider_integration_settings (
    provider_id TEXT PRIMARY KEY,
    client_id   TEXT NOT NULL,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);
