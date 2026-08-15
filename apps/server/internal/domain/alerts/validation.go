package alerts

import (
	"fmt"
)

// Conservative bounds - see the Stage 12A task's own Part 5, Part 13,
// Part 14, and Part 25.
const (
	MaxNameCodePoints = 80

	MinPriority     = 0
	MaxPriority     = 100
	DefaultPriority = 50

	MinDurationMS     = 1000
	MaxDurationMS     = 30000
	DefaultDurationMS = 5000

	MinAnimationDurationMS     = 0
	MaxAnimationDurationMS     = 2000
	DefaultAnimationDurationMS = 400

	DefaultMaxQueueItems = 100
	MinMaxQueueItems     = 1
	MaxMaxQueueItems     = 500

	DefaultMaximumQueueAgeSeconds = 120
	MinMaximumQueueAgeSeconds     = 5
	MaxMaximumQueueAgeSeconds     = 3600

	MaxTemplateCodePoints = 500

	// Stage 17B (docs/alert-audio.md §6.3): rule-owned audio volume
	// bounds. TTSTemplate reuses MaxTemplateCodePoints directly - the
	// identical bound as the visual TextTemplate, never a separate one.
	MinRuleAudioVolume     = 0.0
	MaxRuleAudioVolume     = 1.0
	DefaultRuleAudioVolume = 1.0
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

// ValidateProfileName checks a profile's display name.
func ValidateProfileName(name string) error {
	n := codePointLen(name)
	if n < 1 || n > MaxNameCodePoints {
		return validationErr("name must be 1-%d characters", MaxNameCodePoints)
	}
	return nil
}

// ValidateProfileFields checks a profile's queue bounds, language, and
// fixed presentation choices.
func ValidateProfileFields(p Profile) error {
	if err := ValidateProfileName(p.Name); err != nil {
		return err
	}
	if !p.Language.valid() {
		return validationErr("language %q is not supported", string(p.Language))
	}
	if !p.Theme.valid() {
		return validationErr("theme %q is not a recognized theme", string(p.Theme))
	}
	if !p.Position.valid() {
		return validationErr("position %q is not a recognized position", string(p.Position))
	}
	if !p.TextAlign.valid() {
		return validationErr("text alignment %q is not recognized", string(p.TextAlign))
	}
	if p.MaxQueueItems < MinMaxQueueItems || p.MaxQueueItems > MaxMaxQueueItems {
		return validationErr("max queue items must be between %d and %d", MinMaxQueueItems, MaxMaxQueueItems)
	}
	if p.MaximumQueueAgeSeconds < MinMaximumQueueAgeSeconds || p.MaximumQueueAgeSeconds > MaxMaximumQueueAgeSeconds {
		return validationErr("maximum queue age must be between %d and %d seconds", MinMaximumQueueAgeSeconds, MaxMaximumQueueAgeSeconds)
	}
	return nil
}

// ValidateTemplate checks a rule's stored template length only, before
// placeholder expansion - unknown/malformed placeholder names are
// validated separately by internal/alerts's own template parser at the
// HTTP boundary, mirroring internal/httpapi/chatautomation.go's own
// validateTemplatesKnown split between domain-layer length checks and
// runtime-layer placeholder checks.
func ValidateTemplate(template string) error {
	n := codePointLen(template)
	if n == 0 {
		return validationErr("template must not be empty")
	}
	if n > MaxTemplateCodePoints {
		return validationErr("template must not exceed %d characters", MaxTemplateCodePoints)
	}
	return nil
}

// ValidateRuleAudio checks a rule's Stage 17B audio configuration's own
// structural bounds (docs/alert-audio.md §6/§7): volume ranges, and the
// enabled/required-field mode matrix (a sound asset id is required when
// SoundEnabled, a TTS template is required when TTSEnabled). It does
// **not** check that SoundAssetID actually names a real, existing audio
// asset (that needs an injected lookup - see Service.validateRuleInput)
// and does **not** check TTSTemplate's placeholder syntax/capability
// availability (that lives in the sibling runtime package
// internal/alerts's own template parser, validated at the HTTP boundary
// - mirroring ValidateTemplate's own identical domain/runtime split,
// see its own doc comment above).
func ValidateRuleAudio(a RuleAudio) error {
	if a.SoundVolume < MinRuleAudioVolume || a.SoundVolume > MaxRuleAudioVolume {
		return validationErr("sound volume must be between %.1f and %.1f", MinRuleAudioVolume, MaxRuleAudioVolume)
	}
	if a.TTSVolume < MinRuleAudioVolume || a.TTSVolume > MaxRuleAudioVolume {
		return validationErr("TTS volume must be between %.1f and %.1f", MinRuleAudioVolume, MaxRuleAudioVolume)
	}
	if a.SoundEnabled && a.SoundAssetID == "" {
		return validationErr("a sound asset must be selected when sound is enabled")
	}
	if !a.SoundEnabled && a.SoundAssetID != "" {
		return validationErr("a sound asset must not be set while sound is disabled")
	}
	if a.TTSEnabled {
		n := codePointLen(a.TTSTemplate)
		if n == 0 {
			return validationErr("a TTS template must be set when rule-owned TTS is enabled")
		}
		if n > MaxTemplateCodePoints {
			return validationErr("TTS template must not exceed %d characters", MaxTemplateCodePoints)
		}
	}
	if !a.TTSEnabled && a.TTSTemplate != "" {
		return validationErr("a TTS template must not be set while rule-owned TTS is disabled")
	}
	return nil
}

// ValidateThresholds checks a rule's quantity bounds: each side, when
// set, must be non-negative, and minimum must not exceed maximum.
func ValidateThresholds(minimum, maximum *int64) error {
	if minimum != nil && *minimum < 0 {
		return fmt.Errorf("%w: minimum quantity must not be negative", ErrThresholdInvalid)
	}
	if maximum != nil && *maximum < 0 {
		return fmt.Errorf("%w: maximum quantity must not be negative", ErrThresholdInvalid)
	}
	if minimum != nil && maximum != nil && *minimum > *maximum {
		return fmt.Errorf("%w: minimum quantity must not exceed maximum quantity", ErrThresholdInvalid)
	}
	return nil
}

// maxAmountMicros mirrors internal/domain/engagement's own bound - a
// threshold this application will never need to compare a real provider
// amount against beyond.
const maxAmountMicros int64 = 1_000_000_000_000 // 1,000,000.000000 major units

// NormalizeCurrency uppercases a currency code exactly like
// engagement.NewMoney does, so a rule's stored Currency always matches
// the case a normalized Money.Currency will actually carry - an
// exact-match comparison at match time never fails only because of case.
func NormalizeCurrency(currency string) string {
	b := []byte(currency)
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			b[i] = c - ('a' - 'A')
		}
	}
	return string(b)
}

