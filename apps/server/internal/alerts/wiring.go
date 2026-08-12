package alerts

import (
	"context"
	"errors"

	"github.com/streaming-tree/server/internal/domain/account"
	domain "github.com/streaming-tree/server/internal/domain/alerts"
	"github.com/streaming-tree/server/internal/domain/donationsource"
	"github.com/streaming-tree/server/internal/domain/visualdesign"
)

// AccountLookupAdapter adapts *account.Service and (Stage 16A)
// *donationsource.Service to domain/alerts.AccountLookup - the small,
// concrete bridge between this package's own dependencies and the domain
// package's deliberately decoupled, primitive-typed interface. Mirrors
// internal/chatautomation's own AccountLookupAdapter, extended to also
// recognize a donation source id, since Rule.Accounts may hold either
// kind of id (see docs/domain/alerts.Rule's own doc comment and
// docs/provider-integrations/external-donations.md).
type AccountLookupAdapter struct {
	Accounts        *account.Service
	DonationSources *donationsource.Service
}

func (a AccountLookupAdapter) AccountExists(ctx context.Context, accountID string) (bool, error) {
	_, err := a.Accounts.GetAccount(ctx, accountID)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, account.ErrNotFound) {
		return false, err
	}
	if a.DonationSources == nil {
		return false, nil
	}
	return a.DonationSources.SourceExists(ctx, accountID)
}

// NewDomainService builds the domain/alerts Service over a sqlite
// repository and the combined account/donation-source existence adapter
// above - the one place this package's runtime and its sibling domain
// packages are wired together, mirroring
// internal/chatautomation.NewDomainService.
func NewDomainService(repo domain.Repository, accounts *account.Service, donationSources *donationsource.Service) *domain.Service {
	return domain.NewService(repo, AccountLookupAdapter{Accounts: accounts, DonationSources: donationSources}, nil)
}

// NewVisualDesignService builds the Stage 13A shared visualdesign
// Service over a sqlite repository - the analogous one-liner to
// NewDomainService above, kept here so cmd/server/main.go's own wiring
// stays a flat, uniform list of `alerts.NewXService(...)` calls.
func NewVisualDesignService(repo visualdesign.Repository) *visualdesign.Service {
	return visualdesign.NewService(repo, nil)
}
