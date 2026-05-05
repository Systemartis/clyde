# Clyde — Production-Readiness Master Analysis

**Date:** 2026-05-05
**Branch:** `chore/production-readiness`
**Scope:** Take clyde from "shipped MVP" to "production-grade open-source release under Systemartis."
**Method:** 14 parallel research agents (8 skills.sh catalogs by topic + 6 codebase audits) → 2 fact-checking agents validated all findings → consolidated here.

This builds on `analysis/MASTER-ANALYSIS.md` (2026-05-05) which filed 9 findings (F-01..F-09) under a different lens. Those are still open and prerequisite to Day-1 launch; this report adds **38 production-readiness findings** (PR-01..PR-38) covering release engineering, supply chain, OSS governance, observability, CI hardening, and test depth.

---

## TL;DR

- **Foundation is solid.** Hexagonal layering, race-tested CI, govulncheck, dependabot, conventional commits, table-driven tests, 17k LOC of test code, sensible SECURITY.md trust model. None of that needs to change.
- **Two Critical gaps block any tagged release:**
  - **PR-01** — no `--version` flag at all, despite the bug-report template explicitly asking the user to run `clyde --version`.
  - **PR-02** — no goreleaser, no git tags, no signed releases, no multi-arch binaries; the only install path is `go install` from source.
- **Two High gaps will surface immediately on a Systemartis launch:**
  - **PR-03** — `LICENSE` copyright is `Vlad Popescu-Bejat`, not Systemartis.
  - **PR-07** — the hook server URL printed to stderr at startup contains the auth token; users copy/paste startup output into bug reports today.
- **Three indirect dependencies** (`charmbracelet/ultraviolet`, `charmbracelet/x/exp/golden`, `xo/terminfo`) are pinned to **pseudo-versions**, not tagged releases. The supply-chain risk auditor lens flags this; the trail-of-bits supply-chain-risk-auditor skill is purpose-built for it.
- **No structured logging.** Stderr writes corrupt the TUI mid-run. The fix is `slog`-into-a-log-file with a panic-recover wrapper around every long-lived goroutine.
- **No CodeQL, no SBOM, no signed releases, no SHA-pinned actions.** All four are baseline expectations for an OSS Go project under a corporate org. None require deep work.
- **Top 7 skills installed** as project skills (trailofbits/semgrep, trailofbits/supply-chain-risk-auditor, github/secret-scanning, github/dependabot, composiohq/changelog-generator, boristane/logging-best-practices, github/gh-cli) plus a custom `clyde-release-ritual` skill capturing the launch checklist.

---

## 1. Method

### 1.1 Wave 1 — 14 parallel research agents

| # | Agent | Output |
|---|-------|--------|
| 1 | skills.sh catalog: golang security | 7 candidates (Trail of Bits cluster) |
| 2 | skills.sh catalog: supply chain / SBOM | 5 candidates + sparseness note |
| 3 | skills.sh catalog: release engineering | 5 candidates + goreleaser-shaped gap |
| 4 | skills.sh catalog: testing & fuzzing | 7 candidates |
| 5 | skills.sh catalog: observability | 6 candidates |
| 6 | skills.sh catalog: OSS readiness | 4 candidates |
| 7 | skills.sh catalog: CI hardening | 5 candidates + GHA-shaped gap |
| 8 | skills.sh catalog: performance | 1 Go-native + methodology |
| 9 | clyde audit: dependency tree | 12 findings |
| 10 | clyde audit: release & distribution | 12 findings |
| 11 | clyde audit: CI/CD security | 12 findings |
| 12 | clyde audit: OSS readiness | 14 findings |
| 13 | clyde audit: observability & ops | 10 findings |
| 14 | clyde audit: test gaps (post-master-analysis) | 10 findings |

### 1.2 Wave 2 — 2 fact-checking agents

