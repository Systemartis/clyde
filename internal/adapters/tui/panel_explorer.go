package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
)

// ─── ExplorerState ────────────────────────────────────────────────────────────

// ExplorerRow is a visible row in the interactive tree.
type ExplorerRow struct {
	// DisplayName is the rendered node name (▼ src, Sidebar.tsx, etc.)
	DisplayName string
	// Path is the full relative path for files, dir-name for dirs.
	Path string
	// IsDir signals whether this row is a directory.
	IsDir bool
	// IsAct flags the currently active file.
	IsAct bool
	// Mark is "M", "+", or "".
	Mark string
	// Indent prefix (│ characters).
	Indent string
	// DirKey is the canonical dir name used to look up collapse state.
	DirKey string
}

// ExplorerSection identifies which of the two scrollable regions in the
// explorer panel currently owns the keyboard cursor.
type ExplorerSection int

const (
	// SectionTree — cursor is in the tree below the modified-files window.
	SectionTree ExplorerSection = iota
	// SectionMod — cursor is in the modified-files window above the tree.
	SectionMod
)

// ExplorerState holds the interactive explorer panel state.
// All fields are value-type — no pointers — to keep Model copyable.
type ExplorerState struct {
	// section identifies which region the keyboard cursor is in. Up/down
	// crosses the boundary: ↓ at the bottom of mod jumps to the top of
	// tree; ↑ at the top of tree jumps to the bottom of mod.
	section ExplorerSection
	// highlighted is the index of the currently selected row in the tree.
	// Always valid; rendering only paints it when section == SectionTree.
	highlighted int
	// modHighlight is the index of the currently selected row in the
	// modified-files window. Painted only when section == SectionMod.
	modHighlight int
	// collapsed tracks which directories are collapsed (true = collapsed).
	// Key = DirKey (the raw dir name stored in TreeNode.Name after stripping ▼/▶).
	collapsed map[string]bool
	// rows is the computed list of visible rows (rebuilt on expand/collapse).
	rows []ExplorerRow
	// treeScrollOff is the scroll offset into the tree section. The renderer
	// adjusts it on every paint so that the highlighted row stays visible.
	// Mouse wheel events also write here directly.
	treeScrollOff int
	// modScrollOff is the scroll offset into the "modified files" section.
	// When the user has more changed files than the panel can show in its
	// fractional budget, this offset lets them scroll the list with the
	// mouse wheel (over the modified region) without losing the tree
	// underneath.
	modScrollOff int
	// search holds the search-overlay state. When search.Active is true,
	// the explorer hides its normal tree+modified split and shows a flat
	// match list bound to search.Query — see panel_explorer_search.go.
	// Type-printable keys append to the query, ↑/↓ navigate, Enter opens
	// the highlighted match, Esc / empty-query backspace exits.
	search ExplorerSearch
}

// NewExplorerState builds an ExplorerState from mock data.
// All top-level directories start collapsed so the explorer is compact on
// startup; the user expands individual directories with Enter or Space.
func NewExplorerState(d MockData) ExplorerState {
	collapsed := make(map[string]bool)

	// Pre-collapse every directory node so the tree starts in a tidy state.
	// The modified-files section at the top is a flat list and is unaffected.
	for _, node := range d.Tree {
		if node.IsDir {
			// DirKey = name without the ▼/▶ prefix.
			dirKey := strings.TrimPrefix(node.Name, "▼ ")
			dirKey = strings.TrimPrefix(dirKey, "▶ ")
			collapsed[dirKey] = true
		}
	}

	es := ExplorerState{collapsed: collapsed}
	es.rows = buildVisibleRows(d, es.collapsed)
	return es
}

