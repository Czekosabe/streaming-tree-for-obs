package streaminsights

import (
	"context"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/streamsession"
)

type fakeSessions struct {
	rows []streamsession.Session
}

func (f *fakeSessions) ListSessions(_ context.Context, _ int) ([]streamsession.Session, error) {
	return f.rows, nil
}

func testService(sessions *fakeSessions, now time.Time) *Service {
	svc := NewService(sessions)
	svc.now = func() time.Time { return now }
	return svc
}

var fixedNow = time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)

func endedAt(t time.Time) *time.Time { return &t }

func TestComputeOnZeroSessionsReturnsZeroValuedInsights(t *testing.T) {
	svc := testService(&fakeSessions{}, fixedNow)

	insights, err := svc.Compute(context.Background())
	if err != nil {
		t.Fatalf("Compute() error = %v", err)
	}
	if insights.TotalSessions != 0 || insights.TotalStreamingDuration != 0 || insights.AverageSessionDuration != 0 {
		t.Errorf("Insights = %+v, want all zero", insights)
	}
	if insights.LongestSession != nil {
		t.Errorf("LongestSession = %+v, want nil", insights.LongestSession)
	}
	if len(insights.Destinations) != 0 {
		t.Errorf("Destinations = %+v, want none", insights.Destinations)
	}
}

func TestComputeAggregatesASingleCompletedSession(t *testing.T) {
	start := fixedNow.Add(-2 * time.Hour)
	end := fixedNow.Add(-1 * time.Hour)
	pid := "pf_1"
	svc := testService(&fakeSessions{rows: []streamsession.Session{
		{
			ID: "sess_1", StartedAt: start, EndedAt: endedAt(end), EndReason: streamsession.EndReasonIngestStopped,
			Destinations: []streamsession.Destination{
				{PlatformID: &pid, ProviderID: "twitch", DisplayName: "Main Twitch",
					StartedAt: start, EndedAt: endedAt(end), Outcome: streamsession.OutcomeCompleted},
			},
		},
	}}, fixedNow)

	insights, err := svc.Compute(context.Background())
	if err != nil {
		t.Fatalf("Compute() error = %v", err)
	}
	if insights.TotalSessions != 1 {
		t.Fatalf("TotalSessions = %d, want 1", insights.TotalSessions)
	}
	if insights.TotalStreamingDuration != time.Hour {
		t.Errorf("TotalStreamingDuration = %v, want 1h", insights.TotalStreamingDuration)
	}
	if insights.AverageSessionDuration != time.Hour {
		t.Errorf("AverageSessionDuration = %v, want 1h", insights.AverageSessionDuration)
	}
	if insights.SessionsByEndReason["ingest_stopped"] != 1 {
		t.Errorf("SessionsByEndReason = %+v, want ingest_stopped: 1", insights.SessionsByEndReason)
	}
	if len(insights.Destinations) != 1 || insights.Destinations[0].SessionCount != 1 {
		t.Fatalf("Destinations = %+v, want exactly one with SessionCount 1", insights.Destinations)
	}
	if insights.Destinations[0].TotalDuration != time.Hour {
		t.Errorf("Destination TotalDuration = %v, want 1h", insights.Destinations[0].TotalDuration)
	}
	if insights.Destinations[0].OutcomeCounts["completed"] != 1 {
		t.Errorf("OutcomeCounts = %+v, want completed: 1", insights.Destinations[0].OutcomeCounts)
	}
}

func TestComputeCountsAnOpenSessionAgainstNow(t *testing.T) {
	start := fixedNow.Add(-30 * time.Minute)
	svc := testService(&fakeSessions{rows: []streamsession.Session{
		{ID: "sess_open", StartedAt: start, EndedAt: nil, EndReason: ""},
	}}, fixedNow)

	insights, err := svc.Compute(context.Background())
	if err != nil {
		t.Fatalf("Compute() error = %v", err)
	}
	if insights.TotalStreamingDuration != 30*time.Minute {
		t.Errorf("TotalStreamingDuration = %v, want 30m (counted against now)", insights.TotalStreamingDuration)
	}
	if insights.SessionsByEndReason[""] != 1 {
		t.Errorf("SessionsByEndReason = %+v, want a real \"\" bucket for the still-open session", insights.SessionsByEndReason)
	}
}

