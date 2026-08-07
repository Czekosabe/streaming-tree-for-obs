package chatautomation

import (
	"testing"
	"time"

	"github.com/streaming-tree/server/internal/domain/account"
	domain "github.com/streaming-tree/server/internal/domain/chatautomation"
	engagement "github.com/streaming-tree/server/internal/domain/engagement"
)

func TestParseCommandTokenExamples(t *testing.T) {
	cases := []struct {
		text      string
		wantToken string
		wantOK    bool
	}{
		{"!discord", "discord", true},
		{"hello !discord", "", false},
		{"!!discord", "", false},
		{"!discord extra arguments", "discord", true},
		{"!DISCORD", "discord", true},
		{"!", "", false},
		{"", "", false},
		{"  !discord  ", "discord", true},
	}
	for _, tc := range cases {
		token, ok := parseCommandToken(tc.text)
		if ok != tc.wantOK || token != tc.wantToken {
			t.Errorf("parseCommandToken(%q) = %q, %v, want %q, %v", tc.text, token, ok, tc.wantToken, tc.wantOK)
		}
	}
}

func TestRoleSatisfies(t *testing.T) {
	cases := []struct {
		required domain.Role
		roles    []engagement.Role
		want     bool
	}{
		{domain.RoleEveryone, nil, true},
		{domain.RoleSubscriber, nil, false},
		{domain.RoleSubscriber, []engagement.Role{engagement.RoleSubscriber}, true},
		{domain.RoleVIP, []engagement.Role{engagement.RoleVIP}, true},
		{domain.RoleVIP, []engagement.Role{engagement.RoleBroadcaster}, true},
		{domain.RoleVIP, []engagement.Role{engagement.RoleModerator}, false},
		{domain.RoleModerator, []engagement.Role{engagement.RoleModerator}, true},
		{domain.RoleModerator, []engagement.Role{engagement.RoleBroadcaster}, true},
		{domain.RoleModerator, []engagement.Role{engagement.RoleVIP}, false},
		// Moderator does NOT automatically imply subscriber unless the
		// event independently reports it - Part 15's own explicit rule.
		{domain.RoleSubscriber, []engagement.Role{engagement.RoleModerator}, false},
		{domain.RoleSubscriber, []engagement.Role{engagement.RoleModerator, engagement.RoleSubscriber}, true},
		{domain.RoleBroadcaster, []engagement.Role{engagement.RoleModerator}, false},
		{domain.RoleBroadcaster, []engagement.Role{engagement.RoleBroadcaster}, true},
	}
	for _, tc := range cases {
		if got := roleSatisfies(tc.required, tc.roles); got != tc.want {
			t.Errorf("roleSatisfies(%q, %v) = %v, want %v", tc.required, tc.roles, got, tc.want)
		}
	}
}

func TestCommandRuntimeCooldowns(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cr := &commandRuntime{
		def:               domain.Command{GlobalCooldownSeconds: 10, UserCooldownSeconds: 30},
		userCooldownUntil: map[string]time.Time{},
	}

	if !cr.tryReserveCooldown("user_a", now) {
		t.Fatal("first reservation for user_a should succeed")
	}
	if cr.tryReserveCooldown("user_b", now) {
		t.Error("global cooldown should block a different user immediately after")
	}
	if cr.tryReserveCooldown("user_a", now.Add(5*time.Second)) {
		t.Error("user_a should still be on both cooldowns 5s later")
	}
	if !cr.tryReserveCooldown("user_b", now.Add(11*time.Second)) {
		t.Error("a different user should be eligible once only the global cooldown elapsed")
	}
	if cr.tryReserveCooldown("user_a", now.Add(31*time.Second)) {
		// Global cooldown was reset by user_b's own successful reservation
		// at +11s (10s global from then = until +21s), so by +31s the
		// global cooldown has elapsed, but user_a's own 30s per-user
		// cooldown (from t=0) elapsed at +30s already too - so this
		// reservation is expected to SUCCEED, not be blocked.
	} else {
		t.Error("user_a's own per-user cooldown should have elapsed by +31s")
	}
}

