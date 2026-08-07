package sqlite

import (
	"context"
	"testing"

	"github.com/streaming-tree/server/internal/domain/chatautomation"
)

func TestChatAutomationScheduleCreateThenGetRoundTrips(t *testing.T) {
	db := newTestDB(t)
	accounts := NewAccountRepository(db.DB)
	repo := NewChatAutomationRepository(db.DB)
	createTestAccount(t, accounts, "acct_1")

	s := chatautomation.Schedule{
		ID: "sched_1", Name: "Every hour", Enabled: true,
		IntervalSeconds: 3600, FirstDelaySeconds: 30, JitterSeconds: 60,
		OnlyWhileIngestReceiving: true, MinimumChatMessages: 5, MaximumSendsPerHour: 4,
		Targets: []chatautomation.Target{{AccountID: "acct_1"}},
		Messages: []chatautomation.ScheduleMessage{
			{ID: "schedmsg_1", MessageTemplate: "hello {channelName}", Position: 0},
			{ID: "schedmsg_2", MessageTemplate: "hi there", Position: 1},
		},
	}
	saved, err := repo.CreateSchedule(context.Background(), s)
	if err != nil {
		t.Fatalf("CreateSchedule() error = %v", err)
	}
	if saved.Name != "Every hour" || saved.IntervalSeconds != 3600 || !saved.OnlyWhileIngestReceiving {
		t.Errorf("saved = %+v, want the created fields", saved)
	}
	if len(saved.Targets) != 1 || saved.Targets[0].AccountID != "acct_1" {
		t.Errorf("saved.Targets = %+v, want one target acct_1", saved.Targets)
	}
	if len(saved.Messages) != 2 || saved.Messages[0].MessageTemplate != "hello {channelName}" || saved.Messages[1].Position != 1 {
		t.Fatalf("saved.Messages = %+v, want two ordered messages", saved.Messages)
	}

	got, found, err := repo.GetSchedule(context.Background(), "sched_1")
	if err != nil || !found {
		t.Fatalf("GetSchedule() = %+v, found=%v, err=%v", got, found, err)
	}
	if len(got.Messages) != 2 {
		t.Errorf("GetSchedule() Messages = %+v, want 2", got.Messages)
	}
}

func TestChatAutomationScheduleCreateRejectsUnknownAccount(t *testing.T) {
	db := newTestDB(t)
	repo := NewChatAutomationRepository(db.DB)

	s := chatautomation.Schedule{
		ID: "sched_1", Name: "x", Enabled: true, IntervalSeconds: 60, MaximumSendsPerHour: 1,
		Targets:  []chatautomation.Target{{AccountID: "acct_missing"}},
		Messages: []chatautomation.ScheduleMessage{{ID: "schedmsg_1", MessageTemplate: "hi", Position: 0}},
	}
	if _, err := repo.CreateSchedule(context.Background(), s); err != chatautomation.ErrAccountNotFound {
		t.Errorf("CreateSchedule() error = %v, want ErrAccountNotFound", err)
	}
}

func TestChatAutomationScheduleUpdateReplacesTargetsAndMessages(t *testing.T) {
	db := newTestDB(t)
	accounts := NewAccountRepository(db.DB)
	repo := NewChatAutomationRepository(db.DB)
	createTestAccount(t, accounts, "acct_1")
	createTestAccount(t, accounts, "acct_2")

	s := chatautomation.Schedule{
		ID: "sched_1", Name: "x", Enabled: true, IntervalSeconds: 60, MaximumSendsPerHour: 1,
		Targets:  []chatautomation.Target{{AccountID: "acct_1"}},
		Messages: []chatautomation.ScheduleMessage{{ID: "schedmsg_1", MessageTemplate: "one", Position: 0}},
	}
	if _, err := repo.CreateSchedule(context.Background(), s); err != nil {
		t.Fatalf("CreateSchedule() error = %v", err)
	}

	s.Name = "y"
	s.Targets = []chatautomation.Target{{AccountID: "acct_2"}}
	s.Messages = []chatautomation.ScheduleMessage{
		{ID: "schedmsg_2", MessageTemplate: "two", Position: 0},
		{ID: "schedmsg_3", MessageTemplate: "three", Position: 1},
	}
	updated, err := repo.UpdateSchedule(context.Background(), s)
	if err != nil {
		t.Fatalf("UpdateSchedule() error = %v", err)
	}
	if updated.Name != "y" {
		t.Errorf("updated.Name = %q, want y", updated.Name)
	}
	if len(updated.Targets) != 1 || updated.Targets[0].AccountID != "acct_2" {
		t.Errorf("updated.Targets = %+v, want exactly [acct_2]", updated.Targets)
	}
	if len(updated.Messages) != 2 {
		t.Errorf("updated.Messages = %+v, want 2", updated.Messages)
	}
}

