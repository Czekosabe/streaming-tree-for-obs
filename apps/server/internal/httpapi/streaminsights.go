package httpapi

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/domain/streaminsights"
)

// StreamInsightsService is the subset of streaminsights.Service the
// HTTP layer needs.
type StreamInsightsService interface {
	Compute(ctx context.Context) (streaminsights.Insights, error)
}

type streamInsightsSessionSummaryResponse struct {
	SessionID       string  `json:"sessionId"`
	StartedAt       string  `json:"startedAt"`
	DurationSeconds float64 `json:"durationSeconds"`
}

type streamInsightsDestinationResponse struct {
	PlatformID      *string        `json:"platformId"`
	ProviderID      string         `json:"providerId"`
	DisplayName     string         `json:"displayName"`
	SessionCount    int            `json:"sessionCount"`
	DurationSeconds float64        `json:"durationSeconds"`
	OutcomeCounts   map[string]int `json:"outcomeCounts"`
}

type streamInsightsResponse struct {
	TotalSessions          int                                   `json:"totalSessions"`
	TotalDurationSeconds   float64                               `json:"totalDurationSeconds"`
	AverageDurationSeconds float64                               `json:"averageDurationSeconds"`
	LongestSession         *streamInsightsSessionSummaryResponse `json:"longestSession"`
	SessionsByEndReason    map[string]int                        `json:"sessionsByEndReason"`
	Destinations           []streamInsightsDestinationResponse   `json:"destinations"`
}

func toStreamInsightsResponse(in streaminsights.Insights) streamInsightsResponse {
	var longest *streamInsightsSessionSummaryResponse
	if in.LongestSession != nil {
		longest = &streamInsightsSessionSummaryResponse{
			SessionID:       in.LongestSession.SessionID,
			StartedAt:       platform.FormatTimestamp(in.LongestSession.StartedAt),
			DurationSeconds: in.LongestSession.Duration.Seconds(),
		}
	}

	destinations := make([]streamInsightsDestinationResponse, 0, len(in.Destinations))
	for _, d := range in.Destinations {
		destinations = append(destinations, streamInsightsDestinationResponse{
			PlatformID: d.PlatformID, ProviderID: d.ProviderID, DisplayName: d.DisplayName,
			SessionCount: d.SessionCount, DurationSeconds: d.TotalDuration.Seconds(),
			OutcomeCounts: d.OutcomeCounts,
		})
	}

	return streamInsightsResponse{
		TotalSessions: in.TotalSessions, TotalDurationSeconds: in.TotalStreamingDuration.Seconds(),
		AverageDurationSeconds: in.AverageSessionDuration.Seconds(), LongestSession: longest,
		SessionsByEndReason: in.SessionsByEndReason, Destinations: destinations,
	}
}

// registerStreamInsightsRoutes wires the Stage 27 stream insights API
// (docs/stream-session-history.md §14.4). Never registered under /api/public/*:
// management only, the same route-namespace convention as Stage 24/25/26.
func registerStreamInsightsRoutes(mux *http.ServeMux, logger *slog.Logger, service StreamInsightsService) {
	mux.HandleFunc("GET /api/stream-insights", handleGetStreamInsights(logger, service))
	mux.HandleFunc("/api/stream-insights", methodNotAllowed(logger, http.MethodGet))
}

func handleGetStreamInsights(logger *slog.Logger, service StreamInsightsService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		insights, err := service.Compute(r.Context())
		if err != nil {
			logger.Error("unhandled stream insights error",
				slog.String("path", r.URL.Path), slog.String("method", r.Method), slog.Any("error", err))
			writeError(w, logger, http.StatusInternalServerError, "internal_error", "The server encountered an unexpected error.")
			return
		}
		writeJSON(w, logger, http.StatusOK, toStreamInsightsResponse(insights))
	}
}
