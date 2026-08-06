package chatoverlay

import (
	"context"
	"time"
)

// Repository is the persistence port for chat-overlay profiles, their
// selected accounts, hidden users, blocked terms, and activity-type
// selection.
type Repository interface {
	CreateProfile(ctx context.Context, p Profile) (Profile, error)
	GetProfile(ctx context.Context, id string) (Profile, bool, error)
	// GetProfileByPublicSlug looks up a profile by its current public
	// slug - returns found=false for an unknown or since-rotated slug.
	GetProfileByPublicSlug(ctx context.Context, slug string) (Profile, bool, error)
	ListProfiles(ctx context.Context) ([]Profile, error)
	// UpdateProfile replaces every editable field in full - never a
	// partial patch. The id, public slug, and created_at are unchanged.
	UpdateProfile(ctx context.Context, p Profile) (Profile, error)
	// DeleteProfile removes a profile; every related row (accounts,
	// hidden users, blocked terms, activity types) cascades.
	DeleteProfile(ctx context.Context, id string) error
	// RotatePublicSlug replaces one profile's public slug atomically -
	// the old slug stops resolving the instant this returns.
	RotatePublicSlug(ctx context.Context, id, newSlug string, now time.Time) (Profile, error)

	// ListAccounts returns the connected-account ids explicitly selected
	// for one overlay. An empty result means "all currently available
	// accounts" - see internal/domain/chatoverlay's own doc comment.
	ListAccounts(ctx context.Context, overlayID string) ([]string, error)
	// SetAccounts replaces the full selected-account set for one overlay.
	SetAccounts(ctx context.Context, overlayID string, accountIDs []string) error

	ListHiddenUsers(ctx context.Context, overlayID string) ([]HiddenUser, error)
	// AddHiddenUser adds one user to one overlay's hidden list,
	// idempotently - adding an already-listed provider/account/provider-
	// user-id tuple returns the existing entry unchanged.
	AddHiddenUser(ctx context.Context, ref HiddenUser, now time.Time) (HiddenUser, error)
	// RemoveHiddenUser removes one hidden-user entry by its identity
	// tuple. Removing an absent entry returns ErrUserNotFound.
	RemoveHiddenUser(ctx context.Context, overlayID string, providerID ProviderID, connectedAccountID, providerUserID string) error

	ListBlockedTerms(ctx context.Context, overlayID string) ([]BlockedTerm, error)
	// AddBlockedTerm adds one term to one overlay, idempotently by its
	// normalized (case-folded, trimmed) value - adding an
	// already-present term returns the existing entry unchanged.
	AddBlockedTerm(ctx context.Context, term BlockedTerm, now time.Time) (BlockedTerm, error)
	// RemoveBlockedTerm removes one blocked-term entry by its own id.
	// Removing an absent entry returns ErrTermNotFound.
	RemoveBlockedTerm(ctx context.Context, overlayID, id string) error

	// ListActivityTypes returns the activity types explicitly selected
	// for one overlay. An empty result means "show every activity type."
	ListActivityTypes(ctx context.Context, overlayID string) ([]string, error)
	// SetActivityTypes replaces the full activity-type selection for one
	// overlay.
	SetActivityTypes(ctx context.Context, overlayID string, activityTypes []string) error
}
