package account

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/secrets"
)

// IDGenerator produces identifiers for new connected accounts.
type IDGenerator func() (string, error)

// Clock returns the current time; injected so tests are deterministic.
type Clock func() time.Time

// NewID returns a random, non-sequential account identifier, mirroring
// platform.NewID's own reasoning: no sequential integers as public IDs.
func NewID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate account id: %w", err)
	}
	return "acct_" + hex.EncodeToString(buf), nil
}

// Additional sentinel errors returned by Service, distinct from the
// structural ones in errors.go: these describe a provider-side outcome
// rather than a local storage or validation failure.
var (
	// ErrProviderUnavailable wraps a transient provider failure (network,
	// 5xx) - retryable, and local state is left untouched by callers that
	// see it.
	ErrProviderUnavailable = errors.New("provider unavailable")

	// ErrRateLimited means the provider's rate limit was hit.
	ErrRateLimited = errors.New("provider rate limited")

	// ErrInvalidProviderResponse means the provider returned something this
	// application's adapter could not parse or trust.
	ErrInvalidProviderResponse = errors.New("invalid provider response")
)

// Service holds the connected-account use cases: integration configuration,
// device-flow finalization, validation, single-flight refresh, linking, and
// disconnection.
//
// It is the one place account.Repository, secrets.SecretStore and a
// provider's Provider adapter are used together - internal/httpapi never
// touches any of them directly.
type Service struct {
	repo      Repository
	secrets   secrets.SecretStore
	providers map[ProviderID]Provider
	// envClientIDs holds the resolved STREAMING_TREE_<PROVIDER>_CLIENT_ID
	// values, if any. A present, non-empty entry always wins over the
	// database - see IntegrationConfig.
	envClientIDs map[ProviderID]string
	// requiredScopes is the minimum scope set an account must carry to be
	// considered healthy, per provider.
	requiredScopes map[ProviderID][]string

	newID  IDGenerator
	now    Clock
	logger *slog.Logger

	refreshMu       sync.Mutex
	refreshInFlight map[string]*refreshCall

	// validationInterval defaults to validationInterval (the constant
	// below); tests shorten it. Twitch's own documentation requires
	// hourly validation - see docs/provider-integrations/twitch.md.
	validationInterval time.Duration
	validationJitter   func() time.Duration

	lifecycle context.Context
	cancel    context.CancelFunc
	workers   sync.WaitGroup
}

// validationInterval is the default background-validation cadence: Twitch's
// own documentation requires hourly validation.
const defaultValidationInterval = 1 * time.Hour

// validationJitterSpan bounds the random jitter added before each
// validation pass, so many accounts (or many backend instances) do not all
// call Twitch in the same instant.
const validationJitterSpan = 2 * time.Minute

type refreshCall struct {
	done   chan struct{}
	bundle TokenBundle
	err    error
}

// Options constructs a Service.
type Options struct {
	Repository     Repository
	Secrets        secrets.SecretStore
	Providers      map[ProviderID]Provider
	EnvClientIDs   map[ProviderID]string
	RequiredScopes map[ProviderID][]string
	Logger         *slog.Logger
	// NewID and Now are overridable for tests; both default to the real
	// implementations when nil.
	NewID IDGenerator
	Now   Clock
}

// NewService builds a Service.
func NewService(opts Options) *Service {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	newID := opts.NewID
	if newID == nil {
		newID = NewID
	}
	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{
		repo:               opts.Repository,
		secrets:            opts.Secrets,
		providers:          opts.Providers,
		envClientIDs:       opts.EnvClientIDs,
		requiredScopes:     opts.RequiredScopes,
		newID:              newID,
		now:                now,
		logger:             logger,
		refreshInFlight:    make(map[string]*refreshCall),
		validationInterval: defaultValidationInterval,
		validationJitter: func() time.Duration {
			buf := make([]byte, 1)
			_, _ = rand.Read(buf)
			return time.Duration(buf[0]) * (validationJitterSpan / 256)
		},
	}
}

