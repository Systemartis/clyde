# Design: event-rendering

## Verified facts

Confirmed by reading source:

- `internal/domain/event/event.go:48` — `UserPayload struct{}` is empty today.
- `internal/domain/event/event.go:54-56` — `AssistantPayload` carries only `Usage usage.Usage`.
- `internal/domain/event/event.go:42-44` — `Payload` is a sealed interface (unexported `isPayload()` marker). Adding fields to existing payload structs does NOT break the seal.
- `internal/domain/event/event.go:30` — `KindOpaque` is the canonical opaque kind; `OpaquePayload.Raw` carries the full JSONL line (`internal/adapters/jsonl/jsonl.go:312-315`).
- `internal/application/watchsession/watchsession.go:75-120` — `WatchSession.Run` already does Sessions → Events → sort → take-last-N. The filter step inserts cleanly between sort and take-last-N.
- `internal/adapters/jsonl/jsonl.go:220-227` — the JSONL `envelope` struct does NOT currently capture `isMeta`. We must add it.
- `internal/adapters/jsonl/jsonl.go:289-317` — `resolveKindAndPayload` is the single dispatch point; both `Summary` extraction and `IsMeta` / `IsToolResultOnly` decoding land here.
- `internal/adapters/tui/model.go:164-178` — `formatEventLine` is the only place that handles per-payload rendering. The default branch emits `(opaque)`. Removing it is a one-line change.
- `internal/adapters/tui/model.go:114-149` — `View()` already routes through `formatEventLine` for each event; no opaque-aware branching at the View level beyond `formatEventLine`.
- `internal/adapters/tui/model_test.go:81-96` — current TUI test fixture (`threeEventView`) constructs `UserPayload{}` and `AssistantPayload{Usage: ...}` with positional zero-value Summary acceptable.
- `.golangci.yml:36-59` — depguard `domain-pure` rule denies UI/IO imports for domain. Stdlib (`strings`, `unicode/utf8`) is NOT denied — pure helpers are allowed.
- `internal/domain/usage/usage.go:11-16` — Usage is exported value object — confirms the convention of plain exported structs in domain.

---

## ADRs

### ADR-007: Filter at application/watchsession use case

**Decision.** The filter lives in `WatchSession.Run`, executed AFTER the chronological sort and BEFORE the last-N truncation. N counts visible events (locked by spec scenario "N counts visible events only").

**Implementation.** A package-private predicate `isVisible(ev event.Event) bool` returning false for:
1. `ev.Kind == event.KindOpaque`
2. `UserPayload` where `IsMeta == true`
3. `UserPayload` where `IsToolResultOnly == true`

```go
// inside Run, between sort and last-N:
filtered := evts[:0]
for _, ev := range evts {
    if isVisible(ev) {
        filtered = append(filtered, ev)
    }
}
evts = filtered
```

**Rationale.** The use case already encodes "what counts as recent meaningful activity" (last-N selection). What counts as VISIBLE activity is the same level of concern. Keeping it here:
- Tests assert filter rules without standing up Bubble Tea.
- TUI stays purely presentational — no defensive `if opaque { continue }` scattered in `View()`.
- Future TUI changes (toggle to show tool results) become a parameter to the use case, not a flag through three layers.

**Consequences.** `WatchSession` gains one private function and one extra loop. The N count semantics shift from "last N raw" to "last N visible" — **callers that depended on the old behavior do not exist** (TUI is the only consumer). Existing tests that constructed sessions of pure `KindUser`/`KindAssistant` events keep passing because none of those events match a hide rule (zero-value `IsMeta` and `IsToolResultOnly` are both false).

**Rejected alternative.** "Filter inside the JSONL adapter (drop lines)." Rejected — bypasses the domain. Future changes (logs panel, web UI, a "show everything" toggle) would need to re-read the file. Filtering at the use case keeps the data pipeline untouched.

---

### ADR-008: `IsMeta` as explicit bool on `UserPayload`

**Decision.** Option A from the prompt: add `IsMeta bool` to `UserPayload`. The JSONL adapter populates it from the top-level `isMeta` JSONL field.

