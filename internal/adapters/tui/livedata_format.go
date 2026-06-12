package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// formatDuration formats a duration as a compact human-readable string.
//
// Buckets:
//
//	< 60s              → "Ns"
//	< 60m, exact min   → "Nm"
//	< 60m              → "Nm Ns"
//	>= 60m, exact hour → "Nh"
//	>= 60m             → "Nh Nm"   (seconds dropped — at hour scale they're noise)
func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	secs := int(d.Seconds())
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	mins := secs / 60
	if mins < 60 {
		s := secs % 60
		if s == 0 {
			return fmt.Sprintf("%dm", mins)
		}
		return fmt.Sprintf("%dm %ds", mins, s)
	}
	hours := mins / 60
	m := mins % 60
	if m == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh %dm", hours, m)
}

// formatAge formats an idle duration as "Xs ago", "Xm ago", or "Xh Ym ago".
func formatAge(d time.Duration) string {
	return formatDuration(d) + " ago"
}

// formatTimestamp formats an instant as wall-clock "HH:MM" in the system
// local timezone. Domain/application code stores instants in UTC; the TUI
// is the boundary that converts to local for display so users read times
// that match their watch.
func formatTimestamp(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	local := ts.Local()
	return fmt.Sprintf("%02d:%02d", local.Hour(), local.Minute())
}

// deriveTitlePath extracts a short path string from a project CWD.
// Returns "~/<basename>" to fit in the title bar.
func deriveTitlePath(cwd string) (dirPath, projectName string) {
	if cwd == "" {
		return "~/", ""
	}
	name := filepath.Base(cwd)
	dir := filepath.Dir(cwd)

	// Shorten home directory prefix.
	home := homeDir()
	if home != "" && strings.HasPrefix(dir, home) {
		dir = "~" + dir[len(home):]
	}
	if !strings.HasSuffix(dir, "/") {
		dir += "/"
	}
	return dir, name
}

// homeDir returns the user's home directory path (best-effort, empty on error).
func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return h
}

// formatTokenCount formats an int64 token count as a compact string:
// <1k → "N", <10k → "N.Nk", <1M → "Nk", ≥1M → "N.NM".
func formatTokenCount(n int64) string {
	switch {
	case n < 1_000:
		return fmt.Sprintf("%d", n)
	case n < 10_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	case n < 1_000_000:
		return fmt.Sprintf("%dk", n/1_000)
	default:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
}

// shortModelName converts a full model ID to the compact display name used
// in the usage panel and title bar.
//
// Examples:
//
//	"claude-opus-4-7"       → "opus 4.7"
//	"claude-opus-4-7[1m]"   → "opus 4.7 (1M)"
//	"claude-sonnet-4-6"     → "sonnet 4.6"
func shortModelName(id string) string {
	if id == "" {
		return "unknown"
	}
	// Peel off the [1m] suffix before formatting; re-append as "(1M)".
	const oneMSuffix = "[1m]"
	is1M := strings.HasSuffix(id, oneMSuffix)
	base := id
	if is1M {
		base = id[:len(id)-len(oneMSuffix)]
	}

	// Strip the "claude-" prefix if present.
	name := strings.TrimPrefix(base, "claude-")
	// Pattern: <family>-<major>-<minor> → "<family> <major>.<minor>"
	parts := strings.SplitN(name, "-", 3)
	var short string
	switch len(parts) {
	case 3:
		short = parts[0] + " " + parts[1] + "." + parts[2]
	case 2:
		short = parts[0] + " " + parts[1]
	default:
		short = name
	}
	if is1M {
		short += " (1M)"
	}
	return short
}
