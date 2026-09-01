package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/domain/streamsession"
)

func TestStreamSessionListIsEmptyOnFreshDatabase(t *testing.T) {
	db := newTestDB(t)
	repo := NewStreamSessionRepository(db.DB)

	list, err := repo.ListSessions(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("ListSessions() = %d sessions, want 0 - a fresh database must start with zero real sessions, no seed data", len(list))
	}

	_, found, err := repo.OpenSession(context.Background())
	if err != nil {
		t.Fatalf("OpenSession() error = %v", err)
	}
	if found {
		t.Fatal("OpenSession() found a session on a fresh database")
	}
}

func newTestSession(id string) streamsession.Session {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	return streamsession.Session{ID: id, StartedAt: now, LastSeenAt: now, CreatedAt: now, UpdatedAt: now}
}

func TestStreamSessionCreateThenGetRoundTrips(t *testing.T) {
	db := newTestDB(t)
	repo := NewStreamSessionRepository(db.DB)
	ctx := context.Background()

	s := newTestSession("sess_1")
	if err := repo.CreateSession(ctx, s); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	got, err := repo.GetSession(ctx, "sess_1")
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if !got.Open() {
		t.Error("a freshly created session must be open")
	}
	if !got.StartedAt.Equal(s.StartedAt) {
		t.Errorf("StartedAt = %v, want %v", got.StartedAt, s.StartedAt)
	}

	open, found, err := repo.OpenSession(ctx)
	if err != nil || !found || open.ID != "sess_1" {
		t.Fatalf("OpenSession() = %+v, %v, %v, want sess_1", open, found, err)
	}
}

func TestStreamSessionUpdateClosesIt(t *testing.T) {
	db := newTestDB(t)
	repo := NewStreamSessionRepository(db.DB)
	ctx := context.Background()

	s := newTestSession("sess_1")
	if err := repo.CreateSession(ctx, s); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	endedAt := s.StartedAt.Add(time.Hour)
	s.EndedAt = &endedAt
	s.EndReason = streamsession.EndReasonIngestStopped
	s.UpdatedAt = endedAt
	if err := repo.UpdateSession(ctx, s); err != nil {
		t.Fatalf("UpdateSession() error = %v", err)
	}

	got, err := repo.GetSession(ctx, "sess_1")
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if got.Open() {
		t.Error("the session is still open after closing it")
	}
	if got.EndReason != streamsession.EndReasonIngestStopped {
		t.Errorf("EndReason = %q, want %q", got.EndReason, streamsession.EndReasonIngestStopped)
	}
	if _, found, err := repo.OpenSession(ctx); err != nil || found {
		t.Fatalf("OpenSession() found = %v, err = %v, want no open session", found, err)
	}
}

func TestStreamSessionUpdateUnknownIDReturnsNotFound(t *testing.T) {
	db := newTestDB(t)
	repo := NewStreamSessionRepository(db.DB)

	err := repo.UpdateSession(context.Background(), newTestSession("sess_does_not_exist"))
	if err != streamsession.ErrNotFound {
		t.Fatalf("UpdateSession() error = %v, want ErrNotFound", err)
	}
}

