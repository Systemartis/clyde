package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestHelp_HToggleOpensAndCloses verifies that pressing `h` flips the
// helpOpen flag, and pressing it again closes the overlay. The toggle
// is the single entry point users have to discover per-panel commands,
// so it must be reliable across mode transitions.
func TestHelp_HToggleOpensAndCloses(t *testing.T) {
	t.Parallel()
	m := NewModelWithConfig(DefaultConfig(), LayoutStack)

	if m.helpOpen {
		t.Fatalf("helpOpen should default to false")
	}

	m2, _ := m.handleKey(tea.KeyPressMsg{Code: 'h'})
	mm, ok := m2.(Model)
	if !ok {
		t.Fatalf("expected Model, got %T", m2)
	}
	if !mm.helpOpen {
		t.Fatalf("helpOpen should flip to true after h")
	}

	m3, _ := mm.handleKey(tea.KeyPressMsg{Code: 'h'})
	mmm := m3.(Model)
	if mmm.helpOpen {
		t.Errorf("helpOpen should flip back to false after second h")
	}
}

// TestHelp_RendersPanelSpecificOnly verifies that the help body contains
// the panel's own entries — including its jump-here key — and does NOT
// repeat global navigation keys (those live in the bottom status bar).
// Putting cross-panel commands on every panel made every help block
// look the same; users were asking "ok but what's specific to THIS
// panel?" so global keys are now intentionally omitted.
func TestHelp_RendersPanelSpecificOnly(t *testing.T) {
	t.Parallel()
	m := NewModelWithConfig(DefaultConfig(), LayoutStack)
	body := renderHelpBody(m.styles, PanelExplorer, 60)

	// Panel-specific row sample.
	if !strings.Contains(body, "copy") {
		t.Errorf("explorer help should mention y/Y copy bindings; got:\n%s", body)
	}
	// Jump-here shortcut for this panel — plain `e`, no Ctrl modifier.
	if !strings.Contains(body, "jump here") {
		t.Errorf("explorer help should mention its `e` jump-here key; got:\n%s", body)
	}
	// The OLD globals (settings, quit, layout) must NOT appear inside the
	// panel body — they're surfaced via the status bar.
	for _, gone := range []string{"settings", "quit", "layout mode"} {
		if strings.Contains(body, gone) {
			t.Errorf("global hint %q must not appear in panel help; got:\n%s", gone, body)
		}
	}
}

// TestHelp_BashJumpKey verifies that the bash panel's help advertises its
// new plain-letter jump shortcut (b). Same pattern for `u` (usage) and
// `c` (cache). The user explicitly asked for these as panel-specific
// reminders so the keys are discoverable inside the panel they jump to.
func TestHelp_BashJumpKey(t *testing.T) {
	t.Parallel()
	m := NewModelWithConfig(DefaultConfig(), LayoutStack)
	body := renderHelpBody(m.styles, PanelBash, 50)
	if !strings.Contains(body, "jump here") {
		t.Errorf("bash help missing 'jump here' line; got:\n%s", body)
	}
}

// TestHelp_PanelJumpKeys verifies that the new plain-letter jumps land
// the user on the right panel. The user explicitly asked for `b/u/c`
// (bash/usage/cache) as panel-specific shortcuts shown only on each
// panel's help; these keys complement the existing ⌃e/⌃a/⌃d Ctrl jumps.
func TestHelp_PanelJumpKeys(t *testing.T) {
	t.Parallel()
	cases := []struct {
		key  rune
		want PanelID
	}{
		{'b', PanelBash},
		{'u', PanelUsage},
		{'c', PanelCache},
		{'s', PanelServers},
	}
	for _, tc := range cases {
		m := NewModelWithConfig(DefaultConfig(), LayoutStack)
		// Force-enable the target panel — defaults gate Bash + Cache off.
		switch tc.want {
		case PanelBash:
			m.cfg.Panels.Bash.Enabled = true
		case PanelCache:
			m.cfg.Panels.Cache.Enabled = true
		}
		m2, _ := m.handleKey(tea.KeyPressMsg{Code: tc.key})
		mm := m2.(Model)
		if mm.focused != tc.want {
			t.Errorf("key %q: focused = %v, want %v", string(tc.key), mm.focused, tc.want)
		}
	}
}

