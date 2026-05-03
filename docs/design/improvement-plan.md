# Clyde — Improvement & Fixes Plan

> **Scope.** A staged, evidence-based plan covering critical fixes, code-quality
> debt, and feature work, derived from a deep audit of the V22 codebase on
> 2026-05-01.
> **Method.** Each item lists *evidence* (file:line), *why* (impact /
> rationale), *what* (acceptance), and *risk*. Phases are ordered so that each
> later phase benefits from the earlier one.
> **Last revised.** 2026-05-01 — pivoted Phase 2 from "Tasks panel" to
> "Subagent observatory" after user feedback that Claude Code already shows
> the TodoWrite plan above the chatbox; added Phase 4 (settings menu with
> global + per-project panel toggles) as infrastructure before new panels;
> split pricing into its own phase tied to plan-tier detection.

---

## Guiding principle

**Clyde must surface what is *invisible* in the Claude Code CLI, not duplicate
what is already on screen.** TodoWrite plans, top-level chat messages, and
inline tool invocations are already visible to the user. Subagent internals,
cumulative shell-side effects, cache hit rates, plan-quota %, and cross-session
state are *not* — that is where clyde earns its panel real estate.

---

## TL;DR

V22 ships solid hexagonal foundations and real adapters. Three blockers:

1. A failing golden test on `main` violates the strict-TDD invariant.
2. `internal/adapters/tui/model.go` is 1601 LoC / 67 funcs — adding any new
   panel compounds the debt.
3. Cost display (`$X.XX`) is misleading for Pro/Max users who are not paying
   per token; the existing `ports.PlanUsage.Tier` already lets us detect that.

This plan splits work into nine phases. Phases 0–1 are mandatory before
anything else. Phase 2 pivots `panel_calls` into a true *subagent observatory*.
Phase 3 fixes the pricing-vs-plan display ambiguity. Phase 4 hardens the
settings menu so adding new panels is safe. Phases 5–9 add new value, each
gated by Phase 4's panel-toggle infrastructure.

---

## Phase 0 — Stabilize `main`

Mandatory. Total estimate: < 1 hour.

### 0.1 Fix the failing golden test

- **Evidence.** `go test ./...` reports
  `TestProtoView_ViewerImageFallback130x40` mismatch
  (`internal/adapters/tui/view_golden_test.go:184`):
  - want: `Kitty graphics: V2`
  - got:  `Kitty detected, rendering: V2`
- **Why.** A red `main` violates the project's strict-TDD baseline.
- **What.** Decide canonical copy (current code says
  `Kitty detected, rendering`), regenerate the affected golden, rerun.
- **Acceptance.** `go test ./...` passes everywhere.
- **Risk.** None.

### 0.2 Tighten `.gitignore`

- **Evidence.** `.gitignore` ignores `/clyde` but not `/clyde-proto`; both
  binaries exist in repo root and `clyde-proto` shows as `??` in `git status`.
  (Confirmed neither is tracked via `git ls-files`.)
- **What.** Add `/clyde-proto` to `.gitignore`.
- **Risk.** None.

### 0.3 Bootstrap CI

- **Evidence.** No `.github/` directory exists. The strict `.golangci.yml`
  (depguard hexagonal layering, gocyclo 15, gocognit, revive) is enforced by
  nobody on PRs.
- **What.** Add `.github/workflows/ci.yml`:
  - `go test ./...` (with `-race -cover`)
  - `golangci-lint run ./...`
  - `gofmt -l . | tee /dev/stderr | (! read)`
- **Acceptance.** A trivial PR runs all three jobs green.
- **Risk.** Low. Match local toolchain (Go 1.26, golangci-lint v2.11.4).

---

## Phase 1 — Refactor the TUI core

Mandatory before any new panel work. Estimate: half a day. **Pure refactor —
no behavioural change, no golden updates.**

### 1.1 Split `model.go`

- **Evidence.** `internal/adapters/tui/model.go` — 1601 LoC, 67 funcs holding
  the `Model` struct, three `New*` constructors, every `tea.Cmd` factory, all
  `handle*` keyboard / message handlers, layout math, viewport plumbing,
  settings overlay logic, hook handling, and `View`.
- **Why.** Phases 2, 5, 6 each add panel state to `Model`. Without splitting
  this file first, every later phase compounds the debt.
