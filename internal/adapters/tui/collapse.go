package tui

import (
	"math"

	"github.com/charmbracelet/harmonica"
)

// CollapseSpring is a harmonica-backed spring for animating panel height.
// Each panel has its own CollapseSpring tracking current height, velocity,
// and the target height driven by collapsed/expanded state.
type CollapseSpring struct {
	spring       harmonica.Spring
	pos          float64 // current rendered height (fractional)
	vel          float64 // current velocity
	targetHeight float64 // what we're animating toward
	settled      bool    // true when animation is effectively done
}

// springConfig is the shared spring parameters:
// FPS(60), angularFreq=18.0, dampingRatio=0.9 → smooth ~700-900ms
// settle with very mild overshoot. Tuned for the 50ms tick rate
// (frameTickInterval): the spring covers ~14 → 3 over ~14 distinct
// rendered frames at 20fps, so the motion reads as a clear ease and
// not a snap, but stays under one second so the UI feels alive.
//
// Earlier values (kept here so the next tuning pass has the curve):
//
//	omega=40, damp=0.8 (v12) → first tick dropped ~5 rows (snap)
//	omega=14, damp=0.95      → ~1 row/tick, felt slightly fast
//	omega=10, damp=0.95      → ~0.3-0.7 row/tick, ~2s — too slow
//	omega=18, damp=0.9       → balanced ~800ms (current)
var springConfig = harmonica.NewSpring(harmonica.FPS(60), 18.0, 0.9)

// NewCollapseSpring creates a spring starting at h (already settled).
func NewCollapseSpring(h float64) CollapseSpring {
	return CollapseSpring{
		spring:       springConfig,
		pos:          h,
		vel:          0,
		targetHeight: h,
		settled:      true,
	}
}

// SetTarget changes the target height and marks the spring as unsettled.
func (cs *CollapseSpring) SetTarget(h float64) {
	if cs.targetHeight == h && cs.settled {
		return
	}
	cs.targetHeight = h
	cs.settled = false
}

// Advance runs one frame of the spring simulation.
// At 100ms tick cadence we call this once per FrameMsg.
func (cs *CollapseSpring) Advance() {
	if cs.settled {
		return
	}
	cs.pos, cs.vel = cs.spring.Update(cs.pos, cs.vel, cs.targetHeight)
	// Convergence check: close enough and velocity negligible.
	if math.Abs(cs.targetHeight-cs.pos) < 0.5 && math.Abs(cs.vel) < 0.1 {
		cs.pos = cs.targetHeight
		cs.vel = 0
		cs.settled = true
	}
}

// Height returns the current integer height for rendering.
func (cs *CollapseSpring) Height() int {
	return int(math.Round(cs.pos))
}

// IsSettled reports whether the animation has converged.
func (cs *CollapseSpring) IsSettled() bool {
	return cs.settled
}

// IsSettled reports whether the wrapped spring has converged. Used by
// the model's adaptive frame-tick gate to decide whether the View loop
// can drop to idleTickInterval.
func (p *PanelCollapseState) IsSettled() bool {
	return p.spring.IsSettled()
}

// ─── PanelCollapseState ───────────────────────────────────────────────────────

// PanelCollapseState holds collapse state + animation spring for one panel.
type PanelCollapseState struct {
	collapsed  bool
	spring     CollapseSpring
	expandedH  float64 // natural expanded height
	collapsedH float64 // one-liner collapsed height (always 3: top border + content + bottom)
}

// NewPanelCollapseState initializes collapse state. startCollapsed sets the
// initial visual state. expandedH is the full panel height in rows.
func NewPanelCollapseState(startCollapsed bool, expandedH float64) PanelCollapseState {
	collH := 3.0 // border top + 1 content line + border bottom
	startH := expandedH
	if startCollapsed {
		startH = collH
	}
	return PanelCollapseState{
		collapsed:  startCollapsed,
		spring:     NewCollapseSpring(startH),
		expandedH:  expandedH,
		collapsedH: collH,
	}
}

// Toggle flips collapsed state and updates the spring target.
func (p *PanelCollapseState) Toggle() {
	p.collapsed = !p.collapsed
	if p.collapsed {
		p.spring.SetTarget(p.collapsedH)
	} else {
		p.spring.SetTarget(p.expandedH)
	}
}

// Expand ensures the panel is expanded (does not toggle if already expanded).
func (p *PanelCollapseState) Expand() {
	if p.collapsed {
		p.collapsed = false
		p.spring.SetTarget(p.expandedH)
	}
}

// Collapse ensures the panel is collapsed.
func (p *PanelCollapseState) Collapse() {
	if !p.collapsed {
		p.collapsed = true
		p.spring.SetTarget(p.collapsedH)
	}
}

// SetExpandedHeight updates the target expanded height (e.g. on window resize).
func (p *PanelCollapseState) SetExpandedHeight(h float64) {
	p.expandedH = h
	if !p.collapsed {
		p.spring.SetTarget(h)
	}
}

// Advance ticks the spring one frame.
func (p *PanelCollapseState) Advance() {
	p.spring.Advance()
}

// Height returns the current animated height for rendering.
func (p *PanelCollapseState) Height() int {
	h := p.spring.Height()
	if h < 3 {
		h = 3
	}
	return h
}

// IsCollapsed reports the logical collapsed state (not the animated state).
func (p *PanelCollapseState) IsCollapsed() bool {
	return p.collapsed
}
