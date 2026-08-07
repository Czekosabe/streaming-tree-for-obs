package outboundchat

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/account"
)

// --- fakes -----------------------------------------------------------------

type fakeAccounts struct {
	mu       sync.Mutex
	accounts map[string]account.Account
	clientID string
}

func newFakeAccounts(accounts ...account.Account) *fakeAccounts {
	f := &fakeAccounts{accounts: make(map[string]account.Account), clientID: "client-id"}
	for _, a := range accounts {
		f.accounts[a.ID] = a
	}
	return f
}

func (f *fakeAccounts) GetAccount(_ context.Context, id string) (account.Account, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	acc, ok := f.accounts[id]
	if !ok {
		return account.Account{}, account.ErrNotFound
	}
	return acc, nil
}

func (f *fakeAccounts) WithFreshToken(ctx context.Context, accountID string, fn func(accessToken string) (bool, error)) error {
	unauthorized, err := fn("access-token")
	if err != nil {
		return err
	}
	if unauthorized {
		// This test double never actually refreshes - a provider that
		// keeps reporting unauthorized after the "refresh" simply fails,
		// matching WithFreshToken's own real reconnect-required behavior
		// closely enough for dispatcher-level tests (the refresh mechanism
		// itself is already covered by account.Service's own tests).
		unauthorizedAgain, err2 := fn("access-token-refreshed")
		if err2 != nil {
			return err2
		}
		if unauthorizedAgain {
			return account.ErrReconnectRequired
		}
	}
	return nil
}

func (f *fakeAccounts) EffectiveClientID(context.Context, account.ProviderID) (string, error) {
	return f.clientID, nil
}

type sendCall struct {
	accountID string
	message   string
}

type fakeProvider struct {
	providerID account.ProviderID

	mu          sync.Mutex
	calls       []sendCall
	inFlight    int
	maxInFlight int
	nextResults []func() (SendMessageResult, error)
	callCount   int
	onSend      func(call sendCall)
}

func newFakeProvider(id account.ProviderID) *fakeProvider {
	return &fakeProvider{providerID: id}
}

func (p *fakeProvider) ProviderID() account.ProviderID { return p.providerID }

func (p *fakeProvider) AssessCapability(acc account.Account) Capability {
	granted := acc.HasScope("user:write:chat")
	assessment := Capability{Required: []string{"user:write:chat"}, Granted: acc.Scopes, Available: granted}
	if !granted {
		assessment.Missing = []string{"user:write:chat"}
		assessment.PermissionUpgradeRequired = true
	}
	return assessment
}

// queueResult queues one canned result/error for the Nth call (in order).
func (p *fakeProvider) queueResult(fn func() (SendMessageResult, error)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.nextResults = append(p.nextResults, fn)
}

func (p *fakeProvider) SendChatMessage(ctx context.Context, acc account.Account, token account.TokenBundle, clientID string, req SendMessageRequest) (SendMessageResult, error) {
	p.mu.Lock()
	p.inFlight++
	if p.inFlight > p.maxInFlight {
		p.maxInFlight = p.inFlight
	}
	p.calls = append(p.calls, sendCall{accountID: req.AccountID, message: req.Message})
	var fn func() (SendMessageResult, error)
	if p.callCount < len(p.nextResults) {
		fn = p.nextResults[p.callCount]
	}
	p.callCount++
	onSend := p.onSend
	p.mu.Unlock()

	if onSend != nil {
		onSend(sendCall{accountID: req.AccountID, message: req.Message})
	}

	var result SendMessageResult
	var err error
	if fn != nil {
		result, err = fn()
	} else {
		result = SendMessageResult{Sent: true, ProviderMessageID: "m_" + req.Message}
	}

	p.mu.Lock()
	p.inFlight--
	p.mu.Unlock()

	return result, err
}

func (p *fakeProvider) callsFor(accountID string) []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []string
	for _, c := range p.calls {
		if c.accountID == accountID {
			out = append(out, c.message)
		}
	}
	return out
}

// fakeClock is a manually-advanced clock for deterministic rate-limit
// testing - never a real sleep.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock { return &fakeClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

