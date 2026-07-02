package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestEditHintLine_SelectAllHintReflectsActualKey guards the honesty of the
// edit-mode hint bar: select-all fires only on Cmd/Super+A (⌘a); plain Ctrl+A
// (⌃a) moves to line start. The hint must not advertise ⌃a for "all".
func TestEditHintLine_SelectAllHintReflectsActualKey(t *testing.T) {
	t.Parallel()
	h := editHintLine()
	if strings.Contains(h, "⌃a all") {
		t.Errorf("hint advertises ⌃a for select-all, but ⌃a moves to line start; hint: %q", h)
	}
	if !strings.Contains(h, "⌘a all") {
		t.Errorf("expected ⌘a all (select-all is Cmd/Super+A); hint: %q", h)
	}
}

// loadViewerWithText prepares a Model with the viewer open on a 50-line
// fixture document. Returns the model ready for vim key tests.
func loadViewerWithText(t *testing.T) Model {
	t.Helper()
	m := NewModel()
	m.viewerActive = true
	m.viewerFile = "fixture.go"
	// Mock content: 50 numbered lines so scroll positions are easy to inspect.
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = "line " + intToStr(i+1)
	}
	mockFileContent[m.viewerFile] = strings.Join(lines, "\n")
	t.Cleanup(func() { delete(mockFileContent, m.viewerFile) })

	// Size the viewport: width 80, height 20 → vp height ≈ 17.
	m.viewport.vp.SetWidth(80)
	m.viewport.vp.SetHeight(20)
	m.viewport.vp.SetContent(mockFileContent[m.viewerFile])
	return m
}

// intToStr is a tiny test helper that avoids pulling fmt for one call.
func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	out := ""
	for n > 0 {
		out = string(rune('0'+n%10)) + out
		n /= 10
	}
	return out
}

// sendKey routes a key through the public Update so tests cover the same
// dispatch path as the runtime — handleKey → handleViewerKey → handleViewerVimKey.
func sendKey(t *testing.T, m Model, code rune, mod tea.KeyMod) Model {
	t.Helper()
	next, _ := m.Update(tea.KeyPressMsg{Code: code, Mod: mod})
	return next.(Model)
}

// TestVim_jk_VerticalScroll verifies j and k move the viewport one line.
func TestVim_jk_VerticalScroll(t *testing.T) {

	m := loadViewerWithText(t)
	if got := m.viewport.vp.YOffset(); got != 0 {
		t.Fatalf("setup: yOffset = %d, want 0", got)
	}
	m = sendKey(t, m, 'j', 0)
	if got := m.viewport.vp.YOffset(); got != 1 {
		t.Errorf("after j: yOffset = %d, want 1", got)
	}
	m = sendKey(t, m, 'j', 0)
	m = sendKey(t, m, 'j', 0)
	if got := m.viewport.vp.YOffset(); got != 3 {
		t.Errorf("after jjj: yOffset = %d, want 3", got)
	}
	m = sendKey(t, m, 'k', 0)
	if got := m.viewport.vp.YOffset(); got != 2 {
		t.Errorf("after k: yOffset = %d, want 2", got)
	}
}

// TestVim_hl_HorizontalScroll verifies h and l shift xOffset by horizStep,
// floored at zero.
func TestVim_hl_HorizontalScroll(t *testing.T) {

	m := loadViewerWithText(t)
	m = sendKey(t, m, 'l', 0)
	m = sendKey(t, m, 'l', 0)
	if got := m.viewport.xOffset; got != 8 {
		t.Errorf("after ll: xOffset = %d, want 8", got)
	}
	m = sendKey(t, m, 'h', 0)
	if got := m.viewport.xOffset; got != 4 {
		t.Errorf("after h: xOffset = %d, want 4", got)
	}
	// h at 0 stays at 0 — never goes negative.
	m = sendKey(t, m, 'h', 0)
	m = sendKey(t, m, 'h', 0)
	if got := m.viewport.xOffset; got != 0 {
		t.Errorf("after hh past zero: xOffset = %d, want 0", got)
	}
}

// TestVim_gg_JumpsToTop verifies the gg chord requires two presses.
func TestVim_gg_JumpsToTop(t *testing.T) {

	m := loadViewerWithText(t)
	// Scroll down a bit first.
	m.viewport.vp.SetYOffset(20)
	if got := m.viewport.vp.YOffset(); got != 20 {
		t.Fatalf("setup: yOffset = %d, want 20", got)
	}
	// First g arms the chord but doesn't move.
	m = sendKey(t, m, 'g', 0)
	if !m.vimGPending {
		t.Error("expected vimGPending after first g")
	}
	if got := m.viewport.vp.YOffset(); got != 20 {
		t.Errorf("after first g: yOffset = %d, want 20 (no move)", got)
	}
	// Second g completes gg → top.
	m = sendKey(t, m, 'g', 0)
	if m.vimGPending {
		t.Error("vimGPending should clear after gg completes")
	}
	if got := m.viewport.vp.YOffset(); got != 0 {
		t.Errorf("after gg: yOffset = %d, want 0", got)
	}
}

