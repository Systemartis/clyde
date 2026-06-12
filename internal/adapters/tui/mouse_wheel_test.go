package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// findPanelBounds returns the bounding box for pid in m's current layout.
func findPanelBounds(t *testing.T, m Model, pid PanelID) panelBounds {
	t.Helper()
	for _, b := range m.buildPanelBounds() {
		if b.pid == pid {
			return b
		}
	}
	t.Fatalf("panel %d not found in bounds", pid)
	return panelBounds{}
}

// TestWheel_PassiveScrollablePanelEntersActiveAndScrolls is the regression
// test for "scroll blocking": in passive mode the wheel used to be forwarded
// to the panel's viewport, but passive rendering never displays that viewport
// — the wheel scrolled an invisible buffer and the screen never moved. The
// wheel must instead promote the panel under the cursor into Expanded-Active
// mode (the designed scroll mode, same destination as double-click) and apply
// the scroll there, exactly like the explorer's wheel already steals focus.
func TestWheel_PassiveScrollablePanelEntersActiveAndScrolls(t *testing.T) {
	t.Parallel()
	m := NewModelWithConfig(DefaultConfig(), LayoutStack)
	m.width = 90
	m.height = 50
	m.bp = BreakpointMedium
	m.layoutMode = LayoutStack
	m.focused = PanelNow

	if m.isActiveMode() {
		t.Fatal("precondition: model must start in passive mode")
	}

	b := findPanelBounds(t, m, PanelCalls)
	m2, _ := m.handleMouseWheel(tea.MouseWheelMsg{
		X:      b.xMin + 2,
		Y:      b.yMin + 1,
		Button: tea.MouseWheelDown,
	})

	if !m2.isActiveMode() || m2.activePanelID != PanelCalls {
		t.Fatalf("wheel over passive calls panel: activePanelID = %d (active=%v), want PanelCalls active",
			m2.activePanelID, m2.isActiveMode())
	}
	if m2.focused != PanelCalls {
		t.Errorf("wheel over passive calls panel: focused = %d, want PanelCalls", m2.focused)
	}
	if got := m2.panelVPs[PanelCalls].YOffset(); got <= 0 {
		t.Errorf("wheel-down must scroll the promoted panel's viewport; YOffset = %d, want > 0", got)
	}
}

// TestWheel_ActivePanelStillScrolls guards the existing active-mode path:
// wheel over the already-active panel keeps scrolling its viewport.
func TestWheel_ActivePanelStillScrolls(t *testing.T) {
	t.Parallel()
	m := NewModelWithConfig(DefaultConfig(), LayoutStack)
	m.width = 90
	m.height = 50
	m.bp = BreakpointMedium
	m.layoutMode = LayoutStack
	m.focused = PanelCalls
	m = m.transitionToActive()

	b := findPanelBounds(t, m, PanelCalls)
	m2, _ := m.handleMouseWheel(tea.MouseWheelMsg{
		X:      b.xMin + 2,
		Y:      b.yMin + 1,
		Button: tea.MouseWheelDown,
	})

	if got := m2.panelVPs[PanelCalls].YOffset(); got <= 0 {
		t.Errorf("wheel-down over active panel: YOffset = %d, want > 0", got)
	}
}

// TestWheel_BashPanelScrollable verifies the wheel routes to the bash panel
// too — it was missing from the wheel switch even though it has a viewport
// and keyboard scrolling works.
func TestWheel_BashPanelScrollable(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	cfg.Panels.Bash.Enabled = true
	m := NewModelWithConfig(cfg, LayoutStack)
	m.width = 90
	m.height = 60
	m.bp = BreakpointMedium
	m.layoutMode = LayoutStack
	m.focused = PanelNow

	b := findPanelBounds(t, m, PanelBash)
	m2, _ := m.handleMouseWheel(tea.MouseWheelMsg{
		X:      b.xMin + 2,
		Y:      b.yMin + 1,
		Button: tea.MouseWheelDown,
	})

	if !m2.isActiveMode() || m2.activePanelID != PanelBash {
		t.Fatalf("wheel over bash panel: activePanelID = %d (active=%v), want PanelBash active",
			m2.activePanelID, m2.isActiveMode())
	}
}

// TestApplyLiveView_ResyncsActiveViewport verifies that a live snapshot
// refresh rebuilds the active panel's viewport content. Without the resync
// the panel freezes at whatever content was synced when active mode was
// entered — the activity stream visibly stops while the user scrolls.
func TestApplyLiveView_ResyncsActiveViewport(t *testing.T) {
	t.Parallel()
	m := NewModelWithConfig(DefaultConfig(), LayoutStack)
	m.width = 90
	m.height = 50
	m.bp = BreakpointMedium
	m.layoutMode = LayoutStack
	m.focused = PanelCalls
	m = m.transitionToActive()

	m.panelVPs[PanelCalls].SetContent("stale-marker-line\nstale-marker-line\nstale-marker-line")
	if !strings.Contains(m.panelVPs[PanelCalls].View(), "stale-marker") {
		t.Fatal("precondition: stale content must be visible in the viewport")
	}

	m = m.applyLiveView()

	if strings.Contains(m.panelVPs[PanelCalls].View(), "stale-marker") {
		t.Error("applyLiveView must resync the active panel's viewport — stale content still visible")
	}
}
