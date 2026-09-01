package onboarding

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeRepository struct {
	stored *State
	getErr error
	setErr error
}

func (f *fakeRepository) GetState(ctx context.Context) (State, bool, error) {
	if f.getErr != nil {
		return State{}, false, f.getErr
	}
	if f.stored == nil {
		return State{}, false, nil
	}
	return *f.stored, true, nil
}

func (f *fakeRepository) SetStatus(ctx context.Context, status Status, schemaVersion int, now time.Time) (State, error) {
	if f.setErr != nil {
		return State{}, f.setErr
	}
	st := State{Status: status, SchemaVersion: schemaVersion, UpdatedAt: now}
	if f.stored == nil {
		st.CreatedAt = now
	} else {
		st.CreatedAt = f.stored.CreatedAt
	}
	f.stored = &st
	return st, nil
}

func TestServiceStateReturnsDefaultWhenNoneSaved(t *testing.T) {
	svc := NewService(&fakeRepository{}, nil)

	got, err := svc.State(context.Background())
	if err != nil {
		t.Fatalf("State() error = %v", err)
	}
	if got != Default() {
		t.Fatalf("State() = %+v, want Default() = %+v", got, Default())
	}
}

func TestServiceSetStatusThenGet(t *testing.T) {
	repo := &fakeRepository{}
	fixedNow := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	svc := NewService(repo, func() time.Time { return fixedNow })

	saved, err := svc.SetStatus(context.Background(), StatusCompleted)
	if err != nil {
		t.Fatalf("SetStatus() error = %v", err)
	}
	if saved.Status != StatusCompleted {
		t.Fatalf("SetStatus() Status = %v, want %v", saved.Status, StatusCompleted)
	}
	if saved.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("SetStatus() SchemaVersion = %v, want %v", saved.SchemaVersion, CurrentSchemaVersion)
	}
	if !saved.UpdatedAt.Equal(fixedNow) {
		t.Fatalf("SetStatus() UpdatedAt = %v, want %v", saved.UpdatedAt, fixedNow)
	}

	got, err := svc.State(context.Background())
	if err != nil {
		t.Fatalf("State() error = %v", err)
	}
	if got.Status != StatusCompleted {
		t.Fatalf("State() Status = %v, want %v", got.Status, StatusCompleted)
	}
}

func TestServiceSetStatusRejectsUnknownStatus(t *testing.T) {
	svc := NewService(&fakeRepository{}, nil)

	_, err := svc.SetStatus(context.Background(), Status("bogus"))
	if !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("SetStatus() error = %v, want it to wrap ErrInvalidStatus", err)
	}
}

func TestServiceWrapsRepositoryErrors(t *testing.T) {
	repo := &fakeRepository{getErr: errors.New("disk full")}
	svc := NewService(repo, nil)

	_, err := svc.State(context.Background())
	if !errors.Is(err, ErrStorage) {
		t.Fatalf("State() error = %v, want it to wrap ErrStorage", err)
	}
}

func TestDefaultIsPending(t *testing.T) {
	if Default().Status != StatusPending {
		t.Fatalf("Default().Status = %v, want %v (docs/onboarding.md §4)", Default().Status, StatusPending)
	}
	if Default().SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("Default().SchemaVersion = %v, want %v", Default().SchemaVersion, CurrentSchemaVersion)
	}
}

func TestStatusValid(t *testing.T) {
	for _, s := range []Status{StatusPending, StatusCompleted, StatusDismissed} {
		if !s.Valid() {
			t.Fatalf("Status(%q).Valid() = false, want true", s)
		}
	}
	if Status("bogus").Valid() {
		t.Fatal(`Status("bogus").Valid() = true, want false`)
	}
}
