// Package jsonl implements ports.SessionSource by reading Claude Code JSONL
// session files from ~/.claude/projects/<encoded-cwd>/.
//
// # Directory layout
//
//	~/.claude/projects/
//	  -Users-alice-work-clyde/   ← encodeProjectPath("/Users/alice/work/clyde")
//	    <uuid>.jsonl              ← one file per session
//
// # JSONL format
//
// Each line is a JSON object representing one event. All events share a common
// envelope (uuid, type, timestamp, sessionId, parentUuid). Per-type variable
// fields are captured in json.RawMessage so the domain stays free of JSON types.
//
// # Design notes
//
//   - bufio.Scanner buffer is raised to 4MB (from the default 64KB) because
//     assistant events with extended thinking blocks can exceed 64KB on a single
//     JSONL line.
//   - Unknown event types are returned as event.OpaquePayload{Raw: rawLine}
//     with the ENTIRE line preserved (not just the payload sub-object). This
//     keeps the door open for future panels to decode it without a domain change.
//   - Malformed lines (invalid JSON) return a wrapped error naming file + line
//     number. Skipping vs. failing is a policy choice; we fail to avoid silently
//     returning incomplete data. Document callers that want skip behavior.
//   - Sessions() uses file mtime for LastActivity (cheap: one stat per file).
//     Reading every JSONL to find the last timestamp is O(events) vs O(sessions);
//     mtime is close enough for session ordering and is consistent with the
//     append-only semantics of the JSONL files.
//   - Missing or empty project directory → empty slice, no error (not every
//     project has had a Claude Code session).
package jsonl

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Systemartis/clyde/internal/domain/event"
	"github.com/Systemartis/clyde/internal/domain/session"
	"github.com/Systemartis/clyde/internal/domain/usage"
	"github.com/Systemartis/clyde/internal/ports"
)

// scannerMaxToken is the maximum JSONL line size we will attempt to decode.
// 4MB comfortably covers assistant events with thinking blocks (observed up to
// ~200KB in practice; 4MB leaves ample headroom).
const scannerMaxToken = 4 * 1024 * 1024

// Source implements ports.SessionSource by reading Claude Code JSONL files.
// The zero value is not ready to use — construct with NewSource.
type Source struct {
	// baseDir is the root that contains project subdirectories.
	// In production this is os.UserHomeDir() + "/.claude/projects".
	// In tests it is a t.TempDir() that mirrors the same layout.
	baseDir string

	// eventCache memoises decoded events per session, keyed by file
	// mtime + size. The TUI calls Events() ~1Hz across multiple
	// sessions for the Σ aggregate tab; without caching every tick
	// re-reads + re-parses thousands of JSONL lines for sessions that
	// haven't been touched in minutes. Stat is cheap (~µs); decode is
	// the expensive part. Invalidate on any (mtime, size) mismatch —
	// catches both new appends and out-of-band rewrites.
	cacheMu    sync.Mutex
	eventCache map[session.ID]eventCacheEntry
}

// ShapeMisses is a snapshot of decode-failure counters. Returned by
// Source.ShapeMisses(). Counters are process-global (decode is a free
// function so the fuzz harness can call it without a Source).
type ShapeMisses struct {
	// User is the number of user-message decodes that returned an error.
	User int64
	// Assistant is the number of assistant-message Usage decodes that errored.
	Assistant int64
	// Content is the number of assistant-message Content decodes that errored.
	Content int64
}

// ShapeMisses returns the current snapshot of decode-failure counters.
// Safe to call concurrently with decode-driven traffic.
func (s *Source) ShapeMisses() ShapeMisses {
	return ShapeMisses{
		User:      shapeMissUser.Load(),
		Assistant: shapeMissAssistant.Load(),
		Content:   shapeMissContent.Load(),
	}
}

// eventCacheEntry is one memoised decode result.
type eventCacheEntry struct {
	mtime  time.Time
	size   int64
	path   string
	events []event.Event
}

