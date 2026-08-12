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

	// minPollInterval/maxPollInterval defensively clamp the server-
	// suggested pollingIntervalMillis (docs/provider-integrations/
	// youtube-engagement.md §3.2) - a misbehaving or unexpected value
	// must never make this connector hammer the API or stall
	// indefinitely.
	minPollInterval = 2 * time.Second
	maxPollInterval = 30 * time.Second
)

// connector supervises exactly one YouTube account's live chat poll loop
// for as long as it stays enabled.
type connector struct {
	accountID string
	mgr       *Manager
	cancel    context.CancelFunc

	mu       sync.Mutex
	snapshot Snapshot
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
// is cancelled or a terminal state is reached.
func (c *connector) run(ctx context.Context) {
	backoff := initialBackoff
	for {
		if ctx.Err() != nil {
			c.setState(StateStopping)
			return
		}

		err := c.serve(ctx)

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
			// the failure backoff for it.
			wait = waitingRetryInterval
		} else {
			c.markDataGap()
			c.incrementReconnectCount()
			c.setState(StateReconnecting)
			c.mgr.logger.Info("youtube engagement connector lost its poll loop, retrying",
				"accountId", c.accountID, "error", sanitizeErr(err))
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
		return "poll failed"
	}
}

// serve resolves the account's selected broadcast and live chat, then
// polls it until an error, a chat-ended signal, or ctx cancellation. A
// fresh baseline (docs/provider-integrations/youtube-engagement.md §7) is
// always established here - serve never resumes a continuation token from
// a previous call, since that is exactly what Restart/broadcast-change use
// to guarantee no old-chat events ever leak into a newly-selected
// broadcast, and a process restart never persists one at all.
func (c *connector) serve(ctx context.Context) error {
	broadcastID, ok := c.resolveBroadcast(ctx)
	if !ok {
		c.setWaiting(StateWaitingForBroadcast)
		return nil
	}

	liveChatID, ok := c.resolveLiveChatID(ctx, broadcastID)
	if !ok {
		c.setWaiting(StateWaitingForLiveChat)
		return nil
	}

	c.setConnecting(broadcastID)

	pageToken, err := c.baseline(ctx, liveChatID)
	if err != nil {
		return c.classifyPollError(err)
	}

	c.setConnected()
	return c.pollLoop(ctx, liveChatID, pageToken)
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
// youtube-engagement.md §3.5/§7.
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

// baseline issues the first ListLiveChatMessages call and returns its
// nextPageToken without publishing anything from it - the safe cutover
// point between provider-returned history and genuinely new/live
// messages.
func (c *connector) baseline(ctx context.Context, liveChatID string) (string, error) {
	var page youtube.LiveChatMessagePage
	err := c.mgr.accounts.WithFreshToken(ctx, c.accountID, func(accessToken string) (bool, error) {
		p, err := c.mgr.client.ListLiveChatMessages(ctx, liveChatID, "", accessToken)
		if err != nil {
			if errors.Is(err, youtube.ErrUnauthorized) {
				return true, err
			}
			return false, err
		}
		page = p
		return false, nil
	})
	if err != nil {
		return "", err
	}
	c.touchPoll()
	return page.NextPageToken, nil
}

// pollLoop repeatedly waits the (clamped) server-suggested interval, then
// polls for new messages, publishing every normalized one, until an error
// or a chat-ended signal.
func (c *connector) pollLoop(ctx context.Context, liveChatID, pageToken string) error {
	interval := minPollInterval
	for {
		select {
		case <-time.After(interval):
		case <-ctx.Done():
			return ctx.Err()
		}

		var page youtube.LiveChatMessagePage
		err := c.mgr.accounts.WithFreshToken(ctx, c.accountID, func(accessToken string) (bool, error) {
			p, err := c.mgr.client.ListLiveChatMessages(ctx, liveChatID, pageToken, accessToken)
			if err != nil {
				if errors.Is(err, youtube.ErrUnauthorized) {
					return true, err
				}
				return false, err
			}
			page = p
			return false, nil
		})
		if err != nil {
			return c.classifyPollError(err)
		}
		c.touchPoll()

		for _, msg := range page.Messages {
			ended := c.handleMessage(msg)
			if ended {
				c.setState(StateChatEnded)
				return nil
			}
		}

		if page.Ended {
			c.setState(StateChatEnded)
			return nil
		}

		pageToken = page.NextPageToken
		interval = clampPollInterval(page.PollingIntervalMillis)
	}
}

func clampPollInterval(ms int) time.Duration {
	d := time.Duration(ms) * time.Millisecond
	if d < minPollInterval {
		return minPollInterval
	}
	if d > maxPollInterval {
		return maxPollInterval
	}
	return d
}

// handleMessage normalizes and publishes one message, returning true if
// this message is the chatEndedEvent lifecycle signal (the caller must
// stop polling, not treat it as a published event).
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

// classifyPollError maps a poll/baseline failure onto this connector's own
// state, distinguishing "the chat genuinely ended" (terminal, honest, no
// retry) from "an invalid/expired continuation" (retryable via a fresh
// baseline, marked as a possible gap) from a real transient/auth failure.
func (c *connector) classifyPollError(err error) error {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return err
	case errors.Is(err, youtube.ErrLiveChatEnded):
		c.setState(StateChatEnded)
		return nil
	case errors.Is(err, youtube.ErrLiveChatDisabled), errors.Is(err, youtube.ErrLiveChatNotFound):
		// The chat is no longer reachable the way this connector last
		// knew it (disabled, or the id is stale/invalid) - an honest
		// waiting state, not a hard error; the outer run loop will
		// re-resolve the broadcast/live chat from scratch next attempt.
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
	case errors.Is(err, youtube.ErrUnauthorized):
		c.setError(ErrorReconnectRequired)
		return err
	default:
		c.setError(ErrorProviderUnavailable)
		return err
	}
}

var errWaitingForLiveChat = errors.New("youtube live chat no longer reachable")
