package streamelements

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// newFakeAstroServer starts a minimal in-process Astro-shaped WebSocket
// server; handle runs once per accepted connection. Used to exercise
// Client/Stream against the real wire protocol rather than calling
// package-internal functions directly.
func newFakeAstroServer(t *testing.T, handle func(t *testing.T, r *http.Request, conn *websocket.Conn)) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.CloseNow()
		handle(t, r, conn)
	}))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

func sendEnvelope(t *testing.T, ctx context.Context, conn *websocket.Conn, env Envelope) {
	t.Helper()
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		t.Fatalf("write envelope: %v", err)
	}
}

func readClientEnvelope(t *testing.T, ctx context.Context, conn *websocket.Conn) Envelope {
	t.Helper()
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read from client: %v", err)
	}
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("unmarshal client envelope: %v", err)
	}
	return env
}

func serveWelcome(t *testing.T, ctx context.Context, conn *websocket.Conn) {
	t.Helper()
	sendEnvelope(t, ctx, conn, Envelope{Type: MessageTypeWelcome})
}

// serveSubscribeSuccess reads exactly n subscribe requests and replies to
// each with a matching, error-free response.
func serveSubscribeSuccess(t *testing.T, ctx context.Context, conn *websocket.Conn, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		req := readClientEnvelope(t, ctx, conn)
		if req.Type != MessageTypeSubscribe {
			t.Fatalf("client message type = %q, want subscribe", req.Type)
		}
		sendEnvelope(t, ctx, conn, Envelope{Type: MessageTypeResponse, Nonce: req.Nonce})
	}
}

func TestConnectWelcomeAndSubscribeSucceeds(t *testing.T) {
	url := newFakeAstroServer(t, func(t *testing.T, r *http.Request, conn *websocket.Conn) {
		ctx := r.Context()
		serveWelcome(t, ctx, conn)
		serveSubscribeSuccess(t, ctx, conn, 1)
		<-ctx.Done()
	})

	c := New(Options{WSBaseURL: url})
	stream, err := c.Connect(context.Background(), "room-1", "jwt-token", false, "")
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer stream.Close()
}

func TestConnectSubscribesToBothTopicsWhenModerationRequested(t *testing.T) {
	seenTopics := make(chan string, 2)
	url := newFakeAstroServer(t, func(t *testing.T, r *http.Request, conn *websocket.Conn) {
		ctx := r.Context()
		serveWelcome(t, ctx, conn)
		for i := 0; i < 2; i++ {
			req := readClientEnvelope(t, ctx, conn)
			var sub SubscribeRequest
			if err := json.Unmarshal(req.Data, &sub); err != nil {
				t.Fatalf("unmarshal subscribe request: %v", err)
			}
			seenTopics <- sub.Topic
			sendEnvelope(t, ctx, conn, Envelope{Type: MessageTypeResponse, Nonce: req.Nonce})
		}
		<-ctx.Done()
	})

	c := New(Options{WSBaseURL: url})
	stream, err := c.Connect(context.Background(), "room-1", "jwt-token", true, "")
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer stream.Close()

	got := map[string]bool{<-seenTopics: true, <-seenTopics: true}
	if !got[TopicChannelTips] || !got[TopicChannelTipsModeration] {
		t.Fatalf("subscribed topics = %v, want both channel.tips and channel.tips.moderation", got)
	}
}

func TestConnectSubscribeErrorReturnsErrSubscribeFailed(t *testing.T) {
	url := newFakeAstroServer(t, func(t *testing.T, r *http.Request, conn *websocket.Conn) {
		ctx := r.Context()
		serveWelcome(t, ctx, conn)
		req := readClientEnvelope(t, ctx, conn)
		sendEnvelope(t, ctx, conn, Envelope{Type: MessageTypeResponse, Nonce: req.Nonce, Error: "invalid token"})
	})

	c := New(Options{WSBaseURL: url})
	_, err := c.Connect(context.Background(), "room-1", "bad-token", false, "")
	if !errors.Is(err, ErrSubscribeFailed) {
		t.Fatalf("Connect() error = %v, want ErrSubscribeFailed", err)
	}
}

func TestConnectUnexpectedFirstMessageIsRejected(t *testing.T) {
	url := newFakeAstroServer(t, func(t *testing.T, r *http.Request, conn *websocket.Conn) {
		sendEnvelope(t, r.Context(), conn, Envelope{Type: MessageTypeMessage})
	})

	c := New(Options{WSBaseURL: url})
	_, err := c.Connect(context.Background(), "room-1", "jwt-token", false, "")
	if !errors.Is(err, ErrUnexpectedMessageType) {
		t.Fatalf("Connect() error = %v, want ErrUnexpectedMessageType", err)
	}
}

