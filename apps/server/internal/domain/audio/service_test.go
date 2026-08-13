package audio

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// fakeRepository is an in-memory Repository for Service tests - no SQLite
// involved, since these tests are about use-case logic (seeding, snapshot
// preservation, rotation), not persistence mechanics (covered by
// internal/storage/sqlite's own repository tests).
type fakeRepository struct {
	mu       sync.Mutex
	settings Settings
	found    bool
}

func (f *fakeRepository) GetSettings(ctx context.Context) (Settings, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.settings, f.found, nil
}

func (f *fakeRepository) SetSettings(ctx context.Context, s Settings, now time.Time) (Settings, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Mirrors AudioSettingsRepository.SetSettings's own contract: the
	// persisted timestamps come from the now parameter, not whatever the
	// caller happened to leave on s.UpdatedAt/CreatedAt.
	if s.CreatedAt.IsZero() {
		s.CreatedAt = now
	}
	s.UpdatedAt = now
	f.settings = s
	f.found = true
	return s, nil
}

func fakeClockAt(t time.Time) Clock {
	return func() time.Time { return t }
}

func TestServiceGetSeedsDefaultsWithSlugOnFirstCall(t *testing.T) {
	repo := &fakeRepository{}
	now := time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
	svc := NewService(Options{Repository: repo, Now: fakeClockAt(now)})

	got, err := svc.Get(context.Background())
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.PublicSlug == "" {
		t.Error("PublicSlug is empty after first Get()")
	}
	if got.Enabled {
		t.Error("Enabled = true, want the documented default of false")
	}
	if !got.CreatedAt.Equal(now) || !got.UpdatedAt.Equal(now) {
		t.Errorf("CreatedAt/UpdatedAt = %v/%v, want both %v", got.CreatedAt, got.UpdatedAt, now)
	}
}

func TestServiceGetIsStableAcrossCalls(t *testing.T) {
	repo := &fakeRepository{}
	svc := NewService(Options{Repository: repo, Now: fakeClockAt(time.Now().UTC())})

	first, err := svc.Get(context.Background())
	if err != nil {
		t.Fatalf("Get() first error = %v", err)
	}
	second, err := svc.Get(context.Background())
	if err != nil {
		t.Fatalf("Get() second error = %v", err)
	}
	if first.PublicSlug != second.PublicSlug {
		t.Errorf("PublicSlug changed between calls: %q -> %q", first.PublicSlug, second.PublicSlug)
	}
}

func TestServiceUpdateRejectsInvalidSettings(t *testing.T) {
	repo := &fakeRepository{}
	svc := NewService(Options{Repository: repo, Now: fakeClockAt(time.Now().UTC())})

	invalid := Default()
	invalid.ProviderMode = ProviderMode("bogus")

	_, err := svc.Update(context.Background(), invalid)
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("Update() error = %v, want ErrValidation", err)
	}
}

func TestServiceUpdatePreservesPublicSlugAndCreatedAt(t *testing.T) {
	repo := &fakeRepository{}
	created := time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)
	svc := NewService(Options{Repository: repo, Now: fakeClockAt(created)})

	seeded, err := svc.Get(context.Background())
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	later := created.Add(time.Hour)
	svc2 := NewService(Options{Repository: repo, Now: fakeClockAt(later)})

	input := seeded
	input.Enabled = true
	input.ProviderMode = ProviderModeSystem
	input.PublicSlug = "attacker-supplied-slug"
	input.CreatedAt = time.Time{}

	saved, err := svc2.Update(context.Background(), input)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if saved.PublicSlug != seeded.PublicSlug {
		t.Errorf("PublicSlug = %q, want unchanged %q", saved.PublicSlug, seeded.PublicSlug)
	}
	if !saved.CreatedAt.Equal(seeded.CreatedAt) {
		t.Errorf("CreatedAt = %v, want unchanged %v", saved.CreatedAt, seeded.CreatedAt)
	}
	if !saved.UpdatedAt.Equal(later) {
		t.Errorf("UpdatedAt = %v, want %v", saved.UpdatedAt, later)
	}
	if !saved.Enabled {
		t.Error("Enabled = false, want true (from the update input)")
	}
}

func TestServiceRotatePublicSlugChangesOnlySlug(t *testing.T) {
	repo := &fakeRepository{}
	svc := NewService(Options{Repository: repo, Now: fakeClockAt(time.Now().UTC())})

	before, err := svc.Get(context.Background())
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	after, err := svc.RotatePublicSlug(context.Background())
	if err != nil {
		t.Fatalf("RotatePublicSlug() error = %v", err)
	}
	if after.PublicSlug == before.PublicSlug {
		t.Error("PublicSlug unchanged after RotatePublicSlug()")
	}
	if after.Enabled != before.Enabled || after.ProviderMode != before.ProviderMode {
		t.Errorf("non-slug fields changed: before=%+v after=%+v", before, after)
	}
}
