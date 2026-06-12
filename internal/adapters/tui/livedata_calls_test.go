package tui

import (
	"testing"
)

// TestAgentColorIndex_Deterministic verifies the same agent ID always maps to
// the same color slot — a subagent's color must not flicker between snapshots.
func TestAgentColorIndex_Deterministic(t *testing.T) {
	t.Parallel()
	id := "agent-a09b7f25bb46ce5bc"
	first := agentColorIndex(id)
	for i := 0; i < 100; i++ {
		if got := agentColorIndex(id); got != first {
			t.Fatalf("agentColorIndex(%q) flickered: first=%d, iter %d=%d", id, first, i, got)
		}
	}
}

// TestAgentColorIndex_Bounded verifies the index is always in [0, agentColorCount).
func TestAgentColorIndex_Bounded(t *testing.T) {
	t.Parallel()
	cases := []string{
		"",
		"a",
		"agent-foo",
		"agent-bar-baz",
		"agent-a09b7f25bb46ce5bc",
		"unicode-âéîôû",
	}
	for _, id := range cases {
		idx := agentColorIndex(id)
		if idx < 0 || idx >= agentColorCount {
			t.Errorf("agentColorIndex(%q) = %d, want in [0, %d)", id, idx, agentColorCount)
		}
	}
}

// TestAgentColorIndex_DistinctsAcrossDifferentAgents verifies that several
// distinct subagent IDs produce at least three distinct color slots.
// This is a statistical sanity check on the hash distribution.
func TestAgentColorIndex_DistinctsAcrossDifferentAgents(t *testing.T) {
	t.Parallel()
	ids := []string{
		"agent-explore-content",
		"agent-propose-rendering",
		"agent-run-tests-batch",
		"agent-compaction-check",
		"agent-refactor-helper",
	}
	seen := map[int]bool{}
	for _, id := range ids {
		seen[agentColorIndex(id)] = true
	}
	if len(seen) < 3 {
		t.Errorf("expected ≥3 distinct color slots across %d ids, got %d", len(ids), len(seen))
	}
}

// TestToolHistogram_SortedByCountDesc verifies the histogram lists tools in
// descending count order, with ties broken by first-seen order.
func TestToolHistogram_SortedByCountDesc(t *testing.T) {
	t.Parallel()
	calls := []ToolCall{
		{Tool: "Read"},
		{Tool: "Read"},
		{Tool: "Edit"},
		{Tool: "Bash"},
		{Tool: "Read"},
		{Tool: "Edit"},
	}
	got := toolHistogram(calls)
	want := "Read 3 · Edit 2 · Bash 1"
	if got != want {
		t.Errorf("toolHistogram = %q, want %q", got, want)
	}
}

// TestToolHistogram_Empty verifies the empty-input case returns an empty string,
// so the renderer can branch on the result without checking length elsewhere.
func TestToolHistogram_Empty(t *testing.T) {
	t.Parallel()
	if got := toolHistogram(nil); got != "" {
		t.Errorf("toolHistogram(nil) = %q, want empty", got)
	}
	if got := toolHistogram([]ToolCall{}); got != "" {
		t.Errorf("toolHistogram([]) = %q, want empty", got)
	}
}

// TestMainAgentSummary_WithCalls verifies the one-liner shows the count plus
// the latest call's tool + key arg.
func TestMainAgentSummary_WithCalls(t *testing.T) {
	t.Parallel()
	grp := AgentGroup{
		Calls: []ToolCall{
			{Tool: "Read", KeyArg: "main.go", State: CallDone},
			{Tool: "Edit", KeyArg: "auth.ts", State: CallActive},
		},
	}
	got := mainAgentSummary(grp)
	want := "2 calls · last: Edit auth.ts"
	if got != want {
		t.Errorf("mainAgentSummary = %q, want %q", got, want)
	}
}

