package tui

import (
	"fmt"
	"testing"
	"time"

	"github.com/Systemartis/clyde/internal/application/livesession"
	"github.com/Systemartis/clyde/internal/domain/session"
)

// TestSessionTabsFromView_HidesSingleSession verifies that a cwd with only
// one recently-active session does not show the tab strip — there is
// nothing meaningful to switch between.
func TestSessionTabsFromView_HidesSingleSession(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	v := livesession.View{
		SessionStats: []livesession.SessionStat{
			{ID: "a", Label: "only", ContextPct: 30, LastActivity: now.Add(-5 * time.Second)},
		},
	}
	tabs := sessionTabsFromView(v, 0, now)
	if len(tabs) != 0 {
		t.Fatalf("expected empty tabs for single-session cwd, got %d tabs", len(tabs))
	}
}

// TestSessionTabsFromView_FiltersZombies verifies that sessions outside
// the activeSessionWindow (90s) are excluded so the strip drops closed
// claude code sessions quickly instead of leaving stale tabs around.
func TestSessionTabsFromView_FiltersZombies(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	v := livesession.View{
		SessionStats: []livesession.SessionStat{
			{ID: "fresh1", Label: "fresh1", ContextPct: 10, LastActivity: now.Add(-10 * time.Second)},
			{ID: "fresh2", Label: "fresh2", ContextPct: 20, LastActivity: now.Add(-60 * time.Second)},
			{ID: "zombie", Label: "zombie", ContextPct: 50, LastActivity: now.Add(-5 * time.Minute)},
		},
	}
	tabs := sessionTabsFromView(v, 0, now)
	// 2 fresh + Σ = 3 tabs.
	if len(tabs) != 3 {
		t.Fatalf("expected 3 tabs (Σ + 2 fresh), got %d: %+v", len(tabs), tabs)
	}
	for _, tab := range tabs {
		if tab.Label == "zombie" {
			t.Fatalf("zombie session should have been filtered, got %+v", tab)
		}
	}
}

// TestSessionTabsFromView_MarksActive verifies that the tab matching
// sessionTabIndex is flagged Active. Σ tab is index -1.
func TestSessionTabsFromView_MarksActive(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	v := livesession.View{
		SessionStats: []livesession.SessionStat{
			{ID: "a", Label: "a", ContextPct: 10, LastActivity: now.Add(-5 * time.Second)},
			{ID: "b", Label: "b", ContextPct: 20, LastActivity: now.Add(-15 * time.Second)},
		},
	}
	cases := []struct {
		idx       int
		wantLabel string
	}{
		{-1, "Σ all"},
		{0, "a"},
		{1, "b"},
	}
	for _, tc := range cases {
		tabs := sessionTabsFromView(v, tc.idx, now)
		var activeLabel string
		for _, tab := range tabs {
			if tab.Active {
				activeLabel = tab.Label
				break
			}
		}
		if activeLabel != tc.wantLabel {
			t.Errorf("idx=%d: active tab label = %q, want %q", tc.idx, activeLabel, tc.wantLabel)
		}
	}
}

// TestSessionTabsFromView_WarningFlag verifies the ⚠ flag is set when a
// session's ContextPct crosses sessionCtxWarnThreshold (80%) — and only
// for non-aggregate tabs.
func TestSessionTabsFromView_WarningFlag(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	v := livesession.View{
		SessionStats: []livesession.SessionStat{
			{ID: "danger", Label: "danger", ContextPct: 92, LastActivity: now.Add(-5 * time.Second)},
			{ID: "safe", Label: "safe", ContextPct: 30, LastActivity: now.Add(-15 * time.Second)},
		},
	}
	tabs := sessionTabsFromView(v, 0, now)
	for _, tab := range tabs {
		switch tab.Label {
		case "Σ all":
			if tab.Warning {
				t.Errorf("Σ tab must not carry a warning flag")
			}
		case "danger":
			if !tab.Warning {
				t.Errorf("danger session at 92%% must be flagged as warning")
			}
		case "safe":
			if tab.Warning {
				t.Errorf("safe session at 30%% must not be flagged")
			}
		}
	}
}

// TestSessionLeaderboardFromView_SortsByPctDesc verifies the leaderboard
// surfaces the most-loaded session at the top — the "is anything about
// to compact?" signal must always be at index 0.
func TestSessionLeaderboardFromView_SortsByPctDesc(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	v := livesession.View{
		SessionStats: []livesession.SessionStat{
			{ID: "low", Label: "low", ContextPct: 10, LastActivity: now.Add(-5 * time.Second)},
			{ID: "high", Label: "high", ContextPct: 88, LastActivity: now.Add(-15 * time.Second)},
			{ID: "mid", Label: "mid", ContextPct: 45, LastActivity: now.Add(-25 * time.Second)},
		},
	}
	leaders := sessionLeaderboardFromView(v, now)
	if len(leaders) != 3 {
		t.Fatalf("expected 3 leaderboard rows, got %d", len(leaders))
	}
	if leaders[0].Label != "high" {
		t.Errorf("top of leaderboard = %q, want %q", leaders[0].Label, "high")
	}
	if leaders[2].Label != "low" {
		t.Errorf("bottom of leaderboard = %q, want %q", leaders[2].Label, "low")
	}
}

// TestSessionLeaderboardFromView_CapsRows verifies the leaderboard caps
// at 3 entries so it doesn't push the rest of the panel off-screen.
func TestSessionLeaderboardFromView_CapsRows(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	stats := make([]livesession.SessionStat, 0, 6)
	for i := 0; i < 6; i++ {
		stats = append(stats, livesession.SessionStat{
			ID:           session.ID(fmt.Sprintf("s%d", i)),
			Label:        "s",
			ContextPct:   10 * (i + 1),
			LastActivity: now.Add(-time.Duration(i+1) * 5 * time.Second),
		})
	}
	v := livesession.View{SessionStats: stats}
	leaders := sessionLeaderboardFromView(v, now)
	if len(leaders) != 3 {
		t.Fatalf("expected leaderboard cap of 3, got %d", len(leaders))
	}
}

// TestResolveSessionFocus_AggregateUsesLatest verifies that the Σ tab
// (sessionTabIndex = -1) resolves to the most-recently-active session for
// the events / calls / diff panels — they still need a concrete session
// to render, the leaderboard handles the cross-cutting view in usage.
func TestResolveSessionFocus_AggregateUsesLatest(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	v := livesession.View{
		SessionStats: []livesession.SessionStat{
			{ID: "newest", Label: "newest", LastActivity: now.Add(-5 * time.Second)},
			{ID: "older", Label: "older", LastActivity: now.Add(-45 * time.Second)},
		},
	}
	got := string(resolveSessionFocus(v, -1, now))
	if got != "newest" {
		t.Errorf("Σ tab should resolve to most recent session; got %q, want %q", got, "newest")
	}
}

// TestResolveSessionFocus_OutOfRangeFallsBackToLatest verifies that if the
// user's saved sessionTabIndex no longer points at a live session, focus
// gracefully drops back to the most-recent session instead of crashing or
// showing an empty view.
func TestResolveSessionFocus_OutOfRangeFallsBackToLatest(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	v := livesession.View{
		SessionStats: []livesession.SessionStat{
			{ID: "newest", Label: "newest", LastActivity: now.Add(-5 * time.Second)},
		},
	}
	got := string(resolveSessionFocus(v, 17, now))
	if got != "newest" {
		t.Errorf("out-of-range index should fall back to newest; got %q", got)
	}
}
