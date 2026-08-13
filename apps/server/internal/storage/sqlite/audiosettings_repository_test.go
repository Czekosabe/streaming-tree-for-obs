package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/audio"
	engagement "github.com/streaming-tree/server/internal/domain/engagement"
)

func testAudioSettings() audio.Settings {
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	minimum := int64(5_000_000)
	bits := int64(100)
	return audio.Settings{
		Enabled:                      true,
		ProviderMode:                 audio.ProviderModeSystem,
		EnabledEventTypes:            []engagement.Type{engagement.TypeChatMessage, engagement.TypeDonation},
		EnabledProviderIDs:           []engagement.ProviderID{engagement.ProviderTwitch},
		EnabledSourceIDs:             []string{"acct_1", "donsrc_1"},
		SupporterOnlyMode:            true,
		ThresholdCurrency:            "USD",
		ThresholdMinimumAmountMicros: &minimum,
		MinimumBits:                  &bits,
		MaxTextLengthCodePoints:      400,
		PerUserCooldownSeconds:       45,
		GlobalCooldownSeconds:        5,
		BlockedWords:                 []string{"spam", "badword"},
		RemoveURLs:                   true,
		NormalizeRepeatedChars:       true,
		SuppressCommands:             true,
		QueueCapacity:                150,
		ManualApproval:               true,
		VoiceID:                      "voice-1",
		Language:                     "en-US",
		Speed:                        1.5,
		Volume:                       0.8,
		PublicSlug:                   "abc123",
		CreatedAt:                    now,
		UpdatedAt:                    now,
	}
}

func TestAudioSettingsGetMissingReturnsNotFound(t *testing.T) {
	db := newTestDB(t)
	repo := NewAudioSettingsRepository(db.DB)
	_, found, err := repo.GetSettings(context.Background())
	if err != nil {
		t.Fatalf("GetSettings() error = %v", err)
	}
	if found {
		t.Fatal("found = true for a database with no settings ever written")
	}
}

func TestAudioSettingsSetThenGetRoundTrips(t *testing.T) {
	db := newTestDB(t)
	repo := NewAudioSettingsRepository(db.DB)
	want := testAudioSettings()

	saved, err := repo.SetSettings(context.Background(), want, want.UpdatedAt)
	if err != nil {
		t.Fatalf("SetSettings() error = %v", err)
	}
	if saved.ProviderMode != audio.ProviderModeSystem || saved.PublicSlug != "abc123" {
		t.Fatalf("saved = %+v, want provider mode system and slug abc123", saved)
	}

	got, found, err := repo.GetSettings(context.Background())
	if err != nil {
		t.Fatalf("GetSettings() error = %v", err)
	}
	if !found {
		t.Fatal("found = false after SetSettings")
	}
	if len(got.EnabledEventTypes) != 2 || got.EnabledEventTypes[0] != engagement.TypeChatMessage || got.EnabledEventTypes[1] != engagement.TypeDonation {
		t.Errorf("EnabledEventTypes = %+v, want [chat.message donation]", got.EnabledEventTypes)
	}
	if len(got.EnabledProviderIDs) != 1 || got.EnabledProviderIDs[0] != engagement.ProviderTwitch {
		t.Errorf("EnabledProviderIDs = %+v, want [twitch]", got.EnabledProviderIDs)
	}
	if len(got.EnabledSourceIDs) != 2 || got.EnabledSourceIDs[0] != "acct_1" || got.EnabledSourceIDs[1] != "donsrc_1" {
		t.Errorf("EnabledSourceIDs = %+v, want [acct_1 donsrc_1]", got.EnabledSourceIDs)
	}
	if len(got.BlockedWords) != 2 || got.BlockedWords[0] != "spam" {
		t.Errorf("BlockedWords = %+v, want [spam badword]", got.BlockedWords)
	}
	if got.ThresholdMinimumAmountMicros == nil || *got.ThresholdMinimumAmountMicros != 5_000_000 {
		t.Errorf("ThresholdMinimumAmountMicros = %v, want 5000000", got.ThresholdMinimumAmountMicros)
	}
	if got.MinimumBits == nil || *got.MinimumBits != 100 {
		t.Errorf("MinimumBits = %v, want 100", got.MinimumBits)
	}
	if got.Speed != 1.5 || got.Volume != 0.8 {
		t.Errorf("Speed/Volume = %v/%v, want 1.5/0.8", got.Speed, got.Volume)
	}
	if got.QueueCapacity != 150 {
		t.Errorf("QueueCapacity = %v, want 150", got.QueueCapacity)
	}
}

