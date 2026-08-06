package chatoverlay

import (
	"math"
	"regexp"
	"strings"

	"github.com/streaming-tree/server/internal/domain/platform"
)

// Numeric bounds - see the Stage 10 task's Part 11 and Part 10. Rejecting
// out-of-range values (rather than clamping) means a client's request and
// the saved result are never silently different.
const (
	MaxNameLength = 80

	MinVisibleItems = 1
	MaxVisibleItems = 100

	// A message lifetime of 0 means "no timed expiry" - capacity and
	// moderation still remove items. A non-zero value must be at least 3
	// seconds (anything shorter is indistinguishable from "always expired
	// immediately") and at most 600 (10 minutes - long enough for a slow
	// chat, bounded so the buffer never needs to retain an unbounded
	// history to honor it).
	MinMessageLifetimeSeconds = 3
	MaxMessageLifetimeSeconds = 600

	MinFontSize = 8
	MaxFontSize = 72

	MinFontWeight = 100
	MaxFontWeight = 900

	MinLineHeight = 0.8
	MaxLineHeight = 3.0

	MinBorderRadius = 0
	MaxBorderRadius = 64

	MinItemSpacing = 0
	MaxItemSpacing = 64

	MinAnimationDurationMS = 0
	MaxAnimationDurationMS = 2000

	MinBubbleOpacity = 0.0
	MaxBubbleOpacity = 1.0
)

