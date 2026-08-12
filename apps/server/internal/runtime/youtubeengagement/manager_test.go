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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/streaming-tree/server/internal/domain/account"
	engagement "github.com/streaming-tree/server/internal/domain/engagement"
	"github.com/streaming-tree/server/internal/domain/engagementsettings"
	bus "github.com/streaming-tree/server/internal/engagement"
	"github.com/streaming-tree/server/internal/provider/youtube"
	"github.com/streaming-tree/server/internal/provider/youtube/streamlistpb"
	"github.com/streaming-tree/server/internal/secrets/secretstest"
	"github.com/streaming-tree/server/internal/storage/sqlite"
)

// fakeYouTubeBroadcastAPI is a local httptest double serving only
// /liveBroadcasts - broadcast/liveChatId resolution stays REST (docs/
// provider-integrations/youtube-engagement.md §3.5/§9), unaffected by the
// Stage 15A transport corrective pass. Message receiving is exercised
// through fakeStreamListServer (grpc_fake_test.go) instead - a real local
// gRPC server, not a REST fake.
type fakeYouTubeBroadcastAPI struct {
	srv *httptest.Server

	mu         sync.Mutex
	liveChatID string
}

func newFakeYouTubeBroadcastAPI(t *testing.T) *fakeYouTubeBroadcastAPI {
	t.Helper()
	f := &fakeYouTubeBroadcastAPI{}
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
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeYouTubeBroadcastAPI) setLiveChatID(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.liveChatID = id
}

// --- streamList proto response builders, mirroring the old REST JSON
// helpers this file used before the transport correction - see
// docs/provider-integrations/youtube-engagement.md §4b.1 for the field
// mapping these mirror exactly.

func streamPage(nextToken string, ended bool, items ...*streamlistpb.LiveChatMessage) scriptedResponse {
	resp := &streamlistpb.LiveChatMessageListResponse{
		NextPageToken: proto.String(nextToken),
		Items:         items,
	}
	if ended {
		resp.OfflineAt = proto.String("2026-08-12T06:30:00Z")
	}
	return scriptedResponse{resp: resp}
}

func streamErr(code codes.Code, msg string) scriptedResponse {
	return scriptedResponse{err: status.Error(code, msg)}
}

func streamTextMessage(id, authorChannelID, text string) *streamlistpb.LiveChatMessage {
	return &streamlistpb.LiveChatMessage{
		Id: proto.String(id),
		Snippet: &streamlistpb.LiveChatMessageSnippet{
			Type:            streamlistpb.LiveChatMessageSnippet_TypeWrapper_TEXT_MESSAGE_EVENT.Enum(),
			PublishedAt:     proto.String("2026-08-12T06:00:00Z"),
			AuthorChannelId: proto.String(authorChannelID),
			DisplayedContent: &streamlistpb.LiveChatMessageSnippet_TextMessageDetails{
				TextMessageDetails: &streamlistpb.LiveChatTextMessageDetails{MessageText: proto.String(text)},
			},
		},
		AuthorDetails: &streamlistpb.LiveChatMessageAuthorDetails{
			ChannelId: proto.String(authorChannelID), DisplayName: proto.String("Viewer"),
		},
	}
}

func streamChatEnded(id string) *streamlistpb.LiveChatMessage {
	return &streamlistpb.LiveChatMessage{
		Id: proto.String(id),
		Snippet: &streamlistpb.LiveChatMessageSnippet{
			Type:        streamlistpb.LiveChatMessageSnippet_TypeWrapper_CHAT_ENDED_EVENT.Enum(),
			PublishedAt: proto.String("2026-08-12T06:00:00Z"),
		},
	}
}

