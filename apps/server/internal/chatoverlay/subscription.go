package chatoverlay

import "sync"

// Reasons a Subscription's channel closes - mirrors operatorchat's own
// constants exactly, deliberately not shared as one type across packages
// (this package must not import internal/operatorchat's own subscription
// type - it only consumes operatorchat.Item values, not its Subscription
// type, keeping the two packages' concurrency primitives independent).
const (
	ReasonCancelled      = "cancelled"
	ReasonSlowConsumer   = "slow_consumer"
	ReasonShutdown       = "shutdown"
	ReasonOverlayDeleted = "overlay_deleted"
)

// Subscription is one live consumer's view of one overlay's public
// projection: a bounded channel of revisions (replay, then live), plus a
// single-fire reason channel explaining why Revisions() eventually
// closes.
//
// A Subscription never blocks projection updates: a consumer that falls
// behind is unsubscribed immediately with ReasonSlowConsumer instead of
// blocking every other subscriber (or another overlay's own projection)
// waiting on it - identical policy to internal/operatorchat.Subscription.
type Subscription struct {
	id          uint64
	proj        *Projection
	revisions   chan Revision
	closeReason chan string
	closeOnce   sync.Once
}

// Revisions returns the channel of revisions (replay, then live).
func (s *Subscription) Revisions() <-chan Revision {
	return s.revisions
}

// Closed reports the reason Revisions() closed, exactly once.
func (s *Subscription) Closed() <-chan string {
	return s.closeReason
}

// Cancel ends the subscription explicitly. Safe to call more than once.
func (s *Subscription) Cancel() {
	s.proj.unsubscribe(s.id, ReasonCancelled)
}

func (s *Subscription) close(reason string) {
	s.closeOnce.Do(func() {
		close(s.revisions)
		s.closeReason <- reason
		close(s.closeReason)
	})
}