// StartValidationWorker begins the non-blocking background validation loop.
//
// It never blocks HTTP server startup: the caller invokes this after (or
// concurrently with) starting the listener, and a Twitch or SecretStore
// outage here only affects account status, never the rest of the API - see
// validateAccount's own error handling, which never panics or exits on a
// provider failure.
func (s *Service) StartValidationWorker(ctx context.Context) {
	s.lifecycle, s.cancel = context.WithCancel(context.Background())
	_ = ctx

	s.workers.Add(1)
	go func() {
		defer s.workers.Done()
		s.validationLoop()
	}()
}

// ShutdownValidationWorker stops the background validation loop.
func (s *Service) ShutdownValidationWorker(ctx context.Context) {
	if s.cancel != nil {
		s.cancel()
	}
	done := make(chan struct{})
	go func() {
		s.workers.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func (s *Service) validationLoop() {
	ticker := time.NewTicker(s.validationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.lifecycle.Done():
			return
		case <-ticker.C:
			s.validateAllAccounts()
		}
	}
}

func (s *Service) validateAllAccounts() {
	accounts, err := s.repo.ListAccounts(s.lifecycle)
	if err != nil {
		s.logger.Error("background validation could not list accounts", slog.Any("error", err))
		return
	}
	for _, acc := range accounts {
		select {
		case <-s.lifecycle.Done():
			return
		case <-time.After(s.validationJitter()):
		}
		if _, err := s.validateAccount(s.lifecycle, acc); err != nil {
			s.logger.Warn("background validation failed for an account",
				slog.String("account_id", acc.ID), slog.String("provider_id", string(acc.ProviderID)), slog.Any("error", err))
		}
	}
}

func mapRepoErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrConflict) {
		return err
	}
	return fmt.Errorf("%w: %s", ErrStorage, err)
}

func containsStr(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}

// --- integration settings ------------------------------------------------

// IntegrationConfig resolves the effective Client ID for a provider: the
// environment override if set, otherwise the database-managed value.
func (s *Service) IntegrationConfig(ctx context.Context, providerID ProviderID) (IntegrationConfig, error) {
	if env, ok := s.envClientIDs[providerID]; ok && env != "" {
		return IntegrationConfig{ProviderID: providerID, ClientID: env, Configured: true, Source: SourceEnvironment}, nil
	}

	settings, found, err := s.repo.GetIntegrationSettings(ctx, providerID)
	if err != nil {
		return IntegrationConfig{}, mapRepoErr(err)
	}
	if !found {
		return IntegrationConfig{ProviderID: providerID, Configured: false, Source: SourceMissing}, nil
	}
	return IntegrationConfig{ProviderID: providerID, ClientID: settings.ClientID, Configured: true, Source: SourceDatabase}, nil
}

// EffectiveClientID is the exported form of effectiveClientID, for provider
// metadata services (internal/provider/twitch's MetadataService) that need
// the Client-Id header value but are not part of this package.
func (s *Service) EffectiveClientID(ctx context.Context, providerID ProviderID) (string, error) {
	return s.effectiveClientID(ctx, providerID)
}

func (s *Service) effectiveClientID(ctx context.Context, providerID ProviderID) (string, error) {
	cfg, err := s.IntegrationConfig(ctx, providerID)
	if err != nil {
		return "", err
	}
	if !cfg.Configured {
		return "", ErrIntegrationNotConfigured
	}
	return cfg.ClientID, nil
}