- **What.** Split into cohesive files inside `internal/adapters/tui/`:
  | New file               | Contents                                                                 |
  |------------------------|--------------------------------------------------------------------------|
  | `model.go`             | `Model` struct, `New*`, `Init`, `View`                                   |
  | `model_messages.go`    | All message types (`liveSessionMsg`, `refreshLiveMsg`, `planUsageMsg`, `refreshPlanUsageMsg`, `hookEventMsg`) |
  | `model_commands.go`    | All `tea.Cmd` factories                                                  |
  | `update.go`            | `Update`, `handleWindowSize`, `handleFrame`, `applyLiveView`, `handleLiveSession`, `handleRefreshLive`, `handlePlanUsage`, `handleHookEvent` |
  | `keys.go`              | Every `handle*Key` function and chord helpers                            |
  | `focus.go`             | `advanceFocus`, `setFocus`, `cycleLayoutMode`, `effectiveMode`, `transitionTo*`, column helpers |
  | `viewport_sync.go`     | Viewport plumbing + `ViewerViewport`                                     |
- **Acceptance.** `go test ./...` and `golangci-lint run ./...` both pass with
  the same goldens.
- **Risk.** Low if done in one mechanical PR.

### 1.2 Split `livedata.go` (optional but recommended)

- **Evidence.** 1046 LoC, 38 funcs. Cohesive but trending the same way.
- **What.** Split by surface: `livedata_calls.go`, `livedata_now.go`,
  `livedata_usage.go`, `livedata_explorer.go`, `livedata_diff.go`,
  `livedata_servers.go`, `livedata_format.go`.
- **Risk.** None.

### 1.3 Rename `PanelTasks` → `PanelCalls`

- **Evidence.** `model.go:32` declares `PanelTasks` but the file rendering it
  is `panel_calls.go` — the panel shows tool calls, not a plan. The name is a
  legacy of an earlier design intent that was never built (`v3-notes.md:47`,
  `panel_viewer.go:452` still reference it).
- **Why.** Removes ambiguity before Phase 2 pivots the panel further.
- **What.** Rename the enum and any references; no behaviour change.
- **Risk.** Low. `rg` + edit + tests.

---

## Phase 2 — Subagent Observatory (pivot of `panel_calls`)

Estimate: 1–2 days. **Highest user-visible value per day spent.**

### Why a subagent observatory and not a tasks panel

Claude Code already renders the `TodoWrite` plan above the chatbox; duplicating
it in clyde adds zero unique value. **What is invisible** is the work happening
inside `Task` (subagent) calls — the CLI shows `Agent(...)` going in and
`Agent finished` coming out, with everything in between hidden. Users running
multiple subagents in parallel have *no* visibility into what each one is
doing. This panel fixes that.

### 2.1 Domain emphasis on subagents

- **Evidence.** `internal/adapters/jsonl/subagents.go` already groups events by
  `agentID`; `IsSubagent` already exists on `livesession.AgentTimeline`.
- **What.** No new domain types; existing structures suffice. Verify
  `AgentTimeline.Active` and `IsSubagent` are correctly populated for the
  *currently running* subagent, including parallel dispatches.
- **Tests.** Add `internal/adapters/jsonl/subagents_test.go` cases for: two
  subagents in flight at once, subagent that finishes vs subagent still active,
  nested subagents (subagent dispatching another subagent — clarify expected
  behaviour with the user).

### 2.2 Panel UI redesign

- **What.**
  - **Main agent.** Collapse to a single summary line:
    `main · 12 calls · last: Edit foo.go`. Reasoning: those calls are visible
    in the chat — clyde just confirms the count and the latest.
  - **Subagent cards.** Each subagent gets a labelled card:
    ```
    ▼ refactor-helper (subagent)  · running · 4 calls · 12s elapsed
        ✓ Read   internal/foo.go               0.4s
        ✓ Grep   "TODO" internal/              0.1s
        ▶ Edit   internal/foo.go               2.3s
        □ pending
    ```
  - **Color per agent** so parallel subagents are visually distinct.
  - **Tools histogram (collapsed sub-line).**
    `Read 4 · Edit 3 · Bash 2 · Grep 1`.
  - **Active panel mode.** Up/down navigates between cards; Enter opens the
    full per-subagent timeline as a viewer-pane takeover (reuses
    `ViewerViewport`).
- **Acceptance.** Three new goldens (no subagents, one subagent active, two
  subagents in flight). Demo mode shows a deterministic mock with two
  subagents.
- **Risk.** Medium. UI is new enough that we should specify it via SDD
  (`/sdd-new subagent-observatory`) before implementation.