func newFakeCommandDeps(clock *fakeClock, acc account.Account, provider *fakeOutboundProvider) (*commandEngine, *fakeAccounts) {
	accounts := newFakeAccounts(acc)
	dispatch := newTestDispatcher(clock, accounts, provider)
	e := newCommandEngine(clock.Now, accounts, fakePlatforms{}, dispatch)
	return e, accounts
}

func chatMessageEvent(connectedAccountID, providerUserID, text string, roles []engagement.Role, synthetic bool) engagement.Event {
	return engagement.Event{
		Type: engagement.TypeChatMessage, ConnectedAccountID: connectedAccountID, Synthetic: synthetic,
		ProviderEventID: "msg_1",
		User:            &engagement.User{ProviderUserID: providerUserID, Login: "viewer", Roles: roles},
		Message:         &engagement.Message{Text: text},
	}
}

func TestCommandEngineMatchesCanonicalAndAlias(t *testing.T) {
	clock := newFakeClock()
	acc := testAccount("acct_1")
	provider := &fakeOutboundProvider{}
	e, _ := newFakeCommandDeps(clock, acc, provider)
	e.reload([]domain.Command{{
		ID: "cmd_1", Name: "discord", Enabled: true, ResponseTemplate: "join us", RequiredRole: domain.RoleEveryone,
		Aliases: []string{"disc"}, Targets: []domain.Target{{AccountID: "acct_1"}},
	}})

	e.handleEvent(chatMessageEvent("acct_1", "viewer_1", "!discord", nil, false))
	if provider.callCount() != 1 {
		t.Fatalf("canonical name: provider called %d times, want 1", provider.callCount())
	}

	// The dispatcher's own Stage 11A local rate-limit floor (one dispatch
	// start per account per second) applies here too - advance the fake
	// clock so the second, genuinely distinct command dispatch is not
	// blocked waiting for real wall-clock time nothing in this test ever
	// advances.
	clock.Advance(2 * time.Second)
	e.handleEvent(chatMessageEvent("acct_1", "viewer_2", "!disc", nil, false))
	if provider.callCount() != 2 {
		t.Fatalf("alias: provider called %d times, want 2", provider.callCount())
	}
}

func TestCommandEngineIgnoresMiddleOfMessage(t *testing.T) {
	clock := newFakeClock()
	provider := &fakeOutboundProvider{}
	e, _ := newFakeCommandDeps(clock, testAccount("acct_1"), provider)
	e.reload([]domain.Command{{
		ID: "cmd_1", Name: "discord", Enabled: true, ResponseTemplate: "x", RequiredRole: domain.RoleEveryone,
		Targets: []domain.Target{{AccountID: "acct_1"}},
	}})

	e.handleEvent(chatMessageEvent("acct_1", "viewer_1", "hey check out !discord", nil, false))
	if provider.callCount() != 0 {
		t.Errorf("provider called %d times, want 0 (command not at start)", provider.callCount())
	}
}

func TestCommandEngineIgnoresSelfMessage(t *testing.T) {
	clock := newFakeClock()
	acc := testAccount("acct_1")
	provider := &fakeOutboundProvider{}
	e, _ := newFakeCommandDeps(clock, acc, provider)
	e.reload([]domain.Command{{
		ID: "cmd_1", Name: "discord", Enabled: true, ResponseTemplate: "x", RequiredRole: domain.RoleEveryone,
		Targets: []domain.Target{{AccountID: "acct_1"}},
	}})

	// The message's own chatter provider user id equals the connected
	// account's own provider user id - this is the hard Part 14 rule.
	e.handleEvent(chatMessageEvent("acct_1", acc.ProviderUserID, "!discord", nil, false))
	if provider.callCount() != 0 {
		t.Errorf("provider called %d times, want 0 (self-message must never trigger)", provider.callCount())
	}
	if e.totalSelfSkips.Load() != 1 {
		t.Errorf("totalSelfSkips = %d, want 1", e.totalSelfSkips.Load())
	}
}