// NewSource returns a Source that resolves session files under baseDir.
// In production pass filepath.Join(homeDir, ".claude", "projects").
// Tests pass a temporary directory with the same structure.
func NewSource(baseDir string) *Source {
	return &Source{baseDir: baseDir}
}

// NewProductionSource returns a Source wired to the real Claude Code project
// directory. Returns an error if the user's home directory cannot be determined.
func NewProductionSource() (*Source, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("jsonl: cannot determine home directory: %w", err)
	}
	return NewSource(filepath.Join(home, ".claude", "projects")), nil
}

// Sessions returns all sessions for the given project working directory, ordered
// by LastActivity descending (most recently active first).
//
// LastActivity is derived from the file mtime — this is cheap (one stat per
// file) and consistent with the append-only nature of the JSONL files.
//
// Empty or missing directory → empty slice, no error.
func (s *Source) Sessions(_ context.Context, projectCWD string) ([]session.Summary, error) {
	projectDir := filepath.Join(s.baseDir, encodeProjectPath(projectCWD))

	entries, err := os.ReadDir(projectDir)
	if err != nil {
		if os.IsNotExist(err) {
			// Project has no Claude Code sessions yet — this is normal.
			return nil, nil
		}
		return nil, fmt.Errorf("jsonl: sessions: read dir %s: %w", projectDir, err)
	}

	var summaries []session.Summary
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".jsonl") {
			continue
		}

		info, err := e.Info()
		if err != nil {
			// File may have been deleted between ReadDir and Info; skip.
			continue
		}

		id := session.ID(strings.TrimSuffix(name, ".jsonl"))
		summaries = append(summaries, session.Summary{
			ID:           id,
			LastActivity: info.ModTime().UTC(),
		})
	}

	// Sort descending by LastActivity (most recently active first).
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].LastActivity.After(summaries[j].LastActivity)
	})

	return summaries, nil
}

// Events returns all events for the given session in strictly ascending
// chronological order (file order, which matches Claude Code's append semantics).
//
// Returns an error if the JSONL file does not exist or contains a malformed line.
// The error message includes the file path and 1-based line number to aid debugging.
//
// Cache: results are memoised per session keyed on (mtime, size). When the
// TUI ticks every second on the Σ aggregate tab, only the session(s) that
// just received an append re-decode; idle sessions return the cached slice
// in microseconds. The cached slice is shared by reference — callers MUST
// NOT mutate it (livesession's pipeline only reads).
func (s *Source) Events(_ context.Context, id session.ID) ([]event.Event, error) {
	s.cacheMu.Lock()
	cached, hasCached := s.cacheLookup(id)
	s.cacheMu.Unlock()

	path := ""
	if hasCached {
		path = cached.path
	}
	if path == "" {
		var err error
		path, err = s.findSessionFile(string(id))
		if err != nil {
			return nil, err
		}
	}

	info, err := os.Stat(path)
	if err != nil {
		// File vanished between cache hit and stat — fall back to fresh
		// lookup. Rare; the JSONL is append-only so this only happens
		// on session deletion.
		path, err = s.findSessionFile(string(id))
		if err != nil {
			return nil, err
		}
		info, err = os.Stat(path)
		if err != nil {
			return nil, err
		}
	}
	mtime := info.ModTime()
	size := info.Size()

	if hasCached && cached.path == path && cached.mtime.Equal(mtime) && cached.size == size {
		return cached.events, nil
	}

	events, err := s.decodeFile(path)
	if err != nil {
		return nil, err
	}

	s.cacheMu.Lock()
	if s.eventCache == nil {
		s.eventCache = make(map[session.ID]eventCacheEntry)
	}
	s.eventCache[id] = eventCacheEntry{
		mtime:  mtime,
		size:   size,
		path:   path,
		events: events,
	}
	s.cacheMu.Unlock()
	return events, nil
}

// cacheLookup returns the cached entry for id, or false. Caller must hold
// cacheMu — split out so the lookup happens while the mutex is held but
// the I/O (stat, decode) runs without contention.
func (s *Source) cacheLookup(id session.ID) (eventCacheEntry, bool) {
	if s.eventCache == nil {
		return eventCacheEntry{}, false
	}
	entry, ok := s.eventCache[id]
	return entry, ok
}

