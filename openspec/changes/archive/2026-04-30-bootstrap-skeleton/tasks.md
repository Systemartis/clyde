# Tasks: bootstrap-skeleton

## Strategy

This change is a greenfield walking skeleton executed under Strict TDD. Every domain type, use case, and adapter ships with a failing test committed BEFORE its production file. The test-before-prod ordering is not optional — the first commit to land is `usage_test.go` with a failing `TestUsageZeroIsIdentity`. Infrastructure (module init, dependencies) comes first because nothing else compiles without it; it has no test pairing.

Batch boundaries follow natural dependency edges: infra is self-contained, domain types are pure and can be TDD'd in isolation, ports need domain types to compile, the application use case needs ports, adapters need ports and domain, the TUI needs the application layer, and the composition root wires everything. Verification is a final gate that must see a fully compiled, lint-clean, test-passing tree.

---

## Phase 1: Infrastructure

### 1.1 Initialize Go module
- [x] Run `go mod init github.com/clyde-tui/clyde`
- [x] Confirm `go.mod` contains `module github.com/clyde-tui/clyde` and `go 1.26`
- **Acceptance**: `go.mod` exists with the correct module path and Go directive

### 1.2 Add runtime dependencies
- [x] Run `go get github.com/charmbracelet/bubbletea/v2@v2.0.6` — NOTE: module path changed to `charm.land/bubbletea/v2`; see apply-progress.md
- [x] Run `go get github.com/charmbracelet/lipgloss/v2@v2.0.3` — NOTE: module path changed to `charm.land/lipgloss/v2`
- [x] Confirm both appear in `go.mod` at the pinned versions
- **Acceptance**: `go.mod` lists both dependencies at exact pinned versions; `go.sum` populated

### 1.3 Add test-only dependency (teatest/v2)
- [x] Run `go get github.com/charmbracelet/x/exp/teatest/v2@v2.0.0-20260430013151-79116d1f37bd`
- [x] Confirm it appears in `go.mod` with `// indirect` or direct as appropriate
- **Acceptance**: `go.mod` contains the teatest pseudo-version; `go.sum` updated

### 1.4 Create testdata directory stubs for JSONL adapter
- [x] Create `internal/adapters/jsonl/testdata/` directory with a `.gitkeep` placeholder
- [x] Create `internal/adapters/jsonl/testdata/multi-session/` subdirectory
- **Acceptance**: directories exist; `fd . internal/adapters/jsonl/testdata` shows the structure

### 1.5 Verify .gitignore covers binary
- [x] Check `.gitignore` already ignores `clyde` binary (or add the entry if absent) — `/clyde` already present; no change needed
- **Acceptance**: running `clyde` binary after `go build` won't appear as untracked in `git status`

---

## Phase 2: Domain — Usage

### 2.1 Write failing test: Usage.Zero identity (left)
- [x] Create `internal/domain/usage/usage_test.go`
- [x] Add `TestUsageZeroIsIdentity` table case: `Zero.Add(a) == a` for several `a` values
- [x] Run `go test ./internal/domain/usage/` → FAILED with "no non-test Go files" (confirmed RED)
- **Acceptance**: test file exists; `go test` fails with "cannot find package" or compile error

### 2.2 Implement Usage struct and Zero
- [x] Create `internal/domain/usage/usage.go` with `Usage` struct (four `int64` fields: `Input`, `Output`, `CacheRead`, `CacheCreation`) and `Zero() Usage`; also implemented `Add()` (required to satisfy left-identity test)
- [x] Run `go test ./internal/domain/usage/` → `TestUsageZeroIsIdentity` GREEN
- **Acceptance**: `TestUsageZeroIsIdentity` is green

