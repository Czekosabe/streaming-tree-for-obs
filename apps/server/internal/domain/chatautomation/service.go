package chatautomation

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Clock returns the current time; injected so tests are deterministic.
type Clock func() time.Time

// AccountLookup resolves facts about a connected account needed to
// validate an explicit target - never the account's token, never its
// full record. Deliberately a narrow, primitive-typed interface rather
// than importing internal/domain/account's concrete Service, mirroring
// how every domain package in this project stays decoupled from every
// other one.
type AccountLookup interface {
	// AccountProviderID returns the account's provider id (e.g.
	// "twitch"), or found=false if no such account exists.
	AccountProviderID(ctx context.Context, accountID string) (providerID string, found bool, err error)
}

// PlatformLookup resolves facts about a configured destination platform
// needed to validate an explicit platform-context target.
type PlatformLookup interface {
	// PlatformProviderID returns the platform's provider id, or
	// found=false if no such platform exists.
	PlatformProviderID(ctx context.Context, platformID string) (providerID string, found bool, err error)
	// PlatformLinkedAccountID returns the connected-account id currently
	// linked to platformID, or linked=false if none is.
	PlatformLinkedAccountID(ctx context.Context, platformID string) (accountID string, linked bool, err error)
}

// Service holds the chat-automation use cases: schedule and command
// CRUD, with the target/platform-context and global command-uniqueness
// validation the Stage 11B task requires. Never runs a schedule or
// matches a command itself - see internal/chatautomation's runtime
// Manager for that.
type Service struct {
	repo      Repository
	accounts  AccountLookup
	platforms PlatformLookup
	now       Clock
}

// NewService builds a Service.
func NewService(repo Repository, accounts AccountLookup, platforms PlatformLookup, now Clock) *Service {
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{repo: repo, accounts: accounts, platforms: platforms, now: now}
}

func mapRepoErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrScheduleNotFound) || errors.Is(err, ErrCommandNotFound) ||
		errors.Is(err, ErrAccountNotFound) || errors.Is(err, ErrPlatformNotFound) ||
		errors.Is(err, ErrCommandNameConflict) {
		return err
	}
	return fmt.Errorf("%w: %s", ErrStorage, err)
}

// validateTargets checks the shared target-list rules (Part 5/12: at
// least one target, no duplicates) plus, for every target that supplies
// an explicit platform_id, the deterministic-context rules from Part 4:
// the platform must exist, must share the target account's own
// provider, and must currently be linked to that same account.
func (s *Service) validateTargets(ctx context.Context, targets []Target) error {
	if err := ValidateTargets(targets); err != nil {
		return err
	}
	for _, t := range targets {
		accountProviderID, found, err := s.accounts.AccountProviderID(ctx, t.AccountID)
		if err != nil {
			return fmt.Errorf("%w: %s", ErrStorage, err)
		}
		if !found {
			return ErrAccountNotFound
		}
		if t.PlatformID == "" {
			continue
		}
		platformProviderID, found, err := s.platforms.PlatformProviderID(ctx, t.PlatformID)
		if err != nil {
			return fmt.Errorf("%w: %s", ErrStorage, err)
		}
		if !found {
			return ErrPlatformNotFound
		}
		if platformProviderID != accountProviderID {
			return ErrPlatformProviderMismatch
		}
		linkedAccountID, linked, err := s.platforms.PlatformLinkedAccountID(ctx, t.PlatformID)
		if err != nil {
			return fmt.Errorf("%w: %s", ErrStorage, err)
		}
		if !linked || linkedAccountID != t.AccountID {
			return ErrPlatformNotLinked
		}
	}
	return nil
}

// --- schedules --------------------------------------------------------

// ScheduleInput carries a schedule's editable fields, before persistence
// identifiers exist.
type ScheduleInput struct {
	Name                     string
	Enabled                  bool
	IntervalSeconds          int
	FirstDelaySeconds        int
	JitterSeconds            int
	OnlyWhileIngestReceiving bool
	MinimumChatMessages      int
	MaximumSendsPerHour      int
	Targets                  []Target
	MessageTemplates         []string
}

func (s *Service) validateScheduleInput(ctx context.Context, in ScheduleInput) error {
	if err := ValidateName(in.Name); err != nil {
		return err
	}
	if err := ValidateScheduleTiming(in.IntervalSeconds, in.FirstDelaySeconds, in.JitterSeconds, in.MinimumChatMessages, in.MaximumSendsPerHour); err != nil {
		return err
	}
	if err := ValidateMessages(in.MessageTemplates); err != nil {
		return err
	}
	return s.validateTargets(ctx, in.Targets)
}