func TestChatAutomationScheduleDeleteCascades(t *testing.T) {
	db := newTestDB(t)
	accounts := NewAccountRepository(db.DB)
	repo := NewChatAutomationRepository(db.DB)
	createTestAccount(t, accounts, "acct_1")

	s := chatautomation.Schedule{
		ID: "sched_1", Name: "x", Enabled: true, IntervalSeconds: 60, MaximumSendsPerHour: 1,
		Targets:  []chatautomation.Target{{AccountID: "acct_1"}},
		Messages: []chatautomation.ScheduleMessage{{ID: "schedmsg_1", MessageTemplate: "one", Position: 0}},
	}
	if _, err := repo.CreateSchedule(context.Background(), s); err != nil {
		t.Fatalf("CreateSchedule() error = %v", err)
	}
	if err := repo.DeleteSchedule(context.Background(), "sched_1"); err != nil {
		t.Fatalf("DeleteSchedule() error = %v", err)
	}

	var targetCount, messageCount int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM chat_schedule_targets WHERE schedule_id = ?`, "sched_1").Scan(&targetCount); err != nil {
		t.Fatalf("count targets: %v", err)
	}
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM chat_schedule_messages WHERE schedule_id = ?`, "sched_1").Scan(&messageCount); err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if targetCount != 0 || messageCount != 0 {
		t.Errorf("targets=%d messages=%d after delete, want 0 and 0", targetCount, messageCount)
	}
}

func TestChatAutomationCommandCreateThenGetRoundTrips(t *testing.T) {
	db := newTestDB(t)
	accounts := NewAccountRepository(db.DB)
	repo := NewChatAutomationRepository(db.DB)
	createTestAccount(t, accounts, "acct_1")

	c := chatautomation.Command{
		ID: "cmd_1", Name: "discord", Enabled: true,
		ResponseTemplate: "Join us: {channelUrl}", RequiredRole: chatautomation.RoleEveryone,
		GlobalCooldownSeconds: 10, UserCooldownSeconds: 30,
		Aliases: []string{"disc", "server"},
		Targets: []chatautomation.Target{{AccountID: "acct_1"}},
	}
	saved, err := repo.CreateCommand(context.Background(), c)
	if err != nil {
		t.Fatalf("CreateCommand() error = %v", err)
	}
	if saved.Name != "discord" || len(saved.Aliases) != 2 || len(saved.Targets) != 1 {
		t.Fatalf("saved = %+v, want the created fields", saved)
	}

	got, found, err := repo.GetCommand(context.Background(), "cmd_1")
	if err != nil || !found {
		t.Fatalf("GetCommand() = %+v, found=%v, err=%v", got, found, err)
	}
	if len(got.Aliases) != 2 {
		t.Errorf("got.Aliases = %v, want 2", got.Aliases)
	}
}

func TestChatAutomationCommandNameUniqueAtStorageLevel(t *testing.T) {
	db := newTestDB(t)
	accounts := NewAccountRepository(db.DB)
	repo := NewChatAutomationRepository(db.DB)
	createTestAccount(t, accounts, "acct_1")

	c1 := chatautomation.Command{
		ID: "cmd_1", Name: "discord", Enabled: true, ResponseTemplate: "x", RequiredRole: chatautomation.RoleEveryone,
		Targets: []chatautomation.Target{{AccountID: "acct_1"}},
	}
	if _, err := repo.CreateCommand(context.Background(), c1); err != nil {
		t.Fatalf("CreateCommand(1) error = %v", err)
	}
	c2 := chatautomation.Command{
		ID: "cmd_2", Name: "discord", Enabled: true, ResponseTemplate: "y", RequiredRole: chatautomation.RoleEveryone,
		Targets: []chatautomation.Target{{AccountID: "acct_1"}},
	}
	if _, err := repo.CreateCommand(context.Background(), c2); err != chatautomation.ErrCommandNameConflict {
		t.Errorf("CreateCommand(2) error = %v, want ErrCommandNameConflict", err)
	}
}

