package twitch

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestClient(t *testing.T, oauth, api *httptest.Server) *Client {
	t.Helper()
	opts := Options{}
	if oauth != nil {
		opts.OAuthBaseURL = oauth.URL
		t.Cleanup(oauth.Close)
	}
	if api != nil {
		opts.APIBaseURL = api.URL
		t.Cleanup(api.Close)
	}
	return New(opts)
}

func jsonHandler(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}
}

// --- device flow -----------------------------------------------------------

func TestStartDeviceFlowParsesTheRealResponseShape(t *testing.T) {
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/device" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		jsonHandler(http.StatusOK, `{"device_code":"dc123","user_code":"ABCD-EFGH","verification_uri":"https://www.twitch.tv/activate","expires_in":1800,"interval":5}`)(w, r)
	}))
	client := newTestClient(t, oauth, nil)

	start, err := client.StartDeviceFlow(context.Background(), "cid", []string{"channel:manage:broadcast"})
	if err != nil {
		t.Fatalf("StartDeviceFlow() error = %v", err)
	}
	if start.DeviceCode != "dc123" || start.UserCode != "ABCD-EFGH" {
		t.Errorf("StartDeviceFlow() = %+v", start)
	}
}

func TestPollDeviceFlowHandlesEveryDocumentedStatus(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   PollStatus
	}{
		{"pending", http.StatusBadRequest, `{"status":400,"message":"authorization_pending"}`, PollPending},
		{"slow_down", http.StatusBadRequest, `{"status":400,"message":"slow_down"}`, PollSlowDown},
		{"denied", http.StatusBadRequest, `{"status":400,"message":"access_denied"}`, PollDenied},
		{"expired", http.StatusBadRequest, `{"status":400,"message":"expired_token"}`, PollExpired},
		{"already exchanged", http.StatusBadRequest, `{"status":400,"message":"invalid device code"}`, PollExpired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			oauth := httptest.NewServer(jsonHandler(tc.status, tc.body))
			client := newTestClient(t, oauth, nil)

			outcome, err := client.PollDeviceFlow(context.Background(), "cid", "dc123")
			if err != nil {
				t.Fatalf("PollDeviceFlow() error = %v", err)
			}
			if outcome.Status != tc.want {
				t.Errorf("Status = %q, want %q", outcome.Status, tc.want)
			}
		})
	}
}

func TestPollDeviceFlowParsesASuccessfulExchange(t *testing.T) {
	oauth := httptest.NewServer(jsonHandler(http.StatusOK,
		`{"access_token":"at","expires_in":14400,"refresh_token":"rt","scope":["channel:manage:broadcast"],"token_type":"bearer"}`))
	client := newTestClient(t, oauth, nil)

	outcome, err := client.PollDeviceFlow(context.Background(), "cid", "dc123")
	if err != nil {
		t.Fatalf("PollDeviceFlow() error = %v", err)
	}
	if outcome.Status != PollComplete {
		t.Fatalf("Status = %q, want complete", outcome.Status)
	}
	if outcome.Bundle.AccessToken != "at" || outcome.Bundle.RefreshToken != "rt" {
		t.Errorf("Bundle = %+v", outcome.Bundle)
	}
	if len(outcome.Scopes) != 1 || outcome.Scopes[0] != "channel:manage:broadcast" {
		t.Errorf("Scopes = %v", outcome.Scopes)
	}
}

func TestPollDeviceFlowNeverLogsOrLeaksTheDeviceCodeOnError(t *testing.T) {
	oauth := httptest.NewServer(jsonHandler(http.StatusOK, `not json`))
	client := newTestClient(t, oauth, nil)

	_, err := client.PollDeviceFlow(context.Background(), "cid", "super-secret-device-code")
	if err == nil {
		t.Fatal("expected an error for a malformed response")
	}
	if strings.Contains(err.Error(), "super-secret-device-code") {
		t.Errorf("error message leaked the device code: %v", err)
	}
}

