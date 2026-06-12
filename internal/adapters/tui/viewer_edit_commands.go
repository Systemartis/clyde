package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// maxViewerHistory caps the undo / redo stacks. A few hundred edits cover
// any realistic in-session editing flow without turning the editor into a
// memory hog on multi-thousand-line files.
const maxViewerHistory = 256

// commandPaletteEntry is a single row in the :command discoverability
// palette rendered above the prompt while command mode is open.
type commandPaletteEntry struct {
	Forms []string // accepted text forms (e.g. ":w", ":write")
	Desc  string
}

// commandPaletteEntries returns the static list shown above the :command
// prompt. Each entry's Forms slice is checked prefix-style against the
// user's typed buffer to highlight matches; descriptions are dim by
// default and brighten on match.
func commandPaletteEntries() []commandPaletteEntry {
	return []commandPaletteEntry{
		{Forms: []string{"w", "write"}, Desc: "save buffer"},
		{Forms: []string{"q", "quit"}, Desc: "close viewer (errors if dirty)"},
		{Forms: []string{"q!"}, Desc: "close, discard unsaved changes"},
		{Forms: []string{"wq", "x"}, Desc: "save then close"},
		{Forms: []string{"y", "yank", "%y"}, Desc: "yank entire buffer to clipboard"},
		{Forms: []string{"y N"}, Desc: "yank first N lines"},
		{Forms: []string{"y N,M", "N,M y"}, Desc: "yank line range (1-based, inclusive)"},
		{Forms: []string{"s/old/new/g"}, Desc: "substitute on current line (g = all per line)"},
		{Forms: []string{"%s/old/new/g"}, Desc: "substitute every line in buffer"},
	}
}

// commandPaletteRows returns the visual row count of the rendered palette.
// Used by the viewer renderer to size the bottom strip correctly when
// :command mode is open.
func commandPaletteRows() int {
	// One row per entry plus a separator above the list.
	return len(commandPaletteEntries()) + 1
}

// renderCommandPalette returns the multi-line palette body. Buf is the
// user's typed query (after the leading `:`); entries whose forms have
// buf as a prefix are highlighted in cyan, the rest are dim.
func renderCommandPalette(p Palette, buf string, width int) string {
	entries := commandPaletteEntries()
	dim := lipgloss.NewStyle().Foreground(p.TextDim)
	hi := lipgloss.NewStyle().Foreground(p.Cyan).Bold(true)
	bar := dim.Render(strings.Repeat("·", clamp(width, 1, 20)))
	rows := []string{bar}
	for _, e := range entries {
		matched := false
		if buf != "" {
			for _, f := range e.Forms {
				if strings.HasPrefix(f, buf) {
					matched = true
					break
				}
			}
		}
		formStr := strings.Join(e.Forms, " / ")
		var line string
		if matched {
			line = hi.Render(":"+formStr) + "  " + dim.Render(e.Desc)
		} else {
			line = dim.Render(":" + formStr + "  " + e.Desc)
		}
		rows = append(rows, line)
	}
	return strings.Join(rows, "\n")
}

// viewerDiscardPrompt is the dirty-confirm message shown on the hint row
// the first time Esc is pressed with unsaved changes. Compared verbatim to
// detect "user pressed Esc twice in a row" instead of relying on a
// separate confirmation flag.
const viewerDiscardPrompt = "unsaved changes — esc again to discard, :w to save"

// pushHistory snapshots the buffer onto the undo stack and clears the
// redo stack — every new mutation invalidates redo, matching every
// editor's mental model. Buffer is COPIED (lines slice, cursor) so future
// mutations don't poison history entries.
func (m Model) pushHistory() Model {
	clone := m.viewerEdit
	lines := make([]string, len(clone.Lines))
	copy(lines, clone.Lines)
	clone.Lines = lines
	m.viewerHistory = append(m.viewerHistory, clone)
	if len(m.viewerHistory) > maxViewerHistory {
		m.viewerHistory = m.viewerHistory[len(m.viewerHistory)-maxViewerHistory:]
	}
	m.viewerRedo = nil
	return m
}

// popUndo replaces the current buffer with the most recent snapshot,
// pushing the current state onto the redo stack. No-op when history is
// empty.
func (m Model) popUndo() Model {
	if len(m.viewerHistory) == 0 {
		return m
	}
	curr := m.viewerEdit
	currLines := make([]string, len(curr.Lines))
	copy(currLines, curr.Lines)
	curr.Lines = currLines
	m.viewerRedo = append(m.viewerRedo, curr)

	snap := m.viewerHistory[len(m.viewerHistory)-1]
	m.viewerHistory = m.viewerHistory[:len(m.viewerHistory)-1]
	m.viewerEdit = snap
	m.viewerDirty = true
	return m
}

