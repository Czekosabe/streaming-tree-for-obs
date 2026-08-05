package account

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/secrets/secretstest"
)

// --- fakes -----------------------------------------------------------------

type fakeRepository struct {
	mu       sync.Mutex
	accounts map[string]Account
	links    map[string]Link
	settings map[ProviderID]IntegrationSettings
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		accounts: make(map[string]Account),
		links:    make(map[string]Link),
		settings: make(map[ProviderID]IntegrationSettings),
	}
}

func cloneAccount(a Account) Account {
	a.Scopes = append([]string(nil), a.Scopes...)
	return a
}

func (r *fakeRepository) GetAccount(ctx context.Context, id string) (Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.accounts[id]
	if !ok {
		return Account{}, ErrNotFound
	}
	return cloneAccount(a), nil
}

func (r *fakeRepository) FindByProviderIdentity(ctx context.Context, providerID ProviderID, providerUserID string) (Account, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, a := range r.accounts {
		if a.ProviderID == providerID && a.ProviderUserID == providerUserID {
			return cloneAccount(a), true, nil
		}
	}
	return Account{}, false, nil
}

func (r *fakeRepository) CreateAccount(ctx context.Context, acc Account) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.accounts[acc.ID]; exists {
		return ErrConflict
	}
	r.accounts[acc.ID] = cloneAccount(acc)
	return nil
}

func (r *fakeRepository) UpdateAccount(ctx context.Context, acc Account) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.accounts[acc.ID]; !exists {
		return ErrNotFound
	}
	r.accounts[acc.ID] = cloneAccount(acc)
	return nil
}

func (r *fakeRepository) DeleteAccount(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.accounts[id]; !exists {
		return ErrNotFound
	}
	delete(r.accounts, id)
	for platformID, link := range r.links {
		if link.AccountID == id {
			delete(r.links, platformID)
		}
	}
	return nil
}

func (r *fakeRepository) ListAccounts(ctx context.Context) ([]Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Account, 0, len(r.accounts))
	for _, a := range r.accounts {
		out = append(out, cloneAccount(a))
	}
	return out, nil
}

func (r *fakeRepository) CountAccounts(ctx context.Context, providerID ProviderID) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, a := range r.accounts {
		if a.ProviderID == providerID {
			count++
		}
	}
	return count, nil
}

func (r *fakeRepository) GetLink(ctx context.Context, platformID string) (Link, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	l, ok := r.links[platformID]
	return l, ok, nil
}

func (r *fakeRepository) ListLinksByAccount(ctx context.Context, accountID string) ([]Link, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	links := []Link{}
	for _, l := range r.links {
		if l.AccountID == accountID {
			links = append(links, l)
		}
	}
	return links, nil
}

func (r *fakeRepository) SetLink(ctx context.Context, platformID, accountID string, now time.Time) (Link, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	l := Link{PlatformID: platformID, AccountID: accountID, CreatedAt: now, UpdatedAt: now}
	if existing, ok := r.links[platformID]; ok {
		l.CreatedAt = existing.CreatedAt
	}
	r.links[platformID] = l
	return l, nil
}

func (r *fakeRepository) DeleteLink(ctx context.Context, platformID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.links, platformID)
	return nil
}

func (r *fakeRepository) GetIntegrationSettings(ctx context.Context, providerID ProviderID) (IntegrationSettings, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.settings[providerID]
	return s, ok, nil
}

func (r *fakeRepository) SetIntegrationSettings(ctx context.Context, providerID ProviderID, clientID string, now time.Time) (IntegrationSettings, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := IntegrationSettings{ProviderID: providerID, ClientID: clientID, UpdatedAt: now}
	r.settings[providerID] = s
	return s, nil
}

// fakeProvider is a controllable account.Provider for tests: no real network
// call is ever made.
type fakeProvider struct {
	mu sync.Mutex

	validateResult ValidationResult
	validateErr    error
	refreshBundle  TokenBundle
	refreshErr     error
	revokeErr      error
	identity       Identity
	identityErr    error

	refreshCalls int32
}

