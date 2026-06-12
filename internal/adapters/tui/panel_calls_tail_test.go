package tui

import (
	"fmt"
	"strings"
	"testing"
)

// soloMainData builds MockData with a single main agent group carrying n
// distinctly-labeled calls (call-001 … call-NNN, oldest first).
func soloMainData(n int) MockData {
	d := V3MockData()
	grp := AgentGroup{AgentID: "main", AgentName: "main session", Active: true}
	for i := 1; i <= n; i++ {
		grp.Calls = append(grp.Calls, ToolCall{
			Tool:   "Read",
			KeyArg: fmt.Sprintf("call-%03d", i),
			State:  CallDone,
		})
	}
	d.AgentGroups = []AgentGroup{grp}
	return d
}

// TestActivitySoloMain_TailWindowed is the regression test for "activity
// panel never follows the live tail": the solo-main card rendered every
// call oldest-first and wrapPanel clips from the BOTTOM, so any session
// past ~a panelful of calls showed only stale early-session calls forever.
// The passive card must window to the NEWEST calls that fit, with an
// "earlier" marker for the rest.
func TestActivitySoloMain_TailWindowed(t *testing.T) {
	t.Parallel()
	s := NewStyles(TokyoNightPalette())
	p := TokyoNightPalette()
	d := soloMainData(30)

	out := stripANSI(buildCallsContent(s, p, d, 60, 12))

	if !strings.Contains(out, "call-030") {
		t.Errorf("tail-windowed card must show the NEWEST call (call-030), got:\n%s", out)
	}
	if strings.Contains(out, "call-001") {
		t.Errorf("tail-windowed card must not show the oldest call (call-001) in a 12-row panel, got:\n%s", out)
	}
	if !strings.Contains(out, "earlier") {
		t.Errorf("truncated card must show an 'earlier' marker, got:\n%s", out)
	}
	// Content must respect the row budget (wrapPanel would clip the tail
	// we just fought to show).
	if lines := strings.Count(out, "\n") + 1; lines > 12 {
		t.Errorf("solo-main card produced %d lines for innerH=12 — overflow gets bottom-clipped", lines)
	}
}

// TestActivitySoloMain_NoMarkerWhenFits verifies short sessions render all
// calls with no "earlier" marker.
func TestActivitySoloMain_NoMarkerWhenFits(t *testing.T) {
	t.Parallel()
	s := NewStyles(TokyoNightPalette())
	p := TokyoNightPalette()
	d := soloMainData(4)

	out := stripANSI(buildCallsContent(s, p, d, 60, 20))

	for i := 1; i <= 4; i++ {
		want := fmt.Sprintf("call-%03d", i)
		if !strings.Contains(out, want) {
			t.Errorf("short session must show every call; %s missing:\n%s", want, out)
		}
	}
	if strings.Contains(out, "earlier") {
		t.Errorf("short session must not show an 'earlier' marker:\n%s", out)
	}
}

// TestActivityViewportContent_Unwindowed verifies active mode still gets the
// FULL history (that's what scrolling is for).
func TestActivityViewportContent_Unwindowed(t *testing.T) {
	t.Parallel()
	s := NewStyles(TokyoNightPalette())
	p := TokyoNightPalette()
	d := soloMainData(30)

	content := stripANSI(buildCallsViewportContent(s, p, d, 60))
	if !strings.Contains(content, "call-001") || !strings.Contains(content, "call-030") {
		t.Error("active-mode viewport content must contain the full call history")
	}
}

// TestActivityActiveMode_StartsAtTail verifies entering active mode on the
// calls panel lands at the BOTTOM of the history (newest calls), not the top.
func TestActivityActiveMode_StartsAtTail(t *testing.T) {
	t.Parallel()
	m := NewModelWithConfig(DefaultConfig(), LayoutStack)
	m.width = 90
	m.height = 50
	m.bp = BreakpointMedium
	m.layoutMode = LayoutStack
	m.data = soloMainData(60)
	m.focused = PanelCalls

	m = m.transitionToActive()

	if !m.panelVPs[PanelCalls].AtBottom() {
		t.Errorf("activity active mode must start at the tail; YOffset = %d of %d lines",
			m.panelVPs[PanelCalls].YOffset(), m.panelVPs[PanelCalls].TotalLineCount())
	}
}

// TestActivityResync_SticksToTail verifies a live refresh keeps the view
// pinned to the tail when the user was already there, but does NOT yank a
// user who scrolled up to read history.
func TestActivityResync_SticksToTail(t *testing.T) {
	t.Parallel()
	m := NewModelWithConfig(DefaultConfig(), LayoutStack)
	m.width = 90
	m.height = 50
	m.bp = BreakpointMedium
	m.layoutMode = LayoutStack
	m.data = soloMainData(60)
	m.focused = PanelCalls
	m = m.transitionToActive() // at tail

	// New calls arrive; resync. View must follow the tail.
	m.data = soloMainData(70)
	m = m.syncPanelViewport(PanelCalls)
	if !m.panelVPs[PanelCalls].AtBottom() {
		t.Error("resync while at tail must stick to the tail")
	}

	// User scrolls up to read history; resync must NOT yank to bottom.
	m.panelVPs[PanelCalls].ScrollUp(5)
	before := m.panelVPs[PanelCalls].YOffset()
	m.data = soloMainData(80)
	m = m.syncPanelViewport(PanelCalls)
	if got := m.panelVPs[PanelCalls].YOffset(); got != before {
		t.Errorf("resync while scrolled up must keep YOffset %d, got %d", before, got)
	}
}
