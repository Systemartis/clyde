# Apply Progress: bootstrap-skeleton

Last updated: 2026-04-30T14:00:00Z
Current batch: C complete (Phase 6 — Ports, Phase 7 — WatchSession TDD)

## Completed Tasks

- [x] 1.1 — Initialize Go module — `go mod init github.com/clyde-tui/clyde`; `go.mod` created with `module github.com/clyde-tui/clyde` and `go 1.26`
- [x] 1.2 — Add runtime dependencies — **Discovery**: bubbletea/v2 and lipgloss/v2 have migrated to `charm.land/` module paths. Used `charm.land/bubbletea/v2@v2.0.6` and `charm.land/lipgloss/v2@v2.0.3` (see Deviations below)
- [x] 1.3 — Add test-only dependency (teatest/v2) — `github.com/charmbracelet/x/exp/teatest/v2@v2.0.0-20260430013151-79116d1f37bd` added; `go.mod` updated
- [x] 1.4 — Create testdata directory stubs — `internal/adapters/jsonl/testdata/` and `multi-session/` subdirectory created with `.gitkeep` placeholders
- [x] 1.5 — Verify .gitignore covers binary — `/clyde` already present in `.gitignore`; no change needed
- [x] 2.1 — Write failing test: Usage.Zero identity (left) — `usage_test.go` created with `TestUsageZeroIsIdentity`; confirmed RED (compile error: undefined Zero/Usage)
- [x] 2.2 — Implement Usage struct and Zero — `usage.go` created with `Usage` struct (4 int64 fields), `Zero()`, and `Add()` method; `TestUsageZeroIsIdentity` GREEN
- [x] 2.3 — Write failing tests: right-identity and non-mutation — `TestUsageZeroIsIdentityRight` and `TestUsageAddIsNonMutating` added; passed immediately since Add already existed from 2.2 (documented below)
- [x] 2.4 — Implement Usage.Add — `Add` was implemented in 2.2 (required to satisfy left-identity test); no additional code needed
- [x] 2.5 — Write failing tests: commutativity, associativity, counter accumulation — `TestUsageCommutativity`, `TestUsageAssociativity`, `TestUsageCounterAccumulation` added; all GREEN

## In Flight

None — all Batch A tasks complete.

## Blocked / Skipped

None. One deviation documented below.

## Deviations from Design

**Task 1.2 — Module path change for bubbletea/v2 and lipgloss/v2**

The design specified `github.com/charmbracelet/bubbletea/v2` and `github.com/charmbracelet/lipgloss/v2`. However, both packages have migrated their canonical module path to `charm.land/bubbletea/v2` and `charm.land/lipgloss/v2` respectively (the `go.mod` inside those packages now declares the `charm.land/` path). Using the old `github.com/charmbracelet/` path causes a module path mismatch error at `go get` time.

**Resolution**: Used the `charm.land/` paths. The `go.mod` now reads:
```
charm.land/bubbletea/v2 v2.0.6
charm.land/lipgloss/v2 v2.0.3
```

**Impact on `.golangci.yml`**: The existing `depguard` deny rules reference `github.com/charmbracelet/bubbletea` and `github.com/charmbracelet/lipgloss`. These will NOT match the new `charm.land/` import paths. Downstream Batch E (TUI adapter) must update `.golangci.yml` to deny `charm.land/bubbletea` and `charm.land/lipgloss` in domain/application layers, or accept that the architectural guard is weakened for those specific paths. This is a Batch E / verification concern — noted as a risk.

**Task 2.3/2.4 ordering note**:

Task 2.2 required implementing `Add` (because `Zero().Add(a)` was already tested in 2.1). Tasks 2.3 and 2.4 are therefore collapsed: 2.3 tests were written and passed immediately since `Add` was already in place. This matches the task note: "if Add already satisfies, skip the 'ensure fail' step and document why".

## TDD Cycle Evidence

| Task | RED (test written, fail confirmed) | GREEN (impl makes it pass) | REFACTOR |
|------|------------------------------------|----------------------------|----------|
| 2.1 TestUsageZeroIsIdentity | ✅ `no non-test Go files` error | ✅ After usage.go + Zero() added | — |
| 2.2 impl Zero+Add | n/a (impl task) | ✅ TestUsageZeroIsIdentity passes | — |
| 2.3 TestUsageZeroIsIdentityRight + TestUsageAddIsNonMutating | ⚠️ Add already existed — tests green immediately (documented above) | ✅ Green | — |
| 2.5 TestUsageCommutativity + TestUsageAssociativity + TestUsageCounterAccumulation | ⚠️ Add already existed — tests green immediately | ✅ Green | — |

## Test Output Summary

