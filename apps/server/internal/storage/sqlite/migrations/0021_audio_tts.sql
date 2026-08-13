-- Stage 17A: the global text-to-speech/audio settings singleton - see
-- docs/audio-tts.md and internal/domain/audio's own package doc comment.
--
-- audio_settings holds exactly one row (id fixed to 'singleton', mirroring
-- how other singleton-preference tables in this project are shaped) and
-- only safe, bounded configuration - never a queued utterance, generated
-- audio bytes, a cooldown timestamp, or any chat/donation message text.
-- Runtime queue/cooldown/provider state lives only in internal/audio's
-- own in-memory manager and is never written here.
--
-- The four list-shaped fields (enabled event types, enabled provider ids,
-- enabled source ids, blocked words) are stored as JSON-encoded TEXT
-- columns rather than join tables, since this table only ever has one
-- row - mirrors the JSON-column precedent visualdesign_repository.go
-- already established in this codebase for a similar "one row, several
-- bounded list-shaped fields" shape.
CREATE TABLE audio_settings (
    id                      TEXT    PRIMARY KEY CHECK (id = 'singleton'),

    enabled                 INTEGER NOT NULL DEFAULT 0 CHECK (enabled IN (0, 1)),
    provider_mode           TEXT    NOT NULL DEFAULT 'disabled'
                                CHECK (provider_mode IN ('disabled', 'system', 'local', 'cloud')),

    enabled_event_types     TEXT    NOT NULL DEFAULT '[]',
    enabled_provider_ids    TEXT    NOT NULL DEFAULT '[]',
    enabled_source_ids      TEXT    NOT NULL DEFAULT '[]',

    supporter_only_mode     INTEGER NOT NULL DEFAULT 0 CHECK (supporter_only_mode IN (0, 1)),

    threshold_currency              TEXT    NOT NULL DEFAULT '',
    threshold_minimum_amount_micros INTEGER,
    minimum_bits                    INTEGER,

    max_text_length_code_points INTEGER NOT NULL DEFAULT 500,
    per_user_cooldown_seconds   INTEGER NOT NULL DEFAULT 30,
    global_cooldown_seconds     INTEGER NOT NULL DEFAULT 3,
    blocked_words                TEXT   NOT NULL DEFAULT '[]',
    remove_urls                  INTEGER NOT NULL DEFAULT 1 CHECK (remove_urls IN (0, 1)),
    normalize_repeated_chars     INTEGER NOT NULL DEFAULT 1 CHECK (normalize_repeated_chars IN (0, 1)),
    suppress_commands            INTEGER NOT NULL DEFAULT 1 CHECK (suppress_commands IN (0, 1)),

    queue_capacity            INTEGER NOT NULL DEFAULT 100,
    manual_approval           INTEGER NOT NULL DEFAULT 0 CHECK (manual_approval IN (0, 1)),

    voice_id                  TEXT    NOT NULL DEFAULT '',
    language                  TEXT    NOT NULL DEFAULT '',
    speed                     REAL    NOT NULL DEFAULT 1.0,
    volume                    REAL    NOT NULL DEFAULT 1.0,

    public_slug               TEXT    NOT NULL DEFAULT '',

    created_at                TEXT    NOT NULL,
    updated_at                TEXT    NOT NULL
);
