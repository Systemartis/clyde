package tui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
)

// CallState describes the state of a tool call.
type CallState int

// Call state constants.
const (
	CallDone   CallState = iota // completed call
	CallActive                  // currently running call
	CallFailed                  // failed call
)

// ToolCall is a single tool invocation within an agent session.
type ToolCall struct {
	AgentID    string
	AgentName  string
	IsSubagent bool
	Tool       string    // "Read", "Edit", "Bash", "Grep", etc.
	KeyArg     string    // first/main argument (path, command, pattern)
	Duration   string    // "12s", "1.4m"
	State      CallState // CallDone, CallActive, CallFailed
}

// AgentGroup groups tool calls by the agent that made them.
type AgentGroup struct {
	AgentID    string
	AgentName  string
	IsSubagent bool
	Calls      []ToolCall
	Active     bool // whether this agent is currently running
}

// agentColors is the ordered palette slice used to identify subagents in
// the calls panel. Pink is reserved for the claude voice; Red is reserved
// for error duration coloring — both are excluded.
func agentColors(p Palette) []color.Color {
	return []color.Color{p.Purple, p.Cyan, p.Green, p.Amber, p.Orange}
}

// agentColorStyle returns a lipgloss style with the foreground set to the
// subagent's deterministic color slot. Stable across snapshots: same agent
// ID maps to the same color so users can track a subagent visually.
func agentColorStyle(p Palette, agentID string) lipgloss.Style {
	cs := agentColors(p)
	return lipgloss.NewStyle().Foreground(cs[agentColorIndex(agentID)])
}

// renderCallsExpanded renders the calls panel in expanded state.
// When activeMode is true the panel is in Expanded-Active state:
// content is shown through the viewport (enables real scrolling) and
// a pink double border + mode badge replace the normal chrome.
func renderCallsExpanded(s Styles, p Palette, d MockData, vp viewport.Model, width, height int, focused, activeMode bool) string {
	if activeMode {
		inner := width - 4
		vp.SetWidth(inner)
		vp.SetHeight(height - 2)
		content := vp.View()
		return wrapPanelActive(s, content, "activity", width, height)
	}
	return renderCalls(s, p, d, width, height, focused)
}

// renderCalls renders the calls panel — focused on subagent observability.
//
// The main agent is collapsed to a single summary line because its tool calls
// are already visible in the chat. Each subagent gets its own card with a
// per-agent color so multiple parallel subagents are visually distinct, and
// a tools histogram sub-line summarizes the work it has done.
func renderCalls(s Styles, p Palette, d MockData, width, height int, focused bool) string {
	inner := width - 4 // border + 1-char pad each side
	// Inner content height: total height minus the top + bottom border
	// rows. Used to size the Σ-mode per-session peek so it fills the
	// panel rather than leaving dead space.
	innerH := height - 2
	if innerH < 1 {
		innerH = 1
	}
	meta := callsMeta(d.AgentGroups)
	content := buildCallsContent(s, p, d, inner, innerH)
	return wrapPanel(s, content, "activity", meta, width, height, focused)
}

