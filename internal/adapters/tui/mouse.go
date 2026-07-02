package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// panelDoubleClickWindow is the maximum gap between two left-clicks on
// the same panel that still counts as a double-click. Past this window
// the second click is treated as a fresh single-click.
//
// 800ms is generous on purpose: with the focus-first click model the
// second click of any double-click is preceded by an automatic focus
// hop, which steals the user's attention briefly. macOS' system pref
// goes up to ~900ms for users who want a slow double-click; we match
// the upper end of that range. The trade-off — one more deliberate
// single click waits 800ms before "committing" to single-click intent
// — is invisible because single clicks act immediately (toggle
// collapse on the focused panel); the 800ms only gates the *next*
// click from chaining as a double-click.
const panelDoubleClickWindow = 800 * time.Millisecond

// isPanelDoubleClick reports whether a click on `pid` is the second
// half of a double-click on the same panel within the window. Used by
// handleMouseClick to promote focus to Expanded-Active mode without
// requiring a keyboard press.
func (m Model) isPanelDoubleClick(pid PanelID) bool {
	if m.lastPanelClickAt.IsZero() {
		return false
	}
	if pid != m.lastPanelClickPanel {
		return false
	}
	return time.Since(m.lastPanelClickAt) <= panelDoubleClickWindow
}

// recordPanelClick stamps the latest clicked panel + timestamp so the
// next click on the same panel can fire the double-click handler.
func (m Model) recordPanelClick(pid PanelID) Model {
	m.lastPanelClickPanel = pid
	m.lastPanelClickAt = time.Now()
	return m
}

// resetPanelClick clears the panel-click stamp — used after a double
// click fires so a third click doesn't immediately count as another.
func (m Model) resetPanelClick() Model {
	m.lastPanelClickPanel = PanelNone
	m.lastPanelClickAt = time.Time{}
	return m
}

// Mouse hit-testing strategy: column-aware bounds.
// In 2-col (medium) mode, explorer+servers are in the LEFT column and
// now/tasks/diff/usage are in the RIGHT column. Using only the y-coord would
// pick the wrong panel when panels in different columns share the same y range.
//
// panelAtPos(x, y) first determines the column from x, then walks the vertical
// panel stack for that column matching y — fixing the v7 bug where clicks in the
// explorer's y range were routed to right-column panels.

// panelBounds holds the screen bounding box for a panel in absolute coords.
type panelBounds struct {
	pid        PanelID
	xMin, xMax int // inclusive column range
	yMin, yMax int // inclusive row range
}

// buildPanelBounds computes the screen bounding boxes for all visible
// panels given the current model state. EVERY layout dimension comes
// from m.computeLayout() — the single source of truth defined in
// layout.go. Adding new constants here is a regression: the layout test
// in layout_test.go fails the build if the bounds builder and renderer
// disagree on any panel's y-range.
//
// Critical invariants:
//   - Panels stacked vertically have NO separator row between them
//     (renderer uses lipgloss.JoinVertical / strings.Join("\n") which
//     produce sum-of-heights rows total). Earlier versions added
//     "cur += h + 1" — that phantom row caused clicks at panel
//     transitions to drift down.
//   - The right-column panel order in 2-col / multi-col mode comes
//     from twoColRightPanelOrder() / multiColRightPanelOrder() so
//     adding a new panel updates renderer AND bounds builder
//     simultaneously.
func (m Model) buildPanelBounds() []panelBounds {
	l := m.computeLayout()

	switch {
	case l.Mode == LayoutTabs:
		return m.buildTabsBounds(l)
	case l.Mode == LayoutMultiCol:
		return m.buildMultiColBounds(l)
	case l.Mode == LayoutStack && l.BP == BreakpointMedium:
		return m.build2ColBounds(l)
	default:
		// Stack-narrow/wide: single column.
		return m.buildStackBounds(l)
	}
}

// buildTabsBounds computes bounds for Mode B (tabs). renderTabs draws
// title(2) + tab strip(1) + ONE full-height panel, so there is a single
// clickable panel occupying the region below the strip. Geometry mirrors
// renderTabs exactly (statusH is a fixed 2 rows — the tab status bar has no
// help-expanded form). Clicks on the tab strip row itself are handled
// separately in handleMouseClick via tabStripAtPos.
func (m Model) buildTabsBounds(l Layout) []panelBounds {
	const titleH = 2
	const tabStripH = 1
	const statusH = 2
	notifH := notificationHeight(m.cfg.NotificationStyle, m.notifAck, m.hookNotif, m.compaction, m.quotaNotif)
	panelH := m.height - (titleH + tabStripH + notifH + statusH)
	if panelH < 6 {
		panelH = 6
	}
	yMin := titleH + tabStripH
	return []panelBounds{{
		pid:  m.tabFocused(),
		xMin: 0, xMax: l.Width - 1,
		yMin: yMin, yMax: yMin + panelH - 1,
	}}
}

