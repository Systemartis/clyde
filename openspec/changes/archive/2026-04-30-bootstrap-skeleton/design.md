# Design: bootstrap-skeleton

## Verified facts (Context7 + go list)

| Fact | Value | Source |
|---|---|---|
| Bubble Tea v2 module path | `github.com/charmbracelet/bubbletea/v2` | Context7 `/v2.md:42` + `go list -m -versions` returns `v2.0.0` … `v2.0.6` |
| Bubble Tea v2 latest stable | **v2.0.6** | `go list -m -versions github.com/charmbracelet/bubbletea/v2` |
| Lipgloss v2 module path | `github.com/charmbracelet/lipgloss/v2` | `go list -m -versions` returns `v2.0.0` … `v2.0.3` |
| Lipgloss v2 latest stable | **v2.0.3** | `go list -m -versions github.com/charmbracelet/lipgloss/v2` |
| teatest v2 module path | `github.com/charmbracelet/x/exp/teatest/v2` | `go list -m -json …@latest` resolves; pseudo-version `v2.0.0-2026MMDD…` (rolling) |
| teatest v2 — latest pseudo-version sampled | `v2.0.0-20260430013151-79116d1f37bd` | `go list -m -json` output today (2026-04-30) |
| Go toolchain | `go1.26.2 darwin/arm64` | `go version` |
| `go.mod` directive | `go 1.26` | matches local toolchain; `.golangci.yml` already targets this module |

### Bubble Tea v2 breaking changes that affect this design (from Context7 research)

