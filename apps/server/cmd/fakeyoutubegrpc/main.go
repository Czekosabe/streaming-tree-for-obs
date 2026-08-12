//go:build integration

// Command fakeyoutubegrpc is a real local gRPC server implementing
// youtube.api.v3.V3DataLiveChatMessageService (the streamList RPC),
// script-controllable over a plain HTTP JSON control API, used only by
// scripts/verify-youtube-engagement.mjs (via the -tags integration
// cmd/testserver binary's own STREAMING_TREE_TEST_YOUTUBE_GRPC_TARGET/
// _INSECURE env var overrides - see cmd/testserver/main.go).
//
// This exists so the integration regression exercises the REAL gRPC
// transport (client -> HTTP/2 -> this server -> the real connector -> the
// Event Bus), not a bypass of it - see docs/provider-integrations/
// youtube-engagement.md §4b (Stage 15A transport corrective pass) and its
// governing task's §24-26. Node drives this process over ordinary HTTP
// (the control API below); it never needs to speak gRPC itself.
//
// Like cmd/testserver, this binary is invisible to a normal `go build
// ./...`/`go vet ./...`/`go test ./...` - it only exists in a binary built
// with `go build -tags integration ./cmd/fakeyoutubegrpc`, which the
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
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/streaming-tree/server/internal/provider/youtube/streamlistpb"
)

func main() {
	grpcAddr := flag.String("grpc-addr", "127.0.0.1:0", "address for the fake streamList gRPC server")
	controlAddr := flag.String("control-addr", "127.0.0.1:0", "address for the plain HTTP control API")
	flag.Parse()

	fake := newFakeService()

	grpcLis, err := net.Listen("tcp", *grpcAddr)
	if err != nil {
		log.Fatalf("listen (grpc): %v", err)
	}
	grpcServer := grpc.NewServer(grpc.Creds(insecure.NewCredentials()))
	streamlistpb.RegisterV3DataLiveChatMessageServiceServer(grpcServer, fake)

	controlLis, err := net.Listen("tcp", *controlAddr)
	if err != nil {
		log.Fatalf("listen (control): %v", err)
	}
	controlServer := &http.Server{Handler: newControlHandler(fake)}

	fmt.Printf("FAKE_YOUTUBE_GRPC_ADDR=%s\n", grpcLis.Addr().String())
	fmt.Printf("FAKE_YOUTUBE_CONTROL_ADDR=%s\n", controlLis.Addr().String())

	go func() {
		if err := grpcServer.Serve(grpcLis); err != nil {
			log.Printf("grpc server stopped: %v", err)
		}
	}()
	if err := controlServer.Serve(controlLis); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("control server stopped: %v", err)
	}
}

// --- gRPC data-plane service --------------------------------------------

type scriptEntry struct {
	// "page" or "error".
	Type string `json:"type"`

	// page fields
	Items         []json.RawMessage `json:"items"`
	NextPageToken string            `json:"nextPageToken"`
	OfflineAt     string            `json:"offlineAt"`

	// error fields
	Code    string `json:"code"`
	Message string `json:"message"`
}

// chatState is one liveChatId's append-only scripted feed. entries/cursor
// are shared across every StreamList call for this liveChatId - a
// reconnect (or a script update arriving while a call is already blocked
// waiting) continues from wherever the feed left off, exactly like a real
// server never replays already-delivered content. cond wakes any blocked
// StreamList call when append/disconnect/reset changes state.
type chatState struct {
	mu         sync.Mutex
	cond       *sync.Cond
	entries    []scriptEntry
	cursor     int
	disconnect bool
}

func newChatState() *chatState {
	cs := &chatState{}
	cs.cond = sync.NewCond(&cs.mu)
	return cs
}

func (cs *chatState) append(entries ...scriptEntry) {
	cs.mu.Lock()
	cs.entries = append(cs.entries, entries...)
	cs.mu.Unlock()
	cs.cond.Broadcast()
}

func (cs *chatState) triggerDisconnect() {
	cs.mu.Lock()
	cs.disconnect = true
	cs.mu.Unlock()
	cs.cond.Broadcast()
}

// hasPending reports whether a real scripted entry is already queued and
// unconsumed, without blocking.
func (cs *chatState) hasPending() bool {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.cursor < len(cs.entries)
}

