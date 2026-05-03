# Proposal: bootstrap-skeleton

## Intent

Clyde is a TUI companion for Claude Code that observes session activity, token usage, todos, file changes, and sub-agent topology — all by reading the artifacts Claude Code already writes to disk. Before any of that breadth ships, the project needs a foundation: a Go module, the hexagonal layer tree, lint enforcement of layer boundaries, and a single end-to-end vertical slice that proves the architecture works against real Claude Code data.

This change establishes that foundation as a **walking skeleton** — the thinnest possible end-to-end slice through every hexagonal layer (domain → application → ports → adapters → composition root → TUI). The slice renders the last 5 events of the focused JSONL session for the current cwd in a single Bubble Tea pane. Nothing more. Every panel, every additional adapter, every richer rendering is deferred to a follow-up change with a named scope.

The goal is not a useful product yet — it is a **load-bearing foundation** with all four layers proven, tests in place, and depguard catching layer violations at lint time. Subsequent changes (sessions list, token/cost, todos, file changes, sub-agent tree, compaction warning) plug into this skeleton without re-deciding architecture.

## Scope

### In scope

- `go.mod` initialized at module path `github.com/clyde-tui/clyde` with `go 1.26` directive
- Hexagonal layer tree under `internal/` (domain, application, ports, adapters) plus `cmd/clyde/`
- Domain types (minimal, only what the V1 slice renders): `Session`, `Event`, `Usage` with `Add` value method
- Two ports locked for V1: `SessionSource` and `Clock`
- `jsonl` adapter implementing `SessionSource` — discovers the project's JSONL directory under `~/.claude/projects/<encoded-cwd>/`, decodes events, returns the last N for the focused session
- `systemclock` adapter implementing `Clock`
- `tui` adapter — single Bubble Tea v2 root model with one pane that renders the last 5 events of the focused session
- Application use case `watchsession` — composes `SessionSource` + `Clock` to produce the data the TUI renders
- Composition root in `cmd/clyde/main.go` wiring concrete adapters into the use case and starting the Bubble Tea program
- Strict-TDD coverage: domain unit tests, application use-case tests with fakes, JSONL adapter test against a fixture, `teatest` snapshot for the TUI pane
- `golangci-lint` clean run with the existing `.golangci.yml` depguard rules enforcing layer boundaries
- `go build ./cmd/clyde` produces a working `clyde` binary that, when launched in this repo, reads its own JSONL and renders 5 events

### Out of scope (explicitly deferred — each becomes its own SDD change)

| Deferred capability | Future change |
|---|---|
| Sessions list panel + multi-session focus switching | `sessions-list-panel` |
| Token & cost panel (needs Anthropic pricing config) | `usage-and-cost-panel` |
| Todos panel (separate `~/.claude/todos/*.json` adapter + `TodoSource` port) | `todos-panel` |
| Recent file changes panel (extracts from `Write`/`Edit`/`MultiEdit` tool_use events) | `file-changes-panel` |
| Sub-agent tree panel (`queue-operation` event parsing, non-trivial XML-like content) | `subagent-tree-panel` |
| Compaction warning (token-count-vs-model-limit signal, no native compaction event in JSONL) | `compaction-warning` |
| Live JSONL tailing via `fsnotify` (V1 polls with `tea.Tick`) | `live-tail-fsnotify` |
| Multi-pane lipgloss layout | folded into `sessions-list-panel` (the second pane) |
| Image rendering, mouse support, focus reporting | not yet planned |
| Multi-project view / daemon split | not yet planned; bootstrap is single-project / current-cwd only |

## Approach

### Walking skeleton — why this shape

Three approaches were weighed in exploration: **walking skeleton** (vertical slice end-to-end), **layer-first** (pure domain + use cases first, TUI last), and **TUI-first** (mock data, real adapters last). Walking skeleton wins because:

- It validates the entire stack in one change, including the depguard layer rules and the Bubble Tea v2 integration on this machine.
- The JSONL event shape is now well-understood from sampling three real sessions during exploration — there is no longer a "we don't know the data" excuse for delaying the adapter.
- A visible terminal artifact at the end of the change is motivating and gives subsequent panel changes a known-good baseline to plug into.
- Layer-first defers feedback on the integration surface; TUI-first risks building the wrong domain model from mock data.

### Layer tree (to be created in apply phase)