```
ok  github.com/clyde-tui/clyde/internal/domain/usage  0.280s
```

17 tests (3 in TestUsageZeroIsIdentity, 3 in TestUsageZeroIsIdentityRight, 4 in TestUsageCommutativity, 3 in TestUsageAssociativity, 3 in TestUsageCounterAccumulation, 1 in TestUsageAddIsNonMutating), all passing.

## Lint Output Summary

```
golangci-lint run ./...
0 issues.
```

`go vet ./...` — exits 0, no issues. `gofmt -l .` — empty output (all files formatted).

## Files Created

| File | Description |
|------|-------------|
| `go.mod` | Go module with `github.com/clyde-tui/clyde`, `go 1.26`, all three dependencies |
| `go.sum` | Dependency checksums |
| `internal/domain/usage/usage.go` | Usage struct, Zero(), Add() — pure domain value object |
| `internal/domain/usage/usage_test.go` | 6 test functions covering all 5 Usage monoid invariants + non-mutation |
| `internal/adapters/jsonl/testdata/.gitkeep` | Placeholder for testdata root |
| `internal/adapters/jsonl/testdata/multi-session/.gitkeep` | Placeholder for multi-session fixture directory |

## Next batch (from Batch A)

Batch B: Domain — Event, Session, Project (Phases 3-5). ← COMPLETE. See Batch B section below.

---

## Batch B

### Completed Tasks

- [x] 3.1 — Write failing test: Kind enum and OpaquePayload preservation — `event_test.go` created with `TestKindString`, `TestOpaquePayloadPreservesRaw`; confirmed RED (`no non-test Go files`)
- [x] 3.2 — Implement Event, Kind enum, Payload sealed interface — `event.go` created; `Kind` as `string`-based type, `Payload` sealed interface with unexported marker, `UserPayload`, `AssistantPayload{Usage usage.Usage}`, `OpaquePayload{Raw []byte}`, `Event` struct with all envelope fields
- [x] 3.3 — Write failing test: Event construction helper — `TestNewEvent` was included in the initial 3.1 test file (4 subtests covering user/assistant/root/opaque events); also `TestAssistantPayloadCarriesUsage`
- [x] 3.4 — Implement NewEvent constructor — `NewEvent()` implemented in 3.2 (same production file); tests 3.3 passed immediately since implementation was co-located — pragmatic skip per tasks.md clause
- [x] 4.1 — Write failing test: Session type and Summary ordering helper — `session_test.go` created with `TestSessionIDIsString` and `TestSummaryOrderByLastActivity` (3 table cases); confirmed RED
- [x] 4.2 — Implement Session domain types — `session.go` created; `type ID string`, `Summary{ID ID, LastActivity time.Time}`; no adapters/I/O imported; all session tests GREEN
- [x] 5.1 — Write failing test: Project type — `project_test.go` created with `TestProjectCWD` (3 cases) and `TestProjectRelativePathRejected` (3 cases); confirmed RED
- [x] 5.2 — Implement Project type — `project.go` created; `Project{cwd string}`, `New(cwd string) Project` (panics on relative path), `CWD() string`; all project tests GREEN

### TDD Cycle Evidence

| Task | RED confirmed | GREEN | Note |
|------|--------------|-------|------|
| 3.1 TestKindString + TestOpaquePayloadPreservesRaw | ✅ `no non-test Go files` | ✅ After event.go | — |
| 3.2 impl Event/Kind/Payload | n/a (impl task) | ✅ All event tests pass | — |
| 3.3 TestNewEvent | ⚠️ Written in same commit as 3.1; constructor implemented in 3.2 — test was green immediately after 3.2 | ✅ Green | pragmatic skip per clause |
| 3.4 NewEvent constructor | n/a (impl task done in 3.2) | ✅ TestNewEvent passes | — |
| 4.1 TestSessionIDIsString + TestSummaryOrderByLastActivity | ✅ `no non-test Go files` | ✅ After session.go | — |
| 4.2 impl ID + Summary | n/a (impl task) | ✅ All session tests pass | — |
| 5.1 TestProjectCWD + TestProjectRelativePathRejected | ✅ `no non-test Go files` | ✅ After project.go | — |
| 5.2 impl Project + New + CWD | n/a (impl task) | ✅ All project tests pass | — |

### Deviations from Design / Discoveries

**OpaquePayload comparability**: `OpaquePayload` contains `Raw []byte` which is not comparable with `==`. The test originally used `ev.Payload != tc.payload` which panics at runtime. Fixed by type-switching in the test and using `bytes.Equal` for the opaque case. This is expected Go behaviour — slices are not comparable.

