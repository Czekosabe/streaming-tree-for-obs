package streamelementsengagement

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	engagement "github.com/streaming-tree/server/internal/domain/engagement"

	"github.com/streaming-tree/server/internal/domain/donationsource"
	bus "github.com/streaming-tree/server/internal/engagement"
	"github.com/streaming-tree/server/internal/provider/streamelements"
	"github.com/streaming-tree/server/internal/secrets/secretstest"
	"github.com/streaming-tree/server/internal/storage/sqlite"
)

// fakeAstroServer is a real local WebSocket server implementing enough of
// the Astro protocol to drive this package's connector through its whole
// lifecycle: welcome, subscribe (to both channel.tips and
// channel.tips.moderation, since this connector always requests both),
// subscribe error injection, resume-token handshakes, and test-driven
// pushes/abrupt disconnects of the currently active connection.
type fakeAstroServer struct {
	t   *testing.T
	url string

	subscribeError string

	mu              sync.Mutex
	active          *websocket.Conn
	connCount       int
	lastResumeToken string
	connected       chan struct{}
}

func newFakeAstroServer(t *testing.T) *fakeAstroServer {
	t.Helper()
	f := &fakeAstroServer{t: t, connected: make(chan struct{}, 8)}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		ctx := r.Context()

		if err := writeEnv(ctx, conn, streamelements.Envelope{Type: streamelements.MessageTypeWelcome}); err != nil {
			return
		}

		resumeToken := r.URL.Query().Get("reconnect_token")
		if resumeToken == "" {
			for i := 0; i < 2; i++ {
				_, data, err := conn.Read(ctx)
				if err != nil {
					return
				}
				var req streamelements.Envelope
				if err := json.Unmarshal(data, &req); err != nil {
					return
				}
				resp := streamelements.Envelope{Type: streamelements.MessageTypeResponse, Nonce: req.Nonce}
				f.mu.Lock()
				subErr := f.subscribeError
				f.mu.Unlock()
				if subErr != "" {
					resp.Error = subErr
					_ = writeEnv(ctx, conn, resp)
					return
				}
				if err := writeEnv(ctx, conn, resp); err != nil {
					return
				}
			}
		}

		f.mu.Lock()
		f.active = conn
		f.connCount++
		f.lastResumeToken = resumeToken
		f.mu.Unlock()
		select {
		case f.connected <- struct{}{}:
		default:
		}

		<-ctx.Done()
	}))
	t.Cleanup(srv.Close)
	f.url = "ws" + strings.TrimPrefix(srv.URL, "http")
	return f
}

func writeEnv(ctx context.Context, conn *websocket.Conn, env streamelements.Envelope) error {
	data, err := json.Marshal(env)
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageText, data)
}

func (f *fakeAstroServer) waitConnected(t *testing.T, timeout time.Duration) {
	t.Helper()
	select {
	case <-f.connected:
	case <-time.After(timeout):
		t.Fatal("timed out waiting for the fake Astro server to accept a connection")
	}
}

func (f *fakeAstroServer) push(t *testing.T, env streamelements.Envelope) {
	t.Helper()
	f.mu.Lock()
	conn := f.active
	f.mu.Unlock()
	if conn == nil {
		t.Fatal("push() called with no active connection")
	}
	if err := writeEnv(context.Background(), conn, env); err != nil {
		t.Fatalf("push() write error = %v", err)
	}
}

func (f *fakeAstroServer) pushTip(t *testing.T, topic string, tip streamelements.Tip) {
	t.Helper()
	data, err := json.Marshal(tip)
	if err != nil {
		t.Fatalf("marshal tip: %v", err)
	}
	f.push(t, streamelements.Envelope{Type: streamelements.MessageTypeMessage, Topic: topic, Data: data})
}

func (f *fakeAstroServer) disconnectActive(t *testing.T) {
	t.Helper()
	f.mu.Lock()
	conn := f.active
	f.active = nil
	f.mu.Unlock()
	if conn == nil {
		t.Fatal("disconnectActive() called with no active connection")
	}
	_ = conn.CloseNow()
}

func (f *fakeAstroServer) resumeTokenOfLastConn() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastResumeToken
}

func (f *fakeAstroServer) connectionCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.connCount
}

func allowedTestTip(id string) streamelements.Tip {
	return streamelements.Tip{
		Donation: streamelements.TipDonation{
			User: streamelements.TipUser{Username: "Styler"}, Message: "great stream!",
			Amount: json.Number("4.2"), Currency: "USD",
		},
		ID: id, Approved: streamelements.ApprovedAllowed, Status: streamelements.StatusSuccess,
		CreatedAt: "2025-02-19T15:07:09.302Z",
	}
}

func pendingTestTip(id string) streamelements.Tip {
	tip := allowedTestTip(id)
	tip.Approved = streamelements.ApprovedPending
	return tip
}

type testSetup struct {
	manager *Manager
	sources *donationsource.Service
	secrets *secretstest.Store
	bus     *bus.Bus
	astro   *fakeAstroServer
}

