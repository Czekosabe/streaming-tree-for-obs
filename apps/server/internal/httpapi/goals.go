package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	domain "github.com/streaming-tree/server/internal/domain/goals"
)

// maxGoalsBodyBytes caps goal/widget-profile request bodies.
const maxGoalsBodyBytes = 16 * 1024

// GoalsService is the subset of internal/domain/goals.Service the HTTP
// layer needs - satisfied directly by *domain/goals.Service, exactly
// like DonationSources/VisualAssets are satisfied by their own bare
// domain services with no separate runtime façade (docs/goals-
// widgets.md §24, §26: the runtime internal/goals.Manager has no HTTP
// surface of its own).
type GoalsService interface {
	CreateGoal(ctx context.Context, draft domain.Goal) (domain.Goal, error)
	GetGoal(ctx context.Context, id string) (domain.Goal, error)
	ListGoals(ctx context.Context) ([]domain.Goal, error)
	UpdateGoal(ctx context.Context, draft domain.Goal) (domain.Goal, error)
	DeleteGoal(ctx context.Context, id string) error
	SetCurrent(ctx context.Context, id string, current int64) (domain.Goal, error)
	ResetProgress(ctx context.Context, id string) (domain.Goal, error)

	CreateWidgetProfile(ctx context.Context, draft domain.WidgetProfile) (domain.WidgetProfile, error)
	GetWidgetProfile(ctx context.Context, id string) (domain.WidgetProfile, error)
	GetWidgetProfileByPublicSlug(ctx context.Context, slug string) (domain.WidgetProfile, error)
	ListWidgetProfiles(ctx context.Context, goalID string) ([]domain.WidgetProfile, error)
	UpdateWidgetProfile(ctx context.Context, draft domain.WidgetProfile) (domain.WidgetProfile, error)
	RotatePublicSlug(ctx context.Context, id string) (domain.WidgetProfile, error)
	DeleteWidgetProfile(ctx context.Context, id string) error
}

// registerGoalRoutes wires the Stage 18A goal/widget-profile management
// API (docs/goals-widgets.md §24). The public widget route is
// registered separately (see public_widgets.go).
func registerGoalRoutes(mux *http.ServeMux, logger *slog.Logger, svc GoalsService) {
	mux.HandleFunc("GET /api/goals", handleListGoals(logger, svc))
	mux.HandleFunc("POST /api/goals", handleCreateGoal(logger, svc))
	mux.HandleFunc("/api/goals", methodNotAllowed(logger, http.MethodGet, http.MethodPost))

	mux.HandleFunc("GET /api/goals/{id}", handleGetGoal(logger, svc))
	mux.HandleFunc("PUT /api/goals/{id}", handleUpdateGoal(logger, svc))
	mux.HandleFunc("DELETE /api/goals/{id}", handleDeleteGoal(logger, svc))
	mux.HandleFunc("/api/goals/{id}", methodNotAllowed(logger, http.MethodGet, http.MethodPut, http.MethodDelete))

	mux.HandleFunc("POST /api/goals/{id}/set-current", handleSetGoalCurrent(logger, svc))
	mux.HandleFunc("/api/goals/{id}/set-current", methodNotAllowed(logger, http.MethodPost))

	mux.HandleFunc("POST /api/goals/{id}/reset", handleResetGoal(logger, svc))
	mux.HandleFunc("/api/goals/{id}/reset", methodNotAllowed(logger, http.MethodPost))

	mux.HandleFunc("GET /api/widget-profiles", handleListWidgetProfiles(logger, svc))
	mux.HandleFunc("POST /api/widget-profiles", handleCreateWidgetProfile(logger, svc))
	mux.HandleFunc("/api/widget-profiles", methodNotAllowed(logger, http.MethodGet, http.MethodPost))

	mux.HandleFunc("GET /api/widget-profiles/{id}", handleGetWidgetProfile(logger, svc))
	mux.HandleFunc("PUT /api/widget-profiles/{id}", handleUpdateWidgetProfile(logger, svc))
	mux.HandleFunc("DELETE /api/widget-profiles/{id}", handleDeleteWidgetProfile(logger, svc))
	mux.HandleFunc("/api/widget-profiles/{id}", methodNotAllowed(logger, http.MethodGet, http.MethodPut, http.MethodDelete))

	mux.HandleFunc("POST /api/widget-profiles/{id}/rotate-public-slug", handleRotateWidgetProfileSlug(logger, svc))
	mux.HandleFunc("/api/widget-profiles/{id}/rotate-public-slug", methodNotAllowed(logger, http.MethodPost))
}

