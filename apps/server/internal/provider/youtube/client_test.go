package youtube

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func newTestClient(t *testing.T, auth, oauth, api *httptest.Server) *Client {
	t.Helper()
	opts := Options{}
	if auth != nil {
		opts.AuthBaseURL = auth.URL
		t.Cleanup(auth.Close)
	}
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

// --- PKCE --------------------------------------------------------------

func TestGeneratePKCEVerifierMeetsRFC7636Shape(t *testing.T) {
	verifier, err := GeneratePKCEVerifier()
	if err != nil {
		t.Fatalf("GeneratePKCEVerifier() error = %v", err)
	}
	if len(verifier) < 43 || len(verifier) > 128 {
		t.Errorf("verifier length = %d, want between 43 and 128", len(verifier))
	}
	for _, r := range verifier {
		if !strings.ContainsRune(pkceVerifierAlphabet, r) {
			t.Errorf("verifier contains disallowed rune %q", r)
		}
	}
}

func TestGeneratePKCEVerifierIsRandom(t *testing.T) {
	a, err := GeneratePKCEVerifier()
	if err != nil {
		t.Fatalf("GeneratePKCEVerifier() error = %v", err)
	}
	b, err := GeneratePKCEVerifier()
	if err != nil {
		t.Fatalf("GeneratePKCEVerifier() error = %v", err)
	}
	if a == b {
		t.Error("two generated verifiers were identical")
	}
}

func TestDeriveS256ChallengeIsDeterministicAndURLSafe(t *testing.T) {
	challenge1 := DeriveS256Challenge("fixed-verifier-value")
	challenge2 := DeriveS256Challenge("fixed-verifier-value")
	if challenge1 != challenge2 {
		t.Error("DeriveS256Challenge() is not deterministic for the same verifier")
	}
	if strings.ContainsAny(challenge1, "+/=") {
		t.Errorf("challenge %q contains standard-base64-only characters, want URL-safe unpadded", challenge1)
	}
	if different := DeriveS256Challenge("a-different-verifier"); different == challenge1 {
		t.Error("two different verifiers produced the same challenge")
	}
}

func TestGenerateStateIsRandom(t *testing.T) {
	a, err := GenerateState()
	if err != nil {
		t.Fatalf("GenerateState() error = %v", err)
	}
	b, err := GenerateState()
	if err != nil {
		t.Fatalf("GenerateState() error = %v", err)
	}
	if a == b {
		t.Error("two generated state values were identical")
	}
	if len(a) < 20 {
		t.Errorf("state value %q looks too short to be meaningfully random", a)
	}
}

// --- authorization URL ---------------------------------------------------

func TestBuildAuthorizationURLNeverIncludesAClientSecret(t *testing.T) {
	auth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	client := newTestClient(t, auth, nil, nil)

	raw := client.BuildAuthorizationURL(AuthorizationURLInput{
		ClientID: "cid", RedirectURI: "http://127.0.0.1:12345/callback",
		Scopes: []string{RequiredScope}, State: "state-value", CodeChallenge: "challenge-value",
	})

	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("BuildAuthorizationURL() produced an unparseable URL: %v", err)
	}
	q := parsed.Query()
	if q.Has("client_secret") {
		t.Error("authorization URL contains a client_secret parameter")
	}
	if q.Get("code_challenge_method") != "S256" {
		t.Errorf("code_challenge_method = %q, want S256", q.Get("code_challenge_method"))
	}
	if q.Get("access_type") != "offline" {
		t.Errorf("access_type = %q, want offline", q.Get("access_type"))
	}
	if q.Get("prompt") != "consent" {
		t.Errorf("prompt = %q, want consent - see docs/provider-integrations/youtube.md's access_type/prompt section", q.Get("prompt"))
	}
	if q.Get("state") != "state-value" || q.Get("code_challenge") != "challenge-value" {
		t.Error("state or code_challenge did not round-trip into the URL")
	}
	if !strings.HasPrefix(raw, auth.URL) {
		t.Errorf("authorization URL %q does not use the configured auth base URL", raw)
	}
}

// --- token exchange and refresh -----------------------------------------