### 2.3 Title-bar sub-indicator

- **What.** When ≥1 subagent is active, the title bar gains
  `· 2 subagents` between the cwd and the model.
- **Risk.** None — title bar already supports segments.

---

## Phase 3 — Pricing dual-mode (plan-tier aware)

Estimate: 2 hours.

### 3.1 Hide `$` for Pro/Max users; show plan-quota %

- **Evidence.** `ports.PlanUsage.Tier` (`internal/ports/planusage.go:46`)
  exposes the user's plan as `"max_5x"`, `"max_20x"`, `"pro"`, or empty
  (unknown). `internal/domain/pricing/pricing.go:142` carries an open
  `TODO: verify exact 1M pricing` that only matters for API-key users.
- **Why.** A `$1.42` reading is misleading to a Max subscriber — they did not
  spend $1.42 in any meaningful sense. Plan-quota % already tells them the
  actual constraint. Conversely, an API-key user has no plan-quota; the
  dollar amount is the *only* metric that matters.
- **What.**
  - In `panel_usage.go`: when `planUsage.Tier != ""` *and* both
    `FiveHour.Present` / `SevenDay.Present` are true → render plan-quota bars
    only, hide cost rows.
  - When `planUsage.Tier == ""` (API-key user, or detection failed) → render
    cost rows, hide plan-quota bars.
  - Ambiguous fallback (`Tier != ""` but no successful fetch): show stale
    plan-quota with `(plan offline)` badge, still hide cost.
- **Acceptance.** Two new goldens covering Max-tier and API-key cases.
- **Risk.** Low. Pure presentation logic; no backend changes.
- **Side benefit.** Closes `pricing.go:142` — the 1M-context rate-card
  precision now only matters for the API-key path, which is a much smaller
  audience and easier to validate against the public rate card.

---

## Phase 4 — Settings menu: global + per-project

Estimate: 1 day. **Infrastructure phase — must complete before adding new
panels in Phases 5–6.**

### Why this comes before Phases 5 and 6

Adding a panel without a hide/show mechanism forces the new feature on every
user. Some projects benefit from `bash-audit` (script-heavy work); some don't
(tight, short edits). Per-project toggles let users opt in only where the
panel is useful.

### 4.1 Config schema extension

- **Evidence.** `internal/adapters/tui/config.go` already loads
  `~/.config/clyde/config.toml`; `settings_overlay.go` already toggles a
  `[]SettingsPanelToggle` against in-memory cfg.
- **What.** Extend the schema with three layers, evaluated in order:
  ```toml
  # ~/.config/clyde/config.toml

  # Layer 1 — clyde defaults (built-in, not in file)

  # Layer 2 — global user defaults
  [panels]
  now      = true
  calls    = true   # subagent observatory
  diff     = true
  usage    = true
  explorer = true
  servers  = true
  bash     = false  # Phase 5 — opt-in
  cache    = false  # Phase 6 — opt-in

  [ui]
  theme  = "tokyonight"
  layout = "stack"

  # Layer 3 — per-project overrides, keyed by absolute cwd
  [projects."/Users/vladpb/work/clyde"]
  panels.bash  = true
  panels.cache = true

  [projects."/Users/vladpb/work/api-server"]
  panels.bash  = true
  ui.layout    = "tabs"
  ```
- **Resolution.** At startup the model resolves `EffectiveConfig` =
  builtin ⊕ global ⊕ project[cwd]. Later layers override earlier ones at the
  leaf level (not the table level — i.e. setting one panel false in a project
  does not blank out the global panel block).
- **Tests.** Pure table-driven in `config_test.go`: missing project section
  falls through to global; missing global key falls through to builtin; per-
  project layout override beats global layout.

### 4.2 Settings overlay extension

- **Evidence.** `settings_overlay.go` already renders panel toggles and writes
  back into `cfg`.
- **What.**
  - Add a "Scope" header at the top of the overlay: `[ Global | This project ]`.
    The user picks which scope edits write to. Default: project.
  - Add new toggle rows for `bash` and `cache` panels (greyed out until those
    phases ship — visible but disabled).
  - When the user changes a setting, write back to the on-disk TOML at the
    appropriate scope. Persist immediately (no separate "Save" button).
- **Acceptance.** Round-trip test: open overlay, toggle two panels at project
  scope, close, reopen → state preserved; inspect TOML on disk → exactly the
  expected delta.
