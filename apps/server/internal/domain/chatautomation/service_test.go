package chatautomation

import (
	"context"
	"errors"
	"testing"
)

// fakeRepo is a minimal in-memory Repository for Service tests - the
// sqlite implementation's own behavior (transactions, FK/UNIQUE
// mapping) is covered separately in internal/storage/sqlite.
type fakeRepo struct {
	schedules map[string]Schedule
	commands  map[string]Command
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{schedules: map[string]Schedule{}, commands: map[string]Command{}}
}

func (r *fakeRepo) CreateSchedule(_ context.Context, s Schedule) (Schedule, error) {
	if _, exists := r.schedules[s.ID]; exists {
		return Schedule{}, errors.New("duplicate id")
	}
	r.schedules[s.ID] = s
	return s, nil
}
func (r *fakeRepo) GetSchedule(_ context.Context, id string) (Schedule, bool, error) {
	s, ok := r.schedules[id]
	return s, ok, nil
}
func (r *fakeRepo) ListSchedules(_ context.Context) ([]Schedule, error) {
	out := make([]Schedule, 0, len(r.schedules))
	for _, s := range r.schedules {
		out = append(out, s)
	}
	return out, nil
}
func (r *fakeRepo) UpdateSchedule(_ context.Context, s Schedule) (Schedule, error) {
	if _, ok := r.schedules[s.ID]; !ok {
		return Schedule{}, ErrScheduleNotFound
	}
	r.schedules[s.ID] = s
	return s, nil
}
func (r *fakeRepo) DeleteSchedule(_ context.Context, id string) error {
	delete(r.schedules, id)
	return nil
}

func (r *fakeRepo) CreateCommand(_ context.Context, c Command) (Command, error) {
	if _, exists := r.commands[c.ID]; exists {
		return Command{}, errors.New("duplicate id")
	}
	r.commands[c.ID] = c
	return c, nil
}
func (r *fakeRepo) GetCommand(_ context.Context, id string) (Command, bool, error) {
	c, ok := r.commands[id]
	return c, ok, nil
}
func (r *fakeRepo) ListCommands(_ context.Context) ([]Command, error) {
	out := make([]Command, 0, len(r.commands))
	for _, c := range r.commands {
		out = append(out, c)
	}
	return out, nil
}
func (r *fakeRepo) UpdateCommand(_ context.Context, c Command) (Command, error) {
	if _, ok := r.commands[c.ID]; !ok {
		return Command{}, ErrCommandNotFound
	}
	r.commands[c.ID] = c
	return c, nil
}
func (r *fakeRepo) DeleteCommand(_ context.Context, id string) error {
	delete(r.commands, id)
	return nil
}
func (r *fakeRepo) NameOrAliasInUse(_ context.Context, name, excludeCommandID string) (bool, error) {
	for _, c := range r.commands {
		if c.ID == excludeCommandID {
			continue
		}
		if c.Name == name {
			return true, nil
		}
		for _, a := range c.Aliases {
			if a == name {
				return true, nil
			}
		}
	}
	return false, nil
}

// fakeAccounts/fakePlatforms implement AccountLookup/PlatformLookup over
// simple maps for target-validation tests.
type fakeAccountLookup map[string]string // accountID -> providerID

func (f fakeAccountLookup) AccountProviderID(_ context.Context, accountID string) (string, bool, error) {
	p, ok := f[accountID]
	return p, ok, nil
}

type fakePlatform struct {
	providerID    string
	linkedAccount string
	linked        bool
}
type fakePlatformLookup map[string]fakePlatform // platformID -> info

func (f fakePlatformLookup) PlatformProviderID(_ context.Context, platformID string) (string, bool, error) {
	p, ok := f[platformID]
	return p.providerID, ok, nil
}
func (f fakePlatformLookup) PlatformLinkedAccountID(_ context.Context, platformID string) (string, bool, error) {
	p, ok := f[platformID]
	if !ok {
		return "", false, nil
	}
	return p.linkedAccount, p.linked, nil
}

