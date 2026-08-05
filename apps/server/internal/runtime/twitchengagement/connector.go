package twitchengagement

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/streaming-tree/server/internal/domain/account"
	engagement "github.com/streaming-tree/server/internal/domain/engagement"
	"github.com/streaming-tree/server/internal/provider/twitch"
)

// maxFrameBytes bounds every EventSub WebSocket frame this connector reads -
// Twitch's real payloads are small (a single chat message, a single event),
// so a much larger frame is treated as a malformed/hostile server response
// rather than trusted.
const maxFrameBytes = 1 << 20 // 1 MiB

// welcomeTimeout bounds how long a connector waits for session_welcome after
// dialing, and how long it waits for the new connection's welcome during an
// official session_reconnect handoff.
const welcomeTimeout = 10 * time.Second

// keepaliveGrace is added to Twitch's own negotiated keepalive_timeout_seconds
// before this connector treats a connection as lost - a small allowance for
// network jitter around the exact boundary, not a second independent timeout.
const keepaliveGrace = 5 * time.Second

// defaultKeepaliveTimeout is used only if Twitch's welcome message omits
// keepalive_timeout_seconds (defensive; Twitch's documented default range is
// 10-600s and it is always expected to be present).
const defaultKeepaliveTimeout = 10 * time.Second

// Backoff policy for ordinary (non-official-reconnect) connection loss.
const (
	initialBackoff = 1 * time.Second
	maxBackoff     = 30 * time.Second
)

// connector supervises exactly one Twitch account's EventSub WebSocket
// session for as long as it stays enabled.
type connector struct {
	accountID string
	mgr       *Manager
	cancel    context.CancelFunc

	mu       sync.Mutex
	snapshot Snapshot
}

func newConnector(mgr *Manager, accountID string) *connector {
	return &connector{
		accountID: accountID,
		mgr:       mgr,
		snapshot:  Snapshot{AccountID: accountID, Enabled: true, State: StateConnecting},
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

func (c *connector) setBlocked(codes []string, missingScopes []string) {
	c.mu.Lock()
	c.snapshot.State = StateBlocked
	c.snapshot.BlockerCodes = codes
	c.snapshot.MissingScopes = missingScopes
	c.mu.Unlock()
}

func (c *connector) setConnected(expected, active int) {
	c.mu.Lock()
	now := c.mgr.now()
	c.snapshot.State = StateConnected
	c.snapshot.ConnectedAt = &now
	c.snapshot.ExpectedSubscriptionCount = expected
	c.snapshot.ActiveSubscriptionCount = active
	c.snapshot.BlockerCodes = nil
	c.snapshot.LastError = ""
	c.mu.Unlock()
}

func (c *connector) touchKeepalive() {
	c.mu.Lock()
	now := c.mgr.now()
	c.snapshot.LastKeepaliveAt = &now
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
	c.mu.Unlock()
}

func (c *connector) incrementReconnectCount() {
	c.mu.Lock()
	c.snapshot.ReconnectCount++
	c.mu.Unlock()
}

func (c *connector) setError(code string) {
	c.mu.Lock()
	c.snapshot.State = StateError
	c.snapshot.LastError = code
	c.mu.Unlock()
}

// run is the connector's whole lifetime: repeatedly serve one session,
// reconnecting with bounded backoff after an ordinary loss, until ctx is
// cancelled (Shutdown or explicit Disable/removal) or an unrecoverable
// error is hit (revocation, removed subscription version).
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
			// serve() already placed this connector in a terminal state
			// (error, blocked) that must not auto-retry.
			return
		}

		c.markDataGap()
		c.incrementReconnectCount()
		c.setState(StateReconnecting)
		c.mgr.logger.Info("twitch engagement connector lost connection, reconnecting",
			"accountId", c.accountID, "error", sanitizeErr(err))

		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			c.setState(StateStopping)
			return
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// sanitizeErr returns a short, stable description safe to log - never a raw
// response body or header value.
func sanitizeErr(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "context ended"
	default:
		return "connection lost"
	}
}

