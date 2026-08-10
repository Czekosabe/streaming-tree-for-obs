package chatautomation

import (
	"context"
	"errors"
	"time"

	"github.com/streaming-tree/server/internal/domain/account"
	domain "github.com/streaming-tree/server/internal/domain/chatautomation"
	"github.com/streaming-tree/server/internal/domain/operatorchatprefs"
	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/runtime/mediamtx"
)

// AccountLookupAdapter adapts *account.Service to
// domain/chatautomation.AccountLookup - the small, concrete bridge
// between this package's own dependencies and the domain package's
// deliberately decoupled, primitive-typed interface.
type AccountLookupAdapter struct{ Accounts *account.Service }

func (a AccountLookupAdapter) AccountProviderID(ctx context.Context, accountID string) (string, bool, error) {
	acc, err := a.Accounts.GetAccount(ctx, accountID)
	if err != nil {
		if errors.Is(err, account.ErrNotFound) {
			return "", false, nil
		}
		return "", false, err
	}
	return string(acc.ProviderID), true, nil
}

// PlatformLookupAdapter adapts *platform.Service and *account.Service
// (for the account-link lookup) to domain/chatautomation.PlatformLookup.
type PlatformLookupAdapter struct {
	Platforms *platform.Service
	Accounts  *account.Service
}

func (p PlatformLookupAdapter) PlatformProviderID(ctx context.Context, platformID string) (string, bool, error) {
	pf, err := p.Platforms.Get(ctx, platformID)
	if err != nil {
		if errors.Is(err, platform.ErrNotFound) {
			return "", false, nil
		}
		return "", false, err
	}
	return string(pf.ProviderID), true, nil
}

func (p PlatformLookupAdapter) PlatformLinkedAccountID(ctx context.Context, platformID string) (string, bool, error) {
	link, found, err := p.Accounts.GetLink(ctx, platformID)
	if err != nil {
		return "", false, err
	}
	if !found {
		return "", false, nil
	}
	return link.AccountID, true, nil
}

// MediaMTXIngestChecker adapts *mediamtx.Supervisor to IngestChecker -
// "local Streaming Tree ingest" only (never Twitch stream.online, never
// an FFmpeg output branch) - see the Stage 11B task's own Part 8.
type MediaMTXIngestChecker struct{ Supervisor *mediamtx.Supervisor }

func (m MediaMTXIngestChecker) IsReceiving() bool {
	return m.Supervisor.Snapshot().Ingest.State == mediamtx.IngestReceiving
}

func (m MediaMTXIngestChecker) ReceivingSince() (time.Time, bool) {
	snap := m.Supervisor.Snapshot().Ingest
	if snap.State != mediamtx.IngestReceiving || snap.ConnectedAt == "" {
		return time.Time{}, false
	}
	t, err := platform.ParseTimestamp(snap.ConnectedAt)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// BotUserCheckerAdapter adapts *operatorchatprefs.Service to
// BotUserChecker - reuses the exact same operator-maintained bot-user
// list the operator Chat page itself already shows/edits (Part 9:
// "messages from explicitly marked bot users should not count").
type BotUserCheckerAdapter struct{ Prefs *operatorchatprefs.Service }

func (b BotUserCheckerAdapter) IsBotUser(ctx context.Context, providerID, connectedAccountID, providerUserID string) (bool, error) {
	users, err := b.Prefs.BotUsers(ctx)
	if err != nil {
		return false, err
	}
	for _, u := range users {
		if string(u.ProviderID) == providerID && u.ConnectedAccountID == connectedAccountID && u.ProviderUserID == providerUserID {
			return true, nil
		}
	}
	return false, nil
}

// NewDomainService builds the domain/chatautomation.Service from the
// same *account.Service and *platform.Service already constructed for
// the rest of the application - the one place account/platform-service
// wiring meets the automation domain package.
func NewDomainService(repo domain.Repository, accounts *account.Service, platforms *platform.Service) *domain.Service {
	return domain.NewService(repo, AccountLookupAdapter{Accounts: accounts}, PlatformLookupAdapter{Platforms: platforms, Accounts: accounts}, nil)
}