// next blocks until a not-yet-sent entry is available, a disconnect is
// requested, or ctx ends. ok is false when the caller must stop (either
// disconnected, or ctx ended - check ctx.Err() to tell those apart).
func (cs *chatState) next(ctx context.Context) (entry scriptEntry, ok bool, disconnected bool) {
	stopWatch := make(chan struct{})
	defer close(stopWatch)
	go func() {
		select {
		case <-ctx.Done():
			cs.cond.Broadcast() // wake next()'s Wait() below so it notices ctx ended
		case <-stopWatch:
		}
	}()

	cs.mu.Lock()
	defer cs.mu.Unlock()
	for {
		if ctx.Err() != nil {
			return scriptEntry{}, false, false
		}
		if cs.disconnect {
			cs.disconnect = false
			return scriptEntry{}, false, true
		}
		if cs.cursor < len(cs.entries) {
			e := cs.entries[cs.cursor]
			cs.cursor++
			return e, true, false
		}
		cs.cond.Wait()
	}
}

type fakeService struct {
	streamlistpb.UnimplementedV3DataLiveChatMessageServiceServer

	mu           sync.Mutex
	chats        map[string]*chatState
	lastLiveChat string
	lastPart     []string
	lastPageTok  string
	lastAuthSet  bool
	requestCount int
	restListHit  int
}

func newFakeService() *fakeService {
	return &fakeService{chats: make(map[string]*chatState)}
}

func (f *fakeService) chatFor(liveChatID string) *chatState {
	f.mu.Lock()
	defer f.mu.Unlock()
	cs, ok := f.chats[liveChatID]
	if !ok {
		cs = newChatState()
		f.chats[liveChatID] = cs
	}
	return cs
}

func (f *fakeService) StreamList(req *streamlistpb.LiveChatMessageListRequest, stream streamlistpb.V3DataLiveChatMessageService_StreamListServer) error {
	liveChatID := req.GetLiveChatId()

	f.mu.Lock()
	f.lastLiveChat = liveChatID
	f.lastPart = req.GetPart()
	f.lastPageTok = req.GetPageToken()
	f.requestCount++
	f.lastAuthSet = false
	if md, ok := metadata.FromIncomingContext(stream.Context()); ok {
		if vals := md.Get("authorization"); len(vals) > 0 && vals[0] != "" {
			f.lastAuthSet = true
		}
	}
	f.mu.Unlock()

	cs := f.chatFor(liveChatID)

	// A real streamList connection delivers an initial response promptly
	// (recent history, or an empty-but-valid one) rather than staying
	// silent until something new happens - see docs/provider-
	// integrations/youtube-engagement.md §4b.1. If nothing has been
	// explicitly scripted yet for this liveChatId, synthesize that first
	// response instead of blocking forever; every response after this
	// one waits for real scripted content like normal.
	if !cs.hasPending() {
		steady := &streamlistpb.LiveChatMessageListResponse{NextPageToken: proto.String("steady-" + liveChatID)}
		if err := stream.Send(steady); err != nil {
			return err
		}
	}

	for {
		entry, ok, disconnected := cs.next(stream.Context())
		if !ok {
			if disconnected {
				return status.Error(codes.Unavailable, "forced disconnect (test control)")
			}
			return stream.Context().Err()
		}

		if entry.Type == "error" {
			return status.Error(parseCode(entry.Code), entry.Message)
		}

		resp, err := entryToResponse(entry)
		if err != nil {
			return status.Error(codes.Internal, "fake server: bad scripted entry: "+err.Error())
		}
		if err := stream.Send(resp); err != nil {
			return err
		}
	}
}

func parseCode(name string) codes.Code {
	switch name {
	case "PERMISSION_DENIED":
		return codes.PermissionDenied
	case "INVALID_ARGUMENT":
		return codes.InvalidArgument
	case "UNAUTHENTICATED":
		return codes.Unauthenticated
	case "UNAVAILABLE":
		return codes.Unavailable
	case "DEADLINE_EXCEEDED":
		return codes.DeadlineExceeded
	case "RESOURCE_EXHAUSTED":
		return codes.ResourceExhausted
	case "CANCELLED", "CANCELED":
		return codes.Canceled
	default:
		return codes.Unknown
	}
}

// --- control plane (plain HTTP, driven by scripts/verify-youtube-engagement.mjs) ---

func newControlHandler(f *fakeService) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/control/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/control/script", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			LiveChatID string        `json:"liveChatId"`
			Entries    []scriptEntry `json:"entries"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f.chatFor(body.LiveChatID).append(body.Entries...)
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("/control/disconnect", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			LiveChatID string `json:"liveChatId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f.chatFor(body.LiveChatID).triggerDisconnect()
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("/control/last-request", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		// hasAuthorization only ever reports presence, never the token
		// value itself - see this file's own package doc comment and
		// docs/provider-integrations/youtube-engagement.md §25/§32.
		writeJSON(w, map[string]any{
			"liveChatId":       f.lastLiveChat,
			"part":             f.lastPart,
			"pageToken":        f.lastPageTok,
			"hasAuthorization": f.lastAuthSet,
			"requestCount":     f.requestCount,
		})
	})

	mux.HandleFunc("/control/rest-list-hit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			f.mu.Lock()
			f.restListHit++
			f.mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		f.mu.Lock()
		count := f.restListHit
		f.mu.Unlock()
		writeJSON(w, map[string]any{"count": count})
	})

	return mux
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// --- JSON (REST-shaped) -> protobuf conversion --------------------------
// Mirrors the exact wire shapes scripts/verify-youtube-engagement.mjs
// already builds (the same ones apps/server/internal/provider/youtube/
// models.go's REST structs use) so the script's existing item-builder
// helpers barely change - only where they're sent, not their shape.

