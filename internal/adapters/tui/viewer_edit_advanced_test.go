package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestEdit_UppercaseViaTextField verifies bubbletea v2's KeyPressMsg.Text
// is the source of truth for inserted characters — Shift+a under the kitty
// keyboard protocol arrives as Code='a' + Mod=ModShift + Text="A". Without
// honoring Text, the editor would write lowercase even when the user
// pressed Shift.
func TestEdit_UppercaseViaTextField(t *testing.T) {
	t.Parallel()
	m, _ := editTestModel(t, "")
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'i'})
	// Simulate kitty-style shifted alpha: Code lowercase, Text uppercase.
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'a', Mod: tea.ModShift, Text: "A"})
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'b', Mod: tea.ModShift, Text: "B"})
	if got := m.viewerEdit.String(); got != "AB" {
		t.Errorf("buffer = %q, want %q (uppercase via Text field)", got, "AB")
	}
}

// TestEdit_NonLatinTextField verifies multi-byte runes from msg.Text round-
// trip through the editor — exercises the Text-as-source-of-truth path for
// languages outside the Latin alphabet.
func TestEdit_NonLatinTextField(t *testing.T) {
	t.Parallel()
	m, _ := editTestModel(t, "")
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'i'})
	m = mustUpdate(t, m, tea.KeyPressMsg{Text: "α"})
	m = mustUpdate(t, m, tea.KeyPressMsg{Text: "β"})
	if got := m.viewerEdit.String(); got != "αβ" {
		t.Errorf("buffer = %q, want %q", got, "αβ")
	}
}

// TestEdit_PasteInserts verifies bracketed paste populates the buffer at
// the cursor, expanding tabs to spaces and respecting newlines.
func TestEdit_PasteInserts(t *testing.T) {
	t.Parallel()
	m, _ := editTestModel(t, "x")
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'i'})
	m.viewerEdit.Col = 1
	m = mustUpdate(t, m, tea.PasteMsg{Content: "hello\n\tworld"})
	want := "xhello\n    world"
	if got := m.viewerEdit.String(); got != want {
		t.Errorf("buffer = %q, want %q", got, want)
	}
	if !m.viewerDirty {
		t.Error("paste should mark buffer dirty")
	}
}

// TestEdit_UndoRedo_RoundTrip covers the basic undo / redo cycle.
func TestEdit_UndoRedo_RoundTrip(t *testing.T) {
	t.Parallel()
	m, _ := editTestModel(t, "ab")
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'i'})
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'X', Text: "X"})
	if got := m.viewerEdit.String(); got != "Xab" {
		t.Fatalf("after type X: %q", got)
	}
	// Ctrl+Z undoes.
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'z', Mod: tea.ModCtrl})
	if got := m.viewerEdit.String(); got != "ab" {
		t.Errorf("after Ctrl+Z: %q, want \"ab\"", got)
	}
	// Ctrl+Y redoes.
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl})
	if got := m.viewerEdit.String(); got != "Xab" {
		t.Errorf("after Ctrl+Y: %q, want \"Xab\"", got)
	}
}

// TestEdit_UndoBeyondHistoryNoOp verifies popping more times than there are
// history entries doesn't panic or corrupt state.
func TestEdit_UndoBeyondHistoryNoOp(t *testing.T) {
	t.Parallel()
	m, _ := editTestModel(t, "abc")
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'i'})
	for range 5 {
		m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'z', Mod: tea.ModCtrl})
	}
	if got := m.viewerEdit.String(); got != "abc" {
		t.Errorf("buffer = %q after over-undo, want unchanged \"abc\"", got)
	}
}

