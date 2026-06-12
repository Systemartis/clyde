package tui

// panelIsSelectable reports whether a panel can take keyboard focus.
// PanelNow is intentionally non-selectable — it has no scrollable
// content, no actions, and no scroll viewport, so focusing it gave the
// user a "stuck" feeling. Clicks on PanelNow trigger a mascot reaction
// instead (see handleMouseClick).
func panelIsSelectable(pid PanelID) bool {
	return pid != PanelNow
}

// selectableFrom filters a panel list to only the focus-eligible ones.
func selectableFrom(panels []PanelID) []PanelID {
	out := make([]PanelID, 0, len(panels))
	for _, p := range panels {
		if panelIsSelectable(p) {
			out = append(out, p)
		}
	}
	return out
}

// advanceFocus cycles focus forward (delta=1) or backward (delta=-1).
// In stack mode, auto-expands the newly focused panel. Skips
// non-selectable panels (PanelNow) so tab cycling never lands on them.
func (m Model) advanceFocus(delta int) Model {
	panels := selectableFrom(m.activePanelsForBreakpoint())
	if len(panels) == 0 {
		return m
	}

	// Find current focus index in the active panel list
	cur := 0
	for i, pid := range panels {
		if pid == m.focused {
			cur = i
			break
		}
	}

	next := (cur + delta + len(panels)) % len(panels)
	return m.setFocus(panels[next])
}

// setFocus changes the focused panel without changing its collapse state.
// Focus navigation (Tab, ↑, ↓, ←, →) never auto-expands panels.
// Use collapse[pid].Expand() or Toggle() explicitly when expansion is intended.
// If a panel is in Expanded-Active state and focus moves away, it returns to
// Expanded-Passive (active mode clears, but panel stays expanded).
//
// Non-selectable panels (PanelNow) are silently rejected so any caller —
// click handler, jump shortcut, etc. — can't accidentally trap the user
// on a panel that has nothing to interact with.
func (m Model) setFocus(pid PanelID) Model {
	if !panelIsSelectable(pid) {
		return m
	}
	if m.focused != pid && m.isActiveMode() && m.activePanelID == m.focused {
		// Panel losing focus was in active mode → demote to passive (stay expanded)
		m.activePanelID = PanelNone
	}
	m.focused = pid
	return m
}

// cycleLayoutMode advances to the next layout mode.
func (m Model) cycleLayoutMode() Model {
	switch m.layoutMode {
	case LayoutStack:
		m.layoutMode = LayoutTabs
	case LayoutTabs:
		if m.width >= 160 {
			m.layoutMode = LayoutMultiCol
		} else {
			m.layoutMode = LayoutStack
		}
	case LayoutMultiCol:
		m.layoutMode = LayoutStack
	}
	return m
}

// effectiveMode returns the actual layout mode considering auto-switch threshold.
func (m Model) effectiveMode() LayoutMode {
	return ResolveMode(m.layoutMode, m.cfg.Layout.AutoSwitchThreshold, m.width)
}

// isActiveMode returns true when some panel is in Expanded-Active state.
func (m Model) isActiveMode() bool {
	return m.activePanelID != PanelNone
}

// transitionToActive enters Expanded-Active state for the focused panel.
// Ensures the panel is expanded and resets viewport scroll to 0.
func (m Model) transitionToActive() Model {
	pid := m.focused
	// Ensure the panel is expanded
	wasCollapsed := m.collapse[pid].IsCollapsed()
	m.collapse[pid].Expand()
	if wasCollapsed {
		m.persistLayoutIfEnabled()
	}
	m.activePanelID = pid
	// Reset viewport so active mode starts at top (fresh scroll)
	m.panelVPs[pid].SetYOffset(0)
	// Wire content into viewport now that the panel is being activated
	m = m.syncPanelViewport(pid)
	// The activity panel is a stream — the tail (newest calls) is the
	// natural anchor, mirroring the passive card's tail-windowing.
	if pid == PanelCalls {
		m.panelVPs[pid].GotoBottom()
	}
	return m
}

// transitionToPassive exits Expanded-Active state for the current active panel
// back to Expanded-Passive. The panel stays expanded.
func (m Model) transitionToPassive() Model {
	m.activePanelID = PanelNone
	return m
}

// twoColLeftPanels returns the left-column panels for the 2-col (medium) layout.
// Filters by enabled state so disabled panels (via cfg) drop out of focus
// cycling.
func (m Model) twoColLeftPanels() []PanelID {
	return m.filterPanels([]PanelID{PanelExplorer, PanelServers})
}

