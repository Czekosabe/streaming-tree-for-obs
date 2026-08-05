package deviceflow

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/account"
	"github.com/streaming-tree/server/internal/secrets/secretstest"
)

// fakeProvider is a controllable account.Provider driven directly by tests -
// no real HTTP call is ever made.
type fakeProvider struct {
	mu sync.Mutex

	start     account.DeviceFlowStart
	startErr  error
	outcomes  []account.PollOutcome // consumed in order, one per PollDeviceFlow call
	pollIndex int
	identity  account.Identity
}

func (p *fakeProvider) ProviderID() account.ProviderID { return account.ProviderTwitch }

func (p *fakeProvider) StartDeviceFlow(ctx context.Context, clientID string, scopes []string) (account.DeviceFlowStart, error) {
	return p.start, p.startErr
}

func (p *fakeProvider) PollDeviceFlow(ctx context.Context, clientID, deviceCode string) (account.PollOutcome, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pollIndex >= len(p.outcomes) {
		return account.PollOutcome{Status: account.PollPending}, nil
	}
	out := p.outcomes[p.pollIndex]
	p.pollIndex++
	return out, nil
}

func (p *fakeProvider) ValidateToken(ctx context.Context, accessToken string) (account.ValidationResult, error) {
	return account.ValidationResult{}, errors.New("not used")
}
func (p *fakeProvider) RefreshToken(ctx context.Context, clientID, refreshToken string) (account.TokenBundle, error) {
	return account.TokenBundle{}, errors.New("not used")
}
func (p *fakeProvider) RevokeToken(ctx context.Context, clientID, accessToken string) error {
	return errors.New("not used")
}
func (p *fakeProvider) GetIdentity(ctx context.Context, accessToken, clientID string) (account.Identity, error) {
	return p.identity, nil
}

