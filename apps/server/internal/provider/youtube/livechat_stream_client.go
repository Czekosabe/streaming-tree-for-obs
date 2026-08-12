package youtube

import (
	"context"
	"errors"
	"fmt"
	"io"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/streaming-tree/server/internal/provider/youtube/streamlistpb"
)

// LiveChatStreamPart is the fixed `part` value this application always
// requests over streamList - the same three parts the superseded REST
// connector requested (LiveChatMessagesPart), never operator- or
// frontend-configurable. See docs/provider-integrations/
// youtube-engagement.md §4b.2.
var LiveChatStreamPart = []string{"id", "snippet", "authorDetails"}

// LiveChatStream is one open streamList server-streaming call. Per
// docs/provider-integrations/youtube-engagement.md §12, this application
// deliberately does not pool or reuse gRPC connections across streams - one
// connector, one enabled account, one broadcast selection, one
// *ClientConn, opened fresh by OpenLiveChatStream and closed by Close.
type LiveChatStream struct {
	conn   *grpc.ClientConn
	cancel context.CancelFunc
	stream grpc.ServerStreamingClient[streamlistpb.LiveChatMessageListResponse]
}

// OpenLiveChatStream opens one streamList server-streaming call: dials a
// fresh gRPC connection to c's configured target (production:
// DefaultGRPCTarget over TLS; test-only: Options.GRPCTarget/
// GRPCTransportCredentials), attaches accessToken as
// "authorization: Bearer <token>" request metadata (the same credential
// account.Service.WithFreshToken already manages, docs/provider-
// integrations/youtube-engagement.md §2/§4b.2), and issues one StreamList
// call. pageToken is empty to start a genuinely fresh stream (baseline
// cutover applies - see internal/runtime/youtubeengagement/connector.go)
// or the previously-captured NextPageToken to resume a still-valid
// continuation (no re-baseline - see the same file).
//
// The returned *LiveChatStream owns both the gRPC connection and a
// context derived from ctx; Close must be called exactly once to release
// both, whether or not an error ever occurred.
func (c *Client) OpenLiveChatStream(ctx context.Context, liveChatID, pageToken, accessToken string) (*LiveChatStream, error) {
	if liveChatID == "" {
		return nil, fmt.Errorf("%w: liveChatId is required", ErrInvalidResponse)
	}

	conn, err := grpc.NewClient(c.grpcTarget, grpc.WithTransportCredentials(c.grpcTransportCreds))
	if err != nil {
		return nil, fmt.Errorf("%w: streamList: dial: %s", ErrUnavailable, err)
	}

	streamCtx, cancel := context.WithCancel(ctx)
	streamCtx = metadata.AppendToOutgoingContext(streamCtx, "authorization", "Bearer "+accessToken)

	req := &streamlistpb.LiveChatMessageListRequest{
		LiveChatId: proto.String(liveChatID),
		Part:       LiveChatStreamPart,
	}
	// max_results is deliberately never set - the vendored proto's own
	// comment on this field states "Not used in the streaming RPC" (see
	// docs/provider-integrations/youtube-engagement.md §4b.1); setting it
	// would imply a behavior this application could not verify.
	if pageToken != "" {
		req.PageToken = proto.String(pageToken)
	}

	client := streamlistpb.NewV3DataLiveChatMessageServiceClient(conn)
	stream, err := client.StreamList(streamCtx, req)
	if err != nil {
		cancel()
		_ = conn.Close()
		return nil, classifyStreamError(err)
	}

	return &LiveChatStream{conn: conn, cancel: cancel, stream: stream}, nil
}

// Recv blocks for the next streamList response and converts it into the
// same LiveChatMessagePage shape the superseded REST client used - see
// livechat_stream_convert.go.
func (s *LiveChatStream) Recv() (LiveChatMessagePage, error) {
	resp, err := s.stream.Recv()
	if err != nil {
		return LiveChatMessagePage{}, classifyStreamError(err)
	}
	return fromStreamResponse(resp), nil
}

// Close cancels this stream's context and releases its gRPC connection.
// Safe to call after an error from Recv/OpenLiveChatStream; not safe to
// call twice.
func (s *LiveChatStream) Close() {
	s.cancel()
	_ = s.conn.Close()
}

// classifyStreamError maps a streamList gRPC failure onto this package's
// existing sentinel errors - the same ones the superseded REST transport
// used - so internal/runtime/youtubeengagement's connector needs no
// separate gRPC-aware error-classification path (see
// classifyPollError in connector.go). Only PERMISSION_DENIED and
// INVALID_ARGUMENT are actually documented for this RPC (docs/provider-
// integrations/youtube-engagement.md §4b.3); every other code here is a
// defensive, explicitly-judgment-called mapping, not a confirmed provider
// guarantee - see that same section for the full reasoning behind each.
func classifyStreamError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, io.EOF) {
		// The server closed the stream with no error status - not
		// documented as a normal occurrence while a broadcast/chat is
		// still active. Treated as a transient loss to reconnect from,
		// same bucket as UNAVAILABLE - never silently treated as
		// "chat ended" (that is only ever signaled by an explicit
		// chatEndedEvent message or offlineAt, both handled by the
		// connector's own message/page inspection, not here).
		return fmt.Errorf("%w: streamList: stream closed", ErrUnavailable)
	}

	st, ok := status.FromError(err)
	if !ok {
		return fmt.Errorf("%w: streamList: %s", ErrUnavailable, err)
	}

	switch st.Code() {
	case codes.Canceled:
		return context.Canceled
	case codes.Unauthenticated:
		// Same retry-once-after-refresh contract as REST's 401 - the
		// caller (connector, via account.Service.WithFreshToken) attempts
		// exactly one refresh and retry.
		return fmt.Errorf("%w: streamList: %s", ErrUnauthorized, st.Message())
	case codes.PermissionDenied:
		// Documented for this RPC. Not something a token refresh fixes -
		// mapped like REST's generic 403 (ErrForbidden), which the
		// connector's classifyPollError already sends to its default,
		// non-retrying StateError case.
		return fmt.Errorf("%w: streamList: %s", ErrForbidden, st.Message())
	case codes.InvalidArgument:
		// Documented for this RPC. No structured reason is documented
		// (unlike REST's errors[].reason), so this is an explicit,
		// recorded judgment call: the most likely real-world cause is a
		// liveChatId that is no longer valid (stale/ended/disabled) -
		// the closest documented REST analogue - so this connector
		// re-resolves the broadcast/live chat rather than treating it as
		// a hard, non-recoverable error.
		return fmt.Errorf("%w: streamList: %s", ErrLiveChatNotFound, st.Message())
	case codes.ResourceExhausted:
		return fmt.Errorf("%w: streamList: %s", ErrRateLimited, st.Message())
	case codes.Unavailable, codes.DeadlineExceeded:
		return fmt.Errorf("%w: streamList: %s", ErrUnavailable, st.Message())
	default:
		// An undocumented code for this RPC - never guessed at beyond
		// "something is wrong with the transport," the same honest
		// default this project's own research discipline requires
		// (docs/provider-integrations/youtube-engagement.md §4b.3).
		return fmt.Errorf("%w: streamList: %s (%s)", ErrUnavailable, st.Message(), st.Code())
	}
}