// TestEdit_CommandMode_WSavesQQuits verifies the :w / :q dispatch.
func TestEdit_CommandMode_WSavesQQuits(t *testing.T) {
	t.Parallel()
	m, path := editTestModel(t, "before\n")
	// Edit then save via :w.
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'i'})
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'A', Text: "A"})
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyEsc}) // back to view
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: ':'})
	if !m.viewerCmdActive {
		t.Fatal("expected viewerCmdActive after ':'")
	}
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'w', Text: "w"})
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.viewerDirty {
		t.Error(":w should clear dirty")
	}
	got, _ := os.ReadFile(path)
	if string(got) != "Abefore\n" {
		t.Errorf("on-disk = %q, want %q", got, "Abefore\n")
	}

	// :q closes the viewer when clean.
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: ':'})
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'q', Text: "q"})
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.viewerActive {
		t.Error(":q should close the viewer")
	}
}

// TestEdit_CommandMode_QRefusesWhenDirty verifies bare :q on a dirty
// buffer surfaces an error and keeps the viewer open.
func TestEdit_CommandMode_QRefusesWhenDirty(t *testing.T) {
	t.Parallel()
	m, _ := editTestModel(t, "x")
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'i'})
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'A', Text: "A"})
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyEsc})
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: ':'})
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'q', Text: "q"})
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.viewerActive {
		t.Error(":q on dirty buffer should keep the viewer open")
	}
	if !strings.Contains(m.viewerStatus, "unsaved") {
		t.Errorf("expected unsaved-changes status, got %q", m.viewerStatus)
	}
}

// TestEdit_CommandMode_QBangDiscards verifies :q! force-closes the viewer.
func TestEdit_CommandMode_QBangDiscards(t *testing.T) {
	t.Parallel()
	m, _ := editTestModel(t, "x")
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'i'})
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'A', Text: "A"})
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyEsc})
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: ':'})
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'q', Text: "q"})
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: '!', Text: "!"})
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.viewerActive {
		t.Error(":q! should force-close the viewer")
	}
}

// TestEdit_DirtyConfirm_EscTwiceDiscards verifies the safety net: first
// Esc with unsaved changes warns; second Esc closes.
func TestEdit_DirtyConfirm_EscTwiceDiscards(t *testing.T) {
	t.Parallel()
	m, _ := editTestModel(t, "x")
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'i'})
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'A', Text: "A"})
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyEsc}) // back to view
	if m.viewerMode != ViewerView {
		t.Fatal("setup")
	}
	// First Esc on dirty buffer: warns.
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if !m.viewerActive {
		t.Error("first Esc should warn, not close")
	}
	if m.viewerStatus != viewerDiscardPrompt {
		t.Errorf("expected discard prompt, got %q", m.viewerStatus)
	}
	// Second Esc: closes.
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.viewerActive {
		t.Error("second Esc should close the viewer")
	}
}

// TestEdit_WordMotion_CtrlRightLeft verifies word-jumps via Ctrl-arrow.
func TestEdit_WordMotion_CtrlRightLeft(t *testing.T) {
	t.Parallel()
	m, _ := editTestModel(t, "alpha beta gamma")
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'i'})
	if m.viewerEdit.Col != 0 {
		t.Fatal("setup: cursor not at col 0")
	}
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModCtrl})
	if m.viewerEdit.Col != 6 { // start of "beta"
		t.Errorf("after Ctrl+Right: col = %d, want 6", m.viewerEdit.Col)
	}
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModCtrl})
	if m.viewerEdit.Col != 0 { // back to start of "alpha"
		t.Errorf("after Ctrl+Left: col = %d, want 0", m.viewerEdit.Col)
	}
}

// TestEdit_HomeEnd_LineEdges covers Home / End line-edge motions.
func TestEdit_HomeEnd_LineEdges(t *testing.T) {
	t.Parallel()
	m, _ := editTestModel(t, "hello world")
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'i'})
	m.viewerEdit.Col = 5
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyEnd})
	if m.viewerEdit.Col != 11 {
		t.Errorf("after End: col = %d, want 11", m.viewerEdit.Col)
	}
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyHome})
	if m.viewerEdit.Col != 0 {
		t.Errorf("after Home: col = %d, want 0", m.viewerEdit.Col)
	}
}