func TestExchangeCodeSendsNoClientSecretAndParsesScopes(t *testing.T) {
	var capturedForm url.Values
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		capturedForm = r.PostForm
		jsonHandler(http.StatusOK, `{"access_token":"at1","refresh_token":"rt1","expires_in":3600,"scope":"`+RequiredScope+`","token_type":"Bearer"}`)(w, r)
	}))
	client := newTestClient(t, nil, oauth, nil)

	bundle, scopes, err := client.ExchangeCode(context.Background(), "cid", "auth-code", "verifier", "http://127.0.0.1:1/callback")
	if err != nil {
		t.Fatalf("ExchangeCode() error = %v", err)
	}
	if capturedForm.Has("client_secret") {
		t.Error("token exchange request included a client_secret field")
	}
	if capturedForm.Get("grant_type") != "authorization_code" {
		t.Errorf("grant_type = %q, want authorization_code", capturedForm.Get("grant_type"))
	}
	if bundle.AccessToken != "at1" || bundle.RefreshToken != "rt1" {
		t.Errorf("bundle = %+v, want access/refresh tokens at1/rt1", bundle)
	}
	if bundle.TokenType != "bearer" {
		t.Errorf("TokenType = %q, want lowercased bearer", bundle.TokenType)
	}
	if len(scopes) != 1 || scopes[0] != RequiredScope {
		t.Errorf("scopes = %v, want [%s]", scopes, RequiredScope)
	}
}

func TestRefreshTokenPreservesOldRefreshTokenWhenGoogleOmitsOne(t *testing.T) {
	oauth := httptest.NewServer(jsonHandler(http.StatusOK, `{"access_token":"new-at","expires_in":3600,"token_type":"Bearer"}`))
	client := newTestClient(t, nil, oauth, nil)

	bundle, err := client.RefreshToken(context.Background(), "cid", "original-refresh-token")
	if err != nil {
		t.Fatalf("RefreshToken() error = %v", err)
	}
	if bundle.RefreshToken != "original-refresh-token" {
		t.Errorf("RefreshToken = %q, want the preserved original-refresh-token (Google's response omitted one)", bundle.RefreshToken)
	}
	if bundle.AccessToken != "new-at" {
		t.Errorf("AccessToken = %q, want new-at", bundle.AccessToken)
	}
}

func TestRefreshTokenReplacesRefreshTokenWhenGoogleReturnsANewOne(t *testing.T) {
	oauth := httptest.NewServer(jsonHandler(http.StatusOK, `{"access_token":"new-at","refresh_token":"rotated-rt","expires_in":3600,"token_type":"Bearer"}`))
	client := newTestClient(t, nil, oauth, nil)

	bundle, err := client.RefreshToken(context.Background(), "cid", "original-refresh-token")
	if err != nil {
		t.Fatalf("RefreshToken() error = %v", err)
	}
	if bundle.RefreshToken != "rotated-rt" {
		t.Errorf("RefreshToken = %q, want rotated-rt (Google returned a replacement)", bundle.RefreshToken)
	}
}

func TestRefreshTokenInvalidGrantMapsToErrInvalidGrant(t *testing.T) {
	oauth := httptest.NewServer(jsonHandler(http.StatusBadRequest, `{"error":"invalid_grant","error_description":"Token has been expired or revoked."}`))
	client := newTestClient(t, nil, oauth, nil)

	_, err := client.RefreshToken(context.Background(), "cid", "dead-refresh-token")
	if err == nil {
		t.Fatal("RefreshToken() error = nil, want ErrInvalidGrant")
	}
	if !strings.Contains(err.Error(), "no longer valid") {
		t.Errorf("error = %v, want it to wrap ErrInvalidGrant", err)
	}
}

func TestRevokeTokenTreatsAlreadyInvalidAsSuccess(t *testing.T) {
	oauth := httptest.NewServer(jsonHandler(http.StatusBadRequest, `{"error":"invalid_token"}`))
	client := newTestClient(t, nil, oauth, nil)

	if err := client.RevokeToken(context.Background(), "already-dead-token"); err != nil {
		t.Errorf("RevokeToken() error = %v, want nil (already-invalid treated as success)", err)
	}
}

