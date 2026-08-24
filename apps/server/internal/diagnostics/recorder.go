package diagnostics

import (
	"sync"
	"time"
)

// RingCapacity bounds the recorder's memory use to a fixed, small
// footprint regardless of how long the process runs - never an
// unbounded slice (docs/final-hardening.md §D).
const RingCapacity = 2000

// Entry is one captured, already-redacted diagnostic record.
type Entry struct {
	Time      time.Time      `json:"time"`
	Severity  string         `json:"severity"`
	Subsystem string         `json:"subsystem"`
	Message   string         `json:"message"`
	Attrs     map[string]any `json:"attrs,omitempty"`
	Seq       uint64         `json:"seq"`
}

// Recorder is a fixed-capacity circular buffer of the most recent
// diagnostic entries, safe for concurrent use from many goroutines
// (every HTTP handler and background subsystem logs concurrently).
type Recorder struct {
	mu      sync.Mutex
	entries []Entry
	next    int
	filled  bool
	seq     uint64
}

// NewRecorder returns an empty Recorder with the standard capacity.
func NewRecorder() *Recorder {
	return &Recorder{entries: make([]Entry, RingCapacity)}
}

// Add appends one entry, overwriting the oldest once the buffer is
// full - the defining property of a bounded ring, never a growing
// slice.
func (r *Recorder) Add(e Entry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	e.Seq = r.seq
	r.entries[r.next] = e
	r.next = (r.next + 1) % RingCapacity
	if r.next == 0 {
		r.filled = true
	}
}

// Filter bounds and narrows a Snapshot query.
type Filter struct {
	Severity  string // exact match, empty = any
	Subsystem string // exact match, empty = any
	Search    string // case-insensitive substring of Message, empty = any
	Limit     int    // 0 = DefaultLimit
	// Before, when non-zero, restricts the result to entries strictly
	// older than the given Seq - the cursor for paging further back
	// than one page, using the previous page's oldest returned Seq.
	Before uint64
}

// DefaultLimit bounds a single retrieval when the caller does not
// specify one - the HTTP layer never returns the whole ring on an
// unqualified request.
const DefaultLimit = 200

// MaxLimit is the hard ceiling on any single retrieval, regardless of
// what a caller requests - never an arbitrary-size response.
const MaxLimit = RingCapacity

// ClampLimit applies the same DefaultLimit/MaxLimit clamp Snapshot
// applies internally - exported so a caller (the HTTP layer) can tell
// whether a Snapshot result was truncated by the limit rather than by
// running out of matching entries, without duplicating the clamp
// logic.
func ClampLimit(n int) int {
	if n <= 0 {
		return DefaultLimit
	}
	if n > MaxLimit {
		return MaxLimit
	}
	return n
}

// Snapshot returns the most recent entries matching filter, newest
// first, deterministically ordered by the real capture sequence
// (never map iteration order or wall-clock ties).
func (r *Recorder) Snapshot(filter Filter) []Entry {
	limit := ClampLimit(filter.Limit)

	r.mu.Lock()
	ordered := r.orderedLocked()
	r.mu.Unlock()

	out := make([]Entry, 0, limit)
	for i := len(ordered) - 1; i >= 0 && len(out) < limit; i-- {
		e := ordered[i]
		if filter.Before != 0 && e.Seq >= filter.Before {
			continue
		}
		if filter.Severity != "" && e.Severity != filter.Severity {
			continue
		}
		if filter.Subsystem != "" && e.Subsystem != filter.Subsystem {
			continue
		}
		if filter.Search != "" && !containsFold(e.Message, filter.Search) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// orderedLocked returns the buffer's real contents in chronological
// (oldest-to-newest) order. Caller must hold r.mu.
func (r *Recorder) orderedLocked() []Entry {
	if !r.filled {
		out := make([]Entry, r.next)
		copy(out, r.entries[:r.next])
		return out
	}
	out := make([]Entry, RingCapacity)
	copy(out, r.entries[r.next:])
	copy(out[RingCapacity-r.next:], r.entries[:r.next])
	return out
}

// containsFold reports whether s contains substr, ASCII-case-insensitively -
// avoids importing strings.ToLower on every entry's message for a
// simple substring search.
func containsFold(s, substr string) bool {
	if substr == "" {
		return true
	}
	n, m := len(s), len(substr)
	for i := 0; i+m <= n; i++ {
		if equalFold(s[i:i+m], substr) {
			return true
		}
	}
	return false
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