// buildCallsContent renders the body of the activity panel.
//
// Layout policy:
//   - When at least one subagent is present, main is collapsed to a one-line
//     summary (its tool calls are visible in the chat above the chatbox).
//     Each subagent gets its own colored card.
//   - When NO subagents are running, main itself is rendered as a full card
//     (header + calls + histogram) so the panel is meaningful in the common
//     live-mode case where claude is working solo.
//
// Diff hunks for the file claude is currently editing (d.DiffFile) appear
// inline indented under the matching Edit/Write/MultiEdit call, so the user
// sees WHO made WHICH change without leaving the activity view.
func buildCallsContent(s Styles, p Palette, d MockData, inner, innerH int) string {
	// Empty state — a fresh session has no agent activity yet. A faded
	// hint beats a featureless blank box (house style: bash "no Bash
	// commands recorded yet", cache "no turns observed yet").
	if len(d.AgentGroups) == 0 {
		return s.TextFade.Render("  no tool calls yet — activity appears as claude works")
	}

	var sb strings.Builder
	diffShown := false
	hasSubs := hasAnySubagent(d.AgentGroups)
	// Σ aggregate mode produces multiple non-subagent main groups (one
	// per session). When that happens, switch every main group to the
	// compact one-liner + a peek of the last few calls — fills the
	// otherwise-empty panel with useful per-session context. The peek
	// is suppressed when subagents are present (the subagent cards
	// already eat that vertical space) and in single-session views
	// (full solo-main card already lists every call).
	mainCount := countMainGroups(d.AgentGroups)
	multipleMains := mainCount > 1
	peekN := peekPerSession(innerH, mainCount)
	for i, grp := range d.AgentGroups {
		if i > 0 {
			sb.WriteByte('\n')
		}
		if !grp.IsSubagent {
			switch {
			case multipleMains:
				diffShown = renderMainAgentPeekBlock(&sb, s, grp, d, inner, diffShown, peekN)
			case hasSubs:
				diffShown = renderMainAgentBlock(&sb, s, grp, d, inner, diffShown)
			default:
				diffShown = renderSoloMainBlock(&sb, s, p, grp, d, inner, innerH, diffShown)
			}
			continue
		}
		diffShown = renderSubagentBlock(&sb, s, p, grp, d, inner, diffShown)
	}
	return strings.TrimRight(sb.String(), "\n")
}

// peekPerSession returns how many recent tool calls to show under each
// session row in Σ aggregate mode, sized to the panel's available
// vertical space. Each session reserves one row for its header line;
// the rest of the panel is divided evenly among the sessions, with a
// minimum of 1 call (so the peek is always present) and a soft cap of
// 8 (past which the rows just blur together).
//
// numSessions == 0 or 1 is the single-session path — caller doesn't
// invoke us in that case but we return 0 defensively.
func peekPerSession(innerH, numSessions int) int {
	if numSessions <= 1 {
		return 0
	}
	const minPeek, maxPeek = 1, 8
	available := innerH - numSessions // subtract header rows
	if available < numSessions {
		return minPeek
	}
	per := available / numSessions
	if per < minPeek {
		return minPeek
	}
	if per > maxPeek {
		return maxPeek
	}
	return per
}

// renderMainAgentPeekBlock writes the main-agent one-liner followed by
// the last N tool calls of that session, indented. Used in Σ aggregate
// mode where each session compresses to a row but the user still wants
// a glance at "what is this session doing right now".
//
// Returns the updated diffShown flag — if any of the visible calls
// matches d.DiffFile, the inline diff renders under it (same rule as
// the solo-main card).
func renderMainAgentPeekBlock(sb *strings.Builder, s Styles, grp AgentGroup, d MockData, inner int, diffShown bool, peekN int) bool {
	sb.WriteString(renderMainAgentLine(s, grp, inner))
	sb.WriteByte('\n')
	if peekN <= 0 || len(grp.Calls) == 0 {
		return diffShown
	}
	start := len(grp.Calls) - peekN
	if start < 0 {
		start = 0
	}
	// Newest-first peek: the latest call of the session shows directly under
	// its row, mirroring the full cards.
	for _, call := range reversedCalls(grp.Calls[start:]) {
		// Indent two spaces so the peek reads as subordinate to the
		// session row above. renderCallLine already pads its icon
		// column; the leading spaces stack cleanly.
		sb.WriteString("  ")
		sb.WriteString(renderCallLine(s, call, inner-2))
		sb.WriteByte('\n')
		if !diffShown && callMatchesDiff(call, d) {
			sb.WriteString(renderInlineDiff(s, d.DiffLines, inner))
			diffShown = true
		}
	}
	return diffShown
}

