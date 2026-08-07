package twitch

// EngagementScopeProfile is the Stage 8A "inbound-engagement" scope set: the
// minimum current scopes required for the selected EventSub subscription
// types (see docs/provider-integrations/twitch-engagement.md).
//
// Deliberately additive, never merged into RequiredScope (metadata.go) or
// into account.Service's own RequiredScopes map: an account must stay
// healthy for metadata publishing whether or not it has ever been asked to
// grant these - see AssessEngagementCapability.
var EngagementScopeProfile = []string{
	"user:read:chat",
	"moderator:read:followers",
	"channel:read:subscriptions",
	"bits:read",
	"channel:read:redemptions",
}

// CapabilityAssessment compares an account's currently-granted scopes
// against a capability's required scope set, independently of the
// account's core (metadata) health.
type CapabilityAssessment struct {
	Required                  []string
	Granted                   []string
	Missing                   []string
	Available                 bool
	PermissionUpgradeRequired bool
}

// AssessEngagementCapability reports whether an account's currently-granted
// scopes (as last confirmed by validation - see account.Account.Scopes)
// satisfy EngagementScopeProfile.
func AssessEngagementCapability(grantedScopes []string) CapabilityAssessment {
	return assessCapability(grantedScopes, EngagementScopeProfile)
}

// assessCapability compares grantedScopes against an arbitrary required
// profile - the shared implementation behind AssessEngagementCapability and
// AssessOutboundChatCapability (see outbound_chat_scopes.go). Each
// capability keeps its own named wrapper rather than exposing this directly,
// so a call site can never accidentally pass the wrong profile by mistake.
func assessCapability(grantedScopes, profile []string) CapabilityAssessment {
	granted := make(map[string]bool, len(grantedScopes))
	for _, s := range grantedScopes {
		granted[s] = true
	}

	var missing []string
	for _, required := range profile {
		if !granted[required] {
			missing = append(missing, required)
		}
	}

	return CapabilityAssessment{
		Required:                  profile,
		Granted:                   grantedScopes,
		Missing:                   missing,
		Available:                 len(missing) == 0,
		PermissionUpgradeRequired: len(missing) > 0,
	}
}

// UnionScopes returns the union of the account's current scopes and
// EngagementScopeProfile, with stable, deduplicated ordering: existing
// scopes first (so an upgrade attempt's requested set always visibly
// contains what the account already had), then any newly-needed engagement
// scope not already present. Used to build the scope list a permission-
// upgrade device-flow attempt requests - see
// internal/runtime/twitchengagement and
// docs/provider-integrations/twitch-engagement.md's scope-profile design
// decision.
func UnionScopes(existing []string) []string {
	return unionScopes(existing, EngagementScopeProfile)
}

// unionScopes returns the union of existing and profile, with stable,
// deduplicated ordering: existing scopes first, then any profile scope not
// already present. Shared by UnionScopes and
// UnionScopesWithOutboundChat (outbound_chat_scopes.go).
func unionScopes(existing, profile []string) []string {
	seen := make(map[string]bool, len(existing)+len(profile))
	out := make([]string, 0, len(existing)+len(profile))
	for _, s := range existing {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, s := range profile {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
