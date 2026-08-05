package account

import (
	"context"
	"time"
)

// Repository is the persistence port for connected accounts, their scopes,
// destination links, and provider integration settings.
//
// No implementation of this interface may ever touch a network or a
// provider's API - see Provider (provider.go) for that boundary - and no
// method here ever reads or writes a token: that lives in the OS credential
// store via TokenBundle, addressed only by account ID.
type Repository interface {
	// GetAccount returns one account with its current scope set, or
	// ErrNotFound.
	GetAccount(ctx context.Context, id string) (Account, error)

	// FindByProviderIdentity looks up an account by its real provider
	// identity, so a repeated device-flow authorization for the same
	// provider user is recognized as a reconnect rather than a duplicate.
	FindByProviderIdentity(ctx context.Context, providerID ProviderID, providerUserID string) (Account, bool, error)

	// CreateAccount inserts a new account and its scopes in one transaction.
	CreateAccount(ctx context.Context, acc Account) error

	// UpdateAccount replaces an existing account's mutable fields and its
	// complete scope set in one transaction.
	UpdateAccount(ctx context.Context, acc Account) error

	// DeleteAccount removes the account row; platform_account_links cascade.
	DeleteAccount(ctx context.Context, id string) error

	// ListAccounts returns every connected account, ordered by creation
	// time.
	ListAccounts(ctx context.Context) ([]Account, error)

	// CountAccounts reports how many connected accounts exist for a
	// provider - used to decide whether changing that provider's Client ID
	// is currently safe.
	CountAccounts(ctx context.Context, providerID ProviderID) (int, error)

	// GetLink returns the account linked to a platform, if any.
	GetLink(ctx context.Context, platformID string) (Link, bool, error)

	// ListLinksByAccount returns every platform currently linked to one
	// account - the reverse of GetLink. Used by Service.Disconnect to find
	// which destinations need their own provider-specific remote-target
	// state (a YouTube broadcast selection, for instance) cleared before
	// the account itself, and platform_account_links, are removed.
	ListLinksByAccount(ctx context.Context, accountID string) ([]Link, error)

	// SetLink creates or replaces a platform's link.
	SetLink(ctx context.Context, platformID, accountID string, now time.Time) (Link, error)

	// DeleteLink removes a platform's link without touching the platform or
	// the account.
	DeleteLink(ctx context.Context, platformID string) error

	// GetIntegrationSettings returns the database-managed Client ID for a
	// provider, if one was ever saved.
	GetIntegrationSettings(ctx context.Context, providerID ProviderID) (IntegrationSettings, bool, error)

	// SetIntegrationSettings creates or replaces the database-managed
	// Client ID for a provider.
	SetIntegrationSettings(ctx context.Context, providerID ProviderID, clientID string, now time.Time) (IntegrationSettings, error)
}