// TestEdit_CommandMode_UnknownReportsError verifies an unrecognised :foo
// command surfaces a status error instead of silently failing.
func TestEdit_CommandMode_UnknownReportsError(t *testing.T) {
	t.Parallel()
	m, _ := editTestModel(t, "")
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: ':'})
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'f', Text: "f"})
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'o', Text: "o"})
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'o', Text: "o"})
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !strings.Contains(m.viewerStatus, "unknown command") {
		t.Errorf("expected unknown-command status, got %q", m.viewerStatus)
	}
}

// TestEdit_QuestionMarkInsertsNotSettings verifies typing '?' inside the
// editor inserts a literal question mark instead of opening the global
// settings overlay. The settings shortcut must yield to the editor when
// the user is mid-keystroke.
func TestEdit_QuestionMarkInsertsNotSettings(t *testing.T) {
	t.Parallel()
	m, _ := editTestModel(t, "")
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'i'})
	// `?` (Code='?') OR Shift+'/' (Code='/' + ModShift) — exercise both
	// because terminals deliver the mark differently depending on
	// keyboard layout / kitty protocol mode.
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: '?', Text: "?"})
	if m.settingsOpen {
		t.Fatal("`?` in edit mode must NOT open settings")
	}
	if got := m.viewerEdit.String(); got != "?" {
		t.Errorf("buffer = %q, want %q", got, "?")
	}
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: '/', Mod: tea.ModShift, Text: "?"})
	if m.settingsOpen {
		t.Fatal("Shift+/ in edit mode must NOT open settings either")
	}
	if got := m.viewerEdit.String(); got != "??" {
		t.Errorf("buffer = %q, want %q", got, "??")
	}
}

// TestEdit_QuestionMarkInCommandMode verifies '?' typed while composing a
// :command appends to the command buffer instead of opening settings.
func TestEdit_QuestionMarkInCommandMode(t *testing.T) {
	t.Parallel()
	m, _ := editTestModel(t, "")
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: ':'})
	if !m.viewerCmdActive {
		t.Fatal("setup: command mode should be active after ':'")
	}
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: '?', Text: "?"})
	if m.settingsOpen {
		t.Error("`?` in :command must NOT open settings")
	}
	if m.viewerCmdBuf != "?" {
		t.Errorf("cmd buf = %q, want %q", m.viewerCmdBuf, "?")
	}
}

// TestViewer_FAndI_WorkAfterExplorerClick is the regression for the
// reported bug: f / i had no effect when the user clicked into the
// explorer (which set activePanelID = PanelExplorer) and then opened a
// file. Active-mode dispatch was routing keystrokes to the explorer's
// handler instead of the viewer. The fix prioritises the viewer when
// viewerActive regardless of which panel was last interacted with.
func TestViewer_FAndI_WorkAfterExplorerClick(t *testing.T) {
	t.Parallel()
	m, _ := editTestModel(t, "hello")
	// Simulate the user having clicked the explorer first: activePanelID
	// stays set even after the viewer pops up.
	m.activePanelID = PanelExplorer

	// `i` should enter insert mode regardless.
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'i'})
	if m.viewerMode != ViewerEdit {
		t.Errorf("`i` after explorer-click: mode = %v, want ViewerEdit", m.viewerMode)
	}
	// Drop back to view, try `f` (fullscreen).
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyEsc})
	m.activePanelID = PanelExplorer // restore — exitEditMode shouldn't alter it but be paranoid
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'f'})
	if !m.viewerFullscreen {
		t.Errorf("`f` after explorer-click: viewerFullscreen = false, want true")
	}
}