- `KeyMsg` is now an **interface**; concrete `KeyPressMsg` and `KeyReleaseMsg` implement it. Update handlers MUST type-switch on `tea.KeyPressMsg`, NOT on `tea.KeyMsg` (the latter still compiles but won't match like before).
- `View()` returns `tea.View` (a struct), not `string`. AltScreen is declarative: `v := tea.NewView("..."); v.AltScreen = true; return v`. The legacy `tea.WithAltScreen()` program option is removed.
- `tea.NewProgram(model, opts...)` and the core `Init() tea.Cmd` / `Update(tea.Msg) (tea.Model, tea.Cmd)` shape are unchanged in spirit. `tea.Batch`, `tea.Tick`, `tea.Quit` still exist with the same semantics.
- The `Cursor` API is new and structured (`tea.CursorPositionMsg`, `CursorBlock`, etc.). Bootstrap-skeleton does not touch it.

> **Confidence note**: `KeyPressMsg` and `tea.View`-returning `View()` are confirmed via Context7 research mode against `/v2.md`. The exact `tea.View` field set (beyond `AltScreen`) is not exhaustively documented here — first apply task verifies by compilation.

---

## Architecture Decision Records

### ADR-001: Hexagonal layering with explicit "don't wrap" list

**Decision**: Domain (`internal/domain/`), application (`internal/application/`), ports (`internal/ports/`), adapters (`internal/adapters/*`), composition root (`cmd/clyde/main.go`). Layer rules enforced by `.golangci.yml` `depguard`.

**Rationale**: Hexagonal forces I/O behind interfaces named after their *semantics* (not their tech). Tests stay fast, swapping JSONL for a daemon-fed stream later costs one adapter. depguard catches drift at lint time — no runtime surprise.

**Consequences**: Domain cannot import `os`, `time`, `net/http`, `bubbletea`, `lipgloss`, `fsnotify`, or anything under `internal/adapters` / `internal/application`. Application cannot import adapters or UI.

**What we DO wrap**: `SessionSource` (JSONL stream has Session/Event semantics), `Clock` (tests need deterministic time).

**What we do NOT wrap** (hexagonal-as-religion territory):
- No `FileSystem` port for `os.ReadFile`. The JSONL stream IS the semantic concept; `os` is incidental and already encapsulated inside the JSONL adapter.
- No `JSONDecoder` port. `encoding/json` is stdlib, well-tested. JSON parsing knowledge belongs to the adapter that owns the format.
- No `Logger` port for V1. Adapters use `log/slog` directly. We add the port the day we need structured log routing — not before.
- No `Renderer` port for the TUI. Bubble Tea is the renderer; the TUI adapter IS the rendering layer.

---

### ADR-002: Usage as immutable value object with `Add`

**Decision**: `Usage` is a struct with four `int64` token fields. `Add(other Usage) Usage` returns a new value. No mutation. `Zero()` constructor returns the identity element.

**Rationale**: Token accumulation is a monoid. Modeling it as a value type makes the algebraic laws (identity, commutativity, associativity) testable as table-driven domain tests with no ports involved. A `UsageService.Add(a, b)` would require dependency injection for a pure function — pure overhead.

**Consequences**: Spec contains four explicit invariants (Zero is identity, Add is non-mutating, commutativity, associativity). Tests are pure Go, no fixtures, run in microseconds.

---

### ADR-003: SessionSource returns slices, not iterators (V1)

**Decision**: V1 `SessionSource` returns `[]domain.SessionSummary` and `[]domain.Event` (eager slices). Iterators (`iter.Seq`, channels) are deferred until `live-tail-fsnotify`.

**Rationale**: Walking skeleton reads-on-startup with bounded data (one JSONL, last 5 events shown). A slice is the simplest readable shape; tests assert order with plain `reflect.DeepEqual` / `cmp.Diff`. `iter.Seq` only earns its complexity once we have a long-lived stream that must yield incrementally — not yet.

**Consequences**: When live-tail lands, the port grows a second method (`Tail(ctx) iter.Seq[Event]`) rather than rewriting the V1 method. Existing callers unaffected.

---

### ADR-004: Event uses a `Kind` enum + sealed payload variants

**Decision**: `domain.Event` carries the common envelope (`ID`, `Timestamp`, `Kind`, `SessionID`, `ParentID`) as typed fields. The optional payload is a sealed interface with concrete variants for known kinds (`UserPayload`, `AssistantPayload`) and an `OpaquePayload{Raw []byte}` for unknown kinds. The adapter maps JSONL `type` strings → `Kind` enum and decodes/preserves the payload accordingly. Domain stays unaware of `json.RawMessage` — that's an adapter type.

**Rationale**: Spec mandates "unknown kinds MUST be preserved as opaque". A naked `map[string]any` leaks JSON semantics into the domain. A sealed interface keeps the domain pure-Go, gives exhaustive `switch` coverage on known kinds, and stuffs unknown payloads into a `[]byte` with no parsing commitment. The bytes are JSON in practice but the domain doesn't claim it.

**Consequences**: Adding a new known kind = add an enum value + a payload variant + adapter mapping. No domain rewrite. The `Raw []byte` for opaque events keeps payload available for future panels (they decode it themselves) without the domain importing `encoding/json`.

---

### ADR-005: Clock port — minimal `Now() time.Time`

**Decision**: `Clock` is a one-method interface returning `time.Time`. The systemclock adapter wraps `time.Now().UTC()`. Tests use a `FakeClock` defined in `_test.go` files (no test-only package needed for V1).

**Rationale**: We wrap `time.Now()` because (a) the spec mandates monotonicity for display, (b) the future "Xs ago" rendering and live-tail age computations need deterministic test time, (c) depguard already denies `time` direct calls in domain, so the port is the only legal path. One method is enough — `Since(t)` etc. are pure helpers callable on the returned value.

**Consequences**: Application and domain code never call `time.Now()`. The systemclock adapter is the single line `func (s SystemClock) Now() time.Time { return time.Now().UTC() }`. `FakeClock` is trivial: a struct with a fixed `time.Time` field. UTC normalization happens at the adapter boundary — the domain never reasons about timezones.

---

### ADR-006: TUI Model lives in the adapter, not the application layer

**Decision**: The Bubble Tea root model lives at `internal/adapters/tui/`. The application layer's `WatchSession` use case returns a plain `domain.SessionView` value (immutable, no Tea types). The TUI adapter consumes that value in its `View()` and turns it into a `tea.View`.

**Rationale**: depguard already forbids `bubbletea` imports in `internal/application/**`. Beyond that lint rule, the design intent is: the application produces *what to render* (data); the TUI decides *how to render* (presentation). When a future change adds a non-Tea front-end (web dashboard, log dump), the application layer is reused untouched.

**Consequences**: `WatchSession.Run(ctx, project) (SessionView, error)` is the contract. `SessionView` is a domain/application value — fields like `FocusedSession SessionID`, `Events []Event`, `Now time.Time`, `EmptyReason string`. The TUI adapter wraps a `WatchSessionUseCase` and a `Clock` reference inside its model; on `Init()` it returns a `tea.Cmd` that calls `Run` and emits a `viewModelMsg` carrying the `SessionView`.

---

## Port interfaces (illustrative — final shape locked in /sdd-tasks)

```go
// internal/ports/sessionsource.go
package ports

import (
    "context"

    "github.com/clyde-tui/clyde/internal/domain/event"
    "github.com/clyde-tui/clyde/internal/domain/session"
)

// SessionSource discovers Sessions for a Project and reads their Events.
// Implementations MUST surface unknown event kinds as opaque events
// and MUST NOT mutate underlying storage.
type SessionSource interface {
    // Sessions returns all Sessions belonging to the given Project working
    // directory, ordered by latest activity descending. May be empty.
    Sessions(ctx context.Context, projectCWD string) ([]session.Summary, error)

    // Events returns all Events for the given Session in strictly ascending
    // chronological order. May be empty.
    Events(ctx context.Context, id session.ID) ([]event.Event, error)
}
```

```go
// internal/ports/clock.go
package ports

import "time"

// Clock returns the current time in UTC. Successive calls MUST NOT regress.
type Clock interface {
    Now() time.Time
}
```

---

## Adapter strategies

### JSONL adapter — `internal/adapters/jsonl/`

- **Project directory resolution**: encode the absolute project cwd by replacing every `/` with `-` and prepending `-`, then joining under `~/.claude/projects/<encoded>/`. Example: `/Users/vladpb/work/Personal/clyde` → `-Users-vladpb-work-Personal-clyde`. **Observational, not Anthropic-documented**; the encoding lives in exactly one private function in this package and is asserted by a fixture test.
- **Session discovery**: `os.ReadDir` the encoded directory, filter `*.jsonl`, stat each file for mtime, build `[]session.Summary{ID: filename-without-ext, LastActivity: mtime}` sorted descending by mtime. Empty directory → empty slice, no error. Missing directory → empty slice, no error (project never had a Claude session).
- **Event decoding**: `bufio.Scanner` (default 64KB buffer raised to 4MB via `Buffer(make([]byte, 0, 64*1024), 4*1024*1024)`; assistant events with thinking blocks can exceed 64KB) line-by-line. Each line: first decode an `envelope` struct with the strict-required fields (`uuid`, `type`, `timestamp`, `sessionId`, `parentUuid`); on missing required fields return a wrapped error naming the file path and line number. Per-type variable fields (`message`, `toolUseResult`, etc.) live in a separate `payload json.RawMessage` field; we hand the raw bytes to the kind-specific decoder.
- **Kind mapping**: known `type` strings (`user`, `assistant`) → typed payload struct. Unknown types → `event.OpaquePayload{Raw: rawLine}` with the entire line preserved (not just the `payload` field — keeps the door open for fields the envelope didn't anticipate).
- **Out of scope for V1**: `fsnotify`, polling, partial-line buffering across reads, schema-version detection. These come with `live-tail-fsnotify`.

### SystemClock adapter — `internal/adapters/systemclock/`

Trivial. One file, ~10 lines:

```go
package systemclock

import "time"

type Clock struct{}

func (Clock) Now() time.Time { return time.Now().UTC() }
```

### TUI adapter — `internal/adapters/tui/` (Bubble Tea v2)

- **Model shape**: `Model` struct holds: `usecase WatchSession`, `clock ports.Clock`, `view domain.SessionView` (current snapshot), `err error`, `quitting bool`.
- **`Init() tea.Cmd`**: returns a `tea.Cmd` that executes `usecase.Run(ctx, project)` and yields a `viewModelMsg{view, err}`. V1 is one-shot (no `tea.Tick` polling); a `tea.Cmd` is still used because we want the I/O to happen on Bubble Tea's goroutine, not in `main` before `tea.NewProgram.Run()`.
- **`Update(tea.Msg) (tea.Model, tea.Cmd)`**:
  - `viewModelMsg` → store snapshot, return `m, nil`.
  - `tea.KeyPressMsg` (note: v2 uses `KeyPressMsg`, not `KeyMsg`) → on `q` or `ctrl+c`, set `m.quitting = true` and return `m, tea.Quit`.
  - default → `m, nil`.
- **`View() tea.View`**: builds a string with the title, an empty-state message OR the last 5 events formatted as `<HH:MM:SS UTC>  <kind>  <one-line summary>`, and a footer (`q to quit`). Wraps in `tea.NewView(s)`. `AltScreen = false` for V1 — keeping the rendered output in the scrollback eases manual debugging during the walking-skeleton phase.
- **Event line format (locked)**: `15:04:05Z  assistant  msg_016W…  in=6 out=825` for known kinds; for opaque kinds: `15:04:05Z  <kind>  (opaque)`. No emoji. Single-line per event.

---

## Sequence diagram — startup

```
   user shell                cmd/clyde/main          tui.Model         WatchSession      JSONL adapter      systemclock
       │                            │                    │                   │                  │                  │
       │ ./clyde                    │                    │                   │                  │                  │
       ├───────────────────────────▶│                    │                   │                  │                  │
       │                            │ wire JSONL src      │                   │                  │                  │
       │                            │ wire systemclock    │                   │                  │                  │
       │                            │ wire WatchSession   │                   │                  │                  │
       │                            │ tea.NewProgram(m)   │                   │                  │                  │
       │                            │ p.Run() ───────────▶│                   │                  │                  │
       │                            │                    │ Init()→tea.Cmd     │                  │                  │
       │                            │                    │ ──── runUsecase ──▶│                  │                  │
       │                            │                    │                   │ Sessions(cwd) ───▶│                  │
       │                            │                    │                   │◀──────[] summary ─│                  │
       │                            │                    │                   │ Events(focused) ─▶│                  │
       │                            │                    │                   │◀──────[] event ───│                  │
       │                            │                    │                   │ Now() ────────────────────────────▶ │
       │                            │                    │                   │◀───── time ──────────────────────── │
       │                            │                    │◀── viewModelMsg ──│                  │                  │
       │                            │                    │ Update → View()    │                  │                  │
       │                            │                    │ render last 5 ev   │                  │                  │
       │ press q                    │                    │                    │                  │                  │
       ├───────────────────────────▶│  tea.KeyPressMsg ─▶│ → tea.Quit         │                  │                  │
       │                            │◀── exit ──────────│                    │                  │                  │
```

---

## Testing strategy per layer

| Layer | What to test | Approach |
|---|---|---|
| `internal/domain/usage` | Identity (left/right), commutativity, associativity, `Add` non-mutation, counter accumulation | Table-driven tests on pure funcs. Property-style tables with explicit cases (NO external property-based libs in V1) |
| `internal/domain/event` | `Kind` enum exhaustive switch, opaque payload preserves raw bytes, envelope construction | Minimal — entity is mostly data. One table-driven test of the `Kind` parser |
| `internal/domain/session` | `Summary` ordering helper, `ID` parsing | Trivial table-driven |
| `internal/application/watchsession` | All six spec scenarios (chronological order, last-N truncation, opaque preservation, no-sessions, multi-session focus selection, fewer-than-N) | Table-driven with a stub `SessionSource` defined in `watchsession_test.go` (in-package; no public test fake). Stub `Clock` returns a fixed `time.Time` |
| `internal/adapters/jsonl` | Project-path encoding, single-session decode, multi-session ordering by mtime, missing directory, malformed line surfaces error with file+line, unknown event kind preserved as opaque | Golden-input tests using `t.TempDir()`. JSONL fixtures under `internal/adapters/jsonl/testdata/`: a hand-trimmed real session (~10 events) + a synthetic file with a deliberately unknown `type` |
| `internal/adapters/systemclock` | Returns a `time.Time` whose location is UTC | Single test asserting `Now().Location() == time.UTC` |
| `internal/adapters/tui` | Renders the focused session's last 5 events; renders empty-state when no sessions; quits on `q` and `ctrl+c` | `teatest/v2.NewTestModel(t, m)` driving the model with `Send(tea.KeyPressMsg{...})`. Golden View() output via `teatest.RequireEqualOutput` against `testdata/*.golden`. Fall-back path if teatest/v2 has unforeseen incompatibilities: direct `m.Update(...)` calls with a `tea.KeyPressMsg` literal and assert on the rendered string from `m.View().String()` (or whatever the v2 View struct exposes — verified in first apply task) |
| Composition root | (none — wiring only; covered indirectly by the TUI test running through the use case via fakes; `cmd/clyde/main.go` stays small) | n/a |

**Strict-TDD active**: every test file is committed before the production file in its commit. The first commit is the failing test for `usage.Zero()`.

---

## File changes (greenfield — all new)

| File | Action |
|---|---|
| `go.mod`, `go.sum` | Create |
| `cmd/clyde/main.go` | Create — composition root |
| `internal/domain/session/{session.go,session_test.go}` | Create |
| `internal/domain/event/{event.go,event_test.go}` | Create |
| `internal/domain/usage/{usage.go,usage_test.go}` | Create |
| `internal/application/watchsession/{watchsession.go,watchsession_test.go}` | Create |
| `internal/ports/{sessionsource.go,clock.go}` | Create |
| `internal/adapters/jsonl/{jsonl.go,jsonl_test.go,testdata/...}` | Create |
| `internal/adapters/systemclock/systemclock.go` | Create |
| `internal/adapters/tui/{model.go,model_test.go,testdata/*.golden}` | Create |

Pre-existing files NOT modified: `.golangci.yml`, `.gitignore`, `README.md`, `openspec/`, `.atl/`.

---

## Hexagonal discipline — what we do NOT wrap (recap)

- `time.Now()` direct calls in domain/application — blocked by depguard, routed via `Clock`. **Wrapped intentionally**.
- `os.ReadFile` / `os.ReadDir` / `bufio.Scanner` inside the JSONL adapter — **not wrapped**. The filesystem is incidental to the JSONL stream; the stream IS the port.
- `encoding/json` inside the JSONL adapter — **not wrapped**. Stable stdlib, format-knowledge belongs to the adapter.
- `log/slog` for V1 — **not wrapped**. Add a `Logger` port the day we need routing (web dashboard, file sink). Premature.
- Bubble Tea / lipgloss — **not wrapped**. The TUI adapter IS the rendering layer; abstracting it would be hexagonal-as-religion.

---

## depguard validation (no `.golangci.yml` change)

Existing rules cover everything in this design:
- `domain-pure` denies `os`, `net/http`, `bubbletea`, `lipgloss`, `fsnotify`, `internal/adapters`, `internal/application`. Domain stays pure. ✓
- `application-via-ports` denies `internal/adapters`, `bubbletea`, `lipgloss`. Application stays UI-free and adapter-free. ✓
- `ports-pure` denies `internal/adapters`. Ports stay interface-only. ✓

Optional future tightening (NOT applied in this change): add `time` to the `domain-pure` deny list. Currently, domain values *can* receive a `time.Time` parameter (e.g. `event.Timestamp`); they just can't *call* `time.Now()`. The current ruleset doesn't catch a stray `time.Now()` in domain code. If the apply phase observes a violation slip past, raise a follow-up to add `time` to deny — but only as a separate change.

---

## Risks for /sdd-tasks and /sdd-apply

| Risk | Mitigation |
|---|---|
| `tea.View` v2 struct's exact field set isn't fully documented in Context7 — only `AltScreen` confirmed. The first TUI task may need to adjust `View()` based on actual struct shape | First apply task is "scaffold TUI Model with Init/Update/View, compile against bubbletea/v2, see what `tea.View` exposes". Treat as compilation-driven discovery, not a 2-day spike |
| `teatest/v2` is a pseudo-version (no tagged release) and uses `1.24.2` `go` directive in its mod file. Compatibility with our `go 1.26` is theoretically fine (newer Go reads older `go` directive) but unverified | First TUI test commit pins the exact pseudo-version observed today (`v2.0.0-20260430013151-79116d1f37bd`). If it breaks, fallback path is direct `Model.Update()` table tests + golden `View()` strings — same coverage, no teatest dependency. Documented as ADR addendum in apply phase if triggered |
| JSONL line size — assistant events with thinking blocks have been observed >64KB in real sessions | `bufio.Scanner` buffer raised to 4MB. Apply task includes a `testdata/large-thinking.jsonl` fixture to assert this |
| The `~/.claude/projects/<encoded-cwd>/` encoding is observational. Anthropic could change it without notice | Encoding is a single private function in `jsonl` adapter, asserted by a fixture test. If the scheme changes, exactly one file changes and one test breaks loudly |
| Event payload variants are not exhaustively spec'd for V1 (only `user` and `assistant` are first-class) | All other event types decode to `OpaquePayload{Raw: <line>}` — payload is preserved verbatim. Future changes (`subagent-tree-panel`, `file-changes-panel`) decode the raw bytes on their own without the V1 adapter changing |
| Strict TDD requires test-before-prod for every file. Composition-root wiring (`cmd/clyde/main.go`) is hard to TDD directly | Wiring is intentionally trivial (~30 lines). It's covered transitively by the TUI test running the use case end-to-end through fakes, and a `go build ./cmd/clyde` smoke check is the final task |

---

## Open questions

None blocking. Two clarifications already resolved by this design:

1. **Refresh strategy for V1**: one-shot at startup. No `tea.Tick`. Polling lands with `live-tail-fsnotify`.
2. **Test fixture provenance**: hand-trimmed real session (smaller, controllable) committed under `internal/adapters/jsonl/testdata/`, plus one synthetic file for the unknown-kind case. Real fixtures catch real-world quirks; synthetic fixtures catch deliberate edge cases.