func (p *fakeProvider) ProviderID() ProviderID { return ProviderTwitch }

func (p *fakeProvider) StartDeviceFlow(ctx context.Context, clientID string, scopes []string) (DeviceFlowStart, error) {
	return DeviceFlowStart{}, errors.New("not used in these tests")
}

func (p *fakeProvider) PollDeviceFlow(ctx context.Context, clientID, deviceCode string) (PollOutcome, error) {
	return PollOutcome{}, errors.New("not used in these tests")
}

func (p *fakeProvider) ValidateToken(ctx context.Context, accessToken string) (ValidationResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.validateResult, p.validateErr
}

func (p *fakeProvider) RefreshToken(ctx context.Context, clientID, refreshToken string) (TokenBundle, error) {
	atomic.AddInt32(&p.refreshCalls, 1)
	// A short delay makes the single-flight test meaningful: without it,
	// concurrent callers could race past the in-flight check before the
	// first call even records itself.
	time.Sleep(20 * time.Millisecond)
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.refreshBundle, p.refreshErr
}

func (p *fakeProvider) RevokeToken(ctx context.Context, clientID, accessToken string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.revokeErr
}

func (p *fakeProvider) GetIdentity(ctx context.Context, accessToken, clientID string) (Identity, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.identity, p.identityErr
}

func (p *fakeProvider) refreshCallCount() int { return int(atomic.LoadInt32(&p.refreshCalls)) }

func newTestService(t *testing.T) (*Service, *fakeRepository, *secretstest.Store, *fakeProvider) {
	t.Helper()
	repo := newFakeRepository()
	store := secretstest.New()
	provider := &fakeProvider{}

	svc := NewService(Options{
		Repository: repo, Secrets: store,
		Providers:      map[ProviderID]Provider{ProviderTwitch: provider},
		EnvClientIDs:   map[ProviderID]string{},
		RequiredScopes: map[ProviderID][]string{ProviderTwitch: {"channel:manage:broadcast"}},
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now:            func() time.Time { return time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC) },
	})
	if _, err := svc.SetIntegrationClientID(context.Background(), ProviderTwitch, "test-client-id"); err != nil {
		t.Fatalf("SetIntegrationClientID() error = %v", err)
	}
	return svc, repo, store, provider
}

// --- integration config ------------------------------------------------

func TestIntegrationConfigReportsMissingByDefault(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(Options{Repository: repo, Secrets: secretstest.New(), Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})

	cfg, err := svc.IntegrationConfig(context.Background(), ProviderTwitch)
	if err != nil {
		t.Fatalf("IntegrationConfig() error = %v", err)
	}
	if cfg.Configured || cfg.Source != SourceMissing {
		t.Errorf("IntegrationConfig() = %+v, want missing", cfg)
	}
}