func TestValidateTokenParsesTokenInfoResponse(t *testing.T) {
	oauth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tokeninfo" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		jsonHandler(http.StatusOK, `{"aud":"cid","scope":"`+RequiredScope+`","expires_in":"3599"}`)(w, r)
	}))
	client := newTestClient(t, nil, oauth, nil)

	result, err := client.ValidateToken(context.Background(), "some-token")
	if err != nil {
		t.Fatalf("ValidateToken() error = %v", err)
	}
	if !result.Valid || result.ClientID != "cid" {
		t.Errorf("result = %+v, want Valid=true ClientID=cid", result)
	}
	if len(result.Scopes) != 1 || result.Scopes[0] != RequiredScope {
		t.Errorf("Scopes = %v, want [%s]", result.Scopes, RequiredScope)
	}
}

func TestValidateTokenTreatsNon200AsInvalid(t *testing.T) {
	oauth := httptest.NewServer(jsonHandler(http.StatusBadRequest, `{"error_description":"Invalid Value"}`))
	client := newTestClient(t, nil, oauth, nil)

	result, err := client.ValidateToken(context.Background(), "expired-token")
	if err != nil {
		t.Fatalf("ValidateToken() error = %v", err)
	}
	if result.Valid {
		t.Error("result.Valid = true, want false for a non-200 tokeninfo response")
	}
}

// --- channels ------------------------------------------------------------

func TestListMyChannelsHandlesZeroOneAndManyChannels(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{"zero", `{"items":[]}`, 0},
		{"one", `{"items":[{"id":"UC1","snippet":{"title":"Channel One"}}]}`, 1},
		{"many", `{"items":[{"id":"UC1","snippet":{"title":"One"}},{"id":"UC2","snippet":{"title":"Two"}}]}`, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := httptest.NewServer(jsonHandler(http.StatusOK, tc.body))
			client := newTestClient(t, nil, nil, api)
			channels, err := client.ListMyChannels(context.Background(), "token")
			if err != nil {
				t.Fatalf("ListMyChannels() error = %v", err)
			}
			if len(channels) != tc.want {
				t.Errorf("len(channels) = %d, want %d", len(channels), tc.want)
			}
		})
	}
}

func TestListMyChannelsToleratesAMalformedEntry(t *testing.T) {
	api := httptest.NewServer(jsonHandler(http.StatusOK, `{"items":[{"id":"","snippet":{"title":"No ID"}},{"id":"UC1","snippet":{"title":"Valid"}}]}`))
	client := newTestClient(t, nil, nil, api)

	channels, err := client.ListMyChannels(context.Background(), "token")
	if err != nil {
		t.Fatalf("ListMyChannels() error = %v", err)
	}
	if len(channels) != 1 || channels[0].ID != "UC1" {
		t.Errorf("channels = %+v, want exactly the one valid entry", channels)
	}
}

// --- broadcasts ------------------------------------------------------------

func TestListBroadcastsMergesActiveAndUpcomingAndDeduplicates(t *testing.T) {
	calls := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		status := r.URL.Query().Get("broadcastStatus")
		switch status {
		case "active":
			jsonHandler(http.StatusOK, `{"items":[{"id":"b-shared","snippet":{"title":"Live now"},"status":{"lifeCycleStatus":"live"}}]}`)(w, r)
		case "upcoming":
			jsonHandler(http.StatusOK, `{"items":[{"id":"b-shared","snippet":{"title":"Live now"},"status":{"lifeCycleStatus":"live"}},{"id":"b-upcoming","snippet":{"title":"Later"},"status":{"lifeCycleStatus":"ready"}}]}`)(w, r)
		default:
			t.Errorf("unexpected broadcastStatus %q", status)
		}
	}))
	client := newTestClient(t, nil, nil, api)

	broadcasts, err := client.ListBroadcasts(context.Background(), "token")
	if err != nil {
		t.Fatalf("ListBroadcasts() error = %v", err)
	}
	if calls != 2 {
		t.Errorf("made %d requests, want exactly 2 (active and upcoming)", calls)
	}
	if len(broadcasts) != 2 {
		t.Errorf("len(broadcasts) = %d, want 2 after de-duplicating the shared entry", len(broadcasts))
	}
	for _, b := range broadcasts {
		if b.ID != "b-shared" && b.ID != "b-upcoming" {
			t.Errorf("unexpected broadcast id %q", b.ID)
		}
	}
}

