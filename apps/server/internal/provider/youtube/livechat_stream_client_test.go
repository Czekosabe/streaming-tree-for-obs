package youtube

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/streaming-tree/server/internal/provider/youtube/streamlistpb"
)

// fakeStreamListService is a minimal local gRPC server for this package's
// own client-level tests - dial/request/metadata/error-mapping behavior,
// as opposed to internal/runtime/youtubeengagement's own, more elaborate
// fake (which exercises the full connector state machine on top of this
// same client).
type fakeStreamListService struct {
	streamlistpb.UnimplementedV3DataLiveChatMessageServiceServer

	gotRequest *streamlistpb.LiveChatMessageListRequest
	gotAuth    string

	sendErr  error
	response *streamlistpb.LiveChatMessageListResponse
}

func (f *fakeStreamListService) StreamList(req *streamlistpb.LiveChatMessageListRequest, stream streamlistpb.V3DataLiveChatMessageService_StreamListServer) error {
	f.gotRequest = req
	if md, ok := metadata.FromIncomingContext(stream.Context()); ok {
		if vals := md.Get("authorization"); len(vals) > 0 {
			f.gotAuth = vals[0]
		}
	}
	if f.sendErr != nil {
		return f.sendErr
	}
	if err := stream.Send(f.response); err != nil {
		return err
	}
	<-stream.Context().Done()
	return stream.Context().Err()
}

func newTestStreamClient(t *testing.T, fake *fakeStreamListService) *Client {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	srv := grpc.NewServer()
	streamlistpb.RegisterV3DataLiveChatMessageServiceServer(srv, fake)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	return New(Options{GRPCTarget: lis.Addr().String(), GRPCTransportCredentials: insecure.NewCredentials()})
}

func TestOpenLiveChatStreamRejectsEmptyLiveChatID(t *testing.T) {
	client := New(Options{})
	if _, err := client.OpenLiveChatStream(context.Background(), "", "", "at"); err == nil {
		t.Fatal("expected error for empty liveChatId")
	}
}

func TestOpenLiveChatStreamSendsCorrectRequestAndAuth(t *testing.T) {
	fake := &fakeStreamListService{response: &streamlistpb.LiveChatMessageListResponse{NextPageToken: proto.String("tok")}}
	client := newTestStreamClient(t, fake)

	stream, err := client.OpenLiveChatStream(context.Background(), "chat_1", "prev_token", "access-tok")
	if err != nil {
		t.Fatalf("OpenLiveChatStream() error = %v", err)
	}
	defer stream.Close()

	if _, err := stream.Recv(); err != nil {
		t.Fatalf("Recv() error = %v", err)
	}

	if fake.gotRequest.GetLiveChatId() != "chat_1" {
		t.Fatalf("expected liveChatId=chat_1, got %q", fake.gotRequest.GetLiveChatId())
	}
	if fake.gotRequest.GetPageToken() != "prev_token" {
		t.Fatalf("expected pageToken=prev_token, got %q", fake.gotRequest.GetPageToken())
	}
	wantParts := []string{"id", "snippet", "authorDetails"}
	gotParts := fake.gotRequest.GetPart()
	if len(gotParts) != len(wantParts) {
		t.Fatalf("expected part=%v, got %v", wantParts, gotParts)
	}
	for i := range wantParts {
		if gotParts[i] != wantParts[i] {
			t.Fatalf("expected part=%v, got %v", wantParts, gotParts)
		}
	}
	if fake.gotRequest.MaxResults != nil {
		t.Fatalf("expected max_results to never be set (unused in the streaming RPC), got %v", fake.gotRequest.GetMaxResults())
	}
	if fake.gotAuth != "Bearer access-tok" {
		t.Fatalf("expected authorization=Bearer access-tok, got %q", fake.gotAuth)
	}
}

func TestOpenLiveChatStreamOmitsPageTokenWhenEmpty(t *testing.T) {
	fake := &fakeStreamListService{response: &streamlistpb.LiveChatMessageListResponse{NextPageToken: proto.String("tok")}}
	client := newTestStreamClient(t, fake)

	stream, err := client.OpenLiveChatStream(context.Background(), "chat_1", "", "at")
	if err != nil {
		t.Fatalf("OpenLiveChatStream() error = %v", err)
	}
	defer stream.Close()
	if _, err := stream.Recv(); err != nil {
		t.Fatalf("Recv() error = %v", err)
	}
	if fake.gotRequest.PageToken != nil {
		t.Fatalf("expected no page_token field to be set for a fresh stream, got %q", fake.gotRequest.GetPageToken())
	}
}

