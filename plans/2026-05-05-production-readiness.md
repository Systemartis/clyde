# Production-Readiness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Take clyde from "shipped MVP" to "production-grade open-source release under Systemartis." Close the 38 findings (PR-01..PR-38) recorded in `analysis/PRODUCTION-READINESS.md`.

**Architecture:** No internal/ behavior changes from this plan — all work is in `cmd/clyde`, `.github/`, repo root files, new `internal/adapters/clydelog`, and a new `internal/version` package. Hexagonal layering preserved.

**Tech Stack:** Go 1.26 · `goreleaser` (new) · `cosign` (new) · `cyclonedx-gomod` (new) · `slog` (Go stdlib) · `gh` CLI · GitHub Actions · existing `golangci-lint` + `govulncheck`.

**Sequencing:** Phase 0 (master-analysis prerequisites) → Phase 1 (Day-1 must-fixes) → Phase 2 (release infra) → Phase 3 (CI hardening) → Phase 4 (supply chain) → Phase 5 (observability) → Phase 6 (test depth). Phase 7 deferred to its own plan.

---

## Reading order before starting

1. `analysis/PRODUCTION-READINESS.md` — full evidence and rationale for every PR-NN.
2. `analysis/MASTER-ANALYSIS.md` — F-01..F-09 prerequisites (Phase 0).
3. `plans/2026-05-05-master-analysis.md` — the existing Phase-0 plan; finish it first.
4. `.claude/skills/clyde-release-ritual/SKILL.md` — the launch ritual this plan terminates in.
5. `SECURITY.md` — trust model that PR-07, PR-15, PR-26 sit on.

---

## Phase 0: Master analysis prerequisites

Run `plans/2026-05-05-master-analysis.md` to completion. F-02 (depguard module-path drift) is mandatory before any of the work below ships, because Phase 5 introduces a new package (`internal/adapters/clydelog`) that needs the layering check to fire.

The PR-36 finding from this report (stale `vladpb` doc comments) folds into Task 1 of the master-analysis plan — extend that PR to include the comment cleanup in `internal/ports/sessionsource.go:40`, `internal/adapters/jsonl/encode.go:15-22`, `internal/adapters/jsonl/jsonl.go:7`.

**Exit criterion:** all 8 master-analysis tasks merged; `golangci-lint run ./...` clean; tripwire test still rejects `domain → adapters` import.

---

## Phase 1: Day-1 must-fixes

Smallest, fastest, highest leverage. Each task is a separate PR.

### Task 1.1: Add `--version` flag (PR-01)

**Files:**
- Create: `internal/version/version.go`
- Modify: `cmd/clyde/main.go`
- Test: `internal/version/version_test.go`

- [ ] **Step 1.1.1: Write the failing test**

`internal/version/version_test.go`:

```go
package version

import "testing"

func TestInfo_FallsBackToBuildInfo(t *testing.T) {
    t.Parallel()
    info := Info()
    if info.Version == "" {
        t.Error("Version must be non-empty (use ldflags or BuildInfo)")
    }
    if info.GoVersion == "" {
        t.Error("GoVersion must be populated from runtime/debug")
    }
}
```

- [ ] **Step 1.1.2: Run, observe FAIL**

```
go test ./internal/version/
```

- [ ] **Step 1.1.3: Implement**

`internal/version/version.go`:

```go
// Package version exposes build metadata. Values are populated either by
// ldflags at release-build time (preferred — see .goreleaser.yml) or by
// runtime/debug.ReadBuildInfo() as a fallback for `go install` users.
package version

import (
    "runtime"
    "runtime/debug"
)

// Set by goreleaser via -ldflags "-X github.com/Systemartis/clyde/internal/version.version=..."
var (
    version = ""
    commit  = ""
    date    = ""
)

type BuildInfo struct {
    Version   string
    Commit    string
    Date      string
    GoVersion string
}

func Info() BuildInfo {
    info := BuildInfo{
        Version: version, Commit: commit, Date: date,
        GoVersion: runtime.Version(),
    }
    if info.Version == "" {
        if bi, ok := debug.ReadBuildInfo(); ok {
            if bi.Main.Version != "(devel)" && bi.Main.Version != "" {
                info.Version = bi.Main.Version
            }
            for _, s := range bi.Settings {
                switch s.Key {
                case "vcs.revision":
                    if info.Commit == "" { info.Commit = s.Value }
                case "vcs.time":
                    if info.Date == "" { info.Date = s.Value }
                }
            }
        }
    }
    if info.Version == "" { info.Version = "dev" }
    return info
}
```