// TestNowPanelNonSelectable verifies that PanelNow rejects setFocus
// silently — no exception, no partial state, just returns the model
// unchanged. PanelNow has no scroll, no actions, no content surface;
// landing focus on it would feel like a dead-end.
func TestNowPanelNonSelectable(t *testing.T) {
	t.Parallel()
	m := NewModelWithConfig(DefaultConfig(), LayoutStack)
	m.focused = PanelCalls

	m2 := m.setFocus(PanelNow)
	if m2.focused != PanelCalls {
		t.Errorf("setFocus(PanelNow) must be a no-op; focus moved from PanelCalls to %v", m2.focused)
	}
}

// TestNowClickPreservesFocus verifies that clicking inside PanelNow does
// NOT change focus to it. The mascot reaction itself is harder to
// observe through the public API (FSM has unexported pending state)
// but the focus invariant is the user-visible contract: their cursor
// stays where they last put it.
func TestNowClickPreservesFocus(t *testing.T) {
	t.Parallel()
	m := NewModelWithConfig(DefaultConfig(), LayoutStack)
	m.width = 80
	m.height = 30
	m.focused = PanelCalls

	bounds := m.buildPanelBounds()
	var nowB panelBounds
	for _, b := range bounds {
		if b.pid == PanelNow {
			nowB = b
			break
		}
	}
	if nowB.yMax == 0 {
		t.Skip("PanelNow not laid out at this size — skip")
	}

	clickX := (nowB.xMin + nowB.xMax) / 2
	clickY := (nowB.yMin + nowB.yMax) / 2
	next, _ := m.handleMouseClick(tea.MouseClickMsg{X: clickX, Y: clickY, Button: tea.MouseLeft})
	mm := next.(Model)

	if mm.focused != PanelCalls {
		t.Errorf("click on PanelNow must NOT change focus from PanelCalls; got %v", mm.focused)
	}
}

// TestHelp_FooterShowsGlobalsWhenHelpOpen verifies that the bottom
// status bar swaps from "h commands · [tabs]" to a list of cross-panel
// keybinds (tab/⌃l/⌃0/?/q) while help mode is open. Per-panel keys are
// already on screen inside each panel's body during help mode, so the
// footer can finally surface the globals that have nowhere else to live.
func TestHelp_FooterShowsGlobalsWhenHelpOpen(t *testing.T) {
	t.Parallel()
	m := NewModel()

	closed := stripANSI(renderStatusBar(m.styles, 130, false, "", nil, false, m.version))
	open := stripANSI(renderStatusBar(m.styles, 130, false, "", nil, true, m.version))

	if !strings.Contains(closed, "commands") {
		t.Errorf("closed-help footer should advertise `h commands`; got:\n%s", closed)
	}
	for _, want := range []string{"tab", "settings", "quit"} {
		if !strings.Contains(open, want) {
			t.Errorf("open-help footer must include global hint %q; got:\n%s", want, open)
		}
	}
	// Must not show the `h commands` hint while help is open — pressing
	// `h` again closes the overlay, which is a different action.
	if strings.Contains(open, "commands") {
		t.Errorf("open-help footer should drop `h commands`; got:\n%s", open)
	}
}

// TestHelp_FooterDropsRetiredHints verifies that the help-mode footer
// no longer advertises ⌃l (moved into the settings overlay) or ⌃0 (a
// stealth binding that didn't exist). Their phantom presence in the
// footer used to mislead users into trying keybinds that did nothing.
func TestHelp_FooterDropsRetiredHints(t *testing.T) {
	t.Parallel()
	m := NewModel()
	open := stripANSI(renderStatusBar(m.styles, 130, false, "", nil, true, m.version))

	for _, gone := range []string{"⌃l", "⌃0", "fold all"} {
		if strings.Contains(open, gone) {
			t.Errorf("help-mode footer must not contain %q (retired binding); got:\n%s", gone, open)
		}
	}
}

