package httpapi

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/account"
	"github.com/streaming-tree/server/internal/provider/twitch"
	"github.com/streaming-tree/server/internal/runtime/deviceflow"
	"github.com/streaming-tree/server/internal/secrets/secretstest"
	"github.com/streaming-tree/server/internal/storage/sqlite"

	"github.com/streaming-tree/server/internal/domain/platform"
)

// TestAccessLogNeverContainsOAuthSecrets proves the requirement directly:
// it captures every line the access-log middleware (withLogging) writes
// during a real device-flow start/poll/cancel sequence and a connected
// account's lifecycle, then scans the captured text for the device code,
// access token and refresh token this test controls via fakeHTTPProvider.
//
// withLogging (middleware.go) only ever logs method, r.URL.Path, status and
// duration - never a query string, a header, or a body - so this is a
// belt-and-braces regression test for that design, not a workaround for a
// leak the middleware might otherwise have.
func TestAccessLogNeverContainsOAuthSecrets(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	path := filepath.Join(t.TempDir(), "test.db")
	db, err := sqlite.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("opening the test database failed: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := sqlite.Migrate(context.Background(), db.DB); err != nil {
		t.Fatalf("migrating the test database failed: %v", err)
	}

	const fakeDeviceCode = "dc-super-secret-device-code"
	const fakeAccessToken = "at-super-secret-access-token"
	const fakeRefreshToken = "rt-super-secret-refresh-token"

	provider := &loggingTestProvider{
		start: account.DeviceFlowStart{
			DeviceCode: fakeDeviceCode, UserCode: "WXYZ-1234", VerificationURI: "https://www.twitch.tv/activate",
			ExpiresIn: time.Minute, Interval: 10 * time.Millisecond,
		},
		outcome: account.PollOutcome{
			Status: account.PollComplete,
			Bundle: account.TokenBundle{TokenType: "bearer", AccessToken: fakeAccessToken, RefreshToken: fakeRefreshToken, ExpiresAt: time.Now().Add(time.Hour)},
			Scopes: []string{"channel:manage:broadcast"},
		},
		identity: account.Identity{ProviderUserID: "u1", Login: "streamer", DisplayName: "Streamer"},
	}
	providers := map[account.ProviderID]account.Provider{account.ProviderTwitch: provider}
	requiredScopes := map[account.ProviderID][]string{account.ProviderTwitch: {"channel:manage:broadcast"}}

	platforms := platform.NewService(sqlite.NewPlatformRepository(db.DB))
	accounts := account.NewService(account.Options{
		Repository: sqlite.NewAccountRepository(db.DB), Secrets: secretstest.New(), Providers: providers,
		RequiredScopes: requiredScopes, Logger: logger,
	})
	if _, err := accounts.SetIntegrationClientID(context.Background(), account.ProviderTwitch, "test-client-id"); err != nil {
		t.Fatalf("SetIntegrationClientID() error = %v", err)
	}
	deviceFlow := deviceflow.NewManager(deviceflow.Options{Accounts: accounts, Providers: providers, RequiredScopes: requiredScopes, Logger: logger})
	deviceFlow.Start(context.Background())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		deviceFlow.Shutdown(ctx)
	})
	twitchMetadata := twitch.NewMetadataService(accounts, twitch.New(twitch.Options{}))

	handler := NewRouter(Options{
		Logger: logger, StartedAt: time.Now(), Platforms: platforms,
		Accounts: accounts, DeviceFlow: deviceFlow, TwitchMetadata: twitchMetadata,
	})

	start := do(t, handler, http.MethodPost, "/api/integrations/twitch/device-flow", nil)
	var snap deviceFlowResponse
	decodeBody(t, start, &snap)

	// Wait for the background poll to authorize (real goroutine, real
	// timing - not mocked away).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		get := do(t, handler, http.MethodGet, "/api/integrations/twitch/device-flow/"+snap.AttemptID, nil)
		var current deviceFlowResponse
		decodeBody(t, get, &current)
		if current.State == "authorized" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Exercise the rest of the connected-account surface too.
	listRecorder := do(t, handler, http.MethodGet, "/api/connected-accounts", nil)
	var list struct {
		Accounts []accountResponse `json:"accounts"`
	}
	decodeBody(t, listRecorder, &list)
	if len(list.Accounts) == 1 {
		acctID := list.Accounts[0].ID
		do(t, handler, http.MethodPost, "/api/connected-accounts/"+acctID+"/validate", nil)
		do(t, handler, http.MethodDelete, "/api/connected-accounts/"+acctID, nil)
	}

	logged := logBuf.String()
	for _, secret := range []string{fakeDeviceCode, fakeAccessToken, fakeRefreshToken} {
		if strings.Contains(logged, secret) {
			t.Errorf("the access log contains a secret value %q:\n%s", secret, logged)
		}
	}
}

type loggingTestProvider struct {
	start    account.DeviceFlowStart
	outcome  account.PollOutcome
	identity account.Identity
	polled   bool
}

func (p *loggingTestProvider) ProviderID() account.ProviderID { return account.ProviderTwitch }
func (p *loggingTestProvider) StartDeviceFlow(ctx context.Context, clientID string, scopes []string) (account.DeviceFlowStart, error) {
	return p.start, nil
}
func (p *loggingTestProvider) PollDeviceFlow(ctx context.Context, clientID, deviceCode string) (account.PollOutcome, error) {
	if p.polled {
		return p.outcome, nil
	}
	p.polled = true
	return account.PollOutcome{Status: account.PollPending}, nil
}
func (p *loggingTestProvider) ValidateToken(ctx context.Context, accessToken string) (account.ValidationResult, error) {
	return account.ValidationResult{Valid: true, ClientID: "test-client-id", Scopes: []string{"channel:manage:broadcast"}}, nil
}
func (p *loggingTestProvider) RefreshToken(ctx context.Context, clientID, refreshToken string) (account.TokenBundle, error) {
	return p.outcome.Bundle, nil
}
func (p *loggingTestProvider) RevokeToken(ctx context.Context, clientID, accessToken string) error {
	return nil
}
func (p *loggingTestProvider) GetIdentity(ctx context.Context, accessToken, clientID string) (account.Identity, error) {
	return p.identity, nil
}