// AllProjectSessions returns refs to all sessions across every project directory
// under s.baseDir, ordered by LastActivity descending (most recently active first).
//
// It walks every subdirectory of baseDir, lists *.jsonl files in each, and
// records their mtime as LastActivity. At most maxResults refs are returned;
// pass maxResults=0 for no cap.
//
// Missing or unreadable directories degrade gracefully: errors on individual
// project dirs are skipped so a single corrupt project does not break the whole
// aggregation.
func (s *Source) AllProjectSessions(_ context.Context, maxResults int) ([]ports.GlobalSessionRef, error) {
	topEntries, err := os.ReadDir(s.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("jsonl: all-project-sessions: read base dir: %w", err)
	}

	var refs []ports.GlobalSessionRef
	for _, projEntry := range topEntries {
		if !projEntry.IsDir() {
			continue
		}
		refs = append(refs, s.collectProjectRefs(projEntry.Name())...)
	}

	sort.Slice(refs, func(i, j int) bool {
		return refs[i].LastActivity.After(refs[j].LastActivity)
	})
	if maxResults > 0 && len(refs) > maxResults {
		refs = refs[:maxResults]
	}
	return refs, nil
}

// collectProjectRefs lists JSONL session files in a single project subdirectory.
// Unreadable directories or files degrade silently to an empty slice.
func (s *Source) collectProjectRefs(projectName string) []ports.GlobalSessionRef {
	projectDir := filepath.Join(s.baseDir, projectName)
	fileEntries, err := os.ReadDir(projectDir)
	if err != nil {
		return nil
	}
	var refs []ports.GlobalSessionRef
	for _, fe := range fileEntries {
		if fe.IsDir() || !strings.HasSuffix(fe.Name(), ".jsonl") {
			continue
		}
		info, err := fe.Info()
		if err != nil {
			continue
		}
		refs = append(refs, ports.GlobalSessionRef{
			ProjectEncodedDir: projectName,
			SessionID:         session.ID(strings.TrimSuffix(fe.Name(), ".jsonl")),
			LastActivity:      info.ModTime().UTC(),
			Path:              filepath.Join(projectDir, fe.Name()),
		})
	}
	return refs
}

// findSessionFile locates the JSONL file for the given session ID by scanning
// all project subdirectories under s.baseDir.
func (s *Source) findSessionFile(sessionID string) (string, error) {
	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("jsonl: events: base directory %s does not exist: %w",
				s.baseDir, os.ErrNotExist)
		}
		return "", fmt.Errorf("jsonl: events: read base dir: %w", err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		candidate := filepath.Join(s.baseDir, e.Name(), sessionID+".jsonl")
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("jsonl: events: session file for %q not found under %s: %w",
		sessionID, s.baseDir, os.ErrNotExist)
}

