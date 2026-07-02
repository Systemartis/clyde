package livesession_test

// bugfix_test.go — regression tests for confirmed bugs:
//   - buildTimelines subagent event-slice aliasing (stuck-active tool calls)
//   - usage-window reset anchored on first event, not last activity (mtime)
//   - single-project aggregation excludes focused session by identity, not position

import (
	"context"
	"testing"
	"time"

	"github.com/Systemartis/clyde/internal/application/livesession"
	"github.com/Systemartis/clyde/internal/domain/event"
	"github.com/Systemartis/clyde/internal/domain/session"
	usagePkg "github.com/Systemartis/clyde/internal/domain/usage"
	"github.com/Systemartis/clyde/internal/ports"
)

// mkAsstUsage builds an assistant event carrying a usage snapshot.
func mkAsstUsage(id string, ts time.Time, sid string, inputTok int64) event.Event {
	return event.NewEvent(id, ts, event.KindAssistant, sid, "", event.AssistantPayload{
		Usage: usagePkg.Usage{Input: inputTok},
	})
}

// ── Finding 1: subagent tool-call matching survives a visible event after the
// tool_result. The old code filtered the visible subset in place over the same
// backing array (agEvts[:0]) that was then passed to extractToolCalls, clobbering
// the tool_result and leaving completed calls stuck "active". ──────────────────
func TestSnapshot_SubagentTimeline_VisibleEventAfterToolResult(t *testing.T) {
	t.Parallel()

	mainSID := session.ID("parent-align")
	agentID := "agent-align01"
	base := mustTime("2026-04-30T10:00:00Z")
	toolUseID := "toolu_sub_align"
	durWant := 3 * time.Second

	src := &fakeSessionSource{
		sessions: []session.Summary{{ID: mainSID, LastActivity: base.Add(20 * time.Second)}},
		events: map[session.ID][]event.Event{
			mainSID: {mkUserEvt("u1", base, string(mainSID), event.UserPayload{Summary: "do work"})},
		},
	}

	subSrc := &fakeSubagentSource{
		infos: map[string][]ports.SubagentInfo{
			string(mainSID): {{AgentID: agentID, Description: "Sub worker"}},
		},
		events: map[string]map[string][]event.Event{
			string(mainSID): {
				agentID: {
					mkToolUseEvt("sa1", toolUseID, "Edit", "Tool: Edit /f.go", base, agentID),
					mkToolResultEvt("su1", base.Add(durWant), agentID, []string{toolUseID}, false),
					// A VISIBLE assistant event after the tool_result: this advances
					// the in-place filter write pointer past the tool_result's slot,
					// clobbering it in the shared backing array (the bug).
					mkAsstUsage("sa2", base.Add(5*time.Second), agentID, 0),
				},
			},
		},
	}

	clk := fakeClock{now: mustTime("2026-04-30T12:00:00Z")}
	ls := livesession.NewWithSubagents(src, clk, subSrc)

	view, err := ls.Snapshot(context.Background(), newProject("/align/project"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(view.Timelines) != 2 {
		t.Fatalf("Timelines len = %d, want 2", len(view.Timelines))
	}

	subTL := view.Timelines[1]
	if len(subTL.Calls) != 1 {
		t.Fatalf("subagent Calls len = %d, want 1", len(subTL.Calls))
	}
	tc := subTL.Calls[0]
	if tc.State != livesession.CallDone {
		t.Errorf("tool-call State = %v, want CallDone (completed call stuck active — aliasing bug)", tc.State)
	}
	if tc.Duration != durWant {
		t.Errorf("tool-call Duration = %v, want %v", tc.Duration, durWant)
	}
	if subTL.Active {
		t.Error("subTL.Active = true; a completed subagent call must not report active")
	}
}

// ── Finding 3 (single-project): the 5h/7d reset is anchored on the window's
// EARLIEST first event, not the session's last activity (file mtime). For a
// single continuously-active session this keeps the countdown decreasing. ──────
func TestSnapshot_ResetAnchoredOnFirstEvent_SingleProject(t *testing.T) {
	t.Parallel()

	t0 := mustTime("2026-04-30T08:00:00Z")
	sid := session.ID("active-sess")
	lastActivity := t0.Add(2 * time.Hour) // file mtime = last event
	now := lastActivity.Add(1 * time.Minute)

	src := &fakeSessionSource{
		sessions: []session.Summary{{ID: sid, LastActivity: lastActivity}},
		events: map[session.ID][]event.Event{
			sid: {
				mkAsstUsage("e1", t0, string(sid), 100),          // first event = window start
				mkAsstUsage("e2", lastActivity, string(sid), 50), // last event
			},
		},
	}
	ls := livesession.New(src, fakeClock{now: now}) // no global source → single-project path

	view, err := ls.Snapshot(context.Background(), newProject("/anchor/project"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want5h := t0.Add(5 * time.Hour)
	if !view.Reset5hAt.Equal(want5h) {
		t.Errorf("Reset5hAt = %v; want %v (first event + 5h, NOT mtime + 5h = %v)",
			view.Reset5hAt, want5h, lastActivity.Add(5*time.Hour))
	}
	wantWeek := t0.Add(7 * 24 * time.Hour)
	if !view.ResetWeekAt.Equal(wantWeek) {
		t.Errorf("ResetWeekAt = %v; want %v (first event + 7d)", view.ResetWeekAt, wantWeek)
	}
}

// The reset must not drift as the session keeps appending events, so the
// remaining countdown strictly decreases while the user works.
func TestSnapshot_Reset5h_MonotonicCountdown_SingleProject(t *testing.T) {
	t.Parallel()

	t0 := mustTime("2026-04-30T08:00:00Z")
	sid := session.ID("active-sess")

	snap := func(lastActivity time.Time, evts []event.Event) livesession.View {
		src := &fakeSessionSource{
			sessions: []session.Summary{{ID: sid, LastActivity: lastActivity}},
			events:   map[session.ID][]event.Event{sid: evts},
		}
		v, err := livesession.New(src, fakeClock{now: lastActivity.Add(time.Minute)}).
			Snapshot(context.Background(), newProject("/mono/project"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return v
	}

	la1 := t0.Add(1 * time.Hour)
	v1 := snap(la1, []event.Event{
		mkAsstUsage("e1", t0, string(sid), 10),
		mkAsstUsage("e2", la1, string(sid), 10),
	})

	// Same session, later: a new event appended, mtime advanced, first event fixed.
	la2 := t0.Add(2 * time.Hour)
	v2 := snap(la2, []event.Event{
		mkAsstUsage("e1", t0, string(sid), 10),
		mkAsstUsage("e2", la1, string(sid), 10),
		mkAsstUsage("e3", la2, string(sid), 10),
	})

	if !v1.Reset5hAt.Equal(v2.Reset5hAt) {
		t.Errorf("Reset5hAt drifted: v1=%v v2=%v (must stay anchored to the first event)",
			v1.Reset5hAt, v2.Reset5hAt)
	}
	rem1 := v1.Reset5hAt.Sub(la1.Add(time.Minute))
	rem2 := v2.Reset5hAt.Sub(la2.Add(time.Minute))
	if rem2 >= rem1 {
		t.Errorf("countdown not decreasing: remaining was %v, then %v", rem1, rem2)
	}
}

// ── Finding 3 (global): the reset uses the EARLIEST first event across in-window
// sessions from all projects, including non-focused ones read via the port. ─────
func TestSnapshot_Reset_EarliestFirstEventAcrossSessions_Global(t *testing.T) {
	t.Parallel()

	t0 := mustTime("2026-04-30T08:00:00Z")
	sidA := session.ID("focused-a") // focused, first event t0+1h
	sidB := session.ID("older-b")   // non-focused, first event t0 (earliest)

	tA0 := t0.Add(1 * time.Hour)
	tAlast := t0.Add(2 * time.Hour)
	tB0 := t0
	tBlast := t0.Add(90 * time.Minute)
	now := t0.Add(2*time.Hour + time.Minute)

	src := &fakeSessionSource{
		sessions: []session.Summary{{ID: sidA, LastActivity: tAlast}},
		events: map[session.ID][]event.Event{
			sidA: {mkAsstUsage("a0", tA0, string(sidA), 10), mkAsstUsage("a1", tAlast, string(sidA), 10)},
			sidB: {mkAsstUsage("b0", tB0, string(sidB), 10), mkAsstUsage("b1", tBlast, string(sidB), 10)},
		},
	}
	globalSrc := &fakeGlobalSessionSource{refs: []ports.GlobalSessionRef{
		{SessionID: sidA, LastActivity: tAlast},
		{SessionID: sidB, LastActivity: tBlast},
	}}
	ls := livesession.New(src, fakeClock{now: now}).WithGlobalSessions(globalSrc)

	view, err := ls.Snapshot(context.Background(), newProject("/global/anchor"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Earliest first event across in-window sessions is sidB's t0.
	want := t0.Add(5 * time.Hour)
	if !view.Reset5hAt.Equal(want) {
		t.Errorf("Reset5hAt = %v; want %v (earliest first event t0 + 5h)", view.Reset5hAt, want)
	}
}

// ── Finding 4: single-project aggregation must exclude the focused session by
// IDENTITY. Focusing an older tab previously attributed base.TotalUsage to the
// most-recent session (summaries[0]) and re-read the focused session, so the
// focused session was double-counted and the most-recent one dropped. ──────────
func TestSnapshot_SingleProject_FocusedOlderSession_NoDoubleCount(t *testing.T) {
	t.Parallel()

	now := mustTime("2026-04-30T12:00:00Z")
	recent := session.ID("recent")
	old := session.ID("old")
	tRecent := now.Add(-30 * time.Minute)
	tOld := now.Add(-2 * time.Hour)

	src := &fakeSessionSource{
		sessions: []session.Summary{
			{ID: recent, LastActivity: tRecent},
			{ID: old, LastActivity: tOld},
		},
		events: map[session.ID][]event.Event{
			recent: {mkAsstUsage("r1", tRecent, string(recent), 100)},
			old:    {mkAsstUsage("o1", tOld, string(old), 10)},
		},
	}
	ls := livesession.New(src, fakeClock{now: now}) // single-project path

	// Focus the OLDER session (not summaries[0]).
	view, err := ls.SnapshotForSession(context.Background(), newProject("/focus/project"), old)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if view.FocusedID != old {
		t.Fatalf("FocusedID = %v, want %v", view.FocusedID, old)
	}
	if view.Usage5h.Input != 110 {
		t.Errorf("Usage5h.Input = %d; want 110 (recent 100 + old 10, each counted once)", view.Usage5h.Input)
	}
	if view.UsageWeek.Input != 110 {
		t.Errorf("UsageWeek.Input = %d; want 110", view.UsageWeek.Input)
	}
	if view.Sessions5hCount != 2 {
		t.Errorf("Sessions5hCount = %d; want 2", view.Sessions5hCount)
	}
}
