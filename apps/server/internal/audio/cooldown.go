package audio

import (
	"sync"
	"time"
)

// CooldownKey identifies a per-user cooldown identity - constructed ONLY
// when a genuinely stable identity exists (docs/audio-tts.md §11):
// providerID + connectedAccountID/sourceID + providerUserID. Never a
// display name, donor email, an email hash, a transaction id, or a
// donation id. The zero value is deliberately never valid() - see
// NewCooldownKey.
type CooldownKey struct {
	ProviderID string
	SourceID   string
	UserID     string
}

// NewCooldownKey builds a CooldownKey, or reports ok=false when
// providerID or userID is empty - an anonymous event, or one with no
// stable normalized user id, has no valid per-user identity and must
// fall back to the global cooldown only, never a fabricated key.
func NewCooldownKey(providerID, sourceID, userID string) (CooldownKey, bool) {
	if providerID == "" || userID == "" {
		return CooldownKey{}, false
	}
	return CooldownKey{ProviderID: providerID, SourceID: sourceID, UserID: userID}, true
}

// CooldownTracker enforces the global and per-user cooldown windows
// (docs/audio-tts.md §11). Concurrency-safe; every method takes an
// explicit now so tests can use a fake, monotonic-safe clock rather
// than wall-clock time.
type CooldownTracker struct {
	mu         sync.Mutex
	globalNext time.Time
	perUser    map[CooldownKey]time.Time
}

// NewCooldownTracker builds an empty tracker - every cooldown starts
// "open" until the first Reserve.
func NewCooldownTracker() *CooldownTracker {
	return &CooldownTracker{perUser: make(map[CooldownKey]time.Time)}
}

// Allowed reports whether an event may be accepted right now, without
// reserving anything. hasKey/key mirrors NewCooldownKey's own ok
// return - pass hasKey=false for an anonymous/unstable-identity event,
// which is then checked against the global cooldown only.
func (c *CooldownTracker) Allowed(hasKey bool, key CooldownKey, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if now.Before(c.globalNext) {
		return false
	}
	if hasKey {
		if until, ok := c.perUser[key]; ok && now.Before(until) {
			return false
		}
	}
	return true
}

// Reserve records that a real event was just accepted into the bounded
// queue or pending-approval queue - never called before acceptance, and
// never rolled back merely because a pending item is later rejected
// (rejection is not a do-over; the reservation is itself anti-spam
// protection - docs/audio-tts.md §11). A zero-or-negative duration
// leaves that cooldown untouched (treated as "disabled").
func (c *CooldownTracker) Reserve(hasKey bool, key CooldownKey, now time.Time, perUserCooldown, globalCooldown time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if globalCooldown > 0 {
		c.globalNext = now.Add(globalCooldown)
	}
	if hasKey && perUserCooldown > 0 {
		c.perUser[key] = now.Add(perUserCooldown)
	}
}
