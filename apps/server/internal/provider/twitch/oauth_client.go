package twitch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// StartDeviceFlow begins a device-authorization attempt: POST /oauth2/device
// (docs/provider-integrations/twitch.md).
func (c *Client) StartDeviceFlow(ctx context.Context, clientID string, scopes []string) (DeviceFlowStartResult, error) {
	form := url.Values{
		"client_id": {clientID},
		"scopes":    {strings.Join(scopes, " ")},
	}

	status, body, err := c.doForm(ctx, "/device", form)
	if err != nil {
		return DeviceFlowStartResult{}, err
	}
	if status != http.StatusOK {
		return DeviceFlowStartResult{}, classifyOAuthError(status, body, "/oauth2/device")
	}

	var parsed deviceFlowStartResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return DeviceFlowStartResult{}, fmt.Errorf("%w: /oauth2/device: %s", ErrInvalidResponse, err)
	}
	if parsed.DeviceCode == "" || parsed.UserCode == "" || parsed.VerificationURI == "" {
		return DeviceFlowStartResult{}, fmt.Errorf("%w: /oauth2/device: missing a required field", ErrInvalidResponse)
	}
	if parsed.ExpiresIn <= 0 {
		parsed.ExpiresIn = 1800
	}
	if parsed.Interval <= 0 {
		parsed.Interval = 5
	}

	return DeviceFlowStartResult{
		DeviceCode:      parsed.DeviceCode,
		UserCode:        parsed.UserCode,
		VerificationURI: parsed.VerificationURI,
		ExpiresIn:       time.Duration(parsed.ExpiresIn) * time.Second,
		Interval:        time.Duration(parsed.Interval) * time.Second,
	}, nil
}

// PollStatus is this client's own device-flow poll outcome, distinct from
// account.PollStatus - adapter.go converts between the two.
type PollStatus string

const (
	PollPending  PollStatus = "pending"
	PollSlowDown PollStatus = "slow_down"
	PollDenied   PollStatus = "denied"
	PollExpired  PollStatus = "expired"
	PollComplete PollStatus = "complete"
)

// DeviceFlowStartResult is this client's own return shape for
// POST /oauth2/device.
type DeviceFlowStartResult struct {
	DeviceCode      string
	UserCode        string
	VerificationURI string
	ExpiresIn       time.Duration
	Interval        time.Duration
}

// PollResult is this client's own return shape for one device-flow poll
// attempt. Bundle and Scopes are populated only when Status is PollComplete.
type PollResult struct {
	Status PollStatus
	Bundle TokenBundleWire
	Scopes []string
}

// PollDeviceFlow performs one token-exchange attempt: POST /oauth2/token
// with the device-code grant type.
func (c *Client) PollDeviceFlow(ctx context.Context, clientID, deviceCode string) (PollResult, error) {
	form := url.Values{
		"client_id":   {clientID},
		"device_code": {deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}

	status, body, err := c.doForm(ctx, "/token", form)
	if err != nil {
		return PollResult{}, err
	}

	if status == http.StatusOK {
		bundle, scopes, err := parseTokenResponse(body, "/oauth2/token")
		if err != nil {
			return PollResult{}, err
		}
		return PollResult{Status: PollComplete, Bundle: bundle, Scopes: scopes}, nil
	}

	var msg statusMessageResponse
	if err := json.Unmarshal(body, &msg); err != nil {
		return PollResult{}, fmt.Errorf("%w: /oauth2/token: %s", ErrInvalidResponse, err)
	}

	switch strings.ToLower(msg.Message) {
	case "authorization_pending":
		return PollResult{Status: PollPending}, nil
	case "slow_down":
		return PollResult{Status: PollSlowDown}, nil
	case "access_denied", "authorization_declined":
		return PollResult{Status: PollDenied}, nil
	case "expired_token", "invalid device code":
		// Twitch documents "invalid device code" as the response after a
		// device_code has already been exchanged once; in the context of
		// ongoing polling for a code that was never successfully exchanged
		// before, this is indistinguishable from expiry from the caller's
		// point of view, and is treated the same way: stop polling, offer a
		// fresh attempt.
		return PollResult{Status: PollExpired}, nil
	default:
		return PollResult{}, fmt.Errorf("%w: /oauth2/token: unrecognized status %q", ErrInvalidResponse, msg.Message)
	}
}

// RefreshToken exchanges a refresh token for a new bundle: POST /oauth2/token
// with the refresh_token grant type. No client_secret is sent - this
// application only ever registers a public client (see
// docs/provider-integrations/twitch.md).
func (c *Client) RefreshToken(ctx context.Context, clientID, refreshToken string) (TokenBundleWire, error) {
	form := url.Values{
		"client_id":     {clientID},
		"refresh_token": {refreshToken},
		"grant_type":    {"refresh_token"},
	}

	status, body, err := c.doForm(ctx, "/token", form)
	if err != nil {
		return TokenBundleWire{}, err
	}
	if status != http.StatusOK {
		return TokenBundleWire{}, classifyOAuthError(status, body, "/oauth2/token (refresh)")
	}
	bundle, _, err := parseTokenResponse(body, "/oauth2/token (refresh)")
	return bundle, err
}

// ValidateToken checks a token: GET /oauth2/validate.
func (c *Client) ValidateToken(ctx context.Context, accessToken string) (ValidateResult, error) {
	req, err := newValidateRequest(ctx, c.oauthBaseURL, accessToken)
	if err != nil {
		return ValidateResult{}, fmt.Errorf("%w: build request: %s", ErrInvalidResponse, err)
	}

	status, body, err := c.do(req, "/oauth2/validate")
	if err != nil {
		return ValidateResult{}, err
	}

	if status == http.StatusUnauthorized {
		return ValidateResult{Valid: false}, nil
	}
	if status != http.StatusOK {
		return ValidateResult{}, classifyOAuthError(status, body, "/oauth2/validate")
	}

	var parsed validateResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return ValidateResult{}, fmt.Errorf("%w: /oauth2/validate: %s", ErrInvalidResponse, err)
	}
	return ValidateResult{
		Valid:     true,
		ClientID:  parsed.ClientID,
		UserID:    parsed.UserID,
		Scopes:    parsed.Scopes,
		ExpiresIn: time.Duration(parsed.ExpiresIn) * time.Second,
	}, nil
}

