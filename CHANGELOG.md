# Changelog

All notable changes to clyde are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the version numbers follow [Semantic Versioning](https://semver.org/). This file is hand-curated; GitHub release notes mirror it.

## [Unreleased]

## [1.0.0-rc.2] - 2026-07-02

Hardening pass ahead of the public 1.0.0: a multi-dimension audit surfaced 38 defects; all are fixed here, and the README + demo recordings were rebuilt around the real application.

### Fixed
- Pricing: Opus-tier rates corrected ($5/$25 — costs were inflated 3×), Haiku 4.5 at $1/$5, new entries for Opus 4.8 / Sonnet 5 / Fable 5, family fallback for unknown minor versions, and `[1m]` no longer doubles prices on models where the 1M window is standard pricing.
- Live sessions: completed subagent tool calls no longer stick as "active" (slice-aliasing corruption); focusing an older session tab no longer double-counts it in the 5h/weekly aggregates.
- Git: multi-file diffs no longer misparse `---`/`+++` headers as content lines; `status -z` preserves non-ASCII and space-containing paths.
- Credentials (macOS): the Keychain is now strictly read-only — clyde no longer refreshes OAuth tokens read from it, which could rotate the refresh token out from under Claude Code and log you out of the CLI.
- Hooks: answering a permission request with `y` reliably dismisses the overlay; a burst of hook events auto-denies the superseded request instead of leaving the claude CLI blocked; hook-server errors route to the log instead of corrupting the TUI.
- Tabs layout: every advertised tab is reachable, number jumps match the visible tabs, mouse clicks use real tab geometry, and the footer shows the actual build version (previously hardcoded `v0.5.0-proto`/`v0.6.0-proto`).
- Viewer/editor: find highlights and horizontal scrolling are rune-aware (no more corrupted multi-byte characters), stale find highlights clear on edit, undo back to the saved state clears the dirty marker, and every demo-mode file opens with real content.
- Settings: closing the overlay no longer bakes a transient `--layout` override into config.toml; concurrent clyde instances merge their config writes instead of clobbering each other; invalid `default_mode` values sanitize to `stack`; legacy `[panels.tasks]` maps onto the calls panel as documented.
- Config path honors `XDG_CONFIG_HOME` (the cache already did) — sandboxed runs no longer read or write the real user config.

### Changed
- The usage panel's "next reset" bar is bound strictly to the 5-hour window — the weekly countdown lives on its own row and no longer takes over the bar.
- Window reset anchoring merges the block-tiling implementation with per-session first-event anchors cached alongside usage.

### Docs
- README rewritten around the real control surface: full keybinding reference (plain-letter panel jumps, session tabs, viewer/editor keys, ⌃ chords, mouse), bash/cache panels, all 7 themes, per-project layout memory, privacy/trust model, and an end-to-end-verified hook setup section.
- `assets/` replaced by four reproducible VHS tapes + GIFs under `demo/` (hero, observability, workspace, customization) — every keypress is a live binding and recordings are sandboxed from the real config.
- SLSA claims corrected to Build Level 2 in `SUPPLY_CHAIN.md` and the release workflow.

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

[Unreleased]: https://github.com/Systemartis/clyde/compare/v1.0.0-rc.2...HEAD
[1.0.0-rc.2]: https://github.com/Systemartis/clyde/releases/tag/v1.0.0-rc.2
[1.0.0]: https://github.com/Systemartis/clyde/releases/tag/v1.0.0
