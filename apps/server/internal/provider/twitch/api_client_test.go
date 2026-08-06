package twitch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetGlobalChatBadgesParsesTheRealResponseShape(t *testing.T) {
	api := httptest.NewServer(jsonHandler(http.StatusOK, `{"data":[
		{"set_id":"vip","versions":[{"id":"1","image_url_1x":"https://static-cdn.jtvnw.net/badges/v1/vip/1","image_url_2x":"https://static-cdn.jtvnw.net/badges/v1/vip/2","image_url_4x":"https://static-cdn.jtvnw.net/badges/v1/vip/4"}]}
	]}`))
	client := newTestClient(t, nil, api)

	sets, err := client.GetGlobalChatBadges(context.Background(), "token", "client-id")
	if err != nil {
		t.Fatalf("GetGlobalChatBadges() error = %v", err)
	}
	if len(sets) != 1 || sets[0].SetID != "vip" {
		t.Fatalf("sets = %+v, want one vip set", sets)
	}
	if len(sets[0].Versions) != 1 || sets[0].Versions[0].ID != "1" || sets[0].Versions[0].ImageURL2x != "https://static-cdn.jtvnw.net/badges/v1/vip/2" {
		t.Errorf("versions = %+v", sets[0].Versions)
	}
}

func TestGetChannelChatBadgesSendsBroadcasterID(t *testing.T) {
	var gotBroadcasterID string
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBroadcasterID = r.URL.Query().Get("broadcaster_id")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	client := newTestClient(t, nil, api)

	if _, err := client.GetChannelChatBadges(context.Background(), "broadcaster_42", "token", "client-id"); err != nil {
		t.Fatalf("GetChannelChatBadges() error = %v", err)
	}
	if gotBroadcasterID != "broadcaster_42" {
		t.Errorf("broadcaster_id sent = %q, want broadcaster_42", gotBroadcasterID)
	}
}

func TestBadgeParsingToleratesAMalformedEntry(t *testing.T) {
	api := httptest.NewServer(jsonHandler(http.StatusOK, `{"data":[
		{"set_id":"","versions":[{"id":"1","image_url_2x":"x"}]},
		{"set_id":"vip","versions":[{"id":"","image_url_2x":"x"},{"id":"1","image_url_2x":"https://static-cdn.jtvnw.net/badges/v1/vip/2"}]}
	]}`))
	client := newTestClient(t, nil, api)

	sets, err := client.GetGlobalChatBadges(context.Background(), "token", "client-id")
	if err != nil {
		t.Fatalf("GetGlobalChatBadges() error = %v", err)
	}
	if len(sets) != 1 {
		t.Fatalf("sets = %+v, want the empty-set_id entry dropped, leaving 1", sets)
	}
	if len(sets[0].Versions) != 1 || sets[0].Versions[0].ID != "1" {
		t.Errorf("versions = %+v, want the empty-id version dropped, leaving id=1", sets[0].Versions)
	}
}

func TestGetGlobalChatBadgesRejectsAnErrorStatus(t *testing.T) {
	api := httptest.NewServer(jsonHandler(http.StatusUnauthorized, `{"status":401,"message":"invalid token"}`))
	client := newTestClient(t, nil, api)

	if _, err := client.GetGlobalChatBadges(context.Background(), "bad-token", "client-id"); err == nil {
		t.Fatal("GetGlobalChatBadges() error = nil, want an error for a 401 response")
	}
}
