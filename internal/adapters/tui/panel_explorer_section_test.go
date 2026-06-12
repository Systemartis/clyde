package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestExplorerMoveUp_CrossesIntoModSection verifies that when the cursor
// is at the top of the tree (highlighted=0) and there are modified files,
// pressing ↑ jumps the cursor into the modified-files section. Without
// this, the user could only reach modified entries via mouse click.
func TestExplorerMoveUp_CrossesIntoModSection(t *testing.T) {
	t.Parallel()
	es := ExplorerState{
		section:     SectionTree,
		highlighted: 0,
		rows:        []ExplorerRow{{Path: "f.go"}},
	}
	es.MoveUp(5) // 5 modified files

	if es.section != SectionMod {
		t.Errorf("section = %v, want SectionMod after crossing", es.section)
	}
	if es.modHighlight != 4 {
		t.Errorf("modHighlight = %d, want 4 (last mod row)", es.modHighlight)
	}
}

// TestExplorerMoveDown_CrossesFromModToTree verifies that when the cursor
// is at the last modified file, pressing ↓ jumps into the top of the
// tree section. Symmetric to the up-crossing test above.
func TestExplorerMoveDown_CrossesFromModToTree(t *testing.T) {
	t.Parallel()
	es := ExplorerState{
		section:      SectionMod,
		modHighlight: 4,
		rows:         []ExplorerRow{{Path: "f.go"}, {Path: "g.go"}},
	}
	es.MoveDown(5)

	if es.section != SectionTree {
		t.Errorf("section = %v, want SectionTree after crossing down", es.section)
	}
	if es.highlighted != 0 {
		t.Errorf("highlighted = %d, want 0 (top of tree)", es.highlighted)
	}
}

// TestExplorerMoveUp_TopOfModStays asserts that pressing ↑ at the top of
// the modified section (modHighlight=0) stays put — no wrap-around to
// the tree, which would feel disorienting.
func TestExplorerMoveUp_TopOfModStays(t *testing.T) {
	t.Parallel()
	es := ExplorerState{
		section:      SectionMod,
		modHighlight: 0,
		rows:         []ExplorerRow{{Path: "f.go"}},
	}
	es.MoveUp(3)
	if es.section != SectionMod || es.modHighlight != 0 {
		t.Errorf("expected to stay at top of mod (section=Mod, idx=0); got section=%v idx=%d",
			es.section, es.modHighlight)
	}
}

// TestExplorerMoveDown_BottomOfTreeStays asserts that pressing ↓ at the
// last tree row clamps without wrapping back to mod.
func TestExplorerMoveDown_BottomOfTreeStays(t *testing.T) {
	t.Parallel()
	es := ExplorerState{
		section:     SectionTree,
		highlighted: 1,
		rows:        []ExplorerRow{{Path: "f.go"}, {Path: "g.go"}},
	}
	es.MoveDown(3)
	if es.section != SectionTree || es.highlighted != 1 {
		t.Errorf("expected to clamp at bottom of tree; got section=%v idx=%d",
			es.section, es.highlighted)
	}
}

// TestPanelJump_WorksFromActiveMode verifies that the plain-letter jump
// shortcuts (e/a/d/u/s/b/c) are reachable from inside an active panel,
// not just from passive mode. The old ⌃-prefixed variants worked
// globally; preserving that ergonomic was the whole point of routing
// them through handleKey before the active-mode dispatch.
func TestPanelJump_WorksFromActiveMode(t *testing.T) {
	t.Parallel()
	m := NewModelWithConfig(DefaultConfig(), LayoutStack)
	m.focused = PanelExplorer
	m.activePanelID = PanelExplorer // simulate Enter-to-active

	next, _ := m.handleKey(tea.KeyPressMsg{Code: 'a'})
	mm := next.(Model)
	if mm.focused != PanelCalls {
		t.Errorf("`a` from active mode should focus PanelCalls, got %v", mm.focused)
	}
	if mm.activePanelID == PanelExplorer {
		t.Errorf("jumping panels should drop the previous panel out of active mode")
	}
}