func TestCommandEngineIgnoresSyntheticMessage(t *testing.T) {
	clock := newFakeClock()
	provider := &fakeOutboundProvider{}
	e, _ := newFakeCommandDeps(clock, testAccount("acct_1"), provider)
	e.reload([]domain.Command{{
		ID: "cmd_1", Name: "discord", Enabled: true, ResponseTemplate: "x", RequiredRole: domain.RoleEveryone,
		Targets: []domain.Target{{AccountID: "acct_1"}},
	}})

	e.handleEvent(chatMessageEvent("acct_1", "viewer_1", "!discord", nil, true))
	if provider.callCount() != 0 {
		t.Errorf("provider called %d times, want 0 (synthetic events never trigger)", provider.callCount())
	}
}

func TestCommandEngineIgnoresWrongAccount(t *testing.T) {
	clock := newFakeClock()
	accounts := newFakeAccounts(testAccount("acct_1"), testAccount("acct_2"))
	provider := &fakeOutboundProvider{}
	dispatch := newTestDispatcher(clock, accounts, provider)
	e := newCommandEngine(clock.Now, accounts, fakePlatforms{}, dispatch)
	e.reload([]domain.Command{{
		ID: "cmd_1", Name: "discord", Enabled: true, ResponseTemplate: "x", RequiredRole: domain.RoleEveryone,
		Targets: []domain.Target{{AccountID: "acct_1"}}, // only acct_1 is targeted
	}})

	e.handleEvent(chatMessageEvent("acct_2", "viewer_1", "!discord", nil, false))
	if provider.callCount() != 0 {
		t.Errorf("provider called %d times, want 0 (event belongs to a non-targeted account)", provider.callCount())
	}
}

func TestCommandEngineRoleRequirementBlocksThenAllows(t *testing.T) {
	clock := newFakeClock()
	provider := &fakeOutboundProvider{}
	e, _ := newFakeCommandDeps(clock, testAccount("acct_1"), provider)
	e.reload([]domain.Command{{
		ID: "cmd_1", Name: "vip-only", Enabled: true, ResponseTemplate: "x", RequiredRole: domain.RoleModerator,
		Targets: []domain.Target{{AccountID: "acct_1"}},
	}})

	e.handleEvent(chatMessageEvent("acct_1", "viewer_1", "!vip-only", nil, false))
	if provider.callCount() != 0 {
		t.Fatalf("provider called %d times, want 0 (missing required role)", provider.callCount())
	}
	if e.totalRoleSkips.Load() != 1 {
		t.Errorf("totalRoleSkips = %d, want 1", e.totalRoleSkips.Load())
	}

	e.handleEvent(chatMessageEvent("acct_1", "viewer_1", "!vip-only", []engagement.Role{engagement.RoleModerator}, false))
	if provider.callCount() != 1 {
		t.Errorf("provider called %d times, want 1 once the role requirement is met", provider.callCount())
	}
}

func TestCommandEngineGlobalCooldownBlocksAnotherUser(t *testing.T) {
	clock := newFakeClock()
	provider := &fakeOutboundProvider{}
	e, _ := newFakeCommandDeps(clock, testAccount("acct_1"), provider)
	e.reload([]domain.Command{{
		ID: "cmd_1", Name: "discord", Enabled: true, ResponseTemplate: "x", RequiredRole: domain.RoleEveryone,
		GlobalCooldownSeconds: 30, Targets: []domain.Target{{AccountID: "acct_1"}},
	}})

	e.handleEvent(chatMessageEvent("acct_1", "viewer_1", "!discord", nil, false))
	e.handleEvent(chatMessageEvent("acct_1", "viewer_2", "!discord", nil, false))
	if provider.callCount() != 1 {
		t.Errorf("provider called %d times, want 1 (second, different user still hits the global cooldown)", provider.callCount())
	}
}

func TestCommandEngineUserCooldownIndependentPerUser(t *testing.T) {
	clock := newFakeClock()
	provider := &fakeOutboundProvider{}
	e, _ := newFakeCommandDeps(clock, testAccount("acct_1"), provider)
	e.reload([]domain.Command{{
		ID: "cmd_1", Name: "discord", Enabled: true, ResponseTemplate: "x", RequiredRole: domain.RoleEveryone,
		UserCooldownSeconds: 30, Targets: []domain.Target{{AccountID: "acct_1"}},
	}})

	e.handleEvent(chatMessageEvent("acct_1", "viewer_1", "!discord", nil, false))
	e.handleEvent(chatMessageEvent("acct_1", "viewer_1", "!discord", nil, false))
	if provider.callCount() != 1 {
		t.Errorf("same user twice: provider called %d times, want 1", provider.callCount())
	}
	// Same account-level dispatcher floor as above - the second user's
	// own dispatch is a genuinely new send, so give the fake clock room.
	clock.Advance(2 * time.Second)
	e.handleEvent(chatMessageEvent("acct_1", "viewer_2", "!discord", nil, false))
	if provider.callCount() != 2 {
		t.Errorf("a different user: provider called %d times, want 2 (own cooldown is independent)", provider.callCount())
	}
}