// CreateSchedule validates and persists a new schedule definition.
func (s *Service) CreateSchedule(ctx context.Context, in ScheduleInput) (Schedule, error) {
	if err := s.validateScheduleInput(ctx, in); err != nil {
		return Schedule{}, err
	}
	id, err := NewScheduleID()
	if err != nil {
		return Schedule{}, fmt.Errorf("%w: %s", ErrStorage, err)
	}
	messages, err := s.buildMessages(in.MessageTemplates)
	if err != nil {
		return Schedule{}, err
	}
	sch := Schedule{
		ID: id, Name: in.Name, Enabled: in.Enabled,
		IntervalSeconds: in.IntervalSeconds, FirstDelaySeconds: in.FirstDelaySeconds, JitterSeconds: in.JitterSeconds,
		OnlyWhileIngestReceiving: in.OnlyWhileIngestReceiving, MinimumChatMessages: in.MinimumChatMessages,
		MaximumSendsPerHour: in.MaximumSendsPerHour, Targets: in.Targets, Messages: messages,
	}
	saved, err := s.repo.CreateSchedule(ctx, sch)
	if err != nil {
		return Schedule{}, mapRepoErr(err)
	}
	return saved, nil
}

func (s *Service) buildMessages(templates []string) ([]ScheduleMessage, error) {
	messages := make([]ScheduleMessage, 0, len(templates))
	for i, t := range templates {
		id, err := NewScheduleMessageID()
		if err != nil {
			return nil, fmt.Errorf("%w: %s", ErrStorage, err)
		}
		messages = append(messages, ScheduleMessage{ID: id, MessageTemplate: t, Position: i})
	}
	return messages, nil
}

// GetSchedule returns one schedule by id.
func (s *Service) GetSchedule(ctx context.Context, id string) (Schedule, error) {
	sch, found, err := s.repo.GetSchedule(ctx, id)
	if err != nil {
		return Schedule{}, mapRepoErr(err)
	}
	if !found {
		return Schedule{}, ErrScheduleNotFound
	}
	return sch, nil
}

// ListSchedules returns every schedule.
func (s *Service) ListSchedules(ctx context.Context) ([]Schedule, error) {
	list, err := s.repo.ListSchedules(ctx)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	return list, nil
}

// ReplaceSchedule validates and stores a full replacement of one
// schedule's editable fields, target set, and message set.
func (s *Service) ReplaceSchedule(ctx context.Context, id string, in ScheduleInput) (Schedule, error) {
	if _, found, err := s.repo.GetSchedule(ctx, id); err != nil {
		return Schedule{}, mapRepoErr(err)
	} else if !found {
		return Schedule{}, ErrScheduleNotFound
	}
	if err := s.validateScheduleInput(ctx, in); err != nil {
		return Schedule{}, err
	}
	messages, err := s.buildMessages(in.MessageTemplates)
	if err != nil {
		return Schedule{}, err
	}
	sch := Schedule{
		ID: id, Name: in.Name, Enabled: in.Enabled,
		IntervalSeconds: in.IntervalSeconds, FirstDelaySeconds: in.FirstDelaySeconds, JitterSeconds: in.JitterSeconds,
		OnlyWhileIngestReceiving: in.OnlyWhileIngestReceiving, MinimumChatMessages: in.MinimumChatMessages,
		MaximumSendsPerHour: in.MaximumSendsPerHour, Targets: in.Targets, Messages: messages,
	}
	saved, err := s.repo.UpdateSchedule(ctx, sch)
	if err != nil {
		return Schedule{}, mapRepoErr(err)
	}
	return saved, nil
}

// DeleteSchedule removes a schedule and every related row.
func (s *Service) DeleteSchedule(ctx context.Context, id string) error {
	if err := s.repo.DeleteSchedule(ctx, id); err != nil {
		return mapRepoErr(err)
	}
	return nil
}

// --- commands -----------------------------------------------------------

// CommandInput carries a command's editable fields, before persistence
// identifiers exist.
type CommandInput struct {
	Name                  string
	Enabled               bool
	ResponseTemplate      string
	RequiredRole          Role
	GlobalCooldownSeconds int
	UserCooldownSeconds   int
	Aliases               []string
	Targets               []Target
}

