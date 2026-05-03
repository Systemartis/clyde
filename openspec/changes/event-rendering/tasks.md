# Tasks: event-rendering

## Strategy

Strict TDD throughout. Every domain, application, and adapter layer task pairs a failing test before the implementation — red first, green after, then commit. TUI golden files are an exception: regenerated via `-update` flag once the implementation lands. The batch boundaries below group work so each batch compiles and all tests pass before moving to the next.

Note one pre-existing conflict: `watchsession_test.go:opaque_kind_preserved` asserts opaque events pass through Run() — this contradicts the new filter rule. That test must be updated in Phase 4 (mark it superseded, replace with `TestWatchSessionRun_FiltersOpaqueEvents`). All other existing tests survive unchanged; zero-value `IsMeta` / `IsToolResultOnly` / `Summary` means typed user and assistant events remain visible.

---

## Phase 1: Domain — Truncate helper

### 1.1 Failing tests for Truncate (new file: `internal/domain/event/format_test.go`)

- [ ] Create `internal/domain/event/format_test.go` as `package event_test`.
- [ ] Write `TestTruncate_RuneCountLimit`: input ≤ 80 runes → returned as-is (no ellipsis). Maps to spec rule "Rune-count limit".
- [ ] Write `TestTruncate_AppendsEllipsisOnOverflow`: 120-rune ASCII input → exactly 78 runes, last rune is `…`. Maps to spec scenario "Truncation at rune boundary".
- [ ] Write `TestTruncate_CollapsesNewlines`: input with `\n`, `\r\n`, `\r` → all replaced with space before truncation. Maps to spec rule "Newline collapse".
- [ ] Write `TestTruncate_CollapsesConsecutiveWhitespace`: runs of spaces/tabs collapse to single space. Maps to spec rule "Whitespace normalisation".
- [ ] Write `TestTruncate_RuneBoundaryMultibyte`: input of multibyte UTF-8 runes (e.g. Japanese chars) exceeding 80 runes → truncated on rune boundary, ends with `…`. Maps to spec scenario "Truncation at multibyte rune boundary".
- [ ] Write `TestTruncate_DegenerateCases`: table covers empty string, all-whitespace string, `maxRunes=1`, `maxRunes=0`.
- [ ] Run `go test ./internal/domain/event/...` — confirm all new tests FAIL (file not found / compile error is the expected red state).
- **Acceptance**: six test functions present and failing (package compiles once `format.go` stub is added with correct signature).

### 1.2 Implement Truncate (`internal/domain/event/format.go`)

- [ ] Create `internal/domain/event/format.go` as `package event`.
- [ ] Implement `Truncate(s string, maxRunes int) string` following the seven-step contract in design.md ADR-011: `\r\n` → space, `\r`/`\n`/`\t` → space, collapse consecutive whitespace, trim, rune-count check, first `(maxRunes-1)` runes + `…` on overflow.
- [ ] Import only `strings` and `unicode/utf8` (depguard `domain-pure` allows these).
- [ ] Run `go test ./internal/domain/event/...` — all Phase 1 tests PASS.
- **Acceptance**: `TestTruncate_*` all green; `go vet ./internal/domain/event/...` clean.

---

## Phase 2: Domain — Payload field additions

### 2.1 Add `IsMeta`, `IsToolResultOnly`, `Summary` to `UserPayload`

- [ ] Edit `internal/domain/event/event.go`: add `IsMeta bool`, `IsToolResultOnly bool`, `Summary string` to `UserPayload` struct.
- [ ] Confirm named-field construction `UserPayload{}` in all existing test files still compiles (zero values); no call-site changes needed.
- [ ] Run `go test ./...` — no regressions.
- **Acceptance**: package compiles; existing tests still pass; new fields appear in godoc.

### 2.2 Add `Summary` to `AssistantPayload`

- [ ] Edit `internal/domain/event/event.go`: add `Summary string` to `AssistantPayload` struct.
- [ ] Confirm `AssistantPayload{Usage: u}` construction in `model_test.go` and `jsonl_test.go` still compiles.
- [ ] Run `go test ./...` — no regressions.
- **Acceptance**: package compiles; existing tests still pass.