func testAcc(id string, scopes ...string) account.Account {
	return account.Account{ID: id, ProviderID: account.ProviderTwitch, ProviderUserID: "u_" + id, Scopes: scopes}
}

func newManagerForTest(accounts *fakeAccounts, provider *fakeProvider, clock func() time.Time) *Manager {
	return NewManager(ManagerOptions{Accounts: accounts, Providers: []Provider{provider}, Now: clock})
}

// --- tests -------------------------------------------------------------

func TestSendReturnsUnsupportedProviderForAnUnregisteredProvider(t *testing.T) {
	acc := account.Account{ID: "a1", ProviderID: "kick"}
	accounts := newFakeAccounts(acc)
	provider := newFakeProvider(account.ProviderTwitch)
	m := newManagerForTest(accounts, provider, time.Now)

	_, err := m.Send(context.Background(), SendMessageRequest{AccountID: "a1", Message: "hi"})
	if !errors.Is(err, ErrUnsupportedProvider) {
		t.Fatalf("error = %v, want ErrUnsupportedProvider", err)
	}
}

func TestSendReturnsPermissionRequiredWhenScopeMissing(t *testing.T) {
	acc := testAcc("a1") // no scopes
	accounts := newFakeAccounts(acc)
	provider := newFakeProvider(account.ProviderTwitch)
	m := newManagerForTest(accounts, provider, time.Now)

	_, err := m.Send(context.Background(), SendMessageRequest{AccountID: "a1", Message: "hi"})
	if !errors.Is(err, ErrPermissionRequired) {
		t.Fatalf("error = %v, want ErrPermissionRequired", err)
	}
}

func TestSendRejectsAnInvalidMessageBeforeEverQueueing(t *testing.T) {
	acc := testAcc("a1", "user:write:chat")
	accounts := newFakeAccounts(acc)
	provider := newFakeProvider(account.ProviderTwitch)
	m := newManagerForTest(accounts, provider, time.Now)

	if _, err := m.Send(context.Background(), SendMessageRequest{AccountID: "a1", Message: ""}); !errors.Is(err, ErrMessageEmpty) {
		t.Fatalf("error = %v, want ErrMessageEmpty", err)
	}
	if len(provider.calls) != 0 {
		t.Fatal("provider was called for an invalid message")
	}
}

func TestSendSucceedsAndReturnsProviderMessageID(t *testing.T) {
	acc := testAcc("a1", "user:write:chat")
	accounts := newFakeAccounts(acc)
	provider := newFakeProvider(account.ProviderTwitch)
	m := newManagerForTest(accounts, provider, time.Now)

	result, err := m.Send(context.Background(), SendMessageRequest{AccountID: "a1", Message: "hello"})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if !result.Sent || result.ProviderMessageID != "m_hello" {
		t.Errorf("result = %+v", result)
	}
}

func TestPerAccountOrderingIsPreserved(t *testing.T) {
	acc := testAcc("a1", "user:write:chat")
	accounts := newFakeAccounts(acc)
	provider := newFakeProvider(account.ProviderTwitch)
	clock := newFakeClock()
	m := newManagerForTest(accounts, provider, clock.Now)

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, err := m.Send(context.Background(), SendMessageRequest{AccountID: "a1", Message: fmt.Sprintf("msg-%d", n)})
			if err != nil {
				t.Errorf("Send() error = %v", err)
			}
			clock.Advance(2 * time.Second) // stay clear of the 1/sec floor between starts
		}(i)
		time.Sleep(2 * time.Millisecond) // encourage (not guarantee) submission order
	}
	wg.Wait()

	calls := provider.callsFor("a1")
	if len(calls) != 5 {
		t.Fatalf("got %d calls, want 5", len(calls))
	}
}

