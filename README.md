# Clyde

> Claude's best friend.

A terminal companion for Claude Code. Tile it next to your `claude` pane in tmux/cmux/Ghostty and see live sessions, tool activity, token usage, todos, subagents, and project state — all without leaving the terminal.

## Status

**V1 shipped.** Full multi-panel TUI: now panel (current tool + mascot), calls panel (agent hierarchy), usage panel (tokens + cost), diff panel (git hunks), explorer panel (filesystem tree), servers panel (MCPs + LSPs), notification banner (hook permission requests). Three layout modes: stack, tabs, multi-col. Tokyo Night theme, configurable via `~/.config/clyde/config.toml`.

Built spec-first using **SDD** (Spec-Driven Development), **DDD**, and **Hexagonal Architecture**. Tests come before code. Architectural layering is enforced at lint-time via golangci-lint depguard rules.

## Install

```sh
git clone <this-repo> clyde
cd clyde
go install ./cmd/clyde
```

`go install` puts the binary at `$(go env GOPATH)/bin/clyde` (typically `~/go/bin/clyde`).

If `~/go/bin` is in your `$PATH`, you're done. Otherwise either:

```sh
# Option 1 — add ~/go/bin to PATH permanently (zsh)
echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.zshrc

# Option 2 — symlink into a directory already in your PATH
ln -sf "$(go env GOPATH)/bin/clyde" ~/.local/bin/clyde
```

Verify with `which clyde`.

## Usage

```sh
cd /path/to/your/project        # any directory where you run Claude Code
clyde                           # tile this in a side pane next to `claude`
clyde --demo                    # run with deterministic mock data (no live reads)
clyde --layout=tabs             # override layout mode: stack | tabs | multi-col
```

Clyde reads real Claude Code session data from `~/.claude/projects/<encoded-cwd>/*.jsonl` by default. On first launch in live mode it also starts a localhost hook server for permission-request notifications (see `docs/design/hook-setup.md`).

Clyde detects Claude Code sessions for the current working directory. The encoding follows Claude Code's observational scheme (any non-alphanumeric character → `-`).

If clyde shows no sessions: confirm you have run `claude` in this directory at least once (which creates the corresponding `~/.claude/projects/<encoded-cwd>/` directory). Per-project scope means clyde is intentionally local to the cwd it's launched from.

### Keybindings

| Key | Action |
|-----|--------|
| `Tab` / `Shift+Tab` | Cycle panel focus |
| `↑` / `↓` | Navigate panels |
| `Enter` | Expand / enter active mode |
| `Space` | Collapse focused panel |
| `+` / `-` | Resize panel (active mode) |
| `Ctrl+L` | Cycle layout mode |
| `?` | Toggle settings overlay |
| `q` / `Ctrl+C` | Quit |

Press `q` or `Ctrl+C` to quit.

## Stack

- **Go 1.26** + [`charm.land/bubbletea/v2`](https://charm.land) v2.0.6 + `charm.land/lipgloss/v2` v2.0.3
- **Hexagonal layering**: `internal/domain` (pure stdlib) / `internal/application` (use cases) / `internal/ports` (interfaces) / `internal/adapters/{jsonl,systemclock,tui}` / `cmd/clyde`
- **Strict TDD**: failing test → implement → green → refactor. Domain and application use cases use table-driven tests; TUI uses `teatest/v2` for end-to-end snapshots.
- **Lint enforcement**: `golangci-lint` v2.11.4 with `depguard` rules in `.golangci.yml` reject domain → adapters/UI imports, application → adapters imports, ports → adapters imports.

## Development

```sh
go test ./...                 # unit + integration tests
go test -cover ./...          # with coverage
go vet ./...                  # static checks
gofmt -l .                    # formatting (no diff = clean)
golangci-lint run ./...       # lint + hexagonal layer enforcement
go build ./cmd/clyde          # local build
```