// countMainGroups returns how many non-subagent (top-level "main")
// groups the slice contains. Used to decide whether the activity panel
// is rendering Σ aggregate mode (multiple per-session timelines) and
// should compact each main group to a one-liner.
func countMainGroups(groups []AgentGroup) int {
	n := 0
	for _, g := range groups {
		if !g.IsSubagent {
			n++
		}
	}
	return n
}

// reversedCalls returns a newest-first copy of calls. The activity panel
// renders the latest activity at the TOP so a glance lands on what claude is
// doing right now; older calls sit below and scroll off the bottom. The
// source slice stays oldest-first (histograms, diff matching, and tail
// windowing all reason in chronological order), so we reverse only for
// display.
func reversedCalls(calls []ToolCall) []ToolCall {
	out := make([]ToolCall, len(calls))
	for i, c := range calls {
		out[len(calls)-1-i] = c
	}
	return out
}

// hasAnySubagent reports whether the groups slice contains at least one subagent.
func hasAnySubagent(groups []AgentGroup) bool {
	for _, g := range groups {
		if g.IsSubagent {
			return true
		}
	}
	return false
}

// renderSoloMainBlock renders the main agent as a full card when no subagents
// are present — header with state/count, full call list, inline diff under
// the matching Edit, and tools histogram footer. Reuses renderSubagentBlock's
// layout to keep the visual identical.
func renderSoloMainBlock(sb *strings.Builder, s Styles, p Palette, grp AgentGroup, d MockData, inner, innerH int, diffShown bool) bool {
	// Force the rendering to treat main as a styled card; agentColorIndex on
	// "main" gives a stable slot, distinct from any future subagent. Display
	// name stays "main" rather than the AgentID.
	displayed := grp
	displayed.AgentName = "main"
	displayed.IsSubagent = false // for color selection we still pass directly
	sb.WriteString(renderSoloMainHeader(s, p, displayed, inner))
	sb.WriteByte('\n')

	// Window the call list to the row budget, keeping the NEWEST calls. The
	// card renders newest-first (latest at the top) so a glance lands on
	// what claude is doing right now; older calls that don't fit are clipped
	// behind a "↓ earlier" marker at the bottom. Without windowing, wrapPanel
	// clips from the BOTTOM and the newest calls would fall off the fold
	// (active mode shows it all — buildCallsViewportContent passes an
	// unbounded innerH).
	calls := grp.Calls
	hist := toolHistogram(grp.Calls)
	budget := innerH - 1 // header row
	if hist != "" {
		budget-- // histogram footer row
	}
	if budget < 1 {
		budget = 1
	}
	hidden := 0
	if len(calls) > budget {
		visible := budget - 1 // "earlier" marker eats one row
		if visible < 0 {
			visible = 0
		}
		hidden = len(calls) - visible
		calls = calls[len(calls)-visible:]
	}
	for _, call := range reversedCalls(calls) {
		sb.WriteString(renderCallLine(s, call, inner))
		sb.WriteByte('\n')
		if !diffShown && callMatchesDiff(call, d) {
			sb.WriteString(renderInlineDiff(s, d.DiffLines, inner))
			diffShown = true
		}
	}
	if hidden > 0 {
		sb.WriteString(s.TaskDur.Render(fmt.Sprintf("    ↓ %d earlier", hidden)))
		sb.WriteByte('\n')
	}
	if hist != "" {
		sb.WriteString(s.TaskSubtitle.Render(truncate("    "+hist, inner)))
		sb.WriteByte('\n')
	}
	return diffShown
}

// renderSoloMainHeader renders the header line for the solo-main card. Uses
// the active-task style (purple by default) to keep main visually distinct
// from subagent cards even when they exist.
func renderSoloMainHeader(s Styles, _ Palette, grp AgentGroup, _ int) string {
	chevron := s.TaskSubActIcon.Render("▼")
	name := s.TaskActName.Render(grp.AgentName)
	meta := s.TaskDur.Render(" · " + subagentMeta(grp))
	return chevron + " " + name + meta
}