func TestChatAutomationNameOrAliasInUse(t *testing.T) {
	db := newTestDB(t)
	accounts := NewAccountRepository(db.DB)
	repo := NewChatAutomationRepository(db.DB)
	createTestAccount(t, accounts, "acct_1")

	c := chatautomation.Command{
		ID: "cmd_1", Name: "discord", Enabled: true, ResponseTemplate: "x", RequiredRole: chatautomation.RoleEveryone,
		Aliases: []string{"disc"},
		Targets: []chatautomation.Target{{AccountID: "acct_1"}},
	}
	if _, err := repo.CreateCommand(context.Background(), c); err != nil {
		t.Fatalf("CreateCommand() error = %v", err)
	}

	inUse, err := repo.NameOrAliasInUse(context.Background(), "discord", "")
	if err != nil || !inUse {
		t.Errorf("NameOrAliasInUse(discord) = %v, %v, want true, nil", inUse, err)
	}
	inUse, err = repo.NameOrAliasInUse(context.Background(), "disc", "")
	if err != nil || !inUse {
		t.Errorf("NameOrAliasInUse(disc alias) = %v, %v, want true, nil", inUse, err)
	}
	inUse, err = repo.NameOrAliasInUse(context.Background(), "discord", "cmd_1")
	if err != nil || inUse {
		t.Errorf("NameOrAliasInUse(discord, excluding cmd_1) = %v, %v, want false, nil", inUse, err)
	}
	inUse, err = repo.NameOrAliasInUse(context.Background(), "uptime", "")
	if err != nil || inUse {
		t.Errorf("NameOrAliasInUse(uptime) = %v, %v, want false, nil", inUse, err)
	}
}

func TestChatAutomationCommandDeleteCascades(t *testing.T) {
	db := newTestDB(t)
	accounts := NewAccountRepository(db.DB)
	repo := NewChatAutomationRepository(db.DB)
	createTestAccount(t, accounts, "acct_1")

	c := chatautomation.Command{
		ID: "cmd_1", Name: "discord", Enabled: true, ResponseTemplate: "x", RequiredRole: chatautomation.RoleEveryone,
		Aliases: []string{"disc"},
		Targets: []chatautomation.Target{{AccountID: "acct_1"}},
	}
	if _, err := repo.CreateCommand(context.Background(), c); err != nil {
		t.Fatalf("CreateCommand() error = %v", err)
	}
	if err := repo.DeleteCommand(context.Background(), "cmd_1"); err != nil {
		t.Fatalf("DeleteCommand() error = %v", err)
	}

	var aliasCount, targetCount int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM chat_command_aliases WHERE command_id = ?`, "cmd_1").Scan(&aliasCount); err != nil {
		t.Fatalf("count aliases: %v", err)
	}
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM chat_command_targets WHERE command_id = ?`, "cmd_1").Scan(&targetCount); err != nil {
		t.Fatalf("count targets: %v", err)
	}
	if aliasCount != 0 || targetCount != 0 {
		t.Errorf("aliases=%d targets=%d after delete, want 0 and 0", aliasCount, targetCount)
	}
}

func TestChatAutomationListSchedulesAndCommands(t *testing.T) {
	db := newTestDB(t)
	accounts := NewAccountRepository(db.DB)
	repo := NewChatAutomationRepository(db.DB)
	createTestAccount(t, accounts, "acct_1")

	if _, err := repo.CreateSchedule(context.Background(), chatautomation.Schedule{
		ID: "sched_1", Name: "a", Enabled: true, IntervalSeconds: 60, MaximumSendsPerHour: 1,
		Targets: []chatautomation.Target{{AccountID: "acct_1"}}, Messages: []chatautomation.ScheduleMessage{{ID: "schedmsg_1", MessageTemplate: "x", Position: 0}},
	}); err != nil {
		t.Fatalf("CreateSchedule() error = %v", err)
	}
	if _, err := repo.CreateCommand(context.Background(), chatautomation.Command{
		ID: "cmd_1", Name: "uptime", Enabled: true, ResponseTemplate: "x", RequiredRole: chatautomation.RoleEveryone,
		Targets: []chatautomation.Target{{AccountID: "acct_1"}},
	}); err != nil {
		t.Fatalf("CreateCommand() error = %v", err)
	}

	schedules, err := repo.ListSchedules(context.Background())
	if err != nil || len(schedules) != 1 {
		t.Fatalf("ListSchedules() = %v, err=%v, want 1 entry", schedules, err)
	}
	commands, err := repo.ListCommands(context.Background())
	if err != nil || len(commands) != 1 {
		t.Fatalf("ListCommands() = %v, err=%v, want 1 entry", commands, err)
	}
}