```
clyde/
├── go.mod
├── cmd/clyde/main.go               # composition root
└── internal/
    ├── domain/                      # pure, stdlib-only
    │   ├── session/                 # Session, SessionID, SessionState
    │   ├── event/                   # Event, EventID, EventType
    │   └── usage/                   # Usage with Add() value method
    ├── application/
    │   └── watchsession/            # WatchSession use case
    ├── ports/
    │   ├── sessionsource.go         # SessionSource interface
    │   └── clock.go                 # Clock interface
    └── adapters/
        ├── jsonl/                   # SessionSource impl (file-based, no fsnotify in V1)
        ├── systemclock/             # Clock impl
        └── tui/                     # Bubble Tea v2 root model
```

### Locked decisions

1. **Approach:** walking skeleton — thinnest end-to-end slice through every layer.
2. **Module path:** `github.com/clyde-tui/clyde` (matches `.golangci.yml` deny rules and confirmed GitHub identity).
3. **Go directive:** `go 1.26` in `go.mod`. Local toolchain confirmed as `go1.26.2 darwin/arm64`.
4. **V1 surface:** ONE pane rendering the last 5 events of the focused session under the current cwd. No multi-pane, no other panels, no images.
5. **V1 ports:** `SessionSource` and `Clock`. Nothing else is wrapped.
6. **No fsnotify in V1:** the JSONL adapter reads on demand. The TUI uses `tea.Tick` polling if any refresh is needed at all. `FileWatcher` port is deferred to `live-tail-fsnotify`.
7. **No pricing / cost in V1:** `Usage` carries token counts and an `Add` method only. Cost computation lands in `usage-and-cost-panel`.
8. **Hexagonal discipline:** wrap I/O ONLY when it has SEMANTICS. JSONL stream = port (`SessionSource`). `time.Now()` = port (`Clock`). Generic file open/read with no domain meaning = no port.
9. **TDD:** strict mode active. Every domain type, use case, and adapter ships with a failing test first.

### Dependencies to add in `go.mod` (versions pinned in design phase)

- `github.com/charmbracelet/bubbletea/v2` — TUI runtime
- `github.com/charmbracelet/lipgloss/v2` — layout primitives (used minimally in V1, but pinned for consistency)
- `github.com/charmbracelet/x/exp/teatest` — test-only, for TUI snapshot tests
- `github.com/fsnotify/fsnotify` — **NOT added in this change.** Listed here only to call out its deferral. It enters the build with `live-tail-fsnotify`.

### What the V1 slice does, end to end

1. `clyde` binary launches in a directory (any directory).
2. Composition root computes the encoded project path (`/` → `-`, prepend `-`) for the current cwd, e.g. `~/.claude/projects/-Users-vladpb-work-Personal-clyde/`.
3. `JsonlSessionSource` lists `*.jsonl` files in that directory, picks the most recently modified one as the focused session, and decodes its events.
4. `WatchSession` use case asks the `SessionSource` for the focused session's events, asks the `Clock` for now (for "Xs ago" rendering later), and returns a `SessionView` value with the last 5 events.
5. The TUI root model renders those 5 events as text lines in a single pane. No styling beyond minimal lipgloss padding.
6. Quitting closes cleanly. No live reload, no background goroutines beyond what Bubble Tea itself runs.

## Affected modules / packages

This is a **greenfield** change. Every file is new. The complete set of new artifacts:

- `go.mod`, `go.sum`
- `cmd/clyde/main.go`
- `internal/domain/session/{session.go,session_test.go}`
- `internal/domain/event/{event.go,event_test.go}`
- `internal/domain/usage/{usage.go,usage_test.go}`
- `internal/application/watchsession/{watchsession.go,watchsession_test.go}`
- `internal/ports/sessionsource.go`
- `internal/ports/clock.go`
- `internal/adapters/jsonl/{jsonl.go,jsonl_test.go,testdata/...}`
- `internal/adapters/systemclock/systemclock.go`
- `internal/adapters/tui/{model.go,model_test.go}`

Pre-existing files left unchanged: `.golangci.yml`, `.gitignore`, `README.md`, `openspec/`, `.atl/`.

## Domain concepts touched

All NEW (greenfield):