func TestEnvironmentClientIDAlwaysWinsAndCannotBeOverwritten(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(Options{
		Repository: repo, Secrets: secretstest.New(),
		EnvClientIDs: map[ProviderID]string{ProviderTwitch: "env-client-id"},
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	cfg, err := svc.IntegrationConfig(context.Background(), ProviderTwitch)
	if err != nil || cfg.Source != SourceEnvironment || cfg.ClientID != "env-client-id" {
		t.Fatalf("IntegrationConfig() = %+v, err = %v, want the environment override", cfg, err)
	}

	if _, err := svc.SetIntegrationClientID(context.Background(), ProviderTwitch, "database-value"); !errors.Is(err, ErrIntegrationLocked) {
		t.Errorf("SetIntegrationClientID() error = %v, want ErrIntegrationLocked", err)
	}
}

func TestSetIntegrationClientIDRejectsAnEmptyValue(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(Options{Repository: repo, Secrets: secretstest.New(), Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})

	_, err := svc.SetIntegrationClientID(context.Background(), ProviderTwitch, "   ")
	if err == nil {
		t.Fatal("SetIntegrationClientID(\"\") returned nil error")
	}
}

func TestSetIntegrationClientIDIsLockedWhileAccountsExistButAllowsTheSameValue(t *testing.T) {
	svc, repo, _, _ := newTestService(t)
	repo.accounts["acct_1"] = Account{ID: "acct_1", ProviderID: ProviderTwitch, ProviderUserID: "u1", Status: StatusConnected}

	if _, err := svc.SetIntegrationClientID(context.Background(), ProviderTwitch, "different-client-id"); !errors.Is(err, ErrIntegrationLocked) {
		t.Errorf("changing the client id error = %v, want ErrIntegrationLocked", err)
	}
	if _, err := svc.SetIntegrationClientID(context.Background(), ProviderTwitch, "test-client-id"); err != nil {
		t.Errorf("re-setting the identical client id error = %v, want nil", err)
	}
}

// --- finalize connection -------------------------------------------------

func TestFinalizeConnectionCreatesANewAccount(t *testing.T) {
	svc, _, store, _ := newTestService(t)

	acc, err := svc.FinalizeConnection(context.Background(), ProviderTwitch,
		Identity{ProviderUserID: "u1", Login: "streamer", DisplayName: "Streamer"},
		TokenBundle{TokenType: "bearer", AccessToken: "a", RefreshToken: "r", ExpiresAt: time.Now().Add(time.Hour)},
		[]string{"channel:manage:broadcast"}, "")
	if err != nil {
		t.Fatalf("FinalizeConnection() error = %v", err)
	}
	if acc.Status != StatusConnected || acc.Login != "streamer" {
		t.Errorf("FinalizeConnection() = %+v", acc)
	}
	if !store.Has(tokenBundleKey(acc.ID)) {
		t.Error("the token bundle was not stored")
	}
}

func TestFinalizeConnectionRejectsMissingScope(t *testing.T) {
	svc, _, _, _ := newTestService(t)

	_, err := svc.FinalizeConnection(context.Background(), ProviderTwitch,
		Identity{ProviderUserID: "u1", Login: "streamer"},
		TokenBundle{TokenType: "bearer", AccessToken: "a", RefreshToken: "r", ExpiresAt: time.Now().Add(time.Hour)},
		[]string{"user:read:email"}, "")
	if !errors.Is(err, ErrMissingScope) {
		t.Errorf("FinalizeConnection() error = %v, want ErrMissingScope", err)
	}
}

func TestFinalizeConnectionForTheSameIdentityIsAReconnectNotADuplicate(t *testing.T) {
	svc, repo, _, _ := newTestService(t)
	ctx := context.Background()
	identity := Identity{ProviderUserID: "u1", Login: "streamer", DisplayName: "Streamer"}
	scopes := []string{"channel:manage:broadcast"}
	bundle := TokenBundle{TokenType: "bearer", AccessToken: "a", RefreshToken: "r", ExpiresAt: time.Now().Add(time.Hour)}

	first, err := svc.FinalizeConnection(ctx, ProviderTwitch, identity, bundle, scopes, "")
	if err != nil {
		t.Fatalf("first FinalizeConnection() error = %v", err)
	}

	second, err := svc.FinalizeConnection(ctx, ProviderTwitch, identity, bundle, scopes, "")
	if err != nil {
		t.Fatalf("second FinalizeConnection() error = %v", err)
	}
	if second.ID != first.ID {
		t.Errorf("a repeated authorization for the same identity created a new account: %s vs %s", second.ID, first.ID)
	}
	if len(repo.accounts) != 1 {
		t.Errorf("account count = %d, want exactly 1", len(repo.accounts))
	}
}

func TestFinalizeConnectionRejectsAnIdentityMismatchDuringAnExplicitReconnect(t *testing.T) {
	svc, repo, _, _ := newTestService(t)
	ctx := context.Background()
	repo.accounts["acct_existing"] = Account{ID: "acct_existing", ProviderID: ProviderTwitch, ProviderUserID: "u1", Status: StatusConnected}

	_, err := svc.FinalizeConnection(ctx, ProviderTwitch,
		Identity{ProviderUserID: "u2", Login: "someone-else"},
		TokenBundle{TokenType: "bearer", AccessToken: "a", RefreshToken: "r", ExpiresAt: time.Now().Add(time.Hour)},
		[]string{"channel:manage:broadcast"}, "acct_existing")
	if !errors.Is(err, ErrIdentityMismatch) {
		t.Errorf("FinalizeConnection() error = %v, want ErrIdentityMismatch", err)
	}
}

func TestFinalizeConnectionCleansUpTheSecretWhenTheDatabaseInsertFails(t *testing.T) {
	repo := newFakeRepository()
	store := secretstest.New()
	svc := NewService(Options{
		Repository: repo, Secrets: store,
		RequiredScopes: map[ProviderID][]string{},
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		NewID:          func() (string, error) { return "acct_fixed", nil },
	})
	// Pre-seed a row with the same ID the fixed generator will produce, so
	// CreateAccount fails with ErrConflict.
	repo.accounts["acct_fixed"] = Account{ID: "acct_fixed", ProviderID: ProviderTwitch, ProviderUserID: "someone-else"}

	_, err := svc.FinalizeConnection(context.Background(), ProviderTwitch,
		Identity{ProviderUserID: "u1", Login: "streamer"},
		TokenBundle{TokenType: "bearer", AccessToken: "a", RefreshToken: "r", ExpiresAt: time.Now().Add(time.Hour)},
		nil, "")
	if err == nil {
		t.Fatal("expected the database insert to fail")
	}
	if store.Has(tokenBundleKey("acct_fixed")) {
		t.Error("the orphaned token bundle was not cleaned up after the database failure")
	}
}

// --- validation ----------------------------------------------------------

func TestValidateNowMarksAValidAccountConnected(t *testing.T) {
	svc, repo, _, provider := newTestService(t)
	repo.accounts["acct_1"] = Account{ID: "acct_1", ProviderID: ProviderTwitch, ProviderUserID: "u1", Status: StatusReconnectRequired}
	_ = StoreTokenBundle(context.Background(), svc.secrets, "acct_1", testBundle())
	provider.validateResult = ValidationResult{Valid: true, ClientID: "test-client-id", ProviderUserID: "u1", Scopes: []string{"channel:manage:broadcast"}}

	acc, err := svc.ValidateNow(context.Background(), "acct_1")
	if err != nil {
		t.Fatalf("ValidateNow() error = %v", err)
	}
	if acc.Status != StatusConnected {
		t.Errorf("Status = %q, want connected", acc.Status)
	}
	if acc.LastValidatedAt == nil {
		t.Error("LastValidatedAt was not set")
	}
}

func TestValidateNowRefreshesOnceBeforeGivingUp(t *testing.T) {
	svc, repo, _, provider := newTestService(t)
	repo.accounts["acct_1"] = Account{ID: "acct_1", ProviderID: ProviderTwitch, ProviderUserID: "u1", Status: StatusConnected}
	_ = StoreTokenBundle(context.Background(), svc.secrets, "acct_1", testBundle())
	provider.validateResult = ValidationResult{Valid: false}
	provider.refreshBundle = TokenBundle{TokenType: "bearer", AccessToken: "new-a", RefreshToken: "new-r", ExpiresAt: time.Now().Add(time.Hour)}

	acc, err := svc.ValidateNow(context.Background(), "acct_1")
	if err != nil {
		t.Fatalf("ValidateNow() error = %v", err)
	}
	// The fake still reports Valid:false even after refresh (never changed),
	// so this must land on reconnect_required, but only after exactly one
	// refresh attempt.
	if acc.Status != StatusReconnectRequired {
		t.Errorf("Status = %q, want reconnect_required", acc.Status)
	}
	if provider.refreshCallCount() != 1 {
		t.Errorf("refresh was called %d times, want exactly 1", provider.refreshCallCount())
	}
}

func TestValidateNowMarksReconnectRequiredWhenRefreshFails(t *testing.T) {
	svc, repo, _, provider := newTestService(t)
	repo.accounts["acct_1"] = Account{ID: "acct_1", ProviderID: ProviderTwitch, ProviderUserID: "u1", Status: StatusConnected}
	_ = StoreTokenBundle(context.Background(), svc.secrets, "acct_1", testBundle())
	provider.validateResult = ValidationResult{Valid: false}
	provider.refreshErr = errors.New("refresh token expired")

	acc, err := svc.ValidateNow(context.Background(), "acct_1")
	if err != nil {
		t.Fatalf("ValidateNow() error = %v", err)
	}
	if acc.Status != StatusReconnectRequired {
		t.Errorf("Status = %q, want reconnect_required", acc.Status)
	}
}

func TestValidateNowRejectsAMismatchedClientID(t *testing.T) {
	svc, repo, _, provider := newTestService(t)
	repo.accounts["acct_1"] = Account{ID: "acct_1", ProviderID: ProviderTwitch, ProviderUserID: "u1", Status: StatusConnected}
	_ = StoreTokenBundle(context.Background(), svc.secrets, "acct_1", testBundle())
	provider.validateResult = ValidationResult{Valid: true, ClientID: "some-other-client-id", Scopes: []string{"channel:manage:broadcast"}}
	provider.refreshErr = errors.New("cannot refresh")

	acc, _ := svc.ValidateNow(context.Background(), "acct_1")
	if acc.Status != StatusReconnectRequired {
		t.Errorf("Status = %q, want reconnect_required for a client id mismatch", acc.Status)
	}
}

// --- single-flight refresh -----------------------------------------------

func TestConcurrentRefreshesShareOneProviderCall(t *testing.T) {
	svc, repo, _, provider := newTestService(t)
	repo.accounts["acct_1"] = Account{ID: "acct_1", ProviderID: ProviderTwitch, ProviderUserID: "u1", Status: StatusConnected}
	_ = StoreTokenBundle(context.Background(), svc.secrets, "acct_1", testBundle())
	provider.refreshBundle = TokenBundle{TokenType: "bearer", AccessToken: "new-a", RefreshToken: "new-r", ExpiresAt: time.Now().Add(time.Hour)}

	acc, _ := repo.GetAccount(context.Background(), "acct_1")

	var wg sync.WaitGroup
	results := make([]TokenBundle, 5)
	errs := make([]error, 5)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = svc.singleFlightRefresh(context.Background(), acc)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("refresh %d error = %v", i, err)
		}
		if results[i].AccessToken != "new-a" {
			t.Errorf("refresh %d returned %q, want the shared result", i, results[i].AccessToken)
		}
	}
	if provider.refreshCallCount() != 1 {
		t.Errorf("provider.RefreshToken was called %d times, want exactly 1 for 5 concurrent callers", provider.refreshCallCount())
	}
}

