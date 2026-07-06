package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// BootScreen drives the animated startup splash. The renderer is pure: it
// reads the tick counter and paints whatever the current phase requires,
// so tests can drive it deterministically by setting Tick directly.
type BootScreen struct {
	// Active is true while the splash is on screen. The model gates the
	// regular View() pipeline on this and routes the next FrameMsg into
	// Advance(). Cleared by Dismiss() (any key) or auto-finish.
	Active bool
	// Tick counts FrameMsg ticks since the splash was triggered.
	Tick int
}

// bootDuration is the total length of the splash animation in ticks.
// At the 50ms FrameMsg cadence that's ~1.9s — long enough for the
// wordmark + tagline to fully fade in (last reveal lands at tick 34),
// short enough that the splash auto-dismisses without making a
// habitual launcher tap a key. The user explicitly asked for an
// auto-skip rather than a 3-second linger.
const bootDuration = 38

// Boot animation phase boundaries (in ticks).
const (
	bootPhaseFadeKitten  = 6  // 0..6: kitten silhouette fades in
	bootPhaseWaveStart   = 6  // 6..18: kitten waves both paws
	bootPhaseTypeStart   = 12 // 12..27: "clyde" wordmark types in (3 ticks per letter × 5)
	bootPhaseTaglineFade = 28 // 28..34: tagline + skip-hint fade in
)

// Advance returns a new BootScreen with the tick incremented and Active
// cleared once the auto-finish window passes.
func (b BootScreen) Advance() BootScreen {
	if !b.Active {
		return b
	}
	b.Tick++
	if b.Tick >= bootDuration {
		b.Active = false
	}
	return b
}

// Dismiss flips Active off — used when the user hits any key during boot.
func (b BootScreen) Dismiss() BootScreen {
	b.Active = false
	return b
}

// renderBootScreen paints the boot splash for the given tick + palette,
// sized to fit the full terminal window. Returns a width × height block
// suitable for Bubble Tea's View. Theme-aware: uses the active palette
// for the wordmark gradient + chrome accents.
func renderBootScreen(p Palette, b BootScreen, totalW, totalH int) string {
	if totalW < 30 {
		totalW = 30
	}
	if totalH < 12 {
		totalH = 12
	}

	rows := make([]string, 0, totalH)

	// ── Row composition ────────────────────────────────────────────────
	// 1. blank padding above to vertically center the content stack
	// 2. kitten block (4 rows)
	// 3. blank gap (1 row)
	// 4. wordmark (3 rows)
	// 5. blank gap (1 row)
	// 6. tagline (1 row)
	// 7. blank gap (2 rows)
	// 8. skip-hint (1 row)
	// 9. blank padding below

	kitten := bootKittenLines(p, b.Tick)
	wordmark := bootWordmarkLines(p, b.Tick)
	tagline := bootTaglineLine(p, b.Tick)
	skipHint := bootSkipHintLine(p, b.Tick)

	contentH := len(kitten) + 1 + len(wordmark) + 1 + 1 + 2 + 1
	topPad := (totalH - contentH) / 2
	if topPad < 0 {
		topPad = 0
	}

	for i := 0; i < topPad; i++ {
		rows = append(rows, strings.Repeat(" ", totalW))
	}
	for _, line := range kitten {
		rows = append(rows, bootCenterLine(line, totalW))
	}
	rows = append(rows, strings.Repeat(" ", totalW))
	for _, line := range wordmark {
		rows = append(rows, bootCenterLine(line, totalW))
	}
	rows = append(rows, strings.Repeat(" ", totalW))
	rows = append(rows, bootCenterLine(tagline, totalW))
	rows = append(rows, strings.Repeat(" ", totalW))
	rows = append(rows, strings.Repeat(" ", totalW))
	rows = append(rows, bootCenterLine(skipHint, totalW))

	// Pad the rest down so the splash fills the screen.
	for len(rows) < totalH {
		rows = append(rows, strings.Repeat(" ", totalW))
	}
	if len(rows) > totalH {
		rows = rows[:totalH]
	}

	return strings.Join(rows, "\n")
}

