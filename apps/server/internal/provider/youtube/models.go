// Package youtube is this application's typed adapter over the real Google
// OAuth and YouTube Data/Live Streaming APIs, researched and recorded in
// docs/provider-integrations/youtube.md.
//
// Every wire-format struct in this file exists only here: HTTP handlers
// (internal/httpapi), the connected-account domain (internal/domain/account)
// and the OAuth attempt manager (internal/runtime/youtubeauth) never see a
// Google/YouTube JSON shape directly.
package youtube

// --- oauth2.googleapis.com wire shapes --------------------------------------

// tokenResponse is POST /token's success body, for both the authorization-
// code exchange and a refresh-token exchange.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
	TokenType    string `json:"token_type"`
}

// tokenErrorResponse is the standard OAuth 2.0 token-endpoint error shape
// Google uses: {"error":"invalid_grant","error_description":"..."}.
type tokenErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// tokenInfoResponse is GET /tokeninfo's success body.
type tokenInfoResponse struct {
	Audience  string `json:"aud"`
	Scope     string `json:"scope"`
	ExpiresIn string `json:"expires_in"`
}

// --- www.googleapis.com/youtube/v3 wire shapes ------------------------------

type channelListResponse struct {
	Items []channelResource `json:"items"`
}

type channelResource struct {
	ID      string `json:"id"`
	Snippet struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		CustomURL   string `json:"customUrl"`
		Country     string `json:"country"`
		Thumbnails  struct {
			Default struct {
				URL string `json:"url"`
			} `json:"default"`
		} `json:"thumbnails"`
	} `json:"snippet"`
}

type liveBroadcastListResponse struct {
	Items []liveBroadcastResource `json:"items"`
}

type liveBroadcastResource struct {
	ID      string `json:"id"`
	Snippet struct {
		Title              string `json:"title"`
		ScheduledStartTime string `json:"scheduledStartTime"`
		ActualStartTime    string `json:"actualStartTime"`
	} `json:"snippet"`
	Status struct {
		LifeCycleStatus string `json:"lifeCycleStatus"`
		PrivacyStatus   string `json:"privacyStatus"`
	} `json:"status"`
}

type videoListResponse struct {
	Items []videoResource `json:"items"`
}

// videoResource models exactly the snippet/status sub-fields this
// application reads or writes - see docs/provider-integrations/youtube.md's
// safe-update section for why every mutable field this application does not
// manage is still round-tripped rather than omitted.
type videoResource struct {
	ID      string       `json:"id"`
	Snippet videoSnippet `json:"snippet"`
	Status  videoStatus  `json:"status"`
}

type videoSnippet struct {
	Title           string   `json:"title"`
	Description     string   `json:"description"`
	Tags            []string `json:"tags,omitempty"`
	CategoryID      string   `json:"categoryId"`
	DefaultLanguage string   `json:"defaultLanguage,omitempty"`
}

type videoStatus struct {
	PrivacyStatus           string `json:"privacyStatus"`
	SelfDeclaredMadeForKids *bool  `json:"selfDeclaredMadeForKids,omitempty"`
}

type videoUpdateRequest struct {
	ID      string       `json:"id"`
	Snippet videoSnippet `json:"snippet"`
	Status  videoStatus  `json:"status"`
}

type videoCategoryListResponse struct {
	Items []videoCategoryResource `json:"items"`
}

type videoCategoryResource struct {
	ID      string `json:"id"`
	Snippet struct {
		Title      string `json:"title"`
		Assignable bool   `json:"assignable"`
	} `json:"snippet"`
}

// googleAPIErrorResponse is the standard Google API JSON error envelope:
// {"error":{"code":403,"message":"...","errors":[{"reason":"quotaExceeded"}]}}.
type googleAPIErrorResponse struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Errors  []struct {
			Reason string `json:"reason"`
		} `json:"errors"`
	} `json:"error"`
}
