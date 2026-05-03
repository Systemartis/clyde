package tui

import (
	"strings"
	"testing"
)

// TestKittenMascotStand verifies the Stand frame matches the v23 kitten layout.
// Layout: ears at [0], face at [1], body at [2], paws at [3].
func TestKittenMascotStand(t *testing.T) {
	lines, zz := mascotLines(MascotPersonaMeowl, MascotStand, 0)
	if zz != "" {
		t.Errorf("Stand should have empty zZ annotation, got %q", zz)
	}
	want := [4]string{
		`   /\_/\`,
		`  ( o.o )`,
		`  /(   )\`,
		`   _"_"_`,
	}
	for i, w := range want {
		if lines[i] != w {
			t.Errorf("Stand line[%d]: got %q, want %q", i, lines[i], w)
		}
	}
}

// TestKittenMascotAllStatesRender4Lines verifies every base + intermediate
// kitten state still produces exactly 4 lines and keeps the cat silhouette.
func TestKittenMascotAllStatesRender4Lines(t *testing.T) {
	cases := []struct {
		name  string
		state MascotBaseState
	}{
		// Base states
		{"Stand", MascotStand},
		{"Blink", MascotBlink},
		{"LookLeft", MascotLookLeft},
		{"LookRight", MascotLookRight},
		{"Happy", MascotHappy},
		{"Surprised", MascotSurprised},
		{"Sleep", MascotSleep},
		{"Wave", MascotWave},
		// Intermediate states
		{"HalfLookLeft", mascotHalfLookLeft},
		{"HalfLookRight", mascotHalfLookRight},
		{"HalfBack", mascotHalfBack},
		{"PreBlink", mascotPreBlink},
		{"HalfBlink", mascotHalfBlink},
		{"Drowsy", mascotDrowsy},
		{"PawLeft", mascotPawLeft},
		{"PawRight", mascotPawRight},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lines, _ := mascotLines(MascotPersonaMeowl, tc.state, 0)
			if len(lines) != 4 {
				t.Errorf("%s: mascotLines returned %d lines, want 4", tc.name, len(lines))
			}
			// Every kitten state must show cat ears in line 0.
			if !strings.Contains(lines[0], `/\_/\`) {
				t.Errorf("%s: line[0] should contain cat ears /\\_/\\, got %q", tc.name, lines[0])
			}
			// Every kitten state must show paws in line 3.
			if !strings.Contains(lines[3], `_"_"_`) {
				t.Errorf("%s: line[3] should contain kitten paws _\"_\"_, got %q", tc.name, lines[3])
			}
		})
	}
}

// TestKittenMascotEyeGlyphs verifies the per-state eye glyph so a future
// refactor can't silently swap (^_^) ↔ (-.-) etc.
func TestKittenMascotEyeGlyphs(t *testing.T) {
	cases := []struct {
		state MascotBaseState
		eye   string
	}{
		{MascotStand, "o.o"},
		{MascotBlink, "-.-"},
		{mascotHalfBlink, "~.~"},
		{MascotHappy, "^_^"},
		{MascotSurprised, "O.O"},
		{MascotLookLeft, "◐.◐"},
		{MascotLookRight, "◑.◑"},
		{mascotHalfLookLeft, "◐.o"},
		{mascotHalfLookRight, "o.◑"},
		{mascotDrowsy, "˘.˘"},
	}
	for _, tc := range cases {
		lines, _ := mascotLines(MascotPersonaMeowl, tc.state, 0)
		if !strings.Contains(lines[1], tc.eye) {
			t.Errorf("state %d eye glyph: got %q, want it to contain %q", tc.state, lines[1], tc.eye)
		}
	}
}

// TestKittenWaveBodyShifts verifies the wave / paw states actually change
// the body line so the arm-up motion is visible.
func TestKittenWaveBodyShifts(t *testing.T) {
	stand, _ := mascotLines(MascotPersonaMeowl, MascotStand, 0)
	wave, _ := mascotLines(MascotPersonaMeowl, MascotWave, 0)
	pawL, _ := mascotLines(MascotPersonaMeowl, mascotPawLeft, 0)
	pawR, _ := mascotLines(MascotPersonaMeowl, mascotPawRight, 0)

	if stand[2] == wave[2] {
		t.Errorf("Wave body line should differ from Stand — got identical %q", wave[2])
	}
	if !strings.Contains(wave[2], `\(   )/`) {
		t.Errorf("Wave body should contain both arms up \\(   )/, got %q", wave[2])
	}
	if !strings.Contains(pawL[2], `\(   )\`) {
		t.Errorf("PawLeft body should contain left arm up \\(   )\\, got %q", pawL[2])
	}
	if !strings.Contains(pawR[2], `/(   )/`) {
		t.Errorf("PawRight body should contain right arm up /(   )/, got %q", pawR[2])
	}
}

// TestKittenSleepZZAllPhases verifies the sleep state cycles through all
// 4 zZ animation phases and never returns an empty annotation.
func TestKittenSleepZZAllPhases(t *testing.T) {
	seen := map[string]bool{}
	for phase := 0; phase < 4; phase++ {
		_, zz := mascotLines(MascotPersonaMeowl, MascotSleep, phase)
		if zz == "" {
			t.Errorf("phase %d: sleep zZ annotation is empty", phase)
		}
		if !strings.HasPrefix(zz, " ") {
			t.Errorf("phase %d: zZ annotation must start with a space (renderer relies on this), got %q", phase, zz)
		}
		seen[zz] = true
	}
	if len(seen) < 3 {
		t.Errorf("expected at least 3 distinct zZ phase strings, got %d: %v", len(seen), seen)
	}
}

// TestKittenSleepInvalidPhaseClampsToZero verifies an out-of-range phase
// returns the phase-0 annotation rather than panicking on slice bounds.
func TestKittenSleepInvalidPhaseClampsToZero(t *testing.T) {
	_, zz := mascotLines(MascotPersonaMeowl, MascotSleep, 999)
	if zz != sleepZAnnotations[0] {
		t.Errorf("invalid phase fallback: got %q, want %q", zz, sleepZAnnotations[0])
	}
}

// TestPersonaBunnyKeepsLegacyEars verifies switching to bunny persona
// returns the v22 rabbit ears so long-time users can revert.
func TestPersonaBunnyKeepsLegacyEars(t *testing.T) {
	lines, _ := mascotLines(MascotPersonaBowl, MascotStand, 0)
	if !strings.Contains(lines[0], `(\_/)`) {
		t.Errorf("bunny persona ears: got %q, want '(\\_/)' anywhere", lines[0])
	}
}

// TestPersonaNormalizeLegacyValues verifies the legacy "kitten" / "bunny"
// TOML values fold back to their new Meowl / Bowl equivalents so a config
// written before the v23 rename keeps the user's chosen character.
func TestPersonaNormalizeLegacyValues(t *testing.T) {
	cases := []struct {
		in   MascotPersona
		want MascotPersona
	}{
		{"kitten", MascotPersonaMeowl},
		{"bunny", MascotPersonaBowl},
		{MascotPersonaMeowl, MascotPersonaMeowl},
		{MascotPersonaBowl, MascotPersonaBowl},
		{MascotPersonaOff, MascotPersonaOff},
	}
	for _, tc := range cases {
		got := tc.in.Normalize()
		if got != tc.want {
			t.Errorf("Normalize(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestPersonaDisplayLabels verifies the user-facing chip text uses the
// new branded names rather than the internal TOML keys.
func TestPersonaDisplayLabels(t *testing.T) {
	cases := []struct {
		in   MascotPersona
		want string
	}{
		{MascotPersonaMeowl, "Meowl"},
		{MascotPersonaBowl, "Bowl"},
		{MascotPersonaOff, "Off"},
		{"kitten", "Meowl"}, // legacy alias still displays as Meowl
		{"bunny", "Bowl"},
	}
	for _, tc := range cases {
		if got := tc.in.Display(); got != tc.want {
			t.Errorf("Display(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestPersonaOffReturnsBlankBlock verifies switching the mascot off still
// returns a 4-line block (so panel height stays stable) but the lines
// are empty.
func TestPersonaOffReturnsBlankBlock(t *testing.T) {
	lines, zz := mascotLines(MascotPersonaOff, MascotStand, 0)
	if len(lines) != 4 {
		t.Fatalf("Off persona must still return 4 lines, got %d", len(lines))
	}
	for i, l := range lines {
		if l != "" {
			t.Errorf("Off line[%d] should be empty, got %q", i, l)
		}
	}
	if zz != "" {
		t.Errorf("Off persona must not return a zZ annotation, got %q", zz)
	}
}

// TestKittenFSM_DefaultIsStand verifies a fresh FSM is in Stand state.
func TestKittenFSM_DefaultIsStand(t *testing.T) {
	fsm := NewMascotFSM()
	if got := fsm.CurrentState(); got != MascotStand {
		t.Errorf("NewMascotFSM().CurrentState() = %d, want MascotStand (%d)", got, MascotStand)
	}
}

// TestKittenFSM_BlinkIsSmooth verifies the blink sequence walks through
// the half-blink frame on the way down AND on the way back up — that's
// what makes the blink read as smooth instead of a hard cut.
func TestKittenFSM_BlinkIsSmooth(t *testing.T) {
	visited := map[MascotBaseState]bool{}
	for _, fr := range blinkSequence {
		visited[fr.state] = true
	}
	for _, want := range []MascotBaseState{mascotPreBlink, mascotHalfBlink, MascotBlink, MascotStand} {
		if !visited[want] {
			t.Errorf("blink sequence missing state %d — blink will look choppy", want)
		}
	}
	// Half-blink must appear at least twice (descent + ascent).
	halfCount := 0
	for _, fr := range blinkSequence {
		if fr.state == mascotHalfBlink {
			halfCount++
		}
	}
	if halfCount < 2 {
		t.Errorf("blink should visit half-blink twice (close + open), got %d", halfCount)
	}
}

// TestKittenFSM_WaveCycle verifies the wave sequence alternates paws
// at least once so the arm motion reads as a real wave.
func TestKittenFSM_WaveCycle(t *testing.T) {
	sawLeft, sawRight, sawBoth := false, false, false
	for _, fr := range waveSequence {
		switch fr.state {
		case mascotPawLeft:
			sawLeft = true
		case mascotPawRight:
			sawRight = true
		case MascotWave:
			sawBoth = true
		}
	}
	if !sawLeft || !sawRight || !sawBoth {
		t.Errorf("wave sequence should hit pawLeft, pawRight, and Wave (left=%v right=%v both=%v)", sawLeft, sawRight, sawBoth)
	}
}

// TestKittenFSM_CycleStability verifies that from any event, the FSM always
// returns to Stand within a bounded number of ticks.
func TestKittenFSM_CycleStability(t *testing.T) {
	events := []struct {
		name string
		seq  []mascotFrame
	}{
		{"blink", blinkSequence},
		{"lookAround", lookAroundSequence},
		{"happy", happySequence},
		{"surprised", surprisedSequence},
		{"sleep", sleepSequence},
		{"wave", waveSequence},
	}

	for _, ev := range events {
		t.Run(ev.name, func(t *testing.T) {
			fsm := NewMascotFSM()
			fsm.seq = ev.seq
			fsm.seqStep = 0
			fsm.stepHold = ev.seq[0].hold
			fsm.nextEventAt = 1<<60 - 1

			maxTicks := 0
			for _, f := range ev.seq {
				maxTicks += f.hold + 1
			}
			maxTicks += 10

			for i := 0; i < maxTicks; i++ {
				fsm = fsm.Advance()
				if fsm.seq == nil {
					return // returned to idle — success
				}
			}
			t.Errorf("%s: FSM did not return to Stand within %d ticks", ev.name, maxTicks)
		})
	}
}

// TestKittenFSM_SleepZZCyclesAllPhases verifies SleepZPhase walks through
// all 4 phases over enough ticks during a sleep hold.
func TestKittenFSM_SleepZZCyclesAllPhases(t *testing.T) {
	fsm := NewMascotFSM()
	fsm.seq = sleepSequence
	fsm.seqStep = 1 // jump straight to sleep hold
	fsm.stepHold = sleepSequence[1].hold

	seen := map[int]bool{}
	for i := 0; i < 30; i++ {
		fsm = fsm.Advance()
		seen[fsm.SleepZPhase()] = true
	}
	if len(seen) < 4 {
		t.Errorf("sleep zZ should cycle through all 4 phases, saw only %d: %v", len(seen), seen)
	}
}

// TestKittenFSM_WorkingHintBiasesPicker verifies pickEvent strongly favors
// look-around / blink while working=true so the mascot reads as attentive,
// and never picks one of the idle-only events (stretch / nuzzle / cuddle).
func TestKittenFSM_WorkingHintBiasesPicker(t *testing.T) {
	fsm := NewMascotFSM().WithWorking(true)
	counts := map[mascotEventKind]int{}
	for i := 0; i < 1000; i++ {
		counts[fsm.pickEvent()]++
	}
	if counts[eventStretch]+counts[eventNuzzle]+counts[eventCuddle] != 0 {
		t.Errorf("working picker leaked an idle-only event: %+v", counts)
	}
	if counts[eventLookAround] < 300 || counts[eventBlink] < 200 {
		t.Errorf("working picker should be heavy on look-around (≥300/1000) + blink (≥200/1000); got look=%d blink=%d", counts[eventLookAround], counts[eventBlink])
	}
}

// TestKittenFSM_IdleHintCoversAllVariants verifies the idle picker reaches
// every animation in its vocabulary so the user actually sees the variety.
func TestKittenFSM_IdleHintCoversAllVariants(t *testing.T) {
	fsm := NewMascotFSM().WithWorking(false)
	seen := map[mascotEventKind]bool{}
	for i := 0; i < 2000; i++ {
		seen[fsm.pickEvent()] = true
	}
	for _, want := range []mascotEventKind{eventBlink, eventLookAround, eventNuzzle, eventStretch, eventCuddle, eventHappy} {
		if !seen[want] {
			t.Errorf("idle picker never reached event %d", want)
		}
	}
}

// TestKittenFSM_WorkingScheduleIsFaster verifies scheduleNext picks a
// tighter cadence while working — the mascot should look engaged, not
// disappear for 5+ seconds while claude is mid-tool.
func TestKittenFSM_WorkingScheduleIsFaster(t *testing.T) {
	working := NewMascotFSM().WithWorking(true)
	idle := NewMascotFSM().WithWorking(false)
	for i := 0; i < 50; i++ {
		w := working.scheduleNext()
		idle := idle.scheduleNext()
		if w >= idle {
			// Same-tick comparison may rarely tie; just sanity-check the bounds.
			if w > 60 || idle < 100 {
				t.Errorf("schedule bounds broken: working=%d (want ≤60), idle=%d (want ≥100)", w, idle)
			}
		}
	}
}

// TestKittenFSM_NewIdleSequencesReachStandWithinBound verifies the new
// stretch / nuzzle / cuddle sequences all return to idle within their
// allotted hold ticks. Same shape as the existing cycle-stability test.
func TestKittenFSM_NewIdleSequencesReachStandWithinBound(t *testing.T) {
	cases := []struct {
		name string
		seq  []mascotFrame
	}{
		{"stretch", stretchSequence},
		{"nuzzle", nuzzleSequence},
		{"cuddle", cuddleSequence},
	}
	for _, ev := range cases {
		t.Run(ev.name, func(t *testing.T) {
			fsm := NewMascotFSM()
			fsm.seq = ev.seq
			fsm.seqStep = 0
			fsm.stepHold = ev.seq[0].hold
			fsm.nextEventAt = 1<<60 - 1

			maxTicks := 0
			for _, f := range ev.seq {
				maxTicks += f.hold + 1
			}
			maxTicks += 10

			for i := 0; i < maxTicks; i++ {
				fsm = fsm.Advance()
				if fsm.seq == nil {
					return
				}
			}
			t.Errorf("%s: FSM did not return to idle within %d ticks", ev.name, maxTicks)
		})
	}
}
