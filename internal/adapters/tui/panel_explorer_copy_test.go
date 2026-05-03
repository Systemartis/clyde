package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// TestTruncateCopyLabel verifies that long paths are middle-truncated to
// the cap with a leading ellipsis while short paths pass through unchanged.
func TestTruncateCopyLabel(t *testing.T) {
	short := "src/foo.go"
	if got := truncateCopyLabel(short); got != short {
		t.Errorf("short path mutated: got %q, want %q", got, short)
	}

	long := "/Users/vladpb/work/very/deeply/nested/directory/structure/that/exceeds/the/cap/file.go"
	got := truncateCopyLabel(long)
	if !strings.HasPrefix(got, "…") {
		t.Errorf("long path missing ellipsis prefix: %q", got)
	}
	if len([]rune(got)) > copyToastLabelMax {
		t.Errorf("long path not truncated: len=%d cap=%d", len([]rune(got)), copyToastLabelMax)
	}
	// Suffix must be preserved so the user can still tell which file it was.
	if !strings.HasSuffix(got, "file.go") {
		t.Errorf("filename suffix lost in truncation: %q", got)
	}
}

// TestIsExplorerDoubleClick checks the double-click state machine —
// same path within window counts; different path or expired window does not.
func TestIsExplorerDoubleClick(t *testing.T) {
	t.Run("no prior click", func(t *testing.T) {
		m := Model{}
		if m.isExplorerDoubleClick("foo.go") {
			t.Error("zero-value model should not report double-click")
		}
	})

	t.Run("same path within window", func(t *testing.T) {
		m := Model{
			lastExplorerClickPath: "foo.go",
			lastExplorerClickAt:   time.Now(),
		}
		if !m.isExplorerDoubleClick("foo.go") {
			t.Error("same-path click within window should count as double-click")
		}
	})

	t.Run("different path", func(t *testing.T) {
		m := Model{
			lastExplorerClickPath: "foo.go",
			lastExplorerClickAt:   time.Now(),
		}
		if m.isExplorerDoubleClick("bar.go") {
			t.Error("click on a different path must not trigger double-click")
		}
	})

	t.Run("expired window", func(t *testing.T) {
		m := Model{
			lastExplorerClickPath: "foo.go",
			lastExplorerClickAt:   time.Now().Add(-2 * explorerDoubleClickWindow),
		}
		if m.isExplorerDoubleClick("foo.go") {
			t.Error("click after window expired should not count as double-click")
		}
	})

	t.Run("empty path never matches", func(t *testing.T) {
		m := Model{
			lastExplorerClickPath: "",
			lastExplorerClickAt:   time.Now(),
		}
		if m.isExplorerDoubleClick("") {
			t.Error("empty path should never count as a double-click target")
		}
	})
}

// TestExplorerCopyTarget covers the four resolution paths: tree file,
// mod-section file, basename mode, and empty highlight (no-op).
func TestExplorerCopyTarget(t *testing.T) {
	t.Run("tree section full path (demo mode keeps relative)", func(t *testing.T) {
		m := NewModel() // demo mode, no cwd
		// NewExplorerState collapses every dir, so the highlighted (first)
		// row is whatever the mock tree starts with. Force a known row by
		// rebuilding rows with all dirs expanded.
		m.explorer.collapsed = map[string]bool{}
		m.explorer.RefreshRows(m.data)
		m.explorer.section = SectionTree
		// Find a file row to highlight deterministically.
		idx := -1
		for i, r := range m.explorer.rows {
			if !r.IsDir {
				idx = i
				break
			}
		}
		if idx < 0 {
			t.Skip("mock tree has no file rows — adjust mock data")
		}
		m.explorer.highlighted = idx

		payload, label, ok := m.explorerCopyTarget(false)
		if !ok {
			t.Fatal("expected ok=true for highlighted file row")
		}
		if payload == "" || label == "" {
			t.Errorf("payload/label must not be empty: payload=%q label=%q", payload, label)
		}
	})

	t.Run("basename only returns just filename for both payload and label", func(t *testing.T) {
		m := NewModel()
		m.explorer.collapsed = map[string]bool{}
		m.explorer.RefreshRows(m.data)
		m.explorer.section = SectionTree
		for i, r := range m.explorer.rows {
			if !r.IsDir {
				m.explorer.highlighted = i
				break
			}
		}
		payload, label, ok := m.explorerCopyTarget(true)
		if !ok {
			t.Fatal("expected ok=true")
		}
		if strings.Contains(payload, "/") {
			t.Errorf("basename payload still has separator: %q", payload)
		}
		if payload != label {
			t.Errorf("basename payload and label should match: payload=%q label=%q", payload, label)
		}
	})

	t.Run("mod section out-of-range returns ok=false", func(t *testing.T) {
		m := NewModel()
		m.explorer.section = SectionMod
		m.explorer.modHighlight = 999_999
		_, _, ok := m.explorerCopyTarget(false)
		if ok {
			t.Error("out-of-range mod cursor should produce ok=false")
		}
	})

	t.Run("empty tree returns ok=false", func(t *testing.T) {
		m := NewModel()
		m.data.ModifiedFiles = nil
		m.explorer.rows = nil
		m.explorer.section = SectionTree
		_, _, ok := m.explorerCopyTarget(false)
		if ok {
			t.Error("empty tree should produce ok=false")
		}
	})
}

