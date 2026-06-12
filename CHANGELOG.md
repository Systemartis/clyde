# Changelog

All notable changes to clyde are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the version numbers follow [Semantic Versioning](https://semver.org/).

For releases on GitHub the canonical source is the auto-generated changelog goreleaser produces from conventional-commit messages. This file mirrors those release notes plus any unreleased work.

## [Unreleased]

### Added
- `--version` flag prints version, commit, build date, and Go runtime; populated by goreleaser ldflags or `runtime/debug.ReadBuildInfo()` fallback.
- `Makefile` with `verify` target that runs everything CI runs (fmt-check, vet, lint, race tests).
- `.editorconfig` so contributor editors match gofmt indentation conventions.
- `.goreleaser.yml` building 4 platforms (linux/darwin × amd64/arm64) with SPDX SBOMs (syft) and cosign keyless signatures over `checksums.txt`.
- CodeQL static analysis workflow (security-and-quality query suite).
- Snapshot-mode goreleaser job in CI that gates every PR on a working release pipeline.
- Pre-built binary install path documented in README with sha256 + cosign verify recipes.

### Changed
- Hook server token-bearing URL is written to `~/.cache/clyde/hook-url` (mode 0600) instead of stderr — stops leaking the per-process auth token via copy-pasted bug reports.
- `wrapPanelCollapsed` clamps `innerW` so the initial pre-WindowSize render no longer panics with `strings: negative Repeat count` when terminal width is 0.
- TUI gocyclo/gocognit lint exemptions narrowed to known-shallow files only — adjacent files now lint normally.
- All third-party GitHub Actions pinned to commit SHAs (auto-bumped weekly by Dependabot).
- govulncheck pinned to `v1.3.0` instead of `@latest`.
- Workflows default-deny permissions; jobs re-grant only what they need; checkouts run with `persist-credentials: false`.

### Fixed
- Race in `hookserver.Start` between the listener goroutine and context cancel; `serveErr` is now propagated through a buffered channel.
- Hookserver auto-allow on full event channel replaced with fail-closed deny; user re-prompts in `claude` rather than the TUI silently approving.
- Anthropic API Keychain reader captures `security` stderr instead of discarding it, so credential-not-found errors carry diagnostic text.
- Panic in the hookserver goroutine no longer takes down the TUI — `defer recover()` keeps the rest of clyde alive.

### Security
- SECURITY.md documents the hook-URL-file change, supply-chain artifacts (SBOMs + cosign), and the verify recipe.
- 5 fuzz harnesses (`testing.F`) cover the JSONL session reader, ps-output parser, git status/diff parsers, Anthropic credential reader, and Claude settings reader; CI runs each for ~15s on every PR.

[Unreleased]: https://github.com/Systemartis/clyde/compare/v0.0.0...HEAD
