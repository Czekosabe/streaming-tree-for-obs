package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	domain "github.com/streaming-tree/server/internal/domain/goals"
)

const (
	maxWidgetSSEClientsPerSlug = 8
	widgetSSEKeepalive         = 15 * time.Second
	// widgetPollInterval is how often the public stream re-checks the
	// goal/widget profile for a change - see docs/goals-widgets.md §19's
	// own "implementation note" for why this is a simple poll rather
	// than a push-based ring buffer: the stream only ever carries the
	// current snapshot, never a delta sequence, so there is nothing a
	// push architecture would let a client "catch up on" that a fresh
	// poll does not already provide.
	widgetPollInterval = 1500 * time.Millisecond
)

// registerPublicWidgetRoutes wires the Stage 18A public, unauthenticated
// goal-widget API (docs/goals-widgets.md §19-§20).
func registerPublicWidgetRoutes(mux *http.ServeMux, logger *slog.Logger, svc GoalsService) {
	limiter := newWidgetStreamLimiter()

	mux.HandleFunc("GET /api/public/widgets/{slug}/config", handleGetPublicWidgetConfig(logger, svc))
	mux.HandleFunc("/api/public/widgets/{slug}/config", methodNotAllowed(logger, http.MethodGet))

	mux.HandleFunc("GET /api/public/widgets/{slug}/stream", handlePublicWidgetStream(logger, svc, limiter))
	mux.HandleFunc("/api/public/widgets/{slug}/stream", methodNotAllowed(logger, http.MethodGet))
}

// widgetStreamLimiter bounds live SSE clients per slug - independent per
// slug, so one widget reaching its cap never affects another's stream.
// Mirrors chatOverlayStreamLimiter/alertStreamLimiter exactly.
type widgetStreamLimiter struct {
	mu     sync.Mutex
	counts map[string]int
}

func newWidgetStreamLimiter() *widgetStreamLimiter {
	return &widgetStreamLimiter{counts: make(map[string]int)}
}

func (l *widgetStreamLimiter) acquire(slug string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.counts[slug] >= maxWidgetSSEClientsPerSlug {
		return false
	}
	l.counts[slug]++
	return true
}

func (l *widgetStreamLimiter) release(slug string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.counts[slug]--
	if l.counts[slug] <= 0 {
		delete(l.counts, slug)
	}
}

// resolvePublicWidget resolves slug to its widget profile and goal, or
// ok=false for an unknown/disabled slug or a goal that no longer exists
// (never distinguished from each other in the response - see
// handleGetPublicWidgetConfig's own "never a hard error" convention,
// mirroring resolvePublicAlertProfile exactly).
func resolvePublicWidget(ctx context.Context, svc GoalsService, slug string) (domain.WidgetProfile, domain.Goal, bool) {
	p, err := svc.GetWidgetProfileByPublicSlug(ctx, slug)
	if err != nil || !p.Enabled {
		return domain.WidgetProfile{}, domain.Goal{}, false
	}
	g, err := svc.GetGoal(ctx, p.GoalID)
	if err != nil {
		return domain.WidgetProfile{}, domain.Goal{}, false
	}
	return p, g, true
}

// publicWidgetSnapshotResponse is the only shape ever sent to a public
// widget viewer (docs/goals-widgets.md §20) - never providerEventId, any
// account/source id, any provider user id, dedupe-ledger content,
// database paths, applied-event history, or private user/donor data.
type publicWidgetSnapshotResponse struct {
	Revision            uint64                      `json:"revision"`
	Kind                string                      `json:"kind"`
	GoalKind            string                      `json:"goalKind"`
	Title               string                      `json:"title"`
	Currency            string                      `json:"currency,omitempty"`
	Current             int64                       `json:"current"`
	Target              int64                       `json:"target"`
	ProgressBasisPoints int64                       `json:"progressBasisPoints"`
	Completed           bool                        `json:"completed"`
	Presentation        publicWidgetPresentationDTO `json:"presentation"`
}

type publicWidgetPresentationDTO struct {
	ShowCurrent     bool    `json:"showCurrent"`
	ShowTarget      bool    `json:"showTarget"`
	ShowPercent     bool    `json:"showPercent"`
	Orientation     string  `json:"orientation"`
	TextAlign       string  `json:"textAlign"`
	FontFamily      string  `json:"fontFamily"`
	BackgroundColor string  `json:"backgroundColor"`
	ForegroundColor string  `json:"foregroundColor"`
	FillColor       string  `json:"fillColor"`
	BorderColor     string  `json:"borderColor"`
	BorderRadiusPx  int     `json:"borderRadiusPx"`
	Opacity         float64 `json:"opacity"`
}

