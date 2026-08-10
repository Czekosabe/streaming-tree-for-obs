package alerts

import (
	"context"
	"errors"

	"github.com/streaming-tree/server/internal/domain/account"
	domain "github.com/streaming-tree/server/internal/domain/alerts"
)

// AccountLookupAdapter adapts *account.Service to
// domain/alerts.AccountLookup - the small, concrete bridge between this
// package's own dependencies and the domain package's deliberately
// decoupled, primitive-typed interface. Mirrors
// internal/chatautomation's own AccountLookupAdapter exactly.
type AccountLookupAdapter struct{ Accounts *account.Service }

func (a AccountLookupAdapter) AccountExists(ctx context.Context, accountID string) (bool, error) {
	_, err := a.Accounts.GetAccount(ctx, accountID)
	if err != nil {
		if errors.Is(err, account.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// NewDomainService builds the domain/alerts Service over a sqlite
// repository and the account-existence adapter above - the one place
// this package's runtime and its sibling domain package are wired
// together, mirroring internal/chatautomation.NewDomainService.
func NewDomainService(repo domain.Repository, accounts *account.Service) *domain.Service {
	return domain.NewService(repo, AccountLookupAdapter{Accounts: accounts}, nil)
}
