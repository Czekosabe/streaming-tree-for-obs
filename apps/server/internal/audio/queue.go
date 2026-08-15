package audio

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	domain "github.com/streaming-tree/server/internal/domain/audio"
)

// NewItemID generates a fresh, unpredictable queue item identifier -
// never derived from source event text, provider event id, or any
// content that would let an item id leak information about what it
// contains.
func NewItemID() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate audio item id: %w", err)
	}
	return "auditem_" + hex.EncodeToString(buf), nil
}

// ItemSnapshot captures the configuration needed to reproduce an
// accepted item's intended speech at enqueue time (docs/audio-tts.md
// §11/governing task §26) - a later settings change affects only
// future events, never an item already sitting in the queue.
type ItemSnapshot struct {
	ProviderMode domain.ProviderMode
	VoiceID      string
	Language     string
	Speed        float64
	// Volume is already the final, fully-combined value the public SSE
	// payload serves verbatim - for SourceGlobalTTS this is simply the
	// global output volume (unchanged Stage 17A behavior); for
	// SourceAlertSound/SourceAlertTTS it is globalVolume multiplied by
	// the rule's own SoundVolume/TTSVolume exactly once, computed at
	// enqueue time (docs/alert-audio.md §6.3) - never recomputed or
	// multiplied again anywhere downstream.
	Volume float64
}

// Source classifies which of the three input paths produced a queued
// item (docs/alert-audio.md §3/§8) - Stage 17A's original Event-Bus-
// driven global TTS, or one of Stage 17B's two alert-owned kinds. The
// zero value, SourceGlobalTTS, preserves every pre-Stage-17B call site's
// existing behavior unchanged.
type Source int

const (
	SourceGlobalTTS Source = iota
	SourceAlertSound
	SourceAlertTTS
)

// AlertLink ties a SourceAlertSound/SourceAlertTTS item back to the
// exact alert instance that produced it (docs/alert-audio.md §8.1) - the
// stable runtime identity the synchronization contract requires. Never
// serialized to the public route; internal bookkeeping only. ChainNext
// holds the queued "then TTS" half when a rule configures sound+TTS
// together (docs/alert-audio.md §6.2) - nil for the final, or only, item
// in the chain.
type AlertLink struct {
	ProfileID  string
	InstanceID string
	ChainNext  *Item
}

// Item is one accepted queue entry - waiting in the ready queue,
// waiting for manual approval, or (once popped by the caller) about to
// become the current item. Never persisted; gone on restart.
type Item struct {
	ID        string
	Source    Source
	Text      string
	Synthetic bool
	// AssetID is meaningful only for SourceAlertSound - the managed
	// audioasset.Asset local ID to resolve at promotion time
	// (docs/alert-audio.md §8.2). Empty for every other Source.
	AssetID    string
	Snapshot   ItemSnapshot
	EnqueuedAt time.Time
	ExpiresAt  time.Time
	// AlertLink is non-nil only for SourceAlertSound/SourceAlertTTS.
	AlertLink *AlertLink
}

func (it Item) expired(now time.Time) bool {
	return !it.ExpiresAt.IsZero() && now.After(it.ExpiresAt)
}

// QueueCounters are the bounded, safe-to-expose runtime counters
// governing task §46 requires - never a complete historical message
// list, only totals.
type QueueCounters struct {
	TotalEnqueued        int
	TotalCapacityDropped int
	TotalExpired         int
	TotalRejected        int
	TotalManuallySkipped int
	TotalSynthetic       int
}

// Queue is the Stage 17A bounded, in-memory, never-persisted queue: a
// ready FIFO plus a separate pending-approval FIFO, sharing one total
// capacity bound (governing task §24/§25). Not concurrency-safe on its
// own - Manager holds its own mutex around every call.
type Queue struct {
	capacity int
	ready    []Item
	pending  []Item
	counters QueueCounters
}

// NewQueue builds an empty queue bounded to capacity total items across
// both the ready and pending-approval lists combined.
func NewQueue(capacity int) *Queue {
	return &Queue{capacity: capacity}
}

func (q *Queue) totalLen() int { return len(q.ready) + len(q.pending) }

// Enqueue accepts it directly into the ready queue. Rejects (and counts
// the drop) once the combined ready+pending length reaches capacity -
// the queue is never grown past its bound, and an ordinary incoming
// item never displaces anything already queued (governing task §24's
// "current item is never displaced, reject/drop newest incoming"
// policy, extended here to the whole bounded queue since Stage 17A adds
// no priority/preemption).
func (q *Queue) Enqueue(it Item) (accepted bool) {
	if q.totalLen() >= q.capacity {
		q.counters.TotalCapacityDropped++
		return false
	}
	q.ready = append(q.ready, it)
	q.recordAccepted(it)
	return true
}

// EnqueuePending is Enqueue's manual-approval counterpart: it places it
// in the pending-approval list instead of the ready queue, bounded by
// the same total capacity (§25).
func (q *Queue) EnqueuePending(it Item) (accepted bool) {
	if q.totalLen() >= q.capacity {
		q.counters.TotalCapacityDropped++
		return false
	}
	q.pending = append(q.pending, it)
	q.recordAccepted(it)
	return true
}