// --- validate / refresh / revoke -------------------------------------------

func TestValidateTokenParsesSuccessAndFailure(t *testing.T) {
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "OAuth good-token" {
			jsonHandler(http.StatusUnauthorized, `{"status":401,"message":"invalid access token"}`)(w, r)
			return
		}
		jsonHandler(http.StatusOK, `{"client_id":"cid","login":"streamer","user_id":"u1","scopes":["channel:manage:broadcast"],"expires_in":3600}`)(w, r)
	}))
	client := newTestClient(t, oauth, nil)

	good, err := client.ValidateToken(context.Background(), "good-token")
	if err != nil || !good.Valid || good.ClientID != "cid" {
		t.Fatalf("ValidateToken(good) = %+v, %v", good, err)
	}

	bad, err := client.ValidateToken(context.Background(), "bad-token")
	if err != nil || bad.Valid {
		t.Fatalf("ValidateToken(bad) = %+v, %v, want Valid=false with no error", bad, err)
	}
}

func TestRefreshTokenNeverSendsAClientSecret(t *testing.T) {
	var capturedBody string
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		capturedBody = r.PostForm.Encode()
		jsonHandler(http.StatusOK, `{"access_token":"new-a","expires_in":14400,"refresh_token":"new-r","scope":["channel:manage:broadcast"],"token_type":"bearer"}`)(w, r)
	}))
	client := newTestClient(t, oauth, nil)

	fresh, err := client.RefreshToken(context.Background(), "cid", "old-refresh-token")
	if err != nil {
		t.Fatalf("RefreshToken() error = %v", err)
	}
	if fresh.AccessToken != "new-a" || fresh.RefreshToken != "new-r" {
		t.Errorf("RefreshToken() = %+v", fresh)
	}
	if strings.Contains(capturedBody, "client_secret") {
		t.Errorf("the refresh request included a client_secret field: %s", capturedBody)
	}
}

func TestRevokeTokenTreatsAnAlreadyInvalidTokenAsSuccess(t *testing.T) {
	oauth := httptest.NewServer(jsonHandler(http.StatusBadRequest, `{"status":400,"message":"Invalid token"}`))
	client := newTestClient(t, oauth, nil)

	if err := client.RevokeToken(context.Background(), "cid", "already-gone"); err != nil {
		t.Errorf("RevokeToken() error = %v, want nil for an already-invalid token", err)
	}
}

func TestRevokeTokenReturnsAnErrorForATransientFailure(t *testing.T) {
	oauth := httptest.NewServer(jsonHandler(http.StatusInternalServerError, `{"status":500,"message":"boom"}`))
	client := newTestClient(t, oauth, nil)

	if err := client.RevokeToken(context.Background(), "cid", "token"); !errors.Is(err, ErrUnavailable) {
		t.Errorf("RevokeToken() error = %v, want ErrUnavailable", err)
	}
}

// --- helix -----------------------------------------------------------------

func TestGetCurrentUserParsesTheRealResponseShape(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Client-Id") != "cid" || r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("missing required headers: %v", r.Header)
		}
		jsonHandler(http.StatusOK, `{"data":[{"id":"123","login":"streamer","display_name":"Streamer","profile_image_url":"https://example.invalid/a.png"}]}`)(w, r)
	}))
	client := newTestClient(t, nil, api)

	user, err := client.GetCurrentUser(context.Background(), "tok", "cid")
	if err != nil {
		t.Fatalf("GetCurrentUser() error = %v", err)
	}
	if user.ID != "123" || user.Login != "streamer" {
		t.Errorf("GetCurrentUser() = %+v", user)
	}
}

