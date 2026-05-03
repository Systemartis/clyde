package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// ansiWidth returns the visible (ANSI-stripped) column width of s.
// Delegates to lipgloss.Width which handles multi-byte and ANSI escape sequences.
func ansiWidth(s string) int {
	return lipgloss.Width(s)
}

// Breakpoint describes the width-driven layout mode.
type Breakpoint int

const (
	// BreakpointNarrow is single-column stack only (< 80 cols).
	BreakpointNarrow Breakpoint = iota
	// BreakpointMedium is two-column layout (80-159 cols):
	// left = explorer + servers, right = now/tasks/diff/usage stacked.
	BreakpointMedium
	// BreakpointWide is the full 3-column design (160+ cols).
	BreakpointWide
)

// DetectBreakpoint maps a terminal width to a Breakpoint.
func DetectBreakpoint(width int) Breakpoint {
	switch {
	case width >= 160:
		return BreakpointWide
	case width >= 80:
		return BreakpointMedium
	default:
		return BreakpointNarrow
	}
}

// Column is a rendered panel block ready for horizontal joining.
type Column struct {
	Content string
	Width   int
}

// ComposeColumns joins columns horizontally, padding each to its declared width
// and padding to `height` rows.
func ComposeColumns(height int, columns ...Column) string {
	if len(columns) == 0 {
		return ""
	}
	strs := make([]string, len(columns))
	for i, col := range columns {
		strs[i] = padToHeight(col.Content, col.Width, height)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, strs...)
}

// padToHeight ensures a block has exactly h lines and each line is w chars wide.
func padToHeight(s string, w, h int) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = padLine(l, w)
	}
	for len(lines) < h {
		lines = append(lines, strings.Repeat(" ", w))
	}
	if len(lines) > h {
		lines = lines[:h]
	}
	return strings.Join(lines, "\n")
}

// padLine pads a single ANSI line to w visible chars.
func padLine(line string, w int) string {
	vis := lipgloss.Width(line)
	if vis < w {
		return line + strings.Repeat(" ", w-vis)
	}
	return line
}

// ─── Single source of truth for screen geometry ───────────────────────────────
//
// Layout is the canonical geometry for the current frame. ALL renderers,
// mouse bounds builders, and panel-area helpers MUST consume Layout
// instead of recomputing widths/heights from local constants.
//
// Why this exists:
// The TUI used to compute layout dimensions in three independent places —
// the renderer (render_stack_2col.go etc.), the mouse bounds builder
// (mouse.go buildPanelBounds), and the explorer area helpers
// (explorerOuterHeight). Each carried its own copy of titleH / notifH /
// statusH / serversH / explorerH constants that drifted apart over
// time, producing three observable symptoms:
//
//  1. Cursor offset — clicks landed slightly lower than the visual
//     cursor because the bounds builder added a "+1 separator row"
//     between panels that lipgloss.JoinVertical / strings.Join do NOT
//     add. Each successive panel boundary was shifted by one row, so
//     clicks at panel transitions got routed to the panel below.
//
//  2. Tree-row off-by-one — explorer's hit test computed modAreaH from
//     explorerOuterHeight (which used a hardcoded notifH=3 and
//     serversH=9), while the renderer used the real notificationHeight()
//     and serversH=13. The two modAreaH values diverged at certain
//     terminal heights, so tree clicks selected one row higher than
//     the cursor.
//
//  3. Bash/cache panels unclickable — the renderer painted them in the
//     right column for 2-col and multi-col layouts, but the bounds
//     builder's right-column slice listed only Now/Calls/Diff/Usage.
//     Clicks on bash or cache fell through to no-panel-found.
//
// All three bugs are the same class: independent reinventions of the
// same layout math. The fix is structural — every consumer reads
// Layout, the drift-catcher test in layout_test.go fails the build if
// the renderer and bounds builder disagree on a panel's y-range.
//
// Adding a new panel:
//  1. Add it to twoColRightPanelOrder / multiColRightPanelOrder below.
//  2. Add it to activePanelsForBreakpoint in render_stack.go.
//  3. The renderers and bounds builder pick it up via these slices.
//  4. The drift-catcher test verifies both lists agree.

