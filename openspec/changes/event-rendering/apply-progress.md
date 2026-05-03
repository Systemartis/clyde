# Apply Progress: event-rendering

Last updated: 2026-04-30
Current batch: D (Phase 5 — TUI rendering) — COMPLETE

## Completed Tasks

- [x] 1.1 — Failing tests for Truncate written at `internal/domain/event/format_test.go` — RED confirmed
  - `TestTruncate_RuneCountLimit` (3 subtests: empty, shorter, exactly-max)
  - `TestTruncate_AppendsEllipsisOnOverflow` (120-rune ASCII input → 80 runes ending with …)
  - `TestTruncate_CollapsesNewlines` (4 subtests: LF, CRLF, CR, multiple newlines)
  - `TestTruncate_CollapsesConsecutiveWhitespace` (5 subtests: spaces, leading, trailing, tab, all-whitespace)
  - `TestTruncate_RuneBoundaryMultibyte` (90 × "ă" → 80 runes, valid UTF-8, ends with …)
  - `TestTruncate_DegenerateCases` (5 subtests: empty/80, all-whitespace, max=0, max=1 overflow, max=1 exact)
  - Compile error confirmed: `undefined: event.Truncate` across 6 call sites
- [x] 1.2 — Implement Truncate at `internal/domain/event/format.go` — GREEN
  - 7-step algorithm per ADR-011: CRLF → space, lone CR/LF/tab → space, collapse whitespace, trim, rune count check, take (max-1) + "…"
  - Imports: `strings`, `unicode/utf8` only — depguard domain-pure safe
  - All 6 Truncate test functions pass; all pre-existing event tests still pass
  - `go vet ./internal/domain/event/...` clean; `gofmt -l` no output (clean)
- [x] 2.1 — Added `IsMeta bool`, `IsToolResultOnly bool`, `Summary string` to `UserPayload` in `event.go`
  - Additive change — existing `UserPayload{}` constructions in event_test.go compile unchanged (zero values)
  - `go test ./...` — all 7 packages pass, 0 regressions
- [x] 2.2 — Added `Summary string` to `AssistantPayload` in `event.go`
  - Additive change — existing `AssistantPayload{Usage: u}` constructions in event_test.go, tui/model_test.go, jsonl/events_test.go compile unchanged (zero value for Summary)
  - `go test ./...` — all 7 packages pass, 0 regressions

## In Flight

None — Batch A is complete.

## Blocked / Skipped

None.

## Test Output Summary

- 7 packages tested, 41 individual tests passing (0 failures)
- `internal/domain/event`: 10 subtests passing (4 pre-existing + 6 new Truncate groups)
- `internal/adapters/jsonl`: cached green
- `internal/adapters/tui`: cached green
- `internal/application/watchsession`: cached green
- All other packages: cached green

## Lint Output Summary

- `go vet ./internal/domain/event/...` — 0 issues
- `gofmt -l /Users/vladpb/work/Personal/clyde/internal/domain/event/` — no output (all files formatted)
- Full golangci-lint not run in Batch A (Batch E is the lint gate); depguard compliance manually verified: `format.go` imports only `strings` and `unicode/utf8`

## Backward-compat verification

Confirmed: After additive struct changes to `UserPayload` (added `IsMeta bool`, `IsToolResultOnly bool`, `Summary string`) and `AssistantPayload` (added `Summary string`):

- `event_test.go` — `UserPayload{}` (line 77, 98) and `AssistantPayload{Usage: usage.Usage{...}}` (line 87) compile unchanged with zero-value new fields. Tests pass.
- `model_test.go` (tui) — `UserPayload{}` and `AssistantPayload{Usage: ...}` constructions still compile; tests pass (cached green).
- `watchsession_test.go` — constructs `event.UserPayload{}` and `event.AssistantPayload{Usage: ...}`; tests pass (cached green).
- `events_test.go` (jsonl) — all assertions compile and pass (cached green).
- Both structs remain comparable (no slice/map/func fields added — `bool` and `string` are comparable).
- No struct equality (`==`) breakage detected — tests use field access or interface type assertions, not direct struct equality.

## Next batch (after Batch A)

