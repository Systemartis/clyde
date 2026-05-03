package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// FrameMsg is the tick message driving all frame-counter animations.
//
// Gen identifies the tick chain that scheduled this message. The model
// tracks the current generation in tickGen and bumps it whenever the
// next interval should change (user input, animation start). A FrameMsg
// whose Gen does not match the current tickGen is a stale tick from a
// superseded chain and is silently dropped — without this, switching
// from idle (1Hz) to active (20Hz) would create racing parallel chains.
type FrameMsg struct {
	Gen uint64
}

// activeTickInterval is the inter-frame gap while anything is actively
// animating (collapse springs, mascot sequences, boot splash, focus
// fade, claude working). 50ms = 20Hz — double the v22 default — gives
// spring animations enough render frames to feel smooth.
const activeTickInterval = 50 * time.Millisecond

// idleTickInterval is the inter-frame gap when the dashboard has been
// quiet long enough that nothing visible is changing. 1Hz cuts idle CPU
// drastically while still giving us a heartbeat to notice when claude
// resumes work or a focus-decay needs to start.
const idleTickInterval = 1 * time.Second

// tickCmd schedules the next animation frame after d, tagged with gen
// so the handler can drop stale ticks scheduled by a superseded chain.
func tickCmd(gen uint64, d time.Duration) tea.Cmd {
	return tea.Tick(d, func(_ time.Time) tea.Msg {
		return FrameMsg{Gen: gen}
	})
}

// FrameState holds per-animation phase counters derived from a monotonic
// tick counter. All animations are driven by the FrameMsg tick.
type FrameState struct {
	Tick uint64

	// Bunny FSM — drives all mascot states and transitions.
	Mascot MascotFSM

	// Derived phases — recomputed on each tick.
	SpinnerFrame  int  // 0-3  (~333ms per frame)
	LiveDotDim    bool // alternates every 0.8s
	CursorVisible bool // alternates every 0.4s
	ChevronDim    bool // alternates every 0.7s
}

// spinnerGlyphs rotates through the spinner sequence (fallback for golden tests).
var spinnerGlyphs = []string{"○", "◎", "◉", "●", "◉", "◎"}

// SpinnerGlyph returns the current spinner glyph from frame counter.
func (f FrameState) SpinnerGlyph() string {
	return spinnerGlyphs[f.SpinnerFrame%len(spinnerGlyphs)]
}

// AdvanceTick returns a new FrameState with Tick incremented and all derived
// phases recomputed. Dividers are sized for the active tick interval so
// every wall-clock period below stays the same regardless of the tick rate.
func AdvanceTick(prev FrameState) FrameState {
	t := prev.Tick + 1
	return FrameState{
		Tick:          t,
		Mascot:        prev.Mascot.Advance(),
		SpinnerFrame:  int(t/5) % len(spinnerGlyphs), // ~250ms per frame at 50ms tick
		LiveDotDim:    (t/16)%2 == 1,                 // dim half of every 0.8s cycle
		CursorVisible: (t/8)%2 == 0,                  // visible for first 0.4s of 0.8s cycle
		ChevronDim:    (t/14)%2 == 1,                 // dim half of every 0.7s cycle
	}
}

// InitFrameState returns an initial FrameState (tick=0).
func InitFrameState() FrameState {
	return FrameState{
		Tick:   0,
		Mascot: NewMascotFSM(),
	}
}