// twoColRightPanelOrder is the canonical right-column panel order for
// the medium 2-col layout. The renderer (render_stack_2col.go) and
// the bounds builder (mouse.go) both iterate this slice — keeping the
// order in one place is what stops them drifting apart.
func twoColRightPanelOrder() []PanelID {
	return []PanelID{PanelNow, PanelCalls, PanelDiff, PanelUsage, PanelBash, PanelCache}
}

// multiColRightPanelOrder is the canonical right-column stack order
// for multi-col mode. Usage and servers always lead; bash and cache
// stack after them when enabled. Heights for usage/servers come from
// Layout; bash and cache use the fixed heights MultiBashH / MultiCacheH.
func multiColRightPanelOrder() []PanelID {
	return []PanelID{PanelUsage, PanelServers, PanelBash, PanelCache}
}

// Layout holds the geometry derived from the current model snapshot.
// Never store this — always recompute via m.computeLayout() at the top
// of the consumer function. Stale Layouts (across model updates) are
// silently wrong; recomputing is cheap.
type Layout struct {
	// Mode + breakpoint + width snapshot.
	Mode  LayoutMode
	BP    Breakpoint
	Width int

	// Chrome rows. NotifH only takes height when an actual hook or
	// compaction warning is active; StatusH grows when help is open.
	TitleH  int
	NotifH  int
	StatusH int

	// Vertical budget for panels = Height - TitleH - NotifH - StatusH,
	// floored at a per-mode minimum so degenerate window sizes don't
	// produce negative dimensions downstream.
	GridH int

	// ── 2-col (BreakpointMedium with stack mode) ────────────────────
	// LeftW + RightW = Width. ExplorerH and ServersH split the left
	// column vertically; the right column uses panelStackHeight per
	// panel (collapse spring or panelHeight()).
	LeftW     int
	RightW    int
	ExplorerH int
	ServersH  int

	// ── Multi-col (3 columns) ───────────────────────────────────────
	// MultiExplorerW takes the full GridH on the left. The middle
	// column stacks Now → Calls → optional Diff. The right column
	// stacks Usage → Servers → optional Bash → optional Cache.
	MultiExplorerW int
	MultiMiddleW   int
	MultiRightW    int
	MultiNowH      int
	MultiCallsH    int
	MultiDiffH     int
	MultiUsageH    int
	MultiServersH  int
	MultiBashH     int
	MultiCacheH    int
}

// computeLayout returns the geometry of the current frame. ALL
// renderers and click handlers MUST go through this — see the section
// header for why.
func (m Model) computeLayout() Layout {
	mode := m.effectiveMode()
	titleH := 2
	notifH := notificationHeight(m.cfg.NotificationStyle, m.notifAck, m.hookNotif, m.compaction, m.quotaNotif)
	statusH := statusBarHeight(m.helpOpen)

	l := Layout{
		Mode:    mode,
		BP:      m.bp,
		Width:   m.width,
		TitleH:  titleH,
		NotifH:  notifH,
		StatusH: statusH,
	}
	l.GridH = m.height - titleH - notifH - statusH

	switch {
	case mode == LayoutStack && m.bp == BreakpointMedium:
		if l.GridH < 8 {
			l.GridH = 8
		}
		l.LeftW, l.RightW = computeTwoColSplit(m.width)
		l.ExplorerH, l.ServersH = m.computeTwoColLeftHeights(l.GridH)

	case mode == LayoutMultiCol:
		if l.GridH < 10 {
			l.GridH = 10
		}
		l.MultiExplorerW, l.MultiMiddleW, l.MultiRightW = computeMultiColWidths(m.width)
		l.MultiNowH, l.MultiCallsH, l.MultiDiffH = m.computeMultiColMiddleHeights(l.GridH)
		l.MultiUsageH, l.MultiServersH = m.computeMultiColRightHeights(l.GridH)
		l.MultiBashH = 10
		l.MultiCacheH = 8

	default:
		// Wide single-stack and narrow stack: panel heights come from
		// m.panelStackHeight(pid) on demand — no precompute needed.
		if l.GridH < 8 {
			l.GridH = 8
		}
	}

	return l
}

