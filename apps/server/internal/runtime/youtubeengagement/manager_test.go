package youtubeengagement

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/account"
	engagement "github.com/streaming-tree/server/internal/domain/engagement"
	"github.com/streaming-tree/server/internal/domain/engagementsettings"
	bus "github.com/streaming-tree/server/internal/engagement"
	"github.com/streaming-tree/server/internal/provider/youtube"
	"github.com/streaming-tree/server/internal/secrets/secretstest"
	"github.com/streaming-tree/server/internal/storage/sqlite"
)

// fakeYouTubeAPI is a local httptest double serving only the two live-chat
// endpoints this connector depends on. The broadcast's liveChatId and the
// queue of liveChatMessages.list pages are both mutable at runtime so a
// test can simulate a broadcast/chat becoming available mid-test.
type fakeYouTubeAPI struct {
	srv *httptest.Server

	mu         sync.Mutex
	liveChatID string
	pages      []map[string]any // consumed in order, one per ListLiveChatMessages call; last one repeats
	listCalls  int
}

func newFakeYouTubeAPI(t *testing.T) *fakeYouTubeAPI {
	t.Helper()
	f := &fakeYouTubeAPI{}
	mux := http.NewServeMux()
	mux.HandleFunc("/liveBroadcasts", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		liveChatID := f.liveChatID
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{
					"id":      r.URL.Query().Get("id"),
					"snippet": map[string]any{"title": "Test Broadcast", "liveChatId": liveChatID},
					"status":  map[string]any{"lifeCycleStatus": "live"},
				},
			},
		})
	})
	mux.HandleFunc("/liveChat/messages", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		idx := f.listCalls
		if idx >= len(f.pages) {
			idx = len(f.pages) - 1
		}
		var page map[string]any
		if idx >= 0 {
			page = f.pages[idx]
		} else {
			page = map[string]any{"items": []map[string]any{}}
		}
		f.listCalls++
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(page)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeYouTubeAPI) setLiveChatID(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.liveChatID = id
}

func (f *fakeYouTubeAPI) setPages(pages ...map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pages = pages
	f.listCalls = 0
}

func textMessagePage(nextToken string, ended bool, messages ...map[string]any) map[string]any {
	page := map[string]any{
		"nextPageToken":         nextToken,
		"pollingIntervalMillis": 50, // fast, only used by the test's own timing knobs below
		"items":                 messages,
	}
	if ended {
		page["offlineAt"] = "2026-08-12T06:30:00Z"
	}
	return page
}

func textMessageItem(id, authorChannelID, text string) map[string]any {
	return map[string]any{
		"id": id,
		"snippet": map[string]any{
			"type": "textMessageEvent", "publishedAt": "2026-08-12T06:00:00Z",
			"authorChannelId": authorChannelID, "textMessageDetails": map[string]any{"messageText": text},
		},
		"authorDetails": map[string]any{"channelId": authorChannelID, "displayName": "Viewer"},
	}
}

func chatEndedItem(id string) map[string]any {
	return map[string]any{
		"id":      id,
		"snippet": map[string]any{"type": "chatEndedEvent", "publishedAt": "2026-08-12T06:00:00Z"},
	}
}

func unsupportedTypeItem(id string) map[string]any {
	return map[string]any{
		"id":      id,
		"snippet": map[string]any{"type": "pollEvent", "publishedAt": "2026-08-12T06:00:00Z"},
	}
}

func testBundle() account.TokenBundle {
	return account.TokenBundle{TokenType: "bearer", AccessToken: "fake-access-token", RefreshToken: "fake-refresh-token", ExpiresAt: time.Now().Add(time.Hour)}
}