// SetIntegrationClientID saves a database-managed Client ID.
//
// Rejected when an environment override is active (it always wins and the
// frontend must not be able to overwrite it), and rejected with
// ErrIntegrationLocked when the value would actually change while connected
// accounts for this provider still exist - changing it can invalidate their
// tokens. Setting it to the identical current value is always allowed.
func (s *Service) SetIntegrationClientID(ctx context.Context, providerID ProviderID, clientID string) (IntegrationConfig, error) {
	if env, ok := s.envClientIDs[providerID]; ok && env != "" {
		return IntegrationConfig{}, ErrIntegrationLocked
	}

	normalized, err := ValidateClientID(clientID)
	if err != nil {
		return IntegrationConfig{}, err
	}

	current, found, err := s.repo.GetIntegrationSettings(ctx, providerID)
	if err != nil {
		return IntegrationConfig{}, mapRepoErr(err)
	}
	if found && current.ClientID != normalized {
		count, err := s.repo.CountAccounts(ctx, providerID)
		if err != nil {
			return IntegrationConfig{}, mapRepoErr(err)
		}
		if count > 0 {
			return IntegrationConfig{}, ErrIntegrationLocked
		}
	}

	settings, err := s.repo.SetIntegrationSettings(ctx, providerID, normalized, s.now())
	if err != nil {
		return IntegrationConfig{}, mapRepoErr(err)
	}
	return IntegrationConfig{ProviderID: providerID, ClientID: settings.ClientID, Configured: true, Source: SourceDatabase}, nil
}

// --- accounts --------------------------------------------------------------

// GetAccount returns one connected account.
func (s *Service) GetAccount(ctx context.Context, id string) (Account, error) {
	acc, err := s.repo.GetAccount(ctx, id)
	if err != nil {
		return Account{}, mapRepoErr(err)
	}
	return acc, nil
}

// ListAccounts returns every connected account.
func (s *Service) ListAccounts(ctx context.Context) ([]Account, error) {
	accounts, err := s.repo.ListAccounts(ctx)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	return accounts, nil
}

// FinalizeConnection is called once a device flow has produced a real token
// bundle and this application has confirmed the granted scopes and fetched
// the provider identity behind it - see internal/runtime/deviceflow.
//
// expectedAccountID, when non-empty, means this call is a reconnect
// initiated from a specific existing account (Service's Reconnect action):
// the newly authorized identity must match that account exactly, or the
// call fails with ErrIdentityMismatch rather than silently reconnecting a
// different account or creating a new one. When empty, a matching existing
// identity is still treated as a reconnect (the general "Connect" flow
// recognizes a repeated authorization for the same provider user), and a
// new identity creates a new account.
func (s *Service) FinalizeConnection(
	ctx context.Context, providerID ProviderID, identity Identity, bundle TokenBundle, scopes []string, expectedAccountID string,
) (Account, error) {
	for _, required := range s.requiredScopes[providerID] {
		if !containsStr(scopes, required) {
			return Account{}, ErrMissingScope
		}
	}

	existing, found, err := s.repo.FindByProviderIdentity(ctx, providerID, identity.ProviderUserID)
	if err != nil {
		return Account{}, mapRepoErr(err)
	}

	if expectedAccountID != "" {
		if !found || existing.ID != expectedAccountID {
			return Account{}, ErrIdentityMismatch
		}
	}

	now := s.now()

	if found {
		// Reconnect: rotate the token bundle before touching the database
		// row, and preserve the old bundle if that write fails - see
		// TokenBundle's atomic-replacement contract.
		if err := StoreTokenBundle(ctx, s.secrets, existing.ID, bundle); err != nil {
			return Account{}, err
		}
		existing.Login = identity.Login
		existing.DisplayName = identity.DisplayName
		existing.AvatarURL = identity.AvatarURL
		existing.Scopes = scopes
		existing.Status = StatusConnected
		existing.LastValidatedAt = &now
		existing.UpdatedAt = now
		if err := s.repo.UpdateAccount(ctx, existing); err != nil {
			return Account{}, mapRepoErr(err)
		}
		return existing, nil
	}

	id, err := s.newID()
	if err != nil {
		return Account{}, fmt.Errorf("%w: %s", ErrStorage, err)
	}

	// Store the secret before the database row: if the row insert then
	// fails, the compensating delete below removes the now-orphaned secret,
	// so no account row is ever created without a token bundle behind it.
	if err := StoreTokenBundle(ctx, s.secrets, id, bundle); err != nil {
		return Account{}, err
	}

	acc := Account{
		ID: id, ProviderID: providerID, ProviderUserID: identity.ProviderUserID,
		Login: identity.Login, DisplayName: identity.DisplayName, AvatarURL: identity.AvatarURL,
		Status: StatusConnected, LastValidatedAt: &now, CreatedAt: now, UpdatedAt: now, Scopes: scopes,
	}
	if err := s.repo.CreateAccount(ctx, acc); err != nil {
		if cleanupErr := DeleteTokenBundle(ctx, s.secrets, id); cleanupErr != nil {
			// Never log the token or the account/operation detail beyond
			// opaque identifiers - see the task's compensation requirement.
			s.logger.Error("failed to clean up an orphaned token bundle after a database failure",
				slog.String("account_id", id), slog.String("provider_id", string(providerID)))
		}
		return Account{}, mapRepoErr(err)
	}
	return acc, nil
}