// computeTwoColSplit returns (leftW, rightW) for the medium 2-col
// layout. 40% width clamped to [22, 50].
func computeTwoColSplit(width int) (leftW, rightW int) {
	leftW = (width * 40) / 100
	if leftW < 22 {
		leftW = 22
	}
	if leftW > 50 {
		leftW = 50
	}
	return leftW, width - leftW
}

// computeTwoColLeftHeights returns (explorerH, serversH) for the left
// column of 2-col mode. Servers defaults to 13, honors a user override
// via panelHeights[PanelServers] (when > 4), falls to 0 when disabled.
// Explorer claims whatever's left, with a minimum of 6.
func (m Model) computeTwoColLeftHeights(gridH int) (explorerH, serversH int) {
	serversEnabled := m.panelEnabled(PanelServers)
	explorerEnabled := m.panelEnabled(PanelExplorer)

	serversH = 13
	if h, ok := m.panelHeights[PanelServers]; ok && h > 4 {
		serversH = h
	}
	if !serversEnabled {
		serversH = 0
	}
	if serversH > gridH-6 {
		serversH = gridH - 6
	}
	if serversH < 0 {
		serversH = 0
	}

	explorerH = gridH - serversH
	if !explorerEnabled {
		explorerH = 0
	} else if explorerH < 6 {
		explorerH = 6
	}
	return explorerH, serversH
}

// computeMultiColWidths returns (explorerW, middleW, rightW) for the
// 3-col layout. Explorer fixed at 22, middle fixed at 36, right is
// flex clamped to [18, 42] with overflow going to the middle column.
func computeMultiColWidths(width int) (explorerW, middleW, rightW int) {
	explorerW = 22
	middleW = 36
	rightW = width - explorerW - middleW
	if rightW < 18 {
		rightW = 18
	}
	if rightW > 42 {
		extra := rightW - 42
		rightW = 42
		middleW += extra
	}
	return explorerW, middleW, rightW
}

// computeMultiColMiddleHeights returns (nowH, callsH, diffH) for the
// middle column. NowH is fixed at 6 because renderNow ignores the
// height arg and always paints a 6-row panel — using anything bigger
// here wastes vertical budget that the bounds builder would then
// disagree with the renderer about. When the standalone diff panel
// is hidden (v22+ default), the activity panel claims diff's share.
func (m Model) computeMultiColMiddleHeights(gridH int) (nowH, callsH, diffH int) {
	nowH = 6 // matches renderNow's hardcoded panelH; do NOT raise without changing renderNow
	if m.panelEnabled(PanelDiff) {
		callsH = gridH/2 + 2
		diffH = gridH - nowH - callsH
		if diffH < 5 {
			diffH = 5
			callsH = gridH - nowH - diffH
		}
	} else {
		callsH = gridH - nowH
	}
	return nowH, callsH, diffH
}

// computeMultiColRightHeights returns (usageH, serversH) for the right
// column. Servers floors at 13 to keep the LSP/MCP grid readable;
// usage stays fixed at 14.
func (m Model) computeMultiColRightHeights(gridH int) (usageH, serversH int) {
	usageH = 14
	serversH = gridH - usageH
	if serversH < 13 {
		serversH = 13
	}
	return usageH, serversH
}

// panelStackHeight returns how many on-screen rows a panel occupies in
// stack-mode layouts (single-stack wide, narrow, and the right column
// of 2-col). Mirrors renderStackPanel exactly:
//
//   - collapsed (or spring height ≤ 3): 2 rows (border top + bottom)
//   - PanelNow: always 6 rows (renderNow hardcodes panelH=6)
//   - all other expanded panels: clamp(panelHeight(pid), 4, 40)
//
// This is the function the bounds builder uses for the right column
// in 2-col mode and the entire stack in wide mode.
func (m Model) panelStackHeight(pid PanelID) int {
	cs := m.collapse[pid]
	if cs.IsCollapsed() || cs.Height() <= 3 {
		return 2
	}
	if pid == PanelNow {
		return 6
	}
	return clamp(m.panelHeight(pid), 4, 40)
}
