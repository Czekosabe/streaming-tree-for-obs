package chatautomation

import (
	"context"
	"sync"
	"time"

	domain "github.com/streaming-tree/server/internal/domain/chatautomation"
	engagement "github.com/streaming-tree/server/internal/domain/engagement"
	bus "github.com/streaming-tree/server/internal/engagement"
	"github.com/streaming-tree/server/internal/outboundchat"
)

// resubscribeBackoff bounds how quickly Manager's own shared Event Bus
// subscription retries after a failed Subscribe call.
const resubscribeBackoff = time.Second

// BotUserChecker resolves whether a chat user has been explicitly
// marked as a bot by the operator - see the Stage 11B task's own Part 9
// ("messages from explicitly marked bot users should not count ... do
// not use username heuristics"). Deliberately primitive-typed, mirroring
// AccountAccessor/PlatformAccessor.
type BotUserChecker interface {
	IsBotUser(ctx context.Context, providerID, connectedAccountID, providerUserID string) (bool, error)
}

// ManagerOptions constructs a Manager.
type ManagerOptions struct {
	DomainService *domain.Service
	Outbound      *outboundchat.Manager
	Bus           *bus.Bus
	Ingest        IngestChecker
	Accounts      AccountAccessor
	Platforms     PlatformAccessor
	BotUsers      BotUserChecker
	// Now and RandFrac are test-only fake-clock/randomness overrides;
	// production code leaves them nil.
	Now      func() time.Time
	RandFrac func() float64
}

// Manager is the Stage 11B automation runtime: the CRUD façade over
// internal/domain/chatautomation (triggering a scheduler/command-engine
// reload after every write), the in-memory scheduler, the command
// engine, and the ONE shared Engagement Event Bus subscription both the
// command engine and the scheduler's own activity counter read from -
// never a second outbound dispatcher, never a direct Twitch inbound
// connection. See package doc comment in errors.go.
type Manager struct {
	domainSvc *domain.Service
	scheduler *scheduler
	commands  *commandEngine
	source    *bus.Bus
	botUsers  BotUserChecker
	accounts  AccountAccessor
	platforms PlatformAccessor

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewManager builds a Manager. Call Start to load persisted definitions
// and begin the scheduler/subscription; call Shutdown to stop both
// cleanly.
func NewManager(opts ManagerOptions) *Manager {
	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	dispatch := newDispatcher(opts.Outbound)
	sched := newScheduler(now, opts.RandFrac, opts.Ingest, opts.Accounts, opts.Platforms, dispatch)
	cmds := newCommandEngine(now, opts.Accounts, opts.Platforms, dispatch)
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		domainSvc: opts.DomainService, scheduler: sched, commands: cmds, source: opts.Bus,
		botUsers: opts.BotUsers, accounts: opts.Accounts, platforms: opts.Platforms,
		ctx: ctx, cancel: cancel,
	}
}

// Start loads every persisted schedule and command, then begins the
// scheduler's own polling loop and the shared Event Bus subscription.
func (m *Manager) Start(ctx context.Context) error {
	schedules, err := m.domainSvc.ListSchedules(ctx)
	if err != nil {
		return err
	}
	m.scheduler.reload(schedules)

	commands, err := m.domainSvc.ListCommands(ctx)
	if err != nil {
		return err
	}
	m.commands.reload(commands)

	m.scheduler.start()
	m.wg.Add(1)
	go m.runSubscription()
	return nil
}

// Shutdown stops the scheduler and the shared subscription and waits
// for both to exit, bounded by ctx - no goroutine leak across a backend
// restart.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.cancel()
	m.scheduler.shutdown()

	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Manager) runSubscription() {
	defer m.wg.Done()
	for {
		select {
		case <-m.ctx.Done():
			return
		default:
		}

		snap := m.source.Snapshot()
		sub, _, err := m.source.Subscribe(snap.NewestSequence)
		if err != nil {
			m.commands.lastErrorCode.Store("subscribe_failed")
			select {
			case <-m.ctx.Done():
				return
			case <-time.After(resubscribeBackoff):
				continue
			}
		}

		m.commands.subscribed.Store(true)
		m.consume(sub)
		m.commands.subscribed.Store(false)
	}
}

