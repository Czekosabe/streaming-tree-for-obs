package youtube

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/streaming-tree/server/internal/domain/account"
	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/domain/remotetarget"
)

// RequiredScope is the one OAuth scope this stage's metadata publishing
// needs - see docs/provider-integrations/youtube.md for why nothing broader
// is requested.
const RequiredScope = "https://www.googleapis.com/auth/youtube.force-ssl"

// Publish blocker identifiers - stable, frontend-localized, matching the
// vocabulary docs/provider-integrations/youtube.md and the task define.
const (
	BlockerAccountNotLinked           = "account_not_linked"
	BlockerAccountReconnectRequired   = "account_reconnect_required"
	BlockerCredentialStoreUnavailable = "credential_store_unavailable"
	BlockerMissingScope               = "missing_required_scope"
	BlockerBroadcastNotSelected       = "youtube_broadcast_not_selected"
	BlockerBroadcastNotFound          = "youtube_broadcast_not_found"
	BlockerLiveStreamingNotEnabled    = "youtube_live_streaming_not_enabled"
	BlockerRegionRequired             = "youtube_region_required"
	BlockerCategoryRequired           = "youtube_category_required"
	BlockerQuotaExceeded              = "youtube_quota_exceeded"
	BlockerUnavailable                = "youtube_unavailable"
)

// RegionRepository persists a connected account's explicit category-region
// override.
type RegionRepository interface {
	GetRegion(ctx context.Context, accountID string) (string, bool, error)
	SetRegion(ctx context.Context, accountID, region string, now time.Time) error
}

// FieldDiff is one metadata field's local-vs-remote comparison.
type FieldDiff struct {
	Field   string
	Local   string
	Remote  string
	Changed bool
}

// Preview is the non-secret publish-preview payload.
type Preview struct {
	AccountID      string
	AccountLogin   string
	BroadcastID    string
	BroadcastTitle string
	Fields         []FieldDiff
	Skipped        []string
	Blockers       []string
	Warnings       []string
	Allowed        bool
}

// PublishResult is the non-secret outcome of a publish attempt.
//
// This stage's publish path issues exactly one write (videos.update, see
// docs/provider-integrations/youtube.md's "Why no liveBroadcasts.update
// call exists this stage"), so Failed is always empty on the paths that
// return a result at all - a genuine failure surfaces as an error instead.
// The field still exists so a future stage adding a second write can report
// a real partial result without changing this shape.
type PublishResult struct {
	AccountID     string
	BroadcastID   string
	PublishedAt   time.Time
	FieldsChanged []string
	FieldsSkipped []string
	FieldsFailed  []string
	Warnings      []string
}

// MetadataService coordinates a configured platform's saved metadata, its
// linked connected account, its selected remote broadcast target, and this
// package's YouTube Data API client.
//
// Every provider call goes through account.Service.WithFreshToken, exactly
// like internal/provider/twitch's MetadataService, so "at most one refresh
// and one retry per 401" is enforced in one place.
type MetadataService struct {
	accounts *account.Service
	regions  RegionRepository
	client   *Client
}

// NewMetadataService builds a MetadataService.
func NewMetadataService(accounts *account.Service, regions RegionRepository, client *Client) *MetadataService {
	return &MetadataService{accounts: accounts, regions: regions, client: client}
}

type resolvedAccount struct {
	account  account.Account
	clientID string
}

func (s *MetadataService) resolveAccount(ctx context.Context, platformProviderID string, link account.Link, linked bool) (resolvedAccount, []string) {
	if platformProviderID != string(account.ProviderYouTube) {
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
		return resolvedAccount{}, []string{BlockerUnavailable}
	}
	if acc.Status == account.StatusReconnectRequired {
		return resolvedAccount{}, []string{BlockerAccountReconnectRequired}
	}
	if !acc.HasScope(RequiredScope) {
		return resolvedAccount{}, []string{BlockerMissingScope}
	}

	clientID, err := s.accounts.EffectiveClientID(ctx, acc.ProviderID)
	if err != nil {
		return resolvedAccount{}, []string{BlockerUnavailable}
	}

	return resolvedAccount{account: acc, clientID: clientID}, nil
}

func categoryBlockers(local platform.Metadata) []string {
	if strings.TrimSpace(local.Category) != "" && strings.TrimSpace(local.CategoryID) == "" {
		return []string{BlockerCategoryRequired}
	}
	return nil
}

// skippedFields names the metadata fields this application never sends to
// YouTube, because no real API this application uses supports them as a
// generic equivalent - see docs/provider-integrations/youtube.md.
func skippedFields() []string {
	return []string{"matureContent", "dvr", "latencyMode"}
}