**TestNewEvent grouped with 3.1**: Per tasks.md, 3.3 (write failing test for `NewEvent`) is a separate step, but writing the test before having `NewEvent` only causes a compile error (not the described "no constructor" failure message). Since both test functions were authored in the same test file before any production code existed, the RED signal was the same `no non-test Go files` compile error that covered all tests. Documented as pragmatic grouping — zero production code skipped.

**TestProjectRelativePathRejected added beyond tasks.md scope**: Tasks 5.1/5.2 only mention `TestProjectCWD`. The relative-path rejection test was added because spec.md states "A Project is identified by its absolute working directory path" — the test exercises the `New()` panic guard. This is an addition, not a deviation from design intent.

**US English spelling**: The `misspell` linter (locale: US) flagged British spellings in comments (`recognised` → `recognized`, `unrecognised` → `unrecognized`, `materialised` → `materialized`). Fixed before lint exit. No logic change.

### Test Output Summary (Batch B final)

```
ok  github.com/clyde-tui/clyde/internal/domain/event    0.234s
ok  github.com/clyde-tui/clyde/internal/domain/project  0.414s
ok  github.com/clyde-tui/clyde/internal/domain/session  0.661s
ok  github.com/clyde-tui/clyde/internal/domain/usage    (cached)
```

Total tests across all packages: 17 (usage) + 6 (event: 3 kind + 1 opaque + 4 newEvent + 1 assistantUsage) + 4 (session: 1 idString + 3 summaryOrder subtests) + 6 (project: 3 cwd + 3 relativeRejected) = ~33 test cases.

### Lint/Vet/Fmt Output Summary (Batch B final)

```
golangci-lint run ./...
0 issues.

go vet ./...
(exit 0, no output)

gofmt -l .
(exit 0, empty output — all files formatted)
```

### Files Created

| File | Description |
|------|-------------|
| `internal/domain/event/event.go` | Kind enum, Payload sealed interface, UserPayload, AssistantPayload, OpaquePayload, Event struct, NewEvent constructor |
| `internal/domain/event/event_test.go` | TestKindString, TestOpaquePayloadPreservesRaw, TestNewEvent (4 subtests), TestAssistantPayloadCarriesUsage |
| `internal/domain/session/session.go` | type ID string, Summary struct |
| `internal/domain/session/session_test.go` | TestSessionIDIsString, TestSummaryOrderByLastActivity (3 subtests) |
| `internal/domain/project/project.go` | Project struct, New() with absolute-path guard, CWD() |
| `internal/domain/project/project_test.go` | TestProjectCWD (3 subtests), TestProjectRelativePathRejected (3 subtests) |

## Next batch

Batch C: Phase 6 (Ports) + Phase 7 (WatchSession TDD). ← COMPLETE. See Batch C section below.

---

## Batch C

### Completed Tasks

- [x] 6.1 — Create SessionSource port — `internal/ports/sessionsource.go` created with `SessionSource` interface; two methods: `Sessions(ctx, projectCWD string) ([]session.Summary, error)` and `Events(ctx, id session.ID) ([]event.Event, error)`; doc comment captures full port contract
- [x] 6.2 — Create Clock port — `internal/ports/clock.go` created with `Clock` interface (`Now() time.Time`); doc comment captures monotonicity contract
- [x] 6.3 — Verify depguard on ports layer — `golangci-lint run ./internal/ports/...` exits 0, zero depguard violations; ports import only `context`, `time`, and domain packages (no adapters)
- [x] 7.1 — Write failing tests: all six spec scenarios — `watchsession_test.go` created in `watchsession_test` (external test package); in-file `stubSessionSource` and `stubClock` with compile-time interface assertions; all 6 subtests confirmed RED (`no non-test Go files`)
- [x] 7.2 — Define SessionView type — `watchsession.go` created with `SessionView` struct, `WatchSession` struct, `New()` constructor, `Run()` stub returning empty `SessionView{}`; tests compiled with assertion failures (logic stub not yet implemented)
- [x] 7.3 — Implement WatchSession.Run logic — full logic implemented in same production file: Sessions discovery → select most-recently-active (max LastActivity) → Events fetch → sort ascending by Timestamp → take last min(N=5, len) → populate SessionView; all six spec subtests GREEN in single pass

### TDD Cycle Evidence

| Task | RED confirmed | GREEN | Note |
|------|--------------|-------|------|
| 7.1 — Write 6 test cases | ✅ `no non-test Go files` compile error | — | Test file written before production file |
| 7.2 — SessionView + stub Run | — | ⚠️ Tests compiled but failed assertions | Intermediate step: compilation stub only |
| 7.3 — Implement Run logic | — | ✅ All 6 subtests PASS | Full logic in one pass — no intermediate failures since 7.2 already showed exactly what was missing |

