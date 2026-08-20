package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	co "github.com/streaming-tree/server/internal/chatoverlay"
	"github.com/streaming-tree/server/internal/domain/account"
	chatoverlaydomain "github.com/streaming-tree/server/internal/domain/chatoverlay"
	"github.com/streaming-tree/server/internal/domain/visualdesign"
	bus "github.com/streaming-tree/server/internal/engagement"
	oc "github.com/streaming-tree/server/internal/operatorchat"
	"github.com/streaming-tree/server/internal/secrets/secretstest"
	"github.com/streaming-tree/server/internal/storage/sqlite"
)

// newRemoteOverlayManagementServer builds a real router with the
// management API wired for the chat-overlay domain - sufficient to
// prove the shared, domain-parameterized handler's own contract
// (auth boundary, validation, URL construction, lifecycle) without
// standing up all four domains' full dependency graphs again.
func newRemoteOverlayManagementServer(t *testing.T, overlayOrigin string) (http.Handler, chatoverlaydomain.Profile, *sqlite.RemoteOverlayCapabilityRepository) {
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

	accounts := account.NewService(account.Options{
		Repository: sqlite.NewAccountRepository(db.DB), Secrets: secretstest.New(), Logger: logger,
	})

	eventBus := bus.New(bus.Options{Capacity: 100})
	t.Cleanup(eventBus.Shutdown)
	projection := oc.New(oc.Options{Source: eventBus, Capacity: 100})
	if err := projection.Start(context.Background()); err != nil {
		t.Fatalf("operatorchat projection.Start() error = %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		projection.Shutdown(ctx)
	})

	profiles := chatoverlaydomain.NewService(sqlite.NewChatOverlayRepository(db.DB), nil)
	visualDesigns := visualdesign.NewService(sqlite.NewVisualDesignRepository(db.DB), nil)
	resolver := &co.DefaultSettingsResolver{
		Profiles: profiles, AccountLabel: func(string) (string, bool) { return "", false }, VisualDesigns: visualDesigns,
	}
	runtime := co.NewManager(co.WrapOperatorChatSource(projection), resolver, visualDesigns, logger)
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("chat overlay runtime.Start() error = %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		runtime.Shutdown(ctx)
	})

	overlay, err := profiles.CreateProfile(context.Background(), "Management API Test Overlay")
	if err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}

	capabilities := sqlite.NewRemoteOverlayCapabilityRepository(db.DB)

	handler := NewRouter(Options{
		Logger: logger, StartedAt: time.Now(), Accounts: accounts,
		ChatOverlayProfiles: profiles, ChatOverlayRuntime: runtime,
		RemoteOverlay:                RemoteOverlayOptions{Enabled: true, CanonicalOrigin: overlayOrigin},
		RemoteOverlayResolver:        capabilities,
		RemoteOverlayCapabilities:    capabilities,
		RemoteOverlayOwners:          RemoteOverlayOwners{ChatOverlays: profiles},
		RemoteOverlayCanonicalOrigin: overlayOrigin,
	})

	return handler, overlay, capabilities
}

func TestRemoteOverlayManagementStatusReflectsDisabledByDefault(t *testing.T) {
	handler, overlay, _ := newRemoteOverlayManagementServer(t, "https://overlay.example.com")

	recorder := do(t, handler, http.MethodGet, "/api/remote-overlay/chat-overlay/"+overlay.PublicSlug+"/status", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", recorder.Code, recorder.Body.String())
	}
	var body remoteOverlayStatusResponse
	decodeBody(t, recorder, &body)
	if body.Enabled {
		t.Error("Enabled = true before any capability was ever issued, want false")
	}
	if !body.Available {
		t.Error("Available = false with a configured overlay origin, want true")
	}
	if body.URL != "" {
		t.Errorf("URL = %q, want empty while disabled", body.URL)
	}
}

func TestRemoteOverlayManagementStatusReturnsTheCurrentURLOnceEnabled(t *testing.T) {
	handler, overlay, capabilities := newRemoteOverlayManagementServer(t, "https://overlay.example.com")
	ctx := context.Background()

	cap, err := capabilities.Issue(ctx, "chat-overlay", overlay.PublicSlug)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	recorder := do(t, handler, http.MethodGet, "/api/remote-overlay/chat-overlay/"+overlay.PublicSlug+"/status", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store when the response carries a live URL", got)
	}
	var body remoteOverlayStatusResponse
	decodeBody(t, recorder, &body)
	want := "https://overlay.example.com/overlay/chat/" + cap.Token
	if body.URL != want {
		t.Errorf("URL = %q, want %q", body.URL, want)
	}
}