func writeGoalsError(w http.ResponseWriter, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, domain.ErrGoalNotFound):
		writeError(w, logger, http.StatusNotFound, "goal_not_found", "The requested goal does not exist.")
	case errors.Is(err, domain.ErrWidgetProfileNotFound):
		writeError(w, logger, http.StatusNotFound, "goal_widget_profile_not_found", "The requested widget profile does not exist.")
	case errors.Is(err, domain.ErrAccountNotFound):
		writeError(w, logger, http.StatusNotFound, "goal_account_not_found", "One of the target connected accounts or donation sources does not exist.")
	case errors.Is(err, domain.ErrGoalInUse):
		writeError(w, logger, http.StatusConflict, "goal_in_use", "This goal still has one or more widget profiles - delete them first.")
	case errors.Is(err, domain.ErrWidgetProfileInUse):
		writeError(w, logger, http.StatusConflict, "widget_profile_in_use", "This widget is still referenced by one or more dashboards - remove it from them first.")
	case errors.Is(err, domain.ErrConfigConflict):
		writeError(w, logger, http.StatusConflict, "goal_config_conflict", "Another save already changed this goal - reload and try again.")
	case errors.Is(err, domain.ErrValidation):
		writeError(w, logger, http.StatusUnprocessableEntity, "goal_invalid", "The request failed validation.")
	default:
		writeError(w, logger, http.StatusInternalServerError, "internal_error", "An unexpected error occurred.")
	}
}

// --- goal DTOs ---------------------------------------------------------

type goalRequest struct {
	Name           string   `json:"name"`
	Kind           string   `json:"kind"`
	Enabled        bool     `json:"enabled"`
	Target         int64    `json:"target"`
	Baseline       int64    `json:"baseline"`
	Currency       string   `json:"currency,omitempty"`
	Providers      []string `json:"providers"`
	Accounts       []string `json:"accounts"`
	ConfigRevision int64    `json:"configRevision"`
}

func (r goalRequest) toDomain(id string) domain.Goal {
	providers := make([]domain.ProviderID, 0, len(r.Providers))
	for _, p := range r.Providers {
		providers = append(providers, domain.ProviderID(p))
	}
	return domain.Goal{
		ID: id, Name: r.Name, Kind: domain.Kind(r.Kind), Enabled: r.Enabled,
		Target: r.Target, Baseline: r.Baseline, Currency: r.Currency,
		Providers: providers, Accounts: r.Accounts, ConfigRevision: r.ConfigRevision,
	}
}

type goalResponse struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Kind                string   `json:"kind"`
	Enabled             bool     `json:"enabled"`
	Target              int64    `json:"target"`
	Current             int64    `json:"current"`
	Baseline            int64    `json:"baseline"`
	Currency            string   `json:"currency,omitempty"`
	Providers           []string `json:"providers"`
	Accounts            []string `json:"accounts"`
	ProgressBasisPoints int64    `json:"progressBasisPoints"`
	Completed           bool     `json:"completed"`
	CreatedAt           string   `json:"createdAt"`
	UpdatedAt           string   `json:"updatedAt"`
	StartedAt           string   `json:"startedAt"`
	ConfigRevision      int64    `json:"configRevision"`
}

func toGoalResponse(g domain.Goal) goalResponse {
	providers := make([]string, 0, len(g.Providers))
	for _, p := range g.Providers {
		providers = append(providers, string(p))
	}
	accounts := g.Accounts
	if accounts == nil {
		accounts = []string{}
	}
	return goalResponse{
		ID: g.ID, Name: g.Name, Kind: string(g.Kind), Enabled: g.Enabled,
		Target: g.Target, Current: g.Current, Baseline: g.Baseline, Currency: g.Currency,
		Providers: providers, Accounts: accounts,
		ProgressBasisPoints: g.ProgressBasisPoints(), Completed: g.Completed(),
		CreatedAt: g.CreatedAt.UTC().Format(time.RFC3339Nano), UpdatedAt: g.UpdatedAt.UTC().Format(time.RFC3339Nano),
		StartedAt: g.StartedAt.UTC().Format(time.RFC3339Nano), ConfigRevision: g.ConfigRevision,
	}
}