func TestGetCurrentUserRejectsAnEmptyDataArray(t *testing.T) {
	api := httptest.NewServer(jsonHandler(http.StatusOK, `{"data":[]}`))
	client := newTestClient(t, nil, api)

	if _, err := client.GetCurrentUser(context.Background(), "tok", "cid"); !errors.Is(err, ErrInvalidResponse) {
		t.Errorf("GetCurrentUser() error = %v, want ErrInvalidResponse", err)
	}
}

func TestModifyChannelSendsOnlyVerifiedFields(t *testing.T) {
	var captured map[string]any
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("method = %s, want PATCH", r.Method)
		}
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.WriteHeader(http.StatusNoContent)
	}))
	client := newTestClient(t, nil, api)

	title := "New title"
	gameID := "1469308723"
	err := client.ModifyChannel(context.Background(), "123", ModifyChannelInput{Title: &title, GameID: &gameID, Tags: []string{"go"}}, "tok", "cid")
	if err != nil {
		t.Fatalf("ModifyChannel() error = %v", err)
	}
	for _, forbidden := range []string{"delay", "content_classification_labels", "is_branded_content"} {
		if _, present := captured[forbidden]; present {
			t.Errorf("ModifyChannel request body included the unsupported field %q: %v", forbidden, captured)
		}
	}
	if captured["title"] != "New title" || captured["game_id"] != "1469308723" {
		t.Errorf("captured body = %v", captured)
	}
}

func TestSearchCategoriesToleratesAMalformedSingleEntry(t *testing.T) {
	api := httptest.NewServer(jsonHandler(http.StatusOK,
		`{"data":[{"id":"1","name":"Good Category","box_art_url":"https://example.invalid/b.png"},{"id":"","name":""}]}`))
	client := newTestClient(t, nil, api)

	results, err := client.SearchCategories(context.Background(), "test query", "tok", "cid")
	if err != nil {
		t.Fatalf("SearchCategories() error = %v", err)
	}
	if len(results) != 1 || results[0].ID != "1" {
		t.Errorf("SearchCategories() = %+v, want exactly the one well-formed entry", results)
	}
}

func TestSearchCategoriesRejectsAnEmptyQuery(t *testing.T) {
	client := newTestClient(t, nil, nil)
	if _, err := client.SearchCategories(context.Background(), "   ", "tok", "cid"); err == nil {
		t.Error("expected an error for an empty query")
	}
}

func TestHelixRateLimitIsParsedFromHeaders(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Ratelimit-Limit", "800")
		w.Header().Set("Ratelimit-Remaining", "799")
		w.Header().Set("Ratelimit-Reset", "1735689600")
		jsonHandler(http.StatusOK, `{"data":[{"id":"123","login":"streamer","display_name":"Streamer"}]}`)(w, r)
	}))
	client := newTestClient(t, nil, api)

	_, _, limit, err := client.doHelix(context.Background(), http.MethodGet, "/users", nil, nil, "tok", "cid")
	if err != nil {
		t.Fatalf("doHelix() error = %v", err)
	}
	if !limit.present || limit.remaining != 799 {
		t.Errorf("rate limit = %+v, want remaining 799", limit)
	}
}

func TestHelix429MapsToRateLimited(t *testing.T) {
	api := httptest.NewServer(jsonHandler(http.StatusTooManyRequests, `{"error":"Too Many Requests","status":429,"message":"rate limited"}`))
	client := newTestClient(t, nil, api)

	if _, err := client.GetCurrentUser(context.Background(), "tok", "cid"); !errors.Is(err, ErrRateLimited) {
		t.Errorf("GetCurrentUser() error = %v, want ErrRateLimited", err)
	}
}

func TestHelix401MapsToUnauthorized(t *testing.T) {
	api := httptest.NewServer(jsonHandler(http.StatusUnauthorized, `{"error":"Unauthorized","status":401,"message":"invalid token"}`))
	client := newTestClient(t, nil, api)

	if _, err := client.GetCurrentUser(context.Background(), "tok", "cid"); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("GetCurrentUser() error = %v, want ErrUnauthorized", err)
	}
}
