package tui

import (
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	"github.com/clyde-tui/clyde/internal/application/livesession"
	"github.com/clyde-tui/clyde/internal/domain/event"
)

// agentColorCount is the number of distinct color slots used to identify
// subagents in the calls panel. Picked to fit comfortably within the
// non-semantic palette colors (Pink is reserved for the claude voice; Red is
// reserved for error duration). Five distinct colors are plenty for the
// realistic case of ≤5 concurrent subagents.
const agentColorCount = 5

// agentColorIndex returns a deterministic 0..agentColorCount-1 slot for an
// agent ID. Same ID always produces the same slot, so a given subagent's
// color does not flicker across snapshots.
func agentColorIndex(agentID string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(agentID))
	return int(h.Sum32() % agentColorCount)
}

// toolHistogram produces a "Tool N · Tool N · …" sub-line summarizing the
// tool mix in a list of calls, sorted by count descending then by first
// occurrence. Returns "" when calls is empty so callers can branch on the
// truthiness of the result.
func toolHistogram(calls []ToolCall) string {
	if len(calls) == 0 {
		return ""
	}
	type bucket struct {
		tool  string
		count int
		first int
	}
	idx := map[string]int{}
	var buckets []bucket
	for i, c := range calls {
		if k, ok := idx[c.Tool]; ok {
			buckets[k].count++
			continue
		}
		idx[c.Tool] = len(buckets)
		buckets = append(buckets, bucket{tool: c.Tool, count: 1, first: i})
	}
	// Sort by count desc, then by first-seen asc.
	for i := 1; i < len(buckets); i++ {
		for j := i; j > 0; j-- {
			a, b := buckets[j-1], buckets[j]
			if a.count > b.count || (a.count == b.count && a.first <= b.first) {
				break
			}
			buckets[j-1], buckets[j] = b, a
		}
	}
	parts := make([]string, len(buckets))
	for i, b := range buckets {
		parts[i] = fmt.Sprintf("%s %d", b.tool, b.count)
	}
	return strings.Join(parts, " · ")
}

// mainAgentSummary returns the one-line summary shown for the main agent.
// The main agent's tool calls are visible in the chat itself, so clyde
// only needs to confirm the count and the latest call.
func mainAgentSummary(grp AgentGroup) string {
	if len(grp.Calls) == 0 {
		return "no calls yet"
	}
	last := grp.Calls[len(grp.Calls)-1]
	noun := "calls"
	if len(grp.Calls) == 1 {
		noun = "call"
	}
	tail := last.Tool
	if last.KeyArg != "" {
		tail += " " + last.KeyArg
	}
	return fmt.Sprintf("%d %s · last: %s", len(grp.Calls), noun, tail)
}

// subagentMeta returns the meta string shown next to a subagent's name in
// its card header, summarizing state and call count.
//
//	running · 4 calls
//	done    · 3 calls
//	idle    · 0 calls
func subagentMeta(grp AgentGroup) string {
	state := "done"
	switch {
	case grp.Active:
		state = "running"
	case len(grp.Calls) == 0:
		state = "idle"
	}
	noun := "calls"
	if len(grp.Calls) == 1 {
		noun = "call"
	}
	return fmt.Sprintf("%s · %d %s", state, len(grp.Calls), noun)
}

// activeSubagentCount returns the number of subagents currently running.
// Used by the title bar to show the "· N subagents" indicator only when
// there is at least one running subagent worth surfacing.
func activeSubagentCount(groups []AgentGroup) int {
	n := 0
	for _, g := range groups {
		if g.IsSubagent && g.Active {
			n++
		}
	}
	return n
}

// deriveAgentGroups builds the AgentGroups slice from a LiveSession View.
//
// Phase B: consumes View.Timelines directly when populated. Each AgentTimeline
// maps to one AgentGroup with proper tool_use ↔ tool_result matching
// (duration, state). Falls back to Phase A event-scanning when Timelines is
// empty (e.g. when the use case was constructed without SubagentSource).
func deriveAgentGroups(v livesession.View) []AgentGroup {
	if len(v.Timelines) > 0 {
		return timelinestoGroups(v.Timelines, v.LastUpdate)
	}
	return deriveAgentGroupsFromEvents(v)
}

// agentFreshThreshold is the window during which an agent counts as "running"
// for activity-display purposes even after every tool call has its result.
// Claude Code's tool calls usually complete in <1s, so the strict
// "any unmatched tool_use" definition of Active reads as "0 active" almost
// permanently. Treating recent activity as still active gives the user a
// truthful "is something happening?" signal.
const agentFreshThreshold = 10 * time.Second

