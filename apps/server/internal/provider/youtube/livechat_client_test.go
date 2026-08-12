package youtube

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListLiveChatMessagesParsesAPageAndDetectsOffline(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("liveChatId"); got != "chat_1" {
			t.Errorf("expected liveChatId=chat_1, got %q", got)
		}
		if got := r.URL.Query().Get("part"); got != LiveChatMessagesPart {
			t.Errorf("expected part=%q, got %q", LiveChatMessagesPart, got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"nextPageToken":         "token_2",
			"pollingIntervalMillis": 5000,
			"offlineAt":             "2026-08-12T06:10:00Z",
			"items": []map[string]any{
				{
					"id": "msg_1",
					"snippet": map[string]any{
						"type":               "textMessageEvent",
						"publishedAt":        "2026-08-12T06:00:00Z",
						"authorChannelId":    "UC_1",
						"textMessageDetails": map[string]any{"messageText": "hi"},
					},
					"authorDetails": map[string]any{"channelId": "UC_1", "displayName": "Viewer"},
				},
			},
		})
	}))
	client := newTestClient(t, nil, nil, api)

	page, err := client.ListLiveChatMessages(context.Background(), "chat_1", "", "at-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.NextPageToken != "token_2" {
		t.Fatalf("expected next page token token_2, got %q", page.NextPageToken)
	}
	if !page.Ended {
		t.Fatal("expected Ended=true when offlineAt is present")
	}
	if len(page.Messages) != 1 || page.Messages[0].ID != "msg_1" {
		t.Fatalf("expected one message with id msg_1, got %+v", page.Messages)
	}
}

func TestListLiveChatMessagesSendsPageTokenWhenProvided(t *testing.T) {
	var gotToken string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.URL.Query().Get("pageToken")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{}})
	}))
	client := newTestClient(t, nil, nil, api)

	if _, err := client.ListLiveChatMessages(context.Background(), "chat_1", "continuation_token", "at"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotToken != "continuation_token" {
		t.Fatalf("expected pageToken=continuation_token, got %q", gotToken)
	}
}

func TestListLiveChatMessagesRejectsEmptyLiveChatID(t *testing.T) {
	client := newTestClient(t, nil, nil, nil)
	if _, err := client.ListLiveChatMessages(context.Background(), "", "", "at"); err == nil {
		t.Fatal("expected error for empty liveChatId")
	}
}

func TestListLiveChatMessagesMapsLiveChatDisabled(t *testing.T) {
	api := httptest.NewServer(jsonHandler(http.StatusForbidden, `{"error":{"code":403,"message":"disabled","errors":[{"reason":"liveChatDisabled"}]}}`))
	client := newTestClient(t, nil, nil, api)
	_, err := client.ListLiveChatMessages(context.Background(), "chat_1", "", "at")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrLiveChatDisabled) {
		t.Fatalf("expected ErrLiveChatDisabled, got %v", err)
	}
}

func TestListLiveChatMessagesMapsLiveChatEnded(t *testing.T) {
	api := httptest.NewServer(jsonHandler(http.StatusForbidden, `{"error":{"code":403,"message":"ended","errors":[{"reason":"liveChatEnded"}]}}`))
	client := newTestClient(t, nil, nil, api)
	_, err := client.ListLiveChatMessages(context.Background(), "chat_1", "", "at")
	if !errors.Is(err, ErrLiveChatEnded) {
		t.Fatalf("expected ErrLiveChatEnded, got %v", err)
	}
}

func TestListLiveChatMessagesMapsLiveChatNotFound(t *testing.T) {
	api := httptest.NewServer(jsonHandler(http.StatusNotFound, `{"error":{"code":404,"message":"not found","errors":[{"reason":"liveChatNotFound"}]}}`))
	client := newTestClient(t, nil, nil, api)
	_, err := client.ListLiveChatMessages(context.Background(), "chat_1", "", "at")
	if !errors.Is(err, ErrLiveChatNotFound) {
		t.Fatalf("expected ErrLiveChatNotFound, got %v", err)
	}
}

func TestListLiveChatMessagesMapsUnauthorized(t *testing.T) {
	api := httptest.NewServer(jsonHandler(http.StatusUnauthorized, `{}`))
	client := newTestClient(t, nil, nil, api)
	_, err := client.ListLiveChatMessages(context.Background(), "chat_1", "", "at")
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

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
