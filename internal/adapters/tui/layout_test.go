package tui

import (
	"strings"
	"testing"
)

// TestDetectBreakpoint verifies the breakpoint thresholds match the v7 design spec.
//
//	< 80   → Narrow (single-column stack, 4 core panels)
//	80-159 → Medium (two-column: explorer+servers left, now/tasks/diff/usage right)
//	160+   → Wide   (three-column dashboard)
func TestDetectBreakpoint(t *testing.T) {
	cases := []struct {
		width int
		want  Breakpoint
	}{
		{0, BreakpointNarrow},
		{70, BreakpointNarrow},
		{79, BreakpointNarrow},
		{80, BreakpointMedium},
		{90, BreakpointMedium},
		{130, BreakpointMedium},
		{159, BreakpointMedium},
		{160, BreakpointWide},
		{180, BreakpointWide},
		{220, BreakpointWide},
	}
	for _, tc := range cases {
		got := DetectBreakpoint(tc.width)
		if got != tc.want {
			t.Errorf("DetectBreakpoint(%d) = %d, want %d", tc.width, got, tc.want)
		}
	}
}

// TestComposeColumnsJoinsContent verifies that ComposeColumns returns a
// non-empty string when given non-empty columns.
func TestComposeColumnsJoinsContent(t *testing.T) {
	cols := []Column{
		{Content: "AA\nBB", Width: 4},
		{Content: "CC\nDD", Width: 4},
	}
	got := ComposeColumns(2, cols...)
	if got == "" {
		t.Error("ComposeColumns returned empty string for non-empty columns")
	}
}

// TestComposeColumnsEmpty verifies graceful handling of zero columns.
func TestComposeColumnsEmpty(t *testing.T) {
	got := ComposeColumns(10)
	if got != "" {
		t.Errorf("ComposeColumns() with no columns should return empty string, got %q", got)
	}
}

// TestAutoSwitchThreshold verifies that narrow widths force Mode A (stack)
// regardless of the preferred layout mode.
func TestAutoSwitchThreshold(t *testing.T) {
	cases := []struct {
		preferred LayoutMode
		width     int
		want      LayoutMode
	}{
		{LayoutTabs, 70, LayoutStack},         // below threshold → force stack
		{LayoutMultiCol, 70, LayoutStack},     // below threshold → force stack
		{LayoutTabs, 80, LayoutTabs},          // at threshold → use preferred
		{LayoutMultiCol, 80, LayoutTabs},      // multi-col below 160 → tabs
		{LayoutMultiCol, 130, LayoutTabs},     // multi-col still below 160 → tabs
		{LayoutMultiCol, 160, LayoutMultiCol}, // multi-col at 160 → allowed
		{LayoutStack, 200, LayoutStack},       // user chose stack, wide terminal → respect choice
	}
	for _, tc := range cases {
		got := ResolveMode(tc.preferred, 80, tc.width)
		if got != tc.want {
			t.Errorf("ResolveMode(%q, 80, %d) = %q, want %q",
				tc.preferred, tc.width, got, tc.want)
		}
	}
}

