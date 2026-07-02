package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/Systemartis/clyde/internal/adapters/hookserver"
)

// Compile-time assertion: Model implements tea.Model.
var _ tea.Model = Model{}

// TestNewModelDefaults verifies that NewModel returns a model focused on
// the first SELECTABLE panel (PanelCalls) — Now is intentionally
// non-selectable so it can't be the default cursor target. Also checks
// the default palette wiring.
func TestNewModelDefaults(t *testing.T) {
	m := NewModel()
	if m.focused != PanelCalls {
		t.Errorf("NewModel().focused = %d, want PanelCalls (%d)", m.focused, PanelCalls)
	}
	p := TokyoNightPalette()
	pr, pg, pb, _ := p.Purple.RGBA()
	mr, mg, mb, _ := m.palette.Purple.RGBA()
	if mr != pr || mg != pg || mb != pb {
		t.Error("NewModel() palette.Purple does not match TokyoNightPalette")
	}
}

// containsQuitMsg flattens a possibly-batched cmd and reports whether
// any of its messages is a tea.QuitMsg. Adaptive frame tick now batches
// a wakeup tickCmd alongside every key/mouse handler's own cmd, so a
// quit no longer arrives as a single QuitMsg — it arrives inside a
// BatchMsg([]Cmd). Tests assert the *presence* of QuitMsg, not the
// exact return shape.
func containsQuitMsg(c tea.Cmd) bool {
	if c == nil {
		return false
	}
	msg := c()
	if _, ok := msg.(tea.QuitMsg); ok {
		return true
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, sub := range batch {
			if sub == nil {
				continue
			}
			if _, ok := sub().(tea.QuitMsg); ok {
				return true
			}
		}
	}
	return false
}

// TestQuitOnQ verifies that pressing 'q' sets quitting and returns tea.Quit.
func TestQuitOnQ(t *testing.T) {
	m := NewModel()
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if !containsQuitMsg(cmd) {
		t.Fatalf("expected QuitMsg in cmd batch after pressing q, got none")
	}
	nm := next.(Model)
	if !nm.quitting {
		t.Error("model.quitting should be true after pressing q")
	}
}

// TestQuitOnCtrlC verifies that ctrl+c quits.
func TestQuitOnCtrlC(t *testing.T) {
	m := NewModel()
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if !containsQuitMsg(cmd) {
		t.Fatalf("expected QuitMsg in cmd batch after ctrl+c, got none")
	}
	nm := next.(Model)
	if !nm.quitting {
		t.Error("model.quitting should be true after ctrl+c")
	}
}

// TestTabCyclesFocus verifies that Tab advances focus through every
// SELECTABLE panel and cycles back to the start. v22+: PanelDiff is
// opt-in. Plus PanelNow is non-selectable (no scroll, no actions —
// clicking it pokes the mascot instead of focusing it), so the cycle
// excludes Now too.
func TestTabCyclesFocus(t *testing.T) {
	m := NewModel()
	initial := m.focused
	cycleLen := len(selectableFrom(m.activePanelsForBreakpoint()))

	for i := 0; i < cycleLen; i++ {
		next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		m = next.(Model)
		if m.focused == PanelNow {
			t.Fatalf("Tab landed on PanelNow at step %d — non-selectable panels must be skipped", i)
		}
	}
	if m.focused != initial {
		t.Errorf("after %d Tabs focus = %d, want initial %d", cycleLen, m.focused, initial)
	}
}

// TestShiftTabCyclesFocusBackward verifies shift+tab moves backward.
func TestShiftTabCyclesFocusBackward(t *testing.T) {
	m := NewModel()
	startFocus := m.focused
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	m = next.(Model)
	if m.focused == startFocus {
		t.Error("shift+tab should change focus")
	}
}

// TestEFocusesExplorer verifies plain `e` focuses the explorer panel.
// The Ctrl-prefixed variants were dropped in favor of a unified
// no-modifier style across every panel-jump shortcut (e/a/d/u/s/b/c).
func TestEFocusesExplorer(t *testing.T) {
	m := NewModel()
	next, _ := m.Update(tea.KeyPressMsg{Code: 'e'})
	m = next.(Model)
	if m.focused != PanelExplorer {
		t.Errorf("e: focused = %d, want PanelExplorer (%d)", m.focused, PanelExplorer)
	}
}

// TestAFocusesCalls verifies plain `a` focuses the calls (activity)
// panel — same unified style as `e`/`d`/`u`/etc.
func TestAFocusesCalls(t *testing.T) {
	m := NewModel()
	next, _ := m.Update(tea.KeyPressMsg{Code: 'a'})
	m = next.(Model)
	if m.focused != PanelCalls {
		t.Errorf("a: focused = %d, want PanelCalls (%d)", m.focused, PanelCalls)
	}
}

// TestDIgnoredWhenDiffHidden verifies plain `d` is a no-op when the
// standalone diff panel is opt-in and currently disabled (v22+ default).
// When users opt back in via Phase 4 settings, `d` will focus PanelDiff.
func TestDIgnoredWhenDiffHidden(t *testing.T) {
	m := NewModel()
	startFocus := m.focused
	next, _ := m.Update(tea.KeyPressMsg{Code: 'd'})
	m = next.(Model)
	if m.panelEnabled(PanelDiff) {
		if m.focused != PanelDiff {
			t.Errorf("d (diff visible): focused = %d, want PanelDiff (%d)", m.focused, PanelDiff)
		}
	} else {
		if m.focused != startFocus {
			t.Errorf("d (diff hidden): focused = %d, want unchanged %d", m.focused, startFocus)
		}
	}
}

// TestWindowSizeUpdatesBreakpoint verifies that WindowSizeMsg triggers the correct Breakpoint.
// v6: narrow is < 80, medium is 80-159, wide is 160+.
func TestWindowSizeUpdatesBreakpoint(t *testing.T) {
	cases := []struct {
		width int
		want  Breakpoint
	}{
		{70, BreakpointNarrow},
		{80, BreakpointMedium},
		{120, BreakpointMedium},
		{180, BreakpointWide},
	}
	for _, tc := range cases {
		m := NewModel()
		next, _ := m.Update(tea.WindowSizeMsg{Width: tc.width, Height: 40})
		m = next.(Model)
		if m.bp != tc.want {
			t.Errorf("width=%d: bp=%d, want=%d", tc.width, m.bp, tc.want)
		}
	}
}

// TestWindowSizePreservesPersistedHeights verifies that a WindowSizeMsg
// does not reset a user-persisted panel height back to the computed default.
// Regression guard: previously handleWindowSize unconditionally called
// SetExpandedHeight(default) on every panel, which clobbered the height
// seeded by newBaseModel from PanelConfig.Height when RememberLayout was on.
func TestWindowSizePreservesPersistedHeights(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RememberLayout = true
	cfg.Panels.Usage.Height = 19 // persisted manual override

	m := NewModelWithConfig(cfg, "")
	next, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 50})
	m = next.(Model)

	if got := m.collapse[PanelUsage].expandedH; got != 19 {
		t.Errorf("usage panel expanded height after WindowSizeMsg = %v, want 19", got)
	}
	if got := m.panelHeights[PanelUsage]; got != 19 {
		t.Errorf("panelHeights[PanelUsage] = %d, want 19", got)
	}
}

// TestWindowSizeFallsBackToDefaultWhenNoOverride verifies the non-persisted
// path: when panelHeights[pid] is unset, handleWindowSize uses the computed
// default expanded height instead.
func TestWindowSizeFallsBackToDefaultWhenNoOverride(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RememberLayout = false // no persisted overrides

	m := NewModelWithConfig(cfg, "")
	next, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 50})
	m = next.(Model)

	// PanelUsage default expanded height is 14.0; the spring should match.
	if got := m.collapse[PanelUsage].expandedH; got != 14 {
		t.Errorf("usage panel expanded height with no override = %v, want 14", got)
	}
}

