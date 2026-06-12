// Package event_test contains tests for the Truncate helper in format.go.
package event_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Systemartis/clyde/internal/domain/event"
)

// TestTruncate_RuneCountLimit verifies that strings at or below maxRunes are
// returned unchanged — no ellipsis appended.
func TestTruncate_RuneCountLimit(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		input    string
		maxRunes int
		want     string
	}{
		{
			name:     "empty string",
			input:    "",
			maxRunes: 80,
			want:     "",
		},
		{
			name:     "shorter than max",
			input:    "Hello",
			maxRunes: 80,
			want:     "Hello",
		},
		{
			name:     "exactly max runes",
			input:    strings.Repeat("a", 80),
			maxRunes: 80,
			want:     strings.Repeat("a", 80),
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := event.Truncate(tc.input, tc.maxRunes)
			if got != tc.want {
				t.Errorf("Truncate(%q, %d) = %q, want %q", tc.input, tc.maxRunes, got, tc.want)
			}
			// Extra guard: verify no ellipsis was added.
			if strings.HasSuffix(got, "…") && tc.want == tc.input {
				t.Errorf("Truncate unexpectedly appended ellipsis: %q", got)
			}
		})
	}
}

// TestTruncate_AppendsEllipsisOnOverflow verifies that strings exceeding
// maxRunes are truncated to (maxRunes-1) runes with "…" appended.
// Total rune count of the result MUST equal min(len(input), maxRunes).
func TestTruncate_AppendsEllipsisOnOverflow(t *testing.T) {
	t.Parallel()

	const maxRunes = 80
	// 120 ASCII runes — well over the limit.
	input := strings.Repeat("x", 120)

	got := event.Truncate(input, maxRunes)

	gotRunes := utf8.RuneCountInString(got)
	if gotRunes != maxRunes {
		t.Errorf("rune count = %d, want %d", gotRunes, maxRunes)
	}

	lastRune, _ := utf8.DecodeLastRuneInString(got)
	const ellipsis = '…'
	if lastRune != ellipsis {
		t.Errorf("last rune = %U (%c), want %U (…)", lastRune, lastRune, ellipsis)
	}

	// First (maxRunes-1) runes should be the original content.
	wantPrefix := strings.Repeat("x", maxRunes-1)
	gotPrefix := string([]rune(got)[:maxRunes-1])
	if gotPrefix != wantPrefix {
		t.Errorf("prefix = %q, want %q", gotPrefix, wantPrefix)
	}
}

// TestTruncate_CollapsesNewlines verifies that \n, \r\n, and \r are all
// replaced with a single space.
func TestTruncate_CollapsesNewlines(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		input    string
		maxRunes int
		want     string
	}{
		{
			name:     "LF becomes space",
			input:    "Hello\nWorld",
			maxRunes: 80,
			want:     "Hello World",
		},
		{
			name:     "CRLF becomes single space",
			input:    "Hello\r\nWorld",
			maxRunes: 80,
			want:     "Hello World",
		},
		{
			name:     "CR becomes space",
			input:    "Hello\rWorld",
			maxRunes: 80,
			want:     "Hello World",
		},
		{
			name:     "multiple newlines collapse",
			input:    "a\n\nb",
			maxRunes: 80,
			want:     "a b",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := event.Truncate(tc.input, tc.maxRunes)
			if got != tc.want {
				t.Errorf("Truncate(%q, %d) = %q, want %q", tc.input, tc.maxRunes, got, tc.want)
			}
		})
	}
}

// TestTruncate_CollapsesConsecutiveWhitespace verifies that runs of spaces
// and tabs collapse to a single space and leading/trailing whitespace is trimmed.
func TestTruncate_CollapsesConsecutiveWhitespace(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		input    string
		maxRunes int
		want     string
	}{
		{
			name:     "multiple spaces",
			input:    "Hello   World",
			maxRunes: 80,
			want:     "Hello World",
		},
		{
			name:     "leading whitespace trimmed",
			input:    "  Hello",
			maxRunes: 80,
			want:     "Hello",
		},
		{
			name:     "trailing whitespace trimmed",
			input:    "Hello  ",
			maxRunes: 80,
			want:     "Hello",
		},
		{
			name:     "tab becomes single space",
			input:    "Hello\tWorld",
			maxRunes: 80,
			want:     "Hello World",
		},
		{
			name:     "all whitespace returns empty",
			input:    "   \t\t  ",
			maxRunes: 80,
			want:     "",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := event.Truncate(tc.input, tc.maxRunes)
			if got != tc.want {
				t.Errorf("Truncate(%q, %d) = %q, want %q", tc.input, tc.maxRunes, got, tc.want)
			}
		})
	}
}

// TestTruncate_RuneBoundaryMultibyte verifies that truncation happens on rune
// boundaries for multibyte UTF-8 input.
func TestTruncate_RuneBoundaryMultibyte(t *testing.T) {
	t.Parallel()

	// "ă" is U+0103, 2 bytes in UTF-8.
	// "Hai mortal ă" contains only basic Latin + Romanian "ă".
	// Build a 90-rune string of 2-byte runes to force multibyte truncation.
	singleMultiByte := "ă" // 1 rune, 2 bytes
	input := strings.Repeat(singleMultiByte, 90)
	const maxRunes = 80

	got := event.Truncate(input, maxRunes)

	gotRunes := utf8.RuneCountInString(got)
	if gotRunes != maxRunes {
		t.Errorf("rune count = %d, want %d", gotRunes, maxRunes)
	}

	// Must end with ellipsis.
	lastRune, _ := utf8.DecodeLastRuneInString(got)
	if lastRune != '…' {
		t.Errorf("last rune = %U (%c), want '…' (U+2026)", lastRune, lastRune)
	}

	// Byte length must be a valid UTF-8 string — no partial sequences.
	if !utf8.ValidString(got) {
		t.Errorf("result is not valid UTF-8: %q", got)
	}

	// Content before the ellipsis must be the original multibyte chars.
	runesGot := []rune(got)
	for i := 0; i < maxRunes-1; i++ {
		if runesGot[i] != 'ă' {
			t.Errorf("rune[%d] = %U, want 'ă' (U+0103)", i, runesGot[i])
		}
	}
}

// TestTruncate_DegenerateCases covers edge cases: empty, all-whitespace,
// maxRunes=0, maxRunes=1.
func TestTruncate_DegenerateCases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		input    string
		maxRunes int
		want     string
	}{
		{
			name:     "empty string max=80",
			input:    "",
			maxRunes: 80,
			want:     "",
		},
		{
			name:     "all-whitespace string",
			input:    "   ",
			maxRunes: 80,
			want:     "",
		},
		{
			name:     "max=0 returns empty",
			input:    "anything",
			maxRunes: 0,
			want:     "",
		},
		{
			name:     "max=1 with longer input returns ellipsis",
			input:    "Hello",
			maxRunes: 1,
			want:     "…",
		},
		{
			name:     "max=1 with single rune input unchanged",
			input:    "H",
			maxRunes: 1,
			want:     "H",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := event.Truncate(tc.input, tc.maxRunes)
			if got != tc.want {
				t.Errorf("Truncate(%q, %d) = %q, want %q", tc.input, tc.maxRunes, got, tc.want)
			}
		})
	}
}