func newTestSetup(t *testing.T) *testSetup {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sqlite.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("sqlite.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := sqlite.Migrate(context.Background(), db.DB); err != nil {
		t.Fatalf("sqlite.Migrate() error = %v", err)
	}

	astro := newFakeAstroServer(t)
	client := streamelements.New(streamelements.Options{WSBaseURL: astro.url})

	repo := sqlite.NewDonationSourceRepository(db.DB)
	secretStore := secretstest.New()
	sources := donationsource.NewService(donationsource.Options{Repository: repo, Secrets: secretStore})

	eventBus := bus.New(bus.Options{Capacity: 100})
	t.Cleanup(eventBus.Shutdown)

	m := NewManager(Options{Sources: sources, Secrets: secretStore, Bus: eventBus, Client: client, Logger: logger})
	// Rebuild sources with OnSourceRemoved wired to the manager, exactly the
	// way cmd/server's own real wiring connects donationsource.Service
	// deletion to this package's StopAndRemove.
	sources = donationsource.NewService(donationsource.Options{
		Repository: repo, Secrets: secretStore, OnSourceRemoved: m.StopAndRemove,
	})

	ts := &testSetup{manager: m, sources: sources, secrets: secretStore, bus: eventBus, astro: astro}

	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		m.Shutdown(ctx)
	})

	return ts
}

func speedUpTiming(t *testing.T) {
	t.Helper()
	origBackoff, origMaxBackoff := initialBackoff, maxBackoff
	initialBackoff, maxBackoff = 10*time.Millisecond, 50*time.Millisecond
	t.Cleanup(func() {
		initialBackoff, maxBackoff = origBackoff, origMaxBackoff
	})
}