// --- WithFreshToken --------------------------------------------------------

func TestWithFreshTokenRetriesExactlyOnceOn401(t *testing.T) {
	svc, repo, _, provider := newTestService(t)
	repo.accounts["acct_1"] = Account{ID: "acct_1", ProviderID: ProviderTwitch, ProviderUserID: "u1", Status: StatusConnected}
	_ = StoreTokenBundle(context.Background(), svc.secrets, "acct_1", testBundle())
	provider.refreshBundle = TokenBundle{TokenType: "bearer", AccessToken: "new-a", RefreshToken: "new-r", ExpiresAt: time.Now().Add(time.Hour)}

	calls := 0
	var seenTokens []string
	err := svc.WithFreshToken(context.Background(), "acct_1", func(token string) (bool, error) {
		calls++
		seenTokens = append(seenTokens, token)
		return calls == 1, nil // unauthorized on the first call only
	})
	if err != nil {
		t.Fatalf("WithFreshToken() error = %v", err)
	}
	if calls != 2 {
		t.Errorf("fn was called %d times, want exactly 2 (one retry)", calls)
	}
	if seenTokens[1] != "new-a" {
		t.Errorf("the retry used token %q, want the refreshed one", seenTokens[1])
	}
}

func TestWithFreshTokenMarksReconnectRequiredAfterARepeated401(t *testing.T) {
	svc, repo, _, provider := newTestService(t)
	repo.accounts["acct_1"] = Account{ID: "acct_1", ProviderID: ProviderTwitch, ProviderUserID: "u1", Status: StatusConnected}
	_ = StoreTokenBundle(context.Background(), svc.secrets, "acct_1", testBundle())
	provider.refreshBundle = TokenBundle{TokenType: "bearer", AccessToken: "new-a", RefreshToken: "new-r", ExpiresAt: time.Now().Add(time.Hour)}

	err := svc.WithFreshToken(context.Background(), "acct_1", func(token string) (bool, error) {
		return true, nil // always unauthorized
	})
	if !errors.Is(err, ErrReconnectRequired) {
		t.Errorf("WithFreshToken() error = %v, want ErrReconnectRequired", err)
	}
	acc, _ := repo.GetAccount(context.Background(), "acct_1")
	if acc.Status != StatusReconnectRequired {
		t.Errorf("Status = %q, want reconnect_required", acc.Status)
	}
}

