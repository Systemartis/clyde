# Clyde — Wire-Up Plan: Mock Data → Real Adapters

> Status: **EXECUTED — V1 shipped**. All phases A–J complete. `clyde-proto` promoted to `clyde`; proto package merged into `internal/adapters/tui/`.

## Current state (v14)

**UI complete** at `internal/adapters/tui/proto/` — bunny mascot, three layout modes, three-state interaction model, viewport scrolling, mouse hit-testing accurate, file viewer (text + ASCII image fallback). All 100% mock data.

**Production data layer exists** at `internal/{domain,application,adapters/jsonl,adapters/systemclock,ports}/` from the bootstrap-skeleton + event-rendering SDD changes. Working: JSONL reader, SessionSource, Clock, WatchSession use case, Usage monoid, Event with Kind+Payload, IsMeta/IsToolResultOnly filters, Summary derivation.

**Goal**: replace `mockdata.go` with adapters reading real Claude Code state. Keep proto's UI structure as-is. Promote proto to be the primary `clyde` binary at the end.

## Architecture

```
proto/Model
  ├── adapters injected at construction
  │   ├── SessionSource (jsonl) — exists
  │   ├── Clock (systemclock) — exists
  │   ├── GitSource (NEW) — git status + diff
  │   ├── FilesystemSource (NEW) — cwd tree walk
  │   ├── TodoSource (NEW) — ~/.claude/todos/
  │   └── ConfigSource (NEW) — ~/.config/clyde/config.toml
  ├── live state refresh via tea.Tick or fsnotify Cmds
  └── data flows through use cases → view models → panel render
```

Hexagonal discipline preserved: domain stays pure, application use cases compose ports, adapters concrete-ize.

## Phases

### Phase A — Foundation: Sessions + Events from JSONL

**Goal**: proto reads real session JSONL, picks focused session, exposes events to panels.

- Wire `jsonl.NewProductionSource()` into proto's `Model.New()`.
- New use case `LiveSession` (or extend `WatchSession`): given Project, returns `SessionView{Sessions []SessionSummary, FocusedID, Events []Event}`.
- Replace `mockdata.go` session/event loading with use case call.
- fsnotify watcher → `tea.Cmd` → re-fetch on JSONL change.
- Panels using event data: `now`, `calls`, `usage`, `diff` (partial).

**Files**: `internal/application/livesession/`, `internal/adapters/tui/proto/data.go` (new), update `model.go`.

**Tests**: `LiveSession` use case with fake SessionSource. Verify proto Model loads events.

**Risk**: schema drift on new event types — preserved as `KindOpaque` (already handled).

### Phase B — Calls panel from real tool_use events

**Goal**: hierarchical agent tree from actual JSONL events.

- Filter events: `KindAssistant` with `tool_use` block → tool call. `queue-operation` events → subagent dispatches.
- Build `AgentGroup` hierarchy: main session + subagents (link via parent_uuid + queue events).
- Per call: tool name, key arg (already extracted via Summary helpers in jsonl adapter), state (running if no matching tool_result, done if matching tool_result, failed if error in tool_result).
- Replace `mockAgentGroups` with the derived data.

**Files**: `internal/application/callstree/` (new use case), update `panel_calls.go` to consume.

**Tests**: parse fixture JSONL with main + 2 subagents, assert hierarchy, assert state determination.

### Phase C — Usage panel from real token counts

**Goal**: sum tokens across session events; show cost from pricing config.

- Walk events, accumulate Usage via existing monoid.
- Pricing config: hardcoded model→price-per-token table (Opus/Sonnet/Haiku), or read from a JSON. Compute cost from tokens × price.
- Compaction percent: tokens/model_context_limit. Use known limits (Opus 4.7 = 200k, etc.).
- Issues counts (errors/warnings/tests): defer to V15 (needs LSP/test-runner integration).

**Files**: `internal/domain/pricing/` (new), update `panel_usage.go`.

**Tests**: known event sequence → expected total Usage + cost.

### Phase D — File explorer + git from real cwd

**Goal**: tree walk current project + git status badges.

