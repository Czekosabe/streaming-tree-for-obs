package chatautomation

import (
	"context"
	"errors"

	"github.com/streaming-tree/server/internal/domain/account"
	"github.com/streaming-tree/server/internal/outboundchat"
)

// automationQueueQuota bounds how many of an account's outboundchat
// dispatcher queue slots automation (SourceCommand + SourceScheduled)
// may ever occupy at once - deliberately less than
// outboundchat.MaxQueueDepth, so a manual operator send submitted at
// any moment always finds room. Manual sends are never subject to this
// quota - they only ever hit Stage 11A's own real queue-full ceiling
// (outboundchat.MaxQueueDepth), unchanged. See the Stage 11B task's own
// Part 3 ("manual submissions may still be accepted when the scheduled
// sub-queue is full").
const automationQueueQuota = outboundchat.MaxQueueDepth / 2

// outboundSender is the narrow slice of outboundchat.Manager this
// package depends on - a real *outboundchat.Manager satisfies it
// unchanged; tests supply a trivial fake instead of needing a real
// dispatcher's own goroutines and local rate-limit timing just to
// verify the quota policy below.
type outboundSender interface {
	Status(ctx context.Context, accountID string) (outboundchat.Snapshot, error)
	Send(ctx context.Context, req outboundchat.SendMessageRequest) (outboundchat.SendMessageResult, error)
}

// dispatcher is the one, single place in this package that ever calls
// outboundchat.Manager.Send - both the scheduler and the command engine
// go through it. There is no second outbound queue: automation
// self-limits by checking the account's already-existing dispatcher
// queue depth before submitting, so Stage 11A's own bounded queue stays
// the single source of truth for what is actually pending.
type dispatcher struct {
	outbound outboundSender
}

func newDispatcher(outbound outboundSender) *dispatcher {
	return &dispatcher{outbound: outbound}
}

// send submits req under the automation quota above. A scheduled or
// command send that finds the quota already full is skipped immediately
// (ErrQueueFull) rather than blocking or growing an unbounded backlog -
// see the Stage 11B task's own Part 3 and Part 17 (commands must never
// accumulate a stale response).
//
// A command response's own ReplyParentMessageID (see commands.go's own
// "reply to the triggering message" behavior) is dropped here whenever
// the target provider does not support replying at all (Stage 15A:
// YouTube's send API has no such concept - see
// outboundchat.Capability.SupportsReply) - commands.go itself has no
// reason to know which providers support replies; this is the one place
// both the scheduler and the command engine already funnel through, and
// the same Status() call already made for the quota check above already
// carries the answer. Without this, every command response for a
// non-replying provider would otherwise fail outright with
// ErrReplyUnsupported on every single trigger, never sending anything.
func (d *dispatcher) send(ctx context.Context, req outboundchat.SendMessageRequest) (outboundchat.SendMessageResult, error) {
	snap, err := d.outbound.Status(ctx, req.AccountID)
	if err == nil {
		if snap.QueueDepth >= automationQueueQuota {
			return outboundchat.SendMessageResult{}, outboundchat.ErrQueueFull
		}
		if !snap.Capability.SupportsReply {
			req.ReplyParentMessageID = ""
		}
	}
	return d.outbound.Send(ctx, req)
}

// skipReasonForErr maps an outboundchat/account error (or a nil error
// with Sent == false) to a stable SkipReason - never the error's own
// free-text message, which could in principle embed provider detail.
func skipReasonForErr(err error) SkipReason {
	switch {
	case err == nil:
		return SkipSendFailed
	case errors.Is(err, outboundchat.ErrUnsupportedProvider):
		return SkipProviderUnsupported
	case errors.Is(err, outboundchat.ErrPermissionRequired), errors.Is(err, account.ErrMissingScope):
		return SkipPermissionRequired
	case errors.Is(err, outboundchat.ErrQueueFull):
		return SkipQueueFull
	case errIsRateLimited(err):
		return SkipRateLimited
	default:
		return SkipSendFailed
	}
}

func errIsRateLimited(err error) bool {
	var rateLimitErr *outboundchat.RateLimitedError
	return errors.As(err, &rateLimitErr)
}