### 2.3 Write failing test: Usage.Zero identity (right) and Add non-mutation
- [x] Add `TestUsageZeroIsIdentityRight` and `TestUsageAddIsNonMutating` to `usage_test.go`
- [x] Tests passed immediately — Add was already implemented in 2.2 (task note clause: "if Add already satisfies, skip ensure-fail step and document why")
- **Acceptance**: new tests exist and fail for the right reason (missing `Add`) — NOTE: Add existed; tests skipped RED step per spec note

### 2.4 Implement Usage.Add
- [x] `func (u Usage) Add(other Usage) Usage` implemented in 2.2 (needed for left-identity test to compile)
- [x] All tests pass
- **Acceptance**: right-identity and non-mutation tests green

### 2.5 Write failing tests: commutativity, associativity, counter accumulation
- [x] Added `TestUsageCommutativity` (4 cases), `TestUsageAssociativity` (3 cases), `TestUsageCounterAccumulation` (3 cases including spec example: a{3,4,1,2}+b{1,2,3,4}={4,6,4,6})
- [x] Tests passed immediately — Add was already implemented; skipped RED step per spec note
- [x] `go test ./internal/domain/usage/` exits 0; 17 total tests all GREEN
- **Acceptance**: all five Usage invariants pass; `go test ./internal/domain/usage/` exits 0

---

## Phase 3: Domain — Event

### 3.1 Write failing test: Kind enum and OpaquePayload preservation
- [x] Create `internal/domain/event/event_test.go`
- [x] Add `TestKindString` table: `KindUser`, `KindAssistant` stringify correctly; unknown kind string round-trips through `OpaqueKind` or is preserved
- [x] Add `TestOpaquePayloadPreservesRaw`: `OpaquePayload{Raw: someBytes}.Raw` equals `someBytes`
- [x] Run `go test ./internal/domain/event/` → FAIL (no package)
- **Acceptance**: test file exists; fails with compile error

### 3.2 Implement Event, Kind enum, Payload sealed interface
- [x] Create `internal/domain/event/event.go`
- [x] Define `Kind` as `string`-based type with constants `KindUser = "user"`, `KindAssistant = "assistant"`, `KindOpaque = "opaque"`
- [x] Define `Payload` sealed interface (unexported marker method `isPayload()`)
- [x] Define `UserPayload{}`, `AssistantPayload{Usage usage.Usage}`, `OpaquePayload{Raw []byte}` implementing `Payload`
- [x] Define `Event` struct: `ID string`, `Timestamp time.Time`, `Kind Kind`, `SessionID string`, `ParentID string`, `Payload Payload`
- [x] Run `go test ./internal/domain/event/` → all tests pass
- **Acceptance**: tests green; domain imports only `time` and `internal/domain/usage` (no os, no json)

### 3.3 Write failing test: Event construction helper
- [x] Add `TestNewEvent` in `event_test.go`: constructor populates all fields correctly including nil ParentID for root events
- [x] Run test → FAIL (no constructor)
- **Acceptance**: test exists and fails

### 3.4 Implement NewEvent constructor
- [x] Add `func NewEvent(id string, ts time.Time, kind Kind, sessionID, parentID string, payload Payload) Event`
- [x] Run tests → green
- **Acceptance**: `TestNewEvent` passes; `go test ./internal/domain/event/` exits 0

---

## Phase 4: Domain — Session

### 4.1 Write failing test: Session type and Summary ordering helper
- [x] Create `internal/domain/session/session_test.go`
- [x] Add `TestSessionIDIsString`: `session.ID` wraps a string, accessible
- [x] Add `TestSummaryOrderByLastActivity`: given a slice of `Summary` values with different `LastActivity` timestamps, the slice sorted by `Summary.LastActivity` descending is correct (table-driven, 3 cases)
- [x] Run `go test ./internal/domain/session/` → FAIL (no package)
- **Acceptance**: test file exists; compile error

