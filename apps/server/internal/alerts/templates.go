package alerts

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	domain "github.com/streaming-tree/server/internal/domain/alerts"
)

// MaxRenderedCodePoints bounds a rendered alert's own on-screen text -
// conservative, since an alert is a short banner, never a chat message
// (see the Stage 12A task's own Part 10/21: "Rendered text has a
// conservative maximum length... never truncate").
const MaxRenderedCodePoints = 240

// KnownPlaceholders is the closed, fixed alert placeholder vocabulary -
// deliberately its own set, not chat-automation's (an alert has
// different data: no channel URL or stream title, but a quantity and an
// event type chat commands never have). See the Stage 12A task's own
// Part 10. Adding a new placeholder means adding it here, to
// resolvePlaceholder's own switch, and to capability.go's per-event
// AvailablePlaceholders if it is conditionally available.
var KnownPlaceholders = []string{"username", "platform", "eventType", "quantity", "message", "rewardTitle", "groupCount"}

func isKnownPlaceholder(name string) bool {
	for _, k := range KnownPlaceholders {
		if k == name {
			return true
		}
	}
	return false
}

// AvailablePlaceholders returns the subset of KnownPlaceholders that
// actually make sense for t, driven by domain.CapabilityFor - never a
// hand-maintained second list that can drift from the real capability
// table.
func AvailablePlaceholders(t domain.EventType) []string {
	capability := domain.CapabilityFor(t)
	out := []string{"platform", "eventType"}
	if capability.HasUser {
		out = append(out, "username")
	}
	if capability.HasQuantity {
		out = append(out, "quantity")
	}
	if capability.HasMessage {
		out = append(out, "message")
	}
	if capability.HasRewardTitle {
		out = append(out, "rewardTitle")
	}
	if domain.GroupingCapabilityFor(t).Groupable {
		out = append(out, "groupCount")
	}
	return out
}

type segmentKind int

const (
	segmentLiteral segmentKind = iota
	segmentPlaceholder
)

type segment struct {
	kind segmentKind
	text string
}

// ParseTemplate splits template into literal-text and placeholder
// segments - byte-for-byte the same closed grammar as
// internal/chatautomation/placeholders.go's own ParseTemplate: "{name}"
// is a placeholder, "{{"/"}}" are literal "{"/"}", no conditionals,
// functions, expressions, loops, nesting, or custom formats.
func ParseTemplate(template string) ([]segment, error) {
	runes := []rune(template)
	var segments []segment
	var literal strings.Builder
	flushLiteral := func() {
		if literal.Len() > 0 {
			segments = append(segments, segment{kind: segmentLiteral, text: literal.String()})
			literal.Reset()
		}
	}

	i := 0
	for i < len(runes) {
		c := runes[i]
		switch c {
		case '{':
			if i+1 < len(runes) && runes[i+1] == '{' {
				literal.WriteRune('{')
				i += 2
				continue
			}
			j := i + 1
			for j < len(runes) && runes[j] != '}' && runes[j] != '{' {
				j++
			}
			if j >= len(runes) || runes[j] != '}' {
				return nil, fmt.Errorf("%w: unmatched '{'", ErrPlaceholderInvalid)
			}
			name := string(runes[i+1 : j])
			if name == "" {
				return nil, fmt.Errorf("%w: empty placeholder", ErrPlaceholderInvalid)
			}
			flushLiteral()
			segments = append(segments, segment{kind: segmentPlaceholder, text: name})
			i = j + 1
		case '}':
			if i+1 < len(runes) && runes[i+1] == '}' {
				literal.WriteRune('}')
				i += 2
				continue
			}
			return nil, fmt.Errorf("%w: unmatched '}'", ErrPlaceholderInvalid)
		default:
			literal.WriteRune(c)
			i++
		}
	}
	flushLiteral()
	return segments, nil
}

// ValidateTemplatePlaceholders parses template and rejects any
// placeholder name outside KnownPlaceholders - used at rule save time
// (422). It does NOT check per-event-type availability; that is a
// separate, capability-driven check (ValidateTemplateForEventType)
// since it needs to know the rule's own event type.
func ValidateTemplatePlaceholders(template string) error {
	segments, err := ParseTemplate(template)
	if err != nil {
		return err
	}
	for _, s := range segments {
		if s.kind == segmentPlaceholder && !isKnownPlaceholder(s.text) {
			return fmt.Errorf("%w: unknown placeholder %q", ErrPlaceholderInvalid, s.text)
		}
	}
	return nil
}

