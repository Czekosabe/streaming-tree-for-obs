package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/streaming-tree/server/internal/domain/audio"
	engagement "github.com/streaming-tree/server/internal/domain/engagement"
	"github.com/streaming-tree/server/internal/domain/platform"
)

// AudioSettingsRepository is the SQLite implementation of
// audio.Repository - a singleton row, mirroring
// OperatorChatPrefsRepository's own exact upsert pattern.
type AudioSettingsRepository struct {
	db *sql.DB
}

// NewAudioSettingsRepository builds a repository over an open database.
func NewAudioSettingsRepository(db *sql.DB) *AudioSettingsRepository {
	return &AudioSettingsRepository{db: db}
}

var _ audio.Repository = (*AudioSettingsRepository)(nil)

func audioSettingsStorageErr(op string, err error) error {
	return fmt.Errorf("%w: %s: %v", audio.ErrStorage, op, err)
}

const audioSettingsColumns = `enabled, provider_mode, enabled_event_types, enabled_provider_ids,
	enabled_source_ids, supporter_only_mode, threshold_currency, threshold_minimum_amount_micros,
	minimum_bits, max_text_length_code_points, per_user_cooldown_seconds, global_cooldown_seconds,
	blocked_words, remove_urls, normalize_repeated_chars, suppress_commands, queue_capacity,
	manual_approval, voice_id, language, speed, volume, public_slug, created_at, updated_at`

func scanAudioSettings(scanner interface{ Scan(...any) error }) (audio.Settings, error) {
	var (
		s                                                                               audio.Settings
		enabled, supporterOnly, removeURLs, normalizeRepeated, suppressCommands, manual int
		providerMode                                                                    string
		enabledEventTypesJSON, enabledProviderIDsJSON, enabledSourceIDsJSON             string
		blockedWordsJSON                                                                string
		thresholdMinimum, minimumBits                                                   sql.NullInt64
		createdAt, updatedAt                                                            string
	)
	if err := scanner.Scan(
		&enabled, &providerMode, &enabledEventTypesJSON, &enabledProviderIDsJSON,
		&enabledSourceIDsJSON, &supporterOnly, &s.ThresholdCurrency, &thresholdMinimum,
		&minimumBits, &s.MaxTextLengthCodePoints, &s.PerUserCooldownSeconds, &s.GlobalCooldownSeconds,
		&blockedWordsJSON, &removeURLs, &normalizeRepeated, &suppressCommands, &s.QueueCapacity,
		&manual, &s.VoiceID, &s.Language, &s.Speed, &s.Volume, &s.PublicSlug, &createdAt, &updatedAt,
	); err != nil {
		return audio.Settings{}, err
	}

	s.Enabled = enabled != 0
	s.ProviderMode = audio.ProviderMode(providerMode)
	s.SupporterOnlyMode = supporterOnly != 0
	s.RemoveURLs = removeURLs != 0
	s.NormalizeRepeatedChars = normalizeRepeated != 0
	s.SuppressCommands = suppressCommands != 0
	s.ManualApproval = manual != 0

	if thresholdMinimum.Valid {
		v := thresholdMinimum.Int64
		s.ThresholdMinimumAmountMicros = &v
	}
	if minimumBits.Valid {
		v := minimumBits.Int64
		s.MinimumBits = &v
	}

	var eventTypes []string
	if err := json.Unmarshal([]byte(enabledEventTypesJSON), &eventTypes); err != nil {
		return audio.Settings{}, fmt.Errorf("parse enabled_event_types: %w", err)
	}
	s.EnabledEventTypes = make([]engagement.Type, len(eventTypes))
	for i, t := range eventTypes {
		s.EnabledEventTypes[i] = engagement.Type(t)
	}

	var providerIDs []string
	if err := json.Unmarshal([]byte(enabledProviderIDsJSON), &providerIDs); err != nil {
		return audio.Settings{}, fmt.Errorf("parse enabled_provider_ids: %w", err)
	}
	s.EnabledProviderIDs = make([]engagement.ProviderID, len(providerIDs))
	for i, p := range providerIDs {
		s.EnabledProviderIDs[i] = engagement.ProviderID(p)
	}

	if err := json.Unmarshal([]byte(enabledSourceIDsJSON), &s.EnabledSourceIDs); err != nil {
		return audio.Settings{}, fmt.Errorf("parse enabled_source_ids: %w", err)
	}
	if err := json.Unmarshal([]byte(blockedWordsJSON), &s.BlockedWords); err != nil {
		return audio.Settings{}, fmt.Errorf("parse blocked_words: %w", err)
	}

	var err error
	if s.CreatedAt, err = platform.ParseTimestamp(createdAt); err != nil {
		return audio.Settings{}, fmt.Errorf("parse created_at %q: %w", createdAt, err)
	}
	if s.UpdatedAt, err = platform.ParseTimestamp(updatedAt); err != nil {
		return audio.Settings{}, fmt.Errorf("parse updated_at %q: %w", updatedAt, err)
	}
	return s, nil
}