// tabStripAtPos returns the tab index under (x, y) in the Mode B tab strip,
// or (-1, false) when the click missed the strip or the layout isn't tabs.
// The X math mirrors renderTabStrip: a fixed 5-col left cap ("──── "),
// each tab body (active = "label*", inactive = "label summary"), joined by
// a 4-col separator (" ── ").
func (m Model) tabStripAtPos(x, y int) (int, bool) {
	if m.effectiveMode() != LayoutTabs {
		return -1, false
	}
	const titleH = 2 // tab strip is the single row right after the title bar
	if y != titleH {
		return -1, false
	}
	tabs := m.activeTabs(m.data)
	activeIdx := activeTabIndex(tabs, m.focused)
	cur := ansiWidth("──── ") // left cap
	sepW := ansiWidth(" ── ") // separator between tabs
	for i, tab := range tabs {
		var body string
		if i == activeIdx {
			body = tab.label + "*"
		} else {
			body = tab.label + " " + tab.summary
		}
		w := ansiWidth(body)
		xMin, xMax := cur, cur+w-1
		if x >= xMin && x <= xMax {
			return i, true
		}
		cur = xMax + 1 + sepW
	}
	return -1, false
}

// buildStackBounds computes bounds for a single-column stack layout.
// Panels stack with NO separator row — see invariants in buildPanelBounds.
func (m Model) buildStackBounds(l Layout) []panelBounds {
	panels := m.activePanelsForBreakpoint()
	var bounds []panelBounds
	cur := l.TitleH
	for _, pid := range panels {
		h := m.panelStackHeight(pid)
		bounds = append(bounds, panelBounds{
			pid:  pid,
			xMin: 0, xMax: l.Width - 1,
			yMin: cur, yMax: cur + h - 1,
		})
		cur += h
	}
	return bounds
}

// build2ColBounds computes bounds for the medium 2-col layout.
// Left column: explorer + servers (heights from Layout, fixed at frame
// time). Right column: panels from twoColRightPanelOrder, each at its
// stack-mode height.
func (m Model) build2ColBounds(l Layout) []panelBounds {
	var bounds []panelBounds

	// Left column (x: 0..LeftW-1) — explorer + servers stacked,
	// matching render2ColLeft. Disabled panels (height 0) are skipped.
	cur := l.TitleH
	if m.panelEnabled(PanelExplorer) && l.ExplorerH > 0 {
		bounds = append(bounds, panelBounds{
			pid:  PanelExplorer,
			xMin: 0, xMax: l.LeftW - 1,
			yMin: cur, yMax: cur + l.ExplorerH - 1,
		})
		cur += l.ExplorerH
	}
	if m.panelEnabled(PanelServers) && l.ServersH > 0 {
		bounds = append(bounds, panelBounds{
			pid:  PanelServers,
			xMin: 0, xMax: l.LeftW - 1,
			yMin: cur, yMax: cur + l.ServersH - 1,
		})
	}

	// Right column (x: LeftW..Width-1) — full panel order from the
	// canonical slice; bash + cache included so clicks route correctly.
	rightPanels := m.filterPanels(twoColRightPanelOrder())
	cur = l.TitleH
	for _, pid := range rightPanels {
		h := m.panelStackHeight(pid)
		bounds = append(bounds, panelBounds{
			pid:  pid,
			xMin: l.LeftW, xMax: l.Width - 1,
			yMin: cur, yMax: cur + h - 1,
		})
		cur += h
	}

	return bounds
}

// buildMultiColBounds computes bounds for the 3-column multi-col layout.
// All widths and heights come from Layout. The right column iterates
// multiColRightPanelOrder so bash and cache are wired in.
func (m Model) buildMultiColBounds(l Layout) []panelBounds {
	col0x := 0
	col1x := l.MultiExplorerW
	col2x := l.MultiExplorerW + l.MultiMiddleW

	var bounds []panelBounds

	// Left column: explorer takes the full grid height.
	bounds = append(bounds, panelBounds{
		pid:  PanelExplorer,
		xMin: col0x, xMax: col0x + l.MultiExplorerW - 1,
		yMin: l.TitleH, yMax: l.TitleH + l.GridH - 1,
	})

	// Middle column: now / calls / optional diff stacked.
	bounds = append(bounds, panelBounds{
		pid:  PanelNow,
		xMin: col1x, xMax: col1x + l.MultiMiddleW - 1,
		yMin: l.TitleH, yMax: l.TitleH + l.MultiNowH - 1,
	})
	bounds = append(bounds, panelBounds{
		pid:  PanelCalls,
		xMin: col1x, xMax: col1x + l.MultiMiddleW - 1,
		yMin: l.TitleH + l.MultiNowH, yMax: l.TitleH + l.MultiNowH + l.MultiCallsH - 1,
	})
	if m.panelEnabled(PanelDiff) {
		bounds = append(bounds, panelBounds{
			pid:  PanelDiff,
			xMin: col1x, xMax: col1x + l.MultiMiddleW - 1,
			yMin: l.TitleH + l.MultiNowH + l.MultiCallsH,
			yMax: l.TitleH + l.MultiNowH + l.MultiCallsH + l.MultiDiffH - 1,
		})
	}

	// Right column: usage / servers / optional bash / optional cache stacked,
	// using fixed heights from Layout (matching render_multicol.go).
	cur := l.TitleH
	for _, pid := range multiColRightPanelOrder() {
		if !m.panelEnabled(pid) {
			continue
		}
		var h int
		switch pid {
		case PanelUsage:
			h = l.MultiUsageH
		case PanelServers:
			h = l.MultiServersH
		case PanelBash:
			h = l.MultiBashH
		case PanelCache:
			h = l.MultiCacheH
		}
		bounds = append(bounds, panelBounds{
			pid:  pid,
			xMin: col2x, xMax: col2x + l.MultiRightW - 1,
			yMin: cur, yMax: cur + h - 1,
		})
		cur += h
	}

	return bounds
}