- **Session** — a single Claude Code conversation persisted as one JSONL file. Has an ID and an ordered slice of events. Bootstrap-skeleton models only what the V1 render needs; full state machine evolves in later changes.
- **Event** — one JSONL line. Bootstrap-skeleton models the common envelope (`type`, `uuid`, `timestamp`, `sessionId`, `parentUuid`) plus enough of the `user` / `assistant` payload to render a one-line summary. Other event types (`system`, `attachment`, `progress`, `ai-title`, `permission-mode`, `file-history-snapshot`, `last-prompt`, `queue-operation`) are decoded as the envelope only and skipped from the rendered list. They become first-class in later changes.
- **Usage** — token counts (input, output, cache_creation, cache_read) with an `Add(other Usage) Usage` value method for accumulation. Carried into V1 because it is referenced by domain `Event` (assistant events embed `usage`), but no panel renders it yet.

Concepts deliberately NOT modeled in V1: `Turn`, `ToolCall`, `Subagent`, `Project`, `Todo`. Each lands with the change that needs it. We do not pre-model.

## Risks & mitigations

1. **JSONL schema evolves** — Claude Code is actively developed (versions 2.1.119 → 2.1.123 observed in a single sampled session). New event types or field renames could break the adapter.
   *Mitigation:* the adapter decodes the common envelope strictly and treats unknown event types as opaque (skip without error). Per-type payloads use `json.RawMessage` for optional fields. The sampled schema version is documented in the adapter's package comment.

2. **`isSidechain` is not how sub-agents appear** — exploration confirmed sub-agents are emitted as `queue-operation` events, not `isSidechain=true`. This will surprise anyone modeling sub-agents from older assumptions.
   *Mitigation:* sub-agent rendering is deferred to `subagent-tree-panel`; bootstrap-skeleton does not render `queue-operation` events at all. Discovery captured here so the next phase doesn't relitigate it.

3. **No cost field in JSONL** — cost is not stored; it must be computed from token counts × Anthropic's pricing table, which changes.
   *Mitigation:* deferred entirely to `usage-and-cost-panel`. Bootstrap-skeleton's `Usage` type carries tokens only — no cost surface yet.

4. **Usage is per-assistant-event, not per-turn or per-session** — aggregating requires accumulation across all assistant events.
   *Mitigation:* `Usage.Add` is a pure value method on the domain type; aggregation lives in the use case, not in adapters or the TUI.

5. **Bubble Tea v2 API differences from v1** — much of the publicly available documentation is v1. v2 may have moved or renamed APIs.
   *Mitigation:* the design phase pins specific v2 versions and validates the import paths and Cmd/Batch/Tick patterns against the actual v2 API before tasks are emitted. Core MVU shape is stable across v1/v2.

6. **depguard blocks `os` and `time` in `internal/domain/**`** — domain types cannot read files or call `time.Now()`.
   *Mitigation:* this is intentional architecture. All I/O goes through `SessionSource`; all time goes through `Clock`. The walking skeleton is deliberately structured to honor this from line one.

7. **JSONL project-path encoding** — the `~/.claude/projects/<encoded-cwd>/` encoding (replace `/` with `-`, prepend `-`) is observational, not documented by Anthropic.
   *Mitigation:* the encoding lives ONLY in the JSONL adapter, behind the `SessionSource` port. If Claude Code changes the scheme, exactly one file changes. The adapter test asserts the encoding against a fixture path.

## Rollback plan

This is a greenfield change with no existing Go code in the repository. Rollback is trivial:

```sh
rm -rf go.mod go.sum cmd/ internal/
git checkout -- .golangci.yml   # if any tweaks were made
```

No data migration, no shared services, no production traffic. The repo returns to its pre-change state (just `.golangci.yml`, `.gitignore`, `README.md`, `openspec/`, `.atl/`) with one command. No rollback rehearsal needed.

## Open questions for /sdd-spec and /sdd-design

1. **Bubble Tea v2 exact pinned version** — `bubbletea/v2` and `lipgloss/v2` are in active development. Design phase must pin specific tags and verify the import paths compile against `go 1.26`.
2. **`teatest` import path under v2** — confirm whether `github.com/charmbracelet/x/exp/teatest` works against `bubbletea/v2` or whether a v2-aware variant is needed.
3. **Event rendering format for V1** — what does a single line look like? Proposal suggests `<HH:MM:SS> <type> <one-line-summary>` but design should lock the format and decide whether the TUI shows raw type names or human-friendly labels.
4. **Refresh strategy for V1** — does the TUI re-read the JSONL on a `tea.Tick` interval, or is the V1 render strictly one-shot at startup? Either is consistent with "walking skeleton"; design should pick one.
5. **Test fixture provenance** — the JSONL adapter test needs a fixture file. Should it be a hand-trimmed slice of a real session (smaller, controllable) or a fully synthetic fixture authored from scratch? Design phase decides.