func TestLiveChatStreamRecvConvertsAResponse(t *testing.T) {
	fake := &fakeStreamListService{response: &streamlistpb.LiveChatMessageListResponse{
		NextPageToken: proto.String("next_tok"),
		Items: []*streamlistpb.LiveChatMessage{
			{
				Id: proto.String("msg_1"),
				Snippet: &streamlistpb.LiveChatMessageSnippet{
					Type:            streamlistpb.LiveChatMessageSnippet_TypeWrapper_TEXT_MESSAGE_EVENT.Enum(),
					PublishedAt:     proto.String("2026-08-12T06:00:00Z"),
					AuthorChannelId: proto.String("UC_1"),
					DisplayedContent: &streamlistpb.LiveChatMessageSnippet_TextMessageDetails{
						TextMessageDetails: &streamlistpb.LiveChatTextMessageDetails{MessageText: proto.String("hi")},
					},
				},
				AuthorDetails: &streamlistpb.LiveChatMessageAuthorDetails{ChannelId: proto.String("UC_1"), DisplayName: proto.String("Viewer")},
			},
		},
	}}
	client := newTestStreamClient(t, fake)
	stream, err := client.OpenLiveChatStream(context.Background(), "chat_1", "", "at")
	if err != nil {
		t.Fatalf("OpenLiveChatStream() error = %v", err)
	}
	defer stream.Close()

	page, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv() error = %v", err)
	}
	if page.NextPageToken != "next_tok" {
		t.Fatalf("expected NextPageToken=next_tok, got %q", page.NextPageToken)
	}
	if len(page.Messages) != 1 || page.Messages[0].ID != "msg_1" || page.Messages[0].Type != "textMessageEvent" {
		t.Fatalf("expected one textMessageEvent with id msg_1, got %+v", page.Messages)
	}
}

func TestLiveChatStreamRecvDetectsOfflineAt(t *testing.T) {
	fake := &fakeStreamListService{response: &streamlistpb.LiveChatMessageListResponse{
		NextPageToken: proto.String("tok"), OfflineAt: proto.String("2026-08-12T06:30:00Z"),
	}}
	client := newTestStreamClient(t, fake)
	stream, err := client.OpenLiveChatStream(context.Background(), "chat_1", "", "at")
	if err != nil {
		t.Fatalf("OpenLiveChatStream() error = %v", err)
	}
	defer stream.Close()
	page, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv() error = %v", err)
	}
	if !page.Ended {
		t.Fatal("expected Ended=true when offline_at is present")
	}
}

func TestLiveChatStreamRecvMapsGRPCErrorCodes(t *testing.T) {
	cases := []struct {
		name    string
		code    codes.Code
		wantErr error
	}{
		{"PermissionDenied", codes.PermissionDenied, ErrForbidden},
		{"InvalidArgument", codes.InvalidArgument, ErrLiveChatNotFound},
		{"Unauthenticated", codes.Unauthenticated, ErrUnauthorized},
		{"ResourceExhausted", codes.ResourceExhausted, ErrRateLimited},
		{"Unavailable", codes.Unavailable, ErrUnavailable},
		{"DeadlineExceeded", codes.DeadlineExceeded, ErrUnavailable},
		{"Unknown/undocumented", codes.Unknown, ErrUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeStreamListService{sendErr: status.Error(tc.code, "simulated")}
			client := newTestStreamClient(t, fake)
			stream, err := client.OpenLiveChatStream(context.Background(), "chat_1", "", "at")
			if err != nil {
				// PermissionDenied/InvalidArgument/etc can surface either
				// from the initial StreamList call or the first Recv,
				// depending on gRPC's own header-vs-message timing - both
				// are valid, so check whichever one carried the error.
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected %v, got %v", tc.wantErr, err)
				}
				return
			}
			defer stream.Close()
			_, err = stream.Recv()
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestLiveChatStreamCloseCancelsContext(t *testing.T) {
	fake := &fakeStreamListService{response: &streamlistpb.LiveChatMessageListResponse{NextPageToken: proto.String("tok")}}
	client := newTestStreamClient(t, fake)
	stream, err := client.OpenLiveChatStream(context.Background(), "chat_1", "", "at")
	if err != nil {
		t.Fatalf("OpenLiveChatStream() error = %v", err)
	}
	if _, err := stream.Recv(); err != nil {
		t.Fatalf("Recv() error = %v", err)
	}
	stream.Close()

	done := make(chan struct{})
	go func() {
		_, _ = stream.Recv()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Recv() did not return promptly after Close()")
	}
}