// ValidateMoneyThresholds checks a rule's amount bounds (Stage 15A task
// Part 38): each side, when set, must be non-negative and within the
// supported bound; minimum must not exceed maximum; and setting either
// threshold requires a non-empty currency, since this application never
// compares an amount across currencies (no threshold is meaningful
// without knowing which currency it is bounding).
func ValidateMoneyThresholds(currency string, minimum, maximum *int64) error {
	if minimum == nil && maximum == nil {
		return nil
	}
	if currency == "" {
		return fmt.Errorf("%w: an amount threshold requires a currency", ErrMoneyThresholdInvalid)
	}
	if minimum != nil && (*minimum < 0 || *minimum > maxAmountMicros) {
		return fmt.Errorf("%w: minimum amount is out of range", ErrMoneyThresholdInvalid)
	}
	if maximum != nil && (*maximum < 0 || *maximum > maxAmountMicros) {
		return fmt.Errorf("%w: maximum amount is out of range", ErrMoneyThresholdInvalid)
	}
	if minimum != nil && maximum != nil && *minimum > *maximum {
		return fmt.Errorf("%w: minimum amount must not exceed maximum amount", ErrMoneyThresholdInvalid)
	}
	return nil
}

// ValidateAccounts checks a rule's account-filter list has no duplicate
// entries. Existence of each account id is checked separately by
// Service.validateRuleInput, which needs an AccountLookup to do so.
func ValidateAccounts(accountIDs []string) error {
	seen := make(map[string]bool, len(accountIDs))
	for _, id := range accountIDs {
		if id == "" {
			return validationErr("account id must not be empty")
		}
		if seen[id] {
			return validationErr("account %s is duplicated in the filter", id)
		}
		seen[id] = true
	}
	return nil
}

// ValidateProviders checks a rule's provider-filter list: every entry
// must be a recognized provider, with no duplicates.
func ValidateProviders(providerIDs []ProviderID) error {
	seen := make(map[ProviderID]bool, len(providerIDs))
	for _, p := range providerIDs {
		if p != ProviderTwitch && p != ProviderYouTube && p != ProviderStreamElements {
			return validationErr("provider %q is not supported", string(p))
		}
		if seen[p] {
			return validationErr("provider %s is duplicated in the filter", p)
		}
		seen[p] = true
	}
	return nil
}

