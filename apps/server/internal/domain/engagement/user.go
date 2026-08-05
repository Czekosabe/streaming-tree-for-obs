package engagement

// Badge is one badge a provider attached to a user for a specific event -
// carried as opaque, provider-defined identifiers. This package does not
// resolve a badge to an image; that is a future stage's concern (see
// docs/provider-integrations/twitch-engagement.md's "Areas reserved for
// later stages").
type Badge struct {
	SetID string
	ID    string
	Info  string
}

// User is the identity block attached to a user-caused event.
//
// Every field except ProviderUserID is genuinely optional: a provider may
// not report an avatar, a color, or any badge/role at all, and this model
// must not invent one. Anonymous carries its own explicit signal rather
// than being inferred from an empty ProviderUserID, because "the provider
// told us this was anonymous" and "we don't know who this is" are different
// facts.
type User struct {
	ProviderUserID string
	Login          string
	DisplayName    string
	// AvatarURL is left empty when the provider's own event payload did not
	// include one - never backfilled with a separate profile lookup (see
	// the stage task's explicit "no one profile request per chat message"
	// requirement).
	AvatarURL string
	// Color is the user's chat display color, when the provider reports
	// one. Empty when not reported - never defaulted to a color this
	// application invents.
	Color     string
	Badges    []Badge
	Roles     []Role
	Anonymous bool
}

// HasRole reports whether the user carries the given role.
func (u User) HasRole(r Role) bool {
	for _, have := range u.Roles {
		if have == r {
			return true
		}
	}
	return false
}