// serve performs one full EventSub session: dial, wait for welcome, create
// subscriptions, then process messages (including a transparent official
// session_reconnect handoff) until the connection is lost or ctx is done.
func (c *connector) serve(ctx context.Context) error {
	c.setState(StateConnecting)

	conn, session, err := c.dialAndWelcome(ctx, c.mgr.client.EventSubURL())
	if err != nil {
		return c.classifyStartupError(err)
	}
	defer conn.CloseNow()

	c.setState(StateSubscribing)
	acc, err := c.mgr.accounts.GetAccount(ctx, c.accountID)
	if err != nil {
		c.setError("engagement_connector_unavailable")
		return err
	}

	expected, active, missing, subErr := c.createSubscriptions(ctx, acc, session.ID)
	if subErr != nil {
		c.classifySubscribeError(subErr)
		return subErr
	}
	if active == 0 {
		c.setBlocked([]string{BlockerScopeUpgradeRequired}, missing)
		return fmt.Errorf("no engagement subscriptions could be created")
	}

	c.setConnected(expected, active)
	return c.readLoop(ctx, conn, keepaliveDeadline(session))
}

func keepaliveDeadline(session twitch.EventSubSession) time.Duration {
	if session.KeepaliveTimeoutSeconds == nil || *session.KeepaliveTimeoutSeconds <= 0 {
		return defaultKeepaliveTimeout + keepaliveGrace
	}
	return time.Duration(*session.KeepaliveTimeoutSeconds)*time.Second + keepaliveGrace
}

func (c *connector) classifyStartupError(err error) error {
	switch {
	case errors.Is(err, context.Canceled):
	case errors.Is(err, context.DeadlineExceeded):
		c.setError("twitch_eventsub_unavailable")
	default:
		c.setError("twitch_eventsub_unavailable")
	}
	return err
}

func (c *connector) classifySubscribeError(err error) {
	switch {
	case errors.Is(err, twitch.ErrForbidden):
		c.setBlocked([]string{BlockerScopeUpgradeRequired}, nil)
	case errors.Is(err, twitch.ErrRateLimited):
		c.setError("engagement_rate_limited")
	case errors.Is(err, twitch.ErrUnauthorized):
		c.setError("engagement_connector_unavailable")
	default:
		c.setError("twitch_eventsub_subscription_failed")
	}
}

// dialAndWelcome opens the WebSocket and waits (bounded by welcomeTimeout)
// for session_welcome.
func (c *connector) dialAndWelcome(ctx context.Context, wsURL string) (*websocket.Conn, twitch.EventSubSession, error) {
	dialCtx, cancel := context.WithTimeout(ctx, welcomeTimeout)
	defer cancel()

	conn, _, err := websocket.Dial(dialCtx, wsURL, nil)
	if err != nil {
		return nil, twitch.EventSubSession{}, err
	}
	conn.SetReadLimit(maxFrameBytes)

	c.setState(StateWaitingForWelcome)

	welcomeCtx, cancelWelcome := context.WithTimeout(ctx, welcomeTimeout)
	defer cancelWelcome()

	env, err := readEnvelope(welcomeCtx, conn)
	if err != nil {
		conn.CloseNow()
		return nil, twitch.EventSubSession{}, err
	}
	if env.Metadata.MessageType != twitch.MessageTypeWelcome {
		conn.CloseNow()
		return nil, twitch.EventSubSession{}, fmt.Errorf("%w: expected session_welcome, got %q", errUnexpectedMessage, env.Metadata.MessageType)
	}
	session, err := twitch.ParseWelcome(env.Payload)
	if err != nil || session.ID == "" {
		conn.CloseNow()
		return nil, twitch.EventSubSession{}, fmt.Errorf("%w: malformed session_welcome", errUnexpectedMessage)
	}

	return conn, session, nil
}

var errUnexpectedMessage = errors.New("unexpected eventsub message")