// TestPanelLabel_FlipsToEditorOnInsert verifies the viewer panel's title
// switches from "viewer" to "editor" when the user enters insert mode —
// a visible cue that matches the user's mental model of "i opens the
// editor".
func TestPanelLabel_FlipsToEditorOnInsert(t *testing.T) {
	t.Parallel()
	m, _ := editTestModel(t, "hello")
	out := m.renderViewerPanel(80, 20)
	if !strings.Contains(stripANSI(out), "viewer") {
		t.Errorf("view mode should show 'viewer' label, got: %q", stripANSI(out)[:120])
	}
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'i'})
	out = m.renderViewerPanel(80, 20)
	if !strings.Contains(stripANSI(out), "editor") {
		t.Errorf("insert mode should show 'editor' label, got: %q", stripANSI(out)[:120])
	}
}

// TestViewerHeaderClick_TogglesFullscreen verifies a click on the viewer's
// top-border (anywhere except the close badge) toggles fullscreen — same
// gesture as every windowed editor's title-bar maximize.
func TestViewerHeaderClick_TogglesFullscreen(t *testing.T) {
	t.Parallel()
	m, _ := editTestModel(t, "x")
	m.width = 130
	m.height = 40
	m.bp = BreakpointMedium
	m.layoutMode = LayoutStack
	xMin, _ := m.viewerXBounds()
	// Click in the middle of the top border, well away from the close
	// badge on the right.
	clickX := xMin + 5
	if m.viewerFullscreen {
		t.Fatal("setup: expected viewerFullscreen=false")
	}
	next, _ := m.Update(tea.MouseClickMsg{X: clickX, Y: 2, Button: tea.MouseLeft})
	m = next.(Model)
	if !m.viewerFullscreen {
		t.Errorf("first header click should enable fullscreen")
	}
	next, _ = m.Update(tea.MouseClickMsg{X: clickX, Y: 2, Button: tea.MouseLeft})
	m = next.(Model)
	if m.viewerFullscreen {
		t.Errorf("second header click should toggle fullscreen off")
	}
}

// TestParseSubstituteCommand verifies the :s parser recognises every
// vim-style form and rejects malformed inputs.
func TestParseSubstituteCommand(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in              string
		ok              bool
		find            string
		replace         string
		global, allFlag bool
	}{
		{"s/foo/bar/", true, "foo", "bar", false, false},
		{"s/foo/bar/g", true, "foo", "bar", true, false},
		{"%s/foo/bar/", true, "foo", "bar", false, true},
		{"%s/foo/bar/g", true, "foo", "bar", true, true},
		{"s/foo/", true, "foo", "", false, false},
		{"s/foo/bar", true, "foo", "bar", false, false},
		{"s//bar/", false, "", "", false, false}, // empty find rejected
		{"s/foo", false, "", "", false, false},   // missing delimiter
		{"x/foo/bar/", false, "", "", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			got, ok := parseSubstituteCommand(tc.in)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if !ok {
				return
			}
			if got.find != tc.find || got.replace != tc.replace {
				t.Errorf("find=%q replace=%q, want %q / %q", got.find, got.replace, tc.find, tc.replace)
			}
			if got.global != tc.global || got.all != tc.allFlag {
				t.Errorf("flags global=%v all=%v, want %v / %v", got.global, got.all, tc.global, tc.allFlag)
			}
		})
	}
}

