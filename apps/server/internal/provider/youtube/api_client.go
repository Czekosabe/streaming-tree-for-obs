package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// Channel is this client's normalized return shape for a channel owned by
// the authenticated user.
type Channel struct {
	ID           string
	Title        string
	Description  string
	CustomURL    string
	Country      string
	ThumbnailURL string
}

// maxChannelsReturned bounds how many channels this application will ever
// hold in memory for one channels.list(mine=true) response - a defensive
// ceiling, not a documented Google limit.
const maxChannelsReturned = 50

// ListMyChannels resolves every channel owned by the authenticated user:
// GET /channels?mine=true. Google's own documentation does not explicitly
// confirm whether a Brand Account can cause more than one channel to be
// returned (docs/provider-integrations/youtube.md), so this method returns
// the full list and lets the caller (internal/runtime/youtubeauth) decide
// how to handle zero, one, or many.
func (c *Client) ListMyChannels(ctx context.Context, accessToken string) ([]Channel, error) {
	query := url.Values{"part": {"snippet"}, "mine": {"true"}, "maxResults": {"50"}}
	status, body, err := c.doAPI(ctx, http.MethodGet, "/channels", query, nil, accessToken)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, classifyAPIError(status, body, "/channels")
	}

	var parsed channelListResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("%w: /channels: %s", ErrInvalidResponse, err)
	}

	results := make([]Channel, 0, len(parsed.Items))
	for i, item := range parsed.Items {
		if i >= maxChannelsReturned {
			break
		}
		if item.ID == "" {
			continue // tolerate a malformed single entry rather than failing the whole list
		}
		results = append(results, Channel{
			ID: item.ID, Title: item.Snippet.Title, Description: item.Snippet.Description,
			CustomURL: item.Snippet.CustomURL, Country: item.Snippet.Country,
			ThumbnailURL: item.Snippet.Thumbnails.Default.URL,
		})
	}
	return results, nil
}

// GetChannel reads one specific channel by ID: GET /channels?id=. Used to
// resolve a connected account's country for the category-region default
// (docs/provider-integrations/youtube.md's "Category region" section),
// independent of channels.list(mine=true)'s own multi-channel handling.
func (c *Client) GetChannel(ctx context.Context, channelID, accessToken string) (Channel, error) {
	query := url.Values{"part": {"snippet"}, "id": {channelID}}
	status, body, err := c.doAPI(ctx, http.MethodGet, "/channels", query, nil, accessToken)
	if err != nil {
		return Channel{}, err
	}
	if status != http.StatusOK {
		return Channel{}, classifyAPIError(status, body, "/channels")
	}
	var parsed channelListResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return Channel{}, fmt.Errorf("%w: /channels: %s", ErrInvalidResponse, err)
	}
	if len(parsed.Items) == 0 {
		return Channel{}, fmt.Errorf("%w: /channels: channel not found", ErrInvalidResponse)
	}
	item := parsed.Items[0]
	return Channel{
		ID: item.ID, Title: item.Snippet.Title, Description: item.Snippet.Description,
		CustomURL: item.Snippet.CustomURL, Country: item.Snippet.Country,
		ThumbnailURL: item.Snippet.Thumbnails.Default.URL,
	}, nil
}

// Broadcast is this client's normalized return shape for a live broadcast -
// deliberately excludes anything ingestion-related (no stream name, no
// bound-stream data): see docs/provider-integrations/youtube.md's
// "Broadcast discovery" section.
type Broadcast struct {
	ID                 string
	Title              string
	LifeCycleStatus    string
	PrivacyStatus      string
	ScheduledStartTime string
	ActualStartTime    string
}

// maxBroadcastsReturned bounds each broadcastStatus query's result count.
const maxBroadcastsReturned = 25