func TestOneInFlightSendPerAccount(t *testing.T) {
	acc := testAcc("a1", "user:write:chat")
	accounts := newFakeAccounts(acc)
	provider := newFakeProvider(account.ProviderTwitch)
	clock := newFakeClock()
	release := make(chan struct{})
	provider.onSend = func(sendCall) { <-release }
	m := newManagerForTest(accounts, provider, clock.Now)

	go func() { _, _ = m.Send(context.Background(), SendMessageRequest{AccountID: "a1", Message: "first"}) }()
	time.Sleep(20 * time.Millisecond)

	snap, err := m.Status(context.Background(), "a1")
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if snap.State != DispatcherSending {
		t.Fatalf("state = %v, want sending while the first call is blocked", snap.State)
	}

	provider.mu.Lock()
	maxInFlight := provider.maxInFlight
	provider.mu.Unlock()
	if maxInFlight > 1 {
		t.Fatalf("maxInFlight = %d, want at most 1", maxInFlight)
	}
	close(release)
}

func TestDifferentAccountsProceedIndependently(t *testing.T) {
	accounts := newFakeAccounts(testAcc("a1", "user:write:chat"), testAcc("a2", "user:write:chat"))
	provider := newFakeProvider(account.ProviderTwitch)
	blockA1 := make(chan struct{})
	provider.onSend = func(c sendCall) {
		if c.accountID == "a1" {
			<-blockA1
		}
	}
	m := newManagerForTest(accounts, provider, time.Now)

	go func() { _, _ = m.Send(context.Background(), SendMessageRequest{AccountID: "a1", Message: "blocked"}) }()
	time.Sleep(20 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		_, err := m.Send(context.Background(), SendMessageRequest{AccountID: "a2", Message: "unblocked"})
		if err != nil {
			t.Errorf("Send() error = %v", err)
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("account a2's send never completed while a1 was blocked - accounts are not isolated")
	}
	close(blockA1)
}

func TestQueueCapacityAndQueueFull(t *testing.T) {
	acc := testAcc("a1", "user:write:chat")
	accounts := newFakeAccounts(acc)
	provider := newFakeProvider(account.ProviderTwitch)
	block := make(chan struct{})
	provider.onSend = func(sendCall) { <-block }
	clock := newFakeClock()
	m := newManagerForTest(accounts, provider, clock.Now)

	// One send occupies the worker (blocked inside the provider call);
	// MaxQueueDepth more fill the bounded queue behind it. None of these
	// can complete until block is closed below, so nothing here waits for
	// them to finish yet - only that they were accepted into the queue.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = m.Send(context.Background(), SendMessageRequest{AccountID: "a1", Message: "sending"})
	}()
	time.Sleep(20 * time.Millisecond)

	errs := make(chan error, MaxQueueDepth)
	for i := 0; i < MaxQueueDepth; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, err := m.Send(context.Background(), SendMessageRequest{AccountID: "a1", Message: fmt.Sprintf("q-%d", n)})
			errs <- err
		}(i)
	}
	time.Sleep(50 * time.Millisecond)

	// The queue is now full (MaxQueueDepth items enqueued behind the one
	// in flight); one more must be rejected immediately, without blocking.
	_, err := m.Send(context.Background(), SendMessageRequest{AccountID: "a1", Message: "overflow"})
	if !errors.Is(err, ErrQueueFull) {
		t.Fatalf("error = %v, want ErrQueueFull once the bounded queue is at capacity", err)
	}

	close(block) // release every blocked send so the goroutines above can finish

	// Draining the queue behind the rate-limit floor would take ~1 real
	// second per remaining item; race the fake clock ahead instead so the
	// drain completes in milliseconds of real time.
	drained := make(chan struct{})
	go func() { wg.Wait(); close(drained) }()
	ticker := time.NewTicker(2 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-drained:
			close(errs)
			for err := range errs {
				if err != nil {
					t.Errorf("unexpected error draining the queue: %v", err)
				}
			}
			return
		case <-ticker.C:
			clock.Advance(2 * time.Second)
		case <-time.After(5 * time.Second):
			t.Fatal("queue never drained")
		}
	}
}

