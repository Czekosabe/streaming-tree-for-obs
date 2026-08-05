package youtube

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// pkceVerifierAlphabet is exactly RFC 7636's unreserved character set, which
// Google's own documentation quotes verbatim (docs/provider-integrations/
// youtube.md).
const pkceVerifierAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~"

// pkceVerifierLength is within RFC 7636's 43-128 character range, at the
// high end for extra entropy.
const pkceVerifierLength = 96

// GeneratePKCEVerifier returns a cryptographically random PKCE code
// verifier.
func GeneratePKCEVerifier() (string, error) {
	buf := make([]byte, pkceVerifierLength)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate pkce verifier: %w", err)
	}
	out := make([]byte, pkceVerifierLength)
	for i, b := range buf {
		out[i] = pkceVerifierAlphabet[int(b)%len(pkceVerifierAlphabet)]
	}
	return string(out), nil
}

// DeriveS256Challenge computes the PKCE code_challenge for a verifier, using
// the S256 method - the only method this application ever uses (see
// docs/provider-integrations/youtube.md).
func DeriveS256Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// GenerateState returns a cryptographically random CSRF state value.
func GenerateState() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate oauth state: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// AuthorizationURLInput carries everything BuildAuthorizationURL needs.
type AuthorizationURLInput struct {
	ClientID      string
	RedirectURI   string
	Scopes        []string
	State         string
	CodeChallenge string
}

// BuildAuthorizationURL constructs Google's authorization endpoint URL for
// this application's Desktop Authorization Code + PKCE flow.
//
// access_type=offline and prompt=consent are always included - see
// docs/provider-integrations/youtube.md's "access_type and prompt" section
// for why prompt=consent is unconditional rather than only sent when no
// refresh token was previously seen.
func (c *Client) BuildAuthorizationURL(in AuthorizationURLInput) string {
	q := url.Values{
		"client_id":             {in.ClientID},
		"redirect_uri":          {in.RedirectURI},
		"response_type":         {"code"},
		"scope":                 {strings.Join(in.Scopes, " ")},
		"state":                 {in.State},
		"code_challenge":        {in.CodeChallenge},
		"code_challenge_method": {"S256"},
		"access_type":           {"offline"},
		"prompt":                {"consent"},
	}
	return c.authBaseURL + "/o/oauth2/v2/auth?" + q.Encode()
}

// TokenBundleWire is this client's return shape for a successful token
// exchange or refresh - kept separate from account.TokenBundle so this
// package has no import-time dependency on internal/domain/account; the
// adapter (adapter.go) converts between the two.
type TokenBundleWire struct {
	AccessToken  string
	RefreshToken string
	TokenType    string
	ExpiresIn    time.Duration
}

// ExchangeCode exchanges an authorization code for a token bundle: POST
// /token with grant_type=authorization_code. No client_secret is ever sent -
// this application registers a Desktop-app OAuth client, which does not
// require one (docs/provider-integrations/youtube.md).
func (c *Client) ExchangeCode(ctx context.Context, clientID, code, codeVerifier, redirectURI string) (TokenBundleWire, []string, error) {
	form := url.Values{
		"client_id":     {clientID},
		"code":          {code},
		"code_verifier": {codeVerifier},
		"redirect_uri":  {redirectURI},
		"grant_type":    {"authorization_code"},
	}
	status, body, err := c.doForm(ctx, "/token", form)
	if err != nil {
		return TokenBundleWire{}, nil, err
	}
	if status != http.StatusOK {
		return TokenBundleWire{}, nil, classifyTokenError(status, body, "/token")
	}
	return parseTokenResponse(body, "/token")
}

// RefreshToken exchanges a refresh token for a new access token: POST
// /token with grant_type=refresh_token. Google's response typically omits a
// new refresh_token; when it does, this method preserves the caller's
// existing refreshToken in the returned bundle rather than ever returning
// an empty one (docs/provider-integrations/youtube.md's refresh-token
// section) - this is the one place this package deliberately does not
// mirror Twitch's always-rotates assumption.
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
		return TokenBundleWire{}, classifyTokenError(status, body, "/token (refresh)")
	}
	bundle, _, err := parseTokenResponse(body, "/token (refresh)")
	if err != nil {
		return TokenBundleWire{}, err
	}
	if bundle.RefreshToken == "" {
		bundle.RefreshToken = refreshToken
	}
	return bundle, nil
}