// TestSubstitute_GlobalAllReplaces verifies :%s/old/new/g replaces every
// occurrence in the buffer and surfaces a count.
func TestSubstitute_GlobalAllReplaces(t *testing.T) {
	t.Parallel()
	m, _ := editTestModel(t, "foo bar foo\nfoo baz\n")
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: ':'})
	for _, r := range "%s/foo/X/g" {
		m = mustUpdate(t, m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	want := "X bar X\nX baz\n"
	if got := m.viewerEdit.String(); got != want {
		t.Errorf("buffer = %q, want %q", got, want)
	}
	if !strings.Contains(m.viewerStatus, "3 occurrence") {
		t.Errorf("status = %q, want count 3", m.viewerStatus)
	}
}

// TestSubstitute_FirstMatchOnly verifies :%s/old/new/ (without g) replaces
// only the first match per line.
func TestSubstitute_FirstMatchOnly(t *testing.T) {
	t.Parallel()
	m, _ := editTestModel(t, "foo foo foo\nfoo foo")
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: ':'})
	for _, r := range "%s/foo/X/" {
		m = mustUpdate(t, m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := m.viewerEdit.String(); got != "X foo foo\nX foo" {
		t.Errorf("buffer = %q", got)
	}
}

// TestEditModeKeepsScrollPosition_OnEnterI verifies pressing `i` after
// scrolling in view mode preserves the visible page — the cursor snaps
// to the topmost visible line so the auto-scroll doesn't yank the user
// back to the file's beginning.
func TestEditModeKeepsScrollPosition_OnEnterI(t *testing.T) {
	t.Parallel()
	// 60 lines so we can scroll meaningfully.
	var lines []string
	for i := range 60 {
		lines = append(lines, "line "+intToString(i+1))
	}
	m, _ := editTestModel(t, strings.Join(lines, "\n"))
	// Scroll to line 30 in view mode.
	m.viewport.vp.SetYOffset(30)
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'i'})
	if m.viewerEdit.Line != 30 {
		t.Errorf("after `i` from yOff=30: cursor.Line = %d, want 30 (preserve page)", m.viewerEdit.Line)
	}
}

// TestParseYankCommand verifies the yank-command parser recognises every
// supported form.
func TestParseYankCommand(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in       string
		ok       bool
		from, to int
	}{
		{"y", true, 1, -1},
		{"yank", true, 1, -1},
		{"%y", true, 1, -1},
		{"y 5", true, 1, 5},
		{"y 1,3", true, 1, 3},
		{"1,3 y", true, 1, 3},
		{"1,3", false, 0, 0}, // missing the y suffix
		{"y abc", false, 0, 0},
		{"y", true, 1, -1},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			got, ok := parseYankCommand(tc.in)
			if ok != tc.ok {
				t.Errorf("ok = %v, want %v", ok, tc.ok)
			}
			if ok && (got.from != tc.from || got.to != tc.to) {
				t.Errorf("got (%d, %d), want (%d, %d)", got.from, got.to, tc.from, tc.to)
			}
		})
	}
}

// TestYankCommand_DispatchesClipboardCmd verifies that a `:y N` command
// surfaces a non-nil tea.Cmd from the editor's command runner — that's
// the clipboard-write that lands the yanked text in the user's system
// pasteboard.
func TestYankCommand_DispatchesClipboardCmd(t *testing.T) {
	t.Parallel()
	m, _ := editTestModel(t, "a\nb\nc\nd\n")
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: ':'})
	for _, r := range "y 2" {
		m = mustUpdate(t, m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("expected SetClipboard command from :y N")
	}
	if !strings.Contains(m.viewerStatus, "yanked 2 line") {
		t.Errorf("expected yank status, got %q", m.viewerStatus)
	}
}

// TestEdit_MacAndLinuxModifiers_WordAndLineMotion verifies the cross-
// platform navigation modifier set works in edit mode:
//
//	⌃← / ⌃→  (Linux/Win)  → previous/next word
//	⌥← / ⌥→  (Mac Option) → previous/next word
//	⌘← / ⌘→  (Mac Cmd)    → start/end of line
//
// All map onto moveCursorWordBack/Forward and moveCursorLineStart/End so
// users from any desktop platform get their muscle-memory bindings.
func TestEdit_MacAndLinuxModifiers_WordAndLineMotion(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		mod  tea.KeyMod
		code rune
		// expected cursor column after the move from col 0 on
		// "alpha beta gamma".
		wantCol int
	}{
		{"linux ctrl right → word", tea.ModCtrl, tea.KeyRight, 6},
		{"mac alt right → word", tea.ModAlt, tea.KeyRight, 6},
		{"mac super right → end", tea.ModSuper, tea.KeyRight, 16},
		{"mac meta right → end", tea.ModMeta, tea.KeyRight, 16},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, _ := editTestModel(t, "alpha beta gamma")
			m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'i'})
			m = mustUpdate(t, m, tea.KeyPressMsg{Code: tc.code, Mod: tc.mod})
			if m.viewerEdit.Col != tc.wantCol {
				t.Errorf("col = %d, want %d (%s)", m.viewerEdit.Col, tc.wantCol, tc.name)
			}
		})
	}
}

