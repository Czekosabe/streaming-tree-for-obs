package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// LiveChatMessagesLimit bounds each single liveChatMessages.list call - the
// documented range is 200-2000 (docs/provider-integrations/
// youtube-engagement.md §3.2); this application always requests the
// documented default rather than pushing for the maximum, since Stage 15A
// prioritizes honest quota use over squeezing out every possible message
// in one call.
const LiveChatMessagesLimit = 500

// LiveChatMessagesPart is the fixed `part` value this application always
// requests - `id` (for DedupeKey/ProviderEventID), `snippet` (the message
// content/type), and `authorDetails` (the chatter's identity/roles). Never
// operator- or frontend-configurable.
const LiveChatMessagesPart = "id,snippet,authorDetails"

// LiveChatMessagePage is one page of liveChatMessages.list results, with
// only the fields this application's connector actually needs - see
// docs/provider-integrations/youtube-engagement.md §3.2.
type LiveChatMessagePage struct {
	NextPageToken         string
	PollingIntervalMillis int
	// Ended reports whether the response's own offlineAt field was
	// present - i.e. the underlying broadcast has gone offline. This is
	// a different fact from ErrLiveChatEnded (a 403 the request itself
	// received): Ended can be true on an otherwise-successful response.
	Ended    bool
	Messages []LiveChatMessage
}

// LiveChatMessage is this client's normalized return shape for one
// liveChatMessage resource - never the raw JSON, and never containing more
// than the fields the current normalizer (livechat_normalize.go) actually
// reads. See docs/provider-integrations/youtube-engagement.md §3.3 for the
// full researched field mapping.
type LiveChatMessage struct {
	ID   string
	Type string

	AuthorChannelID string
	PublishedAt     string
	DisplayMessage  string

	Author LiveChatMessageAuthor

	// Raw holds the full parsed wire resource so the normalizer can read
	// whichever "*Details" sub-object matches Type without this client
	// needing a second, parallel accessor method per message type. Never
	// exposed outside this package - the normalizer is the only
	// consumer, and it is itself in this same package.
	raw liveChatMessageResource
}

// LiveChatMessageAuthor is the safe, already-flattened author identity for
// one message.
type LiveChatMessageAuthor struct {
	ChannelID       string
	ChannelURL      string
	DisplayName     string
	ProfileImageURL string
	IsVerified      bool
	IsChatOwner     bool
	IsChatSponsor   bool
	IsChatModerator bool
}

// ListLiveChatMessages issues one liveChatMessages.list call: GET
// /liveChat/messages. pageToken is empty for a fresh baseline call (see
// docs/provider-integrations/youtube-engagement.md §7's baseline-first
// cutover) or the previously-returned NextPageToken to continue.
func (c *Client) ListLiveChatMessages(ctx context.Context, liveChatID, pageToken, accessToken string) (LiveChatMessagePage, error) {
	if liveChatID == "" {
		return LiveChatMessagePage{}, fmt.Errorf("%w: liveChatId is required", ErrInvalidResponse)
	}
	query := url.Values{
		"liveChatId": {liveChatID},
		"part":       {LiveChatMessagesPart},
		"maxResults": {fmt.Sprintf("%d", LiveChatMessagesLimit)},
	}
	if pageToken != "" {
		query.Set("pageToken", pageToken)
	}
	status, body, err := c.doAPI(ctx, http.MethodGet, "/liveChat/messages", query, nil, accessToken)
	if err != nil {
		return LiveChatMessagePage{}, err
	}
	if status != http.StatusOK {
		return LiveChatMessagePage{}, classifyAPIError(status, body, "/liveChat/messages")
	}

	var parsed liveChatMessageListResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return LiveChatMessagePage{}, fmt.Errorf("%w: /liveChat/messages: %s", ErrInvalidResponse, err)
	}

	messages := make([]LiveChatMessage, 0, len(parsed.Items))
	for _, item := range parsed.Items {
		if item.ID == "" {
			continue // tolerate a malformed single entry rather than failing the whole page
		}
		messages = append(messages, LiveChatMessage{
			ID:              item.ID,
			Type:            item.Snippet.Type,
			AuthorChannelID: item.Snippet.AuthorChannelID,
			PublishedAt:     item.Snippet.PublishedAt,
			DisplayMessage:  item.Snippet.DisplayMessage,
			Author: LiveChatMessageAuthor{
				ChannelID: item.AuthorDetails.ChannelID, ChannelURL: item.AuthorDetails.ChannelURL,
				DisplayName: item.AuthorDetails.DisplayName, ProfileImageURL: item.AuthorDetails.ProfileImageURL,
				IsVerified: item.AuthorDetails.IsVerified, IsChatOwner: item.AuthorDetails.IsChatOwner,
				IsChatSponsor: item.AuthorDetails.IsChatSponsor, IsChatModerator: item.AuthorDetails.IsChatModerator,
			},
			raw: item,
		})
	}

	return LiveChatMessagePage{
		NextPageToken: parsed.NextPageToken, PollingIntervalMillis: parsed.PollingIntervalMillis,
		Ended: parsed.OfflineAt != "", Messages: messages,
	}, nil
}

// InsertLiveChatMessage sends a plain text message: POST /liveChat/messages
// with a textMessageEvent - the only message type this application ever
// sends. See docs/provider-integrations/youtube-engagement.md §3.4/§9: no
// reply-parent field exists on this API, so this method accepts none.
func (c *Client) InsertLiveChatMessage(ctx context.Context, liveChatID, messageText, accessToken string) (LiveChatMessage, error) {
	body := liveChatMessageInsertRequest{
		Snippet: liveChatMessageInsertSnippet{
			LiveChatID: liveChatID, Type: "textMessageEvent",
			TextMessageDetails: liveChatMessageInsertTextDetails{MessageText: messageText},
		},
	}
	query := url.Values{"part": {LiveChatMessagesPart}}
	status, respBody, err := c.doAPI(ctx, http.MethodPost, "/liveChat/messages", query, body, accessToken)
	if err != nil {
		return LiveChatMessage{}, err
	}
	if status != http.StatusOK && status != http.StatusCreated {
		return LiveChatMessage{}, classifyAPIError(status, respBody, "/liveChat/messages (insert)")
	}
	var parsed liveChatMessageResource
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return LiveChatMessage{}, fmt.Errorf("%w: /liveChat/messages (insert): %s", ErrInvalidResponse, err)
	}
	return LiveChatMessage{ID: parsed.ID, Type: parsed.Snippet.Type, raw: parsed}, nil
}