- **Code-claims fact-checker** verified 24 specific file:line claims from wave 1. Result: **23/24 confirmed**, 1 nuance (`vladpb` references are in doc comments only — not in production logic, but `.golangci.yml` itself is genuinely stale and that finding stands).
- **Skill-URL fact-checker** verified 18 skill URLs and their methodology claims. Result: **14/18 confirmed**, 4 nuances:
  - `zackkorman/security-review` is **5-phase**, not 6.
  - `trailofbits/differential-review` is **6 OR 7-phase** depending on whether you count Phase 0 (the source page is internally inconsistent).
  - `openai/security-ownership-map` **excludes** `.github/*` from co-change analysis (treats it as noise) rather than flagging it as a separate trust zone.
  - `sickn33/security-scanning-security-sast` lists CodeQL as a capability but only Semgrep + gosec have implementation detail.

All wording in this report uses the fact-checked version.

### 1.3 Constraint

The Go toolchain is not installed on the analysis machine, so no `go vet`, `go test -race`, `golangci-lint run`, `govulncheck` results in this report. Findings come from static reading of files cited at line-level. Each remediation step in the plan tags items as "verify locally" where toolchain output is required before merging.

---

## 2. Findings (PR-01..PR-38)

Severity scale: **Critical** (blocks launch) / **High** (must-fix in launch sprint) / **Medium** (close before 1.0) / **Low** (defer or batch) / **Info** (note for future).

### Release engineering

| ID | Sev | Where | Finding |
|----|-----|-------|---------|
| **PR-01** | **Critical** | `cmd/clyde/main.go` (no version flag) + `.github/ISSUE_TEMPLATE/bug_report.yml` (asks for it) | No `--version` flag despite bug template instructing users to run `clyde --version`. Verified: zero `runtime/debug.ReadBuildInfo()` calls in tree. Users will file bug reports without version info. |
| **PR-02** | **Critical** | repo root | No `.goreleaser.yml`. No `release.yml` workflow. No git tags (verified `git tag --list` returns empty). No multi-arch binaries. Only install path is `go install` from source — requires Go 1.26+ on the user machine. |
| **PR-06** | High | repo root | No pre-built binaries on GitHub Releases. ARM64 users (M1/M2 Macs, RPi, Graviton) must compile from source. |
| **PR-30** | Low | release flags | No `-trimpath`, no `-buildvcs`, `CGO_ENABLED` not pinned. Different machines/times produce different binary hashes. Blocks reproducible-build claims and SLSA provenance. |

### Supply chain

| ID | Sev | Where | Finding |
|----|-----|-------|---------|
| **PR-04** | High | `go.mod:17-19` | Three indirect deps on pseudo-versions, not tagged releases: `charmbracelet/ultraviolet v0.0.0-20260416...`, `charmbracelet/x/exp/golden v0.0.0-20251109...`, `xo/terminfo v0.0.0-20220910...`. Dependabot semver gating doesn't apply to these. |
| **PR-05** | High | `go.mod:6-8` | Charm modules use non-canonical `charm.land/*` redirect path. If `charm.land` DNS / vanity infrastructure is compromised, supply chain is affected without GitHub's HSM-backed release model. |
| **PR-16** | Medium | repo root + CI | No SBOM (CycloneDX/SPDX), no sigstore/cosign signing, no SLSA provenance, no GitHub artifact attestations. Standard for OSS Go releases under corporate orgs. |
| **PR-17** | Medium | CI | `govulncheck` runs but no CodeQL, no `gosec`, no Semgrep. Industry baseline for public Go OSS is at least one additional SAST. Trail of Bits' `semgrep` and `codeql` skills both list Go as first-class. |
| **PR-37** | Info | go.mod | Heavy Charmbracelet ecosystem concentration: 6 of 7 direct deps + 11 of 16 indirects. Acceptable for a TUI; document explicitly in a SUPPLY_CHAIN.md. |

### CI/CD hardening