---

## Phase 3: Adapter — JSONL Summary extraction (TDD)

### 3.1 Failing tests — `extractUserSummary` (`internal/adapters/jsonl/events_test.go`)

- [ ] Add the following test functions to `events_test.go` (package `jsonl_test`):
  - `TestDecodeLine_UserSummaryFromString`: JSONL user event with `message.content` as a JSON string → `UserPayload.Summary` equals truncated string; `IsToolResultOnly = false`. Maps to spec "Typed user prompt visible".
  - `TestDecodeLine_UserSummaryFromTextBlock`: content is array with single `{"type":"text","text":"Hello"}` block → `Summary = "Hello"`.
  - `TestDecodeLine_UserIsMetaFlag`: JSONL line with top-level `"isMeta":true` → `UserPayload.IsMeta = true`. Maps to spec "Meta user events filtered from view".
  - `TestDecodeLine_UserIsToolResultOnly`: content array of exclusively `tool_result` blocks → `IsToolResultOnly = true`, `Summary = ""`. Maps to spec "Tool-result user events filtered from view".
  - `TestDecodeLine_UserMixedNotToolResultOnly`: content array with one text block + one tool_result block → `IsToolResultOnly = false`, `Summary` non-empty.
  - `TestExtractUserSummary_EmptyContentArray`: content is empty array `[]` → `IsToolResultOnly = false`, `Summary = ""` (ADR-009 boundary case).
- [ ] Run `go test ./internal/adapters/jsonl/...` — confirm new tests FAIL.
- **Acceptance**: tests compile and fail before implementation.

### 3.2 Implement `extractUserSummary` in `internal/adapters/jsonl/jsonl.go`

- [ ] Add `isMeta` field to the `envelope` struct: `IsMeta bool \`json:"isMeta"\``.
- [ ] Add `userMessage` struct with `Content json.RawMessage \`json:"content"\``.
- [ ] Implement `extractUserSummary(content json.RawMessage) (summary string, toolResultOnly bool)` following the ADR-009 algorithm: string → `Truncate(s, 80)`; empty array → `(false, "")`; all-`tool_result` array → `(true, "")`; otherwise → `Truncate(first-text-block, 80)`.
- [ ] Update `resolveKindAndPayload` "user" case to decode `message` into `userMessage`, call `extractUserSummary`, read `env.IsMeta`, populate `UserPayload{IsMeta, IsToolResultOnly, Summary}`.
- [ ] Run `go test ./internal/adapters/jsonl/...` — 3.1 tests PASS.
- **Acceptance**: all Phase 3.1 test functions green.

### 3.3 Failing tests — `extractAssistantSummary` (table-driven)

- [ ] Add to `events_test.go`:
  - `TestDecodeLine_AssistantTextSummary`: single `text` block → `AssistantPayload.Summary` equals text (≤ 80 runes). Maps to spec "Assistant text response visible".
  - `TestDecodeLine_AssistantToolUseSummary`: table-driven, covering:
    - `single_tool_use_read`: `tool_use` name=`Read`, `input.file_path="/some/file.go"` → `"Tool: Read /some/file.go"`. Maps to spec "Assistant tool_use response renders tool summary".
    - `multiple_tool_use_first_wins`: two tool_use blocks, Edit first → summary from Edit. Maps to spec "Assistant with multiple tool_use uses first".
    - `text_and_tool_use_tool_wins`: text + tool_use → summary from tool_use. Maps to spec "Assistant mixed text and tool_use".
  - `TestDecodeLine_AssistantThinkingOnly`: only `thinking` block → `Summary = "(thinking)"`. Maps to spec "Assistant thinking-only event".
- [ ] Run `go test ./internal/adapters/jsonl/...` — new tests FAIL.
- **Acceptance**: tests compile and fail.

### 3.4 Implement `extractAssistantSummary`