### 4.2 Implement Session domain types
- [x] Create `internal/domain/session/session.go`
- [x] Define `ID` as `type ID string`
- [x] Define `Summary` struct: `ID ID`, `LastActivity time.Time`
- [x] Export nothing else for V1 (Session entity itself not needed yet — WatchSession operates on Summary + Event)
- [x] Run `go test ./internal/domain/session/` → green
- **Acceptance**: all session tests pass; no adapters, no stdlib I/O imported

---

## Phase 5: Domain — Project

### 5.1 Write failing test: Project type
- [x] Create `internal/domain/project/project_test.go`
- [x] Add `TestProjectCWD`: `project.New("/Users/foo/bar").CWD()` returns `"/Users/foo/bar"`
- [x] Run `go test ./internal/domain/project/` → FAIL (no package)
- **Acceptance**: test file exists; fails with compile error

### 5.2 Implement Project type
- [x] Create `internal/domain/project/project.go`
- [x] Define `Project` struct with unexported `cwd string` field
- [x] Add `func New(cwd string) Project` and `func (p Project) CWD() string`
- [x] Run `go test ./internal/domain/project/` → green
- **Acceptance**: `TestProjectCWD` passes

---

## Phase 6: Ports

### 6.1 Create SessionSource port
- [x] Create `internal/ports/sessionsource.go` with `SessionSource` interface (two methods: `Sessions(ctx, cwd string) ([]session.Summary, error)` and `Events(ctx, id session.ID) ([]event.Event, error)`)
- [x] No test needed — interfaces carry no behavior; depguard check covers correctness
- **Acceptance**: `go build ./internal/ports/...` succeeds

### 6.2 Create Clock port
- [x] Create `internal/ports/clock.go` with `Clock` interface (`Now() time.Time`)
- [x] No test needed
- **Acceptance**: `go build ./internal/ports/...` succeeds

### 6.3 Verify depguard on ports layer
- [x] Run `golangci-lint run ./internal/ports/...`
- [x] Confirm zero `depguard` violations (ports must NOT import adapters)
- **Acceptance**: lint exits 0 with no depguard errors

---

## Phase 7: Application — WatchSession

### 7.1 Write failing tests: all six spec scenarios
- [x] Create `internal/application/watchsession/watchsession_test.go`
- [x] Define in-file `stubSessionSource` implementing `ports.SessionSource` (uses fields for canned responses)
- [x] Define in-file `stubClock` implementing `ports.Clock`
- [x] Add table-driven `TestWatchSession` with subtests for each scenario:
  - `chronological_order`: T1<T2<T3 → result preserves ascending order
  - `last_N_truncation`: >5 events → only last 5 returned
  - `opaque_kind_preserved`: events with unknown kind → included, no panic
  - `no_sessions`: empty source → empty result, no error
  - `multi_session_focus`: two sessions with different latest events → session with most-recent event selected
  - `fewer_than_N`: 3 events → all 3 returned, no padding
- [x] Run `go test ./internal/application/watchsession/` → FAIL (no package)
- **Acceptance**: test file exists with all six scenarios; fails with compile error

### 7.2 Define SessionView type
- [x] Create `internal/application/watchsession/watchsession.go`
- [x] Define `SessionView` struct: `FocusedSession session.ID`, `Events []event.Event`, `Now time.Time`, `EmptyReason string`
- [x] Define `WatchSession` struct holding `source ports.SessionSource` and `clock ports.Clock`
- [x] Add `func New(source ports.SessionSource, clock ports.Clock) WatchSession`
- [x] Add `func (w WatchSession) Run(ctx context.Context, cwd string) (SessionView, error)` — stub returning empty `SessionView{}` to make tests compile
- [x] Run `go test` → tests compile but scenarios fail (logic not implemented)
- **Acceptance**: tests compile and fail on assertions (not on compilation)

### 7.3 Implement WatchSession.Run logic
- [x] Implement: call `source.Sessions`, select session with most-recent `LastActivity`, call `source.Events` for that session, sort ascending by `Timestamp`, take last min(N, len) events (N=5), call `clock.Now()`, populate `SessionView`
- [x] Run `go test ./internal/application/watchsession/` → all six subtests pass
- **Acceptance**: all six spec scenarios green; no adapters or TUI types imported