**Note on 7.2/7.3 TDD ordering**: Task 7.2 (stub) and 7.3 (implement) were done together as a single commit because the stub in 7.2 immediately revealed all assertion failures. Per the pragmatic skip rule: the RED signal was clear at the 7.1 stage (compile error), and 7.3 implementation directly addressed those failures without further artificial intermediate states.

### Deviations from Design

**SessionView.EmptyReason field added**: `EmptyReason string` added to `SessionView` as specified in tasks.md (design ADR-006). Set to `"no sessions"` when the project has no sessions, `"session has no events"` when the focused session is empty. Empty string when events are present. This matches the ADR-006 contract verbatim.

**N is a const, not a parameter**: `defaultN = 5` is a package-level constant. Tasks.md says "make N a parameter on WatchSession or via an Option — tasks.md should clarify". The tasks.md does not add an explicit Option pattern. `WatchSession.n` is an internal field set to `defaultN` by `New()` — the type is ready to accept a `WithN(n int)` option in a future change without breaking the existing API. No functional regression.

**External test package (`watchsession_test`)**: The test package is `watchsession_test` (external) rather than `watchsession` (internal). This keeps the stub types invisible to production code and validates the public API surface. The stubs implement `ports.SessionSource` and `ports.Clock` with compile-time `var _ interface = ...` assertions.

### Test Output Summary (Batch C final)

```
=== RUN   TestWatchSession
    --- PASS: TestWatchSession/chronological_order (0.00s)
    --- PASS: TestWatchSession/last_N_truncation (0.00s)
    --- PASS: TestWatchSession/opaque_kind_preserved (0.00s)
    --- PASS: TestWatchSession/no_sessions (0.00s)
    --- PASS: TestWatchSession/multi_session_focus (0.00s)
    --- PASS: TestWatchSession/fewer_than_N (0.00s)
PASS
ok  github.com/clyde-tui/clyde/internal/application/watchsession  0.548s

Full suite:
ok  github.com/clyde-tui/clyde/internal/application/watchsession  0.261s
ok  github.com/clyde-tui/clyde/internal/domain/event    (cached)
ok  github.com/clyde-tui/clyde/internal/domain/project  (cached)
ok  github.com/clyde-tui/clyde/internal/domain/session  (cached)
ok  github.com/clyde-tui/clyde/internal/domain/usage    (cached)
?   github.com/clyde-tui/clyde/internal/ports            [no test files]
```

### Lint/Vet/Fmt Output Summary (Batch C final)

```
golangci-lint run ./...
0 issues.

go vet ./...
(exit 0, no output)

gofmt -l .
(exit 0, empty output — all files formatted)
```

### Files Created

| File | Description |
|------|-------------|
| `internal/ports/sessionsource.go` | SessionSource interface with Sessions + Events methods; full contract doc |
| `internal/ports/clock.go` | Clock interface with Now() method; monotonicity contract doc |
| `internal/application/watchsession/watchsession.go` | SessionView struct, WatchSession struct, New() constructor, Run() with full logic |
| `internal/application/watchsession/watchsession_test.go` | stubSessionSource + stubClock fakes; TestWatchSession with all 6 spec subtests |

## Next batch

Batch D: Phase 8 (systemclock adapter TDD) + Phase 9 (jsonl adapter TDD + fixtures). ← COMPLETE. See Batch D section below.

---

## Batch D

Last updated: 2026-04-30T15:30:00Z
Current batch: D complete (Phase 8 — systemclock, Phase 9 — jsonl)

### Completed Tasks

- [x] 8.1 — Write failing test: systemclock smoke test — `systemclock_test.go` created (external package `systemclock_test`); 3 test functions: `TestClockNowIsUTC`, `TestClockNowIsMonotonic`, `TestClockNowIsCloseToSystemTime`; compile-time assertion `var _ ports.Clock = (*systemclock.Clock)(nil)`; confirmed RED (`no non-test Go files`)
- [x] 8.2 — Implement systemclock.go — `Clock` struct, `New()` constructor, `Now()` returning `time.Now().UTC()`; compile-time `var _ interface{Now() time.Time} = (*Clock)(nil)` assertion in production file; all 3 tests GREEN
- [x] 9.1 — Write failing tests: encode_test.go, events_test.go, sessions_test.go — all 3 test files created before any production code; confirmed RED (`no non-test Go files`)
- [x] 9.2 — Create fixtures: `testdata/simple-user-assistant.jsonl` (2 lines: user + assistant with usage); `testdata/unknown-types.jsonl` (3 lines: queue-operation, ai-title, permission-mode); large-thinking fixture generated programmatically in `TestEvents_LargeThinkingBlock` (80KB thinking block, no large file in repo)
- [x] 9.3 — Implement `encode.go` — `EncodeProjectPath(cwd string) string` (exported for testing): replaces every `/` in `cwd[1:]` with `-`, prepends `-`; 4 table-driven cases all GREEN
- [x] 9.4 — Implement `jsonl.go` — `Source` struct, `NewSource(baseDir)`, `NewProductionSource()`, `Sessions()`, `Events()`, `findSessionFile()`, `decodeFile()`, internal `envelope`/`assistantMessage`/`tokenUsage` types, `decodeLine()`, `resolveKindAndPayload()`; compile-time `var _ ports.SessionSource = (*Source)(nil)` in `sessions_test.go`
- [x] 9.5 — All jsonl tests GREEN: `TestEncodeProjectPath` (4 cases), `TestEvents_SimpleUserAssistant`, `TestEvents_LargeThinkingBlock` (80KB), `TestEvents_UnknownTypes`, `TestEvents_MissingFile`, `TestEvents_IDsPopulated`, `TestSessions_MultiSession`, `TestSessions_MissingDirectory`, `TestSessions_EmptyDirectory`