func entryToResponse(e scriptEntry) (*streamlistpb.LiveChatMessageListResponse, error) {
	items := make([]*streamlistpb.LiveChatMessage, 0, len(e.Items))
	for _, raw := range e.Items {
		msg, err := jsonToMessage(raw)
		if err != nil {
			return nil, err
		}
		items = append(items, msg)
	}
	resp := &streamlistpb.LiveChatMessageListResponse{
		NextPageToken: proto.String(e.NextPageToken),
		Items:         items,
	}
	if e.OfflineAt != "" {
		resp.OfflineAt = proto.String(e.OfflineAt)
	}
	return resp, nil
}

type jsonItem struct {
	ID      string `json:"id"`
	Snippet struct {
		Type               string `json:"type"`
		PublishedAt        string `json:"publishedAt"`
		AuthorChannelID    string `json:"authorChannelId"`
		DisplayMessage     string `json:"displayMessage"`
		TextMessageDetails *struct {
			MessageText string `json:"messageText"`
		} `json:"textMessageDetails"`
		SuperChatDetails *struct {
			AmountMicros        int64  `json:"amountMicros"`
			Currency            string `json:"currency"`
			AmountDisplayString string `json:"amountDisplayString"`
			UserComment         string `json:"userComment"`
			Tier                int32  `json:"tier"`
		} `json:"superChatDetails"`
		SuperStickerDetails *struct {
			AmountMicros         int64  `json:"amountMicros"`
			Currency             string `json:"currency"`
			AmountDisplayString  string `json:"amountDisplayString"`
			Tier                 int32  `json:"tier"`
			SuperStickerMetadata struct {
				StickerID string `json:"stickerId"`
				AltText   string `json:"altText"`
				Language  string `json:"language"`
			} `json:"superStickerMetadata"`
		} `json:"superStickerDetails"`
		NewSponsorDetails *struct {
			MemberLevelName string `json:"memberLevelName"`
			IsUpgrade       bool   `json:"isUpgrade"`
		} `json:"newSponsorDetails"`
	} `json:"snippet"`
	AuthorDetails struct {
		ChannelID       string `json:"channelId"`
		DisplayName     string `json:"displayName"`
		ProfileImageURL string `json:"profileImageUrl"`
		IsVerified      bool   `json:"isVerified"`
		IsChatOwner     bool   `json:"isChatOwner"`
		IsChatSponsor   bool   `json:"isChatSponsor"`
		IsChatModerator bool   `json:"isChatModerator"`
	} `json:"authorDetails"`
}

var typeNameToEnum = map[string]streamlistpb.LiveChatMessageSnippet_TypeWrapper_Type{
	"textMessageEvent":            streamlistpb.LiveChatMessageSnippet_TypeWrapper_TEXT_MESSAGE_EVENT,
	"tombstone":                   streamlistpb.LiveChatMessageSnippet_TypeWrapper_TOMBSTONE,
	"fanFundingEvent":             streamlistpb.LiveChatMessageSnippet_TypeWrapper_FAN_FUNDING_EVENT,
	"chatEndedEvent":              streamlistpb.LiveChatMessageSnippet_TypeWrapper_CHAT_ENDED_EVENT,
	"sponsorOnlyModeStartedEvent": streamlistpb.LiveChatMessageSnippet_TypeWrapper_SPONSOR_ONLY_MODE_STARTED_EVENT,
	"sponsorOnlyModeEndedEvent":   streamlistpb.LiveChatMessageSnippet_TypeWrapper_SPONSOR_ONLY_MODE_ENDED_EVENT,
	"newSponsorEvent":             streamlistpb.LiveChatMessageSnippet_TypeWrapper_NEW_SPONSOR_EVENT,
	"memberMilestoneChatEvent":    streamlistpb.LiveChatMessageSnippet_TypeWrapper_MEMBER_MILESTONE_CHAT_EVENT,
	"membershipGiftingEvent":      streamlistpb.LiveChatMessageSnippet_TypeWrapper_MEMBERSHIP_GIFTING_EVENT,
	"giftMembershipReceivedEvent": streamlistpb.LiveChatMessageSnippet_TypeWrapper_GIFT_MEMBERSHIP_RECEIVED_EVENT,
	"userBannedEvent":             streamlistpb.LiveChatMessageSnippet_TypeWrapper_USER_BANNED_EVENT,
	"superChatEvent":              streamlistpb.LiveChatMessageSnippet_TypeWrapper_SUPER_CHAT_EVENT,
	"superStickerEvent":           streamlistpb.LiveChatMessageSnippet_TypeWrapper_SUPER_STICKER_EVENT,
	"pollEvent":                   streamlistpb.LiveChatMessageSnippet_TypeWrapper_POLL_EVENT,
	"giftEvent":                   streamlistpb.LiveChatMessageSnippet_TypeWrapper_GIFT_EVENT,
}

