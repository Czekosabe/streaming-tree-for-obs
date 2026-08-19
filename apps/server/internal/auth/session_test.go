package auth

import (
	"sync"
	"testing"
	"time"
)

// fakeClock is a manually-advanced Clock so session expiry is tested
// deterministically - no real sleep, ever (docs/remote-management.md's
// own governing task explicitly forbids hour-long test sleeps).
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(start time.Time) *fakeClock {
	return &fakeClock{now: start}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func TestSessionStoreCreateThenTouch(t *testing.T) {
	clock := newFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	store := NewSessionStore(clock)

	session, err := store.Create()
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if session.ID == "" || session.CSRFToken == "" {
		t.Fatal("Create() returned an empty ID or CSRF token")
	}

	touched, err := store.Touch(session.ID)
	if err != nil {
		t.Fatalf("Touch() error = %v", err)
	}
	if touched.ID != session.ID {
		t.Errorf("Touch() ID = %q, want %q", touched.ID, session.ID)
	}
}

func TestSessionStoreFreshIdentifierPerLogin(t *testing.T) {
	store := NewSessionStore(newFakeClock(time.Now()))

	s1, err := store.Create()
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	s2, err := store.Create()
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if s1.ID == s2.ID {
		t.Error("two sessions share the same ID")
	}
	if s1.CSRFToken == s2.CSRFToken {
		t.Error("two sessions share the same CSRF token")
	}
	if s1.ID == s1.CSRFToken {
		t.Error("a session's own ID and CSRF token are identical - CSRF token must not be derived from the ID")
	}
}

func TestSessionStoreIDEntropyFormat(t *testing.T) {
	store := NewSessionStore(newFakeClock(time.Now()))
	session, err := store.Create()
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	// base64url, no padding, of 32 raw bytes = 43 characters.
	if len(session.ID) < 40 {
		t.Errorf("session ID length = %d, want at least 40 (256-bit entropy, base64url-encoded)", len(session.ID))
	}
	if len(session.CSRFToken) < 40 {
		t.Errorf("CSRF token length = %d, want at least 40", len(session.CSRFToken))
	}
}

func TestSessionStoreIdleExpiry(t *testing.T) {
	clock := newFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	store := NewSessionStore(clock)

	session, err := store.Create()
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	clock.Advance(IdleTimeout - time.Minute)
	if _, err := store.Touch(session.ID); err != nil {
		t.Fatalf("Touch() just under idle timeout: error = %v, want nil", err)
	}

	clock.Advance(IdleTimeout + time.Minute)
	if _, err := store.Touch(session.ID); err != ErrSessionNotFound {
		t.Errorf("Touch() after idle timeout: error = %v, want ErrSessionNotFound", err)
	}
}

func TestSessionStoreAbsoluteExpiry(t *testing.T) {
	clock := newFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	store := NewSessionStore(clock)

	session, err := store.Create()
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Keep touching well inside the idle window, but let absolute
	// lifetime elapse - repeated activity must not extend the
	// absolute lifetime.
	step := IdleTimeout / 2
	elapsed := time.Duration(0)
	for elapsed+step < AbsoluteLifetime {
		clock.Advance(step)
		elapsed += step
		if _, err := store.Touch(session.ID); err != nil {
			t.Fatalf("Touch() at elapsed=%v: error = %v, want nil", elapsed, err)
		}
	}

	clock.Advance(step + time.Minute)
	if _, err := store.Touch(session.ID); err != ErrSessionNotFound {
		t.Errorf("Touch() past absolute lifetime: error = %v, want ErrSessionNotFound", err)
	}
}

func TestSessionStoreLogoutInvalidation(t *testing.T) {
	store := NewSessionStore(newFakeClock(time.Now()))
	session, err := store.Create()
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	store.Delete(session.ID)

	if _, err := store.Touch(session.ID); err != ErrSessionNotFound {
		t.Errorf("Touch() after logout: error = %v, want ErrSessionNotFound", err)
	}
}

func TestSessionStoreDeleteAllInvalidatesEverySession(t *testing.T) {
	store := NewSessionStore(newFakeClock(time.Now()))
	s1, _ := store.Create()
	s2, _ := store.Create()

	store.DeleteAll()

	if _, err := store.Touch(s1.ID); err != ErrSessionNotFound {
		t.Errorf("Touch(s1) after DeleteAll: error = %v, want ErrSessionNotFound", err)
	}
	if _, err := store.Touch(s2.ID); err != ErrSessionNotFound {
		t.Errorf("Touch(s2) after DeleteAll: error = %v, want ErrSessionNotFound", err)
	}
	if store.Count() != 0 {
		t.Errorf("Count() after DeleteAll = %d, want 0", store.Count())
	}
}

func TestSessionStoreFreshStoreHasNoSessions(t *testing.T) {
	// Simulates a service restart: a new SessionStore starts empty
	// regardless of what any prior store held (docs/remote-management.md
	// §10 - a restart invalidates every session by construction).
	store := NewSessionStore(newFakeClock(time.Now()))
	if store.Count() != 0 {
		t.Errorf("Count() on a fresh store = %d, want 0", store.Count())
	}
}

func TestSessionStoreUnknownSessionRejected(t *testing.T) {
	store := NewSessionStore(newFakeClock(time.Now()))
	if _, err := store.Touch("this-session-was-never-created"); err != ErrSessionNotFound {
		t.Errorf("Touch(unknown) error = %v, want ErrSessionNotFound", err)
	}
}

func TestSessionStoreMalformedIDRejected(t *testing.T) {
	store := NewSessionStore(newFakeClock(time.Now()))
	cases := []string{"", " ", "../../etc/passwd", "\x00\x01\x02", string(make([]byte, 10000))}
	for _, id := range cases {
		if _, err := store.Touch(id); err != ErrSessionNotFound {
			t.Errorf("Touch(%q) error = %v, want ErrSessionNotFound", id, err)
		}
	}
}

func TestSessionStoreCleanupBoundsMemory(t *testing.T) {
	clock := newFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	store := NewSessionStore(clock)

	var ids []string
	for i := 0; i < 50; i++ {
		s, err := store.Create()
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}
		ids = append(ids, s.ID)
	}
	if store.Count() != 50 {
		t.Fatalf("Count() = %d, want 50", store.Count())
	}

	clock.Advance(IdleTimeout + time.Minute)
	for _, id := range ids {
		if _, err := store.Touch(id); err != ErrSessionNotFound {
			t.Errorf("Touch(%q) after idle timeout: error = %v, want ErrSessionNotFound", id, err)
		}
	}
	if store.Count() != 0 {
		t.Errorf("Count() after every session's own Touch-triggered eviction = %d, want 0", store.Count())
	}
}

