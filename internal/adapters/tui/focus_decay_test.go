package tui

import (
	"image/color"
	"math"
	"testing"

	"charm.land/lipgloss/v2"
)

// TestFocusAlphaActivePhase verifies the keyboard-target panel reads at
// alpha=1.0 while the user is interacting, and every other panel stays
// at alpha=0.
func TestFocusAlphaActivePhase(t *testing.T) {
	m := NewModel()
	m.focused = PanelCalls
	m.frame.Tick = 100
	m.lastInteractionTick = 100

	if got := m.focusAlpha(PanelCalls); got != 1.0 {
		t.Errorf("focused panel alpha = %v, want 1.0", got)
	}
	if got := m.focusAlpha(PanelNow); got != 0.0 {
		t.Errorf("non-focused panel alpha = %v, want 0.0", got)
	}
}

// TestFocusAlphaFadeOutSweep verifies the fade-out window produces a
// monotonic 1.0 → 0.0 sweep on the keyboard-target panel.
func TestFocusAlphaFadeOutSweep(t *testing.T) {
	m := NewModel()
	m.focused = PanelCalls
	m.lastInteractionTick = 0

	at := func(tick uint64) float64 {
		m.frame.Tick = tick
		return m.focusAlpha(PanelCalls)
	}

	if got := at(focusFadeStartTick); got != 1.0 {
		t.Errorf("at fade start tick alpha = %v, want 1.0 (still active until threshold crossed)", got)
	}
	mid := at((focusFadeStartTick + focusFadeMidTick) / 2)
	if math.Abs(mid-0.5) > 0.05 {
		t.Errorf("at mid-fade-out alpha = %v, want ≈0.5", mid)
	}
	if got := at(focusFadeMidTick - 1); !(got > 0 && got < 0.05) {
		t.Errorf("just before fade-out end alpha = %v, want a tiny positive value approaching 0", got)
	}
	if got := at(focusFadeMidTick); got != 0.0 {
		t.Errorf("at fade-out end alpha = %v, want 0.0", got)
	}
}

// TestFocusAlphaFadeInSweep verifies PanelNow brightens 0 → 1 over the
// fade-in window.
func TestFocusAlphaFadeInSweep(t *testing.T) {
	m := NewModel()
	m.focused = PanelCalls
	m.lastInteractionTick = 0

	at := func(tick uint64) float64 {
		m.frame.Tick = tick
		return m.focusAlpha(PanelNow)
	}

	if got := at(focusFadeMidTick); got != 0.0 {
		t.Errorf("at fade-in start alpha = %v, want 0.0", got)
	}
	mid := at((focusFadeMidTick + focusFadeEndTick) / 2)
	if math.Abs(mid-0.5) > 0.05 {
		t.Errorf("at mid-fade-in alpha = %v, want ≈0.5", mid)
	}
	if got := at(focusFadeEndTick + 100); got != 1.0 {
		t.Errorf("settled phase alpha = %v, want 1.0", got)
	}
}

// TestFocusAlphaDuringFadeOutOthersStayUnfocused verifies that during
// the fade-out window only the previously-focused panel carries fractional
// alpha — the now panel waits its turn.
func TestFocusAlphaDuringFadeOutOthersStayUnfocused(t *testing.T) {
	m := NewModel()
	m.focused = PanelCalls
	m.lastInteractionTick = 0
	m.frame.Tick = focusFadeStartTick + 30

	if got := m.focusAlpha(PanelNow); got != 0.0 {
		t.Errorf("during fade-out PanelNow alpha = %v, want 0.0 (waits for fade-in)", got)
	}
	if got := m.focusAlpha(PanelDiff); got != 0.0 {
		t.Errorf("during fade-out non-target panel alpha = %v, want 0.0", got)
	}
	if got := m.focusAlpha(PanelCalls); !(got > 0 && got < 1) {
		t.Errorf("during fade-out target alpha = %v, want strictly between 0 and 1", got)
	}
}

