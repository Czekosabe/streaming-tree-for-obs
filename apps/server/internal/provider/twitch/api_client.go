package twitch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// User is this client's normalized return shape for GET /helix/users.
type User struct {
	ID          string
	Login       string
	DisplayName string
	AvatarURL   string
}

// GetCurrentUser resolves the identity behind an access token: GET
// /helix/users with no id/login parameter, which Twitch resolves to the
// token's own user.
func (c *Client) GetCurrentUser(ctx context.Context, accessToken, clientID string) (User, error) {
	status, body, _, err := c.doHelix(ctx, http.MethodGet, "/users", nil, nil, accessToken, clientID)
	if err != nil {
		return User{}, err
	}
	if status != http.StatusOK {
		return User{}, classifyHelixError(status, body, "/helix/users")
	}

	var parsed helixUsersResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return User{}, fmt.Errorf("%w: /helix/users: %s", ErrInvalidResponse, err)
	}
	if len(parsed.Data) == 0 {
		return User{}, fmt.Errorf("%w: /helix/users: empty data", ErrInvalidResponse)
	}
	u := parsed.Data[0]
	if u.ID == "" || u.Login == "" {
		return User{}, fmt.Errorf("%w: /helix/users: missing a required field", ErrInvalidResponse)
	}
	return User{ID: u.ID, Login: u.Login, DisplayName: u.DisplayName, AvatarURL: u.ProfileImageURL}, nil
}

// Channel is this client's normalized return shape for the channel-metadata
// fields this application's verified capability table actually supports -
// see docs/provider-integrations/twitch.md.
type Channel struct {
	BroadcasterID string
	Title         string
	GameID        string
	GameName      string
	Language      string
	Tags          []string
}

// GetChannel reads current remote channel metadata: GET /helix/channels.
func (c *Client) GetChannel(ctx context.Context, broadcasterID, accessToken, clientID string) (Channel, error) {
	query := url.Values{"broadcaster_id": {broadcasterID}}
	status, body, _, err := c.doHelix(ctx, http.MethodGet, "/channels", query, nil, accessToken, clientID)
	if err != nil {
		return Channel{}, err
	}
	if status != http.StatusOK {
		return Channel{}, classifyHelixError(status, body, "/helix/channels")
	}

	var parsed helixChannelsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return Channel{}, fmt.Errorf("%w: /helix/channels: %s", ErrInvalidResponse, err)
	}
	if len(parsed.Data) == 0 {
		return Channel{}, fmt.Errorf("%w: /helix/channels: empty data", ErrInvalidResponse)
	}
	ch := parsed.Data[0]
	tags := ch.Tags
	if tags == nil {
		tags = []string{}
	}
	return Channel{
		BroadcasterID: ch.BroadcasterID, Title: ch.Title, GameID: ch.GameID,
		GameName: ch.GameName, Language: ch.BroadcasterLanguage, Tags: tags,
	}, nil
}

// ModifyChannelInput carries only the fields this application ever sends to
// Twitch - see modifyChannelRequest's own doc comment for what is
// deliberately excluded.
type ModifyChannelInput struct {
	Title    *string
	GameID   *string
	Language *string
	Tags     []string
}

// ModifyChannel publishes metadata: PATCH /helix/channels. Requires a user
// access token with channel:manage:broadcast.
func (c *Client) ModifyChannel(ctx context.Context, broadcasterID string, input ModifyChannelInput, accessToken, clientID string) error {
	body := modifyChannelRequest{
		Title: input.Title, GameID: input.GameID, BroadcasterLanguage: input.Language, Tags: input.Tags,
	}
	query := url.Values{"broadcaster_id": {broadcasterID}}
	status, respBody, _, err := c.doHelix(ctx, http.MethodPatch, "/channels", query, body, accessToken, clientID)
	if err != nil {
		return err
	}
	if status != http.StatusNoContent && status != http.StatusOK {
		return classifyHelixError(status, respBody, "/helix/channels (modify)")
	}
	return nil
}

// Category is this client's normalized return shape for a category search
// result.
type Category struct {
	ID        string
	Name      string
	BoxArtURL string
}

// SearchCategoryLimit bounds how many results this application ever
// requests or returns from a single search - no arbitrary pagination URL
// from the browser is ever accepted (see internal/httpapi).
const SearchCategoryLimit = 20

