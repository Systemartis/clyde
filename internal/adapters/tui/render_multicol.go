package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// renderMultiCol renders Mode C — three-column dashboard (140+ cols).
// This is the v3 design updated for v4 (rounded borders, padding, better color hierarchy).
func (m Model) renderMultiCol() string {
	l := m.computeLayout()

	titleBar := renderTitleBar(m.styles, m.palette, m.data, m.frame, l.Width, m.demoMode, m.liveView, m.liveView.LastUpdate)
	statusBar := renderStatusBar(m.styles, l.Width, m.isActiveMode(), m.copyToast, m.data.Sessions, m.helpOpen, m.version)

	if overlay := m.notificationOverlay(l.Width, l.GridH); overlay != "" {
		return strings.Join([]string{titleBar, overlay, statusBar}, "\n")
	}

	explorerPanel := m.renderMultiColExplorer(l)
	middleCol := m.renderMultiColMiddle(l)
	rightCol := m.renderMultiColRight(l)

	grid := lipgloss.JoinHorizontal(lipgloss.Top,
		padToHeight(explorerPanel, l.MultiExplorerW, l.GridH),
		padToHeight(middleCol, l.MultiMiddleW, l.GridH),
		padToHeight(rightCol, l.MultiRightW, l.GridH),
	)

	notification := renderNotificationMaybe(m.styles, m.palette, l.Width, m.cfg.NotificationStyle, m.notifAck, m.hookNotif, m.compaction, m.quotaNotif)

	parts := []string{titleBar, grid}
	if notification != "" {
		parts = append(parts, notification)
	}
	parts = append(parts, statusBar)
	return strings.Join(parts, "\n")
}

func (m Model) renderMultiColExplorer(l Layout) string {
	if !m.panelEnabled(PanelExplorer) {
		return ""
	}
	return m.renderExpandedPanel(PanelExplorer, l.MultiExplorerW, l.GridH, m.visualFocus(PanelExplorer))
}

func (m Model) renderMultiColMiddle(l Layout) string {
	w := l.MultiMiddleW
	var panels []string
	if m.panelEnabled(PanelNow) {
		panels = append(panels, m.renderExpandedPanel(PanelNow, w, l.MultiNowH, m.visualFocus(PanelNow)))
	}
	if m.panelEnabled(PanelCalls) {
		panels = append(panels, m.renderExpandedPanel(PanelCalls, w, l.MultiCallsH, m.visualFocus(PanelCalls)))
	}
	if m.panelEnabled(PanelDiff) {
		panels = append(panels, m.renderExpandedPanel(PanelDiff, w, l.MultiDiffH, m.visualFocus(PanelDiff)))
	}
	return lipgloss.JoinVertical(lipgloss.Left, panels...)
}

func (m Model) renderMultiColRight(l Layout) string {
	w := l.MultiRightW
	var panels []string
	if m.panelEnabled(PanelUsage) {
		panels = append(panels, m.renderExpandedPanel(PanelUsage, w, l.MultiUsageH, m.visualFocus(PanelUsage)))
	}
	if m.panelEnabled(PanelServers) {
		panels = append(panels, m.renderExpandedPanel(PanelServers, w, l.MultiServersH, m.visualFocus(PanelServers)))
	}
	if m.panelEnabled(PanelBash) {
		panels = append(panels, m.renderExpandedPanel(PanelBash, w, l.MultiBashH, m.visualFocus(PanelBash)))
	}
	if m.panelEnabled(PanelCache) {
		panels = append(panels, m.renderExpandedPanel(PanelCache, w, l.MultiCacheH, m.visualFocus(PanelCache)))
	}
	return lipgloss.JoinVertical(lipgloss.Left, panels...)
}
