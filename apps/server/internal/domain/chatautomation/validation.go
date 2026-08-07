package chatautomation

import (
	"fmt"
	"strings"
)

// Conservative bounds - see the Stage 11B task's own Part 5 and Part 12.
const (
	MaxNameCodePoints        = 80
	MaxMessagesPerSchedule   = 20
	MaxTemplateCodePoints    = 500
	MinIntervalSeconds       = 60
	MaxIntervalSeconds       = 24 * 60 * 60
	MaxFirstDelaySeconds     = 24 * 60 * 60
	MaxJitterSeconds         = 15 * 60
	MaxMinimumChatMessages   = 1000
	MinMaximumSendsPerHour   = 1
	MaxMaximumSendsPerHour   = 60
	MinCommandNameLength     = 1
	MaxCommandNameLength     = 32
	MaxGlobalCooldownSeconds = 3600
	MaxUserCooldownSeconds   = 24 * 60 * 60
)

func validationErr(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrValidation, fmt.Sprintf(format, args...))
}

func codePointLen(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

// ValidateName checks a schedule's display name.
func ValidateName(name string) error {
	n := codePointLen(name)
	if n < 1 || n > MaxNameCodePoints {
		return validationErr("name must be 1-%d characters", MaxNameCodePoints)
	}
	return nil
}

// ValidateScheduleTiming checks interval/first-delay/jitter/minimum-
// activity/max-per-hour bounds.
func ValidateScheduleTiming(intervalSeconds, firstDelaySeconds, jitterSeconds, minimumChatMessages, maximumSendsPerHour int) error {
	if intervalSeconds < MinIntervalSeconds || intervalSeconds > MaxIntervalSeconds {
		return validationErr("interval must be between %d and %d seconds", MinIntervalSeconds, MaxIntervalSeconds)
	}
	if firstDelaySeconds < 0 || firstDelaySeconds > MaxFirstDelaySeconds {
		return validationErr("first delay must be between 0 and %d seconds", MaxFirstDelaySeconds)
	}
	if jitterSeconds < 0 || jitterSeconds > MaxJitterSeconds {
		return validationErr("jitter must be between 0 and %d seconds", MaxJitterSeconds)
	}
	if minimumChatMessages < 0 || minimumChatMessages > MaxMinimumChatMessages {
		return validationErr("minimum chat messages must be between 0 and %d", MaxMinimumChatMessages)
	}
	if maximumSendsPerHour < MinMaximumSendsPerHour || maximumSendsPerHour > MaxMaximumSendsPerHour {
		return validationErr("maximum sends per hour must be between %d and %d", MinMaximumSendsPerHour, MaxMaximumSendsPerHour)
	}
	return nil
}

// ValidateTemplate checks a stored template's own length, before
// placeholder expansion. The rendered output is checked separately,
// after expansion, against the outbound provider's own limit - see
// internal/chatautomation/placeholders.go.
func ValidateTemplate(template string) error {
	if codePointLen(strings.TrimSpace(template)) == 0 {
		return validationErr("template must not be empty")
	}
	if codePointLen(template) > MaxTemplateCodePoints {
		return validationErr("template must not exceed %d characters", MaxTemplateCodePoints)
	}
	return nil
}

// ValidateMessages checks a schedule's full message-alternative group.
func ValidateMessages(templates []string) error {
	if len(templates) == 0 {
		return ErrMessageRequired
	}
	if len(templates) > MaxMessagesPerSchedule {
		return validationErr("a schedule may have at most %d message alternatives", MaxMessagesPerSchedule)
	}
	for _, t := range templates {
		if err := ValidateTemplate(t); err != nil {
			return err
		}
	}
	return nil
}

// ValidateTargets checks that at least one target account was supplied
// and that no account is duplicated.
func ValidateTargets(targets []Target) error {
	if len(targets) == 0 {
		return ErrTargetRequired
	}
	seen := make(map[string]bool, len(targets))
	for _, t := range targets {
		if t.AccountID == "" {
			return validationErr("target account id must not be empty")
		}
		if seen[t.AccountID] {
			return validationErr("target account %s is duplicated", t.AccountID)
		}
		seen[t.AccountID] = true
	}
	return nil
}

// commandNamePattern: ASCII lowercase letters, digits, '_' and '-' only.
// Deliberately rejects whitespace, "!", "/", punctuation and Unicode
// lookalike characters - see the Stage 11B task's own Part 12 reasoning
// ("reduces ambiguity and spoofing").
func isValidCommandNameByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '_' || b == '-'
}

// ValidateCommandName checks a canonical command name or an alias -
// both use the same rule. name must already be lowercase; callers
// lowercase user input before validating (see Service.normalizeCommandName).
func ValidateCommandName(name string) error {
	if len(name) < MinCommandNameLength || len(name) > MaxCommandNameLength {
		return validationErr("command name must be %d-%d characters", MinCommandNameLength, MaxCommandNameLength)
	}
	for i := 0; i < len(name); i++ {
		if !isValidCommandNameByte(name[i]) {
			return validationErr("command name may only contain a-z, 0-9, '_' and '-'")
		}
	}
	return nil
}

// NormalizeCommandName lowercases name using simple ASCII-only folding
// (command names are validated ASCII-only, so no Unicode case-folding
// concern applies) and trims surrounding whitespace before validation.
func NormalizeCommandName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// ValidateAliases checks a command's full alias list: each alias must
// pass ValidateCommandName, none may equal the canonical name, and none
// may repeat within the same command.
func ValidateAliases(name string, aliases []string) error {
	seen := map[string]bool{name: true}
	for _, alias := range aliases {
		if err := ValidateCommandName(alias); err != nil {
			return err
		}
		if seen[alias] {
			return validationErr("alias %q is duplicated", alias)
		}
		seen[alias] = true
	}
	return nil
}

// ValidateRole checks a command's required role against the closed enum.
func ValidateRole(r Role) error {
	if !r.valid() {
		return validationErr("required role %q is not a recognized role", string(r))
	}
	return nil
}

// ValidateCooldowns checks a command's global/per-user cooldown bounds.
func ValidateCooldowns(globalSeconds, userSeconds int) error {
	if globalSeconds < 0 || globalSeconds > MaxGlobalCooldownSeconds {
		return validationErr("global cooldown must be between 0 and %d seconds", MaxGlobalCooldownSeconds)
	}
	if userSeconds < 0 || userSeconds > MaxUserCooldownSeconds {
		return validationErr("per-user cooldown must be between 0 and %d seconds", MaxUserCooldownSeconds)
	}
	return nil
}