func TestAudioSettingsUpdateReplacesRowInPlace(t *testing.T) {
	db := newTestDB(t)
	repo := NewAudioSettingsRepository(db.DB)
	first := testAudioSettings()
	if _, err := repo.SetSettings(context.Background(), first, first.UpdatedAt); err != nil {
		t.Fatalf("SetSettings() first error = %v", err)
	}

	second := first
	second.Enabled = false
	second.ProviderMode = audio.ProviderModeDisabled
	second.VoiceID = ""
	second.QueueCapacity = 50
	second.UpdatedAt = first.UpdatedAt.Add(time.Hour)

	saved, err := repo.SetSettings(context.Background(), second, second.UpdatedAt)
	if err != nil {
		t.Fatalf("SetSettings() second error = %v", err)
	}
	if saved.Enabled || saved.ProviderMode != audio.ProviderModeDisabled || saved.QueueCapacity != 50 {
		t.Fatalf("saved after update = %+v, want disabled/disabled/50", saved)
	}

	got, found, err := repo.GetSettings(context.Background())
	if err != nil || !found {
		t.Fatalf("GetSettings() after update = %+v, %v, %v", got, found, err)
	}
	if got.VoiceID != "" {
		t.Errorf("VoiceID = %q, want empty after update", got.VoiceID)
	}
}

func TestAudioSettingsInvalidProviderModeCheckConstraintRejects(t *testing.T) {
	db := newTestDB(t)
	now := time.Now().UTC()
	_, err := db.DB.ExecContext(context.Background(), `
		INSERT INTO audio_settings (id, provider_mode, created_at, updated_at)
		VALUES ('singleton', 'not-a-real-mode', ?, ?)
	`, now.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano))
	if err == nil {
		t.Fatal("expected the provider_mode CHECK constraint to reject an unknown value, got nil error")
	}
}

// TestAudioSettingsTableHasNoRuntimeOrContentColumns proves the Stage 17A
// persistence boundary directly against the real schema (docs/audio-tts.md
// §12/governing task §65): only safe configuration is ever a column here -
// never queued text, chat/donation message content, generated audio bytes,
// or cooldown timestamps.
func TestAudioSettingsTableHasNoRuntimeOrContentColumns(t *testing.T) {
	db := newTestDB(t)
	rows, err := db.DB.QueryContext(context.Background(), `PRAGMA table_info(audio_settings)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info() error = %v", err)
	}
	defer rows.Close()

	forbidden := []string{
		"queue", "utterance", "spoken_text", "message", "message_text",
		"audio_bytes", "audio_data", "wav", "mp3", "current_item",
		"cooldown_at", "cooldown_expires", "last_spoken", "playback_history",
		"donor", "chat_text", "event_text",
	}

	var found []string
	for rows.Next() {
		var (
			cid           int
			name, colType string
			notNull, pk   int
			defaultValue  any
		)
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultValue, &pk); err != nil {
			t.Fatalf("scan table_info row: %v", err)
		}
		found = append(found, name)
		for _, f := range forbidden {
			if name == f {
				t.Errorf("audio_settings has a forbidden runtime/content column %q", name)
			}
		}
	}
	if len(found) == 0 {
		t.Fatal("PRAGMA table_info(audio_settings) returned no columns - table missing?")
	}
}