func TestStreamSessionDestinationRoundTripsWithPlatformSnapshot(t *testing.T) {
	db := newTestDB(t)
	repo := NewStreamSessionRepository(db.DB)
	platformRepo := NewPlatformRepository(db.DB)
	ctx := context.Background()

	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	if err := platformRepo.Create(ctx, platform.Platform{
		ID: "pf_1", ProviderID: platform.ProviderTwitch, DisplayName: "Original Name",
		CreatedAt: now, UpdatedAt: now, Metadata: platform.Metadata{Tags: []string{}, UpdatedAt: now},
	}); err != nil {
		t.Fatalf("seed platform: %v", err)
	}

	s := newTestSession("sess_1")
	if err := repo.CreateSession(ctx, s); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	platformID := "pf_1"
	d := streamsession.Destination{
		ID: "sessdest_1", SessionID: "sess_1", PlatformID: &platformID,
		ProviderID: string(platform.ProviderTwitch), DisplayName: "Original Name",
		StartedAt: now, CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.CreateDestination(ctx, d); err != nil {
		t.Fatalf("CreateDestination() error = %v", err)
	}

	got, err := repo.GetSession(ctx, "sess_1")
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if len(got.Destinations) != 1 || got.Destinations[0].DisplayName != "Original Name" {
		t.Fatalf("Destinations = %+v, want exactly 1 with DisplayName Original Name", got.Destinations)
	}

	// Renaming the platform must NOT rewrite the already-recorded
	// snapshot (docs/stream-session-history.md §3).
	if err := platformRepo.Update(ctx, "pf_1", platform.UpdateInput{DisplayName: "Renamed"}, platform.FormatTimestamp(now.Add(time.Minute))); err != nil {
		t.Fatalf("rename platform: %v", err)
	}
	afterRename, err := repo.GetSession(ctx, "sess_1")
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if afterRename.Destinations[0].DisplayName != "Original Name" {
		t.Errorf("DisplayName = %q after the platform was renamed, want the unchanged snapshot %q", afterRename.Destinations[0].DisplayName, "Original Name")
	}

	// Deleting the platform must SET NULL on platform_id, never delete
	// the session/destination history rows.
	if err := platformRepo.Delete(ctx, "pf_1"); err != nil {
		t.Fatalf("delete platform: %v", err)
	}
	afterDelete, err := repo.GetSession(ctx, "sess_1")
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if len(afterDelete.Destinations) != 1 {
		t.Fatalf("Destinations = %+v after the platform was deleted, want the history row to survive", afterDelete.Destinations)
	}
	if afterDelete.Destinations[0].PlatformID != nil {
		t.Errorf("PlatformID = %v after the platform was deleted, want nil (SET NULL, never CASCADE)", *afterDelete.Destinations[0].PlatformID)
	}
	if afterDelete.Destinations[0].DisplayName != "Original Name" {
		t.Errorf("DisplayName = %q after platform deletion, want the snapshot to survive unchanged", afterDelete.Destinations[0].DisplayName)
	}
}

func TestStreamSessionPruneNeverRemovesAnOpenSession(t *testing.T) {
	db := newTestDB(t)
	repo := NewStreamSessionRepository(db.DB)
	ctx := context.Background()

	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	open := streamsession.Session{ID: "sess_open", StartedAt: old, LastSeenAt: old, CreatedAt: old, UpdatedAt: old}
	if err := repo.CreateSession(ctx, open); err != nil {
		t.Fatalf("CreateSession(open) error = %v", err)
	}

	closedEndedAt := old.Add(time.Hour)
	closed := streamsession.Session{
		ID: "sess_closed", StartedAt: old, LastSeenAt: old, EndedAt: &closedEndedAt,
		EndReason: streamsession.EndReasonIngestStopped, CreatedAt: old, UpdatedAt: old,
	}
	if err := repo.CreateSession(ctx, streamsession.Session{ID: closed.ID, StartedAt: old, LastSeenAt: old, CreatedAt: old, UpdatedAt: old}); err != nil {
		t.Fatalf("CreateSession(closed) error = %v", err)
	}
	if err := repo.UpdateSession(ctx, closed); err != nil {
		t.Fatalf("UpdateSession(closed) error = %v", err)
	}

	n, err := repo.PruneSessionsBefore(ctx, time.Now())
	if err != nil {
		t.Fatalf("PruneSessionsBefore() error = %v", err)
	}
	if n != 1 {
		t.Fatalf("PruneSessionsBefore() removed %d sessions, want exactly 1 (the closed one)", n)
	}

	if _, err := repo.GetSession(ctx, "sess_closed"); err != streamsession.ErrNotFound {
		t.Errorf("GetSession(sess_closed) error = %v, want ErrNotFound", err)
	}
	stillOpen, err := repo.GetSession(ctx, "sess_open")
	if err != nil {
		t.Fatalf("GetSession(sess_open) error = %v, the open session must never be pruned regardless of age", err)
	}
	if !stillOpen.Open() {
		t.Error("sess_open is no longer open")
	}
}

func TestStreamSessionDeleteAllSessionsClearsHistory(t *testing.T) {
	db := newTestDB(t)
	repo := NewStreamSessionRepository(db.DB)
	ctx := context.Background()

	if err := repo.CreateSession(ctx, newTestSession("sess_1")); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if err := repo.CreateDestination(ctx, streamsession.Destination{
		ID: "sessdest_1", SessionID: "sess_1", ProviderID: string(platform.ProviderTwitch), DisplayName: "X",
		StartedAt: time.Now(), CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("CreateDestination() error = %v", err)
	}

	if err := repo.DeleteAllSessions(ctx); err != nil {
		t.Fatalf("DeleteAllSessions() error = %v", err)
	}

	list, err := repo.ListSessions(ctx, 10)
	if err != nil || len(list) != 0 {
		t.Fatalf("ListSessions() = %+v, %v, want empty after DeleteAllSessions", list, err)
	}
}
