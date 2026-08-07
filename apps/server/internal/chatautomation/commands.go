package chatautomation

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/streaming-tree/server/internal/domain/account"
	domain "github.com/streaming-tree/server/internal/domain/chatautomation"
	engagement "github.com/streaming-tree/server/internal/domain/engagement"
	"github.com/streaming-tree/server/internal/outboundchat"
)

// commandPrefix is Stage 11B's one fixed, non-configurable command
// prefix - see the Stage 11B task's own Part 13.
const commandPrefix = "!"

// maxCommandResponseAge bounds how long after a triggering message a
// response may still enter the dispatcher - see the Stage 11B task's
// own Part 17: "do not send a delayed response minutes after the
// user's original command."
const maxCommandResponseAge = 10 * time.Second

// parseCommandToken extracts the lowercase command token from text, or
// ok=false if text does not begin with exactly one "!" followed by a
// non-empty token - see the Stage 11B task's own Part 13 examples
// ("!discord" matches, "hello !discord" does not, "!!discord" does
// not, "!discord extra arguments" matches "discord" with arguments
// ignored).
func parseCommandToken(text string) (token string, ok bool) {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, commandPrefix) {
		return "", false
	}
	rest := trimmed[len(commandPrefix):]
	if rest == "" || strings.HasPrefix(rest, commandPrefix) {
		return "", false
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return "", false
	}
	return strings.ToLower(fields[0]), true
}

// roleSatisfies applies the Stage 11B task's own Part 15 semantic
// (not purely hierarchical) role-matching rules: moderator/VIP satisfy
// subscriber only when the normalized event independently reports it.
func roleSatisfies(required domain.Role, roles []engagement.Role) bool {
	has := func(r engagement.Role) bool {
		for _, x := range roles {
			if x == r {
				return true
			}
		}
		return false
	}
	switch required {
	case domain.RoleEveryone:
		return true
	case domain.RoleSubscriber:
		return has(engagement.RoleSubscriber)
	case domain.RoleVIP:
		return has(engagement.RoleVIP) || has(engagement.RoleBroadcaster)
	case domain.RoleModerator:
		return has(engagement.RoleModerator) || has(engagement.RoleBroadcaster)
	case domain.RoleBroadcaster:
		return has(engagement.RoleBroadcaster)
	default:
		return false
	}
}

type commandRuntime struct {
	mu                  sync.Mutex
	def                 domain.Command
	globalCooldownUntil time.Time
	userCooldownUntil   map[string]time.Time
	matchCount          int64
	responseCount       int64
	lastResponseAt      *time.Time
}

// tryReserveCooldown atomically checks and, if not on cooldown, starts
// both the global and per-user cooldown for providerUserID. This
// package's chosen, documented race-safe policy (Stage 11B task's own
// Part 16): the cooldown slot is reserved BEFORE placeholder rendering
// or dispatch is ever attempted, and is never rolled back afterward,
// even if the send later fails - a simpler and still race-safe
// guarantee against duplicate concurrent responses than a
// reserve-then-roll-back scheme, at the acceptable cost of "spending" a
// cooldown on a rare failed attempt.
func (cr *commandRuntime) tryReserveCooldown(providerUserID string, now time.Time) bool {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	if now.Before(cr.globalCooldownUntil) {
		return false
	}
	if until, ok := cr.userCooldownUntil[providerUserID]; ok && now.Before(until) {
		return false
	}
	cr.globalCooldownUntil = now.Add(time.Duration(cr.def.GlobalCooldownSeconds) * time.Second)
	cr.userCooldownUntil[providerUserID] = now.Add(time.Duration(cr.def.UserCooldownSeconds) * time.Second)
	return true
}

// commandEngine matches safe chat commands against normalized events -
// it never subscribes to the Event Bus itself. Manager owns the one,
// shared Event Bus subscription (see runtime.go) and feeds every
// chat.message event to this engine's handleEvent, exactly the "may
// share one internal subscription with the [activity-counting]
// scheduler" design the Stage 11B task's own Part 29/30 allows - never
// a direct Twitch inbound connection, never the operator Chat page's
// own SSE stream.
type commandEngine struct {
	mu       sync.Mutex
	commands map[string]*commandRuntime // by command id
	byName   map[string]*commandRuntime // canonical name/alias -> command, enabled only

	now      clock
	accounts AccountAccessor
	platform PlatformAccessor
	dispatch *dispatcher

	subscribed    atomic.Bool
	lastErrorCode atomic.Value // string

	totalMatched, totalResponses, totalCooldownSkips, totalRoleSkips, totalSelfSkips atomic.Int64
}

func newCommandEngine(now clock, accounts AccountAccessor, platform PlatformAccessor, dispatch *dispatcher) *commandEngine {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	e := &commandEngine{
		commands: make(map[string]*commandRuntime), byName: make(map[string]*commandRuntime),
		now: now, accounts: accounts, platform: platform, dispatch: dispatch,
	}
	e.lastErrorCode.Store("")
	return e
}

