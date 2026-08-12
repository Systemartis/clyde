package hookserver

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestSanitizeLogValue covers the neutralization applied to
// attacker-influenced hook fields (tool_name, hook_type) before they reach
// slog — CodeQL go/log-injection.
func TestSanitizeLogValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "plain value passes through", in: "Bash", want: "Bash"},
		{name: "plain value with spaces and punctuation", in: "PreToolUse: Edit/Write", want: "PreToolUse: Edit/Write"},
		{name: "newline escaped", in: "Bash\nfake log line", want: `Bash\nfake log line`},
		{name: "carriage return escaped", in: "Bash\rfake", want: `Bash\rfake`},
		{name: "crlf pair escaped", in: "Bash\r\nfake", want: `Bash\r\nfake`},
		{name: "multiple injected records", in: "a\nb\r\nc\rd", want: `a\nb\r\nc\rd`},
		{name: "over-long value truncated", in: strings.Repeat("a", maxLogValueLen+50), want: strings.Repeat("a", maxLogValueLen)},
		{name: "value exactly at cap kept whole", in: strings.Repeat("b", maxLogValueLen), want: strings.Repeat("b", maxLogValueLen)},
		// A stray invalid byte anywhere in an over-cap value must cost at
		// most the split trailing rune — never the whole audit field.
		{name: "invalid byte early, over cap", in: "\xff" + strings.Repeat("a", 300), want: "\xff" + strings.Repeat("a", maxLogValueLen-1)},
		// Sub-cap input is not validated here; the JSON handler substitutes
		// the replacement character downstream.
		{name: "invalid utf-8 under cap passes through", in: "\xffBash", want: "\xffBash"},
		// Known, accepted cosmetic behavior: the cap applies after escaping,
		// so it can bisect an escape sequence and leave a lone backslash.
		// No raw newline can survive either way.
		{name: "escape bisected at cap leaves lone backslash", in: strings.Repeat("a", maxLogValueLen-1) + "\n" + "tail", want: strings.Repeat("a", maxLogValueLen-1) + `\`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := sanitizeLogValue(tt.in)
			if got != tt.want {
				t.Errorf("sanitizeLogValue(%q) = %q, want %q", tt.in, got, tt.want)
			}
			if len(got) > maxLogValueLen {
				t.Errorf("sanitizeLogValue(%q) length = %d, want <= %d", tt.in, len(got), maxLogValueLen)
			}
			if strings.ContainsAny(got, "\n\r") {
				t.Errorf("sanitizeLogValue(%q) = %q still contains a raw newline or carriage return", tt.in, got)
			}
		})
	}
}

// TestSanitizeLogValue_TruncatesOnRuneBoundary guards against byte-slicing a
// multibyte rune in half and emitting invalid UTF-8.
func TestSanitizeLogValue_TruncatesOnRuneBoundary(t *testing.T) {
	t.Parallel()

	// "€" is 3 bytes, so 100 of them is 300 bytes — past the cap. The cap
	// must be a multiple of the rune width for the cut to land on a
	// boundary; 256 mod 3 == 1, so slicing at the cap splits the 86th "€"
	// and the backoff has to trim it, leaving 85 whole runes (255 bytes).
	got := sanitizeLogValue(strings.Repeat("€", 100))
	if want := strings.Repeat("€", 85); got != want {
		t.Errorf("got %q (%d bytes), want %q (%d bytes)", got, len(got), want, len(want))
	}
	if len(got) > maxLogValueLen {
		t.Errorf("length = %d, want <= %d", len(got), maxLogValueLen)
	}
	if !utf8.ValidString(got) {
		t.Errorf("result %q is not valid UTF-8", got)
	}
}