// RevokeToken revokes a token: POST /oauth2/revoke. Twitch reporting the
// token as already invalid (400) is treated as success, matching the task's
// disconnect-ordering rule; only a genuine transient failure is returned as
// an error.
func (c *Client) RevokeToken(ctx context.Context, clientID, accessToken string) error {
	form := url.Values{
		"client_id": {clientID},
		"token":     {accessToken},
	}

	status, body, err := c.doForm(ctx, "/revoke", form)
	if err != nil {
		return err
	}
	switch status {
	case http.StatusOK, http.StatusBadRequest:
		return nil
	default:
		return classifyOAuthError(status, body, "/oauth2/revoke")
	}
}

// ValidateResult is this client's own return shape for /oauth2/validate,
// distinct from the wire validateResponse.
type ValidateResult struct {
	Valid     bool
	ClientID  string
	UserID    string
	Scopes    []string
	ExpiresIn time.Duration
}

// TokenBundleWire is this client's return shape for a successful token
// exchange - kept separate from account.TokenBundle so this package has no
// import-time dependency on internal/domain/account; the adapter
// (adapter.go) converts between the two.
type TokenBundleWire struct {
	AccessToken  string
	RefreshToken string
	TokenType    string
	ExpiresIn    time.Duration
}

func parseTokenResponse(body []byte, endpoint string) (TokenBundleWire, []string, error) {
	var parsed tokenResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return TokenBundleWire{}, nil, fmt.Errorf("%w: %s: %s", ErrInvalidResponse, endpoint, err)
	}
	if parsed.AccessToken == "" || parsed.RefreshToken == "" {
		return TokenBundleWire{}, nil, fmt.Errorf("%w: %s: missing a required field", ErrInvalidResponse, endpoint)
	}
	tokenType := strings.ToLower(parsed.TokenType)
	if tokenType == "" {
		tokenType = "bearer"
	}
	scopes := parsed.Scope
	if scopes == nil {
		scopes = []string{}
	}
	return TokenBundleWire{
		AccessToken:  parsed.AccessToken,
		RefreshToken: parsed.RefreshToken,
		TokenType:    tokenType,
		ExpiresIn:    time.Duration(parsed.ExpiresIn) * time.Second,
	}, scopes, nil
}

func classifyOAuthError(status int, body []byte, endpoint string) error {
	switch status {
	case http.StatusUnauthorized:
		return wireErr(ErrUnauthorized, status, endpoint)
	case http.StatusForbidden:
		return wireErr(ErrForbidden, status, endpoint)
	case http.StatusTooManyRequests:
		return wireErr(ErrRateLimited, status, endpoint)
	}
	if status >= 500 {
		return wireErr(ErrUnavailable, status, endpoint)
	}
	return wireErr(ErrInvalidResponse, status, endpoint)
}

func newValidateRequest(ctx context.Context, oauthBaseURL, accessToken string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, oauthBaseURL+"/validate", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "OAuth "+accessToken)
	return req, nil
}
