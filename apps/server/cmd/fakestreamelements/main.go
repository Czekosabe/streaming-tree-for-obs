//go:build integration

// Command fakestreamelements is a real local WebSocket server implementing
// enough of the StreamElements Astro protocol to drive the real
// internal/provider/streamelements client and internal/runtime/
// streamelementsengagement connector end to end, script-controllable over
// a plain HTTP JSON control API, used only by
// scripts/verify-streamelements-donations.mjs (via the -tags integration
// cmd/testserver binary's own STREAMING_TREE_TEST_STREAMELEMENTS_WS_BASE_URL
// env var override - see cmd/testserver/main.go).
//
// This exists so the integration regression exercises the REAL Astro
// WebSocket wire protocol (client -> real TCP/WebSocket -> this server ->
// the real connector -> the Event Bus), not a bypass of it - mirrors
// cmd/fakeyoutubegrpc's own reasoning exactly, adapted to a hand-rolled
// bidirectional protocol instead of gRPC: this server both sends
// (welcome/response/message/reconnect) and reads (subscribe) real Astro
// envelopes, reusing internal/provider/streamelements's own wire types so
// there is exactly one definition of the Astro envelope shape in this
// codebase. Node drives this process over ordinary HTTP (the control API
// below); it never needs to speak the Astro WebSocket protocol itself.
//
// Like cmd/testserver, this binary is invisible to a normal `go build
// ./...`/`go vet ./...`/`go test ./...` - it only exists in a binary built
// with `go build -tags integration ./cmd/fakestreamelements`, which the
// verification script does itself, in a temporary directory, for that
// script's own use only.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/streaming-tree/server/internal/provider/streamelements"
)

func main() {
	wsAddr := flag.String("ws-addr", "127.0.0.1:0", "address for the fake Astro WebSocket server")
	controlAddr := flag.String("control-addr", "127.0.0.1:0", "address for the plain HTTP control API")
	flag.Parse()

	fake := newFakeServer()

	wsLis, err := net.Listen("tcp", *wsAddr)
	if err != nil {
		log.Fatalf("listen (ws): %v", err)
	}
	wsServer := &http.Server{Handler: http.HandlerFunc(fake.handleWS)}

	controlLis, err := net.Listen("tcp", *controlAddr)
	if err != nil {
		log.Fatalf("listen (control): %v", err)
	}
	controlServer := &http.Server{Handler: newControlHandler(fake)}

	fmt.Printf("FAKE_STREAMELEMENTS_WS_ADDR=%s\n", wsLis.Addr().String())
	fmt.Printf("FAKE_STREAMELEMENTS_CONTROL_ADDR=%s\n", controlLis.Addr().String())

	go func() {
		if err := wsServer.Serve(wsLis); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("ws server stopped: %v", err)
		}
	}()
	if err := controlServer.Serve(controlLis); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("control server stopped: %v", err)
	}
}

// --- connection registry -------------------------------------------------

// fakeConn is one accepted Astro connection's own observable state - never
// the credential value itself (only whether one was supplied), matching
// this application's own privacy boundary for the real server.
type fakeConn struct {
	id     int
	conn   *websocket.Conn
	cancel context.CancelFunc

	mu               sync.Mutex
	room             string
	hasToken         bool
	subscribedTopics []string
	resumeToken      string // the reconnect_token this connection connected WITH, if any
}

func (fc *fakeConn) snapshot() connectionInfo {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	return connectionInfo{
		ID: fc.id, Room: fc.room, HasToken: fc.hasToken,
		SubscribedTopics: append([]string(nil), fc.subscribedTopics...),
		ResumedWithToken: fc.resumeToken != "",
	}
}

type fakeServer struct {
	mu        sync.Mutex
	nextID    int
	latestID  int
	conns     map[int]*fakeConn
	connected chan struct{}

	subscribeErrorTopic string
	subscribeErrorMsg   string

	// requiredToken, when non-empty, is the only subscribe token this
	// server accepts - any other value is rejected exactly like a
	// subscribeErrorTopic match. Used to prove a credential replacement
	// actually took effect (the connector's next reconnect attempt reads
	// the credential fresh from SecretStore - see internal/runtime/
	// streamelementsengagement/connector.go's own serve()) without this
	// control API ever exposing the token's own value.
	requiredToken string

	validReconnectTokens map[string]bool
}

func newFakeServer() *fakeServer {
	return &fakeServer{
		conns:                make(map[int]*fakeConn),
		connected:            make(chan struct{}, 64),
		validReconnectTokens: make(map[string]bool),
	}
}

func writeEnv(ctx context.Context, conn *websocket.Conn, env streamelements.Envelope) error {
	data, err := json.Marshal(env)
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageText, data)
}