// panelAtPos returns the PanelID of the panel under screen position (x, y),
// using a column-aware bounds map. Returns -1 if no panel is hit.
//
// This replaces the v7 panelAtRow(y) which was column-unaware and caused
// explorer clicks to be routed to right-column panels in 2-col mode.
func (m Model) panelAtPos(x, y int) (PanelID, bool) {
	bounds := m.buildPanelBounds()
	for _, b := range bounds {
		if x >= b.xMin && x <= b.xMax && y >= b.yMin && y <= b.yMax {
			return b.pid, true
		}
	}
	return -1, false
}

// panelHeaderAtPos reports whether (x, y) is on the top-border row of any
// panel — i.e. the row that carries the panel's title chip. The header is
// the dedicated "collapse handle" of the panel: a click there toggles
// collapsed/expanded; clicks anywhere else in the body never affect
// collapse state. Returns the panel id and true on hit.
func (m Model) panelHeaderAtPos(x, y int) (PanelID, bool) {
	bounds := m.buildPanelBounds()
	for _, b := range bounds {
		if x >= b.xMin && x <= b.xMax && y == b.yMin {
			return b.pid, true
		}
	}
	return -1, false
}

// sessionTabAtPos returns the tab index under (x, y) in the footer
// session-tab strip, or (-1, false) when the click missed every tab or
// the strip is hidden (toast active, help open, fewer than 2 sessions).
//
// The X math mirrors renderStatusBar's non-help layout exactly:
//
//	" "  + "h commands" + " · " + tab1 + " " + tab2 + " " + ...
//
// Each tab body matches renderSessionTab: "[" + optional "● " bullet +
// label + optional " ⚠" + "]". ansiWidth is used so wide glyphs (●, ⚠)
// count as 1 col like the renderer treats them.
func (m Model) sessionTabAtPos(x, y int) (int, bool) {
	if m.copyToast != "" || m.helpOpen {
		return -1, false
	}
	tabs := m.data.Sessions
	if len(tabs) < 2 {
		return -1, false
	}
	// The tab strip is rendered on the bottom-most row of the status
	// bar (one row below the dashes separator).
	if y != m.height-1 {
		return -1, false
	}
	const hHintPlain = "h commands"
	const sepPlain = " · "
	cur := 1 + ansiWidth(hHintPlain) + ansiWidth(sepPlain) // leading " " + hint + sep
	for i, t := range tabs {
		w := ansiWidth(sessionTabPlainBody(t))
		xMin, xMax := cur, cur+w-1
		if x >= xMin && x <= xMax {
			if i == 0 {
				return -1, true // Σ aggregate
			}
			return i - 1, true
		}
		cur = xMax + 2 // +1 closing col, +1 join space
	}
	return -1, false
}

// sessionTabPlainBody returns the unstyled rendered text of a session
// tab — kept in lockstep with renderSessionTab so the hit-tester and
// the renderer agree on every column.
func sessionTabPlainBody(t SessionTab) string {
	body := "["
	if t.Live && !t.IsAggregate {
		body += "● "
	}
	body += t.Label
	if t.Warning && !t.IsAggregate {
		body += " ⚠"
	}
	body += "]"
	return body
}

// panelIsEffectivelyExpanded returns true when a panel is rendered expanded in the
// current layout, regardless of its logical collapse state.
// In 2-col and multi-col layouts, panels in fixed-size columns are always rendered
// expanded (their collapse spring is irrelevant to the visual layout).
func (m Model) panelIsEffectivelyExpanded(pid PanelID) bool {
	mode := m.effectiveMode()
	// In 2-col medium layout, left-column panels (explorer, servers) are always expanded.
	if mode == LayoutStack && m.bp == BreakpointMedium {
		for _, p := range m.twoColLeftPanels() {
			if p == pid {
				return true
			}
		}
	}
	// In multi-col layout, all panels are always expanded.
	if mode == LayoutMultiCol {
		return true
	}
	// Otherwise, respect the collapse spring.
	return !m.collapse[pid].IsCollapsed()
}