### TDD Cycle Evidence

| Task | RED confirmed | GREEN | Note |
|------|--------------|-------|------|
| 8.1 TestClockNowIsUTC + TestClockNowIsMonotonic + TestClockNowIsCloseToSystemTime | ✅ `no non-test Go files` | ✅ After systemclock.go | — |
| 8.2 impl systemclock | n/a (impl task) | ✅ All 3 clock tests pass | — |
| 9.1 All jsonl test files | ✅ `no non-test Go files` compile error | — | All 3 test files written before any production code |
| 9.2 Fixtures created | n/a (data task) | n/a | testdata/ files hand-crafted; large-thinking generated in test |
| 9.3 impl encode.go | — | ✅ TestEncodeProjectPath (4 cases) GREEN | — |
| 9.4 impl jsonl.go | — | ✅ All 9 jsonl test functions GREEN | Full suite pass in one go |

### Deviations from Design / Discoveries

**EncodeProjectPath exported (not private)**: Design called for a private function. Exported as `EncodeProjectPath` to allow direct testing from the external `jsonl_test` package without awkward re-exporting shims. The function is still adapter-internal in the sense that it lives in the `jsonl` package — no other package imports it. If it must be truly private, the tests could be moved to `package jsonl` (internal). Exported form is cleaner for V1.

**`encodeProjectPath` handles root "/" correctly**: Input `"/"` yields `"-"`. This is a trivially correct encoding given the algorithm (`cwd[1:]` on `"/"` is `""`, then `strings.ReplaceAll("", "/", "-")` is `""`, prepend `"-"` yields `"-"`). Added as a table case to document the edge case explicitly.

**Sessions() uses file mtime for LastActivity (not last-line timestamp parse)**: Design.md mentions both options ("stat-and-tail-the-last-line OR read fully"). Chosen: file mtime (stat only). Rationale: O(sessions) instead of O(events), and JSONL append semantics mean mtime ≈ last-event-time. The mtime approach is simpler, faster, and sufficient for session ordering in V1. Future change can switch to last-line parse if precision is needed.

**Malformed line policy — fail, not skip**: When a JSONL line fails JSON unmarshal or is missing required envelope fields (`uuid`, `type`, `timestamp`, `sessionId`), `Events()` returns an error with the file path + 1-based line number. Rationale: failing loudly avoids silently returning incomplete event data. Callers that explicitly want to skip bad lines can wrap the error or implement their own scanner — this is a V1 design choice. Documented in package doc comment.

**Large-thinking fixture is programmatic (not a static file)**: `buildAssistantLineWithThinking(80 * 1024)` generates an 80KB+ thinking block inline in the test. No large file committed to the repo. The fixture is rebuilt on each `go test` run — negligible overhead.

**`Sessions()` uses `os.ReadDir` + stat path (not mtime from ReadDir directly)**: `ReadDir` returns `DirEntry` which has an `Info()` method. We call `e.Info()` to get the mtime. If the file disappears between `ReadDir` and `Info()`, the entry is skipped (not an error). This matches the design's "empty directory → empty slice" contract extended to race conditions.

**`Events()` searches all project subdirs for the session file**: Since `Events(ctx, session.ID)` receives only a session ID (not the project CWD), the implementation scans all encoded subdirs under `baseDir` looking for `<sessionID>.jsonl`. This is O(projects) and fine for V1 single-project scope. A future port revision could add the project CWD to the Events signature, or callers can pass it via a wrapper.