// createSubscriptions creates every EventSubSubscriptionDef whose required
// scope (if any) the account currently has - capability-aware partial
// operation, per docs/provider-integrations/twitch-engagement.md: a missing
// optional scope skips just that subscription rather than failing the whole
// connector, but at least one chat-capable subscription is required to call
// the connector "active" at all (enforced by the caller checking active>0).
func (c *connector) createSubscriptions(ctx context.Context, acc account.Account, sessionID string) (expected, active int, missingScopes []string, err error) {
	granted := make(map[string]bool, len(acc.Scopes))
	for _, s := range acc.Scopes {
		granted[s] = true
	}

	var subscribeErr error
	err = c.mgr.accounts.WithFreshToken(ctx, c.accountID, func(accessToken string) (bool, error) {
		clientID, cerr := c.mgr.accounts.EffectiveClientID(ctx, account.ProviderTwitch)
		if cerr != nil {
			subscribeErr = cerr
			return false, cerr
		}

		expected = 0
		active = 0
		missingScopes = nil
		for _, def := range twitch.EventSubSubscriptionDefs {
			expected++
			if def.RequiredScope != "" && !granted[def.RequiredScope] {
				missingScopes = append(missingScopes, def.RequiredScope)
				continue
			}
			_, subErr := c.mgr.client.CreateEventSubSubscription(ctx, accessToken, clientID, def, acc.ProviderUserID, sessionID)
			if subErr != nil {
				if errors.Is(subErr, twitch.ErrUnauthorized) {
					return true, subErr // signal WithFreshToken to refresh and retry once
				}
				subscribeErr = subErr
				continue
			}
			active++
		}
		return false, nil
	})

	if err != nil {
		return 0, 0, nil, err
	}
	if subscribeErr != nil && active == 0 {
		return expected, active, missingScopes, subscribeErr
	}
	return expected, active, missingScopes, nil
}

// readLoop processes messages on an established, subscribed session until
// an error, close, or ctx cancellation. It transparently handles the
// official session_reconnect handoff in place, without returning (no
// resubscription, no data-gap marker for that path specifically).
func (c *connector) readLoop(ctx context.Context, conn *websocket.Conn, keepaliveWindow time.Duration) error {
	lastActivity := c.mgr.now()
	for {
		readCtx, cancel := context.WithDeadline(ctx, lastActivity.Add(keepaliveWindow))
		env, err := readEnvelope(readCtx, conn)
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}

		lastActivity = c.mgr.now()

		switch env.Metadata.MessageType {
		case twitch.MessageTypeKeepalive:
			c.touchKeepalive()

		case twitch.MessageTypeNotification:
			c.touchEvent()
			c.handleNotification(env)

		case twitch.MessageTypeReconnect:
			newConn, newSession, err := c.handleOfficialReconnect(ctx, env)
			if err != nil {
				return err
			}
			conn.CloseNow()
			conn = newConn
			keepaliveWindow = keepaliveDeadline(newSession)
			c.mu.Lock()
			c.snapshot.State = StateConnected
			c.mu.Unlock()

		case twitch.MessageTypeRevocation:
			c.handleRevocation(env)

		default:
			// Unknown message type - ignore, never crash the connector for
			// a forward-compatible message this code does not recognize.
		}
	}
}

func readEnvelope(ctx context.Context, conn *websocket.Conn) (twitch.EventSubEnvelope, error) {
	_, data, err := conn.Read(ctx)
	if err != nil {
		return twitch.EventSubEnvelope{}, err
	}
	if len(data) > maxFrameBytes {
		return twitch.EventSubEnvelope{}, fmt.Errorf("%w: frame exceeds %d bytes", errUnexpectedMessage, maxFrameBytes)
	}
	var env twitch.EventSubEnvelope
	if err := json.Unmarshal(data, &env); err != nil {
		return twitch.EventSubEnvelope{}, fmt.Errorf("%w: malformed eventsub message: %s", errUnexpectedMessage, err)
	}
	return env, nil
}