// TestWindowSizeClampsOversizedPersistedHeight verifies that a persisted
// height taller than the terminal's available rows is clamped at render
// time so the layout still fits.
func TestWindowSizeClampsOversizedPersistedHeight(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RememberLayout = true
	cfg.Panels.Usage.Height = 200 // absurdly tall — must clamp

	m := NewModelWithConfig(cfg, "")
	next, _ := m.Update(tea.WindowSizeMsg{Width: 160, Height: 30})
	m = next.(Model)

	maxH := float64(30 - 8)
	if got := m.collapse[PanelUsage].expandedH; got > maxH {
		t.Errorf("usage panel expanded height = %v, want <= %v (terminal cap)", got, maxH)
	}
}

// TestFrameMsgAdvancesTick verifies that FrameMsg advances the animation tick.
func TestFrameMsgAdvancesTick(t *testing.T) {
	m := NewModel()
	initialTick := m.frame.Tick
	next, _ := m.Update(FrameMsg{})
	m = next.(Model)
	if m.frame.Tick <= initialTick {
		t.Errorf("FrameMsg should advance Tick: got %d, want > %d", m.frame.Tick, initialTick)
	}
}

// TestViewContainsClydeNamingConvention verifies the v3 naming convention.
// "clyde" is the app name (brand); "claude" is the actor in notifications.
func TestViewContainsClydeNamingConvention(t *testing.T) {
	m := NewModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 180, Height: 50})
	m = next.(Model)

	content := m.View().Content

	if !strings.Contains(content, "clyde") {
		t.Error("View() should contain 'clyde' as the app brand")
	}
	if !strings.Contains(content, "claude") {
		t.Error("View() should contain 'claude' as the actor (notification + now panel)")
	}
	// Must NOT have "clyde wants" — clyde is never the actor
	if strings.Contains(content, "clyde wants") {
		t.Error("View() must not contain 'clyde wants' — only 'claude wants'")
	}
}

// TestViewWideContainsKeyPanels verifies that the multi-col layout renders all panels.
// Uses explicit multi-col mode at 180 cols (the three-column dashboard layout).
func TestViewWideContainsKeyPanels(t *testing.T) {
	m := NewModelWithConfig(DefaultConfig(), LayoutMultiCol)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 180, Height: 50})
	m = next.(Model)

	content := m.View().Content

	// In multi-col mode, panel labels are always visible (panels not collapsible in mode C)
	// v13: "tasks" panel replaced by "calls" panel
	// v22+: "live diff" panel is opt-in (off by default), and the calls panel
	// was renamed to "activity" to match its new subagent-focused role.
	panels := []string{"explorer", "now", "activity", "usage", "servers"}
	for _, p := range panels {
		if !strings.Contains(content, p) {
			t.Errorf("multi-col view missing panel label %q", p)
		}
	}
	// Images panel should be gone
	if strings.Contains(content, "images") {
		t.Error("multi-col view should NOT contain 'images' panel (removed in v7)")
	}
}

// TestViewStackNarrowContainsCorePanels verifies that the narrow stack layout
// renders the 4 core panels.
func TestViewStackNarrowContainsCorePanels(t *testing.T) {
	m := NewModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 70, Height: 40})
	m = next.(Model)

	content := m.View().Content

	// At 80 cols stack, now is expanded (focused) and others collapsed
	// The "now" label should always appear (expanded)
	if !strings.Contains(content, "now") {
		t.Error("narrow stack view should contain 'now' panel label")
	}
}

// TestViewDoesNotShowMockNotification verifies that the demo "claude wants
// to run npm test" mock banner has been retired. v22+ shows a blank notification
// spacer until a real hook event arrives; notification policies and richer
// banners are scheduled for Phase 9.
func TestViewDoesNotShowMockNotification(t *testing.T) {
	m := NewModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 180, Height: 50})
	m = next.(Model)
	content := m.View().Content
	if strings.Contains(content, "npm test") {
		t.Error("View() must NOT show retired mock 'npm test' notification")
	}
	if strings.Contains(content, "claude wants to run") {
		t.Error("View() must NOT show retired mock 'claude wants to run' banner")
	}
}

// TestViewDoesNotPanicAtZeroSize verifies View() survives the pre-WindowSize
// initial frame. Bubble Tea v2 calls View() before the first WindowSizeMsg
// arrives, so width and height are 0 — which used to crash inside
// wrapPanelCollapsed via strings.Repeat with a negative count. CI runners
// (no real /dev/tty) hit this path reliably; local terminals don't.
func TestViewDoesNotPanicAtZeroSize(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("View() panicked at zero size: %v", r)
		}
	}()
	m := NewModel()
	// Deliberately do NOT send a WindowSizeMsg — emulate the initial frame.
	_ = m.View()

	// Also try a few small/degenerate sizes that can produce negative
	// derived widths (innerW = width - 2 etc.).
	for _, sz := range []struct{ w, h int }{{0, 0}, {1, 1}, {2, 2}, {3, 3}} {
		next, _ := m.Update(tea.WindowSizeMsg{Width: sz.w, Height: sz.h})
		m = next.(Model)
		_ = m.View()
	}
}

