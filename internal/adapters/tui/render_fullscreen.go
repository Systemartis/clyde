package tui

import "strings"

// renderFullscreenViewer composes the viewer-takes-everything layout.
// Stack:
//
//	row 0..titleH-1   title bar (kept — session tabs are still useful)
//	rows in between   viewer panel filling all remaining width × height
//	last row          status bar (kept — keymap hints + copy toast)
//
// We keep the title and status bars because losing them would also lose
// session-cycling and the global keymap discoverability. The win is the
// horizontal cells freed by hiding the explorer + servers columns —
// clyde lives in a side pane next to claude, so a typical setup might
// give clyde 60-80 columns total. Eating 35 of those for the explorer
// while reading a file is the difference between staying in the terminal
// and reaching for a separate vscode window.
func (m Model) renderFullscreenViewer() string {
	titleBar := renderTitleBar(m.styles, m.palette, m.data, m.frame, m.width, m.demoMode, m.liveView, m.liveView.LastUpdate)
	statusBar := renderStatusBar(m.styles, m.width, m.isActiveMode(), m.copyToast, m.data.Sessions, m.helpOpen, m.version)

	titleLines := strings.Count(titleBar, "\n") + 1
	statusLines := strings.Count(statusBar, "\n") + 1
	viewerH := m.height - titleLines - statusLines
	if viewerH < 4 {
		viewerH = 4
	}

	viewer := m.renderViewerPanel(m.width, viewerH)
	return titleBar + "\n" + viewer + "\n" + statusBar
}