// handleMouseClick handles a left-click mouse event.
// Uses column-aware panelAtPos to correctly route clicks in 2-col and 3-col layouts.
//
// Returns (Model, tea.Cmd) so explorer double-click → copy can dispatch
// the OSC 52 SetClipboard command back through Update.
func (m Model) handleMouseClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	if msg.Button != tea.MouseLeft {
		return m, nil
	}

	// While an overlay (settings or help) is up the panels behind it
	// must NOT respond to clicks — without this guard a click that
	// happens to land on the explorer's coordinates while help is open
	// would still open a file in the viewer. The overlay has captured
	// the user's attention; routing clicks through to obscured panels
	// is a bug, not a feature.
	if m.settingsOpen || m.helpOpen {
		return m, nil
	}

	// Viewer close badge click — check before panel routing so the top-border
	// click on the viewer panel closes the viewer even if it hits the border area.
	if m.viewerActive && m.viewerCloseButtonAtPos(msg.X, msg.Y) {
		// Same dirty-confirm gate as the keyboard Esc path.
		if m.viewerDirty && m.viewerStatus != viewerDiscardPrompt {
			m.viewerStatus = viewerDiscardPrompt
			return m, nil
		}
		return m.closeViewer(), nil
	}

	// Click on the viewer's top border (title chrome) toggles fullscreen.
	// Same gesture every windowed editor offers — clicking the chrome
	// is the universal "expand / restore" affordance. Doesn't conflict
	// with the close-button check above (that returned first if the
	// click landed on the esc-close badge).
	if m.viewerActive && m.viewerHeaderAtPos(msg.X, msg.Y) {
		m.viewerFullscreen = !m.viewerFullscreen
		return m, nil
	}

	// Fullscreen viewer obscures every dashboard panel. Clicks anywhere
	// inside its body must NOT be routed to the panel beneath (the
	// explorer / servers / etc. were rendered by buildPanelBounds for
	// the dashboard layout, but the renderer is showing a fullscreen
	// viewer instead). Same class as the settings / help overlay guard
	// above. Close-button + scrollbar + header clicks above this branch
	// already got their chance to fire.
	if m.viewerActive && m.viewerFullscreen {
		return m, nil
	}

	// Viewer scrollbar click → jump to that position.
	if m.viewerActive {
		if m2, ok := m.handleViewerScrollbarClick(msg.X, msg.Y); ok {
			return m2, nil
		}
	}

	// Active-mode "esc back" badge click — clicking the badge text on
	// the active panel's top border mirrors the esc keybind's priority
	// chain. For the explorer this means: close the search overlay
	// first (back to active explorer), THEN drop out of active mode on
	// the next click — so a single click in search mode doesn't lose
	// the user's place. Other panels just drop out of active.
	if m.isActiveMode() && m.activeBadgeAtPos(msg.X, msg.Y) {
		if m.activePanelID == PanelExplorer && m.explorer.search.Active {
			m.explorer.search = endSearch(m.explorer.search)
			m = m.resetPanelClick()
			return m, nil
		}
		m = m.transitionToPassive()
		m = m.resetPanelClick()
		return m, nil
	}

	// Footer session tab click — switch the active tab. Checked before
	// panel hit-testing so a click that lands on the bottom-most row
	// is interpreted as a tab switch even if a panel's bounding box
	// happens to extend that far down.
	if idx, ok := m.sessionTabAtPos(msg.X, msg.Y); ok {
		m = m.selectSession(idx)
		m = m.resetPanelClick()
		return m, m.snapshotCmd()
	}

	// Tabs mode: a click on the tab strip row switches to that tab. Checked
	// before panel hit-testing because the strip sits on its own row above
	// the single full-height panel.
	if idx, ok := m.tabStripAtPos(msg.X, msg.Y); ok {
		tabs := m.activeTabs(m.data)
		if idx >= 0 && idx < len(tabs) {
			m = m.setFocus(tabs[idx].id)
		}
		m = m.resetPanelClick()
		return m, nil
	}

	pid, ok := m.panelAtPos(msg.X, msg.Y)
	if !ok {
		return m, nil // click outside any panel
	}

	// PanelNow is non-selectable — clicking it nudges the mascot into a
	// playful "happy" state instead of changing focus. Gives the user
	// some feedback that the click registered without breaking the
	// "focus only ever lands on interactive panels" invariant.
	if pid == PanelNow {
		m.frame.Mascot = m.frame.Mascot.SetExternalState(eventHappy)
		return m, nil
	}

	// Click on the top-border row → that's the panel's "collapse handle".
	// Toggle collapse and focus, regardless of which panel had focus
	// before. Only meaningful in stack mode — in 2-col / multi-col,
	// the layout pins the panel size so collapse is a no-op visually,
	// and we skip the toggle to avoid confusing the persisted state.
	if hpid, ok := m.panelHeaderAtPos(msg.X, msg.Y); ok && hpid == pid {
		m = m.setFocus(pid)
		if m.effectiveMode() == LayoutStack {
			// Collapsing the ACTIVE panel via its header must also exit
			// active mode, matching the keyboard Space path
			// (handleActiveModeKey). setFocus is a no-op for the
			// already-focused panel, so it won't clear active on its own —
			// without this the panel ends up collapsed-but-active and
			// keyboard input keeps routing to the now-invisible viewport.
			if m.activePanelID == pid && !m.collapse[pid].IsCollapsed() {
				m = m.transitionToPassive()
			}
			m.collapse[pid].Toggle()
			m.persistLayoutIfEnabled()
		}
		m = m.resetPanelClick()
		return m, nil
	}

	// Body click on a panel that is NOT currently focused → just bring
	// it into focus. No collapse toggle, no active-mode promotion. The
	// click is recorded so a quick second click on the now-focused
	// panel can still chain into double-click → active-mode.
	if pid != m.focused {
		m = m.setFocus(pid)
		m = m.recordPanelClick(pid)
		return m, nil
	}

	// Body click on the already-focused panel — handle double-click first
	// so a quick second click promotes / demotes Expanded-Active mode.
	if m.isPanelDoubleClick(pid) {
		m = m.resetPanelClick()
		if m.isActiveMode() && m.activePanelID == pid {
			m = m.transitionToPassive()
		} else {
			m = m.transitionToActive()
		}
		return m, nil
	}

	// Single body click on the focused expanded explorer → dispatch to
	// the row-level handler so file/dir rows respond. The no-row
	// fallback inside that handler just records the click (NOT toggle
	// collapse, since the body is "content", not a collapse handle).
	if pid == PanelExplorer && m.bp != BreakpointNarrow && m.panelIsEffectivelyExpanded(pid) {
		return m.handleExplorerMouseClick(msg)
	}

	// Default: single body click on the focused panel just records the
	// click. No collapse change, no active-mode change — the user has
	// to either click the header to collapse, or double-click to enter
	// active mode.
	m = m.recordPanelClick(pid)
	return m, nil
}