---

## Phase 8: Adapters — systemclock

### 8.1 Write failing test: systemclock returns UTC
- [x] Create `internal/adapters/systemclock/systemclock_test.go`
- [x] Add `TestSystemClockReturnsUTC`: `Clock{}.Now().Location()` equals `time.UTC`
- [x] Run `go test ./internal/adapters/systemclock/` → FAIL (no package)
- **Acceptance**: test file exists; fails with compile error

### 8.2 Implement SystemClock adapter
- [x] Create `internal/adapters/systemclock/systemclock.go`
- [x] Define `Clock struct{}` with `func (Clock) Now() time.Time { return time.Now().UTC() }`
- [x] Confirm `Clock` satisfies `ports.Clock` (add `var _ ports.Clock = Clock{}` compile-time check)
- [x] Run `go test ./internal/adapters/systemclock/` → green
- **Acceptance**: `TestSystemClockReturnsUTC` passes; compile-time interface check in place

---

## Phase 9: Adapters — jsonl

### 9.1 Commit JSONL fixture files
- [x] Create `internal/adapters/jsonl/testdata/simple-user-assistant.jsonl` — 3–5 hand-authored lines with `user` and `assistant` event types, valid UUID, timestamp, sessionId, parentUuid fields
- [x] Create `internal/adapters/jsonl/testdata/large-thinking.jsonl` — one `assistant` event line with a `message.content` block padded to exceed 64KB to assert Scanner buffer size
- [x] Create `internal/adapters/jsonl/testdata/unknown-types.jsonl` — mix of `user`, `assistant`, and two lines with `type: "ai-title"` and `type: "queue-operation"` unknown kinds
- [x] Create `internal/adapters/jsonl/testdata/multi-session/session-a.jsonl` and `session-b.jsonl` — two files with different mtimes (touch one to be older) to assert multi-session ordering
- **Acceptance**: four fixture paths exist; `fd . internal/adapters/jsonl/testdata` lists them; files are valid JSON-lines

### 9.2 Write failing tests: JSONL adapter — all scenarios
- [x] Create `internal/adapters/jsonl/jsonl_test.go`
- [x] Add `TestEncodeCWD` table: `/Users/vladpb/work/Personal/clyde` → `-Users-vladpb-work-Personal-clyde` (and a few more path variants)
- [x] Add `TestSessionsDecodeSimpleFixture`: loads `simple-user-assistant.jsonl` via `t.TempDir()`, calls `Sessions()` and `Events()`, asserts correct ID, event count, chronological order
- [x] Add `TestScannerHandlesLargeLines`: loads `large-thinking.jsonl`, `Events()` must NOT return `bufio.ErrTooLong`
- [x] Add `TestUnknownKindPreservedAsOpaque`: loads `unknown-types.jsonl`, asserts `ai-title` and `queue-operation` events have `OpaquePayload` and are included in result
- [x] Add `TestMissingDirectoryReturnsEmpty`: calls `Sessions()` with a non-existent cwd → empty slice, no error
- [x] Add `TestMultiSessionOrderedByLastActivity`: uses `multi-session/` fixture directory, asserts `Sessions()` returns most-recently-modified session first
- [x] Run `go test ./internal/adapters/jsonl/` → FAIL (no package)
- **Acceptance**: test file with all six test functions exists; fails with compile error