// handleWS implements the Astro connection lifecycle: an invalid
// reconnect_token is rejected at the HTTP-upgrade level (never accepted
// then errored over the socket) - the real client's own Connect() treats
// any such failure identically (a transient connect error, falling back to
// an ordinary fresh connection on its next attempt - see
// docs/provider-integrations/external-donations.md §31), so this is
// sufficient to exercise that fallback without needing a second rejection
// shape.
func (f *fakeServer) handleWS(w http.ResponseWriter, r *http.Request) {
	resumeToken := r.URL.Query().Get("reconnect_token")
	if resumeToken != "" {
		f.mu.Lock()
		ok := f.validReconnectTokens[resumeToken]
		f.mu.Unlock()
		if !ok {
			http.Error(w, "invalid reconnect_token", http.StatusUnauthorized)
			return
		}
	}

	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	fc := &fakeConn{conn: conn, cancel: cancel}

	f.mu.Lock()
	f.nextID++
	fc.id = f.nextID
	f.conns[fc.id] = fc
	f.latestID = fc.id
	f.mu.Unlock()

	defer func() {
		f.mu.Lock()
		delete(f.conns, fc.id)
		f.mu.Unlock()
		cancel()
		_ = conn.CloseNow()
	}()

	if err := writeEnv(ctx, conn, streamelements.Envelope{Type: streamelements.MessageTypeWelcome}); err != nil {
		return
	}

	if resumeToken == "" {
		for i := 0; i < 2; i++ {
			readCtx, readCancel := context.WithTimeout(ctx, 10*time.Second)
			_, data, err := conn.Read(readCtx)
			readCancel()
			if err != nil {
				return
			}
			var req streamelements.Envelope
			if err := json.Unmarshal(data, &req); err != nil {
				return
			}
			var sub streamelements.SubscribeRequest
			_ = json.Unmarshal(req.Data, &sub)

			fc.mu.Lock()
			fc.room = sub.Room
			fc.hasToken = sub.Token != ""
			fc.mu.Unlock()

			f.mu.Lock()
			errTopic, errMsg := f.subscribeErrorTopic, f.subscribeErrorMsg
			requiredToken := f.requiredToken
			f.mu.Unlock()

			if errTopic != "" && errTopic == sub.Topic {
				_ = writeEnv(ctx, conn, streamelements.Envelope{Type: streamelements.MessageTypeResponse, Nonce: req.Nonce, Error: errMsg})
				return
			}
			if requiredToken != "" && sub.Token != requiredToken {
				_ = writeEnv(ctx, conn, streamelements.Envelope{Type: streamelements.MessageTypeResponse, Nonce: req.Nonce, Error: "token mismatch"})
				return
			}

			fc.mu.Lock()
			fc.subscribedTopics = append(fc.subscribedTopics, sub.Topic)
			fc.mu.Unlock()

			if err := writeEnv(ctx, conn, streamelements.Envelope{Type: streamelements.MessageTypeResponse, Nonce: req.Nonce}); err != nil {
				return
			}
		}
	} else {
		// Astro's documented graceful-reconnect protocol restores every
		// subscription automatically - the fake mirrors that by simply
		// carrying the last known topic set forward, without requiring
		// (or accepting) a fresh subscribe request.
		fc.mu.Lock()
		fc.resumeToken = resumeToken
		fc.subscribedTopics = []string{streamelements.TopicChannelTips, streamelements.TopicChannelTipsModeration}
		fc.mu.Unlock()
	}

	f.mu.Lock()
	f.latestID = fc.id
	f.mu.Unlock()
	select {
	case f.connected <- struct{}{}:
	default:
	}

	// A hijacked HTTP connection's own r.Context() is never cancelled by
	// the standard library just because the peer closes its side (the
	// server stopped owning/tracking the raw connection the moment
	// websocket.Accept hijacked it) - so waiting on ctx.Done() alone would
	// never notice an organic client-initiated close (e.g. the real
	// connector's own Stream.Close() on Disable/backend shutdown). Reading
	// (and discarding) in a loop detects that the normal way a server
	// would: the read itself fails once the peer is gone. This same loop
	// also honors /control/disconnect's explicit fc.cancel(), since
	// coder/websocket's own Read(ctx) already closes the connection when
	// ctx is cancelled.
	for {
		if _, _, err := conn.Read(ctx); err != nil {
			return
		}
	}
}

// --- control plane (plain HTTP, driven by scripts/verify-streamelements-donations.mjs) ---

type connectionInfo struct {
	ID               int      `json:"id"`
	Room             string   `json:"room"`
	HasToken         bool     `json:"hasToken"`
	SubscribedTopics []string `json:"subscribedTopics"`
	ResumedWithToken bool     `json:"resumedWithToken"`
}

