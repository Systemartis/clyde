// Package livesession implements the LiveSession use case.
//
// LiveSession composes a SessionSource and a Clock to build a complete view of
// a project's sessions and the focused (most-recently-active) session's events.
// It is the primary data source for the clyde TUI (Phases A–J).
//
// The key difference from WatchSession:
//   - Returns ALL sessions as []session.Summary (for a session-list panel).
//   - Returns ALL visible events of the focused session (no top-N truncation).
//   - Builds AgentTimelines: main session + any subagents, each with their
//     tool calls matched to tool_results for duration and state.
//
// Event ordering: ascending by Timestamp (chronological), consistent with the
// append semantics of Claude Code JSONL files.
package livesession

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Systemartis/clyde/internal/domain/event"
	"github.com/Systemartis/clyde/internal/domain/pricing"
	"github.com/Systemartis/clyde/internal/domain/project"
	"github.com/Systemartis/clyde/internal/domain/session"
	"github.com/Systemartis/clyde/internal/domain/usage"
	"github.com/Systemartis/clyde/internal/ports"
)

// ToolCall is a single tool invocation extracted from an AgentTimeline.
// It carries the matched tool_use and tool_result data for display.
type ToolCall struct {
	// ToolUseID is the unique identifier from the tool_use content block.
	ToolUseID string

	// Tool is the tool name (e.g. "Read", "Bash", "Edit").
	Tool string

	// KeyArg is the primary argument extracted from the summary string
	// (the part after "Tool: Read " in a summary like "Tool: Read /some/path").
	KeyArg string

	// Duration is the time between tool_use and matching tool_result.
	// Zero if no tool_result has been found yet (call is active).
	Duration time.Duration

	// State is the current state of the tool call.
	State CallState

	// StartTime is the timestamp of the tool_use event.
	StartTime time.Time
}

// CallState describes the state of a tool call.
type CallState int

// Call state constants.
const (
	CallDone   CallState = iota // completed call
	CallActive                  // currently running (no matching tool_result yet)
	CallFailed                  // completed with an error in the tool_result
)

// AgentTimeline is the events and tool calls for a single agent (main session
// or a subagent).
type AgentTimeline struct {
	// AgentID is the session ID for the main agent, or the subagent's agent ID
	// (e.g. "agent-a09b7f25bb46ce5bc") for subagents.
	AgentID string

	// AgentName is the human-readable name. "main session" for the main session;
	// derived from meta.json description or a shortened AgentID for subagents.
	AgentName string

	// IsSubagent is true for subagent timelines.
	IsSubagent bool

	// ParentID is the parent session ID. Empty for the main session.
	ParentID string

	// Events holds all visible events for this agent in ascending chronological
	// order.
	Events []event.Event

	// Calls is the list of tool calls extracted from Events, with each call
	// matched to its tool_result where possible.
	Calls []ToolCall

	// Active is true when the agent is currently running (has at least one
	// unmatched tool_use, or the last event is an open assistant turn).
	Active bool
}

// View is the output of LiveSession.Snapshot.
// It is a plain data value safe to pass across layer boundaries.
type View struct {
	// Project is the project this view was built for.
	Project project.Project

	// Sessions is all sessions in this project, sorted by LastActivity
	// descending (most recently active first).
	Sessions []session.Summary

	// FocusedID is the ID of the most-recently-active session.
	// Zero value (empty string) means no sessions exist.
	FocusedID session.ID

	// Events holds ALL visible events of the focused session in strictly
	// ascending chronological order (oldest first). May be empty.
	//
	// "Visible" means the isVisible predicate: excludes KindOpaque events,
	// meta user events, and tool-result-only user events (ADR-007).
	Events []event.Event

	// Timelines is the list of agent timelines for the focused session.
	// [0] is always the main session (if any events exist); subsequent entries
	// are subagents in discovery order.
	Timelines []AgentTimeline

	// TotalUsage is the sum of all assistant event usage across the focused session.
	// Used by the usage panel for cost computation.
	// NOTE: do NOT use TotalUsage.CacheRead for display totals — it counts the
	// same cached content once per turn, inflating the sum massively.
	TotalUsage usage.Usage

	// LatestUsage is the usage snapshot from the most recent assistant event.
	// Use this (not TotalUsage) for compaction percentage calculation because it
	// represents the actual context size of the last API call, not a running sum.
	LatestUsage usage.Usage

	// CurrentModel is the model ID from the most recent assistant event in the
	// focused session, e.g. "claude-opus-4-7". Empty when no assistant events exist.
	CurrentModel string

	// AssistantTurns is the count of KindAssistant events in the focused session.
	// Used by the usage panel to display the "turns" metric.
	AssistantTurns int

	// LastUpdate is the UTC instant captured from Clock at the time Snapshot
	// was called.
	LastUpdate time.Time

	// EmptyReason is a human-readable explanation for why Events is empty.
	// Set when there are no sessions or the focused session has no events.
	// Empty string when Events is non-empty.
	EmptyReason string

	// MCPs is the list of configured MCP servers, populated from the Claude
	// Code settings file via the mcpconfig adapter.
	// Populated at construction time (before Snapshot); empty when no adapter
	// is provided or the settings file is unreadable.
	MCPs []MCP

	// BashLog is the chronological list of Bash tool calls across all
	// agents (main + subagents) in the focused window. Each entry carries
	// the command (KeyArg), state, start time, and duration. Populated by
	// every Snapshot call; nil when no Bash calls have been observed.
	BashLog []ToolCall

	// CacheStats summarizes prompt-cache efficiency across the focused
	// session: hit ratio, totals, biggest cache miss, and a per-turn ratio
	// trend. Powers the cache-efficiency panel.
	CacheStats CacheStats

	// CwdCacheStats is the same metric merged across every session in the
	// current cwd (within applySessionStats's iteration). The TUI shows
	// this when the user is on the Σ aggregate tab — single-session cache
	// stats would be misleading there.
	CwdCacheStats CacheStats

	// SessionStats lists every session in the current project (cwd), sorted
	// by LastActivity desc. Carries per-session context fill, label, and
	// activity flags so the title-bar tab strip and the usage-panel
	// leaderboard can render without further I/O.
	SessionStats []SessionStat

	// LSPs is the list of detected language servers.
	// Always empty in V1 — LSP detection is deferred to V2.
	LSPs []LSP

	// FileTree is the directory tree for the project's cwd, built by the
	// fsexplorer adapter.  Nil when the adapter is not wired or on error.
	FileTree *FileNode

	// ModifiedFiles is the list of files that git reports as changed in the
	// project's cwd, built by the git adapter.  Nil on error or no adapter.
	ModifiedFiles []GitFileStatus

	// DiffFile is the path of the file Claude is currently editing, extracted
	// from the most recent Edit/Write/MultiEdit tool_use event in the focused
	// session.  Empty string when no such event exists.
	DiffFile string

	// DiffHunks holds the parsed git diff hunks for DiffFile (or all changed
	// files when DiffFile is empty).  Populated by applyLiveView in the TUI
	// layer via git.Source.Diff.  Nil when running in demo mode or on error.
	DiffHunks []DiffHunk

	// Usage5h is the sum of assistant event usage across ALL Claude Code projects
	// whose sessions had LastActivity within the last 5 hours.
	// Zero value when no sessions qualify or source is unavailable.
	Usage5h usage.Usage

	// UsageWeek is the sum of assistant event usage across ALL Claude Code projects
	// whose sessions had LastActivity within the last 7 days.
	// Zero value when no sessions qualify or source is unavailable.
	UsageWeek usage.Usage

	// Sessions5hCount is the number of sessions (across all projects) that
	// contributed to Usage5h. Zero when Usage5h is empty.
	Sessions5hCount int

	// SessionsWeekCount is the number of sessions (across all projects) that
	// contributed to UsageWeek. Zero when UsageWeek is empty.
	SessionsWeekCount int

	// Reset5hAt is the time when the rolling 5-hour window resets.
	// Computed as: earliest event in the 5h window + 5h.
	// Zero when no sessions exist in the 5h window.
	Reset5hAt time.Time

	// ResetWeekAt is the time when the rolling 7-day window resets.
	// Computed as: earliest event in the 7d window + 7d.
	// Zero when no sessions exist in the 7d window.
	ResetWeekAt time.Time

	// LLMSource is the name of the LLM CLI source driving this view
	// (e.g. "claude-code", "gemini-cli"). Set by the composition root.
	LLMSource string
}