func newTestService() (*Service, fakeAccountLookup, fakePlatformLookup) {
	accounts := fakeAccountLookup{"acct_1": "twitch", "acct_2": "twitch"}
	platforms := fakePlatformLookup{
		"pf_1": {providerID: "twitch", linkedAccount: "acct_1", linked: true},
		"pf_2": {providerID: "youtube", linkedAccount: "acct_1", linked: true},
		"pf_3": {providerID: "twitch", linkedAccount: "acct_2", linked: true},
	}
	svc := NewService(newFakeRepo(), accounts, platforms, nil)
	return svc, accounts, platforms
}

func validScheduleInput() ScheduleInput {
	return ScheduleInput{
		Name: "Reminder", Enabled: true, IntervalSeconds: 3600, MaximumSendsPerHour: 4,
		Targets:          []Target{{AccountID: "acct_1"}},
		MessageTemplates: []string{"hello"},
	}
}

func TestServiceCreateScheduleValid(t *testing.T) {
	svc, _, _ := newTestService()
	sch, err := svc.CreateSchedule(context.Background(), validScheduleInput())
	if err != nil {
		t.Fatalf("CreateSchedule() error = %v", err)
	}
	if sch.ID == "" || len(sch.Messages) != 1 {
		t.Errorf("sch = %+v, want an id and one message", sch)
	}
}

func TestServiceCreateScheduleRejectsUnknownAccount(t *testing.T) {
	svc, _, _ := newTestService()
	in := validScheduleInput()
	in.Targets = []Target{{AccountID: "acct_missing"}}
	if _, err := svc.CreateSchedule(context.Background(), in); !errors.Is(err, ErrAccountNotFound) {
		t.Errorf("CreateSchedule() error = %v, want ErrAccountNotFound", err)
	}
}

func TestServiceCreateScheduleValidatesPlatformContext(t *testing.T) {
	svc, _, _ := newTestService()

	// pf_1 belongs to and is linked to acct_1 - valid.
	in := validScheduleInput()
	in.Targets = []Target{{AccountID: "acct_1", PlatformID: "pf_1"}}
	if _, err := svc.CreateSchedule(context.Background(), in); err != nil {
		t.Errorf("CreateSchedule(valid platform context) error = %v", err)
	}

	// pf_2 is a YouTube platform but the target account is Twitch.
	in = validScheduleInput()
	in.Targets = []Target{{AccountID: "acct_1", PlatformID: "pf_2"}}
	if _, err := svc.CreateSchedule(context.Background(), in); !errors.Is(err, ErrPlatformProviderMismatch) {
		t.Errorf("CreateSchedule(provider mismatch) error = %v, want ErrPlatformProviderMismatch", err)
	}

	// pf_3 is linked to acct_2, not acct_1.
	in = validScheduleInput()
	in.Targets = []Target{{AccountID: "acct_1", PlatformID: "pf_3"}}
	if _, err := svc.CreateSchedule(context.Background(), in); !errors.Is(err, ErrPlatformNotLinked) {
		t.Errorf("CreateSchedule(not linked) error = %v, want ErrPlatformNotLinked", err)
	}

	// Unknown platform id.
	in = validScheduleInput()
	in.Targets = []Target{{AccountID: "acct_1", PlatformID: "pf_missing"}}
	if _, err := svc.CreateSchedule(context.Background(), in); !errors.Is(err, ErrPlatformNotFound) {
		t.Errorf("CreateSchedule(missing platform) error = %v, want ErrPlatformNotFound", err)
	}
}

func TestServiceReplaceScheduleRejectsMissing(t *testing.T) {
	svc, _, _ := newTestService()
	if _, err := svc.ReplaceSchedule(context.Background(), "sched_missing", validScheduleInput()); !errors.Is(err, ErrScheduleNotFound) {
		t.Errorf("ReplaceSchedule() error = %v, want ErrScheduleNotFound", err)
	}
}

func validCommandInput() CommandInput {
	return CommandInput{
		Name: "Discord", Enabled: true, ResponseTemplate: "join us", RequiredRole: RoleEveryone,
		Targets: []Target{{AccountID: "acct_1"}},
	}
}

