package twitch

import "testing"

func TestAssessEngagementCapabilityReportsAvailableWhenAllScopesGranted(t *testing.T) {
	assessment := AssessEngagementCapability(EngagementScopeProfile)
	if !assessment.Available {
		t.Error("expected Available = true when every required scope is granted")
	}
	if assessment.PermissionUpgradeRequired {
		t.Error("expected PermissionUpgradeRequired = false when every required scope is granted")
	}
	if len(assessment.Missing) != 0 {
		t.Errorf("expected no missing scopes, got %v", assessment.Missing)
	}
}

func TestAssessEngagementCapabilityReportsUnavailableWhenScopesMissing(t *testing.T) {
	assessment := AssessEngagementCapability([]string{"channel:manage:broadcast"})
	if assessment.Available {
		t.Error("expected Available = false for an account with only the metadata scope")
	}
	if !assessment.PermissionUpgradeRequired {
		t.Error("expected PermissionUpgradeRequired = true")
	}
	if len(assessment.Missing) != len(EngagementScopeProfile) {
		t.Errorf("expected every engagement scope missing, got %v", assessment.Missing)
	}
}

func TestAssessEngagementCapabilityReportsPartialMissingSet(t *testing.T) {
	assessment := AssessEngagementCapability([]string{
		"channel:manage:broadcast", "user:read:chat", "bits:read",
	})
	want := map[string]bool{"moderator:read:followers": true, "channel:read:subscriptions": true, "channel:read:redemptions": true}
	if len(assessment.Missing) != len(want) {
		t.Fatalf("expected %d missing scopes, got %v", len(want), assessment.Missing)
	}
	for _, m := range assessment.Missing {
		if !want[m] {
			t.Errorf("unexpected missing scope %q", m)
		}
	}
}

func TestAssessEngagementCapabilityNeverModifiesTheSharedProfileSlice(t *testing.T) {
	before := append([]string(nil), EngagementScopeProfile...)
	AssessEngagementCapability(nil)
	AssessEngagementCapability([]string{"user:read:chat"})
	for i, s := range EngagementScopeProfile {
		if s != before[i] {
			t.Fatalf("EngagementScopeProfile was mutated: got %v, want %v", EngagementScopeProfile, before)
		}
	}
}

func TestUnionScopesPreservesExistingScopesAndAddsMissingOnes(t *testing.T) {
	existing := []string{"channel:manage:broadcast"}
	union := UnionScopes(existing)

	if union[0] != "channel:manage:broadcast" {
		t.Fatalf("expected existing scope to come first, got %v", union)
	}
	for _, required := range EngagementScopeProfile {
		found := false
		for _, s := range union {
			if s == required {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("UnionScopes result missing required engagement scope %q: %v", required, union)
		}
	}
}

func TestUnionScopesNeverDropsAScopeTheAccountAlreadyHad(t *testing.T) {
	// A hypothetical extra scope this account somehow already has, not part
	// of any known profile - UnionScopes must never silently drop it, since
	// dropping a previously granted scope is exactly what the task forbids.
	existing := []string{"channel:manage:broadcast", "some:future:scope"}
	union := UnionScopes(existing)

	for _, want := range existing {
		found := false
		for _, s := range union {
			if s == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("UnionScopes dropped previously granted scope %q: %v", want, union)
		}
	}
}

func TestUnionScopesDeduplicates(t *testing.T) {
	existing := []string{"user:read:chat", "user:read:chat", "bits:read"}
	union := UnionScopes(existing)

	seen := map[string]int{}
	for _, s := range union {
		seen[s]++
	}
	for scope, count := range seen {
		if count > 1 {
			t.Errorf("scope %q appears %d times in union, want at most once", scope, count)
		}
	}
}

func TestUnionScopesDoesNotRequestOutboundChatScope(t *testing.T) {
	union := UnionScopes([]string{"channel:manage:broadcast"})
	for _, s := range union {
		if s == "user:write:chat" {
			t.Fatal("UnionScopes must never include user:write:chat - that belongs to stage 11 outbound chat, not stage 8A")
		}
	}
}
