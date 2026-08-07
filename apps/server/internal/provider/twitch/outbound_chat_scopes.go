package twitch

// OutboundChatScopeProfile is the Stage 11A "outbound-chat" scope set: the
// minimum current scope required to send a chat message through the Send
// Chat Message API - see
// docs/provider-integrations/twitch-outbound-chat.md.
//
// Deliberately additive, never merged into RequiredScope (metadata.go),
// EngagementScopeProfile (engagement_scopes.go), or account.Service's own
// RequiredScopes map: an account must stay healthy for metadata publishing
// and inbound engagement whether or not it has ever been asked to grant
// this - see AssessOutboundChatCapability. Deliberately does not include
// user:bot or channel:bot - this stage sends as the connected account
// itself through a User Access Token, never as a separate bot identity
// (see the contract document's "Selected token type and scope" section).
var OutboundChatScopeProfile = []string{
	"user:write:chat",
}

// AssessOutboundChatCapability reports whether an account's
// currently-granted scopes (as last confirmed by validation - see
// account.Account.Scopes) satisfy OutboundChatScopeProfile, independently
// of metadata and inbound-engagement capability health.
func AssessOutboundChatCapability(grantedScopes []string) CapabilityAssessment {
	return assessCapability(grantedScopes, OutboundChatScopeProfile)
}

// UnionScopesWithOutboundChat returns the union of the account's current
// scopes and OutboundChatScopeProfile, with the same stable, deduplicated,
// existing-first ordering UnionScopes uses for the engagement profile. Used
// to build the scope list an outbound-chat permission-upgrade device-flow
// attempt requests - never narrows what the account can already do.
func UnionScopesWithOutboundChat(existing []string) []string {
	return unionScopes(existing, OutboundChatScopeProfile)
}
