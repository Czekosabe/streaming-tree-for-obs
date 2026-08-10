package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/streaming-tree/server/internal/alerts"
	domain "github.com/streaming-tree/server/internal/domain/alerts"
	"github.com/streaming-tree/server/internal/domain/visualdesign"
)

// maxAlertsBodyBytes caps profile/rule/preview request bodies - generous
// for a rule with a full provider/account filter list, well below the
// general maxRequestBodyBytes ceiling.
const maxAlertsBodyBytes = 32 * 1024

// AlertsService is the subset of alerts.Manager the HTTP layer needs.
type AlertsService interface {
	CreateProfile(ctx context.Context, name string) (domain.Profile, error)
	GetProfile(ctx context.Context, id string) (domain.Profile, error)
	GetProfileByPublicSlug(ctx context.Context, slug string) (domain.Profile, error)
	ListProfiles(ctx context.Context) ([]domain.Profile, error)
	ReplaceProfile(ctx context.Context, id string, in domain.ProfileInput) (domain.Profile, error)
	RotatePublicSlug(ctx context.Context, id string) (domain.Profile, error)
	DeleteProfile(ctx context.Context, id string) error

	CreateRule(ctx context.Context, profileID string, in domain.RuleInput) (domain.Rule, error)
	GetRule(ctx context.Context, id string) (domain.Rule, error)
	ListRules(ctx context.Context, profileID string) ([]domain.Rule, error)
	ReplaceRule(ctx context.Context, id string, in domain.RuleInput) (domain.Rule, error)
	DeleteRule(ctx context.Context, id string) error
	OverlapWarnings(ctx context.Context, profileID string) ([]domain.OverlapWarning, error)

	Pause(profileID string) error
	Resume(profileID string) error
	SkipCurrent(profileID string) error
	ReplayPrevious(profileID string) error
	ClearQueue(profileID string) (int, error)
	ProfileStatus(profileID string) (alerts.ProfileStatus, error)

	TestRule(ctx context.Context, ruleID, edgeScenario string) (alerts.AlertSummary, error)

	SubscribeProfile(profileID string, after uint64) (*alerts.Subscription, bool, error)
	CurrentReset(profileID string) (alerts.Revision, error)
	LatestSequence(profileID string) uint64

	// GetVisualDesign/SaveVisualDesign/DeleteVisualDesign are Stage 13A's
	// own visual-design management façade - see internal/alerts.Manager's
	// own doc comments for the snapshot/cache semantics.
	GetVisualDesign(ctx context.Context, ruleID string) (visualdesign.Record, bool, error)
	SaveVisualDesign(ctx context.Context, ruleID string, doc visualdesign.Document, expectedRevision int) (visualdesign.Record, error)
	DeleteVisualDesign(ctx context.Context, ruleID string) error
}

const (
	maxAlertSSEClientsPerProfile = 8
	alertSSEKeepalive            = 15 * time.Second
)

// registerAlertRoutes wires the Stage 12A alert management API
// (/api/alert-profiles/..., /api/alert-rules/...) and the public,
// unauthenticated alert API (/api/public/alert-profiles/...) an OBS
// Browser Source actually loads.
func registerAlertRoutes(mux *http.ServeMux, logger *slog.Logger, svc AlertsService) {
	mux.HandleFunc("GET /api/alert-event-types", handleListAlertEventTypes(logger))
	mux.HandleFunc("/api/alert-event-types", methodNotAllowed(logger, http.MethodGet))

	mux.HandleFunc("GET /api/alert-profiles", handleListAlertProfiles(logger, svc))
	mux.HandleFunc("POST /api/alert-profiles", handleCreateAlertProfile(logger, svc))
	mux.HandleFunc("/api/alert-profiles", methodNotAllowed(logger, http.MethodGet, http.MethodPost))

	mux.HandleFunc("GET /api/alert-profiles/{id}", handleGetAlertProfile(logger, svc))
	mux.HandleFunc("PUT /api/alert-profiles/{id}", handleUpdateAlertProfile(logger, svc))
	mux.HandleFunc("DELETE /api/alert-profiles/{id}", handleDeleteAlertProfile(logger, svc))
	mux.HandleFunc("/api/alert-profiles/{id}", methodNotAllowed(logger, http.MethodGet, http.MethodPut, http.MethodDelete))

	mux.HandleFunc("POST /api/alert-profiles/{id}/rotate-public-slug", handleRotateAlertProfileSlug(logger, svc))
	mux.HandleFunc("/api/alert-profiles/{id}/rotate-public-slug", methodNotAllowed(logger, http.MethodPost))

	mux.HandleFunc("GET /api/alert-profiles/{id}/rules", handleListAlertRules(logger, svc))
	mux.HandleFunc("POST /api/alert-profiles/{id}/rules", handleCreateAlertRule(logger, svc))
	mux.HandleFunc("/api/alert-profiles/{id}/rules", methodNotAllowed(logger, http.MethodGet, http.MethodPost))

	mux.HandleFunc("GET /api/alert-profiles/{id}/queue", handleGetAlertQueue(logger, svc))
	mux.HandleFunc("/api/alert-profiles/{id}/queue", methodNotAllowed(logger, http.MethodGet))
	mux.HandleFunc("POST /api/alert-profiles/{id}/queue/pause", handleAlertQueuePause(logger, svc))
	mux.HandleFunc("/api/alert-profiles/{id}/queue/pause", methodNotAllowed(logger, http.MethodPost))
	mux.HandleFunc("POST /api/alert-profiles/{id}/queue/resume", handleAlertQueueResume(logger, svc))
	mux.HandleFunc("/api/alert-profiles/{id}/queue/resume", methodNotAllowed(logger, http.MethodPost))
	mux.HandleFunc("POST /api/alert-profiles/{id}/queue/skip-current", handleAlertQueueSkipCurrent(logger, svc))
	mux.HandleFunc("/api/alert-profiles/{id}/queue/skip-current", methodNotAllowed(logger, http.MethodPost))
	mux.HandleFunc("POST /api/alert-profiles/{id}/queue/replay-previous", handleAlertQueueReplayPrevious(logger, svc))
	mux.HandleFunc("/api/alert-profiles/{id}/queue/replay-previous", methodNotAllowed(logger, http.MethodPost))
	mux.HandleFunc("POST /api/alert-profiles/{id}/queue/clear", handleAlertQueueClear(logger, svc))
	mux.HandleFunc("/api/alert-profiles/{id}/queue/clear", methodNotAllowed(logger, http.MethodPost))

	// Deliberately NOT nested under /api/alert-rules/ (e.g. not
	// /api/alert-rules/preview): Go's ServeMux treats a bare,
	// all-methods catch-all registered for a literal path segment as
	// conflicting with a method-specific {id} wildcard pattern covering
	// the same path space ("preview" would also match {id}="preview") -
	// a real registration-time panic, not just a style preference.
	mux.HandleFunc("POST /api/alert-rule-preview", handleAlertRulePreview(logger))
	mux.HandleFunc("/api/alert-rule-preview", methodNotAllowed(logger, http.MethodPost))

	mux.HandleFunc("GET /api/alert-rules/{id}", handleGetAlertRule(logger, svc))
	mux.HandleFunc("PUT /api/alert-rules/{id}", handleUpdateAlertRule(logger, svc))
	mux.HandleFunc("DELETE /api/alert-rules/{id}", handleDeleteAlertRule(logger, svc))
	mux.HandleFunc("/api/alert-rules/{id}", methodNotAllowed(logger, http.MethodGet, http.MethodPut, http.MethodDelete))

	mux.HandleFunc("POST /api/alert-rules/{id}/test", handleTestAlertRule(logger, svc))
	mux.HandleFunc("/api/alert-rules/{id}/test", methodNotAllowed(logger, http.MethodPost))

	streamLimiter := newAlertStreamLimiter()
	mux.HandleFunc("GET /api/public/alert-profiles/{slug}/config", handleGetPublicAlertProfileConfig(logger, svc))
	mux.HandleFunc("/api/public/alert-profiles/{slug}/config", methodNotAllowed(logger, http.MethodGet))

	mux.HandleFunc("GET /api/public/alert-profiles/{slug}/stream", handlePublicAlertStream(logger, svc, streamLimiter))
	mux.HandleFunc("/api/public/alert-profiles/{slug}/stream", methodNotAllowed(logger, http.MethodGet))
}

