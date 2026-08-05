package twitch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Production endpoints. These are Go constants, not configuration: nothing a
// frontend request can influence ever reaches this client, and a test
// injects DefaultOAuthBaseURL / DefaultAPIBaseURL overrides only through
// Options at construction time, in Go code, never over HTTP.
const (
	DefaultOAuthBaseURL = "https://id.twitch.tv/oauth2"
	DefaultAPIBaseURL   = "https://api.twitch.tv/helix"

	// DefaultEventSubURL is Twitch's production EventSub WebSocket endpoint
	// - see docs/provider-integrations/twitch-engagement.md. Only the
	// -tags integration test binary overrides it (via
	// STREAMING_TREE_TEST_TWITCH_EVENTSUB_BASE_URL, read directly in
	// cmd/testserver/main.go), exactly like OAuthBaseURL/APIBaseURL above.
	DefaultEventSubURL = "wss://eventsub.wss.twitch.tv/ws"
)

// requestTimeout bounds every single HTTP call this client makes.
const requestTimeout = 15 * time.Second

// maxResponseBytes bounds how much of a response body is ever read, so a
// hostile or misbehaving server cannot exhaust memory.
const maxResponseBytes = 1 << 20 // 1 MiB

// Client is this application's HTTP client for Twitch's OAuth and Helix
// APIs.
type Client struct {
	httpClient   *http.Client
	oauthBaseURL string
	apiBaseURL   string
	eventSubURL  string
	clientID     string
}

// Options constructs a Client. OAuthBaseURL, APIBaseURL and EventSubURL are
// test-only overrides (an httptest/fake-WebSocket-server address);
// production code leaves all three zero so the real Twitch endpoints above
// are used.
type Options struct {
	HTTPClient   *http.Client
	OAuthBaseURL string
	APIBaseURL   string
	EventSubURL  string
}

// EventSubURL is the WebSocket endpoint this client's connector should dial
// - the real Twitch production endpoint unless a test override was
// configured (see Options.EventSubURL's doc comment).
func (c *Client) EventSubURL() string {
	return c.eventSubURL
}

// New builds a Client.
func New(opts Options) *Client {
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: requestTimeout,
			// A device-flow verification link is meant to be opened by the
			// user, never silently followed by this backend.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 3 {
					return errors.New("stopped after 3 redirects")
				}
				return nil
			},
		}
	}
	oauthBase := opts.OAuthBaseURL
	if oauthBase == "" {
		oauthBase = DefaultOAuthBaseURL
	}
	apiBase := opts.APIBaseURL
	if apiBase == "" {
		apiBase = DefaultAPIBaseURL
	}
	eventSubBase := opts.EventSubURL
	if eventSubBase == "" {
		eventSubBase = DefaultEventSubURL
	}
	return &Client{httpClient: httpClient, oauthBaseURL: oauthBase, apiBaseURL: apiBase, eventSubURL: eventSubBase}
}

// rateLimit is parsed from Twitch's Ratelimit-* response headers, tolerant
// of them being absent (a non-Helix endpoint, or a fake test server that
// does not set them).
type rateLimit struct {
	limit     int
	remaining int
	resetAt   time.Time
	present   bool
}

func parseRateLimit(h http.Header) rateLimit {
	limit, errL := strconv.Atoi(h.Get("Ratelimit-Limit"))
	remaining, errR := strconv.Atoi(h.Get("Ratelimit-Remaining"))
	resetRaw, errT := strconv.ParseInt(h.Get("Ratelimit-Reset"), 10, 64)
	if errL != nil || errR != nil || errT != nil {
		return rateLimit{}
	}
	return rateLimit{limit: limit, remaining: remaining, resetAt: time.Unix(resetRaw, 0), present: true}
}

// doForm posts a form-encoded body to an id.twitch.tv OAuth endpoint and
// returns the response status and bounded body.
func (c *Client) doForm(ctx context.Context, endpoint string, form url.Values) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.oauthBaseURL+endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return 0, nil, fmt.Errorf("%w: build request: %s", ErrInvalidResponse, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return c.do(req, endpoint)
}

// doHelix issues an authenticated Helix request and returns the response
// status, bounded body, and parsed rate-limit headers.
func (c *Client) doHelix(ctx context.Context, method, endpoint string, query url.Values, body any, accessToken, clientID string) (int, []byte, rateLimit, error) {
	target := c.apiBaseURL + endpoint
	if len(query) > 0 {
		target += "?" + query.Encode()
	}

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return 0, nil, rateLimit{}, fmt.Errorf("%w: encode request body: %s", ErrInvalidResponse, err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return 0, nil, rateLimit{}, fmt.Errorf("%w: build request: %s", ErrInvalidResponse, err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Client-Id", clientID)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	status, respBody, limit, err := c.doWithRateLimit(req, endpoint)
	return status, respBody, limit, err
}

func (c *Client) do(req *http.Request, endpoint string) (int, []byte, error) {
	status, body, _, err := c.doWithRateLimit(req, endpoint)
	return status, body, err
}

func (c *Client) doWithRateLimit(req *http.Request, endpoint string) (int, []byte, rateLimit, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, rateLimit{}, fmt.Errorf("%w: %s: %s", ErrUnavailable, endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()

	limited := io.LimitReader(resp.Body, maxResponseBytes)
	data, err := io.ReadAll(limited)
	if err != nil {
		return resp.StatusCode, nil, rateLimit{}, fmt.Errorf("%w: %s: read response: %s", ErrInvalidResponse, endpoint, err)
	}
	return resp.StatusCode, data, parseRateLimit(resp.Header), nil
}
