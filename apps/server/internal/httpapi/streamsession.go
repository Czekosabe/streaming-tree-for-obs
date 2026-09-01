package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/domain/streamsession"
)

// StreamSessionService is the subset of streamsession.Repository the
// HTTP layer needs - a narrow, mostly-read surface (docs/stream-
// session-history.md §8). The same concrete repository instance the
// streamsession.Manager writes through is reused here, exactly like
// Stage 23's own Sources/Sinks reuse.
type StreamSessionService interface {
	ListSessions(ctx context.Context, limit int) ([]streamsession.Session, error)
	GetSession(ctx context.Context, id string) (streamsession.Session, error)
	DeleteAllSessions(ctx context.Context) error
	GetRetentionDays(ctx context.Context) (days int, found bool, err error)
	SetRetentionDays(ctx context.Context, days int, now time.Time) error
}

const (
	defaultStreamSessionListLimit = 50
	maxStreamSessionListLimit     = 200
	clearHistoryRequestBodyLimit  = 256
	retentionSettingsBodyLimit    = 256
)

type streamSessionDestinationResponse struct {
	ID          string  `json:"id"`
	PlatformID  *string `json:"platformId"`
	ProviderID  string  `json:"providerId"`
	DisplayName string  `json:"displayName"`
	StartedAt   string  `json:"startedAt"`
	EndedAt     *string `json:"endedAt"`
	Open        bool    `json:"open"`
	Outcome     string  `json:"outcome"`
}

type streamSessionResponse struct {
	ID           string                             `json:"id"`
	StartedAt    string                             `json:"startedAt"`
	EndedAt      *string                            `json:"endedAt"`
	Open         bool                               `json:"open"`
	EndReason    string                             `json:"endReason"`
	Destinations []streamSessionDestinationResponse `json:"destinations"`
}

func formatOptionalTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := platform.FormatTimestamp(*t)
	return &s
}

func toStreamSessionResponse(s streamsession.Session) streamSessionResponse {
	dests := make([]streamSessionDestinationResponse, 0, len(s.Destinations))
	for _, d := range s.Destinations {
		dests = append(dests, streamSessionDestinationResponse{
			ID: d.ID, PlatformID: d.PlatformID, ProviderID: d.ProviderID, DisplayName: d.DisplayName,
			StartedAt: platform.FormatTimestamp(d.StartedAt), EndedAt: formatOptionalTimePtr(d.EndedAt),
			Open: d.Open(), Outcome: string(d.Outcome),
		})
	}
	return streamSessionResponse{
		ID: s.ID, StartedAt: platform.FormatTimestamp(s.StartedAt), EndedAt: formatOptionalTimePtr(s.EndedAt),
		Open: s.Open(), EndReason: string(s.EndReason), Destinations: dests,
	}
}

// registerStreamSessionRoutes wires the Stage 24 stream session /
// operational history API (docs/stream-session-history.md §8). Never
// registered under /api/public/*: management-only, the same route-
// namespace boundary every other non-public route already relies on
// for withRemoteManagementSecurity.
func registerStreamSessionRoutes(mux *http.ServeMux, logger *slog.Logger, service StreamSessionService) {
	mux.HandleFunc("GET /api/stream-sessions", handleListStreamSessions(logger, service))
	mux.HandleFunc("DELETE /api/stream-sessions", handleClearStreamSessionHistory(logger, service))
	mux.HandleFunc("/api/stream-sessions", methodNotAllowed(logger, http.MethodGet, http.MethodDelete))

	// No bare any-method 405 catch-all is registered for .../settings:
	// it would panic at startup as an ambiguous overlap against the
	// GET-only .../{id} wildcard immediately below (neither pattern is
	// more specific than the other in both the method and path
	// dimensions at once - the same conflict class Stage 23's backup
	// restore-commit route hit). A genuinely wrong method against
	// .../settings still gets Go's own built-in automatic 405 (with an
	// Allow header), just without this package's usual custom JSON 405
	// body.
	mux.HandleFunc("GET /api/stream-sessions/settings", handleGetStreamSessionSettings(logger, service))
	mux.HandleFunc("PUT /api/stream-sessions/settings", handleSetStreamSessionSettings(logger, service))

	mux.HandleFunc("GET /api/stream-sessions/{id}", handleGetStreamSession(logger, service))
	mux.HandleFunc("/api/stream-sessions/{id}", methodNotAllowed(logger, http.MethodGet))
}