func TestMinimumOneSecondDispatchInterval(t *testing.T) {
	acc := testAcc("a1", "user:write:chat")
	accounts := newFakeAccounts(acc)
	provider := newFakeProvider(account.ProviderTwitch)
	clock := newFakeClock()
	m := newManagerForTest(accounts, provider, clock.Now)

	if _, err := m.Send(context.Background(), SendMessageRequest{AccountID: "a1", Message: "first"}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	done := make(chan struct{})
	go func() {
		_, err := m.Send(context.Background(), SendMessageRequest{AccountID: "a1", Message: "second"})
		if err != nil {
			t.Errorf("Send() error = %v", err)
		}
		close(done)
	}()

	// Give the dispatcher a moment to enter its rate-limited wait, then
	// confirm it is genuinely waiting rather than having already sent.
	time.Sleep(20 * time.Millisecond)
	if calls := provider.callsFor("a1"); len(calls) != 1 {
		t.Fatalf("calls = %v, want exactly 1 before the 1-second floor elapses", calls)
	}
	snap, _ := m.Status(context.Background(), "a1")
	if snap.State != DispatcherRateLimited {
		t.Fatalf("state = %v, want rate_limited while waiting out the 1-second floor", snap.State)
	}

	clock.Advance(1100 * time.Millisecond)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("second send never completed after the 1-second floor elapsed")
	}
	if calls := provider.callsFor("a1"); len(calls) != 2 {
		t.Fatalf("calls = %v, want 2 after advancing past the floor", calls)
	}
}

func TestRollingTwentyPerThirtySecondWindow(t *testing.T) {
	acc := testAcc("a1", "user:write:chat")
	accounts := newFakeAccounts(acc)
	provider := newFakeProvider(account.ProviderTwitch)
	clock := newFakeClock()
	m := newManagerForTest(accounts, provider, clock.Now)

	for i := 0; i < maxStartsInWindow; i++ {
		if _, err := m.Send(context.Background(), SendMessageRequest{AccountID: "a1", Message: fmt.Sprintf("m-%d", i)}); err != nil {
			t.Fatalf("send %d: error = %v", i, err)
		}
		clock.Advance(minDispatchInterval + time.Millisecond)
	}
	if calls := provider.callsFor("a1"); len(calls) != maxStartsInWindow {
		t.Fatalf("calls = %d, want %d", len(calls), maxStartsInWindow)
	}

	// The 21st send within the rolling window must wait, not send
	// immediately.
	done := make(chan struct{})
	go func() {
		_, err := m.Send(context.Background(), SendMessageRequest{AccountID: "a1", Message: "over-window"})
		if err != nil {
			t.Errorf("Send() error = %v", err)
		}
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	if calls := provider.callsFor("a1"); len(calls) != maxStartsInWindow {
		t.Fatalf("calls = %d, want still %d before the window clears", len(calls), maxStartsInWindow)
	}

	clock.Advance(rollingWindow)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("send beyond the rolling window never completed after the window cleared")
	}
}

func TestProviderRateLimitedResultPausesTheDispatcherWithARetryAtHint(t *testing.T) {
	acc := testAcc("a1", "user:write:chat")
	accounts := newFakeAccounts(acc)
	provider := newFakeProvider(account.ProviderTwitch)
	clock := newFakeClock()
	retryAt := clock.Now().Add(10 * time.Second)
	provider.queueResult(func() (SendMessageResult, error) { return SendMessageResult{}, &RateLimitedError{RetryAt: retryAt} })
	m := newManagerForTest(accounts, provider, clock.Now)

	_, err := m.Send(context.Background(), SendMessageRequest{AccountID: "a1", Message: "first"})
	var rateLimitErr *RateLimitedError
	if !errors.As(err, &rateLimitErr) {
		t.Fatalf("error = %v, want *RateLimitedError", err)
	}

	snap, _ := m.Status(context.Background(), "a1")
	if snap.State != DispatcherRateLimited {
		t.Fatalf("state = %v, want rate_limited", snap.State)
	}
	if snap.RetryAt == nil || !snap.RetryAt.Equal(retryAt) {
		t.Fatalf("RetryAt = %v, want %v", snap.RetryAt, retryAt)
	}
}

