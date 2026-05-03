package tui

import (
	"fmt"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// maxViewerBytes caps how much of a file we load into the viewer.
// Files larger than this threshold get a "file too large" message instead.
const maxViewerBytes = 512 * 1024 // 512 KB

// renderViewerPanel renders the file viewer that replaces the right column
// when viewerActive is true.
func (m Model) renderViewerPanel(width, height int) string {
	s := m.styles
	p := m.palette
	path := m.viewerFile

	if path == "" {
		return wrapPanel(s, "  no file open", "viewer", "", width, height, true)
	}

	// Classify file
	if isImageFile(path) {
		return m.renderImageViewer(s, p, path, width, height)
	}
	return m.renderTextViewer(s, p, path, width, height)
}

// humanSize formats a byte count as e.g. "12 B", "3.4 KB", "1.2 MB".
func humanSize(n int64) string {
	const (
		kb = 1024
		mb = kb * 1024
	)
	switch {
	case n < kb:
		return fmt.Sprintf("%d B", n)
	case n < mb:
		return fmt.Sprintf("%.1f KB", float64(n)/kb)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/mb)
	}
}

// looksBinary returns true when the content has characteristics typical of
// binary files (NUL bytes within the first 8KB). Quick and good enough for
// the viewer — we just want to avoid dumping garbled bytes for executables
// or compiled artifacts.
func looksBinary(content string) bool {
	limit := 8192
	if len(content) < limit {
		limit = len(content)
	}
	for i := 0; i < limit; i++ {
		if content[i] == 0 {
			return true
		}
	}
	return false
}

// renderViewerScrollbar appends a single-column scrollbar to each line of the
// viewport content. The thumb position reflects scrollPct ∈ [0, 1].
func renderViewerScrollbar(p Palette, viewContent string, scrollPct float64) string {
	if viewContent == "" {
		return viewContent
	}
	lines := strings.Split(viewContent, "\n")
	n := len(lines)
	if n <= 1 {
		return viewContent
	}
	thumbStart := int(scrollPct * float64(n-1))
	if thumbStart < 0 {
		thumbStart = 0
	}
	if thumbStart >= n {
		thumbStart = n - 1
	}
	thumbStyle := lipgloss.NewStyle().Foreground(p.Purple)
	trackStyle := lipgloss.NewStyle().Foreground(p.TextFade)
	for i := range lines {
		if i == thumbStart {
			lines[i] += " " + thumbStyle.Render("█")
		} else {
			lines[i] += " " + trackStyle.Render("│")
		}
	}
	return strings.Join(lines, "\n")
}

// ─── Text viewer ──────────────────────────────────────────────────────────────

