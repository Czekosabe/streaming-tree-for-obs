package audio

import (
	"strings"
	"unicode"
)

// maxRepeatedRun is the fixed cap repeated-character normalization
// collapses any longer run down to (docs/audio-tts.md §10.2 step 5) -
// "sooooo" (5 o's) becomes "sooo" (3 o's); a normal doubled letter (a
// run of 2) is never touched.
const maxRepeatedRun = 3

// PreprocessConfig carries the bounded, per-call inputs the fixed,
// ordered Stage 17A text pipeline needs. IsCommand is computed by the
// caller (only ever true for a chat.message event whose ORIGINAL source
// text, not the utterance builder's wrapped sentence, looks like a
// command) - see docs/audio-tts.md §10.3: command suppression never
// applies to a supporter-family event, even one whose message happens
// to start with "!".
type PreprocessConfig struct {
	SuppressCommands bool
	IsCommand        bool

	RemoveURLs   bool
	BlockedWords []string

	NormalizeRepeatedChars bool

	MaxLengthCodePoints int
}

// Preprocess applies the fixed, ordered pipeline (docs/audio-tts.md
// §10.2) to an already-built utterance. ok is false when the result is
// empty after every step, or the item is dropped outright by command
// suppression - the caller must never enqueue empty or suppressed audio.
// Deterministic; every step is bounded (no HTML parsing, no network
// lookup, no regular expression with unbounded backtracking potential).
func Preprocess(utterance string, cfg PreprocessConfig) (result string, ok bool) {
	if cfg.SuppressCommands && cfg.IsCommand {
		return "", false
	}

	text := utterance
	if cfg.RemoveURLs {
		text = removeURLs(text)
	}
	text = removeBlockedWords(text, cfg.BlockedWords)
	if cfg.NormalizeRepeatedChars {
		text = normalizeRepeatedChars(text, maxRepeatedRun)
	}
	text = normalizeWhitespace(text)
	text = capCodePoints(text, cfg.MaxLengthCodePoints)
	text = strings.TrimSpace(text)

	if text == "" {
		return "", false
	}
	return text, true
}

// removeURLs drops any whitespace-delimited token that looks like a URL
// (an http(s):// scheme, or bare "www." text with no scheme - §10.2
// step 3's own documented choice to treat both as URL-like) - never
// follows, fetches, or resolves anything; token-bounded, so even a
// hostile, very long URL-shaped token is handled in one pass with no
// backtracking.
func removeURLs(text string) string {
	fields := strings.Fields(text)
	kept := make([]string, 0, len(fields))
	for _, f := range fields {
		if isURLLike(f) {
			continue
		}
		kept = append(kept, f)
	}
	return strings.Join(kept, " ")
}

func isURLLike(token string) bool {
	lower := strings.ToLower(token)
	return strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "www.")
}

// removeBlockedWords drops any whitespace-delimited token whose
// letters/digits, case-folded, exactly equal a configured blocked word
// - deterministic, case-insensitive, Unicode-aware whole-token matching,
// never a substring match inside an unrelated larger word (a blocked
// word "ass" never matches inside "class").
func removeBlockedWords(text string, blocked []string) string {
	if len(blocked) == 0 {
		return text
	}
	blockedSet := make(map[string]struct{}, len(blocked))
	for _, w := range blocked {
		blockedSet[strings.ToLower(w)] = struct{}{}
	}

	fields := strings.Fields(text)
	kept := make([]string, 0, len(fields))
	for _, f := range fields {
		core := strings.ToLower(strings.TrimFunc(f, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r)
		}))
		if _, isBlocked := blockedSet[core]; isBlocked {
			continue
		}
		kept = append(kept, f)
	}
	return strings.Join(kept, " ")
}

// normalizeRepeatedChars collapses any run of the same code point
// longer than maxRun down to exactly maxRun, operating rune-by-rune (so
// a combining mark or an emoji is treated as its own code point, never
// split) in one bounded pass.
func normalizeRepeatedChars(text string, maxRun int) string {
	runes := []rune(text)
	var b strings.Builder
	b.Grow(len(text))
	i := 0
	for i < len(runes) {
		j := i + 1
		for j < len(runes) && runes[j] == runes[i] {
			j++
		}
		run := j - i
		if run > maxRun {
			run = maxRun
		}
		for k := 0; k < run; k++ {
			b.WriteRune(runes[i])
		}
		i = j
	}
	return b.String()
}

// normalizeWhitespace collapses any run of whitespace to a single space
// and trims both ends - strings.Fields already does exactly this using
// unicode.IsSpace, in one bounded pass.
func normalizeWhitespace(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// capCodePoints enforces max as a hard Unicode-code-point cap (never
// truncates a UTF-8 byte mid-rune), preferring to trim back to the last
// whitespace boundary within the truncated slice when one exists so a
// word is not cut in half - the hard authority is always the code-point
// count, never the word-boundary preference.
func capCodePoints(text string, max int) string {
	if max <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	truncated := runes[:max]
	for i := len(truncated) - 1; i >= 0; i-- {
		if unicode.IsSpace(truncated[i]) {
			return string(truncated[:i])
		}
	}
	return string(truncated)
}