- [ ] **Step 1.1.4: Wire `--version` in `cmd/clyde/main.go`**

Add to the flag parsing in `run()`:

```go
var versionFlag bool
flag.BoolVar(&versionFlag, "version", false, "print version and exit")
flag.Parse()

if versionFlag {
    info := version.Info()
    fmt.Printf("clyde %s\n", info.Version)
    if info.Commit != "" { fmt.Printf("commit: %s\n", info.Commit) }
    if info.Date != "" { fmt.Printf("built:  %s\n", info.Date) }
    fmt.Printf("go:     %s\n", info.GoVersion)
    return 0
}
```

- [ ] **Step 1.1.5: Verify**

```
go test ./internal/version/
go run ./cmd/clyde --version
```

Expected output: `clyde dev` + go version (real release will show `clyde v0.1.0`).

- [ ] **Step 1.1.6: Commit**

```bash
git commit -m "feat(cli): add --version flag (PR-01)

The bug-report template asked users to run \`clyde --version\` but
the flag did not exist. Implements an internal/version package using
runtime/debug.ReadBuildInfo() as a fallback for go-install users, with
ldflags injection for release builds."
```

### Task 1.2: Update LICENSE copyright (PR-03)

**Files:** `LICENSE`

- [ ] **Step 1.2.1: Confirm with the original author** (Vlad) that the org-copyright change is acceptable. Get written approval (PR comment is fine).
- [ ] **Step 1.2.2: Update line 3** of `LICENSE` to `Copyright (c) 2026 Systemartis SRL` (or whatever the legal entity name is). Optionally add a second line `// Original work by Vlad Popescu-Bejat` if dual attribution is wanted.
- [ ] **Step 1.2.3: Commit**

```bash
git commit -m "docs(license): update copyright to Systemartis (PR-03)"
```

### Task 1.3: Stop printing token-bearing URL to stderr (PR-07)

**Files:**
- Modify: `cmd/clyde/main.go` (lines 149-151)
- Modify: `internal/adapters/hookserver/server.go` (add `WriteHookURLFile` helper, optional)
- Test: `internal/adapters/hookserver/server_test.go`

- [ ] **Step 1.3.1: Write failing test**

```go
func TestServer_HookURLNotInStderr(t *testing.T) {
    // Run cmd/clyde -demo with hookserver wired, capture stderr,
    // assert it does NOT match /\?t=[a-f0-9]{64}/ regex.
}
```

(This is an integration test; may need a separate `cmd/clyde/main_integration_test.go` from Task 6.1.)

- [ ] **Step 1.3.2: Replace stderr URL print with file write**

In `cmd/clyde/main.go`, replace:

```go
fmt.Fprintf(os.Stderr, "clyde: hook server on port %d ...", hs.Port())
fmt.Fprintf(os.Stderr, `  "hooks": ... "url": "%s" ...`, hs.URL())
```

with:

```go
hookURLPath := filepath.Join(xdgCacheHome(), "clyde", "hook-url")
_ = os.MkdirAll(filepath.Dir(hookURLPath), 0o700)
_ = os.WriteFile(hookURLPath, []byte(hs.URL()+"\n"), 0o600)
fmt.Fprintf(os.Stderr, "clyde: hook server on port %d (url written to %s, mode 0600)\n",
    hs.Port(), hookURLPath)
```

- [ ] **Step 1.3.3: Document in README**

