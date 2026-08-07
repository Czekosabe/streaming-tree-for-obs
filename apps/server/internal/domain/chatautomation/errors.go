package chatautomation

import "errors"

var (
	// ErrStorage wraps any unexpected persistence failure.
	ErrStorage = errors.New("storage failure")

	// ErrScheduleNotFound means the referenced schedule does not exist.
	ErrScheduleNotFound = errors.New("chat schedule not found")
	// ErrCommandNotFound means the referenced command does not exist.
	ErrCommandNotFound = errors.New("chat command not found")

	// ErrAccountNotFound means a referenced connected account does not
	// exist.
	ErrAccountNotFound = errors.New("connected account not found")
	// ErrPlatformNotFound means a referenced platform_id does not exist.
	ErrPlatformNotFound = errors.New("platform not found")
	// ErrPlatformProviderMismatch means an explicit platform_id target
	// context uses a different provider than the target account.
	ErrPlatformProviderMismatch = errors.New("platform provider does not match the target account")
	// ErrPlatformNotLinked means an explicit platform_id target context
	// is not actually linked to the target account.
	ErrPlatformNotLinked = errors.New("platform is not linked to the target account")

	// ErrValidation wraps any semantic validation failure (bounds,
	// required fields) - see validation.go for the exact rules.
	ErrValidation = errors.New("chat automation validation failed")
	// ErrTargetRequired means a schedule or command was saved with no
	// target account at all.
	ErrTargetRequired = errors.New("at least one target account is required")
	// ErrMessageRequired means a schedule was saved with no message
	// alternative at all.
	ErrMessageRequired = errors.New("at least one message is required")

	// ErrCommandNameConflict means a command's canonical name or one of
	// its aliases already names a different command or alias somewhere
	// in the application - see the Stage 11B task's own "require all
	// command names and aliases to be globally unique" requirement.
	ErrCommandNameConflict = errors.New("command name or alias is already in use")
)
