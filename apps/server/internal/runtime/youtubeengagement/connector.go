package youtubeengagement

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/streaming-tree/server/internal/domain/account"
	engagement "github.com/streaming-tree/server/internal/domain/engagement"
	"github.com/streaming-tree/server/internal/provider/youtube"
)

// Backoff policy for a genuine transient failure (not a "waiting for
// broadcast/live chat" state, which uses waitingRetryInterval instead -
// that is ordinary streamer behavior, not a failure).
//
// Deliberately `var`, not `const`: tests in this package shrink these to
// keep the suite fast without changing any production behavior (production
// code never overrides them).
var (
	initialBackoff = 1 * time.Second
	maxBackoff     = 30 * time.Second

	// waitingRetryInterval is how often the connector re-checks whether a
	// broadcast has been selected, or a selected broadcast's live chat
	// has become available - fixed, not exponential, since neither is a
	// failure.
	waitingRetryInterval = 10 * time.Second
)

// connector supervises exactly one YouTube account's live chat gRPC
// streamList connection for as long as it stays enabled. See
// docs/provider-integrations/youtube-engagement.md §4b/§7-§8 for the
// full researched transport contract (Stage 15A transport corrective
// pass) this file implements.
type connector struct {
	accountID string
	mgr       *Manager
	cancel    context.CancelFunc

	mu       sync.Mutex
	snapshot Snapshot
}

// chatSession is the small piece of state a *transient* reconnect needs to
// carry forward from one serve() attempt to the next within the same
// run() retry loop, so a stream that merely dropped and is being retried
// can resume without re-baselining - see serve()'s own doc comment and
// docs/provider-integrations/youtube-engagement.md §7's "baseline vs.
// reconnect" distinction. A nil session (or a session for a different
// liveChatID) always forces a fresh baseline.
type chatSession struct {
	liveChatID string
	pageToken  string
}

func newConnector(mgr *Manager, accountID string) *connector {
	return &connector{
		accountID: accountID, mgr: mgr,
		snapshot: Snapshot{AccountID: accountID, Enabled: true, State: StateWaitingForBroadcast},
	}
}

func (c *connector) getSnapshot() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.snapshot
}

func (c *connector) setState(s State) {
	c.mu.Lock()
	c.snapshot.State = s
	c.mu.Unlock()
}

func (c *connector) setBlocked(codes []string) {
	c.mu.Lock()
	c.snapshot.State = StateBlocked
	c.snapshot.BlockerCodes = codes
	c.mu.Unlock()
}

func (c *connector) setWaiting(s State) {
	c.mu.Lock()
	c.snapshot.State = s
	c.snapshot.SelectedBroadcastID = ""
	c.mu.Unlock()
}

func (c *connector) setConnecting(broadcastID string) {
	c.mu.Lock()
	c.snapshot.State = StateConnecting
	c.snapshot.SelectedBroadcastID = broadcastID
	c.mu.Unlock()
}

func (c *connector) setConnected() {
	c.mu.Lock()
	now := c.mgr.now()
	c.snapshot.State = StateConnected
	c.snapshot.ConnectedAt = &now
	c.snapshot.LastError = ""
	c.mu.Unlock()
}

func (c *connector) touchPoll() {
	c.mu.Lock()
	now := c.mgr.now()
	c.snapshot.LastPollAt = &now
	c.mu.Unlock()
}

func (c *connector) touchEvent() {
	c.mu.Lock()
	now := c.mgr.now()
	c.snapshot.LastEventAt = &now
	c.mu.Unlock()
}

func (c *connector) markDataGap() {
	c.mu.Lock()
	now := c.mgr.now()
	c.snapshot.LastDataGapAt = &now
	c.snapshot.PossibleGapCount++
	c.mu.Unlock()
}

func (c *connector) incrementReconnectCount() {
	c.mu.Lock()
	c.snapshot.ReconnectCount++
	c.mu.Unlock()
}