| ID | Sev | Where | Finding |
|----|-----|-------|---------|
| **PR-19** | Medium | `.github/CODEOWNERS` (missing) | No CODEOWNERS file; cannot route workflow-file changes through a required reviewer. `openai/security-ownership-map` skill seeds this from git history. |
| **PR-20** | Medium | `.github/workflows/ci.yml:19,21,33,35,40,49,51,68,70` | All 9 `uses:` lines pin to floating tags (`@v6`, `@v9`) instead of immutable SHAs. OpenSSF Scorecard expects SHA pinning. |
| **PR-21** | Medium | `.github/workflows/ci.yml:76` | `go install golang.org/x/vuln/cmd/govulncheck@latest` — floating `@latest` install. Pin to a tagged version or use `golang/govulncheck-action`. |
| **PR-27** | Low | every `actions/checkout` step | No `with: persist-credentials: false`. Default leaves the `GITHUB_TOKEN` in `.git/config` for any subsequent step. |
| **PR-28** | Low | top of `ci.yml` | No `concurrency:` block. Rapid push / duplicate PR can run multiple workflows in parallel. |
| **PR-29** | Low | `ci.yml` | No per-job `permissions:` blocks. Top-level `contents: read` is correct, but per-job restatement makes future privilege expansions visible in diff. |
| **PR-38** | Info | `ci.yml:12` | `FORCE_JAVASCRIPT_ACTIONS_TO_NODE24: true` undocumented. CI hardening reviews flag opaque feature flags. |

### OSS governance

| ID | Sev | Where | Finding |
|----|-----|-------|---------|
| **PR-03** | High | `LICENSE:3` | `Copyright (c) 2026 Vlad Popescu-Bejat`. For a Systemartis-org OSS launch, copyright should be Systemartis (or dual). |
| **PR-13** | Medium | `README.md` | No "Claude is a trademark of Anthropic" disclaimer despite project name "clyde" + tagline "Claude's best friend." Required when building third-party tooling around a vendor's brand. |
| **PR-08** | Medium | repo root | No `CHANGELOG.md`. No tagged release means no anchor for "what changed in v0.2.0?". |
| **PR-09** | Medium | repo root | No `GOVERNANCE.md` / `MAINTAINERS.md` / `OWNERS`. Contributors don't know who reviews, how disputes are resolved, or what the long-term stewardship model is. |
| **PR-10** | Medium | `README.md` | No badges (CI status, latest release, license, Go Report Card). |
| **PR-11** | Medium | `README.md` | No screenshot, no asciinema cast, no VHS recording. For a TUI, a screencast is the primary sales tool. |
| **PR-12** | Medium | docs | No semver policy documented. README says "V1 shipped" but `git tag --list` is empty. Downstream projects can't depend on stable surface. |
| **PR-35** | Low | `CONTRIBUTING.md` | Doesn't address DCO vs CLA. Either is fine, but ambiguity leaves contributors guessing. |
| **PR-36** | Low | comments in `internal/ports/sessionsource.go:40`, `internal/adapters/jsonl/encode.go:15-22`, `internal/adapters/jsonl/jsonl.go:7` | Stale `vladpb` references in production-file doc comments (not logic — fact-checker noted nuance, but the comments are user-visible via `go doc` output). |

### Observability & ops

| ID | Sev | Where | Finding |
|----|-----|-------|---------|
| **PR-07** | High | `cmd/clyde/main.go:149-151` + `internal/adapters/hookserver/server.go:148` | `hs.URL()` includes the auth token; full URL printed to stderr at startup. Users copy startup output into bug reports today; tokens leak into shells, terminals, screenshots, screen recordings. |
| **PR-14** | Medium | `cmd/clyde/main.go:144-148` | Hook-server goroutine has no `defer recover()`. A panic anywhere inside `hs.Start` kills the process and corrupts the TUI. |
| **PR-15** | Medium | tree-wide | Zero structured logging. Only `fmt.Fprintf(os.Stderr, ...)`. Mid-run stderr writes corrupt the bubbletea-rendered terminal. |
| **PR-25** | Medium | docs + code | No `CLYDE_DEBUG` / `CLYDE_TRACE` env-var-driven verbosity. Users reporting bugs have nothing to attach. |
| **PR-26** | Medium | code | No `clyde crash-report` flow. The privacy-respecting alternative to telemetry is a local crash-dump file the user can voluntarily share. |
| **PR-24** | Medium | TUI | No "offline" / "stale" visual signal when an adapter fails mid-run. Old data persists with no timestamp. |

