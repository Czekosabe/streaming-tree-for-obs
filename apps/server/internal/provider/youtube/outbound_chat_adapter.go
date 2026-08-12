package youtube

import (
	"context"
	"errors"
	"time"

	"github.com/streaming-tree/server/internal/domain/account"
	"github.com/streaming-tree/server/internal/outboundchat"
)

// OutboundChatAdapter implements outboundchat.Provider for YouTube.
//
// Deliberately a separate type from Adapter (internal/domain/
// account.Provider): sending a message needs to resolve a liveChatId
// first (Adapter's own OAuth/identity methods have no reason to carry that
// dependency), and Stage 15A's send path has no reply concept at all,
// unlike Twitch's - see docs/provider-integrations/
// youtube-engagement.md §3.4/§9.
type OutboundChatAdapter struct {
	client *Client
	// destinationLookup and broadcastLookup together resolve a connected
	// account's currently-selected live broadcast id - the exact same
	// two-hop pattern internal/runtime/youtubeengagement's own connector
	// uses (Stage 7B's existing remote-target selection, never a second,
	// invented selector). Kept as injected functions rather than a direct
	// dependency on internal/domain/remotetarget, matching this
	// package's existing "never import a higher-level domain" discipline.
	destinationLookup func(accountID string) (string, bool)
	broadcastLookup   func(platformID string) (broadcastID string, ok bool)
}

// NewOutboundChatAdapter builds an OutboundChatAdapter.
func NewOutboundChatAdapter(
	client *Client,
	destinationLookup func(accountID string) (string, bool),
	broadcastLookup func(platformID string) (broadcastID string, ok bool),
) *OutboundChatAdapter {
	return &OutboundChatAdapter{client: client, destinationLookup: destinationLookup, broadcastLookup: broadcastLookup}
}

var _ outboundchat.Provider = (*OutboundChatAdapter)(nil)

func (a *OutboundChatAdapter) ProviderID() account.ProviderID { return account.ProviderYouTube }

// AssessCapability reports whether acc can send at all - no additional
// scope is required beyond the existing youtube.RequiredScope
// (docs/provider-integrations/youtube-engagement.md §1/§3.6/§3.4), and
// SupportsReply is always false for this provider.
func (a *OutboundChatAdapter) AssessCapability(acc account.Account) outboundchat.Capability {
	granted := acc.HasScope(RequiredScope)
	capability := outboundchat.Capability{Required: []string{RequiredScope}, Available: granted, SupportsReply: false}
	if granted {
		capability.Granted = []string{RequiredScope}
	} else {
		capability.Missing = []string{RequiredScope}
		capability.PermissionUpgradeRequired = true
	}
	return capability
}

// SendChatMessage resolves acc's currently-selected live chat and sends a
// plain text message into it. A reply request is rejected outright - the
// API has no such field to send it with (§9). liveChatId resolution
// happens here, server-side, from the account's own selected broadcast -
// never from a browser-supplied chat id (per this stage's own explicit
// "no arbitrary user-supplied liveChatId" requirement).
func (a *OutboundChatAdapter) SendChatMessage(ctx context.Context, acc account.Account, token account.TokenBundle, clientID string, req outboundchat.SendMessageRequest) (outboundchat.SendMessageResult, error) {
	if req.ReplyParentMessageID != "" {
		return outboundchat.SendMessageResult{}, outboundchat.ErrReplyUnsupported
	}

	broadcastID, ok := a.resolveBroadcast(acc.ID)
	if !ok {
		return outboundchat.SendMessageResult{}, outboundchat.ErrChatUnavailable
	}
	// Resolved fresh on every send, with the same token about to send the
	// message - this application never caches a liveChatId across calls,
	// so a broadcast that just went offline is caught here rather than a
	// send silently failing against a stale id.
	broadcast, err := a.client.GetBroadcast(ctx, broadcastID, token.AccessToken)
	if err != nil {
		return outboundchat.SendMessageResult{}, mapSendChatMessageErr(err)
	}
	if broadcast.LiveChatID == "" {
		return outboundchat.SendMessageResult{}, outboundchat.ErrChatUnavailable
	}

	msg, err := a.client.InsertLiveChatMessage(ctx, broadcast.LiveChatID, req.Message, token.AccessToken)
	if err != nil {
		return outboundchat.SendMessageResult{}, mapSendChatMessageErr(err)
	}
	return outboundchat.SendMessageResult{ProviderMessageID: msg.ID, Sent: true, CompletedAt: time.Now().UTC()}, nil
}

func (a *OutboundChatAdapter) resolveBroadcast(accountID string) (string, bool) {
	if a.destinationLookup == nil || a.broadcastLookup == nil {
		return "", false
	}
	platformID, ok := a.destinationLookup(accountID)
	if !ok {
		return "", false
	}
	return a.broadcastLookup(platformID)
}

// mapSendChatMessageErr converts a Client error into outboundchat's
// provider-independent sentinel vocabulary - the send-path counterpart to
// the read-side error handling in client.go, mirroring
// twitch.mapSendChatMessageErr's own reasoning.
func mapSendChatMessageErr(err error) error {
	switch {
	case errors.Is(err, ErrUnauthorized):
		return outboundchat.ErrUnauthorized
	case errors.Is(err, ErrForbidden), errors.Is(err, ErrLiveChatDisabled):
		return outboundchat.ErrForbidden
	case errors.Is(err, ErrLiveChatEnded), errors.Is(err, ErrLiveChatNotFound):
		return outboundchat.ErrChatUnavailable
	case errors.Is(err, ErrRateLimited), errors.Is(err, ErrQuotaExceeded):
		return &outboundchat.RateLimitedError{}
	case errors.Is(err, ErrMessageInvalid):
		// A clear pre-send validation rejection from the provider itself -
		// never retried, exactly like Twitch's own equivalent case.
		return outboundchat.ErrProviderFailure
	case errors.Is(err, ErrInvalidResponse):
		return outboundchat.ErrDeliveryUnknown
	default:
		// ErrUnavailable (a definite 5xx, or the defensive default for an
		// unexpected status) and anything else unrecognized.
		return outboundchat.ErrProviderFailure
	}
}