// buildVisibleRows builds the visible tree rows respecting collapse state.
// A collapsed directory hides all its children.
func buildVisibleRows(d MockData, collapsed map[string]bool) []ExplorerRow {
	var rows []ExplorerRow
	skipUntilIndentLen := -1 // when >= 0, skip rows whose indent >= this depth

	for _, node := range d.Tree {
		indentDepth := len([]rune(node.Indent)) // rune count of indent prefix

		// If we're inside a collapsed dir, skip until we surface back up
		if skipUntilIndentLen >= 0 && indentDepth > skipUntilIndentLen {
			continue
		} else {
			skipUntilIndentLen = -1
		}

		row := ExplorerRow{
			Indent: node.Indent,
			IsDir:  node.IsDir,
			IsAct:  node.IsAct,
			Mark:   node.Mark,
		}

		if node.IsDir {
			// Strip existing ▼/▶ to get the canonical dir name
			dirKey := strings.TrimPrefix(node.Name, "▼ ")
			dirKey = strings.TrimPrefix(dirKey, "▶ ")
			row.DirKey = dirKey
			row.Path = dirKey

			if collapsed[dirKey] {
				row.DisplayName = "▶ " + dirKey
				// Mark that we should skip children at a deeper indent level
				skipUntilIndentLen = indentDepth
			} else {
				row.DisplayName = "▼ " + dirKey
			}
		} else {
			// Strip active-file chevron from display name
			displayName := strings.TrimPrefix(node.Name, "▸ ")
			row.DisplayName = displayName
			if node.FullPath != "" {
				row.Path = node.FullPath
			} else {
				row.Path = strings.TrimSpace(node.Indent) + node.Name
			}
		}

		rows = append(rows, row)
	}
	return rows
}

// explorerOuterHeight returns the outer panel height for the explorer
// in the current layout. ALL math comes from m.computeLayout() so the
// click handler agrees with the renderer on every dimension. Earlier
// versions duplicated the math here with stale constants (notifH=3,
// serversH=9) that disagreed with the renderer's real values
// (notificationHeight() and serversH=13), causing tree-row clicks to
// land one row higher than the cursor. Single source of truth fixes it.
func (m Model) explorerOuterHeight() int {
	l := m.computeLayout()
	switch {
	case l.Mode == LayoutMultiCol:
		return l.GridH
	case l.Mode == LayoutStack && l.BP == BreakpointMedium:
		return l.ExplorerH
	default:
		return clamp(m.panelHeight(PanelExplorer), 4, 40)
	}
}

// explorerTreeAreaH returns how many rows the tree section can display.
// Mirrors the math inside renderExplorer via the shared budget helper.
func (m Model) explorerTreeAreaH() int {
	h := m.explorerOuterHeight()
	innerH := h - 2
	_, t := explorerSectionBudget(innerH, len(m.data.ModifiedFiles))
	return t
}

// explorerModAreaH returns how many rows the modified-files section can
// display. Mirrors renderExplorer's budget so click + wheel routing can
// translate y coordinates back into row indices accurately.
func (m Model) explorerModAreaH() int {
	h := m.explorerOuterHeight()
	innerH := h - 2
	mod, _ := explorerSectionBudget(innerH, len(m.data.ModifiedFiles))
	return mod
}

// explorerVisibleScrollOff returns the scroll offset the renderer will use
// for the current state. Click handlers and other input code use this to
// translate visible-row indices to absolute tree indices.
func (m Model) explorerVisibleScrollOff() int {
	rows := buildVisibleRows(m.data, m.explorer.collapsed)
	return clampScrollOff(m.explorer.treeScrollOff, m.explorer.highlighted, len(rows), m.explorerTreeAreaH())
}

// MoveUp moves the highlight one row up across the unified
// modified+tree cursor. modCount is the number of modified files in the
// current view — when the cursor is at the top of the tree and any
// modified files exist, the cursor crosses into the modified section.
// Wrapping at the top of mod is intentionally disabled: knowing "I'm at
// the very top" is more useful than wrap-around in a small list.
func (es *ExplorerState) MoveUp(modCount int) {
	if es.section == SectionMod {
		if es.modHighlight > 0 {
			es.modHighlight--
		}
		return
	}
	if len(es.rows) == 0 && modCount > 0 {
		es.section = SectionMod
		es.modHighlight = modCount - 1
		return
	}
	if es.highlighted > 0 {
		es.highlighted--
		return
	}
	// At top of tree — cross into modified section if we have any.
	if modCount > 0 {
		es.section = SectionMod
		es.modHighlight = modCount - 1
	}
}

