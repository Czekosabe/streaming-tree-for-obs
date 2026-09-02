package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/streaming-tree/server/internal/domain/preflight"
	"github.com/streaming-tree/server/internal/domain/streamsetup"
)

// PreflightService is the subset of preflight.Service the HTTP layer
// needs.
type PreflightService interface {
	Evaluate(ctx context.Context, profileID *string) (preflight.Report, error)
}

type preflightActionResponse struct {
	Code       string `json:"code"`
	PlatformID string `json:"platformId,omitempty"`
}

type preflightFindingResponse struct {
	Code       string                   `json:"code"`
	Severity   string                   `json:"severity"`
	PlatformID string                   `json:"platformId,omitempty"`
	Action     *preflightActionResponse `json:"action,omitempty"`
}

type preflightDestinationResponse struct {
	PlatformID  string                     `json:"platformId"`
	ProviderID  string                     `json:"providerId"`
	DisplayName string                     `json:"displayName"`
	Findings    []preflightFindingResponse `json:"findings"`
}

type preflightReportResponse struct {
	Status            string                         `json:"status"`
	Findings          []preflightFindingResponse     `json:"findings"`
	Destinations      []preflightDestinationResponse `json:"destinations"`
	SelectedProfileID *string                        `json:"selectedProfileId,omitempty"`
	StreamingActive   bool                           `json:"streamingActive"`
}

func toPreflightActionResponse(a *preflight.Action) *preflightActionResponse {
	if a == nil {
		return nil
	}
	return &preflightActionResponse{Code: string(a.Code), PlatformID: a.PlatformID}
}

func toPreflightFindingResponse(f preflight.Finding) preflightFindingResponse {
	return preflightFindingResponse{
		Code: f.Code, Severity: string(f.Severity), PlatformID: f.PlatformID,
		Action: toPreflightActionResponse(f.Action),
	}
}

func toPreflightReportResponse(r preflight.Report) preflightReportResponse {
	findings := make([]preflightFindingResponse, 0, len(r.Findings))
	for _, f := range r.Findings {
		findings = append(findings, toPreflightFindingResponse(f))
	}
	destinations := make([]preflightDestinationResponse, 0, len(r.Destinations))
	for _, d := range r.Destinations {
		destFindings := make([]preflightFindingResponse, 0, len(d.Findings))
		for _, f := range d.Findings {
			destFindings = append(destFindings, toPreflightFindingResponse(f))
		}
		destinations = append(destinations, preflightDestinationResponse{
			PlatformID: d.PlatformID, ProviderID: d.ProviderID, DisplayName: d.DisplayName,
			Findings: destFindings,
		})
	}
	return preflightReportResponse{
		Status: string(r.Status), Findings: findings, Destinations: destinations,
		SelectedProfileID: r.SelectedProfileID, StreamingActive: r.StreamingActive,
	}
}

// registerPreflightRoutes wires the Stage 26 stream preflight/launch-
// readiness API (docs/stream-preflight.md §6). Never registered under
// /api/public/*: management only, the same route-namespace convention
// as Stage 24/25.
func registerPreflightRoutes(mux *http.ServeMux, logger *slog.Logger, service PreflightService) {
	mux.HandleFunc("GET /api/preflight", handleGetPreflight(logger, service))
	mux.HandleFunc("/api/preflight", methodNotAllowed(logger, http.MethodGet))
}

func handleGetPreflight(logger *slog.Logger, service PreflightService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var profileID *string
		if raw := strings.TrimSpace(r.URL.Query().Get("profileId")); raw != "" {
			profileID = &raw
		}

		report, err := service.Evaluate(r.Context(), profileID)
		if err != nil {
			writePreflightError(w, logger, r, err)
			return
		}
		writeJSON(w, logger, http.StatusOK, toPreflightReportResponse(report))
	}
}

func writePreflightError(w http.ResponseWriter, logger *slog.Logger, r *http.Request, err error) {
	switch {
	case errors.Is(err, streamsetup.ErrNotFound):
		writeError(w, logger, http.StatusNotFound, "not_found", "The requested stream setup profile does not exist.")

	default:
		logger.Error("unhandled preflight error",
			slog.String("path", r.URL.Path), slog.String("method", r.Method), slog.Any("error", err))
		writeError(w, logger, http.StatusInternalServerError, "internal_error", "The server encountered an unexpected error.")
	}
}
