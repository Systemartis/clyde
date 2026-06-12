package tui

import (
	"testing"
)

// TestCollapseSpringConverges drives a spring through ~60 frames and asserts
// it converges to the target (deterministic physics test).
func TestCollapseSpringConverges(t *testing.T) {
	cs := NewCollapseSpring(20.0)
	cs.SetTarget(3.0) // collapse

	// Run up to 120 frames (at 60fps = 2 seconds — well past convergence)
	frames := 0
	for !cs.IsSettled() && frames < 120 {
		cs.Advance()
		frames++
	}

	if !cs.IsSettled() {
		t.Errorf("spring did not converge after %d frames (pos=%.2f, target=%.2f)",
			frames, cs.pos, cs.targetHeight)
	}

	h := cs.Height()
	if h != 3 {
		t.Errorf("Height() after collapse convergence = %d, want 3", h)
	}
}

// TestCollapseSpringExpandConverges verifies spring expands cleanly.
func TestCollapseSpringExpandConverges(t *testing.T) {
	cs := NewCollapseSpring(3.0)
	cs.SetTarget(20.0)

	frames := 0
	for !cs.IsSettled() && frames < 120 {
		cs.Advance()
		frames++
	}

	if !cs.IsSettled() {
		t.Errorf("spring did not converge after %d frames", frames)
	}
	h := cs.Height()
	if h != 20 {
		t.Errorf("Height() after expand convergence = %d, want 20", h)
	}
}

// TestPanelCollapseStateToggle verifies toggle flips state correctly.
func TestPanelCollapseStateToggle(t *testing.T) {
	ps := NewPanelCollapseState(false, 20.0)
	if ps.IsCollapsed() {
		t.Error("should start expanded")
	}
	ps.Toggle()
	if !ps.IsCollapsed() {
		t.Error("should be collapsed after toggle")
	}
	ps.Toggle()
	if ps.IsCollapsed() {
		t.Error("should be expanded after second toggle")
	}
}

// TestPanelCollapseStateExpandAndCollapse verifies idempotent expand/collapse.
func TestPanelCollapseStateExpandAndCollapse(t *testing.T) {
	ps := NewPanelCollapseState(false, 20.0)
	ps.Expand() // already expanded — no-op
	if ps.IsCollapsed() {
		t.Error("Expand() on already-expanded should not change state")
	}
	ps.Collapse()
	if !ps.IsCollapsed() {
		t.Error("should be collapsed after Collapse()")
	}
	ps.Collapse() // idempotent
	if !ps.IsCollapsed() {
		t.Error("Collapse() twice should still be collapsed")
	}
}

// TestPanelCollapseStateHeightBounds verifies Height() is never below 3.
func TestPanelCollapseStateHeightBounds(t *testing.T) {
	ps := NewPanelCollapseState(true, 20.0)
	h := ps.Height()
	if h < 3 {
		t.Errorf("Height() = %d, want >= 3", h)
	}
}

// TestSpringSmoothConvergence verifies both directions converge in a
// reasonable bounded number of frames. The old assertion (≤16 frames)
// was set for the v12 "snappy" config (omega=40) — at our 10Hz tick
// rate that produced a 5-row drop on tick 1, which read as a snap
// rather than animation. The current config (omega=14, damp=0.95)
// stretches the same motion across more frames so each row drop is
// individually visible. Allow up to 80 frames (well over a second of
// spring sim time) to absorb any future tuning without false alarms.
func TestSpringSmoothConvergence(t *testing.T) {
	// Collapse: expand(20) → collapse(3)
	cs := NewCollapseSpring(20.0)
	cs.SetTarget(3.0)

	frames := 0
	for !cs.IsSettled() && frames < 200 {
		cs.Advance()
		frames++
	}

	if !cs.IsSettled() {
		t.Errorf("collapse spring did not converge after %d frames (pos=%.3f, target=%.3f)",
			frames, cs.pos, cs.targetHeight)
	}
	if frames > 80 {
		t.Errorf("collapse convergence unusually slow: %d frames", frames)
	}
	t.Logf("collapse convergence: %d frames (%.0fms at 60fps spring sim)", frames, float64(frames)/60*1000)

	// Expand: collapsed(3) → expand(20)
	cs2 := NewCollapseSpring(3.0)
	cs2.SetTarget(20.0)

	frames2 := 0
	for !cs2.IsSettled() && frames2 < 200 {
		cs2.Advance()
		frames2++
	}

	if !cs2.IsSettled() {
		t.Errorf("expand spring did not converge after %d frames", frames2)
	}
	if frames2 > 80 {
		t.Errorf("expand convergence unusually slow: %d frames", frames2)
	}
	t.Logf("expand convergence: %d frames (%.0fms at 60fps spring sim)", frames2, float64(frames2)/60*1000)
}