// TestVim_g_OtherKeyClearsPending verifies a non-g keypress after the first g
// clears the chord state instead of leaving the viewer in a sticky mode.
func TestVim_g_OtherKeyClearsPending(t *testing.T) {

	m := loadViewerWithText(t)
	m.viewport.vp.SetYOffset(10)
	m = sendKey(t, m, 'g', 0)
	if !m.vimGPending {
		t.Fatal("setup: vimGPending not armed")
	}
	m = sendKey(t, m, 'j', 0)
	if m.vimGPending {
		t.Error("vimGPending should clear after non-g key")
	}
	if got := m.viewport.vp.YOffset(); got != 11 {
		t.Errorf("j after pending g: yOffset = %d, want 11 (j should still fire)", got)
	}
}

// TestVim_G_JumpsToBottom verifies G goes to the last page.
func TestVim_G_JumpsToBottom(t *testing.T) {

	m := loadViewerWithText(t)
	m = sendKey(t, m, 'G', 0)
	// We can't assert an exact yOffset without knowing the viewport's
	// last-line math; just verify it advanced beyond the start.
	if got := m.viewport.vp.YOffset(); got <= 0 {
		t.Errorf("after G: yOffset = %d, want > 0", got)
	}
}

// TestVim_0_ResetsHorizontal verifies '0' jumps the horizontal scroll back
// to column 0.
func TestVim_0_ResetsHorizontal(t *testing.T) {

	m := loadViewerWithText(t)
	m.viewport.xOffset = 16
	m = sendKey(t, m, '0', 0)
	if got := m.viewport.xOffset; got != 0 {
		t.Errorf("after 0: xOffset = %d, want 0", got)
	}
}

// TestVim_CtrlD_HalfPageDown verifies Ctrl+d moves down by ~half a page.
func TestVim_CtrlD_HalfPageDown(t *testing.T) {

	m := loadViewerWithText(t)
	m = sendKey(t, m, 'd', tea.ModCtrl)
	if got := m.viewport.vp.YOffset(); got <= 0 {
		t.Errorf("after Ctrl+d: yOffset = %d, want > 0", got)
	}
}

// TestVim_CtrlF_PageDown verifies Ctrl+f moves down by ~one page (more than
// Ctrl+d). We check ordering, not exact values, because viewport's page math
// is private and depends on vpHeight which is set by the renderer.
func TestVim_CtrlF_PageDown(t *testing.T) {

	mD := loadViewerWithText(t)
	mD = sendKey(t, mD, 'd', tea.ModCtrl)
	mF := loadViewerWithText(t)
	mF = sendKey(t, mF, 'f', tea.ModCtrl)
	if mF.viewport.vp.YOffset() <= mD.viewport.vp.YOffset() {
		t.Errorf("Ctrl+f (yOff=%d) should advance further than Ctrl+d (yOff=%d)",
			mF.viewport.vp.YOffset(), mD.viewport.vp.YOffset())
	}
}

// TestVim_F_TogglesFullscreen verifies `f` flips the fullscreen takeover
// state and that closing the viewer (Esc) resets it.
func TestVim_F_TogglesFullscreen(t *testing.T) {

	m := loadViewerWithText(t)
	if m.viewerFullscreen {
		t.Fatal("setup: viewerFullscreen should default off")
	}
	m = sendKey(t, m, 'f', 0)
	if !m.viewerFullscreen {
		t.Error("after f: viewerFullscreen should be true")
	}
	m = sendKey(t, m, 'f', 0)
	if m.viewerFullscreen {
		t.Error("after second f: viewerFullscreen should toggle off")
	}
	// Open + go fullscreen + close-via-Esc should leave the flag false
	// so re-opening doesn't surprise the user with a fullscreen view.
	m = sendKey(t, m, 'f', 0)
	if !m.viewerFullscreen {
		t.Fatal("after f: expected fullscreen on")
	}
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = next.(Model)
	if m.viewerFullscreen {
		t.Error("Esc on fullscreen viewer should reset viewerFullscreen")
	}
	if m.viewerActive {
		t.Error("Esc on viewer should also close it")
	}
}

// TestVim_CtrlU_HalfPageUp_FromBottom verifies Ctrl+u rewinds after going to
// the bottom.
func TestVim_CtrlU_HalfPageUp_FromBottom(t *testing.T) {

	m := loadViewerWithText(t)
	m = sendKey(t, m, 'G', 0)
	atBottom := m.viewport.vp.YOffset()
	if atBottom <= 0 {
		t.Skip("viewport too small to demonstrate scroll back")
	}
	m = sendKey(t, m, 'u', tea.ModCtrl)
	if got := m.viewport.vp.YOffset(); got >= atBottom {
		t.Errorf("after Ctrl+u: yOffset = %d, want < %d", got, atBottom)
	}
}
