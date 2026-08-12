package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Production endpoints. These are Go constants, not configuration: nothing a
// frontend request can influence ever reaches this client, and a test
// injects DefaultOAuthBaseURL / DefaultAPIBaseURL / DefaultAuthBaseURL
// overrides only through Options at construction time, in Go code, never
// over HTTP.
const (
	// DefaultAuthBaseURL is Google's authorization endpoint host - the one
	// endpoint this client never calls itself; it only builds the URL for
	// the frontend to open in a real browser (see oauth_client.go).
	DefaultAuthBaseURL  = "https://accounts.google.com"
	DefaultOAuthBaseURL = "https://oauth2.googleapis.com"
	DefaultAPIBaseURL   = "https://www.googleapis.com/youtube/v3"
)

// requestTimeout bounds every single HTTP call this client makes.
const requestTimeout = 15 * time.Second

// maxResponseBytes bounds how much of a response body is ever read, so a
// hostile or misbehaving server cannot exhaust memory.
const maxResponseBytes = 1 << 20 // 1 MiB

// Client is this application's HTTP client for Google's OAuth and the
// YouTube Data/Live Streaming APIs.
type Client struct {
	httpClient   *http.Client
	authBaseURL  string
	oauthBaseURL string
	apiBaseURL   string
}

// Options constructs a Client. AuthBaseURL, OAuthBaseURL and APIBaseURL are
// test-only overrides (an httptest server address); production code leaves
// all three zero so the real Google endpoints above are used.
type Options struct {
	HTTPClient   *http.Client
	AuthBaseURL  string
	OAuthBaseURL string
	APIBaseURL   string
}

// New builds a Client.
func New(opts Options) *Client {
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: requestTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 3 {
					return errors.New("stopped after 3 redirects")
				}
				return nil
			},
		}
	}
	authBase := opts.AuthBaseURL
	if authBase == "" {
		authBase = DefaultAuthBaseURL
	}
	oauthBase := opts.OAuthBaseURL
	if oauthBase == "" {
		oauthBase = DefaultOAuthBaseURL
	}
	apiBase := opts.APIBaseURL
	if apiBase == "" {
		apiBase = DefaultAPIBaseURL
	}
	return &Client{httpClient: httpClient, authBaseURL: authBase, oauthBaseURL: oauthBase, apiBaseURL: apiBase}
}

func (c *Client) doForm(ctx context.Context, endpoint string, form url.Values) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.oauthBaseURL+endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return 0, nil, fmt.Errorf("%w: build request: %s", ErrInvalidResponse, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.do(req, endpoint)
}

// doAPI issues an authenticated YouTube Data API request.
func (c *Client) doAPI(ctx context.Context, method, endpoint string, query url.Values, body any, accessToken string) (int, []byte, error) {
	target := c.apiBaseURL + endpoint
	if len(query) > 0 {
		target += "?" + query.Encode()
	}

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return 0, nil, fmt.Errorf("%w: encode request body: %s", ErrInvalidResponse, err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return 0, nil, fmt.Errorf("%w: build request: %s", ErrInvalidResponse, err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.do(req, endpoint)
}

func (c *Client) do(req *http.Request, endpoint string) (int, []byte, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("%w: %s: %s", ErrUnavailable, endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	contentType := resp.Header.Get("Content-Type")
	if contentType != "" && !strings.Contains(contentType, "json") && !strings.Contains(contentType, "text/plain") {
		return resp.StatusCode, nil, fmt.Errorf("%w: %s: unexpected content type %q", ErrInvalidResponse, endpoint, contentType)
	}

	limited := io.LimitReader(resp.Body, maxResponseBytes)
	data, err := io.ReadAll(limited)
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("%w: %s: read response: %s", ErrInvalidResponse, endpoint, err)
	}
	return resp.StatusCode, data, nil
}

// classifyAPIError maps a non-2xx YouTube Data API response to a sanitized
// sentinel error, using the standard Google error envelope's errors[].reason
// when present (docs/provider-integrations/youtube.md's error mapping).
func classifyAPIError(status int, body []byte, endpoint string) error {
	reason := ""
	var parsed googleAPIErrorResponse
	if json.Unmarshal(body, &parsed) == nil && len(parsed.Error.Errors) > 0 {
		reason = parsed.Error.Errors[0].Reason
	}

	switch {
	case status == http.StatusUnauthorized:
		return wireErr(ErrUnauthorized, status, endpoint)
	case reason == "liveStreamingNotEnabled":
		return fmt.Errorf("%w: %s", ErrLiveStreamingNotEnabled, endpoint)
	case reason == "liveChatDisabled":
		return fmt.Errorf("%w: %s", ErrLiveChatDisabled, endpoint)
	case reason == "liveChatEnded":
		return fmt.Errorf("%w: %s", ErrLiveChatEnded, endpoint)
	case reason == "liveChatNotFound":
		return fmt.Errorf("%w: %s", ErrLiveChatNotFound, endpoint)
	case reason == "messageTextInvalid" || reason == "messageTextRequired" ||
		reason == "liveChatIdRequired" || reason == "typeRequired":
		return fmt.Errorf("%w: %s", ErrMessageInvalid, endpoint)
	case reason == "quotaExceeded":
		return fmt.Errorf("%w: %s", ErrQuotaExceeded, endpoint)
	case reason == "rateLimitExceeded" || status == http.StatusTooManyRequests:
		return wireErr(ErrRateLimited, status, endpoint)
	case status == http.StatusForbidden:
		return wireErr(ErrForbidden, status, endpoint)
	case status >= 500:
		return wireErr(ErrUnavailable, status, endpoint)
	default:
		return wireErr(ErrInvalidResponse, status, endpoint)
	}
}
