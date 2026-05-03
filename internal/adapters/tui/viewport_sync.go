package tui

import (
	"fmt"

	"charm.land/bubbles/v2/viewport"
)

// panelRenderWidth returns the allocated outer width for a panel in the current layout.
// For 2-col medium, right-column panels use rightW; left-column panels use leftW.
// For multi-col, panel widths are fixed. For narrow stack, uses full width.
func (m Model) panelRenderWidth(pid PanelID) int {
	mode := m.effectiveMode()
	switch mode {
	case LayoutStack:
		return m.panelWidth2Col(pid)
	case LayoutMultiCol:
		return m.panelWidthMultiCol(pid)
	default:
		return m.width
	}
}

// panelWidth2Col returns the column width for pid in stack (possibly 2-col) mode.
func (m Model) panelWidth2Col(pid PanelID) int {
	if m.bp != BreakpointMedium {
		return m.width
	}
	leftW := (m.width * 40) / 100
	if leftW < 22 {
		leftW = 22
	}
	if leftW > 50 {
		leftW = 50
	}
	for _, p := range m.twoColLeftPanels() {
		if p == pid {
			return leftW
		}
	}
	return m.width - leftW
}

// panelWidthMultiCol returns the column width for pid in 3-column multi-col mode.
func (m Model) panelWidthMultiCol(pid PanelID) int {
	const explorerW = 22
	const baseMiddleW = 36
	rightW := m.width - explorerW - baseMiddleW
	middleW := baseMiddleW
	if rightW > 42 {
		middleW += rightW - 42
		rightW = 42
	}
	if rightW < 18 {
		rightW = 18
	}
	switch pid {
	case PanelExplorer:
		return explorerW
	case PanelNow, PanelCalls, PanelDiff:
		return middleW
	default: // PanelUsage, PanelServers
		return rightW
	}
}

// syncPanelViewport pushes the current rendered content for a scrollable panel
// into its viewport so ↑/↓ in active mode actually scrolls visible content.
// Only calls, diff, and usage panels have scrollable viewports; others are no-ops.
//
// IMPORTANT: SetWidth must be called BEFORE SetContent because the bubbles/v2
// viewport reformats (pads/clips) content lines relative to its Width at the
// moment SetContent is called. Calling SetWidth after SetContent (or in the
// render function on a local copy) results in stale-width content that clips
// incorrectly when the render-time viewport width differs.
func (m Model) syncPanelViewport(pid PanelID) Model {
	panelW := m.panelRenderWidth(pid)
	// inner = panel outer width - 2 border - 2 padding chars (1 left + 1 right)
	inner := panelW - 4
	if inner < 4 {
		inner = 4
	}
	// Set the viewport width FIRST so SetContent stores lines at the correct width.
	m.panelVPs[pid].SetWidth(inner)
	switch pid {
	case PanelCalls: // calls panel uses the PanelCalls slot
		content := buildCallsViewportContent(m.styles, m.palette, m.data, inner)
		m.panelVPs[pid].SetContent(content)
	case PanelDiff:
		content := buildDiffViewportContent(m.styles, m.data)
		m.panelVPs[pid].SetContent(content)
	case PanelUsage:
		content := buildUsageViewportContent(m.styles, m.palette, m.data, m.progTokens, m.progReset, inner)
		m.panelVPs[pid].SetContent(content)
	case PanelExplorer:
		content := buildExplorerViewportContent(m.styles, m.palette, m.data, m.explorer, inner)
		m.panelVPs[pid].SetContent(content)
	case PanelServers:
		content := buildServersViewportContent(m.styles, m.data, inner)
		m.panelVPs[pid].SetContent(content)
	case PanelBash:
		content := buildBashViewportContent(m.styles, m.data, inner)
		m.panelVPs[pid].SetContent(content)
	case PanelCache:
		content := buildCacheViewportContent(m.styles, m.palette, m.data, inner)
		m.panelVPs[pid].SetContent(content)
	}
	return m
}

// ViewerViewport wraps the bubbles/v2/viewport model for the file viewer.
// xOffset is the horizontal scroll position in characters (0 = leftmost
// column). The bubbles viewport does not natively support horizontal scroll,
// so the rendering layer applies it manually before styling each line.
type ViewerViewport struct {
	vp      viewport.Model
	xOffset int
}

// NewViewerViewport constructs an initial ViewerViewport.
// SoftWrap is OFF: code reads better truncated than wrapped at narrow widths,
// and predictable one-source-line-per-row scrolling keeps the scrollbar honest.
func NewViewerViewport() ViewerViewport {
	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(30))
	vp.SoftWrap = false
	vp.MouseWheelEnabled = true
	return ViewerViewport{vp: vp}
}

// LoadFile loads content for the given file path.
// In demo mode it uses the mock content table; in live mode it reads from disk.
// cwd is used to resolve relative paths in live mode.
func (v *ViewerViewport) LoadFile(path, cwd string, demoMode bool) {
	var content string
	if demoMode {
		content = MockFileContent(path, MockData{})
	} else {
		raw, err := readFileForViewer(path, cwd)
		if err != nil {
			content = fmt.Sprintf("  error reading file: %s", err)
		} else {
			content = raw
		}
	}
	v.vp.SetContent(content)
	v.vp.SetYOffset(0)
	v.xOffset = 0
}
