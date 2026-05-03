package tui

import (
	"sort"
	"time"

	"github.com/clyde-tui/clyde/internal/application/livesession"
	"github.com/clyde-tui/clyde/internal/domain/session"
)

// SessionTab is one tab rendered in the title-bar tab strip. Tabs cover
// every recently-active session in the current cwd, plus a Σ aggregate.
type SessionTab struct {
	// ID is the session identifier; empty for the synthetic Σ aggregate
	// tab (which represents "all sessions in this cwd").
	ID session.ID

	// Label is the short text rendered inside the tab (e.g. "fix auth"
	// or a 6-char session-ID prefix). For the Σ tab this is "Σ all".
	Label string

	// Active is true for the currently-focused tab.
	Active bool

	// IsAggregate is true for the leftmost Σ tab.
	IsAggregate bool

	// ContextPct is the model-context fill percent for this session's
	// most recent assistant turn. Zero for the Σ tab.
	ContextPct int

	// Warning is true when the session is close to compaction
	// (ContextPct > sessionCtxWarnThreshold). Drives the ⚠ glyph on
	// inactive tabs so users notice the danger even when not focused on
	// that session.
	Warning bool

	// Live is true when the session was written to recently (claude code
	// still has it open). Drives the bullet glyph on the tab — orthogonal
	// to Active (which is the user's focused tab in clyde). A closed /
	// idle session that the user happens to have selected reads as
	// "[ resume ]" without a bullet; a running session that is not the
	// current tab still gets "[● label]" so the user can spot it.
	Live bool

	// Tokens is the total session token count (formatted in the panel
	// when needed). Mostly used by the Σ leaderboard view.
	Tokens int64
}

// sessionCtxWarnThreshold is the % above which a session is flagged as
// "about to compact". The user agreed on 80% as the cutoff — below that
// the user has plenty of headroom; above, they should consider compacting
// soon to avoid an automatic mid-turn compact.
const sessionCtxWarnThreshold = 80

// activeSessionWindow is the mtime-fallback window — sessions whose
// JSONL was last appended to within this window count as "active" even
// when the process probe (ports.ProcessSource) is unavailable.
//
// When the probe IS available (the default), a session also counts as
// active if its `claude` process is alive, regardless of mtime — this
// keeps `/resume`-d sessions sitting idle in the strip instead of
// dropping out after ~90s. See livesession.applySessionStats.
//
// 90 seconds is the trade-off for the fallback path: short enough
// that a closed session disappears from the strip while the user can
// still feel the cause (~1.5 min after closing), long enough that
// brief read-pauses during active work don't make tabs flicker out.
const activeSessionWindow = 90 * time.Second

// bulletActivityWindow gates the leading "●" glyph on session tabs.
// Independent from activeSessionWindow / IsLive: a `/resume`-d session
// whose `claude` process is alive but has not produced an event for
// many minutes should keep its TAB but lose the bullet — the bullet
// promises "this session is actively producing output", not "the
// process exists somewhere". 5 minutes absorbs idle-thinking pauses
// while still clearing the bullet on truly dormant sessions.
const bulletActivityWindow = 5 * time.Minute

// maxSessionTabs caps how many session tabs we render in the title bar.
// 5 sessions + Σ = 6 tabs, which fits a typical 120-col terminal.
const maxSessionTabs = 5