// renderMainAgentBlock writes the main-agent one-liner and an optional inline
// diff under it. Since the main agent is collapsed to a single line, any of
// its Edit/Write/MultiEdit calls matching d.DiffFile triggers the inline diff
// (we cannot anchor to a specific call when only the summary is shown).
// Returns the updated diffShown flag.
func renderMainAgentBlock(sb *strings.Builder, s Styles, grp AgentGroup, d MockData, inner int, diffShown bool) bool {
	sb.WriteString(renderMainAgentLine(s, grp, inner))
	sb.WriteByte('\n')
	if diffShown {
		return diffShown
	}
	for _, call := range grp.Calls {
		if callMatchesDiff(call, d) {
			sb.WriteString(renderInlineDiff(s, d.DiffLines, inner))
			return true
		}
	}
	return diffShown
}

// renderSubagentBlock writes a subagent header, its tool calls, optional
// inline diff under the first matching Edit, and the tools histogram footer.
// Returns the updated diffShown flag.
func renderSubagentBlock(sb *strings.Builder, s Styles, p Palette, grp AgentGroup, d MockData, inner int, diffShown bool) bool {
	sb.WriteString(renderSubagentHeader(s, p, grp, inner))
	sb.WriteByte('\n')
	// Newest-first, matching the solo-main card: the latest call sits under
	// the header and older calls fall below (clipped from the bottom when the
	// card overflows).
	for _, call := range reversedCalls(grp.Calls) {
		sb.WriteString(renderCallLine(s, call, inner))
		sb.WriteByte('\n')
		if !diffShown && callMatchesDiff(call, d) {
			sb.WriteString(renderInlineDiff(s, d.DiffLines, inner))
			diffShown = true
		}
	}
	if hist := toolHistogram(grp.Calls); hist != "" {
		sb.WriteString(s.TaskSubtitle.Render(truncate("    "+hist, inner)))
		sb.WriteByte('\n')
	}
	return diffShown
}

// callMatchesDiff reports whether a call edits the file currently in d.DiffFile.
// Matches by basename so absolute / relative path differences do not block.
func callMatchesDiff(call ToolCall, d MockData) bool {
	if len(d.DiffLines) == 0 || d.DiffFile == "" {
		return false
	}
	switch strings.ToLower(call.Tool) {
	case "edit", "multiedit", "write":
	default:
		return false
	}
	return basenameEqual(call.KeyArg, d.DiffFile)
}

// basenameEqual compares the basename of two path strings. The second argument
// may carry trailing diff stats (`"auth.ts · +28 −6"`); only the leading file
// portion is used for the match.
func basenameEqual(callPath, diffLabel string) bool {
	if callPath == "" || diffLabel == "" {
		return false
	}
	// Strip trailing stats from the diff label.
	if idx := strings.Index(diffLabel, " "); idx >= 0 {
		diffLabel = diffLabel[:idx]
	}
	clean := func(s string) string {
		s = strings.Trim(s, "'\" ")
		if idx := strings.LastIndexAny(s, "/\\"); idx >= 0 {
			s = s[idx+1:]
		}
		return s
	}
	return clean(callPath) == clean(diffLabel)
}