type testSetup struct {
	manager  *Manager
	accounts *account.Service
	settings *engagementsettings.Service
	bus      *bus.Bus
	api      *fakeYouTubeAPI

	broadcastID string // empty means "no broadcast selected"
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

	api := newFakeYouTubeAPI(t)
	client := youtube.New(youtube.Options{APIBaseURL: api.srv.URL})

	accountRepo := sqlite.NewAccountRepository(db.DB)
	secretStore := secretstest.New()
	accounts := account.NewService(account.Options{
		Repository: accountRepo, Secrets: secretStore,
		Providers:      map[account.ProviderID]account.Provider{account.ProviderYouTube: youtube.NewAdapter(client)},
		RequiredScopes: map[account.ProviderID][]string{account.ProviderYouTube: {youtube.RequiredScope}},
		Logger:         logger,
	})

	now := time.Now().UTC()
	acc := account.Account{
		ID: "acct_1", ProviderID: account.ProviderYouTube, ProviderUserID: "yt_channel_1",
		Login: "streamer", DisplayName: "Streamer", Status: account.StatusConnected,
		CreatedAt: now, UpdatedAt: now, Scopes: []string{youtube.RequiredScope},
	}
	if err := accountRepo.CreateAccount(context.Background(), acc); err != nil {
		t.Fatalf("CreateAccount() error = %v", err)
	}
	if err := account.StoreTokenBundle(context.Background(), secretStore, "acct_1", testBundle()); err != nil {
		t.Fatalf("StoreTokenBundle() error = %v", err)
	}

	settingsRepo := sqlite.NewEngagementSettingsRepository(db.DB)
	settings := engagementsettings.NewService(settingsRepo, nil)

	eventBus := bus.New(bus.Options{Capacity: 100})
	t.Cleanup(eventBus.Shutdown)

	ts := &testSetup{accounts: accounts, settings: settings, bus: eventBus, api: api}

	m := NewManager(Options{
		Accounts: accounts, Settings: settings, Bus: eventBus, Client: client, Logger: logger,
		DestinationLookup: func(accountID string) (string, bool) {
			if accountID != "acct_1" {
				return "", false
			}
			return "platform_1", true
		},
		BroadcastLookup: func(platformID string) (string, bool) {
			if platformID != "platform_1" || ts.broadcastID == "" {
				return "", false
			}
			return ts.broadcastID, true
		},
	})
	ts.manager = m
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

// speedUpTiming shrinks every real-time knob this connector waits on so
// tests run in milliseconds rather than the production 2-30s range -
// restored automatically via t.Cleanup, never affects production values.
func speedUpTiming(t *testing.T) {
	t.Helper()
	origBackoff, origMaxBackoff := initialBackoff, maxBackoff
	origWaiting := waitingRetryInterval
	origMinPoll, origMaxPoll := minPollInterval, maxPollInterval
	initialBackoff, maxBackoff = 10*time.Millisecond, 50*time.Millisecond
	waitingRetryInterval = 20 * time.Millisecond
	minPollInterval, maxPollInterval = 10*time.Millisecond, 50*time.Millisecond
	t.Cleanup(func() {
		initialBackoff, maxBackoff = origBackoff, origMaxBackoff
		waitingRetryInterval = origWaiting
		minPollInterval, maxPollInterval = origMinPoll, origMaxPoll
	})
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
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("connector for %s never reached state %q, last snapshot = %+v", accountID, want, last)
	return Snapshot{}
}

func TestEnableWithNoBroadcastSelectedWaitsHonestly(t *testing.T) {
	speedUpTiming(t)
	ts := newTestSetup(t)
	if _, err := ts.manager.Enable(context.Background(), "acct_1"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	snap := waitForState(t, ts.manager, "acct_1", StateWaitingForBroadcast, 2*time.Second)
	if snap.SelectedBroadcastID != "" {
		t.Fatalf("expected no selected broadcast id while waiting, got %q", snap.SelectedBroadcastID)
	}
}

func TestEnableWithBroadcastButNoLiveChatWaits(t *testing.T) {
	speedUpTiming(t)
	ts := newTestSetup(t)
	ts.broadcastID = "bcast_1"
	ts.api.setLiveChatID("") // not live yet / chat disabled

	if _, err := ts.manager.Enable(context.Background(), "acct_1"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	waitForState(t, ts.manager, "acct_1", StateWaitingForLiveChat, 2*time.Second)
}

func TestFirstPollBaselinesAndDoesNotPublishHistory(t *testing.T) {
	speedUpTiming(t)
	ts := newTestSetup(t)
	ts.broadcastID = "bcast_1"
	ts.api.setLiveChatID("chat_1")
	ts.api.setPages(
		textMessagePage("token_after_baseline", false, textMessageItem("hist_1", "UC_old", "old history message")),
		textMessagePage("token_2", false), // no new messages on the second poll either
	)

	sub, _, err := ts.bus.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	if _, err := ts.manager.Enable(context.Background(), "acct_1"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	waitForState(t, ts.manager, "acct_1", StateConnected, 2*time.Second)

	select {
	case evt := <-sub.Events():
		t.Fatalf("expected no event published from the baseline history, got %+v", evt)
	case <-time.After(150 * time.Millisecond):
		// expected: nothing published
	}
}

func TestLiveMessageAfterBaselineIsPublished(t *testing.T) {
	speedUpTiming(t)
	ts := newTestSetup(t)
	ts.broadcastID = "bcast_1"
	ts.api.setLiveChatID("chat_1")
	ts.api.setPages(
		textMessagePage("token_1", false), // baseline: nothing
		textMessagePage("token_2", false, textMessageItem("live_1", "UC_viewer", "hello live chat")),
	)

	sub, _, err := ts.bus.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	if _, err := ts.manager.Enable(context.Background(), "acct_1"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	select {
	case evt := <-sub.Events():
		if evt.Type != engagement.TypeChatMessage {
			t.Fatalf("expected TypeChatMessage, got %s", evt.Type)
		}
		if evt.Message == nil || evt.Message.Text != "hello live chat" {
			t.Fatalf("expected live message text, got %+v", evt.Message)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the live message to be published")
	}
}

func TestChatEndedEventStopsPollingAndSetsState(t *testing.T) {
	speedUpTiming(t)
	ts := newTestSetup(t)
	ts.broadcastID = "bcast_1"
	ts.api.setLiveChatID("chat_1")
	ts.api.setPages(
		textMessagePage("token_1", false),
		textMessagePage("token_2", false, chatEndedItem("end_1")),
	)

	if _, err := ts.manager.Enable(context.Background(), "acct_1"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	waitForState(t, ts.manager, "acct_1", StateChatEnded, 2*time.Second)
}

func TestOfflineAtAlsoStopsPollingAsChatEnded(t *testing.T) {
	speedUpTiming(t)
	ts := newTestSetup(t)
	ts.broadcastID = "bcast_1"
	ts.api.setLiveChatID("chat_1")
	ts.api.setPages(
		textMessagePage("token_1", true), // offlineAt present on the very baseline call
	)

	if _, err := ts.manager.Enable(context.Background(), "acct_1"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	waitForState(t, ts.manager, "acct_1", StateChatEnded, 2*time.Second)
}

func TestUnsupportedEventTypeIsCountedNotPublished(t *testing.T) {
	speedUpTiming(t)
	ts := newTestSetup(t)
	ts.broadcastID = "bcast_1"
	ts.api.setLiveChatID("chat_1")
	ts.api.setPages(
		textMessagePage("token_1", false),
		textMessagePage("token_2", false, unsupportedTypeItem("poll_1")),
		textMessagePage("token_3", false),
	)

	sub, _, err := ts.bus.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	if _, err := ts.manager.Enable(context.Background(), "acct_1"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		snap, ok := ts.manager.Snapshot("acct_1")
		if ok && snap.UnsupportedEventCount > 0 {
			return // success
		}
		select {
		case evt := <-sub.Events():
			t.Fatalf("expected no event published for an unsupported type, got %+v", evt)
		default:
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out waiting for UnsupportedEventCount to increment")
}

func TestDisableStopsTheConnector(t *testing.T) {
	speedUpTiming(t)
	ts := newTestSetup(t)
	ts.broadcastID = "bcast_1"
	ts.api.setLiveChatID("chat_1")
	ts.api.setPages(textMessagePage("token_1", false))

	if _, err := ts.manager.Enable(context.Background(), "acct_1"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	waitForState(t, ts.manager, "acct_1", StateConnected, 2*time.Second)

	if _, err := ts.manager.Disable(context.Background(), "acct_1"); err != nil {
		t.Fatalf("Disable() error = %v", err)
	}
	if _, ok := ts.manager.Snapshot("acct_1"); ok {
		t.Fatal("expected no connector snapshot after Disable")
	}
}

func TestEnableRejectsNonYouTubeAccount(t *testing.T) {
	ts := newTestSetup(t)
	if _, err := ts.manager.Enable(context.Background(), "does-not-exist"); err == nil {
		t.Fatal("expected an error for an unknown account")
	}
}

func TestRestartEstablishesAFreshBaselineNoReplay(t *testing.T) {
	speedUpTiming(t)
	ts := newTestSetup(t)
	ts.broadcastID = "bcast_1"
	ts.api.setLiveChatID("chat_1")
	ts.api.setPages(
		textMessagePage("token_1", false, textMessageItem("live_1", "UC_viewer", "first live message")),
	)

	sub, _, err := ts.bus.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}

	if _, err := ts.manager.Enable(context.Background(), "acct_1"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	select {
	case <-sub.Events():
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the first live message")
	}

	// Simulate a broadcast change: same page content queued again. If
	// Restart replayed the old continuation instead of re-baselining,
	// the exact same message would be published a second time.
	ts.api.setPages(
		textMessagePage("token_1", false, textMessageItem("live_1", "UC_viewer", "first live message")),
	)
	if _, err := ts.manager.Restart(context.Background(), "acct_1"); err != nil {
		t.Fatalf("Restart() error = %v", err)
	}

	select {
	case evt := <-sub.Events():
		t.Fatalf("expected the post-restart baseline to suppress the replayed message, got %+v", evt)
	case <-time.After(300 * time.Millisecond):
		// expected: nothing published from the re-baseline
	}
}
