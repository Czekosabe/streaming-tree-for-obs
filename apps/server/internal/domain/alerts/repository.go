package alerts

import "context"

// Repository is the persistence port for alert profiles and alert
// rules, including a rule's provider/account filters. Every write that
// touches more than one table is atomic (see the sqlite implementation)
// - a caller never observes a rule with a provider filter row but no
// canonical row.
type Repository interface {
	CreateProfile(ctx context.Context, p Profile) (Profile, error)
	GetProfile(ctx context.Context, id string) (Profile, bool, error)
	GetProfileByPublicSlug(ctx context.Context, slug string) (Profile, bool, error)
	ListProfiles(ctx context.Context) ([]Profile, error)
	// UpdateProfile replaces every editable field. id, PublicSlug, and
	// CreatedAt are unchanged by this call - see RotatePublicSlug for the
	// only way PublicSlug ever changes.
	UpdateProfile(ctx context.Context, p Profile) (Profile, error)
	// RotatePublicSlug atomically replaces a profile's public slug and
	// returns the updated profile - the previous slug stops resolving
	// immediately (see NewPublicSlug's own doc comment).
	RotatePublicSlug(ctx context.Context, id, newSlug string) (Profile, error)
	// DeleteProfile removes a profile; its rules (and their filters)
	// cascade.
	DeleteProfile(ctx context.Context, id string) error

	CreateRule(ctx context.Context, r Rule) (Rule, error)
	GetRule(ctx context.Context, id string) (Rule, bool, error)
	ListRules(ctx context.Context, profileID string) ([]Rule, error)
	// UpdateRule replaces every editable field and the full provider/
	// account filter sets - never a partial patch. id, ProfileID and
	// CreatedAt are unchanged.
	UpdateRule(ctx context.Context, r Rule) (Rule, error)
	// DeleteRule removes a rule; its filters cascade.
	DeleteRule(ctx context.Context, id string) error
}
