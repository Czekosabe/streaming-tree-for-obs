package twitch

import (
	"context"
	"time"

	"github.com/streaming-tree/server/internal/domain/account"
)

// Adapter implements account.Provider over a Client, converting between this
// package's own wire-adjacent return shapes and account's provider-agnostic
// ones. It is the one place those two vocabularies meet.
type Adapter struct {
	client *Client
}

// NewAdapter builds an Adapter.
func NewAdapter(client *Client) *Adapter {
	return &Adapter{client: client}
}

var _ account.Provider = (*Adapter)(nil)

// ProviderID identifies this adapter as the Twitch provider.
func (a *Adapter) ProviderID() account.ProviderID { return account.ProviderTwitch }

// StartDeviceFlow begins a device-authorization attempt.
func (a *Adapter) StartDeviceFlow(ctx context.Context, clientID string, scopes []string) (account.DeviceFlowStart, error) {
	start, err := a.client.StartDeviceFlow(ctx, clientID, scopes)
	if err != nil {
		return account.DeviceFlowStart{}, err
	}
	return account.DeviceFlowStart{
		DeviceCode:      start.DeviceCode,
		UserCode:        start.UserCode,
		VerificationURI: start.VerificationURI,
		ExpiresIn:       start.ExpiresIn,
		Interval:        start.Interval,
	}, nil
}

// PollDeviceFlow performs one token-exchange attempt.
func (a *Adapter) PollDeviceFlow(ctx context.Context, clientID, deviceCode string) (account.PollOutcome, error) {
	outcome, err := a.client.PollDeviceFlow(ctx, clientID, deviceCode)
	if err != nil {
		return account.PollOutcome{}, err
	}
	result := account.PollOutcome{Status: account.PollStatus(outcome.Status)}
	if outcome.Status == PollComplete {
		result.Bundle = toAccountBundle(outcome.Bundle)
		result.Scopes = outcome.Scopes
	}
	return result, nil
}

// ValidateToken checks an access token.
func (a *Adapter) ValidateToken(ctx context.Context, accessToken string) (account.ValidationResult, error) {
	result, err := a.client.ValidateToken(ctx, accessToken)
	if err != nil {
		return account.ValidationResult{}, err
	}
	return account.ValidationResult{
		Valid: result.Valid, ClientID: result.ClientID, ProviderUserID: result.UserID,
		Scopes: result.Scopes, ExpiresIn: result.ExpiresIn,
	}, nil
}

// RefreshToken exchanges a refresh token for a new bundle.
func (a *Adapter) RefreshToken(ctx context.Context, clientID, refreshToken string) (account.TokenBundle, error) {
	fresh, err := a.client.RefreshToken(ctx, clientID, refreshToken)
	if err != nil {
		return account.TokenBundle{}, err
	}
	return toAccountBundle(fresh), nil
}

// RevokeToken revokes a token.
func (a *Adapter) RevokeToken(ctx context.Context, clientID, accessToken string) error {
	return a.client.RevokeToken(ctx, clientID, accessToken)
}

// GetIdentity resolves the identity behind an access token.
func (a *Adapter) GetIdentity(ctx context.Context, accessToken, clientID string) (account.Identity, error) {
	user, err := a.client.GetCurrentUser(ctx, accessToken, clientID)
	if err != nil {
		return account.Identity{}, err
	}
	return account.Identity{
		ProviderUserID: user.ID, Login: user.Login, DisplayName: user.DisplayName, AvatarURL: user.AvatarURL,
	}, nil
}

func toAccountBundle(w TokenBundleWire) account.TokenBundle {
	return account.TokenBundle{
		TokenType:    w.TokenType,
		AccessToken:  w.AccessToken,
		RefreshToken: w.RefreshToken,
		ExpiresAt:    time.Now().UTC().Add(w.ExpiresIn),
	}
}
