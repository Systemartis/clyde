package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// renderMascotBlock returns the mascot as a 4-line block. The persona
// (kitten / bunny / off) selects which ASCII set is drawn; the block is
// always exactly 4 rows so the panel height never changes between states
// (eliminates layout jitter during animation transitions).
//
// The mascot block and status block are joined horizontally at Top so that
// animation offsets within the mascot block never shift the status text.
func renderMascotBlock(s Styles, frame FrameState, persona MascotPersona) string {
	fsm := frame.Mascot
	state := fsm.CurrentState()
	lines, zz := mascotLines(persona, state, fsm.SleepZPhase())

	rendered := make([]string, 4)
	for i, line := range lines {
		if i == 0 && zz != "" {
			// Sleep state: ears row carries the animated zZ annotation.
			// Render the body in the mascot color, the zZ in the dimmer
			// MascotZZ so the animation reads as breath / steam rising.
			rendered[i] = s.Mascot.Render(line) + s.MascotZZ.Render(zz)
		} else {
			rendered[i] = s.Mascot.Render(line)
		}
	}
	return strings.Join(rendered, "\n")
}

// renderStatusBlock returns the 3-line status block:
//
//	◐ edit auth.ts
//	running ~ 14s
//	(empty — LSP/lint indicators deferred to Phase H)
//
// Row 3 is intentionally blank until LSP integration lands in Phase H.
func renderStatusBlock(s Styles, d MockData, frame FrameState) string {
	spinner := frame.SpinnerGlyph()
	if d.NowMode == "idle" {
		spinner = "●"
	}

	// Line 1: spinner + op
	line1 := s.NowOp.Render(spinner+" ") + s.NowCode.Render(d.NowOp)

	// Line 2: meta (elapsed time, token rate, or path info)
	line2 := s.NowMeta.Render(d.NowMeta)

	// Line 3: blank — LSP/lint/compile indicators deferred to Phase H.
	line3 := ""

	return strings.Join([]string{line1, line2, line3}, "\n")
}

// renderNow renders the "now" panel with bunny mascot + current status.
//
// The mascot (4-line block) and status (3-line block) are joined horizontally
// at Top. The status aligns to the top of the mascot block; line 4 of the
// mascot block stands alone below the status text.
//
// Layout (4 content lines + 2 border = 6 rows total):
//
//	╭ now ──────────────────────────── writing · 47 t/s ╮
//	│   (\_/)    ◐ edit auth.ts                          │
//	│   (o.o)    14 lines staged · line 46               │
//	│  /(   )\   ● ts  ● lint  ▸ compile                 │
//	│    "-"                                              │
//	╰─────────────────────────────────────────────────────╯
func renderNow(s Styles, _ Palette, d MockData, frame FrameState, persona MascotPersona, width, _ int, focused bool) string {
	// Status block: 3 lines (independent — never moves with mascot animation)
	statusBlock := renderStatusBlock(s, d, frame)

	var joined string
	if persona == MascotPersonaOff {
		// Mascot hidden — let the status block carry the panel by itself.
		// We still need a 4-row content block to keep the panel height stable
		// at 6 rows; pad the status with a trailing empty line.
		joined = statusBlock + "\n"
	} else {
		// Mascot block: 4 lines (stable height across all states)
		mascotBlock := renderMascotBlock(s, frame, persona)
		// Join horizontally at Top: mascot (4 lines) + gap + status (3 lines).
		// The status starts at line 1 (top-aligned); line 4 of the mascot block
		// stands alone below the status content.
		gap := "  "
		joined = lipgloss.JoinHorizontal(lipgloss.Top, mascotBlock, gap, statusBlock)
	}

	// Content height = 4 (mascot block height, always stable)
	// Panel height = 4 content + 2 border = 6 rows total
	panelH := 6

	// Border meta intentionally empty: the now-panel's status block already
	// surfaces the operation + meta inside the body (line 1 + line 2), so a
	// border chip echoing the same mode-string read as duplicate noise.
	return wrapPanel(s, joined, "now", "", width, panelH, focused)
}

// collapsedPersonaGlyph returns the inline mascot indicator used in the
// collapsed now-panel summary. Kept short so the one-liner stays readable
// at narrow widths; off returns empty so the layout shifts to a clean
// status-only summary.
func collapsedPersonaGlyph(p MascotPersona) string {
	switch p {
	case MascotPersonaBowl:
		return `(\_/)`
	case MascotPersonaOff:
		return ""
	default:
		return `/\_/\`
	}
}

// renderNowCollapsed renders the collapsed one-liner for the now panel.
func renderNowCollapsed(s Styles, d MockData, frame FrameState, persona MascotPersona, width int, focused bool) string {
	_ = frame
	glyph := collapsedPersonaGlyph(persona)
	var summary string
	if glyph != "" {
		summary = glyph + "  " + d.NowOp + " · " + d.NowMeta
	} else {
		summary = d.NowOp + " · " + d.NowMeta
	}
	return wrapPanelCollapsed(s, "now", summary, "", width, focused)
}