// consume reads sub until it closes for any reason (cancellation,
// shutdown, or a slow-consumer drop) - a drop reconnects in
// runSubscription's own loop using the bus's CURRENT position, never
// replaying historical messages into a late command match or a stale
// activity count (Part 29/30).
func (m *Manager) consume(sub *bus.Subscription) {
	for {
		select {
		case <-m.ctx.Done():
			sub.Cancel()
			return
		case evt, ok := <-sub.Events():
			if !ok {
				return
			}
			m.commands.handleEvent(evt)
			m.recordActivity(evt)
		case <-sub.Closed():
			return
		}
	}
}

// recordActivity applies the eligibility rules from Part 9 (human
// chat.message only, never self/synthetic/explicitly-marked-bot) before
// incrementing the scheduler's own per-schedule/account counters.
func (m *Manager) recordActivity(evt engagement.Event) {
	if evt.Type != engagement.TypeChatMessage || evt.Synthetic {
		return
	}
	if evt.Message == nil || evt.User == nil || evt.User.Anonymous {
		return
	}
	ctx := context.Background()
	acc, err := m.accounts.GetAccount(ctx, evt.ConnectedAccountID)
	if err != nil || evt.User.ProviderUserID == acc.ProviderUserID {
		return
	}
	if m.botUsers != nil {
		if isBot, err := m.botUsers.IsBotUser(ctx, string(acc.ProviderID), evt.ConnectedAccountID, evt.User.ProviderUserID); err == nil && isBot {
			return
		}
	}
	m.scheduler.recordActivity(evt.ConnectedAccountID)
}

// --- schedule CRUD façade --------------------------------------------------

func (m *Manager) CreateSchedule(ctx context.Context, in domain.ScheduleInput) (domain.Schedule, error) {
	sch, err := m.domainSvc.CreateSchedule(ctx, in)
	if err != nil {
		return domain.Schedule{}, err
	}
	m.scheduler.reloadOne(sch)
	return sch, nil
}

func (m *Manager) GetSchedule(ctx context.Context, id string) (domain.Schedule, error) {
	return m.domainSvc.GetSchedule(ctx, id)
}

func (m *Manager) ListSchedules(ctx context.Context) ([]domain.Schedule, error) {
	return m.domainSvc.ListSchedules(ctx)
}

func (m *Manager) ReplaceSchedule(ctx context.Context, id string, in domain.ScheduleInput) (domain.Schedule, error) {
	sch, err := m.domainSvc.ReplaceSchedule(ctx, id, in)
	if err != nil {
		return domain.Schedule{}, err
	}
	m.scheduler.reloadOne(sch)
	return sch, nil
}

func (m *Manager) DeleteSchedule(ctx context.Context, id string) error {
	if err := m.domainSvc.DeleteSchedule(ctx, id); err != nil {
		return err
	}
	m.scheduler.remove(id)
	return nil
}

// SendNow runs one manual, operator-confirmed execution of schedule id
// against accountIDs (every current target when accountIDs is empty) -
// see the Stage 11B task's own Part 11.
func (m *Manager) SendNow(ctx context.Context, id string, accountIDs []string) ([]SendResult, error) {
	if _, err := m.domainSvc.GetSchedule(ctx, id); err != nil {
		return nil, err
	}
	sr, ok := m.scheduler.get(id)
	if !ok {
		return nil, ErrScheduleNotFound
	}
	return m.scheduler.sendNow(sr, accountIDs), nil
}

func (m *Manager) ScheduleStatus(id string) (ScheduleSnapshot, bool) {
	sr, ok := m.scheduler.get(id)
	if !ok {
		return ScheduleSnapshot{}, false
	}
	return m.scheduler.snapshotOf(sr), true
}

// --- command CRUD façade ---------------------------------------------------

func (m *Manager) reloadCommands(ctx context.Context) error {
	commands, err := m.domainSvc.ListCommands(ctx)
	if err != nil {
		return err
	}
	m.commands.reload(commands)
	return nil
}