// decodeFile reads the JSONL file at path and returns the decoded events in file
// order (chronological ascending by Claude Code's append semantics).
//
// # Deduplication
//
// Claude Code JSONL files use a conversation-DAG structure where user edits
// create branching threads.  Every branch replicates the same assistant API
// responses on disk (identical message.id, identical usage). Summing usage
// across all raw lines therefore inflates token counts by 2-18× depending on
// how many branches exist.
//
// Fix: for KindAssistant events we track the Anthropic message ID (message.id
// in the JSON payload).  The first occurrence of each message ID is kept;
// subsequent duplicates are silently dropped.  Non-assistant events are never
// deduplicated — they have unique UUIDs and represent real activity.
func (s *Source) decodeFile(path string) ([]event.Event, error) {
	// G304: path is built internally from the configured Claude projects
	// directory + a directory listing — it never originates from untrusted
	// network input. Reading session files Claude Code writes locally is
	// the entire purpose of this adapter.
	f, err := os.Open(path) //nolint:gosec // see comment
	if err != nil {
		return nil, fmt.Errorf("jsonl: events: open %s: %w", path, err)
	}
	defer f.Close() //nolint:errcheck // read-only; close error is irrelevant

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 64*1024) // initial 64KB backing buffer
	scanner.Buffer(buf, scannerMaxToken)

	var events []event.Event
	// seenMsgIDs tracks which Anthropic message.id values we have already
	// charged for token usage. The first occurrence of each ID gets the
	// real Usage; subsequent occurrences (whether content splits within
	// the same branch or branch duplicates across the conversation DAG)
	// have their Usage zeroed so applyUsageStats doesn't inflate totals.
	//
	// We deliberately KEEP all events — even duplicates — because:
	//   * Content splits: Claude Code's streaming output emits each
	//     content block (thinking / text / tool_use) as its own JSONL
	//     line with a chained parent_uuid. Dropping later blocks loses
	//     real content (tool_use bodies, in particular).
	//   * Branch duplicates: same content replicated across DAG branches.
	//     Keeping the event is harmless because the downstream tool-call
	//     extractor self-dedupes on tool_use_id (livesession.appendToolUseCall).
	//
	// The TUI ends up seeing every content block exactly once via either
	// the unique-id path (content split) or the dedup path (branch dupe).
	seenMsgIDs := make(map[string]struct{})
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue // skip blank lines
		}

		ev, msgID, err := decodeLineWithMsgID(raw)
		if err != nil {
			return nil, fmt.Errorf("jsonl: events: %s line %d: %w", path, lineNum, err)
		}

		if ev.Kind == event.KindAssistant && msgID != "" {
			if _, already := seenMsgIDs[msgID]; already {
				if ap, ok := ev.Payload.(event.AssistantPayload); ok {
					ap.Usage = usage.Zero()
					ev.Payload = ap
				}
			} else {
				seenMsgIDs[msgID] = struct{}{}
			}
		}

		events = append(events, ev)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("jsonl: events: scan %s: %w", path, err)
	}

	return events, nil
}

// envelope holds the fields common to every JSONL event type.
// Variable per-type fields are left in the raw message.
type envelope struct {
	UUID       string          `json:"uuid"`
	Type       string          `json:"type"`
	Timestamp  string          `json:"timestamp"`
	SessionID  string          `json:"sessionId"`
	ParentUUID string          `json:"parentUuid"`
	Message    json.RawMessage `json:"message"`
	// IsMeta is true for system-injected user events (skill prompts, etc.).
	// Only present on user events; defaults to false when absent (ADR-008).
	IsMeta bool `json:"isMeta"`
}

// userMessage holds the fields of the "message" object in a user event.
// Only Content is needed for Summary/IsToolResultOnly derivation.
type userMessage struct {
	Content json.RawMessage `json:"content"`
}

// assistantContent holds the "content" array of an assistant message, used
// for Summary derivation (ADR-010). Decoded separately from assistantMessage
// so the existing usage decode path is unaffected.
type assistantContent struct {
	Content json.RawMessage `json:"content"`
}

// assistantMessage holds the fields of the "message" object in an assistant event.
type assistantMessage struct {
	// ID is the Anthropic API message ID, e.g. "msg_01AbCdEf...".
	// Used by decodeFile to deduplicate branch-replicated events: Claude Code
	// stores the same API response in every conversation branch, so the same
	// message.id can appear 2–18 times in one JSONL file.  We count each unique
	// message ID exactly once to prevent usage inflation.
	ID    string     `json:"id"`
	Model string     `json:"model"`
	Usage tokenUsage `json:"usage"`
}

// tokenUsage mirrors the usage object inside an assistant message.
type tokenUsage struct {
	InputTokens         int64            `json:"input_tokens"`
	OutputTokens        int64            `json:"output_tokens"`
	CacheCreationTokens int64            `json:"cache_creation_input_tokens"`
	CacheReadTokens     int64            `json:"cache_read_input_tokens"`
	CacheCreation       cacheCreationObj `json:"cache_creation"`
}