// MoveDown moves the highlight one row down across the unified
// modified+tree cursor. ↓ at the bottom of mod crosses into the tree
// at row 0; ↓ at the bottom of tree clamps (no wrap).
func (es *ExplorerState) MoveDown(modCount int) {
	if es.section == SectionMod {
		if es.modHighlight < modCount-1 {
			es.modHighlight++
			return
		}
		// At bottom of mod — cross into tree if any rows.
		if len(es.rows) > 0 {
			es.section = SectionTree
			es.highlighted = 0
		}
		return
	}
	if len(es.rows) == 0 {
		return
	}
	if es.highlighted < len(es.rows)-1 {
		es.highlighted++
	}
}

// ToggleDir expands or collapses a directory by its dirKey.
// After the toggle, the visible row list is rebuilt from the data stored
// in MockData — so we need it passed in.
func (es *ExplorerState) ToggleDir(dirKey string) {
	es.collapsed[dirKey] = !es.collapsed[dirKey]
}

// RefreshRows rebuilds the visible row list from mock data (call after toggle).
func (es *ExplorerState) RefreshRows(d MockData) {
	es.rows = buildVisibleRows(d, es.collapsed)
	// Clamp highlight
	if es.highlighted >= len(es.rows) {
		es.highlighted = len(es.rows) - 1
	}
	if es.highlighted < 0 {
		es.highlighted = 0
	}
}

// HighlightedNode returns a pointer to the currently highlighted row, or nil.
func (es *ExplorerState) HighlightedNode() *ExplorerRow {
	if len(es.rows) == 0 {
		return nil
	}
	r := es.rows[es.highlighted]
	return &r
}

// HighlightedPath returns the path of the highlighted row, normalized for mock lookup.
func (es *ExplorerState) HighlightedPath() string {
	if len(es.rows) == 0 {
		return ""
	}
	return es.rows[es.highlighted].Path
}

// ─── Rendering ────────────────────────────────────────────────────────────────

// buildExplorerViewportContent builds the full tree content for the explorer
// viewport (used in Expanded-Active mode for scrolling). Renders at the given
// inner width; the viewport handles clipping and scroll offset.
func buildExplorerViewportContent(s Styles, p Palette, d MockData, es ExplorerState, inner int) string {
	rows := buildVisibleRows(d, es.collapsed)

	hlBg := lipgloss.NewStyle().
		Background(lipgloss.Color("#2D2A3E")).
		Foreground(p.Text)
	hlBgDir := lipgloss.NewStyle().
		Background(lipgloss.Color("#2D2A3E")).
		Foreground(lipgloss.Color("#7aa2f7"))

	var lines []string
	for i, row := range rows {
		isHL := i == es.highlighted
		lines = append(lines, renderExplorerRow(s, p, row, inner, isHL, hlBg, hlBgDir))
	}
	if len(lines) == 0 {
		return ""
	}
	// Join without trailing newline — viewport adds its own line breaks.
	result := ""
	for i, l := range lines {
		if i > 0 {
			result += "\n"
		}
		result += l
	}
	return result
}

// renderExplorerExpanded renders the explorer in expanded state.
// Active mode reuses the dual-region renderer (modified-files window + tree
// with their own per-section scroll) and just swaps the border + label
// styling — the explorer never used the viewport-based active rendering
// other panels use because each region scrolls itself, and viewport-based
// scrolling would collapse them into a single window.
func renderExplorerExpanded(s Styles, p Palette, d MockData, es ExplorerState, _ viewport.Model, width, height int, focused, activeMode bool) string {
	return renderExplorerWithState(s, p, d, es, width, height, focused, activeMode)
}

// renderExplorer renders the explorer in passive (non-active) state.
// Thin wrapper over renderExplorerWithState.
func renderExplorer(s Styles, p Palette, d MockData, es ExplorerState, width, height int, focused bool) string {
	return renderExplorerWithState(s, p, d, es, width, height, focused, false)
}