// --- event-type capability DTO ---------------------------------------------

type alertEventTypeCapabilityResponse struct {
	EventType             string   `json:"eventType"`
	HasUser               bool     `json:"hasUser"`
	HasMessage            bool     `json:"hasMessage"`
	HasQuantity           bool     `json:"hasQuantity"`
	HasAnonymity          bool     `json:"hasAnonymity"`
	HasRewardTitle        bool     `json:"hasRewardTitle"`
	HasRoles              bool     `json:"hasRoles"`
	AvailablePlaceholders []string `json:"availablePlaceholders"`

	// Groupable: Stage 12B task Part 31 - "for unsupported event types,
	// hide grouping controls or present a clear unsupported explanation."
	// GroupingRequiresHiddenMessage: true only when this type also has a
	// real message (Part 11) - the rule editor uses it to explain why
	// enabling grouping will force "show message" off.
	Groupable                     bool `json:"groupable"`
	GroupingRequiresHiddenMessage bool `json:"groupingRequiresHiddenMessage"`
}

// handleListAlertEventTypes exposes the real, capability-derived
// per-event-type table (Part 6/36, and Stage 12B's own grouping
// capability) so the frontend never hand-maintains its own copy of which
// conditions/placeholders/grouping behavior apply to which event type.
func handleListAlertEventTypes(logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out := make([]alertEventTypeCapabilityResponse, 0, len(domain.ValidEventTypes))
		for _, t := range domain.ValidEventTypes {
			capability := domain.CapabilityFor(t)
			grouping := domain.GroupingCapabilityFor(t)
			out = append(out, alertEventTypeCapabilityResponse{
				EventType: string(t), HasUser: capability.HasUser, HasMessage: capability.HasMessage,
				HasQuantity: capability.HasQuantity, HasAnonymity: capability.HasAnonymity, HasRewardTitle: capability.HasRewardTitle,
				HasRoles: capability.HasRoles, AvailablePlaceholders: alerts.AvailablePlaceholders(t),
				Groupable: grouping.Groupable, GroupingRequiresHiddenMessage: grouping.RequiresNoMessage,
			})
		}
		writeJSON(w, logger, http.StatusOK, out)
	}
}

// --- profile DTOs -----------------------------------------------------------

type createAlertProfileRequest struct {
	Name string `json:"name"`
}

type alertProfileRequest struct {
	Name                   string `json:"name"`
	Enabled                bool   `json:"enabled"`
	Language               string `json:"language"`
	Theme                  string `json:"theme"`
	Position               string `json:"position"`
	TextAlign              string `json:"textAlign"`
	MaxQueueItems          int    `json:"maxQueueItems"`
	MaximumQueueAgeSeconds int    `json:"maximumQueueAgeSeconds"`
}