// handleMouseWheel routes mouse wheel events to the panel under the cursor.
//
//   - Over the explorer panel: scrolls the tree by 3 rows in the wheel direction.
//     This focuses the explorer (so the highlight follows the wheel).
//   - Over the viewer (when active): forwards to the viewer viewport.
//   - Over any other panel with a viewport (calls/diff/usage in active mode):
//     forwards the wheel to that viewport so content scrolls.
func (m Model) handleMouseWheel(msg tea.MouseWheelMsg) (Model, tea.Cmd) {
	const wheelStep = 3

	// Same overlay guard as handleMouseClick — wheel scrolling on an
	// underlying panel would also be wrong while settings or help is up.
	if m.settingsOpen || m.helpOpen {
		return m, nil
	}

	if m.viewerActive {
		// Viewer takes the right column when active — wheel scrolls the
		// file. Vertical wheel hits the bubbles viewport's normal scroll
		// path; horizontal wheel (trackpad two-finger swipe) drives the
		// xOffset our renderer applies before each line is drawn.
		//
		// Horizontal step is intentionally 1 cell. macOS trackpads emit
		// MouseWheelLeft/Right events liberally on a small inertia kick
		// (the same swipe that's mostly vertical generates a handful of
		// horizontal events too); larger steps made the page jump
		// sideways every time the user tried to scroll vertically.
		const horizStep = 1
		switch msg.Button {
		case tea.MouseWheelLeft:
			m.viewport.xOffset -= horizStep
			if m.viewport.xOffset < 0 {
				m.viewport.xOffset = 0
			}
			return m, nil
		case tea.MouseWheelRight:
			m.viewport.xOffset += horizStep
			return m, nil
		}
		var cmd tea.Cmd
		m.viewport.vp, cmd = m.viewport.vp.Update(msg)
		return m, cmd
	}

	pid, ok := m.panelAtPos(msg.X, msg.Y)
	if !ok {
		return m, nil
	}

	if pid == PanelExplorer && m.bp != BreakpointNarrow {
		m = m.setFocus(PanelExplorer)
		// Decide which region the wheel applies to: when the cursor is
		// over the modified-files window we scroll that section; over
		// anywhere else in the explorer we move the tree highlight.
		// Without this split the user couldn't see modified files past
		// the first ~7 entries when they had a busy git status.
		inMod := m.wheelOverModSection(msg.Y)
		switch msg.Button {
		case tea.MouseWheelUp:
			for i := 0; i < wheelStep; i++ {
				if inMod {
					if m.explorer.modScrollOff > 0 {
						m.explorer.modScrollOff--
					}
				} else {
					m.explorer.MoveUp(len(m.data.ModifiedFiles))
				}
			}
		case tea.MouseWheelDown:
			for i := 0; i < wheelStep; i++ {
				if inMod {
					modAreaH := m.explorerModAreaH()
					maxOff := len(m.data.ModifiedFiles) - modAreaH
					if maxOff < 0 {
						maxOff = 0
					}
					if m.explorer.modScrollOff < maxOff {
						m.explorer.modScrollOff++
					}
				} else {
					m.explorer.MoveDown(len(m.data.ModifiedFiles))
				}
			}
		}
		return m, nil
	}

	// Scrollable panels: the wheel expresses scroll intent, and the only
	// mode that actually displays the panel's viewport is Expanded-Active.
	// Forwarding the wheel to a passive panel's viewport scrolled an
	// invisible buffer — the screen never moved ("scroll blocking"). So a
	// wheel over a non-active scrollable panel promotes it to active mode
	// first (same destination as double-click; esc backs out), then the
	// scroll applies to the now-visible viewport.
	switch pid {
	case PanelCalls, PanelDiff, PanelUsage, PanelServers, PanelBash, PanelCache:
		if !m.isActiveMode() || m.activePanelID != pid {
			m = m.setFocus(pid)
			m = m.transitionToActive()
		}
		var cmd tea.Cmd
		m.panelVPs[pid], cmd = m.panelVPs[pid].Update(msg)
		return m, cmd
	}
	return m, nil
}