func TestCommandEngineDisabledCommandNeverMatches(t *testing.T) {
	clock := newFakeClock()
	provider := &fakeOutboundProvider{}
	e, _ := newFakeCommandDeps(clock, testAccount("acct_1"), provider)
	e.reload([]domain.Command{{
		ID: "cmd_1", Name: "discord", Enabled: false, ResponseTemplate: "x", RequiredRole: domain.RoleEveryone,
		Targets: []domain.Target{{AccountID: "acct_1"}},
	}})

	e.handleEvent(chatMessageEvent("acct_1", "viewer_1", "!discord", nil, false))
	if provider.callCount() != 0 {
		t.Errorf("provider called %d times, want 0 (command is disabled)", provider.callCount())
	}
}

func TestCommandEngineUsesSourceCommandAndReply(t *testing.T) {
	clock := newFakeClock()
	provider := &fakeOutboundProvider{}
	e, _ := newFakeCommandDeps(clock, testAccount("acct_1"), provider)
	e.reload([]domain.Command{{
		ID: "cmd_1", Name: "discord", Enabled: true, ResponseTemplate: "join us", RequiredRole: domain.RoleEveryone,
		Targets: []domain.Target{{AccountID: "acct_1"}},
	}})

	e.handleEvent(chatMessageEvent("acct_1", "viewer_1", "!discord", nil, false))
	call := provider.lastCall()
	if call.Source != "command" {
		t.Errorf("Source = %q, want command", call.Source)
	}
	if call.ReplyParentMessageID != "msg_1" {
		t.Errorf("ReplyParentMessageID = %q, want msg_1 (same-account reply)", call.ReplyParentMessageID)
	}
}

func TestCommandEngineReloadRebuildsAliasesAtomically(t *testing.T) {
	clock := newFakeClock()
	provider := &fakeOutboundProvider{}
	e, _ := newFakeCommandDeps(clock, testAccount("acct_1"), provider)
	e.reload([]domain.Command{{
		ID: "cmd_1", Name: "discord", Enabled: true, ResponseTemplate: "x", RequiredRole: domain.RoleEveryone,
		Aliases: []string{"disc"}, Targets: []domain.Target{{AccountID: "acct_1"}},
	}})
	if _, ok := e.lookup("disc"); !ok {
		t.Fatal("alias should resolve before reload")
	}

	e.reload([]domain.Command{{
		ID: "cmd_1", Name: "discord", Enabled: true, ResponseTemplate: "x", RequiredRole: domain.RoleEveryone,
		Aliases: []string{"server"}, Targets: []domain.Target{{AccountID: "acct_1"}},
	}})
	if _, ok := e.lookup("disc"); ok {
		t.Error("old alias 'disc' should no longer resolve after reload")
	}
	if _, ok := e.lookup("server"); !ok {
		t.Error("new alias 'server' should resolve immediately after reload")
	}
}

func TestCommandEngineDeleteStopsMatching(t *testing.T) {
	clock := newFakeClock()
	provider := &fakeOutboundProvider{}
	e, _ := newFakeCommandDeps(clock, testAccount("acct_1"), provider)
	e.reload([]domain.Command{{
		ID: "cmd_1", Name: "discord", Enabled: true, ResponseTemplate: "x", RequiredRole: domain.RoleEveryone,
		Targets: []domain.Target{{AccountID: "acct_1"}},
	}})
	e.reload(nil) // deletion -> the full list no longer contains cmd_1

	e.handleEvent(chatMessageEvent("acct_1", "viewer_1", "!discord", nil, false))
	if provider.callCount() != 0 {
		t.Errorf("provider called %d times, want 0 (command was deleted)", provider.callCount())
	}
}