// renderInlineDiff renders diff hunks indented under the call line that
// produced them. The 6-space prefix aligns the body under the "  Edit foo.go"
// portion of the call line.
//
// Only the FIRST hunk is shown inline — the activity panel is meant for
// "what's happening", not a full diff browser. Users who want every hunk
// across every file can opt into the standalone diff panel via settings.
// A "+ N more hunks" trailer signals that more changes exist.
func renderInlineDiff(s Styles, lines []DiffLine, inner int) string {
	const prefix = "      "
	innerForDiff := inner - len(prefix)
	if innerForDiff < 8 {
		innerForDiff = 8
	}
	first, more := firstHunkOnly(lines)
	body := buildDiffLines(s, first, innerForDiff)
	if body == "" {
		return ""
	}
	var sb strings.Builder
	for _, line := range strings.Split(body, "\n") {
		sb.WriteString(prefix)
		sb.WriteString(line)
		sb.WriteByte('\n')
	}
	if more > 0 {
		noun := "hunks"
		if more == 1 {
			noun = "hunk"
		}
		sb.WriteString(prefix)
		sb.WriteString(s.TaskSubtitle.Render(fmt.Sprintf("+ %d more %s", more, noun)))
		sb.WriteByte('\n')
	}
	return sb.String()
}

// firstHunkOnly returns the lines belonging to the first hunk plus a count of
// additional hunks not included. Returns (lines, 0) when there is at most one
// hunk, or ([]nil, 0) when the input is empty.
func firstHunkOnly(lines []DiffLine) ([]DiffLine, int) {
	end := len(lines)
	hunks := 0
	for i, l := range lines {
		if l.Kind != DiffHunkKind {
			continue
		}
		hunks++
		if hunks == 2 {
			end = i
			break
		}
	}
	for _, l := range lines[end:] {
		if l.Kind == DiffHunkKind {
			hunks++
		}
	}
	more := 0
	if hunks > 1 {
		more = hunks - 1
	}
	return lines[:end], more
}

// callsMeta builds the panel header summary string.
//
// "active" counts AGENTS that are currently doing work (any unmatched tool
// call OR a recent event), not unmatched tool calls. Claude Code tool calls
// usually complete instantly, so the per-call active count would read as 0
// even while claude is busy — the per-agent count tracks actual liveness.
func callsMeta(groups []AgentGroup) string {
	if len(groups) == 0 {
		return "idle"
	}
	activeAgents, totalCalls := 0, 0
	for _, g := range groups {
		if g.Active {
			activeAgents++
		}
		totalCalls += len(g.Calls)
	}
	noun := "agents"
	if activeAgents == 1 {
		noun = "agent"
	}
	return fmt.Sprintf("%d %s active · %d calls", activeAgents, noun, totalCalls)
}

// renderMainAgentLine renders the main agent as a single summary line.
// The main agent's calls are visible in the chat above the chatbox, so
// clyde only confirms count + the latest call.
//
// Label: defaults to "main" — the canonical primary-session label —
// but honors a non-empty AgentName so the Σ aggregate view can label
// each session with its own short name (e.g. "fix bug panouri",
// "optimizari mo") rather than every row reading "main".
func renderMainAgentLine(s Styles, grp AgentGroup, inner int) string {
	chevron := s.TaskSubDoneIcon.Render("▾")
	label := "main"
	if grp.AgentName != "" && grp.AgentName != "main session" {
		label = grp.AgentName
	}
	name := s.TaskActName.Render(label)
	summary := s.TaskDur.Render(" · " + mainAgentSummary(grp))
	line := chevron + " " + name + summary
	if w := ansiWidth(line); w > inner {
		// Best-effort truncation by trimming the summary tail.
		over := w - inner
		summary = s.TaskDur.Render(" · " + truncate(mainAgentSummary(grp), len(mainAgentSummary(grp))-over-1))
		line = chevron + " " + name + summary
	}
	return line
}

// renderSubagentHeader renders one subagent card header line in the agent's
// deterministic color, with the meta state next to it.
func renderSubagentHeader(s Styles, p Palette, grp AgentGroup, inner int) string {
	style := agentColorStyle(p, grp.AgentID)
	chevron := style.Render("▼")
	name := style.Bold(true).Render(truncate(grp.AgentName, inner-20))
	meta := s.TaskDur.Render(" · " + subagentMeta(grp))
	return chevron + " " + name + meta
}

