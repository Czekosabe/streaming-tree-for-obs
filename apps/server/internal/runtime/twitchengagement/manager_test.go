package twitchengagement

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/streaming-tree/server/internal/domain/account"
	"github.com/streaming-tree/server/internal/domain/engagementsettings"
	bus "github.com/streaming-tree/server/internal/engagement"
	"github.com/streaming-tree/server/internal/provider/twitch"
	"github.com/streaming-tree/server/internal/secrets/secretstest"
	"github.com/streaming-tree/server/internal/storage/sqlite"
)

// fakeHelix is a local httptest double reproducing only the
// POST /eventsub/subscriptions response shape this connector depends on -
// no real Twitch request is ever made by these tests.
type fakeHelix struct {
	srv            *httptest.Server
	subscribeCount atomic.Int64
	forbiddenTypes map[string]bool // subscription types that respond 403
}

func newFakeHelix(t *testing.T) *fakeHelix {
	t.Helper()
	fh := &fakeHelix{forbiddenTypes: map[string]bool{}}
	fh.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/eventsub/subscriptions" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var body struct {
			Type string `json:"type"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if fh.forbiddenTypes[body.Type] {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		fh.subscribeCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(fmt.Sprintf(`{"data":[{"id":"sub_%d","status":"enabled"}]}`, fh.subscribeCount.Load())))
	}))
	t.Cleanup(fh.srv.Close)
	return fh
}

// fakeEventSubServer accepts WebSocket connections and hands each one to
// the test goroutine via the accepted channel, which then fully drives that
// connection's conversation - no pre-scripting, maximum test flexibility.
type fakeEventSubServer struct {
	srv      *httptest.Server
	accepted chan *websocket.Conn
}

func newFakeEventSubServer(t *testing.T) *fakeEventSubServer {
	t.Helper()
	fs := &fakeEventSubServer{accepted: make(chan *websocket.Conn, 8)}
	mux := http.NewServeMux()
	handler := func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		fs.accepted <- conn
		<-r.Context().Done()
	}
	mux.HandleFunc("/", handler)
	mux.HandleFunc("/reconnect", handler)
	fs.srv = httptest.NewServer(mux)
	t.Cleanup(fs.srv.Close)
	return fs
}

func (fs *fakeEventSubServer) acceptConn(t *testing.T) *websocket.Conn {
	t.Helper()
	select {
	case c := <-fs.accepted:
		return c
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a connection")
		return nil
	}
}

func (fs *fakeEventSubServer) reconnectURL() string {
	return "ws://" + strings.TrimPrefix(fs.srv.URL, "http://") + "/reconnect"
}

func sendEnvelope(t *testing.T, conn *websocket.Conn, messageType string, payload any) {
	t.Helper()
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	env := twitch.EventSubEnvelope{
		Metadata: twitch.EventSubMetadata{
			MessageID:   "wsmsg_" + strconv.FormatInt(time.Now().UnixNano(), 10),
			MessageType: messageType, MessageTimestamp: time.Now().UTC().Format(time.RFC3339Nano),
		},
		Payload: payloadBytes,
	}
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	if err := conn.Write(context.Background(), websocket.MessageText, data); err != nil {
		t.Fatalf("write envelope: %v", err)
	}
}

func sendWelcome(t *testing.T, conn *websocket.Conn, sessionID string, keepaliveSeconds int) {
	t.Helper()
	sendEnvelope(t, conn, twitch.MessageTypeWelcome, map[string]any{
		"session": map[string]any{"id": sessionID, "status": "connected", "keepalive_timeout_seconds": keepaliveSeconds, "reconnect_url": nil},
	})
}

func sendNotification(t *testing.T, conn *websocket.Conn, subType string, event any) {
	t.Helper()
	payloadBytes, _ := json.Marshal(event)
	env := twitch.EventSubEnvelope{
		Metadata: twitch.EventSubMetadata{
			MessageID:   "notif_" + strconv.FormatInt(time.Now().UnixNano(), 10),
			MessageType: twitch.MessageTypeNotification, MessageTimestamp: time.Now().UTC().Format(time.RFC3339Nano),
			SubscriptionType: subType, SubscriptionVersion: "1",
		},
		Payload: mustMarshalNotificationPayload(t, subType, payloadBytes),
	}
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal notification: %v", err)
	}
	if err := conn.Write(context.Background(), websocket.MessageText, data); err != nil {
		t.Fatalf("write notification: %v", err)
	}
}

func mustMarshalNotificationPayload(t *testing.T, subType string, eventBytes []byte) json.RawMessage {
	t.Helper()
	payload := map[string]any{
		"subscription": map[string]any{"id": "sub_x", "status": "enabled", "type": subType, "version": "1"},
		"event":        json.RawMessage(eventBytes),
	}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal notification payload: %v", err)
	}
	return b
}

func sendRevocation(t *testing.T, conn *websocket.Conn, subType, status string) {
	t.Helper()
	sendEnvelope(t, conn, twitch.MessageTypeRevocation, map[string]any{
		"subscription": map[string]any{"id": "sub_x", "status": status, "type": subType, "version": "1"},
	})
}

func testBundle() account.TokenBundle {
	return account.TokenBundle{TokenType: "bearer", AccessToken: "fake-access-token", RefreshToken: "fake-refresh-token", ExpiresAt: time.Now().Add(time.Hour)}
}

type testSetup struct {
	manager  *Manager
	accounts *account.Service
	settings *engagementsettings.Service
	bus      *bus.Bus
	helix    *fakeHelix
	ws       *fakeEventSubServer
}

// newTestSetup builds a Manager wired to fake Twitch servers and a
// pre-created, fully-scoped connected Twitch account "acct_1".
func newTestSetup(t *testing.T, scopes []string) *testSetup {
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

	helix := newFakeHelix(t)
	ws := newFakeEventSubServer(t)

	client := twitch.New(twitch.Options{APIBaseURL: helix.srv.URL, EventSubURL: ws.srv.URL})

	accountRepo := sqlite.NewAccountRepository(db.DB)
	secretStore := secretstest.New()
	accounts := account.NewService(account.Options{
		Repository: accountRepo, Secrets: secretStore,
		Providers:      map[account.ProviderID]account.Provider{account.ProviderTwitch: twitch.NewAdapter(client)},
		RequiredScopes: map[account.ProviderID][]string{account.ProviderTwitch: {"channel:manage:broadcast"}},
		Logger:         logger,
	})

	now := time.Now().UTC()
	acc := account.Account{
		ID: "acct_1", ProviderID: account.ProviderTwitch, ProviderUserID: "twitch_user_1",
		Login: "streamer", DisplayName: "Streamer", Status: account.StatusConnected,
		CreatedAt: now, UpdatedAt: now, Scopes: scopes,
	}
	if err := accountRepo.CreateAccount(context.Background(), acc); err != nil {
		t.Fatalf("CreateAccount() error = %v", err)
	}
	if err := account.StoreTokenBundle(context.Background(), secretStore, "acct_1", testBundle()); err != nil {
		t.Fatalf("StoreTokenBundle() error = %v", err)
	}
	if _, err := accounts.SetIntegrationClientID(context.Background(), account.ProviderTwitch, "test-client-id"); err != nil {
		t.Fatalf("SetIntegrationClientID() error = %v", err)
	}

	settingsRepo := sqlite.NewEngagementSettingsRepository(db.DB)
	settings := engagementsettings.NewService(settingsRepo, nil)

	eventBus := bus.New(bus.Options{Capacity: 100})
	t.Cleanup(eventBus.Shutdown)

	wsHost := strings.TrimPrefix(strings.TrimPrefix(ws.srv.URL, "http://"), "https://")
	if idx := strings.Index(wsHost, ":"); idx >= 0 {
		wsHost = wsHost[:idx]
	}

	m := NewManager(Options{
		Accounts: accounts, Settings: settings, Bus: eventBus, Client: client, Logger: logger,
		AllowedReconnectHosts: []string{wsHost},
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		m.Shutdown(ctx)
	})

	return &testSetup{manager: m, accounts: accounts, settings: settings, bus: eventBus, helix: helix, ws: ws}
}

func waitForState(t *testing.T, m *Manager, accountID string, want State, timeout time.Duration) Snapshot {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last Snapshot
	for time.Now().Before(deadline) {
		snap, ok := m.Snapshot(accountID)
		if ok {
			last = snap
			if snap.State == want {
				return snap
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("connector for %s never reached state %q, last snapshot = %+v", accountID, want, last)
	return Snapshot{}
}

func TestConnectorReachesConnectedStateAfterWelcomeAndSubscriptions(t *testing.T) {
	ts := newTestSetup(t, append([]string{"channel:manage:broadcast"}, twitch.EngagementScopeProfile...))
	ts.manager.lifecycle, ts.manager.cancelAll = context.WithCancel(context.Background())

	go func() {
		conn := ts.ws.acceptConn(t)
		sendWelcome(t, conn, "sess1", 10)
	}()

	if _, err := ts.manager.Enable(context.Background(), "acct_1"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	snap := waitForState(t, ts.manager, "acct_1", StateConnected, 3*time.Second)
	if snap.ExpectedSubscriptionCount != len(twitch.EventSubSubscriptionDefs) {
		t.Errorf("ExpectedSubscriptionCount = %d, want %d", snap.ExpectedSubscriptionCount, len(twitch.EventSubSubscriptionDefs))
	}
	if snap.ActiveSubscriptionCount != len(twitch.EventSubSubscriptionDefs) {
		t.Errorf("ActiveSubscriptionCount = %d, want %d", snap.ActiveSubscriptionCount, len(twitch.EventSubSubscriptionDefs))
	}
	if int(ts.helix.subscribeCount.Load()) != len(twitch.EventSubSubscriptionDefs) {
		t.Errorf("helix received %d subscription requests, want %d", ts.helix.subscribeCount.Load(), len(twitch.EventSubSubscriptionDefs))
	}
}

func TestConnectorBlockedWhenEngagementScopesMissing(t *testing.T) {
	ts := newTestSetup(t, []string{"channel:manage:broadcast"})
	ts.manager.lifecycle, ts.manager.cancelAll = context.WithCancel(context.Background())

	snap, err := ts.manager.Enable(context.Background(), "acct_1")
	if err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	if snap.State != StateBlocked {
		t.Fatalf("State = %q, want blocked when engagement scopes are missing", snap.State)
	}
	if len(snap.BlockerCodes) == 0 || snap.BlockerCodes[0] != BlockerScopeUpgradeRequired {
		t.Errorf("BlockerCodes = %v, want [%s]", snap.BlockerCodes, BlockerScopeUpgradeRequired)
	}
	if len(snap.MissingScopes) != len(twitch.EngagementScopeProfile) {
		t.Errorf("MissingScopes = %v, want every engagement scope", snap.MissingScopes)
	}
}

func TestConnectorPublishesNormalizedEventFromNotification(t *testing.T) {
	ts := newTestSetup(t, append([]string{"channel:manage:broadcast"}, twitch.EngagementScopeProfile...))
	ts.manager.lifecycle, ts.manager.cancelAll = context.WithCancel(context.Background())

	connCh := make(chan *websocket.Conn, 1)
	go func() {
		conn := ts.ws.acceptConn(t)
		sendWelcome(t, conn, "sess1", 10)
		connCh <- conn
	}()

	if _, err := ts.manager.Enable(context.Background(), "acct_1"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	waitForState(t, ts.manager, "acct_1", StateConnected, 3*time.Second)

	sub, _, err := ts.bus.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	defer sub.Cancel()

	conn := <-connCh
	sendNotification(t, conn, "channel.follow", map[string]any{
		"user_id": "u1", "user_login": "viewer", "user_name": "Viewer", "followed_at": time.Now().UTC().Format(time.RFC3339),
	})

	select {
	case evt := <-sub.Events():
		if string(evt.Type) != "follow" {
			t.Errorf("Type = %q, want follow", evt.Type)
		}
		if evt.ConnectedAccountID != "acct_1" {
			t.Errorf("ConnectedAccountID = %q, want acct_1", evt.ConnectedAccountID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for the normalized event to reach the bus")
	}
}

func TestConnectorMarksDataGapOnOrdinaryDisconnect(t *testing.T) {
	ts := newTestSetup(t, append([]string{"channel:manage:broadcast"}, twitch.EngagementScopeProfile...))
	ts.manager.lifecycle, ts.manager.cancelAll = context.WithCancel(context.Background())

	go func() {
		conn := ts.ws.acceptConn(t)
		sendWelcome(t, conn, "sess1", 10)
		time.Sleep(200 * time.Millisecond)
		conn.CloseNow() // ordinary, non-official loss

		conn2 := ts.ws.acceptConn(t)
		sendWelcome(t, conn2, "sess2", 10)
	}()

	if _, err := ts.manager.Enable(context.Background(), "acct_1"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	waitForState(t, ts.manager, "acct_1", StateConnected, 3*time.Second)

	// Wait for the reconnect to actually happen (backoff starts at 1s) -
	// polling for ReconnectCount > 0 rather than merely "state is connected
	// again", since the state was already Connected before the disconnect
	// even occurs and a naive re-check could race and observe that stale
	// value instead of the post-reconnect one.
	snap := waitForReconnectCount(t, ts.manager, "acct_1", 5*time.Second)
	if snap.State != StateConnected {
		t.Errorf("State = %q, want connected again after the reconnect completes", snap.State)
	}
	if snap.LastDataGapAt == nil {
		t.Error("expected LastDataGapAt to be set after an ordinary disconnect")
	}
	if int(ts.helix.subscribeCount.Load()) != 2*len(twitch.EventSubSubscriptionDefs) {
		t.Errorf("helix received %d subscription requests, want subscriptions recreated after ordinary reconnect (%d)",
			ts.helix.subscribeCount.Load(), 2*len(twitch.EventSubSubscriptionDefs))
	}
}

// waitForReconnectCount waits for the connector to have counted at least one
// reconnect AND to have settled back into StateConnected - reconnectCount
// increments immediately when the loss is detected, well before the bounded
// backoff and the new connection's welcome complete, so both conditions are
// required to observe the reconnect as actually finished.
func waitForReconnectCount(t *testing.T, m *Manager, accountID string, timeout time.Duration) Snapshot {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last Snapshot
	for time.Now().Before(deadline) {
		snap, ok := m.Snapshot(accountID)
		if ok {
			last = snap
			if snap.ReconnectCount > 0 && snap.State == StateConnected {
				return snap
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("connector for %s never finished reconnecting, last snapshot = %+v", accountID, last)
	return Snapshot{}
}

func TestConnectorHandlesOfficialReconnectWithoutDataGapOrResubscription(t *testing.T) {
	ts := newTestSetup(t, append([]string{"channel:manage:broadcast"}, twitch.EngagementScopeProfile...))
	ts.manager.lifecycle, ts.manager.cancelAll = context.WithCancel(context.Background())

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn := ts.ws.acceptConn(t)
		sendWelcome(t, conn, "sess1", 10)
		time.Sleep(200 * time.Millisecond)
		sendEnvelope(t, conn, twitch.MessageTypeReconnect, map[string]any{
			"session": map[string]any{"id": "sess1", "status": "reconnecting", "keepalive_timeout_seconds": nil, "reconnect_url": ts.ws.reconnectURL()},
		})

		conn2 := ts.ws.acceptConn(t)
		sendWelcome(t, conn2, "sess2", 10)
		// The old connection must stay open until the new one's welcome is
		// sent - only close it now, mirroring what a real handoff expects.
		time.Sleep(100 * time.Millisecond)
		conn.CloseNow()
	}()

	if _, err := ts.manager.Enable(context.Background(), "acct_1"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	waitForState(t, ts.manager, "acct_1", StateConnected, 3*time.Second)

	wg.Wait()
	time.Sleep(300 * time.Millisecond) // let the handoff settle

	snap, ok := ts.manager.Snapshot("acct_1")
	if !ok {
		t.Fatal("expected a snapshot to still exist")
	}
	if snap.State != StateConnected {
		t.Errorf("State = %q, want connected after a successful official reconnect", snap.State)
	}
	if snap.LastDataGapAt != nil {
		t.Error("expected no data gap after an official session_reconnect handoff")
	}
	if int(ts.helix.subscribeCount.Load()) != len(twitch.EventSubSubscriptionDefs) {
		t.Errorf("helix received %d subscription requests, want subscriptions NOT recreated after official reconnect (%d)",
			ts.helix.subscribeCount.Load(), len(twitch.EventSubSubscriptionDefs))
	}
}

func TestConnectorEntersErrorStateOnAuthorizationRevokedAndDoesNotAutoRetry(t *testing.T) {
	ts := newTestSetup(t, append([]string{"channel:manage:broadcast"}, twitch.EngagementScopeProfile...))
	ts.manager.lifecycle, ts.manager.cancelAll = context.WithCancel(context.Background())

	go func() {
		conn := ts.ws.acceptConn(t)
		sendWelcome(t, conn, "sess1", 10)
		time.Sleep(200 * time.Millisecond)
		sendRevocation(t, conn, "channel.chat.message", "authorization_revoked")
	}()

	if _, err := ts.manager.Enable(context.Background(), "acct_1"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	waitForState(t, ts.manager, "acct_1", StateConnected, 3*time.Second)

	snap := waitForState(t, ts.manager, "acct_1", StateError, 3*time.Second)
	if snap.LastError != "twitch_eventsub_subscription_revoked" {
		t.Errorf("LastError = %q, want twitch_eventsub_subscription_revoked", snap.LastError)
	}

	// Confirm it does not silently start reconnecting afterward.
	time.Sleep(300 * time.Millisecond)
	snap2, _ := ts.manager.Snapshot("acct_1")
	if snap2.State != StateError {
		t.Errorf("State = %q after settling, want it to stay error (no auto-retry)", snap2.State)
	}
}

func TestDisableStopsTheConnector(t *testing.T) {
	ts := newTestSetup(t, append([]string{"channel:manage:broadcast"}, twitch.EngagementScopeProfile...))
	ts.manager.lifecycle, ts.manager.cancelAll = context.WithCancel(context.Background())

	go func() {
		conn := ts.ws.acceptConn(t)
		sendWelcome(t, conn, "sess1", 10)
	}()

	if _, err := ts.manager.Enable(context.Background(), "acct_1"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	waitForState(t, ts.manager, "acct_1", StateConnected, 3*time.Second)

	snap, err := ts.manager.Disable(context.Background(), "acct_1")
	if err != nil {
		t.Fatalf("Disable() error = %v", err)
	}
	if snap.State != StateDisabled {
		t.Errorf("State = %q, want disabled", snap.State)
	}
	if _, ok := ts.manager.Snapshot("acct_1"); ok {
		t.Error("expected no tracked connector after Disable()")
	}

	settings, found, err := ts.settings.Get(context.Background(), "acct_1")
	if err != nil || !found || settings.Enabled {
		t.Errorf("persisted settings = %+v, found=%v, err=%v, want enabled=false", settings, found, err)
	}
}

func TestManagerStartRestoresEnabledConnectorsFromSettings(t *testing.T) {
	ts := newTestSetup(t, append([]string{"channel:manage:broadcast"}, twitch.EngagementScopeProfile...))

	if _, err := ts.settings.SetEnabled(context.Background(), "acct_1", true); err != nil {
		t.Fatalf("SetEnabled() error = %v", err)
	}

	go func() {
		conn := ts.ws.acceptConn(t)
		sendWelcome(t, conn, "sess1", 10)
	}()

	if err := ts.manager.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	waitForState(t, ts.manager, "acct_1", StateConnected, 3*time.Second)
}

func TestShutdownStopsAllConnectorsCleanly(t *testing.T) {
	ts := newTestSetup(t, append([]string{"channel:manage:broadcast"}, twitch.EngagementScopeProfile...))
	ts.manager.lifecycle, ts.manager.cancelAll = context.WithCancel(context.Background())

	go func() {
		conn := ts.ws.acceptConn(t)
		sendWelcome(t, conn, "sess1", 10)
	}()

	if _, err := ts.manager.Enable(context.Background(), "acct_1"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	waitForState(t, ts.manager, "acct_1", StateConnected, 3*time.Second)

	done := make(chan struct{})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		ts.manager.Shutdown(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Shutdown() did not return promptly - possible deadlock")
	}
}
