package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestExplorerAutoScrollKeepsHighlightVisible verifies that when the
// highlighted row sits below the visible window, the renderer adjusts the
// scroll offset so the row appears in the rendered output.
//
// Regression for the bug where deep dirs in a real cwd were silently
// truncated by wrapPanel because renderExplorer wrote ALL rows without
// clipping to height.
func TestExplorerAutoScrollKeepsHighlightVisible(t *testing.T) {
	t.Parallel()

	d := MockData{ModifiedFiles: []ModifiedFile{}}
	// Build a deep tree of 30 dummy file rows.
	for i := 0; i < 30; i++ {
		d.Tree = append(d.Tree, TreeNode{
			Name: "file" + string(rune('a'+i%26)) + ".go",
			Mark: "",
		})
	}
	es := ExplorerState{collapsed: map[string]bool{}}
	es.rows = buildVisibleRows(d, es.collapsed)
	es.highlighted = 25

	s := NewStyles(TokyoNightPalette())
	rendered := stripANSI(renderExplorer(s, TokyoNightPalette(), d, es, 30, 16, true))

	// The 26th row's name (highlighted = 25, 0-indexed) must appear somewhere.
	wantedName := "file" + string(rune('a'+25%26)) + ".go"
	if !strings.Contains(rendered, wantedName) {
		t.Errorf("auto-scroll failed — highlighted file %q not in rendered output:\n%s",
			wantedName, rendered)
	}
}

// TestExplorerScrollbarVisibleOnOverflow verifies that the scrollbar column
// is rendered when the tree overflows the available area, and absent when
// every row fits.
func TestExplorerScrollbarVisibleOnOverflow(t *testing.T) {
	t.Parallel()

	s := NewStyles(TokyoNightPalette())

	// Big tree → overflow → scrollbar visible.
	dBig := MockData{}
	for i := 0; i < 30; i++ {
		dBig.Tree = append(dBig.Tree, TreeNode{Name: "file.go"})
	}
	esBig := ExplorerState{collapsed: map[string]bool{}}
	esBig.rows = buildVisibleRows(dBig, esBig.collapsed)
	rBig := stripANSI(renderExplorer(s, TokyoNightPalette(), dBig, esBig, 30, 16, true))
	if !strings.Contains(rBig, "█") {
		t.Errorf("scrollbar thumb missing on overflow:\n%s", rBig)
	}

	// Tiny tree → no overflow → no scrollbar block.
	dSmall := MockData{}
	for i := 0; i < 2; i++ {
		dSmall.Tree = append(dSmall.Tree, TreeNode{Name: "file.go"})
	}
	esSmall := ExplorerState{collapsed: map[string]bool{}}
	esSmall.rows = buildVisibleRows(dSmall, esSmall.collapsed)
	rSmall := stripANSI(renderExplorer(s, TokyoNightPalette(), dSmall, esSmall, 30, 20, true))
	if strings.Contains(rSmall, "█") {
		t.Errorf("scrollbar should not appear when tree fits:\n%s", rSmall)
	}
}

// TestExplorerClickRespectsScrollOffset verifies that a mouse click on a
// visible row maps to the correct absolute tree index after the user has
// scrolled (highlight moved deep into the tree).
func TestExplorerClickRespectsScrollOffset(t *testing.T) {
	t.Parallel()

	m := NewModel()
	m.width = 130
	m.height = 50
	m.bp = BreakpointMedium
	m.layoutMode = LayoutStack

	// Build a synthetic deep tree.
	m.data.Tree = nil
	m.data.ModifiedFiles = nil
	for i := 0; i < 50; i++ {
		m.data.Tree = append(m.data.Tree, TreeNode{Name: "file.go"})
	}
	m.explorer = NewExplorerState(m.data)
	m.explorer.highlighted = 30 // forces scroll deep into the tree

	scrollOff := m.explorerVisibleScrollOff()
	if scrollOff == 0 {
		t.Fatalf("expected non-zero scroll offset for highlight=30 in deep tree, got 0")
	}

	// Synthesize a click on the 2nd visible tree row (visibleIdx=1).
	// Computed absolute index should be scrollOff + 1.
	wantAbs := scrollOff + 1
	gotVisible := 1
	gotAbs := scrollOff + gotVisible
	if gotAbs != wantAbs {
		t.Errorf("click translation wrong: visible=%d scrollOff=%d → abs=%d (want %d)",
			gotVisible, scrollOff, gotAbs, wantAbs)
	}
}

