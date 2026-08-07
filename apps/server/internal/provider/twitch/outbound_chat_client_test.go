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
)

func TestSendChatMessageSendsExactRequestShape(t *testing.T) {
	var gotMethod, gotAuth, gotClientID, gotContentType string
	var gotBody map[string]any
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotAuth = r.Header.Get("Authorization")
		gotClientID = r.Header.Get("Client-Id")
		gotContentType = r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		jsonHandler(http.StatusOK, `{"data":[{"message_id":"msg_1","is_sent":true}]}`)(w, r)
	}))
	client := newTestClient(t, nil, api)

	result, _, err := client.SendChatMessage(context.Background(), "broadcaster_1", "broadcaster_1", "hello chat", "", "token", "client-id")
	if err != nil {
		t.Fatalf("SendChatMessage() error = %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotAuth != "Bearer token" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if gotClientID != "client-id" {
		t.Errorf("Client-Id = %q", gotClientID)
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q", gotContentType)
	}
	if gotBody["broadcaster_id"] != "broadcaster_1" || gotBody["sender_id"] != "broadcaster_1" {
		t.Errorf("broadcaster_id/sender_id = %v/%v, want both broadcaster_1", gotBody["broadcaster_id"], gotBody["sender_id"])
	}
	if gotBody["message"] != "hello chat" {
		t.Errorf("message = %v", gotBody["message"])
	}
	if _, ok := gotBody["reply_parent_message_id"]; ok {
		t.Errorf("reply_parent_message_id present with an empty value, want omitted: %v", gotBody)
	}
	if _, ok := gotBody["for_source_only"]; ok {
		t.Errorf("for_source_only present, want never sent: %v", gotBody)
	}
	if _, ok := gotBody["pin"]; ok {
		t.Errorf("pin present, want never sent: %v", gotBody)
	}
	if !result.IsSent || result.MessageID != "msg_1" {
		t.Errorf("result = %+v, want IsSent=true MessageID=msg_1", result)
	}
}

func TestSendChatMessageForwardsReplyParentMessageID(t *testing.T) {
	var gotBody map[string]any
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		jsonHandler(http.StatusOK, `{"data":[{"message_id":"msg_2","is_sent":true}]}`)(w, r)
	}))
	client := newTestClient(t, nil, api)

	if _, _, err := client.SendChatMessage(context.Background(), "b1", "b1", "a reply", "parent_msg_1", "token", "client-id"); err != nil {
		t.Fatalf("SendChatMessage() error = %v", err)
	}
	if gotBody["reply_parent_message_id"] != "parent_msg_1" {
		t.Errorf("reply_parent_message_id = %v, want parent_msg_1", gotBody["reply_parent_message_id"])
	}
}

func TestSendChatMessageIsSentFalseIsNotAnError(t *testing.T) {
	api := httptest.NewServer(jsonHandler(http.StatusOK, `{"data":[{"is_sent":false,"drop_reason":{"code":"automod_held","message":"Your message has been held for review by AutoMod."}}]}`))
	client := newTestClient(t, nil, api)

	result, _, err := client.SendChatMessage(context.Background(), "b1", "b1", "spammy", "", "token", "client-id")
	if err != nil {
		t.Fatalf("SendChatMessage() error = %v, want a normal (non-error) dropped result", err)
	}
	if result.IsSent {
		t.Fatal("IsSent = true, want false")
	}
	if result.DropReasonCode != "automod_held" {
		t.Errorf("DropReasonCode = %q, want automod_held", result.DropReasonCode)
	}
}

func TestSendChatMessageRejectsAMissingDataItem(t *testing.T) {
	api := httptest.NewServer(jsonHandler(http.StatusOK, `{"data":[]}`))
	client := newTestClient(t, nil, api)

	if _, _, err := client.SendChatMessage(context.Background(), "b1", "b1", "hi", "", "token", "client-id"); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("error = %v, want ErrInvalidResponse for a missing data item", err)
	}
}