func (f *fakeServer) resolveConn(idParam string) (*fakeConn, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := f.latestID
	if idParam != "" && idParam != "latest" {
		parsed, err := strconv.Atoi(idParam)
		if err != nil {
			return nil, false
		}
		id = parsed
	}
	fc, ok := f.conns[id]
	return fc, ok
}

func newControlHandler(f *fakeServer) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/control/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/control/wait-connection", func(w http.ResponseWriter, r *http.Request) {
		timeout := 5 * time.Second
		select {
		case <-f.connected:
			w.WriteHeader(http.StatusOK)
		case <-time.After(timeout):
			w.WriteHeader(http.StatusRequestTimeout)
		}
	})

	mux.HandleFunc("/control/connections", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		items := make([]connectionInfo, 0, len(f.conns))
		for _, fc := range f.conns {
			items = append(items, fc.snapshot())
		}
		latestID := f.latestID
		f.mu.Unlock()
		writeJSON(w, map[string]any{"items": items, "latestId": latestID})
	})

	mux.HandleFunc("/control/require-token", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		f.requiredToken = body.Token
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("/control/subscribe-error", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Topic   string `json:"topic"`
			Message string `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		f.subscribeErrorTopic = body.Topic
		f.subscribeErrorMsg = body.Message
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("/control/push-tip", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			ConnectionID string          `json:"connectionId"`
			Topic        string          `json:"topic"`
			Tip          json.RawMessage `json:"tip"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		fc, ok := f.resolveConn(body.ConnectionID)
		if !ok {
			http.Error(w, "unknown connection", http.StatusNotFound)
			return
		}
		env := streamelements.Envelope{Type: streamelements.MessageTypeMessage, Topic: body.Topic, Data: body.Tip}
		if err := writeEnv(r.Context(), fc.conn, env); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("/control/push-reconnect", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			ConnectionID string `json:"connectionId"`
			Token        string `json:"token"`
			// MarkValid defaults to true (via markValidOrDefault below) -
			// pass false to send a reconnect envelope advertising a token
			// this server will then reject, exercising the documented
			// fallback-to-fresh-connect path (docs/provider-integrations/
			// external-donations.md §31) without a real expiry race.
			MarkValid *bool `json:"markValid"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		fc, ok := f.resolveConn(body.ConnectionID)
		if !ok {
			http.Error(w, "unknown connection", http.StatusNotFound)
			return
		}
		if body.MarkValid == nil || *body.MarkValid {
			f.mu.Lock()
			f.validReconnectTokens[body.Token] = true
			f.mu.Unlock()
		}
		data, _ := json.Marshal(streamelements.ReconnectData{ReconnectToken: body.Token})
		env := streamelements.Envelope{Type: streamelements.MessageTypeReconnect, Data: data}
		if err := writeEnv(r.Context(), fc.conn, env); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("/control/invalidate-reconnect-token", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		delete(f.validReconnectTokens, body.Token)
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("/control/disconnect", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			ConnectionID string `json:"connectionId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		fc, ok := f.resolveConn(body.ConnectionID)
		if !ok {
			http.Error(w, "unknown connection", http.StatusNotFound)
			return
		}
		fc.cancel()
		_ = fc.conn.CloseNow()
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("/control/malformed", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			ConnectionID string `json:"connectionId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		fc, ok := f.resolveConn(body.ConnectionID)
		if !ok {
			http.Error(w, "unknown connection", http.StatusNotFound)
			return
		}
		if err := fc.conn.Write(r.Context(), websocket.MessageText, []byte("{not valid json")); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("/control/oversized", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			ConnectionID string `json:"connectionId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		fc, ok := f.resolveConn(body.ConnectionID)
		if !ok {
			http.Error(w, "unknown connection", http.StatusNotFound)
			return
		}
		// One byte over the real client's own 1 MiB maxFrameBytes - large
		// enough to trigger either the coder/websocket library's own read
		// limit or this application's len() check, never a claim about a
		// documented Astro message size.
		oversized := make([]byte, (1<<20)+1)
		for i := range oversized {
			oversized[i] = ' '
		}
		if err := fc.conn.Write(r.Context(), websocket.MessageText, oversized); err != nil {
			// The write itself failing (e.g. the peer already closed
			// after seeing an oversized frame) is an acceptable outcome
			// too - not every transport will accept writing past the
			// peer's own read limit.
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("/control/reset", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		f.mu.Lock()
		f.subscribeErrorTopic = ""
		f.subscribeErrorMsg = ""
		f.requiredToken = ""
		f.validReconnectTokens = make(map[string]bool)
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})

	return mux
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
