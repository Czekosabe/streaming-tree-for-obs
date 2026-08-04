package secrets

import "testing"

func TestBuildKeyFormat(t *testing.T) {
	got := BuildKey(SecretTypeDestinationStreamKey, "pf_abc123")
	want := "destination-stream-key:pf_abc123"
	if got != want {
		t.Errorf("BuildKey() = %q, want %q", got, want)
	}
}

func TestBuildKeyIsStableAcrossCalls(t *testing.T) {
	first := BuildKey(SecretTypeDestinationStreamKey, "pf_abc123")
	second := BuildKey(SecretTypeDestinationStreamKey, "pf_abc123")
	if first != second {
		t.Errorf("BuildKey() is not stable: %q != %q", first, second)
	}
}

func TestBuildKeyDependsOnlyOnTypeAndSubjectID(t *testing.T) {
	// The key must be derived purely from the platform's generated ID, never
	// from anything a user can change - such as a display name - so
	// renaming a platform can never orphan its secret.
	a := BuildKey(SecretTypeDestinationStreamKey, "pf_same_id")
	b := BuildKey(SecretTypeDestinationStreamKey, "pf_same_id")
	if a != b {
		t.Errorf("BuildKey() varied for the same subject ID: %q != %q", a, b)
	}
}

func TestBuildKeyGivesDifferentDestinationsIndependentKeys(t *testing.T) {
	// Two configured destinations for the same provider must never share a
	// credential: the provider ID alone is not part of the key at all, only
	// each platform's own generated ID.
	first := BuildKey(SecretTypeDestinationStreamKey, "pf_twitch_one")
	second := BuildKey(SecretTypeDestinationStreamKey, "pf_twitch_two")
	if first == second {
		t.Errorf("two distinct platform IDs produced the same key: %q", first)
	}
}

func TestBuildKeyNamespacesByType(t *testing.T) {
	// Different secret types for the same subject must never collide, so a
	// future OAuth token can reuse this abstraction safely.
	streamKey := BuildKey(SecretTypeDestinationStreamKey, "pf_abc123")
	other := BuildKey(SecretType("oauth-token"), "pf_abc123")
	if streamKey == other {
		t.Errorf("different secret types produced the same key: %q", streamKey)
	}
}

func TestBuildKeyNeverEmbedsSecretTypeOrValueBeyondTheType(t *testing.T) {
	// The key name itself must carry no more than the secret type and the
	// subject ID - never a display name or any part of the secret value.
	key := BuildKey(SecretTypeDestinationStreamKey, "pf_abc123")
	if key != "destination-stream-key:pf_abc123" {
		t.Errorf("BuildKey() = %q, contains unexpected content", key)
	}
}