// --- disconnect ------------------------------------------------------------

func TestDisconnectRevokesDeletesSecretThenDeletesAccount(t *testing.T) {
	svc, repo, store, provider := newTestService(t)
	repo.accounts["acct_1"] = Account{ID: "acct_1", ProviderID: ProviderTwitch, ProviderUserID: "u1", Status: StatusConnected}
	_ = StoreTokenBundle(context.Background(), svc.secrets, "acct_1", testBundle())
	provider.revokeErr = nil

	if err := svc.Disconnect(context.Background(), "acct_1"); err != nil {
		t.Fatalf("Disconnect() error = %v", err)
	}
	if store.Has(tokenBundleKey("acct_1")) {
		t.Error("the token bundle was not deleted")
	}
	if _, err := repo.GetAccount(context.Background(), "acct_1"); !errors.Is(err, ErrNotFound) {
		t.Error("the account row was not deleted")
	}
}

func TestDisconnectPreservesLocalStateWhenRevocationFails(t *testing.T) {
	svc, repo, store, provider := newTestService(t)
	repo.accounts["acct_1"] = Account{ID: "acct_1", ProviderID: ProviderTwitch, ProviderUserID: "u1", Status: StatusConnected}
	_ = StoreTokenBundle(context.Background(), svc.secrets, "acct_1", testBundle())
	provider.revokeErr = errors.New("network error")

	if err := svc.Disconnect(context.Background(), "acct_1"); err == nil {
		t.Fatal("expected Disconnect() to fail when revocation fails")
	}
	if !store.Has(tokenBundleKey("acct_1")) {
		t.Error("the token bundle was deleted even though revocation failed")
	}
	if _, err := repo.GetAccount(context.Background(), "acct_1"); err != nil {
		t.Error("the account row was deleted even though revocation failed")
	}
}