// TestCopyExplorerHighlight_SetsToastAndDispatchesCmd verifies the full
// yank flow: model gets a toast, expiry is set, and a non-nil tea.Cmd
// is returned (which carries the OSC 52 SetClipboard + clear tick).
func TestCopyExplorerHighlight_SetsToastAndDispatchesCmd(t *testing.T) {
	m := NewModel()
	m.explorer.collapsed = map[string]bool{}
	m.explorer.RefreshRows(m.data)
	m.explorer.section = SectionTree
	for i, r := range m.explorer.rows {
		if !r.IsDir {
			m.explorer.highlighted = i
			break
		}
	}

	updated, cmd := m.copyExplorerHighlight(false)
	if cmd == nil {
		t.Fatal("expected a non-nil tea.Cmd carrying SetClipboard + clear tick")
	}
	if !strings.HasPrefix(updated.copyToast, "✓ copied:") {
		t.Errorf("toast not set or wrong prefix: %q", updated.copyToast)
	}
	if updated.copyToastExpires.IsZero() {
		t.Error("toast expiry must be set so the clear tick can compare against it")
	}
}

// TestHandleExplorerActiveKey_Y wires the y keybind end-to-end through
// the active-mode handler. Exercises the route Update would take without
// going through Update itself (which involves more orchestration).
func TestHandleExplorerActiveKey_Y(t *testing.T) {
	m := NewModel()
	m.explorer.collapsed = map[string]bool{}
	m.explorer.RefreshRows(m.data)
	m.explorer.section = SectionTree
	for i, r := range m.explorer.rows {
		if !r.IsDir {
			m.explorer.highlighted = i
			break
		}
	}

	updated, cmd := m.handleExplorerActiveKey(tea.KeyPressMsg{Code: 'y'})
	if cmd == nil {
		t.Fatal("y in explorer active mode must dispatch a tea.Cmd (clipboard)")
	}
	if updated.copyToast == "" {
		t.Error("y must set a toast so the user sees the yank confirmation")
	}
}

// TestHandleExplorerActiveKey_ShiftY_BothReportingModes guards against
// the kitty-keyboard regression: with the kitty protocol on (Bubbletea
// v2 default) Shift+y arrives as Code='y' + Mod=ModShift, not as the
// literal uppercase 'Y' rune. Both reporting modes must yank the
// basename, not the full path.
func TestHandleExplorerActiveKey_ShiftY_BothReportingModes(t *testing.T) {
	cases := []struct {
		name string
		msg  tea.KeyPressMsg
	}{
		{"kitty protocol — y + ModShift", tea.KeyPressMsg{Code: 'y', Mod: tea.ModShift}},
		{"legacy folding — literal Y", tea.KeyPressMsg{Code: 'Y'}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewModel()
			m.explorer.collapsed = map[string]bool{}
			m.explorer.RefreshRows(m.data)
			m.explorer.section = SectionTree
			fileIdx := -1
			for i, r := range m.explorer.rows {
				if !r.IsDir {
					fileIdx = i
					break
				}
			}
			if fileIdx < 0 {
				t.Skip("mock tree has no file rows")
			}
			m.explorer.highlighted = fileIdx
			expectedBasename := m.explorer.rows[fileIdx].Path
			// truncateCopyLabel is identity for short labels — we expect
			// the toast to contain the bare basename, not a separator.
			if strings.Contains(expectedBasename, "/") {
				// Strip to basename for comparison since that's what Y should produce.
				parts := strings.Split(expectedBasename, "/")
				expectedBasename = parts[len(parts)-1]
			}

			updated, cmd := m.handleExplorerActiveKey(tc.msg)
			if cmd == nil {
				t.Fatal("Shift+y must dispatch a tea.Cmd")
			}
			if !strings.Contains(updated.copyToast, expectedBasename) {
				t.Errorf("toast should contain basename %q, got %q", expectedBasename, updated.copyToast)
			}
			if strings.Contains(updated.copyToast, "/") {
				t.Errorf("Shift+y must yank ONLY the basename — toast contains a path separator: %q", updated.copyToast)
			}
		})
	}
}

// TestHandleExplorerActiveKey_NavKeysReturnNilCmd ensures the (Model,
// tea.Cmd) refactor didn't accidentally start emitting commands for the
// pre-existing nav keys (↑/↓/←/Backspace/Tab/Enter).
func TestHandleExplorerActiveKey_NavKeysReturnNilCmd(t *testing.T) {
	keys := []tea.KeyPressMsg{
		{Code: tea.KeyUp},
		{Code: tea.KeyDown},
		{Code: tea.KeyLeft},
		{Code: tea.KeyBackspace},
	}
	for _, k := range keys {
		m := NewModel()
		_, cmd := m.handleExplorerActiveKey(k)
		if cmd != nil {
			t.Errorf("nav key %v should return nil cmd, got %v", k, cmd)
		}
	}
}

// TestClearCopyToastMsg verifies the auto-clear handler: an old tick
// arriving after a fresh yank must NOT wipe the new toast, but a tick
// past the expiry must clear it.
func TestClearCopyToastMsg(t *testing.T) {
	t.Run("expired toast gets cleared", func(t *testing.T) {
		m := NewModel()
		m.copyToast = "✓ copied: foo.go"
		m.copyToastExpires = time.Now().Add(-time.Second) // already expired

		updated, _ := m.Update(clearCopyToastMsg{})
		um := updated.(Model)
		if um.copyToast != "" {
			t.Errorf("expired toast should be cleared, got %q", um.copyToast)
		}
		if !um.copyToastExpires.IsZero() {
			t.Error("expiry should be reset to zero after clear")
		}
	})

	t.Run("fresh toast survives an old tick", func(t *testing.T) {
		m := NewModel()
		m.copyToast = "✓ copied: bar.go"
		m.copyToastExpires = time.Now().Add(time.Second) // still in the future

		updated, _ := m.Update(clearCopyToastMsg{})
		um := updated.(Model)
		if um.copyToast == "" {
			t.Error("fresh toast must NOT be wiped by an old tick")
		}
	})
}
