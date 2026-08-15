package alerts

import (
	"context"
	"errors"

	"github.com/streaming-tree/server/internal/domain/account"
	domain "github.com/streaming-tree/server/internal/domain/alerts"
	"github.com/streaming-tree/server/internal/domain/audioasset"
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

// AudioAssetLookupAdapter adapts *audioasset.Service to
// domain/alerts.AudioAssetLookup (docs/alert-audio.md §5/§7) - the small,
// concrete bridge between this package's own Stage 17B dependency and the
// domain package's deliberately decoupled, primitive-typed interface,
// mirroring AccountLookupAdapter's identical role above.
type AudioAssetLookupAdapter struct {
	Assets *audioasset.Service
}

func (a AudioAssetLookupAdapter) AudioAssetExists(ctx context.Context, assetID string) (bool, error) {
	_, err := a.Assets.Get(ctx, assetID)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, audioasset.ErrNotFound) {
		return false, nil
	}
	return false, err
}

func (a AudioAssetLookupAdapter) SetRuleAudioAssetRefs(ctx context.Context, ruleID string, assetIDs []string) error {
	return a.Assets.SetRuleAssetRefs(ctx, ruleID, assetIDs)
}

func (a AudioAssetLookupAdapter) ClearRuleAudioAssetRefs(ctx context.Context, ruleID string) error {
	return a.Assets.ClearRuleRefs(ctx, ruleID)
}

// NewDomainService builds the domain/alerts Service over a sqlite
// repository, the combined account/donation-source existence adapter, and
// the Stage 17B audio-asset adapter above - the one place this package's
// runtime and its sibling domain packages are wired together, mirroring
// internal/chatautomation.NewDomainService.
func NewDomainService(repo domain.Repository, accounts *account.Service, donationSources *donationsource.Service, audioAssets *audioasset.Service) *domain.Service {
	// A nil audioAssets must become a genuinely nil domain.AudioAssetLookup
	// interface, never a non-nil interface wrapping a nil
	// *audioasset.Service - the classic Go "typed nil" trap, which would
	// make domain/alerts's own `s.audioAssets != nil` guard pass and then
	// panic on the nil pointer the moment a method is called on it.
	var audioLookup domain.AudioAssetLookup
	if audioAssets != nil {
		audioLookup = AudioAssetLookupAdapter{Assets: audioAssets}
	}
	return domain.NewService(
		repo, AccountLookupAdapter{Accounts: accounts, DonationSources: donationSources},
		audioLookup, nil,
	)
}

// NewVisualDesignService builds the Stage 13A shared visualdesign
// Service over a sqlite repository - the analogous one-liner to
// NewDomainService above, kept here so cmd/server/main.go's own wiring
// stays a flat, uniform list of `alerts.NewXService(...)` calls.
func NewVisualDesignService(repo visualdesign.Repository) *visualdesign.Service {
	return visualdesign.NewService(repo, nil)
}
