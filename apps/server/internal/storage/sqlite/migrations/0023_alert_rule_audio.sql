-- Stage 17B: per-alert-rule persistent sound and TTS configuration
-- (docs/alert-audio.md §6/§7), backing internal/domain/alerts.RuleAudio.
--
-- Every column defaults to the safe "no rule-owned audio" zero value, so
-- every existing rule migrates with no behavior change and no existing
-- rule begins producing sound after this migration runs (docs/alert-
-- audio.md §6.5). sound_asset_id has no FK constraint to
-- audioasset_assets deliberately - the reference is tracked separately,
-- transactionally, via alert_rule_audio_asset_refs (migration
-- 0022_audio_assets.sql), the same "no FK on the config column itself"
-- pattern text_template's own placeholder references already use.
ALTER TABLE alert_rules ADD COLUMN sound_enabled INTEGER NOT NULL DEFAULT 0 CHECK (sound_enabled IN (0, 1));
ALTER TABLE alert_rules ADD COLUMN sound_asset_id TEXT NOT NULL DEFAULT '';
ALTER TABLE alert_rules ADD COLUMN sound_volume REAL NOT NULL DEFAULT 1.0 CHECK (sound_volume BETWEEN 0.0 AND 1.0);

ALTER TABLE alert_rules ADD COLUMN tts_enabled INTEGER NOT NULL DEFAULT 0 CHECK (tts_enabled IN (0, 1));
ALTER TABLE alert_rules ADD COLUMN tts_template TEXT NOT NULL DEFAULT '';
ALTER TABLE alert_rules ADD COLUMN tts_volume REAL NOT NULL DEFAULT 1.0 CHECK (tts_volume BETWEEN 0.0 AND 1.0);