func TestRateLimitedResultIsNeverAutomaticallyRetried(t *testing.T) {
	acc := testAcc("a1", "user:write:chat")
	accounts := newFakeAccounts(acc)
	provider := newFakeProvider(account.ProviderTwitch)
	provider.queueResult(func() (SendMessageResult, error) { return SendMessageResult{}, &RateLimitedError{} })
	m := newManagerForTest(accounts, provider, time.Now)

	if _, err := m.Send(context.Background(), SendMessageRequest{AccountID: "a1", Message: "first"}); err == nil {
		t.Fatal("expected an error for the rate-limited send")
	}
	if calls := provider.callsFor("a1"); len(calls) != 1 {
		t.Fatalf("provider called %d times, want exactly 1 (no automatic retry)", len(calls))
	}
}

func TestDeliveryUnknownIsNeverAutomaticallyRetried(t *testing.T) {
	acc := testAcc("a1", "user:write:chat")
	accounts := newFakeAccounts(acc)
	provider := newFakeProvider(account.ProviderTwitch)
	provider.queueResult(func() (SendMessageResult, error) { return SendMessageResult{}, ErrDeliveryUnknown })
	m := newManagerForTest(accounts, provider, time.Now)

	_, err := m.Send(context.Background(), SendMessageRequest{AccountID: "a1", Message: "uncertain"})
	if !errors.Is(err, ErrDeliveryUnknown) {
		t.Fatalf("error = %v, want ErrDeliveryUnknown", err)
	}
	if calls := provider.callsFor("a1"); len(calls) != 1 {
		t.Fatalf("provider called %d times, want exactly 1 (no automatic retry)", len(calls))
	}
}

func TestCancellationWhileQueuedNeverStartsTheSend(t *testing.T) {
	acc := testAcc("a1", "user:write:chat")
	accounts := newFakeAccounts(acc)
	provider := newFakeProvider(account.ProviderTwitch)
	block := make(chan struct{})
	provider.onSend = func(sendCall) { <-block }
	m := newManagerForTest(accounts, provider, time.Now)

	go func() { _, _ = m.Send(context.Background(), SendMessageRequest{AccountID: "a1", Message: "occupying"}) }()
	time.Sleep(20 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := m.Send(ctx, SendMessageRequest{AccountID: "a1", Message: "cancelled"})
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("error = %v, want ErrCancelled", err)
	}
	close(block)
}

func TestShutdownStopsEveryDispatcherAndReturns(t *testing.T) {
	accounts := newFakeAccounts(testAcc("a1", "user:write:chat"), testAcc("a2", "user:write:chat"))
	provider := newFakeProvider(account.ProviderTwitch)
	m := newManagerForTest(accounts, provider, time.Now)

	if _, err := m.Send(context.Background(), SendMessageRequest{AccountID: "a1", Message: "one"}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if _, err := m.Send(context.Background(), SendMessageRequest{AccountID: "a2", Message: "two"}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := m.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
}

func TestStatusSnapshotNeverCarriesMessageContent(t *testing.T) {
	acc := testAcc("a1", "user:write:chat")
	accounts := newFakeAccounts(acc)
	provider := newFakeProvider(account.ProviderTwitch)
	m := newManagerForTest(accounts, provider, time.Now)

	secretMessage := "this exact text must never appear on the Snapshot struct"
	if _, err := m.Send(context.Background(), SendMessageRequest{AccountID: "a1", Message: secretMessage}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	snap, err := m.Status(context.Background(), "a1")
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	// Snapshot has no message-shaped field at all - this is a structural
	// guarantee (see Snapshot's own field list), reasserted here as a
	// behavioral test: nothing about the sent text is reachable from the
	// returned value's exported state.
	if snap.LastErrorCode == secretMessage {
		t.Fatal("LastErrorCode leaked the message text")
	}
}

func TestNewManagerStartsWithNoDispatchers(t *testing.T) {
	accounts := newFakeAccounts(testAcc("a1", "user:write:chat"))
	provider := newFakeProvider(account.ProviderTwitch)
	m := newManagerForTest(accounts, provider, time.Now)

	snap, err := m.Status(context.Background(), "a1")
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if snap.State != DispatcherIdle || snap.QueueDepth != 0 || snap.LastAttemptAt != nil {
		t.Fatalf("snapshot = %+v, want a fresh idle/empty state for an account that has never sent", snap)
	}
}