// popRedo is the inverse of popUndo. Pushes the current state back onto
// the undo stack so the user can undo the redo, etc.
func (m Model) popRedo() Model {
	if len(m.viewerRedo) == 0 {
		return m
	}
	curr := m.viewerEdit
	currLines := make([]string, len(curr.Lines))
	copy(currLines, curr.Lines)
	curr.Lines = currLines
	m.viewerHistory = append(m.viewerHistory, curr)

	snap := m.viewerRedo[len(m.viewerRedo)-1]
	m.viewerRedo = m.viewerRedo[:len(m.viewerRedo)-1]
	m.viewerEdit = snap
	m.viewerDirty = true
	return m
}

// ─── Command mode ─────────────────────────────────────────────────────────────

// beginCommand opens the `:command` prompt. Resets the buffer so a previous
// session's input doesn't bleed in.
func (m Model) beginCommand() Model {
	m.viewerCmdActive = true
	m.viewerCmdBuf = ""
	return m
}

// cancelCommand closes the prompt without executing.
func (m Model) cancelCommand() Model {
	m.viewerCmdActive = false
	m.viewerCmdBuf = ""
	return m
}

// runViewerCommand parses + dispatches a typed command, then closes the
// prompt. Recognized:
//
//	w  / write       save buffer
//	q  / quit        close viewer (errors if dirty)
//	q!               close viewer, discarding unsaved changes
//	wq / x           save then close
//	y / yank         yank entire file to system clipboard
//	y N              yank N lines starting from line 1
//	y N,M / N,M y    yank lines N..M (inclusive, 1-based)
//	%y               yank entire file (vim-compatible alias)
//
// Unknown commands surface an error in the status line.
//
// Returns a tea.Cmd alongside the model so yank can dispatch
// tea.SetClipboard — same OSC 52 path the explorer's copy uses, so the
// yanked range lands in the user's system clipboard.
func (m Model) runViewerCommand() (Model, tea.Cmd) {
	cmd := strings.TrimSpace(m.viewerCmdBuf)
	m.viewerCmdActive = false
	m.viewerCmdBuf = ""
	switch cmd {
	case "w", "write":
		return m.saveViewerBuffer(), nil
	case "q", "quit":
		if m.viewerDirty {
			m.viewerStatus = "unsaved changes — use :q! to discard or :wq to save"
			return m, nil
		}
		return m.closeViewer(), nil
	case "q!":
		return m.closeViewer(), nil
	case "wq", "x":
		m = m.saveViewerBuffer()
		if !m.viewerDirty {
			return m.closeViewer(), nil
		}
		return m, nil
	case "":
		return m, nil
	}
	if yank, ok := parseYankCommand(cmd); ok {
		return m.yankLines(yank.from, yank.to)
	}
	if sub, ok := parseSubstituteCommand(cmd); ok {
		return m.runSubstitute(sub), nil
	}
	m.viewerStatus = "unknown command: " + cmd
	return m, nil
}

// substituteSpec describes a parsed :s/find/replace[/g] command.
type substituteSpec struct {
	find    string
	replace string
	global  bool // /g flag — replace every match per line, not just first
	all     bool // %s — apply to every line in the buffer (else: current cursor line)
}

// parseSubstituteCommand recognizes the vim substitute forms:
//
//	s/old/new/         — first match on current line
//	s/old/new/g        — every match on current line
//	%s/old/new/        — first match per line, every line
//	%s/old/new/g       — every match in the buffer
//
// Delimiter is always '/'. Empty find string is rejected (would match
// nothing useful). Returns ok=false on any parse failure.
func parseSubstituteCommand(cmd string) (substituteSpec, bool) {
	rest, all := strings.CutPrefix(cmd, "%s/")
	if !all {
		var ok bool
		rest, ok = strings.CutPrefix(cmd, "s/")
		if !ok {
			return substituteSpec{}, false
		}
	}
	// rest is "find/replace[/flags]" — split on '/' but allow no
	// trailing flag segment ("find/replace" without the closing slash
	// is a common shorthand).
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 2 {
		return substituteSpec{}, false
	}
	find := parts[0]
	replace := parts[1]
	if find == "" {
		return substituteSpec{}, false
	}
	flags := ""
	if len(parts) == 3 {
		flags = parts[2]
	}
	return substituteSpec{
		find:    find,
		replace: replace,
		global:  strings.Contains(flags, "g"),
		all:     all,
	}, true
}