// renderExplorerWithState renders the left file-explorer panel.
//
// Layout: a vertically-stacked panel with two scrollable regions —
// "modified files" on top, "tree" on the bottom — separated by a header
// dotline. Each region keeps its own scrollOff and renders its own 1-cell
// scrollbar on the right when the content overflows the budget. Without
// the per-region cap, a checkout with 30+ modified files used to push
// the tree off-screen entirely.
//
// activeMode swaps the border + label to the pink "active" style so the
// user can tell the panel is in interactive (cursor-driven) state. The
// keyboard cursor lives in es.section: SectionMod paints the modified
// row, SectionTree paints the tree row.
func renderExplorerWithState(s Styles, p Palette, d MockData, es ExplorerState, width, height int, focused, activeMode bool) string {
	inner := width - 4 // subtract border + 1-char padding each side
	if inner < 4 {
		inner = 4
	}

	// Search overlay takes over the whole panel body when active. We keep
	// the same border/title chrome — replacing the panel wholesale would
	// disorient the user (which panel am I in?) — and reserve the entire
	// inner area for the prompt + matches list.
	if es.search.Active {
		return renderExplorerSearchOverlay(s, p, es.search, inner, width, height, activeMode, focused)
	}

	rows := buildVisibleRows(d, es.collapsed)

	hlBg := lipgloss.NewStyle().
		Background(lipgloss.Color("#2D2A3E")).
		Foreground(p.Text)
	hlBgDir := lipgloss.NewStyle().
		Background(lipgloss.Color("#2D2A3E")).
		Foreground(lipgloss.Color("#7aa2f7"))

	innerH := height - 2
	modAreaH, treeAreaH := explorerSectionBudget(innerH, len(d.ModifiedFiles))

	dotLine := strings.Repeat("·", clamp(inner, 1, 20))

	var lines []string

	// ── search button row ────────────────────────────────────────────────────
	// Always rendered as the first content row so the user can mouse-click
	// to enter search even without knowing the / shortcut. Hit-testing on
	// the row is in mouse.go::explorerSearchButtonAtPos. Visible width is
	// inner cells; the row text is dim by default and brightens slightly
	// when the explorer panel itself is focused so it acts as a discoverable
	// affordance without competing with the tree highlight.
	lines = append(lines, renderExplorerSearchButton(p, inner, focused))

	// ── modified section ─────────────────────────────────────────────────────
	if modAreaH > 0 {
		lines = append(lines,
			s.SectionHeader.Render("modified")+" "+s.SectionCount.Render(fmt.Sprintf("%d", len(d.ModifiedFiles))),
		)
		modOverflow := len(d.ModifiedFiles) > modAreaH
		// Auto-scroll the modified window so a keyboard cursor in this
		// section stays visible — reuses clampScrollOff (which keeps a
		// "highlight" index inside the visible window) when section is
		// SectionMod, falls back to free-scroll when the user only used
		// the mouse wheel.
		modScroll := es.modScrollOff
		if es.section == SectionMod {
			modScroll = clampScrollOff(modScroll, es.modHighlight, len(d.ModifiedFiles), modAreaH)
		} else {
			modScroll = clampSimpleScroll(modScroll, len(d.ModifiedFiles), modAreaH)
		}
		modRowInner := inner
		if modOverflow {
			modRowInner = inner - 1 // reserve column for scrollbar
		}
		modEnd := modScroll + modAreaH
		if modEnd > len(d.ModifiedFiles) {
			modEnd = len(d.ModifiedFiles)
		}
		modCursorPainted := focused && es.section == SectionMod
		for i := modScroll; i < modEnd; i++ {
			isHL := modCursorPainted && i == es.modHighlight
			row := renderModifiedFileLineMaybeHL(s, p, d.ModifiedFiles[i], modRowInner, isHL, hlBg)
			if modOverflow {
				row = padRight(row, modRowInner) + renderScrollbarCell(p, i-modScroll, modAreaH, modScroll, len(d.ModifiedFiles))
			}
			lines = append(lines, row)
		}
		for blank := modEnd - modScroll; blank < modAreaH; blank++ {
			lines = append(lines, "")
		}
	}

	// ── separator + tree header ──────────────────────────────────────────────
	lines = append(lines, s.SectionHeader.Render(dotLine))
	lines = append(lines, s.SectionHeader.Render("tree"))

	// ── tree (clipped to its own window) ─────────────────────────────────────
	scrollOff := clampScrollOff(es.treeScrollOff, es.highlighted, len(rows), treeAreaH)
	overflow := len(rows) > treeAreaH
	rowInner := inner
	if overflow {
		rowInner = inner - 1
	}
	end := scrollOff + treeAreaH
	if end > len(rows) {
		end = len(rows)
	}
	for i := scrollOff; i < end; i++ {
		row := rows[i]
		isHL := focused && i == es.highlighted
		rowStr := renderExplorerRow(s, p, row, rowInner, isHL, hlBg, hlBgDir)
		if overflow {
			rowStr = padRight(rowStr, rowInner) + renderScrollbarCell(p, i-scrollOff, treeAreaH, scrollOff, len(rows))
		}
		lines = append(lines, rowStr)
	}
	for blank := end - scrollOff; blank < treeAreaH; blank++ {
		lines = append(lines, "")
	}

	// No footer hint bar — explorer commands live in the panel's `h`
	// help overlay alongside every other panel's bindings. The old
	// "↵ open · ⌫ collapse · / filter…" line was redundant and stole
	// vertical space the tree section badly needed on narrow terminals.

	body := strings.Join(lines, "\n")
	if activeMode {
		return wrapPanelActive(s, body, "explorer", width, height)
	}
	meta := fmt.Sprintf("%d", len(rows))
	return wrapPanel(s, body, "explorer", meta, width, height, focused)
}