**Rationale.**
- **Explicit beats implicit.** A reader of the domain sees the meta concept named; no map lookup, no string sniffing.
- **Narrow blast radius.** Only `UserPayload` carries the flag. Assistant events do not have `isMeta` in JSONL. Adding a generic `Meta map[string]any` (Option B) opens the door to undisciplined ad-hoc fields.
- **Easy to remove.** When the filter rule eventually evolves into a toggle, `IsMeta` stays as data; only the predicate flips.
- **Test ergonomics.** Constructing a meta user event in a test is `UserPayload{IsMeta: true}` — readable and self-documenting.

**Consequences.** Existing tests that build `UserPayload{}` keep compiling — zero value is `false`, indistinguishable from a non-meta event today. New JSONL fixtures must carry `"isMeta":true` for the meta case.

**Rejected alternatives.**
- **Option B (generic `Meta map[string]any` on Event envelope):** rejected. Slippery slope — invites adding everything as untyped data and erodes the domain.
- **Option C (drop meta lines at the adapter, never enter the domain):** rejected. Loses raw-on-disk recoverability principle. Same rationale as ADR-007.

---

### ADR-009: tool_result-only detection — flag on payload, not drop at adapter

**Decision.** The JSONL adapter inspects `message.content`. If it is an array containing exclusively `tool_result` blocks (zero `text` blocks, zero string variant), set `UserPayload.IsToolResultOnly = true`. The use case filter consults the flag.

**Rationale.**
- **Consistency with ADR-008.** Same shape: domain carries the fact, application layer decides what to do.
- **Future toggle is free.** When V1.6 adds "show tool results", the data is already on every event — the toggle flips the predicate.
- **Adapter stays a translator.** Decoding semantics live in the adapter; visibility policy lives in the use case. Don't conflate.
- **Empty-array safety.** A user event with `content: []` (degenerate) sets `IsToolResultOnly = false` — there are no tool_result blocks at all. The event is treated as a typed user prompt with empty `Summary`. Not pretty, but observable. We do NOT invent a third "empty user" rule for this change.

**Detection algorithm (adapter):**

```
content = decode message.content
if content is string:
    IsToolResultOnly = false
    Summary = truncate(string)
else if content is array:
    if len(array) == 0:
        IsToolResultOnly = false
        Summary = ""
    else if every block has type "tool_result":
        IsToolResultOnly = true
        Summary = ""
    else:
        IsToolResultOnly = false
        Summary = truncate(first text block's text, if any)
```

**Consequences.** The JSONL adapter must look one level deeper into `message.content` for user events. Adds ~30 lines of decode logic, all unit-testable in `jsonl/events_test.go`.

---

### ADR-010: Summary extraction in the JSONL adapter

**Decision.** Three private helpers in `internal/adapters/jsonl/`:

```go
// extractUserSummary returns the displayable text for a user event's
// content. content may be a JSON string or an array of blocks. Returns
// summary text (already truncated) and whether the event is tool_result-only.
func extractUserSummary(content json.RawMessage) (summary string, toolResultOnly bool)

// extractAssistantSummary returns the priority-chain summary for an
// assistant event's content array (tool_use > text > thinking > "").
func extractAssistantSummary(content json.RawMessage) string

// extractToolSummary formats a single tool_use block per the locked
// per-tool table.
func extractToolSummary(name string, input json.RawMessage) string
```

**Why adapter, not domain.** Summary derivation is JSON-shape-aware: it parses raw bytes, dispatches on `type` field, and reaches into per-tool input fields. That is ENCODING knowledge — adapter territory. The domain stores the result (`Summary string`); the domain does not know JSON exists. Honors hexagonal: adapter speaks JSON, domain speaks types.