// ValidateRuleConditions is the capability-driven check from the Stage
// 12A task's own Part 6: a rule may only set a quantity threshold, a
// non-default visibility toggle, or a non-"everyone" role when its own
// event type's Capability actually supports it. An explicitly submitted
// impossible value is rejected (422) rather than silently forced to a
// safe default, per the task's own stated preference.
func ValidateRuleConditions(r Rule) error {
	if !r.EventType.valid() {
		return validationErr("event type %q is not a recognized alert event type", string(r.EventType))
	}
	capability := CapabilityFor(r.EventType)

	if (r.MinimumQuantity != nil || r.MaximumQuantity != nil) && !capability.HasQuantity {
		return fmt.Errorf("%w: event type %q has no quantity to threshold", ErrConditionUnsupported, string(r.EventType))
	}
	if r.ShowQuantity && !capability.HasQuantity {
		return fmt.Errorf("%w: event type %q has no quantity to show", ErrConditionUnsupported, string(r.EventType))
	}
	if r.ShowMessage && !capability.HasMessage {
		return fmt.Errorf("%w: event type %q has no message to show", ErrConditionUnsupported, string(r.EventType))
	}
	if r.ShowUsername && !capability.HasUser {
		return fmt.Errorf("%w: event type %q has no user to show", ErrConditionUnsupported, string(r.EventType))
	}
	if (r.MinimumAmountMicros != nil || r.MaximumAmountMicros != nil || r.ShowAmount) && !capability.HasAmount {
		return fmt.Errorf("%w: event type %q has no amount to threshold or show", ErrConditionUnsupported, string(r.EventType))
	}
	if r.RequiredRole != RoleEveryone && !capability.HasRoles {
		return fmt.Errorf("%w: event type %q has no role data to condition on", ErrConditionUnsupported, string(r.EventType))
	}
	if !r.RequiredRole.valid() {
		return validationErr("required role %q is not a recognized role", string(r.RequiredRole))
	}
	if r.AllowGrouping {
		groupCapability := GroupingCapabilityFor(r.EventType)
		if !groupCapability.Groupable {
			return fmt.Errorf("%w: event type %q has no safe grouping strategy", ErrConditionUnsupported, string(r.EventType))
		}
		if groupCapability.RequiresNoMessage && r.ShowMessage {
			return fmt.Errorf("%w: grouping requires showMessage=false for event type %q (never concatenate arbitrary messages)", ErrConditionUnsupported, string(r.EventType))
		}
	}
	return nil
}

// ValidateRuleFields checks a rule's name, priority, duration, animation
// bounds and enum values - everything not already covered by
// ValidateRuleConditions/ValidateThresholds/ValidateTemplate.
func ValidateRuleFields(r Rule) error {
	if n := codePointLen(r.Name); n < 1 || n > MaxNameCodePoints {
		return validationErr("name must be 1-%d characters", MaxNameCodePoints)
	}
	if r.Priority < MinPriority || r.Priority > MaxPriority {
		return validationErr("priority must be between %d and %d", MinPriority, MaxPriority)
	}
	if r.DurationMS < MinDurationMS || r.DurationMS > MaxDurationMS {
		return validationErr("duration must be between %d and %d milliseconds", MinDurationMS, MaxDurationMS)
	}
	if r.AnimationDurationMS < MinAnimationDurationMS || r.AnimationDurationMS > MaxAnimationDurationMS {
		return validationErr("animation duration must be between %d and %d milliseconds", MinAnimationDurationMS, MaxAnimationDurationMS)
	}
	if !r.EntryAnimation.valid() {
		return validationErr("entry animation %q is not recognized", string(r.EntryAnimation))
	}
	if !r.ExitAnimation.valid() {
		return validationErr("exit animation %q is not recognized", string(r.ExitAnimation))
	}
	if r.GroupWindowMS < MinGroupWindowMS || r.GroupWindowMS > MaxGroupWindowMS {
		return validationErr("group window must be between %d and %d milliseconds", MinGroupWindowMS, MaxGroupWindowMS)
	}
	if !r.InterruptMode.valid() {
		return validationErr("interrupt mode %q is not recognized", string(r.InterruptMode))
	}
	if err := ValidateTemplate(r.TextTemplate); err != nil {
		return err
	}
	if err := ValidateRuleAudio(r.Audio); err != nil {
		return err
	}
	if err := ValidateThresholds(r.MinimumQuantity, r.MaximumQuantity); err != nil {
		return err
	}
	if err := ValidateMoneyThresholds(r.Currency, r.MinimumAmountMicros, r.MaximumAmountMicros); err != nil {
		return err
	}
	if err := ValidateProviders(r.Providers); err != nil {
		return err
	}
	if err := ValidateAccounts(r.Accounts); err != nil {
		return err
	}
	return ValidateRuleConditions(r)
}
