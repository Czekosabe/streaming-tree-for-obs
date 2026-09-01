package streamsession

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/runtime/branch"
	"github.com/streaming-tree/server/internal/runtime/mediamtx"
)

// --- fakes -----------------------------------------------------------

type fakeRepo struct {
	mu           sync.Mutex
	sessions     map[string]Session
	destinations map[string]Destination
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{sessions: map[string]Session{}, destinations: map[string]Destination{}}
}

func (f *fakeRepo) CreateSession(_ context.Context, s Session) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessions[s.ID] = s
	return nil
}

func (f *fakeRepo) UpdateSession(_ context.Context, s Session) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.sessions[s.ID]; !ok {
		return ErrNotFound
	}
	f.sessions[s.ID] = s
	return nil
}

func (f *fakeRepo) OpenSession(_ context.Context) (Session, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.sessions {
		if s.Open() {
			return s, true, nil
		}
	}
	return Session{}, false, nil
}

func (f *fakeRepo) GetSession(_ context.Context, id string) (Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.sessions[id]
	if !ok {
		return Session{}, ErrNotFound
	}
	s.Destinations = f.destinationsForLocked(id)
	return s, nil
}

func (f *fakeRepo) ListSessions(_ context.Context, limit int) ([]Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Session, 0, len(f.sessions))
	for _, s := range f.sessions {
		s.Destinations = f.destinationsForLocked(s.ID)
		out = append(out, s)
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (f *fakeRepo) destinationsForLocked(sessionID string) []Destination {
	var out []Destination
	for _, d := range f.destinations {
		if d.SessionID == sessionID {
			out = append(out, d)
		}
	}
	return out
}

func (f *fakeRepo) CreateDestination(_ context.Context, d Destination) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.destinations[d.ID] = d
	return nil
}

func (f *fakeRepo) UpdateDestination(_ context.Context, d Destination) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.destinations[d.ID]; !ok {
		return ErrNotFound
	}
	f.destinations[d.ID] = d
	return nil
}

func (f *fakeRepo) OpenDestinations(_ context.Context, sessionID string) ([]Destination, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Destination
	for _, d := range f.destinations {
		if d.SessionID == sessionID && d.Open() {
			out = append(out, d)
		}
	}
	return out, nil
}

func (f *fakeRepo) PruneSessionsBefore(_ context.Context, cutoff time.Time) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for id, s := range f.sessions {
		if s.EndedAt != nil && s.EndedAt.Before(cutoff) {
			delete(f.sessions, id)
			for did, d := range f.destinations {
				if d.SessionID == id {
					delete(f.destinations, did)
				}
			}
			n++
		}
	}
	return n, nil
}

func (f *fakeRepo) DeleteAllSessions(_ context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessions = map[string]Session{}
	f.destinations = map[string]Destination{}
	return nil
}

type fakeIngest struct{ state mediamtx.IngestState }

func (f *fakeIngest) Snapshot() mediamtx.Snapshot {
	return mediamtx.Snapshot{Ingest: mediamtx.IngestSnapshot{State: f.state}}
}

type fakeBranches struct{ snapshots []branch.Snapshot }

func (f *fakeBranches) Snapshot(context.Context) ([]branch.Snapshot, error) {
	return f.snapshots, nil
}

type fakePlatforms struct{ byID map[string]platform.Platform }

func (f *fakePlatforms) Get(_ context.Context, id string) (platform.Platform, error) {
	p, ok := f.byID[id]
	if !ok {
		return platform.Platform{}, platform.ErrNotFound
	}
	return p, nil
}

func testManager(t *testing.T, repo Repository, ingest *fakeIngest, branches *fakeBranches, platforms *fakePlatforms, clock *fakeClock) *Manager {
	t.Helper()
	return NewManager(repo, branches, ingest, platforms, nil,
		WithClock(clock.Now), WithGraceWindow(60*time.Second), WithPollInterval(time.Hour))
}

type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time          { return c.t }
func (c *fakeClock) Advance(d time.Duration) { c.t = c.t.Add(d) }

const testPlatformID = "pf_test"

func testPlatforms() *fakePlatforms {
	return &fakePlatforms{byID: map[string]platform.Platform{
		testPlatformID: {ID: testPlatformID, ProviderID: platform.ProviderTwitch, DisplayName: "Test Destination"},
	}}
}

