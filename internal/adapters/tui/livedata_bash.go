package tui

import (
	"fmt"
	"time"

	"github.com/clyde-tui/clyde/internal/application/livesession"
)

// deriveBashLog converts the live session's chronological Bash ToolCall list
// into the BashRow rows the panel renders. Always overwrites in live mode
// (caller is applyLiveView, which is live-only) — when the focused session
// has zero Bash calls we want an empty panel, not stale entries from a
// previously-focused session.
func deriveBashLog(v livesession.View, d MockData) MockData {
	rows := make([]BashRow, 0, len(v.BashLog))
	for _, c := range v.BashLog {
		rows = append(rows, BashRow{
			Time:     formatBashTime(c.StartTime),
			Command:  c.KeyArg,
			Duration: formatBashDuration(c.Duration, c.State),
			State:    mapCallState(c.State),
		})
	}
	d.BashLog = rows
	return d
}

func formatBashTime(t time.Time) string {
	if t.IsZero() {
		return "        "
	}
	local := t.Local()
	return fmt.Sprintf("%02d:%02d:%02d", local.Hour(), local.Minute(), local.Second())
}

func formatBashDuration(d time.Duration, state livesession.CallState) string {
	if state == livesession.CallActive || d <= 0 {
		return ""
	}
	secs := d.Seconds()
	if secs < 1 {
		return "<1s"
	}
	if secs < 60 {
		return fmt.Sprintf("%ds", int(secs))
	}
	return fmt.Sprintf("%.1fm", secs/60)
}