func TestRemoteOverlayManagementEnableReturnsAWorkingURL(t *testing.T) {
	handler, overlay, capabilities := newRemoteOverlayManagementServer(t, "https://overlay.example.com")

	recorder := do(t, handler, http.MethodPost, "/api/remote-overlay/chat-overlay/"+overlay.PublicSlug+"/enable", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	var body remoteOverlayURLResponse
	decodeBody(t, recorder, &body)

	wantPrefix := "https://overlay.example.com/overlay/chat/"
	if !strings.HasPrefix(body.URL, wantPrefix) {
		t.Fatalf("url = %q, want prefix %q", body.URL, wantPrefix)
	}
	token := strings.TrimPrefix(body.URL, wantPrefix)

	// The returned URL's own token must actually resolve through the
	// public overlay routes - proves enable's URL construction and
	// the capability it issued are the same value, not two
	// independently-computed things that happen to look similar.
	_, ok, err := capabilities.Resolve(context.Background(), "chat-overlay", token)
	if err != nil || !ok {
		t.Errorf("Resolve(%q) = ok:%v err:%v, want ok:true", token, ok, err)
	}
}

func TestRemoteOverlayManagementEnableRejectsAFabricatedSlug(t *testing.T) {
	handler, _, _ := newRemoteOverlayManagementServer(t, "https://overlay.example.com")

	recorder := do(t, handler, http.MethodPost, "/api/remote-overlay/chat-overlay/does-not-exist/enable", nil)
	if recorder.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for a fabricated local slug", recorder.Code)
	}
}

func TestRemoteOverlayManagementEnableRejectsAnUnknownDomain(t *testing.T) {
	handler, overlay, _ := newRemoteOverlayManagementServer(t, "https://overlay.example.com")

	recorder := do(t, handler, http.MethodPost, "/api/remote-overlay/not-a-real-domain/"+overlay.PublicSlug+"/enable", nil)
	if recorder.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for an unrecognized domain", recorder.Code)
	}
}

func TestRemoteOverlayManagementRotateInvalidatesThePreviousURL(t *testing.T) {
	handler, overlay, capabilities := newRemoteOverlayManagementServer(t, "https://overlay.example.com")
	ctx := context.Background()

	first, err := capabilities.Issue(ctx, "chat-overlay", overlay.PublicSlug)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	recorder := do(t, handler, http.MethodPost, "/api/remote-overlay/chat-overlay/"+overlay.PublicSlug+"/rotate", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", recorder.Code, recorder.Body.String())
	}
	var body remoteOverlayURLResponse
	decodeBody(t, recorder, &body)
	if strings.Contains(body.URL, first.Token) {
		t.Error("rotate returned the same token as before")
	}

	_, oldStillWorks, _ := capabilities.Resolve(ctx, "chat-overlay", first.Token)
	if oldStillWorks {
		t.Error("the old token still resolves after rotate, want it invalidated immediately")
	}
}

func TestRemoteOverlayManagementDisableRevokesTheCapability(t *testing.T) {
	handler, overlay, capabilities := newRemoteOverlayManagementServer(t, "https://overlay.example.com")
	ctx := context.Background()

	cap, err := capabilities.Issue(ctx, "chat-overlay", overlay.PublicSlug)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	recorder := do(t, handler, http.MethodPost, "/api/remote-overlay/chat-overlay/"+overlay.PublicSlug+"/disable", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", recorder.Code, recorder.Body.String())
	}

	_, stillWorks, _ := capabilities.Resolve(ctx, "chat-overlay", cap.Token)
	if stillWorks {
		t.Error("the token still resolves after disable")
	}

	statusRecorder := do(t, handler, http.MethodGet, "/api/remote-overlay/chat-overlay/"+overlay.PublicSlug+"/status", nil)
	var status remoteOverlayStatusResponse
	decodeBody(t, statusRecorder, &status)
	if status.Enabled {
		t.Error("status still reports Enabled = true after disable")
	}
}

func TestRemoteOverlayManagementRoutesRejectWrongMethod(t *testing.T) {
	handler, overlay, _ := newRemoteOverlayManagementServer(t, "https://overlay.example.com")

	recorder := do(t, handler, http.MethodPost, "/api/remote-overlay/chat-overlay/"+overlay.PublicSlug+"/status", nil)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", recorder.Code)
	}
}

func TestRemoteOverlayManagementRoutesNotRegisteredWhenCapabilitiesIsNil(t *testing.T) {
	handler := NewRouter(Options{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), StartedAt: time.Now(),
	})

	recorder := do(t, handler, http.MethodGet, "/api/remote-overlay/chat-overlay/anything/status", nil)
	if recorder.Code == http.StatusOK {
		t.Error("status = 200 with RemoteOverlayCapabilities nil, want not-registered")
	}
}

func TestRemoteOverlayManagementEnableFailsClosedWithNoConfiguredOrigin(t *testing.T) {
	// A deployment where the management routes exist (an operator
	// could call status) but no overlay origin is configured yet -
	// enable must fail closed with a clear conflict, never construct
	// a URL from an empty origin.
	handler, overlay, _ := newRemoteOverlayManagementServer(t, "")

	recorder := do(t, handler, http.MethodPost, "/api/remote-overlay/chat-overlay/"+overlay.PublicSlug+"/enable", nil)
	if recorder.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409 when no overlay origin is configured", recorder.Code)
	}
}
