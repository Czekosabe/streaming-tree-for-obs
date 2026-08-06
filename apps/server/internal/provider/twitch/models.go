// Package twitch is this application's typed adapter over the real Twitch
// APIs, researched and recorded in docs/provider-integrations/twitch.md.
//
// Every wire-format struct in this file exists only here: HTTP handlers
// (internal/httpapi) and the connected-account domain (internal/domain/account)
// never see a Twitch JSON shape directly, only the provider-agnostic types
// account.Provider's methods return.
package twitch

// --- id.twitch.tv (OAuth) wire shapes --------------------------------------

// deviceFlowStartResponse is POST /oauth2/device's success body.
type deviceFlowStartResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// tokenResponse is POST /oauth2/token's success body, for both the
// device-code exchange and a refresh-token exchange.
type tokenResponse struct {
	AccessToken  string   `json:"access_token"`
	ExpiresIn    int      `json:"expires_in"`
	RefreshToken string   `json:"refresh_token"`
	Scope        []string `json:"scope"`
	TokenType    string   `json:"token_type"`
}

// statusMessageResponse is the shape every Twitch OAuth failure uses:
// {"status":400,"message":"authorization_pending"} - see
// docs/provider-integrations/twitch.md for why this is checked by message
// content, not a generic OAuth "error" field.
type statusMessageResponse struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
}

// validateResponse is GET /oauth2/validate's success body.
type validateResponse struct {
	ClientID  string   `json:"client_id"`
	Login     string   `json:"login"`
	UserID    string   `json:"user_id"`
	Scopes    []string `json:"scopes"`
	ExpiresIn int      `json:"expires_in"`
}

// --- api.twitch.tv/helix wire shapes ---------------------------------------

type helixUser struct {
	ID              string `json:"id"`
	Login           string `json:"login"`
	DisplayName     string `json:"display_name"`
	ProfileImageURL string `json:"profile_image_url"`
}

type helixUsersResponse struct {
	Data []helixUser `json:"data"`
}

type helixChannel struct {
	BroadcasterID       string   `json:"broadcaster_id"`
	BroadcasterLogin    string   `json:"broadcaster_login"`
	BroadcasterName     string   `json:"broadcaster_name"`
	BroadcasterLanguage string   `json:"broadcaster_language"`
	GameID              string   `json:"game_id"`
	GameName            string   `json:"game_name"`
	Title               string   `json:"title"`
	Tags                []string `json:"tags"`
}

type helixChannelsResponse struct {
	Data []helixChannel `json:"data"`
}

// modifyChannelRequest sends only the fields this application's verified
// capability table actually supports (docs/provider-integrations/twitch.md):
// title, game_id, broadcaster_language, tags. Never delay,
// content_classification_labels, or is_branded_content - see the "fields
// deliberately not published" section of that document.
type modifyChannelRequest struct {
	Title               *string  `json:"title,omitempty"`
	GameID              *string  `json:"game_id,omitempty"`
	BroadcasterLanguage *string  `json:"broadcaster_language,omitempty"`
	Tags                []string `json:"tags,omitempty"`
}

type helixCategory struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	BoxArtURL string `json:"box_art_url"`
}

type helixCategoriesResponse struct {
	Data []helixCategory `json:"data"`
}

type helixBadgeVersion struct {
	ID         string `json:"id"`
	ImageURL1x string `json:"image_url_1x"`
	ImageURL2x string `json:"image_url_2x"`
	ImageURL4x string `json:"image_url_4x"`
}

type helixBadgeSet struct {
	SetID    string              `json:"set_id"`
	Versions []helixBadgeVersion `json:"versions"`
}

type helixBadgesResponse struct {
	Data []helixBadgeSet `json:"data"`
}