// timelinestoGroups converts livesession.AgentTimeline entries to proto AgentGroups.
// Agents with activity in the last agentFreshThreshold are marked Active even
// when every individual call is already CallDone — see the const docstring.
func timelinestoGroups(timelines []livesession.AgentTimeline, snapshotAt time.Time) []AgentGroup {
	result := make([]AgentGroup, 0, len(timelines))
	for _, tl := range timelines {
		calls := make([]ToolCall, 0, len(tl.Calls))
		for _, tc := range tl.Calls {
			calls = append(calls, protoToolCall(tc))
		}
		active := tl.Active || agentIsFresh(tl, snapshotAt)
		result = append(result, AgentGroup{
			AgentID:    tl.AgentID,
			AgentName:  tl.AgentName,
			IsSubagent: tl.IsSubagent,
			Active:     active,
			Calls:      calls,
		})
	}
	return result
}

// agentIsFresh reports whether the agent had any event within
// agentFreshThreshold of snapshotAt. Used to keep solo main "running" while
// claude rapidly fires tool calls that complete instantly.
func agentIsFresh(tl livesession.AgentTimeline, snapshotAt time.Time) bool {
	if snapshotAt.IsZero() || len(tl.Events) == 0 {
		return false
	}
	last := tl.Events[len(tl.Events)-1].Timestamp
	if last.IsZero() {
		return false
	}
	return snapshotAt.Sub(last) < agentFreshThreshold
}

// protoToolCall maps a livesession.ToolCall to the proto layer's ToolCall.
func protoToolCall(tc livesession.ToolCall) ToolCall {
	return ToolCall{
		Tool:     tc.Tool,
		KeyArg:   tc.KeyArg,
		Duration: formatToolDuration(tc.Duration, tc.State),
		State:    mapCallState(tc.State),
	}
}

// mapCallState converts a livesession.CallState to the proto layer's CallState.
func mapCallState(s livesession.CallState) CallState {
	switch s {
	case livesession.CallActive:
		return CallActive
	case livesession.CallFailed:
		return CallFailed
	default:
		return CallDone
	}
}

// formatToolDuration formats a tool call duration for display.
// Active calls (no result yet) → empty string.
// <1s → "<1s", 1-60s → "Ns", >60s → "N.Nm".
func formatToolDuration(d time.Duration, state livesession.CallState) string {
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
	mins := secs / 60
	return fmt.Sprintf("%.1fm", mins)
}

// deriveAgentGroupsFromEvents is the Phase A fallback: scans events directly
// without tool_result matching. Used when View.Timelines is empty.
func deriveAgentGroupsFromEvents(v livesession.View) []AgentGroup {
	if len(v.Events) == 0 {
		return nil
	}

	focusedSID := string(v.FocusedID)

	type groupEntry struct {
		sid   string
		calls []ToolCall
	}
	var order []string
	groups := make(map[string]*groupEntry)

	for _, ev := range v.Events {
		if ev.Kind != event.KindAssistant {
			continue
		}
		ap, ok := ev.Payload.(event.AssistantPayload)
		if !ok || !strings.HasPrefix(ap.Summary, "Tool: ") {
			continue
		}

		sid := ev.SessionID
		if sid == "" {
			sid = focusedSID
		}

		if _, seen := groups[sid]; !seen {
			order = append(order, sid)
			groups[sid] = &groupEntry{sid: sid}
		}

		groups[sid].calls = append(groups[sid].calls, toolCallFromSummary(ap.Summary))
	}

	if len(order) == 0 {
		return nil
	}

	// Mark last call of last group as active (Phase A heuristic).
	lastGrp := groups[order[len(order)-1]]
	if len(lastGrp.calls) > 0 {
		last := &lastGrp.calls[len(lastGrp.calls)-1]
		if last.State == CallDone && last.Duration == "" {
			last.State = CallActive
		}
	}

	result := make([]AgentGroup, 0, len(order))
	for _, sid := range order {
		g := groups[sid]
		isSubagent := sid != focusedSID
		result = append(result, AgentGroup{
			AgentID:    sid,
			AgentName:  agentName(sid, focusedSID),
			IsSubagent: isSubagent,
			Active:     !isSubagent && len(g.calls) > 0,
			Calls:      g.calls,
		})
	}
	return result
}

// toolCallFromSummary builds a ToolCall from a pre-parsed tool summary string.
// Phase A fallback: no duration measured, state approximated.
func toolCallFromSummary(summary string) ToolCall {
	tool, keyArg := parseSummary(summary)
	return ToolCall{
		Tool:   tool,
		KeyArg: truncate(keyArg, 50),
		State:  CallDone,
	}
}

// parseSummary splits "Tool: Read /some/path" into ("Read", "/some/path").
// Falls back to (summary, "") if the format is unexpected.
func parseSummary(summary string) (tool, keyArg string) {
	rest := strings.TrimPrefix(summary, "Tool: ")
	parts := strings.SplitN(rest, " ", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	if len(parts) == 1 {
		return parts[0], ""
	}
	return summary, ""
}

// agentName returns a human-readable name for a session.
func agentName(sid, focusedSID string) string {
	if sid == focusedSID {
		return "main session"
	}
	if len(sid) > 8 {
		return sid[len(sid)-8:]
	}
	return sid
}
