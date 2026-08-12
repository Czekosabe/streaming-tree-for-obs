package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/streaming-tree/server/internal/domain/account"
	"github.com/streaming-tree/server/internal/outboundchat"
)

func testAccount(scopes ...string) account.Account {
	return account.Account{ID: "acct_1", ProviderID: account.ProviderYouTube, ProviderUserID: "UC_channel", Scopes: scopes}
}

func testToken() account.TokenBundle {
	return account.TokenBundle{AccessToken: "at-token"}
}

func TestOutboundAdapterAssessCapability(t *testing.T) {
	adapter := NewOutboundChatAdapter(New(Options{}), nil, nil)

	available := adapter.AssessCapability(testAccount(RequiredScope))
	if !available.Available || available.SupportsReply {
		t.Fatalf("expected available=true, supportsReply=false, got %+v", available)
	}

	unavailable := adapter.AssessCapability(testAccount())
	if unavailable.Available || !unavailable.PermissionUpgradeRequired {
		t.Fatalf("expected available=false, permissionUpgradeRequired=true, got %+v", unavailable)
	}
}

func TestOutboundAdapterRejectsReplyOutright(t *testing.T) {
	adapter := NewOutboundChatAdapter(New(Options{}), nil, nil)
	_, err := adapter.SendChatMessage(context.Background(), testAccount(), testToken(), "client-id",
		outboundchat.SendMessageRequest{Message: "hi", ReplyParentMessageID: "some_id"})
	if !errors.Is(err, outboundchat.ErrReplyUnsupported) {
		t.Fatalf("expected ErrReplyUnsupported, got %v", err)
	}
}

func TestOutboundAdapterReturnsChatUnavailableWithNoDestination(t *testing.T) {
	adapter := NewOutboundChatAdapter(New(Options{}), nil, nil) // no lookups configured
	_, err := adapter.SendChatMessage(context.Background(), testAccount(), testToken(), "client-id",
		outboundchat.SendMessageRequest{Message: "hi"})
	if !errors.Is(err, outboundchat.ErrChatUnavailable) {
		t.Fatalf("expected ErrChatUnavailable, got %v", err)
	}
}

func TestOutboundAdapterReturnsChatUnavailableWithNoBroadcastSelected(t *testing.T) {
	adapter := NewOutboundChatAdapter(New(Options{}),
		func(accountID string) (string, bool) { return "platform_1", true },
		func(platformID string) (string, bool) { return "", false }, // no broadcast selected
	)
	_, err := adapter.SendChatMessage(context.Background(), testAccount(), testToken(), "client-id",
		outboundchat.SendMessageRequest{Message: "hi"})
	if !errors.Is(err, outboundchat.ErrChatUnavailable) {
		t.Fatalf("expected ErrChatUnavailable, got %v", err)
	}
}

func TestOutboundAdapterReturnsChatUnavailableWhenBroadcastHasNoLiveChat(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{{"id": "bcast_1", "snippet": map[string]any{"liveChatId": ""}, "status": map[string]any{}}},
		})
	}))
	defer api.Close()
	client := New(Options{APIBaseURL: api.URL})
	adapter := NewOutboundChatAdapter(client,
		func(accountID string) (string, bool) { return "platform_1", true },
		func(platformID string) (string, bool) { return "bcast_1", true },
	)
	_, err := adapter.SendChatMessage(context.Background(), testAccount(), testToken(), "client-id",
		outboundchat.SendMessageRequest{Message: "hi"})
	if !errors.Is(err, outboundchat.ErrChatUnavailable) {
		t.Fatalf("expected ErrChatUnavailable, got %v", err)
	}
}

func TestOutboundAdapterSendsSuccessfully(t *testing.T) {
	var insertedBody map[string]any
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{{"id": "bcast_1", "snippet": map[string]any{"liveChatId": "chat_1"}, "status": map[string]any{}}},
			})
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&insertedBody)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "sent_msg_1", "snippet": map[string]any{"type": "textMessageEvent"}})
	}))
	defer api.Close()
	client := New(Options{APIBaseURL: api.URL})
	adapter := NewOutboundChatAdapter(client,
		func(accountID string) (string, bool) { return "platform_1", true },
		func(platformID string) (string, bool) { return "bcast_1", true },
	)

	result, err := adapter.SendChatMessage(context.Background(), testAccount(), testToken(), "client-id",
		outboundchat.SendMessageRequest{Message: "hello chat"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Sent || result.ProviderMessageID != "sent_msg_1" {
		t.Fatalf("expected a sent result with provider id sent_msg_1, got %+v", result)
	}
	snippet, _ := insertedBody["snippet"].(map[string]any)
	if snippet["liveChatId"] != "chat_1" {
		t.Fatalf("expected the resolved liveChatId to be used, got %+v", insertedBody)
	}
}

func TestOutboundAdapterMapsErrors(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"unauthorized", http.StatusUnauthorized, `{}`, outboundchat.ErrUnauthorized},
		{"liveChatEnded", http.StatusForbidden, `{"error":{"errors":[{"reason":"liveChatEnded"}]}}`, outboundchat.ErrChatUnavailable},
		{"liveChatDisabled", http.StatusForbidden, `{"error":{"errors":[{"reason":"liveChatDisabled"}]}}`, outboundchat.ErrForbidden},
		{"rateLimited", http.StatusTooManyRequests, `{}`, &outboundchat.RateLimitedError{}},
		{"serverError", http.StatusInternalServerError, `{}`, outboundchat.ErrProviderFailure},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					_ = json.NewEncoder(w).Encode(map[string]any{
						"items": []map[string]any{{"id": "bcast_1", "snippet": map[string]any{"liveChatId": "chat_1"}, "status": map[string]any{}}},
					})
					return
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer api.Close()
			client := New(Options{APIBaseURL: api.URL})
			adapter := NewOutboundChatAdapter(client,
				func(accountID string) (string, bool) { return "platform_1", true },
				func(platformID string) (string, bool) { return "bcast_1", true },
			)
			_, err := adapter.SendChatMessage(context.Background(), testAccount(), testToken(), "client-id",
				outboundchat.SendMessageRequest{Message: "hi"})
			if err == nil {
				t.Fatal("expected an error")
			}
			var rateLimited *outboundchat.RateLimitedError
			if errors.As(tc.want, &rateLimited) {
				if !errors.As(err, &rateLimited) {
					t.Fatalf("expected a RateLimitedError, got %v", err)
				}
				return
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, err)
			}
		})
	}
}