// ListBroadcasts lists the authenticated user's active and upcoming
// broadcasts, merged and de-duplicated by ID.
//
// Google deprecated persistent/default broadcasts in 2020
// (docs/provider-integrations/youtube.md); broadcastType=persistent returns
// no results today, so this method never requests it, and "completed"
// broadcasts are never requested either, since a finished broadcast cannot
// receive this application's live metadata publish.
func (c *Client) ListBroadcasts(ctx context.Context, accessToken string) ([]Broadcast, error) {
	seen := make(map[string]struct{})
	var merged []Broadcast

	for _, status := range []string{"active", "upcoming"} {
		query := url.Values{
			"part": {"snippet,status"}, "mine": {"true"},
			"broadcastStatus": {status}, "broadcastType": {"all"}, "maxResults": {"25"},
		}
		respStatus, body, err := c.doAPI(ctx, http.MethodGet, "/liveBroadcasts", query, nil, accessToken)
		if err != nil {
			return nil, err
		}
		if respStatus != http.StatusOK {
			return nil, classifyAPIError(respStatus, body, "/liveBroadcasts")
		}

		var parsed liveBroadcastListResponse
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("%w: /liveBroadcasts: %s", ErrInvalidResponse, err)
		}
		for i, item := range parsed.Items {
			if i >= maxBroadcastsReturned {
				break
			}
			if item.ID == "" {
				continue
			}
			if _, dup := seen[item.ID]; dup {
				continue
			}
			seen[item.ID] = struct{}{}
			merged = append(merged, Broadcast{
				ID: item.ID, Title: item.Snippet.Title, LifeCycleStatus: item.Status.LifeCycleStatus,
				PrivacyStatus: item.Status.PrivacyStatus, ScheduledStartTime: item.Snippet.ScheduledStartTime,
				ActualStartTime: item.Snippet.ActualStartTime,
			})
		}
	}

	if merged == nil {
		merged = []Broadcast{}
	}
	return merged, nil
}

// GetBroadcast reads one broadcast by ID: GET /liveBroadcasts?id=. Used to
// verify a broadcast an operator selected actually belongs to the linked
// channel's own list before it is stored as a remote target.
func (c *Client) GetBroadcast(ctx context.Context, broadcastID, accessToken string) (Broadcast, error) {
	query := url.Values{"part": {"snippet,status"}, "id": {broadcastID}}
	status, body, err := c.doAPI(ctx, http.MethodGet, "/liveBroadcasts", query, nil, accessToken)
	if err != nil {
		return Broadcast{}, err
	}
	if status != http.StatusOK {
		return Broadcast{}, classifyAPIError(status, body, "/liveBroadcasts")
	}
	var parsed liveBroadcastListResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return Broadcast{}, fmt.Errorf("%w: /liveBroadcasts: %s", ErrInvalidResponse, err)
	}
	if len(parsed.Items) == 0 {
		return Broadcast{}, fmt.Errorf("%w: /liveBroadcasts: broadcast not found", ErrInvalidResponse)
	}
	item := parsed.Items[0]
	return Broadcast{
		ID: item.ID, Title: item.Snippet.Title, LifeCycleStatus: item.Status.LifeCycleStatus,
		PrivacyStatus: item.Status.PrivacyStatus, ScheduledStartTime: item.Snippet.ScheduledStartTime,
		ActualStartTime: item.Snippet.ActualStartTime,
	}, nil
}

// Video is this client's normalized return shape for the video-metadata
// fields this application's verified capability table supports - see
// docs/provider-integrations/youtube.md.
type Video struct {
	ID              string
	Title           string
	Description     string
	Tags            []string
	CategoryID      string
	DefaultLanguage string
	PrivacyStatus   string
	// raw is the exact resource this application read, kept only long
	// enough to build a safe read-modify-write update - see UpdateVideo's
	// own doc comment. Never exposed outside this package.
	raw videoResource
}

