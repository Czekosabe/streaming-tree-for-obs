package goals

import (
	"context"
	"errors"

	"github.com/streaming-tree/server/internal/domain/account"
	"github.com/streaming-tree/server/internal/domain/donationsource"
)

// SourceLookupAdapter adapts *account.Service and *donationsource.Service
// to domain/goals.AccountLookup - mirrors internal/alerts's own
// AccountLookupAdapter exactly (docs/goals-widgets.md §14): a goal's
// Accounts filter may hold either a connected_accounts id or a donation
// source id.
type SourceLookupAdapter struct {
	Accounts        *account.Service
	DonationSources *donationsource.Service
}

func (a SourceLookupAdapter) AccountExists(ctx context.Context, accountID string) (bool, error) {
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