// ListBroadcasts lists a connected account's active and upcoming broadcasts.
func (s *MetadataService) ListBroadcasts(ctx context.Context, accountID string) ([]Broadcast, error) {
	acc, err := s.accounts.GetAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if acc.Status == account.StatusReconnectRequired {
		return nil, errBlockerOnly(BlockerAccountReconnectRequired)
	}

	var (
		results []Broadcast
		callErr error
	)
	err = s.accounts.WithFreshToken(ctx, accountID, func(token string) (bool, error) {
		r, listErr := s.client.ListBroadcasts(ctx, token)
		if listErr != nil {
			if errors.Is(listErr, ErrUnauthorized) {
				return true, nil
			}
			callErr = listErr
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

// EffectiveRegion resolves the region used for category listing: an
// explicit saved override first, otherwise the connected channel's own
// country when YouTube reports one - see docs/provider-integrations/
// youtube.md's "Category region" section. Empty means neither is available
// and the operator must choose one explicitly.
func (s *MetadataService) EffectiveRegion(ctx context.Context, accountID string) (string, error) {
	if override, found, err := s.regions.GetRegion(ctx, accountID); err == nil && found && override != "" {
		return override, nil
	} else if err != nil {
		return "", err
	}

	acc, err := s.accounts.GetAccount(ctx, accountID)
	if err != nil {
		return "", err
	}
	if acc.Status == account.StatusReconnectRequired {
		return "", nil
	}

	var country string
	_ = s.accounts.WithFreshToken(ctx, accountID, func(token string) (bool, error) {
		ch, chErr := s.client.GetChannel(ctx, acc.ProviderUserID, token)
		if chErr != nil {
			if errors.Is(chErr, ErrUnauthorized) {
				return true, nil
			}
			return false, nil
		}
		country = ch.Country
		return false, nil
	})
	return country, nil
}

// SetRegion saves an explicit region override for a connected account.
func (s *MetadataService) SetRegion(ctx context.Context, accountID, region string, now time.Time) error {
	return s.regions.SetRegion(ctx, accountID, strings.ToUpper(strings.TrimSpace(region)), now)
}

// ListCategories lists assignable categories for an account's effective
// region.
func (s *MetadataService) ListCategories(ctx context.Context, accountID string) ([]Category, error) {
	region, err := s.EffectiveRegion(ctx, accountID)
	if err != nil {
		return nil, mapProviderErr(err)
	}
	if region == "" {
		return nil, errBlockerOnly(BlockerRegionRequired)
	}

	var (
		results []Category
		callErr error
	)
	err = s.accounts.WithFreshToken(ctx, accountID, func(token string) (bool, error) {
		r, listErr := s.client.ListCategories(ctx, region, token)
		if listErr != nil {
			if errors.Is(listErr, ErrUnauthorized) {
				return true, nil
			}
			callErr = listErr
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

func fieldDiffs(local platform.Metadata, remote Video) []FieldDiff {
	return []FieldDiff{
		{Field: "title", Local: local.Title, Remote: remote.Title, Changed: local.Title != remote.Title},
		{Field: "description", Local: local.Description, Remote: remote.Description, Changed: local.Description != remote.Description},
		{Field: "category", Local: local.Category, Remote: remote.CategoryID, Changed: local.CategoryID != remote.CategoryID},
		{Field: "tags", Local: strings.Join(local.Tags, ", "), Remote: strings.Join(remote.Tags, ", "), Changed: !sameTags(local.Tags, remote.Tags)},
		{Field: "language", Local: local.Language, Remote: remote.DefaultLanguage, Changed: local.Language != remote.DefaultLanguage},
		{Field: "visibility", Local: local.Visibility, Remote: remote.PrivacyStatus, Changed: local.Visibility != remote.PrivacyStatus},
	}
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

// Preview computes a publish preview without publishing anything.
func (s *MetadataService) Preview(
	ctx context.Context, platformProviderID string, local platform.Metadata,
	link account.Link, linked bool, target remotetarget.Target, hasTarget bool,
) (Preview, error) {
	resolved, blockers := s.resolveAccount(ctx, platformProviderID, link, linked)
	if len(blockers) > 0 {
		return Preview{Blockers: blockers, Allowed: false, Skipped: skippedFields()}, nil
	}
	if !hasTarget {
		return Preview{AccountID: resolved.account.ID, AccountLogin: resolved.account.Login,
			Blockers: []string{BlockerBroadcastNotSelected}, Allowed: false, Skipped: skippedFields()}, nil
	}

	var (
		video   Video
		callErr error
	)
	err := s.accounts.WithFreshToken(ctx, resolved.account.ID, func(token string) (bool, error) {
		v, getErr := s.client.GetVideo(ctx, target.ResourceID, token)
		if getErr != nil {
			if errors.Is(getErr, ErrUnauthorized) {
				return true, nil
			}
			callErr = getErr
			return false, nil
		}
		video = v
		return false, nil
	})
	if err != nil {
		return Preview{}, mapProviderErr(err)
	}
	if callErr != nil {
		if errors.Is(callErr, ErrInvalidResponse) {
			return Preview{AccountID: resolved.account.ID, AccountLogin: resolved.account.Login,
				Blockers: []string{BlockerBroadcastNotFound}, Allowed: false, Skipped: skippedFields()}, nil
		}
		return Preview{}, mapProviderErr(callErr)
	}

	blockers = categoryBlockers(local)
	warnings := []string{"testing_mode_seven_day_token", "not_verified_stream_key_binding"}
	return Preview{
		AccountID: resolved.account.ID, AccountLogin: resolved.account.Login,
		BroadcastID: target.ResourceID, BroadcastTitle: video.Title,
		Fields: fieldDiffs(local, video), Skipped: skippedFields(), Blockers: blockers, Warnings: warnings,
		Allowed: len(blockers) == 0,
	}, nil
}

// Publish sends the latest saved metadata to YouTube via a safe read-
// modify-write videos.update - see docs/provider-integrations/youtube.md's
// "Safe read-modify-write" section for why the current remote resource is
// always re-fetched immediately before the write rather than built from
// local fields alone.
func (s *MetadataService) Publish(
	ctx context.Context, platformProviderID string, local platform.Metadata,
	link account.Link, linked bool, target remotetarget.Target, hasTarget bool, now time.Time,
) (PublishResult, []string, error) {
	resolved, blockers := s.resolveAccount(ctx, platformProviderID, link, linked)
	if len(blockers) > 0 {
		return PublishResult{}, blockers, nil
	}
	if !hasTarget {
		return PublishResult{}, []string{BlockerBroadcastNotSelected}, nil
	}
	if blockers := categoryBlockers(local); len(blockers) > 0 {
		return PublishResult{}, blockers, nil
	}

	var (
		current Video
		callErr error
	)
	err := s.accounts.WithFreshToken(ctx, resolved.account.ID, func(token string) (bool, error) {
		v, getErr := s.client.GetVideo(ctx, target.ResourceID, token)
		if getErr != nil {
			if errors.Is(getErr, ErrUnauthorized) {
				return true, nil
			}
			callErr = getErr
			return false, nil
		}
		current = v
		return false, nil
	})
	if err != nil {
		return PublishResult{}, nil, mapProviderErr(err)
	}
	if callErr != nil {
		if errors.Is(callErr, ErrInvalidResponse) {
			return PublishResult{}, []string{BlockerBroadcastNotFound}, nil
		}
		return PublishResult{}, nil, mapProviderErr(callErr)
	}

	input := VideoUpdateInput{
		Title: local.Title, Description: local.Description, Tags: local.Tags,
		CategoryID: local.CategoryID, DefaultLanguage: local.Language, PrivacyStatus: local.Visibility,
	}

	var updateErr error
	err = s.accounts.WithFreshToken(ctx, resolved.account.ID, func(token string) (bool, error) {
		writeErr := s.client.UpdateVideo(ctx, current, input, token)
		if writeErr != nil {
			if errors.Is(writeErr, ErrUnauthorized) {
				return true, nil
			}
			updateErr = writeErr
			return false, nil
		}
		return false, nil
	})
	if err != nil {
		return PublishResult{}, nil, mapProviderErr(err)
	}
	if updateErr != nil {
		return PublishResult{}, nil, mapProviderErr(updateErr)
	}

	return PublishResult{
		AccountID: resolved.account.ID, BroadcastID: target.ResourceID, PublishedAt: now,
		FieldsChanged: []string{"title", "description", "category", "tags", "language", "visibility"},
		FieldsSkipped: skippedFields(),
	}, nil, nil
}

func mapProviderErr(err error) error {
	switch {
	case errors.Is(err, account.ErrReconnectRequired):
		return errBlockerOnly(BlockerAccountReconnectRequired)
	case errors.Is(err, ErrQuotaExceeded):
		return errBlockerOnly(BlockerQuotaExceeded)
	case errors.Is(err, ErrRateLimited):
		return errBlockerOnly(BlockerQuotaExceeded)
	case errors.Is(err, ErrLiveStreamingNotEnabled):
		return errBlockerOnly(BlockerLiveStreamingNotEnabled)
	case errors.Is(err, ErrUnavailable), errors.Is(err, ErrForbidden):
		return errBlockerOnly(BlockerUnavailable)
	default:
		return err
	}
}

// blockerErr lets mapProviderErr turn a provider-level failure into the
// same blocker vocabulary Preview/Publish already use.
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