### Testing depth

| ID | Sev | Where | Finding |
|----|-----|-------|---------|
| **PR-22** | Medium | `cmd/clyde/main.go` | Composition root untested. Demo mode is heavily tested (`tui.NewModel()`); the live wiring (jsonl + git + fsexplorer + claudesettings + hookserver + anthropicapi + lspscan + mcpconfig + processscan → `tui.NewModelLive`) has zero integration coverage. |
| **PR-23** | Medium | `internal/adapters/anthropicapi/credentials.go:73-120` | Keychain darwin-only branch is untested. No `//go:build darwin` tagged test file. Refactors that change the Keychain contract go uncaught. |
| **PR-18** | Medium | `.github/workflows/ci.yml` | `go test -cover` runs but no threshold gate. Per-package thresholds (domain 90% / application 85% / adapters 70%) recommended by the `django-tdd` skill's per-layer pattern. |
| **PR-31** | Low | tree | Zero `func Benchmark*` functions. The 1Hz tick loop and JSONL parser are textbook benchmark targets. |
| **PR-32** | Low | tree | No mutation testing. `gremlins` or `go-mutesting` would surface tautological tests in `internal/domain` and `internal/application`. |
| **PR-33** | Low | tree | No property-based testing (`pgregory.net/rapid`). Complements `go test -fuzz` for structured-input parsers. |
| **PR-34** | Low | tree | No `goleak`-style assertion. A long-running TUI with goroutine leaks accumulates over a session. |

---

## 3. Skill ranking

### 3.1 Top picks — installed under `.claude/skills/`

