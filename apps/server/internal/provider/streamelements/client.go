package streamelements

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/coder/websocket"
)

// DefaultWSURL is the production Astro WebSocket endpoint, fixed in code -
// see docs/provider-integrations/external-donations.md §5. Never a
// user-editable normal setting; only a `-tags integration` test binary
// may override it (see Options.WSBaseURL and cmd/testserver/main.go's own
// STREAMING_TREE_TEST_STREAMELEMENTS_WS_BASE_URL).
const DefaultWSURL = "wss://astro.streamelements.com/"

// maxFrameBytes bounds every Astro WebSocket frame this client reads - a
// real tip payload is a few hundred bytes; this is a conservative ceiling
// against a malformed/hostile oversized frame, never a documented Astro
// message size.
const maxFrameBytes = 1 << 20 // 1 MiB

// connectTimeout bounds dialing and waiting for `welcome` plus each
// subscribe round trip.
const connectTimeout = 10 * time.Second

// Client is this application's typed client for the StreamElements Astro
// WebSocket API.
type Client struct {
	wsURL string
}

// Options constructs a Client. WSBaseURL is a test-only override (a local
// fake Astro server address); production code leaves it zero so DefaultWSURL
// is used.
type Options struct {
	WSBaseURL string
}

// New builds a Client.
func New(opts Options) *Client {
	wsURL := opts.WSBaseURL
	if wsURL == "" {
		wsURL = DefaultWSURL
	}
	return &Client{wsURL: wsURL}
}

// Stream is one open, authenticated, subscribed Astro connection.
type Stream struct {
	conn *websocket.Conn
}

// Kind discriminates what ReceivedEvent.Recv returned.
type Kind int

const (
	// KindTip: ReceivedEvent.Tip is a channel.tips message.
	KindTip Kind = iota
	// KindModeration: ReceivedEvent.Tip is a channel.tips.moderation
	// message - same shape as KindTip, different topic.
	KindModeration
	// KindReconnect: ReceivedEvent.ReconnectToken is the graceful-
	// shutdown reconnect token the server sent.
	KindReconnect
)

// ReceivedEvent is one application-relevant message Stream.Recv returns.
type ReceivedEvent struct {
	Kind           Kind
	Tip            Tip
	ReconnectToken string
}

// Connect dials the Astro WebSocket, waits for `welcome`, and either
// subscribes explicitly (a fresh connect - resumeToken empty) or relies on
// the server's own documented automatic subscription restoration (resuming
// with resumeToken - docs/provider-integrations/external-donations.md §5:
// "the new server verifies the token and restores all subscriptions
// automatically" - this client deliberately never re-subscribes on that
// path). room is the StreamElements channel id (Source.RemoteChannelID);
// token is the operator's own personal JWT, attached only to the subscribe
// request itself, never logged.
func (c *Client) Connect(ctx context.Context, room, token string, withModeration bool, resumeToken string) (*Stream, error) {
	target := c.wsURL
	if resumeToken != "" {
		u, err := url.Parse(c.wsURL)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid websocket url: %s", ErrConnectionClosed, err)
		}
		q := u.Query()
		q.Set("reconnect_token", resumeToken)
		u.RawQuery = q.Encode()
		target = u.String()
	}

	dialCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	conn, _, err := websocket.Dial(dialCtx, target, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: dial: %s", ErrConnectionClosed, err)
	}
	conn.SetReadLimit(maxFrameBytes)
	stream := &Stream{conn: conn}

	welcomeCtx, wcancel := context.WithTimeout(ctx, connectTimeout)
	env, err := readEnvelope(welcomeCtx, conn)
	wcancel()
	if err != nil {
		stream.Close()
		return nil, err
	}
	if env.Type != MessageTypeWelcome {
		stream.Close()
		return nil, fmt.Errorf("%w: expected welcome, got %q", ErrUnexpectedMessageType, env.Type)
	}

	if resumeToken == "" {
		if err := subscribe(ctx, conn, room, token, TopicChannelTips); err != nil {
			stream.Close()
			return nil, err
		}
		if withModeration {
			if err := subscribe(ctx, conn, room, token, TopicChannelTipsModeration); err != nil {
				stream.Close()
				return nil, err
			}
		}
	}

	return stream, nil
}