// TestClick_GatedDuringOverlay verifies that mouse clicks land nowhere
// when the settings or help overlay is open. The bug we fixed: clicking
// where explorer rendered (now hidden by an overlay) would still open a
// file in the viewer because the click handler trusted panelAtPos
// without checking which surface the user could actually see.
func TestClick_GatedDuringOverlay(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		set  func(*Model)
	}{
		{"settingsOpen", func(m *Model) { m.settingsOpen = true }},
		{"helpOpen", func(m *Model) { m.helpOpen = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := NewModel()
			m.width = 130
			m.height = 40
			m.bp = DetectBreakpoint(130)
			tc.set(&m)
			startFocus := m.focused

			next, _ := m.handleMouseClick(tea.MouseClickMsg{X: 5, Y: 5, Button: tea.MouseLeft})
			mm := next.(Model)
			if mm.focused != startFocus {
				t.Errorf("click during %s overlay must not change focus; was %v, got %v",
					tc.name, startFocus, mm.focused)
			}
			if mm.viewerActive {
				t.Errorf("click during %s overlay must not open the viewer", tc.name)
			}
		})
	}
}

// TestHelp_EscClosesOverlay verifies Esc dismisses the help overlay —
// the universal "back out" key. Without this, users who opened help
// with `h` had to remember to press `h` again instead of the natural
// Esc that closes every other modal in clyde.
func TestHelp_EscClosesOverlay(t *testing.T) {
	t.Parallel()
	m := NewModel()
	m.helpOpen = true

	next, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	mm := next.(Model)
	if mm.helpOpen {
		t.Errorf("Esc must close help; helpOpen still true")
	}
}

// TestHelp_BackspaceClosesOverlay verifies Backspace also dismisses
// help. Backspace is the secondary "go back" key in many TUIs;
// supporting it alongside Esc keeps the overlay easy to leave from
// any keyboard.
func TestHelp_BackspaceClosesOverlay(t *testing.T) {
	t.Parallel()
	m := NewModel()
	m.helpOpen = true

	next, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	mm := next.(Model)
	if mm.helpOpen {
		t.Errorf("Backspace must close help; helpOpen still true")
	}
}

// TestHelp_FooterContainsGlobalNav verifies the help-mode footer now
// surfaces the basic global navigation keys (arrows, enter, backspace,
// tab, esc) — they don't appear in any panel's per-panel help body
// because they apply everywhere, so the footer is their home.
func TestHelp_FooterContainsGlobalNav(t *testing.T) {
	t.Parallel()
	m := NewModel()
	open := stripANSI(renderStatusBar(m.styles, 200, false, "", nil, true, m.version))

	for _, want := range []string{"focus", "columns", "expand", "collapse", "tab", "session", "esc", "settings", "quit"} {
		if !strings.Contains(open, want) {
			t.Errorf("help-mode footer missing global nav hint %q; got:\n%s", want, open)
		}
	}
}

// TestStatusBarHeight_ExpandsWhenHelpOpen is the regression for the
// "all global keybinds crammed onto one cramped row" UX: when help is
// on the footer expands to a 4-row grouped block (dashes + nav row +
// panel row + modal row). Layout calls subtract this height from the
// terminal so panels behind the footer don't bleed under it.
func TestStatusBarHeight_ExpandsWhenHelpOpen(t *testing.T) {
	t.Parallel()
	if h := statusBarHeight(false); h != 2 {
		t.Errorf("normal-mode footer height = %d; want 2", h)
	}
	if h := statusBarHeight(true); h != 4 {
		t.Errorf("help-mode footer height = %d; want 4 (dashes + 3 grouped rows)", h)
	}
}

// TestHelp_FooterRendersThreeGroupedRows verifies the help-mode footer
// is rendered as a grouped multi-row block — one row per command
// category — instead of a single overflowing line. Counts newlines:
// dashes + nav + panel + modal = 4 rows = 3 newlines between them.
func TestHelp_FooterRendersThreeGroupedRows(t *testing.T) {
	t.Parallel()
	m := NewModel()
	footer := renderStatusBar(m.styles, 120, false, "", nil, true, m.version)
	got := strings.Count(footer, "\n") + 1
	if got != 4 {
		t.Errorf("help-mode footer rendered %d rows; want 4", got)
	}
}

// TestHelp_DoesNotTriggerInsideSettingsOverlay verifies that `h` while
// the settings overlay is open does NOT toggle helpOpen — settings
// captures all keys, including `h` for whatever its own purposes.
func TestHelp_DoesNotTriggerInsideSettingsOverlay(t *testing.T) {
	t.Parallel()
	m := NewModelWithConfig(DefaultConfig(), LayoutStack)
	m.settingsOpen = true

	m2, _ := m.handleKey(tea.KeyPressMsg{Code: 'h'})
	mm := m2.(Model)
	if mm.helpOpen {
		t.Errorf("h must not toggle help while settings overlay is open")
	}
}
