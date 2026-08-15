-- Stage 17B: template-level alert-audio preset persistence (docs/
-- alert-audio.md §10.5), backing internal/domain/visualtemplate.
-- RuleAudioPreset. Only a package v2 import ever writes a non-default
-- value here (docs/alert-audio.md §10.7: the plain Stage 14A JSON
-- create/import path never carries audio) - every existing template
-- migrates with the safe "no preset" zero value and no behavior
-- change. sound_asset_id has no FK constraint to audioasset_assets,
-- mirroring migration 0023_alert_rule_audio.sql's own rule-level
-- columns exactly - the reference is tracked separately,
-- transactionally, via alert_template_audio_asset_refs (migration
-- 0022_audio_assets.sql).
ALTER TABLE visual_templates ADD COLUMN audio_sound_enabled INTEGER NOT NULL DEFAULT 0 CHECK (audio_sound_enabled IN (0, 1));
ALTER TABLE visual_templates ADD COLUMN audio_sound_asset_id TEXT NOT NULL DEFAULT '';
ALTER TABLE visual_templates ADD COLUMN audio_sound_volume REAL NOT NULL DEFAULT 1.0 CHECK (audio_sound_volume BETWEEN 0.0 AND 1.0);

ALTER TABLE visual_templates ADD COLUMN audio_tts_enabled INTEGER NOT NULL DEFAULT 0 CHECK (audio_tts_enabled IN (0, 1));
ALTER TABLE visual_templates ADD COLUMN audio_tts_template TEXT NOT NULL DEFAULT '';
ALTER TABLE visual_templates ADD COLUMN audio_tts_volume REAL NOT NULL DEFAULT 1.0 CHECK (audio_tts_volume BETWEEN 0.0 AND 1.0);
