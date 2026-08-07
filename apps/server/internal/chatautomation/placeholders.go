package chatautomation

import (
	"fmt"
	"strings"

	"github.com/streaming-tree/server/internal/domain/account"
	"github.com/streaming-tree/server/internal/outboundchat"
)

// KnownPlaceholders is the closed, fixed set of placeholder names this
// engine ever resolves - see the Stage 11B task's own Part 18. Adding a
// new placeholder means adding it here and to Render's own switch,
// never accepting an arbitrary name.
var KnownPlaceholders = []string{"channelName", "platform", "channelUrl", "streamTitle", "streamUptime"}

func isKnownPlaceholder(name string) bool {
	for _, k := range KnownPlaceholders {
		if k == name {
			return true
		}
	}
	return false
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
// segments. Syntax: "{name}" is a placeholder, "{{" and "}}" are
// literal "{" and "}". No conditionals, functions, expressions, loops,
// nested placeholders, or custom format strings are ever recognized -
// this is a closed, declarative substitution language only.
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
// placeholder name outside KnownPlaceholders - used at schedule/command
// save time (Part 19: "Unknown placeholder names: reject the
// configuration with 422"). A syntactically malformed template (an
// unmatched brace) is rejected the same way.
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

// Context carries every value Render can substitute. ChannelName,
// Platform and ChannelURL are always resolvable given a connected
// account, so they are plain strings. StreamTitle and StreamUptime are
// pointers: nil means "known placeholder, but currently unresolvable
// given this target's own context" (see the Stage 11B task's own Part
// 18 streamTitle/streamUptime rules) - never a fabricated value.
type Context struct {
	ChannelName  string
	Platform     string
	ChannelURL   string
	StreamTitle  *string
	StreamUptime *string
}

// RenderResult is the outcome of rendering one template against one
// Context - returned by both the preview API and (internally) by the
// scheduler/command engine before a real send.
type RenderResult struct {
	Text             string
	CodePointCount   int
	Resolved         []string
	Unresolved       []string
	ValidForProvider bool
}

// Render expands template's placeholders against ctx. An unresolved or
// unknown placeholder substitutes as empty text and is reported in
// Unresolved - Render itself never fails for that (callers - automatic
// execution vs. preview - decide what "unresolved" means for them; see
// the Stage 11B task's own Part 19). Render only returns an error for a
// genuinely malformed template (an unmatched brace), which
// ValidateTemplatePlaceholders should already have rejected at save
// time for anything actually persisted.
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
		ValidForProvider: len(unresolved) == 0 && count <= outboundchat.MaxMessageCodePoints,
	}, nil
}

func resolvePlaceholder(name string, ctx Context) (string, bool) {
	switch name {
	case "channelName":
		return ctx.ChannelName, true
	case "platform":
		return ctx.Platform, true
	case "channelUrl":
		return ctx.ChannelURL, true
	case "streamTitle":
		if ctx.StreamTitle == nil {
			return "", false
		}
		return *ctx.StreamTitle, true
	case "streamUptime":
		if ctx.StreamUptime == nil {
			return "", false
		}
		return *ctx.StreamUptime, true
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
// name for a provider - "Twitch" for account.ProviderTwitch. Brand
// names are never localized anywhere in this application (see
// README.md's own "What is not translated" section).
func PlatformDisplayName(providerID account.ProviderID) string {
	switch providerID {
	case account.ProviderTwitch:
		return "Twitch"
	case account.ProviderYouTube:
		return "YouTube"
	default:
		return string(providerID)
	}
}

// ChannelURL builds a provider's own normal public channel URL from a
// connected account's login - a pure, local, allow-listed construction,
// never accepted from a chat message and never fetched from the
// provider. Returns ok=false for a provider with no known URL form.
func ChannelURL(providerID account.ProviderID, login string) (string, bool) {
	switch providerID {
	case account.ProviderTwitch:
		return "https://www.twitch.tv/" + login, true
	default:
		return "", false
	}
}