// ValidateTemplateForEventType additionally rejects a placeholder that
// is known but not available for t (e.g. {quantity} on a "follow"
// rule) - the alert-specific equivalent of the Stage 12A task's own
// Part 6 capability rule, applied to templates.
func ValidateTemplateForEventType(template string, t domain.EventType) error {
	segments, err := ParseTemplate(template)
	if err != nil {
		return err
	}
	available := AvailablePlaceholders(t)
	for _, s := range segments {
		if s.kind != segmentPlaceholder {
			continue
		}
		if !isKnownPlaceholder(s.text) {
			return fmt.Errorf("%w: unknown placeholder %q", ErrPlaceholderInvalid, s.text)
		}
		ok := false
		for _, a := range available {
			if a == s.text {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("%w: placeholder %q is not available for event type %q", ErrPlaceholderInvalid, s.text, string(t))
		}
	}
	return nil
}

// ValidateGroupingTemplate closes the last half of Stage 12B's Part 11
// safety rule: a rule may only enable grouping for a message-bearing type
// when ShowMessage is false (domain.ValidateRuleConditions already
// enforces that half) AND the template itself never references
// {message} either - otherwise a template could still render one
// member's real message on every re-render regardless of the toggle,
// since buildInstance resolves {message} for the template independently
// of ShowMessage (see its own doc comment). A no-op when allowGrouping is
// false or the event type's grouping capability does not require this.
func ValidateGroupingTemplate(template string, t domain.EventType, allowGrouping bool) error {
	if !allowGrouping || !domain.GroupingCapabilityFor(t).RequiresNoMessage {
		return nil
	}
	segments, err := ParseTemplate(template)
	if err != nil {
		return err
	}
	for _, s := range segments {
		if s.kind == segmentPlaceholder && s.text == "message" {
			return fmt.Errorf("%w: template must not reference {message} while grouping is enabled for event type %q", ErrPlaceholderInvalid, string(t))
		}
	}
	return nil
}

// Context carries every value Render can substitute for one alert
// instance. Username/Message/RewardTitle are pointers: nil means "not
// shown for this alert" (an anonymous actor, no message, a rule that
// disabled that visibility toggle) - never a fabricated placeholder
// value.
type Context struct {
	Username    *string
	Platform    string
	EventType   string
	Quantity    *int64
	Message     *string
	RewardTitle *string
	// GroupCount is always a known, non-optional value (an instance
	// starts at 1 and only ever increments) - never a pointer, unlike
	// the fields above whose absence is meaningful. Only ever resolvable
	// in a saved template for a grouping-capable event type - see
	// AvailablePlaceholders.
	GroupCount int
}

// RenderResult is the outcome of rendering one template against one
// Context.
type RenderResult struct {
	Text             string
	CodePointCount   int
	Resolved         []string
	Unresolved       []string
	ValidForProvider bool
}

// Render expands template's placeholders against ctx. An unresolved or
// unknown placeholder substitutes as empty text and is reported in
// Unresolved, never a hard failure - only a genuinely malformed
// template (an unmatched brace) fails, mirroring
// internal/chatautomation/placeholders.go's own Render contract.
func Render(template string, ctx Context) (RenderResult, error) {
	segments, err := ParseTemplate(template)
	if err != nil {
		return RenderResult{}, err
	}

	var out strings.Builder
	var resolved, unresolved []string
	seenResolved := map[string]bool{}
	seenUnresolved := map[string]bool{}

	for _, s := range segments {
		if s.kind == segmentLiteral {
			out.WriteString(s.text)
			continue
		}
		value, ok := resolvePlaceholder(s.text, ctx)
		if ok {
			out.WriteString(value)
			if !seenResolved[s.text] {
				resolved = append(resolved, s.text)
				seenResolved[s.text] = true
			}
		} else if !seenUnresolved[s.text] {
			unresolved = append(unresolved, s.text)
			seenUnresolved[s.text] = true
		}
	}

	text := out.String()
	count := codePointLen(text)
	return RenderResult{
		Text: text, CodePointCount: count, Resolved: resolved, Unresolved: unresolved,
		ValidForProvider: len(unresolved) == 0 && count <= MaxRenderedCodePoints,
	}, nil
}

func resolvePlaceholder(name string, ctx Context) (string, bool) {
	switch name {
	case "username":
		if ctx.Username == nil {
			return "", false
		}
		return *ctx.Username, true
	case "platform":
		return ctx.Platform, true
	case "eventType":
		return ctx.EventType, true
	case "quantity":
		if ctx.Quantity == nil {
			return "", false
		}
		return strconv.FormatInt(*ctx.Quantity, 10), true
	case "message":
		if ctx.Message == nil {
			return "", false
		}
		return *ctx.Message, true
	case "rewardTitle":
		if ctx.RewardTitle == nil {
			return "", false
		}
		return *ctx.RewardTitle, true
	case "groupCount":
		return strconv.Itoa(ctx.GroupCount), true
	default:
		return "", false
	}
}

func codePointLen(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

// PlatformDisplayName returns the fixed, never-translated presentation
// name for a provider - brand names are never localized anywhere in
// this application (see README.md's own "What is not translated"
// section).
func PlatformDisplayName(providerID domain.ProviderID) string {
	switch providerID {
	case domain.ProviderTwitch:
		return "Twitch"
	default:
		return string(providerID)
	}
}

// eventTypeLabels is the small, closed, per-language mapping used to
// resolve {eventType} - the one alert placeholder that is a plain,
// translatable English/Polish word rather than a provider brand name,
// hence rendered using the alert profile's own explicit `language`
// field (Part 44: "never infer the public alert's language from
// whoever last edited it").
var eventTypeLabels = map[domain.Language]map[domain.EventType]string{
	domain.LanguageEnglish: {
		domain.EventFollow:                 "Follow",
		domain.EventSubscription:           "Subscription",
		domain.EventResubscription:         "Resubscription",
		domain.EventGiftedSubscription:     "Gifted Subscription",
		domain.EventSubscriptionGiftBatch:  "Gift Sub Batch",
		domain.EventBits:                   "Bits",
		domain.EventRaid:                   "Raid",
		domain.EventChannelPointRedemption: "Channel Point Redemption",
	},
	domain.LanguagePolish: {
		domain.EventFollow:                 "Obserwacja",
		domain.EventSubscription:           "Subskrypcja",
		domain.EventResubscription:         "Przedłużona subskrypcja",
		domain.EventGiftedSubscription:     "Podarowana subskrypcja",
		domain.EventSubscriptionGiftBatch:  "Pakiet podarowanych subskrypcji",
		domain.EventBits:                   "Bits",
		domain.EventRaid:                   "Najazd",
		domain.EventChannelPointRedemption: "Wymiana punktów kanału",
	},
}

// EventTypeLabel returns t's fixed presentation label in lang, falling
// back to the raw type string for an unrecognized combination (never
// happens for a validated Rule, but keeps this function total).
func EventTypeLabel(t domain.EventType, lang domain.Language) string {
	if byLang, ok := eventTypeLabels[lang]; ok {
		if label, ok := byLang[t]; ok {
			return label
		}
	}
	if label, ok := eventTypeLabels[domain.LanguageEnglish][t]; ok {
		return label
	}
	return string(t)
}

// PreviewTemplate renders template against representative fixture data
// for eventType - the Stage 12A task's own Part 37 "editor preview:
// local and instant, does not touch queue." Every capability-available
// field is populated from the fixture regardless of any rule's own
// Show* toggles (those are separate, additional presentation elements,
// not part of the template text itself) - never sends, never persists,
// never contacts Twitch, never touches a real queue.
func PreviewTemplate(eventType domain.EventType, template string, lang domain.Language) (RenderResult, error) {
	evt := buildFixtureEvent(eventType, "", time.Now().UTC())
	capability := domain.CapabilityFor(eventType)

	ctx := Context{Platform: PlatformDisplayName(domain.ProviderID(evt.ProviderID)), EventType: EventTypeLabel(eventType, lang)}
	if capability.HasUser && evt.User != nil && !evt.User.Anonymous {
		name := evt.User.DisplayName
		ctx.Username = &name
	}
	if capability.HasMessage && evt.Message != nil {
		text := evt.Message.Text
		ctx.Message = &text
	}
	if capability.HasQuantity && evt.Quantity != nil {
		q := *evt.Quantity
		ctx.Quantity = &q
	}
	if capability.HasRewardTitle {
		if title, ok := evt.ProviderExtra["rewardTitle"]; ok {
			ctx.RewardTitle = &title
		}
	}
	if domain.GroupingCapabilityFor(eventType).Groupable {
		// An illustrative, obviously-fictional count (never 1) so a
		// template author previewing {groupCount} sees what a genuinely
		// grouped alert looks like, not the trivial single-member case.
		ctx.GroupCount = 3
	}
	return Render(template, ctx)
}