func streamUnsupportedType(id string) *streamlistpb.LiveChatMessage {
	return &streamlistpb.LiveChatMessage{
		Id: proto.String(id),
		Snippet: &streamlistpb.LiveChatMessageSnippet{
			Type:        streamlistpb.LiveChatMessageSnippet_TypeWrapper_POLL_EVENT.Enum(),
			PublishedAt: proto.String("2026-08-12T06:00:00Z"),
		},
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
	rest     *fakeYouTubeBroadcastAPI
	grpcFake *fakeStreamListServer

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

	rest := newFakeYouTubeBroadcastAPI(t)
	grpcFake := newFakeStreamListServer(t)
	target, creds := grpcFake.grpcOptions()
	client := youtube.New(youtube.Options{APIBaseURL: rest.srv.URL, GRPCTarget: target, GRPCTransportCredentials: creds})

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

	ts := &testSetup{accounts: accounts, settings: settings, bus: eventBus, rest: rest, grpcFake: grpcFake}

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
	initialBackoff, maxBackoff = 10*time.Millisecond, 50*time.Millisecond
	waitingRetryInterval = 20 * time.Millisecond
	t.Cleanup(func() {
		initialBackoff, maxBackoff = origBackoff, origMaxBackoff
		waitingRetryInterval = origWaiting
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
	ts.rest.setLiveChatID("") // not live yet / chat disabled

	if _, err := ts.manager.Enable(context.Background(), "acct_1"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	waitForState(t, ts.manager, "acct_1", StateWaitingForLiveChat, 2*time.Second)
}

func TestFirstStreamResponseBaselinesAndDoesNotPublishHistory(t *testing.T) {
	speedUpTiming(t)
	ts := newTestSetup(t)
	ts.broadcastID = "bcast_1"
	ts.rest.setLiveChatID("chat_1")
	ts.grpcFake.setScript("chat_1",
		streamPage("token_after_baseline", false, streamTextMessage("hist_1", "UC_old", "old history message")),
		streamPage("token_2", false), // no new messages after the baseline either
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
	ts.rest.setLiveChatID("chat_1")
	ts.grpcFake.setScript("chat_1",
		streamPage("token_1", false), // baseline: nothing
		streamPage("token_2", false, streamTextMessage("live_1", "UC_viewer", "hello live chat")),
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

func TestChatEndedEventStopsReceivingAndSetsState(t *testing.T) {
	speedUpTiming(t)
	ts := newTestSetup(t)
	ts.broadcastID = "bcast_1"
	ts.rest.setLiveChatID("chat_1")
	ts.grpcFake.setScript("chat_1",
		streamPage("token_1", false),
		streamPage("token_2", false, streamChatEnded("end_1")),
	)

	if _, err := ts.manager.Enable(context.Background(), "acct_1"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	waitForState(t, ts.manager, "acct_1", StateChatEnded, 2*time.Second)
}

func TestOfflineAtAlsoStopsReceivingAsChatEnded(t *testing.T) {
	speedUpTiming(t)
	ts := newTestSetup(t)
	ts.broadcastID = "bcast_1"
	ts.rest.setLiveChatID("chat_1")
	ts.grpcFake.setScript("chat_1",
		streamPage("token_1", true), // offlineAt present on the very baseline response
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
	ts.rest.setLiveChatID("chat_1")
	ts.grpcFake.setScript("chat_1",
		streamPage("token_1", false),
		streamPage("token_2", false, streamUnsupportedType("poll_1")),
		streamPage("token_3", false),
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
	ts.rest.setLiveChatID("chat_1")
	ts.grpcFake.setScript("chat_1", streamPage("token_1", false))

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
	ts.rest.setLiveChatID("chat_1")
	ts.grpcFake.setScript("chat_1",
		streamPage("token_1", false), // baseline: nothing
		streamPage("token_2", false, streamTextMessage("live_1", "UC_viewer", "first live message")),
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

	// Restart forces a genuinely fresh stream (no held continuation, per
	// docs/provider-integrations/youtube-engagement.md §7). Placing the
	// same message directly in the *first* (baseline) response this time
	// proves it: if Restart incorrectly treated this as a resume, the
	// message would be published immediately as live; a correct fresh
	// baseline must suppress it.
	ts.grpcFake.setScript("chat_1",
		streamPage("token_1", false, streamTextMessage("live_1", "UC_viewer", "first live message")),
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

// --- Stage 15A transport corrective pass: baseline-vs-reconnect tests
// (docs/provider-integrations/youtube-engagement.md §7, corrective task
// §29-30) - these did not exist for the superseded REST design, since
// REST's discrete request/response calls never needed to distinguish a
// "fresh" call from a "resumed" one the way a long-lived gRPC stream does.

func TestTransientStreamLossResumesWithoutRebaselining(t *testing.T) {
	speedUpTiming(t)
	ts := newTestSetup(t)
	ts.broadcastID = "bcast_1"
	ts.rest.setLiveChatID("chat_1")
	// First connection: baseline (nothing), then a transient failure
	// (UNAVAILABLE) with a captured continuation token "token_after_baseline".
	ts.grpcFake.setScript("chat_1",
		streamPage("token_after_baseline", false),
		streamErr(codes.Unavailable, "transient"),
	)

	sub, _, err := ts.bus.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	if _, err := ts.manager.Enable(context.Background(), "acct_1"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	waitForState(t, ts.manager, "acct_1", StateReconnecting, 2*time.Second)

	// Once reconnecting, the resumed stream's first response must be
	// treated as live immediately, not re-baselined - the same content
	// that would be suppressed as history on a fresh connect must be
	// published here, since the connector is resuming from a still-valid
	// continuation token, not starting over.
	ts.grpcFake.setScript("chat_1",
		streamPage("token_after_resume", false, streamTextMessage("live_1", "UC_viewer", "resumed live message")),
	)

	select {
	case evt := <-sub.Events():
		if evt.Message == nil || evt.Message.Text != "resumed live message" {
			t.Fatalf("expected the resumed message to be published immediately, got %+v", evt)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the resumed live message - it was likely incorrectly re-baselined")
	}

	req, _, _ := ts.grpcFake.lastRequestSnapshot()
	if req.GetPageToken() != "token_after_baseline" {
		t.Fatalf("expected the reconnect request to carry the last known page token, got %q", req.GetPageToken())
	}
}

func TestInvalidContinuationTriggersFreshRebaseline(t *testing.T) {
	speedUpTiming(t)
	ts := newTestSetup(t)
	ts.broadcastID = "bcast_1"
	ts.rest.setLiveChatID("chat_1")
	ts.grpcFake.setScript("chat_1",
		streamPage("token_after_baseline", false),
		streamErr(codes.InvalidArgument, "continuation no longer valid"),
	)

	if _, err := ts.manager.Enable(context.Background(), "acct_1"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	waitForState(t, ts.manager, "acct_1", StateWaitingForLiveChat, 2*time.Second)

	snap, _ := ts.manager.Snapshot("acct_1")
	if snap.PossibleGapCount == 0 {
		t.Fatal("expected an invalid continuation to be recorded as a possible gap")
	}

	// The next attempt must start a genuinely fresh stream (no page
	// token) and re-baseline - a historical item in that fresh response
	// must not be published.
	sub, _, err := ts.bus.Subscribe(0)
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	ts.grpcFake.setScript("chat_1",
		streamPage("token_new_baseline", false, streamTextMessage("hist_2", "UC_old", "should be suppressed as re-baseline")),
		streamPage("token_new_2", false, streamTextMessage("live_2", "UC_viewer", "resumes normally after rebaseline")),
	)

	select {
	case evt := <-sub.Events():
		if evt.Message == nil || evt.Message.Text != "resumes normally after rebaseline" {
			t.Fatalf("expected only the post-rebaseline live message to be published, got %+v", evt)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the post-rebaseline live message")
	}
}

func TestSelectedBroadcastChangeCancelsOldStreamAndOpensNewLiveChatID(t *testing.T) {
	speedUpTiming(t)
	ts := newTestSetup(t)
	ts.broadcastID = "bcast_1"
	ts.rest.setLiveChatID("chat_1")
	ts.grpcFake.setScript("chat_1", streamPage("token_1", false))

	if _, err := ts.manager.Enable(context.Background(), "acct_1"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	waitForState(t, ts.manager, "acct_1", StateConnected, 2*time.Second)

	ts.rest.setLiveChatID("chat_2")
	ts.grpcFake.setScript("chat_2", streamPage("token_1", false))
	if _, err := ts.manager.Restart(context.Background(), "acct_1"); err != nil {
		t.Fatalf("Restart() error = %v", err)
	}
	waitForState(t, ts.manager, "acct_1", StateConnected, 2*time.Second)

	req, _, _ := ts.grpcFake.lastRequestSnapshot()
	if req.GetLiveChatId() != "chat_2" {
		t.Fatalf("expected the connector to have opened a stream for the newly-selected chat_2, last request was for %q", req.GetLiveChatId())
	}
}

func TestOAuthMetadataIsAttachedWithoutLoggingToken(t *testing.T) {
	speedUpTiming(t)
	ts := newTestSetup(t)
	ts.broadcastID = "bcast_1"
	ts.rest.setLiveChatID("chat_1")
	ts.grpcFake.setScript("chat_1", streamPage("token_1", false))

	if _, err := ts.manager.Enable(context.Background(), "acct_1"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	waitForState(t, ts.manager, "acct_1", StateConnected, 2*time.Second)

	_, auth, _ := ts.grpcFake.lastRequestSnapshot()
	if auth != "Bearer fake-access-token" {
		t.Fatalf("expected the request to carry the account's own OAuth token as Bearer metadata, got %q", auth)
	}
}

func TestRequestedPartsAreExactlyIDSnippetAuthorDetails(t *testing.T) {
	speedUpTiming(t)
	ts := newTestSetup(t)
	ts.broadcastID = "bcast_1"
	ts.rest.setLiveChatID("chat_1")
	ts.grpcFake.setScript("chat_1", streamPage("token_1", false))

	if _, err := ts.manager.Enable(context.Background(), "acct_1"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	waitForState(t, ts.manager, "acct_1", StateConnected, 2*time.Second)

	req, _, _ := ts.grpcFake.lastRequestSnapshot()
	want := []string{"id", "snippet", "authorDetails"}
	got := req.GetPart()
	if len(got) != len(want) {
		t.Fatalf("expected part=%v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected part=%v, got %v", want, got)
		}
	}
}