func handleListGoals(logger *slog.Logger, svc GoalsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := svc.ListGoals(r.Context())
		if err != nil {
			writeGoalsError(w, logger, err)
			return
		}
		out := make([]goalResponse, 0, len(list))
		for _, g := range list {
			out = append(out, toGoalResponse(g))
		}
		writeJSON(w, logger, http.StatusOK, out)
	}
}

func handleCreateGoal(logger *slog.Logger, svc GoalsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body goalRequest
		if err := decodeJSONWithLimit(w, r, &body, maxGoalsBodyBytes); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		g, err := svc.CreateGoal(r.Context(), body.toDomain(""))
		if err != nil {
			writeGoalsError(w, logger, err)
			return
		}
		w.Header().Set("Location", "/api/goals/"+g.ID)
		writeJSON(w, logger, http.StatusCreated, toGoalResponse(g))
	}
}

func handleGetGoal(logger *slog.Logger, svc GoalsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		g, err := svc.GetGoal(r.Context(), r.PathValue("id"))
		if err != nil {
			writeGoalsError(w, logger, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toGoalResponse(g))
	}
}

func handleUpdateGoal(logger *slog.Logger, svc GoalsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var body goalRequest
		if err := decodeJSONWithLimit(w, r, &body, maxGoalsBodyBytes); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		g, err := svc.UpdateGoal(r.Context(), body.toDomain(id))
		if err != nil {
			writeGoalsError(w, logger, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toGoalResponse(g))
	}
}