// GetSettings returns the singleton settings row, if any.
func (r *AudioSettingsRepository) GetSettings(ctx context.Context) (audio.Settings, bool, error) {
	row := r.db.QueryRowContext(ctx, `SELECT `+audioSettingsColumns+` FROM audio_settings WHERE id = 'singleton'`)
	s, err := scanAudioSettings(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return audio.Settings{}, false, nil
		}
		return audio.Settings{}, false, audioSettingsStorageErr("get settings", err)
	}
	return s, true, nil
}

// SetSettings replaces the singleton settings row in full.
func (r *AudioSettingsRepository) SetSettings(ctx context.Context, s audio.Settings, now time.Time) (audio.Settings, error) {
	eventTypes := make([]string, len(s.EnabledEventTypes))
	for i, t := range s.EnabledEventTypes {
		eventTypes[i] = string(t)
	}
	providerIDs := make([]string, len(s.EnabledProviderIDs))
	for i, p := range s.EnabledProviderIDs {
		providerIDs[i] = string(p)
	}
	sourceIDs := s.EnabledSourceIDs
	if sourceIDs == nil {
		sourceIDs = []string{}
	}
	blockedWords := s.BlockedWords
	if blockedWords == nil {
		blockedWords = []string{}
	}

	eventTypesJSON, err := json.Marshal(eventTypes)
	if err != nil {
		return audio.Settings{}, audioSettingsStorageErr("marshal enabled_event_types", err)
	}
	providerIDsJSON, err := json.Marshal(providerIDs)
	if err != nil {
		return audio.Settings{}, audioSettingsStorageErr("marshal enabled_provider_ids", err)
	}
	sourceIDsJSON, err := json.Marshal(sourceIDs)
	if err != nil {
		return audio.Settings{}, audioSettingsStorageErr("marshal enabled_source_ids", err)
	}
	blockedWordsJSON, err := json.Marshal(blockedWords)
	if err != nil {
		return audio.Settings{}, audioSettingsStorageErr("marshal blocked_words", err)
	}

	nowText := platform.FormatTimestamp(now)
	createdAtText := nowText
	if !s.CreatedAt.IsZero() {
		createdAtText = platform.FormatTimestamp(s.CreatedAt)
	}

	if _, err := r.db.ExecContext(ctx, `
        INSERT INTO audio_settings (
            id, enabled, provider_mode, enabled_event_types, enabled_provider_ids,
            enabled_source_ids, supporter_only_mode, threshold_currency, threshold_minimum_amount_micros,
            minimum_bits, max_text_length_code_points, per_user_cooldown_seconds, global_cooldown_seconds,
            blocked_words, remove_urls, normalize_repeated_chars, suppress_commands, queue_capacity,
            manual_approval, voice_id, language, speed, volume, public_slug, created_at, updated_at
        ) VALUES ('singleton', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT (id) DO UPDATE SET
            enabled = excluded.enabled,
            provider_mode = excluded.provider_mode,
            enabled_event_types = excluded.enabled_event_types,
            enabled_provider_ids = excluded.enabled_provider_ids,
            enabled_source_ids = excluded.enabled_source_ids,
            supporter_only_mode = excluded.supporter_only_mode,
            threshold_currency = excluded.threshold_currency,
            threshold_minimum_amount_micros = excluded.threshold_minimum_amount_micros,
            minimum_bits = excluded.minimum_bits,
            max_text_length_code_points = excluded.max_text_length_code_points,
            per_user_cooldown_seconds = excluded.per_user_cooldown_seconds,
            global_cooldown_seconds = excluded.global_cooldown_seconds,
            blocked_words = excluded.blocked_words,
            remove_urls = excluded.remove_urls,
            normalize_repeated_chars = excluded.normalize_repeated_chars,
            suppress_commands = excluded.suppress_commands,
            queue_capacity = excluded.queue_capacity,
            manual_approval = excluded.manual_approval,
            voice_id = excluded.voice_id,
            language = excluded.language,
            speed = excluded.speed,
            volume = excluded.volume,
            public_slug = excluded.public_slug,
            updated_at = excluded.updated_at
    `,
		boolToInt(s.Enabled), string(s.ProviderMode), string(eventTypesJSON), string(providerIDsJSON),
		string(sourceIDsJSON), boolToInt(s.SupporterOnlyMode), s.ThresholdCurrency, s.ThresholdMinimumAmountMicros,
		s.MinimumBits, s.MaxTextLengthCodePoints, s.PerUserCooldownSeconds, s.GlobalCooldownSeconds,
		string(blockedWordsJSON), boolToInt(s.RemoveURLs), boolToInt(s.NormalizeRepeatedChars), boolToInt(s.SuppressCommands),
		s.QueueCapacity, boolToInt(s.ManualApproval), s.VoiceID, s.Language, s.Speed, s.Volume, s.PublicSlug,
		createdAtText, nowText,
	); err != nil {
		return audio.Settings{}, audioSettingsStorageErr("set settings", err)
	}

	saved, found, err := r.GetSettings(ctx)
	if err != nil {
		return audio.Settings{}, err
	}
	if !found {
		return audio.Settings{}, audioSettingsStorageErr("set settings", errors.New("row missing immediately after upsert"))
	}
	return saved, nil
}