func waitForState(t *testing.T, m *Manager, sourceID string, want State, timeout time.Duration) Snapshot {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last Snapshot
	for time.Now().Before(deadline) {
		snap, ok := m.Snapshot(sourceID)
		if ok {
			last = snap
			if snap.State == want {
				return snap
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("connector for %s never reached state %q, last snapshot = %+v", sourceID, want, last)
	return Snapshot{}
}

func createSource(t *testing.T, sources *donationsource.Service) donationsource.Source {
	t.Helper()
	src, err := sources.Create(context.Background(), donationsource.CreateInput{
		ProviderID: donationsource.ProviderStreamElements, Label: "Test", RemoteChannelID: "room-1", Token: "jwt-token",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	return src
}

func TestEnableConnectsAndReachesConnected(t *testing.T) {
	speedUpTiming(t)
	ts := newTestSetup(t)
	src := createSource(t, ts.sources)

	if _, err := ts.manager.Enable(context.Background(), src.ID); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	ts.astro.waitConnected(t, 2*time.Second)
	waitForState(t, ts.manager, src.ID, StateConnected, 2*time.Second)
}

func TestAllowedTipIsPublished(t *testing.T) {
	speedUpTiming(t)
	ts := newTestSetup(t)
	src := createSource(t, ts.sources)

	sub, _, err := ts.bus.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	if _, err := ts.manager.Enable(context.Background(), src.ID); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	ts.astro.waitConnected(t, 2*time.Second)
	waitForState(t, ts.manager, src.ID, StateConnected, 2*time.Second)

	ts.astro.pushTip(t, streamelements.TopicChannelTips, allowedTestTip("tip_1"))

	select {
	case evt := <-sub.Events():
		if evt.Type != engagement.TypeDonation {
			t.Fatalf("Type = %q, want donation", evt.Type)
		}
		if evt.ProviderID != engagement.ProviderStreamElements {
			t.Fatalf("ProviderID = %q, want streamelements", evt.ProviderID)
		}
		if evt.ConnectedAccountID != src.ID {
			t.Fatalf("ConnectedAccountID = %q, want %q", evt.ConnectedAccountID, src.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the donation event to be published")
	}
}

func TestPendingTipIsNotPublished(t *testing.T) {
	speedUpTiming(t)
	ts := newTestSetup(t)
	src := createSource(t, ts.sources)

	sub, _, err := ts.bus.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	if _, err := ts.manager.Enable(context.Background(), src.ID); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	ts.astro.waitConnected(t, 2*time.Second)
	waitForState(t, ts.manager, src.ID, StateConnected, 2*time.Second)

	ts.astro.pushTip(t, streamelements.TopicChannelTips, pendingTestTip("tip_pending_1"))

	select {
	case evt := <-sub.Events():
		t.Fatalf("expected no event for a pending tip, got %+v", evt)
	case <-time.After(150 * time.Millisecond):
		// expected: nothing published
	}
}

func TestUnexpectedDisconnectMarksPossibleGapThenReconnects(t *testing.T) {
	speedUpTiming(t)
	ts := newTestSetup(t)
	src := createSource(t, ts.sources)

	if _, err := ts.manager.Enable(context.Background(), src.ID); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	ts.astro.waitConnected(t, 2*time.Second)
	waitForState(t, ts.manager, src.ID, StateConnected, 2*time.Second)

	ts.astro.disconnectActive(t)

	waitForState(t, ts.manager, src.ID, StatePossibleGap, 2*time.Second)
	snap, _ := ts.manager.Snapshot(src.ID)
	if snap.PossibleGapCount != 1 {
		t.Fatalf("PossibleGapCount = %d, want 1", snap.PossibleGapCount)
	}

	// A real tip arriving clears the possible-gap display.
	ts.astro.waitConnected(t, 2*time.Second)
	ts.astro.pushTip(t, streamelements.TopicChannelTips, allowedTestTip("tip_after_gap"))
	waitForState(t, ts.manager, src.ID, StateConnected, 2*time.Second)
}

func TestGracefulReconnectUsesResumeTokenAndSkipsSubscribe(t *testing.T) {
	speedUpTiming(t)
	ts := newTestSetup(t)
	src := createSource(t, ts.sources)

	if _, err := ts.manager.Enable(context.Background(), src.ID); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	ts.astro.waitConnected(t, 2*time.Second)
	waitForState(t, ts.manager, src.ID, StateConnected, 2*time.Second)

	data, _ := json.Marshal(streamelements.ReconnectData{ReconnectToken: "grace-token-xyz"})
	ts.astro.push(t, streamelements.Envelope{Type: streamelements.MessageTypeReconnect, Data: data})

	ts.astro.waitConnected(t, 2*time.Second)
	if got := ts.astro.resumeTokenOfLastConn(); got != "grace-token-xyz" {
		t.Fatalf("resume connection's reconnect_token = %q, want grace-token-xyz", got)
	}
	// Graceful resume never displays a possible gap.
	waitForState(t, ts.manager, src.ID, StateConnected, 2*time.Second)
	snap, _ := ts.manager.Snapshot(src.ID)
	if snap.PossibleGapCount != 0 {
		t.Fatalf("PossibleGapCount = %d, want 0 after a graceful reconnect", snap.PossibleGapCount)
	}
}

func TestSubscribeErrorReachesReconnectRequiredAndStopsRetrying(t *testing.T) {
	speedUpTiming(t)
	ts := newTestSetup(t)
	src := createSource(t, ts.sources)
	ts.astro.subscribeError = "invalid token"

	if _, err := ts.manager.Enable(context.Background(), src.ID); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	waitForState(t, ts.manager, src.ID, StateReconnectRequired, 2*time.Second)

	connCountAtTerminal := ts.astro.connectionCount()
	time.Sleep(200 * time.Millisecond)
	if got := ts.astro.connectionCount(); got != connCountAtTerminal {
		t.Fatalf("connection count grew after reaching reconnect_required (%d -> %d) - the connector kept retrying a rejected credential", connCountAtTerminal, got)
	}

	snap, _ := ts.manager.Snapshot(src.ID)
	if snap.LastError != ErrorAuthFailed {
		t.Fatalf("LastError = %q, want %q", snap.LastError, ErrorAuthFailed)
	}
}

func TestCredentialMissingReachesErrorState(t *testing.T) {
	speedUpTiming(t)
	ts := newTestSetup(t)
	src := createSource(t, ts.sources)
	// Simulate the credential having disappeared from SecretStore out from
	// under the connector (a data-integrity edge case, not a normal path).
	if err := donationsource.DeleteCredential(context.Background(), ts.secrets, src.ID); err != nil {
		t.Fatalf("DeleteCredential() error = %v", err)
	}

	if _, err := ts.manager.Enable(context.Background(), src.ID); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	snap := waitForState(t, ts.manager, src.ID, StateError, 2*time.Second)
	if snap.LastError != ErrorCredentialMissing {
		t.Fatalf("LastError = %q, want %q", snap.LastError, ErrorCredentialMissing)
	}
}

func TestDisableClosesTheConnection(t *testing.T) {
	speedUpTiming(t)
	ts := newTestSetup(t)
	src := createSource(t, ts.sources)

	if _, err := ts.manager.Enable(context.Background(), src.ID); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	ts.astro.waitConnected(t, 2*time.Second)
	waitForState(t, ts.manager, src.ID, StateConnected, 2*time.Second)

	if _, err := ts.manager.Disable(context.Background(), src.ID); err != nil {
		t.Fatalf("Disable() error = %v", err)
	}
	if _, ok := ts.manager.Snapshot(src.ID); ok {
		t.Fatal("Snapshot() found a connector after Disable(), want none tracked")
	}
}

func TestSourceDeletionStopsAndRemovesTheConnector(t *testing.T) {
	speedUpTiming(t)
	ts := newTestSetup(t)

	src := createSource(t, ts.sources)
	if _, err := ts.manager.Enable(context.Background(), src.ID); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	ts.astro.waitConnected(t, 2*time.Second)
	waitForState(t, ts.manager, src.ID, StateConnected, 2*time.Second)

	// newTestSetup wires donationsource.Options.OnSourceRemoved to
	// m.StopAndRemove, exactly the way cmd/server's own real wiring will -
	// deleting the source through the domain service must itself stop the
	// connector.
	if err := ts.sources.Delete(context.Background(), src.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, ok := ts.manager.Snapshot(src.ID); ok {
		t.Fatal("Snapshot() found a connector after Delete(), want none tracked")
	}
}