// TestLayoutBoundsMatchesRendered is the drift-catcher described in
// layout.go. For every (width, height, layoutMode) configuration it
// asserts that the renderer (View()) and the click handler
// (buildPanelBounds) agree on every panel's vertical position. Catches
// the class of bug that used to manifest as cursor-vs-click offset,
// tree-row off-by-one, and bash/cache panels being unclickable.
//
// The test is intentionally generous about what counts as "label
// visible" — it looks for the panel's label substring on the row
// where the bounds say the top border lives. That keeps the test
// robust to cosmetic tweaks (border style, badge text, focus color)
// while still catching every off-by-one drift between renderer and
// click handler.
func TestLayoutBoundsMatchesRendered(t *testing.T) {
	type config struct {
		name   string
		width  int
		height int
		mode   LayoutMode
	}
	cases := []config{
		{"medium-2col-130x40", 130, 40, LayoutStack},
		{"medium-2col-110x40", 110, 40, LayoutStack},
		{"wide-multicol-180x50", 180, 50, LayoutMultiCol},
		{"wide-stack-180x50", 180, 50, LayoutStack},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := NewModel()
			m.width = c.width
			m.height = c.height
			m.bp = DetectBreakpoint(c.width)
			m.layoutMode = c.mode

			view := m.View()
			lines := strings.Split(view.Content, "\n")

			bounds := m.buildPanelBounds()
			boundSet := make(map[PanelID]bool)
			for _, b := range bounds {
				boundSet[b.pid] = true
			}

			// (1) Each panel's bounds.yMin row must contain that
			// panel's label in the rendered view. This catches +1/-1
			// drift between the renderer and the click handler.
			for _, b := range bounds {
				if b.yMin < 0 || b.yMin >= len(lines) {
					t.Errorf("panel %s bounds yMin=%d out of view range [0,%d)", driftLabel(b.pid), b.yMin, len(lines))
					continue
				}
				row := lines[b.yMin]
				if !rowContainsPanelLabel(row, b.pid) {
					t.Errorf("panel %s bounds yMin=%d (yMax=%d) does NOT match rendered top row.\nrow: %q",
						driftLabel(b.pid), b.yMin, b.yMax, row)
				}
			}

			// (2) Every panel that the renderer would paint must
			// have a bounds entry — otherwise clicks on it are
			// silently ignored (the bash/cache regression).
			for _, pid := range expectedPanelsForLayout(m) {
				if !boundSet[pid] {
					t.Errorf("panel %s is rendered but missing from buildPanelBounds — clicks on it would be ignored", driftLabel(pid))
				}
			}
		})
	}
}

// expectedPanelsForLayout returns the set of panels that should
// appear in bounds for the current model state. Mirrors the
// renderer's panel selection so the drift-catcher can verify nothing
// is accidentally dropped from the bounds builder.
func expectedPanelsForLayout(m Model) []PanelID {
	mode := m.effectiveMode()
	out := make([]PanelID, 0, 8)

	if mode == LayoutStack && m.bp == BreakpointMedium {
		if m.panelEnabled(PanelExplorer) {
			out = append(out, PanelExplorer)
		}
		if m.panelEnabled(PanelServers) {
			out = append(out, PanelServers)
		}
		for _, pid := range twoColRightPanelOrder() {
			if m.panelEnabled(pid) {
				out = append(out, pid)
			}
		}
		return out
	}

	if mode == LayoutMultiCol {
		if m.panelEnabled(PanelExplorer) {
			out = append(out, PanelExplorer)
		}
		out = append(out, PanelNow) // always present, non-selectable but bounded
		if m.panelEnabled(PanelCalls) {
			out = append(out, PanelCalls)
		}
		if m.panelEnabled(PanelDiff) {
			out = append(out, PanelDiff)
		}
		for _, pid := range multiColRightPanelOrder() {
			if m.panelEnabled(pid) {
				out = append(out, pid)
			}
		}
		return out
	}

	// Stack (narrow + wide): activePanelsForBreakpoint is the source.
	return m.activePanelsForBreakpoint()
}

// driftLabel returns the short label for a panel — used in failure
// messages. Different name from panel-internal label functions to
// avoid collisions.
func driftLabel(pid PanelID) string {
	switch pid {
	case PanelNow:
		return "now"
	case PanelCalls:
		return "activity"
	case PanelDiff:
		return "diff"
	case PanelUsage:
		return "usage"
	case PanelExplorer:
		return "explorer"
	case PanelServers:
		return "servers"
	case PanelBash:
		return "bash"
	case PanelCache:
		return "cache"
	}
	return "?"
}

// rowContainsPanelLabel checks whether a rendered row contains the
// panel's label. Accepts both the expanded-border style
// (`╭ label ─...─╮`) and the collapsed-summary style
// (`╭ ▸ label: summary ─...─╮`).
func rowContainsPanelLabel(row string, pid PanelID) bool {
	label := driftLabel(pid)
	return strings.Contains(row, " "+label+" ") || strings.Contains(row, " "+label+":")
}
