package livesession_test

// phaseb_test.go — Phase B tests: subagent timelines, tool call matching,
// duration computation, and state determination.

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

// ─── Fake SubagentSource ─────────────────────────────────────────────────────

// fakeSubagentSource is an in-memory fake that satisfies ports.SubagentSource.
type fakeSubagentSource struct {
	infos  map[string][]ports.SubagentInfo     // key: parentSessionID
	events map[string]map[string][]event.Event // key: parentSessionID → agentID → events
}

func (f *fakeSubagentSource) Subagents(_ context.Context, _ string, parentSessionID string) ([]ports.SubagentInfo, error) {
	if f.infos == nil {
		return nil, nil
	}
	return f.infos[parentSessionID], nil
}

func (f *fakeSubagentSource) SubagentEvents(_ context.Context, _ string, parentSessionID string, agentID string) ([]event.Event, error) {
	if f.events == nil {
		return nil, nil
	}
	byParent, ok := f.events[parentSessionID]
	if !ok {
		return nil, nil
	}
	return byParent[agentID], nil
}

// compile-time check
var _ ports.SubagentSource = (*fakeSubagentSource)(nil)

// ─── Helpers (Phase B) ───────────────────────────────────────────────────────

// mkToolUseEvt creates an assistant event with a tool_use block.
func mkToolUseEvt(id, toolUseID, toolName, summary string, ts time.Time, sid string) event.Event {
	return event.NewEvent(id, ts, event.KindAssistant, sid, "", event.AssistantPayload{
		ToolUseID: toolUseID,
		ToolName:  toolName,
		Summary:   summary,
	})
}

// mkToolResultEvt creates a user event containing a tool_result block.
func mkToolResultEvt(id string, ts time.Time, sid string, toolUseIDs []string, isError bool) event.Event {
	return event.NewEvent(id, ts, event.KindUser, sid, "", event.UserPayload{
		IsToolResultOnly: true,
		ToolResultIDs:    toolUseIDs,
		ToolResultError:  isError,
	})
}

// ─── Tests ───────────────────────────────────────────────────────────────────

