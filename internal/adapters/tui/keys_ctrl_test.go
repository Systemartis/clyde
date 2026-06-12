package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// newCtrlTestModel builds a passive wide-stack model with the boot splash
// already dismissed so ctrl-chords reach the global handler.
func newCtrlTestModel() Model {
	m := NewModelWithConfig(DefaultConfig(), LayoutStack)
	m.width = 180
	m.height = 50
	m.bp = BreakpointWide
	m.layoutMode = LayoutStack
	m.boot = m.boot.Dismiss()
	return m
}

// TestCtrlL_CyclesLayoutMode verifies the status bar's advertised "⌃l mode"
// hotkey actually cycles stack → tabs → multi-col. The binding existed in
// the keymap (and FullHelp) but was never dispatched.
func TestCtrlL_CyclesLayoutMode(t *testing.T) {
	t.Parallel()
	m := newCtrlTestModel()

	next, _ := m.Update(tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl})
	m = next.(Model)
	if m.layoutMode != LayoutTabs {
		t.Fatalf("after first ⌃l: layoutMode = %q, want %q", m.layoutMode, LayoutTabs)
	}

	next, _ = m.Update(tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl})
	m = next.(Model)
	if m.layoutMode != LayoutMultiCol {
		t.Fatalf("after second ⌃l at 180 cols: layoutMode = %q, want %q", m.layoutMode, LayoutMultiCol)
	}

	next, _ = m.Update(tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl})
	m = next.(Model)
	if m.layoutMode != LayoutStack {
		t.Fatalf("after third ⌃l: layoutMode = %q, want %q (wraps around)", m.layoutMode, LayoutStack)
	}
}

// TestCtrlJumps_FocusPanels verifies the FullHelp-advertised focus chords:
// ⌃e explorer · ⌃a calls · ⌃d diff.
func TestCtrlJumps_FocusPanels(t *testing.T) {
	t.Parallel()
	cases := []struct {
		code rune
		want PanelID
	}{
		{'e', PanelExplorer},
		{'a', PanelCalls},
		{'d', PanelDiff},
	}
	for _, tc := range cases {
		cfg := DefaultConfig()
		cfg.Panels.Diff.Enabled = true // ⌃d target must exist
		m := NewModelWithConfig(cfg, LayoutStack)
		m.width = 180
		m.height = 50
		m.bp = BreakpointWide
		m.boot = m.boot.Dismiss()
		m.focused = PanelNow

		next, _ := m.Update(tea.KeyPressMsg{Code: tc.code, Mod: tea.ModCtrl})
		m = next.(Model)
		if m.focused != tc.want {
			t.Errorf("⌃%c: focused = %d, want %d", tc.code, m.focused, tc.want)
		}
	}
}

// TestCtrl0_CollapsesOthers verifies "⌃0 collapse others": every collapsible
// panel except the focused one collapses; the focused panel stays expanded.
func TestCtrl0_CollapsesOthers(t *testing.T) {
	t.Parallel()
	m := newCtrlTestModel()
	m.focused = PanelCalls
	m.collapse[PanelCalls].Expand()
	m.collapse[PanelUsage].Expand()
	m.collapse[PanelExplorer].Expand()

	next, _ := m.Update(tea.KeyPressMsg{Code: '0', Mod: tea.ModCtrl})
	m = next.(Model)

	if m.collapse[PanelCalls].IsCollapsed() {
		t.Error("⌃0 must keep the focused panel expanded")
	}
	if !m.collapse[PanelUsage].IsCollapsed() {
		t.Error("⌃0 must collapse the usage panel (not focused)")
	}
	if !m.collapse[PanelExplorer].IsCollapsed() {
		t.Error("⌃0 must collapse the explorer panel (not focused)")
	}
}

// TestEnter_EntersActiveModeInMultiCol is the keyboard half of the "scroll
// blocking" fix: handleEnter used to no-op outside stack mode, so in
// multi-col (where panels are pinned expanded) there was NO keyboard path
// into Expanded-Active — and therefore no way to scroll a panel.
func TestEnter_EntersActiveModeInMultiCol(t *testing.T) {
	t.Parallel()
	m := newCtrlTestModel()
	m.layoutMode = LayoutMultiCol
	m.focused = PanelCalls

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)

	if !m.isActiveMode() || m.activePanelID != PanelCalls {
		t.Fatalf("Enter in multi-col: activePanelID = %d (active=%v), want PanelCalls active",
			m.activePanelID, m.isActiveMode())
	}
}