// subscribe sends one subscribe request and waits (bounded by
// connectTimeout) for its own matching `response`, returning
// ErrSubscribeFailed if the server reports an error. Any `message`
// envelope that happens to arrive before the response is discarded here -
// nothing should be flowing before the corresponding subscription exists,
// but this is defensive, not assumed.
func subscribe(ctx context.Context, conn *websocket.Conn, room, token, topic string) error {
	nonce := newNonce()
	req := Envelope{Type: MessageTypeSubscribe, Nonce: nonce}
	data, err := json.Marshal(SubscribeRequest{Topic: topic, Room: room, Token: token, TokenType: TokenTypeJWT})
	if err != nil {
		return fmt.Errorf("%w: encode subscribe request: %s", ErrMalformedPayload, err)
	}
	req.Data = data
	if err := writeEnvelope(ctx, conn, req); err != nil {
		return err
	}

	deadline := time.Now().Add(connectTimeout)
	for time.Now().Before(deadline) {
		subCtx, cancel := context.WithTimeout(ctx, connectTimeout)
		env, err := readEnvelope(subCtx, conn)
		cancel()
		if err != nil {
			return err
		}
		if env.Type != MessageTypeResponse || env.Nonce != nonce {
			continue
		}
		if env.Error != "" {
			return fmt.Errorf("%w: topic %s: %s", ErrSubscribeFailed, topic, env.Error)
		}
		return nil
	}
	return fmt.Errorf("%w: subscribe to %s timed out waiting for a response", ErrSubscribeFailed, topic)
}

// Recv blocks for the next application-relevant message: a real tip (from
// either topic) or a graceful-reconnect notice. Any other envelope type
// (an unexpected `response`, an unknown/forward-compatible `type`) is a
// bounded diagnostic handled internally - Recv keeps reading rather than
// returning it, and never crashes on one.
func (s *Stream) Recv(ctx context.Context) (ReceivedEvent, error) {
	for {
		env, err := readEnvelope(ctx, s.conn)
		if err != nil {
			return ReceivedEvent{}, err
		}
		switch env.Type {
		case MessageTypeMessage:
			switch env.Topic {
			case TopicChannelTips:
				tip, err := ParseTip(env.Data)
				if err != nil {
					// A single malformed tip is a bounded diagnostic -
					// never crashes the stream; the caller's own logger
					// records it, this loop just moves on.
					continue
				}
				return ReceivedEvent{Kind: KindTip, Tip: tip}, nil
			case TopicChannelTipsModeration:
				tip, err := ParseTip(env.Data)
				if err != nil {
					continue
				}
				return ReceivedEvent{Kind: KindModeration, Tip: tip}, nil
			default:
				continue
			}
		case MessageTypeReconnect:
			var data ReconnectData
			if err := json.Unmarshal(env.Data, &data); err != nil || data.ReconnectToken == "" {
				continue
			}
			return ReceivedEvent{Kind: KindReconnect, ReconnectToken: data.ReconnectToken}, nil
		default:
			// welcome (only expected during Connect)/response/unknown -
			// ignored here, never fatal.
			continue
		}
	}
}

// Close ends the connection immediately (no graceful close handshake -
// mirrors internal/runtime/twitchengagement's own conn.CloseNow() usage).
func (s *Stream) Close() {
	_ = s.conn.CloseNow()
}

func readEnvelope(ctx context.Context, conn *websocket.Conn) (Envelope, error) {
	_, data, err := conn.Read(ctx)
	if err != nil {
		return Envelope{}, fmt.Errorf("%w: %s", ErrConnectionClosed, err)
	}
	if len(data) > maxFrameBytes {
		return Envelope{}, fmt.Errorf("%w: frame exceeds %d bytes", ErrFrameTooLarge, maxFrameBytes)
	}
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return Envelope{}, fmt.Errorf("%w: %s", ErrMalformedPayload, err)
	}
	return env, nil
}

func writeEnvelope(ctx context.Context, conn *websocket.Conn, env Envelope) error {
	data, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("%w: encode envelope: %s", ErrMalformedPayload, err)
	}
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		return fmt.Errorf("%w: %s", ErrConnectionClosed, err)
	}
	return nil
}

// nonceCounter is a process-local, monotonically increasing counter used
// to build a subscribe request's own nonce - never a security value,
// only a correlation id for matching a response to its own request; a
// simple counter is sufficient and avoids pulling in a UUID dependency
// for something that never leaves this process.
var nonceCounter uint64

func newNonce() string {
	nonceCounter++
	return fmt.Sprintf("stel-%d-%d", time.Now().UnixNano(), nonceCounter)
}
