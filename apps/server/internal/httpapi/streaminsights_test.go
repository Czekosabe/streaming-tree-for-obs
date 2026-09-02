package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/streaminsights"
)

type stubStreamInsightsService struct {
	insights streaminsights.Insights
	err      error
}

func (s *stubStreamInsightsService) Compute(context.Context) (streaminsights.Insights, error) {
	if s.err != nil {
		return streaminsights.Insights{}, s.err
	}
	return s.insights, nil
}

func newStreamInsightsServer(t *testing.T, service StreamInsightsService) http.Handler {
	t.Helper()
	return NewRouter(Options{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		StartedAt:      time.Now(),
		StreamInsights: service,
	})
}

func TestGetStreamInsightsReturnsTheComputedReport(t *testing.T) {
	pid := "pf_1"
	stub := &stubStreamInsightsService{insights: streaminsights.Insights{
		TotalSessions: 2, TotalStreamingDuration: 2 * time.Hour, AverageSessionDuration: time.Hour,
		LongestSession:      &streaminsights.SessionSummary{SessionID: "sess_1", StartedAt: time.Now(), Duration: time.Hour},
		SessionsByEndReason: map[string]int{"ingest_stopped": 2},
		Destinations: []streaminsights.DestinationInsights{
			{PlatformID: &pid, ProviderID: "twitch", DisplayName: "Main Twitch", SessionCount: 2,
				TotalDuration: 2 * time.Hour, OutcomeCounts: map[string]int{"completed": 2}},
		},
	}}
	handler := newStreamInsightsServer(t, stub)

	recorder := do(t, handler, http.MethodGet, "/api/stream-insights", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", recorder.Code, recorder.Body.String())
	}
	var body streamInsightsResponse
	decodeBody(t, recorder, &body)
	if body.TotalSessions != 2 || body.TotalDurationSeconds != 7200 {
		t.Fatalf("body = %+v", body)
	}
	if len(body.Destinations) != 1 || body.Destinations[0].PlatformID == nil || *body.Destinations[0].PlatformID != "pf_1" {
		t.Fatalf("Destinations = %+v", body.Destinations)
	}
	if body.LongestSession == nil || body.LongestSession.SessionID != "sess_1" {
		t.Fatalf("LongestSession = %+v", body.LongestSession)
	}
}

func TestGetStreamInsightsOnEmptyHistoryReturnsZeroValues(t *testing.T) {
	stub := &stubStreamInsightsService{insights: streaminsights.Insights{SessionsByEndReason: map[string]int{}}}
	handler := newStreamInsightsServer(t, stub)

	recorder := do(t, handler, http.MethodGet, "/api/stream-insights", nil)
	var body streamInsightsResponse
	decodeBody(t, recorder, &body)
	if body.TotalSessions != 0 || body.LongestSession != nil {
		t.Fatalf("body = %+v, want zero-valued", body)
	}
}

func TestStreamInsightsWrongMethodReturns405(t *testing.T) {
	stub := &stubStreamInsightsService{}
	handler := newStreamInsightsServer(t, stub)

	recorder := do(t, handler, http.MethodPost, "/api/stream-insights", nil)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", recorder.Code)
	}
}
