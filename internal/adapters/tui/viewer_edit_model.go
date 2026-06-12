package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// paintSelectionSpan overlays a selection background on a rendered viewer
// row from fromCol to toCol (exclusive end; toCol == -1 means "to the end
// of the row"). Operates on plain text (ANSI-stripped) to avoid corrupting
// SGR sequences during the slice — edit mode disables syntax highlighting
// so this is lossless.
//
// fromCol / toCol are SOURCE-line column indices; the on-screen column
// adds the line-number gutter offset and subtracts xOff for horizontal
// scroll. Out-of-window spans no-op gracefully.
func paintSelectionSpan(row string, fromCol, toCol, numWidth, xOff int, p Palette) string {
	plain := ansi.Strip(row)
	rs := []rune(plain)
	gutter := numWidth + 3 // "%d" + " │ "
	startScreen := gutter + (fromCol - xOff)
	endScreen := gutter + (toCol - xOff)
	if toCol == -1 {
		endScreen = len(rs)
	}
	if startScreen < gutter {
		startScreen = gutter
	}
	if endScreen > len(rs) {
		endScreen = len(rs)
	}
	if startScreen >= endScreen || startScreen >= len(rs) {
		return row
	}
	style := lipgloss.NewStyle().Foreground(p.Bg).Background(p.Cyan)
	leftPart := string(rs[:startScreen])
	mid := style.Render(string(rs[startScreen:endScreen]))
	rightPart := string(rs[endScreen:])
	return leftPart + mid + rightPart
}

// paintFindMatch overlays a highlight on the rendered viewer row for a
// find-match span. focused=true uses bold + a more visible background to
// signal which match the user is currently positioned on; non-focused
// matches get a dimmer background. Operates on the plain (ANSI-stripped)
// row to avoid corrupting SGR sequences during the slice.
func paintFindMatch(row string, startCol, length, numWidth, xOff int, p Palette, focused bool) string {
	plain := ansi.Strip(row)
	rs := []rune(plain)
	gutter := numWidth + 3 // "%d" + " │ "
	startScreen := gutter + (startCol - xOff)
	endScreen := startScreen + length
	if startScreen < gutter {
		startScreen = gutter
	}
	if endScreen > len(rs) {
		endScreen = len(rs)
	}
	if startScreen >= endScreen || startScreen >= len(rs) {
		return row
	}
	style := lipgloss.NewStyle().
		Foreground(p.Bg).
		Background(p.Cyan)
	if focused {
		style = style.Bold(true).Background(p.Pink)
	}
	leftPart := string(rs[:startScreen])
	mid := style.Render(string(rs[startScreen:endScreen]))
	rightPart := string(rs[endScreen:])
	return leftPart + mid + rightPart
}

// paintCursorCell overlays an inverted-block cursor on a rendered viewer
// row at the given source-buffer column. The visible row already has the
// "<lineNum> │ " prefix prepended, so the screen x of the cursor is
// numWidth+3 (gutter) + (cursorCol - xOff). Out-of-window columns no-op
// gracefully — when the cursor scrolls horizontally past the edge we
// just don't paint, which is preferable to drawing it on top of the
// scrollbar.
//
// Strips the row to plain text first because slicing a styled row would
// corrupt SGR sequences. Edit mode disables syntax highlighting upstream
// so this is a lossless transform.
func paintCursorCell(row string, cursorCol, numWidth, xOff int, p Palette) string {
	plain := ansi.Strip(row)
	rs := []rune(plain)
	gutter := numWidth + 3 // "%d" + " │ "
	target := gutter + (cursorCol - xOff)
	if target < gutter || target >= len(rs) {
		return row
	}
	cellChar := string(rs[target])
	if cellChar == "" {
		cellChar = " "
	}
	cursorStyle := lipgloss.NewStyle().
		Foreground(p.Bg).
		Background(p.Pink).
		Bold(true)
	out := make([]rune, 0, len(rs)+8)
	out = append(out, rs[:target]...)
	leftPart := string(out)
	rightPart := string(rs[target+1:])
	return leftPart + cursorStyle.Render(cellChar) + rightPart
}