// sessionTabsFromView builds the tab strip from livesession.View.SessionStats.
// activeIdx is the user-selected tab index (-1 = Σ, 0..N-1 = session index).
//
// Returns an empty slice when fewer than two sessions in the cwd are within
// activeSessionWindow — a single-session cwd has nothing meaningful to switch
// between, so the title bar falls back to the path display.
func sessionTabsFromView(v livesession.View, activeIdx int, now time.Time) []SessionTab {
	if len(v.SessionStats) == 0 {
		return nil
	}
	// A session earns a tab when EITHER the `claude` process is alive
	// (IsLive — the user can still type into it) OR its JSONL was
	// appended to within activeSessionWindow (mtime fallback for hosts
	// where the process probe is unavailable). The OR is intentional:
	// a `/resume`-d session sitting idle has stale mtime but live
	// process; a freshly-closed session has dead process but recent
	// mtime — both are user-relevant tabs.
	recent := make([]livesession.SessionStat, 0, len(v.SessionStats))
	for _, st := range v.SessionStats {
		if st.IsLive || now.Sub(st.LastActivity) <= activeSessionWindow {
			recent = append(recent, st)
		}
	}
	if len(recent) < 2 {
		return nil
	}
	if len(recent) > maxSessionTabs {
		recent = recent[:maxSessionTabs]
	}

	// Active index is interpreted against the recent slice.
	out := make([]SessionTab, 0, len(recent)+1)
	out = append(out, SessionTab{
		Label:       "Σ all",
		IsAggregate: true,
		Active:      activeIdx == -1,
	})
	for i, st := range recent {
		out = append(out, SessionTab{
			ID:         st.ID,
			Label:      st.Label,
			Active:     activeIdx == i,
			ContextPct: st.ContextPct,
			Warning:    st.ContextPct > sessionCtxWarnThreshold,
			Live:       now.Sub(st.LastActivity) <= bulletActivityWindow,
			Tokens:     st.SessionTokens,
		})
	}
	return out
}

// sessionLeaderboardFromView returns the per-session list rendered as the
// "session ctx" leaderboard in the usage panel when the Σ aggregate tab is
// active. Sorted by ContextPct desc so the user sees the most-loaded
// session at the top — that's the "is anything about to compact?" signal.
//
// Capped at 3 entries so the leaderboard does not push the rest of the
// usage panel off-screen on small terminals.
func sessionLeaderboardFromView(v livesession.View, now time.Time) []SessionTab {
	if len(v.SessionStats) == 0 {
		return nil
	}
	// Same liveness rule as sessionTabsFromView: process alive OR
	// mtime within window. Keeps the leaderboard consistent with the
	// tab strip — a session appears in both views or neither.
	stats := make([]livesession.SessionStat, 0, len(v.SessionStats))
	for _, st := range v.SessionStats {
		if st.IsLive || now.Sub(st.LastActivity) <= activeSessionWindow {
			stats = append(stats, st)
		}
	}
	if len(stats) < 2 {
		return nil
	}
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].ContextPct > stats[j].ContextPct
	})
	const maxLeaderboard = 3
	if len(stats) > maxLeaderboard {
		stats = stats[:maxLeaderboard]
	}
	out := make([]SessionTab, 0, len(stats))
	for _, st := range stats {
		out = append(out, SessionTab{
			ID:         st.ID,
			Label:      st.Label,
			ContextPct: st.ContextPct,
			Warning:    st.ContextPct > sessionCtxWarnThreshold,
			Live:       st.Active,
			Tokens:     st.SessionTokens,
		})
	}
	return out
}

// resolveSessionFocus maps a SessionTabIndex into a session.ID for snapshot.
// activeIdx == -1 (Σ aggregate) returns the most-recently-active session's
// ID — the events / calls / diff panels still need *some* concrete session
// to show. The user indicates "no specific focus" via the Σ tab; we render
// the leaderboard in the usage panel but keep the rest of the UI on the
// latest session as a sensible default.
func resolveSessionFocus(v livesession.View, activeIdx int, now time.Time) session.ID {
	if len(v.SessionStats) == 0 {
		return ""
	}
	// Same liveness rule as sessionTabsFromView; without the same
	// filter here a click on tab N would resolve to a different
	// session than the renderer thinks tab N is.
	recent := make([]livesession.SessionStat, 0, len(v.SessionStats))
	for _, st := range v.SessionStats {
		if st.IsLive || now.Sub(st.LastActivity) <= activeSessionWindow {
			recent = append(recent, st)
		}
	}
	if len(recent) == 0 {
		return v.SessionStats[0].ID
	}
	if activeIdx < 0 || activeIdx >= len(recent) {
		return recent[0].ID
	}
	return recent[activeIdx].ID
}