func TestServiceCreateCommandNormalizesNameAndAliases(t *testing.T) {
	svc, _, _ := newTestService()
	in := validCommandInput()
	in.Aliases = []string{"DISC", " Server "}
	cmd, err := svc.CreateCommand(context.Background(), in)
	if err != nil {
		t.Fatalf("CreateCommand() error = %v", err)
	}
	if cmd.Name != "discord" {
		t.Errorf("cmd.Name = %q, want lowercase discord", cmd.Name)
	}
	if len(cmd.Aliases) != 2 || cmd.Aliases[0] != "disc" || cmd.Aliases[1] != "server" {
		t.Errorf("cmd.Aliases = %v, want normalized [disc server]", cmd.Aliases)
	}
}

func TestServiceCreateCommandRejectsGlobalConflict(t *testing.T) {
	svc, _, _ := newTestService()
	if _, err := svc.CreateCommand(context.Background(), validCommandInput()); err != nil {
		t.Fatalf("first CreateCommand() error = %v", err)
	}

	// A second command whose canonical name equals the first's own name.
	if _, err := svc.CreateCommand(context.Background(), validCommandInput()); !errors.Is(err, ErrCommandNameConflict) {
		t.Errorf("CreateCommand(duplicate name) error = %v, want ErrCommandNameConflict", err)
	}

	// A second command whose alias equals the first's canonical name.
	in := validCommandInput()
	in.Name = "socials"
	in.Aliases = []string{"discord"}
	if _, err := svc.CreateCommand(context.Background(), in); !errors.Is(err, ErrCommandNameConflict) {
		t.Errorf("CreateCommand(alias conflicts with name) error = %v, want ErrCommandNameConflict", err)
	}
}

func TestServiceReplaceCommandAllowsKeepingItsOwnName(t *testing.T) {
	svc, _, _ := newTestService()
	cmd, err := svc.CreateCommand(context.Background(), validCommandInput())
	if err != nil {
		t.Fatalf("CreateCommand() error = %v", err)
	}
	in := validCommandInput()
	in.ResponseTemplate = "updated response"
	updated, err := svc.ReplaceCommand(context.Background(), cmd.ID, in)
	if err != nil {
		t.Fatalf("ReplaceCommand(own name unchanged) error = %v", err)
	}
	if updated.ResponseTemplate != "updated response" {
		t.Errorf("updated.ResponseTemplate = %q, want the new value", updated.ResponseTemplate)
	}
}

func TestServiceCreateCommandRejectsInvalidRole(t *testing.T) {
	svc, _, _ := newTestService()
	in := validCommandInput()
	in.RequiredRole = Role("follower")
	if _, err := svc.CreateCommand(context.Background(), in); !errors.Is(err, ErrValidation) {
		t.Errorf("CreateCommand(follower role) error = %v, want ErrValidation", err)
	}
}

func TestServiceDeleteScheduleAndCommand(t *testing.T) {
	svc, _, _ := newTestService()
	sch, _ := svc.CreateSchedule(context.Background(), validScheduleInput())
	cmd, _ := svc.CreateCommand(context.Background(), validCommandInput())

	if err := svc.DeleteSchedule(context.Background(), sch.ID); err != nil {
		t.Errorf("DeleteSchedule() error = %v", err)
	}
	if _, err := svc.GetSchedule(context.Background(), sch.ID); !errors.Is(err, ErrScheduleNotFound) {
		t.Errorf("GetSchedule(deleted) error = %v, want ErrScheduleNotFound", err)
	}

	if err := svc.DeleteCommand(context.Background(), cmd.ID); err != nil {
		t.Errorf("DeleteCommand() error = %v", err)
	}
	if _, err := svc.GetCommand(context.Background(), cmd.ID); !errors.Is(err, ErrCommandNotFound) {
		t.Errorf("GetCommand(deleted) error = %v, want ErrCommandNotFound", err)
	}
}
