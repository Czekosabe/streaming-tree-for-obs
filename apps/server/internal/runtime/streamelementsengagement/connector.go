package streamelementsengagement

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/streaming-tree/server/internal/domain/donationsource"
	"github.com/streaming-tree/server/internal/provider/streamelements"
)

// Backoff policy for a failed connect/read attempt. Deliberately `var`, not
// `const`: tests in this package shrink these to keep the suite fast
// without changing any production behavior.
//
// Unlike internal/runtime/youtubeengagement's own connector (whose backoff
// never resets except on its "waiting" cycle - noted there as "worth
// deciding differently"), this connector resets to initialBackoff whenever
// a connect attempt actually succeeds (regardless of how long the
// resulting stream then stayed up), since a successful connect means the
// "can we reach the provider at all" problem is over; only read-loop
// failures after that grow the backoff again from a fresh baseline.
var (
	initialBackoff = 1 * time.Second
	maxBackoff     = 30 * time.Second
)

// connector supervises exactly one donation source's Astro WebSocket
// connection for as long as it stays enabled.
type connector struct {
	sourceID string
	mgr      *Manager
	cancel   context.CancelFunc

	mu       sync.Mutex
	snapshot Snapshot
}

func newConnector(mgr *Manager, sourceID string) *connector {
	return &connector{
		sourceID: sourceID, mgr: mgr,
		snapshot: Snapshot{SourceID: sourceID, Enabled: true, State: StateConnecting},
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

func (c *connector) setConnected() {
	c.mu.Lock()
	now := c.mgr.now()
	c.snapshot.State = StateConnected
	c.snapshot.ConnectedAt = &now
	c.snapshot.LastError = ""
	c.mu.Unlock()
}

func (c *connector) setPossibleGap() {
	c.mu.Lock()
	now := c.mgr.now()
	c.snapshot.State = StatePossibleGap
	c.snapshot.ConnectedAt = &now
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

func (c *connector) setTerminal(s State, code string) {
	c.mu.Lock()
	c.snapshot.State = s
	c.snapshot.LastError = code
	c.mu.Unlock()
}

// run is the connector's whole lifetime: repeatedly attempt one serve()
// pass, backing off between attempts, until ctx is cancelled or a terminal
// state is reached. resumeToken and hadPriorGap are threaded from one
// attempt to the next exactly the way youtubeengagement threads its own
// chatSession.
func (c *connector) run(ctx context.Context) {
	backoff := initialBackoff
	resumeToken := ""
	hadPriorGap := false

	for {
		if ctx.Err() != nil {
			c.setState(StateStopping)
			return
		}

		nextResumeToken, graceful, connected, err := c.serve(ctx, resumeToken, hadPriorGap)

		if ctx.Err() != nil {
			c.setState(StateStopping)
			return
		}

		snap := c.getSnapshot()
		if snap.State.terminalForRetryLoop() {
			return
		}

		c.incrementReconnectCount()
		if graceful {
			resumeToken = nextResumeToken
			hadPriorGap = false
		} else {
			resumeToken = ""
			if !hadPriorGap {
				c.markDataGap()
			}
			hadPriorGap = true
		}
		c.setState(StateReconnecting)
		c.mgr.logger.Info("streamelements engagement connector lost its connection, retrying",
			"sourceId", c.sourceID, "error", sanitizeErr(err), "graceful", graceful)

		if connected {
			backoff = initialBackoff
		} else {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}

		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			c.setState(StateStopping)
			return
		}
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
		return "connection failed"
	}
}

// serve loads the source's current credential/room, opens one Astro
// connection, and receives from it until an error, a graceful reconnect
// handoff, or ctx cancellation.
//
// Returns: nextResumeToken (valid only when graceful is true); graceful
// (true for a documented `reconnect` handoff, false for any other loss);
// connected (true iff the WebSocket connect+subscribe step itself
// succeeded, regardless of what happened afterward - the signal run() uses
// to decide whether to reset its backoff); and err.
func (c *connector) serve(ctx context.Context, resumeToken string, hadPriorGap bool) (nextResumeToken string, graceful bool, connected bool, err error) {
	src, ok := c.mgr.getSource(ctx, c.sourceID)
	if !ok {
		return "", false, false, errSourceGone
	}

	token, loadErr := donationsource.LoadCredential(ctx, c.mgr.secrets, c.sourceID)
	if loadErr != nil {
		c.setTerminal(StateError, ErrorCredentialMissing)
		return "", false, false, loadErr
	}

	c.setState(StateConnecting)
	stream, connErr := c.mgr.client.Connect(ctx, src.RemoteChannelID, token, true, resumeToken)
	if connErr != nil {
		if errors.Is(connErr, streamelements.ErrSubscribeFailed) {
			c.setTerminal(StateReconnectRequired, ErrorAuthFailed)
			return "", false, false, connErr
		}
		return "", false, false, connErr
	}
	defer stream.Close()

	if resumeToken != "" || !hadPriorGap {
		c.setConnected()
	} else {
		c.setPossibleGap()
	}

	for {
		evt, recvErr := stream.Recv(ctx)
		if recvErr != nil {
			return "", false, true, recvErr
		}
		switch evt.Kind {
		case streamelements.KindReconnect:
			return evt.ReconnectToken, true, true, nil
		case streamelements.KindTip, streamelements.KindModeration:
			c.handleTip(evt.Tip)
		}
	}
}

// handleTip normalizes and publishes one tip, if it currently represents a
// real, completed, moderator-approved donation (see
// docs/provider-integrations/external-donations.md §7/§17) - a pending or
// rejected tip is silently ignored (entirely normal, not logged), never
// published.
func (c *connector) handleTip(tip streamelements.Tip) {
	evt, err := streamelements.NormalizeTip(c.sourceID, tip)
	if err != nil {
		if errors.Is(err, streamelements.ErrTipNotPublishable) {
			return
		}
		c.mgr.logger.Warn("streamelements engagement connector received a malformed tip",
			"sourceId", c.sourceID)
		return
	}

	c.touchEvent()
	// A tip actually arriving clears any outstanding possible-gap display -
	// the connection is demonstrably delivering events again.
	c.setConnected()
	if _, _, err := c.mgr.bus.Publish(evt); err != nil {
		c.mgr.logger.Warn("streamelements engagement connector could not publish a normalized event",
			"sourceId", c.sourceID)
	}
}

var errSourceGone = errors.New("streamelements donation source no longer exists")
