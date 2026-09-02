package streaminsights

import (
	"context"
	"fmt"
	"time"

	"github.com/streaming-tree/server/internal/domain/streamsession"
)

// SessionLister is the narrow slice of streamsession.Repository this
// domain depends on - reused unchanged, never a second implementation
// of session storage.
type SessionLister interface {
	ListSessions(ctx context.Context, limit int) ([]streamsession.Session, error)
}

// unbounded is SQLite's own documented "no LIMIT" convention -
// PruneSessionsBefore already bounds this table by the operator's own
// retention setting, so reading every retained row here is safe.
const unbounded = -1

// Clock returns the current time; injected so tests are deterministic.
type Clock func() time.Time

// Service computes Insights - it writes nothing anywhere.
type Service struct {
	sessions SessionLister
	now      Clock
}

// NewService builds a Service.
func NewService(sessions SessionLister) *Service {
	return &Service{sessions: sessions, now: time.Now}
}

// Compute aggregates every currently-retained session into Insights.
func (s *Service) Compute(ctx context.Context) (Insights, error) {
	sessions, err := s.sessions.ListSessions(ctx, unbounded)
	if err != nil {
		return Insights{}, fmt.Errorf("list sessions: %w", err)
	}

	now := s.now()
	insights := Insights{SessionsByEndReason: map[string]int{}}
	destByKey := map[string]*DestinationInsights{}
	destOrder := make([]string, 0)
	var longest *SessionSummary

	for _, sess := range sessions {
		insights.TotalSessions++
		duration := durationOf(sess.StartedAt, sess.EndedAt, now)
		insights.TotalStreamingDuration += duration
		insights.SessionsByEndReason[string(sess.EndReason)]++

		if longest == nil || duration > longest.Duration {
			longest = &SessionSummary{SessionID: sess.ID, StartedAt: sess.StartedAt, Duration: duration}
		}

		for _, d := range sess.Destinations {
			key := destinationKey(d)
			di, ok := destByKey[key]
			if !ok {
				di = &DestinationInsights{
					PlatformID: d.PlatformID, ProviderID: d.ProviderID, DisplayName: d.DisplayName,
					OutcomeCounts: map[string]int{},
				}
				destByKey[key] = di
				destOrder = append(destOrder, key)
			}
			di.SessionCount++
			di.TotalDuration += durationOf(d.StartedAt, d.EndedAt, now)
			di.OutcomeCounts[string(d.Outcome)]++
		}
	}

	if insights.TotalSessions > 0 {
		insights.AverageSessionDuration = insights.TotalStreamingDuration / time.Duration(insights.TotalSessions)
	}
	insights.LongestSession = longest

	insights.Destinations = make([]DestinationInsights, 0, len(destOrder))
	for _, key := range destOrder {
		insights.Destinations = append(insights.Destinations, *destByKey[key])
	}

	return insights, nil
}

// durationOf computes an interval's duration, treating a nil end as
// still ongoing against now - an in-progress stream is real
// operational time already happening, never excluded or zeroed.
func durationOf(start time.Time, end *time.Time, now time.Time) time.Duration {
	stop := now
	if end != nil {
		stop = *end
	}
	d := stop.Sub(start)
	if d < 0 {
		return 0
	}
	return d
}

// destinationKey groups by real platform id when the destination
// still exists, or by its own provider/display-name snapshot when it
// has since been deleted (docs/stream-insights.md §2).
func destinationKey(d streamsession.Destination) string {
	if d.PlatformID != nil {
		return "id:" + *d.PlatformID
	}
	return "snap:" + d.ProviderID + ":" + d.DisplayName
}
