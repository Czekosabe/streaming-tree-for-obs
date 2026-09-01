package streamsession_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/domain/streamsession"
	"github.com/streaming-tree/server/internal/runtime/branch"
	"github.com/streaming-tree/server/internal/runtime/mediamtx"
	"github.com/streaming-tree/server/internal/storage/sqlite"
)

// Stage 24D: a real end-to-end integration test - a genuinely migrated
// SQLite database and the real production Repository
// (internal/storage/sqlite.StreamSessionRepository), driving the real
// Manager through a full session lifecycle. manager_test.go already
// exhaustively covers the state machine itself against an in-memory
// fake repository, and streamsession_repository_test.go already covers
// each repository method in isolation against real SQLite - this test
// instead proves the two work correctly TOGETHER: the Manager's own
// SQL usage patterns (the exact sequence of Create/Update/Open calls a
// real tick sequence produces) actually land correctly, the same gap
// Stage 23's own security_integration_test.go closed for the backup
// domain. Real MediaMTX/OBS/branch processes are not available in a
// hermetic test, so ingest/branch state is still supplied through fake
// Snapshotters - docs/stream-session-history.md §11 24D explicitly
// allows this as the fallback when driving a real MediaMTX transport
// is impractical.

type fixedIngest struct{ state mediamtx.IngestState }

func (f *fixedIngest) Snapshot() mediamtx.Snapshot {
	return mediamtx.Snapshot{Ingest: mediamtx.IngestSnapshot{State: f.state}}
}

type fixedBranches struct{ snapshots []branch.Snapshot }

func (f *fixedBranches) Snapshot(context.Context) ([]branch.Snapshot, error) {
	return f.snapshots, nil
}

type fixedClock struct{ t time.Time }

func (c *fixedClock) Now() time.Time          { return c.t }
func (c *fixedClock) Advance(d time.Duration) { c.t = c.t.Add(d) }

func newRealRepo(t *testing.T) (*sqlite.StreamSessionRepository, *sqlite.PlatformRepository) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	db, err := sqlite.Open(ctx, filepath.Join(dir, "streaming-tree.db"))
	if err != nil {
		t.Fatalf("sqlite.Open() error = %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := sqlite.Migrate(ctx, db.DB); err != nil {
		t.Fatalf("sqlite.Migrate() error = %v", err)
	}
	return sqlite.NewStreamSessionRepository(db.DB), sqlite.NewPlatformRepository(db.DB)
}