Batch B: Phase 3 — JSONL Summary extraction (`internal/adapters/jsonl/jsonl.go`, `summary.go`, `summary_test.go`, 7 new JSONL fixtures).
Depends on Batch A: calls `event.Truncate`, populates `UserPayload.IsMeta`, `UserPayload.IsToolResultOnly`, `UserPayload.Summary`, `AssistantPayload.Summary`.

---

## Batch B — Phase 3 (JSONL Summary extraction)

### Completed Tasks

- [x] 3.1 — Failing tests written at `internal/adapters/jsonl/summary_test.go` — RED confirmed
  - `TestDecodeLine_UserSummaryFromString` — user string content → Summary populated, IsToolResultOnly = false
  - `TestDecodeLine_UserSummaryFromTextBlock` — user array with text block → Summary = "Hello"
  - `TestDecodeLine_UserIsMetaFlag` — isMeta:true envelope → UserPayload.IsMeta = true
  - `TestDecodeLine_UserIsToolResultOnly` — tool_result-only array → IsToolResultOnly = true, Summary = ""
  - `TestDecodeLine_UserMixedNotToolResultOnly` — text + tool_result → IsToolResultOnly = false, Summary non-empty
  - `TestExtractUserSummary_EmptyContentArray` — empty array [] → IsToolResultOnly = false, Summary = ""

- [x] 3.2 — Implement `extractUserSummary` — GREEN
  - Added `IsMeta bool` field to `envelope` struct in `jsonl.go`
  - Added `userMessage` and `assistantContent` structs to `jsonl.go`
  - Implemented `extractUserSummary(content json.RawMessage)` in `summary.go` — dispatches on '"' vs '[' first byte
  - Implemented `extractUserSummaryFromArray` helper in `summary.go` — ADR-009 algorithm
  - Updated `resolveKindAndPayload` signature to accept `env envelope` parameter
  - Updated "user" case in `resolveKindAndPayload` to decode message, call `extractUserSummary`, populate `UserPayload{IsMeta, IsToolResultOnly, Summary}`
  - Updated `decodeLine` call site to pass `env` as 4th argument

- [x] 3.3 — Failing tests for `extractAssistantSummary` — RED confirmed (written in same `summary_test.go`)
  - `TestDecodeLine_AssistantTextSummary` — text block → Summary = "Let me check the file for you."
  - `TestDecodeLine_AssistantToolUseSummary` (table):
    - `single_tool_use_read` — tool_use Read → "Tool: Read /some/file.go"
    - `multiple_tool_use_first_wins` — two tool_use blocks, Edit first → "Tool: Edit /a.go"
    - `text_and_tool_use_tool_wins` — text + tool_use → summary from tool_use
  - `TestDecodeLine_AssistantThinkingOnly` — thinking block only → "(thinking)"

- [x] 3.4 — Implement `extractAssistantSummary` — GREEN
  - `extractAssistantSummary(content json.RawMessage) string` in `summary.go`
  - Split into helpers: `firstToolUseBlock`, `firstTextBlock`, `hasThinkingBlock` (all <10 cyclomatic — gocyclo compliant)
  - Priority chain: tool_use > text > thinking > ""
  - Updated "assistant" case in `resolveKindAndPayload` to decode message.content and populate `AssistantPayload.Summary`

- [x] 3.5 — Failing tests for `extractToolSummary` (full table) — RED confirmed
  - `TestExtractToolSummary` table (16 cases): read_file_path, write_file_path, edit_file_path, multiedit_file_path,
    bash_with_description, bash_command_short, bash_command_long_truncated, grep_pattern, rg_maps_to_grep_label,
    glob_pattern, task_description, agent_maps_to_task_label, todowrite_n_items, todowrite_zero_items,
    unknown_tool_name_only, mcp_default_branch

- [x] 3.6 — Implement `extractToolSummary` — GREEN
  - `extractToolSummary(name string, input json.RawMessage) string` in `summary.go`
  - Per-tool helpers: `extractFilePathTool`, `extractBashTool`, `extractPatternTool`, `extractDescriptionTool`, `extractTodoWriteTool`
  - `truncateRunes` helper for 60-rune Bash inner truncation (no normalization, no ellipsis — just hard rune cut)
  - Outer `event.Truncate(result, 80)` applied in `extractAssistantSummary` at the call site