// TestMainAgentSummary_NoCalls verifies the zero-call case shows a sensible
// placeholder rather than an empty string.
func TestMainAgentSummary_NoCalls(t *testing.T) {
	t.Parallel()
	grp := AgentGroup{}
	got := mainAgentSummary(grp)
	want := "no calls yet"
	if got != want {
		t.Errorf("mainAgentSummary(empty) = %q, want %q", got, want)
	}
}

// TestMainAgentSummary_LatestNoArg verifies the latest-call line still works
// when the most recent call has an empty KeyArg.
func TestMainAgentSummary_LatestNoArg(t *testing.T) {
	t.Parallel()
	grp := AgentGroup{
		Calls: []ToolCall{
			{Tool: "Bash"},
		},
	}
	got := mainAgentSummary(grp)
	want := "1 call · last: Bash"
	if got != want {
		t.Errorf("mainAgentSummary = %q, want %q", got, want)
	}
}

// TestSubagentMeta_Running verifies the running state appears with its call count.
func TestSubagentMeta_Running(t *testing.T) {
	t.Parallel()
	grp := AgentGroup{
		Active: true,
		Calls: []ToolCall{
			{Tool: "Bash"}, {Tool: "Bash"},
		},
	}
	got := subagentMeta(grp)
	want := "running · 2 calls"
	if got != want {
		t.Errorf("subagentMeta = %q, want %q", got, want)
	}
}

// TestSubagentMeta_Done verifies an inactive subagent with calls reads as done.
func TestSubagentMeta_Done(t *testing.T) {
	t.Parallel()
	grp := AgentGroup{
		Active: false,
		Calls: []ToolCall{
			{Tool: "Read"}, {Tool: "Edit"}, {Tool: "Bash"},
		},
	}
	got := subagentMeta(grp)
	want := "done · 3 calls"
	if got != want {
		t.Errorf("subagentMeta = %q, want %q", got, want)
	}
}

// TestSubagentMeta_Idle verifies a subagent with zero calls reads as idle.
func TestSubagentMeta_Idle(t *testing.T) {
	t.Parallel()
	grp := AgentGroup{Active: false, Calls: []ToolCall{}}
	got := subagentMeta(grp)
	want := "idle · 0 calls"
	if got != want {
		t.Errorf("subagentMeta = %q, want %q", got, want)
	}
}

// TestSubagentMeta_SingularCall verifies the singular form for one call.
func TestSubagentMeta_SingularCall(t *testing.T) {
	t.Parallel()
	grp := AgentGroup{
		Active: false,
		Calls:  []ToolCall{{Tool: "Read"}},
	}
	got := subagentMeta(grp)
	want := "done · 1 call"
	if got != want {
		t.Errorf("subagentMeta = %q, want %q", got, want)
	}
}

// TestActiveSubagentCount_OnlySubagents verifies the helper counts active
// subagents and excludes the main agent.
func TestActiveSubagentCount_OnlySubagents(t *testing.T) {
	t.Parallel()
	groups := []AgentGroup{
		{IsSubagent: false, Active: true}, // main agent — excluded
		{IsSubagent: true, Active: true},  // counted
		{IsSubagent: true, Active: false}, // not active — excluded
		{IsSubagent: true, Active: true},  // counted
	}
	got := activeSubagentCount(groups)
	if got != 2 {
		t.Errorf("activeSubagentCount = %d, want 2", got)
	}
}

// TestActiveSubagentCount_None verifies the empty + nothing-active cases.
func TestActiveSubagentCount_None(t *testing.T) {
	t.Parallel()
	if got := activeSubagentCount(nil); got != 0 {
		t.Errorf("activeSubagentCount(nil) = %d, want 0", got)
	}
	groups := []AgentGroup{
		{IsSubagent: false, Active: true},
		{IsSubagent: true, Active: false},
	}
	if got := activeSubagentCount(groups); got != 0 {
		t.Errorf("activeSubagentCount(no active subs) = %d, want 0", got)
	}
}
