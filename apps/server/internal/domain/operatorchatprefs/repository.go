package operatorchatprefs

import (
	"context"
	"time"
)

// Repository is the persistence port for operator-chat preferences,
// per-account visibility, and the hidden-user/bot-user lists.
type Repository interface {
	// GetPreferences returns the singleton preferences row. found is false
	// when no row has ever been written - callers treat that identically to
	// Default().
	GetPreferences(ctx context.Context) (Preferences, bool, error)

	// SetPreferences replaces the singleton preferences row in full - the
	// API this backs is a full-replacement PUT, never a partial patch.
	SetPreferences(ctx context.Context, p Preferences, now time.Time) (Preferences, error)

	// ListAccountVisibility returns every account with an explicit
	// visibility preference recorded. An account absent from this list is
	// visible by default.
	ListAccountVisibility(ctx context.Context) ([]AccountVisibility, error)

	// SetAccountVisibility creates or replaces one account's visibility
	// preference.
	SetAccountVisibility(ctx context.Context, accountID string, visible bool, now time.Time) (AccountVisibility, error)

	// ListHiddenUsers returns every operator-hidden user, in a stable order.
	ListHiddenUsers(ctx context.Context) ([]UserRef, error)
	// AddHiddenUser adds one user to the hidden list, idempotently: adding
	// an already-listed provider/connected-account/provider-user-id tuple
	// returns the existing entry unchanged rather than a duplicate or an
	// error.
	AddHiddenUser(ctx context.Context, ref UserRef, now time.Time) (UserRef, error)
	// RemoveHiddenUser removes one hidden-user entry by its own id.
	// Removing an absent entry returns ErrUserNotFound.
	RemoveHiddenUser(ctx context.Context, id string) error

	// ListBotUsers returns every operator-marked bot user, in a stable
	// order.
	ListBotUsers(ctx context.Context) ([]UserRef, error)
	// AddBotUser adds one user to the bot list, idempotently - see
	// AddHiddenUser's own doc comment.
	AddBotUser(ctx context.Context, ref UserRef, now time.Time) (UserRef, error)
	// RemoveBotUser removes one bot-user entry by its own id. Removing an
	// absent entry returns ErrUserNotFound.
	RemoveBotUser(ctx context.Context, id string) error
}
