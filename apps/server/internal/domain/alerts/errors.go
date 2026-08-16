package alerts

import "errors"

var (
	// ErrStorage wraps any unexpected persistence failure.
	ErrStorage = errors.New("storage failure")

	// ErrProfileNotFound means the referenced alert profile does not exist.
	ErrProfileNotFound = errors.New("alert profile not found")
	// ErrPublicSlugNotFound means no profile currently has the given
	// public slug (never distinguished from "disabled" at this layer -
	// see internal/httpapi/alerts.go for why the public API deliberately
	// treats both the same way).
	ErrPublicSlugNotFound = errors.New("alert profile public slug not found")
	// ErrRuleNotFound means the referenced alert rule does not exist.
	ErrRuleNotFound = errors.New("alert rule not found")

	// ErrAccountNotFound means a rule's account filter names a connected
	// account that does not exist.
	ErrAccountNotFound = errors.New("connected account not found")

	// ErrValidation wraps a profile-level semantic validation failure
	// (bounds, required fields) - see validation.go for the exact rules.
	// Never returned for a rule-level failure - see ErrRuleInvalid.
	ErrValidation = errors.New("alert validation failed")
	// ErrRuleInvalid wraps a rule-level semantic validation failure
	// (bounds, required fields, enum values) that has no more specific
	// sentinel of its own - see validation.go for the exact rules. Kept
	// distinct from the profile-level ErrValidation above so
	// internal/httpapi/alerts.go's own error mapping never reports a
	// rule validation failure as "alert_profile_invalid" (a real,
	// discovered bug: both used to share ErrValidation, so a rule-level
	// 422 - e.g. Stage 17B's own RuleAudio bound checks - was reported
	// with the wrong, profile-shaped stable error code and message).
	ErrRuleInvalid = errors.New("alert rule validation failed")
	// ErrConditionUnsupported means a rule set a condition (quantity
	// threshold, role, a visibility toggle) that its own event type's
	// capability does not support - see capability.go and the Stage 12A
	// task's own Part 6.
	ErrConditionUnsupported = errors.New("alert rule condition is not supported by this event type")
	// ErrThresholdInvalid means a rule's minimum/maximum quantity bounds
	// are individually out of range, or minimum > maximum.
	ErrThresholdInvalid = errors.New("alert rule quantity threshold is invalid")
	// ErrMoneyThresholdInvalid means a rule's minimum/maximum amount
	// bounds are individually invalid, minimum > maximum, or an amount
	// threshold was set with no currency - see ValidateMoneyThresholds.
	ErrMoneyThresholdInvalid = errors.New("alert rule money threshold is invalid")
	// ErrTemplateInvalid means a rule's text template is malformed or
	// uses an unknown/unsupported placeholder - see templates.go.
	ErrTemplateInvalid = errors.New("alert template is invalid")

	// ErrAudioAssetNotFound means a rule's Stage 17B SoundAssetID names
	// a managed audio asset that does not exist (docs/alert-audio.md
	// §7) - stable API error audio_rule_asset_not_found.
	ErrAudioAssetNotFound = errors.New("alert rule audio asset not found")
)
