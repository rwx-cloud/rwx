package text

import "strings"

// Truncate shortens s to at most max characters (counting runes), replacing the
// trailing overflow with a single-rune ellipsis so the result still fits within
// max. A non-positive max returns s unchanged.
func Truncate(s string, max int) string {
	if max <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max == 1 {
		return "…"
	}
	return string(runes[:max-1]) + "…"
}

// WrapText breaks s into lines of at most width characters, splitting at word
// boundaries when possible. If width is zero or negative, or the string fits
// within the width, the original string is returned as a single-element slice.
func WrapText(s string, width int) []string {
	if width <= 0 || len(s) <= width {
		return []string{s}
	}

	var lines []string
	for len(s) > width {
		// Find the last space at or before the width limit
		breakAt := strings.LastIndex(s[:width+1], " ")
		if breakAt <= 0 {
			// No space found; break at width
			breakAt = width
		}
		lines = append(lines, s[:breakAt])
		s = strings.TrimLeft(s[breakAt:], " ")
	}
	if len(s) > 0 {
		lines = append(lines, s)
	}
	return lines
}
