package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// editTestModel returns a Model with the viewer open on a real on-disk file
// so save tests can round-trip to the filesystem. The file lives under
// t.TempDir() and is cleaned up automatically.
func editTestModel(t *testing.T, initial string) (Model, string) {
	t.Helper()
	tmp := t.TempDir()
	path := filepath.Join(tmp, "fixture.txt")
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatalf("seed fixture: %v", err)
	}
	m := NewModel()
	m.demoMode = false
	m = m.loadViewerFile(path)
	return m, path
}

func TestEdit_iEntersInsert_EscReturns(t *testing.T) {
	t.Parallel()
	m, _ := editTestModel(t, "hello\n")
	if m.viewerMode != ViewerView {
		t.Fatal("setup: expected ViewerView mode")
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: 'i'})
	m = next.(Model)
	if m.viewerMode != ViewerEdit {
		t.Errorf("after i: mode = %v, want ViewerEdit", m.viewerMode)
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = next.(Model)
	if m.viewerMode != ViewerView {
		t.Errorf("after esc: mode = %v, want ViewerView", m.viewerMode)
	}
}

func TestEdit_TypeInsertsAndDirty(t *testing.T) {
	t.Parallel()
	m, _ := editTestModel(t, "abc")
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'i'})
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'X'})
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'Y'})
	if got := m.viewerEdit.String(); got != "XYabc" {
		t.Errorf("buffer = %q, want %q", got, "XYabc")
	}
	if !m.viewerDirty {
		t.Error("expected viewerDirty = true after typing")
	}
}

func TestEdit_EnterSplitsLine(t *testing.T) {
	t.Parallel()
	m, _ := editTestModel(t, "abc")
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'i'})
	// Move cursor to position 2 (after "ab").
	m.viewerEdit.Col = 2
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := m.viewerEdit.String(); got != "ab\nc" {
		t.Errorf("buffer = %q, want %q", got, "ab\nc")
	}
}

func TestEdit_BackspaceDeletes(t *testing.T) {
	t.Parallel()
	m, _ := editTestModel(t, "abc")
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'i'})
	m.viewerEdit.Col = 2
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	if got := m.viewerEdit.String(); got != "ac" {
		t.Errorf("buffer = %q, want %q", got, "ac")
	}
}

func TestEdit_CtrlSWritesToDisk(t *testing.T) {
	t.Parallel()
	m, path := editTestModel(t, "original\n")
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'i'})
	for _, r := range []rune{'!', ' '} {
		m = mustUpdate(t, m, tea.KeyPressMsg{Code: r})
	}
	if !m.viewerDirty {
		t.Fatal("expected dirty after typing")
	}
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if m.viewerDirty {
		t.Error("dirty should clear after save")
	}
	if !strings.Contains(m.viewerStatus, "saved") {
		t.Errorf("expected saved status, got %q", m.viewerStatus)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("readback: %v", err)
	}
	want := "! original\n"
	if string(got) != want {
		t.Errorf("on-disk content = %q, want %q", got, want)
	}
}

func TestEdit_TabInsertsSpaces(t *testing.T) {
	t.Parallel()
	m, _ := editTestModel(t, "")
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'i'})
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyTab})
	want := strings.Repeat(" ", viewerTabWidth)
	if got := m.viewerEdit.String(); got != want {
		t.Errorf("buffer = %q, want %q (tab → spaces)", got, want)
	}
}

func TestEdit_OpeningNewFileResetsState(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	a := filepath.Join(tmp, "a.txt")
	b := filepath.Join(tmp, "b.txt")
	if err := os.WriteFile(a, []byte("aaa"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("bbb"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := NewModel()
	m.demoMode = false
	m = m.loadViewerFile(a)
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'i'})
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'X'})
	if !m.viewerDirty {
		t.Fatal("setup: should be dirty after typing")
	}
	// Switch to a different file — edit state must reset.
	m = m.loadViewerFile(b)
	if m.viewerMode != ViewerView {
		t.Errorf("after open new file: mode = %v, want ViewerView", m.viewerMode)
	}
	if m.viewerDirty {
		t.Error("after open new file: dirty should reset")
	}
	if m.viewerEdit.String() != "bbb" {
		t.Errorf("buffer should reflect new file; got %q", m.viewerEdit.String())
	}
}
