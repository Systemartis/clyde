package tui

import (
	"fmt"
	"strings"
)

// Tab definition for Mode B.
type tabDef struct {
	id      PanelID
	label   string
	summary string // short status summary shown in the strip
}

// activeTabs returns the ordered set of tabs for Mode B.
func (m Model) activeTabs(d MockData) []tabDef {
	// Summarize calls: "N done · M active"
	callsActive, callsDone := 0, 0
	for _, g := range d.AgentGroups {
		for _, c := range g.Calls {
			switch c.State {
			case CallActive:
				callsActive++
			case CallDone:
				callsDone++
			}
		}
	}
	callsSummary := fmt.Sprintf("%d done · %d active", callsDone, callsActive)

	// PanelNow is deliberately NOT a tab: it is non-selectable (no scroll,
	// no actions — see panelIsSelectable), so setFocus rejects it and it
	// could never be brought into the full-panel view. Listing it as tab 1
	// left an advertised tab whose jump key was a dead no-op. The now-panel
	// content still surfaces in the stack / multi-col layouts.
	tabs := []tabDef{
		{PanelCalls, "activity", callsSummary},
	}
	if m.panelEnabled(PanelDiff) {
		tabs = append(tabs, tabDef{PanelDiff, "diff", d.DiffFile})
	}
	tabs = append(tabs, tabDef{PanelUsage, "usage", fmt.Sprintf("%d%%", d.TokenPct)})
	return tabs
}

// activeTabIndex returns the strip index of the currently focused tab, or
// 0 when the focused panel is not in the strip (keeps the highlight and
// the shown panel from disagreeing).
func activeTabIndex(tabs []tabDef, focused PanelID) int {
	for i, t := range tabs {
		if t.id == focused {
			return i
		}
	}
	return 0
}

// renderTabs renders Mode B — tab strip with full-focus single panel.
func (m Model) renderTabs() string {
	w := m.width
	h := m.height

	titleH := 2
	tabStripH := 1
	notifH := notificationHeight(m.cfg.NotificationStyle, m.notifAck, m.hookNotif, m.compaction, m.quotaNotif)
	// The tab status bar (renderTabStatusBar) is ALWAYS 2 rows — it has no
	// help-expanded form. Reserving statusBarHeight(helpOpen)=4 when help
	// is open would under-fill the panel by 2 rows, leaving a gap at the
	// bottom. Budget exactly what we render.
	statusH := 2
	chromH := titleH + tabStripH + notifH + statusH

	panelH := h - chromH
	if panelH < 6 {
		panelH = 6
	}

	tabs := m.activeTabs(m.data)
	activeIdx := activeTabIndex(tabs, m.focused)
	// shown is always a strip panel: focus is constrained to the strip in
	// tabs mode (advanceTabFocus / gated jumps), but if focus somehow sits
	// off-strip (e.g. entering tabs mode from the explorer) fall back to the
	// first tab so the highlight and the shown panel never disagree.
	shown := tabs[activeIdx].id

	titleBar := renderTitleBar(m.styles, m.palette, m.data, m.frame, w, m.demoMode, m.liveView, m.liveView.LastUpdate)
	notification := renderNotificationMaybe(m.styles, m.palette, w, m.cfg.NotificationStyle, m.notifAck, m.hookNotif, m.compaction, m.quotaNotif)
	statusBar := renderTabStatusBar(m.styles, w, activeIdx, len(tabs), m.version)

	if overlay := m.notificationOverlay(w, panelH+tabStripH); overlay != "" {
		return strings.Join([]string{titleBar, overlay, statusBar}, "\n")
	}

	// Build tab strip
	tabStrip := renderTabStrip(m.styles, tabs, activeIdx, w)

	// Render the shown panel full-width
	panel := m.renderExpandedPanel(shown, w, panelH, true)

	parts := []string{titleBar, tabStrip, panel}
	if notification != "" {
		parts = append(parts, notification)
	}
	parts = append(parts, statusBar)
	return strings.Join(parts, "\n")
}

// renderTabStrip renders the horizontal tab strip for Mode B.
// Format: ─── now* ── tasks 3/7 ── diff +14 ── usage 23% ── ···  › ────
func renderTabStrip(s Styles, tabs []tabDef, activeIdx int, width int) string {
	var parts []string

	for i, tab := range tabs {
		var label string
		if i == activeIdx {
			// Active tab: asterisk + purple
			label = s.TabActive.Render(tab.label + "*")
		} else {
			// Inactive: dim label + summary
			label = s.TabInactive.Render(tab.label + " " + tab.summary)
		}
		parts = append(parts, label)
	}

	// Join with separator
	sep := s.TabBorder.Render(" ── ")
	content := strings.Join(parts, sep)

	// Pad to width
	contentW := ansiWidth(content)
	pad := width - contentW - 4
	if pad < 0 {
		pad = 0
	}

	left := s.TabBorder.Render("──── ")
	right := strings.Repeat(" ", pad) + s.TabBorder.Render(" ────")

	return left + content + right
}