func jsonToMessage(raw json.RawMessage) (*streamlistpb.LiveChatMessage, error) {
	var item jsonItem
	if err := json.Unmarshal(raw, &item); err != nil {
		return nil, err
	}
	typ, ok := typeNameToEnum[item.Snippet.Type]
	if !ok {
		return nil, fmt.Errorf("unknown snippet.type %q", item.Snippet.Type)
	}

	snippet := &streamlistpb.LiveChatMessageSnippet{
		Type:            typ.Enum(),
		PublishedAt:     proto.String(item.Snippet.PublishedAt),
		AuthorChannelId: proto.String(item.Snippet.AuthorChannelID),
		DisplayMessage:  proto.String(item.Snippet.DisplayMessage),
	}

	switch {
	case item.Snippet.TextMessageDetails != nil:
		snippet.DisplayedContent = &streamlistpb.LiveChatMessageSnippet_TextMessageDetails{
			TextMessageDetails: &streamlistpb.LiveChatTextMessageDetails{MessageText: proto.String(item.Snippet.TextMessageDetails.MessageText)},
		}
	case item.Snippet.SuperChatDetails != nil:
		d := item.Snippet.SuperChatDetails
		snippet.DisplayedContent = &streamlistpb.LiveChatMessageSnippet_SuperChatDetails{
			SuperChatDetails: &streamlistpb.LiveChatSuperChatDetails{
				AmountMicros: proto.Uint64(uint64(d.AmountMicros)), Currency: proto.String(d.Currency),
				AmountDisplayString: proto.String(d.AmountDisplayString), UserComment: proto.String(d.UserComment),
				Tier: proto.Uint32(uint32(d.Tier)),
			},
		}
	case item.Snippet.SuperStickerDetails != nil:
		d := item.Snippet.SuperStickerDetails
		snippet.DisplayedContent = &streamlistpb.LiveChatMessageSnippet_SuperStickerDetails{
			SuperStickerDetails: &streamlistpb.LiveChatSuperStickerDetails{
				AmountMicros: proto.Uint64(uint64(d.AmountMicros)), Currency: proto.String(d.Currency),
				AmountDisplayString: proto.String(d.AmountDisplayString), Tier: proto.Uint32(uint32(d.Tier)),
				SuperStickerMetadata: &streamlistpb.SuperStickerMetadata{
					StickerId: proto.String(d.SuperStickerMetadata.StickerID), AltText: proto.String(d.SuperStickerMetadata.AltText),
					AltTextLanguage: proto.String(d.SuperStickerMetadata.Language),
				},
			},
		}
	case item.Snippet.NewSponsorDetails != nil:
		d := item.Snippet.NewSponsorDetails
		snippet.DisplayedContent = &streamlistpb.LiveChatMessageSnippet_NewSponsorDetails{
			NewSponsorDetails: &streamlistpb.LiveChatNewSponsorDetails{
				MemberLevelName: proto.String(d.MemberLevelName), IsUpgrade: proto.Bool(d.IsUpgrade),
			},
		}
	}

	return &streamlistpb.LiveChatMessage{
		Id:      proto.String(item.ID),
		Snippet: snippet,
		AuthorDetails: &streamlistpb.LiveChatMessageAuthorDetails{
			ChannelId: proto.String(item.AuthorDetails.ChannelID), DisplayName: proto.String(item.AuthorDetails.DisplayName),
			ProfileImageUrl: proto.String(item.AuthorDetails.ProfileImageURL), IsVerified: proto.Bool(item.AuthorDetails.IsVerified),
			IsChatOwner: proto.Bool(item.AuthorDetails.IsChatOwner), IsChatSponsor: proto.Bool(item.AuthorDetails.IsChatSponsor),
			IsChatModerator: proto.Bool(item.AuthorDetails.IsChatModerator),
		},
	}, nil
}