func (s *Service) validateCommandInput(ctx context.Context, in CommandInput, excludeCommandID string) (name string, aliases []string, err error) {
	name = NormalizeCommandName(in.Name)
	if err = ValidateCommandName(name); err != nil {
		return "", nil, err
	}
	aliases = make([]string, len(in.Aliases))
	for i, a := range in.Aliases {
		aliases[i] = NormalizeCommandName(a)
	}
	if err = ValidateAliases(name, aliases); err != nil {
		return "", nil, err
	}
	if err = ValidateTemplate(in.ResponseTemplate); err != nil {
		return "", nil, err
	}
	if err = ValidateRole(in.RequiredRole); err != nil {
		return "", nil, err
	}
	if err = ValidateCooldowns(in.GlobalCooldownSeconds, in.UserCooldownSeconds); err != nil {
		return "", nil, err
	}
	if err = s.validateTargets(ctx, in.Targets); err != nil {
		return "", nil, err
	}

	inUse, err := s.repo.NameOrAliasInUse(ctx, name, excludeCommandID)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %s", ErrStorage, err)
	}
	if inUse {
		return "", nil, ErrCommandNameConflict
	}
	for _, alias := range aliases {
		inUse, err := s.repo.NameOrAliasInUse(ctx, alias, excludeCommandID)
		if err != nil {
			return "", nil, fmt.Errorf("%w: %s", ErrStorage, err)
		}
		if inUse {
			return "", nil, ErrCommandNameConflict
		}
	}
	return name, aliases, nil
}

// CreateCommand validates and persists a new command definition.
func (s *Service) CreateCommand(ctx context.Context, in CommandInput) (Command, error) {
	name, aliases, err := s.validateCommandInput(ctx, in, "")
	if err != nil {
		return Command{}, err
	}
	id, err := NewCommandID()
	if err != nil {
		return Command{}, fmt.Errorf("%w: %s", ErrStorage, err)
	}
	cmd := Command{
		ID: id, Name: name, Enabled: in.Enabled, ResponseTemplate: in.ResponseTemplate, RequiredRole: in.RequiredRole,
		GlobalCooldownSeconds: in.GlobalCooldownSeconds, UserCooldownSeconds: in.UserCooldownSeconds,
		Aliases: aliases, Targets: in.Targets,
	}
	saved, err := s.repo.CreateCommand(ctx, cmd)
	if err != nil {
		return Command{}, mapRepoErr(err)
	}
	return saved, nil
}

// GetCommand returns one command by id.
func (s *Service) GetCommand(ctx context.Context, id string) (Command, error) {
	cmd, found, err := s.repo.GetCommand(ctx, id)
	if err != nil {
		return Command{}, mapRepoErr(err)
	}
	if !found {
		return Command{}, ErrCommandNotFound
	}
	return cmd, nil
}

// ListCommands returns every command.
func (s *Service) ListCommands(ctx context.Context) ([]Command, error) {
	list, err := s.repo.ListCommands(ctx)
	if err != nil {
		return nil, mapRepoErr(err)
	}
	return list, nil
}

// ReplaceCommand validates and stores a full replacement of one
// command's editable fields, alias set, and target set.
func (s *Service) ReplaceCommand(ctx context.Context, id string, in CommandInput) (Command, error) {
	if _, found, err := s.repo.GetCommand(ctx, id); err != nil {
		return Command{}, mapRepoErr(err)
	} else if !found {
		return Command{}, ErrCommandNotFound
	}
	name, aliases, err := s.validateCommandInput(ctx, in, id)
	if err != nil {
		return Command{}, err
	}
	cmd := Command{
		ID: id, Name: name, Enabled: in.Enabled, ResponseTemplate: in.ResponseTemplate, RequiredRole: in.RequiredRole,
		GlobalCooldownSeconds: in.GlobalCooldownSeconds, UserCooldownSeconds: in.UserCooldownSeconds,
		Aliases: aliases, Targets: in.Targets,
	}
	saved, err := s.repo.UpdateCommand(ctx, cmd)
	if err != nil {
		return Command{}, mapRepoErr(err)
	}
	return saved, nil
}

// DeleteCommand removes a command and every related row.
func (s *Service) DeleteCommand(ctx context.Context, id string) error {
	if err := s.repo.DeleteCommand(ctx, id); err != nil {
		return mapRepoErr(err)
	}
	return nil
}