// hexColorPattern accepts #RRGGBB or #RRGGBBAA only - never an arbitrary
// CSS color expression (named colors, rgb(), hsl(), CSS variables) per
// the Stage 10 task's Part 11.
var hexColorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}([0-9A-Fa-f]{2})?$`)

// IsValidHexColor reports whether a string is a normalized #RRGGBB or
// #RRGGBBAA color.
func IsValidHexColor(value string) bool {
	return hexColorPattern.MatchString(value)
}

var validLayoutModes = map[LayoutMode]bool{LayoutHorizontal: true, LayoutVertical: true}
var validStackDirections = map[StackDirection]bool{StackTopDown: true, StackBottomUp: true}
var validAlignments = map[HorizontalAlignment]bool{AlignLeft: true, AlignCenter: true, AlignRight: true}
var validFontFamilies = map[FontFamily]bool{FontSansSerif: true, FontSerif: true, FontMonospace: true, FontRounded: true}
var validUsernameColorModes = map[UsernameColorMode]bool{UsernameColorProvider: true, UsernameColorFixed: true}
var validAnimations = map[Animation]bool{
	AnimationNone: true, AnimationFade: true, AnimationSlideUp: true, AnimationSlideLeft: true, AnimationScale: true,
}
var validLanguages = map[Language]bool{LanguageEnglish: true, LanguagePolish: true}

// ValidateProfile checks every field of a profile before it is ever
// persisted or used to render a public overlay. Returns a
// *platform.ValidationError (nil when valid), reusing the same
// validation-error shape every other domain package in this project
// already uses, so the HTTP layer needs no new mapping code.
func ValidateProfile(p Profile) error {
	v := &platform.ValidationError{}

	name := strings.TrimSpace(p.Name)
	if name == "" {
		v.Add("name", platform.RuleRequired, "Name is required.", nil)
	} else if len(name) > MaxNameLength {
		v.Addf("name", platform.RuleTooLong, map[string]any{"max": MaxNameLength},
			"Name cannot exceed %d characters.", MaxNameLength)
	}

	if !validLayoutModes[p.LayoutMode] {
		v.Add("layoutMode", platform.RuleUnsupported, "Unsupported layout mode.", nil)
	}
	if !validStackDirections[p.StackDirection] {
		v.Add("stackDirection", platform.RuleUnsupported, "Unsupported stack direction.", nil)
	}
	if !validAlignments[p.HorizontalAlignment] {
		v.Add("horizontalAlignment", platform.RuleUnsupported, "Unsupported horizontal alignment.", nil)
	}
	if !validFontFamilies[p.FontFamily] {
		v.Add("fontFamily", platform.RuleUnsupported, "Unsupported font family.", nil)
	}
	if !validUsernameColorModes[p.UsernameColorMode] {
		v.Add("usernameColorMode", platform.RuleUnsupported, "Unsupported username color mode.", nil)
	}
	if !validAnimations[p.EntryAnimation] {
		v.Add("entryAnimation", platform.RuleUnsupported, "Unsupported entry animation.", nil)
	}
	if !validAnimations[p.ExitAnimation] {
		v.Add("exitAnimation", platform.RuleUnsupported, "Unsupported exit animation.", nil)
	}
	if !validLanguages[p.Language] {
		v.Add("language", platform.RuleUnsupported, "Unsupported language.", nil)
	}

	validateIntRange(v, "maxVisibleItems", p.MaxVisibleItems, MinVisibleItems, MaxVisibleItems)

	if p.MessageLifetimeSeconds != 0 {
		validateIntRange(v, "messageLifetimeSeconds", p.MessageLifetimeSeconds, MinMessageLifetimeSeconds, MaxMessageLifetimeSeconds)
	}

	validateIntRange(v, "fontSize", p.FontSize, MinFontSize, MaxFontSize)
	validateIntRange(v, "fontWeight", p.FontWeight, MinFontWeight, MaxFontWeight)
	validateFloatRange(v, "lineHeight", p.LineHeight, MinLineHeight, MaxLineHeight)
	validateIntRange(v, "borderRadius", p.BorderRadius, MinBorderRadius, MaxBorderRadius)
	validateIntRange(v, "itemSpacing", p.ItemSpacing, MinItemSpacing, MaxItemSpacing)
	validateIntRange(v, "animationDurationMs", p.AnimationDurationMS, MinAnimationDurationMS, MaxAnimationDurationMS)
	validateFloatRange(v, "bubbleOpacity", p.BubbleOpacity, MinBubbleOpacity, MaxBubbleOpacity)

	if !IsValidHexColor(p.TextColor) {
		v.Add("textColor", platform.RuleInvalid, "Text color must be a #RRGGBB or #RRGGBBAA hex value.", nil)
	}
	if !IsValidHexColor(p.BubbleColor) {
		v.Add("bubbleColor", platform.RuleInvalid, "Bubble color must be a #RRGGBB or #RRGGBBAA hex value.", nil)
	}

	return v.OrNil()
}

func validateIntRange(v *platform.ValidationError, field string, value, min, max int) {
	if value < min || value > max {
		v.Addf(field, platform.RuleInvalid, map[string]any{"min": min, "max": max},
			"%s must be between %d and %d.", field, min, max)
	}
}

func validateFloatRange(v *platform.ValidationError, field string, value float64, min, max float64) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < min || value > max {
		v.Addf(field, platform.RuleInvalid, map[string]any{"min": min, "max": max},
			"%s must be a finite number between %v and %v.", field, min, max)
	}
}

// ValidateBlockedTerm checks one blocked-term value before storage - see
// the Stage 10 task's Part 7: trimmed, non-empty, a conservative maximum
// length, and a recognized match mode.
func ValidateBlockedTerm(value string, mode MatchMode) error {
	v := &platform.ValidationError{}

	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		v.Add("value", platform.RuleRequired, "A blocked term cannot be empty.", nil)
	} else if len([]rune(trimmed)) > MaxBlockedTermLength {
		v.Addf("value", platform.RuleTooLong, map[string]any{"max": MaxBlockedTermLength},
			"A blocked term cannot exceed %d characters.", MaxBlockedTermLength)
	}

	if mode != MatchContains && mode != MatchWholeWord {
		v.Add("matchMode", platform.RuleUnsupported, "Unsupported match mode.", nil)
	}

	return v.OrNil()
}