func handleDeleteGoal(logger *slog.Logger, svc GoalsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := svc.DeleteGoal(r.Context(), r.PathValue("id")); err != nil {
			writeGoalsError(w, logger, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

type setGoalCurrentRequest struct {
	Current int64 `json:"current"`
}

func handleSetGoalCurrent(logger *slog.Logger, svc GoalsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var body setGoalCurrentRequest
		if err := decodeJSONWithLimit(w, r, &body, maxGoalsBodyBytes); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		g, err := svc.SetCurrent(r.Context(), id, body.Current)
		if err != nil {
			writeGoalsError(w, logger, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toGoalResponse(g))
	}
}

func handleResetGoal(logger *slog.Logger, svc GoalsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		g, err := svc.ResetProgress(r.Context(), r.PathValue("id"))
		if err != nil {
			writeGoalsError(w, logger, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toGoalResponse(g))
	}
}

// --- widget profile DTOs ------------------------------------------------

// dashboardChildRequest/dashboardChildResponse are one dashboard's own
// bounded grid placement of an existing widget profile (docs/supporter-
// widgets.md §9) - referenced by internal id in the management API
// (never in the public one - see public_widgets.go's own presentation-
// only child DTO).
type dashboardChildRequest struct {
	WidgetProfileID string `json:"widgetProfileId"`
	Column          int    `json:"column"`
	ColumnSpan      int    `json:"columnSpan"`
	Row             int    `json:"row"`
	RowSpan         int    `json:"rowSpan"`
}

func (r dashboardChildRequest) toDomain() domain.DashboardChild {
	return domain.DashboardChild{WidgetProfileID: r.WidgetProfileID, Column: r.Column, ColumnSpan: r.ColumnSpan, Row: r.Row, RowSpan: r.RowSpan}
}

type dashboardChildResponse struct {
	WidgetProfileID string `json:"widgetProfileId"`
	Column          int    `json:"column"`
	ColumnSpan      int    `json:"columnSpan"`
	Row             int    `json:"row"`
	RowSpan         int    `json:"rowSpan"`
}

func toDashboardChildResponse(c domain.DashboardChild) dashboardChildResponse {
	return dashboardChildResponse{WidgetProfileID: c.WidgetProfileID, Column: c.Column, ColumnSpan: c.ColumnSpan, Row: c.Row, RowSpan: c.RowSpan}
}

// widgetProfileRequest is kind-aware (docs/supporter-widgets.md §19):
// only the fields meaningful for the chosen Kind are ever validated as
// non-empty/non-zero by the backend - every other field must be left at
// its zero value or the request is rejected (see
// domain/goals.ValidateWidgetProfileFields).
type widgetProfileRequest struct {
	Kind            string                  `json:"kind"`
	GoalID          string                  `json:"goalId,omitempty"`
	Name            string                  `json:"name"`
	Enabled         bool                    `json:"enabled"`
	Providers       []string                `json:"providers,omitempty"`
	Accounts        []string                `json:"accounts,omitempty"`
	TitleOverride   string                  `json:"titleOverride,omitempty"`
	ShowCurrent     bool                    `json:"showCurrent"`
	ShowTarget      bool                    `json:"showTarget"`
	ShowPercent     bool                    `json:"showPercent"`
	ShowProvider    bool                    `json:"showProvider"`
	ShowTime        bool                    `json:"showTime"`
	ShowMessage     bool                    `json:"showMessage"`
	MaxItems        int                     `json:"maxItems,omitempty"`
	Currency        string                  `json:"currency,omitempty"`
	Metric          string                  `json:"metric,omitempty"`
	EventTypes      []string                `json:"eventTypes,omitempty"`
	Columns         int                     `json:"columns,omitempty"`
	Children        []dashboardChildRequest `json:"children,omitempty"`
	Orientation     string                  `json:"orientation"`
	TextAlign       string                  `json:"textAlign"`
	FontFamily      string                  `json:"fontFamily"`
	BackgroundColor string                  `json:"backgroundColor"`
	ForegroundColor string                  `json:"foregroundColor"`
	FillColor       string                  `json:"fillColor"`
	BorderColor     string                  `json:"borderColor"`
	BorderRadiusPx  int                     `json:"borderRadiusPx"`
	Opacity         float64                 `json:"opacity"`
}

func (r widgetProfileRequest) toDomain(id string) domain.WidgetProfile {
	providers := make([]domain.ProviderID, 0, len(r.Providers))
	for _, p := range r.Providers {
		providers = append(providers, domain.ProviderID(p))
	}
	eventTypes := make([]domain.SupporterEventType, 0, len(r.EventTypes))
	for _, t := range r.EventTypes {
		eventTypes = append(eventTypes, domain.SupporterEventType(t))
	}
	children := make([]domain.DashboardChild, 0, len(r.Children))
	for _, c := range r.Children {
		children = append(children, c.toDomain())
	}
	return domain.WidgetProfile{
		ID: id, Kind: domain.WidgetProfileKind(r.Kind), GoalID: r.GoalID, Name: r.Name, Enabled: r.Enabled,
		Providers: providers, Accounts: r.Accounts, TitleOverride: r.TitleOverride,
		ShowCurrent: r.ShowCurrent, ShowTarget: r.ShowTarget, ShowPercent: r.ShowPercent,
		ShowProvider: r.ShowProvider, ShowTime: r.ShowTime, ShowMessage: r.ShowMessage,
		MaxItems: r.MaxItems, Currency: r.Currency, Metric: domain.SessionMetric(r.Metric), EventTypes: eventTypes,
		Columns: r.Columns, Children: children,
		Orientation: domain.Orientation(r.Orientation), TextAlign: domain.TextAlign(r.TextAlign), FontFamily: domain.FontFamily(r.FontFamily),
		BackgroundColor: r.BackgroundColor, ForegroundColor: r.ForegroundColor, FillColor: r.FillColor, BorderColor: r.BorderColor,
		BorderRadiusPx: r.BorderRadiusPx, Opacity: r.Opacity,
	}
}

type widgetProfileResponse struct {
	ID              string                   `json:"id"`
	Kind            string                   `json:"kind"`
	GoalID          string                   `json:"goalId,omitempty"`
	Name            string                   `json:"name"`
	Enabled         bool                     `json:"enabled"`
	PublicSlug      string                   `json:"publicSlug"`
	Providers       []string                 `json:"providers,omitempty"`
	Accounts        []string                 `json:"accounts,omitempty"`
	TitleOverride   string                   `json:"titleOverride,omitempty"`
	ShowCurrent     bool                     `json:"showCurrent"`
	ShowTarget      bool                     `json:"showTarget"`
	ShowPercent     bool                     `json:"showPercent"`
	ShowProvider    bool                     `json:"showProvider"`
	ShowTime        bool                     `json:"showTime"`
	ShowMessage     bool                     `json:"showMessage"`
	MaxItems        int                      `json:"maxItems,omitempty"`
	Currency        string                   `json:"currency,omitempty"`
	Metric          string                   `json:"metric,omitempty"`
	EventTypes      []string                 `json:"eventTypes,omitempty"`
	Columns         int                      `json:"columns,omitempty"`
	Children        []dashboardChildResponse `json:"children,omitempty"`
	Orientation     string                   `json:"orientation"`
	TextAlign       string                   `json:"textAlign"`
	FontFamily      string                   `json:"fontFamily"`
	BackgroundColor string                   `json:"backgroundColor"`
	ForegroundColor string                   `json:"foregroundColor"`
	FillColor       string                   `json:"fillColor"`
	BorderColor     string                   `json:"borderColor"`
	BorderRadiusPx  int                      `json:"borderRadiusPx"`
	Opacity         float64                  `json:"opacity"`
	CreatedAt       string                   `json:"createdAt"`
	UpdatedAt       string                   `json:"updatedAt"`
}

func toWidgetProfileResponse(p domain.WidgetProfile) widgetProfileResponse {
	providers := make([]string, 0, len(p.Providers))
	for _, v := range p.Providers {
		providers = append(providers, string(v))
	}
	eventTypes := make([]string, 0, len(p.EventTypes))
	for _, v := range p.EventTypes {
		eventTypes = append(eventTypes, string(v))
	}
	children := make([]dashboardChildResponse, 0, len(p.Children))
	for _, c := range p.Children {
		children = append(children, toDashboardChildResponse(c))
	}
	return widgetProfileResponse{
		ID: p.ID, Kind: string(p.Kind), GoalID: p.GoalID, Name: p.Name, Enabled: p.Enabled, PublicSlug: p.PublicSlug,
		Providers: providers, Accounts: p.Accounts, TitleOverride: p.TitleOverride,
		ShowCurrent: p.ShowCurrent, ShowTarget: p.ShowTarget, ShowPercent: p.ShowPercent,
		ShowProvider: p.ShowProvider, ShowTime: p.ShowTime, ShowMessage: p.ShowMessage,
		MaxItems: p.MaxItems, Currency: p.Currency, Metric: string(p.Metric), EventTypes: eventTypes,
		Columns: p.Columns, Children: children,
		Orientation: string(p.Orientation), TextAlign: string(p.TextAlign), FontFamily: string(p.FontFamily),
		BackgroundColor: p.BackgroundColor, ForegroundColor: p.ForegroundColor, FillColor: p.FillColor, BorderColor: p.BorderColor,
		BorderRadiusPx: p.BorderRadiusPx, Opacity: p.Opacity,
		CreatedAt: p.CreatedAt.UTC().Format(time.RFC3339Nano), UpdatedAt: p.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func handleListWidgetProfiles(logger *slog.Logger, svc GoalsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		goalID := r.URL.Query().Get("goalId")
		list, err := svc.ListWidgetProfiles(r.Context(), goalID)
		if err != nil {
			writeGoalsError(w, logger, err)
			return
		}
		out := make([]widgetProfileResponse, 0, len(list))
		for _, p := range list {
			out = append(out, toWidgetProfileResponse(p))
		}
		writeJSON(w, logger, http.StatusOK, out)
	}
}

func handleCreateWidgetProfile(logger *slog.Logger, svc GoalsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body widgetProfileRequest
		if err := decodeJSONWithLimit(w, r, &body, maxGoalsBodyBytes); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		p, err := svc.CreateWidgetProfile(r.Context(), body.toDomain(""))
		if err != nil {
			writeGoalsError(w, logger, err)
			return
		}
		w.Header().Set("Location", "/api/widget-profiles/"+p.ID)
		writeJSON(w, logger, http.StatusCreated, toWidgetProfileResponse(p))
	}
}

func handleGetWidgetProfile(logger *slog.Logger, svc GoalsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := svc.GetWidgetProfile(r.Context(), r.PathValue("id"))
		if err != nil {
			writeGoalsError(w, logger, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toWidgetProfileResponse(p))
	}
}

func handleUpdateWidgetProfile(logger *slog.Logger, svc GoalsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var body widgetProfileRequest
		if err := decodeJSONWithLimit(w, r, &body, maxGoalsBodyBytes); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		p, err := svc.UpdateWidgetProfile(r.Context(), body.toDomain(id))
		if err != nil {
			writeGoalsError(w, logger, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toWidgetProfileResponse(p))
	}
}

func handleDeleteWidgetProfile(logger *slog.Logger, svc GoalsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := svc.DeleteWidgetProfile(r.Context(), r.PathValue("id")); err != nil {
			writeGoalsError(w, logger, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleRotateWidgetProfileSlug(logger *slog.Logger, svc GoalsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := svc.RotatePublicSlug(r.Context(), r.PathValue("id"))
		if err != nil {
			writeGoalsError(w, logger, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toWidgetProfileResponse(p))
	}
}
