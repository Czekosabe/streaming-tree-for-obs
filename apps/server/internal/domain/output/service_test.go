package output

import (
	"context"
	"testing"
	"time"
)

// fakeRepository is a minimal in-memory output.Repository for service tests.
type fakeRepository struct {
	rows map[string]Settings
}

func newFakeRepository(seed map[string]Settings) *fakeRepository {
	return &fakeRepository{rows: seed}
}

func (f *fakeRepository) Get(_ context.Context, platformID string) (Settings, error) {
	settings, ok := f.rows[platformID]
	if !ok {
		return Settings{}, ErrNotFound
	}
	return settings, nil
}

func (f *fakeRepository) Update(_ context.Context, platformID string, input UpdateInput, updatedAt string) (Settings, error) {
	if _, ok := f.rows[platformID]; !ok {
		return Settings{}, ErrNotFound
	}
	parsed, _ := time.Parse(time.RFC3339Nano, updatedAt)
	settings := Settings{ServerURL: input.ServerURL, AutoRestart: input.AutoRestart, UpdatedAt: parsed}
	f.rows[platformID] = settings
	return settings, nil
}

func TestServiceUpdateValidatesBeforeTouchingTheRepository(t *testing.T) {
	repo := newFakeRepository(map[string]Settings{"pf_1": {}})
	svc := NewService(repo)

	_, err := svc.Update(context.Background(), "pf_1", UpdateInput{
		ServerURL: "not a valid url ??? \x07",
	})
	if err == nil {
		t.Fatal("an invalid server URL was accepted")
	}
}

func TestServiceUpdateStoresTheNormalizedValue(t *testing.T) {
	repo := newFakeRepository(map[string]Settings{"pf_1": {}})
	svc := NewService(repo)

	updated, err := svc.Update(context.Background(), "pf_1", UpdateInput{
		ServerURL:   "  rtmp://example.invalid/app  ",
		AutoRestart: true,
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.ServerURL != "rtmp://example.invalid/app" {
		t.Errorf("ServerURL = %q, want the trimmed value", updated.ServerURL)
	}
}

func TestServiceUpdateOnAnUnknownPlatformReturnsNotFound(t *testing.T) {
	repo := newFakeRepository(map[string]Settings{})
	svc := NewService(repo)

	_, err := svc.Update(context.Background(), "pf_missing", UpdateInput{ServerURL: "rtmp://example.invalid/app"})
	if err != ErrNotFound {
		t.Errorf("Update() error = %v, want ErrNotFound", err)
	}
}

func TestServiceGetReturnsTheStoredSettings(t *testing.T) {
	repo := newFakeRepository(map[string]Settings{
		"pf_1": {ServerURL: "rtmp://example.invalid/app", AutoRestart: true},
	})
	svc := NewService(repo)

	settings, err := svc.Get(context.Background(), "pf_1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if settings.ServerURL != "rtmp://example.invalid/app" {
		t.Errorf("ServerURL = %q", settings.ServerURL)
	}
}
