// Package youtube is this application's typed adapter over the real Google
// OAuth and YouTube Data/Live Streaming APIs, researched and recorded in
// docs/provider-integrations/youtube.md.
//
// Every wire-format struct in this file exists only here: HTTP handlers
// (internal/httpapi), the connected-account domain (internal/domain/account)
// and the OAuth attempt manager (internal/runtime/youtubeauth) never see a
// Google/YouTube JSON shape directly.
package youtube

import (
	"bytes"
	"encoding/json"
	"strconv"
)

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
		// LiveChatID is the live chat this broadcast owns, when one
		// exists - empty/absent when chat is unavailable (not yet live,
		// or disabled by the owner). See docs/provider-integrations/
		// youtube-engagement.md §3.5.
		LiveChatID string `json:"liveChatId"`
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

// --- liveChatMessages wire shapes (Stage 15A) -------------------------------
//
// See docs/provider-integrations/youtube-engagement.md §3.2/§3.3 for the
// full researched contract these structs mirror.

// flexibleInt64 unmarshals a JSON value that may be encoded either as a
// plain number or as a string containing digits - Google's protobuf-derived
// JSON mapping commonly encodes a 64-bit integer field as a string (to
// avoid JavaScript's float64 precision loss), and the exact encoding for
// liveChatMessage's own amountMicros/banDurationSeconds fields could not be
// confirmed from the documentation prose alone (docs/provider-integrations/
// youtube-engagement.md's own research notes this ambiguity). Accepting
// both defensively is safer than guessing one and silently failing to
// parse real Super Chat amounts.
type flexibleInt64 int64

func (f *flexibleInt64) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		*f = 0
		return nil
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return err
		}
		if s == "" {
			*f = 0
			return nil
		}
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return err
		}
		*f = flexibleInt64(v)
		return nil
	}
	var v int64
	if err := json.Unmarshal(trimmed, &v); err != nil {
		return err
	}
	*f = flexibleInt64(v)
	return nil
}

type liveChatMessageResource struct {
	ID            string                       `json:"id"`
	Snippet       liveChatMessageSnippet       `json:"snippet"`
	AuthorDetails liveChatMessageAuthorDetails `json:"authorDetails"`
}

type liveChatMessageAuthorDetails struct {
	ChannelID       string `json:"channelId"`
	ChannelURL      string `json:"channelUrl"`
	DisplayName     string `json:"displayName"`
	ProfileImageURL string `json:"profileImageUrl"`
	IsVerified      bool   `json:"isVerified"`
	IsChatOwner     bool   `json:"isChatOwner"`
	IsChatSponsor   bool   `json:"isChatSponsor"`
	IsChatModerator bool   `json:"isChatModerator"`
}

// liveChatMessageSnippet carries every "*Details" sub-object as an optional
// pointer, exactly mirroring the API's own "exactly one of, selected by
// type" oneof discipline (docs/provider-integrations/
// youtube-engagement.md §3.3) - the normalizer (livechat_normalize.go)
// switches on Type and reads only the one matching field, never assumes
// more than one is populated.
type liveChatMessageSnippet struct {
	Type              string `json:"type"`
	LiveChatID        string `json:"liveChatId"`
	AuthorChannelID   string `json:"authorChannelId"`
	PublishedAt       string `json:"publishedAt"`
	HasDisplayContent bool   `json:"hasDisplayContent"`
	DisplayMessage    string `json:"displayMessage"`

	TextMessageDetails *struct {
		MessageText string `json:"messageText"`
	} `json:"textMessageDetails,omitempty"`

	SuperChatDetails *struct {
		AmountMicros        flexibleInt64 `json:"amountMicros"`
		Currency            string        `json:"currency"`
		AmountDisplayString string        `json:"amountDisplayString"`
		UserComment         string        `json:"userComment"`
		Tier                int           `json:"tier"`
	} `json:"superChatDetails,omitempty"`

	SuperStickerDetails *struct {
		AmountMicros         flexibleInt64 `json:"amountMicros"`
		Currency             string        `json:"currency"`
		AmountDisplayString  string        `json:"amountDisplayString"`
		Tier                 int           `json:"tier"`
		SuperStickerMetadata struct {
			StickerID string `json:"stickerId"`
			AltText   string `json:"altText"`
			Language  string `json:"language"`
		} `json:"superStickerMetadata"`
	} `json:"superStickerDetails,omitempty"`

	NewSponsorDetails *struct {
		MemberLevelName string `json:"memberLevelName"`
		IsUpgrade       bool   `json:"isUpgrade"`
	} `json:"newSponsorDetails,omitempty"`

	MemberMilestoneChatDetails *struct {
		UserComment     string `json:"userComment"`
		MemberMonth     int    `json:"memberMonth"`
		MemberLevelName string `json:"memberLevelName"`
	} `json:"memberMilestoneChatDetails,omitempty"`

	MembershipGiftingDetails *struct {
		GiftMembershipsCount     int    `json:"giftMembershipsCount"`
		GiftMembershipsLevelName string `json:"giftMembershipsLevelName"`
	} `json:"membershipGiftingDetails,omitempty"`

	GiftMembershipReceivedDetails *struct {
		MemberLevelName                      string `json:"memberLevelName"`
		GifterChannelID                      string `json:"gifterChannelId"`
		AssociatedMembershipGiftingMessageID string `json:"associatedMembershipGiftingMessageId"`
	} `json:"giftMembershipReceivedDetails,omitempty"`

	UserBannedDetails *struct {
		BannedUserDetails struct {
			ChannelID       string `json:"channelId"`
			ChannelURL      string `json:"channelUrl"`
			DisplayName     string `json:"displayName"`
			ProfileImageURL string `json:"profileImageUrl"`
		} `json:"bannedUserDetails"`
		BanType            string        `json:"banType"`
		BanDurationSeconds flexibleInt64 `json:"banDurationSeconds"`
	} `json:"userBannedDetails,omitempty"`
}

// liveChatMessageInsertRequest is the request body for POST
// /liveChat/messages inserting a textMessageEvent - the only message type
// this application ever sends (docs/provider-integrations/
// youtube-engagement.md §3.4/§9: no reply field exists on this API at all).
type liveChatMessageInsertRequest struct {
	Snippet liveChatMessageInsertSnippet `json:"snippet"`
}

type liveChatMessageInsertSnippet struct {
	LiveChatID         string                           `json:"liveChatId"`
	Type               string                           `json:"type"`
	TextMessageDetails liveChatMessageInsertTextDetails `json:"textMessageDetails"`
}

type liveChatMessageInsertTextDetails struct {
	MessageText string `json:"messageText"`
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