func TestDisconnectProceedsWhenTwitchReportsTheTokenAlreadyInvalid(t *testing.T) {
	// A provider adapter treats "already invalid" as success itself (see
	// internal/provider/twitch's RevokeToken) - this test exercises the
	// account.Service side of that contract with a fake provider that
	// returns nil, matching what a real adapter would do for that case.
	svc, repo, store, provider := newTestService(t)
	repo.accounts["acct_1"] = Account{ID: "acct_1", ProviderID: ProviderTwitch, ProviderUserID: "u1", Status: StatusConnected}
	_ = StoreTokenBundle(context.Background(), svc.secrets, "acct_1", testBundle())
	provider.revokeErr = nil

	if err := svc.Disconnect(context.Background(), "acct_1"); err != nil {
		t.Fatalf("Disconnect() error = %v", err)
	}
	if store.Has(tokenBundleKey("acct_1")) {
		t.Error("the token bundle still exists")
	}
}

func TestDisconnectClearsRemoteTargetsForEveryLinkedPlatform(t *testing.T) {
	// A real bug caught while writing the stage 7B YouTube integration
	// script: platform_remote_targets has no foreign key to
	// connected_accounts (only to platforms), so nothing previously cleared
	// a destination's selected broadcast when the account behind it was
	// disconnected - it would have survived, dangling, pointing at a
	// channel this application no longer has access to.
	repo := newFakeRepository()
	store := secretstest.New()
	provider := &fakeProvider{}
	svc := NewService(Options{
		Repository: repo, Secrets: store,
		Providers:      map[ProviderID]Provider{ProviderTwitch: provider},
		EnvClientIDs:   map[ProviderID]string{},
		RequiredScopes: map[ProviderID][]string{ProviderTwitch: {"channel:manage:broadcast"}},
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now:            func() time.Time { return time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC) },
	})
	repo.accounts["acct_1"] = Account{ID: "acct_1", ProviderID: ProviderTwitch, ProviderUserID: "u1", Status: StatusConnected}
	_ = StoreTokenBundle(context.Background(), svc.secrets, "acct_1", testBundle())
	if _, err := repo.SetLink(context.Background(), "pf_1", "acct_1", time.Now()); err != nil {
		t.Fatalf("SetLink() error = %v", err)
	}
	if _, err := repo.SetLink(context.Background(), "pf_2", "acct_1", time.Now()); err != nil {
		t.Fatalf("SetLink() error = %v", err)
	}

	var cleaned []string
	svc2 := NewService(Options{
		Repository: repo, Secrets: store,
		Providers:      map[ProviderID]Provider{ProviderTwitch: provider},
		EnvClientIDs:   map[ProviderID]string{},
		RequiredScopes: map[ProviderID][]string{ProviderTwitch: {"channel:manage:broadcast"}},
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now:            func() time.Time { return time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC) },
		OnAccountDisconnected: func(ctx context.Context, platformID string) error {
			cleaned = append(cleaned, platformID)
			return nil
		},
	})

	if err := svc2.Disconnect(context.Background(), "acct_1"); err != nil {
		t.Fatalf("Disconnect() error = %v", err)
	}

	sort.Strings(cleaned)
	if len(cleaned) != 2 || cleaned[0] != "pf_1" || cleaned[1] != "pf_2" {
		t.Errorf("cleaned platforms = %v, want exactly [pf_1 pf_2]", cleaned)
	}
}

