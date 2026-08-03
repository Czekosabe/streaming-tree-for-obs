package platform

import "unicode"

// isLetterOrDigit accepts letters and digits from any script, so user-authored
// tags in Polish, Greek or Japanese are treated exactly like ASCII ones.
func isLetterOrDigit(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}
