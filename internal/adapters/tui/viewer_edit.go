package tui

import "strings"

// ─── Pure buffer mutations ────────────────────────────────────────────────────
//
// All edit operations work on a []string (one entry per line) plus a cursor
// (line, col). Pure functions — no Model coupling — so they're easy to test
// and easy to reason about. The Model wiring lives in keys.go and just
// translates key events into calls here.
//
// Cursor is "between characters" in vim's normal-mode sense: col == 0 means
// before the first character; col == len(rune line) means after the last
// character. This matches how every editor I've ever used handles cursors;
// the alternative (cursor "on" a character) makes append-at-end-of-line a
// special case.

// editBuffer wraps the line slice + cursor so callers don't have to thread
// three return values through every operation. Returned by every mutation.
type editBuffer struct {
	Lines []string
	Line  int
	Col   int
}

// newEditBuffer initializes a buffer from a string by splitting on \n. Empty
// input still yields one empty line so the cursor has somewhere to sit.
func newEditBuffer(content string) editBuffer {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}
	return editBuffer{Lines: lines}
}

// String serializes the buffer back to a single string. Inverse of
// newEditBuffer.
func (b editBuffer) String() string {
	return strings.Join(b.Lines, "\n")
}

// clampCursor returns the buffer with the cursor moved into a valid position.
// Used after operations that might leave the cursor out of bounds (e.g. line
// deletion, content reload).
func (b editBuffer) clampCursor() editBuffer {
	if len(b.Lines) == 0 {
		b.Lines = []string{""}
	}
	if b.Line < 0 {
		b.Line = 0
	}
	if b.Line >= len(b.Lines) {
		b.Line = len(b.Lines) - 1
	}
	maxCol := runeLen(b.Lines[b.Line])
	if b.Col < 0 {
		b.Col = 0
	}
	if b.Col > maxCol {
		b.Col = maxCol
	}
	return b
}

// runeLen returns the character (not byte) length of s — what the user
// thinks of as "the position the cursor can land on past the end".
func runeLen(s string) int {
	return len([]rune(s))
}

// moveCursorH / moveCursorJ / moveCursorK / moveCursorL implement vim-style
// cursor motion. They deliberately don't wrap to next/previous lines on
// horizontal moves — that surprises users coming from any modern editor.
func moveCursorH(b editBuffer) editBuffer {
	if b.Col > 0 {
		b.Col--
	}
	return b
}

func moveCursorL(b editBuffer) editBuffer {
	max := runeLen(b.Lines[b.Line])
	if b.Col < max {
		b.Col++
	}
	return b
}

func moveCursorJ(b editBuffer) editBuffer {
	if b.Line < len(b.Lines)-1 {
		b.Line++
	}
	return b.clampCursor()
}

func moveCursorK(b editBuffer) editBuffer {
	if b.Line > 0 {
		b.Line--
	}
	return b.clampCursor()
}

// insertRune inserts r at the cursor position and advances col. Edits the
// current line in place; never spans line boundaries (use insertNewline for
// that).
func insertRune(b editBuffer, r rune) editBuffer {
	rs := []rune(b.Lines[b.Line])
	if b.Col < 0 {
		b.Col = 0
	}
	if b.Col > len(rs) {
		b.Col = len(rs)
	}
	out := make([]rune, 0, len(rs)+1)
	out = append(out, rs[:b.Col]...)
	out = append(out, r)
	out = append(out, rs[b.Col:]...)
	b.Lines[b.Line] = string(out)
	b.Col++
	return b
}

// insertNewline splits the current line at the cursor: text before the
// cursor stays on the current line, text from the cursor onwards becomes
// the next line. Cursor moves to col 0 of the new line.
func insertNewline(b editBuffer) editBuffer {
	rs := []rune(b.Lines[b.Line])
	if b.Col < 0 {
		b.Col = 0
	}
	if b.Col > len(rs) {
		b.Col = len(rs)
	}
	left := string(rs[:b.Col])
	right := string(rs[b.Col:])
	// Insert a new line after b.Line.
	out := make([]string, 0, len(b.Lines)+1)
	out = append(out, b.Lines[:b.Line]...)
	out = append(out, left, right)
	out = append(out, b.Lines[b.Line+1:]...)
	b.Lines = out
	b.Line++
	b.Col = 0
	return b
}

// deleteBackward deletes the rune immediately before the cursor. When the
// cursor is at column 0, joins the current line onto the previous line and
// places the cursor at the join point — matching the behavior of every
// modern editor's Backspace key.
func deleteBackward(b editBuffer) editBuffer {
	if b.Col > 0 {
		rs := []rune(b.Lines[b.Line])
		out := make([]rune, 0, len(rs)-1)
		out = append(out, rs[:b.Col-1]...)
		out = append(out, rs[b.Col:]...)
		b.Lines[b.Line] = string(out)
		b.Col--
		return b
	}
	if b.Line == 0 {
		return b
	}
	prev := b.Lines[b.Line-1]
	curr := b.Lines[b.Line]
	joined := prev + curr
	out := make([]string, 0, len(b.Lines)-1)
	out = append(out, b.Lines[:b.Line-1]...)
	out = append(out, joined)
	out = append(out, b.Lines[b.Line+1:]...)
	b.Lines = out
	b.Line--
	b.Col = runeLen(prev)
	return b
}

// deleteForward removes the rune at the cursor. When the cursor sits past
// the last character of a line, joins the next line onto this one. Mirror
// of deleteBackward, used by the Delete key.
func deleteForward(b editBuffer) editBuffer {
	rs := []rune(b.Lines[b.Line])
	if b.Col < len(rs) {
		out := make([]rune, 0, len(rs)-1)
		out = append(out, rs[:b.Col]...)
		out = append(out, rs[b.Col+1:]...)
		b.Lines[b.Line] = string(out)
		return b
	}
	if b.Line == len(b.Lines)-1 {
		return b
	}
	merged := b.Lines[b.Line] + b.Lines[b.Line+1]
	out := make([]string, 0, len(b.Lines)-1)
	out = append(out, b.Lines[:b.Line]...)
	out = append(out, merged)
	out = append(out, b.Lines[b.Line+2:]...)
	b.Lines = out
	return b
}