func (c *connector) incrementUnsupported() {
	c.mu.Lock()
	c.snapshot.UnsupportedEventCount++
	c.mu.Unlock()
}

func (c *connector) setError(code string) {
	c.mu.Lock()
	c.snapshot.State = StateError
	c.snapshot.LastError = code
	c.mu.Unlock()
}

// run is the connector's whole lifetime: repeatedly attempt one serve()
// pass, waiting or backing off between attempts as appropriate, until ctx
// is cancelled or a terminal state is reached. A resumable chatSession
// (see serve()) is threaded from one attempt to the next so a stream that
// merely dropped can reconnect without re-baselining; it is cleared
// whenever serve() reports there is nothing left to resume (a fresh
// baseline was already consumed, the chat ended, or the connector is
// waiting for a broadcast/live chat to reappear).
func (c *connector) run(ctx context.Context) {
	backoff := initialBackoff
	var session *chatSession
	for {
		if ctx.Err() != nil {
			c.setState(StateStopping)
			return
		}

		nextSession, err := c.serve(ctx, session)
		session = nextSession

		if ctx.Err() != nil {
			c.setState(StateStopping)
			return
		}

		snap := c.getSnapshot()
		if snap.State.terminalForRetryLoop() {
			return
		}

		wait := backoff
		if snap.State.waitingState() {
			// Waiting for a broadcast/live chat to appear is ordinary,
			// expected streamer behavior, not a failure - retry on a
			// fixed interval, never count it as a reconnect, never grow
			// the failure backoff for it. A held session cannot survive
			// this anyway (serve() only returns a non-nil session
			// alongside a retryable stream error, never alongside a
			// waiting state), but clearing it here is the honest,
			// explicit statement of that invariant.
			wait = waitingRetryInterval
			session = nil
		} else {
			c.incrementReconnectCount()
			c.setState(StateReconnecting)
			c.mgr.logger.Info("youtube engagement connector lost its live chat stream, retrying",
				"accountId", c.accountID, "error", sanitizeErr(err), "resuming", session != nil)
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}

		select {
		case <-time.After(wait):
		case <-ctx.Done():
			c.setState(StateStopping)
			return
		}
		if !snap.State.waitingState() {
			continue
		}
		backoff = initialBackoff // reset failure backoff after a clean waiting cycle
	}
}

func sanitizeErr(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "context ended"
	default:
		return "stream failed"
	}
}

// serve resolves the account's selected broadcast and live chat, then opens
// one streamList gRPC stream and receives from it until an error, a
// chat-ended signal, or ctx cancellation. It returns the chatSession the
// caller (run) should pass back on its next attempt, or nil if nothing is
// resumable (a fresh baseline is required next time, or the loop should
// stop/wait).
//
// Two distinct cutover behaviors apply, per docs/provider-integrations/
// youtube-engagement.md §7:
//
//   - session is nil, or is for a different liveChatID (broadcast/chat
//     changed): this is a genuinely fresh stream. Its first response is
//     consumed entirely to establish a baseline continuation token and is
//     never published to the Event Bus - only responses after that
//     baseline are live.
//   - session matches this liveChatID and carries a page token: this is a
//     resume of a still-live stream that merely dropped. Its first
//     response is treated as live immediately - baselining it would
//     silently drop real chat on every transient reconnect.
func (c *connector) serve(ctx context.Context, session *chatSession) (*chatSession, error) {
	broadcastID, ok := c.resolveBroadcast(ctx)
	if !ok {
		c.setWaiting(StateWaitingForBroadcast)
		return nil, nil
	}

	liveChatID, ok := c.resolveLiveChatID(ctx, broadcastID)
	if !ok {
		c.setWaiting(StateWaitingForLiveChat)
		return nil, nil
	}

	resuming := session != nil && session.liveChatID == liveChatID && session.pageToken != ""
	pageToken := ""
	if resuming {
		pageToken = session.pageToken
	}

	c.setConnecting(broadcastID)

	stream, err := c.mgr.openStream(ctx, c.accountID, liveChatID, pageToken)
	if err != nil {
		if resuming && !continuationRejected(err) {
			// A transient failure opening a resume attempt - the
			// continuation token itself was never rejected by the
			// provider, only the connection attempt failed. Preserve it
			// so the next retry still resumes instead of re-baselining.
			return session, c.classifyPollError(err)
		}
		if resuming {
			// The held continuation was rejected outright (§16) - a
			// possible gap, not a silent loss: the next attempt starts
			// over with a fresh baseline.
			c.markDataGap()
		}
		return nil, c.classifyPollError(err)
	}
	defer stream.Close()

	if !resuming {
		page, err := stream.Recv()
		if err != nil {
			return nil, c.classifyPollError(err)
		}
		c.touchPoll()
		if page.Ended {
			c.setState(StateChatEnded)
			return nil, nil
		}
		pageToken = page.NextPageToken
	}

	c.setConnected()

	for {
		page, err := stream.Recv()
		if err != nil {
			if continuationRejected(err) {
				c.markDataGap()
				return nil, c.classifyPollError(err)
			}
			return &chatSession{liveChatID: liveChatID, pageToken: pageToken}, c.classifyPollError(err)
		}
		c.touchPoll()

		for _, msg := range page.Messages {
			if c.handleMessage(msg) {
				c.setState(StateChatEnded)
				return nil, nil
			}
		}
		if page.Ended {
			c.setState(StateChatEnded)
			return nil, nil
		}
		pageToken = page.NextPageToken
	}
}

