-- Stage 21: the one persisted first-run onboarding preference.
--
-- Scope note: exactly like update_preferences (Stage 20B), this table
-- holds a UI-flow preference only - no secret, no machine/installation
-- id, no step-by-step history. See docs/onboarding.md §4.

CREATE TABLE onboarding_state (
    id             INTEGER PRIMARY KEY CHECK (id = 1),
    status         TEXT NOT NULL CHECK (status IN ('pending', 'completed', 'dismissed')),
    schema_version INTEGER NOT NULL,
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);

-- Existing-user migration rule (docs/onboarding.md §4.3): a database
-- where nothing has ever been touched beyond the one-time seed migration
-- (0002_seed_default_platforms.sql, which always creates exactly four
-- *disabled* platforms; 0003_platform_output_settings.sql, which then
-- gives every platform - seeded or not - a default settings row with
-- server_url = '' specifically so "a row's mere existence never implies
-- readiness to stream", per that migration's own comment) starts
-- 'pending' - the frontend auto-shows onboarding once. Any real prior
-- use - a connected account, an output server actually configured
-- (server_url <> '', never mere row existence, which the seed above
-- already produces for every platform), an enabled seed platform, or any
-- user-created platform beyond the four seeded rows - starts 'dismissed'
-- instead: onboarding stays available on request, never auto-shown to
-- an operator who was already using the application before this stage
-- existed. This runs once, for every database (fresh or years-old) that
-- applies this migration, exactly like every other migration in this
-- project.
INSERT INTO onboarding_state (id, status, schema_version, created_at, updated_at)
SELECT 1,
    CASE WHEN EXISTS (
        SELECT 1 FROM connected_accounts
        UNION ALL
        SELECT 1 FROM platform_output_settings WHERE server_url <> ''
        UNION ALL
        SELECT 1 FROM platforms WHERE enabled = 1
        UNION ALL
        SELECT 1 FROM platforms WHERE id NOT IN
            ('pf_seed_twitch', 'pf_seed_youtube', 'pf_seed_kick', 'pf_seed_tiktok')
    ) THEN 'dismissed' ELSE 'pending' END,
    1, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), strftime('%Y-%m-%dT%H:%M:%fZ', 'now');
