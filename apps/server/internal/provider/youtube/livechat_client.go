package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// LiveChatMessagePage is one page of liveChatMessage results - produced
// today only by the streamList gRPC transport (livechat_stream_client.go/
// livechat_stream_convert.go); this type predates that transport (it
// originally modeled one liveChatMessages.list REST page, before the
// Stage 15A transport corrective pass replaced REST polling with gRPC -
// see docs/provider-integrations/youtube-engagement.md §4b) and was kept
// unchanged so livechat_normalize.go and internal/runtime/
// youtubeengagement's connector need no changes to consume either
// transport's output.
type LiveChatMessagePage struct {
	NextPageToken string
	// Ended reports whether the response's own offlineAt field was
	// present - i.e. the underlying broadcast has gone offline. This is
	// a different fact from ErrLiveChatEnded (a gRPC/REST error the
	// request itself received): Ended can be true on an otherwise-
	// successful response.
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

// liveChatMessagesInsertPart is the fixed `part` value this application
// requests on the one remaining REST liveChatMessages call
// (InsertLiveChatMessage, below) - `id`/`snippet`/`authorDetails`, the same
// three parts the streamList gRPC transport requests
// (livechat_stream_client.go's LiveChatStreamPart) for receiving. Receiving
// no longer uses REST at all as of the Stage 15A transport corrective pass
// (docs/provider-integrations/youtube-engagement.md §4b/§9) - the
// liveChatMessages.list REST receive method and its own `part`/
// `maxResults` constants were removed with it.
const liveChatMessagesInsertPart = "id,snippet,authorDetails"

// InsertLiveChatMessage sends a plain text message: POST /liveChat/messages
// with a textMessageEvent - the only message type this application ever
// sends. See docs/provider-integrations/youtube-engagement.md §3.4/§9: no
// reply-parent field exists on this API, so this method accepts none. This
// stays REST - only the receive transport moved to gRPC (§9).
func (c *Client) InsertLiveChatMessage(ctx context.Context, liveChatID, messageText, accessToken string) (LiveChatMessage, error) {
	body := liveChatMessageInsertRequest{
		Snippet: liveChatMessageInsertSnippet{
			LiveChatID: liveChatID, Type: "textMessageEvent",
			TextMessageDetails: liveChatMessageInsertTextDetails{MessageText: messageText},
		},
	}
	query := url.Values{"part": {liveChatMessagesInsertPart}}
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