// cacheCreationObj mirrors the nested cache_creation object in usage.
// It is only present when the model used extended prompt caching.
//
//	"cache_creation": {
//	  "ephemeral_1h_input_tokens": 24944,
//	  "ephemeral_5m_input_tokens": 0
//	}
//
// ephemeral_1h_input_tokens > 0 indicates a Max-plan (1M context) session.
// See event.AssistantPayload.Has1hCache for the full rationale.
type cacheCreationObj struct {
	Ephemeral1hTokens int64 `json:"ephemeral_1h_input_tokens"`
	Ephemeral5mTokens int64 `json:"ephemeral_5m_input_tokens"`
}

// decodeLine parses a single JSONL line into a domain event.Event.
//
// Only `type` is hard-required — it determines how we dispatch the line.
// The remaining envelope fields (uuid, timestamp, sessionId, parentUuid) are
// best-effort: meta lines such as "last-prompt", "ai-title", "permission-mode",
// and "file-history-snapshot" have a different shape and lack these fields.
// Per spec, such lines are preserved as KindOpaque rather than dropped or
// errored — the raw bytes go into OpaquePayload.Raw for future panels.
//
// Unparseable timestamps fall back to time.Time{} (zero). File order is the
// real ordering for events; timestamps are display-only.
// decodeLineWithMsgID is the full decode path that additionally returns the
// Anthropic message ID (message.id field) for assistant events.  The message ID
// is used by decodeFile for deduplication of branch-replicated events.
//
// msgID is empty for non-assistant events and for assistant events that lack a
// message.id field (older JSONL files).
func decodeLineWithMsgID(raw []byte) (ev event.Event, msgID string, err error) {
	// Enforce the reader's line cap here too: the Scanner buffer already
	// bounds lines at scannerMaxToken, so anything larger reaching this
	// decoder is garbage input (or a fuzz-grown blob) — reject it before
	// json.Unmarshal spends real time on it.
	if len(raw) > scannerMaxToken {
		return event.Event{}, "", fmt.Errorf("line too large: %d bytes (max %d)", len(raw), scannerMaxToken)
	}
	var env envelope
	if jsonErr := json.Unmarshal(raw, &env); jsonErr != nil {
		return event.Event{}, "", fmt.Errorf("decode envelope: %w", jsonErr)
	}

	if env.Type == "" {
		return event.Event{}, "", fmt.Errorf("missing required field 'type'")
	}

	// Timestamp: best-effort. Try RFC3339Nano first, then a millisecond-precision
	// fallback. Missing or unparseable → zero time (event still surfaces).
	var ts time.Time
	if env.Timestamp != "" {
		if parsed, parseErr := time.Parse(time.RFC3339Nano, env.Timestamp); parseErr == nil {
			ts = parsed.UTC()
		} else if parsed, parseErr := time.Parse("2006-01-02T15:04:05.000Z", env.Timestamp); parseErr == nil {
			ts = parsed.UTC()
		}
	}

	kind, payload, resolvedMsgID, miss := resolveKindAndPayloadWithMsgID(env.Type, env.Message, raw, env)
	if miss.user {
		shapeMissUser.Add(1)
	}
	if miss.assistant {
		shapeMissAssistant.Add(1)
	}
	if miss.content {
		shapeMissContent.Add(1)
	}
	return event.NewEvent(env.UUID, ts, kind, env.SessionID, env.ParentUUID, payload), resolvedMsgID, nil
}

// shapeMissDelta records which best-effort decodes failed in a single
// call to resolveKindAndPayloadWithMsgID. The decoder remains a free
// function so the fuzz harness can exercise it without constructing a
// Source; counters are folded by the caller.
type shapeMissDelta struct {
	user, assistant, content bool
}

// Package-scoped sinks consumed by Source.ShapeMisses(). The decoder is
// a free function (the fuzz harness depends on that), so we route counter
// increments through these atomics. A future refactor that makes the
// decode chain a method on *Source can drop these in favor of fields on
// the receiver — the public ShapeMisses() shape stays identical.
var (
	shapeMissUser      atomic.Int64
	shapeMissAssistant atomic.Int64
	shapeMissContent   atomic.Int64
)

