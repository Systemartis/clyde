package livesession_test

import (
	"context"
	"testing"
	"time"

	"github.com/clyde-tui/clyde/internal/application/livesession"
	"github.com/clyde-tui/clyde/internal/domain/event"
	"github.com/clyde-tui/clyde/internal/domain/session"
	usagePkg "github.com/clyde-tui/clyde/internal/domain/usage"
)

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// TestSnapshot_PopulatesSessionStats verifies that every session in the
// current cwd gets a SessionStat entry with a label, latest usage, and a
// computed ContextPct. This is the data backing the title-bar tab strip
// and the usage-panel leaderboard.
func TestSnapshot_PopulatesSessionStats(t *testing.T) {
	t.Parallel()

	now := mustTime("2026-05-02T10:00:00Z")
	sidA := session.ID("sess-a")
	sidB := session.ID("sess-b")

	// Session A: latest usage is ~25% of 200k context.
	aLatest := usagePkg.Usage{Input: 30_000, CacheRead: 20_000, CacheCreation: 5_000}
	// Session B: latest usage near 90% of 200k context — should warn.
	bLatest := usagePkg.Usage{Input: 80_000, CacheRead: 90_000, CacheCreation: 12_000}

	src := &fakeSessionSource{
		sessions: []session.Summary{
			{ID: sidA, LastActivity: now.Add(-2 * time.Minute)},
			{ID: sidB, LastActivity: now.Add(-30 * time.Minute)},
		},
		events: map[session.ID][]event.Event{
			sidA: {
				mkUserEvt("a-u1", now.Add(-3*time.Minute), string(sidA), event.UserPayload{Summary: "fix the auth flow"}),
				event.NewEvent("a-a1", now.Add(-2*time.Minute), event.KindAssistant, string(sidA), "", event.AssistantPayload{
					Usage: aLatest,
					Model: "claude-opus-4-7",
				}),
			},
			sidB: {
				mkUserEvt("b-u1", now.Add(-31*time.Minute), string(sidB), event.UserPayload{Summary: "polish the docs section"}),
				event.NewEvent("b-a1", now.Add(-30*time.Minute), event.KindAssistant, string(sidB), "", event.AssistantPayload{
					Usage: bLatest,
					Model: "claude-opus-4-7",
				}),
			},
		},
	}
	ls := livesession.New(src, fakeClock{now: now})

	view, err := ls.Snapshot(context.Background(), newProject("/cwd"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(view.SessionStats) != 2 {
		t.Fatalf("SessionStats len = %d, want 2", len(view.SessionStats))
	}

	// Order matches LastActivity desc — A is more recent than B.
	if view.SessionStats[0].ID != sidA {
		t.Errorf("SessionStats[0].ID = %q, want %q", view.SessionStats[0].ID, sidA)
	}
	if view.SessionStats[1].ID != sidB {
		t.Errorf("SessionStats[1].ID = %q, want %q", view.SessionStats[1].ID, sidB)
	}

	// Labels come from the first user prompt; truncated to fit a tab.
	// "fix the auth flow" is 17 chars; the 16-char cap leaves room for an
	// ellipsis at the end.
	wantLabelPrefix := "fix the auth"
	if !startsWith(view.SessionStats[0].Label, wantLabelPrefix) {
		t.Errorf("SessionStats[0].Label = %q, want it to start with %q", view.SessionStats[0].Label, wantLabelPrefix)
	}
	if view.SessionStats[1].Label == "" {
		t.Errorf("SessionStats[1].Label must not be empty")
	}

	// Per-session ContextPct comes from each session's latest assistant
	// turn, NOT a shared running sum. Session A is around 25%; B around
	// 90%. We allow for rounding noise but not for cross-session bleed.
	if pct := view.SessionStats[0].ContextPct; pct < 20 || pct > 35 {
		t.Errorf("SessionStats[0].ContextPct = %d, want roughly 25 (sess A latest)", pct)
	}
	if pct := view.SessionStats[1].ContextPct; pct < 85 || pct > 95 {
		t.Errorf("SessionStats[1].ContextPct = %d, want roughly 90 (sess B latest)", pct)
	}
}

// TestSnapshotForSession_HonorsExplicitFocus verifies that passing a
// specific session ID makes that session the focused one even when it is
// NOT the most-recently-active. This is the path used by ] / [ session
// switching in the TUI.
func TestSnapshotForSession_HonorsExplicitFocus(t *testing.T) {
	t.Parallel()

	now := mustTime("2026-05-02T10:00:00Z")
	sidNew := session.ID("newest")
	sidOld := session.ID("older")

	src := &fakeSessionSource{
		sessions: []session.Summary{
			{ID: sidNew, LastActivity: now.Add(-1 * time.Minute)},
			{ID: sidOld, LastActivity: now.Add(-30 * time.Minute)},
		},
		events: map[session.ID][]event.Event{
			sidNew: {
				event.NewEvent("n-a1", now.Add(-1*time.Minute), event.KindAssistant, string(sidNew), "", event.AssistantPayload{
					Usage: usagePkg.Usage{Input: 1000},
					Model: "claude-opus-4-7",
				}),
			},
			sidOld: {
				event.NewEvent("o-a1", now.Add(-30*time.Minute), event.KindAssistant, string(sidOld), "", event.AssistantPayload{
					Usage: usagePkg.Usage{Input: 2000},
					Model: "claude-opus-4-7",
				}),
			},
		},
	}
	ls := livesession.New(src, fakeClock{now: now})

	// Default Snapshot picks the most recent.
	defaultView, err := ls.Snapshot(context.Background(), newProject("/cwd"))
	if err != nil {
		t.Fatalf("Snapshot error: %v", err)
	}
	if defaultView.FocusedID != sidNew {
		t.Errorf("default Snapshot focused = %q, want %q (most recent)", defaultView.FocusedID, sidNew)
	}

	// Explicit focus on the older session must override.
	overrideView, err := ls.SnapshotForSession(context.Background(), newProject("/cwd"), sidOld)
	if err != nil {
		t.Fatalf("SnapshotForSession error: %v", err)
	}
	if overrideView.FocusedID != sidOld {
		t.Errorf("explicit focus = %q, want %q", overrideView.FocusedID, sidOld)
	}
}

// TestSessionStats_SyntheticDoesNotInflateContextPct is the regression
// test for the "warning ⚠ appears on a low-context session" bug. Before
// the fix, sessionUsageFromEvents picked up the LAST assistant event's
// usage even when that event was a "<synthetic>" compaction summary
// with high token counts — falsely flagging the tab as near-compaction.
//
// New behavior: skip synthetic + zero-usage events when capturing
// `latest`, so ContextPct reflects only the most recent REAL turn.
func TestSessionStats_SyntheticDoesNotInflateContextPct(t *testing.T) {
	t.Parallel()

	now := mustTime("2026-05-02T10:00:00Z")
	sidLow := session.ID("low-ctx")

	// A real low-context turn (~5%) followed by a synthetic compaction
	// summary that reports a huge context — the kind of trailing event
	// claude code writes after a /compact.
	src := &fakeSessionSource{
		sessions: []session.Summary{{ID: sidLow, LastActivity: now.Add(-5 * time.Minute)}},
		events: map[session.ID][]event.Event{
			sidLow: {
				event.NewEvent("real", now.Add(-10*time.Minute), event.KindAssistant, string(sidLow), "", event.AssistantPayload{
					Usage: usagePkg.Usage{Input: 10_000, CacheRead: 0, CacheCreation: 0},
					Model: "claude-opus-4-7",
				}),
				// Synthetic event: high "context" but it's a compaction
				// summary — must NOT be treated as the user's current ctx.
				event.NewEvent("synthetic-compaction", now.Add(-9*time.Minute), event.KindAssistant, string(sidLow), "real", event.AssistantPayload{
					Usage: usagePkg.Usage{Input: 850_000, CacheRead: 50_000},
					Model: "<synthetic>",
				}),
			},
		},
	}
	ls := livesession.New(src, fakeClock{now: now})

	view, err := ls.SnapshotForSession(context.Background(), newProject("/cwd"), session.ID("other-focused"))
	if err != nil {
		t.Fatalf("snapshot error: %v", err)
	}
	// ^ Use a different focusedID so sidLow takes the non-focused branch
	// (sessionUsageFromEvents path — exactly what the bug touched).

	if len(view.SessionStats) == 0 {
		t.Fatalf("expected at least one SessionStat, got 0")
	}
	stat := view.SessionStats[0]
	if stat.ID != sidLow {
		t.Fatalf("unexpected SessionStat[0]: %+v", stat)
	}
	if stat.ContextPct >= 80 {
		t.Errorf("ContextPct = %d, expected < 80 (synthetic event must not inflate)", stat.ContextPct)
	}
}

// TestSessionStats_1MContextDetectedForNonFocused is the regression
// test for the "84% in Σ leaderboard, 16% in panel" bug. Real Claude
// JSONL stamps the model field as "claude-opus-4-7" — the 1M context
// tier is only detectable through Has1hCache=true on assistant events.
// applyUsageStats already synthesizes the "[1m]" model-id suffix; the
// non-focused path (sessionUsageFromEvents) used to skip that step,
// causing pricing.Lookup to return the 200k limit and inflating
// ContextPct by 5x for any session not currently focused.
//
// Without this test, the next refactor of sessionUsageFromEvents could
// silently regress the divisor for every Σ-leaderboard tab again.
func TestSessionStats_1MContextDetectedForNonFocused(t *testing.T) {
	t.Parallel()

	now := mustTime("2026-05-02T10:00:00Z")
	sid1M := session.ID("on-1m")
	sidFocused := session.ID("focused-elsewhere")

	src := &fakeSessionSource{
		sessions: []session.Summary{
			{ID: sidFocused, LastActivity: now.Add(-1 * time.Minute)},
			{ID: sid1M, LastActivity: now.Add(-2 * time.Minute)},
		},
		events: map[session.ID][]event.Event{
			sidFocused: {
				event.NewEvent("focused-a1", now.Add(-1*time.Minute), event.KindAssistant, string(sidFocused), "", event.AssistantPayload{
					Usage: usagePkg.Usage{Input: 1000},
					Model: "claude-opus-4-7",
				}),
			},
			// 1M-tier session: latest assistant event has Has1hCache=true
			// AND a context fill that's ~17% of 1M (= ~85% of 200k). The
			// bug shows up only in the non-focused path because that's
			// where the synthesized "[1m]" suffix used to be missing.
			sid1M: {
				event.NewEvent("real-a1", now.Add(-3*time.Minute), event.KindAssistant, string(sid1M), "", event.AssistantPayload{
					Usage: usagePkg.Usage{
						Input:         1,
						Output:        437,
						CacheRead:     167_709,
						CacheCreation: 895,
					},
					Model:      "claude-opus-4-7",
					Has1hCache: true,
				}),
			},
		},
	}
	ls := livesession.New(src, fakeClock{now: now})

	// Snapshot focused on the OTHER session — sid1M goes through the
	// sessionUsageFromEvents path.
	view, err := ls.SnapshotForSession(context.Background(), newProject("/cwd"), sidFocused)
	if err != nil {
		t.Fatalf("snapshot error: %v", err)
	}

	var stat livesession.SessionStat
	for _, s := range view.SessionStats {
		if s.ID == sid1M {
			stat = s
			break
		}
	}
	if stat.ID == "" {
		t.Fatalf("sid1M missing from SessionStats: %+v", view.SessionStats)
	}

	// 168605 / 1_000_000 ≈ 17%. If the [1m] suffix is dropped the math
	// becomes 168605 / 200_000 ≈ 84% and the warning ⚠ would appear.
	if stat.ContextPct >= 50 {
		t.Errorf("ContextPct = %d; want < 50 (1M tier should give ~17%%, got 200k-divisor result)", stat.ContextPct)
	}
}

// TestSessionStats_StaleClosedSessionMarkedNotLive verifies that a
// session whose last real event is older than the live-activity
// window reads as IsLive=false. The tab strip filters by recency on
// top of this flag, so this is the underlying "process is gone"
// signal — a 91-second-old session must NOT pretend to be live.
func TestSessionStats_StaleClosedSessionMarkedNotLive(t *testing.T) {
	t.Parallel()

	now := mustTime("2026-05-02T10:00:00Z")
	sidStale := session.ID("just-closed")

	src := &fakeSessionSource{
		sessions: []session.Summary{{ID: sidStale, LastActivity: now}},
		events: map[session.ID][]event.Event{
			sidStale: {
				event.NewEvent("a1", now.Add(-91*time.Second), event.KindAssistant, string(sidStale), "", event.AssistantPayload{
					Usage: usagePkg.Usage{Input: 100},
					Model: "claude-opus-4-7",
				}),
			},
		},
	}
	ls := livesession.New(src, fakeClock{now: now})

	view, err := ls.SnapshotForSession(context.Background(), newProject("/cwd"), session.ID("other"))
	if err != nil {
		t.Fatalf("snapshot error: %v", err)
	}
	if len(view.SessionStats) == 0 {
		t.Fatalf("expected SessionStats, got empty")
	}
	if view.SessionStats[0].IsLive {
		t.Errorf("session whose last real event is 91s old must be IsLive=false")
	}
}

// TestSessionStats_LastActivityPrefersRealEvents verifies that the
// "session is live" detection uses real event timestamps, not file mtime.
// /resume and similar no-op operations touch the JSONL file without
// producing real events; mtime alone would falsely flag a closed session
// as "live" and keep it in the title-bar tab strip.
func TestSessionStats_LastActivityPrefersRealEvents(t *testing.T) {
	t.Parallel()

	now := mustTime("2026-05-02T10:00:00Z")
	sidStale := session.ID("stale")

	// Summary mtime claims "now" (simulating a /resume touch), but the
	// last real event is 10h ago. The TUI's 1h activity window should
	// see this session as stale and exclude it from the tab strip.
	src := &fakeSessionSource{
		sessions: []session.Summary{{ID: sidStale, LastActivity: now}},
		events: map[session.ID][]event.Event{
			sidStale: {
				mkUserEvt("u1", now.Add(-10*time.Hour), string(sidStale), event.UserPayload{Summary: "long ago"}),
				event.NewEvent("a1", now.Add(-10*time.Hour).Add(time.Second), event.KindAssistant, string(sidStale), "u1", event.AssistantPayload{
					Usage: usagePkg.Usage{Input: 1000},
					Model: "claude-opus-4-7",
				}),
			},
		},
	}
	ls := livesession.New(src, fakeClock{now: now})

	view, err := ls.SnapshotForSession(context.Background(), newProject("/cwd"), session.ID("other"))
	if err != nil {
		t.Fatalf("snapshot error: %v", err)
	}
	if len(view.SessionStats) == 0 {
		t.Fatalf("expected SessionStats, got empty")
	}
	stat := view.SessionStats[0]
	age := now.Sub(stat.LastActivity)
	if age < 9*time.Hour {
		t.Errorf("LastActivity should reflect last real event (≈10h ago); got age=%v", age)
	}
	if stat.IsLive {
		t.Errorf("IsLive must be false when last real event is 10h old; got true")
	}
}

// TestSnapshotForSession_FallsBackOnUnknownID verifies that requesting a
// session ID that no longer exists in the cwd does not crash or render an
// empty view — it falls back to the most-recently-active session, which
// keeps the TUI usable when the user's saved focus has been deleted.
func TestSnapshotForSession_FallsBackOnUnknownID(t *testing.T) {
	t.Parallel()

	now := mustTime("2026-05-02T10:00:00Z")
	sidExisting := session.ID("alive")

	src := &fakeSessionSource{
		sessions: []session.Summary{
			{ID: sidExisting, LastActivity: now.Add(-1 * time.Minute)},
		},
		events: map[session.ID][]event.Event{
			sidExisting: {
				event.NewEvent("a-a1", now.Add(-1*time.Minute), event.KindAssistant, string(sidExisting), "", event.AssistantPayload{
					Usage: usagePkg.Usage{Input: 1000},
					Model: "claude-opus-4-7",
				}),
			},
		},
	}
	ls := livesession.New(src, fakeClock{now: now})

	view, err := ls.SnapshotForSession(context.Background(), newProject("/cwd"), session.ID("ghost"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if view.FocusedID != sidExisting {
		t.Errorf("expected fallback to %q, got %q", sidExisting, view.FocusedID)
	}
}
