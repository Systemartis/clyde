package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// renderStack2Col renders the medium (80-159 col) two-column layout.
//
// v7 column ordering:
//
//	Left column  (~40% width): explorer (top, large) + servers
//	Right column (~60% width): now + tasks + diff + usage stacked (OR viewer if viewerActive)
//
// This matches v3's "explorer with tree, modified files, lsp/mcp servers below" architecture.
func (m Model) renderStack2Col() string {
	l := m.computeLayout()

	titleBar := renderTitleBar(m.styles, m.palette, m.data, m.frame, l.Width, m.demoMode, m.liveView, m.liveView.LastUpdate)
	statusBar := renderStatusBar(m.styles, l.Width, m.isActiveMode(), m.copyToast, m.data.Sessions, m.helpOpen, m.version)

	// Fullscreen overlay short-circuits before any panel rendering — saves
	// the per-panel render work when we're going to throw it away.
	if overlay := m.notificationOverlay(l.Width, l.GridH); overlay != "" {
		return strings.Join([]string{titleBar, overlay, statusBar}, "\n")
	}

	leftCol := m.render2ColLeft(l)

	// ── Right column: viewer OR now/tasks/diff/usage/bash/cache stacked ─────
	var rightCol string
	if m.viewerActive {
		rightCol = m.renderViewerPanel(l.RightW, l.GridH)
	} else {
		rightPanels := m.filterPanels(twoColRightPanelOrder())
		var rightRendered []string
		for _, pid := range rightPanels {
			rightRendered = append(rightRendered, m.renderStackPanel(pid, l.RightW))
		}
		rightCol = strings.Join(rightRendered, "\n")
	}

	// Compose columns side by side
	grid := lipgloss.JoinHorizontal(lipgloss.Top,
		padToHeight(leftCol, l.LeftW, l.GridH),
		padToHeight(rightCol, l.RightW, l.GridH),
	)

	notification := renderNotificationMaybe(m.styles, m.palette, l.Width, m.cfg.NotificationStyle, m.notifAck, m.hookNotif, m.compaction, m.quotaNotif)

	parts := []string{titleBar, grid}
	if notification != "" {
		parts = append(parts, notification)
	}
	parts = append(parts, statusBar)
	return strings.Join(parts, "\n")
}

// render2ColLeft renders the explorer + servers stack for 2-col mode,
// honoring per-panel Enabled flags (disabled panels claim no rows).
// Heights come from Layout (single source of truth) so the click
// handler agrees on where each panel ends.
func (m Model) render2ColLeft(l Layout) string {
	var panels []string
	if m.panelEnabled(PanelExplorer) && l.ExplorerH > 0 {
		panels = append(panels, m.renderExpandedPanel(PanelExplorer, l.LeftW, l.ExplorerH, m.visualFocus(PanelExplorer)))
	}
	if m.panelEnabled(PanelServers) && l.ServersH > 0 {
		panels = append(panels, m.renderExpandedPanel(PanelServers, l.LeftW, l.ServersH, m.visualFocus(PanelServers)))
	}
	return lipgloss.JoinVertical(lipgloss.Left, panels...)
}
