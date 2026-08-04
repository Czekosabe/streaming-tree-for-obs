package twitch

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/streaming-tree/server/internal/domain/account"
	"github.com/streaming-tree/server/internal/domain/platform"
)

// RequiredScope is the one OAuth scope this stage's metadata publishing
// needs - see docs/provider-integrations/twitch.md for why nothing broader
// is requested.
const RequiredScope = "channel:manage:broadcast"

// Publish blocker identifiers - stable, frontend-localized, matching the
// style already established by internal/runtime/branch's Blocker
// constants.
const (
	BlockerAccountNotLinked           = "account_not_linked"
	BlockerAccountReconnectRequired   = "account_reconnect_required"
	BlockerCredentialStoreUnavailable = "credential_store_unavailable"
	BlockerMissingScope               = "missing_required_scope"
	BlockerCategoryNotSelected        = "category_not_selected"
	BlockerProviderUnavailable        = "provider_unavailable"
	BlockerRateLimited                = "rate_limited"
)

// FieldDiff is one metadata field's local-vs-remote comparison.
type FieldDiff struct {
	Field   string
	Local   string
	Remote  string
	Changed bool
}

// Preview is the non-secret publish-preview payload.
type Preview struct {
	AccountID    string
	AccountLogin string
	Fields       []FieldDiff
	Skipped      []string
	Blockers     []string
	Allowed      bool
}

// PublishResult is the non-secret outcome of a successful publish.
type PublishResult struct {
	AccountID     string
	PublishedAt   time.Time
	FieldsChanged []string
	FieldsSkipped []string
	Warnings      []string
}

// MetadataService coordinates a configured platform's saved metadata, its
// linked connected account, and this package's Helix client to preview and
// publish to Twitch.
//
// Every provider call goes through account.Service.WithFreshToken, so a
// single-flight refresh and one retry on 401 is applied uniformly here,
// exactly like every other Twitch-calling path.
type MetadataService struct {
	accounts *account.Service
	client   *Client
}

// NewMetadataService builds a MetadataService.
func NewMetadataService(accounts *account.Service, client *Client) *MetadataService {
	return &MetadataService{accounts: accounts, client: client}
}

// resolvedAccount is what both Preview and Publish need before making any
// Twitch call.
type resolvedAccount struct {
	account  account.Account
	clientID string
}

// resolve returns the linked account and blockers that stop publishing
// before any network call is made. A non-empty blocker list is not an
// error: it is the normal, structured answer for "not eligible right now."
func (s *MetadataService) resolve(ctx context.Context, platformProviderID string, link account.Link, linked bool) (resolvedAccount, []string) {
	if platformProviderID != string(account.ProviderTwitch) {
		// Only Twitch has a metadata adapter in this stage; any other
		// provider is handled entirely by the frontend's "not implemented
		// yet" state before this service is ever called, so reaching here
		// with another provider is a caller error, not a normal blocker.
		return resolvedAccount{}, []string{BlockerAccountNotLinked}
	}
	if !linked {
		return resolvedAccount{}, []string{BlockerAccountNotLinked}
	}

	acc, err := s.accounts.GetAccount(ctx, link.AccountID)
	if err != nil {
		if errors.Is(err, account.ErrNotFound) {
			return resolvedAccount{}, []string{BlockerAccountNotLinked}
		}
		return resolvedAccount{}, []string{BlockerProviderUnavailable}
	}
	if acc.Status == account.StatusReconnectRequired {
		return resolvedAccount{}, []string{BlockerAccountReconnectRequired}
	}
	if !acc.HasScope(RequiredScope) {
		return resolvedAccount{}, []string{BlockerMissingScope}
	}

	clientID, err := s.accounts.EffectiveClientID(ctx, acc.ProviderID)
	if err != nil {
		return resolvedAccount{}, []string{BlockerProviderUnavailable}
	}

	return resolvedAccount{account: acc, clientID: clientID}, nil
}

func fieldDiffs(local platform.Metadata, remote Channel) []FieldDiff {
	diffs := []FieldDiff{
		{Field: "title", Local: local.Title, Remote: remote.Title, Changed: local.Title != remote.Title},
		{Field: "category", Local: local.Category, Remote: remote.GameName, Changed: local.CategoryID != remote.GameID},
		{Field: "language", Local: local.Language, Remote: remote.Language, Changed: local.Language != remote.Language},
		{Field: "tags", Local: strings.Join(local.Tags, ", "), Remote: strings.Join(remote.Tags, ", "), Changed: !sameTags(local.Tags, remote.Tags)},
	}
	return diffs
}

func sameTags(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	sortedA := append([]string(nil), a...)
	sortedB := append([]string(nil), b...)
	sort.Strings(sortedA)
	sort.Strings(sortedB)
	for i := range sortedA {
		if !strings.EqualFold(sortedA[i], sortedB[i]) {
			return false
		}
	}
	return true
}

