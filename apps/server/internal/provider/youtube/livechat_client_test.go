package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInsertLiveChatMessageSendsExactTextMessageEventBody(t *testing.T) {
	var captured map[string]any
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&captured)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "sent_1", "snippet": map[string]any{"type": "textMessageEvent"}})
	}))
	client := newTestClient(t, nil, nil, api)

	msg, err := client.InsertLiveChatMessage(context.Background(), "chat_1", "hello", "at-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.ID != "sent_1" {
		t.Fatalf("expected id sent_1, got %q", msg.ID)
	}
	snippet, _ := captured["snippet"].(map[string]any)
	if snippet["liveChatId"] != "chat_1" || snippet["type"] != "textMessageEvent" {
		t.Fatalf("expected liveChatId/type in request body, got %+v", captured)
	}
	details, _ := snippet["textMessageDetails"].(map[string]any)
	if details["messageText"] != "hello" {
		t.Fatalf("expected messageText=hello, got %+v", details)
	}
	// No reply field of any kind should ever be sent - the API has none.
	for k := range snippet {
		if k == "replyParentMessageId" || k == "replyToMessageId" {
			t.Fatalf("no reply field should ever be sent, found key %q", k)
		}
	}
}

func TestInsertLiveChatMessageMapsMessageTextInvalid(t *testing.T) {
	api := httptest.NewServer(jsonHandler(http.StatusBadRequest, `{"error":{"code":400,"message":"invalid","errors":[{"reason":"messageTextInvalid"}]}}`))
	client := newTestClient(t, nil, nil, api)
	_, err := client.InsertLiveChatMessage(context.Background(), "chat_1", "", "at")
	if !errors.Is(err, ErrMessageInvalid) {
		t.Fatalf("expected ErrMessageInvalid, got %v", err)
	}
}

func TestInsertLiveChatMessageMapsRateLimit(t *testing.T) {
	api := httptest.NewServer(jsonHandler(http.StatusTooManyRequests, `{"error":{"code":429,"message":"too many","errors":[{"reason":"rateLimitExceeded"}]}}`))
	client := newTestClient(t, nil, nil, api)
	_, err := client.InsertLiveChatMessage(context.Background(), "chat_1", "hi", "at")
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
}