- [ ] Implement `extractAssistantSummary(content json.RawMessage) string` with priority chain: scan for first `tool_use` block (if found call `extractToolSummary`); else first `text` block (Truncate); else if any `thinking` block return `"(thinking)"`; else return `""`.
- [ ] Split into private helpers (`firstToolUseBlock`, `firstTextBlock`, `hasThinkingBlock`) to stay below gocyclo 15.
- [ ] Update `resolveKindAndPayload` "assistant" case to also decode `message.Content` and populate `AssistantPayload.Summary`.
- [ ] Run `go test ./internal/adapters/jsonl/...` — 3.3 tests PASS.
- **Acceptance**: all Phase 3.3 tests green.

### 3.5 Failing tests — `extractToolSummary` (full table)

- [ ] Add `TestExtractToolSummary` table to `events_test.go` covering every entry from the locked per-tool table:
  - `read_file_path`: `Read` + `input.file_path="/a/b.go"` → `"Tool: Read /a/b.go"`. Maps to spec per-tool table row.
  - `write_file_path`: same pattern for `Write`.
  - `edit_file_path`: same for `Edit`.
  - `multiedit_file_path`: same for `MultiEdit`.
  - `bash_with_description`: `Bash` + `input.description="Run tests"` → `"Tool: Bash Run tests"`. Maps to spec "Per-tool summary — Bash with description".
  - `bash_command_short`: `Bash` + no description + `input.command="go test ./..."` → `"Tool: Bash 'go test ./...'"`. Maps to spec "Per-tool summary — Bash without description".
  - `bash_command_long_truncated`: `Bash` + command of 100 runes → inner truncated at 60 runes, result ≤ 80 runes total (outer Truncate no-op). Maps to spec "Per-tool summary — Bash without description" with overflow.
  - `grep_pattern`: `Grep` → `"Tool: Grep <pattern>"`.
  - `rg_maps_to_grep_label`: `rg` → `"Tool: Grep <pattern>"`.
  - `glob_pattern`: `Glob` → `"Tool: Glob <pattern>"`.
  - `task_description`: `Task` → `"Tool: Task <description>"`.
  - `agent_maps_to_task_label`: `Agent` → `"Tool: Task <description>"`.
  - `todowrite_n_items`: `TodoWrite` + `input.todos` of 3 items → `"Tool: TodoWrite (3 items)"`.
  - `todowrite_zero_items`: `TodoWrite` + empty todos → `"Tool: TodoWrite (0 items)"`.
  - `unknown_tool_name_only`: `ImaginaryTool` → `"Tool: ImaginaryTool"`. Maps to spec "Per-tool summary — unknown tool".
  - `mcp_default_branch`: `mcp__server__action` → `"Tool: mcp__server__action"` (outer Truncate clips if >80 runes).
- [ ] Run `go test ./internal/adapters/jsonl/...` — new test cases FAIL.
- **Acceptance**: table compiled and failing.

### 3.6 Implement `extractToolSummary`

- [ ] Implement `extractToolSummary(name string, input json.RawMessage) string` with case switch on `name` per the locked table.
- [ ] Bash special case: decode `input.description` and `input.command`; if description non-empty use it; else Truncate command at 60 runes, wrap in single quotes.
- [ ] TodoWrite: decode `input.todos` as `[]json.RawMessage`, use `len()`.
- [ ] Apply outer `event.Truncate(result, 80)` to the final string.
- [ ] Run `go test ./internal/adapters/jsonl/...` — 3.5 table PASSES.
- **Acceptance**: all Phase 3.5 table cases green; `go vet` clean.

### 3.7 Add JSONL test fixtures

- [ ] Create `internal/adapters/jsonl/testdata/event-rendering-user-typed.jsonl` (user event with string content "Hello").
- [ ] Create `internal/adapters/jsonl/testdata/event-rendering-user-meta.jsonl` (user event with `"isMeta":true`).
- [ ] Create `internal/adapters/jsonl/testdata/event-rendering-user-tool-result.jsonl` (user event with tool_result-only content array).
- [ ] Create `internal/adapters/jsonl/testdata/event-rendering-assistant-text.jsonl` (assistant event with single text block "Let me check the file for you.").
- [ ] Create `internal/adapters/jsonl/testdata/event-rendering-assistant-tool-use.jsonl` (assistant event with `tool_use` Read block).
- [ ] Create `internal/adapters/jsonl/testdata/event-rendering-assistant-thinking.jsonl` (assistant event with only a thinking block).
- [ ] Create `internal/adapters/jsonl/testdata/event-rendering-assistant-mixed.jsonl` (assistant with text + tool_use blocks).
- [ ] Do NOT modify existing fixtures (deterministic for their own tests).
- **Acceptance**: fixtures exist on disk; existing tests still pass.