// enterEditMode primes the viewer's editable buffer from the cached
// content and switches into ViewerEdit. Cursor lands on the first line
// currently visible in the viewport so toggling view ↔ edit doesn't snap
// the page back to the top — useful when the user scrolled / found-jumped
// somewhere and wants to edit AT that location.
//
// Lazy-init: skip rebuilding the buffer if it already matches the cached
// content (re-entering edit mode after a quick view-mode peek shouldn't
// throw away the cursor position).
func (m Model) enterEditMode() Model {
	current := m.viewerEdit.String()
	if current != m.viewerCachedContent || len(m.viewerEdit.Lines) == 0 {
		m.viewerEdit = newEditBuffer(m.viewerCachedContent)
	}
	m.viewerEdit = m.viewerEdit.clampCursor()
	// Snap cursor to the topmost visible line ONLY when it would
	// otherwise force an auto-scroll. The renderer auto-scrolls when
	// cursor < yOff or cursor >= yOff+vpHeight; matching the cursor to
	// yOff (or to a clamp inside the visible window) keeps the page
	// where the user left it. We can't compute vpHeight here without
	// the panel dimensions, so a conservative line-only check works:
	// keep the cursor in view by bumping it to yOff when above.
	yOff := m.viewport.vp.YOffset()
	if m.viewerEdit.Line < yOff {
		m.viewerEdit.Line = yOff
		m.viewerEdit.Col = 0
		m.viewerEdit = m.viewerEdit.clampCursor()
	}
	m.viewerMode = ViewerEdit
	m.viewerStatus = ""
	return m
}

// exitEditMode flips back to ViewerView. The buffer is left intact in case
// the user wants to resume editing — we only reset it on file close. The
// syntax-highlight cache is rebuilt from the current buffer because the
// content may have changed; a stale highlight slice would mismatch the new
// line count and the renderer's "len(hl) == len(lines)" guard would force
// a full re-tokenise on every frame anyway.
func (m Model) exitEditMode() Model {
	m.viewerCachedContent = m.viewerEdit.String()
	m.viewerCachedSize = int64(len(m.viewerCachedContent))
	m.viewerCachedHL = nil
	if hasLexerFor(m.viewerFile) {
		lines := strings.Split(m.viewerCachedContent, "\n")
		if hl, ok := highlightCode(m.viewerCachedContent, m.viewerFile, m.palette); ok && len(hl) == len(lines) {
			m.viewerCachedHL = hl
		}
	}
	m.viewerMode = ViewerView
	// Clear any active selection — view mode has no cursor, so a stale
	// highlight would be visually anchored at a position the user can no
	// longer move.
	m.viewerSelActive = false
	m.viewerSelAnchor = cursorPos{}
	return m
}

// saveViewerBuffer writes the current edit buffer to disk under the open
// file's path. Resolves relative paths against the project cwd, preserves
// the existing file's mode where possible, and reports the outcome via
// m.viewerStatus so the user gets visible feedback.
//
// Demo mode is a no-op with a friendly message — the mock paths don't
// correspond to real files and writing would either fail or pollute the
// working tree.
func (m Model) saveViewerBuffer() Model {
	if m.demoMode {
		m.viewerStatus = "save unavailable in demo mode"
		return m
	}
	if m.viewerFile == "" {
		return m
	}
	// Make sure the buffer reflects the latest cache before serializing.
	// Edit ops keep viewerEdit authoritative, but view mode never touches
	// it — so if the user opens a file and saves immediately (no edits),
	// the buffer is still empty / stale.
	if len(m.viewerEdit.Lines) == 0 {
		m.viewerEdit = newEditBuffer(m.viewerCachedContent)
	}
	body := m.viewerEdit.String()

	abs := m.viewerFile
	if !filepath.IsAbs(abs) {
		cwd := m.liveView.Project.CWD()
		if cwd != "" {
			abs = filepath.Join(cwd, abs)
		}
	}

	// Preserve mode bits so a save doesn't accidentally widen permissions
	// on a private config file. Fall back to 0o644 when the file didn't
	// exist before (new file via edit-then-save flow).
	mode := os.FileMode(0o644)
	if info, err := os.Stat(abs); err == nil {
		mode = info.Mode().Perm()
	}
	if err := os.WriteFile(abs, []byte(body), mode); err != nil {
		m.viewerStatus = fmt.Sprintf("save failed: %s", err.Error())
		return m
	}

	m.viewerCachedContent = body
	m.viewerCachedSize = int64(len(body))
	// Re-highlight after save so the View-mode color matches the saved
	// buffer if the user was in the middle of changing identifiers.
	m.viewerCachedHL = nil
	if hasLexerFor(m.viewerFile) {
		lines := strings.Split(body, "\n")
		if hl, ok := highlightCode(body, m.viewerFile, m.palette); ok && len(hl) == len(lines) {
			m.viewerCachedHL = hl
		}
	}
	m.viewerDirty = false
	m.viewerStatus = fmt.Sprintf("saved %s (%s)", filepath.Base(m.viewerFile), humanSize(m.viewerCachedSize))
	return m
}