// renderTextViewer renders a scrollable text file using bubbles/v2/viewport.
// Reads from the Model's viewer cache (populated by m.loadViewerFile when
// the file was opened) — no disk I/O or chroma tokenisation per frame.
// On cache miss (e.g. demo mode never primed it, or path mismatch) falls
// back to a fresh read so we never render an empty panel; the next open
// will refresh the cache properly.
func (m Model) renderTextViewer(s Styles, p Palette, path string, width, height int) string {
	var content string
	var fileSize int64
	switch {
	case m.viewerMode == ViewerEdit && len(m.viewerEdit.Lines) > 0:
		// Edit mode renders directly off the mutable buffer so each
		// keystroke surfaces immediately; the cached string lags one
		// state behind because we only resync on Esc / save.
		content = m.viewerEdit.String()
		fileSize = int64(len(content))
	case m.viewerCachedFile == path && m.viewerCachedContent != "":
		content = m.viewerCachedContent
		fileSize = m.viewerCachedSize
	case m.demoMode:
		mock, ok := mockFileContent[normalizePath(path)]
		if !ok {
			mock = fmt.Sprintf("  (no mock content for %s)", path)
		}
		content = mock
		fileSize = int64(len(content))
	default:
		raw, sz, err := readFileForViewerWithSize(path, m.liveView.Project.CWD())
		if err != nil {
			content = fmt.Sprintf("  error reading file: %s", err)
		} else {
			content = raw
			fileSize = sz
		}
	}

	// Inner dimensions
	innerW := width - 4 // border + 1-char pad each side
	innerH := height - 2
	if innerW < 10 {
		innerW = 10
	}
	if innerH < 4 {
		innerH = 4
	}

	// Detect binary content and short-circuit to a friendly message.
	if looksBinary(content) {
		body := renderViewerHeader(p, path, innerW, fmt.Sprintf("%s · binary file", humanSize(fileSize))) +
			"\n" + lipgloss.NewStyle().Foreground(p.TextDim).Render("  binary file — preview not available")
		return wrapPanel(s, body, "viewer", "esc close", width, height, true)
	}

	// Build line-numbered content. Reserve 2 cells on the right so the
	// scrollbar lands cleanly at column innerW-1.
	lines := strings.Split(content, "\n")
	totalLines := len(lines)
	numWidth := len(fmt.Sprintf("%d", totalLines))

	// Pull the syntax-highlighted line slice from the cache when it matches
	// the open file. Re-tokenising via chroma on every frame is the single
	// largest contributor to scroll stutter on multi-thousand-line files
	// (50-200 ms per call for a 5k-line Go file). When the cache misses
	// — e.g. demo paths that never went through loadViewerFile — fall back
	// to a one-shot highlight so behavior stays consistent.
	//
	// Edit mode skips highlighting entirely. Each keystroke would otherwise
	// invalidate the cache and force a per-frame re-tokenise; plain text
	// also makes the cursor cell easier to see (no competing colors on
	// the character under the cursor).
	var styledLines []string
	if m.viewerMode != ViewerEdit {
		switch {
		case m.viewerCachedFile == path && len(m.viewerCachedHL) == len(lines):
			styledLines = m.viewerCachedHL
		default:
			if hl, ok := highlightCode(content, path, p); ok && len(hl) == len(lines) {
				styledLines = hl
			}
		}
	}

	dimNum := lipgloss.NewStyle().Foreground(p.TextFade)
	codeStyle := lipgloss.NewStyle().Foreground(p.TextMid)
	addedNum := lipgloss.NewStyle().Foreground(p.Green).Bold(true)
	addedSep := lipgloss.NewStyle().Foreground(p.Green)
	addedCode := lipgloss.NewStyle().Foreground(p.Green)
	removedNum := lipgloss.NewStyle().Foreground(p.Red).Bold(true)
	removedSep := lipgloss.NewStyle().Foreground(p.Red)
	removedCode := lipgloss.NewStyle().Foreground(p.Red)

	// Two diff sources feed the viewer:
	//   1. m.viewerDiffLines: lazily-fetched hunks for any modified file
	//      the user opened from the explorer — works for every changed
	//      file, not just the one claude is currently editing.
	//   2. m.data.DiffLines: the focused session's DiffFile hunks. These
	//      come "for free" each snapshot and are the right source when
	//      the user is viewing the file claude is mid-edit.
	// Prefer (1) when its file matches; fall back to (2) otherwise.
	addedLines, removalAdjacent := viewerDiffMapsFor(path, m.viewerDiffFile, m.viewerDiffLines, m.data)

	// contentInnerW is the per-line width budget after we reserve 2 cells
	// for the scrollbar (rendered by renderViewerScrollbar after slicing).
	contentInnerW := innerW - 2
	xOff := m.viewport.xOffset
	if xOff < 0 {
		xOff = 0
	}
	styles := viewerLineStyles{
		dimNum:      dimNum,
		codeStyle:   codeStyle,
		addedNum:    addedNum,
		addedSep:    addedSep,
		addedCode:   addedCode,
		removedNum:  removedNum,
		removedSep:  removedSep,
		removedCode: removedCode,
	}
	numbered := make([]string, 0, len(lines))
	for i, line := range lines {
		var styled string
		if styledLines != nil {
			styled = styledLines[i]
		}
		numbered = append(numbered, renderViewerLine(line, styled, i+1, numWidth, xOff, styles, addedLines, removalAdjacent))
	}

	// innerH chrome accounting:
	//   1 header line + 1 separator line + N hint lines at the bottom.
	// The hint row grows when :command mode is open (palette overlay)
	// or the / find prompt is open (two-field find/replace).
	hintRows := 1
	if m.viewerCmdActive {
		hintRows = commandPaletteRows() + 1 // palette + prompt
	}
	if m.viewerFindActive {
		hintRows = 2 // find line + replace line
	}
	vpHeight := innerH - 2 - hintRows
	if vpHeight < 1 {
		vpHeight = 1
	}

	// Clamp YOffset against the no-wrap max so resizes don't leave the
	// viewport scrolled past the end of the file.
	maxOffset := totalLines - vpHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	yOff := m.viewport.vp.YOffset()
	if m.viewerMode == ViewerEdit {
		// Auto-scroll so the cursor stays inside the visible window. We
		// override yOff outright instead of clamping the existing value
		// — without that, typing past the bottom row would push the
		// cursor off-screen and the user couldn't see what they were
		// inserting.
		cl := m.viewerEdit.Line
		if cl < yOff {
			yOff = cl
		} else if cl >= yOff+vpHeight {
			yOff = cl - vpHeight + 1
		}
	}
	if yOff > maxOffset {
		yOff = maxOffset
	}
	if yOff < 0 {
		yOff = 0
	}

	// Slice the visible window directly off the numbered slice instead of
	// going through bubbles/v2/viewport.SetContent + View(). The viewport
	// path was a layered indirection that occasionally returned content
	// whose visual line count diverged from vpHeight (e.g. styled syntax
	// lines wider than the viewport width could expand visual rows even
	// with SoftWrap off, which pushed the bottom hint out of the panel
	// for files with very long lines). Direct slicing keeps the math
	// honest: exactly vpHeight lines, padded with empty when scrolled
	// near the end.
	end := yOff + vpHeight
	if end > len(numbered) {
		end = len(numbered)
	}
	// Cursor overlay: in edit mode we paint an inverted cell at the
	// cursor's (line, col). Built outside the visible loop so we can
	// match it against the screen-row index (line - yOff). The paint
	// step replaces that one rune in the rendered visible line — keeping
	// line numbers and surrounding code untouched.
	cursorScreenRow := -1
	cursorCol := -1
	if m.viewerMode == ViewerEdit {
		if m.viewerEdit.Line >= yOff && m.viewerEdit.Line < end {
			cursorScreenRow = m.viewerEdit.Line - yOff
			cursorCol = m.viewerEdit.Col
		}
	}
	visible := make([]string, 0, vpHeight)
	if yOff < len(numbered) {
		for _, line := range numbered[yOff:end] {
			// Truncate-then-pad each visible line to contentInnerW so the
			// scrollbar column lands at a fixed x. Without padding, lines
			// shorter than the budget would push the scrollbar leftwards
			// and break the panel right-edge alignment.
			//
			// Width clip uses BOTH ansi.StringWidth and lipgloss.Width: the
			// outer wrapPanel pads via lipgloss.Width while we truncate via
			// ansi, and the two can disagree on chroma-emitted SGR streams
			// (uniseg-grapheme handling vs. raw cell count). Disagreement
			// caused the reported "long Go lines push the panel border off
			// screen" bug. clipToWidth iterates until lipgloss agrees.
			line = clipToWidth(line, contentInnerW)
			visible = append(visible, padLine(line, contentInnerW))
		}
	}
	for len(visible) < vpHeight {
		visible = append(visible, strings.Repeat(" ", contentInnerW))
	}

	// Paint the cursor cell. Done after padding so the inversion is on
	// the visible (already width-clipped) version of the line.
	if cursorScreenRow >= 0 && cursorScreenRow < len(visible) {
		visible[cursorScreenRow] = paintCursorCell(
			visible[cursorScreenRow], cursorCol, numWidth, xOff, p,
		)
	}

	// Paint find-match highlights. Each match on a visible line gets a
	// background overlay; the focused match (m.viewerFindIdx) is bold so
	// the user can tell which one the viewport jumped to.
	if len(m.viewerFindMatches) > 0 {
		for i, match := range m.viewerFindMatches {
			row := match.Line - yOff
			if row < 0 || row >= len(visible) {
				continue
			}
			focused := i == m.viewerFindIdx
			visible[row] = paintFindMatch(
				visible[row], match.Col, match.End-match.Col,
				numWidth, xOff, p, focused,
			)
		}
	}

	// Paint the active selection in edit mode. Visible portion only —
	// rows outside the window are left untouched so the highlight scrolls
	// with the buffer view.
	if start, end, ok := m.activeSelectionRange(); ok {
		for srcLine := start.Line; srcLine <= end.Line; srcLine++ {
			row := srcLine - yOff
			if row < 0 || row >= len(visible) {
				continue
			}
			fromCol := 0
			toCol := -1 // sentinel: paint to end of line
			if srcLine == start.Line {
				fromCol = start.Col
			}
			if srcLine == end.Line {
				toCol = end.Col
			}
			visible[row] = paintSelectionSpan(visible[row], fromCol, toCol, numWidth, xOff, p)
		}
	}

	// Manual scroll-percent computation.
	var scrollPct float64
	if maxOffset > 0 {
		scrollPct = float64(yOff) / float64(maxOffset)
	} else {
		scrollPct = 1.0 // content fits — thumb at bottom (nothing more below)
	}

	viewContent := strings.Join(visible, "\n")
	viewContent = renderViewerScrollbar(p, viewContent, scrollPct)

	// Header (title bar + separator). Mode badge ([VIEW] or [INSERT]) and
	// the dirty marker ("*") are merged into the meta string so the user
	// can tell at a glance which mode they're in and whether their last
	// edit is on disk yet.
	pctText := fmt.Sprintf("%.0f%%", scrollPct*100)
	modeBadge := "VIEW"
	if m.viewerMode == ViewerEdit {
		modeBadge = "INSERT"
	}
	dirtyMark := ""
	if m.viewerDirty {
		dirtyMark = " *"
	}
	meta := fmt.Sprintf("%s · %d lines · %s · %s%s",
		humanSize(fileSize), totalLines, pctText, modeBadge, dirtyMark)
	header := renderViewerHeader(p, path, innerW, meta)

	hintStyle := lipgloss.NewStyle().Foreground(p.TextFade)
	statusStyle := lipgloss.NewStyle().Foreground(p.Cyan)
	hint := vimHintLine()
	if m.viewerMode == ViewerEdit {
		hint = editHintLine()
	}
	hintRow := hintStyle.Render(hint)
	if m.viewerStatus != "" {
		hintRow = statusStyle.Render(m.viewerStatus)
	}
	// Command prompt overrides the hint row AND grows the body upward
	// with a multi-line command palette so the user can discover what
	// `:command`s are available without leaving the prompt. Each row in
	// the palette is "<form>  <description>"; matching forms (prefix-
	// match against viewerCmdBuf) are highlighted, the rest dimmed.
	if m.viewerCmdActive {
		promptStyle := lipgloss.NewStyle().Foreground(p.Pink).Bold(true)
		cursor := lipgloss.NewStyle().Foreground(p.Bg).Background(p.Pink).Render(" ")
		prompt := promptStyle.Render(":") +
			lipgloss.NewStyle().Foreground(p.Text).Render(m.viewerCmdBuf) +
			cursor
		palette := renderCommandPalette(p, m.viewerCmdBuf, contentInnerW+2)
		hintRow = palette + "\n" + prompt
	}
	// Find prompt: two-line overlay with `/find` and `→replace` fields.
	// Tab toggles which field receives input; Enter executes — find-only
	// when replace is empty, find+replace otherwise. The active field
	// shows a block cursor; both empty fields show a faded placeholder
	// ("find file…" / "replace with… (optional)") so the user knows
	// what each row does without referencing docs.
	if m.viewerFindActive {
		promptStyle := lipgloss.NewStyle().Foreground(p.Cyan).Bold(true)
		dimStyle := lipgloss.NewStyle().Foreground(p.TextDim)
		fadeStyle := lipgloss.NewStyle().Foreground(p.TextFade).Italic(true)
		textStyle := lipgloss.NewStyle().Foreground(p.Text)
		cursor := lipgloss.NewStyle().Foreground(p.Bg).Background(p.Cyan).Render(" ")
		hint := dimStyle.Render("  ↹ switch field  ↵ apply  esc cancel")

		findGlyph := promptStyle.Render("/")
		replaceGlyph := dimStyle.Render("→")
		if m.viewerFindFocusReplace {
			replaceGlyph = promptStyle.Render("→")
		}

		findField := promptFieldRender(
			m.viewerFindQuery, "find text…",
			!m.viewerFindFocusReplace, cursor, textStyle, fadeStyle,
		)
		replaceField := promptFieldRender(
			m.viewerFindReplace, "replace with… (optional)",
			m.viewerFindFocusReplace, cursor, textStyle, fadeStyle,
		)
		hintRow = findGlyph + findField + "\n" + replaceGlyph + replaceField + hint
	} else if len(m.viewerFindMatches) > 0 {
		idxStyle := lipgloss.NewStyle().Foreground(p.Cyan)
		hintRow = idxStyle.Render(fmt.Sprintf("/%s  [%d/%d]  n next  N prev",
			m.viewerFindQuery, m.viewerFindIdx+1, len(m.viewerFindMatches)))
	}

	body := header + "\n" + viewContent + "\n" + hintRow
	// Panel label flips to "editor" while in edit mode so the user sees
	// at a glance which mode they're in. The chrome already shows
	// [INSERT] in the meta string, but the title is the more prominent
	// signal and matches what users expect from "i opens the editor".
	label := "viewer"
	if m.viewerMode == ViewerEdit {
		label = "editor"
	}
	return wrapPanel(s, body, label, "esc close", width, height, true)
}