func handleListStreamSessions(logger *slog.Logger, service StreamSessionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := defaultStreamSessionListLimit
		if raw := r.URL.Query().Get("limit"); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed <= 0 {
				writeError(w, logger, http.StatusBadRequest, "invalid_limit", "limit must be a positive integer.")
				return
			}
			limit = parsed
		}
		if limit > maxStreamSessionListLimit {
			limit = maxStreamSessionListLimit
		}

		sessions, err := service.ListSessions(r.Context(), limit)
		if err != nil {
			writeStreamSessionError(w, logger, r, err)
			return
		}
		resp := make([]streamSessionResponse, 0, len(sessions))
		for _, s := range sessions {
			resp = append(resp, toStreamSessionResponse(s))
		}
		writeJSON(w, logger, http.StatusOK, map[string]any{"sessions": resp})
	}
}

func handleGetStreamSession(logger *slog.Logger, service StreamSessionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s, err := service.GetSession(r.Context(), r.PathValue("id"))
		if err != nil {
			writeStreamSessionError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toStreamSessionResponse(s))
	}
}

// clearStreamSessionHistoryRequest mirrors POST /api/system/shutdown's
// own {"confirm":true} convention for a destructive action with no
// other parameters.
type clearStreamSessionHistoryRequest struct {
	Confirm bool `json:"confirm"`
}

func handleClearStreamSessionHistory(logger *slog.Logger, service StreamSessionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body clearStreamSessionHistoryRequest
		if err := decodeJSONWithLimit(w, r, &body, clearHistoryRequestBodyLimit); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		if !body.Confirm {
			writeError(w, logger, http.StatusBadRequest, "confirmation_required", `The request body must be {"confirm":true}.`)
			return
		}
		if err := service.DeleteAllSessions(r.Context()); err != nil {
			writeStreamSessionError(w, logger, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

type streamSessionSettingsResponse struct {
	RetentionDays int `json:"retentionDays"`
}

func handleGetStreamSessionSettings(logger *slog.Logger, service StreamSessionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		days, found, err := service.GetRetentionDays(r.Context())
		if err != nil {
			writeStreamSessionError(w, logger, r, err)
			return
		}
		if !found {
			days = streamsession.DefaultRetentionDays
		}
		writeJSON(w, logger, http.StatusOK, streamSessionSettingsResponse{RetentionDays: days})
	}
}

type setStreamSessionSettingsRequest struct {
	RetentionDays int `json:"retentionDays"`
}

func handleSetStreamSessionSettings(logger *slog.Logger, service StreamSessionService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body setStreamSessionSettingsRequest
		if err := decodeJSONWithLimit(w, r, &body, retentionSettingsBodyLimit); err != nil {
			writeDecodeError(w, logger, err)
			return
		}
		if body.RetentionDays <= 0 {
			writeError(w, logger, http.StatusUnprocessableEntity, "validation_failed", "retentionDays must be a positive integer.")
			return
		}
		if err := service.SetRetentionDays(r.Context(), body.RetentionDays, time.Now()); err != nil {
			writeStreamSessionError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, streamSessionSettingsResponse{RetentionDays: body.RetentionDays})
	}
}

func writeStreamSessionError(w http.ResponseWriter, logger *slog.Logger, r *http.Request, err error) {
	if errors.Is(err, streamsession.ErrNotFound) {
		writeError(w, logger, http.StatusNotFound, "not_found", "The requested stream session does not exist.")
		return
	}
	logger.Error("unhandled stream session error",
		slog.String("path", r.URL.Path), slog.String("method", r.Method), slog.Any("error", err))
	writeError(w, logger, http.StatusInternalServerError, "internal_error", "The server encountered an unexpected error.")
}
