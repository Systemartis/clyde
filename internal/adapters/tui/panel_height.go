package tui

// panelHeightMin is the minimum allowed panel height (border + 2 content rows).
const panelHeightMin = 4

// resizeFocusedPanel adjusts the focused panel's custom height by delta rows.
// Positive delta grows the panel; negative delta shrinks it.
// The height is clamped: [panelHeightMin, termHeight-8].
func (m *Model) resizeFocusedPanel(delta int) {
	cur := m.panelHeights[m.focused]
	if cur == 0 {
		// Use current spring height as the baseline when no override exists yet.
		cur = m.collapse[m.focused].Height()
	}
	next := cur + delta
	maxH := m.height - 8
	if maxH < panelHeightMin {
		maxH = panelHeightMin
	}
	if next < panelHeightMin {
		next = panelHeightMin
	}
	if next > maxH {
		next = maxH
	}
	if m.panelHeights == nil {
		m.panelHeights = make(map[PanelID]int)
	}
	m.panelHeights[m.focused] = next
	// Also update the spring target so animation converges to the new height.
	m.collapse[m.focused].SetExpandedHeight(float64(next))
	m.persistLayoutIfEnabled()
}

// panelHeight returns the EFFECTIVE settled height for a panel — the
// user's manual resize override when present, otherwise the current
// spring height. Used by layout.go to reserve stable row counts for
// the 2-col layout (explorer / servers); the in-stack renderer uses
// `cs.Height()` directly so collapse / expand / +/- transitions
// animate frame-by-frame. Read both sides this way to avoid layout
// jitter while still showing motion in the panel body.
func (m Model) panelHeight(pid PanelID) int {
	if h, ok := m.panelHeights[pid]; ok && h > 0 {
		return h
	}
	return m.collapse[pid].Height()
}

// panelSettledHeight returns the height pid settles at in the current
// layout once animations finish: layout-pinned heights in multi-col and
// the 2-col left column, otherwise the manual override or the expanded
// spring target. Used to size panel viewports so scroll clamping matches
// what the panel will actually display.
func (m Model) panelSettledHeight(pid PanelID) int {
	l := m.computeLayout()
	switch {
	case l.Mode == LayoutMultiCol:
		switch pid {
		case PanelExplorer:
			return l.GridH
		case PanelNow:
			return l.MultiNowH
		case PanelCalls:
			return l.MultiCallsH
		case PanelDiff:
			return l.MultiDiffH
		case PanelUsage:
			return l.MultiUsageH
		case PanelServers:
			return l.MultiServersH
		case PanelBash:
			return l.MultiBashH
		case PanelCache:
			return l.MultiCacheH
		}
	case l.Mode == LayoutStack && l.BP == BreakpointMedium:
		switch pid {
		case PanelExplorer:
			return l.ExplorerH
		case PanelServers:
			return l.ServersH
		}
	}
	if h, ok := m.panelHeights[pid]; ok && h > 0 {
		return clamp(h, panelHeightMin, 40)
	}
	return clamp(m.collapse[pid].ExpandedHeight(), panelHeightMin, 40)
}

// persistLayoutIfEnabled writes the current per-panel collapse + height
// state to baseCfg and the on-disk config file when RememberLayout is on.
// No-op otherwise — runtime changes stay in-session.
//
// Called from every code path that mutates collapse[pid] or
// panelHeights[pid] interactively (resize, space-toggle, header click,
// backspace-collapse, focus-expand). Pure save-on-change — no debounce;
// disk write cost is negligible compared to the input event cadence.
func (m *Model) persistLayoutIfEnabled() {
	if !m.baseCfg.RememberLayout {
		return
	}
	for pid := PanelID(0); pid < panelCount; pid++ {
		setPanelLayoutInCfg(&m.baseCfg, pid, m.collapse[pid].IsCollapsed(), m.panelHeights[pid])
	}
	writeConfigFile(m.baseCfg)
}

// setPanelLayoutInCfg writes collapsed + height into the per-panel slot
// of the given Config. Height==0 clears any stored override (back to
// computed default). Kept narrow on purpose: settings overlay edits go
// through applySettings, which leaves these fields alone.
func setPanelLayoutInCfg(cfg *Config, pid PanelID, collapsed bool, height int) {
	pc := panelConfigPtr(cfg, pid)
	if pc == nil {
		return
	}
	pc.DefaultCollapsed = collapsed
	pc.Height = height
}

// panelConfigPtr returns a pointer to the PanelConfig slot in cfg for
// the given PanelID, or nil if the id has no slot. Centralizes the
// PanelID → cfg field mapping that previously appeared inline in three
// places (apply, override, current-value lookups).
func panelConfigPtr(cfg *Config, pid PanelID) *PanelConfig {
	switch pid {
	case PanelNow:
		return &cfg.Panels.Now
	case PanelCalls:
		return &cfg.Panels.Calls
	case PanelDiff:
		return &cfg.Panels.Diff
	case PanelUsage:
		return &cfg.Panels.Usage
	case PanelExplorer:
		return &cfg.Panels.Explorer
	case PanelServers:
		return &cfg.Panels.Servers
	case PanelBash:
		return &cfg.Panels.Bash
	case PanelCache:
		return &cfg.Panels.Cache
	}
	return nil
}