// --- tests -------------------------------------------------------------

func TestTickOpensASessionWhenIngestStartsReceiving(t *testing.T) {
	repo := newFakeRepo()
	ingest := &fakeIngest{state: mediamtx.IngestReceiving}
	branches := &fakeBranches{}
	clock := &fakeClock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	m := testManager(t, repo, ingest, branches, testPlatforms(), clock)

	if err := m.tick(context.Background()); err != nil {
		t.Fatalf("tick() error = %v", err)
	}

	open, found, err := repo.OpenSession(context.Background())
	if err != nil || !found {
		t.Fatalf("OpenSession() = %+v, %v, %v, want a session to exist", open, found, err)
	}
	if !open.StartedAt.Equal(clock.t) {
		t.Errorf("StartedAt = %v, want %v", open.StartedAt, clock.t)
	}
}

func TestTickTracksDestinationParticipationWhileLive(t *testing.T) {
	repo := newFakeRepo()
	ingest := &fakeIngest{state: mediamtx.IngestReceiving}
	branches := &fakeBranches{}
	clock := &fakeClock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	m := testManager(t, repo, ingest, branches, testPlatforms(), clock)
	ctx := context.Background()

	if err := m.tick(ctx); err != nil {
		t.Fatalf("tick() error = %v", err)
	}

	// The destination goes live.
	clock.Advance(5 * time.Second)
	branches.snapshots = []branch.Snapshot{{PlatformID: testPlatformID, State: branch.StateLive}}
	if err := m.tick(ctx); err != nil {
		t.Fatalf("tick() error = %v", err)
	}
	open, _, _ := repo.OpenSession(ctx)
	dests, err := repo.OpenDestinations(ctx, open.ID)
	if err != nil || len(dests) != 1 {
		t.Fatalf("OpenDestinations() = %+v, %v, want exactly 1 open destination", dests, err)
	}
	if dests[0].DisplayName != "Test Destination" || dests[0].ProviderID != string(platform.ProviderTwitch) {
		t.Errorf("destination snapshot fields = %+v, want the resolved platform's own provider/name", dests[0])
	}

	// The destination stops being live cleanly (not an error).
	clock.Advance(5 * time.Second)
	branches.snapshots = []branch.Snapshot{{PlatformID: testPlatformID, State: branch.StateIdle}}
	if err := m.tick(ctx); err != nil {
		t.Fatalf("tick() error = %v", err)
	}
	got, err := repo.GetSession(ctx, open.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if len(got.Destinations) != 1 || got.Destinations[0].Outcome != OutcomeCompleted {
		t.Fatalf("destinations = %+v, want exactly 1 with OutcomeCompleted", got.Destinations)
	}
}

func TestDestinationStoppedViaErrorGetsOutcomeError(t *testing.T) {
	repo := newFakeRepo()
	ingest := &fakeIngest{state: mediamtx.IngestReceiving}
	branches := &fakeBranches{snapshots: []branch.Snapshot{{PlatformID: testPlatformID, State: branch.StateLive}}}
	clock := &fakeClock{t: time.Now()}
	m := testManager(t, repo, ingest, branches, testPlatforms(), clock)
	ctx := context.Background()
	if err := m.tick(ctx); err != nil {
		t.Fatalf("tick() error = %v", err)
	}

	branches.snapshots = []branch.Snapshot{{PlatformID: testPlatformID, State: branch.StateError}}
	if err := m.tick(ctx); err != nil {
		t.Fatalf("tick() error = %v", err)
	}

	open, _, _ := repo.OpenSession(ctx)
	got, err := repo.GetSession(ctx, open.ID)
	if err != nil || len(got.Destinations) != 1 || got.Destinations[0].Outcome != OutcomeError {
		t.Fatalf("destinations = %+v, %v, want exactly 1 with OutcomeError", got.Destinations, err)
	}
}

func TestBriefDisconnectShorterThanGraceWindowDoesNotFragmentASession(t *testing.T) {
	repo := newFakeRepo()
	ingest := &fakeIngest{state: mediamtx.IngestReceiving}
	branches := &fakeBranches{}
	clock := &fakeClock{t: time.Now()}
	m := testManager(t, repo, ingest, branches, testPlatforms(), clock)
	ctx := context.Background()
	if err := m.tick(ctx); err != nil {
		t.Fatalf("tick() error = %v", err)
	}
	first, _, _ := repo.OpenSession(ctx)

	// A blip well within the 60s grace window.
	ingest.state = mediamtx.IngestWaiting
	clock.Advance(10 * time.Second)
	if err := m.tick(ctx); err != nil {
		t.Fatalf("tick() error = %v", err)
	}
	ingest.state = mediamtx.IngestReceiving
	clock.Advance(5 * time.Second)
	if err := m.tick(ctx); err != nil {
		t.Fatalf("tick() error = %v", err)
	}

	second, found, err := repo.OpenSession(ctx)
	if err != nil || !found {
		t.Fatalf("OpenSession() = %+v, %v, %v, want the session to still be open", second, found, err)
	}
	if second.ID != first.ID {
		t.Errorf("session id changed across a sub-grace-window blip: %q -> %q, want the same session", first.ID, second.ID)
	}
}

func TestDisconnectLongerThanGraceWindowClosesAndReopensAsTwoSessions(t *testing.T) {
	repo := newFakeRepo()
	ingest := &fakeIngest{state: mediamtx.IngestReceiving}
	branches := &fakeBranches{}
	clock := &fakeClock{t: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	m := testManager(t, repo, ingest, branches, testPlatforms(), clock)
	ctx := context.Background()
	if err := m.tick(ctx); err != nil {
		t.Fatalf("tick() error = %v", err)
	}
	first, _, _ := repo.OpenSession(ctx)
	lastReceivingAt := clock.t

	ingest.state = mediamtx.IngestWaiting
	clock.Advance(70 * time.Second) // past the 60s grace window
	if err := m.tick(ctx); err != nil {
		t.Fatalf("tick() error = %v", err)
	}

	closed, err := repo.GetSession(ctx, first.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if closed.Open() {
		t.Fatal("session is still open after the grace window elapsed")
	}
	if closed.EndReason != EndReasonIngestStopped {
		t.Errorf("EndReason = %q, want %q", closed.EndReason, EndReasonIngestStopped)
	}
	if closed.EndedAt == nil || !closed.EndedAt.Equal(lastReceivingAt) {
		t.Errorf("EndedAt = %v, want the last real receiving moment %v (not the tick that noticed the grace window elapsed)", closed.EndedAt, lastReceivingAt)
	}

	ingest.state = mediamtx.IngestReceiving
	clock.Advance(time.Second)
	if err := m.tick(ctx); err != nil {
		t.Fatalf("tick() error = %v", err)
	}
	second, found, err := repo.OpenSession(ctx)
	if err != nil || !found {
		t.Fatalf("OpenSession() = %+v, %v, %v, want a new session to have opened", second, found, err)
	}
	if second.ID == first.ID {
		t.Error("a genuinely new session after the grace window reused the previous session's id")
	}
}

func TestSessionEndClosesAnyStillOpenDestinationAsSessionEnded(t *testing.T) {
	repo := newFakeRepo()
	ingest := &fakeIngest{state: mediamtx.IngestReceiving}
	branches := &fakeBranches{snapshots: []branch.Snapshot{{PlatformID: testPlatformID, State: branch.StateLive}}}
	clock := &fakeClock{t: time.Now()}
	m := testManager(t, repo, ingest, branches, testPlatforms(), clock)
	ctx := context.Background()
	if err := m.tick(ctx); err != nil {
		t.Fatalf("tick() error = %v", err)
	}
	open, _, _ := repo.OpenSession(ctx)

	ingest.state = mediamtx.IngestWaiting
	clock.Advance(70 * time.Second)
	if err := m.tick(ctx); err != nil {
		t.Fatalf("tick() error = %v", err)
	}

	closed, err := repo.GetSession(ctx, open.ID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if len(closed.Destinations) != 1 || closed.Destinations[0].Outcome != OutcomeSessionEnded {
		t.Fatalf("destinations = %+v, want exactly 1 with OutcomeSessionEnded", closed.Destinations)
	}
	if closed.Destinations[0].EndedAt == nil || !closed.Destinations[0].EndedAt.Equal(*closed.EndedAt) {
		t.Errorf("destination EndedAt = %v, want the session's own EndedAt %v", closed.Destinations[0].EndedAt, closed.EndedAt)
	}
}

func TestStartRecoversAnOrphanedOpenSessionUsingItsOwnLastSeenAt(t *testing.T) {
	repo := newFakeRepo()
	staleHeartbeat := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	orphanID := "sess_orphan"
	if err := repo.CreateSession(context.Background(), Session{
		ID: orphanID, StartedAt: staleHeartbeat.Add(-time.Hour), LastSeenAt: staleHeartbeat,
		CreatedAt: staleHeartbeat.Add(-time.Hour), UpdatedAt: staleHeartbeat,
	}); err != nil {
		t.Fatalf("seed orphaned session: %v", err)
	}
	orphanDestID := "sessdest_orphan"
	if err := repo.CreateDestination(context.Background(), Destination{
		ID: orphanDestID, SessionID: orphanID, ProviderID: string(platform.ProviderTwitch), DisplayName: "Orphaned",
		StartedAt: staleHeartbeat.Add(-time.Minute), CreatedAt: staleHeartbeat, UpdatedAt: staleHeartbeat,
	}); err != nil {
		t.Fatalf("seed orphaned destination: %v", err)
	}

	// "Now" is much later than the stale heartbeat - recovery must use
	// the heartbeat, never this current time.
	clock := &fakeClock{t: staleHeartbeat.Add(24 * time.Hour)}
	ingest := &fakeIngest{state: mediamtx.IngestUnavailable}
	branches := &fakeBranches{}
	m := testManager(t, repo, ingest, branches, testPlatforms(), clock)

	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer m.Shutdown(context.Background())

	recovered, err := repo.GetSession(context.Background(), orphanID)
	if err != nil {
		t.Fatalf("GetSession() error = %v", err)
	}
	if recovered.Open() {
		t.Fatal("the orphaned session is still open after Start()")
	}
	if recovered.EndReason != EndReasonUncleanShutdown {
		t.Errorf("EndReason = %q, want %q", recovered.EndReason, EndReasonUncleanShutdown)
	}
	if recovered.EndedAt == nil || !recovered.EndedAt.Equal(staleHeartbeat) {
		t.Errorf("EndedAt = %v, want the session's own last heartbeat %v, never the current time", recovered.EndedAt, staleHeartbeat)
	}
	if len(recovered.Destinations) != 1 || recovered.Destinations[0].Outcome != OutcomeSessionEnded {
		t.Fatalf("destinations = %+v, want exactly 1 with OutcomeSessionEnded", recovered.Destinations)
	}
	if !recovered.Destinations[0].EndedAt.Equal(staleHeartbeat) {
		t.Errorf("destination EndedAt = %v, want %v", recovered.Destinations[0].EndedAt, staleHeartbeat)
	}
}

func TestStartWithNoOrphanedSessionLeavesHistoryEmpty(t *testing.T) {
	repo := newFakeRepo()
	clock := &fakeClock{t: time.Now()}
	ingest := &fakeIngest{state: mediamtx.IngestUnavailable}
	branches := &fakeBranches{}
	m := testManager(t, repo, ingest, branches, testPlatforms(), clock)

	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer m.Shutdown(context.Background())

	sessions, err := repo.ListSessions(context.Background(), 10)
	if err != nil || len(sessions) != 0 {
		t.Fatalf("ListSessions() = %+v, %v, want an empty history", sessions, err)
	}
}

func TestShutdownStopsThePollLoopAndReturnsPromptly(t *testing.T) {
	repo := newFakeRepo()
	clock := &fakeClock{t: time.Now()}
	ingest := &fakeIngest{state: mediamtx.IngestReceiving}
	branches := &fakeBranches{}
	m := NewManager(repo, branches, ingest, testPlatforms(), nil,
		WithClock(clock.Now), WithPollInterval(10*time.Millisecond))

	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, found, _ := repo.OpenSession(context.Background()); found {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no session was created by the real poll loop within 2s")
		}
		time.Sleep(5 * time.Millisecond)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := m.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}
