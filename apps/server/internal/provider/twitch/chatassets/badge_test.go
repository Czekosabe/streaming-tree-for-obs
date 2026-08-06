package chatassets

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/account"
	twitch "github.com/streaming-tree/server/internal/provider/twitch"
	"github.com/streaming-tree/server/internal/secrets/secretstest"
	"github.com/streaming-tree/server/internal/storage/sqlite"
)

// fakeBadgeServer is a local httptest double reproducing only the
// GET /chat/badges/global and GET /chat/badges response shape this
// resolver depends on - no real Twitch request is ever made by these
// tests.
type fakeBadgeServer struct {
	srv           *httptest.Server
	globalCalls   atomic.Int64
	channelCalls  atomic.Int64
	channelBadges string
	globalBadges  string
}

func newFakeBadgeServer(t *testing.T) *fakeBadgeServer {
	t.Helper()
	fb := &fakeBadgeServer{
		globalBadges: `{"data":[
			{"set_id":"vip","versions":[{"id":"1","image_url_1x":"https://static-cdn.jtvnw.net/badges/v1/vip/1","image_url_2x":"https://static-cdn.jtvnw.net/badges/v1/vip/2","image_url_4x":"https://static-cdn.jtvnw.net/badges/v1/vip/4"}]}
		]}`,
		channelBadges: `{"data":[
			{"set_id":"subscriber","versions":[{"id":"0","image_url_1x":"https://static-cdn.jtvnw.net/badges/v1/sub/1","image_url_2x":"https://static-cdn.jtvnw.net/badges/v1/sub/2","image_url_4x":"https://static-cdn.jtvnw.net/badges/v1/sub/4"}]}
		]}`,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/chat/badges/global", func(w http.ResponseWriter, r *http.Request) {
		fb.globalCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fb.globalBadges))
	})
	mux.HandleFunc("/chat/badges", func(w http.ResponseWriter, r *http.Request) {
		fb.channelCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fb.channelBadges))
	})
	fb.srv = httptest.NewServer(mux)
	t.Cleanup(fb.srv.Close)
	return fb
}

type testFixture struct {
	resolver  *Resolver
	badges    *fakeBadgeServer
	accountID string
}

func newTestFixture(t *testing.T, now func() time.Time) *testFixture {
	t.Helper()
	dbPath := t.TempDir() + "/test.db"
	db, err := sqlite.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("sqlite.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := sqlite.Migrate(context.Background(), db.DB); err != nil {
		t.Fatalf("sqlite.Migrate() error = %v", err)
	}

	badges := newFakeBadgeServer(t)
	client := twitch.New(twitch.Options{APIBaseURL: badges.srv.URL})

	accountRepo := sqlite.NewAccountRepository(db.DB)
	secretStore := secretstest.New()
	accounts := account.NewService(account.Options{
		Repository: accountRepo, Secrets: secretStore,
		Providers: map[account.ProviderID]account.Provider{account.ProviderTwitch: twitch.NewAdapter(client)},
	})

	nowTime := time.Now().UTC()
	acc := account.Account{
		ID: "acct_badge_1", ProviderID: account.ProviderTwitch, ProviderUserID: "broadcaster_1",
		Login: "streamer", DisplayName: "Streamer", Status: account.StatusConnected,
		CreatedAt: nowTime, UpdatedAt: nowTime,
	}
	if err := accountRepo.CreateAccount(context.Background(), acc); err != nil {
		t.Fatalf("CreateAccount() error = %v", err)
	}
	if err := account.StoreTokenBundle(context.Background(), secretStore, "acct_badge_1", account.TokenBundle{
		TokenType: "bearer", AccessToken: "fake-token", RefreshToken: "fake-refresh", ExpiresAt: nowTime.Add(time.Hour),
	}); err != nil {
		t.Fatalf("StoreTokenBundle() error = %v", err)
	}
	if _, err := accounts.SetIntegrationClientID(context.Background(), account.ProviderTwitch, "test-client-id"); err != nil {
		t.Fatalf("SetIntegrationClientID() error = %v", err)
	}

	resolver := NewResolver(client, accounts, now)
	return &testFixture{resolver: resolver, badges: badges, accountID: "acct_badge_1"}
}