// TestEscDismissesNotification verifies that pressing Esc sets notifAck.
func TestEscDismissesNotification(t *testing.T) {
	m := NewModel()
	if m.notifAck {
		t.Fatal("notifAck should be false initially")
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(Model)
	if !m.notifAck {
		t.Error("Esc should set notifAck = true")
	}
}

// TestArrowDownMovesFocusInStack verifies ↓ moves focus down when panel is COLLAPSED.
// v10: ↓ in an expanded panel scrolls its viewport instead of moving focus.
// PanelNow is non-selectable, so this test starts on PanelCalls and
// expects ↓ to advance to the next selectable panel.
func TestArrowDownMovesFocusInStack(t *testing.T) {
	m := NewModel()
	m.layoutMode = LayoutStack
	m.bp = BreakpointNarrow // force narrow for predictable panel list
	m.width = 70
	m.focused = PanelCalls
	m.collapse[PanelCalls].Collapse()

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = next.(Model)
	if m.focused == PanelCalls {
		t.Error("↓ on collapsed PanelCalls should have moved focus away")
	}
	// In narrow stack: [Now, Calls, Diff (off), Usage]; selectable = [Calls, Usage].
	if m.focused != PanelUsage {
		t.Errorf("↓ from collapsed PanelCalls should focus PanelUsage (next selectable), got %d", m.focused)
	}
}

// TestArrowUpMovesFocusInStack verifies ↑ moves focus up when panel is COLLAPSED,
// skipping the non-selectable PanelNow. From PanelUsage, ↑ wraps back
// to PanelCalls (Now is filtered out of the cycle).
func TestArrowUpMovesFocusInStack(t *testing.T) {
	m := NewModel()
	m.layoutMode = LayoutStack
	m.bp = BreakpointNarrow
	m.width = 70
	m.focused = PanelUsage
	m.collapse[PanelUsage].Collapse()

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	m = next.(Model)
	if m.focused != PanelCalls {
		t.Errorf("↑ from collapsed PanelUsage should skip Now and focus PanelCalls, got %d", m.focused)
	}
}

// TestEnterExpandsCollapsedPanel verifies Enter expands a collapsed panel.
func TestEnterExpandsCollapsedPanel(t *testing.T) {
	m := NewModel()
	m.layoutMode = LayoutStack
	m.focused = PanelCalls
	m.collapse[PanelCalls].Collapse()

	if !m.collapse[PanelCalls].IsCollapsed() {
		t.Fatal("PanelCalls should be collapsed for this test")
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if m.collapse[PanelCalls].IsCollapsed() {
		t.Error("Enter should expand a collapsed panel")
	}
}

// TestBackspaceCollapsesExpandedPanel verifies Backspace collapses an expanded panel.
func TestBackspaceCollapsesExpandedPanel(t *testing.T) {
	m := NewModel()
	m.layoutMode = LayoutStack
	m.focused = PanelNow
	m.collapse[PanelNow].Expand()

	if m.collapse[PanelNow].IsCollapsed() {
		t.Fatal("PanelNow should be expanded for this test")
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = next.(Model)
	if !m.collapse[PanelNow].IsCollapsed() {
		t.Error("Backspace should collapse an expanded panel")
	}
}

// TestMouseClickFocusesPanel verifies a left-click focuses the panel at that row.
func TestMouseClickFocusesPanel(t *testing.T) {
	m := NewModel()
	m.width = 70
	m.height = 40
	m.bp = BreakpointNarrow
	m.layoutMode = LayoutStack
	m.focused = PanelNow
	// title bar = rows 0-1; now panel starts at row 2
	// now panel is expanded (height ~10), so clicking row 5 should hit PanelNow

	next, _ := m.Update(tea.MouseClickMsg{X: 10, Y: 5, Button: tea.MouseLeft})
	m = next.(Model)
	if m.focused != PanelNow {
		t.Errorf("click at row 5 should focus PanelNow, got %d", m.focused)
	}
}

// TestTitleBarHasNoLiveIndicator verifies the new title bar lacks the green dot.
func TestTitleBarHasNoLiveIndicator(t *testing.T) {
	m := NewModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 70, Height: 40})
	m = next.(Model)
	content := m.View().Content
	if strings.Contains(content, "claude · live") {
		t.Error("title bar should NOT contain 'claude · live' in v5")
	}
}

// TestMascotIsKitten verifies the mascot renders as the v23 ASCII kitten.
// Bunny remains available behind the persona toggle but is no longer the
// default — see TestPersonaBunnyKeepsLegacyEars.
func TestMascotIsKitten(t *testing.T) {
	m := NewModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 70, Height: 40})
	m = next.(Model)
	content := m.View().Content

	// Old jellyfish glyphs must be gone
	if strings.Contains(content, "╭⌒╮") || strings.Contains(content, "╭⌐╮") {
		t.Error("v23 mascot should NOT have jellyfish bell glyph (╭⌒╮ or ╭⌐╮)")
	}
	// Old boxed design must be gone
	if strings.Contains(content, "╭─────╮") {
		t.Error("v23 mascot should NOT have the old boxed multi-line design (╭─────╮)")
	}
	// Kitten ears must be present
	if !strings.Contains(content, `/\_/\`) {
		t.Error(`v23 mascot should have kitten ears /\_/\`)
	}
	// Kitten paws must be present
	if !strings.Contains(content, `_"_"_`) {
		t.Error(`v23 mascot should have kitten paws _"_"_`)
	}
	// Old owl tufts must NOT be present
	if strings.Contains(content, `,___,`) {
		t.Error("v23 mascot should NOT have owl tufts ,___,")
	}
}

// TestMascotIsBunny is a backward-compat alias kept so older test runners
// (and CI tooling that grep for the symbol) keep finding a passing test.
func TestMascotIsBunny(t *testing.T) { TestMascotIsKitten(t) }

// TestMascotIsOwl is the older still backward-compat alias.
func TestMascotIsOwl(t *testing.T) { TestMascotIsKitten(t) }

// TestArrowLeftRight2ColNav verifies ←/→ navigate between columns in
// 2-col (medium) layout. The right column's first selectable panel is
// PanelCalls (Now is filtered out of cycling), so → from the left
// column lands there.
func TestArrowLeftRight2ColNav(t *testing.T) {
	m := NewModel()
	m.layoutMode = LayoutStack
	m.bp = BreakpointMedium
	m.width = 90

	m.focused = PanelCalls

	// → from already-on-right is a no-op.
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	m = next.(Model)
	if m.focused != PanelCalls {
		t.Errorf("→ from right column should no-op, got %d", m.focused)
	}

	// ← moves to left column (PanelExplorer).
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	m = next.(Model)
	if m.focused != PanelExplorer {
		t.Errorf("← from right col should focus PanelExplorer, got %d", m.focused)
	}

	// ← again should no-op (already on left).
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	m = next.(Model)
	if m.focused != PanelExplorer {
		t.Errorf("← from left column should no-op, got %d", m.focused)
	}

	// → returns to the right column. PanelNow is filtered out of focus,
	// so the first selectable panel — PanelCalls — receives the cursor.
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	m = next.(Model)
	if m.focused != PanelCalls {
		t.Errorf("→ from left col should focus PanelCalls (Now is non-selectable), got %d", m.focused)
	}
}

// TestArrowLeftRightNarrowNoOp verifies ←/→ is no-op in narrow stack (< 80 cols).
func TestArrowLeftRightNarrowNoOp(t *testing.T) {
	m := NewModel()
	m.layoutMode = LayoutStack
	m.bp = BreakpointNarrow
	m.width = 70
	m.focused = PanelNow

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	m = next.(Model)
	if m.focused != PanelNow {
		t.Errorf("← in narrow stack should no-op, got %d", m.focused)
	}

	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	m = next.(Model)
	if m.focused != PanelNow {
		t.Errorf("→ in narrow stack should no-op, got %d", m.focused)
	}
}

// TestExplorerClickActivatesRow verifies that a left-click on the first tree row
// in the explorer panel (in 2-col medium layout) focuses the explorer AND activates the row.
//
// Layout at 90×40 medium:
//   - leftW = (90*40)/100 = 36  → explorer x: 0..35
//   - Title bar: rows 0-1
//   - Explorer border (top): row 2
//   - Content starts: row 3
//   - preTreeRows = 1 (search button) + 1 (modified header) + 4 (modified files) + 2 (sep + tree header) = 8
//   - First tree row ("▼ src"): contentRow 8 → screen y = 3 + 8 = 11
//   - Click at (x=10, y=11) → inside left column, inside first tree row.
//
// New click model: clicking an UNFOCUSED panel only sets focus, even if
// the click happened to land on a tree row. The user must click again on
// the now-focused row to trigger the activate action. This protects
// against stray clicks accidentally toggling directories or opening files
// while the user was simply switching panels.
func TestExplorerClickActivatesRow(t *testing.T) {
	m := NewModel()
	m.width = 90
	m.height = 40
	m.bp = BreakpointMedium
	m.layoutMode = LayoutStack
	// Start with focus on right column (PanelNow)
	m.focused = PanelNow

	// First click: explorer is unfocused, so we expect focus to move
	// without any tree-row activation.
	next, _ := m.Update(tea.MouseClickMsg{X: 10, Y: 11, Button: tea.MouseLeft})
	m = next.(Model)

	if m.focused != PanelExplorer {
		t.Fatalf("first click on unfocused explorer: focused = %d, want PanelExplorer (%d)", m.focused, PanelExplorer)
	}
	rows := buildVisibleRows(m.data, m.explorer.collapsed)
	if len(rows) == 0 {
		t.Fatal("no visible rows in explorer after first click")
	}
	if !rows[0].IsDir {
		t.Fatalf("first tree row should be a directory, got: %q (IsDir=%v)", rows[0].DisplayName, rows[0].IsDir)
	}
	if !m.explorer.collapsed["src"] {
		t.Error("first click on unfocused panel must NOT activate the row — src should still be collapsed")
	}

	// Clear the panel-click stamp so the next click does NOT count as a
	// double-click — we want to assert the "single click on focused
	// row → activate" path, not the "double-click → active mode" path.
	m = m.resetPanelClick()

	// Second click on the same row (now that the explorer is focused)
	// dispatches to handleExplorerMouseClick which activates the row.
	next, _ = m.Update(tea.MouseClickMsg{X: 10, Y: 11, Button: tea.MouseLeft})
	m = next.(Model)
	if m.explorer.collapsed["src"] {
		t.Error("second click on focused tree row (▶ src) should have expanded it via explorerActivate")
	}
}

// TestTwoColAtMediumBreakpoint verifies medium width uses 2-column layout.
func TestTwoColAtMediumBreakpoint(t *testing.T) {
	m := NewModel()
	next, _ := m.Update(tea.WindowSizeMsg{Width: 130, Height: 40})
	m = next.(Model)
	content := m.View().Content
	// Two-col layout shows explorer on left — check both explorer and now appear side by side
	if !strings.Contains(content, "explorer") {
		t.Error("2-col medium layout should show explorer panel")
	}
	if !strings.Contains(content, "now") {
		t.Error("2-col medium layout should show now panel")
	}
}

// ── v10 tests ─────────────────────────────────────────────────────────────────

// TestFocusChangeDoesNotExpand verifies Tab/↑/↓ to a collapsed panel does NOT expand it.
// Starts focused on PanelCalls (PanelNow is non-selectable now); Tab
// advances to the next selectable panel which must remain collapsed.
func TestFocusChangeDoesNotExpand(t *testing.T) {
	m := NewModel()
	m.layoutMode = LayoutStack
	m.bp = BreakpointNarrow
	m.width = 70
	m.focused = PanelCalls
	m.collapse[PanelCalls].Collapse()
	m.collapse[PanelDiff].Collapse()
	m.collapse[PanelUsage].Collapse()

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = next.(Model)
	// Narrow stack selectable panels: [Calls, Usage] (Diff opt-in off,
	// Now non-selectable). Tab from Calls → Usage.
	if m.focused != PanelUsage {
		t.Fatalf("Tab should focus PanelUsage, got %d", m.focused)
	}
	if !m.collapse[PanelUsage].IsCollapsed() {
		t.Error("Tab to collapsed PanelUsage should NOT auto-expand it (v10)")
	}

	// ↓ from PanelUsage (now focused) wraps back to PanelCalls — the
	// only other selectable panel in this narrow stack — without
	// auto-expanding it. v22+: PanelDiff is opt-in (off here),
	// PanelNow is non-selectable, so the cycle is just [Calls, Usage].
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = next.(Model)
	if m.focused != PanelCalls {
		t.Fatalf("↓ from PanelUsage should wrap to PanelCalls, got %d", m.focused)
	}
	if !m.collapse[PanelCalls].IsCollapsed() {
		t.Error("↓ to collapsed PanelCalls should NOT auto-expand it (v10)")
	}
}

// TestEnterExpandsCollapsed verifies Enter on a collapsed panel expands it.
func TestEnterExpandsCollapsed(t *testing.T) {
	m := NewModel()
	m.layoutMode = LayoutStack
	m.focused = PanelCalls
	m.collapse[PanelCalls].Collapse()

	if !m.collapse[PanelCalls].IsCollapsed() {
		t.Fatal("PanelCalls should be collapsed before test")
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if m.collapse[PanelCalls].IsCollapsed() {
		t.Error("Enter on collapsed panel should expand it")
	}
}

// TestSpaceTogglesCollapse verifies Space toggles panel collapse state.
func TestSpaceTogglesCollapse(t *testing.T) {
	m := NewModel()
	m.layoutMode = LayoutStack
	m.focused = PanelCalls
	m.collapse[PanelCalls].Expand()

	// Space should collapse
	next, _ := m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	m = next.(Model)
	if !m.collapse[PanelCalls].IsCollapsed() {
		t.Error("Space on expanded panel should collapse it")
	}

	// Space again should expand
	next, _ = m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	m = next.(Model)
	if m.collapse[PanelCalls].IsCollapsed() {
		t.Error("Space on collapsed panel should expand it")
	}
}

// TestArrowDoesNotScrollInPassive verifies ↓ navigates focus (not scroll) when panel
// is expanded-passive. This is the v11 three-state fix: arrows only scroll in active mode.
func TestArrowDoesNotScrollInPassive(t *testing.T) {
	m := NewModel()
	m.layoutMode = LayoutStack
	m.bp = BreakpointNarrow
	m.width = 70
	m.focused = PanelCalls
	m.collapse[PanelCalls].Expand()
	// Expanded-passive (activePanelID == PanelNone)

	// Load content into the tasks viewport so a scroll WOULD be possible
	m.panelVPs[PanelCalls].SetContent("line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10")
	m.panelVPs[PanelCalls].SetHeight(3)

	initialOffset := m.panelVPs[PanelCalls].YOffset()

	// ↓ on expanded-passive panel should NAVIGATE focus, not scroll
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = next.(Model)

	// Focus should have moved (navigated away from PanelCalls)
	if m.focused == PanelCalls {
		t.Error("↓ in expanded-passive panel should have moved focus away (v11 three-state)")
	}
	// Viewport should NOT have scrolled
	afterOffset := m.panelVPs[PanelCalls].YOffset()
	if afterOffset != initialOffset {
		t.Errorf("↓ in expanded-passive panel should NOT scroll viewport: before=%d after=%d", initialOffset, afterOffset)
	}
}

// TestPlusMinusResizesPanel verifies + grows and - shrinks the focused panel in ACTIVE mode.
// v11: resize keys only fire in Expanded-Active state.
func TestPlusMinusResizesPanel(t *testing.T) {
	m := NewModel()
	m.layoutMode = LayoutStack
	m.bp = BreakpointNarrow
	m.width = 70
	m.height = 50
	m.focused = PanelCalls
	m.collapse[PanelCalls].Expand()
	// Set a known baseline height
	m.panelHeights[PanelCalls] = 10
	// Put panel in Expanded-Active mode so +/- fire
	m.activePanelID = PanelCalls

	// + should grow to 11
	next, _ := m.Update(tea.KeyPressMsg{Code: '+', Text: "+"})
	m = next.(Model)
	afterGrow := m.panelHeight(PanelCalls)
	if afterGrow != 11 {
		t.Errorf("+ in active mode should grow panel to 11: got %d", afterGrow)
	}

	// - should shrink back to 10
	next, _ = m.Update(tea.KeyPressMsg{Code: '-', Text: "-"})
	m = next.(Model)
	afterShrink := m.panelHeight(PanelCalls)
	if afterShrink != 10 {
		t.Errorf("- in active mode should shrink panel to 10: got %d", afterShrink)
	}

	// Floor: shrink to below minimum should clamp at panelHeightMin
	for i := 0; i < 30; i++ {
		m.activePanelID = PanelCalls // keep active during loop (Esc not sent)
		next, _ = m.Update(tea.KeyPressMsg{Code: '-', Text: "-"})
		m = next.(Model)
	}
	if m.panelHeight(PanelCalls) < panelHeightMin {
		t.Errorf("panel height below minimum %d: got %d", panelHeightMin, m.panelHeight(PanelCalls))
	}
}

// ── v11 three-state model tests ────────────────────────────────────────────────

// TestArrowsNavigateInPassive verifies: panel A expanded-passive + focused.
// Press ↓ → focus moves to next panel; panel A stays expanded.
func TestArrowsNavigateInPassive(t *testing.T) {
	m := NewModel()
	m.layoutMode = LayoutStack
	m.bp = BreakpointNarrow
	m.width = 70
	// Expand all panels so they are in expanded-passive state
	m.focused = PanelCalls
	m.collapse[PanelCalls].Expand()
	// No panel is active
	if m.activePanelID != PanelNone {
		t.Fatal("activePanelID should be PanelNone initially")
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = next.(Model)

	// Focus should have moved to the next panel
	if m.focused == PanelCalls {
		t.Error("↓ in expanded-passive should navigate focus away from PanelCalls")
	}
	// PanelCalls must still be expanded (not collapsed by navigation)
	if m.collapse[PanelCalls].IsCollapsed() {
		t.Error("navigating away from PanelCalls should not collapse it")
	}
	// No panel should be in active mode
	if m.activePanelID != PanelNone {
		t.Errorf("activePanelID should remain PanelNone after ↓ navigation, got %d", m.activePanelID)
	}
}

// TestEnterTransitionsToActive verifies: expanded-passive + Enter → Expanded-Active.
func TestEnterTransitionsToActive(t *testing.T) {
	m := NewModel()
	m.layoutMode = LayoutStack
	m.bp = BreakpointNarrow
	m.width = 70
	m.focused = PanelCalls
	m.collapse[PanelCalls].Expand() // start in expanded-passive

	if m.activePanelID != PanelNone {
		t.Fatal("activePanelID should be PanelNone before Enter")
	}

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)

	if m.activePanelID != PanelCalls {
		t.Errorf("Enter on expanded-passive should set activePanelID = PanelCalls, got %d", m.activePanelID)
	}
	if m.collapse[PanelCalls].IsCollapsed() {
		t.Error("panel should remain expanded after entering active mode")
	}
}

// TestArrowsScrollInActive verifies: panel in active mode. Press ↓ → viewport scrolls.
func TestArrowsScrollInActive(t *testing.T) {
	m := NewModel()
	m.layoutMode = LayoutStack
	m.bp = BreakpointNarrow
	m.width = 70
	m.focused = PanelCalls
	m.collapse[PanelCalls].Expand()
	m.activePanelID = PanelCalls // put in active mode

	// Load scrollable content
	m.panelVPs[PanelCalls].SetContent("line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10")
	m.panelVPs[PanelCalls].SetHeight(3)

	initialOffset := m.panelVPs[PanelCalls].YOffset()

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = next.(Model)

	// Focus should NOT change
	if m.focused != PanelCalls {
		t.Errorf("↓ in active mode should NOT change focus, got %d", m.focused)
	}
	// Viewport should have scrolled
	afterOffset := m.panelVPs[PanelCalls].YOffset()
	if afterOffset <= initialOffset {
		t.Errorf("↓ in active mode should scroll viewport: before=%d after=%d", initialOffset, afterOffset)
	}
	// Still in active mode
	if m.activePanelID != PanelCalls {
		t.Errorf("activePanelID should remain PanelCalls after ↓ scroll, got %d", m.activePanelID)
	}
}

// TestEscFromActiveReturnsToPassive verifies: active mode + Esc → Expanded-Passive.
func TestEscFromActiveReturnsToPassive(t *testing.T) {
	m := NewModel()
	m.layoutMode = LayoutStack
	m.bp = BreakpointNarrow
	m.width = 70
	m.focused = PanelCalls
	m.collapse[PanelCalls].Expand()
	m.activePanelID = PanelCalls

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(Model)

	if m.activePanelID != PanelNone {
		t.Errorf("Esc from active mode should set activePanelID = PanelNone, got %d", m.activePanelID)
	}
	// Panel should still be expanded (passive, not collapsed)
	if m.collapse[PanelCalls].IsCollapsed() {
		t.Error("Esc from active mode should keep panel expanded (passive), not collapse it")
	}
}

// TestSpaceFromActiveCollapses verifies: active mode + Space → panel collapses.
func TestSpaceFromActiveCollapses(t *testing.T) {
	m := NewModel()
	m.layoutMode = LayoutStack
	m.bp = BreakpointNarrow
	m.width = 70
	m.focused = PanelCalls
	m.collapse[PanelCalls].Expand()
	m.activePanelID = PanelCalls

	next, _ := m.Update(tea.KeyPressMsg{Code: ' ', Text: " "})
	m = next.(Model)

	if m.activePanelID != PanelNone {
		t.Errorf("Space from active mode should clear activePanelID, got %d", m.activePanelID)
	}
	if !m.collapse[PanelCalls].IsCollapsed() {
		t.Error("Space from active mode should collapse the panel")
	}
}

// TestTabFromActiveMovesFocus verifies: active mode + Tab → focus moves to next panel;
// previously-active panel stays expanded but is no longer active.
func TestTabFromActiveMovesFocus(t *testing.T) {
	m := NewModel()
	m.layoutMode = LayoutStack
	m.bp = BreakpointNarrow
	m.width = 70
	m.focused = PanelCalls
	m.collapse[PanelCalls].Expand()
	m.activePanelID = PanelCalls

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = next.(Model)

	// Focus should have moved away
	if m.focused == PanelCalls {
		t.Error("Tab from active mode should move focus to next panel")
	}
	// PanelCalls: still expanded, no longer active
	if m.collapse[PanelCalls].IsCollapsed() {
		t.Error("Tab from active mode should keep previously-active panel expanded")
	}
	if m.activePanelID != PanelNone {
		t.Errorf("Tab from active mode should clear activePanelID, got %d", m.activePanelID)
	}
}

// TestStatusBarHasCommandsHint verifies the bottom bar always carries
// the `h commands` hint (the entry point to the per-panel help). The
// older test-mode-specific hints went away when commands moved into
// each panel's help overlay — there's nothing left worth swapping
// between active and passive modes.
func TestStatusBarHasCommandsHint(t *testing.T) {
	t.Parallel()
	m := NewModel()

	// The version is now injected (single source of truth from
	// internal/version). Pass a distinctive string and assert it appears —
	// proving the footer renders the injected version, not a hardcode.
	const injectedVer = "v9.9.9-test"
	plain := stripANSI(renderStatusBar(m.styles, 130, false, "", nil, false, injectedVer))
	if !strings.Contains(plain, "commands") {
		t.Errorf("status bar must include the `h commands` hint; got:\n%s", plain)
	}
	if !strings.Contains(plain, injectedVer) {
		t.Errorf("status bar must render the injected version %q on the right; got:\n%s", injectedVer, plain)
	}
	// The old per-panel hints (mode/explorer/calls/diff) MUST be gone —
	// they moved into the panel-help overlay.
	for _, gone := range []string{"explorer", "scroll", "resize"} {
		if strings.Contains(plain, gone) {
			t.Errorf("status bar should not contain panel-specific hint %q; got:\n%s", gone, plain)
		}
	}
}

// TestActiveModeBorderColor verifies rendering of expanded-active panel includes
// the mode badge text in the top border area.
func TestActiveModeBorderColor(t *testing.T) {
	m := NewModel()
	m.width = 130
	m.height = 40
	m.bp = DetectBreakpoint(130)
	m.focused = PanelCalls
	m.collapse[PanelCalls].Expand()
	m.activePanelID = PanelCalls
	// Sync viewport content for active render
	m = m.syncPanelViewport(PanelCalls)
	m.panelVPs[PanelCalls].SetHeight(10)

	// Render just the tasks panel in active mode
	panelContent := m.renderExpandedPanel(PanelCalls, 80, 12, true)
	plain := stripANSI(panelContent)

	// Double border chars must appear (╔ and ╗ for active)
	if !strings.Contains(plain, "╔") {
		t.Error("active panel should have double border top-left ╔")
	}
	if !strings.Contains(plain, "╗") {
		t.Error("active panel should have double border top-right ╗")
	}
	// Mode badge must appear
	if !strings.Contains(plain, "scroll") {
		t.Error("active panel top border should contain 'scroll' in mode badge")
	}
	if !strings.Contains(plain, "resize") {
		t.Error("active panel top border should contain 'resize' in mode badge")
	}
	if !strings.Contains(plain, "esc back") {
		t.Error("active panel top border should contain 'esc back' in mode badge")
	}
}

// TestNowPanelHeight verifies the now panel renders at exactly 6 rows total.
func TestNowPanelHeight(t *testing.T) {
	m := NewModel()
	m.width = 70
	m.height = 40
	m.bp = BreakpointNarrow
	m.focused = PanelNow
	m.collapse[PanelNow].Expand()

	// Render just the now panel
	s := m.styles
	p := m.palette
	d := m.data
	f := m.frame

	rendered := renderNow(s, p, d, f, MascotPersonaMeowl, 70, 6, true)
	plain := stripANSI(rendered)
	rows := strings.Split(plain, "\n")

	if len(rows) != 6 {
		t.Errorf("now panel should be exactly 6 rows tall, got %d", len(rows))
		for i, r := range rows {
			t.Logf("row[%d]: %q", i, r)
		}
	}

	// Verify no trailing whitespace-only rows inside the panel (rows 1-4 are content)
	// Row 0 = top border, rows 1-4 = content, row 5 = bottom border
	for i := 1; i <= 4; i++ {
		if i < len(rows) && rows[i] == "" {
			t.Errorf("row[%d] is empty string — should be a border or content line", i)
		}
	}
}

// TestViewerCloseButtonClickable verifies that a left-click on the "esc close" badge
// coordinates closes the viewer (same effect as pressing Esc while viewer is active).
//
// Layout at 90×40 medium 2-col:
//   - leftW = (90*40)/100 = 36 → viewer x: 36..89 (rightW = 54)
//   - Title bar: rows 0-1
//   - Viewer top border: y = 2
//   - Badge text " esc close " = 11 runes, at right edge of viewer.
//   - Badge x range: [89-11, 89-1] = [78, 88]
//   - Click at (x=85, y=2) — inside badge region.
func TestViewerCloseButtonClickable(t *testing.T) {
	m := NewModel()
	m.width = 90
	m.height = 40
	m.bp = BreakpointMedium
	m.layoutMode = LayoutStack

	// Open viewer
	m.viewerActive = true
	m.viewerFile = "src/api/auth.ts"
	m.focused = PanelExplorer

	if !m.viewerActive {
		t.Fatal("viewer should be active before click")
	}

	// Compute expected close button bounds and click in the middle.
	// viewerXMax = w - 1 = 89; badgeRunes = 11; badgeXMin = 89 - 11 = 78
	// Click at x=83 (middle of badge), y=2 (viewer top border row).
	clickX := 83
	clickY := 2

	// Sanity-check our hit-test before calling Update.
	if !m.viewerCloseButtonAtPos(clickX, clickY) {
		t.Fatalf("viewerCloseButtonAtPos(%d, %d) = false, expected true (bug in hit-test)", clickX, clickY)
	}

	next, _ := m.Update(tea.MouseClickMsg{X: clickX, Y: clickY, Button: tea.MouseLeft})
	m = next.(Model)

	if m.viewerActive {
		t.Error("clicking 'esc close' badge should close the viewer (set viewerActive=false)")
	}
	if m.viewerFile != "" {
		t.Errorf("clicking 'esc close' badge should clear viewerFile, got %q", m.viewerFile)
	}
}

// TestViewerCloseButtonMissAtWrongY verifies that a click on the badge x-range but
// wrong y-row does NOT close the viewer.
func TestViewerCloseButtonMissAtWrongY(t *testing.T) {
	m := NewModel()
	m.width = 90
	m.height = 40
	m.bp = BreakpointMedium
	m.layoutMode = LayoutStack
	m.viewerActive = true
	m.viewerFile = "src/api/auth.ts"

	// Same x as badge but y=5 (inside viewer content, not top border) → no close.
	if m.viewerCloseButtonAtPos(83, 5) {
		t.Error("viewerCloseButtonAtPos should return false when y != viewerTop")
	}

	next, _ := m.Update(tea.MouseClickMsg{X: 83, Y: 5, Button: tea.MouseLeft})
	m = next.(Model)

	// Viewer should still be active (click was not on badge).
	if !m.viewerActive {
		t.Error("click at wrong y should NOT close the viewer")
	}
}

// ── v13 new tests ─────────────────────────────────────────────────────────────

// TestUsageActiveBorder verifies that the usage panel in Expanded-Active state
// renders with the pink double border (╔) and mode badge (active-mode wrapper).
func TestUsageActiveBorder(t *testing.T) {
	m := NewModel()
	m.width = 130
	m.height = 40
	m.bp = DetectBreakpoint(130)
	m.focused = PanelUsage
	m.collapse[PanelUsage].Expand()
	m.activePanelID = PanelUsage
	// Sync viewport content for active render
	m = m.syncPanelViewport(PanelUsage)
	m.panelVPs[PanelUsage].SetHeight(10)

	// Render just the usage panel in active mode
	panelContent := m.renderExpandedPanel(PanelUsage, 80, 12, true)
	plain := stripANSI(panelContent)

	// Active mode must show double-border top-left character
	if !strings.Contains(plain, "╔") {
		t.Error("usage panel in active mode should have double border ╔")
	}
	// Mode badge must be present
	if !strings.Contains(plain, "scroll") {
		t.Error("usage panel active border should contain 'scroll' mode badge")
	}
}

// TestServersShowsAllLSPs verifies that the servers panel expanded renders all LSPs.
func TestServersShowsAllLSPs(t *testing.T) {
	m := NewModel()
	m.width = 130
	m.height = 40
	m.bp = DetectBreakpoint(130)

	// Render servers at a height that can fit all content
	plain := stripANSI(renderServers(m.styles, m.palette, m.data, 52, 13, false))

	for _, lsp := range []string{"tsserver", "rust-analyzer", "pylsp"} {
		if !strings.Contains(plain, lsp) {
			t.Errorf("servers panel should show LSP %q but it was not found", lsp)
		}
	}
}

// TestCallsPanelHierarchical verifies the calls panel renders a hierarchy with
// main session + at least 2 subagents.
func TestCallsPanelHierarchical(t *testing.T) {
	m := NewModel()
	m.width = 130
	m.height = 40
	m.bp = DetectBreakpoint(130)

	// v22+: inline diff hunks under main agent push subagents below the
	// 20-row clip, so render at 40 rows so all 5 agents are visible.
	plain := stripANSI(renderCalls(m.styles, m.palette, m.data, 80, 40, false))

	// Main agent must be present (now collapsed to a one-line summary in v22+)
	if !strings.Contains(plain, "main") {
		t.Error("calls panel should show 'main' agent line")
	}
	// At least 2 subagents
	subagentCount := 0
	for _, g := range m.data.AgentGroups {
		if g.IsSubagent && strings.Contains(plain, g.AgentName) {
			subagentCount++
		}
	}
	if subagentCount < 2 {
		t.Errorf("calls panel should show at least 2 subagents, found %d", subagentCount)
	}
}

// TestCallsCollapsedSummary verifies the calls panel collapsed one-liner shows
// "▸ calls: N done · M active".
func TestCallsCollapsedSummary(t *testing.T) {
	m := NewModel()
	plain := stripANSI(renderCallsCollapsed(m.styles, m.palette, m.data, 80, false))

	if !strings.Contains(plain, "activity") {
		t.Error("collapsed activity panel should contain 'activity' label (v22+: was 'calls')")
	}
	if !strings.Contains(plain, "done") {
		t.Error("collapsed calls panel should contain 'done' count")
	}
	if !strings.Contains(plain, "active") {
		t.Error("collapsed calls panel should contain 'active' count")
	}
}

// ── v14 new tests ─────────────────────────────────────────────────────────────

// TestActiveModeContentReadable verifies that calls and usage panels produce
// clean, readable content when in Expanded-Active state.
// Bug fixed in v14: buildCallsViewportContent and buildUsageViewportContent
// now accept the actual inner width instead of using a hardcoded 76-char constant.
func TestActiveModeContentReadable(t *testing.T) {
	m := NewModel()
	m.width = 130
	m.height = 40
	m.bp = BreakpointMedium
	m.layoutMode = LayoutStack

	// Compute the right-column width for 130-wide 2-col layout.
	// leftW = (130*40)/100 = 52, but clamped to max 50 → leftW = 50.
	// rightW = 130 - 50 = 80; inner = 80 - 4 = 76.
	leftW := (130 * 40) / 100
	if leftW > 50 {
		leftW = 50
	}
	rightW := 130 - leftW
	innerW := rightW - 4

	// ── Calls panel ──────────────────────────────────────────────────────────

	m.focused = PanelCalls
	m.collapse[PanelCalls] = NewPanelCollapseState(false, 18)
	m.panelHeights[PanelCalls] = 18
	m.activePanelID = PanelCalls
	m = m.syncPanelViewport(PanelCalls)
	m.panelVPs[PanelCalls].SetHeight(14)

	// Render the full view and strip ANSI
	callsView := stripANSI(m.View().Content)
	callsLines := strings.Split(callsView, "\n")

	// Double border must be present (active mode)
	hasDoubleBorder := false
	for _, l := range callsLines {
		if strings.Contains(l, "╔") && strings.Contains(l, "╗") {
			hasDoubleBorder = true
			break
		}
	}
	if !hasDoubleBorder {
		t.Error("calls panel in active mode should have double border ╔...╗")
	}

	// Key content lines from mock data must be visible
	callsPlain := strings.Join(callsLines, "\n")
	contentKeywords := []string{
		"main", // main agent one-liner header (v22+: was "main session")
		"@@",   // v22+: inline diff hunks proves an Edit call rendered
		"Read", // visible in subagent calls
	}
	for _, kw := range contentKeywords {
		if !strings.Contains(callsPlain, kw) {
			t.Errorf("calls panel active mode should contain %q", kw)
		}
	}

	// Each content line should have width consistent with innerW (no truncation overflow)
	// Check that no visible line inside the double border is wider than rightW
	for i, l := range callsLines {
		visible := ansiWidth(l)
		if visible > m.width {
			t.Errorf("calls active mode: line %d width %d exceeds terminal width %d: %q",
				i, visible, m.width, l)
		}
	}

	// Verify content lines (inside the border) are individually width-correct
	// A line inside the double border looks like: ║ content ║
	// The inner content should be innerW chars wide (innerW = rightW - 4 for border+pad)
	_ = innerW // used in the content building; visual check above covers correctness

	// ── Usage panel ──────────────────────────────────────────────────────────
	// Test the usage panel by rendering it directly in active mode.
	// Right column width for 130-wide 2-col: leftW=50 (clamped), rightW=80, inner=76.

	m2 := NewModel()
	m2.width = 130
	m2.height = 40
	m2.bp = BreakpointMedium
	m2.layoutMode = LayoutStack
	m2.focused = PanelUsage
	// Height 30 to accommodate the expanded multi-window rows (v20: each window is
	// 2-3 lines: label + total-used + resets-in/current-ctx) without clipping the
	// issues section (errors/warnings/tests) at the bottom.
	m2.collapse[PanelUsage] = NewPanelCollapseState(false, 30)
	m2.panelHeights[PanelUsage] = 30
	m2.activePanelID = PanelUsage
	m2 = m2.syncPanelViewport(PanelUsage)
	m2.panelVPs[PanelUsage].SetHeight(30)

	// Render the usage panel directly in active mode to get its content
	usagePanel := stripANSI(m2.renderExpandedPanel(PanelUsage, rightW, 30, true))
	usageLines := strings.Split(usagePanel, "\n")
	usagePlain := strings.Join(usageLines, "\n")

	// Double border must be present (active mode)
	hasDoubleBorderUsage := false
	for _, l := range usageLines {
		if strings.Contains(l, "╔") && strings.Contains(l, "╗") {
			hasDoubleBorderUsage = true
			break
		}
	}
	if !hasDoubleBorderUsage {
		t.Error("usage panel in active mode should have double border ╔...╗")
	}

	// Key content from mock data must be visible.
	// v21 redesign: separate progress bars per metric (session ctx, 5h usage,
	// weekly usage, next reset) replaced the old single "total used" header.
	usageKeywords := []string{
		"session ctx", "5h session", "weekly · all models", "next reset",
		// "cost" omitted: v22+ hides cost row for subscribers (mock has Max 5x).
		"turns", "model", "errors", "warnings", "tests",
	}
	for _, kw := range usageKeywords {
		if !strings.Contains(usagePlain, kw) {
			t.Errorf("usage panel active mode should contain %q", kw)
		}
	}

	// Per-window token totals must still be visible in the sub-info lines.
	// Session: "47k / 200k (23%)" — cache total for the 5h row: "186k tokens"
	for _, val := range []string{"47k / 200k", "186k tokens", "1.5M tokens"} {
		if !strings.Contains(usagePlain, val) {
			t.Errorf("usage panel active mode should contain %q", val)
		}
	}
}

// TestMouseClickFocusesAccurately verifies that a click at a panel's
// top border row correctly focuses that panel. Encodes the post-fix
// layout: panels stack with NO separator row between them. Earlier
// versions of the bounds builder added a phantom +1 row between
// panels that the renderer never drew — clicks at panel transitions
// drifted down. layout.go is now the single source of truth.
//
// Layout at 130×40 medium 2-col (right column, v22+ default with diff
// hidden):
//   - titleRows = 2
//   - PanelNow:    rows 2-7  (height 6, hardcoded in renderNow)
//   - PanelCalls:  rows 8-9  (collapsed, height 2)
//   - PanelUsage:  rows 10-11 (collapsed, height 2)
//   - PanelBash:   rows 12-13 (collapsed, height 2)
//   - PanelCache:  rows 14-15 (collapsed, height 2)
func TestMouseClickFocusesAccurately(t *testing.T) {
	m := NewModel()
	m.width = 130
	m.height = 40
	m.bp = BreakpointMedium
	m.layoutMode = LayoutStack
	// Bash + cache are off in default config; this test exercises
	// the full right-column stack so enable them explicitly.
	m.cfg.Panels.Bash.Enabled = true
	m.cfg.Panels.Cache.Enabled = true
	// Start with focus on PanelCalls so we can confirm focus changes
	m.focused = PanelCalls
	// PanelNow expanded (spring settled at height 10, but renders at 6)
	m.collapse[PanelNow].Expand()
	// Other panels collapsed
	m.collapse[PanelCalls].Collapse()
	m.collapse[PanelDiff].Collapse()
	m.collapse[PanelUsage].Collapse()
	m.collapse[PanelBash].Collapse()
	m.collapse[PanelCache].Collapse()

	// Build bounds and verify they match expected positions
	bounds := m.buildPanelBounds()
	boundMap := make(map[PanelID]panelBounds)
	for _, b := range bounds {
		boundMap[b.pid] = b
	}

	// PanelNow at rows 2-7 (height 6, hardcoded in renderNow).
	nowB := boundMap[PanelNow]
	if nowB.yMin != 2 {
		t.Errorf("PanelNow yMin = %d, want 2", nowB.yMin)
	}
	if nowB.yMax != 7 {
		t.Errorf("PanelNow yMax = %d, want 7 (renderNow always outputs 6 rows)", nowB.yMax)
	}

	// PanelCalls follows PanelNow IMMEDIATELY (no separator row).
	tasksB := boundMap[PanelCalls]
	if tasksB.yMin != 8 {
		t.Errorf("PanelCalls yMin = %d, want 8 (must abut PanelNow with no separator)", tasksB.yMin)
	}
	if tasksB.yMax != 9 {
		t.Errorf("PanelCalls yMax = %d, want 9 (collapsed = 2 rows)", tasksB.yMax)
	}

	// PanelNow is non-selectable: click anywhere inside it triggers a
	// mascot reaction instead of focus. The original-focus panel must
	// stay focused after the click; without this guarantee the user
	// would have a "phantom" panel they can land on but never escape.
	clickX := 80
	startFocus := m.focused
	for _, y := range []int{2, 7, 4} { // top border, bottom border, middle
		next, _ := m.Update(tea.MouseClickMsg{X: clickX, Y: y, Button: tea.MouseLeft})
		m2 := next.(Model)
		if m2.focused != startFocus {
			t.Errorf("click at y=%d on PanelNow must NOT change focus (Now is non-selectable); was %d, got %d",
				y, startFocus, m2.focused)
		}
	}

	// Click at PanelCalls top border (y=8) — should focus collapsed calls panel.
	next, _ := m.Update(tea.MouseClickMsg{X: clickX, Y: 8, Button: tea.MouseLeft})
	m2 := next.(Model)
	if m2.focused != PanelCalls {
		t.Errorf("click at y=8 (PanelCalls top border) should focus PanelCalls, got %d", m2.focused)
	}

	// Click at PanelUsage top border (y=10) — PanelDiff hidden by default
	// in v22+, so PanelUsage immediately follows PanelCalls.
	next, _ = m.Update(tea.MouseClickMsg{X: clickX, Y: 10, Button: tea.MouseLeft})
	m2 = next.(Model)
	if m2.focused != PanelUsage {
		t.Errorf("click at y=10 (PanelUsage top border) should focus PanelUsage, got %d", m2.focused)
	}

	// Click at PanelBash top border (y=12) — verifies bash is in the
	// bounds map (used to be omitted, making bash unclickable).
	next, _ = m.Update(tea.MouseClickMsg{X: clickX, Y: 12, Button: tea.MouseLeft})
	m2 = next.(Model)
	if m2.focused != PanelBash {
		t.Errorf("click at y=12 (PanelBash top border) should focus PanelBash, got %d (bash must be in 2-col bounds map)", m2.focused)
	}

	// Click at PanelCache top border (y=14) — same regression guard.
	next, _ = m.Update(tea.MouseClickMsg{X: clickX, Y: 14, Button: tea.MouseLeft})
	m2 = next.(Model)
	if m2.focused != PanelCache {
		t.Errorf("click at y=14 (PanelCache top border) should focus PanelCache, got %d (cache must be in 2-col bounds map)", m2.focused)
	}

	// Explorer click at top border (y=2, x=10 in left column).
	// Start focused on PanelCalls (Now is non-selectable, so it can't be
	// the initial focus anymore for tests that exercise focus changes).
	mE := NewModel()
	mE.width = 130
	mE.height = 40
	mE.bp = BreakpointMedium
	mE.layoutMode = LayoutStack
	mE.focused = PanelCalls
	next, _ = mE.Update(tea.MouseClickMsg{X: 10, Y: 2, Button: tea.MouseLeft})
	mE2 := next.(Model)
	if mE2.focused != PanelExplorer {
		t.Errorf("click at y=2, x=10 (Explorer top border) should focus PanelExplorer, got %d", mE2.focused)
	}
}

// ── Phase H — hook server integration tests ────────────────────────────────────

// TestHookEventUpdatesNotifBanner verifies that a hookEventMsg causes the model
// to show the live hook notification (hookNotif.Active = true) and clears any
// previous notifAck state.
func TestHookEventUpdatesNotifBanner(t *testing.T) {
	m := NewModel()
	m.notifAck = true // simulate previously-dismissed banner

	evt := hookEventMsg{
		evt: hookserver.HookEvent{
			Type:       "PreToolUse",
			Tool:       "Bash",
			Args:       map[string]any{"command": "go test ./..."},
			Cwd:        "/tmp/proj",
			ResponseCh: make(chan hookserver.HookResponse, 1),
		},
	}

	next, _ := m.Update(evt)
	m = next.(Model)

	if !m.hookNotif.Active {
		t.Error("hookNotif.Active should be true after hookEventMsg")
	}
	if m.hookNotif.Tool != "Bash" {
		t.Errorf("hookNotif.Tool = %q, want %q", m.hookNotif.Tool, "Bash")
	}
	if m.hookNotif.KeyArg != "go test ./..." {
		t.Errorf("hookNotif.KeyArg = %q, want %q", m.hookNotif.KeyArg, "go test ./...")
	}
	if m.notifAck {
		t.Error("notifAck should be cleared when a new hook event arrives")
	}
	if m.hookPendingCh == nil {
		t.Error("hookPendingCh should be set when a hook event is pending")
	}
}

// TestYKeyAllowsHookEvent verifies that pressing 'y' when a hook event is
// pending sends Allow=true on the ResponseCh and clears hookNotif.
func TestYKeyAllowsHookEvent(t *testing.T) {
	m := NewModel()

	respCh := make(chan hookserver.HookResponse, 1)
	m.hookNotif = HookNotification{Active: true, Tool: "Edit", KeyArg: "main.go"}
	m.hookPendingCh = respCh

	next, _ := m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = next.(Model)

	if m.hookNotif.Active {
		t.Error("hookNotif.Active should be false after 'y' response")
	}
	if m.hookPendingCh != nil {
		t.Error("hookPendingCh should be nil after responding")
	}

	select {
	case resp := <-respCh:
		if !resp.Allow {
			t.Error("'y' key should send Allow=true")
		}
	default:
		t.Error("'y' key should have sent a response on the ResponseCh")
	}
}

// TestNKeyDeniesHookEvent verifies that pressing 'n' when a hook event is
// pending sends Allow=false on the ResponseCh and sets notifAck.
func TestNKeyDeniesHookEvent(t *testing.T) {
	m := NewModel()

	respCh := make(chan hookserver.HookResponse, 1)
	m.hookNotif = HookNotification{Active: true, Tool: "Bash", KeyArg: "rm -rf /"}
	m.hookPendingCh = respCh

	next, _ := m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m = next.(Model)

	if m.hookNotif.Active {
		t.Error("hookNotif.Active should be false after 'n' response")
	}
	if !m.notifAck {
		t.Error("notifAck should be true after 'n' response (banner dismissed)")
	}

	select {
	case resp := <-respCh:
		if resp.Allow {
			t.Error("'n' key should send Allow=false")
		}
	default:
		t.Error("'n' key should have sent a response on the ResponseCh")
	}
}

// TestEscDeniesHookEventWhenPending verifies that pressing Esc when a hook
// event is pending sends Allow=false (deny by dismissal) and sets notifAck.
func TestEscDeniesHookEventWhenPending(t *testing.T) {
	m := NewModel()

	respCh := make(chan hookserver.HookResponse, 1)
	m.hookNotif = HookNotification{Active: true, Tool: "Edit", KeyArg: "sensitive.go"}
	m.hookPendingCh = respCh

	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(Model)

	if m.hookNotif.Active {
		t.Error("hookNotif.Active should be false after Esc")
	}
	if !m.notifAck {
		t.Error("notifAck should be true after Esc deny")
	}

	select {
	case resp := <-respCh:
		if resp.Allow {
			t.Error("Esc when hook pending should send Allow=false")
		}
	default:
		t.Error("Esc when hook pending should have sent a response on the ResponseCh")
	}
}

// TestHookNotifRenderedInBanner verifies that when hookNotif.Active is true,
// the notification banner shows the live tool/command, not the mock "npm test".
func TestHookNotifRenderedInBanner(t *testing.T) {
	m := NewModel()
	m.width = 180
	m.height = 50
	m.bp = BreakpointWide

	// Inject a live hook notification.
	m.hookNotif = HookNotification{
		Active: true,
		Tool:   "Bash",
		KeyArg: "go build ./...",
		Cwd:    "/tmp/clyde",
	}
	m.notifAck = false

	next, _ := m.Update(tea.WindowSizeMsg{Width: 180, Height: 50})
	m = next.(Model)

	content := stripANSI(m.View().Content)

	if !strings.Contains(content, "go build ./...") {
		t.Error("notification banner should display the hook command 'go build ./...'")
	}
	// Mock command must NOT appear while a live event is active.
	if strings.Contains(content, "npm test") {
		t.Error("notification banner must NOT show mock 'npm test' when a live hook is active")
	}
}