func (m *Manager) CreateCommand(ctx context.Context, in domain.CommandInput) (domain.Command, error) {
	cmd, err := m.domainSvc.CreateCommand(ctx, in)
	if err != nil {
		return domain.Command{}, err
	}
	if err := m.reloadCommands(ctx); err != nil {
		return domain.Command{}, err
	}
	return cmd, nil
}

func (m *Manager) GetCommand(ctx context.Context, id string) (domain.Command, error) {
	return m.domainSvc.GetCommand(ctx, id)
}

func (m *Manager) ListCommands(ctx context.Context) ([]domain.Command, error) {
	return m.domainSvc.ListCommands(ctx)
}

func (m *Manager) ReplaceCommand(ctx context.Context, id string, in domain.CommandInput) (domain.Command, error) {
	cmd, err := m.domainSvc.ReplaceCommand(ctx, id, in)
	if err != nil {
		return domain.Command{}, err
	}
	if err := m.reloadCommands(ctx); err != nil {
		return domain.Command{}, err
	}
	return cmd, nil
}

func (m *Manager) DeleteCommand(ctx context.Context, id string) error {
	if err := m.domainSvc.DeleteCommand(ctx, id); err != nil {
		return err
	}
	return m.reloadCommands(ctx)
}

func (m *Manager) CommandStatus(id string) (CommandSnapshot, bool) {
	cr, ok := m.commands.getRuntime(id)
	if !ok {
		return CommandSnapshot{}, false
	}
	return m.commands.snapshotOf(cr), true
}

// --- status and preview -----------------------------------------------

// Status returns the full automation runtime status - the HTTP status
// endpoint's own source of truth.
func (m *Manager) Status(ctx context.Context) (Status, error) {
	schedules, err := m.domainSvc.ListSchedules(ctx)
	if err != nil {
		return Status{}, err
	}
	scheduleSnaps := make([]ScheduleSnapshot, 0, len(schedules))
	for _, sch := range schedules {
		if snap, ok := m.ScheduleStatus(sch.ID); ok {
			scheduleSnaps = append(scheduleSnaps, snap)
		}
	}

	commands, err := m.domainSvc.ListCommands(ctx)
	if err != nil {
		return Status{}, err
	}
	commandSnaps := make([]CommandSnapshot, 0, len(commands))
	for _, c := range commands {
		if snap, ok := m.CommandStatus(c.ID); ok {
			commandSnaps = append(commandSnaps, snap)
		}
	}

	return Status{Engine: m.commands.status(), Schedules: scheduleSnaps, Commands: commandSnaps}, nil
}

// Preview renders template against accountID's own already-available
// local context - never sends anything, never persists anything, never
// makes a provider network request. A syntactically malformed template
// (an unmatched brace) is the only case Preview itself fails on; an
// unknown or currently-unresolvable placeholder is reported through the
// returned RenderResult's own Unresolved list instead - see the Stage
// 11B task's own Part 22.
func (m *Manager) Preview(ctx context.Context, template, accountID, platformID string) (RenderResult, error) {
	rc, err := m.buildPreviewContext(ctx, accountID, platformID)
	if err != nil {
		return RenderResult{}, err
	}
	return Render(template, rc)
}

func (m *Manager) buildPreviewContext(ctx context.Context, accountID, platformID string) (Context, error) {
	acc, err := m.accounts.GetAccount(ctx, accountID)
	if err != nil {
		return Context{}, err
	}
	rc := Context{ChannelName: acc.DisplayName, Platform: PlatformDisplayName(acc.ProviderID)}
	if rc.ChannelName == "" {
		rc.ChannelName = acc.Login
	}
	if url, ok := ChannelURL(acc.ProviderID, acc.Login); ok {
		rc.ChannelURL = url
	}
	if platformID != "" {
		if p, err := m.platforms.Get(ctx, platformID); err == nil {
			title := p.Metadata.Title
			rc.StreamTitle = &title
		}
	}
	if uptime, ok := m.scheduler.streamUptime(); ok {
		rc.StreamUptime = &uptime
	}
	return rc, nil
}