| # | Skill | Source | Score | Why installed |
|---|-------|--------|-------|---------------|
| 1 | **trailofbits/semgrep** | [skills.sh](https://skills.sh/trailofbits/skills/semgrep) | ★★★★★ | Parallel SAST with curated security rulesets (ToB + 0xdea + Decurity). Hits clyde's hookserver / OAuth / shellouts directly. Filled gap PR-17. |
| 2 | **trailofbits/supply-chain-risk-auditor** | [skills.sh](https://skills.sh/trailofbits/skills/supply-chain-risk-auditor) | ★★★★ | Six-dimension dep audit (single maintainer, unmaintained, low popularity, FFI/deser, past CVEs, missing security contacts). Scopes PR-04 + PR-05 + PR-37. |
| 3 | **github/secret-scanning** | [skills.sh](https://skills.sh/github/awesome-copilot/secret-scanning) | ★★★★ | Mandatory pre-public hygiene gate. Activates GHAS secret scanning + push protection + `.github/secret_scanning.yml`. |
| 4 | **github/dependabot** | [skills.sh](https://skills.sh/github/awesome-copilot/dependabot) | ★★★★ | Tunes the existing `.github/dependabot.yml` for grouping, security-update policies, glob `directories:`. |
| 5 | **composiohq/changelog-generator** | [skills.sh](https://skills.sh/composiohq/awesome-claude-skills/changelog-generator) | ★★★★ | Bootstraps `CHANGELOG.md` from history; produces release-ready Markdown grouped by Features/Improvements/Fixes/Breaking/Security. |
| 6 | **boristane/logging-best-practices** | [skills.sh](https://skills.sh/boristane/agent-skills/logging-best-practices) | ★★★★ | Wide-events / canonical-log-line model. JSON-only, two levels (info/error). The architectural backbone for fixing PR-15. |
| 7 | **github/gh-cli** | [skills.sh](https://skills.sh/github/awesome-copilot/gh-cli) | ★★★ | The execution layer. `gh release create / upload / verify`. |
| 8 | **clyde-release-ritual** | (custom) | n/a | Captures the launch checklist as a single skill so future releases follow the same ordered ritual. |

### 3.2 Cited but not installed

These were strong candidates but either overlap with installed skills, are too narrow, or apply only to non-clyde stacks:

- **trailofbits/codeql** ★★★★ — Adds CodeQL on top of govulncheck + Semgrep; high-value but the install + maintenance cost (database build per CI run) is non-trivial. Recommended for **post-1.0** when contributor surface grows. Captured in plan as a Phase-2 item.
- **trailofbits/differential-review** ★★★★ — PR-time security review with blast-radius analysis. Worth installing once external contributor PRs start landing; for now Systemartis maintainers self-review.
- **trailofbits/variant-analysis** ★★★★ — Systematic search for similar bugs across the tree. Best deployed reactively (after the first finding), not pre-emptively.
- **marcelorodrigo/conventional-commit** ★★★★ — Spec contract for the Conventional Commits semver mapping. Already practiced per `CONTRIBUTING.md`; spec is captured in plan rather than as a separate skill file.
- **openai/security-ownership-map** ★★★★ — CODEOWNERS seeding from git history. **Used once** to generate the initial `.github/CODEOWNERS` then archived; no ongoing value as a project skill.
- **jeffallan/golang-pro** ★★★★★ — Already installed by `chore/master-analysis`.
- **mattpocock/improve-codebase-architecture** — Already installed.
- **sickn33/test-automator** ★★★ — Mutation + property-based + contract testing. Compelling, but Phase-2 (after F-04 fuzz harnesses land).
- **boristane/logging-best-practices** companion `mindrally/logging-best-practices` — overlaps; we cherry-pick the PII/redaction section into the boristane skill.

### 3.3 Skipped — wrong stack or no fit

- All blockchain/smart-contract security skills (`trailofbits/insecure-defaults` js-flavored, `trailofbits/secure-workflow-guide` Solidity-only, `*-vulnerability-scanner` for solana/cosmos/algorand/substrate/ton/cairo).
- `trailofbits/zeroize-audit` — C/C++/Rust; doesn't apply to GC'd Go memory model.
- `trailofbits/coverage-analysis`, `trailofbits/fuzzing-obstacles` — C/C++/Rust fuzzing only.
- `trailofbits/seatbelt-sandboxer` — macOS sandbox profile generator; possibly interesting for a hardened launcher, not a security review skill.
- `wshobson/dependency-upgrade` — Node.js-centric.
- `wshobson/github-actions-templates`, `wshobson/deployment-pipeline-design`, `wshobson/secrets-management` — none cover SBOM/cosign/SLSA/Go-modules supply chain.
- `mcp-security-audit` — clyde is not an MCP server.
- `addyosmani/web-quality-skills/performance` — browser-only.
- `monitoring-guidelines` (mindrally) — production-service SLOs/probes don't apply to a single-user TUI.
- `sentry-cli` — auto-collects argv/env/host data, violates clyde's privacy stance.

### 3.4 Real gaps in the skills.sh catalog

Honest assessment from the eight catalog agents:

- **No goreleaser skill.** No skill wraps goreleaser, semantic-release, or release-please. Authoritative source is `goreleaser.com` directly.
- **No SBOM / sigstore / SLSA skill.** No skill on the directory teaches `syft`, `cyclonedx-gomod`, cosign keyless OIDC signing, or `actions/attest-build-provenance`.
- **No GitHub Actions hardening skill.** No skill teaches SHA pinning, `persist-credentials: false`, zizmor scanning, OIDC trust policies. Closest is `openai/security-ownership-map` (CODEOWNERS lens only).
- **No `slog` / `log/slog` / Go-specific structured-logging skill.** All observability skills are language-agnostic; the boristane skill is the closest fit.
- **No Bubble Tea / TUI testing skill.** No skill addresses TUI-specific testing patterns (teatest snapshots, terminal-state corruption, ANSI-aware diffing).

Where catalog gaps exist, the plan references upstream documentation directly (goreleaser docs, sigstore docs, OpenSSF Scorecard, slog docs).

---

## 4. Cross-references with `MASTER-ANALYSIS.md`

The 9 findings from `MASTER-ANALYSIS.md` (F-01..F-09) are still open and are prerequisites. This report's findings are additive — production-readiness on top of correctness. F-02 (depguard module-path drift) is reaffirmed by PR-36's grep evidence; PR-36 widens the scope to doc-comment cleanup for OSS polish.

The `plans/2026-05-05-master-analysis.md` plan should ship first; this report's plan (`plans/2026-05-05-production-readiness.md`) sequences after.

---

## 5. Recommendations summary (sequenced for the plan)

| Phase | ID(s) | Theme | Effort |
|-------|-------|-------|--------|
| 0 | F-02 + PR-36 | Restore lint enforcement; clean stale `vladpb` doc comments | XS (already in master plan) |
| 1 | PR-01 | Add `--version` flag with `runtime/debug.ReadBuildInfo` + ldflags injection | S |
| 1 | PR-03 | Update `LICENSE` copyright holder to Systemartis | XS |
| 1 | PR-07 | Stop printing token-bearing URL to stderr; write to `~/.cache/clyde/hook-url` 0600 | S |
| 1 | PR-14 | Add `defer recover()` to long-lived goroutines | S |
| 2 | PR-02 + PR-06 | Add `.goreleaser.yml`, release workflow, multi-arch builds, `v0.1.0` tag | M |
| 2 | PR-08 + PR-12 | Bootstrap `CHANGELOG.md` (changelog-generator skill); document semver policy | XS |
| 2 | PR-09 | Add `GOVERNANCE.md` and `MAINTAINERS.md` | XS |
| 2 | PR-10 + PR-11 | README badges + asciinema demo | S |
| 2 | PR-13 | Add Anthropic trademark disclaimer | XS |
| 3 | PR-19 | Generate `.github/CODEOWNERS` (security-ownership-map skill, one-off) | XS |
| 3 | PR-20 + PR-21 | SHA-pin all GitHub Actions; pin `govulncheck@v1.x` | XS |
| 3 | PR-27 + PR-28 + PR-29 | `persist-credentials: false`, `concurrency:` group, per-job `permissions:` | XS |
| 3 | PR-38 | Document or remove `FORCE_JAVASCRIPT_ACTIONS_TO_NODE24` env var | XS |
| 4 | PR-04 + PR-05 + PR-37 | Add `SUPPLY_CHAIN.md` documenting Charm concentration + pseudo-version policy | S |
| 4 | PR-16 + PR-30 | SBOM (`cyclonedx-gomod`), cosign keyless signing, `-trimpath -buildvcs`, SLSA provenance | M |
| 4 | PR-17 | Add CodeQL workflow (Go), gosec, Semgrep | M |
| 5 | PR-15 | Introduce `slog` + JSON log file at `$XDG_STATE_HOME/clyde/clyde.log` (boristane skill) | M |
| 5 | PR-25 | Add `CLYDE_DEBUG=1` env-driven verbosity | S |
| 5 | PR-26 | Add `clyde crash-report` subcommand | S |
| 5 | PR-24 | "Stale" / "offline" visual signal on adapter failure | S |
| 6 | PR-22 | Composition-root integration test (`cmd/clyde/main_integration_test.go`) | M |
| 6 | PR-23 | `//go:build darwin` Keychain test file | S |
| 6 | PR-18 | Coverage threshold gate per layer | S |
| 7 (deferred) | PR-31..PR-34 | Benchmarks, mutation testing, property-based testing, goleak | L (separate plan) |
| 7 (deferred) | PR-35 | DCO vs CLA decision + enforcement | S (governance call) |

Phases 0–4 are launch-blocking. Phase 5 elevates to "polished launch." Phase 6 is post-launch hardening. Phase 7 is ongoing.

---

## 6. References

- 14 wave-1 agent transcripts (in this conversation; not committed)
- 2 wave-2 fact-check transcripts (in this conversation)
- `analysis/MASTER-ANALYSIS.md` (prior pass)
- skills.sh URLs cited inline above
- `goreleaser.com` (authoritative for goreleaser config)
- `sigstore.dev` (cosign keyless signing)
- OpenSSF Scorecard (CI hardening)
- `pkg.go.dev/runtime/debug` (BuildInfo)
- `pkg.go.dev/log/slog` (structured logging)