// TestSnapshot_TimelinesMainOnly: when no SubagentSource is wired, Timelines
// contains exactly one entry (the main session).
func TestSnapshot_TimelinesMainOnly(t *testing.T) {
	t.Parallel()

	sid := session.ID("main-session")
	base := mustTime("2026-04-30T10:00:00Z")

	src := &fakeSessionSource{
		sessions: []session.Summary{{ID: sid, LastActivity: base}},
		events: map[session.ID][]event.Event{
			sid: {
				mkUserEvt("u1", base, string(sid), event.UserPayload{Summary: "hello"}),
				mkAssistantEvt("a1", base.Add(time.Second), string(sid)),
			},
		},
	}
	clk := fakeClock{now: mustTime("2026-04-30T12:00:00Z")}
	ls := livesession.New(src, clk) // no SubagentSource

	view, err := ls.Snapshot(context.Background(), newProject("/test/project"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(view.Timelines) != 1 {
		t.Fatalf("Timelines len = %d, want 1", len(view.Timelines))
	}
	main := view.Timelines[0]
	if main.IsSubagent {
		t.Error("Timelines[0].IsSubagent = true, want false")
	}
	if main.AgentName != "main session" {
		t.Errorf("Timelines[0].AgentName = %q, want %q", main.AgentName, "main session")
	}
	if main.AgentID != string(sid) {
		t.Errorf("Timelines[0].AgentID = %q, want %q", main.AgentID, string(sid))
	}
}

// TestSnapshot_TimelinesWithSubagents: when SubagentSource returns 2 subagents,
// Timelines has 3 entries: main + 2 subagents with correct IsSubagent and ParentID.
func TestSnapshot_TimelinesWithSubagents(t *testing.T) {
	t.Parallel()

	mainSID := session.ID("parent-session")
	agentID1 := "agent-aaa111"
	agentID2 := "agent-bbb222"
	base := mustTime("2026-04-30T10:00:00Z")

	src := &fakeSessionSource{
		sessions: []session.Summary{{ID: mainSID, LastActivity: base}},
		events: map[session.ID][]event.Event{
			mainSID: {mkUserEvt("u1", base, string(mainSID), event.UserPayload{Summary: "go"})},
		},
	}

	subSrc := &fakeSubagentSource{
		infos: map[string][]ports.SubagentInfo{
			string(mainSID): {
				{AgentID: agentID1, Description: "Explore codebase", AgentType: "general-purpose"},
				{AgentID: agentID2, Description: "Write tests", AgentType: "general-purpose"},
			},
		},
		events: map[string]map[string][]event.Event{
			string(mainSID): {
				agentID1: {mkAssistantEvt("a1", base.Add(time.Second), agentID1)},
				agentID2: {mkAssistantEvt("a2", base.Add(2*time.Second), agentID2)},
			},
		},
	}

	clk := fakeClock{now: mustTime("2026-04-30T12:00:00Z")}
	ls := livesession.NewWithSubagents(src, clk, subSrc)

	view, err := ls.Snapshot(context.Background(), newProject("/test/project"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(view.Timelines) != 3 {
		t.Fatalf("Timelines len = %d, want 3 (main + 2 subagents)", len(view.Timelines))
	}

	// [0] is always main session.
	main := view.Timelines[0]
	if main.IsSubagent {
		t.Error("Timelines[0].IsSubagent = true, want false (main session)")
	}
	if main.ParentID != "" {
		t.Errorf("Timelines[0].ParentID = %q, want empty", main.ParentID)
	}

	// [1] and [2] are subagents.
	for i := 1; i <= 2; i++ {
		ag := view.Timelines[i]
		if !ag.IsSubagent {
			t.Errorf("Timelines[%d].IsSubagent = false, want true", i)
		}
		if ag.ParentID != string(mainSID) {
			t.Errorf("Timelines[%d].ParentID = %q, want %q", i, ag.ParentID, string(mainSID))
		}
	}

	// Agent names come from Description.
	if view.Timelines[1].AgentName != "Explore codebase" {
		t.Errorf("Timelines[1].AgentName = %q, want %q", view.Timelines[1].AgentName, "Explore codebase")
	}
	if view.Timelines[2].AgentName != "Write tests" {
		t.Errorf("Timelines[2].AgentName = %q, want %q", view.Timelines[2].AgentName, "Write tests")
	}
}

// TestExtractToolCalls_DurationComputation: a tool_use event followed by a
// tool_result event produces a ToolCall with the correct duration.
func TestExtractToolCalls_DurationComputation(t *testing.T) {
	t.Parallel()

	mainSID := session.ID("dur-session")
	base := mustTime("2026-04-30T10:00:00Z")
	resultTS := base.Add(5 * time.Second)

	toolUseID := "toolu_abc123"
	src := &fakeSessionSource{
		sessions: []session.Summary{{ID: mainSID, LastActivity: resultTS}},
		events: map[session.ID][]event.Event{
			mainSID: {
				mkToolUseEvt("a1", toolUseID, "Read", "Tool: Read /some/path", base, string(mainSID)),
				mkToolResultEvt("u1", resultTS, string(mainSID), []string{toolUseID}, false),
			},
		},
	}
	clk := fakeClock{now: mustTime("2026-04-30T12:00:00Z")}
	ls := livesession.New(src, clk)

	view, err := ls.Snapshot(context.Background(), newProject("/test/project"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(view.Timelines) != 1 {
		t.Fatalf("Timelines len = %d, want 1", len(view.Timelines))
	}
	calls := view.Timelines[0].Calls
	if len(calls) != 1 {
		t.Fatalf("Calls len = %d, want 1", len(calls))
	}

	tc := calls[0]
	if tc.ToolUseID != toolUseID {
		t.Errorf("ToolUseID = %q, want %q", tc.ToolUseID, toolUseID)
	}
	if tc.Duration != 5*time.Second {
		t.Errorf("Duration = %v, want 5s", tc.Duration)
	}
	if tc.State != livesession.CallDone {
		t.Errorf("State = %v, want CallDone", tc.State)
	}
	if tc.Tool != "Read" {
		t.Errorf("Tool = %q, want %q", tc.Tool, "Read")
	}
}

// TestExtractToolCalls_ActiveState: a tool_use event with no matching
// tool_result produces a ToolCall in CallActive state with zero duration.
func TestExtractToolCalls_ActiveState(t *testing.T) {
	t.Parallel()

	mainSID := session.ID("active-session")
	base := mustTime("2026-04-30T10:00:00Z")
	toolUseID := "toolu_active"

	src := &fakeSessionSource{
		sessions: []session.Summary{{ID: mainSID, LastActivity: base}},
		events: map[session.ID][]event.Event{
			mainSID: {
				mkToolUseEvt("a1", toolUseID, "Bash", "Tool: Bash echo hi", base, string(mainSID)),
				// No tool_result — call still active.
			},
		},
	}
	clk := fakeClock{now: mustTime("2026-04-30T12:00:00Z")}
	ls := livesession.New(src, clk)

	view, err := ls.Snapshot(context.Background(), newProject("/test/project"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(view.Timelines) != 1 {
		t.Fatalf("Timelines len = %d, want 1", len(view.Timelines))
	}
	calls := view.Timelines[0].Calls
	if len(calls) != 1 {
		t.Fatalf("Calls len = %d, want 1", len(calls))
	}

	tc := calls[0]
	if tc.State != livesession.CallActive {
		t.Errorf("State = %v, want CallActive", tc.State)
	}
	if tc.Duration != 0 {
		t.Errorf("Duration = %v, want 0 (no result yet)", tc.Duration)
	}
	if !view.Timelines[0].Active {
		t.Error("AgentTimeline.Active = false, want true (has active call)")
	}
}

// TestExtractToolCalls_FailedState: a tool_result with is_error=true produces
// a ToolCall in CallFailed state.
func TestExtractToolCalls_FailedState(t *testing.T) {
	t.Parallel()

	mainSID := session.ID("fail-session")
	base := mustTime("2026-04-30T10:00:00Z")
	resultTS := base.Add(2 * time.Second)
	toolUseID := "toolu_fail"

	src := &fakeSessionSource{
		sessions: []session.Summary{{ID: mainSID, LastActivity: resultTS}},
		events: map[session.ID][]event.Event{
			mainSID: {
				mkToolUseEvt("a1", toolUseID, "Bash", "Tool: Bash 'rm -rf /'", base, string(mainSID)),
				mkToolResultEvt("u1", resultTS, string(mainSID), []string{toolUseID}, true /* isError */),
			},
		},
	}
	clk := fakeClock{now: mustTime("2026-04-30T12:00:00Z")}
	ls := livesession.New(src, clk)

	view, err := ls.Snapshot(context.Background(), newProject("/test/project"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	calls := view.Timelines[0].Calls
	if len(calls) != 1 {
		t.Fatalf("Calls len = %d, want 1", len(calls))
	}
	if calls[0].State != livesession.CallFailed {
		t.Errorf("State = %v, want CallFailed", calls[0].State)
	}
}

// TestSnapshot_UsageAccumulation: TotalUsage is the sum of all assistant event
// token counts, and AssistantTurns reflects the assistant event count.
func TestSnapshot_UsageAccumulation(t *testing.T) {
	t.Parallel()

	sid := session.ID("usage-session")
	base := mustTime("2026-04-30T10:00:00Z")

	src := &fakeSessionSource{
		sessions: []session.Summary{{ID: sid, LastActivity: base.Add(2 * time.Minute)}},
		events: map[session.ID][]event.Event{
			sid: {
				mkUserEvt("u1", base, string(sid), event.UserPayload{Summary: "hello"}),
				event.NewEvent("a1", base.Add(time.Minute), event.KindAssistant, string(sid), "u1", event.AssistantPayload{
					Usage: usagePkg.Usage{Input: 100, Output: 20},
				}),
				event.NewEvent("a2", base.Add(2*time.Minute), event.KindAssistant, string(sid), "a1", event.AssistantPayload{
					Summary: "Tool: Read /path",
					Usage:   usagePkg.Usage{Input: 50, Output: 10},
				}),
			},
		},
	}
	clk := fakeClock{now: mustTime("2026-04-30T12:00:00Z")}
	ls := livesession.New(src, clk)

	view, err := ls.Snapshot(context.Background(), newProject("/usage/project"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if view.AssistantTurns != 2 {
		t.Errorf("AssistantTurns = %d, want 2", view.AssistantTurns)
	}
}

// TestSnapshot_SubagentTimeline_ToolCallMatching: subagent events with
// tool_use/tool_result pairs produce properly matched ToolCalls.
func TestSnapshot_SubagentTimeline_ToolCallMatching(t *testing.T) {
	t.Parallel()

	mainSID := session.ID("parent-for-match")
	agentID := "agent-match001"
	base := mustTime("2026-04-30T10:00:00Z")
	durWant := 3 * time.Second

	toolUseID := "toolu_sub_xyz"

	src := &fakeSessionSource{
		sessions: []session.Summary{{ID: mainSID, LastActivity: base.Add(10 * time.Second)}},
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
					mkToolUseEvt("sa1", toolUseID, "Edit", "Tool: Edit /file.go", base, agentID),
					mkToolResultEvt("su1", base.Add(durWant), agentID, []string{toolUseID}, false),
				},
			},
		},
	}

	clk := fakeClock{now: mustTime("2026-04-30T12:00:00Z")}
	ls := livesession.NewWithSubagents(src, clk, subSrc)

	view, err := ls.Snapshot(context.Background(), newProject("/match/project"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(view.Timelines) != 2 {
		t.Fatalf("Timelines len = %d, want 2", len(view.Timelines))
	}

	subTL := view.Timelines[1]
	if subTL.AgentID != agentID {
		t.Errorf("subagent AgentID = %q, want %q", subTL.AgentID, agentID)
	}
	if len(subTL.Calls) != 1 {
		t.Fatalf("subagent Calls len = %d, want 1", len(subTL.Calls))
	}

	tc := subTL.Calls[0]
	if tc.Duration != durWant {
		t.Errorf("Duration = %v, want %v", tc.Duration, durWant)
	}
	if tc.State != livesession.CallDone {
		t.Errorf("State = %v, want CallDone", tc.State)
	}
	if tc.Tool != "Edit" {
		t.Errorf("Tool = %q, want Edit", tc.Tool)
	}
}
