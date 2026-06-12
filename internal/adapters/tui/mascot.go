package tui

// MascotBaseState identifies the named base states of the mascot FSM.
// Intermediate / transitional frames are encoded as additional constants.
//
// Kitten shape reference (Stand state):
//
//	 /\_/\     ← ears (row 0)
//	( o.o )    ← round face with eyes (row 1)
//	/(   )\    ← body / arms (row 2)
//	 _"_"_     ← paws (row 3)
//
// Width budget: 9 chars wide on every row (lipgloss pads to the longest
// line anyway, but staying balanced keeps the block from drifting under
// the spinner column). Block height: always exactly 4 rows.
type MascotBaseState int

// Base state constants. The iota starts at MascotStand.
const (
	MascotStand     MascotBaseState = iota // default — ears up, eyes open
	MascotBlink                            // eyes closed (-.-)
	MascotLookLeft                         // eyes look left (◐.◐)
	MascotLookRight                        // eyes look right (◑.◑)
	MascotHappy                            // eyes smile up (^_^)
	MascotSurprised                        // eyes wide open (O.O)
	MascotSleep                            // eyes droop, animated zZ next to ears
	MascotWave                             // both paws up, greeting

	// Intermediate states (unexported — used inside FSM only)
	mascotHalfLookLeft  // transition: starting to look left (◐.o)
	mascotHalfLookRight // transition: starting to look right (o.◑)
	mascotHalfBack      // transition: returning to center (o.o)
	mascotPreBlink      // one-frame pre-blink hold (o.o)
	mascotHalfBlink     // mid-blink, eyes squinting (~.~)
	mascotDrowsy        // half-closed — sleep onset/wake (˘.˘)
	mascotPawLeft       // greeting variant: only left paw up
	mascotPawRight      // greeting variant: only right paw up
)

// mascotEventKind is the kind of autonomous idle event.
type mascotEventKind int

const (
	eventBlink      mascotEventKind = iota // quick blink
	eventLookAround                        // look left+right cycle
	eventHappy                             // happy hold
	eventSurprised                         // surprised hold
	eventSleep                             // sleep sequence
	eventWave                              // greeting wave
	eventStretch                           // lazy stretch — both paws alternating
	eventNuzzle                            // rapid head tilt left+right
	eventCuddle                            // happy + alternating paw raises
)

// mascotFrame is one step in an animation sequence.
type mascotFrame struct {
	state MascotBaseState
	hold  int // how many ticks to hold this frame (≥1)
}

// blinkSequence: pre-blink (1) + half-blink (1) + closed (2) + half-blink (1) + recover (1).
// Five steps instead of three so the eye-close motion reads as smooth instead
// of a hard cut between open and closed.
var blinkSequence = []mascotFrame{
	{mascotPreBlink, 1},
	{mascotHalfBlink, 1},
	{MascotBlink, 2},
	{mascotHalfBlink, 1},
	{MascotStand, 1},
}

// lookAroundSequence: left then right with intermediate half-frames.
var lookAroundSequence = []mascotFrame{
	{mascotHalfLookLeft, 1},  // starting to look left
	{MascotLookLeft, 3},      // hold left
	{mascotHalfBack, 1},      // returning to center
	{MascotStand, 1},         // brief neutral pause
	{mascotHalfLookRight, 1}, // starting to look right
	{MascotLookRight, 3},     // hold right
	{mascotHalfBack, 1},      // returning to center
	{MascotStand, 1},         // settle
}

// happySequence: quick happy flash.
var happySequence = []mascotFrame{
	{MascotHappy, 8},
	{MascotStand, 2},
}

// surprisedSequence: brief surprise.
var surprisedSequence = []mascotFrame{
	{MascotSurprised, 4},
	{MascotStand, 2},
}

// sleepSequence: drowsy → deep sleep → wake. Sleep is held long enough
// (24 ticks ≈ 2.4s) for the 4-phase animated zZ to cycle through twice
// before the kitten wakes — without that, the puff cycle would feel
// truncated.
var sleepSequence = []mascotFrame{
	{mascotDrowsy, 3}, // half-closed, onset
	{MascotSleep, 24}, // deep sleep — full zZ cycle
	{mascotDrowsy, 3}, // waking
	{MascotStand, 2},  // recovered
}

