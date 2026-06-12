# Changelog

All notable changes to clyde are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the version numbers follow [Semantic Versioning](https://semver.org/). This file is hand-curated; GitHub release notes mirror it.

## [Unreleased]

## [1.0.0] - 2026-06-12

First public release. A terminal companion TUI for Claude Code: tile it next to your `claude` pane and see live sessions, tool activity, token usage and plan limits, todos, git diff, file explorer, and MCP servers — offline, no account, reading only the files Claude Code already writes.

### Added
- Multi-panel dashboard (now, activity, usage, diff, explorer, servers, bash ledger, cache efficiency) with three layouts (stack, tabs, multi-col), seven themes, mouse support, and per-panel help.
- Plan-limit tracking: the same 5-hour and weekly percentages as claude.ai/settings/usage via the locally stored OAuth token, with a block-tiled time-elapsed fallback when offline.
- Multi-session tabs with a Σ aggregate view and per-session context leaderboard.
- Hook notifications: approve/deny Claude Code permission requests from the dashboard. The token-bearing callback URL lives in `~/.cache/clyde/hook-url` (0600); `clyde setup` prints the settings.json snippet.
- `--demo` deterministic mock mode, `--crash-report` diagnostics bundle, `--version`, `--layout`, `--source`.
- Release supply chain: 4 platforms (linux/darwin × amd64/arm64), SPDX SBOMs, cosign keyless signature over `checksums.txt`, SLSA build provenance, sha256+cosign-verifying `install.sh`.
- Structured JSON logging to `~/.cache/clyde/clyde.log` (`CLYDE_DEBUG=1` for debug level).

### Fixed
- Mouse-wheel scrolling works on every scrollable panel (wheel promotes the panel to its scrollable active mode); Enter enters active mode in all layouts.
- The activity panel follows the live tail, shows an empty-state hint, and windowing keeps the newest calls visible.
- The usage panel's "next reset" bar tracks window time-elapsed (not quota %), picks the soonest window, anchors offline fallbacks Anthropic-style, and its countdown ticks every refresh.
- The advertised `⌃l` layout-cycle hotkey (and `⌃e`/`⌃a`/`⌃d`/`⌃0` chords) actually work.

### Security
- Localhost-only hook server with per-process auth token, fail-closed on queue overflow; pending hooks auto-denied on quit.
- Credentials are read-only: clyde never writes or refreshes the Claude Code OAuth token, and denying Keychain access degrades gracefully.
- CI: gosec, Semgrep, CodeQL, govulncheck, gitleaks config, OpenSSF Scorecard, SHA-pinned actions, default-deny workflow permissions, fuzz harnesses on every parser.

[Unreleased]: https://github.com/Systemartis/clyde/compare/v1.0.0...HEAD
[1.0.0]: https://github.com/Systemartis/clyde/releases/tag/v1.0.0