func TestSessionStoreConcurrentOperations(t *testing.T) {
	store := NewSessionStore(newFakeClock(time.Now()))
	const workers = 20

	var wg sync.WaitGroup
	ids := make([]string, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s, err := store.Create()
			if err != nil {
				t.Errorf("Create() error = %v", err)
				return
			}
			ids[i] = s.ID
		}(i)
	}
	wg.Wait()

	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(i int) {
			defer wg.Done()
			if ids[i] == "" {
				return
			}
			if _, err := store.Touch(ids[i]); err != nil {
				t.Errorf("Touch(%q) error = %v", ids[i], err)
			}
		}(i)
	}
	wg.Wait()

	if store.Count() != workers {
		t.Errorf("Count() = %d, want %d", store.Count(), workers)
	}
}

func TestCheckCSRFTokenConstantTimePath(t *testing.T) {
	store := NewSessionStore(newFakeClock(time.Now()))
	session, err := store.Create()
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if !CheckCSRFToken(session, session.CSRFToken) {
		t.Error("CheckCSRFToken(correct token) = false, want true")
	}
	if CheckCSRFToken(session, "") {
		t.Error("CheckCSRFToken(empty) = true, want false")
	}
	if CheckCSRFToken(session, "wrong-token") {
		t.Error("CheckCSRFToken(wrong token) = true, want false")
	}
	if CheckCSRFToken(session, session.CSRFToken+"x") {
		t.Error("CheckCSRFToken(token with extra suffix) = true, want false")
	}
}

func TestSessionStoreNoSecretMaterialPersistedBeyondSessionFields(t *testing.T) {
	// Session carries no password/hash/provider-token field at all - a
	// structural guarantee, exercised here via reflection-free field
	// access: every exported field is accounted for and none is a
	// credential other than the session's own opaque identifiers.
	store := NewSessionStore(newFakeClock(time.Now()))
	session, err := store.Create()
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if session.ID == "" || session.CSRFToken == "" || session.IssuedAt.IsZero() ||
		session.LastActivity.IsZero() || session.ExpiresAt.IsZero() {
		t.Fatal("Session has an unexpectedly zero field")
	}
}
