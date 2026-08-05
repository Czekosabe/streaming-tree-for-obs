package bus

import (
	"sync"
	"time"
)

// dedupeSet is a bounded, TTL'd set of recently-seen deduplication keys.
//
// Bounded on two independent axes, both documented and both real limits (not
// just a soft target): an entry is forgotten once it is older than ttl, and
// regardless of age, the set never holds more than capacity entries at once
// - the oldest entry is evicted first if a new one would exceed it. Neither
// axis can produce an unbounded map: this is what keeps deduplication from
// becoming a slow memory leak across a long backend session, at the
// documented cost of no longer catching a redelivery older than the window.
type dedupeSet struct {
	mu       sync.Mutex
	ttl      time.Duration
	capacity int
	order    []dedupeEntry
	index    map[string]time.Time
	now      func() time.Time
}

type dedupeEntry struct {
	key string
	at  time.Time
}

func newDedupeSet(capacity int, ttl time.Duration, now func() time.Time) *dedupeSet {
	return &dedupeSet{
		ttl:      ttl,
		capacity: capacity,
		index:    make(map[string]time.Time),
		now:      now,
	}
}

// seen reports whether key was already recorded within the TTL window. If
// not (a fresh key, or one that expired), it records key as seen now and
// returns false.
func (d *dedupeSet) seen(key string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := d.now()
	d.evictExpiredLocked(now)

	if at, ok := d.index[key]; ok && now.Sub(at) < d.ttl {
		return true
	}
	d.recordLocked(key, now)
	return false
}

func (d *dedupeSet) evictExpiredLocked(now time.Time) {
	i := 0
	for i < len(d.order) && now.Sub(d.order[i].at) >= d.ttl {
		delete(d.index, d.order[i].key)
		i++
	}
	if i > 0 {
		d.order = d.order[i:]
	}
}

func (d *dedupeSet) recordLocked(key string, now time.Time) {
	if len(d.order) >= d.capacity {
		oldest := d.order[0]
		delete(d.index, oldest.key)
		d.order = d.order[1:]
	}
	d.order = append(d.order, dedupeEntry{key: key, at: now})
	d.index[key] = now
}

// len reports how many keys are currently retained. Test-only introspection.
func (d *dedupeSet) len() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.order)
}