// viewerLineStyles bundles the styled renderers used by renderViewerLine so
// renderTextViewer's line loop stays under the gocognit complexity limit.
type viewerLineStyles struct {
	dimNum, codeStyle                   lipgloss.Style
	addedNum, addedSep, addedCode       lipgloss.Style
	removedNum, removedSep, removedCode lipgloss.Style
}

// renderViewerLine builds a single styled viewer row: line number + gutter +
// horizontally-offset code text. Diff-touched lines (added / removal-adjacent)
// are colored green / red; all others use the syntax-highlighted body when
// available, falling back to dim plain text. Diff signals override syntax —
// the green/red overlay communicates change, and that's the more important
// channel for a user scanning a viewer in clyde.
//
// The styled argument is the pre-highlighted line (chroma output) or "" when
// we have no lexer for this file. Horizontal scrolling on styled lines uses
// ansi.TruncateLeft so we never slice through an SGR escape; on raw lines
// plain byte slicing is safe because there are no escapes to corrupt.
func renderViewerLine(raw, styled string, lineNo, numWidth, xOff int, st viewerLineStyles, added, removalAdjacent map[int]bool) string {
	switch {
	case added[lineNo]:
		display := raw
		if xOff >= len(display) {
			display = ""
		} else if xOff > 0 {
			display = display[xOff:]
		}
		return st.addedNum.Render(fmt.Sprintf("%*d", numWidth, lineNo)) +
			st.addedSep.Render(" │ ") +
			st.addedCode.Render(display)
	case removalAdjacent[lineNo]:
		display := raw
		if xOff >= len(display) {
			display = ""
		} else if xOff > 0 {
			display = display[xOff:]
		}
		return st.removedNum.Render(fmt.Sprintf("%*d", numWidth, lineNo)) +
			st.removedSep.Render(" │ ") +
			st.removedCode.Render(display)
	}

	if styled != "" {
		display := styled
		if xOff > 0 {
			display = ansi.TruncateLeft(display, xOff, "")
		}
		return st.dimNum.Render(fmt.Sprintf("%*d", numWidth, lineNo)) +
			st.dimNum.Render(" │ ") +
			display
	}

	display := raw
	if xOff >= len(display) {
		display = ""
	} else if xOff > 0 {
		display = display[xOff:]
	}
	return st.dimNum.Render(fmt.Sprintf("%*d", numWidth, lineNo)) +
		st.dimNum.Render(" │ ") +
		st.codeStyle.Render(display)
}