// explorerSectionBudget splits the panel's interior height between the
// modified-files window and the tree window. Chrome rows:
//
//	chromeRows = 1 (search button row — always present)
//	           + 1 (mod header — only when modified > 0)
//	           + 1 (separator between mod and tree)
//	           + 1 (tree header)
//	           = 4 normally, 3 when no modified files.
//
// The footer hint bar is gone — commands live in the per-panel `h` help
// overlay instead. Modified gets at most 1/3 of the available rows; tree
// takes the rest.
func explorerSectionBudget(innerH, modifiedCount int) (modAreaH, treeAreaH int) {
	chromeRows := 3 // search button + separator + tree header — always present
	if modifiedCount > 0 {
		chromeRows++ // mod header row
	}
	available := innerH - chromeRows
	if available < 2 {
		available = 2
	}
	if modifiedCount == 0 {
		return 0, available
	}
	maxMod := available / 3
	if maxMod < 3 {
		maxMod = 3
	}
	if maxMod > available-1 {
		maxMod = available - 1
	}
	if maxMod < 1 {
		maxMod = 1
	}
	modAreaH = modifiedCount
	if modAreaH > maxMod {
		modAreaH = maxMod
	}
	treeAreaH = available - modAreaH
	if treeAreaH < 1 {
		treeAreaH = 1
	}
	return modAreaH, treeAreaH
}

// clampSimpleScroll clamps a scroll offset into [0, max(0, total - window)].
// Used by the modified-files region; the tree has its own clampScrollOff
// because it must also keep the highlighted row visible.
func clampSimpleScroll(off, total, window int) int {
	if total <= window {
		return 0
	}
	maxOff := total - window
	if off < 0 {
		return 0
	}
	if off > maxOff {
		return maxOff
	}
	return off
}

// clampScrollOff returns a scroll offset that keeps the highlighted row
// inside [scrollOff, scrollOff+treeAreaH) and within the valid range
// [0, max(0, total-treeAreaH)].
func clampScrollOff(scrollOff, highlighted, total, treeAreaH int) int {
	if total == 0 {
		return 0
	}
	if scrollOff < 0 {
		scrollOff = 0
	}
	if scrollOff > total-1 {
		scrollOff = total - 1
	}
	if highlighted < scrollOff {
		scrollOff = highlighted
	}
	if highlighted >= scrollOff+treeAreaH {
		scrollOff = highlighted - treeAreaH + 1
	}
	maxOff := total - treeAreaH
	if maxOff < 0 {
		maxOff = 0
	}
	if scrollOff > maxOff {
		scrollOff = maxOff
	}
	if scrollOff < 0 {
		scrollOff = 0
	}
	return scrollOff
}