- **Risk.** Medium — TOML write-back must preserve user comments and
  unrelated keys. Use a TOML library that supports comment-preserving rewrite
  (or, simpler: use a hand-rolled "merge into existing" writer that touches
  only the keys we own).

### 4.3 Live re-layout on toggle

- **What.** Toggling a panel in the overlay must apply immediately (no
  restart). Hidden panels are skipped in `Tab` cycling, layout math, and
  rendering.
- **Risk.** Touches focus management — depends on Phase 1.1 already having
  isolated `focus.go`.

---

## Phase 5 — Bash Audit panel

Estimate: 1 day. Gated by Phase 4 (off by default; opt-in per project).

### 5.1 Why this panel earns its space

The chat shows individual `Bash` tool calls inline, but reconstructing
"everything claude ran in this session, in order, with exit codes" requires
scrolling and skipping intermediate text. A dedicated chronological log is
genuinely faster than the chat for this question.

### 5.2 Domain + adapter

- **What.** Add a `BashEntry` projection (timestamp, command, exit code,
  duration, stdout-tail) computed from `Bash` `tool_use` + `tool_result` pairs
  in the session events.
- **Adapter.** Extract in `livesession.LiveSession.refresh` into a new
  `View.BashLog []BashEntry`. No new port — events are already available.
- **Tests.** Fixtures with a successful Bash, a failed Bash (non-zero exit), a
  long-running Bash (interrupted), and a Bash with no tool_result yet.

### 5.3 Panel UI

```
bash audit · 14 ran · 1 failed
14:32:08  $ npm test                  ✓ 12.3s
14:32:21  $ rg -n 'TODO' src/         ✓ 0.1s
14:33:04  $ npm run build             ✗ 4.2s   exit=1
14:33:09  $ npm run lint              ✓ 2.1s
```

- Click / Enter on a row in active mode → expanded view with full command
  and stdout-tail in `ViewerViewport`.
- Filter `?` failed-only.

### 5.4 Settings entry

- New panel toggle row (default off in global, the user opts in per project
  via overlay or TOML).

---

## Phase 6 — Cache Efficiency panel

Estimate: half a day. Gated by Phase 4.

### 6.1 Why this earns its space

`usage.Usage.CacheRead` is already extracted but only used internally to fix
token-counting (V20 bugfix). Surfacing the cache-hit ratio gives users — both
API-key and Pro/Max — actionable feedback about prompt structure: a low ratio
means cache busts, which means slow + expensive turns.

### 6.2 Panel UI

```
cache efficiency · 87% hit
   3.2M from cache · 470k recomputed
   biggest miss: 14:32  +210k
   trend (last 10 turns):  ▁▂▃▅▆▇█▇▆▅
```

- Row 1: aggregate hit % across the visible window (5h or session — the
  same selector the usage panel uses).
- Row 2: tokens served from cache vs. tokens recomputed.
- Row 3: largest cache-miss event (timestamp + delta), helps users find
  the prompt tweak that broke caching.
- Row 4: sparkline of per-turn hit ratio.

### 6.3 Domain projection

- **What.** Add `View.CacheStats` with the four numbers above. All derivable
  from existing per-turn `usage.Usage` snapshots.
- **Tests.** Pure derivation tests in domain.

---

## Phase 7 — Multi-session strip in the title bar

Estimate: 1 day.

- **Evidence.** `LLMSource.AllProjectSessions` and the global session source
  are already wired (`cmd/clyde/main.go:100`).
- **What.**
  - Title-bar tab strip: `[● clyde] [api-server] [diary]`.
  - `]` / `[` cycle; click a tab to switch; the active project gets the
    coloured bullet.
  - On switch: swap `liveProject`, fire fresh `snapshotCmd`, re-resolve
    per-project config (Phase 4).
  - Hide the strip when only one project has activity.
- **Risk.** Medium. Re-renders touch most panels.

---

## Phase 8 — Command palette (`Ctrl+K`)

Estimate: half a day.

- **Why.** Discoverability. With Phases 4–7 the keybind list grows; a fuzzy
  command runner pays for itself across every future feature.
- **What.** `charm.land/bubbles/v2/list` with filter; actions mirror current
  bindings + Switch project / Toggle panel X / Cycle layout.
- **Risk.** Low.

---

## Phase 9 — Notification policies + theme picker (parallel)

Estimate: 1 day combined.

### 9.1 Hook policies