---

## Phase 4: Application — WatchSession filter (TDD)

### 4.1 Failing tests for filter rules (`internal/application/watchsession/watchsession_test.go`)

- [ ] Add the following test functions (parallel subtests under `TestWatchSessionFilter`):
  - `TestWatchSessionRun_FiltersOpaqueEvents`: session with user + assistant + KindOpaque events → `SessionView.Events` contains no KindOpaque; remaining events in ascending timestamp order. Maps to spec "Opaque events filtered from view".
  - `TestWatchSessionRun_FiltersMetaUserEvents`: session with meta user event (`IsMeta=true`) and typed user event → only typed event visible. Maps to spec "Meta user events filtered from view".
  - `TestWatchSessionRun_FiltersToolResultOnlyEvents`: session with tool-result-only user event and typed user event → only typed event visible. Maps to spec "Tool-result user events filtered from view".
  - `TestWatchSessionRun_TypedPromptVisibleWithSummary`: typed user event with `Summary="Hello"` → appears in output with `Summary` intact. Maps to spec "Typed user prompt visible".
  - `TestWatchSessionRun_NCountsVisibleOnly`: 5 typed user events + 10 tool-result-only events → exactly 5 events returned, all typed. Maps to spec "N counts visible events only".
- [ ] Update existing `opaque_kind_preserved` subtest: add comment `// Superseded by filter rule 1 (ADR-007). Test is now replaced by TestWatchSessionRun_FiltersOpaqueEvents.` and mark it skipped via `t.Skip("superseded by ADR-007 filter rule")`.
- [ ] Run `go test ./internal/application/watchsession/...` — new tests FAIL.
- **Acceptance**: five new test functions compile and fail.

### 4.2 Implement `isVisible` and insert filter loop

- [ ] Add package-private `isVisible(ev event.Event) bool` to `watchsession.go`:
  - Returns `false` if `ev.Kind == event.KindOpaque`.
  - Returns `false` if payload is `event.UserPayload` and `IsMeta == true`.
  - Returns `false` if payload is `event.UserPayload` and `IsToolResultOnly == true`.
  - Returns `true` otherwise.
- [ ] In `Run`, insert filter loop between sort and take-last-N (ADR-007 placement).
- [ ] Update `EmptyReason` logic: post-filter empty slice still returns no error (zero-event result is valid).
- [ ] Run `go test ./internal/application/watchsession/...` — all Phase 4.1 tests PASS; all pre-existing passing tests remain green.
- **Acceptance**: all filter rule tests green; `go vet` clean.

---

## Phase 5: TUI — rendering (TDD)

### 5.1 Failing tests — new TUI behaviors (`internal/adapters/tui/model_test.go`)

- [ ] Add `TestTUIView_NoOpaqueMarkers`: inject SessionView with user + assistant events (summaries populated); assert `View().Content` does NOT contain the substring `"(opaque)"`. Maps to spec "Focused pane shows snippets, not opaque markers".
- [ ] Add `TestTUIView_EmptyConversation`: inject empty `SessionView` with no events and no focused session (EmptyReason set) → `View().Content` contains `"No active conversation."`. Maps to spec "Empty Session".
- [ ] Update `threeEventView()` fixture: replace the third event (KindOpaque) with a second typed user event (`event.UserPayload{Summary: "World"}`) and a second assistant event with summary. This represents the post-filter shape the TUI sees in production. Regenerate fixture to have 2 events total (user + assistant with summaries) OR keep 3 non-opaque events (user, assistant, user) — use 3 non-opaque events to preserve the "three" in the name.
- [ ] Run `go test ./internal/adapters/tui/...` — new tests FAIL; golden tests will also fail because fixture changed.
- **Acceptance**: new tests compile and fail; existing tests show expected golden mismatch.