- New port `FilesystemSource` (or just stdlib walk — domain doesn't need an interface here per ADR-001).
- New port `GitSource`: shell out to `git status --porcelain` and `git diff --stat`.
- Tree walk respects `.gitignore` (use `go-gitignore` or simple impl).
- Modified-files section: parse `git status` output.
- Highlight current file: read claude's last-edited file from JSONL tool_use events.

**Files**: `internal/adapters/git/`, update `panel_explorer.go`.

**Tests**: walk a tempdir, mock git status output, assert tree + badges.

**Risk**: large repos slow walk — debounce + lazy expand.

### Phase E — Live diff from git

**Goal**: show pending edits hunks.

- `git diff` output → parse into hunks (lines, line numbers, +/− markers).
- For active file: filter to that file's diff.
- Update on every JSONL tool_use Edit/Write/MultiEdit event.

**Files**: `internal/adapters/git/diff.go`, update `panel_diff.go`.

**Tests**: parse known diff output, assert hunk extraction.

### Phase F — Now panel from latest activity

**Goal**: status text reflects real state.

- Last tool_use event → "edit auth.ts" / "Read /path" text.
- Streaming indicator: events with no closing tool_result yet → "writing..." with token rate (compute from event timestamps).
- LSP/lint indicators: defer (Phase H).

**Files**: update `panel_now.go` to consume LiveSession state.

### Phase G — Servers panel (MCPs + LSPs)

**Goal**: real MCP server status; LSP if detectable.

- MCPs: read `~/.claude/settings.json` enabledPlugins/MCP server config; ping each for tool count via local socket if available.
- LSPs: detect by scanning tcp ports for known LSP servers OR shell out to `gopls`/`tsserver` etc. Punt: V14.5 hardcoded based on file types in cwd (Go file → assume gopls).

**Files**: `internal/adapters/mcpconfig/`, update `panel_servers.go`.

**Risk**: live MCP status checking is non-trivial. V1 acceptable: just list configured MCPs, show "configured" state without live ping.

### Phase H — Notification banner from hooks

**Goal**: claude permission requests appear in the banner.

- Spin up tiny localhost HTTP server (`http.Server` on `127.0.0.1:0` random port).
- Write port to a temp file claude can find.
- Configure claude hooks for PreToolUse with HTTP transport pointing to clyde's port.
- Banner subscribes to incoming POSTs.
- Y/N/Esc routes back via response body.

**Files**: `internal/adapters/hooks/server.go`, update `panel_notification.go`.

**Risk**: hook integration touches user's `~/.claude/settings.json` — auto-config or document manual setup. V14: manual setup only.

### Phase I — Polish + QoL

After H, lift remaining mock-data and add quality-of-life:
- **Bunny event triggers**: happy on task done, surprised on tool error, sleep on idle.
- **Compaction warning**: when tokens > 75% of context limit, show banner.
- **Image viewer Kitty rendering**: emit APC sequence in viewer when terminal supports it.
- **Settings overlay**: `?` menu lets user toggle panel visibility, change theme.
- **Tab strip in title bar**: switch between projects (multi-project later).
- **Performance**: debounce JSONL re-reads, lazy explorer tree expansion, viewport recompute on resize.
- **Dependency cleanup**: remove `BurntSushi/toml` if unused, audit go.mod.

### Phase J — Promote proto to clyde

- Replace `internal/adapters/tui/` with `internal/adapters/tui/proto/` contents.
- Replace `cmd/clyde/main.go` to use the new model.
- Delete `cmd/clyde-proto/`.
- Final naming pass: `clyde` is the binary, `proto` package goes away.
- Update README, docs, design docs.

## Execution order

1. **A** (foundation) — must come first; others depend on it.
2. **B + C + F** in parallel — all three consume events from A.
3. **D + E** in parallel — filesystem + git, after B/C/F.
4. **G** — server status, can run alongside D/E.
5. **H** — hooks, can start after A.
6. **I** — polish pass.
7. **J** — promotion to clyde.

## Token budget awareness

Each phase = one sub-agent dispatch (sometimes parallel). Roughly:
- Phase A: ~30-50k tokens
- Phases B+C+F (parallel): ~80-120k
- Phases D+E (parallel): ~50-80k
- Phase G: ~30k
- Phase H: ~50k
- Phase I: ~80-150k
- Phase J: ~30k

Total estimate: 400-600k tokens. Pace accordingly. If usage gets tight, pause via `ScheduleWakeup` until reset window.

## Validation strategy

After each phase:
- Build, vet, gofmt, golangci-lint, all tests pass.
- Manual smoke: run `clyde-proto` against the actual current Claude Code session, verify the relevant panel shows real data.
- No regression: existing goldens still pass, naming convention preserved.

After Phase J: full integration test running clyde against a live `claude` session in a tmux split.

## Open architectural questions (resolved here)

1. **Real-time updates**: fsnotify on JSONL, debounced 100ms. Already-proven pattern.
2. **Multi-session focus**: pick most-recently-active session by mtime. User can switch via UI (V14.5).
3. **Cost source of truth**: pricing table in code, not external API. Update on Anthropic price changes.
4. **Hook server lifecycle**: starts on clyde launch, dies on quit. No persistence.
5. **Config schema migration**: not yet versioned. V1 tolerates missing fields, applies defaults.

## Out of scope for this plan

- Edit mode in viewer (read-only suffices for V1; bubbles/textarea for V2).
- Multi-project tab strip in title bar.
- Custom themes (theme picker UI; the dark/light/catppuccin presets exist as code constants).
- Settings persistence (TOML config exists; runtime UI to edit it is V2).
- Performance optimization for huge sessions (>10k events) — assume reasonable session sizes for now.
- Image viewer Kitty rendering polish (basic emission only; iTerm2 fallback later).