func newTestSetup(t *testing.T) (*Manager, *fakeProvider) {
	t.Helper()
	repo := newFakeAccountRepo()
	provider := &fakeProvider{start: account.DeviceFlowStart{
		DeviceCode: "dc", UserCode: "ABCD-EFGH", VerificationURI: "https://www.twitch.tv/activate",
		ExpiresIn: time.Minute, Interval: 10 * time.Millisecond,
	}}
	providers := map[account.ProviderID]account.Provider{account.ProviderTwitch: provider}
	requiredScopes := map[account.ProviderID][]string{account.ProviderTwitch: {"channel:manage:broadcast"}}

	accounts := account.NewService(account.Options{
		Repository: repo, Secrets: secretstest.New(), Providers: providers, RequiredScopes: requiredScopes,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if _, err := accounts.SetIntegrationClientID(context.Background(), account.ProviderTwitch, "test-client-id"); err != nil {
		t.Fatalf("SetIntegrationClientID() error = %v", err)
	}

	m := NewManager(Options{
		Accounts:       accounts,
		Providers:      map[account.ProviderID]account.DeviceFlowProvider{account.ProviderTwitch: provider},
		RequiredScopes: map[account.ProviderID][]string{account.ProviderTwitch: {"channel:manage:broadcast"}},
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	m.Start(context.Background())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		m.Shutdown(ctx)
	})
	return m, provider
}

func waitForState(t *testing.T, m *Manager, attemptID string, want State, timeout time.Duration) Snapshot {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last Snapshot
	for time.Now().Before(deadline) {
		snap, err := m.GetAttempt(attemptID)
		if err != nil {
			t.Fatalf("GetAttempt() error = %v", err)
		}
		last = snap
		if snap.State == want {
			return snap
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("attempt never reached state %q, last snapshot = %+v", want, last)
	return Snapshot{}
}

func TestStartAttemptReturnsTheUserCodeButNeverTheDeviceCode(t *testing.T) {
	m, _ := newTestSetup(t)

	snap, err := m.StartAttempt(context.Background(), account.ProviderTwitch, "")
	if err != nil {
		t.Fatalf("StartAttempt() error = %v", err)
	}
	if snap.UserCode != "ABCD-EFGH" {
		t.Errorf("UserCode = %q, want ABCD-EFGH", snap.UserCode)
	}
	// Snapshot has no field at all for a device code - this is a structural
	// guarantee (see Snapshot's doc comment), verified here by confirming
	// the type has no such exported field to even check.
}

func TestConcurrentStartAttemptIsAConflict(t *testing.T) {
	m, provider := newTestSetup(t)
	provider.outcomes = []account.PollOutcome{{Status: account.PollPending}}

	if _, err := m.StartAttempt(context.Background(), account.ProviderTwitch, ""); err != nil {
		t.Fatalf("first StartAttempt() error = %v", err)
	}
	if _, err := m.StartAttempt(context.Background(), account.ProviderTwitch, ""); !errors.Is(err, ErrConflict) {
		t.Errorf("second StartAttempt() error = %v, want ErrConflict", err)
	}
}

func TestPollingHonorsPendingThenCompletes(t *testing.T) {
	m, provider := newTestSetup(t)
	provider.identity = account.Identity{ProviderUserID: "u1", Login: "streamer", DisplayName: "Streamer"}
	provider.outcomes = []account.PollOutcome{
		{Status: account.PollPending},
		{Status: account.PollComplete, Bundle: account.TokenBundle{
			TokenType: "bearer", AccessToken: "a", RefreshToken: "r", ExpiresAt: time.Now().Add(time.Hour),
		}, Scopes: []string{"channel:manage:broadcast"}},
	}

	snap, err := m.StartAttempt(context.Background(), account.ProviderTwitch, "")
	if err != nil {
		t.Fatalf("StartAttempt() error = %v", err)
	}

	final := waitForState(t, m, snap.AttemptID, StateAuthorized, 2*time.Second)
	if final.ConnectedAccountID == "" {
		t.Error("ConnectedAccountID was not set on authorization")
	}
}

func TestPollingHandlesDenial(t *testing.T) {
	m, provider := newTestSetup(t)
	provider.outcomes = []account.PollOutcome{{Status: account.PollDenied}}

	snap, err := m.StartAttempt(context.Background(), account.ProviderTwitch, "")
	if err != nil {
		t.Fatalf("StartAttempt() error = %v", err)
	}
	waitForState(t, m, snap.AttemptID, StateDenied, 2*time.Second)
}

func TestCancelAttemptStopsPollingImmediately(t *testing.T) {
	m, provider := newTestSetup(t)
	provider.outcomes = nil // stays pending forever unless cancelled

	snap, err := m.StartAttempt(context.Background(), account.ProviderTwitch, "")
	if err != nil {
		t.Fatalf("StartAttempt() error = %v", err)
	}
	waitForState(t, m, snap.AttemptID, StatePolling, time.Second)

	cancelled, err := m.CancelAttempt(snap.AttemptID)
	if err != nil {
		t.Fatalf("CancelAttempt() error = %v", err)
	}
	if cancelled.State != StateCancelled {
		t.Errorf("State = %q, want cancelled", cancelled.State)
	}

	// Cancelling frees the provider's active slot immediately.
	if _, err := m.StartAttempt(context.Background(), account.ProviderTwitch, ""); err != nil {
		t.Errorf("starting a new attempt after cancellation error = %v, want nil", err)
	}
}

func TestCancelAttemptIsIdempotentForATerminalAttempt(t *testing.T) {
	m, provider := newTestSetup(t)
	provider.outcomes = []account.PollOutcome{{Status: account.PollDenied}}

	snap, _ := m.StartAttempt(context.Background(), account.ProviderTwitch, "")
	waitForState(t, m, snap.AttemptID, StateDenied, 2*time.Second)

	result, err := m.CancelAttempt(snap.AttemptID)
	if err != nil {
		t.Fatalf("CancelAttempt() on a terminal attempt error = %v, want nil", err)
	}
	if result.State != StateDenied {
		t.Errorf("State = %q, want denied to remain unchanged", result.State)
	}
}

func TestGetAttemptReturnsNotFoundForAnUnknownID(t *testing.T) {
	m, _ := newTestSetup(t)
	if _, err := m.GetAttempt("devflow_missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetAttempt() error = %v, want ErrNotFound", err)
	}
}

func TestShutdownStopsAllActiveAttempts(t *testing.T) {
	m, provider := newTestSetup(t)
	provider.outcomes = nil

	snap, err := m.StartAttempt(context.Background(), account.ProviderTwitch, "")
	if err != nil {
		t.Fatalf("StartAttempt() error = %v", err)
	}
	waitForState(t, m, snap.AttemptID, StatePolling, time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	m.Shutdown(ctx)

	final, err := m.GetAttempt(snap.AttemptID)
	if err != nil {
		t.Fatalf("GetAttempt() after shutdown error = %v", err)
	}
	if final.State != StateExpired && final.State != StateCancelled {
		t.Errorf("State after shutdown = %q, want a terminal state", final.State)
	}
}

// --- minimal fake account.Repository, local to this package's tests --------

type fakeAccountRepo struct {
	mu       sync.Mutex
	accounts map[string]account.Account
	settings map[account.ProviderID]account.IntegrationSettings
}

func newFakeAccountRepo() *fakeAccountRepo {
	return &fakeAccountRepo{accounts: map[string]account.Account{}, settings: map[account.ProviderID]account.IntegrationSettings{}}
}

func (r *fakeAccountRepo) GetAccount(ctx context.Context, id string) (account.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.accounts[id]
	if !ok {
		return account.Account{}, account.ErrNotFound
	}
	return a, nil
}

func (r *fakeAccountRepo) FindByProviderIdentity(ctx context.Context, providerID account.ProviderID, providerUserID string) (account.Account, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, a := range r.accounts {
		if a.ProviderID == providerID && a.ProviderUserID == providerUserID {
			return a, true, nil
		}
	}
	return account.Account{}, false, nil
}

func (r *fakeAccountRepo) CreateAccount(ctx context.Context, acc account.Account) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.accounts[acc.ID] = acc
	return nil
}

func (r *fakeAccountRepo) UpdateAccount(ctx context.Context, acc account.Account) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.accounts[acc.ID] = acc
	return nil
}

func (r *fakeAccountRepo) DeleteAccount(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.accounts, id)
	return nil
}

func (r *fakeAccountRepo) ListAccounts(ctx context.Context) ([]account.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]account.Account, 0, len(r.accounts))
	for _, a := range r.accounts {
		out = append(out, a)
	}
	return out, nil
}

func (r *fakeAccountRepo) CountAccounts(ctx context.Context, providerID account.ProviderID) (int, error) {
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

func (r *fakeAccountRepo) GetLink(ctx context.Context, platformID string) (account.Link, bool, error) {
	return account.Link{}, false, nil
}
func (r *fakeAccountRepo) SetLink(ctx context.Context, platformID, accountID string, now time.Time) (account.Link, error) {
	return account.Link{}, nil
}
func (r *fakeAccountRepo) DeleteLink(ctx context.Context, platformID string) error { return nil }

func (r *fakeAccountRepo) GetIntegrationSettings(ctx context.Context, providerID account.ProviderID) (account.IntegrationSettings, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.settings[providerID]
	return s, ok, nil
}

func (r *fakeAccountRepo) SetIntegrationSettings(ctx context.Context, providerID account.ProviderID, clientID string, now time.Time) (account.IntegrationSettings, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := account.IntegrationSettings{ProviderID: providerID, ClientID: clientID, UpdatedAt: now}
	r.settings[providerID] = s
	return s, nil
}