// renderModifiedFileLineMaybeHL renders one row of the "modified files"
// section. When isHL is
// true the row is rendered as a single uniform-background block so the
// keyboard cursor reads as a clear "this row is selected" affordance —
// the per-component foregrounds (mark color, stats color) are dropped on
// the highlighted line because lipgloss can't reliably layer them under
// a wrapping background style.
func renderModifiedFileLineMaybeHL(s Styles, _ Palette, f ModifiedFile, inner int, isHL bool, hlBg lipgloss.Style) string {
	markGlyph := "M"
	if f.Mark != "M" {
		markGlyph = "+"
	}
	nameMaxW := inner - 10
	if nameMaxW < 4 {
		nameMaxW = 4
	}
	nameTrunc := truncateMid(f.Path, nameMaxW)

	if isHL {
		// Plain-text row, padded to inner so the bg paints the full width.
		plain := markGlyph + " " + nameTrunc
		// Stats: drop ANSI, keep the text content.
		statsPlain := f.Stats
		gapW := inner - len([]rune(plain)) - len([]rune(statsPlain))
		if gapW < 1 {
			gapW = 1
		}
		full := plain + strings.Repeat(" ", gapW) + statsPlain
		return hlBg.Render(padRight(full, inner))
	}

	var mark string
	if f.Mark == "M" {
		mark = s.FileModMark.Render(markGlyph)
	} else {
		mark = s.FileAddMark.Render(markGlyph)
	}
	name := s.FileName.Render(nameTrunc)
	stats := colorizeDiffStats(s, f.Stats)
	line := mark + " " + name
	lineW := ansiWidth(line)
	statsW := ansiWidth(stats)
	gapW := inner - lineW - statsW
	if gapW < 1 {
		gapW = 1
	}
	return line + strings.Repeat(" ", gapW) + stats
}

// colorizeDiffStats splits a "+14 −3" / "+47" / "−5" style string and renders
// each token in its semantic color. Tokens starting with "+" are green
// (DiffAdd); tokens starting with "−" (Unicode minus, U+2212) or "-" are red
// (DiffRem). Anything else falls back to the dim FileStats style.
func colorizeDiffStats(s Styles, stats string) string {
	if stats == "" {
		return ""
	}
	parts := strings.Fields(stats)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		switch {
		case strings.HasPrefix(p, "+"):
			out = append(out, s.DiffAdd.Render(p))
		case strings.HasPrefix(p, "−"), strings.HasPrefix(p, "-"):
			out = append(out, s.DiffRem.Render(p))
		default:
			out = append(out, s.FileStats.Render(p))
		}
	}
	return strings.Join(out, " ")
}

// renderScrollbarCell returns the single-cell scrollbar character for the tree
// row at visible index `vIdx` (0 = top of viewport). The thumb covers a
// proportion of the track equal to viewport/total, with at least 1 cell.
func renderScrollbarCell(p Palette, vIdx, treeAreaH, scrollOff, total int) string {
	if treeAreaH <= 0 || total <= 0 {
		return " "
	}
	thumbSize := treeAreaH * treeAreaH / total
	if thumbSize < 1 {
		thumbSize = 1
	}
	if thumbSize > treeAreaH {
		thumbSize = treeAreaH
	}
	travel := treeAreaH - thumbSize
	maxOff := total - treeAreaH
	if maxOff < 1 {
		maxOff = 1
	}
	thumbStart := scrollOff * travel / maxOff
	if vIdx >= thumbStart && vIdx < thumbStart+thumbSize {
		return lipgloss.NewStyle().Foreground(p.Purple).Render("█")
	}
	return lipgloss.NewStyle().Foreground(p.TextFade).Render("│")
}

