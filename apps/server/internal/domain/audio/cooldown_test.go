package audio

import (
	"testing"
	"time"
)

func TestNewCooldownKeyRequiresProviderAndUser(t *testing.T) {
	if _, ok := NewCooldownKey("", "src", "u1"); ok {
		t.Error("NewCooldownKey() ok = true with empty providerID, want false")
	}
	if _, ok := NewCooldownKey("twitch", "src", ""); ok {
		t.Error("NewCooldownKey() ok = true with empty userID, want false")
	}
	if _, ok := NewCooldownKey("twitch", "", "u1"); !ok {
		t.Error("NewCooldownKey() ok = false with empty sourceID (optional), want true")
	}
}

func TestCooldownTrackerGlobalCooldown(t *testing.T) {
	now := time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC)
	tr := NewCooldownTracker()

	if !tr.Allowed(false, CooldownKey{}, now) {
		t.Fatal("Allowed() = false before any reservation, want true")
	}
	tr.Reserve(false, CooldownKey{}, now, 0, 3*time.Second)

	if tr.Allowed(false, CooldownKey{}, now.Add(time.Second)) {
		t.Error("Allowed() = true within the global cooldown window, want false")
	}
	if !tr.Allowed(false, CooldownKey{}, now.Add(3*time.Second)) {
		t.Error("Allowed() = false exactly at the global cooldown boundary, want true")
	}
}

func TestCooldownTrackerPerUserCooldownIndependentOfOtherUsers(t *testing.T) {
	now := time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC)
	tr := NewCooldownTracker()
	keyA, _ := NewCooldownKey("twitch", "acct_1", "userA")
	keyB, _ := NewCooldownKey("twitch", "acct_1", "userB")

	tr.Reserve(true, keyA, now, 30*time.Second, 0)

	if tr.Allowed(true, keyA, now.Add(time.Second)) {
		t.Error("Allowed() = true for userA within their own cooldown, want false")
	}
	if !tr.Allowed(true, keyB, now.Add(time.Second)) {
		t.Error("Allowed() = false for userB, want true (per-user cooldowns are independent)")
	}
}

func TestCooldownTrackerAnonymousUsesGlobalOnly(t *testing.T) {
	now := time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC)
	tr := NewCooldownTracker()

	// hasKey=false (anonymous/no stable identity): Reserve must only ever
	// touch the global cooldown, never fabricate a per-user entry.
	tr.Reserve(false, CooldownKey{}, now, 30*time.Second, 3*time.Second)

	if tr.Allowed(false, CooldownKey{}, now.Add(time.Second)) {
		t.Error("Allowed() = true within the global cooldown, want false")
	}
	if !tr.Allowed(false, CooldownKey{}, now.Add(3*time.Second)) {
		t.Error("Allowed() = false after the global cooldown elapsed, want true")
	}
}

func TestCooldownTrackerZeroDurationDisablesThatCooldown(t *testing.T) {
	now := time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC)
	tr := NewCooldownTracker()
	key, _ := NewCooldownKey("twitch", "acct_1", "userA")

	tr.Reserve(true, key, now, 0, 0)

	if !tr.Allowed(true, key, now) {
		t.Error("Allowed() = false immediately after a zero-duration reservation, want true")
	}
}