- **What.** `[hooks.policies]` table in config — `auto-approve` /
  `always-ask` per tool. `respondHook` (in the new `update.go` after
  Phase 1) short-circuits when the policy says so. Inline
  "(auto-approved by policy)" line in the calls panel.
- **Default.** Nothing auto-approved out of the box.

### 9.2 Theme picker

- **What.** Move palette to `themes/{tokyonight,catppuccin,gruvbox,dracula}.go`;
  select via `[ui] theme = "..."`; settings overlay row to switch live.

---

## Phase 10 — Multi-LLM adapters (strategic, demand-driven)

Already designed in `docs/design/multi-llm-plan.md`. Implementation order:

1. **Codex CLI** — JSONL is closest to Anthropic's shape; lowest cost.
2. **Gemini CLI** — different on-disk format; needs format research.
3. **Kimi CLI** — last; smallest user base.

**Do not start Phase 10 until Phases 0–6 are done.** Every clyde user benefits
from earlier phases; only a subset benefits from multi-LLM.

---

## Backlog (lower priority, captured for completeness)

- **Test/build status timeline** — extract `go test`, `npm test`, `pytest` from
  Bash calls; sparkline of pass/fail. Could be a sub-section of the Bash Audit
  panel rather than its own panel.
- **Diff: side-by-side toggle.** Non-trivial alignment.
- **Diff: per-language syntax highlighting** (chroma).
- **Diff: stage / unstage hunks** writing to git index.
- **Explorer: `/` fuzzy filter** + `g` jump-to-active-file.
- **Explorer: collapse / expand directories** with `←` / `→`.
- **Images panel** — last 4 images claude saw, with full Kitty rendering.
- **Notification: timeout with auto-deny + reason.**
- **Notification: stack** (top expanded, others as pills).
- **Frame-level visual states** (idle / paused / error / success).
- **Config hot-reload** via `fsnotify`.
- **Viewer scrollbar thumb drag.** V22 ships click-to-jump on the scrollbar
  column — a click anywhere along the track sets `YOffset` proportionally.
  Continuous thumb drag (mouse-down on the thumb, then drag) needs Bubble
  Tea v2 `MouseMotion` plumbing through `Update` so the YOffset tracks the
  cursor while the button stays held. Marked low priority — wheel + arrow
  keys + click-to-jump cover the typical use case.
- **Servers panel: order by last-used in session.** The activity panel
  already has the source data (every Bash/MCP/tool call shows up in the
  session events). For each server, find the latest session event whose
  Tool name maps to it and use that timestamp as the sort key (descending).
  Servers never used in the session sink to the bottom in alphabetical
  order. Useful for users with 30+ MCPs/LSPs who want recently-touched
  ones surfaced first. Cost: a per-snapshot scan of events plus a server
  → tool-name lookup table.

---

## Suggested order of execution

```
0.1 → 0.2 → 0.3                # Stabilize main (one PR)
1.1 → 1.2 → 1.3                # Refactor (one PR per item, mechanical)
2                              # Subagent observatory (one feature PR via SDD)
3                              # Pricing dual-mode (one small PR)
4                              # Settings menu: global + per-project
5                              # Bash Audit panel
6                              # Cache Efficiency panel
7                              # Multi-session tabs
8                              # Command palette
9                              # Hook policies + theme picker
10                             # Multi-LLM adapters, on demand
```

Phases 0–1 are pure debt repayment with no user-visible change.
Phases 2–9 are feature work; spec each via SDD (`/sdd-new <change>`) before
implementing, with goldens as the acceptance gate. Phase 10 is strategic.

---

## Open questions to resolve before starting

1. **Pricing rate card source of truth.** Phase 3 narrows the pricing TODO to
   API-key users only — do we still maintain the rate card by hand, or pull
   from a published source at build-time? (Smaller blast radius than before.)
2. **Per-project config keying.** Absolute path keys break when a project is
   moved or symlinked. Alternative: hash-of-cwd or "nearest config file in
   ancestor". Recommend absolute path for v1, revisit if it bites.
3. **Settings overlay scope default.** When the user opens the overlay, does
   the scope toggle default to "Global" or "This project"? Recommendation:
   project — fewer footguns, more frequent edit case.
4. **Subagent nesting.** Does `Task` inside a subagent render as a nested
   card under the parent, or as a peer subagent at the top level? The current
   timeline treats them as peers — confirm this matches user expectation.
5. **Auto-approve hook policy defaults (Phase 9).** My recommendation: nothing
   auto-approved out of the box; opt-in per tool.