// GetVideo reads a video's current remote metadata: GET /videos?id=.
func (c *Client) GetVideo(ctx context.Context, videoID, accessToken string) (Video, error) {
	query := url.Values{"part": {"snippet,status"}, "id": {videoID}}
	status, body, err := c.doAPI(ctx, http.MethodGet, "/videos", query, nil, accessToken)
	if err != nil {
		return Video{}, err
	}
	if status != http.StatusOK {
		return Video{}, classifyAPIError(status, body, "/videos")
	}
	var parsed videoListResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return Video{}, fmt.Errorf("%w: /videos: %s", ErrInvalidResponse, err)
	}
	if len(parsed.Items) == 0 {
		return Video{}, fmt.Errorf("%w: /videos: empty data", ErrInvalidResponse)
	}
	item := parsed.Items[0]
	tags := item.Snippet.Tags
	if tags == nil {
		tags = []string{}
	}
	return Video{
		ID: item.ID, Title: item.Snippet.Title, Description: item.Snippet.Description,
		Tags: tags, CategoryID: item.Snippet.CategoryID, DefaultLanguage: item.Snippet.DefaultLanguage,
		PrivacyStatus: item.Status.PrivacyStatus, raw: item,
	}, nil
}

// VideoUpdateInput carries only the fields this application ever changes.
type VideoUpdateInput struct {
	Title           string
	Description     string
	Tags            []string
	CategoryID      string
	DefaultLanguage string
	PrivacyStatus   string
}

// UpdateVideo publishes metadata: PUT /videos?part=snippet,status.
//
// current must be a Video this application just fetched with GetVideo -
// Google's videos.update deletes any mutable property the submitted part
// does not specify (directly confirmed in docs/provider-integrations/
// youtube.md), so this method starts from the just-read resource and
// overwrites only the fields VideoUpdateInput actually carries, preserving
// every other mutable property (including selfDeclaredMadeForKids, which
// this application never manages) exactly as read.
func (c *Client) UpdateVideo(ctx context.Context, current Video, in VideoUpdateInput, accessToken string) error {
	snippet := current.raw.Snippet
	snippet.Title = in.Title
	snippet.Description = in.Description
	snippet.Tags = in.Tags
	snippet.CategoryID = in.CategoryID
	snippet.DefaultLanguage = in.DefaultLanguage

	status := current.raw.Status
	status.PrivacyStatus = in.PrivacyStatus

	body := videoUpdateRequest{ID: current.ID, Snippet: snippet, Status: status}
	query := url.Values{"part": {"snippet,status"}}
	respStatus, respBody, err := c.doAPI(ctx, http.MethodPut, "/videos", query, body, accessToken)
	if err != nil {
		return err
	}
	if respStatus != http.StatusOK {
		return classifyAPIError(respStatus, respBody, "/videos (update)")
	}
	return nil
}

// Category is this client's normalized return shape for a video category.
type Category struct {
	ID   string
	Name string
}

// ListCategoriesLimit bounds how many results this application ever
// returns from a single region's category list.
const ListCategoriesLimit = 50

// ListCategories lists assignable video categories for a region: GET
// /videoCategories?regionCode=. regionCode is required by this application
// (docs/provider-integrations/youtube.md's "Category region" section) - no
// arbitrary pagination URL from the browser is ever accepted.
func (c *Client) ListCategories(ctx context.Context, regionCode, accessToken string) ([]Category, error) {
	query := url.Values{"part": {"snippet"}, "regionCode": {regionCode}}
	status, body, err := c.doAPI(ctx, http.MethodGet, "/videoCategories", query, nil, accessToken)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, classifyAPIError(status, body, "/videoCategories")
	}

	var parsed videoCategoryListResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("%w: /videoCategories: %s", ErrInvalidResponse, err)
	}

	results := make([]Category, 0, len(parsed.Items))
	for i, item := range parsed.Items {
		if i >= ListCategoriesLimit {
			break
		}
		if !item.Snippet.Assignable || item.ID == "" || item.Snippet.Title == "" {
			continue
		}
		results = append(results, Category{ID: item.ID, Name: item.Snippet.Title})
	}
	return results, nil
}