// RevokeToken revokes a token: POST /revoke. Google reporting the token as
// already invalid is treated as success too, matching the disconnect-
// ordering rule docs/provider-integrations/twitch.md already established
// for Twitch and docs/provider-integrations/youtube.md restates for
// YouTube: a token Google has already expired (Testing-mode's 7-day cliff,
// for instance) must never block a local disconnect.
func (c *Client) RevokeToken(ctx context.Context, token string) error {
	form := url.Values{"token": {token}}
	status, body, err := c.doForm(ctx, "/revoke", form)
	if err != nil {
		return err
	}
	switch status {
	case http.StatusOK, http.StatusBadRequest:
		return nil
	default:
		return classifyTokenError(status, body, "/revoke")
	}
}

// ValidateResult is this client's own return shape for /tokeninfo.
type ValidateResult struct {
	Valid     bool
	ClientID  string
	Scopes    []string
	ExpiresIn time.Duration
}

// ValidateToken checks a token: GET /tokeninfo?access_token=... - confirmed
// via Google's own API reference to return aud/scope/expires_in
// (docs/provider-integrations/youtube.md).
func (c *Client) ValidateToken(ctx context.Context, accessToken string) (ValidateResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.oauthBaseURL+"/tokeninfo?"+url.Values{"access_token": {accessToken}}.Encode(), nil)
	if err != nil {
		return ValidateResult{}, fmt.Errorf("%w: build request: %s", ErrInvalidResponse, err)
	}
	status, body, err := c.do(req, "/tokeninfo")
	if err != nil {
		return ValidateResult{}, err
	}
	if status != http.StatusOK {
		return ValidateResult{Valid: false}, nil
	}

	var parsed tokenInfoResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return ValidateResult{}, fmt.Errorf("%w: /tokeninfo: %s", ErrInvalidResponse, err)
	}
	expiresIn, _ := strconv.Atoi(parsed.ExpiresIn)
	scopes := []string{}
	if parsed.Scope != "" {
		scopes = strings.Fields(parsed.Scope)
	}
	return ValidateResult{
		Valid: true, ClientID: parsed.Audience, Scopes: scopes,
		ExpiresIn: time.Duration(expiresIn) * time.Second,
	}, nil
}

func parseTokenResponse(body []byte, endpoint string) (TokenBundleWire, []string, error) {
	var parsed tokenResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return TokenBundleWire{}, nil, fmt.Errorf("%w: %s: %s", ErrInvalidResponse, endpoint, err)
	}
	if parsed.AccessToken == "" {
		return TokenBundleWire{}, nil, fmt.Errorf("%w: %s: missing a required field", ErrInvalidResponse, endpoint)
	}
	tokenType := strings.ToLower(parsed.TokenType)
	if tokenType == "" {
		tokenType = "bearer"
	}
	scopes := []string{}
	if parsed.Scope != "" {
		scopes = strings.Fields(parsed.Scope)
	}
	return TokenBundleWire{
		AccessToken: parsed.AccessToken, RefreshToken: parsed.RefreshToken,
		TokenType: tokenType, ExpiresIn: time.Duration(parsed.ExpiresIn) * time.Second,
	}, scopes, nil
}

// classifyTokenError maps a non-200 /token or /revoke response, recognizing
// Google's standard {"error":"invalid_grant",...} shape so a dead refresh
// token maps to ErrInvalidGrant rather than a generic failure.
func classifyTokenError(status int, body []byte, endpoint string) error {
	var parsed tokenErrorResponse
	if json.Unmarshal(body, &parsed) == nil && parsed.Error == "invalid_grant" {
		return fmt.Errorf("%w: %s", ErrInvalidGrant, endpoint)
	}
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