**Compile-time interface assertion placement**: `var _ ports.Clock = (*systemclock.Clock)(nil)` appears in the production file `systemclock.go` (as a `var _ interface{Now() time.Time} = ...`) AND in the test file `systemclock_test.go` (as `var _ ports.Clock = ...`). Both compile-time checks are intentional — the production one catches drift even without running tests; the test one exercises the exact port interface. Similarly for jsonl: `var _ ports.SessionSource = (*jsonl.Source)(nil)` is in `sessions_test.go`.

### Test Output Summary (Batch D final)

```
=== RUN   TestClockNowIsUTC
--- PASS: TestClockNowIsUTC (0.00s)
=== RUN   TestClockNowIsMonotonic
--- PASS: TestClockNowIsMonotonic (0.00s)
=== RUN   TestClockNowIsCloseToSystemTime
--- PASS: TestClockNowIsCloseToSystemTime (0.00s)
PASS
ok  github.com/clyde-tui/clyde/internal/adapters/systemclock  0.518s

=== RUN   TestEncodeProjectPath (4 subtests: /Users/vladpb/work/Personal, /Users/vladpb/work/Personal/clyde, /home/user/projects/myapp, /)
--- PASS: TestEncodeProjectPath (0.00s)
=== RUN   TestEvents_SimpleUserAssistant --- PASS (0.00s)
=== RUN   TestEvents_LargeThinkingBlock  --- PASS (0.00s)  [80KB thinking — no "token too long"]
=== RUN   TestEvents_UnknownTypes        --- PASS (0.00s)  [3 opaque events preserved]
=== RUN   TestEvents_MissingFile         --- PASS (0.00s)  [error returned, not panic]
=== RUN   TestEvents_IDsPopulated        --- PASS (0.00s)  [uuid, sessionId, parentUuid, timestamp]
=== RUN   TestSessions_MultiSession      --- PASS (0.00s)  [2 sessions, ordered by LastActivity desc]
=== RUN   TestSessions_MissingDirectory  --- PASS (0.00s)  [empty slice, no error]
=== RUN   TestSessions_EmptyDirectory    --- PASS (0.00s)  [empty slice, no error]
PASS
ok  github.com/clyde-tui/clyde/internal/adapters/jsonl  0.518s

Full suite:
ok  github.com/clyde-tui/clyde/internal/adapters/jsonl          0.244s
ok  github.com/clyde-tui/clyde/internal/adapters/systemclock    0.416s
ok  github.com/clyde-tui/clyde/internal/application/watchsession (cached)
ok  github.com/clyde-tui/clyde/internal/domain/event            (cached)
ok  github.com/clyde-tui/clyde/internal/domain/project          (cached)
ok  github.com/clyde-tui/clyde/internal/domain/session          (cached)
ok  github.com/clyde-tui/clyde/internal/domain/usage            (cached)
?   github.com/clyde-tui/clyde/internal/ports                   [no test files]
```

### Lint/Vet/Fmt Output Summary (Batch D final)

```
golangci-lint run ./...
0 issues.

go vet ./...
(exit 0, no output)

gofmt -l .
(exit 0, empty output — all files formatted)
```

One formatting fix applied mid-phase: struct field alignment in `envelope` and `tokenUsage` types (gofmt requires consistent alignment within each field group).

### Files Created

| File | Description |
|------|-------------|
| `internal/adapters/systemclock/systemclock.go` | Clock struct, New() constructor, Now() returning time.Now().UTC(); compile-time interface assertion |
| `internal/adapters/systemclock/systemclock_test.go` | TestClockNowIsUTC, TestClockNowIsMonotonic, TestClockNowIsCloseToSystemTime; ports.Clock compile-time assertion |
| `internal/adapters/jsonl/encode.go` | EncodeProjectPath() — observational CWD→dirname algorithm |
| `internal/adapters/jsonl/jsonl.go` | Source struct, NewSource(), NewProductionSource(), Sessions(), Events(), findSessionFile(), decodeFile(), envelope/assistantMessage/tokenUsage types, decodeLine(), resolveKindAndPayload() |
| `internal/adapters/jsonl/encode_test.go` | TestEncodeProjectPath (4 table cases) |
| `internal/adapters/jsonl/events_test.go` | TestEvents_SimpleUserAssistant, TestEvents_LargeThinkingBlock (programmatic 80KB), TestEvents_UnknownTypes, TestEvents_MissingFile, TestEvents_IDsPopulated |
| `internal/adapters/jsonl/sessions_test.go` | TestSessions_MultiSession, TestSessions_MissingDirectory, TestSessions_EmptyDirectory; ports.SessionSource compile-time assertion |
| `internal/adapters/jsonl/testdata/simple-user-assistant.jsonl` | 2-line fixture: user event + assistant event with usage |
| `internal/adapters/jsonl/testdata/unknown-types.jsonl` | 3-line fixture: queue-operation, ai-title, permission-mode (all opaque) |