// TestEdit_EmacsReadlineShortcuts verifies the universal Unix-shell-style
// shortcuts the user would have in muscle memory regardless of terminal
// configuration:
//
//	Alt+b / Alt+f   word back / forward (default Option+arrow encoding
//	                in iTerm2 / Ghostty / Mac Terminal)
//	Ctrl+a / Ctrl+e line start / end (Emacs / bash readline)
//
// These are independent of the arrow-key modifier paths so users on
// terminals that swallow Cmd+arrow / Option+arrow still have working
// navigation.
func TestEdit_EmacsReadlineShortcuts(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		mod     tea.KeyMod
		code    rune
		startAt int
		wantCol int
	}{
		{"alt+f from start → word forward", tea.ModAlt, 'f', 0, 6},
		{"alt+b from end → word back", tea.ModAlt, 'b', 16, 11},
		{"ctrl+a from middle → line start", tea.ModCtrl, 'a', 8, 0},
		{"ctrl+e from middle → line end", tea.ModCtrl, 'e', 8, 16},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, _ := editTestModel(t, "alpha beta gamma")
			m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'i'})
			m.viewerEdit.Col = tc.startAt
			m = mustUpdate(t, m, tea.KeyPressMsg{Code: tc.code, Mod: tc.mod})
			if m.viewerEdit.Col != tc.wantCol {
				t.Errorf("col = %d, want %d", m.viewerEdit.Col, tc.wantCol)
			}
		})
	}
}

// TestEdit_MacCmdSaveUndoRedo verifies macOS Cmd-modifier shortcuts
// (Cmd+S, Cmd+Z, Cmd+Shift+Z) hit the same handlers as the Ctrl-prefixed
// equivalents.
func TestEdit_MacCmdSaveUndoRedo(t *testing.T) {
	t.Parallel()
	m, path := editTestModel(t, "hello\n")
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'i'})
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'A', Text: "A", Mod: tea.ModShift})
	if !m.viewerDirty {
		t.Fatal("setup: expected dirty")
	}
	// Cmd+S saves.
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 's', Mod: tea.ModSuper})
	if m.viewerDirty {
		t.Error("Cmd+S should clear dirty")
	}
	got, _ := readFile(t, path)
	if got != "Ahello\n" {
		t.Errorf("on-disk = %q, want %q", got, "Ahello\n")
	}
	// More edits then Cmd+Z undoes.
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'B', Text: "B", Mod: tea.ModShift})
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'z', Mod: tea.ModSuper})
	if m.viewerEdit.String() != "Ahello\n" {
		t.Errorf("Cmd+Z: buffer = %q, want %q", m.viewerEdit.String(), "Ahello\n")
	}
	// Cmd+Shift+Z redoes (re-applies the B insert).
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'z', Mod: tea.ModSuper | tea.ModShift})
	if m.viewerEdit.String() != "ABhello\n" {
		t.Errorf("Cmd+Shift+Z: buffer = %q, want %q", m.viewerEdit.String(), "ABhello\n")
	}
}

// readFile is a tiny helper so test bodies don't have to handle the err.
func readFile(t *testing.T, path string) (string, error) {
	t.Helper()
	b, err := osReadFile(path)
	return string(b), err
}

// osReadFile is indirected so the import block in this test file doesn't
// add the os package twice (the integration file already imports it).
var osReadFile = osReadFileImpl

// keepGoModFromShadowing is a sentinel call so go test treats this as a
// real test file in the package layout (filepath import otherwise unused
// in a no-op CI pipeline that strips comments).
var _ = filepath.Separator

// osReadFileImpl is the real implementation used by osReadFile above.
func osReadFileImpl(path string) ([]byte, error) {
	return os.ReadFile(path)
}