func TestComputeGroupsADeletedDestinationBySnapshotNotDropped(t *testing.T) {
	start := fixedNow.Add(-time.Hour)
	end := fixedNow
	svc := testService(&fakeSessions{rows: []streamsession.Session{
		{
			ID: "sess_1", StartedAt: start, EndedAt: endedAt(end), EndReason: streamsession.EndReasonIngestStopped,
			Destinations: []streamsession.Destination{
				{PlatformID: nil, ProviderID: "youtube", DisplayName: "Old YouTube",
					StartedAt: start, EndedAt: endedAt(end), Outcome: streamsession.OutcomeSessionEnded},
			},
		},
	}}, fixedNow)

	insights, err := svc.Compute(context.Background())
	if err != nil {
		t.Fatalf("Compute() error = %v", err)
	}
	if len(insights.Destinations) != 1 {
		t.Fatalf("Destinations = %+v, want exactly one (grouped by snapshot, not dropped)", insights.Destinations)
	}
	got := insights.Destinations[0]
	if got.PlatformID != nil {
		t.Errorf("PlatformID = %v, want nil for a deleted destination", *got.PlatformID)
	}
	if got.DisplayName != "Old YouTube" || got.ProviderID != "youtube" {
		t.Errorf("got = %+v, want the snapshot preserved", got)
	}
}

func TestComputeGroupsTwoSessionsOnTheSameDestinationTogether(t *testing.T) {
	start1 := fixedNow.Add(-3 * time.Hour)
	end1 := fixedNow.Add(-2 * time.Hour)
	start2 := fixedNow.Add(-time.Hour)
	end2 := fixedNow
	pid := "pf_1"
	svc := testService(&fakeSessions{rows: []streamsession.Session{
		{ID: "sess_1", StartedAt: start1, EndedAt: endedAt(end1), EndReason: streamsession.EndReasonIngestStopped,
			Destinations: []streamsession.Destination{
				{PlatformID: &pid, ProviderID: "twitch", DisplayName: "Main Twitch",
					StartedAt: start1, EndedAt: endedAt(end1), Outcome: streamsession.OutcomeCompleted},
			}},
		{ID: "sess_2", StartedAt: start2, EndedAt: endedAt(end2), EndReason: streamsession.EndReasonIngestStopped,
			Destinations: []streamsession.Destination{
				{PlatformID: &pid, ProviderID: "twitch", DisplayName: "Main Twitch",
					StartedAt: start2, EndedAt: endedAt(end2), Outcome: streamsession.OutcomeError},
			}},
	}}, fixedNow)

	insights, err := svc.Compute(context.Background())
	if err != nil {
		t.Fatalf("Compute() error = %v", err)
	}
	if len(insights.Destinations) != 1 {
		t.Fatalf("Destinations = %+v, want exactly one (grouped by platform id)", insights.Destinations)
	}
	got := insights.Destinations[0]
	if got.SessionCount != 2 {
		t.Errorf("SessionCount = %d, want 2", got.SessionCount)
	}
	if got.TotalDuration != 2*time.Hour {
		t.Errorf("TotalDuration = %v, want 2h", got.TotalDuration)
	}
	if got.OutcomeCounts["completed"] != 1 || got.OutcomeCounts["error"] != 1 {
		t.Errorf("OutcomeCounts = %+v, want completed:1 error:1", got.OutcomeCounts)
	}
}

func TestComputeSelectsTheLongestSession(t *testing.T) {
	shortStart := fixedNow.Add(-30 * time.Minute)
	shortEnd := fixedNow
	longStart := fixedNow.Add(-5 * time.Hour)
	longEnd := fixedNow.Add(-1 * time.Hour)
	svc := testService(&fakeSessions{rows: []streamsession.Session{
		{ID: "sess_short", StartedAt: shortStart, EndedAt: endedAt(shortEnd), EndReason: streamsession.EndReasonIngestStopped},
		{ID: "sess_long", StartedAt: longStart, EndedAt: endedAt(longEnd), EndReason: streamsession.EndReasonIngestStopped},
	}}, fixedNow)

	insights, err := svc.Compute(context.Background())
	if err != nil {
		t.Fatalf("Compute() error = %v", err)
	}
	if insights.LongestSession == nil || insights.LongestSession.SessionID != "sess_long" {
		t.Fatalf("LongestSession = %+v, want sess_long", insights.LongestSession)
	}
	if insights.LongestSession.Duration != 4*time.Hour {
		t.Errorf("LongestSession.Duration = %v, want 4h", insights.LongestSession.Duration)
	}
}