// --- validation and refresh -------------------------------------------------

// ValidateNow immediately validates one account, refreshing its token if
// needed, and returns the account's updated status.
func (s *Service) ValidateNow(ctx context.Context, accountID string) (Account, error) {
	acc, err := s.repo.GetAccount(ctx, accountID)
	if err != nil {
		return Account{}, mapRepoErr(err)
	}
	return s.validateAccount(ctx, acc)
}

func (s *Service) validateAccount(ctx context.Context, acc Account) (Account, error) {
	provider, ok := s.providers[acc.ProviderID]
	if !ok {
		return Account{}, fmt.Errorf("%w: no provider adapter for %q", ErrStorage, acc.ProviderID)
	}
	clientID, err := s.effectiveClientID(ctx, acc.ProviderID)
	if err != nil {
		return Account{}, err
	}

	bundle, err := LoadTokenBundle(ctx, s.secrets, acc.ID)
	if err != nil {
		return Account{}, err
	}

	if result, err := provider.ValidateToken(ctx, bundle.AccessToken); err == nil && s.acceptValidation(acc.ProviderID, clientID, result) {
		return s.markConnected(ctx, acc, result.Scopes)
	}

	fresh, refreshErr := s.singleFlightRefresh(ctx, acc)
	if refreshErr != nil {
		return s.markReconnectRequired(ctx, acc)
	}

	if result, err := provider.ValidateToken(ctx, fresh.AccessToken); err == nil && s.acceptValidation(acc.ProviderID, clientID, result) {
		return s.markConnected(ctx, acc, result.Scopes)
	}
	return s.markReconnectRequired(ctx, acc)
}

func (s *Service) acceptValidation(providerID ProviderID, clientID string, result ValidationResult) bool {
	if !result.Valid || result.ClientID != clientID {
		return false
	}
	for _, required := range s.requiredScopes[providerID] {
		if !containsStr(result.Scopes, required) {
			return false
		}
	}
	return true
}

func (s *Service) markConnected(ctx context.Context, acc Account, scopes []string) (Account, error) {
	now := s.now()
	acc.Status = StatusConnected
	acc.LastValidatedAt = &now
	acc.UpdatedAt = now
	acc.Scopes = scopes
	if err := s.repo.UpdateAccount(ctx, acc); err != nil {
		return Account{}, mapRepoErr(err)
	}
	return acc, nil
}

func (s *Service) markReconnectRequired(ctx context.Context, acc Account) (Account, error) {
	now := s.now()
	acc.Status = StatusReconnectRequired
	acc.LastValidatedAt = &now
	acc.UpdatedAt = now
	if err := s.repo.UpdateAccount(ctx, acc); err != nil {
		return Account{}, mapRepoErr(err)
	}
	return acc, nil
}