func TestDisconnectSucceedsEvenWhenRemoteTargetCleanupFails(t *testing.T) {
	repo := newFakeRepository()
	store := secretstest.New()
	provider := &fakeProvider{}
	repo.accounts["acct_1"] = Account{ID: "acct_1", ProviderID: ProviderTwitch, ProviderUserID: "u1", Status: StatusConnected}
	if _, err := repo.SetLink(context.Background(), "pf_1", "acct_1", time.Now()); err != nil {
		t.Fatalf("SetLink() error = %v", err)
	}

	svc := NewService(Options{
		Repository: repo, Secrets: store,
		Providers:      map[ProviderID]Provider{ProviderTwitch: provider},
		EnvClientIDs:   map[ProviderID]string{},
		RequiredScopes: map[ProviderID][]string{ProviderTwitch: {"channel:manage:broadcast"}},
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now:            func() time.Time { return time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC) },
		OnAccountDisconnected: func(ctx context.Context, platformID string) error {
			return errors.New("remote target cleanup failed")
		},
	})
	_ = StoreTokenBundle(context.Background(), svc.secrets, "acct_1", testBundle())

	if err := svc.Disconnect(context.Background(), "acct_1"); err != nil {
		t.Fatalf("Disconnect() error = %v, want disconnect to still succeed despite a cleanup failure", err)
	}
	if _, err := repo.GetAccount(context.Background(), "acct_1"); !errors.Is(err, ErrNotFound) {
		t.Error("the account row was not deleted even though only the cleanup hook failed")
	}
}