// SearchCategories searches Twitch categories/games: GET
// /helix/search/categories.
func (c *Client) SearchCategories(ctx context.Context, query, accessToken, clientID string) ([]Category, error) {
	cleaned, err := sanitizeQuery(query)
	if err != nil {
		return nil, err
	}
	params := url.Values{"query": {cleaned}, "first": {"20"}}
	status, body, _, err := c.doHelix(ctx, http.MethodGet, "/search/categories", params, nil, accessToken, clientID)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, classifyHelixError(status, body, "/helix/search/categories")
	}

	var parsed helixCategoriesResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("%w: /helix/search/categories: %s", ErrInvalidResponse, err)
	}

	results := make([]Category, 0, len(parsed.Data))
	for _, item := range parsed.Data {
		if item.ID == "" || item.Name == "" {
			continue // tolerate a malformed single entry rather than failing the whole search
		}
		results = append(results, Category{ID: item.ID, Name: item.Name, BoxArtURL: item.BoxArtURL})
	}
	return results, nil
}

// ChatBadgeVersion is one version of a chat badge set, normalized from
// Twitch's Get Global/Channel Chat Badges response. See
// docs/provider-integrations/twitch-engagement.md's Stage 9 addendum for
// the endpoints this was researched against.
type ChatBadgeVersion struct {
	ID         string
	ImageURL1x string
	ImageURL2x string
	ImageURL4x string
}

// ChatBadgeSet is one badge set (e.g. "moderator", "subscriber") with every
// version Twitch returned for it.
type ChatBadgeSet struct {
	SetID    string
	Versions []ChatBadgeVersion
}

func normalizeBadgeSets(sets []helixBadgeSet) []ChatBadgeSet {
	out := make([]ChatBadgeSet, 0, len(sets))
	for _, s := range sets {
		if s.SetID == "" {
			continue // tolerate a malformed single entry rather than failing the whole catalog
		}
		versions := make([]ChatBadgeVersion, 0, len(s.Versions))
		for _, v := range s.Versions {
			if v.ID == "" {
				continue
			}
			versions = append(versions, ChatBadgeVersion{
				ID: v.ID, ImageURL1x: v.ImageURL1x, ImageURL2x: v.ImageURL2x, ImageURL4x: v.ImageURL4x,
			})
		}
		out = append(out, ChatBadgeSet{SetID: s.SetID, Versions: versions})
	}
	return out
}

// GetGlobalChatBadges reads Twitch's global chat badge catalog: GET
// /helix/chat/badges/global.
func (c *Client) GetGlobalChatBadges(ctx context.Context, accessToken, clientID string) ([]ChatBadgeSet, error) {
	status, body, _, err := c.doHelix(ctx, http.MethodGet, "/chat/badges/global", nil, nil, accessToken, clientID)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, classifyHelixError(status, body, "/helix/chat/badges/global")
	}
	var parsed helixBadgesResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("%w: /helix/chat/badges/global: %s", ErrInvalidResponse, err)
	}
	return normalizeBadgeSets(parsed.Data), nil
}

// GetChannelChatBadges reads one broadcaster's channel-specific chat badge
// catalog: GET /helix/chat/badges.
func (c *Client) GetChannelChatBadges(ctx context.Context, broadcasterID, accessToken, clientID string) ([]ChatBadgeSet, error) {
	query := url.Values{"broadcaster_id": {broadcasterID}}
	status, body, _, err := c.doHelix(ctx, http.MethodGet, "/chat/badges", query, nil, accessToken, clientID)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, classifyHelixError(status, body, "/helix/chat/badges")
	}
	var parsed helixBadgesResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("%w: /helix/chat/badges: %s", ErrInvalidResponse, err)
	}
	return normalizeBadgeSets(parsed.Data), nil
}

func classifyHelixError(status int, body []byte, endpoint string) error {
	switch status {
	case http.StatusUnauthorized:
		return wireErr(ErrUnauthorized, status, endpoint)
	case http.StatusForbidden:
		return wireErr(ErrForbidden, status, endpoint)
	case http.StatusTooManyRequests:
		return wireErr(ErrRateLimited, status, endpoint)
	}
	if status >= 500 {
		return wireErr(ErrUnavailable, status, endpoint)
	}
	return wireErr(ErrInvalidResponse, status, endpoint)
}

// sanitizeQuery trims and lower-bounds a search query before it is ever sent
// to Twitch - the HTTP layer (internal/httpapi) performs the primary
// length/emptiness validation; this is a defensive second check so this
// package is safe to call directly from a test or a future caller too.
func sanitizeQuery(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("%w: empty category search query", ErrInvalidResponse)
	}
	return trimmed, nil
}
