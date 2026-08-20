package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	co "github.com/streaming-tree/server/internal/chatoverlay"
	"github.com/streaming-tree/server/internal/domain/account"
	chatoverlaydomain "github.com/streaming-tree/server/internal/domain/chatoverlay"
	"github.com/streaming-tree/server/internal/domain/remoteoverlay"
	"github.com/streaming-tree/server/internal/domain/visualdesign"
	bus "github.com/streaming-tree/server/internal/engagement"
	oc "github.com/streaming-tree/server/internal/operatorchat"
	"github.com/streaming-tree/server/internal/secrets/secretstest"
	"github.com/streaming-tree/server/internal/storage/sqlite"
)

// End-to-end proof that a remote overlay capability token, presented
// through a request confirmed forwarded via the overlay origin,
// resolves to the correct chat-overlay profile (docs/remote-ingest.md
// §11/§12) - and that the legacy local publicSlug does not grant
// remote access on its own, even though it would still work over a
// direct loopback request.

func newRemoteOverlayChatServer(t *testing.T, overlayOrigin string) (http.Handler, chatoverlaydomain.Profile, *sqlite.RemoteOverlayCapabilityRepository) {
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

	overlay, err := profiles.CreateProfile(context.Background(), "Remote Test Overlay")
	if err != nil {
		t.Fatalf("CreateProfile() error = %v", err)
	}

	capabilities := sqlite.NewRemoteOverlayCapabilityRepository(db.DB)

	handler := NewRouter(Options{
		Logger: logger, StartedAt: time.Now(), Accounts: accounts,
		ChatOverlayProfiles: profiles, ChatOverlayRuntime: runtime,
		RemoteOverlay:         RemoteOverlayOptions{Enabled: true, CanonicalOrigin: overlayOrigin},
		RemoteOverlayResolver: capabilities,
	})

	return handler, overlay, capabilities
}

func forwardedOverlayRequest(method, target, overlayHost string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	req.RemoteAddr = "127.0.0.1:54321"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", overlayHost)
	return req
}

func TestRemoteOverlayChatOverlayCapabilityTokenResolvesThroughTheOverlayOrigin(t *testing.T) {
	handler, overlay, capabilities := newRemoteOverlayChatServer(t, "https://overlay.example.com")

	cap, err := capabilities.Issue(context.Background(), remoteoverlay.DomainChatOverlay, overlay.PublicSlug)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	req := forwardedOverlayRequest(http.MethodGet, "/api/public/chat-overlays/"+cap.Token+"/config", "overlay.example.com")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestRemoteOverlayChatOverlayLegacyLocalSlugDoesNotResolveRemotely(t *testing.T) {
	handler, overlay, _ := newRemoteOverlayChatServer(t, "https://overlay.example.com")

	// The overlay's own real local publicSlug, presented as if it were
	// a remote capability token - it must NOT work, even though it is
	// a real, valid, high-entropy value. Remote access requires an
	// explicitly-issued capability, never the legacy local slug.
	req := forwardedOverlayRequest(http.MethodGet, "/api/public/chat-overlays/"+overlay.PublicSlug+"/config", "overlay.example.com")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 - the legacy local publicSlug must not grant remote access", recorder.Code)
	}
}

func TestRemoteOverlayChatOverlayDirectLoopbackStillUsesTheLocalSlug(t *testing.T) {
	handler, overlay, capabilities := newRemoteOverlayChatServer(t, "https://overlay.example.com")

	// Issue a remote capability too, to prove its existence does not
	// change direct-loopback behavior at all.
	if _, err := capabilities.Issue(context.Background(), remoteoverlay.DomainChatOverlay, overlay.PublicSlug); err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	recorder := do(t, handler, http.MethodGet, "/api/public/chat-overlays/"+overlay.PublicSlug+"/config", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for a direct loopback request using the real local slug, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestRemoteOverlayChatOverlayRevokedTokenNoLongerResolves(t *testing.T) {
	handler, overlay, capabilities := newRemoteOverlayChatServer(t, "https://overlay.example.com")
	ctx := context.Background()

	cap, err := capabilities.Issue(ctx, remoteoverlay.DomainChatOverlay, overlay.PublicSlug)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if err := capabilities.Revoke(ctx, remoteoverlay.DomainChatOverlay, overlay.PublicSlug); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}

	req := forwardedOverlayRequest(http.MethodGet, "/api/public/chat-overlays/"+cap.Token+"/config", "overlay.example.com")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for a revoked token", recorder.Code)
	}
}

func TestRemoteOverlayChatOverlayRejectsTheManagementHostnameEvenWithAValidToken(t *testing.T) {
	handler, overlay, capabilities := newRemoteOverlayChatServer(t, "https://overlay.example.com")

	cap, err := capabilities.Issue(context.Background(), remoteoverlay.DomainChatOverlay, overlay.PublicSlug)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	// A valid token, but forwarded through the wrong (management)
	// hostname - withRemoteOverlaySecurity must reject this before the
	// handler ever runs, regardless of token validity.
	req := forwardedOverlayRequest(http.MethodGet, "/api/public/chat-overlays/"+cap.Token+"/config", "stream.example.com")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403 for the wrong forwarded hostname, even with a valid token", recorder.Code)
	}
}
