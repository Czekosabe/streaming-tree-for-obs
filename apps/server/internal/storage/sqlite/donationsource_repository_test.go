package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/donationsource"
)

func testSource(id string) donationsource.Source {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	return donationsource.Source{
		ID: id, ProviderID: donationsource.ProviderStreamElements,
		Label: "Main channel donations", Enabled: false,
		RemoteChannelID: "5ad23dcc18fff500d78c5348",
		CreatedAt:       now, UpdatedAt: now,
	}
}

func TestDonationSourceCreateThenGetRoundTrips(t *testing.T) {
	db := newTestDB(t)
	repo := NewDonationSourceRepository(db.DB)

	src := testSource("donsrc_1")
	if err := repo.CreateSource(context.Background(), src); err != nil {
		t.Fatalf("CreateSource() error = %v", err)
	}
	got, err := repo.GetSource(context.Background(), "donsrc_1")
	if err != nil {
		t.Fatalf("GetSource() error = %v", err)
	}
	if got.Label != src.Label || got.RemoteChannelID != src.RemoteChannelID || got.Enabled != src.Enabled {
		t.Errorf("got = %+v, want %+v", got, src)
	}
	if got.ProviderID != donationsource.ProviderStreamElements {
		t.Errorf("ProviderID = %q, want streamelements", got.ProviderID)
	}
}

func TestDonationSourceGetMissingReturnsErrNotFound(t *testing.T) {
	db := newTestDB(t)
	repo := NewDonationSourceRepository(db.DB)
	if _, err := repo.GetSource(context.Background(), "donsrc_missing"); !errors.Is(err, donationsource.ErrNotFound) {
		t.Errorf("GetSource() error = %v, want ErrNotFound", err)
	}
}

func TestDonationSourceUpdateReplacesMutableFields(t *testing.T) {
	db := newTestDB(t)
	repo := NewDonationSourceRepository(db.DB)
	src := testSource("donsrc_1")
	if err := repo.CreateSource(context.Background(), src); err != nil {
		t.Fatalf("CreateSource() error = %v", err)
	}
	src.Label = "Renamed"
	src.Enabled = true
	src.RemoteChannelID = "different_channel"
	if err := repo.UpdateSource(context.Background(), src); err != nil {
		t.Fatalf("UpdateSource() error = %v", err)
	}
	got, err := repo.GetSource(context.Background(), "donsrc_1")
	if err != nil {
		t.Fatalf("GetSource() error = %v", err)
	}
	if got.Label != "Renamed" || !got.Enabled || got.RemoteChannelID != "different_channel" {
		t.Errorf("got = %+v after update", got)
	}
}

func TestDonationSourceUpdateMissingReturnsErrNotFound(t *testing.T) {
	db := newTestDB(t)
	repo := NewDonationSourceRepository(db.DB)
	if err := repo.UpdateSource(context.Background(), testSource("donsrc_missing")); !errors.Is(err, donationsource.ErrNotFound) {
		t.Errorf("UpdateSource() error = %v, want ErrNotFound", err)
	}
}

func TestDonationSourceDeleteThenGetIsNotFound(t *testing.T) {
	db := newTestDB(t)
	repo := NewDonationSourceRepository(db.DB)
	src := testSource("donsrc_1")
	if err := repo.CreateSource(context.Background(), src); err != nil {
		t.Fatalf("CreateSource() error = %v", err)
	}
	if err := repo.DeleteSource(context.Background(), "donsrc_1"); err != nil {
		t.Fatalf("DeleteSource() error = %v", err)
	}
	if _, err := repo.GetSource(context.Background(), "donsrc_1"); !errors.Is(err, donationsource.ErrNotFound) {
		t.Errorf("GetSource() after delete error = %v, want ErrNotFound", err)
	}
}

func TestDonationSourceDeleteMissingIsNotAnError(t *testing.T) {
	db := newTestDB(t)
	repo := NewDonationSourceRepository(db.DB)
	if err := repo.DeleteSource(context.Background(), "donsrc_missing"); err != nil {
		t.Errorf("DeleteSource() of a missing row error = %v, want nil", err)
	}
}

func TestDonationSourceListOrdersByCreationTime(t *testing.T) {
	db := newTestDB(t)
	repo := NewDonationSourceRepository(db.DB)

	first := testSource("donsrc_1")
	first.CreatedAt = time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC)
	first.UpdatedAt = first.CreatedAt
	second := testSource("donsrc_2")
	second.CreatedAt = time.Date(2026, 8, 12, 11, 0, 0, 0, time.UTC)
	second.UpdatedAt = second.CreatedAt

	if err := repo.CreateSource(context.Background(), second); err != nil {
		t.Fatalf("CreateSource(second) error = %v", err)
	}
	if err := repo.CreateSource(context.Background(), first); err != nil {
		t.Fatalf("CreateSource(first) error = %v", err)
	}

	list, err := repo.ListSources(context.Background())
	if err != nil {
		t.Fatalf("ListSources() error = %v", err)
	}
	if len(list) != 2 || list[0].ID != "donsrc_1" || list[1].ID != "donsrc_2" {
		t.Fatalf("ListSources() = %+v, want [donsrc_1, donsrc_2] in creation order", list)
	}
}

func TestDonationSourceProviderCheckConstraintRejectsUnknownProvider(t *testing.T) {
	db := newTestDB(t)
	repo := NewDonationSourceRepository(db.DB)
	src := testSource("donsrc_1")
	src.ProviderID = donationsource.ProviderID("streamlabs")
	if err := repo.CreateSource(context.Background(), src); err == nil {
		t.Fatal("expected the CHECK constraint to reject an unsupported provider_id")
	}
}