Add to README a note: "On first run, clyde writes the hook callback URL to `~/.cache/clyde/hook-url` (mode 0600). Add it to your Claude Code `~/.claude/settings.json` `hooks` block."

- [ ] **Step 1.3.4: Verify, commit**

```bash
go test -race ./internal/adapters/hookserver/
git commit -m "fix(hookserver): write token-bearing URL to ~/.cache/clyde/hook-url instead of stderr (PR-07)"
```

### Task 1.4: Add panic recovery to hookserver goroutine (PR-14)

**Files:** `cmd/clyde/main.go` (lines 144-148)

- [ ] **Step 1.4.1: Wrap the goroutine**

```go
go func() {
    defer func() {
        if r := recover(); r != nil {
            fmt.Fprintf(os.Stderr,
                "clyde: hook server panicked: %v (continuing without hooks)\n", r)
        }
    }()
    if serveErr := hs.Start(hookCtx); serveErr != nil {
        // ...
    }
}()
```

- [ ] **Step 1.4.2: Verify**

```
go test -race ./...
```

- [ ] **Step 1.4.3: Commit**

```bash
git commit -m "fix(main): add defer recover around hookserver goroutine (PR-14)"
```

---

## Phase 2: Release infrastructure

This phase ends with `git tag v0.1.0` shipping a signed multi-arch release. **Most leverage in the entire plan.**

### Task 2.1: Author `.goreleaser.yml` (PR-02 + PR-06 + PR-30)

**Files:**
- Create: `.goreleaser.yml`
- Create: `.github/workflows/release.yml`

- [ ] **Step 2.1.1: Install goreleaser locally** for testing: `brew install goreleaser`.

- [ ] **Step 2.1.2: Create `.goreleaser.yml`**

```yaml
version: 2
project_name: clyde

before:
  hooks:
    - go mod tidy
    - go test -race ./...

builds:
  - id: clyde
    main: ./cmd/clyde
    binary: clyde
    env:
      - CGO_ENABLED=0
    flags:
      - -trimpath
      - -buildvcs=true
    ldflags:
      - -s -w
      - -X github.com/Systemartis/clyde/internal/version.version={{.Version}}
      - -X github.com/Systemartis/clyde/internal/version.commit={{.Commit}}
      - -X github.com/Systemartis/clyde/internal/version.date={{.Date}}
    goos:    [linux, darwin, windows]
    goarch:  [amd64, arm64]
    ignore:
      - goos: windows
        goarch: arm64

archives:
  - format: tar.gz
    name_template: >-
      {{ .ProjectName }}_{{ .Version }}_{{- title .Os }}_
      {{- if eq .Arch "amd64" }}x86_64
      {{- else }}{{ .Arch }}{{ end }}
    format_overrides:
      - goos: windows
        format: zip
    files:
      - LICENSE
      - README.md
      - CHANGELOG.md

checksum:
  name_template: 'checksums.txt'
  algorithm: sha256

sboms:
  - artifacts: archive
    documents: ["{{ .ArtifactName }}.sbom.cdx.json"]

# cosign keyless signing (Phase 4 wires this; left commented for Phase 2)
# signs:
#   - cmd: cosign
#     env: [COSIGN_EXPERIMENTAL=1]
#     args: ["sign-blob", "--yes", "--output-signature", "${signature}", "--output-certificate", "${certificate}", "${artifact}"]
#     artifacts: archive

changelog:
  disable: true   # we hand-curate CHANGELOG.md

release:
  github:
    owner: Systemartis
    name: clyde
  draft: false
  prerelease: auto
  mode: replace
  header: |
    # clyde {{ .Tag }}
    See `CHANGELOG.md` for full release notes.

# Homebrew (Phase 2.4 wires this; commented for the first run)
# brews:
#   - repository:
#       owner: Systemartis
#       name: homebrew-tap
#     directory: Formula
#     homepage: https://github.com/Systemartis/clyde
#     description: Terminal companion for Claude Code
#     license: MIT
#     test: |
#       system "#{bin}/clyde", "--version"
```