### 9.3 Implement JSONL adapter
- [x] Create `internal/adapters/jsonl/jsonl.go`
- [x] Implement private `encodeCWD(path string) string` — replace all `/` with `-`, prepend `-`
- [x] Implement `SessionSource` struct with `Sessions(ctx, cwd string)` using `os.ReadDir`, stat mtime, build `[]session.Summary` sorted descending by `LastActivity`; missing directory → empty slice, no error
- [x] Implement `Events(ctx, id session.ID)` using `bufio.Scanner` with 4MB buffer (`Buffer(make([]byte, 0, 64*1024), 4*1024*1024)`); decode envelope struct per line; map `user`/`assistant` to typed payloads; map unknown kinds to `OpaquePayload{Raw: rawLine}`; return events sorted ascending by `Timestamp`
- [x] Add `var _ ports.SessionSource = (*SessionSource)(nil)` compile-time check
- [x] Run `go test ./internal/adapters/jsonl/` → all tests pass
- **Acceptance**: all six jsonl test functions green; `TestEncodeCWD`, `TestScannerHandlesLargeLines`, `TestUnknownKindPreservedAsOpaque`, `TestMissingDirectoryReturnsEmpty`, `TestMultiSessionOrderedByLastActivity` all pass

---

## Phase 10: Adapters — TUI

### 10.1 Write failing tests: Update() behavior (quit keys, resize)
- [x] Create `internal/adapters/tui/model_test.go`
- [x] Add `TestModelQuitOnQ`: create a `Model` with stub usecase; call `m.Update(tea.KeyPressMsg{Code: tea.KeyRunes, Runes: []rune{'q'}})`, assert returned cmd is `tea.Quit` (or `m.quitting == true`)
- [x] Add `TestModelQuitOnCtrlC`: same for `tea.KeyPressMsg{Code: tea.KeyCtrlC}`
- [x] Add `TestModelWindowResize`: `tea.WindowSizeMsg{Width: 80, Height: 24}` → no panic, model returns non-nil model
- [x] Run `go test ./internal/adapters/tui/` → FAIL (no package)
- **Acceptance**: test file exists; fails with compile error

### 10.2 Scaffold TUI Model (compilation spike)
- [x] Create `internal/adapters/tui/model.go`
- [x] Define `Model` struct with fields: `usecase interface{ Run(context.Context, string) (watchsession.SessionView, error) }`, `cwd string`, `view watchsession.SessionView`, `err error`, `quitting bool`
- [x] Implement `Init() tea.Cmd` — returns a `tea.Cmd` that calls `usecase.Run(ctx, cwd)` and sends a `viewModelMsg`
- [x] Implement `Update(msg tea.Msg) (tea.Model, tea.Cmd)` — switch on `viewModelMsg`, `tea.KeyPressMsg` (q / ctrl+c → `tea.Quit`), `tea.WindowSizeMsg`, default no-op
- [x] Implement `View() tea.View` — build a string with title + event lines or empty state; return `tea.NewView(s)` (adjust based on actual `tea.View` struct shape found at compile time)
- [x] Add event line format: `15:04:05Z  <kind>  <summary>` for known kinds; `15:04:05Z  <kind>  (opaque)` for opaque
- [x] Run `go build ./internal/adapters/tui/` → must compile clean (discover and fix any `tea.View` field surprises here)
- [x] Run `go test ./internal/adapters/tui/` → Update() tests pass
- **Acceptance**: `TestModelQuitOnQ`, `TestModelQuitOnCtrlC`, `TestModelWindowResize` green; package compiles

### 10.3 Write TUI snapshot test (teatest/v2 primary path)
- [x] Create `internal/adapters/tui/testdata/` directory
- [x] Add `TestTUISnapshot` in `model_test.go` using `teatest.NewTestModel(t, m)`:
  - Use a stub usecase that returns a canned `SessionView` with 2 events
  - Wait for the model to emit a `viewModelMsg` (initial render)
  - Assert rendered output contains expected event lines via `teatest.RequireEqualOutput` against `testdata/snapshot.golden`
  - Send quit key; assert program exits cleanly
- [x] Run `go test ./internal/adapters/tui/` — if teatest/v2 compiles and passes, commit golden file
- **Acceptance (primary)**: `TestTUISnapshot` passes; `testdata/snapshot.golden` committed