func TestSendChatMessageRejectsMultipleDataItems(t *testing.T) {
	api := httptest.NewServer(jsonHandler(http.StatusOK, `{"data":[{"message_id":"a","is_sent":true},{"message_id":"b","is_sent":true}]}`))
	client := newTestClient(t, nil, api)

	if _, _, err := client.SendChatMessage(context.Background(), "b1", "b1", "hi", "", "token", "client-id"); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("error = %v, want ErrInvalidResponse for multiple data items", err)
	}
}

func TestSendChatMessageRejectsMalformedJSON(t *testing.T) {
	api := httptest.NewServer(jsonHandler(http.StatusOK, `not json`))
	client := newTestClient(t, nil, api)

	if _, _, err := client.SendChatMessage(context.Background(), "b1", "b1", "hi", "", "token", "client-id"); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("error = %v, want ErrInvalidResponse for malformed JSON", err)
	}
}

func TestSendChatMessageRejectsOversizedResponse(t *testing.T) {
	huge := make([]byte, maxResponseBytes+1024)
	for i := range huge {
		huge[i] = ' '
	}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(huge)
		_, _ = w.Write([]byte(`{"data":[{"message_id":"a","is_sent":true}]}`))
	}))
	client := newTestClient(t, nil, api)

	if _, _, err := client.SendChatMessage(context.Background(), "b1", "b1", "hi", "", "token", "client-id"); !errors.Is(err, ErrInvalidResponse) {
		t.Fatalf("error = %v, want ErrInvalidResponse for a response truncated by the size limit", err)
	}
}

func TestSendChatMessageTimeout(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		jsonHandler(http.StatusOK, `{"data":[{"message_id":"a","is_sent":true}]}`)(w, r)
	}))
	client := newTestClient(t, nil, api)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	_, _, err := client.SendChatMessage(ctx, "b1", "b1", "hi", "", "token", "client-id")
	if !errors.Is(err, ErrTransportUncertain) {
		t.Fatalf("error = %v, want ErrTransportUncertain for a timeout", err)
	}
}

func TestSendChatMessageStatusMapping(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		wantErr error
	}{
		{"400", http.StatusBadRequest, `{"error":"Bad Request","status":400,"message":"the message field is required"}`, ErrUnavailable},
		{"401", http.StatusUnauthorized, `{"error":"Unauthorized","status":401,"message":"invalid token"}`, ErrUnauthorized},
		{"403", http.StatusForbidden, `{"error":"Forbidden","status":403,"message":"banned"}`, ErrForbidden},
		{"404", http.StatusNotFound, `{"error":"Not Found","status":404,"message":"broadcaster not found"}`, ErrUnavailable},
		{"420", chatBackendRateLimitedStatus, `{"error":"Enhance Your Calm","status":420,"message":"sending too fast"}`, ErrRateLimited},
		{"429", http.StatusTooManyRequests, `{"error":"Too Many Requests","status":429,"message":"rate limited"}`, ErrRateLimited},
		{"500", http.StatusInternalServerError, `{"error":"Internal Server Error"}`, ErrUnavailable},
		{"503", http.StatusServiceUnavailable, `{"error":"Service Unavailable"}`, ErrUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := httptest.NewServer(jsonHandler(tc.status, tc.body))
			client := newTestClient(t, nil, api)

			_, _, err := client.SendChatMessage(context.Background(), "b1", "b1", "hi", "", "token", "client-id")
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("status %d: error = %v, want %v", tc.status, err, tc.wantErr)
			}
		})
	}
}

func TestSendChatMessageParsesRateLimitHeadersOn429(t *testing.T) {
	resetAt := time.Now().Add(30 * time.Second).Unix()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Ratelimit-Limit", "20")
		w.Header().Set("Ratelimit-Remaining", "0")
		w.Header().Set("Ratelimit-Reset", strconv.FormatInt(resetAt, 10))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"Too Many Requests"}`))
	}))
	client := newTestClient(t, nil, api)

	_, limit, err := client.SendChatMessage(context.Background(), "b1", "b1", "hi", "", "token", "client-id")
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("error = %v, want ErrRateLimited", err)
	}
	if !limit.present {
		t.Fatal("rate limit not parsed as present")
	}
	if limit.resetAt.Unix() != resetAt {
		t.Errorf("resetAt = %v, want %v", limit.resetAt.Unix(), resetAt)
	}
}