// twoColRightPanels returns the right-column panels for the 2-col layout.
// PanelNow is excluded because it's non-selectable — keeping it would
// make ↑/↓ pause on a panel the user can't interact with.
func (m Model) twoColRightPanels() []PanelID {
	return selectableFrom(m.filterPanels([]PanelID{PanelNow, PanelCalls, PanelDiff, PanelUsage, PanelBash, PanelCache}))
}

// advanceFocusInColumn2Col cycles focus within the current 2-col column.
// Used by ↑/↓ at BreakpointMedium so navigating in the left column
// (explorer ↔ servers) doesn't accidentally jump to the right column.
func (m Model) advanceFocusInColumn2Col(delta int) Model {
	var col []PanelID
	if m.twoColColumnIndex(m.focused) == 0 {
		col = m.twoColLeftPanels()
	} else {
		col = m.twoColRightPanels()
	}
	if len(col) == 0 {
		return m
	}
	cur := 0
	for i, p := range col {
		if p == m.focused {
			cur = i
			break
		}
	}
	next := (cur + delta + len(col)) % len(col)
	return m.setFocus(col[next])
}

// twoColColumnIndex returns 0 (left) or 1 (right) for the given panel in 2-col mode.
func (m Model) twoColColumnIndex(pid PanelID) int {
	for _, p := range []PanelID{PanelExplorer, PanelServers} {
		if p == pid {
			return 0
		}
	}
	return 1
}

// moveColumn2ColLeft moves focus to the left column (top panel) in 2-col mode.
// No-op if already on the left column or no left-column panels are enabled.
func (m Model) moveColumn2ColLeft() Model {
	if m.twoColColumnIndex(m.focused) == 0 {
		return m
	}
	left := m.twoColLeftPanels()
	if len(left) == 0 {
		return m
	}
	return m.setFocus(left[0])
}

// moveColumn2ColRight moves focus to the right column (top panel) in 2-col mode.
// No-op if already on the right column or no right-column panels are enabled.
func (m Model) moveColumn2ColRight() Model {
	if m.twoColColumnIndex(m.focused) == 1 {
		return m
	}
	right := m.twoColRightPanels()
	if len(right) == 0 {
		return m
	}
	return m.setFocus(right[0])
}

// columnForPanel returns 0=left, 1=middle, 2=right column index for a panel in multi-col mode.
func columnForPanel(pid PanelID) int {
	switch pid {
	case PanelExplorer:
		return 0
	case PanelNow, PanelCalls, PanelDiff:
		return 1
	default: // PanelUsage, PanelServers, PanelBash
		return 2
	}
}

// panelsInColumn returns ordered panel IDs for a given column in multi-col mode.
// Non-selectable panels (PanelNow) are filtered out — the cursor would
// otherwise pause on a panel that has no interactive surface.
func (m Model) panelsInColumn(col int) []PanelID {
	switch col {
	case 0:
		return selectableFrom(m.filterPanels([]PanelID{PanelExplorer}))
	case 1:
		return selectableFrom(m.filterPanels([]PanelID{PanelNow, PanelCalls, PanelDiff}))
	default:
		return selectableFrom(m.filterPanels([]PanelID{PanelUsage, PanelServers, PanelBash, PanelCache}))
	}
}

// advanceFocusInColumn moves focus within the same column in multi-col mode.
func (m Model) advanceFocusInColumn(delta int) Model {
	col := columnForPanel(m.focused)
	panels := m.panelsInColumn(col)
	if len(panels) == 0 {
		return m
	}
	cur := 0
	for i, p := range panels {
		if p == m.focused {
			cur = i
			break
		}
	}
	next := (cur + delta + len(panels)) % len(panels)
	return m.setFocus(panels[next])
}

// focusNextColumn moves focus to the top panel of the next column.
func (m Model) focusNextColumn() Model {
	col := columnForPanel(m.focused)
	nextCol := (col + 1) % 3
	panels := m.panelsInColumn(nextCol)
	if len(panels) > 0 {
		return m.setFocus(panels[0])
	}
	return m
}

// focusPrevColumn moves focus to the top panel of the previous column.
func (m Model) focusPrevColumn() Model {
	col := columnForPanel(m.focused)
	prevCol := (col + 2) % 3
	panels := m.panelsInColumn(prevCol)
	if len(panels) > 0 {
		return m.setFocus(panels[0])
	}
	return m
}