// DiffHunk is a single changed region in a unified diff.
// It mirrors git.Hunk without importing the adapters/git package from the
// application layer (hexagonal discipline: application layer must not depend
// on a concrete adapter).
type DiffHunk struct {
	// Header is the raw @@ line.
	Header string

	OldStart int
	OldCount int
	NewStart int
	NewCount int

	// Lines are the individual diff lines (context, add, remove).
	Lines []DiffHunkLine
}

// DiffHunkLine is a single line within a DiffHunk.
type DiffHunkLine struct {
	// Type is ' ' for context, '+' for added, '-' for removed.
	Type rune
	// Text is the line content without the leading +/- marker.
	Text string
}

// MCP represents a single MCP server entry in the servers panel view.
type MCP struct {
	// Name is the plugin identifier, e.g. "engram@engram".
	Name string

	// Enabled is true when the plugin is enabled in the settings file.
	Enabled bool

	// ToolCount is the number of tools exposed by this server.
	// 0 when live ping is unavailable (V1).
	ToolCount int
}

// SessionStat is a per-session summary computed at Snapshot time. It carries
// everything the TUI needs to render the session-tab strip and the usage-panel
// leaderboard without extra I/O.
//
// One SessionStat per session JSONL in the current project's cwd, ordered by
// LastActivity desc.
type SessionStat struct {
	// ID is the session identifier (matches the JSONL filename).
	ID session.ID

	// Label is a short human-readable hint for the tab — typically a
	// truncated first user prompt. Falls back to ID prefix when no
	// prompt was extracted.
	Label string

	// LastActivity mirrors session.Summary.LastActivity. Used by the TUI
	// to filter zombies (older than now-1h) out of the leaderboard.
	LastActivity time.Time

	// LatestUsage is the usage from the most recent assistant event in this
	// session. Drives ContextPct.
	LatestUsage usage.Usage

	// ContextPct is the model-context fill percentage in [0, 100],
	// computed via pricing.CompactionPercent against the session's model.
	// This is the actionable "is this session about to compact?" signal.
	ContextPct int

	// SessionTokens is the total tokens used by the session
	// (pricing.TotalTokens of the running sum). Powers the Σ aggregate
	// view for the current cwd.
	SessionTokens int64

	// Active is true when the focused session has an open assistant turn
	// (running tool call). Only set for the focused session — derived from
	// timeline state, which is populated from the focused session's events.
	Active bool

	// IsLive is true when the session was written to recently — its JSONL
	// was modified within the live-activity window of the snapshot's clock.
	// Recency is a reliable signal that claude code itself still has the
	// session open, regardless of which session the user has focused in
	// clyde. Closed/idle sessions read as IsLive=false.
	IsLive bool
}

// CacheStats summarizes prompt-cache efficiency over the focused session.
//
// The hit ratio is the share of tokens served from the prompt cache, vs
// tokens re-charged as fresh input or new cache writes:
//
//	HitRatio = CacheRead / (CacheRead + Input + CacheCreation)
//
// Tracking this matters because cache hits are MUCH cheaper / faster than
// recomputed input. A low ratio on a long session usually signals prompt
// instability that breaks caching.
type CacheStats struct {
	// HitRatio is the aggregate share in [0, 1] across all observed turns.
	HitRatio float64

	// FromCache is the total tokens served from cache across the session.
	FromCache int64

	// Recomputed is total Input + CacheCreation tokens (everything not
	// served from cache).
	Recomputed int64

	// BiggestMissTokens is the largest single-turn (Input + CacheCreation)
	// figure seen this session — the worst cache-bust moment.
	BiggestMissTokens int64

	// BiggestMissAt is the timestamp of the worst cache-bust turn.
	BiggestMissAt time.Time

	// Trend is the per-turn hit-ratio history (oldest → newest), capped at
	// the last 10 turns. Powers the sparkline.
	Trend []float64

	// TurnCount is the number of assistant turns observed (Trend may be
	// shorter when the cap kicks in).
	TurnCount int
}

// LSP represents a single language server in the servers panel view.
type LSP struct {
	// Name is the language server identifier, e.g. "gopls", "tsserver".
	Name string

	// Active is true when the binary is installed and on PATH.
	Active bool

	// ClaudeEnabled is true when Claude Code's enabledPlugins map references
	// this LSP (i.e. claude is actually using it). Combined with Active, the
	// TUI distinguishes installed-and-wired (green) from installed-but-not-
	// wired (amber) so the user can see when claude code missed an LSP they
	// have available.
	ClaudeEnabled bool
}

// FileNode is a single node in the filesystem tree returned by the
// fsexplorer adapter.  Mirrors fsexplorer.Node without importing the adapter
// package (keep the application layer free of concrete adapter dependencies).
type FileNode struct {
	Name     string
	IsDir    bool
	Path     string
	Children []*FileNode
}

// GitFileStatus describes a single file entry from `git status --porcelain`.
// Mirrors git.FileStatus without importing the adapter package.
type GitFileStatus struct {
	Path   string
	Status rune // 'M', 'A', 'D', 'R', '?', …
	Staged bool
}

// FileTreeSource is the port for filesystem tree walking.
// Adapters that implement this live in internal/adapters/fsexplorer.
type FileTreeSource interface {
	WalkToView(cwd string) (*FileNode, error)
}

// GitSource is the port for git status.
// Adapters that implement this live in internal/adapters/git.
type GitSource interface {
	StatusView(cwd string) ([]GitFileStatus, error)
}

// LiveSession is the use case that provides a full live view of a project's
// most-recently-active session. Construct via New or NewWithSubagents.
type LiveSession struct {
	src       ports.SessionSource
	clk       ports.Clock
	subSrc    ports.SubagentSource      // optional; nil means no subagent loading
	globalSrc ports.GlobalSessionSource // optional; nil means single-project aggregation
	gitSrc    GitSource                 // optional; nil means no git status
	fsSrc     FileTreeSource            // optional; nil means no file tree
	mcpSrc    ports.MCPSource           // optional; nil means no MCP configuration
	lspSrc    ports.LSPSource           // optional; nil means no LSP detection
	procSrc   ports.ProcessSource       // optional; nil falls back to mtime-only liveness

	// usage is the per-session usage memoisation. Held behind a pointer
	// so With* builder methods (which copy LiveSession by value) all
	// share the same cache. See sessionUsageCache.
	//
	// Memoisation key is the session's LastActivity stamp — when it
	// matches the last observation, the JSONL hasn't been appended to
	// and the cached sum is still valid. Cuts JSONL re-parses across
	// up to 50 sessions × every snapshot tick down to "once per actual
	// activity event per session".
	usage *sessionUsageCache
}

// sessionUsageCache is the shared memoisation table for
// applyMultiWindowUsageGlobal. Lives behind a pointer in LiveSession so
// builder copies share state.
type sessionUsageCache struct {
	mu      sync.Mutex
	entries map[session.ID]sessionUsageCacheEntry
}

// sessionUsageCacheEntry is the per-session memoised usage.Usage (plus the
// session's first-event time, used to anchor the window reset) along with the
// LastActivity stamp at fetch time. A change in LastActivity invalidates the
// entry — the JSONL has new events.
type sessionUsageCacheEntry struct {
	usage    usage.Usage
	activity time.Time
	// firstEvent is the session's first event timestamp — the reset-window
	// anchor. Cached alongside usage so window anchoring never forces a
	// JSONL re-read.
	firstEvent time.Time
}

// newUsageCache returns an initialized cache table.
func newUsageCache() *sessionUsageCache {
	return &sessionUsageCache{entries: make(map[session.ID]sessionUsageCacheEntry)}
}

