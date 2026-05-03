# Explore: event-rendering

## Goal

Replace the raw-kind + "(opaque)" event list with a filtered, human-readable feed that shows only `user` and `assistant` events, each with a one-line content snippet extracted from `message.content`.

---

## user message.content shape

`message.content` has two variants:

### Variant A — typed user prompt (string)

```json
{
  "type": "user",
  "isMeta": false,
  "message": {
    "role": "user",
    "content": "why is the project not running?\nvladpb@MacBook-Air scry % npm run dev\nnpm error..."
  }
}
```

Content is a raw `string`. May contain newlines and terminal output. This is the "real" human turn.

### Variant B — typed user prompt (array with text block)

Occasionally the human turn arrives as an array. Observed when interrupted or when the message includes inline context:

```json
{
  "type": "user",
  "isMeta": false,
  "message": {
    "role": "user",
    "content": [
      { "type": "text", "text": "[Request interrupted by user]" }
    ]
  }
}
```

Note: `isMeta: true` user events carry system-injected context (skill prompts, `<local-command-caveat>`, etc.) — not human text. Filter these out the same way as opaques.

### Variant C — tool result return (array with tool_result block)

The most common `user` event — Claude's tool call results being returned:

```json
{
  "type": "user",
  "message": {
    "role": "user",
    "content": [
      {
        "type": "tool_result",
        "tool_use_id": "toolu_01S71JtrHucjs9xCUDGRpWd7",
        "content": "cmd\ndocs\ngo.mod\ngo.sum\ninternal\nLICENSE\nmain.go",
        "is_error": false
      }
    ]
  }
}
```

`content` inside `tool_result` can be:
- **string** — raw text output (e.g. bash stdout, file listing)
- **`[{"type":"text","text":"..."}]`** — wrapped text (MCP tools, engram responses)
- **`[{"type":"tool_reference","tool_name":"..."}]`** — deferred tool stubs (no actual result text; `tool_reference` is a capability pointer, not output)

Multiple `tool_result` blocks in one `user` event are possible (batch tool results).

---

## assistant message.content shape

`message.content` is always an array of blocks. Three block types observed: `text`, `thinking`, `tool_use`.

### Example A — text only (end_turn)

```json
[
  {
    "type": "text",
    "text": "Băi, frate, acum vorbim! This is a fantastic idea..."
  }
]
```

`stop_reason: "end_turn"`. Pure prose response with no tool calls.

### Example B — tool_use only (no text, no thinking)

```json
[
  {
    "type": "tool_use",
    "id": "toolu_017ukhjrGSKrLR1futTA3t9A",
    "name": "mcp__plugin_engram_engram__mem_context",
    "input": { "limit": 15 },
    "caller": { "type": "direct" }
  }
]
```

`stop_reason: "tool_use"`. The assistant calls a tool with no accompanying text. Most `assistant` events in active work sessions are this shape.

### Example C — thinking only (no text, no tool_use)

```json
[
  {
    "type": "thinking",
    "thinking": "",
    "signature": "Er4PClkIDRgCKkD..."
  }
]
```

`thinking` text is always empty string (`""`); the actual thought is encrypted in `signature`. Thinking-only events appear between a tool result and the next tool call in extended thinking mode.

### Example D — mixed (text + tool_use) — rare

```json
[
  { "type": "text", "text": "\n\nThree things: brighter calendar background, fullscreen mode, and auto-refresh. Let me knock them out." },
  { "type": "tool_use", "id": "toolu_...", "name": "Read", "input": { "file_path": "/path/to/file.tsx" }, "caller": { "type": "direct" } }
]
```

Observed only in sessions with complex multi-step responses. Rare but real.

---

## tool_use payload shapes (per tool)

| Tool | Key input fields | Best one-liner field |
|------|-----------------|----------------------|
| `Read` | `file_path`, optional `offset`, `limit` | `file_path` |
| `Edit` | `file_path`, `old_string`, `new_string`, `replace_all` | `file_path` |
| `Write` | `file_path`, `content` | `file_path` |
| `MultiEdit` | `file_path`, `edits: [{old_string,new_string}]` | `file_path` (not seen in sampled sessions; shape from spec) |
| `Bash` | `command`, `description` | `description` if non-empty, else first 60 chars of `command` |
| `Grep` | `pattern`, optional `path`, `output_mode` | `pattern` (+ `path` if present) |
| `Glob` | `pattern` | `pattern` |
| `TodoWrite` | `todos: [{id,content,status,priority}]` | `"update todos"` (not seen in sampled sessions; shape from Claude Code docs) |
| `TaskCreate` | `subject`, `description`, `activeForm` | `subject` |
| `Agent` | `description`, `subagent_type`, `prompt` | `description` |
| `ToolSearch` | `query`, `max_results` | `query` |
| `Skill` | `skill`, optional `args` | `skill` |
| `WebSearch` | `query` | `query` |
| `WebFetch` | `url`, `prompt` | `url` |
| MCP tools (`mcp__*`) | varies by tool | last segment of `name` (after last `__`) |