// defaultPublicWidgetSnapshot is the safe, empty response for an
// unknown/disabled slug - never a hard HTTP error (mirrors
// handleGetPublicAlertProfileConfig's identical "safe, empty/default
// config" convention). Target is 1, never 0, so ProgressBasisPoints
// stays a well-defined 0 rather than implying a division by zero ever
// occurred.
func defaultPublicWidgetSnapshot() publicWidgetSnapshotResponse {
	return publicWidgetSnapshotResponse{
		Revision: 1, Kind: "goal", GoalKind: string(domain.KindFollowers), Target: 1,
		Presentation: publicWidgetPresentationDTO{
			ShowCurrent: true, ShowTarget: true, ShowPercent: true,
			Orientation: string(domain.OrientationHorizontal), TextAlign: string(domain.AlignCenter), FontFamily: string(domain.FontSansSerif),
			BackgroundColor: "#00000080", ForegroundColor: "#ffffff", FillColor: "#7c3aed", BorderColor: "#ffffff33",
			BorderRadiusPx: domain.DefaultBorderRadiusPx, Opacity: 1.0,
		},
	}
}

func toPublicWidgetSnapshot(revision uint64, p domain.WidgetProfile, g domain.Goal) publicWidgetSnapshotResponse {
	title := p.TitleOverride
	if title == "" {
		title = g.Name
	}
	return publicWidgetSnapshotResponse{
		Revision: revision, Kind: "goal", GoalKind: string(g.Kind), Title: title, Currency: g.Currency,
		Current: g.Current, Target: g.Target, ProgressBasisPoints: g.ProgressBasisPoints(), Completed: g.Completed(),
		Presentation: publicWidgetPresentationDTO{
			ShowCurrent: p.ShowCurrent, ShowTarget: p.ShowTarget, ShowPercent: p.ShowPercent,
			Orientation: string(p.Orientation), TextAlign: string(p.TextAlign), FontFamily: string(p.FontFamily),
			BackgroundColor: p.BackgroundColor, ForegroundColor: p.ForegroundColor, FillColor: p.FillColor, BorderColor: p.BorderColor,
			BorderRadiusPx: p.BorderRadiusPx, Opacity: p.Opacity,
		},
	}
}

func handleGetPublicWidgetConfig(logger *slog.Logger, svc GoalsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, g, ok := resolvePublicWidget(r.Context(), svc, r.PathValue("slug"))
		if !ok {
			writeJSON(w, logger, http.StatusOK, defaultPublicWidgetSnapshot())
			return
		}
		writeJSON(w, logger, http.StatusOK, toPublicWidgetSnapshot(1, p, g))
	}
}

// changeFingerprint is a cheap, sufficient way to detect "the snapshot
// would render differently now" without a real revision counter -
// concatenating every field the public DTO actually derives from.
func changeFingerprint(p domain.WidgetProfile, g domain.Goal) string {
	return p.UpdatedAt.String() + "|" + g.UpdatedAt.String() + "|" + g.Currency
}

// handlePublicWidgetStream serves GET /api/public/widgets/{slug}/stream
// over Server-Sent Events - see docs/goals-widgets.md §19's own
// "implementation note": one widget.reset on connect, another whenever
// a poll detects a real change, periodic keepalive, a bounded client
// count. An unknown or disabled slug never answers with a hard HTTP
// error (mirrors handlePublicAlertStream/handlePublicChatOverlayStream's
// identical convention) - it opens a normal 200 SSE connection, sends
// one safe/empty reset, and then idles on keepalives only.
func handlePublicWidgetStream(logger *slog.Logger, svc GoalsService, limiter *widgetStreamLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")

		flusher, ok := w.(http.Flusher)
		if !ok {
			writeError(w, logger, http.StatusInternalServerError, "internal_error", "Streaming is not supported.")
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)

		if !limiter.acquire(slug) {
			_ = writeSSEEvent(w, "widget.reset", 1, defaultPublicWidgetSnapshot())
			flusher.Flush()
			return
		}
		defer limiter.release(slug)

		var revision uint64 = 1
		lastFingerprint := ""

		p, g, resolvedOK := resolvePublicWidget(r.Context(), svc, slug)
		if resolvedOK {
			_ = writeSSEEvent(w, "widget.reset", revision, toPublicWidgetSnapshot(revision, p, g))
			lastFingerprint = changeFingerprint(p, g)
		} else {
			_ = writeSSEEvent(w, "widget.reset", revision, defaultPublicWidgetSnapshot())
		}
		flusher.Flush()

		poll := time.NewTicker(widgetPollInterval)
		defer poll.Stop()
		keepalive := time.NewTicker(widgetSSEKeepalive)
		defer keepalive.Stop()

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				return
			case <-keepalive.C:
				writeSSEComment(w, "keepalive")
				flusher.Flush()
			case <-poll.C:
				p, g, resolvedOK := resolvePublicWidget(ctx, svc, slug)
				if !resolvedOK {
					continue
				}
				fp := changeFingerprint(p, g)
				if fp == lastFingerprint {
					continue
				}
				lastFingerprint = fp
				revision++
				if err := writeSSEEvent(w, "widget.reset", revision, toPublicWidgetSnapshot(revision, p, g)); err != nil {
					logger.Warn("failed to write widget SSE event", "error", err)
					return
				}
				flusher.Flush()
			}
		}
	}
}
