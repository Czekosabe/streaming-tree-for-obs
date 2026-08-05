package youtube

import (
	"context"
	"fmt"
	"time"

	"github.com/streaming-tree/server/internal/domain/account"
)

// Adapter implements account.Provider over a Client, converting between this
// package's own wire-adjacent return shapes and account's provider-agnostic
// ones. Deliberately does NOT implement account.DeviceFlowProvider: YouTube
// uses Authorization Code Flow with PKCE and a loopback callback (see
// internal/runtime/youtubeauth), not Twitch's device-code polling - see
// account.DeviceFlowProvider's own doc comment.
type Adapter struct {
	client *Client
}

// NewAdapter builds an Adapter.
func NewAdapter(client *Client) *Adapter {
	return &Adapter{client: client}
}

var _ account.Provider = (*Adapter)(nil)

// ProviderID identifies this adapter as the YouTube provider.
func (a *Adapter) ProviderID() account.ProviderID { return account.ProviderYouTube }

// ValidateToken checks an access token via Google's /tokeninfo endpoint.
//
// The result's ProviderUserID is deliberately left empty: Google's
// tokeninfo response carries no channel identity at all (only aud, scope,
// expires_in - docs/provider-integrations/youtube.md), and
// account.Service.acceptValidation never compares ProviderUserID for any
// provider, so this is not a functional gap.
func (a *Adapter) ValidateToken(ctx context.Context, accessToken string) (account.ValidationResult, error) {
	result, err := a.client.ValidateToken(ctx, accessToken)
	if err != nil {
		return account.ValidationResult{}, err
	}
	return account.ValidationResult{
		Valid: result.Valid, ClientID: result.ClientID, Scopes: result.Scopes, ExpiresIn: result.ExpiresIn,
	}, nil
}

// RefreshToken exchanges a refresh token for a new bundle, preserving the
// old refresh token when Google's response omits a new one - see
// Client.RefreshToken's own doc comment.
func (a *Adapter) RefreshToken(ctx context.Context, clientID, refreshToken string) (account.TokenBundle, error) {
	fresh, err := a.client.RefreshToken(ctx, clientID, refreshToken)
	if err != nil {
		return account.TokenBundle{}, err
	}
	return toAccountBundle(fresh), nil
}

// RevokeToken revokes a token. clientID is accepted for interface symmetry
// with account.Provider but unused: Google's /revoke endpoint identifies
// the token by value alone, not by client - see docs/provider-integrations/
// youtube.md's revocation section.
func (a *Adapter) RevokeToken(ctx context.Context, clientID, accessToken string) error {
	_ = clientID
	return a.client.RevokeToken(ctx, accessToken)
}

// GetIdentity resolves the identity behind an access token by listing owned
// channels and requiring exactly one.
//
// In this application's actual code paths, this method is never reached in
// practice: internal/runtime/youtubeauth resolves channel identity (and any
// necessary multi-channel selection) itself, directly through
// Client.ListMyChannels, before ever calling account.Service.
// FinalizeConnection - see docs/provider-integrations/youtube.md's
// "Account/channel identity behavior" section. It is still implemented
// correctly, not stubbed, to honestly satisfy account.Provider.
func (a *Adapter) GetIdentity(ctx context.Context, accessToken, clientID string) (account.Identity, error) {
	_ = clientID
	channels, err := a.client.ListMyChannels(ctx, accessToken)
	if err != nil {
		return account.Identity{}, err
	}
	if len(channels) != 1 {
		return account.Identity{}, fmt.Errorf("%w: expected exactly one channel, found %d", ErrInvalidResponse, len(channels))
	}
	ch := channels[0]
	return account.Identity{ProviderUserID: ch.ID, Login: ch.Title, DisplayName: ch.Title, AvatarURL: ch.ThumbnailURL}, nil
}

func toAccountBundle(w TokenBundleWire) account.TokenBundle {
	return account.TokenBundle{
		TokenType:    w.TokenType,
		AccessToken:  w.AccessToken,
		RefreshToken: w.RefreshToken,
		ExpiresAt:    time.Now().UTC().Add(w.ExpiresIn),
	}
}