---

## Snippet extraction strategy

### user events

**Filtering first**: skip events where:
- `isMeta: true` — system-injected context, not human text
- `content` is a list containing only `tool_result` items — these are tool returns, not human prompts. *Display them? Optional — see Open Questions.*

For displayable user events:
- `content` is string → first 80 chars, newlines collapsed to space, trimmed
- `content` is array with `text` block → text of the first `text` item, same treatment
- `content` is array with `tool_result` blocks → show `"← <tool_name>"` if `sourceToolAssistantUUID` resolves, else `"← tool result"` (the `tool_use_id` alone is opaque to the reader)

### assistant events

Priority order (first match wins):

1. Has `tool_use` block → `"<name> <key-arg>"` (see table above).
   - `Read /path/to/file.go`
   - `Edit /path/to/file.go`
   - `Bash 'go test ./...'`
   - `Agent research feasibility`
   - If multiple tool_use blocks, show the first one + `"(+N)"` if N > 1.
2. Has `text` block → first 80 chars, newlines collapsed to space.
3. Has only `thinking` block → `"(thinking)"`.
4. Empty content → `"(empty)"`.

**Rationale**: tool_use is more informative than prose for "what happened in this turn" — the tool name reveals the action immediately. Prose often starts with filler ("Let me...", "I'll...").

---

## Truncation rules

- **Target**: 80 chars (fits 80-column terminals; TUI can trim further based on actual width in a later change).
- **Single-line**: replace all `\n`, `\r`, `\t` with a single space before truncation.
- **Truncation indicator**: append `"…"` when the source exceeds 80 chars (Unicode ellipsis, 1 rune).
- **ANSI**: JSONL content is plain text / markdown. No ANSI escape sequences present. No stripping needed in V1.5.
- **Wide chars / tabs**: basic rune-count truncation is fine for V1.5. Grapheme-cluster awareness deferred.

---

## Filter location

**Recommendation: application layer (`watchsession` use case).**

Reasoning:
- The use case already decides _which_ events to return (`last N`). Deciding _which kinds count_ is the same level of concern — what is "recent meaningful activity."
- Filtering in the use case keeps the TUI adapter purely presentational. The TUI receives only events it knows how to render — no defensive `if opaque { continue }` guards scattered in View().
- Testable without a TUI: a `WatchSession` test can assert `[]event.Event` contains no OpaquePayload items without standing up Bubble Tea.
- Domain-aligned: "only show user/assistant activity" is a business rule, not a rendering detail.

Implementation: `WatchSession` should expose a `WithFilter(func(event.Event) bool)` option or simply hardcode `IsOpaque` exclusion in `Run()`. The filter predicate `event.Kind != KindOpaque` is a one-liner and needs no new interface.

---

## Out-of-scope flags for /sdd-propose

- Decoding other event types (`system`, `attachment`, `progress`, `queue-operation`, etc.) — still opaque, filtered out of view.
- ANSI / color rendering — not in this change.
- Multi-pane layout — not in this change.
- Configurable summary length — hard-code 80; revisit when TUI handles dynamic width.
- Tool result `user` events — filtering is the primary goal; whether to show `← tool result` rows is a UI preference, not a data problem. Default: hide tool_result rows (same as opaques).
- Pagination / scrolling — not in this change.
- Session switching / multiple sessions — not in this change.
- Cost / token display changes — not in this change.

---

## Open questions for /sdd-propose

1. **Show tool_result user events or hide them?**  
   Tool results are 80%+ of `user` events. Showing them creates noise ("← tool result", "← tool result", ...). Hiding them makes the feed feel like a conversation (human prompt → assistant response). Recommendation: hide by default (filter same as opaques). But this is a UX decision the user should confirm.

2. **`isMeta: true` user events — hide or show?**  
   These carry skill prompts and system context. They are not human-typed text. Recommendation: hide (treat as opaque).

3. **`Summary` field name in domain or only in TUI?**  
   The snippet extraction logic could live in: (a) a `Summary() string` method on `event.Event` (domain — keeps TUI thin but couples domain to display concerns), or (b) a `FormatSummary(ev event.Event) string` function in the TUI adapter (adapter — correct hexagonal placement, but not reusable for future panels). Recommendation: (b) adapter-local function, same pattern as existing `formatEventLine`. Revisit when a second consumer appears.

4. **N for filtered events — keep at 5 or increase?**  
   With opaques filtered, 5 visible events may become 5 user/assistant events (more meaningful) or fewer if the session is young. Should N count raw events or filtered events? Recommendation: N counts _filtered_ events — i.e., the use case returns the last 5 non-opaque events. This way the display always shows up to 5 meaningful rows.
