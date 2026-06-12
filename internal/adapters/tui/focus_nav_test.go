package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// newNavTestModel builds a wide-stack passive model with the boot splash
// dismissed so navigation keys reach the handlers.
func newNavTestModel() Model {
	m := NewModelWithConfig(DefaultConfig(), LayoutStack)
	m.width = 180
	m.height = 50
	m.bp = BreakpointWide
	m.layoutMode = LayoutStack
	m.boot = m.boot.Dismiss()
	return m
}

// TestAdvanceFocus_CyclesAllInteractivePanels verifies Tab walks every
// enabled interactive panel exactly once before wrapping, and Shift+Tab
// returns to the start — the core focus-navigation contract.
func TestAdvanceFocus_CyclesAllInteractivePanels(t *testing.T) {
	t.Parallel()
	m := newNavTestModel()
	start := m.focused

	seen := map[PanelID]bool{start: true}
	cur := m
	for i := 0; i < int(panelCount)+2; i++ {
		cur = cur.advanceFocus(1)
		if cur.focused == start {
			break
		}
		if seen[cur.focused] {
			t.Fatalf("focus revisited panel %d before completing the cycle", cur.focused)
		}
		seen[cur.focused] = true
	}
	if cur.focused != start {
		t.Errorf("Tab cycle did not wrap back to the starting panel %d (ended on %d)", start, cur.focused)
	}
	if len(seen) < 3 {
		t.Errorf("focus cycle visited only %d panels — expected the full interactive set", len(seen))
	}

	// One step back returns to the previous panel.
	fwd := m.advanceFocus(1)
	back := fwd.advanceFocus(-1)
	if back.focused != m.focused {
		t.Errorf("Shift+Tab after Tab = panel %d, want %d", back.focused, m.focused)
	}
}

// TestAdvanceFocus_SkipsDisabledPanels verifies a disabled panel never
// receives focus.
func TestAdvanceFocus_SkipsDisabledPanels(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	cfg.Panels.Servers.Enabled = false
	m := NewModelWithConfig(cfg, LayoutStack)
	m.width = 180
	m.height = 50
	m.bp = BreakpointWide
	m.boot = m.boot.Dismiss()

	cur := m
	for i := 0; i < int(panelCount)+2; i++ {
		cur = cur.advanceFocus(1)
		if cur.focused == PanelServers {
			t.Fatal("focus landed on the disabled servers panel")
		}
		if cur.focused == m.focused {
			break
		}
	}
}

// TestSessionCycle_BracketKeysWalkTabs verifies session cycling order:
// Σ aggregate (-1) → session 0 → session 1 → back to Σ, and that
// selectSession rejects out-of-range indices.
func TestSessionCycle_BracketKeysWalkTabs(t *testing.T) {
	t.Parallel()
	m := newNavTestModel()
	m.data.Sessions = []SessionTab{
		{Label: "Σ", IsAggregate: true},
		{Label: "fix-bug", Live: true},
		{Label: "refactor", Live: true},
	}
	m.sessionTabIndex = -1 // Σ

	m = m.cycleSession(1)
	if m.sessionTabIndex != 0 {
		t.Fatalf("after first cycle: sessionTabIndex = %d, want 0", m.sessionTabIndex)
	}
	m = m.cycleSession(1)
	if m.sessionTabIndex != 1 {
		t.Fatalf("after second cycle: sessionTabIndex = %d, want 1", m.sessionTabIndex)
	}
	m = m.cycleSession(1)
	if m.sessionTabIndex != -1 {
		t.Fatalf("third cycle must wrap to Σ (-1), got %d", m.sessionTabIndex)
	}
	m = m.cycleSession(-1)
	if m.sessionTabIndex != 1 {
		t.Fatalf("reverse cycle from Σ must land on the last session (1), got %d", m.sessionTabIndex)
	}

	if got := m.selectSession(99); got.sessionTabIndex != m.sessionTabIndex {
		t.Error("selectSession(out-of-range) must be a no-op")
	}
	if got := m.selectSession(-1); got.sessionTabIndex != -1 {
		t.Error("selectSession(-1) must select the Σ aggregate")
	}
}

// TestSessionCycle_SingleSessionNoTabs verifies cycling is a no-op without a
// tab strip (fewer than 2 entries).
func TestSessionCycle_SingleSessionNoTabs(t *testing.T) {
	t.Parallel()
	m := newNavTestModel()
	m.data.Sessions = nil
	m.sessionTabIndex = -1
	if got := m.cycleSession(1); got.sessionTabIndex != -1 {
		t.Errorf("cycleSession without tabs must be a no-op, got %d", got.sessionTabIndex)
	}
}

// TestTabKey_AdvancesFocusThroughUpdate exercises the real key path (not
// just the helper): a Tab keypress through Update must move focus.
func TestTabKey_AdvancesFocusThroughUpdate(t *testing.T) {
	t.Parallel()
	m := newNavTestModel()
	start := m.focused

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = next.(Model)
	if m.focused == start {
		t.Error("Tab through Update did not advance focus")
	}
}