// handleExplorerMouseClick handles a left-click inside the explorer panel.
// It focuses the explorer, sets the highlighted row to the clicked row,
// and triggers the same activate logic as Enter.
//
// Single-click on a file row → open viewer (existing behavior).
// Double-click on the SAME file row within explorerDoubleClickWindow →
// copy the absolute path to the clipboard via OSC 52 instead of
// re-opening. Directory rows are excluded from double-click → copy
// because the first click toggles the dir, which is already a useful
// repeated action and shouldn't morph into a yank.
//
// The visible row index from explorerRowAtPos is offset by the current
// scroll position so deep tree rows can be clicked after scrolling.
func (m Model) handleExplorerMouseClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	// Search button row — the affordance lives at the very top of the
	// explorer body. Click promotes the panel into active mode AND opens
	// the search overlay so the user lands directly in the prompt.
	if m.explorerSearchButtonAtPos(msg.X, msg.Y) {
		m = m.setFocus(PanelExplorer)
		m.activePanelID = PanelExplorer
		m.explorer.search = beginSearch(m.explorer.search)
		return m, nil
	}
	// Modified-files section sits above the tree — its rows have their own
	// hit zone, so check that first. A click on a modified file opens it
	// directly in the viewer; a double-click yanks its path. Row clicks
	// reset the panel-level click stamp so a prior header click can't
	// chain into active-mode after a yank.
	if idx, ok := m.modifiedFileAtPos(msg.X, msg.Y); ok {
		mf := m.data.ModifiedFiles[idx]
		m = m.setFocus(PanelExplorer)
		m = m.resetPanelClick()
		m.explorer.section = SectionMod
		m.explorer.modHighlight = idx
		if m.isExplorerDoubleClick(mf.Path) {
			m = m.resetExplorerClick()
			return m.copyExplorerHighlight(false)
		}
		m = m.recordExplorerClick(mf.Path)
		m = m.loadViewerFile(mf.Path)
		m = m.loadViewerDiff()
		return m, nil
	}

	visibleIdx, ok := m.explorerRowAtPos(msg.X, msg.Y)
	if !ok {
		// Click landed inside the focused expanded explorer but NOT on
		// a tree row (mid-panel blank area). Body clicks no longer
		// toggle collapse — that's the top-border row's job. Record
		// the click so the outer handler's double-click → active-mode
		// promotion still works on a quick second click.
		m = m.recordPanelClick(PanelExplorer)
		return m, nil
	}

	scrollOff := m.explorerVisibleScrollOff()
	treeIdx := scrollOff + visibleIdx
	rows := buildVisibleRows(m.data, m.explorer.collapsed)
	if treeIdx < 0 || treeIdx >= len(rows) {
		return m.setFocus(PanelExplorer), nil
	}

	row := rows[treeIdx]
	m = m.setFocus(PanelExplorer)
	m = m.resetPanelClick()
	m.explorer.section = SectionTree
	m.explorer.highlighted = treeIdx

	// File rows: detect double-click → copy path; otherwise record click
	// + open viewer. Dir rows skip the double-click path so users can
	// expand/collapse rapidly without accidentally copying a dir name.
	if !row.IsDir {
		if m.isExplorerDoubleClick(row.Path) {
			m = m.resetExplorerClick()
			return m.copyExplorerHighlight(false)
		}
		m = m.recordExplorerClick(row.Path)
	} else {
		m = m.resetExplorerClick()
	}
	return m.explorerActivate(), nil
}

// wheelOverModSection reports whether the screen-row y is inside the
// modified-files window of the explorer panel. Used by handleMouseWheel
// to route wheel events to the right section.
func (m Model) wheelOverModSection(y int) bool {
	modAreaH := m.explorerModAreaH()
	if modAreaH == 0 {
		return false
	}
	panelTop := m.explorerPanelTopRow()
	contentStart := panelTop + 1
	// Mod area: header at contentStart, rows at contentStart+1..contentStart+modAreaH.
	return y >= contentStart && y <= contentStart+modAreaH
}

// modifiedFileAtPos returns the absolute index (into d.ModifiedFiles) of
// the modified-file row under the cursor, or -1, false when the click is
// not on a modified-file row. The modified section is now scrollable, so
// the visible row offset must be added to es.modScrollOff to recover the
// real index.
func (m Model) modifiedFileAtPos(x, y int) (int, bool) {
	xMin, xMax := m.explorerPanelXBounds()
	if x < xMin || x > xMax {
		return -1, false
	}
	modAreaH := m.explorerModAreaH()
	if modAreaH == 0 {
		return -1, false
	}
	panelTop := m.explorerPanelTopRow()
	// +1 to skip the panel top border, +1 more to skip the search button row
	// that always sits as the first content row, +1 more to skip the
	// "modified N" header — total +3.
	contentStart := panelTop + 3
	contentRow := y - contentStart
	if contentRow < 0 || contentRow >= modAreaH {
		return -1, false
	}
	scrollOff := clampSimpleScroll(m.explorer.modScrollOff, len(m.data.ModifiedFiles), modAreaH)
	idx := scrollOff + contentRow
	if idx < 0 || idx >= len(m.data.ModifiedFiles) {
		return -1, false
	}
	return idx, true
}

// explorerSearchButtonAtPos reports whether (x, y) lands on the always-on
// search button row at the top of the explorer body (the first content row,
// just below the panel border). When true, the click handler should trigger
// the same code path as pressing /.
func (m Model) explorerSearchButtonAtPos(x, y int) bool {
	xMin, xMax := m.explorerPanelXBounds()
	if x < xMin || x > xMax {
		return false
	}
	panelTop := m.explorerPanelTopRow()
	// panelTop is the border row; row +1 is the search button.
	return y == panelTop+1
}

