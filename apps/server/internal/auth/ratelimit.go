package auth

import (
	"sync"
	"time"
)

// Login rate-limit bounds (docs/remote-management.md §12): a per-IP
// bound blunts a single attacker, and a smaller global secondary bound
// blunts a distributed attempt without creating an easy denial-of-
// service against the single legitimate administrator IP.
const (
	PerIPMaxFailures  = 5
	GlobalMaxFailures = 30
	RateLimitWindow   = 5 * time.Minute
)

// LoginLimiter tracks failed login attempts, per-IP and globally, over
// a rolling window - bounded memory via lazy eviction on access, never
// an unbounded per-attacker history. Safe for concurrent use. Never
// stores the attempted password (docs/remote-management.md §12/§13).
type LoginLimiter struct {
	clock Clock

	mu     sync.Mutex
	perIP  map[string][]time.Time
	global []time.Time
}

// NewLoginLimiter builds an empty limiter using clock as its time
// source.
func NewLoginLimiter(clock Clock) *LoginLimiter {
	if clock == nil {
		clock = RealClock
	}
	return &LoginLimiter{clock: clock, perIP: map[string][]time.Time{}}
}

// Allow reports whether a login attempt from clientIP may proceed
// right now, and if not, how long the caller should wait
// (Retry-After). A blocked attempt is NOT itself recorded as a new
// failure - only RecordFailure (called after a real password check
// fails) adds to the window, so a client hammering a blocked endpoint
// does not extend its own block indefinitely.
func (l *LoginLimiter) Allow(clientIP string) (allowed bool, retryAfter time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.clock.Now()
	l.evictLocked(clientIP, now)

	if len(l.global) >= GlobalMaxFailures {
		return false, retryAfterLocked(l.global, now)
	}
	if len(l.perIP[clientIP]) >= PerIPMaxFailures {
		return false, retryAfterLocked(l.perIP[clientIP], now)
	}
	return true, 0
}

// RecordFailure adds one failed attempt for clientIP to both the
// per-IP and global windows. Call only after a real, completed
// password verification has failed - never for a request rejected by
// Allow itself (see Allow's own doc comment).
func (l *LoginLimiter) RecordFailure(clientIP string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.clock.Now()
	l.evictLocked(clientIP, now)
	l.perIP[clientIP] = append(l.perIP[clientIP], now)
	l.global = append(l.global, now)
}

// evictLocked drops every timestamp older than RateLimitWindow, for
// both clientIP's own history and the global one - must be called
// with mu already held.
func (l *LoginLimiter) evictLocked(clientIP string, now time.Time) {
	l.perIP[clientIP] = evictOld(l.perIP[clientIP], now)
	if len(l.perIP[clientIP]) == 0 {
		delete(l.perIP, clientIP)
	}
	l.global = evictOld(l.global, now)
}

func evictOld(timestamps []time.Time, now time.Time) []time.Time {
	cutoff := now.Add(-RateLimitWindow)
	kept := timestamps[:0]
	for _, t := range timestamps {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		return nil
	}
	return kept
}

// retryAfterLocked computes how long until the oldest entry in
// timestamps falls out of the window - must be called with mu already
// held, and timestamps must be non-empty (both call sites in Allow
// only reach here after confirming the relevant slice has reached its
// bound).
func retryAfterLocked(timestamps []time.Time, now time.Time) time.Duration {
	oldest := timestamps[0]
	for _, t := range timestamps[1:] {
		if t.Before(oldest) {
			oldest = t
		}
	}
	remaining := oldest.Add(RateLimitWindow).Sub(now)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// TrackedIPs reports how many distinct client IPs currently have at
// least one un-evicted failure recorded - a test/diagnostic helper for
// bounded-memory assertions, not part of the security contract.
func (l *LoginLimiter) TrackedIPs() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.perIP)
}