// New constructs a LiveSession with the given SessionSource and Clock.
// Subagent reading is disabled — use NewWithSubagents to enable it.
func New(src ports.SessionSource, clk ports.Clock) *LiveSession {
	return &LiveSession{src: src, clk: clk, usage: newUsageCache()}
}

// NewWithSubagents constructs a LiveSession with subagent support enabled.
// The subSrc is used to discover and read subagent JSONL files.
func NewWithSubagents(src ports.SessionSource, clk ports.Clock, subSrc ports.SubagentSource) *LiveSession {
	return &LiveSession{src: src, clk: clk, subSrc: subSrc, usage: newUsageCache()}
}

// WithGlobalSessions returns a new LiveSession that also aggregates usage across
// ALL Claude Code projects (not just the current project) for the 5h and 7d
// time windows. Pass nil to clear a previously set source.
func (l *LiveSession) WithGlobalSessions(globalSrc ports.GlobalSessionSource) *LiveSession {
	cp := *l
	cp.globalSrc = globalSrc
	return &cp
}

// WithExplorer returns a new LiveSession that also populates FileTree and
// ModifiedFiles on every Snapshot call.  gitSrc or fsSrc may be nil to skip
// that half individually; both nil is a no-op equivalent to not calling this.
func (l *LiveSession) WithExplorer(gitSrc GitSource, fsSrc FileTreeSource) *LiveSession {
	cp := *l
	cp.gitSrc = gitSrc
	cp.fsSrc = fsSrc
	return &cp
}

// WithMCPs returns a new LiveSession that also populates View.MCPs on every
// Snapshot call from the given MCPSource.
// Pass nil to clear a previously set source.
func (l *LiveSession) WithMCPs(mcpSrc ports.MCPSource) *LiveSession {
	cp := *l
	cp.mcpSrc = mcpSrc
	return &cp
}

// WithLSPs returns a new LiveSession that populates View.LSPs on every
// Snapshot call from the given LSPSource. Pass nil to clear.
func (l *LiveSession) WithLSPs(lspSrc ports.LSPSource) *LiveSession {
	cp := *l
	cp.lspSrc = lspSrc
	return &cp
}

// WithProcesses returns a new LiveSession that consults the given
// ProcessSource on every Snapshot to mark sessions as live when their
// `claude` process is running, even if the JSONL is mtime-stale. The
// per-cwd session set is unchanged — process info enriches IsLive but
// never adds sessions from outside the current project. Pass nil to
// clear (falls back to mtime-only liveness, the v0.6 behavior).
func (l *LiveSession) WithProcesses(procSrc ports.ProcessSource) *LiveSession {
	cp := *l
	cp.procSrc = procSrc
	return &cp
}

// Snapshot executes the use case for the given project, focusing the
// most-recently-active session. Equivalent to SnapshotForSession with an
// empty session ID.
func (l *LiveSession) Snapshot(ctx context.Context, p project.Project) (View, error) {
	return l.SnapshotForSession(ctx, p, "")
}

// SnapshotForSession executes the use case for the given project, focusing
// the session whose ID matches focusedID. Pass an empty focusedID to focus
// the most-recently-active session (default behavior).
//
// It discovers all sessions, retrieves the focused session's events, builds
// tool-call timelines (including subagents), and returns a View.
//
// When focusedID is non-empty but no session matches, falls back to the
// most-recently-active session — this keeps the TUI usable when the user's
// last-focused session has since been deleted or renamed.
//
// Returns an empty view (no error) when no sessions exist for the project.
// Returns an empty view (no error) when the project directory is missing.
func (l *LiveSession) SnapshotForSession(ctx context.Context, p project.Project, focusedID session.ID) (View, error) {
	now := l.clk.Now()

	base := View{Project: p, LastUpdate: now}

	summaries, err := l.src.Sessions(ctx, p.CWD())
	if err != nil {
		return base, err
	}
	if len(summaries) == 0 {
		base.EmptyReason = "no sessions"
		return base, nil
	}

	// Sort descending by LastActivity (most recently active first).
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].LastActivity.After(summaries[j].LastActivity)
	})

	base.Sessions = summaries
	base.FocusedID = pickFocusedID(summaries, focusedID)

	allEvts, err := l.src.Events(ctx, base.FocusedID)
	if err != nil {
		return base, err
	}

	// Sort ascending by Timestamp (chronological order).
	sort.Slice(allEvts, func(i, j int) bool {
		return allEvts[i].Timestamp.Before(allEvts[j].Timestamp)
	})

	// Filter: remove events that should not reach the TUI view (ADR-007).
	// Keep allEvts for tool_use ↔ tool_result matching (tool_result-only user
	// events carry the matching IDs but are excluded from display).
	visEvts := make([]event.Event, 0, len(allEvts))
	for _, ev := range allEvts {
		if isVisible(ev) {
			visEvts = append(visEvts, ev)
		}
	}

	base.Events = visEvts
	if len(visEvts) == 0 {
		base.EmptyReason = "session has no events"
	}

	base = l.applyUsageStats(base, visEvts)

	// Build timelines using allEvts (includes tool_result-only events needed for matching).
	base.Timelines = l.buildTimelines(ctx, p, base.FocusedID, allEvts)

	base = l.applyExplorerData(base, p.CWD())
	base = l.applyMCPs(base)
	base = l.applyLSPs(base)
	base = l.applyBashLog(base)
	base = l.applyCacheStats(base)
	// focusedStart anchors the usage-window reset on the focused session's first
	// event (allEvts is sorted ascending above, so its earliest event is the
	// session start).
	base = l.applyMultiWindowUsage(ctx, base, summaries, now, earliestEventTime(allEvts))
	base = l.applySessionStats(ctx, base, summaries)

	// ── Phase E: active file (diff target) ───────────────────────────────────
	// Scan visible events for the most recent Edit/Write/MultiEdit tool call
	// and record the file path as DiffFile.  Pure event scan — no I/O.
	// DiffHunks are populated by the TUI layer (applyLiveView) via git.Source.Diff
	// because the application layer must not import the concrete git adapter.
	base.DiffFile = activeFileFromEvents(visEvts)

	return base, nil
}

// SnapshotAggregated returns a View whose Events stream is the time-merged
// union of every recent session in cwd, with one main-session AgentTimeline
// per contributing session. Used by the Σ all tab in the TUI so the
// activity / bash / diff panels show a true cwd-wide aggregate instead of
// duplicating tab[0]'s content.
//
// FocusedID is anchored on the most-recently-active session so panels that
// need a single anchor (usage headline, current model, cache stats per
// session) keep working. CwdCacheStats already aggregates cache across
// sessions, so the cache panel already has the right number to show on Σ.
//
// Cost: one extra Events read per non-anchor session (capped at
// maxSessionStatsPerProject). Tolerable on the Σ tab since the user
// explicitly opted into it; not on the per-session tabs.
func (l *LiveSession) SnapshotAggregated(ctx context.Context, p project.Project) (View, error) {
	now := l.clk.Now()

	summaries, err := l.src.Sessions(ctx, p.CWD())
	if err != nil {
		return View{Project: p, LastUpdate: now}, err
	}
	if len(summaries) == 0 {
		return View{Project: p, LastUpdate: now, EmptyReason: "no sessions"}, nil
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].LastActivity.After(summaries[j].LastActivity)
	})

	// Phase 1 — anchored snapshot for usage / cache / per-session stats.
	anchor := summaries[0].ID
	base, err := l.SnapshotForSession(ctx, p, anchor)
	if err != nil {
		return base, err
	}

	// Phase 2 — pull events from every OTHER session that earns a tab
	// (live or mtime within aggregateSessionWindow). Filtering by
	// SessionStats.IsLive + the same lastActivity rule the tab strip
	// uses guarantees Σ never aggregates a session the user can't see.
	// Closed/idle sessions from days ago no longer pollute the counts.
	mergedAll := make([]event.Event, 0, len(base.Events))
	mergedAll = append(mergedAll, base.Events...)

	otherTimelines := make([]AgentTimeline, 0, len(base.SessionStats))
	for _, st := range base.SessionStats {
		if st.ID == anchor {
			continue
		}
		// Same predicate as the TUI tab strip, expressed against the
		// already-populated SessionStat (IsLive carries the process /
		// 30s mtime check; the broader 90s mtime check uses the
		// aggregateSessionWindow constant).
		if !st.IsLive && now.Sub(st.LastActivity) > aggregateSessionWindow {
			continue
		}
		evts, err := l.src.Events(ctx, st.ID)
		if err != nil {
			continue
		}
		sort.Slice(evts, func(a, b int) bool {
			return evts[a].Timestamp.Before(evts[b].Timestamp)
		})
		visEvts := make([]event.Event, 0, len(evts))
		for _, ev := range evts {
			if isVisible(ev) {
				visEvts = append(visEvts, ev)
			}
		}
		mergedAll = append(mergedAll, visEvts...)

		label := firstUserPromptLabel(evts)
		if label == "" {
			label = sessionLabelFromID(st.ID)
		}
		calls := extractToolCalls(evts)
		otherTimelines = append(otherTimelines, AgentTimeline{
			AgentID:   string(st.ID),
			AgentName: label,
			Events:    visEvts,
			Calls:     calls,
			Active:    hasActiveCall(calls),
		})
	}
	sort.Slice(mergedAll, func(a, b int) bool {
		return mergedAll[a].Timestamp.Before(mergedAll[b].Timestamp)
	})

	base.Events = mergedAll
	base.Timelines = append(base.Timelines, otherTimelines...)

	// Re-run the helpers whose output should reflect the merged stream
	// rather than the anchor session alone. BashLog → cwd-wide command
	// ledger; DiffFile → most recent edit across any session.
	base = l.applyBashLog(base)
	base.DiffFile = activeFileFromEvents(mergedAll)

	if len(mergedAll) == 0 {
		base.EmptyReason = "no events across sessions"
	} else {
		base.EmptyReason = ""
	}
	return base, nil
}