func (q *Queue) recordAccepted(it Item) {
	q.counters.TotalEnqueued++
	if it.Synthetic {
		q.counters.TotalSynthetic++
	}
}

// Approve moves a pending item into the ready queue unchanged - its
// enqueue-time snapshot is never touched (§26). ok is false when id is
// not currently pending.
func (q *Queue) Approve(id string) (it Item, ok bool) {
	for i, candidate := range q.pending {
		if candidate.ID == id {
			q.pending = append(q.pending[:i], q.pending[i+1:]...)
			q.ready = append(q.ready, candidate)
			return candidate, true
		}
	}
	return Item{}, false
}

// Reject discards a pending item outright - never a provider side
// effect, never enters the ready queue. ok is false when id is not
// currently pending.
func (q *Queue) Reject(id string) (it Item, ok bool) {
	for i, candidate := range q.pending {
		if candidate.ID == id {
			q.pending = append(q.pending[:i], q.pending[i+1:]...)
			q.counters.TotalRejected++
			return candidate, true
		}
	}
	return Item{}, false
}

// PopNextEligible removes and returns the oldest ready item that has
// not expired, discarding (and counting) any expired item found along
// the way (§7's queue item expiry) - never resurrects an expired item,
// never speaks it late.
func (q *Queue) PopNextEligible(now time.Time) (it Item, ok bool) {
	for len(q.ready) > 0 {
		candidate := q.ready[0]
		q.ready = q.ready[1:]
		if candidate.expired(now) {
			q.counters.TotalExpired++
			continue
		}
		return candidate, true
	}
	return Item{}, false
}

// Clear empties both the ready and pending-approval lists, returning
// the total number of items removed - never touches a separately
// tracked current item (Manager's own responsibility, since the
// currently-playing item is never part of this struct).
func (q *Queue) Clear() int {
	n := q.totalLen()
	q.ready = nil
	q.pending = nil
	return n
}

// SetCapacity updates the queue's own capacity bound - never evicts an
// item already queued, even if that leaves the queue temporarily over
// the new, lower bound; only future Enqueue/EnqueuePending calls are
// affected. Called whenever Stage 17A settings are (re)loaded, since
// QueueCapacity is itself an operator-configurable setting.
func (q *Queue) SetCapacity(capacity int) { q.capacity = capacity }

// DiscardExpired removes every already-expired ready item without
// promoting anything, counting each as expired - called by the poll
// loop even while no renderer is connected, so a long renderer outage
// never lets the queue fill up with items that are already expired
// instead of naturally clearing (§7/§59: an hour-old chat message must
// never suddenly speak once a renderer finally connects).
func (q *Queue) DiscardExpired(now time.Time) (discarded int) {
	kept := q.ready[:0]
	for _, it := range q.ready {
		if it.expired(now) {
			discarded++
			continue
		}
		kept = append(kept, it)
	}
	q.ready = kept
	q.counters.TotalExpired += discarded
	return discarded
}

// ReadyLen and PendingLen report the current length of each list.
func (q *Queue) ReadyLen() int   { return len(q.ready) }
func (q *Queue) PendingLen() int { return len(q.pending) }

// PendingList returns a bounded, defensive-copy snapshot of every
// currently pending item, oldest first - never mutates the queue.
func (q *Queue) PendingList() []Item {
	out := make([]Item, len(q.pending))
	copy(out, q.pending)
	return out
}

// Counters returns a copy of the queue's own running counters.
func (q *Queue) Counters() QueueCounters { return q.counters }

// RecordManualSkip increments the manual-skip counter - called by
// Manager when the operator explicitly skips the current item (a fact
// this struct itself has no notion of, since the current item is not
// part of Queue).
func (q *Queue) RecordManualSkip() { q.counters.TotalManuallySkipped++ }

// ContainsWhere reports whether any ready/pending item matches pred -
// read-only, never mutates. Used by Manager.AlertAudioState to
// distinguish "still queued, waiting its turn" from "no linked audio at
// all" for an instance whose audio has not yet been promoted to current
// (docs/alert-audio.md §8.5).
func (q *Queue) ContainsWhere(pred func(Item) bool) bool {
	for _, it := range q.ready {
		if pred(it) {
			return true
		}
	}
	for _, it := range q.pending {
		if pred(it) {
			return true
		}
	}
	return false
}

// RemoveWhere removes every ready/pending item matching pred, returning
// the count removed - used by Manager.CancelAlertAudio (docs/alert-
// audio.md §9.3) to drop a not-yet-promoted chained item belonging to a
// cancelled alert instance. Never touches a separately tracked current
// item, exactly like Clear.
func (q *Queue) RemoveWhere(pred func(Item) bool) int {
	removed := 0
	keptReady := q.ready[:0]
	for _, it := range q.ready {
		if pred(it) {
			removed++
			continue
		}
		keptReady = append(keptReady, it)
	}
	q.ready = keptReady

	keptPending := q.pending[:0]
	for _, it := range q.pending {
		if pred(it) {
			removed++
			continue
		}
		keptPending = append(keptPending, it)
	}
	q.pending = keptPending
	return removed
}
