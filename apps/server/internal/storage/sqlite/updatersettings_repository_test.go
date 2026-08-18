package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/updatersettings"
)

func TestUpdateSettingsGetReturnsNotFoundWhenAbsent(t *testing.T) {
	db := newTestDB(t)
	repo := NewUpdateSettingsRepository(db.DB)

	_, found, err := repo.GetPreferences(context.Background())
	if err != nil {
		t.Fatalf("GetPreferences() error = %v", err)
	}
	if found {
		t.Error("GetPreferences() found = true, want false before anything is set")
	}
}

func TestUpdateSettingsSetThenGetRoundTrips(t *testing.T) {
	db := newTestDB(t)
	repo := NewUpdateSettingsRepository(db.DB)
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)

	saved, err := repo.SetPreferences(context.Background(), updatersettings.Preferences{AutoCheck: false}, now)
	if err != nil {
		t.Fatalf("SetPreferences() error = %v", err)
	}
	if saved.AutoCheck != false {
		t.Errorf("SetPreferences() AutoCheck = %v, want false", saved.AutoCheck)
	}

	got, found, err := repo.GetPreferences(context.Background())
	if err != nil {
		t.Fatalf("GetPreferences() error = %v", err)
	}
	if !found {
		t.Fatal("GetPreferences() found = false, want true after Set")
	}
	if got.AutoCheck != false {
		t.Errorf("GetPreferences() AutoCheck = %v, want false", got.AutoCheck)
	}
}

func TestUpdateSettingsSetReplacesTheSingletonRowInPlace(t *testing.T) {
	db := newTestDB(t)
	repo := NewUpdateSettingsRepository(db.DB)
	ctx := context.Background()

	if _, err := repo.SetPreferences(ctx, updatersettings.Preferences{AutoCheck: true}, time.Now().UTC()); err != nil {
		t.Fatalf("first SetPreferences() error = %v", err)
	}
	if _, err := repo.SetPreferences(ctx, updatersettings.Preferences{AutoCheck: false}, time.Now().UTC()); err != nil {
		t.Fatalf("second SetPreferences() error = %v", err)
	}

	var count int
	if err := db.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM update_preferences`).Scan(&count); err != nil {
		t.Fatalf("count query error = %v", err)
	}
	if count != 1 {
		t.Fatalf("row count = %d, want exactly 1 (singleton)", count)
	}

	got, found, err := repo.GetPreferences(ctx)
	if err != nil {
		t.Fatalf("GetPreferences() error = %v", err)
	}
	if !found || got.AutoCheck != false {
		t.Fatalf("GetPreferences() = (%+v, %v), want the second write's values", got, found)
	}
}