// resolveKindAndPayload maps the JSONL event type string to a domain Kind and
// constructs the appropriate Payload variant.
//
// Known types:
//   - "user"      → KindUser, UserPayload{IsMeta, IsToolResultOnly, Summary}
//   - "assistant" → KindAssistant, AssistantPayload{Usage, Summary}
//
// All other types → KindOpaque, OpaquePayload{Raw: rawLine}.
// The ENTIRE line is preserved in OpaquePayload.Raw (not just the payload
// sub-field) so future panels can decode any field without the domain changing.
//
// env is the already-decoded envelope (carries IsMeta for user events).
// resolveKindAndPayloadWithMsgID is the full implementation that additionally
// returns the Anthropic message ID for assistant events (used for dedup).
// msgID is empty for non-assistant events and when message.id is absent.
func resolveKindAndPayloadWithMsgID(typeStr string, message json.RawMessage, rawLine []byte, env envelope) (event.Kind, event.Payload, string, shapeMissDelta) {
	var miss shapeMissDelta
	switch typeStr {
	case "user":
		// Decode message.content for Summary / IsToolResultOnly derivation (ADR-009).
		var uMsg userMessage
		if len(message) > 0 {
			if err := json.Unmarshal(message, &uMsg); err != nil {
				miss.user = true
			}
		}
		summary, toolResultOnly := extractUserSummary(uMsg.Content)
		toolResultIDs, toolResultError := extractToolResultInfo(uMsg.Content)
		return event.KindUser, event.UserPayload{
			IsMeta:           env.IsMeta,
			IsToolResultOnly: toolResultOnly,
			Summary:          summary,
			ToolResultIDs:    toolResultIDs,
			ToolResultError:  toolResultError,
		}, "", miss

	case "assistant":
		var msg assistantMessage
		// Best-effort decode of usage. If the message field is absent or
		// malformed, we still return a valid AssistantPayload with zero usage
		// rather than failing the whole parse.
		if len(message) > 0 {
			if err := json.Unmarshal(message, &msg); err != nil {
				miss.assistant = true
			}
		}
		u := usage.Usage{
			Input:         msg.Usage.InputTokens,
			Output:        msg.Usage.OutputTokens,
			CacheRead:     msg.Usage.CacheReadTokens,
			CacheCreation: msg.Usage.CacheCreationTokens,
		}

		// Decode message.content for Summary + tool_use derivation (ADR-010).
		var aContent assistantContent
		if len(message) > 0 {
			if err := json.Unmarshal(message, &aContent); err != nil {
				miss.content = true
			}
		}
		summary := extractAssistantSummary(aContent.Content)
		toolUseID, toolName := extractAssistantToolUse(aContent.Content)

		// Detect 1M context: ephemeral_1h cache is exclusive to Max plan / 1M context window.
		has1hCache := msg.Usage.CacheCreation.Ephemeral1hTokens > 0

		return event.KindAssistant, event.AssistantPayload{
			Usage:      u,
			Model:      msg.Model,
			Summary:    summary,
			ToolUseID:  toolUseID,
			ToolName:   toolName,
			Has1hCache: has1hCache,
		}, msg.ID, miss

	default:
		// Preserve the entire raw line so no information is lost.
		// Make a copy — the scanner reuses its buffer between calls.
		lineCopy := make([]byte, len(rawLine))
		copy(lineCopy, rawLine)
		return event.KindOpaque, event.OpaquePayload{Raw: lineCopy}, "", miss
	}
}

// ─── LLMSource implementation ─────────────────────────────────────────────────

// Name returns the LLM source identifier. Satisfies ports.LLMSource.
func (s *Source) Name() string {
	return "claude-code"
}

// PlanLimits returns nil — Claude Code does not expose plan limits in JSONL files.
// The 5h/7d rolling windows are enforced server-side; clyde displays countdown
// timers but cannot determine the exact token cap from local data.
// Satisfies ports.LLMSource.
func (s *Source) PlanLimits(_ context.Context) *ports.PlanLimits {
	return nil
}