func TestResolveBadgeFindsChannelSpecificBadgeFirst(t *testing.T) {
	f := newTestFixture(t, nil)

	img, ok := f.resolver.ResolveBadge(context.Background(), f.accountID, "subscriber", "0")
	if !ok {
		t.Fatal("ResolveBadge() ok = false, want true for a channel-scoped badge")
	}
	if img.URL2x != "https://static-cdn.jtvnw.net/badges/v1/sub/2" {
		t.Errorf("URL2x = %q, want the channel catalog's image", img.URL2x)
	}
}

func TestResolveBadgeFallsBackToGlobalCatalog(t *testing.T) {
	f := newTestFixture(t, nil)

	img, ok := f.resolver.ResolveBadge(context.Background(), f.accountID, "vip", "1")
	if !ok {
		t.Fatal("ResolveBadge() ok = false, want true for a global-only badge")
	}
	if img.URL2x != "https://static-cdn.jtvnw.net/badges/v1/vip/2" {
		t.Errorf("URL2x = %q, want the global catalog's image", img.URL2x)
	}
}

func TestResolveBadgeUnknownReturnsFalseWithoutError(t *testing.T) {
	f := newTestFixture(t, nil)

	_, ok := f.resolver.ResolveBadge(context.Background(), f.accountID, "does_not_exist", "0")
	if ok {
		t.Error("ResolveBadge() ok = true, want false for an unknown set/version")
	}
}

func TestResolveBadgeCachesAndDoesNotRefetchWithinTTL(t *testing.T) {
	f := newTestFixture(t, nil)

	f.resolver.ResolveBadge(context.Background(), f.accountID, "subscriber", "0")
	f.resolver.ResolveBadge(context.Background(), f.accountID, "subscriber", "0")
	f.resolver.ResolveBadge(context.Background(), f.accountID, "vip", "1")
	f.resolver.ResolveBadge(context.Background(), f.accountID, "vip", "1")

	if got := f.badges.channelCalls.Load(); got != 1 {
		t.Errorf("channel catalog fetched %d times, want exactly 1 (cached)", got)
	}
	if got := f.badges.globalCalls.Load(); got != 1 {
		t.Errorf("global catalog fetched %d times, want exactly 1 (cached)", got)
	}
}

func TestResolveBadgeRefetchesAfterTTLExpires(t *testing.T) {
	current := time.Now().UTC()
	f := newTestFixture(t, func() time.Time { return current })

	f.resolver.ResolveBadge(context.Background(), f.accountID, "subscriber", "0")
	current = current.Add(2 * time.Hour)
	f.resolver.ResolveBadge(context.Background(), f.accountID, "subscriber", "0")

	if got := f.badges.channelCalls.Load(); got != 2 {
		t.Errorf("channel catalog fetched %d times, want exactly 2 (one per TTL window)", got)
	}
}

func TestResolveBadgeConcurrentCallsSingleFlight(t *testing.T) {
	f := newTestFixture(t, nil)

	const workers = 20
	done := make(chan struct{}, workers)
	for i := 0; i < workers; i++ {
		go func() {
			f.resolver.ResolveBadge(context.Background(), f.accountID, "subscriber", "0")
			done <- struct{}{}
		}()
	}
	for i := 0; i < workers; i++ {
		<-done
	}

	if got := f.badges.channelCalls.Load(); got != 1 {
		t.Errorf("channel catalog fetched %d times under concurrent load, want exactly 1 (single-flight)", got)
	}
}

func TestResolveBadgeUnknownAccountReturnsFalse(t *testing.T) {
	f := newTestFixture(t, nil)

	_, ok := f.resolver.ResolveBadge(context.Background(), "acct_does_not_exist", "subscriber", "0")
	if ok {
		t.Error("ResolveBadge() ok = true, want false for an unknown account")
	}
}
