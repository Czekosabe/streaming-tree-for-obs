package twitch

import (
	"context"
	"errors"
	"time"

	"github.com/streaming-tree/server/internal/domain/account"
	"github.com/streaming-tree/server/internal/outboundchat"
)

var _ outboundchat.Provider = (*Adapter)(nil)

// AssessCapability reports whether acc's currently-granted scopes satisfy
// OutboundChatScopeProfile - independent of metadata and inbound-engagement
// capability, exactly like AssessEngagementCapability already is.
func (a *Adapter) AssessCapability(acc account.Account) outboundchat.Capability {
	assessment := AssessOutboundChatCapability(acc.Scopes)
	return outboundchat.Capability{
		Required: assessment.Required, Granted: assessment.Granted, Missing: assessment.Missing,
		Available: assessment.Available, PermissionUpgradeRequired: assessment.PermissionUpgradeRequired,
	}
}

// SendChatMessage sends req as acc, converting between this package's wire-
// adjacent Client.SendChatMessage and outboundchat's provider-independent
// vocabulary - the one place those two meet, mirroring how the rest of this
// Adapter already converts for account.Provider.
func (a *Adapter) SendChatMessage(ctx context.Context, acc account.Account, token account.TokenBundle, clientID string, req outboundchat.SendMessageRequest) (outboundchat.SendMessageResult, error) {
	result, limit, err := a.client.SendChatMessage(
		ctx, acc.ProviderUserID, acc.ProviderUserID, req.Message, req.ReplyParentMessageID,
		token.AccessToken, clientID,
	)
	if err != nil {
		return outboundchat.SendMessageResult{}, mapSendChatMessageErr(err, limit)
	}

	if !result.IsSent {
		return outboundchat.SendMessageResult{
			Sent: false, Code: "dropped", CompletedAt: time.Now().UTC(),
		}, nil
	}
	return outboundchat.SendMessageResult{
		ProviderMessageID: result.MessageID, Sent: true, CompletedAt: time.Now().UTC(),
	}, nil
}

// mapSendChatMessageErr converts a Client.SendChatMessage error into
// outboundchat's provider-independent sentinel vocabulary - the send-path
// counterpart to mapProviderErr (metadata.go), kept separate because the
// send path's retry-safety requirements (delivery-unknown vs. provider-
// failure vs. rate-limited-with-a-retry-hint) are stricter than a read
// call's.
func mapSendChatMessageErr(err error, limit rateLimit) error {
	switch {
	case errors.Is(err, ErrUnauthorized):
		// Propagated as-is: the caller drives this through
		// account.Service.WithFreshToken, which refreshes and retries
		// exactly once on ErrUnauthorized-shaped failures - see
		// Client.SendChatMessage's own doc comment.
		return err
	case errors.Is(err, ErrForbidden):
		return outboundchat.ErrForbidden
	case errors.Is(err, ErrRateLimited):
		return &outboundchat.RateLimitedError{RetryAt: rateLimitRetryAt(limit)}
	case errors.Is(err, ErrTransportUncertain):
		return outboundchat.ErrDeliveryUnknown
	case errors.Is(err, ErrInvalidResponse):
		// A malformed/non-Twitch-shaped "success" response is exactly as
		// untrustworthy as a lost response - see the outbound-chat
		// contract's retry policy.
		return outboundchat.ErrDeliveryUnknown
	default:
		// ErrUnavailable (a definite 5xx, or the defensive default for an
		// unexpected status) and anything else unrecognized.
		return outboundchat.ErrProviderFailure
	}
}

func rateLimitRetryAt(limit rateLimit) time.Time {
	if !limit.present {
		return time.Time{}
	}
	return limit.resetAt
}
