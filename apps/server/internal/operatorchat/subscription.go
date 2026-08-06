package operatorchat

import "sync"

// Reasons a Subscription's channel closes - mirrors internal/engagement's
// own constants exactly, deliberately not shared as one type across
// packages (this package must not import internal/engagement's bus
// package - see this file's own package doc comment).
const (
	ReasonCancelled    = "cancelled"
	ReasonSlowConsumer = "slow_consumer"
	ReasonShutdown     = "shutdown"
)

// Subscription is one live consumer's view of the projection: a bounded
// channel of item revisions (new items and lifecycle upserts alike,
// undifferentiated - see docs/progress.md's Stage 9 entry for why a
// complete-upsert stream was chosen over a separate update-event type),
// plus a single-fire reason channel explaining why Items() eventually
// closes.
//
// A Subscription never blocks projection updates: a consumer that falls
// behind (its buffered channel is full when a new revision is produced) is
// unsubscribed immediately with ReasonSlowConsumer instead of blocking
// every other publisher and subscriber waiting on it - identical policy to
// internal/engagement.Bus.
type Subscription struct {
	id          uint64
	proj        *Projection
	items       chan Item
	closeReason chan string
	closeOnce   sync.Once
}

// Items returns the channel of revisions (replay, then live).
func (s *Subscription) Items() <-chan Item {
	return s.items
}

// Closed reports the reason Items() closed, exactly once.
func (s *Subscription) Closed() <-chan string {
	return s.closeReason
}

// Cancel ends the subscription explicitly. Safe to call more than once.
func (s *Subscription) Cancel() {
	s.proj.unsubscribe(s.id, ReasonCancelled)
}

func (s *Subscription) close(reason string) {
	s.closeOnce.Do(func() {
		close(s.items)
		s.closeReason <- reason
		close(s.closeReason)
	})
}