- [x] 3.7 — JSONL test fixtures created (7 new files in `internal/adapters/jsonl/testdata/`)
  - `event-rendering-user-typed.jsonl` — user event with string content
  - `event-rendering-user-meta.jsonl` — user event with isMeta:true
  - `event-rendering-user-tool-result.jsonl` — user event with tool_result-only content array
  - `event-rendering-assistant-text.jsonl` — assistant event with single text block
  - `event-rendering-assistant-tool-use.jsonl` — assistant event with tool_use Read block
  - `event-rendering-assistant-thinking.jsonl` — assistant event with only a thinking block
  - `event-rendering-assistant-mixed.jsonl` — assistant with text + tool_use blocks
  - Existing fixtures NOT modified (deterministic for their own tests)

### Files Created

- `internal/adapters/jsonl/summary.go` (NEW — 284 lines) — all 3 extractor functions + private helpers
- `internal/adapters/jsonl/summary_test.go` (NEW — ~450 lines) — all Phase 3 tests
- `internal/adapters/jsonl/testdata/event-rendering-user-typed.jsonl` (NEW)
- `internal/adapters/jsonl/testdata/event-rendering-user-meta.jsonl` (NEW)
- `internal/adapters/jsonl/testdata/event-rendering-user-tool-result.jsonl` (NEW)
- `internal/adapters/jsonl/testdata/event-rendering-assistant-text.jsonl` (NEW)
- `internal/adapters/jsonl/testdata/event-rendering-assistant-tool-use.jsonl` (NEW)
- `internal/adapters/jsonl/testdata/event-rendering-assistant-thinking.jsonl` (NEW)
- `internal/adapters/jsonl/testdata/event-rendering-assistant-mixed.jsonl` (NEW)

### Files Modified

- `internal/adapters/jsonl/jsonl.go` — added `IsMeta` to `envelope` struct; added `userMessage` and `assistantContent` structs; updated `resolveKindAndPayload` signature and both user/assistant cases; updated call site in `decodeLine`

### Test Output Summary (Batch B final)

- 7 packages tested, all pass (0 failures, 0 regressions)
- `internal/adapters/jsonl`: 20 top-level test functions, all PASS
  - New Batch B tests: 10 (6 user summary + 3 assistant summary table + 1 thinking + 16-case tool summary table)
  - Pre-existing tests: all unmodified and passing
- `go vet ./...` — clean
- `gofmt -l` — clean (all files formatted)
- `golangci-lint run ./...` — 0 issues

### Backward-compat verification (Batch B)

- `TestEvents_SimpleUserAssistant` — fixture has string content "Hello, Claude!". After change, `UserPayload.Summary` is now populated to "Hello, Claude!" (truncated to 80 runes — fits). The test only asserts `events[0].Kind == KindUser` and `ap.Usage.Output != 0` — no Summary assertion. Still passes.
- `TestEvents_IDsPopulated` — fixture has user event with string content. UserPayload.Summary is now "user-uuid-1" for the line content (actually no content field). Test does not assert on Summary. Still passes.
- All pre-existing jsonl tests pass unchanged.
- No struct equality (`==`) tests found in events_test.go — all use field access or type assertions.

## In Flight (after Batch B)

None — Batch B is complete.

## Blocked / Skipped

None.

## Next batch (after Batch B)

Batch C: Phase 4 — WatchSession filter (`internal/application/watchsession/watchsession.go`, `watchsession_test.go`).
Depends on Batch A (reads `IsMeta`, `IsToolResultOnly` from payload — fields now populated by Batch B).

---

## Batch C — Phase 4 (WatchSession filter)

### Completed Tasks

- [x] 4.1 — Failing tests written in `internal/application/watchsession/watchsession_test.go` — RED confirmed
  - `TestWatchSession_FiltersOpaque` — 3 events (user + opaque + assistant) → 2 returned (opaque filtered)
  - `TestWatchSession_FiltersMetaUser` — 2 events (IsMeta=true + typed) → 1 returned (meta filtered)
  - `TestWatchSession_FiltersToolResultOnly` — 2 events (IsToolResultOnly=true + typed) → 1 returned
  - `TestWatchSession_FilterPreservesOrder` — 6 events (3 visible, 3 invisible) → 3 visible in ascending order
  - `TestWatchSession_FilterDoesNotAffectN` — 10 visible + 5 opaque interleaved → last 5 visible (not last 5 of unfiltered)
  - All 5 new tests confirmed FAIL before implementation (correct RED state)
  - `opaque_kind_preserved` subtest marked SKIP with reason: "superseded by ADR-007 filter rule — opaque events are now filtered from view"

