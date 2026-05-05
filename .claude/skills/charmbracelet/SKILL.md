---
name: charmbracelet
description: Use when writing/modifying clyde's TUI — Bubble Tea, Lip Gloss, Bubbles. Default to charm.land/bubbletea v2 imports. Keeps logic out of `Update`/`View` and uses `tea.Cmd` for I/O. Source — skills.sh/aaronflorey/agent-skills/charmbracelet, clyde-tuned.
---

# charmbracelet (clyde-tuned)

Authoritative skill for Bubble Tea + Lip Gloss + Bubbles work in clyde's `internal/adapters/tui` package. The upstream skill ships reference files for `bubbletea-core`, `bubbletea-patterns`, `bubbles`, `huh`, `lipgloss`, `wish`, `glamour`, `log`, `common-use-cases`, `troubleshooting-workarounds`, and `examples`. Read those directly when designing a new panel.

## clyde's invariants (don't violate)

- **All TUI code lives in `internal/adapters/tui`.** Domain and application layers stay free of `bubbletea` and `lipgloss` imports — depguard enforces this (once F-02 lands).
- **`Update` and `View` do no I/O.** Every read of `~/.claude/projects/<...>.jsonl`, every git command, every hookserver event lookup is dispatched as a `tea.Cmd` and returns via a `tea.Msg`. The use case (e.g., `internal/application/livesession`) is what runs in the goroutine the Cmd kicks off — **not the model**.
- **Per-tick adapters share construction.** `gitSource` is created once in `cmd/clyde/main.go` and shared between `livesession` and the diff adapter. `claudesettings.New()` is shared between `mcpconfig` and `lspscan`. Don't construct adapters inside `Init()` or `Update()` — that breaks the cache coalescing.
- **Mock data for tests.** `mockdata.go` is the single source of fixture state for `--demo` mode and `teatest/v2` golden snapshots. New panels must be representable in mockdata before they ship.
- **`tea.WindowSizeMsg` resizes everything.** Layouts use Lip Gloss with explicit widths derived from the last `WindowSizeMsg`. Never hardcode terminal width.
- **Background-color awareness.** Use `tea.RequestBackgroundColor` (Bubble Tea v2) when style decisions depend on light/dark terminals. clyde theming follows Tokyo Night by default but should not assume dark.
- **Logs go to stderr / file, never the TUI surface.** charm `log` package writes to `$XDG_STATE_HOME/clyde/clyde.log` — see the `logging-best-practices` skill for the redactor wrapper.

## When to reach for which library

| Need | Library | Where in clyde |
|------|---------|----------------|
| Stateful TUI runtime, key handling, message loop | Bubble Tea | `tui/model.go`, `tui/keys.go` |
| Reusable widgets (list, table, viewport, help, textinput) | Bubbles | `tui/panel_*.go` panels |
| Layout, spacing, borders, color | Lip Gloss | `tui/styles.go` |
| Forms, prompts, wizards | Huh | not currently used — would suit a setup wizard |
| Markdown → ANSI rendering | Glamour | not used — could render Claude Code session messages |
| Structured CLI logging | charm `log` | `logging-best-practices` skill — local file only |
| SSH-served TUI | Wish | **not in scope** — clyde is local-first |

## Bubble Tea v2 reminders (clyde uses v2)

- Imports: `charm.land/bubbletea/v2`, not `github.com/charmbracelet/bubbletea`. The vendored module path resolves through the `charm.land` redirect (PR-05 in production-readiness — risky single point of failure, but unavoidable upstream).
- `tea.NewProgram(...)` constructor signature changed. Use `tea.NewProgram(model, tea.WithOutput(os.Stderr), ...)` patterns from `cmd/clyde/main.go`.
- `Init() (tea.Model, tea.Cmd)` returns the model in v2 (was just `tea.Cmd` in v1). When migrating tests, watch for this.
- `tea.Quit` semantics unchanged — but `tea.QuitMsg` can be intercepted for graceful shutdown (we cancel the hookserver context in `Update` before returning `tea.Quit`).

## Testing

- Use `teatest/v2` (golden snapshots) for any visible rendering change. The flag to re-record snapshots is package-local — check the test file before assuming it's `-update`.
- For non-visible logic (key handlers, message routing), pull state out of the model into a small pure function and test that with table-driven tests in the application layer.
- `go test -race ./internal/adapters/tui` must pass on every PR — race conditions in the `tea.Program`'s message loop are common when adapters share state.

## Common breakage

- **TUI corrupts the terminal on crash** — usually a panic inside `Update` or `View`. Bubble Tea v2 partially recovers, but once stderr is mixed with the alt-screen, output is garbled. Add a deferred `recover()` per PR-14 of the production-readiness plan.
- **Slow first paint** — adapters initialised in `Init()` block the first frame. Move heavy I/O into `tea.Cmd`s that fire after the first render.
- **`charm.land` resolution fails** — vendor everything (`go mod vendor`) before publishing a release; never depend on the redirect being available at install time for production users.

## Sources

- Upstream skill: [skills.sh/aaronflorey/agent-skills/charmbracelet](https://skills.sh/aaronflorey/agent-skills/charmbracelet)
- Authoritative: [charm.land](https://charm.land), `pkg.go.dev/github.com/charmbracelet/bubbletea/v2`
- Internal: `CLAUDE.md` §Architecture, `internal/adapters/tui/*`