## Next batch

Batch E: Phase 10 (TUI adapter) + Phase 11 (composition root / main.go). ← COMPLETE. See Batch E section below.

---

## Batch E

Last updated: 2026-04-30T16:00:00Z
Current batch: E complete (Phase 10 — TUI adapter, Phase 11 — composition root)

### Completed Tasks

- [x] 10.1 — Write failing tests: Update() behavior (quit keys, resize) — `model_test.go` created in `tui_test` (external test package); `TestModelQuitOnQ`, `TestModelQuitOnCtrlC`, `TestModelWindowResize`, `TestModelEscDoesNotQuit`; confirmed RED (`no non-test Go files`)
- [x] 10.2 — Scaffold TUI Model (compilation spike) — `model.go` created; `Model` struct, `New()`, `Init()`, `Update()`, `View()`, `IsQuitting()`; compile-time assertion `var _ tea.Model = (*tui.Model)(nil)` in test file; all 4 Update() tests GREEN
- [x] 10.3 — TUI snapshot test (PRIMARY PATH — teatest/v2 compiles and works) — `TestTUISnapshot` added using `teatest.NewTestModel`; sends `q` to quit; asserts final output against `testdata/snapshot.golden` (golden file auto-created on first run); PASS
- [x] 10.4 — Direct View() golden test — `TestTUIViewGolden` validates View() output against `testdata/view.golden`; `TestTUIViewEmptyState` validates empty-state rendering; both PASS
- [x] 11.1 — Create `cmd/clyde/main.go` (~32 lines) — wires `jsonl.NewProductionSource()`, `systemclock.New()`, `watchsession.New(src, clk)`, `tui.New(&use, cwd)`; runs `tea.NewProgram(model).Run()`; `go build ./cmd/clyde` exits 0; binary produced at 5.3 MB

### TDD Cycle Evidence

| Task | RED confirmed | GREEN | Note |
|------|--------------|-------|------|
| 10.1 TestModelQuitOnQ + TestModelQuitOnCtrlC + TestModelWindowResize | ✅ `no non-test Go files` | ✅ After model.go | — |
| 10.2 impl Model / Init / Update / View | n/a (impl task) | ✅ All 4 Update() tests pass | — |
| 10.3 TestTUISnapshot | — | ✅ teatest/v2 compiles, golden created on first run | PRIMARY PATH confirmed |
| 10.4 TestTUIViewGolden + TestTUIViewEmptyState | — | ✅ Golden auto-created; empty-state assertion passes | — |
| 11.1 main.go | n/a (composition root, no unit test) | ✅ `go build ./cmd/clyde` exits 0 | — |

### teatest/v2 status

**PRIMARY PATH ACTIVE** — teatest/v2 (`v2.0.0-20260430013151-79116d1f37bd`) compiled and ran successfully. No fallback activated.

### Bubble Tea v2 API Discoveries

The following v2 API differences from v1 were discovered and documented during this batch:

1. **`Init() tea.Cmd`** — v2 `Init` returns only `tea.Cmd`, not `(tea.Model, tea.Cmd)`. The model is not returned from Init.
2. **`View() tea.View`** — v2 `View()` returns a `tea.View` struct (not a string). Use `tea.NewView(s string) tea.View` to create it. The content is at `tea.View.Content`.
3. **`AltScreen` is declarative on the View** — Set `v.AltScreen = true` on the returned `tea.View`, not a program option. `tea.WithAltScreen()` does NOT exist in v2.
4. **`KeyPressMsg` is a concrete struct** — `tea.KeyMsg` is now an interface (covers both press and release). Use `tea.KeyPressMsg` for key-down events. Pattern: `case tea.KeyPressMsg:`.
5. **ctrl+c detection** — `msg.Code == 'c' && msg.Mod == tea.ModCtrl`. The old `tea.KeyCtrlC` constant does not exist; use `ModCtrl` from `charm.land/bubbletea/v2`.
6. **`tea.Program.Run()` takes no context** — Context is passed via `tea.WithContext(ctx)` program option, not as a Run() parameter.
7. **`tea.WithAltScreen()` removed** — Not a program option in v2. AltScreen is per-view.
8. **`golden` package `--update` flag conflict** — `github.com/charmbracelet/x/exp/golden` registers its own `-update` flag. Tests that define their own `-update` flag will panic with "flag redefined". Resolution: check `flag.Lookup("update")` before defining, or use `isUpdateMode()` helper that always reads from the flag registry.
9. **`tea.View.Content` field** — The rendered string is at `v.Content`. Other fields: `AltScreen`, `WindowTitle`, `Cursor`, `MouseMode`, `KeyboardEnhancements`, `ProgressBar`, `OnMouse`, `BackgroundColor`, `ForegroundColor`.

