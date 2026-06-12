package tui

import (
	"testing"
	"time"
)

// TestNextTickInterval_IdleAfterFocusFade — once the focus-fade window
// has elapsed and nothing else is animating, the loop drops to the
// idle interval. This is the perf-audit fix that takes idle CPU from
// the 20Hz floor (~3-5%) down toward k9s territory (~0.5%).
func TestNextTickInterval_IdleAfterFocusFade(t *testing.T) {
	t.Parallel()

	m := NewModel()
	// Pretend the user last interacted long, long ago — well past the
	// focus-fade end.
	m.frame.Tick = uint64(focusFadeEndTick + 100)
	m.lastInteractionTick = 0
	// Notification fully landed — not animating.
	m.notifPos = 1.0
	// Mascot at rest.
	m.frame.Mascot = NewMascotFSM()
	// Boot off.
	m.boot.Active = false
	// Mock data defaults to a "writing" mode that reads as "claude is
	// working"; force idle so the only animation source can be the
	// focus fade (which we've also expired above).
	m.data.NowMode = "idle"

	if got := m.nextTickInterval(); got != idleTickInterval {
		t.Errorf("idle: nextTickInterval = %s, want %s", got, idleTickInterval)
	}
}

// TestNextTickInterval_FastWhileFocusFading — within the focus-fade
// decay window we MUST stay at activeTickInterval so the alpha blend
// renders smoothly. Dropping to 1Hz would make the decay visibly stuttered.
func TestNextTickInterval_FastWhileFocusFading(t *testing.T) {
	t.Parallel()

	m := NewModel()
	m.frame.Tick = 5
	m.lastInteractionTick = 0 // idle for 5 ticks (well below focusFadeEndTick)
	m.notifPos = 1.0
	m.boot.Active = false

	if got := m.nextTickInterval(); got != activeTickInterval {
		t.Errorf("focus-fading: nextTickInterval = %s, want %s", got, activeTickInterval)
	}
}

// TestNextTickInterval_FastDuringBoot — the splash animation MUST run
// at fast cadence regardless of idle time.
func TestNextTickInterval_FastDuringBoot(t *testing.T) {
	t.Parallel()

	m := NewModel()
	m.frame.Tick = uint64(focusFadeEndTick + 100)
	m.lastInteractionTick = 0
	m.notifPos = 1.0
	m.boot.Active = true

	if got := m.nextTickInterval(); got != activeTickInterval {
		t.Errorf("boot active: nextTickInterval = %s, want %s", got, activeTickInterval)
	}
}

// TestHandleFrame_DropsStaleGen — a FrameMsg with the wrong gen is a
// stale tick from a superseded chain (e.g. a slow tick that fired
// after the user's keypress had already swapped us to fast mode). The
// handler MUST drop it without rescheduling — otherwise the parallel
// chains would run forever, doubling CPU.
func TestHandleFrame_DropsStaleGen(t *testing.T) {
	t.Parallel()

	m := NewModel()
	m.tickGen = 7 // model is on gen 7

	// A stale tick from a prior chain (gen 3) arrives.
	stale := FrameMsg{Gen: 3}
	next, cmd := m.handleFrame(stale)
	if cmd != nil {
		t.Errorf("stale tick should not reschedule (got non-nil cmd)")
	}
	nm := next.(Model)
	if nm.frame.Tick != m.frame.Tick {
		t.Errorf("stale tick advanced frame counter: got %d, was %d", nm.frame.Tick, m.frame.Tick)
	}
	if nm.tickGen != m.tickGen {
		t.Errorf("stale tick mutated tickGen: got %d, was %d", nm.tickGen, m.tickGen)
	}
}

// TestMarkInteraction_BumpsGenAndReturnsWakeup — confirms the gen
// invariant that lets a keypress during idle mode invalidate the
// pending slow tick AND immediately schedule a fresh fast one.
func TestMarkInteraction_BumpsGenAndReturnsWakeup(t *testing.T) {
	t.Parallel()

	m := NewModel()
	beforeGen := m.tickGen
	wake := m.markInteraction()
	if wake == nil {
		t.Fatalf("markInteraction returned nil cmd; expected wakeup tickCmd")
	}
	if m.tickGen != beforeGen+1 {
		t.Errorf("tickGen = %d, want %d (must bump on interaction)", m.tickGen, beforeGen+1)
	}
	// Run the wakeup — we get a FrameMsg whose Gen matches the new tickGen.
	msg := wake()
	fm, ok := msg.(FrameMsg)
	if !ok {
		t.Fatalf("wake() returned %T, want FrameMsg", msg)
	}
	if fm.Gen != m.tickGen {
		t.Errorf("wake FrameMsg.Gen = %d, want %d", fm.Gen, m.tickGen)
	}

	// Drain any inherent latency so the test isn't dependent on tea.Tick
	// internals across versions.
	_ = time.Millisecond
}