func (r alertProfileRequest) toInput() domain.ProfileInput {
	return domain.ProfileInput{
		Name: r.Name, Enabled: r.Enabled, Language: domain.Language(r.Language),
		Theme: domain.Theme(r.Theme), Position: domain.Position(r.Position), TextAlign: domain.TextAlign(r.TextAlign),
		MaxQueueItems: r.MaxQueueItems, MaximumQueueAgeSeconds: r.MaximumQueueAgeSeconds,
	}
}

type alertProfileResponse struct {
	ID                     string `json:"id"`
	PublicSlug             string `json:"publicSlug"`
	Name                   string `json:"name"`
	Enabled                bool   `json:"enabled"`
	Language               string `json:"language"`
	Theme                  string `json:"theme"`
	Position               string `json:"position"`
	TextAlign              string `json:"textAlign"`
	MaxQueueItems          int    `json:"maxQueueItems"`
	MaximumQueueAgeSeconds int    `json:"maximumQueueAgeSeconds"`
	CreatedAt              string `json:"createdAt"`
	UpdatedAt              string `json:"updatedAt"`
}

func toAlertProfileResponse(p domain.Profile) alertProfileResponse {
	return alertProfileResponse{
		ID: p.ID, PublicSlug: p.PublicSlug, Name: p.Name, Enabled: p.Enabled,
		Language: string(p.Language), Theme: string(p.Theme), Position: string(p.Position), TextAlign: string(p.TextAlign),
		MaxQueueItems: p.MaxQueueItems, MaximumQueueAgeSeconds: p.MaximumQueueAgeSeconds,
		CreatedAt: p.CreatedAt.UTC().Format(time.RFC3339Nano), UpdatedAt: p.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

// --- rule DTOs ---------------------------------------------------------

type alertRuleRequest struct {
	Name                string   `json:"name"`
	Enabled             bool     `json:"enabled"`
	EventType           string   `json:"eventType"`
	Priority            int      `json:"priority"`
	DurationMS          int      `json:"durationMs"`
	MinimumQuantity     *int64   `json:"minimumQuantity,omitempty"`
	MaximumQuantity     *int64   `json:"maximumQuantity,omitempty"`
	RequiredRole        string   `json:"requiredRole"`
	ShowPlatform        bool     `json:"showPlatform"`
	ShowUsername        bool     `json:"showUsername"`
	ShowMessage         bool     `json:"showMessage"`
	ShowQuantity        bool     `json:"showQuantity"`
	TextTemplate        string   `json:"textTemplate"`
	EntryAnimation      string   `json:"entryAnimation"`
	ExitAnimation       string   `json:"exitAnimation"`
	AnimationDurationMS int      `json:"animationDurationMs"`
	Providers           []string `json:"providers"`
	Accounts            []string `json:"accounts"`

	// AllowGrouping, GroupWindowMS, InterruptMode and Interruptible are
	// Stage 12B additions. GroupWindowMS and Interruptible are pointers
	// (and InterruptMode defaults on the empty string, never itself a
	// valid value) so a request from a client that predates Stage 12B -
	// scripts/verify-alerts.mjs's own unmodified ruleBody() being the
	// concrete, tested example (Part 44: "run every existing integration
	// script unchanged") - and simply omits these keys still gets the
	// documented Stage-12A-preserving safe defaults (AllowGrouping=false,
	// GroupWindowMS=domain.DefaultGroupWindowMS, InterruptMode=never,
	// Interruptible=true) in toInput() below, rather than Go's zero
	// values (0, "", false) being indistinguishable from an explicit
	// choice and failing GroupWindowMS's own unconditional bound check
	// or silently making every legacy-created rule non-interruptible.
	AllowGrouping bool   `json:"allowGrouping"`
	GroupWindowMS *int   `json:"groupWindowMs,omitempty"`
	InterruptMode string `json:"interruptMode,omitempty"`
	Interruptible *bool  `json:"interruptible,omitempty"`
}

func (r alertRuleRequest) toInput() domain.RuleInput {
	groupWindowMS := domain.DefaultGroupWindowMS
	if r.GroupWindowMS != nil {
		groupWindowMS = *r.GroupWindowMS
	}
	interruptMode := domain.InterruptNever
	if r.InterruptMode != "" {
		interruptMode = domain.InterruptMode(r.InterruptMode)
	}
	interruptible := true
	if r.Interruptible != nil {
		interruptible = *r.Interruptible
	}
	providers := make([]domain.ProviderID, len(r.Providers))
	for i, p := range r.Providers {
		providers[i] = domain.ProviderID(p)
	}
	accounts := make([]string, len(r.Accounts))
	copy(accounts, r.Accounts)
	return domain.RuleInput{
		Name: r.Name, Enabled: r.Enabled, EventType: domain.EventType(r.EventType),
		Priority: r.Priority, DurationMS: r.DurationMS,
		MinimumQuantity: r.MinimumQuantity, MaximumQuantity: r.MaximumQuantity,
		RequiredRole: domain.Role(r.RequiredRole),
		ShowPlatform: r.ShowPlatform, ShowUsername: r.ShowUsername, ShowMessage: r.ShowMessage, ShowQuantity: r.ShowQuantity,
		TextTemplate: r.TextTemplate, EntryAnimation: domain.Animation(r.EntryAnimation), ExitAnimation: domain.Animation(r.ExitAnimation),
		AnimationDurationMS: r.AnimationDurationMS, Providers: providers, Accounts: accounts,
		AllowGrouping: r.AllowGrouping, GroupWindowMS: groupWindowMS,
		InterruptMode: interruptMode, Interruptible: interruptible,
	}
}

type alertRuleResponse struct {
	ID                  string   `json:"id"`
	ProfileID           string   `json:"profileId"`
	Name                string   `json:"name"`
	Enabled             bool     `json:"enabled"`
	EventType           string   `json:"eventType"`
	Priority            int      `json:"priority"`
	DurationMS          int      `json:"durationMs"`
	MinimumQuantity     *int64   `json:"minimumQuantity,omitempty"`
	MaximumQuantity     *int64   `json:"maximumQuantity,omitempty"`
	RequiredRole        string   `json:"requiredRole"`
	ShowPlatform        bool     `json:"showPlatform"`
	ShowUsername        bool     `json:"showUsername"`
	ShowMessage         bool     `json:"showMessage"`
	ShowQuantity        bool     `json:"showQuantity"`
	TextTemplate        string   `json:"textTemplate"`
	EntryAnimation      string   `json:"entryAnimation"`
	ExitAnimation       string   `json:"exitAnimation"`
	AnimationDurationMS int      `json:"animationDurationMs"`
	Providers           []string `json:"providers"`
	Accounts            []string `json:"accounts"`

	AllowGrouping bool `json:"allowGrouping"`
	GroupWindowMS int  `json:"groupWindowMs"`

	InterruptMode string `json:"interruptMode"`
	Interruptible bool   `json:"interruptible"`

	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

func toAlertRuleResponse(r domain.Rule) alertRuleResponse {
	providers := make([]string, 0, len(r.Providers))
	for _, p := range r.Providers {
		providers = append(providers, string(p))
	}
	accounts := make([]string, 0, len(r.Accounts))
	accounts = append(accounts, r.Accounts...)
	return alertRuleResponse{
		ID: r.ID, ProfileID: r.ProfileID, Name: r.Name, Enabled: r.Enabled, EventType: string(r.EventType),
		Priority: r.Priority, DurationMS: r.DurationMS, MinimumQuantity: r.MinimumQuantity, MaximumQuantity: r.MaximumQuantity,
		RequiredRole: string(r.RequiredRole), ShowPlatform: r.ShowPlatform, ShowUsername: r.ShowUsername,
		ShowMessage: r.ShowMessage, ShowQuantity: r.ShowQuantity, TextTemplate: r.TextTemplate,
		EntryAnimation: string(r.EntryAnimation), ExitAnimation: string(r.ExitAnimation), AnimationDurationMS: r.AnimationDurationMS,
		Providers: providers, Accounts: accounts,
		AllowGrouping: r.AllowGrouping, GroupWindowMS: r.GroupWindowMS,
		InterruptMode: string(r.InterruptMode), Interruptible: r.Interruptible,
		CreatedAt: r.CreatedAt.UTC().Format(time.RFC3339Nano), UpdatedAt: r.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

type overlapWarningDTO struct {
	RuleID      string `json:"ruleId"`
	OtherRuleID string `json:"otherRuleId"`
	EventType   string `json:"eventType"`
}

type listAlertRulesResponse struct {
	Rules           []alertRuleResponse `json:"rules"`
	OverlapWarnings []overlapWarningDTO `json:"overlapWarnings"`
}

// --- queue DTOs -----------------------------------------------------

type alertSummaryDTO struct {
	AlertID       string `json:"alertId"`
	RuleID        string `json:"ruleId"`
	EventType     string `json:"eventType"`
	QueuedAt      string `json:"queuedAt"`
	Priority      int    `json:"priority"`
	Username      string `json:"username,omitempty"`
	Message       string `json:"message,omitempty"`
	Quantity      *int64 `json:"quantity,omitempty"`
	RenderedText  string `json:"renderedText"`
	Synthetic     bool   `json:"synthetic"`
	Replayed      bool   `json:"replayed"`
	GroupCount    int    `json:"groupCount"`
	Interruptible bool   `json:"interruptible"`
}

func toAlertSummaryDTO(s alerts.AlertSummary) alertSummaryDTO {
	return alertSummaryDTO{
		AlertID: s.AlertID, RuleID: s.RuleID, EventType: string(s.EventType), QueuedAt: s.QueuedAt.UTC().Format(time.RFC3339Nano),
		Priority: s.Priority, Username: s.Username, Message: s.Message, Quantity: s.Quantity,
		RenderedText: s.RenderedText, Synthetic: s.Synthetic, Replayed: s.Replayed,
		GroupCount: s.GroupCount, Interruptible: s.Interruptible,
	}
}

type alertQueueStatusResponse struct {
	ProfileID     string            `json:"profileId"`
	Enabled       bool              `json:"enabled"`
	Paused        bool              `json:"paused"`
	Current       *alertSummaryDTO  `json:"current,omitempty"`
	QueuedCount   int               `json:"queuedCount"`
	QueueCapacity int               `json:"queueCapacity"`
	NextQueued    []alertSummaryDTO `json:"nextQueued"`

	TotalEnqueued        int64 `json:"totalEnqueued"`
	TotalPlayed          int64 `json:"totalPlayed"`
	TotalExpired         int64 `json:"totalExpired"`
	TotalCapacityDropped int64 `json:"totalCapacityDropped"`
	TotalManuallySkipped int64 `json:"totalManuallySkipped"`
	TotalSynthetic       int64 `json:"totalSynthetic"`
	TotalGroupedMembers  int64 `json:"totalGroupedMembers"`
	TotalGroupsCreated   int64 `json:"totalGroupsCreated"`
	TotalPreempted       int64 `json:"totalPreempted"`

	ReplayAvailable   bool   `json:"replayAvailable"`
	ActiveSubscribers int    `json:"activeSubscribers"`
	LastAlertAt       string `json:"lastAlertAt,omitempty"`
	LastSkipReason    string `json:"lastSkipReason,omitempty"`
	InputGap          bool   `json:"inputGap"`
}

func toAlertQueueStatusResponse(st alerts.ProfileStatus) alertQueueStatusResponse {
	resp := alertQueueStatusResponse{
		ProfileID: st.ProfileID, Enabled: st.Enabled, Paused: st.Paused,
		QueuedCount: st.QueuedCount, QueueCapacity: st.QueueCapacity, NextQueued: make([]alertSummaryDTO, 0, len(st.NextQueued)),
		TotalEnqueued: st.TotalEnqueued, TotalPlayed: st.TotalPlayed, TotalExpired: st.TotalExpired,
		TotalCapacityDropped: st.TotalCapacityDropped, TotalManuallySkipped: st.TotalManuallySkipped, TotalSynthetic: st.TotalSynthetic,
		TotalGroupedMembers: st.TotalGroupedMembers, TotalGroupsCreated: st.TotalGroupsCreated, TotalPreempted: st.TotalPreempted,
		ReplayAvailable: st.ReplayAvailable, ActiveSubscribers: st.ActiveSubscribers, LastSkipReason: string(st.LastSkipReason),
		InputGap: st.InputGap,
	}
	if st.Current != nil {
		dto := toAlertSummaryDTO(*st.Current)
		resp.Current = &dto
	}
	for _, s := range st.NextQueued {
		resp.NextQueued = append(resp.NextQueued, toAlertSummaryDTO(s))
	}
	if st.LastAlertAt != nil {
		resp.LastAlertAt = st.LastAlertAt.UTC().Format(time.RFC3339Nano)
	}
	return resp
}

// --- handlers: profiles -----------------------------------------------

func handleListAlertProfiles(logger *slog.Logger, svc AlertsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := svc.ListProfiles(r.Context())
		if err != nil {
			writeAlertsError(w, logger, err)
			return
		}
		out := make([]alertProfileResponse, 0, len(list))
		for _, p := range list {
			out = append(out, toAlertProfileResponse(p))
		}
		writeJSON(w, logger, http.StatusOK, out)
	}
}

func handleCreateAlertProfile(logger *slog.Logger, svc AlertsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body createAlertProfileRequest
		if err := decodeJSONWithLimit(w, r, &body, maxAlertsBodyBytes); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		p, err := svc.CreateProfile(r.Context(), body.Name)
		if err != nil {
			writeAlertsError(w, logger, err)
			return
		}
		w.Header().Set("Location", "/api/alert-profiles/"+p.ID)
		writeJSON(w, logger, http.StatusCreated, toAlertProfileResponse(p))
	}
}

func handleGetAlertProfile(logger *slog.Logger, svc AlertsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := svc.GetProfile(r.Context(), r.PathValue("id"))
		if err != nil {
			writeAlertsError(w, logger, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toAlertProfileResponse(p))
	}
}

func handleUpdateAlertProfile(logger *slog.Logger, svc AlertsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var body alertProfileRequest
		if err := decodeJSONWithLimit(w, r, &body, maxAlertsBodyBytes); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		p, err := svc.ReplaceProfile(r.Context(), id, body.toInput())
		if err != nil {
			writeAlertsError(w, logger, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toAlertProfileResponse(p))
	}
}

func handleDeleteAlertProfile(logger *slog.Logger, svc AlertsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := svc.DeleteProfile(r.Context(), r.PathValue("id")); err != nil {
			writeAlertsError(w, logger, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleRotateAlertProfileSlug(logger *slog.Logger, svc AlertsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		p, err := svc.RotatePublicSlug(r.Context(), r.PathValue("id"))
		if err != nil {
			writeAlertsError(w, logger, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toAlertProfileResponse(p))
	}
}

// --- handlers: rules ----------------------------------------------------

func handleListAlertRules(logger *slog.Logger, svc AlertsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		profileID := r.PathValue("id")
		list, err := svc.ListRules(r.Context(), profileID)
		if err != nil {
			writeAlertsError(w, logger, err)
			return
		}
		warnings, err := svc.OverlapWarnings(r.Context(), profileID)
		if err != nil {
			writeAlertsError(w, logger, err)
			return
		}
		resp := listAlertRulesResponse{Rules: make([]alertRuleResponse, 0, len(list)), OverlapWarnings: make([]overlapWarningDTO, 0, len(warnings))}
		for _, ru := range list {
			resp.Rules = append(resp.Rules, toAlertRuleResponse(ru))
		}
		for _, w2 := range warnings {
			resp.OverlapWarnings = append(resp.OverlapWarnings, overlapWarningDTO{RuleID: w2.RuleID, OtherRuleID: w2.OtherRuleID, EventType: string(w2.EventType)})
		}
		writeJSON(w, logger, http.StatusOK, resp)
	}
}

func handleCreateAlertRule(logger *slog.Logger, svc AlertsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		profileID := r.PathValue("id")
		var body alertRuleRequest
		if err := decodeJSONWithLimit(w, r, &body, maxAlertsBodyBytes); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		if err := alerts.ValidateTemplateForEventType(body.TextTemplate, domain.EventType(body.EventType)); err != nil {
			writeAlertsError(w, logger, err)
			return
		}
		if err := alerts.ValidateGroupingTemplate(body.TextTemplate, domain.EventType(body.EventType), body.AllowGrouping); err != nil {
			writeAlertsError(w, logger, err)
			return
		}
		ru, err := svc.CreateRule(r.Context(), profileID, body.toInput())
		if err != nil {
			writeAlertsError(w, logger, err)
			return
		}
		w.Header().Set("Location", "/api/alert-rules/"+ru.ID)
		writeJSON(w, logger, http.StatusCreated, toAlertRuleResponse(ru))
	}
}

func handleGetAlertRule(logger *slog.Logger, svc AlertsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ru, err := svc.GetRule(r.Context(), r.PathValue("id"))
		if err != nil {
			writeAlertsError(w, logger, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toAlertRuleResponse(ru))
	}
}

func handleUpdateAlertRule(logger *slog.Logger, svc AlertsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var body alertRuleRequest
		if err := decodeJSONWithLimit(w, r, &body, maxAlertsBodyBytes); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		if err := alerts.ValidateTemplateForEventType(body.TextTemplate, domain.EventType(body.EventType)); err != nil {
			writeAlertsError(w, logger, err)
			return
		}
		if err := alerts.ValidateGroupingTemplate(body.TextTemplate, domain.EventType(body.EventType), body.AllowGrouping); err != nil {
			writeAlertsError(w, logger, err)
			return
		}
		ru, err := svc.ReplaceRule(r.Context(), id, body.toInput())
		if err != nil {
			writeAlertsError(w, logger, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toAlertRuleResponse(ru))
	}
}

func handleDeleteAlertRule(logger *slog.Logger, svc AlertsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := svc.DeleteRule(r.Context(), r.PathValue("id")); err != nil {
			writeAlertsError(w, logger, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

type alertRuleTestRequest struct {
	Scenario string `json:"scenario,omitempty"`
}

// handleTestAlertRule creates one synthetic alert through the real
// queue/playback path (Part 27/28) - never a real Twitch account or
// Event Bus event.
func handleTestAlertRule(logger *slog.Logger, svc AlertsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var body alertRuleTestRequest
		if r.ContentLength > 0 {
			if err := decodeJSONWithLimit(w, r, &body, maxAlertsBodyBytes); err != nil {
				writeDecodeError(w, logger, err)
				return
			}
		}
		summary, err := svc.TestRule(r.Context(), id, body.Scenario)
		if err != nil {
			writeAlertsError(w, logger, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toAlertSummaryDTO(summary))
	}
}

// --- handlers: preview (Part 37) ----------------------------------------

type alertRulePreviewRequest struct {
	EventType string `json:"eventType"`
	Template  string `json:"template"`
	Language  string `json:"language,omitempty"`
}

type alertRulePreviewResponse struct {
	RenderedText           string   `json:"renderedText"`
	CodePointCount         int      `json:"codePointCount"`
	ResolvedPlaceholders   []string `json:"resolvedPlaceholders"`
	UnresolvedPlaceholders []string `json:"unresolvedPlaceholders"`
	ValidForProvider       bool     `json:"validForProvider"`
}

// handleAlertRulePreview renders a template locally against
// representative fixture data for eventType - never sends, never
// persists, never touches the queue or a real Twitch account (Part 37:
// "editor preview: local and instant, does not touch queue").
func handleAlertRulePreview(logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body alertRulePreviewRequest
		if err := decodeJSONWithLimit(w, r, &body, maxAlertsBodyBytes); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		eventType := domain.EventType(body.EventType)
		if err := alerts.ValidateTemplateForEventType(body.Template, eventType); err != nil {
			writeAlertsError(w, logger, err)
			return
		}
		lang := domain.Language(body.Language)
		if lang != domain.LanguagePolish {
			lang = domain.LanguageEnglish
		}
		result, err := alerts.PreviewTemplate(eventType, body.Template, lang)
		if err != nil {
			writeAlertsError(w, logger, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, alertRulePreviewResponse{
			RenderedText: result.Text, CodePointCount: result.CodePointCount,
			ResolvedPlaceholders: result.Resolved, UnresolvedPlaceholders: result.Unresolved,
			ValidForProvider: result.ValidForProvider,
		})
	}
}

// --- handlers: queue commands --------------------------------------------

func handleGetAlertQueue(logger *slog.Logger, svc AlertsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		st, err := svc.ProfileStatus(r.PathValue("id"))
		if err != nil {
			writeAlertsError(w, logger, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toAlertQueueStatusResponse(st))
	}
}

func handleAlertQueuePause(logger *slog.Logger, svc AlertsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		if err := svc.Pause(r.PathValue("id")); err != nil {
			writeAlertsError(w, logger, err)
			return
		}
		st, err := svc.ProfileStatus(r.PathValue("id"))
		if err != nil {
			writeAlertsError(w, logger, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toAlertQueueStatusResponse(st))
	}
}

func handleAlertQueueResume(logger *slog.Logger, svc AlertsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		if err := svc.Resume(r.PathValue("id")); err != nil {
			writeAlertsError(w, logger, err)
			return
		}
		st, err := svc.ProfileStatus(r.PathValue("id"))
		if err != nil {
			writeAlertsError(w, logger, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toAlertQueueStatusResponse(st))
	}
}

func handleAlertQueueSkipCurrent(logger *slog.Logger, svc AlertsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		if err := svc.SkipCurrent(r.PathValue("id")); err != nil {
			writeAlertsError(w, logger, err)
			return
		}
		st, err := svc.ProfileStatus(r.PathValue("id"))
		if err != nil {
			writeAlertsError(w, logger, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toAlertQueueStatusResponse(st))
	}
}

func handleAlertQueueReplayPrevious(logger *slog.Logger, svc AlertsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		if err := svc.ReplayPrevious(r.PathValue("id")); err != nil {
			writeAlertsError(w, logger, err)
			return
		}
		st, err := svc.ProfileStatus(r.PathValue("id"))
		if err != nil {
			writeAlertsError(w, logger, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toAlertQueueStatusResponse(st))
	}
}

func handleAlertQueueClear(logger *slog.Logger, svc AlertsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireEmptyBody(w, r, logger) {
			return
		}
		if _, err := svc.ClearQueue(r.PathValue("id")); err != nil {
			writeAlertsError(w, logger, err)
			return
		}
		st, err := svc.ProfileStatus(r.PathValue("id"))
		if err != nil {
			writeAlertsError(w, logger, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toAlertQueueStatusResponse(st))
	}
}

// --- public config ---------------------------------------------------------

type publicAlertProfileConfigResponse struct {
	SchemaVersion int    `json:"schemaVersion"`
	Theme         string `json:"theme"`
	Position      string `json:"position"`
	TextAlign     string `json:"textAlign"`
	Language      string `json:"language"`
}

// resolvePublicAlertProfile resolves slug to a profile, treating "not
// found" and "disabled" identically for the caller (Part 40: the slug
// is a locator, not authentication - the public API deliberately never
// distinguishes the two to a viewer).
func resolvePublicAlertProfile(ctx context.Context, svc AlertsService, slug string) (domain.Profile, bool) {
	p, err := svc.GetProfileByPublicSlug(ctx, slug)
	if err != nil || !p.Enabled {
		return domain.Profile{}, false
	}
	return p, true
}

func handleGetPublicAlertProfileConfig(logger *slog.Logger, svc AlertsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, ok := resolvePublicAlertProfile(r.Context(), svc, r.PathValue("slug"))
		if !ok {
			// Never a hard error for an unknown/disabled slug (Part 40) -
			// a safe, empty/default config instead.
			writeJSON(w, logger, http.StatusOK, publicAlertProfileConfigResponse{
				SchemaVersion: 1, Theme: string(domain.ThemeMinimal), Position: string(domain.PositionBottom),
				TextAlign: string(domain.AlignCenter), Language: string(domain.LanguageEnglish),
			})
			return
		}
		writeJSON(w, logger, http.StatusOK, publicAlertProfileConfigResponse{
			SchemaVersion: 1, Theme: string(p.Theme), Position: string(p.Position), TextAlign: string(p.TextAlign), Language: string(p.Language),
		})
	}
}

// --- public SSE stream -------------------------------------------------

// alertStreamLimiter bounds live SSE clients per profile - independent
// per profile, so one profile reaching its cap never affects another's
// stream (Part 4).
type alertStreamLimiter struct {
	mu     sync.Mutex
	counts map[string]int
}

func newAlertStreamLimiter() *alertStreamLimiter {
	return &alertStreamLimiter{counts: make(map[string]int)}
}

func (l *alertStreamLimiter) acquire(profileID string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.counts[profileID] >= maxAlertSSEClientsPerProfile {
		return false
	}
	l.counts[profileID]++
	return true
}

func (l *alertStreamLimiter) release(profileID string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.counts[profileID]--
	if l.counts[profileID] <= 0 {
		delete(l.counts, profileID)
	}
}

func toPublicAlertDTO(a *alerts.PublicAlert) map[string]any {
	if a == nil {
		return nil
	}
	out := map[string]any{
		"schemaVersion": a.SchemaVersion, "alertId": a.AlertID, "eventType": a.EventType, "providerId": a.ProviderID,
		"synthetic": a.Synthetic, "replayed": a.Replayed, "username": a.Username, "message": a.Message,
		"quantity": a.Quantity, "groupCount": a.GroupCount, "renderedText": a.RenderedText, "durationMs": a.DurationMS,
		"entryAnimation": a.EntryAnimation, "exitAnimation": a.ExitAnimation, "animationDurationMs": a.AnimationDurationMS,
		// renderingMode/visualDesign: Stage 13A task Part 23's own
		// additive discriminator - "legacy" (visualDesign omitted/null)
		// or "visual_design" (the complete safe snapshot this instance
		// captured at match/test/replay time).
		"renderingMode": a.RenderingMode,
	}
	if a.VisualDesign != nil {
		out["visualDesign"] = toPublicVisualDesignDTO(a.VisualDesign)
	}
	return out
}

// writeAlertRevision serializes rev over SSE. An OpHide revision
// deliberately never carries "alert" (Stage 12B task Part 20/36: "the
// public hide operation should contain only operation, revision, alert
// ID, stable reason" - no prior rendered content) - only its own
// hiddenAlertId/reason fields.
func writeAlertRevision(w http.ResponseWriter, rev alerts.Revision) error {
	eventName := "alert." + string(rev.Operation)
	var data map[string]any
	if rev.Operation == alerts.OpHide {
		data = map[string]any{"paused": rev.Paused, "alertId": rev.HiddenAlertID, "reason": string(rev.Reason)}
	} else {
		data = map[string]any{"paused": rev.Paused, "alert": toPublicAlertDTO(rev.Alert)}
	}
	return writeSSEEvent(w, eventName, rev.Sequence, data)
}

// handlePublicAlertStream serves GET /api/public/alert-profiles/{slug}/stream
// over Server-Sent Events - mirrors handlePublicChatOverlayStream's own
// contract (Last-Event-ID replay, an explicit gap event, periodic
// keepalive, a bounded client count). An unknown or disabled profile
// never answers with a hard HTTP error (Part 40) - it opens a normal
// 200 SSE connection, sends one empty reset, and idles on keepalives
// only. A fresh connection (no Last-Event-ID) never replays historical
// show/hide revisions - only the current state, then live continuation
// (Part 23: "no historical queue content").
func handlePublicAlertStream(logger *slog.Logger, svc AlertsService, limiter *alertStreamLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeError(w, logger, http.StatusInternalServerError, "internal_error", "Streaming is not supported by this response writer.")
			return
		}

		p, available := resolvePublicAlertProfile(r.Context(), svc, r.PathValue("slug"))

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		keepalive := time.NewTicker(alertSSEKeepalive)
		defer keepalive.Stop()

		if !available {
			_ = writeSSEEvent(w, "alert.reset", 0, map[string]any{"paused": false, "alert": nil})
			flusher.Flush()
			for {
				select {
				case <-r.Context().Done():
					return
				case <-keepalive.C:
					writeSSEComment(w, "keepalive")
					flusher.Flush()
				}
			}
		}

		if !limiter.acquire(p.ID) {
			_ = writeSSEEvent(w, "alert.gap", 0, map[string]string{"reason": "stream_limit_reached"})
			flusher.Flush()
			return
		}
		defer limiter.release(p.ID)

		raw := r.Header.Get("Last-Event-ID")
		if raw == "" {
			raw = r.URL.Query().Get("after")
		}

		if raw == "" {
			// Fresh connection: a synthetic reset reflecting current
			// state right now, tagged with the profile's own latest
			// sequence so a LATER reconnect using it resumes with
			// nothing missed - never a replay of past show/hide
			// revisions.
			reset, err := svc.CurrentReset(p.ID)
			if err != nil {
				_ = writeSSEEvent(w, "alert.reset", 0, map[string]any{"paused": false, "alert": nil})
				flusher.Flush()
				return
			}
			reset.Sequence = svc.LatestSequence(p.ID)
			_ = writeAlertRevision(w, reset)
			flusher.Flush()
		}

		var after uint64
		if raw != "" {
			if parsed, err := strconv.ParseUint(raw, 10, 64); err == nil {
				after = parsed
			}
		} else {
			after = svc.LatestSequence(p.ID)
		}

		sub, gap, err := svc.SubscribeProfile(p.ID, after)
		if err != nil {
			_ = writeSSEEvent(w, "alert.reset", 0, map[string]any{"paused": false, "alert": nil})
			flusher.Flush()
			return
		}
		defer sub.Cancel()

		if gap {
			_ = writeSSEEvent(w, "alert.gap", 0, map[string]string{"reason": "sequence_evicted"})
			flusher.Flush()
			if reset, err := svc.CurrentReset(p.ID); err == nil {
				reset.Sequence = svc.LatestSequence(p.ID)
				_ = writeAlertRevision(w, reset)
				flusher.Flush()
			}
		}

		for {
			select {
			case <-r.Context().Done():
				return
			case rev, open := <-sub.Revisions():
				if !open {
					select {
					case reason := <-sub.Closed():
						if reason == alerts.ReasonSlowConsumer {
							_ = writeSSEEvent(w, "alert.gap", 0, map[string]string{"reason": "slow_consumer"})
							flusher.Flush()
						}
					default:
					}
					return
				}
				_ = writeAlertRevision(w, rev)
				flusher.Flush()
			case <-keepalive.C:
				writeSSEComment(w, "keepalive")
				flusher.Flush()
			}
		}
	}
}

// --- errors -------------------------------------------------------------

func writeAlertsError(w http.ResponseWriter, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, domain.ErrProfileNotFound), errors.Is(err, alerts.ErrProfileNotFound):
		writeError(w, logger, http.StatusNotFound, "alert_profile_not_found", "The requested alert profile does not exist.")
	case errors.Is(err, alerts.ErrProfileDisabled):
		writeError(w, logger, http.StatusConflict, "alert_profile_disabled", "This alert profile is currently disabled.")
	case errors.Is(err, domain.ErrRuleNotFound), errors.Is(err, alerts.ErrRuleNotFound):
		writeError(w, logger, http.StatusNotFound, "alert_rule_not_found", "The requested alert rule does not exist.")
	case errors.Is(err, domain.ErrAccountNotFound):
		writeError(w, logger, http.StatusNotFound, "alert_rule_account_not_found", "One of the target connected accounts does not exist.")
	case errors.Is(err, domain.ErrThresholdInvalid):
		writeError(w, logger, http.StatusUnprocessableEntity, "alert_rule_threshold_invalid", "The quantity threshold is invalid.")
	case errors.Is(err, domain.ErrConditionUnsupported):
		writeError(w, logger, http.StatusUnprocessableEntity, "alert_rule_condition_unsupported", "This condition is not supported by the rule's event type.")
	case errors.Is(err, alerts.ErrPlaceholderInvalid):
		writeError(w, logger, http.StatusUnprocessableEntity, "alert_template_invalid", "The template uses an unknown or malformed placeholder.")
	case errors.Is(err, domain.ErrValidation):
		writeError(w, logger, http.StatusUnprocessableEntity, "alert_profile_invalid", "The request failed validation.")
	case errors.Is(err, alerts.ErrQueueEmpty):
		writeError(w, logger, http.StatusConflict, "alert_queue_empty", "There is nothing to act on right now.")
	case errors.Is(err, alerts.ErrQueueFull):
		writeError(w, logger, http.StatusTooManyRequests, "alert_queue_full", "The alert queue is currently full.")
	case errors.Is(err, alerts.ErrNoReplaySnapshot):
		writeError(w, logger, http.StatusConflict, "alert_queue_empty", "There is no previous alert to replay yet.")
	case errors.Is(err, visualdesign.ErrRevisionConflict):
		writeError(w, logger, http.StatusConflict, "visual_design_revision_conflict", "Another save already changed this design - reload and try again.")
	case errors.Is(err, visualdesign.ErrTooLarge):
		writeError(w, logger, http.StatusUnprocessableEntity, "visual_design_too_large", "The design document is too large.")
	case errors.Is(err, visualdesign.ErrValidation):
		writeError(w, logger, http.StatusUnprocessableEntity, "visual_design_invalid", "The design failed validation.")
	case errors.Is(err, alerts.ErrVisualDesignUnavailable):
		writeError(w, logger, http.StatusServiceUnavailable, "visual_design_unavailable", "The visual design service is not available.")
	default:
		writeError(w, logger, http.StatusInternalServerError, "internal_error", "An unexpected error occurred.")
	}
}
