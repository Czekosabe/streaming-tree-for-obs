-- One-time seed: four configured platforms, one per built-in provider.
--
-- This runs exactly once, when a brand-new database is migrated. It is recorded
-- in schema_migrations like any other migration, so deleting a seeded platform
-- is permanent: restarting the application does NOT bring it back.
--
-- Every seeded platform is DISABLED and carries no runtime state. Nothing here
-- is a stream key, token or credential - only example metadata mirroring what
-- the dashboard showed before persistence existed.
--
-- IDs are stable and predefined so documentation and the integration script can
-- refer to them. Platforms created later get random backend-generated IDs.

INSERT INTO platforms (id, provider_id, display_name, enabled, sort_order, created_at, updated_at)
VALUES
    ('pf_seed_twitch',  'twitch',  'Twitch',       0, 0, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    ('pf_seed_youtube', 'youtube', 'YouTube Live', 0, 1, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    ('pf_seed_kick',    'kick',    'Kick',         0, 2, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    ('pf_seed_tiktok',  'tiktok',  'TikTok Live',  0, 3, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));

-- NULL is used for every field the provider does not support, which is why the
-- four rows below differ in shape rather than all carrying empty strings.

INSERT INTO platform_metadata
    (platform_id, title, description, category, language, visibility, mature_content, dvr, latency_mode, updated_at)
VALUES
    ('pf_seed_twitch',
     'Building Streaming Tree for OBS - foundations',
     NULL,
     'Software and Game Development',
     'pl',
     NULL,
     0,
     NULL,
     'low',
     strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),

    ('pf_seed_youtube',
     'Building Streaming Tree for OBS - foundations',
     'Live coding session. We are building a local multistreaming control panel for OBS.',
     'Science & Technology',
     'pl',
     'public',
     0,
     1,
     'low',
     strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),

    ('pf_seed_kick',
     'Building Streaming Tree for OBS',
     NULL,
     'Just Chatting',
     'pl',
     NULL,
     0,
     NULL,
     NULL,
     strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),

    ('pf_seed_tiktok',
     'Building a multistream tool',
     NULL,
     'Gaming',
     NULL,
     NULL,
     NULL,
     NULL,
     NULL,
     strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));

-- Twitch is the only seeded provider with tag support, so it is the only one
-- with tag rows. Positions are explicit because tag order is user-visible.
INSERT INTO platform_metadata_tags (platform_id, position, value)
VALUES
    ('pf_seed_twitch', 0, 'programming'),
    ('pf_seed_twitch', 1, 'go'),
    ('pf_seed_twitch', 2, 'react'),
    ('pf_seed_twitch', 3, 'obs');