- [ ] **Step 2.1.3: Test goreleaser locally**

```bash
goreleaser release --snapshot --clean
ls dist/
./dist/clyde_*_darwin_arm64/clyde --version    # smoke test
```

Expected: 5 binaries, 5 archives, `checksums.txt`, 5 SBOM files.

- [ ] **Step 2.1.4: Author `.github/workflows/release.yml`**

```yaml
name: release

on:
  push:
    tags: ["v*"]

permissions:
  contents: write
  id-token: write
  attestations: write

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@<full-SHA>  # pinned in Phase 3
        with:
          fetch-depth: 0
          persist-credentials: false
      - uses: actions/setup-go@<full-SHA>
        with:
          go-version: '1.26'
          cache: true
      - uses: anchore/sbom-action/download-syft@<full-SHA>
      - uses: sigstore/cosign-installer@<full-SHA>
      - uses: goreleaser/goreleaser-action@<full-SHA>
        with:
          version: latest
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

- [ ] **Step 2.1.5: Test workflow on a feature branch with a `vX.Y.Z-rc1` tag** before tagging real `v0.1.0`. Iterate until green.

- [ ] **Step 2.1.6: Commit**

```bash
git commit -m "feat(release): add goreleaser config + release workflow (PR-02, PR-06, PR-30)"
```

### Task 2.2: Bootstrap CHANGELOG.md (PR-08 + PR-12)

Use the `changelog-generator` skill at `.claude/skills/changelog-generator/SKILL.md`.

- [ ] **Step 2.2.1: Create `CHANGELOG.md`** in Keep a Changelog format (see skill).
- [ ] **Step 2.2.2: Backfill `[0.1.0]` entry** — categorize commits since `d5e916b` into Added/Changed/Fixed/Security. Hand-edit into customer-grade voice.
- [ ] **Step 2.2.3: Add semver policy section** to README:

```markdown
## Versioning
clyde follows [Semantic Versioning 2.0.0](https://semver.org). Pre-1.0 (0.x.y), patch
releases are bug fixes; minor releases may include breaking changes. Once 1.0.0 ships,
we commit to no breaking changes in minor versions. Breaking changes are clearly
marked in `CHANGELOG.md`.
```

- [ ] **Step 2.2.4: Commit**

### Task 2.3: GOVERNANCE.md, MAINTAINERS.md (PR-09)

- [ ] **Step 2.3.1: Author `GOVERNANCE.md`**

Defines: maintainer team (initial seat: Systemartis), decision process (consensus among maintainers, Systemartis breaks ties), how to become a maintainer (sustained quality contributions over 6 months), escalation (security → SECURITY.md private flow, conduct → CODE_OF_CONDUCT.md enforcement contact).

- [ ] **Step 2.3.2: Author `MAINTAINERS.md`** with a single named maintainer team and contact emails.
- [ ] **Step 2.3.3: Commit**

### Task 2.4: README badges + asciinema demo (PR-10 + PR-11)

- [ ] **Step 2.4.1: Add badges row** below the title in README.md:

```markdown
[![CI](https://github.com/Systemartis/clyde/actions/workflows/ci.yml/badge.svg)](https://github.com/Systemartis/clyde/actions/workflows/ci.yml)
[![Latest release](https://img.shields.io/github/v/release/Systemartis/clyde)](https://github.com/Systemartis/clyde/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/Systemartis/clyde)](https://goreportcard.com/report/github.com/Systemartis/clyde)
[![govulncheck](https://github.com/Systemartis/clyde/actions/workflows/ci.yml/badge.svg?event=push&job=vuln)](https://github.com/Systemartis/clyde/actions/workflows/ci.yml)
```

- [ ] **Step 2.4.2: Record asciinema** of `clyde --demo`. Cycle layouts. Trigger a hook approval. ~30 seconds. Upload to asciinema.org or commit a `.cast` file under `docs/screencast/` (un-gitignore that path).
- [ ] **Step 2.4.3: Embed in README:**

```markdown
[![asciicast](https://asciinema.org/a/<id>.svg)](https://asciinema.org/a/<id>)
```

- [ ] **Step 2.4.4: Commit**

### Task 2.5: Trademark disclaimer (PR-13)

- [ ] **Step 2.5.1: Add to README footer:**

```markdown
---
Claude is a trademark of Anthropic, PBC. clyde is an independent open-source project not affiliated with, endorsed by, or sponsored by Anthropic. clyde reads from local Claude Code session files only and does not modify Anthropic's products.
```

- [ ] **Step 2.5.2: Commit**

### Task 2.6: Tag `v0.1.0`

Use `clyde-release-ritual` skill. After Tasks 2.1–2.5 are merged on `main`:

- [ ] CI green
- [ ] CHANGELOG `[Unreleased]` → `[0.1.0]`
- [ ] `git tag -a v0.1.0 -m "v0.1.0"` && `git push origin v0.1.0`
- [ ] Watch the release workflow
- [ ] Verify the GitHub Release page lists 5 binaries + checksums + SBOMs

---

## Phase 3: CI hardening

All XS effort. Ship as a single PR per item or one big bundle.

### Task 3.1: Generate CODEOWNERS (PR-19)

Use the `openai/security-ownership-map` skill (one-off).

- [ ] **Step 3.1.1: Run `gitleaks` against history first** (Phase 4 will do this formally; this run is just to confirm clean history).
- [ ] **Step 3.1.2: Generate CODEOWNERS from git history** using the skill, OR hand-write:

```
# .github/CODEOWNERS
*                                @Systemartis/maintainers
/.github/                        @Systemartis/maintainers
/internal/adapters/hookserver/   @Systemartis/maintainers
/internal/adapters/anthropicapi/ @Systemartis/maintainers
/SECURITY.md                     @Systemartis/maintainers
/.golangci.yml                   @Systemartis/maintainers
/.goreleaser.yml                 @Systemartis/maintainers
```

- [ ] **Step 3.1.3: Commit**

### Task 3.2: SHA-pin all GitHub Actions (PR-20)

Use the `gh action-ref` workflow or visit each action's GitHub Releases page.

- [ ] **Step 3.2.1:** For every `uses: <org>/<repo>@<tag>` in `.github/workflows/*.yml`, replace `<tag>` with the full 40-char SHA, keep the version as a comment:

```yaml
- uses: actions/checkout@b4ffde65f46336ab88eb53be808477a3936bae11  # v6.0.0
- uses: actions/setup-go@d35c59abb061a4a6fb18e82ac0862c26744d6ab5   # v6.0.0
- uses: golangci/golangci-lint-action@a4f60bb28d35aeee14e6880718e0c85ff1882e64  # v9.0.0
```

- [ ] **Step 3.2.2:** Verify the SHAs match the listed version on each repo's Releases page.
- [ ] **Step 3.2.3:** Update `.github/dependabot.yml` to keep these SHAs current automatically (already covered).
- [ ] **Step 3.2.4: Commit**

### Task 3.3: Pin govulncheck (PR-21)

- [ ] **Step 3.3.1:** Replace `go install golang.org/x/vuln/cmd/govulncheck@latest` with `@v1.1.4` (or current) in `.github/workflows/ci.yml:76`.
- [ ] **Step 3.3.2:** Add to `.github/dependabot.yml` a pattern for `golang.org/x/vuln/cmd/govulncheck` if the gomod ecosystem doesn't catch it. (It probably won't — this is a separate `go install` invocation.)
- [ ] **Step 3.3.3: Commit**

### Task 3.4: persist-credentials, concurrency, per-job permissions (PR-27 + PR-28 + PR-29)

- [ ] **Step 3.4.1:** Add `with: persist-credentials: false` to every `actions/checkout` step.
- [ ] **Step 3.4.2:** Add at the top of `ci.yml`:

```yaml
concurrency:
  group: ${{ github.workflow }}-${{ github.ref }}
  cancel-in-progress: true
```

- [ ] **Step 3.4.3:** Add per-job `permissions:` blocks restating `contents: read` (and nothing more for current jobs).
- [ ] **Step 3.4.4: Commit**

### Task 3.5: Document or remove `FORCE_JAVASCRIPT_ACTIONS_TO_NODE24` (PR-38)

- [ ] **Step 3.5.1:** If still needed, add an inline comment in `ci.yml:11` linking to the GitHub blog post or runner docs that explain the flag.
- [ ] **Step 3.5.2:** If no longer needed (Node 24 is now the default), remove the env var.
- [ ] **Step 3.5.3: Commit**

---

## Phase 4: Supply chain — SBOM + signing + SAST

### Task 4.1: Author SUPPLY_CHAIN.md (PR-04 + PR-05 + PR-37)

Use `supply-chain-risk-auditor` skill at `.claude/skills/supply-chain-risk-auditor/SKILL.md`.

- [ ] **Step 4.1.1:** Run the audit workflow from the skill across `go.mod`.
- [ ] **Step 4.1.2:** Author `SUPPLY_CHAIN.md` with:
  - Direct deps table (8 columns from skill)
  - Pseudo-version section (PR-04) — outreach plan to Charm + replacement plan for `xo/terminfo`
  - Charm concentration (PR-37) — accepted with documented reasoning
- [ ] **Step 4.1.3:** Open issues upstream:
  - charmbracelet/ultraviolet — request a tagged release
  - charmbracelet/x/exp/golden — request a tagged release or move out of the dep tree
- [ ] **Step 4.1.4:** Replace `xo/terminfo` if a less-stale alternative exists (`golang.org/x/term`?) — if it's pulled by a dep we don't control, document and move on.
- [ ] **Step 4.1.5: Commit**

### Task 4.2: Wire SBOM generation in goreleaser (PR-16)

- [ ] **Step 4.2.1:** The `.goreleaser.yml` already declares `sboms:` in Phase 2. Verify SBOM files are produced on the next snapshot release: `goreleaser release --snapshot --clean && ls dist/*.sbom*`.
- [ ] **Step 4.2.2:** Ensure `cyclonedx-gomod` or `syft` is the configured generator. goreleaser defaults to syft via `anchore/sbom-action`.
- [ ] **Step 4.2.3: Commit**

### Task 4.3: Wire cosign keyless signing (PR-16)

- [ ] **Step 4.3.1:** Uncomment the `signs:` block in `.goreleaser.yml`.
- [ ] **Step 4.3.2:** Verify `release.yml` workflow has `id-token: write`.
- [ ] **Step 4.3.3:** Add `actions/attest-build-provenance` for SLSA L3 provenance.
- [ ] **Step 4.3.4:** Update `SECURITY.md` with verification recipe (cosign verify-blob + slsa-verifier).
- [ ] **Step 4.3.5: Tag `v0.1.1` (or rc) to test.** Verify `cosign verify-blob` succeeds against the published signature.
- [ ] **Step 4.3.6: Commit**

### Task 4.4: Add Semgrep + gosec + CodeQL (PR-17)

Use the `trailofbits-semgrep` skill.

- [ ] **Step 4.4.1: Add Semgrep job to `ci.yml`:**

```yaml
  semgrep:
    runs-on: ubuntu-latest
    permissions: { contents: read, security-events: write }
    steps:
      - uses: actions/checkout@<SHA>
        with: { persist-credentials: false }
      - uses: returntocorp/semgrep-action@<SHA>
        with:
          config: >-
            p/golang
            p/security-audit
            p/owasp-top-ten
        env: { SEMGREP_RULES_CACHE_DIR: /tmp/semgrep-rules-cache }
```

- [ ] **Step 4.4.2: Add gosec job:**

```yaml
  gosec:
    runs-on: ubuntu-latest
    permissions: { contents: read, security-events: write }
    steps:
      - uses: actions/checkout@<SHA>
        with: { persist-credentials: false }
      - uses: securego/gosec@<SHA>
        with: { args: "-fmt sarif -out gosec.sarif ./..." }
      - uses: github/codeql-action/upload-sarif@<SHA>
        with: { sarif_file: gosec.sarif }
```

- [ ] **Step 4.4.3: Add CodeQL workflow:**

`.github/workflows/codeql.yml`:

```yaml
name: codeql
on:
  push: { branches: [main] }
  pull_request:
  schedule: [{ cron: '0 6 * * 1' }]
permissions: { contents: read, security-events: write }
jobs:
  analyze:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@<SHA>
        with: { persist-credentials: false }
      - uses: github/codeql-action/init@<SHA>
        with: { languages: go }
      - uses: github/codeql-action/analyze@<SHA>
```

- [ ] **Step 4.4.4: Triage findings.** First run will likely produce some. File each in `analysis/SEMGREP-FINDINGS.md` per the skill. Fix or annotate.
- [ ] **Step 4.4.5: Commit**

---

## Phase 5: Observability — slog + crash-report

Use the `logging-best-practices` skill.

### Task 5.1: Introduce structured logging (PR-15)

**Files:**
- Create: `internal/adapters/clydelog/log.go`
- Create: `internal/adapters/clydelog/log_test.go`
- Modify: `cmd/clyde/main.go`

- [ ] **Step 5.1.1: Author `clydelog` package** per the skill template (slog JSON handler, redactor, XDG state dir).
- [ ] **Step 5.1.2: Author redactor tests** — assert that `sk-ant-...` strings, 64-char hex hookserver tokens, and full home-dir paths are stripped from log records.
- [ ] **Step 5.1.3: Wire in `main.go`:**

```go
logFile, log, err := clydelog.Open()
if err != nil {
    fmt.Fprintf(os.Stderr, "clyde: cannot open log file: %v\n", err)
    // continue anyway — TUI still works without logs
} else {
    defer logFile.Close()
    slog.SetDefault(log)
}
```

- [ ] **Step 5.1.4: Replace `fmt.Fprintf(os.Stderr, ...)` calls in adapters with `slog.Error(...)`** for runtime errors. Keep stderr ONLY for startup-banner messages that fire BEFORE `tea.NewProgram(model).Run()`.
- [ ] **Step 5.1.5: Commit**

### Task 5.2: Add CLYDE_DEBUG env-driven verbosity (PR-25)

- [ ] **Step 5.2.1:** In `clydelog.Open()`, switch handler level to `slog.LevelDebug` when `os.Getenv("CLYDE_DEBUG") != ""`.
- [ ] **Step 5.2.2:** Add `slog.Debug(...)` instrumentation at hot points: hookserver request, snapshot fetch, jsonl decode error, git diff.
- [ ] **Step 5.2.3:** Document in README.

### Task 5.3: Add `clyde crash-report` subcommand (PR-26)

- [ ] **Step 5.3.1: Add subcommand handling** to `main.go` (currently flag-only; need a tiny dispatcher).
- [ ] **Step 5.3.2: `crash-report`:** read last N lines from `$XDG_STATE_HOME/clyde/clyde.log`, print to stdout, REDACTED ALREADY (because slog wrote redacted records). User reviews and pastes.
- [ ] **Step 5.3.3:** Add to README the "no telemetry" stance + crash-report flow.

### Task 5.4: Stale / offline visual signal (PR-24)

- [ ] **Step 5.4.1:** Add `LastRefreshedAt` + `StaleSince` fields to relevant TUI view-models.
- [ ] **Step 5.4.2:** In panel renderers, append `(stale)` or grey-out when stale > 30s.
- [ ] **Step 5.4.3: Visual snapshot test** for the stale state.

---

## Phase 6: Test depth

### Task 6.1: Composition-root integration test (PR-22)

**Files:** `cmd/clyde/main_integration_test.go`

- [ ] **Step 6.1.1:** Add `--dry-run` flag to `run()` that constructs the live model but doesn't call `tea.NewProgram(...).Run()`.
- [ ] **Step 6.1.2:** Test wires the live chain on a `t.TempDir()`-backed home and asserts no nil pointers, no panics, and that the model returns from `View()`.

### Task 6.2: Keychain darwin-only tests (PR-23)

**Files:** `internal/adapters/anthropicapi/credentials_darwin_test.go`

- [ ] **Step 6.2.1:** `//go:build darwin` tag.
- [ ] **Step 6.2.2:** Mock `exec.CommandContext` (via the existing seam if present) to inject Keychain responses + errors. Assert fallback to file path on Keychain miss.

### Task 6.3: Coverage threshold gate (PR-18)

- [ ] **Step 6.3.1:** Add to CI: `go test -coverprofile=coverage.out -covermode=atomic ./...`.
- [ ] **Step 6.3.2:** Author `scripts/check-coverage.sh` that fails the job when:
  - `internal/domain/...` < 90%
  - `internal/application/...` < 85%
  - `internal/adapters/...` < 70%
- [ ] **Step 6.3.3:** Add codecov badge to README (Task 2.4) once threshold is enforced.

---

## Phase 7 (deferred — separate plan)

The following findings are intentionally out of scope for this plan:

- **PR-31** — Benchmark suite. Open `plans/YYYY-MM-DD-benchmarks.md` after Phase 6. Use the `python-performance-optimization` methodology adapted to `testing.B` + `benchstat`.
- **PR-32** — Mutation testing (`gremlins` or `go-mutesting`). Best done after coverage threshold gate is stable.
- **PR-33** — Property-based testing (`pgregory.net/rapid`). Pair with the F-04 fuzz harnesses from the master-analysis plan.
- **PR-34** — `goleak` integration.
- **PR-35** — DCO vs CLA decision. Governance call by maintainers.

---

## Final verification

After Phases 0-6 land on `main`, before declaring "production-ready":

- [ ] `go test -race -cover ./...` → all PASS, thresholds met
- [ ] `gofmt -l .` → empty
- [ ] `go vet ./...` → clean
- [ ] `golangci-lint run ./...` → clean (depguard fires on tripwire)
- [ ] `govulncheck ./...` → clean
- [ ] `gitleaks detect --source .` → no findings
- [ ] `gh release view v0.1.0` shows: 5 binaries + checksums + SBOM + cosign sig + SLSA provenance
- [ ] `cosign verify-blob` succeeds against a published binary
- [ ] `slsa-verifier verify-artifact` succeeds against the same
- [ ] `clyde --version` prints expected metadata
- [ ] README shows: badges, asciinema, install instructions, semver policy, trademark disclaimer
- [ ] CHANGELOG.md has `[0.1.0]` entry; `[Unreleased]` empty
- [ ] LICENSE shows Systemartis copyright
- [ ] CODEOWNERS, GOVERNANCE.md, MAINTAINERS.md, SUPPLY_CHAIN.md exist
- [ ] CI dashboards (Actions tab) show: ci, codeql, semgrep, gosec, fuzz, smoke, lint, fmt, vuln all passing

When all green, run the `clyde-release-ritual` skill for `v0.1.0` (or `v1.0.0` if maintainers decide stability is established).

---

## Skill references

- `.claude/skills/golang-pro/SKILL.md` — pre-merge checklist
- `.claude/skills/security-review/SKILL.md` — for Phase 1, 4, 5
- `.claude/skills/trailofbits-semgrep/SKILL.md` — Phase 4
- `.claude/skills/supply-chain-risk-auditor/SKILL.md` — Phase 4 Task 4.1
- `.claude/skills/secret-scanning/SKILL.md` — pre-public sweep before any tag
- `.claude/skills/dependabot-tuning/SKILL.md` — Phase 3
- `.claude/skills/changelog-generator/SKILL.md` — Phase 2 Task 2.2
- `.claude/skills/logging-best-practices/SKILL.md` — Phase 5
- `.claude/skills/gh-cli/SKILL.md` — Phase 2.6 + every release
- `.claude/skills/clyde-release-ritual/SKILL.md` — terminates this plan
