package bus

import (
	"sync"

	engagement "github.com/streaming-tree/server/internal/domain/engagement"
)

// Reasons a Subscription's channel closes. Exposed so a consumer (the SSE
// handler, primarily) can tell a deliberate cancellation apart from being
// dropped for falling behind, and react honestly rather than silently
// reconnecting as if nothing happened.
const (
	ReasonCancelled    = "cancelled"
	ReasonSlowConsumer = "slow_consumer"
	ReasonShutdown     = "shutdown"
)

// Subscription is one live consumer's view of the Bus: a bounded channel of
// events published from the moment of subscription onward (plus any
// requested replay - see Bus.Subscribe), and a single-fire reason channel
// explaining why Events() eventually closes.
//
// A Subscription never blocks Bus.Publish: if this consumer falls behind
// (its buffered channel is full when a new event arrives), the Bus
// unsubscribes it immediately with ReasonSlowConsumer instead of blocking
// every other publisher and subscriber waiting on it.
type Subscription struct {
	id          uint64
	bus         *Bus
	events      chan engagement.Event
	closeReason chan string
	closeOnce   sync.Once
}

// Events returns the channel of live (and, if requested, replayed) events.
// It is closed exactly once, after which Closed() reports why.
func (s *Subscription) Events() <-chan engagement.Event {
	return s.events
}

// Closed reports the reason Events() closed, exactly once. Never reads
// before Events() has actually closed.
func (s *Subscription) Closed() <-chan string {
	return s.closeReason
}

// Cancel ends the subscription explicitly. Safe to call more than once and
// safe to call after the subscription already closed for another reason.
func (s *Subscription) Cancel() {
	s.bus.unsubscribe(s.id, ReasonCancelled)
}

func (s *Subscription) close(reason string) {
	s.closeOnce.Do(func() {
		close(s.events)
		s.closeReason <- reason
		close(s.closeReason)
	})
}
