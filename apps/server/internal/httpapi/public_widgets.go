package httpapi

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	domain "github.com/streaming-tree/server/internal/domain/goals"
	"github.com/streaming-tree/server/internal/supporterwidgets"
)

const (
	maxWidgetSSEClientsPerSlug = 8
	widgetSSEKeepalive         = 15 * time.Second
	// widgetPollInterval is how often the public stream re-checks the
	// widget profile (and, for an event-derived kind, its own runtime
	// projection) for a change - see docs/supporter-widgets.md §10's own
	// "implementation note": the stream only ever carries the current
	// snapshot, never a delta sequence, so there is nothing a push
	// architecture would let a client "catch up on" that a fresh poll
	// does not already provide. Kept identical for every Stage 18B kind,
	// including dashboard - a deliberate decision, not an oversight (see
	// docs/supporter-widgets.md §10's own reasoning for not adding a
	// push/broadcast notifier).
	widgetPollInterval = 1500 * time.Millisecond
)

// registerPublicWidgetRoutes wires the Stage 18A/18B public,
// unauthenticated widget API (docs/goals-widgets.md §19-§20, docs/
// supporter-widgets.md §10). runtime may be nil in a test that never
// exercises a Stage 18B kind - every such kind then renders its own
// well-defined empty state.
func registerPublicWidgetRoutes(mux *http.ServeMux, logger *slog.Logger, svc GoalsService, runtime SupporterWidgetsRuntime) {
	limiter := newWidgetStreamLimiter()

	mux.HandleFunc("GET /api/public/widgets/{slug}/config", handleGetPublicWidgetConfig(logger, svc, runtime))
	mux.HandleFunc("/api/public/widgets/{slug}/config", methodNotAllowed(logger, http.MethodGet))

	mux.HandleFunc("GET /api/public/widgets/{slug}/stream", handlePublicWidgetStream(logger, svc, runtime, limiter))
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

// resolvePublicWidget resolves slug to its widget profile, or ok=false
// for an unknown/disabled slug (never distinguished from each other in
// the response - see handleGetPublicWidgetConfig's own "never a hard
// error" convention, mirroring resolvePublicAlertProfile exactly).
func resolvePublicWidget(ctx context.Context, svc GoalsService, slug string) (domain.WidgetProfile, bool) {
	p, err := svc.GetWidgetProfileByPublicSlug(ctx, slug)
	if err != nil || !p.Enabled {
		return domain.WidgetProfile{}, false
	}
	return p, true
}

// --- public/runtime-status item DTOs (docs/supporter-widgets.md §9-§12) --

// publicSupporterItemDTO is one presentation-safe row - never a
// providerEventId, account/source id, provider user id, or any other
// internal identifier (docs/supporter-widgets.md §12). Reused unchanged
// by the private runtime-status endpoint, since it already carries
// nothing more sensitive than the public route would eventually show.
type publicSupporterItemDTO struct {
	ItemID       string `json:"itemId"`
	DisplayName  string `json:"displayName,omitempty"`
	Provider     string `json:"provider,omitempty"`
	AmountMicros int64  `json:"amountMicros,omitempty"`
	Currency     string `json:"currency,omitempty"`
	Quantity     int64  `json:"quantity,omitempty"`
	Message      string `json:"message,omitempty"`
	ObservedAt   string `json:"observedAt"`
}

func toPublicSupporterItemDTO(item supporterwidgets.SupporterItem) publicSupporterItemDTO {
	return publicSupporterItemDTO{
		ItemID: item.ItemID, DisplayName: item.DisplayName, Provider: item.Provider,
		AmountMicros: item.AmountMicros, Currency: item.Currency, Quantity: item.Quantity,
		Message: item.Message, ObservedAt: item.ObservedAt.UTC().Format(time.RFC3339Nano),
	}
}

type publicTickerItemDTO struct {
	publicSupporterItemDTO
	EventType string `json:"eventType"`
}

func toPublicTickerItemDTO(item supporterwidgets.TickerItem) publicTickerItemDTO {
	return publicTickerItemDTO{publicSupporterItemDTO: toPublicSupporterItemDTO(item.SupporterItem), EventType: item.EventType}
}

// --- runtime-status (private/management, docs/supporter-widgets.md §19) --

type runtimeStatusResponse struct {
	Kind    string                   `json:"kind"`
	Latest  *publicSupporterItemDTO  `json:"latest,omitempty"`
	Largest *publicSupporterItemDTO  `json:"largest,omitempty"`
	Recent  []publicSupporterItemDTO `json:"recent,omitempty"`
	Ticker  []publicTickerItemDTO    `json:"ticker,omitempty"`
	Counter *int64                   `json:"counter,omitempty"`
}

func toRuntimeStatusResponse(kind domain.WidgetProfileKind, proj supporterwidgets.Projection) runtimeStatusResponse {
	resp := runtimeStatusResponse{Kind: string(kind)}
	applyProjectionToDTO(kind, proj, &resp.Latest, &resp.Largest, &resp.Recent, &resp.Ticker, &resp.Counter)
	return resp
}

// applyProjectionToDTO fills exactly the one field kind's own runtime
// state uses - shared by both the runtime-status and public snapshot
// builders so the two can never drift.
func applyProjectionToDTO(
	kind domain.WidgetProfileKind, proj supporterwidgets.Projection,
	latest, largest **publicSupporterItemDTO, recent *[]publicSupporterItemDTO, ticker *[]publicTickerItemDTO, counter **int64,
) {
	switch kind {
	case domain.WidgetProfileKindLatestFollower, domain.WidgetProfileKindLatestSubscriber, domain.WidgetProfileKindLatestDonation:
		if proj.Latest != nil {
			v := toPublicSupporterItemDTO(*proj.Latest)
			*latest = &v
		}
	case domain.WidgetProfileKindLargestDonation:
		if proj.Largest != nil {
			v := toPublicSupporterItemDTO(*proj.Largest)
			*largest = &v
		}
	case domain.WidgetProfileKindRecentSupporters:
		out := make([]publicSupporterItemDTO, 0, len(proj.Recent))
		for _, it := range proj.Recent {
			out = append(out, toPublicSupporterItemDTO(it))
		}
		*recent = out
	case domain.WidgetProfileKindEventTicker:
		out := make([]publicTickerItemDTO, 0, len(proj.Ticker))
		for _, it := range proj.Ticker {
			out = append(out, toPublicTickerItemDTO(it))
		}
		*ticker = out
	case domain.WidgetProfileKindSessionCounter:
		v := proj.Counter
		*counter = &v
	}
}

// --- public snapshot (docs/supporter-widgets.md §10, §20) ----------------

// publicWidgetSnapshotResponse is the only shape ever sent to a public
// widget viewer. The Stage 18A goal fields (GoalKind through Completed)
// are byte-for-byte unchanged from before this stage - an existing goal
// widget's own response never grows a single new field. Every Stage 18B
// field is a pointer/slice with omitempty, so a goal snapshot's JSON is
// identical to what it always was.
type publicWidgetSnapshotResponse struct {
	Revision uint64 `json:"revision"`
	Kind     string `json:"kind"`
	Title    string `json:"title"`

	// --- goal (Stage 18A, unchanged - no field here gained omitempty,
	// so an existing goal widget's own JSON shape is byte-for-byte
	// identical to before this stage) ---
	GoalKind            string `json:"goalKind"`
	Currency            string `json:"currency,omitempty"`
	Current             int64  `json:"current"`
	Target              int64  `json:"target"`
	ProgressBasisPoints int64  `json:"progressBasisPoints"`
	Completed           bool   `json:"completed"`

	// --- Stage 18B ---
	Latest    *publicSupporterItemDTO   `json:"latest,omitempty"`
	Largest   *publicSupporterItemDTO   `json:"largest,omitempty"`
	Recent    []publicSupporterItemDTO  `json:"recent,omitempty"`
	Ticker    []publicTickerItemDTO     `json:"ticker,omitempty"`
	Counter   *int64                    `json:"counter,omitempty"`
	Dashboard []publicDashboardChildDTO `json:"dashboard,omitempty"`

	Presentation publicWidgetPresentationDTO `json:"presentation"`
}

// publicWidgetPresentationDTO: ShowCurrent/ShowTarget/ShowPercent keep
// their original non-omitempty tags (Stage 18A) so an existing goal
// widget's own JSON is unchanged even when one is false. ShowProvider/
// ShowTime/Columns are new Stage 18B fields, meaningless (and always
// their zero value) for a goal widget, so omitempty is correct there.
type publicWidgetPresentationDTO struct {
	ShowCurrent     bool    `json:"showCurrent"`
	ShowTarget      bool    `json:"showTarget"`
	ShowPercent     bool    `json:"showPercent"`
	ShowProvider    bool    `json:"showProvider,omitempty"`
	ShowTime        bool    `json:"showTime,omitempty"`
	Columns         int     `json:"columns,omitempty"`
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

func toPublicWidgetPresentationDTO(p domain.WidgetProfile) publicWidgetPresentationDTO {
	return publicWidgetPresentationDTO{
		ShowCurrent: p.ShowCurrent, ShowTarget: p.ShowTarget, ShowPercent: p.ShowPercent,
		ShowProvider: p.ShowProvider, ShowTime: p.ShowTime, Columns: p.Columns,
		Orientation: string(p.Orientation), TextAlign: string(p.TextAlign), FontFamily: string(p.FontFamily),
		BackgroundColor: p.BackgroundColor, ForegroundColor: p.ForegroundColor, FillColor: p.FillColor, BorderColor: p.BorderColor,
		BorderRadiusPx: p.BorderRadiusPx, Opacity: p.Opacity,
	}
}

// publicDashboardChildDTO is one dashboard child's own composed
// presentation snapshot (docs/supporter-widgets.md §9, §27) - Key is a
// synthetic, position-based, non-identifying string, never the child's
// real internal widget-profile id.
type publicDashboardChildDTO struct {
	Key        string                       `json:"key"`
	Column     int                          `json:"column"`
	ColumnSpan int                          `json:"columnSpan"`
	Row        int                          `json:"row"`
	RowSpan    int                          `json:"rowSpan"`
	Snapshot   publicWidgetSnapshotResponse `json:"snapshot"`
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

// buildPublicSnapshot builds p's own full public snapshot - resolving a
// goal, reading the runtime projection, or (for a dashboard) composing
// every child's own snapshot in turn. A dashboard child can never itself
// be a dashboard (enforced at validation/save time - domain/goals.
// Service.validateWidgetProfileRefs), so this never recurses more than
// one level deep.
func buildPublicSnapshot(ctx context.Context, svc GoalsService, runtime SupporterWidgetsRuntime, revision uint64, p domain.WidgetProfile) publicWidgetSnapshotResponse {
	resp := publicWidgetSnapshotResponse{Revision: revision, Kind: string(p.Kind), Presentation: toPublicWidgetPresentationDTO(p)}

	switch {
	case p.Kind.RequiresGoal():
		g, err := svc.GetGoal(ctx, p.GoalID)
		if err != nil {
			return defaultPublicWidgetSnapshot()
		}
		title := p.TitleOverride
		if title == "" {
			title = g.Name
		}
		resp.Title = title
		resp.GoalKind = string(g.Kind)
		resp.Currency = g.Currency
		resp.Current = g.Current
		resp.Target = g.Target
		resp.ProgressBasisPoints = g.ProgressBasisPoints()
		resp.Completed = g.Completed()

	case p.Kind.IsDashboard():
		resp.Title = titleOrName(p)
		children := make([]publicDashboardChildDTO, 0, len(p.Children))
		for i, c := range p.Children {
			child, err := svc.GetWidgetProfile(ctx, c.WidgetProfileID)
			if err != nil || !child.Enabled {
				continue
			}
			children = append(children, publicDashboardChildDTO{
				Key: fmt.Sprintf("dashboard_child_%d", i), Column: c.Column, ColumnSpan: c.ColumnSpan, Row: c.Row, RowSpan: c.RowSpan,
				Snapshot: buildPublicSnapshot(ctx, svc, runtime, revision, child),
			})
		}
		resp.Dashboard = children

	default:
		resp.Title = titleOrName(p)
		// The widget's own configured comparison currency (largest_
		// donation always; session_counter only for its own money
		// metric) is presentation-relevant even before any matching
		// event has been observed yet - e.g. "No matching donation
		// observed yet (USD)" - so it is surfaced here regardless of
		// whether Latest/Largest/Counter has any real content yet.
		resp.Currency = p.Currency
		var proj supporterwidgets.Projection
		if runtime != nil {
			proj = runtime.Snapshot(p.ID)
		}
		applyProjectionToDTO(p.Kind, proj, &resp.Latest, &resp.Largest, &resp.Recent, &resp.Ticker, &resp.Counter)
	}
	return resp
}

func titleOrName(p domain.WidgetProfile) string {
	if p.TitleOverride != "" {
		return p.TitleOverride
	}
	return p.Name
}

// widgetFingerprint is a cheap, sufficient way to detect "the snapshot
// would render differently now" without a real revision counter -
// mirrors the Stage 18A original exactly for a goal widget, and extends
// the same idea to every Stage 18B kind (its own UpdatedAt plus its own
// runtime Revision) and to a dashboard (its own UpdatedAt plus every
// child's own fingerprint, one level deep - never recursing into a
// grandchild, since a dashboard can never contain another dashboard).
func widgetFingerprint(ctx context.Context, svc GoalsService, runtime SupporterWidgetsRuntime, p domain.WidgetProfile) string {
	switch {
	case p.Kind.RequiresGoal():
		g, err := svc.GetGoal(ctx, p.GoalID)
		if err != nil {
			return p.UpdatedAt.String() + "|missing"
		}
		return p.UpdatedAt.String() + "|" + g.UpdatedAt.String() + "|" + g.Currency
	case p.Kind.IsDashboard():
		fp := p.UpdatedAt.String()
		for _, c := range p.Children {
			child, err := svc.GetWidgetProfile(ctx, c.WidgetProfileID)
			if err != nil {
				continue
			}
			fp += "|" + widgetFingerprint(ctx, svc, runtime, child)
		}
		return fp
	default:
		var proj supporterwidgets.Projection
		if runtime != nil {
			proj = runtime.Snapshot(p.ID)
		}
		return fmt.Sprintf("%s|%d|%s", p.UpdatedAt.String(), proj.Revision, proj.UpdatedAt.String())
	}
}

func handleGetPublicWidgetConfig(logger *slog.Logger, svc GoalsService, runtime SupporterWidgetsRuntime) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := resolvePublicWidget(r.Context(), svc, r.PathValue("slug"))
		if !ok {
			writeJSON(w, logger, http.StatusOK, defaultPublicWidgetSnapshot())
			return
		}
		writeJSON(w, logger, http.StatusOK, buildPublicSnapshot(r.Context(), svc, runtime, 1, p))
	}
}

// handlePublicWidgetStream serves GET /api/public/widgets/{slug}/stream
// over Server-Sent Events - see docs/goals-widgets.md §19/docs/
// supporter-widgets.md §10's own "implementation note": one widget.reset
// on connect, another whenever a poll detects a real change, periodic
// keepalive, a bounded client count. An unknown or disabled slug never
// answers with a hard HTTP error (mirrors handlePublicAlertStream/
// handlePublicChatOverlayStream's identical convention) - it opens a
// normal 200 SSE connection, sends one safe/empty reset, and then idles
// on keepalives only.
func handlePublicWidgetStream(logger *slog.Logger, svc GoalsService, runtime SupporterWidgetsRuntime, limiter *widgetStreamLimiter) http.HandlerFunc {
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

		p, resolvedOK := resolvePublicWidget(r.Context(), svc, slug)
		if resolvedOK {
			_ = writeSSEEvent(w, "widget.reset", revision, buildPublicSnapshot(r.Context(), svc, runtime, revision, p))
			lastFingerprint = widgetFingerprint(r.Context(), svc, runtime, p)
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
				p, resolvedOK := resolvePublicWidget(ctx, svc, slug)
				if !resolvedOK {
					continue
				}
				fp := widgetFingerprint(ctx, svc, runtime, p)
				if fp == lastFingerprint {
					continue
				}
				lastFingerprint = fp
				revision++
				if err := writeSSEEvent(w, "widget.reset", revision, buildPublicSnapshot(ctx, svc, runtime, revision, p)); err != nil {
					logger.Warn("failed to write widget SSE event", "error", err)
					return
				}
				flusher.Flush()
			}
		}
	}
}