// waveSequence: alternating paws + a final two-paw flourish so the wave
// reads as a real greeting motion rather than a single static frame.
var waveSequence = []mascotFrame{
	{mascotPawRight, 2},
	{MascotWave, 2},
	{mascotPawLeft, 2},
	{MascotWave, 2},
	{mascotPawRight, 2},
	{MascotWave, 3}, // final flourish
	{MascotHappy, 2},
	{MascotStand, 1},
}

// stretchSequence: lazy stretch — both paws up alternately, then a beat
// of happy before settling. Used as an occasional idle filler so the
// mascot doesn't only blink during quiet moments.
var stretchSequence = []mascotFrame{
	{mascotPawLeft, 3},
	{MascotWave, 2},
	{mascotPawRight, 3},
	{MascotHappy, 3},
	{MascotStand, 1},
}

// nuzzleSequence: rapid head-tilt left+right. Faster than lookAround so
// the kitten reads as alert/curious rather than scanning the horizon.
var nuzzleSequence = []mascotFrame{
	{mascotHalfLookLeft, 1},
	{MascotLookLeft, 2},
	{mascotHalfLookRight, 1},
	{MascotLookRight, 2},
	{mascotHalfLookLeft, 1},
	{MascotLookLeft, 2},
	{MascotStand, 1},
}

// cuddleSequence: a "yay" — happy face with paws raised in alternation
// before settling. Different shape from waveSequence (which uses Stand
// at the start) so the user can tell the two animations apart.
var cuddleSequence = []mascotFrame{
	{MascotHappy, 2},
	{mascotPawLeft, 2},
	{MascotHappy, 2},
	{mascotPawRight, 2},
	{MascotHappy, 3},
	{MascotStand, 1},
}

// MascotFSM drives the mascot state machine on each 50ms tick.
// It holds the current playback position within an event sequence plus
// scheduling state for the next idle event.
type MascotFSM struct {
	// Current event being played. nil means idle (Stand).
	seq      []mascotFrame
	seqStep  int // index into seq
	stepHold int // ticks remaining in current step (counts down from hold)

	// Global tick counter used for scheduling and zZ pulsing.
	tick int

	// nextEventAt: global tick at which the next idle event fires.
	nextEventAt int

	// sleepZPhase: 0..3 cycling phase for the animated zZ during sleep.
	// Drives the 4-frame puff cycle (rising z's that fade out and restart).
	sleepZPhase int

	// rng seed state — simple LCG so we have no import dependency.
	rng uint64

	// Forced event queue — set by external triggers (mock timers or live events).
	pendingEvent *mascotEventKind

	// working is the per-tick hint set by the model — true while claude is
	// actively producing output (streaming text or running a tool). Biases
	// pickEvent toward attentive look-around / blink, suppresses sleep, and
	// fires events sooner so the mascot reads as engaged. Cleared when
	// claude goes idle.
	working bool
}

// WithWorking returns an FSM with the working hint applied. The hint is
// consulted by scheduleNext + pickEvent to decide cadence and weights.
// Idempotent and value-typed so callers store the result.
func (f MascotFSM) WithWorking(b bool) MascotFSM {
	f.working = b
	return f
}

// HasActiveSequence reports whether the mascot is currently mid-animation
// (a sequence is queued and playback hasn't returned to the resting Stand
// state). Callers use this to decide whether to keep the frame loop in
// fast mode — a settled mascot at the idle cadence is one fewer reason
// to keep the View() rebuild rate at 20Hz.
func (f MascotFSM) HasActiveSequence() bool {
	return len(f.seq) > 0 || f.pendingEvent != nil
}

// NewMascotFSM returns an FSM ready to play. Locked at frame 0 for tests.
func NewMascotFSM() MascotFSM {
	f := MascotFSM{rng: 0xdeadbeef42}
	f.nextEventAt = f.scheduleNext()
	return f
}

// lcgNext advances the LCG and returns a pseudo-random uint64.
func (f *MascotFSM) lcgNext() uint64 {
	f.rng = f.rng*6364136223846793005 + 1442695040888963407
	return f.rng
}

// randN returns a pseudo-random int in [0, n).
func (f *MascotFSM) randN(n int) int {
	if n <= 0 {
		return 0
	}
	return int(f.lcgNext()>>33) % n
}