func TestConnectDiscardsResponsesWithMismatchedNonceBeforeTheRealOne(t *testing.T) {
	url := newFakeAstroServer(t, func(t *testing.T, r *http.Request, conn *websocket.Conn) {
		ctx := r.Context()
		serveWelcome(t, ctx, conn)
		req := readClientEnvelope(t, ctx, conn)
		sendEnvelope(t, ctx, conn, Envelope{Type: MessageTypeResponse, Nonce: "stale-nonce-from-nowhere"})
		sendEnvelope(t, ctx, conn, Envelope{Type: MessageTypeResponse, Nonce: req.Nonce})
		<-ctx.Done()
	})

	c := New(Options{WSBaseURL: url})
	stream, err := c.Connect(context.Background(), "room-1", "jwt-token", false, "")
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer stream.Close()
}

func TestConnectWithResumeTokenSkipsSubscribeAndUsesQueryParam(t *testing.T) {
	const token = "resume-token-abc"
	url := newFakeAstroServer(t, func(t *testing.T, r *http.Request, conn *websocket.Conn) {
		if got := r.URL.Query().Get("reconnect_token"); got != token {
			t.Errorf("reconnect_token query param = %q, want %q", got, token)
		}
		serveWelcome(t, r.Context(), conn)
		// Deliberately never reads a subscribe request - the official
		// protocol restores subscriptions automatically on this path, and
		// the client must never re-subscribe (docs/provider-integrations/
		// external-donations.md §5).
		<-r.Context().Done()
	})

	c := New(Options{WSBaseURL: url})
	stream, err := c.Connect(context.Background(), "room-1", "jwt-token", false, token)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer stream.Close()
}

func TestRecvReturnsTip(t *testing.T) {
	url := newFakeAstroServer(t, func(t *testing.T, r *http.Request, conn *websocket.Conn) {
		ctx := r.Context()
		serveWelcome(t, ctx, conn)
		serveSubscribeSuccess(t, ctx, conn, 1)
		tipData, _ := json.Marshal(allowedTip())
		sendEnvelope(t, ctx, conn, Envelope{Type: MessageTypeMessage, Topic: TopicChannelTips, Data: tipData})
		<-ctx.Done()
	})

	c := New(Options{WSBaseURL: url})
	stream, err := c.Connect(context.Background(), "room-1", "jwt-token", false, "")
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer stream.Close()

	recvCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	evt, err := stream.Recv(recvCtx)
	if err != nil {
		t.Fatalf("Recv() error = %v", err)
	}
	if evt.Kind != KindTip {
		t.Fatalf("Kind = %v, want KindTip", evt.Kind)
	}
	if evt.Tip.ID != allowedTip().ID {
		t.Fatalf("Tip.ID = %q, want %q", evt.Tip.ID, allowedTip().ID)
	}
}

func TestRecvReturnsModerationTip(t *testing.T) {
	url := newFakeAstroServer(t, func(t *testing.T, r *http.Request, conn *websocket.Conn) {
		ctx := r.Context()
		serveWelcome(t, ctx, conn)
		serveSubscribeSuccess(t, ctx, conn, 2)
		tip := allowedTip()
		tip.Approved = ApprovedPending
		tipData, _ := json.Marshal(tip)
		sendEnvelope(t, ctx, conn, Envelope{Type: MessageTypeMessage, Topic: TopicChannelTipsModeration, Data: tipData})
		<-ctx.Done()
	})

	c := New(Options{WSBaseURL: url})
	stream, err := c.Connect(context.Background(), "room-1", "jwt-token", true, "")
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer stream.Close()

	recvCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	evt, err := stream.Recv(recvCtx)
	if err != nil {
		t.Fatalf("Recv() error = %v", err)
	}
	if evt.Kind != KindModeration {
		t.Fatalf("Kind = %v, want KindModeration", evt.Kind)
	}
}

func TestRecvSkipsUnknownEnvelopesThenReturnsTip(t *testing.T) {
	url := newFakeAstroServer(t, func(t *testing.T, r *http.Request, conn *websocket.Conn) {
		ctx := r.Context()
		serveWelcome(t, ctx, conn)
		serveSubscribeSuccess(t, ctx, conn, 1)
		sendEnvelope(t, ctx, conn, Envelope{Type: MessageTypeResponse})
		sendEnvelope(t, ctx, conn, Envelope{Type: "future_unknown_type"})
		tipData, _ := json.Marshal(allowedTip())
		sendEnvelope(t, ctx, conn, Envelope{Type: MessageTypeMessage, Topic: TopicChannelTips, Data: tipData})
		<-ctx.Done()
	})

	c := New(Options{WSBaseURL: url})
	stream, err := c.Connect(context.Background(), "room-1", "jwt-token", false, "")
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer stream.Close()

	recvCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	evt, err := stream.Recv(recvCtx)
	if err != nil {
		t.Fatalf("Recv() error = %v", err)
	}
	if evt.Kind != KindTip {
		t.Fatalf("Kind = %v, want KindTip", evt.Kind)
	}
}