// viewerDiffMapsFor picks the right diff source for the viewer file and
// delegates to the kind-walker. When viewerDiffFile is set and matches
// the open path, those lazily-fetched hunks win; otherwise we fall back
// to the focused session's d.DiffFile / d.DiffLines (the file claude is
// currently editing).
func viewerDiffMapsFor(path, viewerDiffFile string, viewerDiffLines []DiffLine, d MockData) (added, removalAdjacent map[int]bool) {
	if viewerDiffFile != "" && len(viewerDiffLines) > 0 && basenameEqual(path, viewerDiffFile) {
		return diffMapsFromLines(viewerDiffLines)
	}
	if basenameEqual(path, d.DiffFile) && len(d.DiffLines) > 0 {
		return diffMapsFromLines(d.DiffLines)
	}
	return nil, nil
}

// diffMapsFromLines walks a DiffLine slice and returns the added / removal-
// adjacent line-number sets used by the viewer for green / red highlighting.
func diffMapsFromLines(lines []DiffLine) (added, removalAdjacent map[int]bool) {
	added = map[int]bool{}
	removalAdjacent = map[int]bool{}
	pendingRemoval := false
	for _, dl := range lines {
		switch dl.Kind {
		case DiffHunkKind:
			pendingRemoval = false
		case DiffAddKind:
			n := atoiOrZero(dl.LineNo)
			if n > 0 {
				added[n] = true
			}
			pendingRemoval = false
		case DiffRemKind:
			pendingRemoval = true
		case DiffCtxKind:
			if pendingRemoval {
				n := atoiOrZero(dl.LineNo)
				if n > 0 {
					removalAdjacent[n] = true
				}
				pendingRemoval = false
			}
		}
	}
	return added, removalAdjacent
}