// applyUsageStats accumulates token usage and model info from visible assistant events.
//
// 1M context detection: if any event in this session has Has1hCache=true, the
// effective model ID is suffixed with "[1m]" (e.g. "claude-opus-4-7[1m]").
// The 1-hour prompt-cache tier is exclusively available to Max plan subscribers
// who have the 1M-token context window; it is the only reliable machine-readable
// indicator of the 1M variant since the JSONL message.model field does not
// carry a context-size suffix.
func (l *LiveSession) applyUsageStats(base View, visEvts []event.Event) View {
	totalUsage := usage.Zero()
	latestUsage := usage.Zero()
	assistantTurns := 0
	currentModel := ""
	has1MContext := false
	for _, ev := range visEvts {
		if ev.Kind != event.KindAssistant {
			continue
		}
		ap, ok := ev.Payload.(event.AssistantPayload)
		if !ok {
			continue
		}
		totalUsage = totalUsage.Add(ap.Usage)
		// `<synthetic>` is what Claude Code writes for compaction summaries
		// and other system-generated assistant turns. Their `model` field is
		// the literal string "<synthetic>" and their usage is just whatever
		// the summarizer used — not the user's real conversation context.
		// Letting them overwrite currentModel/latestUsage causes two visible
		// bugs:
		//   - title-bar model reads "<synthetic>" instead of "opus 4.7"
		//   - context % drops to ~0 because the synthetic event's tiny usage
		//     replaces the real latest turn's snapshot.
		// We still accumulate them into totalUsage (compaction tokens are real
		// billable work) but pin currentModel/latestUsage to the most recent
		// non-synthetic turn.
		//
		// Zero-usage assistant events are content-split duplicates produced
		// by Claude Code's streaming output: a single API call emitted as
		// multiple JSONL lines, with usage attributed to only the first
		// line. Skipping them here keeps latestUsage at the real value
		// instead of getting clobbered by the trailing zero-usage block.
		const syntheticModel = "<synthetic>"
		if ap.Model != syntheticModel && !ap.Usage.IsZero() {
			latestUsage = ap.Usage
			assistantTurns++
			if ap.Model != "" {
				currentModel = ap.Model
			}
			if ap.Has1hCache {
				has1MContext = true
			}
		}
	}
	// Synthesize "[1m]" suffix when the session is running under the 1M context
	// window (detected via 1h-cache tier usage).  Only appended once.
	const oneMSuffix = "[1m]"
	if has1MContext && currentModel != "" && !strings.HasSuffix(currentModel, oneMSuffix) {
		currentModel += oneMSuffix
	}
	base.TotalUsage = totalUsage
	base.LatestUsage = latestUsage
	base.CurrentModel = currentModel
	base.AssistantTurns = assistantTurns
	return base
}

// applyExplorerData populates FileTree and ModifiedFiles from the fs/git sources.
// Both sources degrade gracefully: errors leave the fields nil/empty.
func (l *LiveSession) applyExplorerData(base View, cwd string) View {
	if cwd == "" {
		return base
	}
	if l.fsSrc != nil {
		if tree, err := l.fsSrc.WalkToView(cwd); err == nil {
			base.FileTree = tree
		}
	}
	if l.gitSrc != nil {
		if statuses, err := l.gitSrc.StatusView(cwd); err == nil {
			base.ModifiedFiles = statuses
		}
	}
	return base
}

// maxMultiWindowSessions caps how many sessions are read across ALL projects when
// computing multi-window (5h / 7d) usage aggregations to bound snapshot latency.
const maxMultiWindowSessions = 50

// applyMultiWindowUsage populates Usage5h, UsageWeek, Sessions5hCount, and
// SessionsWeekCount by reading the most recent sessions whose LastActivity falls
// within each time window.
//
// When a GlobalSessionSource is configured, ALL projects under ~/.claude/projects/
// are walked (capped at maxMultiWindowSessions). This ensures that usage from
// other Claude Code projects (e.g. BorderGen, Nexus) is included in the 5h/7d
// totals.
//
// When no GlobalSessionSource is available, the method falls back to the current
// project's sessions only (legacy behavior).
//
// The focused session's events are already accumulated in base.TotalUsage; we
// reuse that value to avoid double-reading its JSONL file.
func (l *LiveSession) applyMultiWindowUsage(ctx context.Context, base View, summaries []session.Summary, now time.Time, focusedStart time.Time) View {
	if l.globalSrc != nil {
		return l.applyMultiWindowUsageGlobal(ctx, base, summaries, now, focusedStart)
	}
	return l.applyMultiWindowUsageSingleProject(ctx, base, summaries, now, focusedStart)
}

// earliestEventTime returns the minimum event timestamp, or the zero time when
// evts is empty. Used to anchor the usage-window reset on a session's first
// event rather than its last activity (file mtime).
func earliestEventTime(evts []event.Event) time.Time {
	var earliest time.Time
	for i := range evts {
		ts := evts[i].Timestamp
		if ts.IsZero() {
			continue
		}
		if earliest.IsZero() || ts.Before(earliest) {
			earliest = ts
		}
	}
	return earliest
}

// applyMultiWindowUsageGlobal aggregates usage across all projects using the
// GlobalSessionSource port. It caps at maxMultiWindowSessions sessions.
// windowAccumulator tracks per-window totals while iterating sessions.
type windowAccumulator struct {
	total      usage.Usage
	count      int
	earliest   time.Time
	windowSize time.Duration

	// earliestStart is the earliest FIRST-event timestamp among in-window
	// sessions. The reset countdown anchors here: anchoring on earliest
	// LastActivity (the old behavior) pinned the countdown at the full
	// window size forever during continuous use, because a live session's
	// LastActivity ≈ now on every snapshot.
	earliestStart time.Time
}

