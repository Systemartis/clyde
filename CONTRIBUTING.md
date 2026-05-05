# Contributing to Clyde

Thanks for considering a contribution! Clyde is a small, focused TUI and we'd love hearing from people who use it. Whether you're filing a bug, suggesting an idea, or sending a patch, you're welcome here.

## Code of conduct

This project follows the [Contributor Covenant](./CODE_OF_CONDUCT.md). By participating you agree to uphold it. Be kind.

## Asking questions

For open-ended questions ("how do I…", "what's the right way to…", "is this a bug?"), please use [GitHub Discussions](https://github.com/Systemartis/clyde/discussions). Issues are reserved for actionable bug reports and feature requests.

## Reporting bugs

Open a [bug report](https://github.com/Systemartis/clyde/issues/new?template=bug_report.yml). The template asks for the things we usually need: `clyde --version`, OS + terminal emulator, Claude Code version, repro steps, and what you expected vs. what happened. A short screencast (asciinema or VHS) is gold.

Please search existing issues first — chances are someone hit it too.

## Suggesting features

Open a [feature request](https://github.com/Systemartis/clyde/issues/new?template=feature_request.yml). Describe the problem you're trying to solve before describing the solution — it makes the discussion much more productive.

## Security issues

Do **not** file public issues for security vulnerabilities. See [SECURITY.md](./SECURITY.md) for the private reporting process.

## Pull requests

1. **Fork** the repo and create a topic branch from `main`. Naming convention: `feat/<short-name>`, `fix/<short-name>`, `docs/<short-name>`, `refactor/<short-name>`.
2. **Write a test first.** Clyde is built strict-TDD: a failing test, then the implementation, then refactor. Domain and application layers use table-driven tests; TUI uses `teatest/v2`.
3. **Keep commits clean.** We use [Conventional Commits](https://www.conventionalcommits.org/) — `feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`, `perf:`. Squash if your PR has noise.
4. **Make sure CI is green locally:**
   ```sh
   go test -race ./...
   golangci-lint run ./...
   gofmt -l .              # output should be empty
   ```
5. **Open the PR** using the template. Link any related issue and include screencasts for TUI changes — they help reviewers a lot.

We'll usually respond within a few days. If you don't hear back in a week, ping the PR.

## Architecture pointer

Clyde follows hexagonal / DDD layering. Before you add code, know which layer it belongs to:

- `internal/domain` — pure business types and rules. Stdlib only. No I/O, no `bubbletea`, no `os.Open`.
- `internal/application` — use cases that orchestrate the domain. Depends on `internal/ports` (interfaces), never on adapters directly.
- `internal/ports` — interfaces the application needs from the outside world.
- `internal/adapters` — I/O implementations: `jsonl` (Claude Code session reader), `systemclock`, and `tui` (the Bubble Tea program). The TUI lives at `internal/adapters/tui`.
- `cmd/clyde` — wiring + entry point.

New features generally land as a new adapter or as an extension to an existing one. **Don't reach into the domain from an adapter; don't put I/O in the domain.** These layering rules are enforced at lint-time by `golangci-lint`'s `depguard` rules in `.golangci.yml` — if you violate them the build fails before tests run.

## Need help?

Open a [Discussion](https://github.com/Systemartis/clyde/discussions) or email vlad.bejat@gmail.com. We're happy to help you find your way around.