// reload replaces the full tracked command set from the persisted
// definitions - "create/update becomes active immediately, disable
// stops matching immediately, delete stops matching immediately,
// aliases update atomically" (Part 24). Every reload builds fresh
// runtime state (matched/response counters, cooldowns) for every
// command, exactly like the scheduler's own uniform "any edit resets
// runtime state" policy, for the same reason: simple, safe, and never
// leaking stale cooldown state into a changed configuration.
func (e *commandEngine) reload(commands []domain.Command) {
	e.mu.Lock()
	defer e.mu.Unlock()

	byID := make(map[string]*commandRuntime, len(commands))
	byName := make(map[string]*commandRuntime)
	for _, c := range commands {
		cr := &commandRuntime{def: c, userCooldownUntil: make(map[string]time.Time)}
		byID[c.ID] = cr
		if !c.Enabled {
			continue
		}
		byName[c.Name] = cr
		for _, alias := range c.Aliases {
			byName[alias] = cr
		}
	}
	e.commands = byID
	e.byName = byName
}

func (e *commandEngine) lookup(token string) (*commandRuntime, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	cr, ok := e.byName[token]
	return cr, ok
}

func (e *commandEngine) getRuntime(id string) (*commandRuntime, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	cr, ok := e.commands[id]
	return cr, ok
}

// handleEvent is called by Manager's own shared Event Bus subscription
// (see runtime.go) for every event, in publish order - it is a pure
// event handler with no subscription of its own.
func (e *commandEngine) handleEvent(evt engagement.Event) {
	if evt.Type != engagement.TypeChatMessage || evt.Synthetic {
		return
	}
	if evt.Message == nil || evt.User == nil || evt.User.Anonymous {
		return
	}

	ctx := context.Background()
	acc, err := e.accounts.GetAccount(ctx, evt.ConnectedAccountID)
	if err != nil {
		return
	}
	// Hard self-message loop-protection rule (Part 14): identity
	// comparison against the connected sending account's own provider
	// user id, never a tracked outbound message id.
	if evt.User.ProviderUserID == acc.ProviderUserID {
		e.totalSelfSkips.Add(1)
		return
	}

	token, ok := parseCommandToken(evt.Message.Text)
	if !ok {
		return
	}
	cr, found := e.lookup(token)
	if !found {
		return
	}

	cr.mu.Lock()
	def := cr.def
	cr.mu.Unlock()

	targeted := false
	for _, t := range def.Targets {
		if t.AccountID == evt.ConnectedAccountID {
			targeted = true
			break
		}
	}
	if !targeted {
		return
	}

	e.totalMatched.Add(1)
	cr.mu.Lock()
	cr.matchCount++
	cr.mu.Unlock()

	if !roleSatisfies(def.RequiredRole, evt.User.Roles) {
		e.totalRoleSkips.Add(1)
		return
	}

	now := e.now()
	if !cr.tryReserveCooldown(evt.User.ProviderUserID, now) {
		e.totalCooldownSkips.Add(1)
		return
	}

	renderCtx, err := e.buildContext(ctx, acc, def)
	if err != nil {
		return
	}
	rendered, err := Render(def.ResponseTemplate, renderCtx)
	if err != nil || len(rendered.Unresolved) > 0 || !rendered.ValidForProvider {
		return
	}

	if e.now().Sub(now) > maxCommandResponseAge {
		return
	}

	result, sendErr := e.dispatch.send(ctx, outboundchat.SendMessageRequest{
		AccountID: evt.ConnectedAccountID, Message: rendered.Text, Source: outboundchat.SourceCommand,
		ReplyParentMessageID: evt.ProviderEventID,
	})
	if sendErr != nil || !result.Sent {
		return
	}

	completedAt := e.now()
	cr.mu.Lock()
	cr.responseCount++
	cr.lastResponseAt = &completedAt
	cr.mu.Unlock()
	e.totalResponses.Add(1)
}

func (e *commandEngine) buildContext(ctx context.Context, acc account.Account, def domain.Command) (Context, error) {
	rc := Context{ChannelName: acc.DisplayName, Platform: PlatformDisplayName(acc.ProviderID)}
	if rc.ChannelName == "" {
		rc.ChannelName = acc.Login
	}
	if url, ok := ChannelURL(acc.ProviderID, acc.Login); ok {
		rc.ChannelURL = url
	}
	for _, t := range def.Targets {
		if t.AccountID == acc.ID && t.PlatformID != "" {
			if p, err := e.platform.Get(ctx, t.PlatformID); err == nil {
				title := p.Metadata.Title
				rc.StreamTitle = &title
			}
			break
		}
	}
	return rc, nil
}

func (e *commandEngine) snapshotOf(cr *commandRuntime) CommandSnapshot {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	return CommandSnapshot{
		CommandID: cr.def.ID, Enabled: cr.def.Enabled, TargetCount: len(cr.def.Targets),
		MatchCount: cr.matchCount, ResponseCount: cr.responseCount, LastResponseAt: cr.lastResponseAt,
	}
}

func (e *commandEngine) status() EngineStatus {
	e.mu.Lock()
	total, enabled := len(e.commands), 0
	for _, cr := range e.commands {
		if cr.def.Enabled {
			enabled++
		}
	}
	e.mu.Unlock()
	return EngineStatus{
		Running: true, SubscribedToBus: e.subscribed.Load(),
		CommandCount: total, EnabledCommandCount: enabled,
		TotalMatched: e.totalMatched.Load(), TotalResponses: e.totalResponses.Load(),
		TotalCooldownSkips: e.totalCooldownSkips.Load(), TotalRoleSkips: e.totalRoleSkips.Load(),
		TotalSelfSkips: e.totalSelfSkips.Load(), LastErrorCode: e.lastErrorCode.Load().(string),
	}
}