// add includes a session's usage if it falls within the window.
// firstEvent may be zero when unknown (e.g. unreadable JSONL); the reset is
// anchored on the earliest first-event across in-window sessions because
// Anthropic anchors the 5h/7d window at the first message, not the last
// activity (file mtime) — anchoring on mtime made the countdown slide
// forward every tick for an active session and never decrease.
func (w *windowAccumulator) add(u usage.Usage, lastActivity, firstEvent time.Time, age time.Duration) {
	if age > w.windowSize {
		return
	}
	w.total = w.total.Add(u)
	w.count++
	if !lastActivity.IsZero() && (w.earliest.IsZero() || lastActivity.Before(w.earliest)) {
		w.earliest = lastActivity
	}
	if !firstEvent.IsZero() && (w.earliestStart.IsZero() || firstEvent.Before(w.earliestStart)) {
		w.earliestStart = firstEvent
	}
}

// anchor returns the timestamp the reset countdown tiles from: the earliest
// first-event when known, else the earliest last-activity (legacy fallback).
func (w *windowAccumulator) anchor() time.Time {
	if !w.earliestStart.IsZero() {
		return w.earliestStart
	}
	return w.earliest
}

// nextResetAfter computes when a rolling window anchored at `anchor` next
// resets, strictly after `now`. The anchor is truncated to the hour and the
// window tiles forward in whole blocks — mirroring Anthropic's billing
// blocks (a 5h session starts at the first message, rounded to the hour).
func nextResetAfter(anchor, now time.Time, window time.Duration) time.Time {
	if anchor.IsZero() || window <= 0 {
		return time.Time{}
	}
	a := anchor.UTC().Truncate(time.Hour)
	reset := a.Add(window)
	for !reset.After(now) {
		blocks := now.Sub(a) / window
		reset = a.Add(window * (blocks + 1))
	}
	return reset
}

func (l *LiveSession) applyMultiWindowUsageGlobal(ctx context.Context, base View, summaries []session.Summary, now time.Time, focusedStart time.Time) View {
	w5h := windowAccumulator{windowSize: 5 * time.Hour}
	wWeek := windowAccumulator{windowSize: 7 * 24 * time.Hour}

	refs, err := l.globalSrc.AllProjectSessions(ctx, maxMultiWindowSessions)
	if err != nil {
		return l.applyMultiWindowUsageSingleProject(ctx, base, summaries, now, focusedStart)
	}

	focusedID := string(base.FocusedID)
	for _, ref := range refs {
		age := now.Sub(ref.LastActivity)
		if age > wWeek.windowSize {
			break // refs sorted desc; remaining are all too old
		}
		u, first := l.usageForRef(ctx, ref, focusedID, base.TotalUsage, focusedStart)
		w5h.add(u, ref.LastActivity, first, age)
		wWeek.add(u, ref.LastActivity, first, age)
	}

	base.Usage5h = w5h.total
	base.UsageWeek = wWeek.total
	base.Sessions5hCount = w5h.count
	base.SessionsWeekCount = wWeek.count
	base.Reset5hAt = nextResetAfter(w5h.anchor(), now, w5h.windowSize)
	base.ResetWeekAt = nextResetAfter(wWeek.anchor(), now, wWeek.windowSize)
	return base
}

// firstEventTime returns the timestamp of the first event, or zero.
func firstEventTime(evts []event.Event) time.Time {
	if len(evts) == 0 {
		return time.Time{}
	}
	return evts[0].Timestamp
}

// usageForRef returns the usage for a session ref, reusing base.TotalUsage when
// the ref is the focused session (avoids re-reading the JSONL).
//
// Non-focused sessions are served from usageCache when the cached
// LastActivity matches the ref's current LastActivity — the JSONL
// hasn't been appended to since the last read, so the sum is unchanged.
// This caps JSONL re-parses at "once per session activity event" rather
// than "once per snapshot tick × every recent session in every project".
func (l *LiveSession) usageForRef(ctx context.Context, ref ports.GlobalSessionRef, focusedID string, focusedUsage usage.Usage, focusedFirst time.Time) (usage.Usage, time.Time) {
	if string(ref.SessionID) == focusedID {
		return focusedUsage, focusedFirst
	}

	if l.usage != nil {
		l.usage.mu.Lock()
		if entry, ok := l.usage.entries[ref.SessionID]; ok && entry.activity.Equal(ref.LastActivity) {
			l.usage.mu.Unlock()
			return entry.usage, entry.firstEvent
		}
		l.usage.mu.Unlock()
	}

	evts, err := l.src.Events(ctx, ref.SessionID)
	if err != nil {
		return usage.Zero(), time.Time{}
	}
	u := sumUsageFromEvents(evts)
	first := firstEventTime(evts)

	if l.usage != nil {
		l.usage.mu.Lock()
		l.usage.entries[ref.SessionID] = sessionUsageCacheEntry{usage: u, activity: ref.LastActivity, firstEvent: first}
		l.usage.mu.Unlock()
	}
	return u, first
}

// applyMultiWindowUsageSingleProject is the legacy fallback that aggregates
// usage only within the current project's sessions.
func (l *LiveSession) applyMultiWindowUsageSingleProject(ctx context.Context, base View, summaries []session.Summary, now time.Time, focusedStart time.Time) View {
	w5h := windowAccumulator{windowSize: 5 * time.Hour}
	wWeek := windowAccumulator{windowSize: 7 * 24 * time.Hour}

	// summaries are sorted descending by LastActivity (see SnapshotForSession).
	reads := 0
	for _, sum := range summaries {
		age := now.Sub(sum.LastActivity)
		if age > wWeek.windowSize {
			break // remaining summaries are all older than the widest window
		}

		var u usage.Usage
		var start time.Time
		if sum.ID == base.FocusedID {
			// The focused session's usage is already accumulated in
			// base.TotalUsage — reuse it (avoids a re-read). Match by IDENTITY,
			// not position: focusing an OLDER tab must not double-count the
			// focused session nor drop the most-recent one.
			u = base.TotalUsage
			start = focusedStart
		} else {
			if reads >= maxMultiWindowSessions {
				continue
			}
			evts, err := l.src.Events(ctx, sum.ID)
			reads++
			if err != nil {
				continue
			}
			u = sumUsageFromEvents(evts)
			start = firstEventTime(evts)
		}
		w5h.add(u, sum.LastActivity, start, age)
		wWeek.add(u, sum.LastActivity, start, age)
	}

	base.Usage5h = w5h.total
	base.UsageWeek = wWeek.total
	base.Sessions5hCount = w5h.count
	base.SessionsWeekCount = wWeek.count
	base.Reset5hAt = nextResetAfter(w5h.anchor(), now, w5h.windowSize)
	base.ResetWeekAt = nextResetAfter(wWeek.anchor(), now, wWeek.windowSize)
	return base
}

// pickFocusedID returns the explicitly-requested focused session ID when it
// matches one of the available summaries; otherwise it falls back to the
// first summary (most-recently-active by sort order).
func pickFocusedID(summaries []session.Summary, requested session.ID) session.ID {
	if requested == "" {
		return summaries[0].ID
	}
	for _, s := range summaries {
		if s.ID == requested {
			return requested
		}
	}
	return summaries[0].ID
}

// maxSessionStatsPerProject caps how many sessions in a single cwd are
// surfaced to the TUI tab strip and leaderboard. Above this we'd just be
// rendering zombies anyway, and the per-session event read cost would grow.
const maxSessionStatsPerProject = 8

// liveActivityWindow is how recent a session's JSONL must have been written
// to count as "live" (claude code still has it open). 30s comfortably
// covers normal tool-call cadence and idle thinking, while excluding
// sessions the user has long since stopped touching.
const liveActivityWindow = 30 * time.Second

// aggregateSessionWindow mirrors the TUI's session_tabs.go
// activeSessionWindow constant. A session contributes to the Σ tab's
// aggregate counts (cwd-wide cache stats, merged events for activity /
// bash / diff) ONLY when it would also earn a tab in the title-bar
// strip, i.e. process running OR mtime within this window. Without
// this guard Σ inflates with every long-dead session that ever
// touched the cwd.
const aggregateSessionWindow = 90 * time.Second

// sessionContributesToAggregate reports whether the given (running,
// lastActivity) pair earns a slot in the Σ aggregate view. Mirrors the
// tab-strip inclusion rule.
func sessionContributesToAggregate(running bool, lastActivity, snapshotAt time.Time) bool {
	return running || snapshotAt.Sub(lastActivity) <= aggregateSessionWindow
}

