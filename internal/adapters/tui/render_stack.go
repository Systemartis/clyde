package tui

import (
	"strings"
)

// renderStack renders Mode A — vertical accordion stack.
//
// Layout:
//
//	Narrow (< 80 cols):  titlebar, now, tasks, diff, usage, notification, statusbar
//	Medium (80-159 cols): two-column: left=explorer+servers, right=now/tasks/diff/usage
//	Wide (160+ cols, user chose stack): all 6 panels single column
//
// Each panel is either expanded (wrapPanel) or collapsed (wrapPanelCollapsed).
// The focused panel is always auto-expanded. spring heights animate transitions.
func (m Model) renderStack() string {
	// At medium breakpoint, delegate to two-column renderer
	if m.bp == BreakpointMedium {
		return m.renderStack2Col()
	}

	l := m.computeLayout()
	w := l.Width

	titleBar := renderTitleBar(m.styles, m.palette, m.data, m.frame, w, m.demoMode, m.liveView, m.liveView.LastUpdate)
	notification := renderNotificationMaybe(m.styles, m.palette, w, m.cfg.NotificationStyle, m.notifAck, m.hookNotif, m.compaction, m.quotaNotif)
	statusBar := renderStatusBar(m.styles, w, m.isActiveMode(), m.copyToast, m.data.Sessions, m.helpOpen, m.version)

	// Fullscreen notification: replace the panel grid with an animated
	// overlay so the user can't miss the prompt. The titlebar and status
	// bar stay so the user keeps their bearings.
	if overlay := m.notificationOverlay(w, l.GridH); overlay != "" {
		return strings.Join([]string{titleBar, overlay, statusBar}, "\n")
	}

	// Determine which panels are active at this breakpoint
	panels := m.activePanelsForBreakpoint()

	// Render each panel
	var rendered []string
	for _, pid := range panels {
		rendered = append(rendered, m.renderStackPanel(pid, w))
	}

	grid := strings.Join(rendered, "\n")

	parts := []string{titleBar, grid}
	if notification != "" {
		parts = append(parts, notification)
	}
	parts = append(parts, statusBar)
	return strings.Join(parts, "\n")
}

// activePanelsForBreakpoint returns ordered panel IDs for the current breakpoint.
// For the medium 2-col layout, the right column stack uses only now/tasks/diff/usage;
// explorer and servers live in the left column.
func (m Model) activePanelsForBreakpoint() []PanelID {
	switch m.bp {
	case BreakpointNarrow:
		return m.filterPanels([]PanelID{PanelNow, PanelCalls, PanelDiff, PanelUsage})
	case BreakpointMedium:
		// 2-col layout — Tab cycles through all panels in both columns so
		// the user can reach left-column panels (explorer + servers) without
		// having to remember Ctrl+E. Order matches visual top-to-bottom by
		// column: left first, then right.
		return m.filterPanels([]PanelID{PanelExplorer, PanelServers, PanelNow, PanelCalls, PanelDiff, PanelUsage, PanelBash, PanelCache})
	default:
		// Wide stack: every panel.
		return m.filterPanels([]PanelID{PanelNow, PanelCalls, PanelDiff, PanelUsage, PanelBash, PanelCache, PanelExplorer, PanelServers})
	}
}

// filterPanels drops panels whose Enabled flag is false in the current cfg.
// Replaces the old filterDiff helper — every panel is now individually
// gated via PanelConfig.Enabled, configurable via the settings overlay or
// the TOML file. Per-project overrides go through cfg.EffectiveFor at
// startup and on-the-fly via the settings overlay.
func (m Model) filterPanels(panels []PanelID) []PanelID {
	out := panels[:0]
	for _, p := range panels {
		if m.panelEnabled(p) {
			out = append(out, p)
		}
	}
	return out
}

// panelEnabled returns whether a given panel is enabled in the active cfg.
func (m Model) panelEnabled(p PanelID) bool {
	switch p {
	case PanelNow:
		return m.cfg.Panels.Now.Enabled
	case PanelCalls:
		return m.cfg.Panels.Calls.Enabled
	case PanelDiff:
		return m.cfg.Panels.Diff.Enabled
	case PanelUsage:
		return m.cfg.Panels.Usage.Enabled
	case PanelExplorer:
		return m.cfg.Panels.Explorer.Enabled
	case PanelServers:
		return m.cfg.Panels.Servers.Enabled
	case PanelBash:
		return m.cfg.Panels.Bash.Enabled
	case PanelCache:
		return m.cfg.Panels.Cache.Enabled
	}
	return true
}

// renderStackPanel renders one panel in stack mode (collapsed or expanded).
// Height is driven entirely by the panel's CollapseSpring so collapse,
// expand, AND +/- resize all animate symmetrically. The IsCollapsed()
// flag is a logical-state hint; we deliberately do NOT short-circuit on
// it — that used to flip the renderer to the collapsed body the moment
// Toggle() flipped state, hiding the spring's collapse-direction frames
// entirely (expand looked smooth, collapse snapped).
func (m Model) renderStackPanel(pid PanelID, width int) string {
	focused := m.visualFocus(pid)
	cs := m.collapse[pid]

	springH := cs.Height()
	if springH <= 3 {
		return m.renderCollapsedPanel(pid, width, focused)
	}

	panelH := springH
	if panelH < 4 {
		panelH = 4
	}
	return m.renderExpandedPanel(pid, width, panelH, focused)
}