// scheduleNext returns the tick at which the next idle event should fire.
// Cadence depends on the working flag:
//
//	working=true  → 30..60 ticks (1.5..3s) — keeps the mascot alert while
//	                claude is actively producing output
//	working=false → 100..180 ticks (5..9s) — relaxed idle cadence so quiet
//	                moments don't get a constant blink-tic
//
// The earlier 3..6s value made the mascot feel twitchy when the user was
// reading; bumping it to 5..9s when truly idle reads calmer.
func (f *MascotFSM) scheduleNext() int {
	if f.working {
		return f.tick + 30 + f.randN(30)
	}
	return f.tick + 100 + f.randN(80)
}

// pickEvent picks a weighted random idle event. The weights split based on
// the working hint so the mascot reads attentive while claude is producing
// output and varied (with rare stretches / cuddles / nuzzles) when idle.
//
// Sleep + Wave + Surprised fire from external triggers, never from the
// random picker — those need to mean something.
func (f *MascotFSM) pickEvent() mascotEventKind {
	if f.working {
		// Working: heavy on look-around so the eyes follow what claude is
		// doing; sprinkle blinks + occasional happy for warmth.
		r := f.randN(10)
		switch {
		case r < 5:
			return eventLookAround
		case r < 9:
			return eventBlink
		default:
			return eventHappy
		}
	}
	// Idle: 20-slot spread mixing the new stretch / nuzzle / cuddle
	// animations so the user sees a bigger vocabulary instead of just
	// blink-blink-blink.
	r := f.randN(20)
	switch {
	case r < 8:
		return eventBlink
	case r < 12:
		return eventLookAround
	case r < 15:
		return eventNuzzle
	case r < 17:
		return eventStretch
	case r < 19:
		return eventCuddle
	default:
		return eventHappy
	}
}

// sequenceFor returns the frame sequence for an event kind.
func sequenceFor(kind mascotEventKind) []mascotFrame {
	switch kind {
	case eventBlink:
		return blinkSequence
	case eventLookAround:
		return lookAroundSequence
	case eventHappy:
		return happySequence
	case eventSurprised:
		return surprisedSequence
	case eventSleep:
		return sleepSequence
	case eventWave:
		return waveSequence
	case eventStretch:
		return stretchSequence
	case eventNuzzle:
		return nuzzleSequence
	case eventCuddle:
		return cuddleSequence
	default:
		return blinkSequence
	}
}

// Advance moves the FSM one tick forward and returns a new FSM. Autonomous
// idle events (blink / look-around / nuzzle / stretch / cuddle / happy) and
// external triggers (pendingEvent set via SetExternalState / WithWorking)
// both fire from here.
func (f MascotFSM) Advance() MascotFSM {
	f.tick++

	// Demo-mode external triggers (hardcoded for visual variety in proto).
	// Every ~600 ticks (30s): happy flash.
	// Every ~1200 ticks (60s): surprised.
	// Every ~2400 ticks (120s): sleep.
	// Every ~3600 ticks (180s): wave.
	if f.seq == nil {
		t := f.tick
		switch {
		case t > 0 && t%3600 == 0:
			k := eventWave
			f.pendingEvent = &k
		case t > 0 && t%2400 == 0:
			k := eventSleep
			f.pendingEvent = &k
		case t > 0 && t%1200 == 0:
			k := eventSurprised
			f.pendingEvent = &k
		case t > 0 && t%600 == 0:
			k := eventHappy
			f.pendingEvent = &k
		}
	}

	// Start a new event if idle and it's time.
	if f.seq == nil {
		var kind mascotEventKind
		switch {
		case f.pendingEvent != nil:
			kind = *f.pendingEvent
			f.pendingEvent = nil
		case f.tick >= f.nextEventAt:
			kind = f.pickEvent()
		default:
			// Still idle — nothing to do.
			return f
		}
		f.seq = sequenceFor(kind)
		f.seqStep = 0
		f.stepHold = f.seq[0].hold
	}

	// Advance within the current sequence.
	f.stepHold--
	if f.stepHold <= 0 {
		f.seqStep++
		if f.seqStep >= len(f.seq) {
			// Sequence finished — return to idle.
			f.seq = nil
			f.seqStep = 0
			f.stepHold = 0
			f.nextEventAt = f.scheduleNext()
			return f
		}
		f.stepHold = f.seq[f.seqStep].hold
	}

	// Cycle the sleep zZ animation. 4 phases at 6 ticks each → ~1.2s
	// per puff cycle while the kitten is asleep.
	f.sleepZPhase = (f.tick / 6) % 4

	return f
}

