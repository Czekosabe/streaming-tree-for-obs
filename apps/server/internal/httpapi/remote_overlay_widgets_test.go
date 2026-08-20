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

	domain "github.com/streaming-tree/server/internal/domain/goals"
	"github.com/streaming-tree/server/internal/domain/remoteoverlay"
	bus "github.com/streaming-tree/server/internal/engagement"
	"github.com/streaming-tree/server/internal/storage/sqlite"
	"github.com/streaming-tree/server/internal/supporterwidgets"
)

// End-to-end proof that BOTH widget families the shared widget domain
// serves resolve through a remote capability token identically: a
// Stage 18A goal widget (WidgetProfileKindGoal) and a Stage 18B
// event-derived widget (WidgetProfileKindLatestFollower) - both are
// domain.WidgetProfile records reached through the exact same
// resolvePublicWidget helper, so this proves the fix in the previous
// commit was never accidentally goal-only or supporter-only.

func newRemoteOverlayWidgetsServer(t *testing.T, overlayOrigin string) (http.Handler, *domain.Service, *sqlite.RemoteOverlayCapabilityRepository) {
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

	svc := domain.NewService(sqlite.NewGoalsRepository(db.DB), alwaysExistsLookup{}, nil)
	eventBus := bus.New(bus.Options{})
	t.Cleanup(eventBus.Shutdown)
	runtime := supporterwidgets.NewManager(supporterwidgets.ManagerOptions{Profiles: svc, Bus: eventBus})
	if err := runtime.Start(context.Background()); err != nil {
		t.Fatalf("runtime.Start() error = %v", err)
	}
	waitUntilTrue(t, time.Second, runtime.Subscribed)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = runtime.Shutdown(ctx)
	})

	capabilities := sqlite.NewRemoteOverlayCapabilityRepository(db.DB)

	handler := NewRouter(Options{
		Logger: logger, StartedAt: time.Now(), Goals: svc, SupporterWidgets: runtime,
		RemoteOverlay:         RemoteOverlayOptions{Enabled: true, CanonicalOrigin: overlayOrigin},
		RemoteOverlayResolver: capabilities,
	})

	return handler, svc, capabilities
}

func defaultWidgetPresentation() domain.WidgetProfile {
	return domain.WidgetProfile{
		Enabled: true, Orientation: domain.OrientationHorizontal, TextAlign: domain.AlignCenter, FontFamily: domain.FontSansSerif,
		BackgroundColor: "#00000080", ForegroundColor: "#ffffff", FillColor: "#7c3aed", BorderColor: "#ffffff33",
		BorderRadiusPx: domain.DefaultBorderRadiusPx, Opacity: 1.0,
	}
}

func TestRemoteOverlayStage18AGoalWidgetResolvesThroughACapabilityToken(t *testing.T) {
	handler, svc, capabilities := newRemoteOverlayWidgetsServer(t, "https://overlay.example.com")
	ctx := context.Background()

	goal, err := svc.CreateGoal(ctx, domain.Goal{Name: "Followers", Kind: domain.KindFollowers, Target: 1000, Baseline: 100})
	if err != nil {
		t.Fatalf("CreateGoal() error = %v", err)
	}

	draft := defaultWidgetPresentation()
	draft.Name = "Follower Goal Widget"
	draft.Kind = domain.WidgetProfileKindGoal
	draft.GoalID = goal.ID
	widget, err := svc.CreateWidgetProfile(ctx, draft)
	if err != nil {
		t.Fatalf("CreateWidgetProfile(goal) error = %v", err)
	}

	cap, err := capabilities.Issue(ctx, remoteoverlay.DomainWidget, widget.PublicSlug)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	req := forwardedOverlayRequest(http.MethodGet, "/api/public/widgets/"+cap.Token+"/config", "overlay.example.com")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", recorder.Code, recorder.Body.String())
	}
	var body publicWidgetSnapshotResponse
	decodeBody(t, recorder, &body)
	if body.Kind != "goal" {
		t.Errorf("kind = %q, want %q - resolved to the wrong/default widget", body.Kind, "goal")
	}
	if body.Target != 1000 {
		t.Errorf("target = %d, want 1000 - the real goal widget's own data must be returned, not the safe default", body.Target)
	}

	// The legacy local publicSlug must not work remotely.
	req2 := forwardedOverlayRequest(http.MethodGet, "/api/public/widgets/"+widget.PublicSlug+"/config", "overlay.example.com")
	recorder2 := httptest.NewRecorder()
	handler.ServeHTTP(recorder2, req2)
	var defaultBody publicWidgetSnapshotResponse
	decodeBody(t, recorder2, &defaultBody)
	if defaultBody.Target == 1000 {
		t.Error("the legacy local publicSlug resolved the real goal widget remotely - it must fall back to the safe default instead")
	}
}

func TestRemoteOverlayStage18BSupporterWidgetResolvesThroughACapabilityToken(t *testing.T) {
	handler, svc, capabilities := newRemoteOverlayWidgetsServer(t, "https://overlay.example.com")
	ctx := context.Background()

	draft := defaultWidgetPresentation()
	draft.Name = "Latest Follower"
	draft.Kind = domain.WidgetProfileKindLatestFollower
	widget, err := svc.CreateWidgetProfile(ctx, draft)
	if err != nil {
		t.Fatalf("CreateWidgetProfile(latest_follower) error = %v", err)
	}

	cap, err := capabilities.Issue(ctx, remoteoverlay.DomainWidget, widget.PublicSlug)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}

	req := forwardedOverlayRequest(http.MethodGet, "/api/public/widgets/"+cap.Token+"/config", "overlay.example.com")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", recorder.Code, recorder.Body.String())
	}
	var body publicWidgetSnapshotResponse
	decodeBody(t, recorder, &body)
	if body.Kind != "latest_follower" {
		t.Errorf("kind = %q, want %q", body.Kind, "latest_follower")
	}
}

func TestRemoteOverlayWidgetRevokedTokenFallsBackToDefault(t *testing.T) {
	handler, svc, capabilities := newRemoteOverlayWidgetsServer(t, "https://overlay.example.com")
	ctx := context.Background()

	draft := defaultWidgetPresentation()
	draft.Name = "Latest Follower"
	draft.Kind = domain.WidgetProfileKindLatestFollower
	widget, err := svc.CreateWidgetProfile(ctx, draft)
	if err != nil {
		t.Fatalf("CreateWidgetProfile() error = %v", err)
	}

	cap, err := capabilities.Issue(ctx, remoteoverlay.DomainWidget, widget.PublicSlug)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if err := capabilities.Revoke(ctx, remoteoverlay.DomainWidget, widget.PublicSlug); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}

	req := forwardedOverlayRequest(http.MethodGet, "/api/public/widgets/"+cap.Token+"/config", "overlay.example.com")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (this domain never hard-errors), body = %s", recorder.Code, recorder.Body.String())
	}
	var body publicWidgetSnapshotResponse
	decodeBody(t, recorder, &body)
	if body.Kind != "goal" {
		t.Errorf("kind = %q, want the safe default (%q) for a revoked token", body.Kind, "goal")
	}
}