### Deviations from Design

**`IsQuitting()` exported**: Design did not call for an exported quitting accessor. Added to enable test assertions without reflection or internal package access. No functional impact.

**`ViewModelMsg` exported**: Design was silent on visibility. Exported to allow test injection (bypassing Init → Cmd pipeline for deterministic View() tests). This is the correct pattern for testing Bubble Tea models without a full program loop.

**`UsecaseRunner` interface defined in tui package**: Rather than importing `*watchsession.WatchSession` directly, a minimal `UsecaseRunner` interface with a single `Run` method is defined in the `tui` package. This keeps the TUI adapter decoupled from the concrete use case type and makes it easy to swap in a stub in tests.

**`update` flag conflict mitigation**: The `golden` package registers `-update` at init time. Our test file uses `flag.Lookup("update")` to detect this and falls back to a shared registry reference. The `isUpdateMode()` helper always reads from the flag registry regardless of which package registered it.

**British spelling fixed**: `behaviour` → `behavior` in doc comments (misspell linter, US locale). Applied in `cmd/clyde/main.go` and `model_test.go`.

### Test Output Summary (Batch E final)

```
=== RUN   TestModelQuitOnQ
--- PASS: TestModelQuitOnQ (0.00s)
=== RUN   TestModelQuitOnCtrlC
--- PASS: TestModelQuitOnCtrlC (0.00s)
=== RUN   TestModelWindowResize
--- PASS: TestModelWindowResize (0.00s)
=== RUN   TestModelEscDoesNotQuit
--- PASS: TestModelEscDoesNotQuit (0.00s)
=== RUN   TestTUISnapshot
    model_test.go:244: created golden file testdata/snapshot.golden (first run)
--- PASS: TestTUISnapshot (0.05s)
=== RUN   TestTUIViewGolden
    model_test.go:283: created golden file testdata/view.golden (first run)
--- PASS: TestTUIViewGolden (0.00s)
=== RUN   TestTUIViewEmptyState
--- PASS: TestTUIViewEmptyState (0.00s)
PASS
ok  github.com/clyde-tui/clyde/internal/adapters/tui  0.461s

Full suite:
?   github.com/clyde-tui/clyde/cmd/clyde                      [no test files]
ok  github.com/clyde-tui/clyde/internal/adapters/jsonl         0.453s
ok  github.com/clyde-tui/clyde/internal/adapters/systemclock   0.770s
ok  github.com/clyde-tui/clyde/internal/adapters/tui           1.946s
ok  github.com/clyde-tui/clyde/internal/application/watchsession 2.211s
ok  github.com/clyde-tui/clyde/internal/domain/event           1.338s
ok  github.com/clyde-tui/clyde/internal/domain/project         1.034s
ok  github.com/clyde-tui/clyde/internal/domain/session         2.518s
ok  github.com/clyde-tui/clyde/internal/domain/usage           1.702s
?   github.com/clyde-tui/clyde/internal/ports                  [no test files]
```

### Lint/Vet/Fmt Output Summary (Batch E final)

```
golangci-lint run ./...
0 issues.

go vet ./...
(exit 0, no output)

gofmt -l .
(exit 0, empty output — all files formatted)
```

Two misspell fixes applied: `behaviour` → `behavior` in `cmd/clyde/main.go` and `model_test.go`.

### Binary

```
go build -o /tmp/clyde-test ./cmd/clyde
ls -la /tmp/clyde-test
-rwxr-xr-x  1 vladpb  staff  5588146  Apr 30 12:24 /tmp/clyde-test
```

### Files Created

| File | Description |
|------|-------------|
| `internal/adapters/tui/model.go` | Model struct, New(), Init(), Update(), View(), IsQuitting(), formatEventLine(); UsecaseRunner interface; ViewModelMsg; Bubble Tea v2 model implementation |
| `internal/adapters/tui/model_test.go` | TestModelQuitOnQ, TestModelQuitOnCtrlC, TestModelWindowResize, TestModelEscDoesNotQuit, TestTUISnapshot, TestTUIViewGolden, TestTUIViewEmptyState; compile-time tea.Model assertion; stub fakes |
| `internal/adapters/tui/testdata/view.golden` | Golden file for View() output with 3-event fixture (auto-created on first run) |
| `internal/adapters/tui/testdata/snapshot.golden` | Golden file for teatest snapshot (ANSI escape sequence output; auto-created on first run) |
| `cmd/clyde/main.go` | Composition root: wires jsonl source + systemclock + watchsession + tui.Model; runs tea.NewProgram(model).Run() |

## Next batch

Batch F: Phase 12 (Verification — sdd-verify gate).
