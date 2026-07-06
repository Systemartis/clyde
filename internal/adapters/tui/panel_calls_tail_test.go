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

// TestActivitySoloMain_HeadWindowed verifies the activity panel renders
// newest-first: the latest call sits at the TOP of the card, older calls
// fall below it, and the overflow that doesn't fit is clipped at the bottom
// behind an "earlier" marker. This preserves the original guarantee that the
// newest activity is always visible — it just anchors it to the head so a
// glance lands on what claude is doing right now.
func TestActivitySoloMain_HeadWindowed(t *testing.T) {
	t.Parallel()
	s := NewStyles(TokyoNightPalette())
	p := TokyoNightPalette()
	d := soloMainData(30)

	out := stripANSI(buildCallsContent(s, p, d, 60, 12))

	if !strings.Contains(out, "call-030") {
		t.Errorf("head-windowed card must show the NEWEST call (call-030), got:\n%s", out)
	}
	if strings.Contains(out, "call-001") {
		t.Errorf("head-windowed card must clip the oldest call (call-001) in a 12-row panel, got:\n%s", out)
	}
	if !strings.Contains(out, "earlier") {
		t.Errorf("truncated card must show an 'earlier' marker, got:\n%s", out)
	}
	// Newest-first: the latest call must render ABOVE the next-latest.
	if i030, i029 := strings.Index(out, "call-030"), strings.Index(out, "call-029"); i030 < 0 || i029 < 0 || i030 > i029 {
		t.Errorf("newest call must appear above older calls: call-030 at %d, call-029 at %d\n%s", i030, i029, out)
	}
	// Content must respect the row budget (wrapPanel would clip whatever
	// overflows the bottom — which must NOT be the newest calls).
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
	// Even when everything fits, order is newest-first.
	if i4, i1 := strings.Index(out, "call-004"), strings.Index(out, "call-001"); i4 < 0 || i1 < 0 || i4 > i1 {
		t.Errorf("newest call must render above oldest: call-004 at %d, call-001 at %d\n%s", i4, i1, out)
	}
}

// TestActivityHistogramFitsWidth guards against horizontal overflow. The
// tools-histogram footer ("Bash 3 · Read 2 · …") is the one card line that
// was not width-bounded to the panel, so a session touching many distinct
// tools produced a line wider than every other row — stretching the
// container's content width and letting a stray touchpad swipe scroll the
// content off-screen. Every rendered line must fit within inner.
func TestActivityHistogramFitsWidth(t *testing.T) {
	t.Parallel()
	s := NewStyles(TokyoNightPalette())
	p := TokyoNightPalette()
	d := V3MockData()
	grp := AgentGroup{AgentID: "main", AgentName: "main", Active: true}
	for _, tool := range []string{
		"Bash", "Read", "Edit", "Grep", "Write", "MultiEdit",
		"Glob", "WebFetch", "Task", "mcp__github", "mcp__playwright", "NotebookEdit",
	} {
		grp.Calls = append(grp.Calls, ToolCall{Tool: tool, KeyArg: "x", State: CallDone})
	}
	d.AgentGroups = []AgentGroup{grp}

	const inner = 48
	out := buildCallsContent(s, p, d, inner, 40)
	for i, line := range strings.Split(out, "\n") {
		if w := ansiWidth(line); w > inner {
			t.Errorf("activity line %d exceeds inner width %d (got %d): %q", i, inner, w, stripANSI(line))
		}
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
	// Newest-first ordering holds in the scrollable content too.
	if i30, i1 := strings.Index(content, "call-030"), strings.Index(content, "call-001"); i30 > i1 {
		t.Errorf("viewport must be newest-first: call-030 at %d, call-001 at %d", i30, i1)
	}
}

// TestActivityActiveMode_StartsAtHead verifies entering active mode on the
// calls panel lands at the TOP of the history (newest calls), so the user
// scrolls DOWN to reach older activity.
func TestActivityActiveMode_StartsAtHead(t *testing.T) {
	t.Parallel()
	m := NewModelWithConfig(DefaultConfig(), LayoutStack)
	m.width = 90
	m.height = 50
	m.bp = BreakpointMedium
	m.layoutMode = LayoutStack
	m.data = soloMainData(60)
	m.focused = PanelCalls

	m = m.transitionToActive()

	if !m.panelVPs[PanelCalls].AtTop() {
		t.Errorf("activity active mode must start at the head; YOffset = %d of %d lines",
			m.panelVPs[PanelCalls].YOffset(), m.panelVPs[PanelCalls].TotalLineCount())
	}
}

// TestActivityResync_SticksToHead verifies a live refresh keeps the view
// pinned to the head (newest) when the user was already there, but does NOT
// yank a user who scrolled down to read history.
func TestActivityResync_SticksToHead(t *testing.T) {
	t.Parallel()
	m := NewModelWithConfig(DefaultConfig(), LayoutStack)
	m.width = 90
	m.height = 50
	m.bp = BreakpointMedium
	m.layoutMode = LayoutStack
	m.data = soloMainData(60)
	m.focused = PanelCalls
	m = m.transitionToActive() // at head

	// New calls arrive; resync. View must stay at the head (newest).
	m.data = soloMainData(70)
	m = m.syncPanelViewport(PanelCalls)
	if !m.panelVPs[PanelCalls].AtTop() {
		t.Error("resync while at head must stick to the head")
	}

	// User scrolls down to read history; resync must NOT yank to the head.
	m.panelVPs[PanelCalls].ScrollDown(5)
	before := m.panelVPs[PanelCalls].YOffset()
	if before == 0 {
		t.Fatal("precondition: user should be scrolled away from the head")
	}
	m.data = soloMainData(80)
	m = m.syncPanelViewport(PanelCalls)
	if got := m.panelVPs[PanelCalls].YOffset(); got != before {
		t.Errorf("resync while scrolled down must keep YOffset %d, got %d", before, got)
	}
}