// runSubstitute applies the substitution to the edit buffer (in edit mode)
// or to the cached content (in view mode — buffer is rebuilt afterwards).
// Reports the count of replaced occurrences via the status row.
//
// Snapshots the pre-edit state for undo when in edit mode so the user
// can ⌃z out of an unwanted substitute.
func (m Model) runSubstitute(sub substituteSpec) Model {
	// Make sure the edit buffer reflects the latest content even if the
	// user is in view mode — substitute is allowed there too.
	if len(m.viewerEdit.Lines) == 0 || (m.viewerMode != ViewerEdit && m.viewerEdit.String() != m.viewerCachedContent) {
		m.viewerEdit = newEditBuffer(m.viewerCachedContent)
	}
	if m.viewerMode == ViewerEdit {
		m = m.pushHistory()
	}

	count := 0
	if sub.all {
		for i, line := range m.viewerEdit.Lines {
			n := 0
			m.viewerEdit.Lines[i], n = applySubstitute(line, sub.find, sub.replace, sub.global)
			count += n
		}
	} else {
		// Current line = the cursor's line in edit mode; in view mode the
		// "current line" mental model maps to the topmost visible line.
		idx := m.viewerEdit.Line
		if m.viewerMode != ViewerEdit {
			idx = m.viewport.vp.YOffset()
		}
		if idx >= 0 && idx < len(m.viewerEdit.Lines) {
			n := 0
			m.viewerEdit.Lines[idx], n = applySubstitute(m.viewerEdit.Lines[idx], sub.find, sub.replace, sub.global)
			count += n
		}
	}

	if count == 0 {
		m.viewerStatus = "no matches for: " + sub.find
		return m
	}
	m.viewerDirty = true
	// Resync the cached content + highlight whether or not we're in edit
	// mode — view mode renders off cached content, so the substitution
	// would otherwise be invisible until the next mode toggle.
	m.viewerCachedContent = m.viewerEdit.String()
	m.viewerCachedSize = int64(len(m.viewerCachedContent))
	m.viewerCachedHL = nil
	if hasLexerFor(m.viewerFile) {
		lines := strings.Split(m.viewerCachedContent, "\n")
		if hl, ok := highlightCode(m.viewerCachedContent, m.viewerFile, m.palette); ok && len(hl) == len(lines) {
			m.viewerCachedHL = hl
		}
	}
	m.viewerEdit = m.viewerEdit.clampCursor()
	m.viewerStatus = fmt.Sprintf("replaced %d occurrence(s)", count)
	return m
}

// applySubstitute does the per-line text replacement. global=true uses
// strings.ReplaceAll; global=false replaces only the first occurrence.
// Returns the new line and the count of replacements made.
func applySubstitute(line, find, replace string, global bool) (string, int) {
	if find == "" {
		return line, 0
	}
	if global {
		count := strings.Count(line, find)
		if count == 0 {
			return line, 0
		}
		return strings.ReplaceAll(line, find, replace), count
	}
	idx := strings.Index(line, find)
	if idx < 0 {
		return line, 0
	}
	return line[:idx] + replace + line[idx+len(find):], 1
}

// yankRange describes a range of lines (1-based, inclusive) to yank. from
// and to are clamped against the buffer size in yankLines.
type yankRange struct{ from, to int }

// parseYankCommand recognizes the variants of the yank command. Returns
// (range, true) on a match. Both 1-based; the caller clamps. Requires
// an explicit "y" or "yank" token so a typo'd line range like "1,3" on
// its own doesn't accidentally trigger a yank.
func parseYankCommand(cmd string) (yankRange, bool) {
	if cmd == "y" || cmd == "yank" || cmd == "%y" {
		return yankRange{from: 1, to: -1}, true // -1 means "to end"
	}
	// "y N"  → first N lines starting from line 1.
	if rest, ok := strings.CutPrefix(cmd, "y "); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(rest)); err == nil && n > 0 {
			return yankRange{from: 1, to: n}, true
		}
		if from, to, ok := parseLineRange(rest); ok {
			return yankRange{from: from, to: to}, true
		}
	}
	// "N,M y" — explicit range with trailing y.
	if rest, ok := strings.CutSuffix(cmd, " y"); ok {
		if from, to, ok := parseLineRange(rest); ok {
			return yankRange{from: from, to: to}, true
		}
	}
	return yankRange{}, false
}

// parseLineRange parses "N,M" → (N, M, true). 1-based; both inclusive.
func parseLineRange(s string) (int, int, bool) {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, ",")
	if len(parts) != 2 {
		return 0, 0, false
	}
	from, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	to, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || from < 1 || to < from {
		return 0, 0, false
	}
	return from, to, true
}

