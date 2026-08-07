package chatautomation

import "context"

// Repository is the persistence port for schedule and command
// definitions, including their targets, message alternatives, and
// aliases. Every write that touches more than one table is atomic (see
// the sqlite implementation) - a caller never observes a schedule with
// targets but no messages, or a command with an alias but no canonical
// row.
type Repository interface {
	CreateSchedule(ctx context.Context, s Schedule) (Schedule, error)
	GetSchedule(ctx context.Context, id string) (Schedule, bool, error)
	ListSchedules(ctx context.Context) ([]Schedule, error)
	// UpdateSchedule replaces every editable field, its full target set,
	// and its full message set - never a partial patch. The id and
	// created_at are unchanged.
	UpdateSchedule(ctx context.Context, s Schedule) (Schedule, error)
	// DeleteSchedule removes a schedule; its targets and messages
	// cascade.
	DeleteSchedule(ctx context.Context, id string) error

	CreateCommand(ctx context.Context, c Command) (Command, error)
	GetCommand(ctx context.Context, id string) (Command, bool, error)
	ListCommands(ctx context.Context) ([]Command, error)
	// UpdateCommand replaces every editable field, its full alias set,
	// and its full target set - never a partial patch.
	UpdateCommand(ctx context.Context, c Command) (Command, error)
	// DeleteCommand removes a command; its aliases and targets cascade.
	DeleteCommand(ctx context.Context, id string) error

	// NameOrAliasInUse reports whether name already names a different
	// command's canonical name or any command's alias, anywhere in the
	// application - see the Stage 11B task's own global-uniqueness
	// requirement. excludeCommandID is the command currently being
	// saved (empty on create), so renaming a command to its own current
	// name is never rejected as a conflict with itself.
	NameOrAliasInUse(ctx context.Context, name, excludeCommandID string) (bool, error)
}
