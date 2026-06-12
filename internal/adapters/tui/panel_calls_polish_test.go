package tui

import (
	"strings"
	"testing"
)

// TestActivityEmptyState_ShowsHint verifies the activity panel renders a
// faded hint instead of a featureless blank box when no agent activity
// exists yet — the first thing a live-mode user sees on a fresh session.
// Matches the bash ("no Bash commands recorded yet") and cache ("no turns
// observed yet") empty-state house style.
func TestActivityEmptyState_ShowsHint(t *testing.T) {
	t.Parallel()
	s := NewStyles(TokyoNightPalette())
	p := TokyoNightPalette()
	d := V3MockData()
	d.AgentGroups = nil

	out := stripANSI(renderCalls(s, p, d, 70, 20, false))

	if !strings.Contains(out, "no tool calls yet") {
		t.Errorf("empty activity panel must show the 'no tool calls yet' hint, got:\n%s", out)
	}
	if strings.Contains(out, "0 agents active · 0 calls") {
		t.Errorf("empty activity panel meta should read 'idle', not the noisy zero-count, got:\n%s", out)
	}
	if !strings.Contains(out, "idle") {
		t.Errorf("empty activity panel meta should read 'idle', got:\n%s", out)
	}
}

// TestActivityEmptyState_ViewportContent verifies the active-mode viewport
// content builder shows the same hint (so double-clicking an empty panel
// doesn't flip it back to a blank buffer).
func TestActivityEmptyState_ViewportContent(t *testing.T) {
	t.Parallel()
	s := NewStyles(TokyoNightPalette())
	p := TokyoNightPalette()
	d := V3MockData()
	d.AgentGroups = nil

	content := stripANSI(buildCallsViewportContent(s, p, d, 60))
	if !strings.Contains(content, "no tool calls yet") {
		t.Errorf("empty activity viewport content must show the hint, got: %q", content)
	}
}

// TestSyncPanelViewport_HeightMatchesPanel verifies syncPanelViewport sizes
// the viewport to the panel's rendered inner height. Without it the
// viewport keeps its constructor default (10 rows) and scroll clamping is
// wrong: content that fits the panel can still be scrolled up into blank
// space because maxYOffset is computed against the stale height.
func TestSyncPanelViewport_HeightMatchesPanel(t *testing.T) {
	t.Parallel()
	m := NewModelWithConfig(DefaultConfig(), LayoutStack)
	m.width = 90
	m.height = 50
	m.bp = BreakpointMedium
	m.layoutMode = LayoutStack
	m.focused = PanelCalls

	m = m.transitionToActive()

	// Settle the expand animation so the panel bounds reflect the final
	// height the viewport must have been synced against.
	for i := 0; i < 500; i++ {
		m.collapse[PanelCalls].Advance()
	}

	b := findPanelBounds(t, m, PanelCalls)
	wantH := (b.yMax - b.yMin + 1) - 2 // panel height minus top+bottom border
	if wantH <= 2 {
		t.Fatalf("precondition: settled calls panel too small (inner %d)", wantH)
	}
	if got := m.panelVPs[PanelCalls].Height(); got != wantH {
		t.Errorf("synced viewport height = %d, want %d (panel inner height)", got, wantH)
	}

	// Overscroll must clamp against the real visible height.
	m.panelVPs[PanelCalls].ScrollDown(1000)
	total := m.panelVPs[PanelCalls].TotalLineCount()
	maxOff := total - wantH
	if maxOff < 0 {
		maxOff = 0
	}
	if got := m.panelVPs[PanelCalls].YOffset(); got != maxOff {
		t.Errorf("overscroll YOffset = %d, want clamp at %d (total %d lines, height %d)",
			got, maxOff, total, wantH)
	}
}