// singleFlightRefresh performs at most one concurrent refresh per account:
// a caller that finds one already in flight waits for and reuses its
// result instead of starting a second one.
func (s *Service) singleFlightRefresh(ctx context.Context, acc Account) (TokenBundle, error) {
	s.refreshMu.Lock()
	if call, inFlight := s.refreshInFlight[acc.ID]; inFlight {
		s.refreshMu.Unlock()
		<-call.done
		return call.bundle, call.err
	}
	call := &refreshCall{done: make(chan struct{})}
	s.refreshInFlight[acc.ID] = call
	s.refreshMu.Unlock()

	bundle, err := s.doRefresh(ctx, acc)
	call.bundle, call.err = bundle, err
	close(call.done)

	s.refreshMu.Lock()
	delete(s.refreshInFlight, acc.ID)
	s.refreshMu.Unlock()

	return bundle, err
}

func (s *Service) doRefresh(ctx context.Context, acc Account) (TokenBundle, error) {
	provider, ok := s.providers[acc.ProviderID]
	if !ok {
		return TokenBundle{}, fmt.Errorf("%w: no provider adapter for %q", ErrStorage, acc.ProviderID)
	}
	old, err := LoadTokenBundle(ctx, s.secrets, acc.ID)
	if err != nil {
		return TokenBundle{}, err
	}
	clientID, err := s.effectiveClientID(ctx, acc.ProviderID)
	if err != nil {
		return TokenBundle{}, err
	}
	fresh, err := provider.RefreshToken(ctx, clientID, old.RefreshToken)
	if err != nil {
		return TokenBundle{}, fmt.Errorf("%w: %s", ErrProviderUnavailable, err)
	}
	// The refresh-token rotation must be durable before any caller is
	// allowed to use the new access token.
	if err := StoreTokenBundle(ctx, s.secrets, acc.ID, fresh); err != nil {
		return TokenBundle{}, err
	}
	return fresh, nil
}

// WithFreshToken retrieves an account's current access token and calls fn.
// If fn reports the token was rejected, WithFreshToken performs a
// single-flight refresh and calls fn exactly one more time with the new
// token. Repeated rejection after a successful refresh - or a refresh that
// itself fails - marks the account reconnect_required and returns
// ErrReconnectRequired.
//
// Used by every Twitch-calling operation outside device-flow finalization
// and the validation worker (category search, metadata read, metadata
// publish), so "at most one refresh and one retry per 401" is enforced in
// one place rather than reimplemented per caller.
func (s *Service) WithFreshToken(ctx context.Context, accountID string, fn func(accessToken string) (unauthorized bool, err error)) error {
	acc, err := s.repo.GetAccount(ctx, accountID)
	if err != nil {
		return mapRepoErr(err)
	}
	if acc.Status == StatusReconnectRequired {
		return ErrReconnectRequired
	}

	bundle, err := LoadTokenBundle(ctx, s.secrets, accountID)
	if err != nil {
		return err
	}

	unauthorized, err := fn(bundle.AccessToken)
	if err != nil {
		return err
	}
	if !unauthorized {
		return nil
	}

	fresh, refreshErr := s.singleFlightRefresh(ctx, acc)
	if refreshErr != nil {
		_, _ = s.markReconnectRequired(ctx, acc)
		return ErrReconnectRequired
	}

	unauthorizedAgain, err := fn(fresh.AccessToken)
	if err != nil {
		return err
	}
	if unauthorizedAgain {
		_, _ = s.markReconnectRequired(ctx, acc)
		return ErrReconnectRequired
	}
	return nil
}

// --- disconnect --------------------------------------------------------