**Why not a port.** A port for "summarize a content blob" would be a port with one implementer (the JSONL adapter) and one consumer (the JSONL adapter calling itself). That is ceremony. The summary derivation is decode-time pure logic; encapsulating it as a port adds an interface for no testability gain (it's already tested in the adapter package).

**Consequences.** Three new private functions, three sets of unit tests in `jsonl/events_test.go`. The JSONL adapter grows from ~318 lines to ~450 lines (estimate). Acceptable.

---

### ADR-011: Truncation helper in domain

**Decision.** Add `internal/domain/event/format.go` containing:

```go
// Truncate normalizes whitespace and limits s to maxRunes runes.
// Newlines (\n, \r, \r\n) and tabs become a single space.
// Consecutive whitespace collapses to a single space.
// If the rune count exceeds maxRunes, the result is the first
// (maxRunes-1) runes followed by "…" (U+2026).
func Truncate(s string, maxRunes int) string
```

**Why domain.** Truncation IS the Summary contract. The spec dictates the rune count, the ellipsis character, the whitespace rules. Those rules belong with the type that holds Summary — `event.UserPayload` and `event.AssistantPayload`. Adapters use the domain helper to produce values that match the contract. Future consumers (logs, web UI) reach for the same helper.

**Stdlib-only check (depguard).** The implementation uses `strings` (whitespace handling) and `unicode/utf8` (rune iteration). Neither is denied by `domain-pure` (`.golangci.yml:39-59` denies UI and I/O packages — stdlib pure helpers pass). Confirmed safe.

**Why exported, not lowercase `truncate`.** Exposing it lets tests in any package verify its contract without reimplementation. It is also a candidate for reuse when V1.6 adds a "show full text" expanded view that re-truncates at terminal width.

**Consequences.** One new file in domain, ~30 lines including doc comment. One new test file with three test cases (rune count, newline collapse, ellipsis).

**Rejected alternative.** Inline the helper inside `jsonl/events_test.go` and unexport. Rejected — duplicates the truncation contract across packages and risks drift when the rule evolves.

---

### ADR-012: TUI single-row rendering with usage appended

**Decision.** Each event renders as a single line:

```
HH:MM:SSZ  user        <Summary>
HH:MM:SSZ  assistant   <Summary>  in=N out=M
```

`(opaque)` branch is deleted from `formatEventLine`. The default switch arm becomes unreachable in practice (filter rule 1 guarantees no opaque events arrive); it is replaced with a defensive fallback that prints `<kind>  <empty>` rather than panicking — invariant-preserving but invisible to passing tests.

**Rationale.**
- **Single-line is scannable.** Multi-line per event explodes vertical density, useless for a 24-row terminal.
- **Usage at end of assistant row.** Tokens are a secondary signal; appending preserves the alignment of the kind column. Empty user rows leave usage absent.
- **Empty state.** When `len(Events) == 0`, the View renders `"No active conversation."` (ASCII, dimmed via `lipgloss.NewStyle().Faint(true)` if lipgloss is added — otherwise plain text). Centered horizontally if `m.width > 0`.

**Lipgloss usage.** Currently `formatEventLine` returns plain `fmt.Sprintf` strings — no lipgloss. This change CAN keep it that way and ship; lipgloss styling is OPTIONAL polish. Recommendation: hold lipgloss for a follow-up change (visual styling is a separate concern; introducing lipgloss now expands the diff and the golden-file regen scope).

**Consequences.** `formatEventLine` becomes simpler (no opaque branch), gains the Summary field. The `testdata/snapshot.golden` and `testdata/view.golden` files regenerate. Existing TUI tests keep their fixtures but expect the new format; the `threeEventView` fixture's opaque event is replaced with a second user/assistant event because opaque events would now never reach the View (or kept as-is and verified that View() filters defensively — see test mapping).

**Rejected alternative.** "Two rows per assistant: one for summary, one for tokens." Rejected for vertical density. A future "expanded view" mode (toggle 'e' to expand selected event) is the right home for multi-line.

---

## Updated entity shapes

```go
// internal/domain/event/event.go (additions only — full file in apply)

// UserPayload is the payload for KindUser events.
type UserPayload struct {
    // IsMeta is true for system-injected user events (skill prompts,
    // <local-command-caveat>, etc.). Set from the JSONL "isMeta" field.
    IsMeta bool

    // IsToolResultOnly is true when the user event's message.content is
    // an array containing exclusively tool_result blocks. Set during
    // JSONL decode by inspecting the content array.
    IsToolResultOnly bool

    // Summary is the truncated, single-line text of the typed user prompt.
    // Empty when IsMeta or IsToolResultOnly is true. Bounded by Truncate().
    Summary string
}

// AssistantPayload is the payload for KindAssistant events.
type AssistantPayload struct {
    // Usage carries the token counts reported by the Claude API.
    Usage usage.Usage

    // Summary is derived by the priority chain in design.md ADR-010:
    // tool_use > text > "(thinking)" > "". Bounded by Truncate().
    Summary string
}
```

`OpaquePayload` is unchanged. `NewEvent` constructor is unchanged (still accepts `Payload` interface).

---

## Truncation contract

`event.Truncate(s string, maxRunes int) string` rules (locked):

1. Replace `\r\n` with single space (do this before single `\r` and `\n`).
2. Replace each remaining `\r`, `\n`, `\t` with single space.
3. Collapse runs of consecutive whitespace (space, tab) to a single space using a single pass.
4. Trim leading and trailing space (no leading/trailing whitespace artifacts).
5. Count runes via `utf8.RuneCountInString`.
6. If rune count ≤ maxRunes → return as-is.
7. Else → take first `(maxRunes - 1)` runes (rune-boundary slicing via range loop or `utf8.DecodeRuneInString`) and append `"…"` (U+2026, single rune).

**Boundary cases:**
- `maxRunes ≤ 1`: return `"…"` (degenerate but defined).
- `s == ""`: return `""`.
- All-whitespace `s`: returns `""` after trim.

The Summary fields are bounded by `Truncate(text, 80)`. The `Bash` rule is special — it pre-truncates the command at 60 RUNES before wrapping in single quotes (see per-tool table).

---

## Per-tool summary table (canonical)

Reproduced from spec for executor convenience. Lookup is case-sensitive on the tool `name`.

| Tool name(s) | Summary format | Notes |
|---|---|---|
| `Read`, `Write`, `Edit`, `MultiEdit` | `Tool: <Name> <file_path>` | Read `input.file_path` as string. Empty path → `Tool: <Name>`. |
| `Bash` | `Tool: Bash <description>` if `input.description` non-empty; else `Tool: Bash '<command[:60]>'` | Inner command is truncated at 60 RUNES (not 80) before quoting. |
| `Grep`, `rg` | `Tool: Grep <pattern>` | Both names produce the `Grep` label. |
| `Glob` | `Tool: Glob <pattern>` | |
| `Task`, `Agent` | `Tool: Task <description>` | Both names produce the `Task` label. |
| `TodoWrite` | `Tool: TodoWrite (<n> items)` | `n` = `len(input.todos)`. Zero → `(0 items)`. |
| Default (any other) | `Tool: <Name>` | MCP tools `mcp__*` and unknown tools land here. |

After per-tool formatting, the result is passed through `Truncate(s, 80)` to enforce the global rune cap. Per-tool truncation is INDEPENDENT of `Truncate` — the Bash 60-rune cap exists to protect the closing quote within the 80-rune envelope.

---

## Test mapping

Spec scenario → test function (apply phase will implement; design names them so tasks can reference).

### Application layer — `internal/application/watchsession/watchsession_test.go`

| Spec scenario | Test name |
|---|---|
| Opaque events filtered from view | `TestWatchSessionRun_FiltersOpaqueEvents` |
| Meta user events filtered from view | `TestWatchSessionRun_FiltersMetaUserEvents` |
| Tool-result user events filtered from view | `TestWatchSessionRun_FiltersToolResultOnlyEvents` |
| Typed user prompt visible | `TestWatchSessionRun_TypedPromptVisibleWithSummary` |
| N counts visible events only | `TestWatchSessionRun_NCountsVisibleOnly` |
| Chronological order preserved post-filter | covered inside `TestWatchSessionRun_FiltersOpaqueEvents` |

### Domain layer — `internal/domain/event/format_test.go` (NEW)

| Spec rule | Test name |
|---|---|
| Rune-count limit | `TestTruncate_RuneCountLimit` |
| Ellipsis on overflow | `TestTruncate_AppendsEllipsisOnOverflow` |
| Newline collapse | `TestTruncate_CollapsesNewlines` |
| Whitespace normalisation | `TestTruncate_CollapsesConsecutiveWhitespace` |
| Truncation at multibyte rune boundary | `TestTruncate_RuneBoundaryMultibyte` |
| Boundary: empty / whitespace-only / tiny max | `TestTruncate_DegenerateCases` |

### Adapter layer — `internal/adapters/jsonl/events_test.go`

| Spec scenario | Test name |
|---|---|
| Typed user prompt summary (string) | `TestDecodeLine_UserSummaryFromString` |
| Typed user prompt summary (text block) | `TestDecodeLine_UserSummaryFromTextBlock` |
| User isMeta flag set | `TestDecodeLine_UserIsMetaFlag` |
| User tool_result-only flag set | `TestDecodeLine_UserIsToolResultOnly` |
| User mixed text + tool_result (NOT tool-result-only) | `TestDecodeLine_UserMixedNotToolResultOnly` |
| Assistant text response visible | `TestDecodeLine_AssistantTextSummary` |
| Assistant tool_use response renders tool summary | `TestDecodeLine_AssistantToolUseSummary` (table) |
| Assistant with multiple tool_use uses first | covered by table case `multiple_tool_use_first_wins` |
| Assistant thinking-only event | `TestDecodeLine_AssistantThinkingOnly` |
| Assistant mixed text + tool_use, tool wins | covered by table case `text_and_tool_use_tool_wins` |
| Per-tool — Read/Write/Edit/MultiEdit | `TestExtractToolSummary` (table) |
| Per-tool — Bash with description | table case |
| Per-tool — Bash command-only ≤60 runes | table case |
| Per-tool — Bash command-only >60 runes (truncated) | table case |
| Per-tool — Grep / rg / Glob | table cases |
| Per-tool — Task / Agent | table cases |
| Per-tool — TodoWrite n items | table case |
| Per-tool — unknown tool | table case `unknown_tool_name_only` |
| Per-tool — MCP tool (mcp__*) | table case `mcp_default_branch` |

### TUI layer — `internal/adapters/tui/model_test.go`

| Spec scenario | Test name |
|---|---|
| Focused pane shows snippets, not opaque markers | `TestTUIView_NoOpaqueMarkers` (new; assert `(opaque)` substring absent) |
| Row format | golden update — `TestTUIViewGolden` regenerated; new fixture covers user + assistant with summaries |
| Empty Session | `TestTUIView_EmptyConversation` (renames/extends current `TestTUIViewEmptyState`) |

### Test fixtures (JSONL) — `internal/adapters/jsonl/testdata/`

New `*.jsonl` fixtures created (do NOT modify existing ones — bootstrap-skeleton fixtures stay deterministic for their own tests):
- `event-rendering-user-typed.jsonl`
- `event-rendering-user-meta.jsonl`
- `event-rendering-user-tool-result.jsonl`
- `event-rendering-assistant-text.jsonl`
- `event-rendering-assistant-tool-use.jsonl`
- `event-rendering-assistant-thinking.jsonl`
- `event-rendering-assistant-mixed.jsonl`

---

## Backward compatibility

**Source-level.** Adding fields to `UserPayload` and `AssistantPayload` is additive. Code that constructs `UserPayload{}` or `AssistantPayload{Usage: u}` (NAMED-field construction — confirmed in `model_test.go:85-89`) keeps compiling; new fields default to zero values.

**Struct comparability.** Both updated payloads remain comparable structs (no slice/map/func fields added — `IsMeta bool`, `IsToolResultOnly bool`, `Summary string` are all comparable). Tests that use `==` on payloads keep working.

**Existing tests that may need updates:**
- `internal/adapters/tui/model_test.go:81-96` — `threeEventView` opaque event will be filtered out by the use case in production. The TUI test stubs `usecase.Run` directly with a SessionView — bypasses the filter — so the existing snapshot test still receives the opaque event. **Decision:** update the fixture in `model_test.go` to drop the opaque event and add a typed user/assistant pair with `Summary` populated. This represents the post-filter shape the TUI sees in production. The golden files regenerate.
- `internal/application/watchsession/watchsession_test.go` — existing tests construct user/assistant events without `IsMeta` or `Summary` — they pass through the filter as visible events. No breakage. New tests (above) extend coverage.
- `internal/adapters/jsonl/events_test.go` — existing decode tests assert payload shapes via `==` or field access. They will need updates IF they assert payload equality on user/assistant events, since the new fields appear in the actual output. **Mitigation:** review each existing assertion; update to match new shape (Summary populated for typed prompts, zero for synthetic fixtures lacking content).

**Golden file regen.** `testdata/snapshot.golden` and `testdata/view.golden` MUST regenerate via `go test -update ./internal/adapters/tui/...`. This is expected and the test scaffold already supports it.

---

## Hexagonal "don't wrap" list

Explicitly NOT introducing in this change:

- **No port for summary extraction.** Pure decode logic; lives in adapter. (See ADR-010.)
- **No port for the visibility filter.** Use-case-internal predicate; not a stable contract worth interfacing.
- **No port for truncation.** Pure stdlib transform; lives in domain as a free function.
- **No new adapter package.** All work fits in the existing `jsonl`, `watchsession`, and `tui` packages.
- **No event Builder / fluent API.** Plain struct literal construction is sufficient.
- **No JSON-shape types in the domain.** `json.RawMessage` stays at the adapter boundary; domain only sees the decoded `Summary`/`IsMeta`/`IsToolResultOnly`.
- **No lipgloss in this change.** Visual polish is a follow-up; this change is correctness + content.

---

## Risks for /sdd-tasks and /sdd-apply

| Risk | Likelihood | Mitigation |
|---|---|---|
| `isMeta` field is absent from many existing JSONL fixtures | High | Default false. Adapter uses `*bool` decode or omitempty pattern: `var env struct { IsMeta bool `json:"isMeta"` }` — Go's `encoding/json` defaults missing booleans to false. Document the default in `decodeLine`. |
| `tool_result-only` detection trips on empty content arrays | Med | Algorithm in ADR-009 handles `len == 0` → false. Add explicit unit test `TestExtractUserSummary_EmptyContentArray`. |
| Existing JSONL test fixtures lack `Summary` content; assertions break | Med | DO NOT modify existing fixtures (deterministic for their own tests). ADD new fixtures for new tests. Review existing tests and update assertions only where they collide with new fields. |
| Golden files become stale | High (expected) | Run `go test -update ./internal/adapters/tui/...` once after the apply batch lands. Review the diff in PR. |
| Strict TDD ordering for domain `Truncate` | Med | Apply order: write `format_test.go` first (red); implement `format.go` (green); refactor. Same for jsonl extractors. |
| Domain test imports — verify `event_test.go` is `package event_test` or `package event` and adapt | Low | Currently `event_test.go` exists; add `format_test.go` as `package event_test` to mirror existing convention. |
| `Bash` 60-rune inner truncation interacts unexpectedly with outer 80-rune `Truncate` | Low | Inner truncation produces at most ~70 runes (`Tool: Bash '...60...'`). Outer `Truncate(80)` is a no-op for that case. Document with a unit test pinning the exact output for a 100-rune command. |
| `mcp__*` tool names hit default branch and emit long names | Low | Default branch is `Tool: <Name>` then outer `Truncate(80)` clips with `…`. Acceptable. Tested by table case `mcp_default_branch`. |
| `extractAssistantSummary` complexity exceeds gocyclo 15 | Med | Split into per-block-type private helpers (`firstToolUse`, `firstText`, `hasThinking`). Each <10 cyclomatic. |
| Filter changes N semantics; downstream display shows fewer events than before for noisy sessions | Low | This is the desired V1.5 outcome. Documented in spec scenario "N counts visible events only". |