// isSessionLive resolves whether a session in this cwd should appear as
// live in the title-bar tab strip.
//
// Rules:
//  1. JSONL appended within liveActivityWindow → always live.
//  2. ps(1) reports a `claude --session-id <X>` process for this ID:
//     a. If this session IS the freshest in its cwd → live.
//     b. Else, only live when the freshest session in cwd ALSO has its
//     own argv-detected process (freshestIsArgvDetected). That
//     signals genuinely parallel `claude` invocations — each PID is
//     bound to its own session ID and every sibling is real.
//
// Why the freshestIsArgvDetected guard: claude code's `/new` command
// starts a fresh session inside the SAME OS process, so the original
// argv still references the previous session ID. The freshest JSONL
// in the cwd then has no matching process entry. In that case the
// older argv-detected session is a ghost and we demote it. With two
// separate `claude` invocations, both session IDs appear in argv —
// the older sibling is a real process the user can still type into.
func isSessionLive(id session.ID, runningIDs map[session.ID]bool, lastActivity, freshestInCwd time.Time, freshestIsArgvDetected bool, snapshotAt time.Time) bool {
	if snapshotAt.Sub(lastActivity) <= liveActivityWindow {
		return true
	}
	if !runningIDs[id] {
		return false
	}
	if !lastActivity.Before(freshestInCwd) {
		return true
	}
	return freshestIsArgvDetected
}

// applySessionStats walks every session in the current project (cwd) and
// computes per-session display data: a short label, the latest assistant
// usage, the model context % at that latest turn, and an active flag.
//
// The focused session reuses base.LatestUsage / base.Events to avoid a
// duplicate read. Other sessions' events are read once each — this is
// bounded by maxSessionStatsPerProject.
//
// Sessions are returned in summaries-order (LastActivity desc).
func (l *LiveSession) applySessionStats(ctx context.Context, base View, summaries []session.Summary) View {
	if len(summaries) == 0 {
		return base
	}

	// Probe the OS process list for live `claude --session-id <X>`
	// invocations. The set is intersected against `summaries` below,
	// which is already filtered to THIS project's cwd via
	// SessionSource.Sessions(cwd) — so a `claude` process running in a
	// different project's cwd cannot appear in the resulting tabs even
	// if its session ID came back from the scan. Empty set when
	// procSrc is unset or the scan errors (best-effort: degrade to
	// mtime-only liveness, the v0.6 behavior).
	runningIDs := make(map[session.ID]bool)
	if l.procSrc != nil {
		if ids, err := l.procSrc.RunningClaudeSessionIDs(ctx); err == nil {
			for _, id := range ids {
				runningIDs[id] = true
			}
		}
	}

	// Pre-compute the freshest activity timestamp across all visible
	// sessions in this cwd, plus whether the freshest session is itself
	// argv-detected. isSessionLive uses both to distinguish a /new
	// ghost (older sibling with stale mtime, freshest has no process)
	// from parallel `claude` invocations (older sibling with its own
	// running process, freshest also has a process).
	freshestInCwd := time.Time{}
	var freshestID session.ID
	for _, sum := range summaries {
		if sum.LastActivity.After(freshestInCwd) {
			freshestInCwd = sum.LastActivity
			freshestID = sum.ID
		}
	}
	freshestIsArgvDetected := runningIDs[freshestID]

	stats := make([]SessionStat, 0, len(summaries))
	cwdCache := CacheStats{}
	for i, sum := range summaries {
		if i >= maxSessionStatsPerProject {
			break
		}
		var (
			latest   usage.Usage
			total    usage.Usage
			label    string
			active   bool
			modelID  string
			cache    CacheStats
			realLast time.Time
		)
		if sum.ID == base.FocusedID {
			latest = base.LatestUsage
			total = base.TotalUsage
			label = firstUserPromptLabel(base.Events)
			active = sessionIsActive(base.Timelines)
			modelID = base.CurrentModel
			cache = base.CacheStats
			realLast = lastRealEventTime(base.Events)
		} else {
			evts, err := l.src.Events(ctx, sum.ID)
			if err != nil {
				continue
			}
			latest, total, modelID = sessionUsageFromEvents(evts)
			label = firstUserPromptLabel(evts)
			active = false // only the focused view's timelines tell us about live tool calls
			cache = cacheStatsFromEvents(evts)
			realLast = lastRealEventTime(evts)
		}
		if label == "" {
			label = sessionLabelFromID(sum.ID)
		}
		ctxPct := 0
		if modelID != "" {
			m := pricing.Lookup(modelID)
			ctxPct = int(pricing.CompactionPercent(latest, m) * 100)
		}
		// LastActivity prefers the latest real-event timestamp over the
		// JSONL file's mtime, because /resume and other no-op operations
		// inside claude code touch the file without producing real events.
		// mtime can lie; event timestamps don't.
		effectiveLast := realLast
		if effectiveLast.IsZero() {
			effectiveLast = sum.LastActivity
		}
		// IsLive: see isSessionLive. JSONL-mtime activity wins outright;
		// argv-based detection is honored when the session is the freshest
		// in its cwd, or when the freshest sibling also has its own
		// running process (parallel `claude` invocations).
		isLive := isSessionLive(sum.ID, runningIDs, effectiveLast, freshestInCwd, freshestIsArgvDetected, base.LastUpdate)

		stats = append(stats, SessionStat{
			ID:            sum.ID,
			Label:         label,
			LastActivity:  effectiveLast,
			LatestUsage:   latest,
			ContextPct:    ctxPct,
			SessionTokens: pricing.TotalTokens(total),
			Active:        active,
			IsLive:        isLive,
		})
		// Only merge into the cwd-wide cache totals when this session
		// would also appear as a tab in the strip — Σ should mirror
		// the tabs the user sees, never report on closed sessions
		// from days ago that happened to live in the same cwd.
		if sessionContributesToAggregate(runningIDs[sum.ID], effectiveLast, base.LastUpdate) {
			cwdCache = mergeCacheStats(cwdCache, cache)
		}
	}
	base.SessionStats = stats
	base.CwdCacheStats = cwdCache
	return base
}

// sessionUsageFromEvents extracts the latest assistant Usage, the running
// total Usage, and the effective model ID (with the synthesized "[1m]"
// suffix when the session is on the 1M-context tier) from a session's
// full event stream. Returns zero values when no assistant events are
// present.
//
// Mirrors the synthetic + zero-usage skip AND the 1M context detection
// from applyUsageStats so the per-session ContextPct on the Σ tab's
// leaderboard divides by the correct ContextLimit. Without 1M detection
// here, a 200k-tokens-used 1M session reads as 84% (200k/200k) when
// non-focused but 20% (200k/1M) when focused — same usage, different
// limit, drastically different "compaction imminent?" signal.
func sessionUsageFromEvents(evts []event.Event) (latest, total usage.Usage, modelID string) {
	const syntheticModel = "<synthetic>"
	total = usage.Zero()
	has1MContext := false
	for _, ev := range evts {
		if ev.Kind != event.KindAssistant {
			continue
		}
		ap, ok := ev.Payload.(event.AssistantPayload)
		if !ok {
			continue
		}
		total = total.Add(ap.Usage)
		if ap.Model == syntheticModel || ap.Usage.IsZero() {
			continue
		}
		latest = ap.Usage
		if ap.Model != "" {
			modelID = ap.Model
		}
		if ap.Has1hCache {
			has1MContext = true
		}
	}
	const oneMSuffix = "[1m]"
	if has1MContext && modelID != "" && !strings.HasSuffix(modelID, oneMSuffix) {
		modelID += oneMSuffix
	}
	return latest, total, modelID
}