// CurrentState returns the rendered base state for this tick.
func (f MascotFSM) CurrentState() MascotBaseState {
	if f.seq == nil || f.seqStep >= len(f.seq) {
		return MascotStand
	}
	return f.seq[f.seqStep].state
}

// SleepZPhase returns the current zZ animation phase (0..3).
func (f MascotFSM) SleepZPhase() int { return f.sleepZPhase }

// SetExternalState queues an external event kind to play on the next Advance tick.
// If the FSM is already mid-sequence, the new event replaces the pending event
// (latest-wins semantics). This is the hook for live-data triggers from model.go.
func (f MascotFSM) SetExternalState(kind mascotEventKind) MascotFSM {
	k := kind
	f.pendingEvent = &k
	// Interrupt any running autonomous sequence so the external event fires
	// on the very next Advance tick rather than waiting for the sequence to finish.
	f.seq = nil
	f.seqStep = 0
	f.stepHold = 0
	return f
}

// ─── ASCII frame data ────────────────────────────────────────────────────────

// kittenFrames maps every non-sleep state to its 4-line ASCII block.
// States not in the map fall back to Stand.
//
// Block layout — always exactly 4 strings so panel height stays stable:
//
//	[0] ears:   /\_/\
//	[1] face: ( o.o )
//	[2] body: /(   )\
//	[3] paws:  _"_"_
//
// Color: pink (#ff75a0) on Tokyo Night — the Mascot style.
var kittenFrames = map[MascotBaseState][4]string{
	MascotStand:         {`   /\_/\`, `  ( o.o )`, `  /(   )\`, `   _"_"_`},
	MascotBlink:         {`   /\_/\`, `  ( -.- )`, `  /(   )\`, `   _"_"_`},
	mascotPreBlink:      {`   /\_/\`, `  ( o.o )`, `  /(   )\`, `   _"_"_`},
	mascotHalfBlink:     {`   /\_/\`, `  ( ~.~ )`, `  /(   )\`, `   _"_"_`},
	MascotLookLeft:      {`   /\_/\`, `  ( ◐.◐ )`, `  /(   )\`, `   _"_"_`},
	MascotLookRight:     {`   /\_/\`, `  ( ◑.◑ )`, `  /(   )\`, `   _"_"_`},
	MascotHappy:         {`   /\_/\`, `  ( ^_^ )`, `  /(   )\`, `   _"_"_`},
	MascotSurprised:     {`   /\_/\`, `  ( O.O )`, `  /(   )\`, `   _"_"_`},
	MascotWave:          {`   /\_/\`, `  ( ^_^ )`, `  \(   )/`, `   _"_"_`},
	mascotHalfLookLeft:  {`   /\_/\`, `  ( ◐.o )`, `  /(   )\`, `   _"_"_`},
	mascotHalfLookRight: {`   /\_/\`, `  ( o.◑ )`, `  /(   )\`, `   _"_"_`},
	mascotHalfBack:      {`   /\_/\`, `  ( o.o )`, `  /(   )\`, `   _"_"_`},
	mascotDrowsy:        {`   /\_/\`, `  ( ˘.˘ )`, `  /(   )\`, `   _"_"_`},
	mascotPawLeft:       {`   /\_/\`, `  ( ^_^ )`, `  \(   )\`, `   _"_"_`},
	mascotPawRight:      {`   /\_/\`, `  ( ^_^ )`, `  /(   )/`, `   _"_"_`},
}

// kittenSleepBody is the body block for the Sleep state. The ears row gets
// the animated zZ annotation appended at render time so the puff cycles
// without disturbing the underlying ASCII layout.
var kittenSleepBody = [4]string{`   /\_/\`, `  ( -.- )`, `  /(   )\`, `   _"_"_`}

// bunnyFrames is the legacy bunny mascot kept behind the persona toggle so
// long-time clyde users can revert. Same layout as the kitten — ears, face,
// body, feet — just with the v9 rabbit shape.
var bunnyFrames = map[MascotBaseState][4]string{
	MascotStand:         {`   (\_/)`, `   (o.o)`, `  /(   )\`, `    "-"`},
	MascotBlink:         {`   (\_/)`, `   (-.-)`, `  /(   )\`, `    "-"`},
	mascotPreBlink:      {`   (\_/)`, `   (o.o)`, `  /(   )\`, `    "-"`},
	mascotHalfBlink:     {`   (\_/)`, `   (~.~)`, `  /(   )\`, `    "-"`},
	MascotLookLeft:      {`   (\_/)`, `   (◐.◐)`, `  /(   )\`, `    "-"`},
	MascotLookRight:     {`   (\_/)`, `   (◑.◑)`, `  /(   )\`, `    "-"`},
	MascotHappy:         {`   (\_/)`, `   (^.^)`, `  /(   )\`, `    "-"`},
	MascotSurprised:     {`   (\_/)`, `   (O.O)`, `  /(   )\`, `    "-"`},
	MascotWave:          {`   (\_/)`, `   (^.^)`, `  \(   )/`, `    "-"`},
	mascotHalfLookLeft:  {`   (\_/)`, `   (◐.o)`, `  /(   )\`, `    "-"`},
	mascotHalfLookRight: {`   (\_/)`, `   (o.◑)`, `  /(   )\`, `    "-"`},
	mascotHalfBack:      {`   (\_/)`, `   (o.o)`, `  /(   )\`, `    "-"`},
	mascotDrowsy:        {`   (\_/)`, `   (˘.˘)`, `  /(   )\`, `    "-"`},
	mascotPawLeft:       {`   (\_/)`, `   (^.^)`, `  \(   )\`, `    "-"`},
	mascotPawRight:      {`   (\_/)`, `   (^.^)`, `  /(   )/`, `    "-"`},
}

var bunnySleepBody = [4]string{`   (\_/)`, `   (v.v)`, `  /(   )\`, `    "-"`}

// sleepZAnnotations renders the animated zZ puff for the given phase. The
// strings always start with a single space so the renderer can split on
// " z" / " Z" and apply the dimmer MascotZZ style without touching the body.
//
// Phase cycle (4 frames, 6 ticks each at 50ms = ~1.2s per puff):
//
//	0:  " z"     ← initial poof
//	1:  " zZ"    ← growing
//	2:  " zZz"   ← peak
//	3:  " Zz"    ← dispersing
var sleepZAnnotations = [4]string{" z", " zZ", " zZz", " Zz"}

// personaFrames returns the body-frame map for the given persona. Off
// returns nil — the renderer handles that as "blank block".
func personaFrames(p MascotPersona) map[MascotBaseState][4]string {
	switch p {
	case MascotPersonaBowl:
		return bunnyFrames
	case MascotPersonaOff:
		return nil
	default:
		return kittenFrames
	}
}

// personaSleepBody returns the sleep-state body block for the given persona.
func personaSleepBody(p MascotPersona) [4]string {
	switch p {
	case MascotPersonaBowl:
		return bunnySleepBody
	default:
		return kittenSleepBody
	}
}

// blankFrame is the empty block used when persona is Off. Same height as a
// real frame so the now-panel layout stays stable.
var blankFrame = [4]string{"", "", "", ""}

// mascotLines returns the 4 raw (unstyled) ASCII lines for a given state
// plus the optional zZ annotation. The annotation is empty for non-sleep
// states; for Sleep it carries the current puff-cycle phase.
//
// Always returns exactly 4 lines (block height stays stable across states).
func mascotLines(persona MascotPersona, state MascotBaseState, sleepZPhase int) ([4]string, string) {
	if persona == MascotPersonaOff {
		return blankFrame, ""
	}
	frames := personaFrames(persona)
	if state == MascotSleep {
		body := personaSleepBody(persona)
		phase := sleepZPhase
		if phase < 0 || phase >= len(sleepZAnnotations) {
			phase = 0
		}
		return body, sleepZAnnotations[phase]
	}
	if f, ok := frames[state]; ok {
		return f, ""
	}
	return frames[MascotStand], ""
}
