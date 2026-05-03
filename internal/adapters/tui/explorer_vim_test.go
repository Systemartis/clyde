package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// explorerActiveModel returns a Model with the explorer in active mode and a
// long synthetic tree so cursor movement is observable.
func explorerActiveModel(t *testing.T) Model {
	t.Helper()
	m := NewModel()
	// Build a 60-row tree fixture so half/full-page jumps have real headroom.
	tree := make([]TreeNode, 60)
	for i := range tree {
		tree[i] = TreeNode{Name: "f" + intToStr(i) + ".go", FullPath: "f" + intToStr(i) + ".go"}
	}
	m.data = MockData{Tree: tree}
	m.explorer = NewExplorerState(m.data)
	m.explorer.section = SectionTree
	m = m.setFocus(PanelExplorer)
	m.activePanelID = PanelExplorer
	// Bump panel height so explorerTreeAreaH() returns a real budget.
	m.panelHeights = map[PanelID]int{PanelExplorer: 30}
	return m
}

func TestExplorerVim_ggJumpsToTop(t *testing.T) {
	t.Parallel()
	m := explorerActiveModel(t)
	m.explorer.highlighted = 25
	next, _ := m.Update(tea.KeyPressMsg{Code: 'g'})
	m = next.(Model)
	if !m.vimGPending {
		t.Fatal("expected vimGPending after first g")
	}
	if m.explorer.highlighted != 25 {
		t.Errorf("first g should not move cursor; got hl=%d", m.explorer.highlighted)
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: 'g'})
	m = next.(Model)
	if m.explorer.highlighted != 0 {
		t.Errorf("after gg: highlighted = %d, want 0", m.explorer.highlighted)
	}
}

func TestExplorerVim_GBottom_ShiftRune(t *testing.T) {
	t.Parallel()
	m := explorerActiveModel(t)
	// Terminals WITHOUT kitty keyboard protocol fold shift into the
	// uppercase rune directly.
	next, _ := m.Update(tea.KeyPressMsg{Code: 'G'})
	m = next.(Model)
	if got := m.explorer.highlighted; got != len(m.explorer.rows)-1 {
		t.Errorf("after G (uppercase rune): highlighted = %d, want %d", got, len(m.explorer.rows)-1)
	}
}

func TestExplorerVim_GBottom_KittyShift(t *testing.T) {
	t.Parallel()
	m := explorerActiveModel(t)
	// With kitty keyboard protocol Shift+g arrives as Code='g' + Mod=Shift,
	// not as the uppercase rune. The handler must accept both.
	next, _ := m.Update(tea.KeyPressMsg{Code: 'g', Mod: tea.ModShift})
	m = next.(Model)
	if got := m.explorer.highlighted; got != len(m.explorer.rows)-1 {
		t.Errorf("after Shift+g: highlighted = %d, want %d", got, len(m.explorer.rows)-1)
	}
}

func TestExplorerVim_CtrlD_HalfPageDown(t *testing.T) {
	t.Parallel()
	m := explorerActiveModel(t)
	before := m.explorer.highlighted
	next, _ := m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	m = next.(Model)
	if m.explorer.highlighted <= before {
		t.Errorf("after Ctrl+d: highlighted = %d, want > %d", m.explorer.highlighted, before)
	}
}

func TestExplorerVim_CtrlF_FullPageGreaterThanHalf(t *testing.T) {
	t.Parallel()
	mD := explorerActiveModel(t)
	mD = mustUpdate(t, mD, tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	mF := explorerActiveModel(t)
	mF = mustUpdate(t, mF, tea.KeyPressMsg{Code: 'f', Mod: tea.ModCtrl})
	if mF.explorer.highlighted <= mD.explorer.highlighted {
		t.Errorf("Ctrl+f (hl=%d) should advance further than Ctrl+d (hl=%d)",
			mF.explorer.highlighted, mD.explorer.highlighted)
	}
}

func mustUpdate(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	next, _ := m.Update(msg)
	return next.(Model)
}