// renderExplorerRow renders a single tree row with optional highlight.
//
// File rows include a colored 1-cell icon prefix derived from the extension
// (◆ for code, ◇ for config, ▤ for docs, ▦ for images). Directories keep their
// ▼/▶ chevron — a separate icon would be redundant there.
func renderExplorerRow(s Styles, _ Palette, row ExplorerRow, inner int, isHL bool, hlBg, hlBgDir lipgloss.Style) string {
	indent := s.TreeIndent.Render(row.Indent)
	indentW := ansiWidth(row.Indent)

	switch {
	case row.IsDir:
		// Dirs: ▼/▶ acts as their own icon. Budget = inner - indent - 1 (chevron+space).
		nameW := inner - indentW - 1
		if nameW < 3 {
			nameW = 3
		}
		dispName := truncateMid(row.DisplayName, nameW)
		if isHL {
			return indent + hlBgDir.Render(padRight(dispName, inner-indentW))
		}
		return indent + s.TreeDir.Render(dispName)

	case row.IsAct:
		// Active file: ▸ + space + icon + space + name.
		ic := iconForFile(row.DisplayName)
		iconStr := lipgloss.NewStyle().Foreground(ic.Color).Render(ic.Glyph)
		// 4 cells fixed: ▸ (1) + space (1) + icon (1) + space (1)
		nameW := inner - indentW - 4
		if nameW < 3 {
			nameW = 3
		}
		fname := truncateMid(row.DisplayName, nameW)
		if isHL {
			plain := "▸ " + ic.Glyph + " " + fname
			return indent + hlBg.Render(padRight(plain, inner-indentW))
		}
		return indent + s.FileNameAct.Render("▸") + " " + iconStr + " " + s.FileNameAct.Render(fname)

	default:
		// Default file: mark + space + icon + space + name.
		ic := iconForFile(row.DisplayName)
		iconStr := lipgloss.NewStyle().Foreground(ic.Color).Render(ic.Glyph)
		// 4 cells fixed: mark (1) + space (1) + icon (1) + space (1)
		nameW := inner - indentW - 4
		if nameW < 3 {
			nameW = 3
		}
		fname := truncateMid(row.DisplayName, nameW)
		if isHL {
			markRaw := strings.TrimSpace(row.Mark)
			if markRaw == "" {
				markRaw = " "
			}
			plain := markRaw + " " + ic.Glyph + " " + fname
			return indent + hlBg.Render(padRight(plain, inner-indentW))
		}
		mark := renderFileMark(s, row.Mark)
		return indent + mark + " " + iconStr + " " + s.FileName.Render(fname)
	}
}

// renderFileMark returns the styled mark character for a file row.
func renderFileMark(s Styles, mark string) string {
	switch mark {
	case "M":
		return s.FileModMark.Render("M")
	case "+":
		return s.FileAddMark.Render("+")
	default:
		return " "
	}
}

// padRight pads s to exactly w visible chars with spaces.
func padRight(s string, w int) string {
	vis := ansiWidth(s)
	if vis >= w {
		return s
	}
	return s + strings.Repeat(" ", w-vis)
}

// renderExplorerCollapsed renders the collapsed one-liner for explorer.
func renderExplorerCollapsed(s Styles, d MockData, width int, focused bool) string {
	modCount := len(d.ModifiedFiles)
	summary := fmt.Sprintf("%d modified", modCount)
	return wrapPanelCollapsed(s, "explorer", summary, "", width, focused)
}

// ─── shared utilities ─────────────────────────────────────────────────────────

// rowSpread spreads left and right content across w chars.
func rowSpread(left, right string, w int) string {
	lw := ansiWidth(left)
	rw := ansiWidth(right)
	gap := w - lw - rw
	if gap < 0 {
		gap = 0
	}
	return left + strings.Repeat(" ", gap) + right
}

// truncate clips a string to max visible chars, adding ellipsis if needed.
func truncate(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if ansiWidth(s) <= maxWidth {
		return s
	}
	runes := []rune(s)
	for len(runes) > 0 && ansiWidth(string(runes)) > maxWidth-1 {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…"
}

// truncateMid clips a string using middle-truncation, preserving the suffix.
// e.g. "Sidebar.tsx" → "Side…tsx" so extensions remain visible.
func truncateMid(s string, maxWidth int) string {
	if maxWidth <= 3 {
		return truncate(s, maxWidth)
	}
	if ansiWidth(s) <= maxWidth {
		return s
	}
	runes := []rune(s)
	// Keep at most 4 suffix chars (extension like .tsx, .go, .ts)
	suffixLen := 4
	if len(runes) < suffixLen+4 {
		suffixLen = 0
	}
	suffix := string(runes[len(runes)-suffixLen:])
	prefixMax := maxWidth - 1 - ansiWidth(suffix) // 1 for ellipsis
	if prefixMax < 1 {
		prefixMax = 1
	}
	prefix := []rune(s)
	for ansiWidth(string(prefix)) > prefixMax {
		prefix = prefix[:len(prefix)-1]
	}
	return string(prefix) + "…" + suffix
}

// clamp returns v clamped to [lo, hi].
func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