// continuationRejected reports whether err means the provider rejected the
// held continuation/liveChatId itself (mapped from gRPC INVALID_ARGUMENT,
// or from the chat becoming disabled/not-found) - as opposed to a merely
// transient transport failure that leaves the continuation still good to
// retry. See docs/provider-integrations/youtube-engagement.md §4b.3/§16.
func continuationRejected(err error) bool {
	return errors.Is(err, youtube.ErrLiveChatNotFound) || errors.Is(err, youtube.ErrLiveChatDisabled)
}

// resolveBroadcast resolves this account's linked destination, then that
// destination's currently-selected YouTube live-broadcast id - reusing
// Stage 7B's existing remote-target selection rather than inventing a
// second selector.
func (c *connector) resolveBroadcast(ctx context.Context) (string, bool) {
	if c.mgr.destinationLookup == nil || c.mgr.broadcastLookup == nil {
		return "", false
	}
	platformID, ok := c.mgr.destinationLookup(c.accountID)
	if !ok {
		return "", false
	}
	return c.mgr.broadcastLookup(platformID)
}

// resolveLiveChatID reads the broadcast's current liveChatId, treating an
// empty value (not live yet, or chat disabled) as an honest "not
// available yet" rather than an error - see docs/provider-integrations/
// youtube-engagement.md §3.5/§7. This stays REST (GetBroadcast) - only the
// message-receive transport moved to gRPC (§9).
func (c *connector) resolveLiveChatID(ctx context.Context, broadcastID string) (string, bool) {
	var liveChatID string
	err := c.mgr.accounts.WithFreshToken(ctx, c.accountID, func(accessToken string) (bool, error) {
		b, err := c.mgr.client.GetBroadcast(ctx, broadcastID, accessToken)
		if err != nil {
			if errors.Is(err, youtube.ErrUnauthorized) {
				return true, err
			}
			return false, err
		}
		liveChatID = b.LiveChatID
		return false, nil
	})
	if err != nil || liveChatID == "" {
		return "", false
	}
	return liveChatID, true
}