### 10.4 FALLBACK: direct Update() + golden View() test if teatest/v2 fails
- [x] IF `TestTUISnapshot` in 10.3 fails to compile or panics due to teatest/v2 incompatibility:
  - Remove or skip `TestTUISnapshot`
  - Add `TestTUIViewOutput` in `model_test.go`: create Model with canned SessionView already loaded (bypass Init), call `m.View()`, call `.String()` (or equivalent) on the result, compare against `testdata/view.golden`
  - Create `testdata/view.golden` with expected string output
- [x] Run `go test ./internal/adapters/tui/` → green
- **Acceptance (fallback)**: `TestTUIViewOutput` passes; golden file committed; teatest failure documented in a `// FALLBACK:` comment in the test file

---

## Phase 11: Composition Root

### 11.1 Create cmd/clyde/main.go
- [x] Create `cmd/clyde/main.go` (~30 lines)
- [x] Wire: get cwd via `os.Getwd()`, create `jsonl.NewSessionSource()`, create `systemclock.Clock{}`, create `watchsession.New(source, clock)`, create `tui.NewModel(usecase, cwd)`, run `tea.NewProgram(model).Run()`
- [x] No unit test — covered by `go build` smoke + TUI tests through fakes
- [x] Run `go build ./cmd/clyde` → binary produced
- **Acceptance**: `go build ./cmd/clyde` succeeds; binary exists at `./clyde` or `./cmd/clyde/clyde`

---

## Phase 12: Verification

### 12.1 Full test suite
- [x] Run `go test ./...`
- [x] All tests pass with exit code 0
- **Acceptance**: zero failures, zero panics

### 12.2 Go vet
- [x] Run `go vet ./...`
- [x] Zero issues
- **Acceptance**: exits 0

### 12.3 gofmt check
- [x] Run `gofmt -l .`
- [x] Output is empty (no unformatted files)
- **Acceptance**: empty output

### 12.4 golangci-lint full run
- [x] Run `golangci-lint run ./...`
- [x] Zero issues; specifically confirm depguard passes for domain-pure, application-via-ports, and ports-pure rules
- **Acceptance**: lint exits 0; no depguard violations

### 12.5 Build smoke test
- [x] Run `go build ./cmd/clyde`
- [x] Binary produced without errors
- **Acceptance**: `clyde` binary exists and is executable

### 12.6 Integration smoke run
- [x] Run `./clyde` (or `go run ./cmd/clyde`) from the repo root (which has real Claude Code sessions under `~/.claude/projects/-Users-vladpb-work-Personal-clyde/`)
- [x] TUI renders at least one event without panic; `q` exits cleanly
- **Acceptance**: binary runs, renders output, exits 0 on `q`

---

## Batch suggestions for /sdd-apply

Natural batch boundaries following dependency order and rough time estimate per session:

- **Batch A** — Phase 1 (infrastructure) + Phase 2 (Usage TDD)
  *Why together*: infra has no logic, Usage is pure domain with no imports — both batch nicely into "get the module standing and TDD the first domain type"

- **Batch B** — Phases 3–5 (Event TDD, Session TDD, Project TDD)
  *Why together*: three pure domain types with no cross-dependencies; all TDD pairs; each is small

- **Batch C** — Phase 6 (Ports) + Phase 7 (WatchSession TDD)
  *Why together*: ports are prerequisites for the use case; both are fast; depguard check closes the batch

- **Batch D** — Phases 8–9 (systemclock + jsonl TDD)
  *Why together*: adapters are independent of each other; fixtures and implementation for jsonl are the bulk of the batch

- **Batch E** — Phases 10–11 (TUI TDD + main.go composition root)
  *Why together*: main.go is 30 lines and only compiles once TUI exists; teatest/v2 fallback path may add a small detour but stays in the same batch

- **Batch F** — Phase 12 (Verification)
  *Why alone*: final gate; must run after all prior batches complete; any lint or test failures here feed back to the owning batch