// explorerPanelTopRow returns the screen row of the top of the explorer panel content area.
// In 2-col mode, explorer is always the top panel in the left column (row 2 after title).
// In multi-col mode, explorer is also the top panel in the left column.
// In wide single stack mode, explorer comes after now/tasks/diff/usage.
func (m Model) explorerPanelTopRow() int {
	titleRows := 2

	mode := m.effectiveMode()

	if mode == LayoutStack && m.bp == BreakpointMedium {
		// 2-col: explorer is the top-left panel, starts immediately after title bar.
		return titleRows
	}

	if mode == LayoutMultiCol {
		// Multi-col: explorer is the leftmost top panel.
		return titleRows
	}

	// Wide single stack: explorer comes after now/tasks/diff/usage
	panels := m.activePanelsForBreakpoint()
	cur := titleRows
	for _, pid := range panels {
		if pid == PanelExplorer {
			return cur
		}
		cs := m.collapse[pid]
		var h int
		if cs.IsCollapsed() || cs.Height() <= 3 {
			h = 2
		} else {
			h = clamp(cs.Height(), 4, 40)
		}
		cur += h + 1 // +1 for separator newline
	}
	return cur
}

// explorerPanelXBounds returns the [xMin, xMax] x-range for the explorer panel
// in the current layout. Used by explorerRowAtPos to guard against out-of-column clicks.
func (m Model) explorerPanelXBounds() (xMin, xMax int) {
	mode := m.effectiveMode()
	w := m.width

	switch {
	case mode == LayoutStack && m.bp == BreakpointMedium:
		// Left column in 2-col: 0..leftW-1
		leftW := (w * 40) / 100
		if leftW < 22 {
			leftW = 22
		}
		if leftW > 50 {
			leftW = 50
		}
		return 0, leftW - 1

	case mode == LayoutMultiCol:
		// Left column: 0..21 (explorerW=22)
		return 0, 21

	default:
		// Single stack: full width
		return 0, w - 1
	}
}

// handleViewerScrollbarClick detects clicks on the viewer panel's scrollbar
// column and jumps the viewport to a YOffset proportional to the click's Y
// position within the scroll region. Returns (updated model, true) when the
// click was on the scrollbar; (m, false) otherwise so the caller can keep
// dispatching through the normal panel-routing path.
//
// Approximations (acceptable for a TUI scrollbar):
//   - Scrollbar column is the rightmost interior cell of the right column —
//     i.e. screenWidth-3 (border + padding + scrollbar character).
//   - Scroll region runs from titleH + 3 (after panel top border + 2 header
//     rows) down to the bottom border row of the panel.
func (m Model) handleViewerScrollbarClick(x, y int) (Model, bool) {
	if x < m.width-3 || x > m.width-2 {
		return m, false
	}
	titleH := 2
	notifH := notificationHeight(m.cfg.NotificationStyle, m.notifAck, m.hookNotif, m.compaction, m.quotaNotif)
	statusH := statusBarHeight(m.helpOpen)
	gridH := m.height - titleH - notifH - statusH
	if gridH < 4 {
		return m, false
	}
	scrollTop := titleH + 3 // panel top border + 2 header rows
	scrollBottom := titleH + gridH - 2
	if y < scrollTop || y > scrollBottom {
		return m, false
	}
	scrollRows := scrollBottom - scrollTop
	if scrollRows <= 0 {
		return m, false
	}
	// pct in [0, 1] of the click position within the scroll track.
	pct := float64(y-scrollTop) / float64(scrollRows)
	if pct < 0 {
		pct = 0
	}
	if pct > 1 {
		pct = 1
	}
	// Estimate the maxOffset using the model viewport's current line count;
	// renderTextViewer will clamp again on the next View pass.
	totalLines := strings.Count(m.viewport.vp.View(), "\n") + 1
	if totalLines < 2 {
		// Try the underlying total via TotalLineCount when available.
		totalLines = m.viewport.vp.TotalLineCount()
	}
	vpHeight := scrollRows + 1 // visible rows in scroll region
	maxOffset := totalLines - vpHeight
	if maxOffset < 1 {
		return m, true
	}
	target := int(pct*float64(maxOffset) + 0.5)
	m.viewport.vp.SetYOffset(target)
	return m, true
}

// activeBadgeAtPos reports whether (x, y) hits the active-mode badge
// on the currently-active panel's top border. The badge contains the
// "esc back" hint, and clicking it exits active mode (mirroring esc).
// Returns false when no panel is active or the click is elsewhere.
//
// Width math mirrors wrapPanelState's badge fallback (long vs short)
// via activeBadgeRunes, so the clickable region matches exactly what
// the user sees painted on the border.
func (m Model) activeBadgeAtPos(x, y int) bool {
	if !m.isActiveMode() {
		return false
	}
	pid := m.activePanelID
	var pb panelBounds
	found := false
	for _, b := range m.buildPanelBounds() {
		if b.pid == pid {
			pb = b
			found = true
			break
		}
	}
	if !found {
		return false
	}
	if y != pb.yMin {
		return false
	}
	panelW := pb.xMax - pb.xMin + 1
	badgeRunes := activeBadgeRunes(pid, panelW)
	// Badge sits flush with the right border: [xMax - badgeRunes, xMax - 1].
	badgeXMax := pb.xMax - 1
	badgeXMin := badgeXMax - badgeRunes + 1
	if badgeXMin < pb.xMin+1 {
		badgeXMin = pb.xMin + 1
	}
	return x >= badgeXMin && x <= badgeXMax
}

