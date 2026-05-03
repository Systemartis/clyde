package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// cursorPos is a (line, col) pair used by the selection state. Same shape
// as editBuffer's Line/Col fields but kept as a separate type so the
// selection anchor doesn't get accidentally mutated by moveCursor* helpers.
type cursorPos struct {
	Line int
	Col  int
}

// activeSelectionRange returns the selection's start / end (line, col) in
// document order — start always precedes end on the page even if the user
// dragged backwards. Returns ok=false when no selection is active or the
// anchor coincides with the cursor (zero-width selection isn't worth
// rendering).
func (m Model) activeSelectionRange() (start, end cursorPos, ok bool) {
	if !m.viewerSelActive {
		return cursorPos{}, cursorPos{}, false
	}
	cur := cursorPos{Line: m.viewerEdit.Line, Col: m.viewerEdit.Col}
	a := m.viewerSelAnchor
	if a == cur {
		return cursorPos{}, cursorPos{}, false
	}
	if a.Line < cur.Line || (a.Line == cur.Line && a.Col < cur.Col) {
		return a, cur, true
	}
	return cur, a, true
}

// selectedText returns the substring of the buffer covered by the active
// selection. Multi-line ranges include the full middle lines plus the
// partial first/last lines. Returns "" when there's no active selection.
func (m Model) selectedText() string {
	start, end, ok := m.activeSelectionRange()
	if !ok {
		return ""
	}
	lines := m.viewerEdit.Lines
	if start.Line == end.Line {
		l := lines[start.Line]
		rs := []rune(l)
		from := clampSelCol(start.Col, len(rs))
		to := clampSelCol(end.Col, len(rs))
		return string(rs[from:to])
	}
	var sb strings.Builder
	// First line: from start.Col to end of line.
	first := []rune(lines[start.Line])
	sb.WriteString(string(first[clampSelCol(start.Col, len(first)):]))
	sb.WriteByte('\n')
	// Full middle lines.
	for ln := start.Line + 1; ln < end.Line; ln++ {
		sb.WriteString(lines[ln])
		sb.WriteByte('\n')
	}
	// Last line: from 0 to end.Col.
	last := []rune(lines[end.Line])
	sb.WriteString(string(last[:clampSelCol(end.Col, len(last))]))
	return sb.String()
}

// clampSelCol guards against the half-open [0, n] selection range exceeding
// the line's rune length — happens when the cursor is past the visible end
// (vim's "after-the-line" position) and we're slicing for a copy.
func clampSelCol(col, n int) int {
	if col < 0 {
		return 0
	}
	if col > n {
		return n
	}
	return col
}

// startSelectionIfNeeded snapshots the cursor as the anchor when a shifted
// motion arrives without an active selection. Subsequent shifted motions
// extend from this anchor; the cursor itself moves freely via the existing
// motion helpers.
func (m Model) startSelectionIfNeeded() Model {
	if m.viewerSelActive {
		return m
	}
	m.viewerSelActive = true
	m.viewerSelAnchor = cursorPos{Line: m.viewerEdit.Line, Col: m.viewerEdit.Col}
	return m
}

// clearSelection drops the active range. Called from any plain (non-
// shifted) motion so the user's next arrow press doesn't accidentally
// append to a stale selection.
func (m Model) clearSelection() Model {
	m.viewerSelActive = false
	m.viewerSelAnchor = cursorPos{}
	return m
}

// copySelectionToClipboard writes the current selection's text via OSC 52
// (tea.SetClipboard). Returns the command alongside an updated status
// hint so the user gets visible feedback on a successful copy.
func (m Model) copySelectionToClipboard() (Model, tea.Cmd) {
	text := m.selectedText()
	if text == "" {
		// Nothing to copy — common when the user hits ⌃c with an empty
		// selection. Don't bury the keystroke; surface a hint so the user
		// knows what happened.
		m.viewerStatus = "no selection — shift+arrow to select first"
		return m, nil
	}
	lineCount := strings.Count(text, "\n") + 1
	m.viewerStatus = fmtCopyStatus(lineCount, len(text))
	return m, tea.SetClipboard(text)
}

// fmtCopyStatus is a tiny helper so the status string lives in one place
// and both selection-copy + line-yank can share the same wording.
func fmtCopyStatus(lineCount, byteLen int) string {
	if lineCount <= 1 {
		return "copied " + intToString(byteLen) + " char(s) to clipboard"
	}
	return "copied " + intToString(lineCount) + " line(s) to clipboard"
}

// intToString avoids pulling fmt for a single int format on a hot path.
func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	out := ""
	for n > 0 {
		out = string(rune('0'+n%10)) + out
		n /= 10
	}
	if neg {
		return "-" + out
	}
	return out
}

// applySelectionExtension is the pre-motion hook every directional key
// in edit mode runs. When Shift is held, it ensures a selection anchor
// exists (so the upcoming motion extends the selection); when Shift is
// not held, it clears any existing selection so plain motion drops the
// highlight cleanly. Pure on Model — no side effects beyond viewerSel*.
func applySelectionExtension(m Model, msg tea.KeyPressMsg) Model {
	if msg.Mod.Contains(tea.ModShift) {
		return m.startSelectionIfNeeded()
	}
	return m.clearSelection()
}

// selectAllBuffer expands the selection to cover the entire buffer.
func (m Model) selectAllBuffer() Model {
	if len(m.viewerEdit.Lines) == 0 {
		return m
	}
	m.viewerSelActive = true
	m.viewerSelAnchor = cursorPos{Line: 0, Col: 0}
	last := len(m.viewerEdit.Lines) - 1
	m.viewerEdit.Line = last
	m.viewerEdit.Col = runeLen(m.viewerEdit.Lines[last])
	return m
}
