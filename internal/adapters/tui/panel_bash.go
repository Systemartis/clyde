package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
)

// renderBashExpanded renders the bash audit panel with active-mode support
// (scrollable viewport in active state, normal flow otherwise).
func renderBashExpanded(s Styles, p Palette, d MockData, vp viewport.Model, width, height int, focused, activeMode bool) string {
	if activeMode {
		inner := width - 4
		vp.SetWidth(inner)
		vp.SetHeight(height - 2)
		content := vp.View()
		return wrapPanelActive(s, content, "bash", width, height)
	}
	return renderBash(s, p, d, width, height, focused)
}

// renderBash renders the bash audit panel — a chronological ledger of every
// Bash command claude has run in this session. Failed runs are colored red.
func renderBash(s Styles, _ Palette, d MockData, width, height int, focused bool) string {
	inner := width - 4
	innerH := height - 2
	if innerH < 1 {
		innerH = 1
	}

	failed, total := bashCounts(d.BashLog)
	var meta string
	if total == 0 {
		meta = "no commands yet"
	} else {
		meta = fmt.Sprintf("%d ran · %d failed", total, failed)
	}

	if total == 0 {
		empty := s.TextFade.Render("  no Bash commands recorded yet")
		return wrapPanel(s, empty, "bash", "", width, height, focused)
	}

	// Show the last innerH-1 commands (newest at the bottom — fits the
	// "what just happened" mental model). User scrolls back via active mode.
	from := 0
	if len(d.BashLog) > innerH-1 {
		from = len(d.BashLog) - (innerH - 1)
	}
	visible := d.BashLog[from:]
	more := from
	var sb strings.Builder
	if more > 0 {
		sb.WriteString(s.TextFade.Render(fmt.Sprintf("  + %d earlier", more)))
		sb.WriteByte('\n')
	}
	for _, b := range visible {
		sb.WriteString(renderBashRow(s, b, inner))
		sb.WriteByte('\n')
	}
	content := strings.TrimRight(sb.String(), "\n")
	return wrapPanel(s, content, "bash", meta, width, height, focused)
}

// renderBashRow renders one ledger line: TIME  $ COMMAND  ✓/✗  DURATION
// formatted as a left-flush prefix + right-flush duration.
func renderBashRow(s Styles, b BashRow, inner int) string {
	timeCell := s.TaskDur.Render(b.Time)
	dollar := s.TextFade.Render(" $ ")

	cmdMaxW := inner - 18 // time(8) + $(3) + state(2) + dur(~5) + padding
	if cmdMaxW < 12 {
		cmdMaxW = 12
	}
	cmd := truncate(b.Command, cmdMaxW)
	var cmdRendered string
	switch b.State {
	case CallFailed:
		cmdRendered = s.DiffRem.Render(cmd)
	case CallActive:
		cmdRendered = s.TaskSubActName.Render(cmd)
	default:
		cmdRendered = s.TaskSubDoneName.Render(cmd)
	}

	state := ""
	switch b.State {
	case CallDone:
		state = s.StatusGreen.Render("✓")
	case CallFailed:
		state = s.DiffRem.Render("✗")
	case CallActive:
		state = s.TaskSubActIcon.Render("▶")
	}

	dur := s.TaskDur.Render(b.Duration)

	left := timeCell + dollar + cmdRendered + " " + state
	leftW := ansiWidth(left)
	durW := ansiWidth(dur)
	gapW := inner - leftW - durW
	if gapW < 1 {
		gapW = 1
	}
	return left + strings.Repeat(" ", gapW) + dur
}

// renderBashCollapsed renders the collapsed one-liner.
func renderBashCollapsed(s Styles, d MockData, width int, focused bool) string {
	failed, total := bashCounts(d.BashLog)
	var summary string
	switch {
	case total == 0:
		summary = "no commands"
	case failed == 0:
		summary = fmt.Sprintf("%d ran · all clean", total)
	default:
		summary = fmt.Sprintf("%d ran · %d failed", total, failed)
	}
	return wrapPanelCollapsed(s, "bash", summary, "", width, focused)
}

// buildBashViewportContent returns the full ledger for active-mode scrolling.
func buildBashViewportContent(s Styles, d MockData, inner int) string {
	if len(d.BashLog) == 0 {
		return s.TextFade.Render("  no Bash commands recorded yet")
	}
	var sb strings.Builder
	for _, b := range d.BashLog {
		sb.WriteString(renderBashRow(s, b, inner))
		sb.WriteByte('\n')
	}
	return strings.TrimRight(sb.String(), "\n")
}

// bashCounts returns (failed, total) for a BashLog slice.
func bashCounts(log []BashRow) (failed, total int) {
	total = len(log)
	for _, b := range log {
		if b.State == CallFailed {
			failed++
		}
	}
	return failed, total
}
