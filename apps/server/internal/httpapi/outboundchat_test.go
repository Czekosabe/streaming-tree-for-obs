package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/account"
	"github.com/streaming-tree/server/internal/domain/platform"
	"github.com/streaming-tree/server/internal/outboundchat"
	"github.com/streaming-tree/server/internal/provider/twitch"
	"github.com/streaming-tree/server/internal/runtime/deviceflow"
	"github.com/streaming-tree/server/internal/secrets/secretstest"
	"github.com/streaming-tree/server/internal/storage/sqlite"
)

// fakeOutboundProvider is a fully controllable outboundchat.Provider test
// double - the real Twitch adapter is already exhaustively tested in
// internal/provider/twitch; these tests only need to verify the HTTP
// layer's own request/response mapping and error handling.
type fakeOutboundProvider struct {
	mu      sync.Mutex
	nextErr error
	nextRes outboundchat.SendMessageResult
	lastReq outboundchat.SendMessageRequest
	block   chan struct{} // when non-nil, SendChatMessage waits on it
}

func (p *fakeOutboundProvider) ProviderID() account.ProviderID { return account.ProviderTwitch }

func (p *fakeOutboundProvider) AssessCapability(acc account.Account) outboundchat.Capability {
	granted := acc.HasScope("user:write:chat")
	c := outboundchat.Capability{Required: []string{"user:write:chat"}, Granted: acc.Scopes, Available: granted}
	if !granted {
		c.Missing = []string{"user:write:chat"}
		c.PermissionUpgradeRequired = true
	}
	return c
}

func (p *fakeOutboundProvider) SendChatMessage(ctx context.Context, acc account.Account, token account.TokenBundle, clientID string, req outboundchat.SendMessageRequest) (outboundchat.SendMessageResult, error) {
	p.mu.Lock()
	p.lastReq = req
	block := p.block
	p.mu.Unlock()
	if block != nil {
		<-block
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.nextErr != nil {
		return outboundchat.SendMessageResult{}, p.nextErr
	}
	if p.nextRes.CompletedAt.IsZero() && p.nextRes.Sent {
		p.nextRes.CompletedAt = time.Now().UTC()
	}
	return p.nextRes, nil
}

func (p *fakeOutboundProvider) setNext(res outboundchat.SendMessageResult, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.nextErr, p.nextRes = err, res
}

type outboundChatTestServer struct {
	handler  http.Handler
	accounts *account.Service
	provider *fakeOutboundProvider
}

func newOutboundChatTestServer(t *testing.T) *outboundChatTestServer {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sqlite.Open(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("sqlite.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := sqlite.Migrate(context.Background(), db.DB); err != nil {
		t.Fatalf("sqlite.Migrate() error = %v", err)
	}

	platforms := platform.NewService(sqlite.NewPlatformRepository(db.DB))
	secretStore := secretstest.New()
	provider := fakeHTTPProvider{}
	accounts := account.NewService(account.Options{
		Repository: sqlite.NewAccountRepository(db.DB), Secrets: secretStore,
		Providers:      map[account.ProviderID]account.Provider{account.ProviderTwitch: provider},
		RequiredScopes: map[account.ProviderID][]string{account.ProviderTwitch: {"channel:manage:broadcast"}},
		Logger:         logger,
	})
	if _, err := accounts.SetIntegrationClientID(context.Background(), account.ProviderTwitch, "test-client-id"); err != nil {
		t.Fatalf("SetIntegrationClientID() error = %v", err)
	}

	deviceFlow := deviceflow.NewManager(deviceflow.Options{
		Accounts:       accounts,
		Providers:      map[account.ProviderID]account.DeviceFlowProvider{account.ProviderTwitch: provider},
		RequiredScopes: map[account.ProviderID][]string{account.ProviderTwitch: {"channel:manage:broadcast"}},
		Logger:         logger,
	})
	deviceFlow.Start(context.Background())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		deviceFlow.Shutdown(ctx)
	})

	outboundProvider := &fakeOutboundProvider{}
	outboundManager := outboundchat.NewManager(outboundchat.ManagerOptions{
		Accounts: accounts, Providers: []outboundchat.Provider{outboundProvider},
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = outboundManager.Shutdown(ctx)
	})

	handler := NewRouter(Options{
		Logger: logger, StartedAt: time.Now(), Platforms: platforms,
		Accounts: accounts, DeviceFlow: deviceFlow, TwitchMetadata: twitch.NewMetadataService(accounts, twitch.New(twitch.Options{})),
		OutboundChat: outboundManager,
	})

	return &outboundChatTestServer{handler: handler, accounts: accounts, provider: outboundProvider}
}