// yankLines copies lines [from, to] (1-based, inclusive) from the current
// viewer content to the system clipboard via OSC 52. to == -1 means "to
// end of buffer". Surfaces a status hint with the line count copied.
func (m Model) yankLines(from, to int) (Model, tea.Cmd) {
	content := m.currentViewerContent()
	lines := strings.Split(content, "\n")
	// 1-based clamp.
	if from < 1 {
		from = 1
	}
	if to == -1 || to > len(lines) {
		to = len(lines)
	}
	if from > to {
		m.viewerStatus = "yank: empty range"
		return m, nil
	}
	body := strings.Join(lines[from-1:to], "\n")
	m.viewerStatus = fmt.Sprintf("yanked %d line(s) to clipboard", to-from+1)
	return m, tea.SetClipboard(body)
}

// closeViewer is the centralized viewer-close path. Resets every viewer-
// related Model field so a re-open lands in a clean state. Used by :q,
// :wq, the close button, and the global Esc handler.
func (m Model) closeViewer() Model {
	m.viewerActive = false
	m.viewerFile = ""
	m.viewerFullscreen = false
	m.viewerMode = ViewerView
	m.viewerEdit = editBuffer{}
	m.viewerDirty = false
	m.viewerStatus = ""
	m.viewerCmdActive = false
	m.viewerCmdBuf = ""
	m.viewerHistory = nil
	m.viewerRedo = nil
	m.viewerFindActive = false
	m.viewerFindQuery = ""
	m.viewerFindMatches = nil
	m.viewerFindIdx = 0
	return m
}

// handleViewerCommandKey owns keystrokes while the `:command` prompt is
// open. Type to append, Backspace to trim, Enter to execute, Esc to
// cancel. Returns (Model, tea.Cmd) so :y can dispatch the SetClipboard
// command up through Update.
func (m Model) handleViewerCommandKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.Code {
	case tea.KeyEsc:
		return m.cancelCommand(), nil
	case tea.KeyEnter:
		return m.runViewerCommand()
	case tea.KeyBackspace:
		if m.viewerCmdBuf == "" {
			return m.cancelCommand(), nil
		}
		rs := []rune(m.viewerCmdBuf)
		m.viewerCmdBuf = string(rs[:len(rs)-1])
		return m, nil
	}
	if msg.Text != "" && (msg.Mod == 0 || msg.Mod == tea.ModShift) {
		m.viewerCmdBuf += msg.Text
		return m, nil
	}
	return m, nil
}

// ─── View-mode cursor + word motions ──────────────────────────────────────────

// moveCursorWordForward moves the cursor to the start of the next word —
// vim's `w` motion. Words are runs of letters/digits/underscore separated
// by anything else. Crosses line boundaries when the current line has
// nothing left to skip past.
func moveCursorWordForward(b editBuffer) editBuffer {
	rs := []rune(b.Lines[b.Line])
	col := b.Col
	// Skip current word.
	for col < len(rs) && isWordRune(rs[col]) {
		col++
	}
	// Skip non-word runs to land on the next word's start.
	for col < len(rs) && !isWordRune(rs[col]) {
		col++
	}
	if col >= len(rs) && b.Line < len(b.Lines)-1 {
		b.Line++
		b.Col = 0
		return b.clampCursor()
	}
	b.Col = col
	return b
}

// moveCursorWordBack moves the cursor to the start of the previous word —
// vim's `b` motion.
func moveCursorWordBack(b editBuffer) editBuffer {
	if b.Col == 0 {
		if b.Line == 0 {
			return b
		}
		b.Line--
		b.Col = runeLen(b.Lines[b.Line])
	}
	rs := []rune(b.Lines[b.Line])
	col := b.Col
	if col > 0 {
		col--
	}
	// Skip non-word runs.
	for col > 0 && !isWordRune(rs[col]) {
		col--
	}
	// Walk back to the start of the current word.
	for col > 0 && isWordRune(rs[col-1]) {
		col--
	}
	b.Col = col
	return b
}

// moveCursorLineStart / moveCursorLineEnd implement vim's `0` and `$`.
func moveCursorLineStart(b editBuffer) editBuffer { b.Col = 0; return b }
func moveCursorLineEnd(b editBuffer) editBuffer {
	b.Col = runeLen(b.Lines[b.Line])
	return b
}

// isWordRune reports whether r is part of a "word" for word-motion
// purposes. Matches vim's default — letters, digits, underscore.
func isWordRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z':
		return true
	case r >= 'A' && r <= 'Z':
		return true
	case r >= '0' && r <= '9':
		return true
	case r == '_':
		return true
	}
	return false
}