// SearchCategories searches Twitch categories for a directly-specified
// connected account (GET /api/connected-accounts/{id}/twitch/categories) -
// used by the metadata editor's category picker, independent of whether
// that account is currently linked to any destination.
func (s *MetadataService) SearchCategories(ctx context.Context, accountID, query string) ([]Category, error) {
	acc, err := s.accounts.GetAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if acc.Status == account.StatusReconnectRequired {
		return nil, errBlockerOnly(BlockerAccountReconnectRequired)
	}
	clientID, err := s.accounts.EffectiveClientID(ctx, acc.ProviderID)
	if err != nil {
		return nil, mapProviderErr(err)
	}

	var (
		results []Category
		callErr error
	)
	err = s.accounts.WithFreshToken(ctx, accountID, func(token string) (bool, error) {
		r, searchErr := s.client.SearchCategories(ctx, query, token, clientID)
		if searchErr != nil {
			if errors.Is(searchErr, ErrUnauthorized) {
				return true, nil
			}
			callErr = searchErr
			return false, nil
		}
		results = r
		return false, nil
	})
	if err != nil {
		return nil, mapProviderErr(err)
	}
	if callErr != nil {
		return nil, mapProviderErr(callErr)
	}
	return results, nil
}

// Preview computes a publish preview without publishing anything.
func (s *MetadataService) Preview(ctx context.Context, platformProviderID string, local platform.Metadata, link account.Link, linked bool) (Preview, error) {
	resolved, blockers := s.resolve(ctx, platformProviderID, link, linked)
	if len(blockers) > 0 {
		return Preview{Blockers: blockers, Allowed: false, Skipped: skippedFields()}, nil
	}

	var (
		channel Channel
		callErr error
	)
	err := s.accounts.WithFreshToken(ctx, resolved.account.ID, func(token string) (bool, error) {
		ch, err := s.client.GetChannel(ctx, resolved.account.ProviderUserID, token, resolved.clientID)
		if err != nil {
			if errors.Is(err, ErrUnauthorized) {
				return true, nil
			}
			callErr = err
			return false, nil
		}
		channel = ch
		return false, nil
	})
	if err != nil {
		return Preview{}, mapProviderErr(err)
	}
	if callErr != nil {
		return Preview{}, mapProviderErr(callErr)
	}

	blockers = categoryBlockers(local)
	return Preview{
		AccountID: resolved.account.ID, AccountLogin: resolved.account.Login,
		Fields: fieldDiffs(local, channel), Skipped: skippedFields(), Blockers: blockers, Allowed: len(blockers) == 0,
	}, nil
}

func categoryBlockers(local platform.Metadata) []string {
	if strings.TrimSpace(local.Category) != "" && strings.TrimSpace(local.CategoryID) == "" {
		return []string{BlockerCategoryNotSelected}
	}
	return nil
}

// skippedFields names the metadata fields this application never sends to
// Twitch, because no real API this application uses supports them - see
// docs/provider-integrations/twitch.md.
func skippedFields() []string {
	return []string{"description", "visibility", "matureContent", "dvr", "latencyMode"}
}

// Publish sends the latest saved metadata to Twitch. Blockers (a non-empty
// return with a nil error) mean publishing did not happen and nothing was
// sent; an error means an infrastructure or provider failure.
func (s *MetadataService) Publish(ctx context.Context, platformProviderID string, local platform.Metadata, link account.Link, linked bool, now time.Time) (PublishResult, []string, error) {
	resolved, blockers := s.resolve(ctx, platformProviderID, link, linked)
	if len(blockers) > 0 {
		return PublishResult{}, blockers, nil
	}
	if blockers := categoryBlockers(local); len(blockers) > 0 {
		return PublishResult{}, blockers, nil
	}

	title := local.Title
	gameID := local.CategoryID
	language := local.Language
	input := ModifyChannelInput{Title: &title, GameID: &gameID, Language: &language, Tags: local.Tags}

	var callErr error
	err := s.accounts.WithFreshToken(ctx, resolved.account.ID, func(token string) (bool, error) {
		modifyErr := s.client.ModifyChannel(ctx, resolved.account.ProviderUserID, input, token, resolved.clientID)
		if modifyErr != nil {
			if errors.Is(modifyErr, ErrUnauthorized) {
				return true, nil
			}
			callErr = modifyErr
			return false, nil
		}
		return false, nil
	})
	if err != nil {
		return PublishResult{}, nil, mapProviderErr(err)
	}
	if callErr != nil {
		return PublishResult{}, nil, mapProviderErr(callErr)
	}

	return PublishResult{
		AccountID: resolved.account.ID, PublishedAt: now,
		FieldsChanged: []string{"title", "category", "language", "tags"},
		FieldsSkipped: skippedFields(),
	}, nil, nil
}

func mapProviderErr(err error) error {
	switch {
	case errors.Is(err, account.ErrReconnectRequired):
		return errBlockerOnly(BlockerAccountReconnectRequired)
	case errors.Is(err, ErrRateLimited):
		return errBlockerOnly(BlockerRateLimited)
	case errors.Is(err, ErrUnavailable), errors.Is(err, ErrForbidden):
		return errBlockerOnly(BlockerProviderUnavailable)
	default:
		return err
	}
}

// blockerErr lets mapProviderErr turn a provider-level failure into the same
// blocker vocabulary Preview/Publish already use, so callers only ever
// branch on one shape (blockers) instead of two.
type blockerErr struct{ blocker string }

func (e *blockerErr) Error() string { return "blocked: " + e.blocker }

func errBlockerOnly(blocker string) error { return &blockerErr{blocker: blocker} }

// AsBlocker extracts the blocker identifier from an error produced by
// mapProviderErr, if any.
func AsBlocker(err error) (string, bool) {
	var b *blockerErr
	if errors.As(err, &b) {
		return b.blocker, true
	}
	return "", false
}