// createAccount always grants the base metadata scope this test server's
// RequiredScopes demands (FinalizeConnection rejects a scope set missing
// it); pass any additional outbound-chat scope on top.
func (ts *outboundChatTestServer) createAccount(t *testing.T, id string, extraScopes ...string) account.Account {
	t.Helper()
	scopes := append([]string{"channel:manage:broadcast"}, extraScopes...)
	acc, err := ts.accounts.FinalizeConnection(context.Background(), account.ProviderTwitch,
		account.Identity{ProviderUserID: id + "_provider_user", Login: "viewer_" + id, DisplayName: "Viewer " + id},
		account.TokenBundle{TokenType: "bearer", AccessToken: "fake-access", RefreshToken: "fake-refresh", ExpiresAt: time.Now().Add(time.Hour)},
		scopes, "")
	if err != nil {
		t.Fatalf("createAccount() error = %v", err)
	}
	return acc
}

func (ts *outboundChatTestServer) do(t *testing.T, method, path string, body any) *http.Response {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}
		reader = bytes.NewReader(encoded)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	ts.handler.ServeHTTP(rec, req)
	return rec.Result()
}

func decodeOutboundChatBody(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	return out
}

// --- GET status ------------------------------------------------------------

func TestGetOutboundChatStatusReady(t *testing.T) {
	ts := newOutboundChatTestServer(t)
	acc := ts.createAccount(t, "a1", "user:write:chat")

	resp := ts.do(t, http.MethodGet, "/api/connected-accounts/"+acc.ID+"/outbound-chat", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body := decodeOutboundChatBody(t, resp)
	if body["capability"] != "ready" {
		t.Errorf("capability = %v, want ready", body["capability"])
	}
	if body["canSendNow"] != true {
		t.Errorf("canSendNow = %v, want true", body["canSendNow"])
	}
	if body["sharedChatWarning"] == "" || body["sharedChatWarning"] == nil {
		t.Error("sharedChatWarning missing")
	}
	if body["queueCapacity"] != float64(outboundchat.MaxQueueDepth) {
		t.Errorf("queueCapacity = %v, want %d", body["queueCapacity"], outboundchat.MaxQueueDepth)
	}
}

func TestGetOutboundChatStatusPermissionRequired(t *testing.T) {
	ts := newOutboundChatTestServer(t)
	acc := ts.createAccount(t, "a1")

	resp := ts.do(t, http.MethodGet, "/api/connected-accounts/"+acc.ID+"/outbound-chat", nil)
	body := decodeOutboundChatBody(t, resp)
	if body["capability"] != "permission_required" {
		t.Errorf("capability = %v, want permission_required", body["capability"])
	}
	if body["canSendNow"] != false {
		t.Errorf("canSendNow = %v, want false", body["canSendNow"])
	}
}

func TestGetOutboundChatStatusAccountNotFound(t *testing.T) {
	ts := newOutboundChatTestServer(t)
	resp := ts.do(t, http.MethodGet, "/api/connected-accounts/does-not-exist/outbound-chat", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	body := decodeOutboundChatBody(t, resp)
	if body["error"] != "account_not_found" {
		t.Errorf("error = %v", body["error"])
	}
}

func TestOutboundChatStatusMethodNotAllowed(t *testing.T) {
	ts := newOutboundChatTestServer(t)
	acc := ts.createAccount(t, "a1", "user:write:chat")

	resp := ts.do(t, http.MethodPost, "/api/connected-accounts/"+acc.ID+"/outbound-chat", nil)
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
	if resp.Header.Get("Allow") == "" {
		t.Error("Allow header missing on 405")
	}
}

// --- authorize ---------------------------------------------------------

func TestAuthorizeOutboundChatReturnsDeviceFlowShape(t *testing.T) {
	ts := newOutboundChatTestServer(t)
	acc := ts.createAccount(t, "a1")

	resp := ts.do(t, http.MethodPost, "/api/connected-accounts/"+acc.ID+"/outbound-chat/authorize", nil)
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", resp.StatusCode)
	}
	body := decodeOutboundChatBody(t, resp)
	if body["userCode"] == nil || body["verificationUri"] == nil {
		t.Errorf("expected the standard device-flow shape, got %v", body)
	}
	raw, _ := json.Marshal(body)
	if strings.Contains(string(raw), "dc-should-never-leave-the-backend") {
		t.Fatal("the device code leaked into the HTTP response")
	}
}

func TestAuthorizeOutboundChatRejectsANonEmptyBody(t *testing.T) {
	ts := newOutboundChatTestServer(t)
	acc := ts.createAccount(t, "a1")

	resp := ts.do(t, http.MethodPost, "/api/connected-accounts/"+acc.ID+"/outbound-chat/authorize", map[string]string{"x": "y"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for an unexpected body", resp.StatusCode)
	}
}

// --- send messages -----------------------------------------------------

func TestSendOutboundChatMessageSuccess(t *testing.T) {
	ts := newOutboundChatTestServer(t)
	acc := ts.createAccount(t, "a1", "user:write:chat")
	ts.provider.setNext(outboundchat.SendMessageResult{Sent: true, ProviderMessageID: "msg_1"}, nil)

	resp := ts.do(t, http.MethodPost, "/api/connected-accounts/"+acc.ID+"/outbound-chat/messages",
		map[string]string{"message": "hello chat"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body := decodeOutboundChatBody(t, resp)
	if body["sent"] != true || body["providerMessageId"] != "msg_1" || body["sentAt"] == nil {
		t.Errorf("body = %v", body)
	}
	if _, ok := body["message"]; ok {
		t.Fatal("response echoed the sent message text")
	}

	ts.provider.mu.Lock()
	gotMessage := ts.provider.lastReq.Message
	ts.provider.mu.Unlock()
	if gotMessage != "hello chat" {
		t.Errorf("provider received message = %q, want %q", gotMessage, "hello chat")
	}
}

func TestSendOutboundChatMessageForwardsReplyParentID(t *testing.T) {
	ts := newOutboundChatTestServer(t)
	acc := ts.createAccount(t, "a1", "user:write:chat")
	ts.provider.setNext(outboundchat.SendMessageResult{Sent: true, ProviderMessageID: "m2"}, nil)

	resp := ts.do(t, http.MethodPost, "/api/connected-accounts/"+acc.ID+"/outbound-chat/messages",
		map[string]string{"message": "a reply", "replyParentMessageId": "parent_1"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	ts.provider.mu.Lock()
	got := ts.provider.lastReq.ReplyParentMessageID
	ts.provider.mu.Unlock()
	if got != "parent_1" {
		t.Errorf("ReplyParentMessageID = %q, want parent_1", got)
	}
}

func TestSendOutboundChatMessagePermissionRequired(t *testing.T) {
	ts := newOutboundChatTestServer(t)
	acc := ts.createAccount(t, "a1")

	resp := ts.do(t, http.MethodPost, "/api/connected-accounts/"+acc.ID+"/outbound-chat/messages", map[string]string{"message": "hi"})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", resp.StatusCode)
	}
	body := decodeOutboundChatBody(t, resp)
	if body["error"] != "outbound_chat_permission_required" {
		t.Errorf("error = %v", body["error"])
	}
}

// Unsupported-provider behavior is not re-exercised at the HTTP layer here:
// this test server only ever creates Twitch accounts (the only provider
// wired to a device flow in this harness), so there is no reachable
// connected account whose provider genuinely lacks an outbound-chat
// Provider through this server's own public surface. The behavior itself
// (outboundchat.ErrUnsupportedProvider -> 503 outbound_chat_unsupported)
// is fully covered directly at the outboundchat.Manager level - see
// TestSendReturnsUnsupportedProviderForAnUnregisteredProvider in
// internal/outboundchat/dispatcher_test.go - and the identical writeError
// call for a non-Twitch account is already reached by the authorize
// handler's own explicit provider check whenever accounts.ProviderID !=
// account.ProviderTwitch.

func TestSendOutboundChatMessageAccountNotFound(t *testing.T) {
	ts := newOutboundChatTestServer(t)
	resp := ts.do(t, http.MethodPost, "/api/connected-accounts/does-not-exist/outbound-chat/messages", map[string]string{"message": "hi"})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestSendOutboundChatMessageValidationFailure(t *testing.T) {
	ts := newOutboundChatTestServer(t)
	acc := ts.createAccount(t, "a1", "user:write:chat")

	resp := ts.do(t, http.MethodPost, "/api/connected-accounts/"+acc.ID+"/outbound-chat/messages", map[string]string{"message": ""})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 for an empty message", resp.StatusCode)
	}
	body := decodeOutboundChatBody(t, resp)
	if body["error"] != "validation_failed" {
		t.Errorf("error = %v", body["error"])
	}
}

func TestSendOutboundChatMessageUnknownField(t *testing.T) {
	ts := newOutboundChatTestServer(t)
	acc := ts.createAccount(t, "a1", "user:write:chat")

	resp := ts.do(t, http.MethodPost, "/api/connected-accounts/"+acc.ID+"/outbound-chat/messages",
		map[string]string{"message": "hi", "broadcasterId": "some-arbitrary-id"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for an unknown field", resp.StatusCode)
	}
	body := decodeOutboundChatBody(t, resp)
	if body["error"] != "unknown_field" {
		t.Errorf("error = %v, want unknown_field (the browser must never choose a broadcaster id)", body["error"])
	}
}

func TestSendOutboundChatMessageWrongContentType(t *testing.T) {
	ts := newOutboundChatTestServer(t)
	acc := ts.createAccount(t, "a1", "user:write:chat")

	req := httptest.NewRequest(http.MethodPost, "/api/connected-accounts/"+acc.ID+"/outbound-chat/messages", strings.NewReader(`{"message":"hi"}`))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	ts.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415", rec.Code)
	}
}

func TestSendOutboundChatMessageBodyTooLarge(t *testing.T) {
	ts := newOutboundChatTestServer(t)
	acc := ts.createAccount(t, "a1", "user:write:chat")

	huge := strings.Repeat("a", 9*1024)
	req := httptest.NewRequest(http.MethodPost, "/api/connected-accounts/"+acc.ID+"/outbound-chat/messages",
		strings.NewReader(`{"message":"`+huge+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	ts.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413 for an oversized body", rec.Code)
	}
}

func TestSendOutboundChatMessageDropped(t *testing.T) {
	ts := newOutboundChatTestServer(t)
	acc := ts.createAccount(t, "a1", "user:write:chat")
	ts.provider.setNext(outboundchat.SendMessageResult{Sent: false, Code: "dropped"}, nil)

	resp := ts.do(t, http.MethodPost, "/api/connected-accounts/"+acc.ID+"/outbound-chat/messages", map[string]string{"message": "spam"})
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 for a dropped message", resp.StatusCode)
	}
	body := decodeOutboundChatBody(t, resp)
	if body["error"] != "outbound_chat_message_dropped" {
		t.Errorf("error = %v", body["error"])
	}
	raw, _ := json.Marshal(body)
	if strings.Contains(string(raw), "spam") {
		t.Fatal("the dropped-message response echoed the sent text")
	}
}

func TestSendOutboundChatMessageRateLimited(t *testing.T) {
	ts := newOutboundChatTestServer(t)
	acc := ts.createAccount(t, "a1", "user:write:chat")
	ts.provider.setNext(outboundchat.SendMessageResult{}, &outboundchat.RateLimitedError{RetryAt: time.Now().Add(30 * time.Second)})

	resp := ts.do(t, http.MethodPost, "/api/connected-accounts/"+acc.ID+"/outbound-chat/messages", map[string]string{"message": "fast"})
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", resp.StatusCode)
	}
	body := decodeOutboundChatBody(t, resp)
	if body["error"] != "outbound_chat_rate_limited" {
		t.Errorf("error = %v", body["error"])
	}
}

func TestSendOutboundChatMessageDeliveryUnknown(t *testing.T) {
	ts := newOutboundChatTestServer(t)
	acc := ts.createAccount(t, "a1", "user:write:chat")
	ts.provider.setNext(outboundchat.SendMessageResult{}, outboundchat.ErrDeliveryUnknown)

	resp := ts.do(t, http.MethodPost, "/api/connected-accounts/"+acc.ID+"/outbound-chat/messages", map[string]string{"message": "uncertain"})
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", resp.StatusCode)
	}
	body := decodeOutboundChatBody(t, resp)
	if body["error"] != "outbound_chat_delivery_unknown" {
		t.Errorf("error = %v", body["error"])
	}
}

func TestSendOutboundChatMessageProviderFailure(t *testing.T) {
	ts := newOutboundChatTestServer(t)
	acc := ts.createAccount(t, "a1", "user:write:chat")
	ts.provider.setNext(outboundchat.SendMessageResult{}, outboundchat.ErrProviderFailure)

	resp := ts.do(t, http.MethodPost, "/api/connected-accounts/"+acc.ID+"/outbound-chat/messages", map[string]string{"message": "boom"})
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", resp.StatusCode)
	}
	body := decodeOutboundChatBody(t, resp)
	if body["error"] != "outbound_chat_provider_failure" {
		t.Errorf("error = %v", body["error"])
	}
}

func TestSendOutboundChatMessageQueueFull(t *testing.T) {
	ts := newOutboundChatTestServer(t)
	acc := ts.createAccount(t, "a1", "user:write:chat")

	block := make(chan struct{})
	ts.provider.mu.Lock()
	ts.provider.block = block
	ts.provider.nextRes = outboundchat.SendMessageResult{Sent: true, ProviderMessageID: "blocked"}
	ts.provider.mu.Unlock()

	// Occupy the dispatcher's single worker, then fill its bounded queue
	// completely before asserting the next request is rejected.
	var wg sync.WaitGroup
	for i := 0; i < 1+outboundchat.MaxQueueDepth; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ts.do(t, http.MethodPost, "/api/connected-accounts/"+acc.ID+"/outbound-chat/messages", map[string]string{"message": "queued"})
		}()
	}
	time.Sleep(100 * time.Millisecond) // let them all reach "queued" or "sending"

	resp := ts.do(t, http.MethodPost, "/api/connected-accounts/"+acc.ID+"/outbound-chat/messages", map[string]string{"message": "overflow"})
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429 once the bounded queue is at capacity", resp.StatusCode)
	}
	body := decodeOutboundChatBody(t, resp)
	if body["error"] != "outbound_chat_queue_full" {
		t.Errorf("error = %v, want outbound_chat_queue_full", body["error"])
	}

	// This test server's Manager uses the real wall clock, so fully
	// draining the queue would take ~1 real second per remaining item due
	// to the local rate-limit floor (already proven deterministically with
	// a fake clock in internal/outboundchat's own dispatcher_test.go).
	// Unblocking here and letting the test finish is enough to prove the
	// queue-full rejection; the background goroutines above finish on
	// their own once this test's t.Cleanup cancels the dispatcher via
	// Manager.Shutdown.
	close(block)
}