// renderCallLine renders one tool call row with icon, tool+arg, and duration.
func renderCallLine(s Styles, call ToolCall, inner int) string {
	// Icon + color based on state
	var icon string
	switch call.State {
	case CallDone:
		icon = s.TaskSubDoneIcon.Render("✓")
	case CallActive:
		icon = s.TaskSubActIcon.Render("▶")
	case CallFailed:
		icon = s.DiffRem.Render("✗")
	default:
		icon = s.TaskSubPenIcon.Render("○")
	}

	// Tool label + key arg
	toolStr := call.Tool
	if call.KeyArg != "" {
		toolStr += " " + call.KeyArg
	}
	maxToolW := inner - 12 // leave room for icon(1) + space(1) + dur(~8) + gaps
	if maxToolW < 10 {
		maxToolW = 10
	}

	var nameStyle lipglossStyle
	switch call.State {
	case CallActive:
		nameStyle = s.TaskSubActName
	case CallFailed:
		nameStyle = s.DiffRem
	default:
		nameStyle = s.TaskSubDoneName
	}

	name := nameStyle.Render(truncate("  "+toolStr, maxToolW))

	// Duration with color coding: <5s green, 5-15s amber, >15s red
	dur := callDurStyle(s, call).Render(call.Duration)

	line := icon + name
	lineW := ansiWidth(line)
	durW := ansiWidth(dur)
	gapW := inner - lineW - durW
	if gapW < 1 {
		gapW = 1
	}
	return line + strings.Repeat(" ", gapW) + dur
}

// lipglossStyle is a local alias to avoid import of lipgloss in switch.
// We reuse the existing Styles struct fields directly.
type lipglossStyle = interface {
	Render(strs ...string) string
}

// callDurStyle returns the appropriate duration style based on the duration string.
// <5s → green, 5-15s → amber, >15s → red, empty → dim.
func callDurStyle(s Styles, call ToolCall) interface{ Render(strs ...string) string } {
	if call.Duration == "" {
		return s.TaskDur
	}
	// Quick heuristic parse: if contains 'm' it's multi-minute → red
	if strings.Contains(call.Duration, "m") {
		return s.DiffRem
	}
	// Strip 's' suffix and parse digits
	digits := strings.TrimSuffix(call.Duration, "s")
	secs := 0
	_, _ = fmt.Sscanf(digits, "%d", &secs)
	switch {
	case secs < 5:
		return s.StatusGreen
	case secs < 15:
		return s.Amber
	default:
		return s.DiffRem
	}
}

// buildCallsViewportContent builds the full activity content for viewport scrolling.
// inner is the visible content width (panel width - 4 for borders + padding).
//
// Active mode (Expanded-Active) renders content into a scrollable
// viewport, so vertical room is effectively unbounded — pass a generous
// innerH so the per-session peek in Σ mode shows every call available.
// Capped at the soft maxPeek inside peekPerSession either way.
func buildCallsViewportContent(s Styles, p Palette, d MockData, inner int) string {
	const unboundedH = 1 << 16
	return buildCallsContent(s, p, d, inner, unboundedH)
}

// renderCallsCollapsed renders the collapsed one-liner for the calls panel.
// Surfaces active subagents in the meta so the user knows there's hidden work.
func renderCallsCollapsed(s Styles, _ Palette, d MockData, width int, focused bool) string {
	active, done := 0, 0
	for _, g := range d.AgentGroups {
		for _, c := range g.Calls {
			switch c.State {
			case CallActive:
				active++
			case CallDone:
				done++
			}
		}
	}
	subs := activeSubagentCount(d.AgentGroups)
	summary := fmt.Sprintf("%d done · %d active", done, active)
	if subs > 0 {
		noun := "subagents"
		if subs == 1 {
			noun = "subagent"
		}
		summary = fmt.Sprintf("%s · %d %s", summary, subs, noun)
	}
	return wrapPanelCollapsed(s, "activity", summary, "", width, focused)
}