// bootKittenLines returns the styled 4-line kitten block for the given tick.
// Phase 1 (0..6): the silhouette fades in by stepping from TextFade →
// TextDim → Pink. Phase 2 (6..18): a wave sequence (pawRight → wave →
// pawLeft → wave) plays on a 3-tick cadence. After 18 ticks the kitten
// settles back into a neutral happy stand.
func bootKittenLines(p Palette, tick int) []string {
	col := p.Pink
	switch {
	case tick < 2:
		col = p.TextFade
	case tick < 4:
		col = p.TextDim
	case tick < bootPhaseFadeKitten:
		col = p.TextMid
	}

	// Pick the kitten frame based on the wave-phase sub-tick.
	var state MascotBaseState
	switch {
	case tick < bootPhaseWaveStart:
		state = MascotHappy // pre-wave: warm grin while fading in
	case tick < bootPhaseWaveStart+12:
		// Wave cycle: 4 frames × 3 ticks each = 12 ticks total.
		switch (tick - bootPhaseWaveStart) / 3 % 4 {
		case 0:
			state = mascotPawRight
		case 1:
			state = MascotWave
		case 2:
			state = mascotPawLeft
		case 3:
			state = MascotWave
		}
	default:
		state = MascotHappy
	}

	body, _ := mascotLines(MascotPersonaMeowl, state, 0)

	// Normalize every row to the block's widest line before styling. The
	// raw frames are block-aligned (ears/paws are 8 cells, face/body 9),
	// but bootCenterLine centers each row independently by its own width —
	// so the narrower rows round ~1 cell to the right and the ears drift
	// off-center. Equal widths make the whole mascot center as a unit.
	maxW := 0
	for _, line := range body {
		if w := ansiWidth(line); w > maxW {
			maxW = w
		}
	}

	style := lipgloss.NewStyle().Foreground(col)
	out := make([]string, len(body))
	for i, line := range body {
		if pad := maxW - ansiWidth(line); pad > 0 {
			line += strings.Repeat(" ", pad)
		}
		out[i] = style.Render(line)
	}
	return out
}

// clydeLetters holds the 3-row block-glyph design for each letter of the
// wordmark. Width: 3 cols per glyph + 1 col gutter between letters.
var clydeLetters = [5][3]string{
	// C
	{`█▀▀`, `█  `, `▀▀▀`},
	// L
	{`█  `, `█  `, `▀▀▀`},
	// Y
	{`█ █`, ` █ `, ` ▀ `},
	// D
	{`█▀▄`, `█ █`, `▀▀ `},
	// E
	{`█▀▀`, `█▀▀`, `▀▀▀`},
}

// bootWordmarkLines returns the styled 3-row "CLYDE" wordmark for the
// given tick. Letters appear one-by-one starting at bootPhaseTypeStart
// at a cadence of 3 ticks per letter. Each letter gets a distinct color
// from the active palette (Cyan → BorderAcc → Purple → Magenta → Pink)
// so the finished wordmark reads as a left-to-right gradient.
func bootWordmarkLines(p Palette, tick int) []string {
	letterColors := []color.Color{p.Cyan, p.BorderAcc, p.Purple, p.Magenta, p.Pink}
	revealed := 0
	if tick > bootPhaseTypeStart {
		revealed = (tick - bootPhaseTypeStart) / 3
		if revealed > 5 {
			revealed = 5
		}
	}
	rows := make([]string, 3)
	for r := 0; r < 3; r++ {
		var b strings.Builder
		for i := 0; i < 5; i++ {
			if i > 0 {
				b.WriteString(" ")
			}
			if i < revealed {
				style := lipgloss.NewStyle().Foreground(letterColors[i]).Bold(true)
				b.WriteString(style.Render(clydeLetters[i][r]))
			} else {
				// Placeholder spaces so the layout doesn't shift when each
				// letter pops in — the wordmark grows in place rather than
				// sliding right.
				b.WriteString(strings.Repeat(" ", 3))
			}
		}
		rows[r] = b.String()
	}
	return rows
}

// bootTaglineLine returns the tagline shown below the wordmark. Fades in
// from TextFade → TextDim → TextMid over 6 ticks once the wordmark has
// finished typing.
func bootTaglineLine(p Palette, tick int) string {
	const tagline = "your claude code companion"
	if tick < bootPhaseTaglineFade {
		return ""
	}
	col := p.TextMid
	switch {
	case tick < bootPhaseTaglineFade+2:
		col = p.TextFade
	case tick < bootPhaseTaglineFade+4:
		col = p.TextDim
	}
	style := lipgloss.NewStyle().Foreground(col).Italic(true)
	return style.Render(tagline)
}

// bootSkipHintLine is intentionally empty now — at the tightened ~1.9s
// total duration, a "press any key to skip" hint would barely surface
// before auto-finish. Kept as a no-op so the layout math in
// renderBootScreen doesn't shift if the hint comes back in a future
// design.
func bootSkipHintLine(_ Palette, _ int) string { return "" }

// bootCenterLine pads a styled line so its visible content is horizontally
// centered inside a totalW-wide row. Uses ansiWidth so ANSI escape
// sequences don't inflate the measured width. Local to boot.go to avoid
// colliding with the notification overlay's centerLine helper.
func bootCenterLine(line string, totalW int) string {
	visible := ansiWidth(line)
	if visible >= totalW {
		return line
	}
	left := (totalW - visible) / 2
	right := totalW - visible - left
	return strings.Repeat(" ", left) + line + strings.Repeat(" ", right)
}