// Disconnect revokes the account's token where possible, then removes its
// secret and its database row, in that order - see the task's disconnect
// ordering. A transient provider failure or an unavailable SecretStore
// leaves the account exactly as it was, so the caller can safely retry.
func (s *Service) Disconnect(ctx context.Context, accountID string) error {
	acc, err := s.repo.GetAccount(ctx, accountID)
	if err != nil {
		return mapRepoErr(err)
	}

	provider, ok := s.providers[acc.ProviderID]
	if !ok {
		return fmt.Errorf("%w: no provider adapter for %q", ErrStorage, acc.ProviderID)
	}

	bundle, err := LoadTokenBundle(ctx, s.secrets, accountID)
	switch {
	case err == nil:
		clientID, cfgErr := s.effectiveClientID(ctx, acc.ProviderID)
		if cfgErr == nil {
			// A provider adapter treats "already invalid" as success
			// itself (see internal/provider/twitch); only a genuine
			// transient failure reaches here as an error.
			if revokeErr := provider.RevokeToken(ctx, clientID, bundle.AccessToken); revokeErr != nil {
				return fmt.Errorf("%w: %s", ErrProviderUnavailable, revokeErr)
			}
		}
	case errors.Is(err, ErrNotFound):
		// No bundle to revoke - proceed to remove the account row.
	default:
		// SecretStore unavailable: preserve the account, report an
		// actionable error.
		return err
	}

	if err := DeleteTokenBundle(ctx, s.secrets, accountID); err != nil {
		return err
	}

	if err := s.repo.DeleteAccount(ctx, accountID); err != nil {
		return mapRepoErr(err)
	}
	return nil
}

// --- platform links ------------------------------------------------------

// GetLink returns the account linked to a platform, if any.
func (s *Service) GetLink(ctx context.Context, platformID string) (Link, bool, error) {
	link, found, err := s.repo.GetLink(ctx, platformID)
	if err != nil {
		return Link{}, false, mapRepoErr(err)
	}
	return link, found, nil
}

// LinkPlatform links a configured destination to a connected account.
//
// platformProviderID is the destination's own provider identifier, resolved
// by the caller (internal/httpapi, via the existing platform service) - this
// package deliberately does not depend on internal/domain/platform, so the
// only cross-domain fact it needs is passed in as a plain string.
func (s *Service) LinkPlatform(ctx context.Context, platformID, platformProviderID, accountID string) (Link, error) {
	acc, err := s.repo.GetAccount(ctx, accountID)
	if err != nil {
		return Link{}, mapRepoErr(err)
	}
	if string(acc.ProviderID) != platformProviderID {
		return Link{}, ErrProviderMismatch
	}
	link, err := s.repo.SetLink(ctx, platformID, accountID, s.now())
	if err != nil {
		return Link{}, mapRepoErr(err)
	}
	return link, nil
}

// UnlinkPlatform removes a platform's link without deleting either side.
func (s *Service) UnlinkPlatform(ctx context.Context, platformID string) error {
	if err := s.repo.DeleteLink(ctx, platformID); err != nil {
		return mapRepoErr(err)
	}
	return nil
}

// ClientIDMaxLength bounds a saved Client ID - short opaque tokens, never
// free text.
const ClientIDMaxLength = 128

// ValidateClientID normalizes and validates a Twitch (or future provider)
// Client ID: trimmed, required, bounded, and free of whitespace or control
// characters.
//
// Reuses platform.ValidationError, the same cross-domain trade-off already
// made by internal/domain/output (see its own validation.go) rather than
// this package inventing a second, parallel validation-error shape the HTTP
// layer would need to special-case.
func ValidateClientID(raw string) (string, error) {
	v := &platform.ValidationError{}
	trimmed := strings.TrimSpace(raw)

	switch {
	case trimmed == "":
		v.Add("clientId", platform.RuleRequired, "Client ID is required.", nil)
	case len(trimmed) > ClientIDMaxLength:
		v.Addf("clientId", platform.RuleTooLong, map[string]any{"max": ClientIDMaxLength},
			"Client ID cannot exceed %d characters.", ClientIDMaxLength)
	default:
		for _, r := range trimmed {
			if r <= 0x1F || r == 0x7F || r == ' ' {
				v.Add("clientId", platform.RuleInvalid,
					"Client ID cannot contain spaces or control characters.", nil)
				break
			}
		}
	}

	if err := v.OrNil(); err != nil {
		return "", err
	}
	return trimmed, nil
}