// TestFocusAlphaDisabledOutsideDashboardMode verifies the fade snaps to
// binary values when an overlay is open. The user explicitly asked that
// the fade only run when clyde is acting as a passive dashboard.
func TestFocusAlphaDisabledOutsideDashboardMode(t *testing.T) {
	cases := []struct {
		name string
		set  func(*Model)
	}{
		{"settings open", func(m *Model) { m.settingsOpen = true }},
		{"viewer active", func(m *Model) { m.viewerActive = true }},
		{"help open", func(m *Model) { m.helpOpen = true }},
		{"hook notif active", func(m *Model) { m.hookNotif.Active = true }},
		{"boot active", func(m *Model) { m.boot.Active = true }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewModel()
			m.focused = PanelCalls
			m.lastInteractionTick = 0
			m.frame.Tick = focusFadeStartTick + 30 // mid-fade-out under normal rules
			tc.set(&m)

			if got := m.focusAlpha(PanelCalls); got != 1.0 {
				t.Errorf("with %s, focused panel alpha = %v, want 1.0 (no fade)", tc.name, got)
			}
			if got := m.focusAlpha(PanelNow); got != 0.0 {
				t.Errorf("with %s, now panel alpha = %v, want 0.0 (no migration)", tc.name, got)
			}
		})
	}
}

// TestVisualFocusReturnsTrueDuringFade verifies the bool returned by
// visualFocus reports "focused-ish" while alpha is positive — so the
// chrome stays in Passive state and the blended border remains visible.
func TestVisualFocusReturnsTrueDuringFade(t *testing.T) {
	m := NewModel()
	m.focused = PanelCalls
	m.lastInteractionTick = 0
	m.frame.Tick = focusFadeStartTick + 30

	if !m.visualFocus(PanelCalls) {
		t.Errorf("during fade-out the previously-focused panel should still report visualFocus=true so the border remains styled")
	}
}

// TestMarkInteractionResetsIdleTimer verifies markInteraction snaps the
// idle counter back to zero so the user's keystroke immediately reclaims
// the focus highlight.
func TestMarkInteractionResetsIdleTimer(t *testing.T) {
	m := NewModel()
	m.frame.Tick = focusFadeEndTick + 100
	m.lastInteractionTick = 0
	if m.idleTicks() == 0 {
		t.Fatal("setup invalid: idleTicks must be non-zero before reset")
	}
	m.markInteraction()
	if m.idleTicks() != 0 {
		t.Errorf("after markInteraction(), idleTicks must be 0, got %d", m.idleTicks())
	}
}

// TestBlendColorEndpoints verifies blendColor returns the input colors
// exactly at alpha=0 and alpha=1.
func TestBlendColorEndpoints(t *testing.T) {
	from := lipgloss.Color("#000000")
	to := lipgloss.Color("#ffffff")

	if !rgbaEqual(blendColor(from, to, 0), from) {
		t.Errorf("blendColor at alpha=0 should equal `from`")
	}
	if !rgbaEqual(blendColor(from, to, 1), to) {
		t.Errorf("blendColor at alpha=1 should equal `to`")
	}
}

// TestBlendColorMidpointIsAverage verifies the midpoint of black↔white
// blends to a neutral mid-gray. The exact 16-bit value is 32767/65535
// after rounding so check within a tight tolerance.
func TestBlendColorMidpointIsAverage(t *testing.T) {
	black := lipgloss.Color("#000000")
	white := lipgloss.Color("#ffffff")
	mid := blendColor(black, white, 0.5)
	r, g, b, _ := mid.RGBA()
	want := uint32(0x7f7f) // half of 0xffff, give or take 1 LSB
	tol := uint32(0x0200)
	for _, ch := range []uint32{r, g, b} {
		if absDiff(ch, want) > tol {
			t.Errorf("blendColor midpoint channel = 0x%04x, want ≈0x%04x (±0x%04x)", ch, want, tol)
		}
	}
}

// TestWithFadedFocusBlendsBorder verifies that the styles returned by
// WithFadedFocus carry a border color that is strictly between BorderDim
// and BorderAcc when alpha is in (0,1).
func TestWithFadedFocusBlendsBorder(t *testing.T) {
	p := TokyoNightPalette()
	s := NewStyles(p)
	half := s.WithFadedFocus(p, 0.5)
	got := half.PanelFocus.GetBorderTopForeground()

	if rgbaEqual(got, p.BorderAcc) {
		t.Errorf("at alpha=0.5 the focus border should not equal BorderAcc")
	}
	if rgbaEqual(got, p.BorderDim) {
		t.Errorf("at alpha=0.5 the focus border should not equal BorderDim")
	}
}

func rgbaEqual(a, b color.Color) bool {
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

func absDiff(a, b uint32) uint32 {
	if a > b {
		return a - b
	}
	return b - a
}
