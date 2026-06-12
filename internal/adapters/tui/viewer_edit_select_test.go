package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestSelect_ShiftRightExtends verifies shift+right starts a selection,
// extends it, and clears it on plain right.
func TestSelect_ShiftRightExtends(t *testing.T) {
	t.Parallel()
	m, _ := editTestModel(t, "hello")
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'i'})
	// Shift+Right anchors at col 0, moves cursor to col 1.
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift})
	if !m.viewerSelActive {
		t.Fatal("expected selection active")
	}
	if m.viewerSelAnchor.Col != 0 || m.viewerEdit.Col != 1 {
		t.Errorf("anchor=(%d,%d) cursor=(%d,%d), want anchor at col 0 cursor at col 1",
			m.viewerSelAnchor.Line, m.viewerSelAnchor.Col,
			m.viewerEdit.Line, m.viewerEdit.Col)
	}
	if got := m.selectedText(); got != "h" {
		t.Errorf("selectedText = %q, want %q", got, "h")
	}
	// Plain Right clears selection.
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyRight})
	if m.viewerSelActive {
		t.Error("plain Right should clear selection")
	}
}

func TestSelect_ShiftDownAcrossLines(t *testing.T) {
	t.Parallel()
	m, _ := editTestModel(t, "abc\ndef\nghi")
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'i'})
	// Shift+Down extends across to line 1, col 0.
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift})
	if got := m.selectedText(); got != "abc\n" {
		t.Errorf("selectedText = %q, want %q", got, "abc\n")
	}
}

func TestSelect_CtrlCCopiesToClipboard(t *testing.T) {
	t.Parallel()
	m, _ := editTestModel(t, "alpha beta")
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'i'})
	for range 5 {
		m = mustUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift})
	}
	if got := m.selectedText(); got != "alpha" {
		t.Fatalf("setup: selection = %q, want %q", got, "alpha")
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m = next.(Model)
	if cmd == nil {
		t.Error("Ctrl+C with selection should produce a SetClipboard cmd")
	}
	if !strings.Contains(m.viewerStatus, "copied") {
		t.Errorf("expected copy status, got %q", m.viewerStatus)
	}
}

func TestSelect_CtrlAAllSelectsBuffer(t *testing.T) {
	t.Parallel()
	m, _ := editTestModel(t, "line1\nline2\nline3")
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'i'})
	// Use Cmd+A (Mac) to select all — Ctrl+A is line-start by Emacs convention.
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'a', Mod: tea.ModSuper})
	if !m.viewerSelActive {
		t.Fatal("expected selection active after Cmd+A")
	}
	if got := m.selectedText(); got != "line1\nline2\nline3" {
		t.Errorf("selectedText = %q, want full buffer", got)
	}
}

func TestSelect_PlainMotionClearsSelection(t *testing.T) {
	t.Parallel()
	m, _ := editTestModel(t, "abc")
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'i'})
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift})
	if !m.viewerSelActive {
		t.Fatal("setup: expected selection")
	}
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyLeft})
	if m.viewerSelActive {
		t.Error("plain motion should clear selection")
	}
}

func TestSelect_TypingWithSelectionClears(t *testing.T) {
	t.Parallel()
	m, _ := editTestModel(t, "abc")
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'i'})
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift})
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'X', Text: "X"})
	if m.viewerSelActive {
		t.Error("typing should clear selection (we keep current insert-AT-cursor semantics)")
	}
}

// TestSelect_ClearedOnExitEditMode verifies a leftover selection from
// edit mode doesn't bleed back into view mode — the highlight would be
// anchored at a position view mode has no cursor for, which looks broken.
func TestSelect_ClearedOnExitEditMode(t *testing.T) {
	t.Parallel()
	m, _ := editTestModel(t, "abc")
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'i'})
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift})
	if !m.viewerSelActive {
		t.Fatal("setup: expected selection active")
	}
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.viewerMode != ViewerView {
		t.Fatal("setup: expected ViewerView after esc")
	}
	if m.viewerSelActive {
		t.Error("selection should clear on exit-to-view-mode")
	}
}

// TestSelect_ParseCommandPaletteContains verifies the discoverability
// palette has every documented command form.
func TestSelect_ParseCommandPaletteContains(t *testing.T) {
	t.Parallel()
	entries := commandPaletteEntries()
	want := []string{"w", "q", "q!", "wq", "y", "y N", "y N,M"}
	for _, expected := range want {
		found := false
		for _, e := range entries {
			for _, f := range e.Forms {
				if f == expected {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("palette missing form %q", expected)
		}
	}
}
