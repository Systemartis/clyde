# Clyde

[![CI](https://github.com/Systemartis/clyde/actions/workflows/ci.yml/badge.svg)](https://github.com/Systemartis/clyde/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/Systemartis/clyde)](https://goreportcard.com/report/github.com/Systemartis/clyde)
[![Go Reference](https://pkg.go.dev/badge/github.com/Systemartis/clyde.svg)](https://pkg.go.dev/github.com/Systemartis/clyde)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

> Claude's best friend.

A terminal companion for Claude Code. Tile it next to your `claude` pane in tmux/cmux/Ghostty and see live sessions, tool activity, token usage, todos, subagents, and project state — all without leaving the terminal.

## Status

**V1 shipped.** Full multi-panel TUI: now panel (current tool + mascot), calls panel (agent hierarchy), usage panel (tokens + cost), diff panel (git hunks), explorer panel (filesystem tree), servers panel (MCPs + LSPs), notification banner (hook permission requests). Three layout modes: stack, tabs, multi-col. Tokyo Night theme, configurable via `~/.config/clyde/config.toml`.

Built spec-first using **SDD** (Spec-Driven Development), **DDD**, and **Hexagonal Architecture**. Tests come before code. Architectural layering is enforced at lint-time via golangci-lint depguard rules.

## Install

### Pre-built binaries (recommended)

Each release ships a `tar.gz` for `linux/{amd64,arm64}` and `darwin/{amd64,arm64}` plus a `checksums.txt`.

```sh
# Replace VERSION + OS/ARCH for your platform; check the assets list at
# https://github.com/Systemartis/clyde/releases for the exact filenames.
VERSION=0.1.0
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
curl -fsSL "https://github.com/Systemartis/clyde/releases/download/v${VERSION}/clyde_${VERSION}_${OS}_${ARCH}.tar.gz" \
  | tar -xz -C /tmp clyde
install -m 0755 /tmp/clyde "$HOME/.local/bin/clyde"   # or anywhere on $PATH
clyde --version
```

Verify the checksum before running an unfamiliar binary:

```sh
curl -fsSL "https://github.com/Systemartis/clyde/releases/download/v${VERSION}/checksums.txt" -o /tmp/clyde-checksums.txt
( cd /tmp && sha256sum -c clyde-checksums.txt --ignore-missing )
```

### Build from source (`go install`)

Requires Go 1.26+:

```sh
go install github.com/Systemartis/clyde/cmd/clyde@latest
```

The binary lands at `$(go env GOPATH)/bin/clyde` (typically `~/go/bin/clyde`). If `~/go/bin` is on your `$PATH`, you're done. Otherwise:

```sh
# Option 1 — add ~/go/bin to PATH permanently (zsh)
echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.zshrc

# Option 2 — symlink into a directory already in your PATH
ln -sf "$(go env GOPATH)/bin/clyde" ~/.local/bin/clyde
```

Verify with `which clyde && clyde --version`.

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

## Security

Found a vulnerability? Please **don't** open a public issue. See [SECURITY.md](SECURITY.md) for the disclosure process and trust model. Each tagged release ships SPDX SBOMs and a keyless cosign signature over `checksums.txt` — verify before running.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for the local-dev setup, the strict-TDD requirement, and the conventional-commits + branch-naming rules.

## Governance

[GOVERNANCE.md](GOVERNANCE.md) describes how decisions get made and who holds the keys; [MAINTAINERS.md](MAINTAINERS.md) lists the current crew.

## License

Apache 2.0 — see [LICENSE](LICENSE).

## Trademarks

**Clyde** and the visual identity are trademarks of [Systemartis](https://systemartis.com). The Apache 2.0 license covers the source code; it does not grant rights to the name or branding. Forks are welcome — please pick a different name.

**Claude** and **Claude Code** are trademarks of [Anthropic, PBC](https://www.anthropic.com). This project is not affiliated with, endorsed by, or sponsored by Anthropic. Clyde is a third-party companion that reads files Claude Code writes locally; it is not a client of any Anthropic API on its own (the optional plan-usage check uses the same credentials Claude Code already wrote to your Keychain).
