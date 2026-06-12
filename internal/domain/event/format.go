// Package event — format.go provides the Truncate helper for normalizing and
// length-bounding Summary strings. This file uses only stdlib packages
// (strings, unicode/utf8) to satisfy the domain-pure depguard rule.
package event

import (
	"strings"
	"unicode/utf8"
)

// Truncate normalizes whitespace and limits the string to max runes.
//
// Rules applied in order:
//  1. Replace "\r\n" with a single space (before handling lone "\r"/"\n").
//  2. Replace each remaining "\r", "\n", "\t" with a single space.
//  3. Collapse consecutive whitespace (space, tab) to a single space.
//  4. Trim leading and trailing whitespace.
//  5. Count runes via utf8.RuneCountInString.
//  6. If rune count ≤ max → return as-is.
//  7. Else → take first (max-1) runes and append "…" (U+2026, 1 rune).
//     The final rune count is exactly min(len(input_runes), max).
//
// Special cases:
//   - max ≤ 0 → "" (defensive).
//   - empty or all-whitespace input → "".
//   - max = 1 with non-empty input → "…".
//
// Pure stdlib: strings and unicode/utf8 only. depguard domain-pure safe.
func Truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}

	// Step 1: replace \r\n with space (must precede lone \r/\n handling).
	s = strings.ReplaceAll(s, "\r\n", " ")

	// Step 2: replace remaining \r, \n, \t with space.
	s = strings.NewReplacer("\r", " ", "\n", " ", "\t", " ").Replace(s)

	// Step 3: collapse consecutive whitespace to a single space.
	// We do this with a simple rune-by-rune scan to stay stdlib-only
	// without requiring regexp.
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if r == ' ' {
			if !prevSpace {
				b.WriteRune(r)
			}
			prevSpace = true
		} else {
			b.WriteRune(r)
			prevSpace = false
		}
	}
	s = b.String()

	// Step 4: trim leading/trailing whitespace.
	s = strings.TrimSpace(s)

	// Step 5–6: count runes; return as-is if within limit.
	n := utf8.RuneCountInString(s)
	if n <= max {
		return s
	}

	// Step 7: truncate to (max-1) runes and append ellipsis.
	// Walk by rune to find the byte offset of the (max-1)-th rune boundary.
	count := 0
	byteIdx := 0
	for i := range s {
		if count == max-1 {
			byteIdx = i
			break
		}
		count++
	}

	return s[:byteIdx] + "…"
}