func TestManagerAgainstARealDatabaseDrivesAFullSessionLifecycle(t *testing.T) {
	repo, platformRepo := newRealRepo(t)
	ctx := context.Background()
	now := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)

	// sqlite.Migrate seeds demo destinations - removed so this test's
	// own platform is the only one in play, the same fixture-hygiene
	// step security_integration_test.go's own newInstallation helper
	// already established for Stage 23.
	seeded, err := platformRepo.List(ctx)
	if err != nil {
		t.Fatalf("list seeded platforms: %v", err)
	}
	for _, p := range seeded {
		if err := platformRepo.Delete(ctx, p.ID); err != nil {
			t.Fatalf("remove seeded demo platform %q: %v", p.ID, err)
		}
	}

	const platformID = "pf_real_test"
	if err := platformRepo.Create(ctx, platform.Platform{
		ID: platformID, ProviderID: platform.ProviderTwitch, DisplayName: "Real Test Destination",
		Enabled: true, CreatedAt: now, UpdatedAt: now, Metadata: platform.Metadata{Tags: []string{}, UpdatedAt: now},
	}); err != nil {
		t.Fatalf("seed platform: %v", err)
	}

	ingest := &fixedIngest{state: mediamtx.IngestReceiving}
	branches := &fixedBranches{}
	clock := &fixedClock{t: now}

	// A short real poll interval so this test observes the loop's own
	// timer-driven ticks directly, rather than calling an unexported
	// method - this exercises the exact same public lifecycle
	// production code uses (Start/Shutdown), against the real
	// database, end to end.
	manager := streamsession.NewManager(repo, branches, ingest, platformRepo, nil,
		streamsession.WithClock(clock.Now), streamsession.WithGraceWindow(60*time.Second), streamsession.WithPollInterval(5*time.Millisecond))
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	waitForOpenSession(t, repo)

	open, found, err := repo.OpenSession(ctx)
	if err != nil || !found {
		t.Fatalf("OpenSession() = %+v, %v, %v, want an open session", open, found, err)
	}
	sessionID := open.ID

	// The destination goes live.
	branches.snapshots = []branch.Snapshot{{PlatformID: platformID, State: branch.StateLive}}
	waitForOpenDestination(t, repo, sessionID)

	openDests, err := repo.OpenDestinations(ctx, sessionID)
	if err != nil || len(openDests) != 1 {
		t.Fatalf("OpenDestinations() = %+v, %v, want exactly 1", openDests, err)
	}
	if openDests[0].DisplayName != "Real Test Destination" || openDests[0].ProviderID != string(platform.ProviderTwitch) {
		t.Errorf("destination snapshot = %+v, want the real resolved platform's own provider/name", openDests[0])
	}

	// The destination errors out.
	branches.snapshots = []branch.Snapshot{{PlatformID: platformID, State: branch.StateError}}
	waitForNoOpenDestinations(t, repo, sessionID)

	afterError, err := repo.GetSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if len(afterError.Destinations) != 1 || afterError.Destinations[0].Outcome != streamsession.OutcomeError {
		t.Fatalf("destinations = %+v, want exactly 1 with OutcomeError", afterError.Destinations)
	}

	// Simulate the process stopping mid-session (a crash, or the
	// operator quitting without stopping OBS first) - Shutdown
	// deliberately leaves the session exactly as it is, still open
	// (docs/stream-session-history.md §2).
	ingest.state = mediamtx.IngestWaiting
	if err := manager.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	stillOpenAfterShutdown, found, err := repo.OpenSession(ctx)
	if err != nil || !found || stillOpenAfterShutdown.ID != sessionID {
		t.Fatalf("OpenSession() = %+v, %v, %v, want the session still open after a plain Shutdown", stillOpenAfterShutdown, found, err)
	}

	// A fresh manager instance recovers: since the previous one was
	// shut down (not crashed), the session is still open in the real
	// database (Shutdown deliberately leaves it exactly as-is - docs/
	// stream-session-history.md §2) - the next Start recovers it using
	// its own last real heartbeat.
	manager2 := streamsession.NewManager(repo, branches, ingest, platformRepo, nil,
		streamsession.WithClock(clock.Now), streamsession.WithGraceWindow(60*time.Second), streamsession.WithPollInterval(time.Hour))
	if err := manager2.Start(ctx); err != nil {
		t.Fatalf("Start() (recovery manager) error = %v", err)
	}
	defer manager2.Shutdown(context.Background())

	recovered, err := repo.GetSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetSession() after recovery error = %v", err)
	}
	if recovered.Open() {
		t.Fatal("the session is still open after a fresh Manager's recovery pass")
	}
	if recovered.EndReason != streamsession.EndReasonUncleanShutdown {
		t.Errorf("EndReason = %q, want %q", recovered.EndReason, streamsession.EndReasonUncleanShutdown)
	}
	if len(recovered.Destinations) != 1 || recovered.Destinations[0].Outcome != streamsession.OutcomeError {
		t.Fatalf("recovered destinations = %+v, want the OutcomeError participation to still be there, untouched by recovery", recovered.Destinations)
	}

	if _, found, err := repo.OpenSession(ctx); err != nil || found {
		t.Fatalf("OpenSession() found = %v, err = %v, want no open session after recovery", found, err)
	}
}

func waitForOpenSession(t *testing.T, repo *sqlite.StreamSessionRepository) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, found, _ := repo.OpenSession(context.Background()); found {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("no open session appeared within 3s of real polling against the real database")
}

func waitForOpenDestination(t *testing.T, repo *sqlite.StreamSessionRepository, sessionID string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if dests, err := repo.OpenDestinations(context.Background(), sessionID); err == nil && len(dests) == 1 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("no open destination appeared within 3s of real polling against the real database")
}

func waitForNoOpenDestinations(t *testing.T, repo *sqlite.StreamSessionRepository, sessionID string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if dests, err := repo.OpenDestinations(context.Background(), sessionID); err == nil && len(dests) == 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the open destination never closed within 3s of real polling against the real database")
}