// lastRealEventTime returns the timestamp of the most recent non-synthetic
// assistant or user event in evts, or the zero time when none is found.
// Used as a more reliable "is this session live?" signal than file mtime —
// mtime updates on /resume or other no-op operations claude code performs,
// so a closed session can read as "recent" via mtime even though no real
// event has been written for hours.
func lastRealEventTime(evts []event.Event) time.Time {
	const syntheticModel = "<synthetic>"
	var latest time.Time
	for _, ev := range evts {
		switch p := ev.Payload.(type) {
		case event.AssistantPayload:
			if p.Model == syntheticModel {
				continue
			}
			if ev.Timestamp.After(latest) {
				latest = ev.Timestamp
			}
		case event.UserPayload:
			if p.IsMeta || p.IsToolResultOnly {
				continue
			}
			if ev.Timestamp.After(latest) {
				latest = ev.Timestamp
			}
		}
	}
	return latest
}

// firstUserPromptLabel scans the visible events for the earliest user prompt
// and returns a short, single-line prefix suitable for a tab label. Returns
// "" when no user event is present (caller should fall back to the session ID).
func firstUserPromptLabel(evts []event.Event) string {
	for _, ev := range evts {
		if ev.Kind != event.KindUser {
			continue
		}
		up, ok := ev.Payload.(event.UserPayload)
		if !ok || up.IsToolResultOnly || up.IsMeta {
			continue
		}
		text := strings.TrimSpace(up.Summary)
		if text == "" {
			continue
		}
		// Collapse newlines and tabs to single spaces.
		text = strings.Map(func(r rune) rune {
			if r == '\n' || r == '\t' || r == '\r' {
				return ' '
			}
			return r
		}, text)
		// Strip Claude Code's <command-...> wrapper tags from the rare
		// session-start prompts so labels read as plain text.
		if strings.HasPrefix(text, "<") {
			if cut := strings.Index(text, ">"); cut > 0 && cut < 64 {
				text = strings.TrimSpace(text[cut+1:])
			}
		}
		// Cap to a tab-friendly length. 16 visual chars keeps the strip
		// compact without making labels useless.
		const maxLabelChars = 16
		if len([]rune(text)) > maxLabelChars {
			runes := []rune(text)
			text = string(runes[:maxLabelChars-1]) + "…"
		}
		return text
	}
	return ""
}

// sessionLabelFromID returns a fallback label using the first 6 chars of the
// session ID. Used when no first-user-prompt could be extracted.
func sessionLabelFromID(id session.ID) string {
	s := string(id)
	if len(s) > 6 {
		return s[:6]
	}
	return s
}

// sessionIsActive returns true when any timeline in the focused view has at
// least one running tool call (Active=true).
func sessionIsActive(timelines []AgentTimeline) bool {
	for _, tl := range timelines {
		if tl.Active {
			return true
		}
	}
	return false
}

// sumUsageFromEvents accumulates token usage from all KindAssistant events.
func sumUsageFromEvents(evts []event.Event) usage.Usage {
	total := usage.Zero()
	for _, ev := range evts {
		if ev.Kind != event.KindAssistant {
			continue
		}
		ap, ok := ev.Payload.(event.AssistantPayload)
		if !ok {
			continue
		}
		total = total.Add(ap.Usage)
	}
	return total
}

// applyMCPs populates MCPs from the MCP source.
// Degrades gracefully: errors or missing source leave MCPs empty.
func (l *LiveSession) applyMCPs(base View) View {
	if l.mcpSrc == nil {
		return base
	}
	entries, err := l.mcpSrc.MCPs()
	if err != nil || len(entries) == 0 {
		return base
	}
	mcps := make([]MCP, 0, len(entries))
	for _, e := range entries {
		mcps = append(mcps, MCP{
			Name:      e.Name,
			Enabled:   e.Enabled,
			ToolCount: e.ToolCount,
		})
	}
	base.MCPs = mcps
	return base
}

// applyBashLog flattens every Bash tool call from every agent timeline into
// a single chronological list ordered by StartTime. This powers the bash
// audit panel: a session-wide ledger of "what shells did claude run".
func (l *LiveSession) applyBashLog(base View) View {
	var bashes []ToolCall
	for _, tl := range base.Timelines {
		for _, c := range tl.Calls {
			if c.Tool == "Bash" {
				bashes = append(bashes, c)
			}
		}
	}
	// Sort by StartTime ascending — chronological reading order.
	for i := 1; i < len(bashes); i++ {
		for j := i; j > 0 && bashes[j-1].StartTime.After(bashes[j].StartTime); j-- {
			bashes[j-1], bashes[j] = bashes[j], bashes[j-1]
		}
	}
	base.BashLog = bashes
	return base
}

// applyCacheStats walks every assistant event in the focused window and
// computes prompt-cache efficiency. Per-turn ratio history (capped at 10
// entries) drives the sparkline; aggregate totals drive the headline.
func (l *LiveSession) applyCacheStats(base View) View {
	base.CacheStats = cacheStatsFromEvents(base.Events)
	return base
}

// cacheStatsFromEvents computes CacheStats from a slice of events. Pure
// function — used both by the focused-session path (applyCacheStats) and
// the cwd-aggregate path (applySessionStats accumulates across sessions).
func cacheStatsFromEvents(evts []event.Event) CacheStats {
	const trendCap = 10
	var (
		fromCache, recomputed int64
		biggestMissTok        int64
		biggestMissAt         time.Time
		trend                 []float64
		turnCount             int
	)
	for _, ev := range evts {
		if ev.Kind != event.KindAssistant {
			continue
		}
		ap, ok := ev.Payload.(event.AssistantPayload)
		if !ok {
			continue
		}
		miss := ap.Usage.Input + ap.Usage.CacheCreation
		hit := ap.Usage.CacheRead
		if miss == 0 && hit == 0 {
			continue
		}
		fromCache += hit
		recomputed += miss
		turnCount++
		if miss > biggestMissTok {
			biggestMissTok = miss
			biggestMissAt = ev.Timestamp
		}
		var ratio float64
		if hit+miss > 0 {
			ratio = float64(hit) / float64(hit+miss)
		}
		trend = append(trend, ratio)
		if len(trend) > trendCap {
			trend = trend[len(trend)-trendCap:]
		}
	}
	stats := CacheStats{
		FromCache:         fromCache,
		Recomputed:        recomputed,
		BiggestMissTokens: biggestMissTok,
		BiggestMissAt:     biggestMissAt,
		Trend:             trend,
		TurnCount:         turnCount,
	}
	if total := fromCache + recomputed; total > 0 {
		stats.HitRatio = float64(fromCache) / float64(total)
	}
	return stats
}

// mergeCacheStats combines two CacheStats into one — used to roll the
// per-session results from applySessionStats up into a single cwd-wide
// aggregate (the Σ tab in the TUI). Trends are concatenated and trimmed
// to the last 10 entries; biggest miss is the max over both.
func mergeCacheStats(a, b CacheStats) CacheStats {
	const trendCap = 10
	out := CacheStats{
		FromCache:         a.FromCache + b.FromCache,
		Recomputed:        a.Recomputed + b.Recomputed,
		BiggestMissTokens: a.BiggestMissTokens,
		BiggestMissAt:     a.BiggestMissAt,
		TurnCount:         a.TurnCount + b.TurnCount,
	}
	if b.BiggestMissTokens > out.BiggestMissTokens {
		out.BiggestMissTokens = b.BiggestMissTokens
		out.BiggestMissAt = b.BiggestMissAt
	}
	out.Trend = append(out.Trend, a.Trend...)
	out.Trend = append(out.Trend, b.Trend...)
	if len(out.Trend) > trendCap {
		out.Trend = out.Trend[len(out.Trend)-trendCap:]
	}
	if total := out.FromCache + out.Recomputed; total > 0 {
		out.HitRatio = float64(out.FromCache) / float64(total)
	}
	return out
}

// applyLSPs populates LSPs from the LSP source. Degrades gracefully when
// the source is nil or returns an empty / errored result.
func (l *LiveSession) applyLSPs(base View) View {
	if l.lspSrc == nil {
		return base
	}
	entries, err := l.lspSrc.LSPs()
	if err != nil || len(entries) == 0 {
		return base
	}
	lsps := make([]LSP, 0, len(entries))
	for _, e := range entries {
		lsps = append(lsps, LSP{Name: e.Name, Active: e.Active, ClaudeEnabled: e.ClaudeEnabled})
	}
	base.LSPs = lsps
	return base
}

