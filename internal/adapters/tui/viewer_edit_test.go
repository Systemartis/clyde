package tui

import "testing"

// stateOf returns a string representation of a buffer for compact assertions:
// "lines | line:col". Easier to read than separate equality checks.
func stateOf(b editBuffer) string {
	var sb []byte
	for i, l := range b.Lines {
		if i > 0 {
			sb = append(sb, '\\', 'n')
		}
		sb = append(sb, l...)
	}
	sb = append(sb, ' ', '|', ' ')
	sb = append(sb, []byte(itoa(b.Line))...)
	sb = append(sb, ':')
	sb = append(sb, []byte(itoa(b.Col))...)
	return string(sb)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	out := ""
	for n > 0 {
		out = string(rune('0'+n%10)) + out
		n /= 10
	}
	if negative {
		out = "-" + out
	}
	return out
}

func TestNewEditBuffer(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, in, want string
	}{
		{"empty", "", " | 0:0"},
		{"single-line", "abc", "abc | 0:0"},
		{"two-lines", "a\nb", "a\\nb | 0:0"},
		{"trailing-nl", "a\n", "a\\n | 0:0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := newEditBuffer(tc.in)
			if got := stateOf(b); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestInsertRune_AppendsAndAdvancesCol(t *testing.T) {
	t.Parallel()
	b := newEditBuffer("ab")
	b = insertRune(b, 'X')
	if got := stateOf(b); got != "Xab | 0:1" {
		t.Errorf("after X at start: %q", got)
	}
	b.Col = 3
	b = insertRune(b, 'Y')
	if got := stateOf(b); got != "XabY | 0:4" {
		t.Errorf("after Y at end: %q", got)
	}
}

func TestInsertNewline_SplitsAtCursor(t *testing.T) {
	t.Parallel()
	b := newEditBuffer("hello world")
	b.Col = 5
	b = insertNewline(b)
	if got := stateOf(b); got != "hello\\n world | 1:0" {
		t.Errorf("got %q", got)
	}
}

func TestDeleteBackward_RemovesCharThenMergesLines(t *testing.T) {
	t.Parallel()
	// Mid-line: simple char delete.
	b := newEditBuffer("abc")
	b.Col = 2
	b = deleteBackward(b)
	if got := stateOf(b); got != "ac | 0:1" {
		t.Errorf("mid-line: %q", got)
	}
	// Col 0 on second line: merges with first line.
	b = newEditBuffer("foo\nbar")
	b.Line = 1
	b.Col = 0
	b = deleteBackward(b)
	if got := stateOf(b); got != "foobar | 0:3" {
		t.Errorf("col 0 line 1: %q", got)
	}
	// Col 0 on first line: no-op.
	b = newEditBuffer("abc")
	b = deleteBackward(b)
	if got := stateOf(b); got != "abc | 0:0" {
		t.Errorf("col 0 line 0 should be no-op: %q", got)
	}
}

func TestDeleteForward_RemovesCharThenMergesLines(t *testing.T) {
	t.Parallel()
	b := newEditBuffer("abc")
	b.Col = 1
	b = deleteForward(b)
	if got := stateOf(b); got != "ac | 0:1" {
		t.Errorf("mid-line: %q", got)
	}
	// At end of first line: merges with next line.
	b = newEditBuffer("foo\nbar")
	b.Col = 3
	b = deleteForward(b)
	if got := stateOf(b); got != "foobar | 0:3" {
		t.Errorf("end of line: %q", got)
	}
}

func TestMoveCursor_ClampsAtBounds(t *testing.T) {
	t.Parallel()
	b := newEditBuffer("ab\nlonger\nx")
	// Move down then up — col should clamp on the short final line.
	b.Col = 5
	b.Line = 1
	b = moveCursorJ(b)
	if got := stateOf(b); got != "ab\\nlonger\\nx | 2:1" {
		t.Errorf("after J onto short line: %q", got)
	}
	// Move H past start of line.
	b = newEditBuffer("abc")
	b = moveCursorH(b)
	if b.Col != 0 {
		t.Errorf("H at col 0 should clamp; got %d", b.Col)
	}
	// Move L past end of line.
	b = newEditBuffer("ab")
	b.Col = 2
	b = moveCursorL(b)
	if b.Col != 2 {
		t.Errorf("L at end of line should clamp; got %d", b.Col)
	}
}

// TestRoundtrip_String verifies a buffer survives a serialise+parse cycle
// unchanged. Important because saving to disk goes through buffer.String().
func TestRoundtrip_String(t *testing.T) {
	t.Parallel()
	cases := []string{
		"",
		"single",
		"two\nlines",
		"trailing\n",
		"unicode αβγδε\nemoji 😀\n",
	}
	for _, in := range cases {
		got := newEditBuffer(in).String()
		if got != in {
			t.Errorf("roundtrip failed: in=%q out=%q", in, got)
		}
	}
}