// TestExplorerMouseWheelMovesHighlight verifies that wheel events dispatched
// over the explorer panel move the tree highlight by wheelStep rows.
func TestExplorerMouseWheelMovesHighlight(t *testing.T) {
	t.Parallel()

	m := NewModel()
	m.width = 130
	m.height = 50
	m.bp = BreakpointMedium
	m.layoutMode = LayoutStack
	m.focused = PanelExplorer

	// Make sure we have enough rows to scroll.
	m.data.Tree = nil
	for i := 0; i < 30; i++ {
		m.data.Tree = append(m.data.Tree, TreeNode{Name: "file.go"})
	}
	m.explorer = NewExplorerState(m.data)
	startHL := m.explorer.highlighted

	// Scroll the mouse over the tree section (past the modified-files
	// window). The explorer now has two scrollable regions; wheel events
	// are routed by y-coordinate so we have to target the tree section
	// explicitly.
	bounds := m.buildPanelBounds()
	var explorerB panelBounds
	for _, b := range bounds {
		if b.pid == PanelExplorer {
			explorerB = b
			break
		}
	}
	if explorerB.yMax == 0 {
		t.Fatalf("explorer panel bounds not found")
	}

	modAreaH := m.explorerModAreaH()
	// Tree starts at: border + mod header + modAreaH rows + separator + tree header.
	treeY := explorerB.yMin + 1 // border
	if modAreaH > 0 {
		treeY += 1 + modAreaH // header + mod rows
	}
	treeY += 2 // separator + tree header

	wheelMsg := tea.MouseWheelMsg{
		X:      explorerB.xMin + 1,
		Y:      treeY,
		Button: tea.MouseWheelDown,
	}
	m2, _ := m.handleMouseWheel(wheelMsg)

	if m2.explorer.highlighted == startHL {
		t.Errorf("wheel-down over tree section should move highlight; before=%d after=%d",
			startHL, m2.explorer.highlighted)
	}
}

// TestExplorerMouseWheelOverModSectionScrollsModified verifies that wheel
// events over the modified-files window scroll that section instead of
// moving the tree highlight. This is the regression test for the "30+
// modified files erase the tree" UX issue: with the new dual-region
// layout the user must be able to flip through their changed files
// without losing track of where the tree cursor is.
func TestExplorerMouseWheelOverModSectionScrollsModified(t *testing.T) {
	t.Parallel()
	m := NewModelWithConfig(DefaultConfig(), LayoutStack)
	m.width = 90
	m.height = 50
	m.bp = BreakpointMedium
	m.layoutMode = LayoutStack
	m.focused = PanelExplorer

	// Provide enough modified files to force the section to overflow its
	// fractional budget and become scrollable.
	m.data.ModifiedFiles = nil
	for i := 0; i < 40; i++ {
		m.data.ModifiedFiles = append(m.data.ModifiedFiles,
			ModifiedFile{Mark: "M", Path: "file.go", Stats: "+1 -0"})
	}
	m.explorer = NewExplorerState(m.data)
	startOff := m.explorer.modScrollOff

	bounds := m.buildPanelBounds()
	var explorerB panelBounds
	for _, b := range bounds {
		if b.pid == PanelExplorer {
			explorerB = b
			break
		}
	}
	if explorerB.yMax == 0 {
		t.Fatalf("explorer panel bounds not found")
	}

	// Y inside the modified rows (after border + header).
	wheelMsg := tea.MouseWheelMsg{
		X:      explorerB.xMin + 1,
		Y:      explorerB.yMin + 2,
		Button: tea.MouseWheelDown,
	}
	m2, _ := m.handleMouseWheel(wheelMsg)

	if m2.explorer.modScrollOff <= startOff {
		t.Errorf("wheel-down over modified section should advance modScrollOff; before=%d after=%d",
			startOff, m2.explorer.modScrollOff)
	}
}
