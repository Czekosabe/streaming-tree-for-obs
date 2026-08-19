package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"sync"
	"time"
)

// Session lifetime bounds (docs/remote-management.md §10): a
// forgotten open tab does not stay authenticated indefinitely, and a
// genuine working session is not interrupted mid-task by an
// aggressively short idle window.
const (
	IdleTimeout      = 30 * time.Minute
	AbsoluteLifetime = 12 * time.Hour

	// sessionIDLen and csrfTokenLen are each independently generated
	// from crypto/rand - 256 bits of entropy, far beyond any practical
	// guessing budget, and the CSRF token is never derived from the
	// session ID (docs/remote-management.md §10).
	sessionIDLen = 32
	csrfTokenLen = 32
)

// ErrSessionNotFound covers both "never existed" and "evicted after
// expiry" - a caller must not be able to distinguish the two, since
// both mean exactly the same thing to an unauthenticated request:
// "there is no valid session here."
var ErrSessionNotFound = errors.New("session not found")

// Session is the bounded, non-secret-bearing state tracked per active
// login (docs/remote-management.md §10) - no IP/User-Agent/analytics
// metadata is ever added here.
type Session struct {
	ID           string
	CSRFToken    string
	IssuedAt     time.Time
	LastActivity time.Time
	ExpiresAt    time.Time
}

// Clock is the minimal time source SessionStore and LoginLimiter
// depend on, injected so expiry can be tested deterministically
// without a real sleep (docs/remote-management.md's own governing
// task explicitly forbids hour-long sleeps in tests).
type Clock interface {
	Now() time.Time
}

// realClock is the production Clock - time.Now(), nothing else.
type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// RealClock is the Clock every production caller (cmd/server) uses.
var RealClock Clock = realClock{}

// SessionStore is the in-memory, opaque, server-side session registry
// (docs/remote-management.md §10) - never a JWT, never persisted
// beyond process lifetime. Safe for concurrent use.
type SessionStore struct {
	clock Clock

	mu       sync.Mutex
	sessions map[string]Session
}

// NewSessionStore builds an empty store using clock as its time
// source.
func NewSessionStore(clock Clock) *SessionStore {
	if clock == nil {
		clock = RealClock
	}
	return &SessionStore{clock: clock, sessions: map[string]Session{}}
}

// Create generates a brand-new session (fresh ID, fresh CSRF token -
// never reused from any prior login) and stores it.
func (s *SessionStore) Create() (Session, error) {
	id, err := randomToken(sessionIDLen)
	if err != nil {
		return Session{}, err
	}
	csrfToken, err := randomToken(csrfTokenLen)
	if err != nil {
		return Session{}, err
	}

	now := s.clock.Now()
	session := Session{
		ID:           id,
		CSRFToken:    csrfToken,
		IssuedAt:     now,
		LastActivity: now,
		ExpiresAt:    now.Add(AbsoluteLifetime),
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[id] = session
	return session, nil
}

// Touch validates id and, if it is still valid, refreshes its idle
// timer and returns the (updated) session. Every expiry check
// (absolute lifetime, then idle timeout) happens here, on every
// lookup - there is no separate background sweep goroutine at this
// scale (docs/remote-management.md §10); an expired entry
// encountered here is evicted immediately as a side effect, so store
// size stays bounded by genuinely active sessions.
func (s *SessionStore) Touch(id string) (Session, error) {
	if id == "" {
		return Session{}, ErrSessionNotFound
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[id]
	if !ok {
		return Session{}, ErrSessionNotFound
	}

	now := s.clock.Now()
	if now.After(session.ExpiresAt) || now.Sub(session.LastActivity) > IdleTimeout {
		delete(s.sessions, id)
		return Session{}, ErrSessionNotFound
	}

	session.LastActivity = now
	s.sessions[id] = session
	return session, nil
}

// Delete removes a session unconditionally (logout) - a delete of an
// already-absent id is a harmless no-op, never an error, so a
// double-logout or a logout racing an idle expiry both behave
// identically from the caller's perspective.
func (s *SessionStore) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
}

// DeleteAll clears every active session - used by a local
// administrator-password reset (docs/remote-management.md §10: every
// active session must be invalidated by a reset).
func (s *SessionStore) DeleteAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions = map[string]Session{}
}

// Count reports the number of currently-tracked sessions, including
// ones that have not yet been lazily evicted by a Touch call - a test/
// diagnostic helper, not part of the security contract.
func (s *SessionStore) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessions)
}

// CheckCSRFToken performs a constant-time comparison of candidate
// against session's own CSRF token.
func CheckCSRFToken(session Session, candidate string) bool {
	if candidate == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(session.CSRFToken), []byte(candidate)) == 1
}

// randomToken returns a base64url (no padding), crypto/rand-backed
// token of n raw bytes - used for both session IDs and CSRF tokens,
// each call independent.
func randomToken(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