- [x] 4.2 — Implement `isVisible` and filter loop — GREEN
  - Added `isVisible(ev event.Event) bool` to `watchsession.go` (package-private, bottom of file)
  - Filter rules: KindOpaque → false; UserPayload.IsMeta → false; UserPayload.IsToolResultOnly → false; else → true
  - Filter loop inserted in `Run()` AFTER sort, BEFORE last-N slice — so N counts visible events only (ADR-007 placement)
  - Used `evts[:0]` reslice pattern for zero-allocation filter (reuses backing array)
  - All 5 new filter tests PASS; all pre-existing tests pass (opaque_kind_preserved correctly SKIP'd)
  - Fixed misspell lint error: "unrecognised" → "unrecognized" in `isVisible` comment

### Files Modified

- `internal/application/watchsession/watchsession.go` — updated package doc comment (steps 5–8); added `isVisible` function; added filter loop in `Run()` between sort and take-last-N
- `internal/application/watchsession/watchsession_test.go` — added `opaque_kind_preserved` skip + comment; added helper functions (`mkUserEvent`, `mkAssistantEvent`, `mkOpaqueEvent`); added 5 new top-level test functions

### Files Created

- None

### Tests Added

- 5 new test functions (top-level, all parallel)

### Tests Skipped

- 1 existing subtest (`TestWatchSession/opaque_kind_preserved`) — skipped with superseded comment

### Test Output Summary (Batch C final)

- 9 packages tested, all pass (0 failures, 0 regressions)
- `internal/application/watchsession`: 6 top-level test functions, all PASS
  - `TestWatchSession` (parent with 5 subtests — chronological_order, last_N_truncation, opaque_kind_preserved [SKIP], no_sessions, multi_session_focus, fewer_than_N)
  - `TestWatchSession_FiltersOpaque` — PASS
  - `TestWatchSession_FiltersMetaUser` — PASS
  - `TestWatchSession_FiltersToolResultOnly` — PASS
  - `TestWatchSession_FilterPreservesOrder` — PASS
  - `TestWatchSession_FilterDoesNotAffectN` — PASS
- All other packages: cached green

### Gate Results (Batch C)

- `go test ./...` — 0 failures, 0 regressions
- `go vet ./...` — 0 issues
- `gofmt -l` — no output (clean)
- `golangci-lint run ./...` — 0 issues (misspell fixed)

## In Flight (after Batch C)

None — Batch C is complete.

## Blocked / Skipped

None.

## Next batch (after Batch C)

Batch D: Phase 5 — TUI rendering (`internal/adapters/tui/model.go`, `model_test.go`, golden files).
Depends on Batch A (reads `Summary` from payloads). Run `go test -update` at end of this batch.

---

## Batch D — Phase 5 (TUI rendering)

### Completed Tasks

- [x] 5.4 — Updated `threeEventView()` fixture in `model_test.go`
  - Replaced KindOpaque third event with a second `KindAssistant` event (tool-use with `Summary: "Tool: Read /path/to/file.go"`)
  - First event: `UserPayload{Summary: "Hello"}` (typed user)
  - Second event: `AssistantPayload{Usage: {6, 825}, Summary: "Let me check the file for you."}` (text assistant)
  - Third event: `AssistantPayload{Usage: {0, 0}, Summary: "Tool: Read /path/to/file.go"}` (tool-use assistant)
  - This represents the post-filter shape TUI sees in production (ADR-007 — opaque filtered by WatchSession)

- [x] 5.5 — New direct-Update tests added to `model_test.go` — RED confirmed, then GREEN after model.go update
  - `TestModelView_RendersUserSummary` — user event with Summary "Hello" → View().Content contains "Hello" and "user"
  - `TestModelView_RendersAssistantTextSummary` — assistant text Summary → View().Content contains "assistant" and "Let me help you."
  - `TestModelView_RendersAssistantToolUseSummary` — `Summary: "Tool: Read /path/to/file.go"` → View() contains "Tool: Read"
  - `TestModelView_AssistantUsageAppended` — non-zero usage (42/100) → View() contains "in=42" and "out=100"
  - `TestModelView_NoOpaqueLabel` — 3 visible events → View().Content does NOT contain "(opaque)"
  - `TestModelView_EmptyState` — empty events list → View() contains "No active conversation."
  - RED state confirmed before model.go changes; all 6 pass after implementation

- [x] 5.1/5.2 — Updated `model.go` — new row format, empty state, drop opaque branch
  - Added `charm.land/lipgloss/v2` import for `lipgloss.NewStyle().Faint(true).Render()`
  - `formatEventLine()` — removed opaque default branch; defensive fallback now silently skips (no "(opaque)" label)
  - `formatEventLine()` — UserPayload case: appends `p.Summary` — `"%-10s  %-12s  %s"` format
  - `formatEventLine()` — AssistantPayload case: appends `p.Summary` then usage (only if Input>0 || Output>0) — `"%-10s  %-12s  %s  in=%d out=%d"` format; zero-usage omits tail
  - `View()` — empty events branch replaced: `lipgloss.NewStyle().Faint(true).Render("No active conversation.")`

- [x] 5.6 — Golden files regenerated with `go test ./internal/adapters/tui/... -update`
  - `testdata/view.golden` — regenerated with new row format (3 rows: user+Hello, assistant+text+usage, assistant+tool-use)
  - `testdata/snapshot.golden` — regenerated with teatest capture (ANSI escape sequences, structurally unchanged)
  - Goldens manually verified — `view.golden` shows correct HH:MM:SSZ + kind label + summary + usage format
  - Re-ran `go test ./internal/adapters/tui/...` without `-update` — all 13 tests PASS

### Files Modified

- `internal/adapters/tui/model.go` — added lipgloss import; rewrote `formatEventLine` (new row format, drop opaque branch); updated `View()` empty state to "No active conversation." with faint style
- `internal/adapters/tui/model_test.go` — updated `threeEventView()` fixture (3 post-filter events with Summaries); added 6 new test functions (TestModelView_*)
- `internal/adapters/tui/testdata/view.golden` — regenerated with new row format
- `internal/adapters/tui/testdata/snapshot.golden` — regenerated (teatest capture)

### Files Created

- None

### Tests Added

- 6 new test functions: `TestModelView_RendersUserSummary`, `TestModelView_RendersAssistantTextSummary`, `TestModelView_RendersAssistantToolUseSummary`, `TestModelView_AssistantUsageAppended`, `TestModelView_NoOpaqueLabel`, `TestModelView_EmptyState`

### Goldens Regenerated

- 2 golden files: `testdata/view.golden` (new format visible), `testdata/snapshot.golden` (teatest capture)

### Test Output Summary (Batch D final)

- 9 packages tested, all pass (0 failures, 0 regressions)
- `internal/adapters/tui`: 13 test functions, all PASS
  - Pre-existing: TestModelQuitOnQ, TestModelQuitOnCtrlC, TestModelWindowResize, TestModelEscDoesNotQuit, TestTUISnapshot, TestTUIViewGolden, TestTUIViewEmptyState
  - New Batch D: TestModelView_RendersUserSummary, TestModelView_RendersAssistantTextSummary, TestModelView_RendersAssistantToolUseSummary, TestModelView_AssistantUsageAppended, TestModelView_NoOpaqueLabel, TestModelView_EmptyState
- All other packages: cached green

### Gate Results (Batch D)

- `go test ./...` — 0 failures, 0 regressions (9 packages)
- `go vet ./...` — 0 issues
- `gofmt -l` — no output (all files formatted)
- `golangci-lint run ./...` — 0 issues
- `go build ./cmd/clyde` — binary built successfully

### Golden File Visual Verification

`testdata/view.golden` contents:
```
clyde — sess-1
─────────────────────────────────────────
10:00:00Z   user          Hello
10:00:05Z   assistant     Let me check the file for you.  in=6 out=825
10:00:10Z   assistant     Tool: Read /path/to/file.go
─────────────────────────────────────────
q to quit
```
Format is correct: timestamp (9 chars + padding), kind label (padded to 12), summary, optional usage tail.
No "(opaque)" markers. Third row omits usage tail (both 0). 

## In Flight (after Batch D)

None — Batch D is complete.

## Blocked / Skipped

None.

## Next batch

Batch E: Phase 6 — smoke + verify + reinstall (integration smoke test, sdd-verify, `go install`).
All 4 phases (A, B, C, D) complete. All gates clean.