func TestListBroadcastsNeverRequestsPersistentBroadcastType(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("broadcastType") == "persistent" {
			t.Error("request used the deprecated broadcastType=persistent - see docs/provider-integrations/youtube.md")
		}
		jsonHandler(http.StatusOK, `{"items":[]}`)(w, r)
	}))
	client := newTestClient(t, nil, nil, api)
	if _, err := client.ListBroadcasts(context.Background(), "token"); err != nil {
		t.Fatalf("ListBroadcasts() error = %v", err)
	}
}

// --- videos: safe read-modify-write ---------------------------------------

func TestUpdateVideoPreservesUnmanagedFieldsFromTheJustReadResource(t *testing.T) {
	var captured videoUpdateRequest
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			jsonHandler(http.StatusOK, `{"items":[{"id":"vid1","snippet":{"title":"Old title","description":"Old desc","categoryId":"20","tags":["old"]},"status":{"privacyStatus":"private","selfDeclaredMadeForKids":false}}]}`)(w, r)
			return
		}
		body, _ := readAll(r)
		_ = json.Unmarshal(body, &captured)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	client := newTestClient(t, nil, nil, api)

	current, err := client.GetVideo(context.Background(), "vid1", "token")
	if err != nil {
		t.Fatalf("GetVideo() error = %v", err)
	}

	err = client.UpdateVideo(context.Background(), current, VideoUpdateInput{
		Title: "New title", Description: "New desc", CategoryID: "20", PrivacyStatus: "public",
	}, "token")
	if err != nil {
		t.Fatalf("UpdateVideo() error = %v", err)
	}

	if captured.Status.SelfDeclaredMadeForKids == nil || *captured.Status.SelfDeclaredMadeForKids != false {
		t.Errorf("selfDeclaredMadeForKids was not preserved from the read resource: %+v", captured.Status)
	}
	if captured.Snippet.Title != "New title" {
		t.Errorf("Title = %q, want the new value to be applied", captured.Snippet.Title)
	}
	if captured.Status.PrivacyStatus != "public" {
		t.Errorf("PrivacyStatus = %q, want public", captured.Status.PrivacyStatus)
	}
}

func readAll(r *http.Request) ([]byte, error) {
	defer func() { _ = r.Body.Close() }()
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)
	for {
		n, err := r.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	return buf, nil
}

// --- categories ------------------------------------------------------------

func TestListCategoriesFiltersToAssignableOnly(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("regionCode") != "US" {
			t.Errorf("regionCode = %q, want US", r.URL.Query().Get("regionCode"))
		}
		jsonHandler(http.StatusOK, `{"items":[
			{"id":"1","snippet":{"title":"Film","assignable":true}},
			{"id":"2","snippet":{"title":"Not assignable","assignable":false}}
		]}`)(w, r)
	}))
	client := newTestClient(t, nil, nil, api)

	categories, err := client.ListCategories(context.Background(), "US", "token")
	if err != nil {
		t.Fatalf("ListCategories() error = %v", err)
	}
	if len(categories) != 1 || categories[0].ID != "1" {
		t.Errorf("categories = %+v, want only the assignable entry", categories)
	}
}

// --- errors ------------------------------------------------------------

func TestClassifyAPIErrorRecognizesLiveStreamingNotEnabled(t *testing.T) {
	api := httptest.NewServer(jsonHandler(http.StatusForbidden, `{"error":{"code":403,"message":"...","errors":[{"reason":"liveStreamingNotEnabled"}]}}`))
	client := newTestClient(t, nil, nil, api)

	_, err := client.ListBroadcasts(context.Background(), "token")
	if err == nil || !strings.Contains(err.Error(), "not enabled") {
		t.Errorf("error = %v, want ErrLiveStreamingNotEnabled", err)
	}
}

func TestClassifyAPIErrorRecognizesQuotaExceeded(t *testing.T) {
	api := httptest.NewServer(jsonHandler(http.StatusForbidden, `{"error":{"code":403,"message":"...","errors":[{"reason":"quotaExceeded"}]}}`))
	client := newTestClient(t, nil, nil, api)

	_, err := client.ListMyChannels(context.Background(), "token")
	if err == nil || !strings.Contains(err.Error(), "quota") {
		t.Errorf("error = %v, want ErrQuotaExceeded", err)
	}
}