// atoiOrZero parses s as int; returns 0 on failure (used for diff line nums).
func atoiOrZero(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// renderViewerHeader renders the viewer's two-line header (title + separator).
// The icon is placed before the basename, with the path and the meta string
// (size · lines · scroll-pct) trailing in a dim color.
func renderViewerHeader(p Palette, path string, innerW int, meta string) string {
	titleStyle := lipgloss.NewStyle().Foreground(p.Purple).Bold(true)
	metaStyle := lipgloss.NewStyle().Foreground(p.TextDim)

	icon := renderFileIcon(fileBasename(path))
	titleStr := icon + " " + titleStyle.Render(fileBasename(path)) +
		"  " + metaStyle.Render(path) +
		"  " + metaStyle.Render(meta)

	sep := strings.Repeat("─", clamp(innerW, 1, innerW))
	sepLine := lipgloss.NewStyle().Foreground(p.TextFade).Render(sep)
	return titleStr + "\n" + sepLine
}

// loadViewerFile is the open-time hook that primes everything the viewer
// needs to render the given path:
//   - Sets m.viewerFile / m.viewerActive.
//   - Reads + caches file content (capped at maxViewerBytes).
//   - Pre-tokenises with chroma so the per-frame render is cache-only.
//   - Resets the bubbles viewport to the top.
//
// Replacing the old "set viewerFile + viewport.LoadFile" pair with this one
// call is what gets us off the per-frame I/O + tokenisation hot path. The
// cost moves to a single open-time spike that the user already expects
// when they hit Enter.
func (m Model) loadViewerFile(path string) Model {
	m.viewerActive = true
	m.viewerFile = path
	cwd := m.liveView.Project.CWD()
	m.viewport.LoadFile(path, cwd, m.demoMode)

	var content string
	var size int64
	if m.demoMode {
		mock, ok := mockFileContent[normalizePath(path)]
		if !ok {
			mock = fmt.Sprintf("  (no mock content for %s)", path)
		}
		content = mock
		size = int64(len(content))
	} else {
		raw, sz, err := readFileForViewerWithSize(path, cwd)
		if err != nil {
			content = fmt.Sprintf("  error reading file: %s", err)
			size = 0
		} else {
			content = raw
			size = sz
		}
	}
	// Tabs render as N cells in the terminal (typically 4 or 8) but the
	// width-measurement libraries we use (lipgloss.Width, ansi.StringWidth)
	// count them as a single cell. The disagreement causes Go files (which
	// use tab indentation) to push past the panel right border. Expand to
	// spaces here, before caching, so every downstream width measurement
	// matches the terminal's actual cell consumption.
	content = expandTabs(content, viewerTabWidth)
	m.viewerCachedFile = path
	m.viewerCachedContent = content
	m.viewerCachedSize = size

	// Edit state is per-file: opening a new file resets the buffer + cursor
	// + dirty marker so the previous file's in-progress edits don't leak
	// across. ViewerView is the natural landing mode (no surprise inserts).
	m.viewerEdit = newEditBuffer(content)
	m.viewerMode = ViewerView
	m.viewerDirty = false
	m.viewerStatus = ""

	// Pre-highlight only when chroma has a lexer for this path. Skipping
	// the call on unknown extensions saves the analyser-on-content fallback
	// cost — short snippets and binary-ish files fall through to dim
	// plaintext rendering anyway.
	m.viewerCachedHL = nil
	if hasLexerFor(path) {
		lines := strings.Split(content, "\n")
		if hl, ok := highlightCode(content, path, m.palette); ok && len(hl) == len(lines) {
			m.viewerCachedHL = hl
		}
	}
	return m
}

// readFileForViewerWithSize reads a file like readFileForViewer and also
// returns its size in bytes for display in the viewer header.
func readFileForViewerWithSize(path, cwd string) (string, int64, error) {
	absPath := path
	if len(path) == 0 || path[0] != '/' {
		if cwd != "" {
			absPath = cwd + "/" + path
		}
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return "", 0, err
	}
	if info.Size() > maxViewerBytes {
		return fmt.Sprintf("  file too large (%s) — open in editor for full view", humanSize(info.Size())), info.Size(), nil
	}

	raw, err := os.ReadFile(absPath)
	if err != nil {
		return "", info.Size(), err
	}
	return string(raw), info.Size(), nil
}

// ─── Image viewer ─────────────────────────────────────────────────────────────

// renderImageViewer renders an image file as an ANSI half-block preview.
//
// Each terminal cell encodes 2 vertical pixels via the upper-half-block glyph
// (foreground = top pixel, background = bottom pixel). Universal — works in
// any 24-bit-color terminal (iTerm2, Terminal.app, Ghostty, kitty, alacritty,
// even tmux). Kitty's native graphics protocol is more pixel-perfect but
// requires emitting APC sequences outside the lipgloss composition pass; that
// optimization is still deferred (see kitty_graphics.go).
//
// On decode failure (e.g. SVG, WebP without the x/image module wired up,
// corrupt file) we render a friendly error panel instead of silently
// degrading to an empty preview.
func (m Model) renderImageViewer(s Styles, p Palette, path string, width, height int) string {
	innerW := width - 4
	innerH := height - 2
	if innerW < 10 {
		innerW = 10
	}
	if innerH < 6 {
		innerH = 6
	}

	titleStyle := lipgloss.NewStyle().Foreground(p.Purple).Bold(true)
	metaStyle := lipgloss.NewStyle().Foreground(p.TextDim)
	errStyle := lipgloss.NewStyle().Foreground(p.TextDim)

	icon := renderFileIcon(fileBasename(path))
	header := func(meta string) string {
		titleStr := icon + " " + titleStyle.Render(fileBasename(path)) +
			"  " + metaStyle.Render(path) +
			"  " + metaStyle.Render(meta)
		sep := strings.Repeat("─", clamp(innerW, 1, innerW))
		sepLine := lipgloss.NewStyle().Foreground(p.TextFade).Render(sep)
		return titleStr + "\n" + sepLine
	}

	// Demo mode: no real bytes on disk for the placeholder paths, fall back
	// to the procedural pattern so golden tests stay deterministic.
	if m.demoMode {
		body := header("(image preview — demo)") + "\n" + renderASCIIImagePlaceholder(innerW, innerH-4)
		return wrapPanel(s, body, "viewer", "esc close", width, height, true)
	}

	data, sz, err := readImageBytes(path, m.liveView.Project.CWD())
	if err != nil {
		body := header("error reading image") +
			"\n" + errStyle.Render("  "+err.Error())
		return wrapPanel(s, body, "viewer", "esc close", width, height, true)
	}

	previewRows := innerH - 3 // header + separator + vim-hint line
	if previewRows < 1 {
		previewRows = 1
	}
	preview, srcSize, err := renderImagePreview(data, innerW, previewRows)
	if err != nil {
		body := header(fmt.Sprintf("%s · format not supported", humanSize(sz))) +
			"\n" + errStyle.Render("  preview not available — "+err.Error())
		return wrapPanel(s, body, "viewer", "esc close", width, height, true)
	}

	hintStyle := lipgloss.NewStyle().Foreground(p.TextFade)
	meta := fmt.Sprintf("%dx%d · %s", srcSize.X, srcSize.Y, humanSize(sz))
	body := header(meta) + "\n" + preview + "\n" + hintStyle.Render(vimHintLine())
	return wrapPanel(s, body, "viewer", "esc close", width, height, true)
}

// viewerTabWidth is the cell count used to expand tab characters in viewer
// content. 4 matches what most editors / Github diffs render at — visible
// indentation without taking too much horizontal real estate, especially
// in narrow side-pane usage.
const viewerTabWidth = 4

// expandTabs replaces every '\t' with enough spaces to reach the next
// column that is a multiple of width. Tracks per-line column position
// because resetting on each newline matches what the terminal does (the
// elastic-tabstops behavior is intentional — a tab in the middle of a
// line aligns to the next column boundary, not always +width spaces).
func expandTabs(s string, width int) string {
	if width <= 0 || !strings.ContainsRune(s, '\t') {
		return s
	}
	var sb strings.Builder
	sb.Grow(len(s) + len(s)/8)
	col := 0
	for _, r := range s {
		switch r {
		case '\n':
			sb.WriteRune(r)
			col = 0
		case '\t':
			// Pad to next tab stop.
			pad := width - (col % width)
			for range pad {
				sb.WriteByte(' ')
			}
			col += pad
		default:
			sb.WriteRune(r)
			col++
		}
	}
	return sb.String()
}

// clipToWidth returns line clipped to a visible width of at most w, using
// lipgloss.Width as the source of truth (because that's what padLine and
// the panel wrapper consume downstream). ansi.Truncate is the workhorse
// for the actual cut, but on tricky chroma output ansi.StringWidth and
// lipgloss.Width can disagree — we therefore loop, shrinking the budget
// by one cell per iteration, until lipgloss is satisfied. As a final
// safety net we strip SGR escapes and rune-slice the plain text, which
// guarantees termination.
func clipToWidth(line string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(line) <= w {
		return line
	}
	budget := w
	for budget > 0 {
		candidate := ansi.Truncate(line, budget, "")
		if lipgloss.Width(candidate) <= w {
			return candidate
		}
		budget--
	}
	// Last resort: strip styles and rune-slice. Loses color but keeps the
	// panel border from breaking, which is the more visible bug.
	plain := ansi.Strip(line)
	rs := []rune(plain)
	if len(rs) > w {
		rs = rs[:w]
	}
	return string(rs)
}

// readImageBytes reads an image file from disk, resolving relative paths
// against the project cwd. Caps reads at maxImageBytes so a multi-megabyte
// PNG can't lock the UI thread on large-image decode.
func readImageBytes(path, cwd string) ([]byte, int64, error) {
	absPath := path
	if len(path) == 0 || path[0] != '/' {
		if cwd != "" {
			absPath = cwd + "/" + path
		}
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, 0, err
	}
	if info.Size() > maxImageBytes {
		return nil, info.Size(), fmt.Errorf("image too large (%s) — preview cap is %s",
			humanSize(info.Size()), humanSize(maxImageBytes))
	}
	raw, err := os.ReadFile(absPath)
	if err != nil {
		return nil, info.Size(), err
	}
	return raw, info.Size(), nil
}

// maxImageBytes caps how much of an image file we load. Stdlib's image
// decoder reads bytes lazily but memory peaks for large PNGs are real;
// 8 MB is comfortable for the screenshot/diagram use case without inviting
// pathological input.
const maxImageBytes = 8 * 1024 * 1024

// renderASCIIImagePlaceholder renders an improved ASCII art image preview using
// ▓▒░ block characters with brightness-mapped rows to simulate a real image.
//
// The palette creates a gradient from bright (top-left) to dim (bottom-right),
// with a subtle diagonal sweep — more convincing than a static tile pattern.
// Kitty graphics protocol emission is deferred to V2 (see kitty_graphics.go).
func renderASCIIImagePlaceholder(width, height int) string {
	if width < 4 {
		width = 4
	}
	if height < 3 {
		height = 3
	}

	// brightness maps a (row, col) position to a block character.
	// We compute a brightness value in [0, 3] using a diagonal gradient
	// so the image looks like it has light from the top-left corner.
	brightness := func(row, col, rows, cols int) string {
		// Diagonal brightness: b ∈ [0.0, 1.0]
		b := 1.0 - (float64(row)/float64(rows)*0.5 + float64(col)/float64(cols)*0.5)
		// Add a subtle wave to avoid flat monotone bands.
		wave := 0.08 * sineApprox(float64(row*3+col*2)*0.4)
		b += wave
		switch {
		case b >= 0.70:
			return "▓"
		case b >= 0.45:
			return "▒"
		case b >= 0.20:
			return "░"
		default:
			return " "
		}
	}

	var lines []string
	for row := 0; row < height; row++ {
		var sb strings.Builder
		for col := 0; col < width; col++ {
			sb.WriteString(brightness(row, col, height, width))
		}
		lines = append(lines, sb.String())
	}
	return strings.Join(lines, "\n")
}

// sineApprox returns a rough sine approximation in [-1, 1] for a given angle
// in radians using a Bhaskara I polynomial approximation.  No math import needed.
func sineApprox(angle float64) float64 {
	// Reduce to [0, 2π] range with a simple mod-like operation.
	const pi = 3.14159265358979323846
	const twoPi = 2 * pi
	for angle > twoPi {
		angle -= twoPi
	}
	for angle < 0 {
		angle += twoPi
	}
	// Map to [-π, π].
	if angle > pi {
		angle -= twoPi
	}
	// Bhaskara I approximation: sin(x) ≈ 16x(π−x) / (5π² − 4x(π−x))
	x := angle
	num := 16 * x * (pi - x)
	den := 5*pi*pi - 4*x*(pi-x)
	if den == 0 {
		return 0
	}
	return num / den
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// readFileForViewer reads a file for the viewer, resolving relative paths against
// cwd. Returns an error string when the file cannot be read. Files larger than
// maxViewerBytes return a "file too large" message instead of content.
func readFileForViewer(path, cwd string) (string, error) {
	// Resolve relative paths against the project cwd.
	absPath := path
	if len(path) == 0 || path[0] != '/' {
		if cwd != "" {
			absPath = cwd + "/" + path
		}
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return "", err
	}
	if info.Size() > maxViewerBytes {
		return fmt.Sprintf("  file too large (%d KB) — open in editor for full view", info.Size()/1024), nil
	}

	raw, err := os.ReadFile(absPath)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// isImageFile returns true for known image extensions.
func isImageFile(path string) bool {
	lower := strings.ToLower(path)
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// fileBasename returns the last path component.
func fileBasename(path string) string {
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

// normalizePath strips leading "src/" and similar prefixes for mock lookup.
func normalizePath(path string) string {
	return strings.TrimPrefix(path, "./")
}

// mockFileContent is the lookup table for openable mock files.
var mockFileContent = map[string]string{
	"src/api/auth.ts": authTSContent,
	"README.md":       readmeMDContent,
	"public/logo.png": "", // image — handled by renderImageViewer
}

// authTSContent is the mock content for src/api/auth.ts — matches the diff fixture.
const authTSContent = `import { verify, decode } from './jwt';
import { db } from './database';

/**
 * authenticate — validates a JWT token and returns the session user.
 * Returns { ok: false, reason: string } on any failure.
 */
export async function authenticate(token: string) {
  if (!token) {
    return { ok: false, reason: 'missing' };
  }

  const decoded = await verify(token);
  if (!decoded?.sub) {
    return { ok: false, reason: 'expired' };
  }

  return { ok: true, user: decoded };
}

/**
 * refreshToken — issues a new access token from a valid refresh token.
 * Returns null if the refresh token is missing or expired.
 */
export async function refreshToken(uid: string) {
  const stored = await db.get(uid);
  if (!stored?.refreshExp) return null;
  if (stored.refreshExp < now()) {
    await db.del(uid);
    return null;
  }

  return signToken({ sub: uid });
}

/**
 * verifyToken — wrapper that decodes without throwing.
 * Use in middleware where expired tokens should degrade gracefully.
 */
export function verifyToken(token: string) {
  try {
    return decode(token);
  } catch {
    return null;
  }
}

function now(): number {
  return Math.floor(Date.now() / 1000);
}`

// readmeMDContent is the mock content for README.md.
const readmeMDContent = `# claude-companion

A TUI companion for Claude Code. Opens alongside ` + "`claude`" + ` in a split
terminal window. Read-mostly observer with light interactivity.

## Features

- Live task list pulled from claude's TodoWrite calls
- Accumulating diff view for files claude is editing
- MCP/LSP server status panel
- Token usage + cost tracking
- File explorer with session modifications highlighted

## Usage

` + "```bash" + `
# Build
go build ./cmd/clyde

# Run
./clyde
` + "```" + `

## Key Bindings

| Key       | Action              |
|-----------|---------------------|
| Tab       | Next panel          |
| Shift+Tab | Previous panel      |
| ↑ / ↓     | Navigate / scroll   |
| Enter     | Open / expand       |
| Esc       | Close viewer        |
| Ctrl+L    | Cycle layout mode   |
| q         | Quit                |
`

// MockFileContent returns the mock content for a given file path.
// Exported for use by the viewport loader in model.go.
func MockFileContent(path string, _ MockData) string {
	if content, ok := mockFileContent[normalizePath(path)]; ok {
		return content
	}
	return fmt.Sprintf("(no mock content for %q — will be populated in v7)", path)
}