func (c *connector) handleNotification(env twitch.EventSubEnvelope) {
	sub, raw, err := twitch.ParseNotification(env.Payload)
	if err != nil {
		c.mgr.logger.Warn("twitch engagement connector received a malformed notification", "accountId", c.accountID)
		return
	}
	ts, _ := time.Parse(time.RFC3339Nano, env.Metadata.MessageTimestamp)
	if ts.IsZero() {
		ts = c.mgr.now()
	}
	evt, err := twitch.NormalizeEventSubNotification(c.accountID, sub.Type, env.Metadata.MessageID, ts, raw)
	if err != nil {
		c.mgr.logger.Warn("twitch engagement connector could not normalize a notification",
			"accountId", c.accountID, "subscriptionType", sub.Type)
		return
	}
	c.attachDestination(&evt)
	if _, _, err := c.mgr.bus.Publish(evt); err != nil {
		c.mgr.logger.Warn("twitch engagement connector could not publish a normalized event",
			"accountId", c.accountID, "type", string(evt.Type))
	}
}

// attachDestination sets DestinationID when the account is linked to
// exactly one configured destination - best-effort; a lookup failure just
// leaves it empty rather than failing the whole event.
func (c *connector) attachDestination(evt *engagement.Event) {
	if c.mgr.destinationLookup == nil {
		return
	}
	if id, ok := c.mgr.destinationLookup(c.accountID); ok {
		evt.DestinationID = id
	}
}

func (c *connector) handleRevocation(env twitch.EventSubEnvelope) {
	sub, err := twitch.ParseRevocation(env.Payload)
	if err != nil {
		return
	}
	c.mu.Lock()
	c.snapshot.ActiveSubscriptionCount--
	if c.snapshot.ActiveSubscriptionCount < 0 {
		c.snapshot.ActiveSubscriptionCount = 0
	}
	c.mu.Unlock()

	c.mgr.logger.Info("twitch engagement subscription revoked",
		"accountId", c.accountID, "subscriptionType", sub.Type, "status", sub.Status)

	switch sub.Status {
	case "authorization_revoked", "user_removed":
		// The whole authorization is gone - every subscription on this
		// connection is effectively dead. Stop retrying automatically;
		// account.Service's own validation worker will independently mark
		// the account reconnect_required the next time it checks.
		c.setError("twitch_eventsub_subscription_revoked")
	case "version_removed":
		c.setError("twitch_eventsub_version_removed")
	default:
		// A single subscription stopped for another reason - the connector
		// keeps serving its remaining subscriptions rather than tearing
		// down the whole session for one.
	}
}

// handleOfficialReconnect implements Twitch's documented session_reconnect
// flow: connect to the given URL exactly as supplied, keep the old
// connection open until the new one's welcome arrives, then hand back the
// new connection for the caller to switch to - see
// docs/provider-integrations/twitch-engagement.md.
func (c *connector) handleOfficialReconnect(ctx context.Context, env twitch.EventSubEnvelope) (*websocket.Conn, twitch.EventSubSession, error) {
	session, err := twitch.ParseReconnect(env.Payload)
	if err != nil || session.ReconnectURL == "" {
		return nil, twitch.EventSubSession{}, fmt.Errorf("%w: malformed session_reconnect", errUnexpectedMessage)
	}
	if err := validateReconnectURL(session.ReconnectURL, c.mgr.allowedReconnectHosts); err != nil {
		return nil, twitch.EventSubSession{}, fmt.Errorf("%w: %s", errInvalidReconnectURL, err)
	}

	c.setState(StateReconnecting)
	newConn, newSession, err := c.dialAndWelcome(ctx, session.ReconnectURL)
	if err != nil {
		return nil, twitch.EventSubSession{}, err
	}
	return newConn, newSession, nil
}

var errInvalidReconnectURL = errors.New("invalid eventsub reconnect url")

// validateReconnectURL enforces: wss scheme, no embedded credentials, and a
// host from the allow-list (the real Twitch host in production; a test host
// only when the caller explicitly configured one for the integration
// build).
func validateReconnectURL(raw string, allowedHosts []string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "wss" && u.Scheme != "ws" {
		return fmt.Errorf("scheme %q is not allowed", u.Scheme)
	}
	if u.User != nil {
		return errors.New("reconnect url must not carry userinfo")
	}
	host := u.Hostname()
	for _, allowed := range allowedHosts {
		if strings.EqualFold(host, allowed) {
			return nil
		}
	}
	return fmt.Errorf("host %q is not an allowed eventsub reconnect host", host)
}