// activeFileFromEvents scans a slice of events (ascending chronological order)
// and returns the file_path argument from the most recent Edit, Write, or
// MultiEdit tool_use event.  Returns "" when no such event exists.
func activeFileFromEvents(evts []event.Event) string {
	active := ""
	for _, ev := range evts {
		if ev.Kind != event.KindAssistant {
			continue
		}
		ap, ok := ev.Payload.(event.AssistantPayload)
		if !ok {
			continue
		}
		tool, arg := parseToolFromSummary(ap.Summary)
		switch strings.ToLower(tool) {
		case "edit", "write", "multiedit":
			if arg != "" {
				active = arg
			}
		}
	}
	return active
}

// buildTimelines constructs AgentTimeline entries for the main session and any
// discovered subagents. allMainEvts is the full event list (before visibility
// filtering) so tool_result-only user events are available for matching.
func (l *LiveSession) buildTimelines(ctx context.Context, p project.Project, focusedID session.ID, allMainEvts []event.Event) []AgentTimeline {
	var timelines []AgentTimeline

	// Build the visible subset for display (tool_result-only events excluded).
	visMainEvts := make([]event.Event, 0, len(allMainEvts))
	for _, ev := range allMainEvts {
		if isVisible(ev) {
			visMainEvts = append(visMainEvts, ev)
		}
	}

	// Main session timeline: use allMainEvts for matching, visMainEvts for display.
	mainCalls := extractToolCalls(allMainEvts)
	mainActive := hasActiveCall(mainCalls)
	timelines = append(timelines, AgentTimeline{
		AgentID:    string(focusedID),
		AgentName:  "main session",
		IsSubagent: false,
		Events:     visMainEvts,
		Calls:      mainCalls,
		Active:     mainActive,
	})

	// Subagent timelines (requires SubagentSource).
	if l.subSrc == nil {
		return timelines
	}

	infos, err := l.subSrc.Subagents(ctx, p.CWD(), string(focusedID))
	if err != nil || len(infos) == 0 {
		return timelines
	}

	for _, info := range infos {
		agEvts, err := l.subSrc.SubagentEvents(ctx, p.CWD(), string(focusedID), info.AgentID)
		if err != nil {
			// Skip unreadable subagents rather than failing the whole snapshot.
			continue
		}

		// Sort chronologically.
		sort.Slice(agEvts, func(i, j int) bool {
			return agEvts[i].Timestamp.Before(agEvts[j].Timestamp)
		})

		// Use ALL events (not just visible) for tool-call matching, since
		// tool_result-only user events carry the matching IDs.
		//
		// Build the visible subset into a FRESH slice. Filtering in place with
		// agEvts[:0] shares agEvts' backing array, so the compaction would
		// overwrite the tool_result-only events that extractToolCalls needs —
		// leaving completed calls stuck "active". Mirror the main-session path.
		visAgEvts := make([]event.Event, 0, len(agEvts))
		for _, ev := range agEvts {
			if isVisible(ev) {
				visAgEvts = append(visAgEvts, ev)
			}
		}

		calls := extractToolCalls(agEvts)
		active := hasActiveCall(calls)

		timelines = append(timelines, AgentTimeline{
			AgentID:    info.AgentID,
			AgentName:  subagentName(info),
			IsSubagent: true,
			ParentID:   string(focusedID),
			Events:     visAgEvts,
			Calls:      calls,
			Active:     active,
		})
	}

	return timelines
}

// extractToolCalls walks a list of events and builds ToolCall entries by
// matching tool_use events with their corresponding tool_result events.
//
// Algorithm:
//  1. Walk events in order.
//  2. On KindAssistant with ToolUseID set → create a ToolCall with State=Active.
//  3. On KindUser with ToolResultIDs → for each ID, find and close the matching
//     ToolCall (computing duration, setting state to Done or Failed).
func extractToolCalls(evts []event.Event) []ToolCall {
	// pending maps tool_use_id → index in calls slice.
	pending := make(map[string]int)
	var calls []ToolCall

	for i := range evts {
		switch evts[i].Kind {
		case event.KindAssistant:
			appendToolUseCall(evts[i], pending, &calls)
		case event.KindUser:
			closeToolResultCalls(evts[i], pending, calls)
		}
	}

	return calls
}

// appendToolUseCall registers a new active ToolCall from an assistant event.
// No-op when the event has no ToolUseID. Self-dedupes on ToolUseID: branch
// duplicates emitted by Claude Code's DAG (same tool_use_id replicated
// across branches) are recorded only once.
func appendToolUseCall(ev event.Event, pending map[string]int, calls *[]ToolCall) {
	ap, ok := ev.Payload.(event.AssistantPayload)
	if !ok || ap.ToolUseID == "" {
		return
	}
	if _, already := pending[ap.ToolUseID]; already {
		return
	}
	tool, keyArg := parseToolFromSummary(ap.Summary)
	if tool == "" {
		tool = ap.ToolName
	}
	idx := len(*calls)
	*calls = append(*calls, ToolCall{
		ToolUseID: ap.ToolUseID,
		Tool:      tool,
		KeyArg:    truncateStr(keyArg, 50),
		State:     CallActive,
		StartTime: ev.Timestamp,
	})
	pending[ap.ToolUseID] = idx
}

// closeToolResultCalls matches tool_result IDs in a user event to their
// pending tool_use ToolCalls and marks them Done or Failed.
func closeToolResultCalls(ev event.Event, pending map[string]int, calls []ToolCall) {
	up, ok := ev.Payload.(event.UserPayload)
	if !ok || len(up.ToolResultIDs) == 0 {
		return
	}
	for _, toolUseID := range up.ToolResultIDs {
		idx, found := pending[toolUseID]
		if !found {
			continue
		}
		calls[idx].Duration = ev.Timestamp.Sub(calls[idx].StartTime)
		if up.ToolResultError {
			calls[idx].State = CallFailed
		} else {
			calls[idx].State = CallDone
		}
		delete(pending, toolUseID)
	}
}

// hasActiveCall returns true if any call in the slice is in CallActive state.
func hasActiveCall(calls []ToolCall) bool {
	for _, c := range calls {
		if c.State == CallActive {
			return true
		}
	}
	return false
}

// parseToolFromSummary splits a summary like "Tool: Read /some/path" into
// ("Read", "/some/path"). Returns ("", "") if the summary is not in "Tool: " format.
func parseToolFromSummary(summary string) (tool, keyArg string) {
	const prefix = "Tool: "
	if len(summary) <= len(prefix) {
		return "", ""
	}
	if summary[:len(prefix)] != prefix {
		return "", ""
	}
	rest := summary[len(prefix):]
	for i, r := range rest {
		if r == ' ' {
			return rest[:i], rest[i+1:]
		}
	}
	return rest, ""
}

// truncateStr truncates s to at most maxChars runes.
func truncateStr(s string, maxChars int) string {
	if maxChars <= 0 {
		return ""
	}
	count := 0
	for i := range s {
		if count == maxChars {
			return s[:i]
		}
		count++
	}
	return s
}

// subagentName returns a human-readable name for a subagent.
func subagentName(info ports.SubagentInfo) string {
	if info.Description != "" {
		return truncateStr(info.Description, 40)
	}
	// Fall back to a shortened agent ID.
	id := info.AgentID
	if len(id) > 12 {
		return fmt.Sprintf("agent \u2026%s", id[len(id)-8:])
	}
	return id
}

// isVisible returns true for events that should reach the TUI view.
//
// Hides:
//   - Opaque events (KindOpaque): unrecognized JSONL event types.
//   - User events with IsMeta = true: system-injected scaffolding.
//   - User events with IsToolResultOnly = true: plumbing turns.
func isVisible(ev event.Event) bool {
	if ev.Kind == event.KindOpaque {
		return false
	}
	if up, ok := ev.Payload.(event.UserPayload); ok {
		if up.IsMeta || up.IsToolResultOnly {
			return false
		}
	}
	return true
}
