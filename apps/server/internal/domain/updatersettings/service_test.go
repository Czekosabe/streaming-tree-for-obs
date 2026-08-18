package updatersettings

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeRepository struct {
	stored *Preferences
	getErr error
	setErr error
}

func (f *fakeRepository) GetPreferences(ctx context.Context) (Preferences, bool, error) {
	if f.getErr != nil {
		return Preferences{}, false, f.getErr
	}
	if f.stored == nil {
		return Preferences{}, false, nil
	}
	return *f.stored, true, nil
}

func (f *fakeRepository) SetPreferences(ctx context.Context, p Preferences, now time.Time) (Preferences, error) {
	if f.setErr != nil {
		return Preferences{}, f.setErr
	}
	p.UpdatedAt = now
	if f.stored == nil {
		p.CreatedAt = now
	} else {
		p.CreatedAt = f.stored.CreatedAt
	}
	f.stored = &p
	return p, nil
}

func TestServicePreferencesReturnsDefaultWhenNoneSaved(t *testing.T) {
	svc := NewService(&fakeRepository{}, nil)

	got, err := svc.Preferences(context.Background())
	if err != nil {
		t.Fatalf("Preferences() error = %v", err)
	}
	if got != Default() {
		t.Fatalf("Preferences() = %+v, want Default() = %+v", got, Default())
	}
}

func TestServiceReplacePreferencesThenGet(t *testing.T) {
	repo := &fakeRepository{}
	fixedNow := time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)
	svc := NewService(repo, func() time.Time { return fixedNow })

	saved, err := svc.ReplacePreferences(context.Background(), Preferences{AutoCheck: false})
	if err != nil {
		t.Fatalf("ReplacePreferences() error = %v", err)
	}
	if saved.AutoCheck != false {
		t.Fatalf("ReplacePreferences() AutoCheck = %v, want false", saved.AutoCheck)
	}
	if !saved.UpdatedAt.Equal(fixedNow) {
		t.Fatalf("ReplacePreferences() UpdatedAt = %v, want %v", saved.UpdatedAt, fixedNow)
	}

	got, err := svc.Preferences(context.Background())
	if err != nil {
		t.Fatalf("Preferences() error = %v", err)
	}
	if got.AutoCheck != false {
		t.Fatalf("Preferences() AutoCheck = %v, want false", got.AutoCheck)
	}
}

func TestServiceWrapsRepositoryErrors(t *testing.T) {
	repo := &fakeRepository{getErr: errors.New("disk full")}
	svc := NewService(repo, nil)

	_, err := svc.Preferences(context.Background())
	if !errors.Is(err, ErrStorage) {
		t.Fatalf("Preferences() error = %v, want it to wrap ErrStorage", err)
	}
}

func TestDefaultIsAutoCheckOn(t *testing.T) {
	if !Default().AutoCheck {
		t.Fatal("Default().AutoCheck = false, want true (docs/updater.md §10/§27)")
	}
}
