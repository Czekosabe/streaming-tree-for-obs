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
	granted := make(map[string]bool, len(grantedScopes))
	for _, s := range grantedScopes {
		granted[s] = true
	}

	var missing []string
	for _, required := range EngagementScopeProfile {
		if !granted[required] {
			missing = append(missing, required)
		}
	}

	return CapabilityAssessment{
		Required:                  EngagementScopeProfile,
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
	seen := make(map[string]bool, len(existing)+len(EngagementScopeProfile))
	out := make([]string, 0, len(existing)+len(EngagementScopeProfile))
	for _, s := range existing {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, s := range EngagementScopeProfile {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