### 5.2 Update `formatEventLine` in `model.go`

- [ ] Remove the `default` branch that emits `(opaque)` from `formatEventLine`; replace with a defensive fallback: `return fmt.Sprintf("%-10s  %-12s", ts, kind)`.
- [ ] Update `UserPayload` case: append `p.Summary` — format `"%-10s  %-12s  %s"`.
- [ ] Update `AssistantPayload` case: append `p.Summary` then usage — format `"%-10s  %-12s  %s  in=%d out=%d"`.
- [ ] In `View()`, replace the `(no events)` fallback with `"No active conversation."` when `len(m.view.Events) == 0`.
- **Acceptance**: `formatEventLine` compiles; model.go produces the new row format.

### 5.3 Regenerate golden files and verify row format

- [ ] Run `go test -update ./internal/adapters/tui/...` to regenerate `testdata/snapshot.golden` and `testdata/view.golden`.
- [ ] Confirm `TestTUIViewGolden` and `TestTUISnapshot` PASS with the new golden files.
- [ ] Manually inspect `testdata/view.golden`: each row must match `HH:MM:SSZ  <kind>  <Summary>` format. Maps to spec "Row format".
- [ ] Run `go test ./internal/adapters/tui/...` without `-update` — all tests green.
- **Acceptance**: all TUI tests pass; `"(opaque)"` absent from golden files; empty-state test passes.

---

## Phase 6: Integration / smoke

### 6.1 Manual smoke test against real session

- [ ] Run `go build ./cmd/clyde` to confirm the binary builds.
- [ ] Run the binary against a real `~/.claude/projects/` session and confirm:
  - At least one visible event appears with non-empty `Summary`.
  - No row shows `(opaque)`.
  - Tool-result-only and meta events are absent from the display.
  - Assistant rows show `in=N out=M` usage.
  - Empty-state line `"No active conversation."` appears when all events filter out (test with a session of only meta events if available).
- **Acceptance**: visual confirmation of post-filter behavior; binary does not panic.

### 6.2 Reinstall binary

- [ ] Run `go install ./cmd/clyde` to update the PATH binary.
- **Acceptance**: `clyde` in PATH reflects the new rendering.

---

## Phase 7: Verification

### 7.1 Full test suite

- [ ] Run `go test ./...` — all tests PASS, zero failures.
- **Acceptance**: green.

### 7.2 Vet and format

- [ ] Run `go vet ./...` — no issues.
- [ ] Run `gofmt -l .` — no files listed (everything formatted).
- **Acceptance**: clean output.

### 7.3 Lint (depguard MUST pass)

- [ ] Run `golangci-lint run ./...`.
- [ ] Confirm `domain-pure` depguard rule passes: `format.go` imports only `strings` and `unicode/utf8` (both allowed).
- [ ] Confirm no cyclomatic complexity violations in `extractAssistantSummary` (split into helpers if needed).
- **Acceptance**: zero lint errors, depguard passes.

### 7.4 Build

- [ ] Run `go build ./cmd/clyde` — clean build, no warnings.
- **Acceptance**: binary produced successfully.

---

## Batch suggestions for /sdd-apply

- **Batch A** — Phases 1–2 (Truncate helper + Payload fields): `format.go`, `format_test.go`, `event.go`. Zero external dependencies; pure domain work. Safest first batch.
- **Batch B** — Phase 3 (JSONL Summary extraction): `jsonl.go`, `events_test.go`, seven new JSONL fixtures. Depends on Batch A (calls `event.Truncate`, populates new payload fields).
- **Batch C** — Phase 4 (WatchSession filter): `watchsession.go`, `watchsession_test.go`. Depends on Batch A (reads `IsMeta`, `IsToolResultOnly` from payload). Supersedes `opaque_kind_preserved` subtest.
- **Batch D** — Phase 5 (TUI rendering): `model.go`, `model_test.go`, golden files. Depends on Batch A (reads `Summary` from payloads). Run `go test -update` at end of this batch.
- **Batch E** — Phases 6–7 (smoke + verification + reinstall): manual test, `go install`, full lint/vet/test pass.