// handleMessage normalizes and publishes one message, returning true if
// this message is the chatEndedEvent lifecycle signal (the caller must
// stop receiving, not treat it as a published event).
func (c *connector) handleMessage(msg youtube.LiveChatMessage) bool {
	res, err := youtube.NormalizeLiveChatMessage(c.accountID, msg)
	if err != nil {
		// A malformed message (unexpected shape for its own declared
		// type) - never the raw payload in the log.
		c.mgr.logger.Warn("youtube engagement connector received a malformed message",
			"accountId", c.accountID, "type", msg.Type)
		return false
	}
	if res.Lifecycle == LifecycleChatEnded {
		return true
	}
	if !res.Supported {
		c.incrementUnsupported()
		c.mgr.logger.Info("youtube engagement connector saw an unsupported provider event type",
			"accountId", c.accountID, "type", msg.Type)
		return false
	}

	evt := res.Event
	c.attachDestination(&evt)
	c.touchEvent()
	if _, _, err := c.mgr.bus.Publish(evt); err != nil {
		c.mgr.logger.Warn("youtube engagement connector could not publish a normalized event",
			"accountId", c.accountID, "type", string(evt.Type))
	}
	return false
}

const (
	// LifecycleChatEnded mirrors youtube.LifecycleChatEnded, re-exported
	// here so callers of this package never need to import
	// internal/provider/youtube just to compare against it.
	LifecycleChatEnded = youtube.LifecycleChatEnded
)

// attachDestination sets DestinationID when the account is linked to
// exactly one configured destination.
func (c *connector) attachDestination(evt *engagement.Event) {
	if c.mgr.destinationLookup == nil {
		return
	}
	if id, ok := c.mgr.destinationLookup(c.accountID); ok {
		evt.DestinationID = id
	}
}

// classifyPollError maps a stream-open/Recv failure onto this connector's
// own state, distinguishing terminal operator-facing problems (require an
// explicit Restart/re-Enable) from retryable transport loss (bounded
// exponential backoff - the expected, normal way a long-lived gRPC stream
// recovers from an ordinary network blip). This is a deliberate difference
// from the superseded REST-polling connector, whose discrete request/
// response calls treated most unrecognized failures as terminal by
// default: a streaming connection dropping and reconnecting is ordinary
// operation, not usually a real problem. See docs/provider-integrations/
// youtube-engagement.md §4b.3/§18.
func (c *connector) classifyPollError(err error) error {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.Is(err, youtube.ErrLiveChatEnded):
		c.setState(StateChatEnded)
		return nil
	case errors.Is(err, youtube.ErrLiveChatDisabled), errors.Is(err, youtube.ErrLiveChatNotFound):
		// The chat is no longer reachable the way this connector last
		// knew it (disabled, or the id is stale/invalid - including a
		// gRPC INVALID_ARGUMENT on the held continuation, §4b.3) - an
		// honest waiting state, not a hard error; the outer run loop
		// will re-resolve the broadcast/live chat from scratch next
		// attempt.
		c.setWaiting(StateWaitingForLiveChat)
		return errWaitingForLiveChat
	case errors.Is(err, account.ErrReconnectRequired):
		c.setError(ErrorReconnectRequired)
		return err
	case errors.Is(err, youtube.ErrRateLimited):
		c.setError(ErrorRateLimited)
		return err
	case errors.Is(err, youtube.ErrQuotaExceeded):
		c.setError(ErrorQuotaExceeded)
		return err
	case errors.Is(err, youtube.ErrForbidden):
		// gRPC PERMISSION_DENIED - documented for this RPC, and not
		// something a retry or token refresh fixes (§4b.3).
		c.setError(ErrorProviderUnavailable)
		return err
	case errors.Is(err, youtube.ErrUnauthorized):
		c.setError(ErrorReconnectRequired)
		return err
	case errors.Is(err, youtube.ErrUnavailable):
		// gRPC UNAVAILABLE/DEADLINE_EXCEEDED, or a dial failure -
		// retryable transport loss. Deliberately no setState call here:
		// the outer run() loop's own generic branch transitions to
		// StateReconnecting and retries with bounded exponential
		// backoff.
		return err
	default:
		c.setError(ErrorProviderUnavailable)
		return err
	}
}

var errWaitingForLiveChat = errors.New("youtube live chat no longer reachable")
