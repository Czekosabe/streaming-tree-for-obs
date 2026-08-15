package visualtemplate

// RuleAudioPreset is a template-scoped variant of internal/domain/
// alerts.RuleAudio (docs/alert-audio.md §10.5) - it references a
// template-owned managed audio asset rather than a live alert rule's
// own asset. Kept as its own small type, field-identical to RuleAudio
// by convention, precisely so this package never needs to import
// internal/domain/alerts for the shared shape (this package's own top
// doc comment) - the same deliberate parallel-but-independent
// relationship alerts.Capability/audio.Capability already have. Only a
// package v2 import (visualpackage's own alertAudio manifest object)
// ever populates this on a Template - the plain Stage 14A JSON create/
// import path never carries audio (docs/alert-audio.md §10.7).
type RuleAudioPreset struct {
	SoundEnabled bool
	SoundAssetID string
	SoundVolume  float64

	TTSEnabled  bool
	TTSTemplate string
	TTSVolume   float64
}

// Bounds mirroring internal/domain/alerts.RuleAudio's own bounds
// exactly (docs/alert-audio.md §6.3/§10.2) - kept in sync by
// convention rather than by import, since this package may never
// depend on internal/domain/alerts.
const (
	MinRuleAudioVolume     = 0.0
	MaxRuleAudioVolume     = 1.0
	DefaultRuleAudioVolume = 1.0

	// MaxTTSTemplateCodePoints mirrors alerts.MaxTemplateCodePoints
	// exactly - the identical bound the live rule-owned TTS template
	// field uses.
	MaxTTSTemplateCodePoints = 500
)

// ValidateRuleAudioPreset checks a's own structural bounds - volume
// ranges and the enabled/required-field mode matrix - mirroring
// alerts.ValidateRuleAudio's own rules exactly (docs/alert-audio.md
// §10.2: "validated with the exact same validator §7 uses for a live
// rule's audio object"). A nil a is always valid (no preset
// configured). It does NOT check that SoundAssetID actually names a
// real, existing audio asset (that needs an injected lookup - see
// Service.create) and does NOT check TTSTemplate's placeholder syntax
// (internal/alerts's own template parser is the one shared grammar,
// and this package may never import internal/alerts either - the
// httpapi bridge layer, which already imports both, is responsible for
// that check on package import, exactly like it already is for a live
// rule's own TTSTemplate).
func ValidateRuleAudioPreset(a *RuleAudioPreset) error {
	if a == nil {
		return nil
	}
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
		n := utf8RuneCount(a.TTSTemplate)
		if n == 0 {
			return validationErr("a TTS template must be set when the audio preset's TTS is enabled")
		}
		if n > MaxTTSTemplateCodePoints {
			return validationErr("TTS template must not exceed %d characters", MaxTTSTemplateCodePoints)
		}
	}
	if !a.TTSEnabled && a.TTSTemplate != "" {
		return validationErr("a TTS template must not be set while the audio preset's TTS is disabled")
	}
	return nil
}

func utf8RuneCount(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

// HasAudio reports whether a carries any actual configured audio - the
// same "sound or TTS, either enables package-only export" condition
// docs/alert-audio.md §10.7 defines. Exported since both this package's
// own export-format gating (validation.go) and visualpackage.Service.
// ExportTemplate's own manifest-schema-version gating need the
// identical condition.
func (a *RuleAudioPreset) HasAudio() bool {
	return a != nil && (a.SoundEnabled || a.TTSEnabled)
}
