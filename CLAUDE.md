# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What clyde is

A terminal companion TUI for Claude Code, written in Go. Single binary, per-project scope: it reads `~/.claude/projects/<encoded-cwd>/*.jsonl` for sessions originating in the cwd it's launched from, plus `~/.claude/todos/*.json`, `~/.claude/settings.json`, and `.git/`.

The README has user-facing install/usage. CONTRIBUTING.md has the contributor flow. This file captures the things you only learn by reading the code.

## Commands

```sh
go test ./...                      # full unit + integration suite
go test -race ./...                # what CI runs (use this before claiming green)
go test ./internal/domain/session  # single package
go test -run TestUsageAdd ./...    # single test by name
go test -cover ./...               # coverage
go vet ./...
gofmt -l .                         # must produce empty output
golangci-lint run ./...            # see "Layering enforcement" below
go build ./cmd/clyde
go run ./cmd/clyde --demo          # deterministic mock data, no live reads
```

TUI tests use `teatest/v2` (golden snapshots). Re-record by setting `-update` when invoking go test on the affected package — check the test file for the exact flag name before assuming.

## Architecture

Hexagonal + DDD. Layers, with the dependency arrow pointing **inward**:

```
cmd/clyde            -> wiring/composition root (main.go)
internal/adapters/*  -> I/O: jsonl, git, fsexplorer, hookserver, anthropicapi,
                        claudesettings, mcpconfig, lspscan, processscan,
                        systemclock, tui (Bubble Tea program lives here)
internal/ports       -> interfaces the application needs (SessionSource,
                        Clock, LLMSource, MCPSource, LSPSource, ProcessSource,
                        SubagentSource, PlanUsageSource)
internal/application -> use cases (livesession, watchsession). Depends on
                        ports, never on adapters.
internal/domain      -> pure value objects + rules: session, event, project,
                        usage, pricing. Stdlib only.
```

`cmd/clyde/main.go` is the only place adapters are constructed and injected. Read it first to understand the wire-up — every adapter you'll see referenced elsewhere is plugged in there. Note the **shared `gitSource`** between `livesession` and the diff adapter, and the **shared `claudesettings.New()`** between `mcpconfig` and `lspscan`: these exist so the per-tick caches coalesce instead of each adapter spawning its own subprocess / re-parsing the same file.

### Layering enforcement

`.golangci.yml` declares `depguard` rules that forbid:

- `internal/domain/**` from importing `bubbletea`, `lipgloss`, `net/http`, `os`, fsnotify, or any adapter/application package.
- `internal/application/**` from importing adapters or UI packages.
- `internal/ports/**` from importing adapters.

These rules are active — `golangci-lint run` rejects layering violations. CONTRIBUTING.md treats this as enforced; write code as if the contract were checked at every commit, because it is.

## Strict TDD

Write the failing test first, then the implementation, then refactor. This is non-negotiable for domain and application code (table-driven tests there). For TUI work, write a `teatest/v2` snapshot or assertion before the rendering change. The `openspec/` tree (gitignored) holds Given/When/Then specs that pre-date implementations — when in doubt about expected behavior, look there.

## Project conventions

- **Module path:** `github.com/Systemartis/clyde` (recently migrated from `github.com/vladpb/clyde` — commit `70d5596`). Use the new path in any new imports.
- **Conventional Commits**, no AI co-author trailers.
- **Branches:** `feat/…`, `fix/…`, `docs/…`, `refactor/…`.
- **`docs/`, `openspec/`, and `.atl/` are gitignored** — they are internal design/spec/agent-tooling artefacts. Anything you put there will not ship; if a doc needs to be public, put it at the repo root or under `internal/` with the code.
- **Pure value objects** in domain: methods on the type (e.g. `Usage.Add`), not anemic structs + service classes. The TUI never reaches into the domain to mutate state — adapters convert domain → view-model.

## Live mode vs demo mode

`clyde --demo` runs against `mockdata.go` in the tui adapter — useful for golden tests, offline work, and reproducing UI bugs without needing real Claude sessions. Live mode (default) wires every adapter and starts a localhost hook server on a random port; the token-bearing callback URL is written to `~/.cache/clyde/hook-url` (mode 0600, never stderr), and a command-type `PreToolUse` hook in `~/.claude/settings.json` reads that file at call time so restarts need no settings edit (public snippet: README → "Hook notifications", or `clyde setup`). The hook server's lifecycle is bound to the Bubble Tea program context (`hookCancel` on shutdown). When debugging hook flow, see `docs/design/hook-setup.md` (untracked — local only).

## When adding a new feature

1. Identify the layer. New panel = adapter (`internal/adapters/tui` + maybe a new domain concept). New data source = port + adapter pair. New rule = domain.
2. Write the spec (`openspec/specs/<layer>/...`) if the change is non-trivial.
3. Failing test first.
4. Wire it in `cmd/clyde/main.go` — don't add construction logic inside the use case.
5. Run `go test -race ./...` and `golangci-lint run ./...` before opening a PR.