// --- platform links --------------------------------------------------------

func TestLinkPlatformRejectsAProviderMismatch(t *testing.T) {
	svc, repo, _, _ := newTestService(t)
	repo.accounts["acct_1"] = Account{ID: "acct_1", ProviderID: ProviderTwitch, ProviderUserID: "u1", Status: StatusConnected}

	_, err := svc.LinkPlatform(context.Background(), "pf_1", "youtube", "acct_1")
	if !errors.Is(err, ErrProviderMismatch) {
		t.Errorf("LinkPlatform() error = %v, want ErrProviderMismatch", err)
	}
}

func TestLinkPlatformPersistsAndCanBeReplaced(t *testing.T) {
	svc, repo, _, _ := newTestService(t)
	repo.accounts["acct_1"] = Account{ID: "acct_1", ProviderID: ProviderTwitch, ProviderUserID: "u1", Status: StatusConnected}
	repo.accounts["acct_2"] = Account{ID: "acct_2", ProviderID: ProviderTwitch, ProviderUserID: "u2", Status: StatusConnected}

	if _, err := svc.LinkPlatform(context.Background(), "pf_1", "twitch", "acct_1"); err != nil {
		t.Fatalf("LinkPlatform() error = %v", err)
	}
	link, found, err := svc.GetLink(context.Background(), "pf_1")
	if err != nil || !found || link.AccountID != "acct_1" {
		t.Fatalf("GetLink() = %+v, %v, %v", link, found, err)
	}

	if _, err := svc.LinkPlatform(context.Background(), "pf_1", "twitch", "acct_2"); err != nil {
		t.Fatalf("replacing the link error = %v", err)
	}
	link, _, _ = svc.GetLink(context.Background(), "pf_1")
	if link.AccountID != "acct_2" {
		t.Errorf("AccountID = %q, want acct_2 after replacing the link", link.AccountID)
	}
}

func TestUnlinkPlatformDoesNotDeleteThePlatformOrTheAccount(t *testing.T) {
	svc, repo, _, _ := newTestService(t)
	repo.accounts["acct_1"] = Account{ID: "acct_1", ProviderID: ProviderTwitch, ProviderUserID: "u1", Status: StatusConnected}
	if _, err := svc.LinkPlatform(context.Background(), "pf_1", "twitch", "acct_1"); err != nil {
		t.Fatalf("LinkPlatform() error = %v", err)
	}

	if err := svc.UnlinkPlatform(context.Background(), "pf_1"); err != nil {
		t.Fatalf("UnlinkPlatform() error = %v", err)
	}
	if _, found, _ := svc.GetLink(context.Background(), "pf_1"); found {
		t.Error("the link still exists after UnlinkPlatform")
	}
	if _, err := svc.GetAccount(context.Background(), "acct_1"); err != nil {
		t.Error("the account was affected by unlinking a platform")
	}
}

func TestDeletingAnAccountRemovesItsLinks(t *testing.T) {
	svc, repo, _, _ := newTestService(t)
	repo.accounts["acct_1"] = Account{ID: "acct_1", ProviderID: ProviderTwitch, ProviderUserID: "u1", Status: StatusConnected}
	if _, err := svc.LinkPlatform(context.Background(), "pf_1", "twitch", "acct_1"); err != nil {
		t.Fatalf("LinkPlatform() error = %v", err)
	}

	if err := repo.DeleteAccount(context.Background(), "acct_1"); err != nil {
		t.Fatalf("DeleteAccount() error = %v", err)
	}
	if _, found, _ := svc.GetLink(context.Background(), "pf_1"); found {
		t.Error("the link survived the account's deletion")
	}
}