// viewerCloseButtonAtPos returns true iff (x, y) falls within the "esc close"
// badge region on the viewer panel's top border.
//
// The viewer occupies the right column in 2-col (medium) and multicol layouts,
// or the full width in narrow single-stack layout.
//
// Badge text rendered is " esc close " (11 runes including surrounding spaces).
// It sits at the top-right of the viewer panel, just inside the right border char.
// Region: y == viewerTop (the top border row), x in [xMax - badgeWidth, xMax - 1].
//
// badgeWidth = len(" esc close ") = 11 runes + 1 for right border char = 12 cols from right edge.
// viewerHeaderAtPos reports whether (x, y) lands on the viewer panel's
// top border but NOT on the close-button badge (which already has its
// own handler). Used to make a click on the title / chrome toggle the
// fullscreen state — a familiar gesture from any windowed editor.
func (m Model) viewerHeaderAtPos(x, y int) bool {
	if !m.viewerActive {
		return false
	}
	if m.viewerCloseButtonAtPos(x, y) {
		return false
	}
	titleRows := 2
	viewerTop := titleRows
	viewerXMin, viewerXMax := m.viewerXBounds()
	if y != viewerTop {
		return false
	}
	return x >= viewerXMin && x <= viewerXMax
}

func (m Model) viewerCloseButtonAtPos(x, y int) bool {
	if !m.viewerActive {
		return false
	}

	// The viewer panel top row is always titleRows (2), since the viewer
	// replaces the right-column content which starts immediately after the title bar.
	titleRows := 2
	viewerTop := titleRows

	// Viewer x bounds depend on layout.
	viewerXMin, viewerXMax := m.viewerXBounds()

	// y must be the top border row of the viewer panel.
	if y != viewerTop {
		return false
	}

	// Badge text: " esc close " = 11 runes (including surrounding spaces).
	// Sits flush with the right border, so it occupies columns [xMax-11, xMax-1].
	const badgeRunes = 11 // len(" esc close ")
	badgeXMin := viewerXMax - badgeRunes
	badgeXMax := viewerXMax - 1

	if badgeXMin < viewerXMin {
		badgeXMin = viewerXMin
	}

	return x >= badgeXMin && x <= badgeXMax
}

// viewerXBounds returns the [xMin, xMax] x-range for the viewer panel.
// In 2-col and multicol layouts the viewer occupies the right column.
// In narrow/single-stack it takes the full width.
func (m Model) viewerXBounds() (xMin, xMax int) {
	mode := m.effectiveMode()
	w := m.width

	switch {
	case mode == LayoutStack && m.bp == BreakpointMedium:
		// 2-col: viewer is in the right column (leftW..w-1)
		leftW := (w * 40) / 100
		if leftW < 22 {
			leftW = 22
		}
		if leftW > 50 {
			leftW = 50
		}
		return leftW, w - 1

	case mode == LayoutMultiCol:
		// Multi-col: viewer would be in the right column but viewer is not
		// normally active in multi-col; still compute for robustness.
		explorerW := 22
		middleW := 36
		return explorerW + middleW, w - 1

	default:
		// Narrow single-stack: viewer takes full width
		return 0, w - 1
	}
}

// explorerRowAtPos returns the *visible* tree row index under the cursor
// (x, y), or -1, false if not on a tree row. The visible index is relative
// to the top of the rendered tree section — callers add the current scroll
// offset to translate to an absolute tree index.
//
// v8: first checks that (x) is inside the explorer panel's column before
// computing the tree row — prevents false positives from right-column panels
// sharing the same y-range in 2-col mode.
//
// Explorer panel internal layout (inside border):
//
//	row 0:  "modified" section header
//	rows 1..N: modified files (len(d.ModifiedFiles) rows)
//	row N+1: dotted separator
//	row N+2: "tree" section header
//	rows N+3..N+2+T: tree rows (T = treeAreaH)
//	(separator + hint bar at bottom — not interactive)
func (m Model) explorerRowAtPos(x, y int) (visibleIndex int, ok bool) {
	// Guard: click must be in the explorer panel's x-range
	xMin, xMax := m.explorerPanelXBounds()
	if x < xMin || x > xMax {
		return -1, false
	}

	panelTop := m.explorerPanelTopRow()

	// Border line: panelTop is the border row itself — content starts at panelTop+1
	contentStart := panelTop + 1

	// Row offset within the content area
	contentRow := y - contentStart
	if contentRow < 0 {
		return -1, false
	}

	// Fixed rows before tree rows (modified section is now capped to a
	// fractional budget instead of len(ModifiedFiles)):
	//   0:                   search button row (always present)
	//   1:                   "modified" header  (skipped when no modified files)
	//   2..1+modAreaH:       modified rows window
	//   2+modAreaH:          dotted separator
	//   3+modAreaH:          "tree" header
	//   4+modAreaH..:        tree rows
	preTreeRows := 1 // search button row — always present
	if m.explorerModAreaH() > 0 {
		preTreeRows++ // mod header
	}
	preTreeRows += m.explorerModAreaH() + 2 // mod rows + sep + tree header

	treeOffset := contentRow - preTreeRows
	if treeOffset < 0 {
		return -1, false // click on modified section or header rows
	}

	// Bound by visible tree area, NOT by total row count, so clicks on the
	// padded blank rows or the footer don't register as tree hits.
	treeAreaH := m.explorerTreeAreaH()
	if treeOffset >= treeAreaH {
		return -1, false
	}

	// Also reject if there's no actual row at that visible position.
	rows := buildVisibleRows(m.data, m.explorer.collapsed)
	scrollOff := m.explorerVisibleScrollOff()
	if scrollOff+treeOffset >= len(rows) {
		return -1, false
	}

	return treeOffset, true
}