func TestRecvSkipsUnparseableTipDataThenReturnsNextTip(t *testing.T) {
	url := newFakeAstroServer(t, func(t *testing.T, r *http.Request, conn *websocket.Conn) {
		ctx := r.Context()
		serveWelcome(t, ctx, conn)
		serveSubscribeSuccess(t, ctx, conn, 1)
		sendEnvelope(t, ctx, conn, Envelope{Type: MessageTypeMessage, Topic: TopicChannelTips, Data: json.RawMessage(`{"donation":{}}`)})
		tipData, _ := json.Marshal(allowedTip())
		sendEnvelope(t, ctx, conn, Envelope{Type: MessageTypeMessage, Topic: TopicChannelTips, Data: tipData})
		<-ctx.Done()
	})

	c := New(Options{WSBaseURL: url})
	stream, err := c.Connect(context.Background(), "room-1", "jwt-token", false, "")
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer stream.Close()

	recvCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	evt, err := stream.Recv(recvCtx)
	if err != nil {
		t.Fatalf("Recv() error = %v", err)
	}
	if evt.Kind != KindTip || evt.Tip.ID != allowedTip().ID {
		t.Fatalf("Recv() = %+v, want the second, well-formed tip", evt)
	}
}

func TestRecvReturnsReconnectToken(t *testing.T) {
	url := newFakeAstroServer(t, func(t *testing.T, r *http.Request, conn *websocket.Conn) {
		ctx := r.Context()
		serveWelcome(t, ctx, conn)
		serveSubscribeSuccess(t, ctx, conn, 1)
		data, _ := json.Marshal(ReconnectData{ReconnectToken: "grace-token-xyz"})
		sendEnvelope(t, ctx, conn, Envelope{Type: MessageTypeReconnect, Data: data})
		<-ctx.Done()
	})

	c := New(Options{WSBaseURL: url})
	stream, err := c.Connect(context.Background(), "room-1", "jwt-token", false, "")
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer stream.Close()

	recvCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	evt, err := stream.Recv(recvCtx)
	if err != nil {
		t.Fatalf("Recv() error = %v", err)
	}
	if evt.Kind != KindReconnect || evt.ReconnectToken != "grace-token-xyz" {
		t.Fatalf("Recv() = %+v, want KindReconnect with the server's own token", evt)
	}
}

func TestRecvMalformedTopLevelJSONReturnsError(t *testing.T) {
	url := newFakeAstroServer(t, func(t *testing.T, r *http.Request, conn *websocket.Conn) {
		ctx := r.Context()
		serveWelcome(t, ctx, conn)
		serveSubscribeSuccess(t, ctx, conn, 1)
		if err := conn.Write(ctx, websocket.MessageText, []byte("{not json")); err != nil {
			t.Fatalf("write malformed frame: %v", err)
		}
		<-ctx.Done()
	})

	c := New(Options{WSBaseURL: url})
	stream, err := c.Connect(context.Background(), "room-1", "jwt-token", false, "")
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer stream.Close()

	recvCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := stream.Recv(recvCtx); !errors.Is(err, ErrMalformedPayload) {
		t.Fatalf("Recv() error = %v, want ErrMalformedPayload", err)
	}
}

func TestRecvCancellationReturnsPromptly(t *testing.T) {
	url := newFakeAstroServer(t, func(t *testing.T, r *http.Request, conn *websocket.Conn) {
		ctx := r.Context()
		serveWelcome(t, ctx, conn)
		serveSubscribeSuccess(t, ctx, conn, 1)
		<-ctx.Done()
	})

	c := New(Options{WSBaseURL: url})
	stream, err := c.Connect(context.Background(), "room-1", "jwt-token", false, "")
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer stream.Close()

	recvCtx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err = stream.Recv(recvCtx)
	if err == nil {
		t.Fatal("Recv() error = nil, want a context-deadline error")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Recv() took %v to observe cancellation, want well under connectTimeout", elapsed)
	}
}

func TestStreamCloseEndsTheConnection(t *testing.T) {
	url := newFakeAstroServer(t, func(t *testing.T, r *http.Request, conn *websocket.Conn) {
		ctx := r.Context()
		serveWelcome(t, ctx, conn)
		serveSubscribeSuccess(t, ctx, conn, 1)
		<-ctx.Done()
	})

	c := New(Options{WSBaseURL: url})
	stream, err := c.Connect(context.Background(), "room-1", "jwt-token", false, "")
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	stream.Close()

	recvCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := stream.Recv(recvCtx); err == nil {
		t.Fatal("Recv() after Close() error = nil, want an error")
	}
}
