package youtubeengagement

import (
	"net"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/streaming-tree/server/internal/provider/youtube/streamlistpb"
)

// scriptedResponse is one queued streamList server response - either a
// real message-list response, or a gRPC status error to send instead
// (simulating PERMISSION_DENIED/INVALID_ARGUMENT/UNAVAILABLE/etc).
type scriptedResponse struct {
	resp *streamlistpb.LiveChatMessageListResponse
	err  error
}

// fakeStreamListServer is a real local gRPC server implementing
// V3DataLiveChatMessageServiceServer - this package's own tests dial it
// exactly the way the production connector dials the real
// youtube.googleapis.com:443, so these tests exercise the real gRPC
// transport (client -> HTTP/2 -> server -> Recv loop) rather than bypassing
// it by calling Go functions directly. See scripts/verify-youtube-
// engagement.mjs / cmd/testserver for the equivalent Node-integration-level
// fake used for the full-stack regression.
type fakeStreamListServer struct {
	streamlistpb.UnimplementedV3DataLiveChatMessageServiceServer

	addr       string
	grpcServer *grpc.Server

	mu           sync.Mutex
	scripts      map[string][]scriptedResponse // keyed by liveChatId; consumed in order, last entry repeats
	lastRequest  *streamlistpb.LiveChatMessageListRequest
	lastAuth     string
	requestCount int
}

func newFakeStreamListServer(t *testing.T) *fakeStreamListServer {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	f := &fakeStreamListServer{addr: lis.Addr().String(), scripts: make(map[string][]scriptedResponse)}
	f.grpcServer = grpc.NewServer()
	streamlistpb.RegisterV3DataLiveChatMessageServiceServer(f.grpcServer, f)
	go func() { _ = f.grpcServer.Serve(lis) }()
	t.Cleanup(f.grpcServer.Stop)
	return f
}

// setScript queues the exact sequence of responses/errors StreamList will
// send for one liveChatId, one per Recv() the client makes, consumed in
// order; the last entry repeats once exhausted (so a test that doesn't
// care about further calls doesn't have to script every single one).
func (f *fakeStreamListServer) setScript(liveChatID string, entries ...scriptedResponse) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.scripts[liveChatID] = entries
}

func (f *fakeStreamListServer) lastRequestSnapshot() (*streamlistpb.LiveChatMessageListRequest, string, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastRequest, f.lastAuth, f.requestCount
}

func (f *fakeStreamListServer) StreamList(req *streamlistpb.LiveChatMessageListRequest, stream streamlistpb.V3DataLiveChatMessageService_StreamListServer) error {
	f.mu.Lock()
	f.lastRequest = req
	f.requestCount++
	if md, ok := metadata.FromIncomingContext(stream.Context()); ok {
		if vals := md.Get("authorization"); len(vals) > 0 {
			f.lastAuth = vals[0]
		}
	}
	entries := f.scripts[req.GetLiveChatId()]
	f.mu.Unlock()

	if len(entries) == 0 {
		return status.Error(codes.Unknown, "test error: no script configured for this liveChatId")
	}

	for i := 0; ; i++ {
		idx := i
		if idx >= len(entries) {
			idx = len(entries) - 1
		}
		entry := entries[idx]
		if entry.err != nil {
			return entry.err
		}
		if err := stream.Send(entry.resp); err != nil {
			return err
		}
		if idx == len(entries)-1 {
			// Nothing more scripted beyond the last real response -
			// block until the test context is cancelled (the client
			// closes the stream), simulating a real long-lived
			// connection that simply has nothing new to say yet,
			// exactly like the production server between messages.
			<-stream.Context().Done()
			return stream.Context().Err()
		}
	}
}

// grpcOptions returns the youtube.Options fields a test needs to point a
// *youtube.Client's gRPC transport at f instead of the real production
// host - the same test-only override mechanism
// STREAMING_TREE_TEST_YOUTUBE_*_BASE_URL already uses for REST.
func (f *fakeStreamListServer) grpcOptions() (target string, creds credentials.TransportCredentials) {
	return f.addr, insecure.NewCredentials()
}
