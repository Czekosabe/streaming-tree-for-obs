package twitch

import "testing"

func TestAssessOutboundChatCapabilityReportsAvailableWhenScopeGranted(t *testing.T) {
	assessment := AssessOutboundChatCapability([]string{"user:write:chat"})
	if !assessment.Available {
		t.Error("expected Available = true when user:write:chat is granted")
	}
	if assessment.PermissionUpgradeRequired {
		t.Error("expected PermissionUpgradeRequired = false")
	}
	if len(assessment.Missing) != 0 {
		t.Errorf("expected no missing scopes, got %v", assessment.Missing)
	}
}

func TestAssessOutboundChatCapabilityReportsUnavailableWithoutTheScope(t *testing.T) {
	assessment := AssessOutboundChatCapability([]string{"channel:manage:broadcast"})
	if assessment.Available {
		t.Error("expected Available = false for an account with only the metadata scope")
	}
	if !assessment.PermissionUpgradeRequired {
		t.Error("expected PermissionUpgradeRequired = true")
	}
	if len(assessment.Missing) != 1 || assessment.Missing[0] != "user:write:chat" {
		t.Errorf("expected Missing = [user:write:chat], got %v", assessment.Missing)
	}
}

func TestAssessOutboundChatCapabilityIsIndependentOfEngagementScopes(t *testing.T) {
	// An account with every engagement scope but not user:write:chat must
	// still report outbound chat unavailable - the two profiles are
	// independent, never conflated.
	assessment := AssessOutboundChatCapability(EngagementScopeProfile)
	if assessment.Available {
		t.Error("expected Available = false: engagement scopes alone never satisfy outbound chat")
	}
}

func TestUnionScopesWithOutboundChatPreservesExistingScopes(t *testing.T) {
	existing := []string{"channel:manage:broadcast", "user:read:chat"}
	union := UnionScopesWithOutboundChat(existing)

	for i, want := range existing {
		if union[i] != want {
			t.Fatalf("existing scopes must come first and stay in order: got %v, want %v first", union, existing)
		}
	}
	found := false
	for _, s := range union {
		if s == "user:write:chat" {
			found = true
		}
	}
	if !found {
		t.Errorf("UnionScopesWithOutboundChat result missing user:write:chat: %v", union)
	}
}

func TestUnionScopesWithOutboundChatNeverDropsAnExistingScope(t *testing.T) {
	existing := []string{"channel:manage:broadcast", "user:read:chat", "bits:read", "some:future:scope"}
	union := UnionScopesWithOutboundChat(existing)

	for _, want := range existing {
		found := false
		for _, s := range union {
			if s == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("UnionScopesWithOutboundChat dropped previously granted scope %q: %v", want, union)
		}
	}
}

func TestUnionScopesWithOutboundChatDeduplicates(t *testing.T) {
	existing := []string{"user:write:chat", "user:write:chat"}
	union := UnionScopesWithOutboundChat(existing)

	count := 0
	for _, s := range union {
		if s == "user:write:chat" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("user:write:chat appears %d times in union, want exactly once", count)
	}
}

func TestUnionScopesWithOutboundChatNeverIncludesBotScopes(t *testing.T) {
	union := UnionScopesWithOutboundChat([]string{"channel:manage:broadcast"})
	for _, s := range union {
		if s == "user:bot" || s == "channel:bot" {
			t.Fatalf("UnionScopesWithOutboundChat must never include %q - this stage sends as the connected account, never a bot identity", s)
		}
	}
}

func TestUnionScopesDoesNotRequestOutboundChatScopeReciprocalCheck(t *testing.T) {
	// The engagement union already has its own test that it never requests
	// user:write:chat (engagement_scopes_test.go); the reciprocal check
	// belongs here: the outbound-chat union must never request an
	// engagement-only scope either, confirming the two profiles stay
	// genuinely independent in both directions.
	union := UnionScopesWithOutboundChat([]string{"channel:manage:broadcast"})
	for _, s := range union {
		for _, engagementScope := range EngagementScopeProfile {
			if s == engagementScope {
				t.Fatalf("UnionScopesWithOutboundChat must never request engagement scope %q", s)
			}
		}
	}
}
