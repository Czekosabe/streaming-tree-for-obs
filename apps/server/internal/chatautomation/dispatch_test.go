package chatautomation

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/streaming-tree/server/internal/outboundchat"
)

// fakeOutboundSender is a trivial, deterministic outboundSender double -
// its own reported QueueDepth is set directly by the test, so the
// automation quota policy in dispatch.go can be verified without any
// real dispatcher goroutines, provider blocking, or clock timing.
type fakeOutboundSender struct {
	mu         sync.Mutex
	queueDepth int
	sendCalls  []outboundchat.SendMessageRequest
	sendResult outboundchat.SendMessageResult
	sendErr    error
}

func (f *fakeOutboundSender) Status(_ context.Context, accountID string) (outboundchat.Snapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return outboundchat.Snapshot{AccountID: accountID, QueueDepth: f.queueDepth, QueueCapacity: outboundchat.MaxQueueDepth}, nil
}

func (f *fakeOutboundSender) Send(_ context.Context, req outboundchat.SendMessageRequest) (outboundchat.SendMessageResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sendCalls = append(f.sendCalls, req)
	if f.sendErr != nil {
		return outboundchat.SendMessageResult{}, f.sendErr
	}
	return f.sendResult, nil
}

func (f *fakeOutboundSender) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sendCalls)
}

func TestDispatcherRejectsScheduledSendAtQuota(t *testing.T) {
	sender := &fakeOutboundSender{queueDepth: automationQueueQuota}
	d := newDispatcher(sender)

	_, err := d.send(context.Background(), outboundchat.SendMessageRequest{
		AccountID: "acct_1", Message: "x", Source: outboundchat.SourceScheduled,
	})
	if !errors.Is(err, outboundchat.ErrQueueFull) {
		t.Errorf("send() at quota = %v, want ErrQueueFull", err)
	}
	if sender.callCount() != 0 {
		t.Errorf("Send() was called %d times, want 0 (rejected before reaching the real dispatcher)", sender.callCount())
	}
}

func TestDispatcherAllowsScheduledSendBelowQuota(t *testing.T) {
	sender := &fakeOutboundSender{queueDepth: automationQueueQuota - 1, sendResult: outboundchat.SendMessageResult{Sent: true}}
	d := newDispatcher(sender)

	result, err := d.send(context.Background(), outboundchat.SendMessageRequest{
		AccountID: "acct_1", Message: "x", Source: outboundchat.SourceScheduled,
	})
	if err != nil || !result.Sent {
		t.Errorf("send() below quota = %+v, %v, want Sent, nil", result, err)
	}
	if sender.callCount() != 1 {
		t.Errorf("Send() was called %d times, want 1", sender.callCount())
	}
}

func TestDispatcherCommandAlsoSubjectToQuota(t *testing.T) {
	sender := &fakeOutboundSender{queueDepth: automationQueueQuota}
	d := newDispatcher(sender)

	_, err := d.send(context.Background(), outboundchat.SendMessageRequest{
		AccountID: "acct_1", Message: "x", Source: outboundchat.SourceCommand,
	})
	if !errors.Is(err, outboundchat.ErrQueueFull) {
		t.Errorf("command send() at quota = %v, want ErrQueueFull (command traffic shares the same automation quota as scheduled)", err)
	}
}

// TestDispatcherManualNeverGoesThroughThisQuota documents (rather than
// exercises in isolation) that manual sends never call dispatcher.send
// at all - the HTTP layer's manual-send handler calls
// outboundchat.Manager.Send directly (see internal/httpapi/outboundchat.go,
// unchanged since Stage 11A), so a full scheduled queue can never block a
// manual message: this package's own quota simply never applies to it.
func TestDispatcherManualNeverGoesThroughThisQuota(t *testing.T) {
	sender := &fakeOutboundSender{queueDepth: outboundchat.MaxQueueDepth} // fully saturated
	d := newDispatcher(sender)

	// Even a hypothetical manual-sourced call through this package's own
	// dispatch wrapper would still be rejected at quota - which is
	// exactly why manual sending in this application never routes
	// through dispatcher.send in the first place (Part 3's own "manual
	// submissions may still be accepted when the scheduled sub-queue is
	// full" requirement is satisfied by manual bypassing this type
	// entirely, not by this type special-casing the Source field).
	_, err := d.send(context.Background(), outboundchat.SendMessageRequest{
		AccountID: "acct_1", Message: "x", Source: outboundchat.SourceManual,
	})
	if !errors.Is(err, outboundchat.ErrQueueFull) {
		t.Errorf("send() = %v, want ErrQueueFull - proving manual sending must never call this method", err)
	}
}

func TestSkipReasonForErr(t *testing.T) {
	cases := []struct {
		err  error
		want SkipReason
	}{
		{outboundchat.ErrUnsupportedProvider, SkipProviderUnsupported},
		{outboundchat.ErrPermissionRequired, SkipPermissionRequired},
		{outboundchat.ErrQueueFull, SkipQueueFull},
		{&outboundchat.RateLimitedError{}, SkipRateLimited},
		{outboundchat.ErrProviderFailure, SkipSendFailed},
		{nil, SkipSendFailed},
	}
	for _, tc := range cases {
		if got := skipReasonForErr(tc.err); got != tc.want {
			t.Errorf("skipReasonForErr(%v) = %q, want %q", tc.err, got, tc.want)
		}
	}
}