// renderExpandedPanel renders a panel in its full expanded form.
func (m Model) renderExpandedPanel(pid PanelID, width, height int, focused bool) string {
	s := m.styles
	p := m.palette
	d := m.data
	f := m.frame
	// A panel is in Expanded-Active state only when it IS the activePanelID.
	activeMode := m.activePanelID == pid && m.isActiveMode()
	// Smooth focus fade: tint the chrome partway between dim and full
	// purple based on m.focusAlpha. Skipped in active mode — the magenta
	// double-border is a discrete UX state that shouldn't fade.
	if !activeMode {
		if a := m.focusAlpha(pid); a > 0 && a < 1 {
			s = s.WithFadedFocus(p, a)
		}
	}

	// Help mode swaps every expanded panel's body for its keybind cheat
	// sheet. The user toggles via `h`; collapsed panels skip this and
	// keep their normal one-liner so the layout doesn't get bigger when
	// the user just wants to peek at commands.
	if m.helpOpen {
		return renderPanelHelp(s, pid, width, height, focused, activeMode)
	}

	switch pid {
	case PanelNow:
		return renderNow(s, p, d, f, m.cfg.MascotPersona, width, height, focused)
	case PanelCalls: // calls panel occupies the PanelCalls slot in v13
		return renderCallsExpanded(s, p, d, m.panelVPs[pid], width, height, focused, activeMode)
	case PanelDiff:
		return renderDiffExpanded(s, p, d, m.panelVPs[pid], width, height, focused, activeMode)
	case PanelUsage:
		return renderUsageExpanded(s, p, d, m.progTokens, m.progReset, m.panelVPs[pid], width, height, focused, activeMode)
	case PanelExplorer:
		return renderExplorerExpanded(s, p, d, m.explorer, m.panelVPs[pid], width, height, focused, activeMode)
	case PanelServers:
		return renderServersExpanded(s, p, d, m.panelVPs[pid], width, height, focused, activeMode)
	case PanelBash:
		return renderBashExpanded(s, p, d, m.panelVPs[pid], width, height, focused, activeMode)
	case PanelCache:
		return renderCacheExpanded(s, p, d, m.panelVPs[pid], width, height, focused, activeMode)
	}
	return ""
}

// renderCollapsedPanel renders the collapsed one-liner for a panel.
func (m Model) renderCollapsedPanel(pid PanelID, width int, focused bool) string {
	s := m.styles
	p := m.palette
	d := m.data
	f := m.frame
	// Smooth focus fade for the collapsed-panel chrome — same blend as the
	// expanded path so the tint stays consistent regardless of whether the
	// panel is open.
	if a := m.focusAlpha(pid); a > 0 && a < 1 {
		s = s.WithFadedFocus(p, a)
	}

	switch pid {
	case PanelNow:
		return renderNowCollapsed(s, d, f, m.cfg.MascotPersona, width, focused)
	case PanelCalls: // calls panel occupies the PanelCalls slot in v13
		return renderCallsCollapsed(s, p, d, width, focused)
	case PanelDiff:
		return renderDiffCollapsed(s, d, width, focused)
	case PanelUsage:
		return renderUsageCollapsed(s, d, m.compaction, width, focused)
	case PanelExplorer:
		return renderExplorerCollapsed(s, d, width, focused)
	case PanelServers:
		return renderServersCollapsed(s, d, width, focused)
	case PanelBash:
		return renderBashCollapsed(s, d, width, focused)
	case PanelCache:
		return renderCacheCollapsed(s, d, width, focused)
	}
	return ""
}

// defaultExpandedHeight returns the natural expanded height for a panel
// given the available height and breakpoint.
func (m Model) defaultExpandedHeight(pid PanelID, availH int, bp Breakpoint) float64 {
	// Distribute available height across panels based on their natural size needs
	switch pid {
	case PanelNow:
		return 10.0
	case PanelCalls:
		// When the standalone diff panel is hidden (v22+ default), the activity
		// panel inherits its share of the vertical budget so subagent cards
		// and inline diff hunks have room to breathe.
		divisor := 2.5
		if !m.panelEnabled(PanelDiff) {
			divisor = 1.6
		}
		h := float64(availH) / divisor
		if h < 12 {
			h = 12
		}
		return h
	case PanelDiff:
		h := float64(availH) / 3.0
		if h < 8 {
			h = 8
		}
		return h
	case PanelUsage:
		return 14.0
	case PanelExplorer:
		if bp == BreakpointNarrow {
			return 15.0
		}
		return 18.0
	case PanelServers:
		// 4 MCPs + 3 LSPs + 2 sub-headers + 1 divider + 2 border = 12 content + 2 border
		return 13.0
	case PanelBash:
		// Compact ledger — 8 rows of commands by default; user can resize.
		return 10.0
	case PanelCache:
		// 4 content rows (headline+bar, breakdown, biggest miss, sparkline)
		// + 2 border + 1 spacing → 7. Keep at 8 for breathing room.
		return 8.0
	}
	return 10.0
}
