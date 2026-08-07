package twitch

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/account"
	"github.com/streaming-tree/server/internal/outboundchat"
)

func testAccount(scopes ...string) account.Account {
	return account.Account{ID: "acc_1", ProviderID: account.ProviderTwitch, ProviderUserID: "u_123", Scopes: scopes}
}

func TestAdapterAssessCapabilityMirrorsAssessOutboundChatCapability(t *testing.T) {
	adapter := NewAdapter(New(Options{}))

	cap := adapter.AssessCapability(testAccount("user:write:chat"))
	if !cap.Available {
		t.Error("expected Available = true")
	}

	cap = adapter.AssessCapability(testAccount("channel:manage:broadcast"))
	if cap.Available {
		t.Error("expected Available = false")
	}
	if len(cap.Missing) != 1 || cap.Missing[0] != "user:write:chat" {
		t.Errorf("Missing = %v, want [user:write:chat]", cap.Missing)
	}
}

func TestAdapterSendChatMessageUsesTheAccountsOwnProviderUserIDForBothIDs(t *testing.T) {
	var gotBroadcaster, gotSender string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			BroadcasterID string `json:"broadcaster_id"`
			SenderID      string `json:"sender_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotBroadcaster, gotSender = body.BroadcasterID, body.SenderID
		jsonHandler(http.StatusOK, `{"data":[{"message_id":"m1","is_sent":true}]}`)(w, r)
	}))
	adapter := NewAdapter(newTestClient(t, nil, api))

	acc := testAccount("user:write:chat")
	result, err := adapter.SendChatMessage(context.Background(), acc, account.TokenBundle{AccessToken: "tok"}, "client-id", outboundchat.SendMessageRequest{
		AccountID: acc.ID, Message: "hi", Source: outboundchat.SourceManual,
	})
	if err != nil {
		t.Fatalf("SendChatMessage() error = %v", err)
	}
	if gotBroadcaster != "u_123" || gotSender != "u_123" {
		t.Errorf("broadcaster/sender = %q/%q, want both u_123 (the account's own provider user id)", gotBroadcaster, gotSender)
	}
	if !result.Sent || result.ProviderMessageID != "m1" {
		t.Errorf("result = %+v", result)
	}
}

func TestAdapterSendChatMessageMapsDroppedResultWithoutError(t *testing.T) {
	api := httptest.NewServer(jsonHandler(http.StatusOK, `{"data":[{"is_sent":false,"drop_reason":{"code":"automod_held","message":"held for review"}}]}`))
	adapter := NewAdapter(newTestClient(t, nil, api))

	result, err := adapter.SendChatMessage(context.Background(), testAccount("user:write:chat"), account.TokenBundle{AccessToken: "tok"}, "client-id", outboundchat.SendMessageRequest{Message: "hi"})
	if err != nil {
		t.Fatalf("SendChatMessage() error = %v, want a normal dropped result", err)
	}
	if result.Sent {
		t.Error("Sent = true, want false")
	}
	if result.Code != "dropped" {
		t.Errorf("Code = %q, want dropped", result.Code)
	}
}

func TestAdapterSendChatMessageMapsErrorsToOutboundChatSentinels(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		wantErr error
	}{
		{"unauthorized", http.StatusUnauthorized, ErrUnauthorized},
		{"forbidden", http.StatusForbidden, outboundchat.ErrForbidden},
		{"serverError", http.StatusInternalServerError, outboundchat.ErrProviderFailure},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := httptest.NewServer(jsonHandler(tc.status, `{"error":"x"}`))
			adapter := NewAdapter(newTestClient(t, nil, api))

			_, err := adapter.SendChatMessage(context.Background(), testAccount("user:write:chat"), account.TokenBundle{AccessToken: "tok"}, "client-id", outboundchat.SendMessageRequest{Message: "hi"})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestAdapterSendChatMessageMapsRateLimitedWithRetryAt(t *testing.T) {
	resetAt := time.Now().Add(45 * time.Second).Unix()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Ratelimit-Reset", formatUnix(resetAt))
		w.Header().Set("Ratelimit-Limit", "20")
		w.Header().Set("Ratelimit-Remaining", "0")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{}`))
	}))
	adapter := NewAdapter(newTestClient(t, nil, api))

	_, err := adapter.SendChatMessage(context.Background(), testAccount("user:write:chat"), account.TokenBundle{AccessToken: "tok"}, "client-id", outboundchat.SendMessageRequest{Message: "hi"})
	var rateLimitErr *outboundchat.RateLimitedError
	if !errors.As(err, &rateLimitErr) {
		t.Fatalf("error = %v, want *outboundchat.RateLimitedError", err)
	}
	if rateLimitErr.RetryAt.Unix() != resetAt {
		t.Errorf("RetryAt = %v, want %v", rateLimitErr.RetryAt.Unix(), resetAt)
	}
}

func TestAdapterSendChatMessageMapsTransportFailureToDeliveryUnknown(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
	}))
	adapter := NewAdapter(newTestClient(t, nil, api))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	_, err := adapter.SendChatMessage(ctx, testAccount("user:write:chat"), account.TokenBundle{AccessToken: "tok"}, "client-id", outboundchat.SendMessageRequest{Message: "hi"})
	if !errors.Is(err, outboundchat.ErrDeliveryUnknown) {
		t.Fatalf("error = %v, want outboundchat.ErrDeliveryUnknown", err)
	}
}

func TestAdapterSendChatMessageMapsMalformedResponseToDeliveryUnknown(t *testing.T) {
	api := httptest.NewServer(jsonHandler(http.StatusOK, `not json`))
	adapter := NewAdapter(newTestClient(t, nil, api))

	_, err := adapter.SendChatMessage(context.Background(), testAccount("user:write:chat"), account.TokenBundle{AccessToken: "tok"}, "client-id", outboundchat.SendMessageRequest{Message: "hi"})
	if !errors.Is(err, outboundchat.ErrDeliveryUnknown) {
		t.Fatalf("error = %v, want outboundchat.ErrDeliveryUnknown for a malformed success response", err)
	}
}

func formatUnix(sec int64) string {
	return strconv.FormatInt(sec, 10)
}
