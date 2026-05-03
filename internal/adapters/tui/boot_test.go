package tui

import (
	"strings"
	"testing"
)

// TestBootScreenInactiveSkipsTick verifies an inactive BootScreen never
// counts ticks — Advance is a no-op until Active flips on.
func TestBootScreenInactiveSkipsTick(t *testing.T) {
	b := BootScreen{}
	for i := 0; i < 100; i++ {
		b = b.Advance()
	}
	if b.Active {
		t.Errorf("inactive boot should never become active via Advance")
	}
	if b.Tick != 0 {
		t.Errorf("inactive boot tick should stay at 0, got %d", b.Tick)
	}
}

// TestBootScreenAutoDismiss verifies the splash auto-finishes after
// bootDuration ticks without any keypress.
func TestBootScreenAutoDismiss(t *testing.T) {
	b := BootScreen{Active: true}
	for i := 0; i < bootDuration; i++ {
		b = b.Advance()
	}
	if b.Active {
		t.Errorf("boot should auto-dismiss after %d ticks, still Active=%v Tick=%d", bootDuration, b.Active, b.Tick)
	}
}

// TestBootScreenDismiss verifies Dismiss() turns the splash off immediately.
func TestBootScreenDismiss(t *testing.T) {
	b := BootScreen{Active: true, Tick: 5}
	b = b.Dismiss()
	if b.Active {
		t.Errorf("after Dismiss(), Active should be false")
	}
}

// TestBootScreenWordmarkRevealsOverTime verifies that as the tick
// progresses past bootPhaseTypeStart, more letters of the wordmark
// become visible.
func TestBootScreenWordmarkRevealsOverTime(t *testing.T) {
	p := TokyoNightPalette()

	// Before the type phase: no letters revealed.
	earlyLines := bootWordmarkLines(p, bootPhaseTypeStart-1)
	earlyVisible := stripANSI(earlyLines[0])
	if strings.TrimSpace(earlyVisible) != "" {
		t.Errorf("before type phase, wordmark should be all spaces, got %q", earlyVisible)
	}

	// After the full type window (5 letters × 3 ticks): all letters revealed.
	lateLines := bootWordmarkLines(p, bootPhaseTypeStart+5*3+1)
	lateVisible := stripANSI(lateLines[0])
	if strings.TrimSpace(lateVisible) == "" {
		t.Errorf("after type phase, wordmark must show letters, got %q", lateVisible)
	}
}

// TestBootScreenTaglineDeferredUntilPhase verifies the tagline is empty
// until the tick crosses bootPhaseTaglineFade.
func TestBootScreenTaglineDeferredUntilPhase(t *testing.T) {
	p := TokyoNightPalette()
	if got := bootTaglineLine(p, bootPhaseTaglineFade-1); got != "" {
		t.Errorf("tagline should be empty before phase, got %q", got)
	}
	if got := bootTaglineLine(p, bootPhaseTaglineFade+5); got == "" {
		t.Error("tagline should be visible after phase, got empty")
	}
}

// TestBootScreenRendersFullSize verifies renderBootScreen produces exactly
// totalH lines of width >= totalW so it fills the terminal cleanly.
func TestBootScreenRendersFullSize(t *testing.T) {
	p := TokyoNightPalette()
	b := BootScreen{Active: true, Tick: 30}
	out := renderBootScreen(p, b, 80, 24)
	rows := strings.Split(out, "\n")
	if len(rows) != 24 {
		t.Errorf("renderBootScreen should produce 24 rows, got %d", len(rows))
	}
	for i, r := range rows {
		// stripANSI is defined in view_golden_test.go (same package).
		visible := stripANSI(r)
		if len(visible) < 80 {
			// Centered content can carry trailing spaces; this just confirms
			// the row is at least wide enough to fill the screen.
			t.Errorf("row %d visible width %d, want >= 80", i, len(visible))
			break
		}
	}
}

// TestBootKittenWavesDuringWavePhase verifies the kitten frame switches
// to a paw / wave variant during the wave window.
func TestBootKittenWavesDuringWavePhase(t *testing.T) {
	p := TokyoNightPalette()
	// Sample several ticks within the wave window — the kitten should hit
	// at least one paw or wave body line across the cycle.
	sawPaw := false
	for tick := bootPhaseWaveStart; tick < bootPhaseWaveStart+12; tick++ {
		lines := bootKittenLines(p, tick)
		joined := strings.Join(lines, "\n")
		// Strip ANSI to look at the underlying ASCII shape.
		if strings.Contains(stripANSI(joined), `\(   )/`) ||
			strings.Contains(stripANSI(joined), `\(   )\`) ||
			strings.Contains(stripANSI(joined), `/(   )/`) {
			sawPaw = true
			break
		}
	}
	if !sawPaw {
		t.Error("expected the kitten to wave (arms-up body shape) during the wave phase")
	}
}
