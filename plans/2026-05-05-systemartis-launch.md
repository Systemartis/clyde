# Systemartis Launch — Consolidated Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Take clyde from "shipped MVP" through correctness fixes to a production-grade open-source release under Systemartis with one-command install on every major platform. Closes 47 findings (F-01..F-09 from master analysis + PR-01..PR-38 from production readiness) and adds Phase 8 install capability based on `analysis/INSTALL-OPTIONS.md`.

**Architecture:** Hexagonal — `domain` (pure stdlib) / `application` (use cases via ports) / `ports` (interfaces) / `adapters` (I/O) / `cmd/clyde` (composition root). Layering enforced by `golangci-lint`'s `depguard` rules. Phase 0 Task 1 restores that enforcement; everything afterward assumes it works. New packages added: `internal/version` (Phase 1), `internal/adapters/clydelog` (Phase 5).

**Tech Stack:** Go 1.26 · `charm.land/bubbletea/v2` · `golangci-lint v2.11.4` (depguard) · `govulncheck` · `goreleaser` (Phase 2) · `cosign` keyless (Phase 4) · `cyclonedx-gomod` + `syft` (Phase 4) · `slog` (Phase 5) · `gh` CLI · GitHub Actions.

**Phase ordering & dependencies:**

```
Phase 0: master-analysis prerequisites      ← block everything else
  ↓
Phase 1: Day-1 must-fixes                    ← --version, LICENSE, token URL, panic recovery
  ↓
Phase 2: Release infrastructure              ← .goreleaser.yml, CHANGELOG, governance, v0.1.0 tag
  ↓
Phase 3: CI hardening                        ← parallelizable with Phase 2
  ↓
Phase 4: Supply chain — SBOM, signing, SAST  ← needs Phase 2's tag working
  ↓
Phase 5: Observability — slog, crash-report  ← can start after Phase 0
  ↓
Phase 6: Test depth                          ← parallelizable with Phase 5
  ↓
Phase 7 (deferred): bench / mutation / property tests / DCO·CLA
  ↓
Phase 8: Install capability                  ← brew tap + scoop bucket + curl|sh installer
  ↓
Phase 9 (deferred): TUI module deepening (F-09)
```

---

## Reading order before starting

1. `analysis/MASTER-ANALYSIS.md` — F-01..F-09 (correctness) findings + evidence.
2. `analysis/PRODUCTION-READINESS.md` — PR-01..PR-38 findings, skill ranking, gaps in skills.sh.
3. `analysis/INSTALL-OPTIONS.md` — channel-by-channel pros/cons feeding Phase 8.
4. `CLAUDE.md` — architectural orientation, depguard known-issue note (removed by Phase 0 Task 1).
5. `SECURITY.md` — trust model that Phase 0 Tasks 2/3 + Phase 1 Task 1.3 sit on.
6. `CONTRIBUTING.md` — TDD discipline + Conventional Commits, no AI co-author trailer.
7. `.claude/skills/clyde-release-ritual/SKILL.md` — the launch ritual this plan terminates in.
8. `.claude/skills/golang-pro/SKILL.md` — pre-merge checklist for every task below.

If you change behavior in `internal/adapters/hookserver`, also re-read its package doc.

---

## Phase 0: Master analysis — correctness prerequisites

These 8 tasks come from `analysis/MASTER-ANALYSIS.md`. Tasks 5–7 can run in parallel after Task 1 lands. Tasks 2 and 3 are sequential (same file).

| # | Title | Severity | Effort | Depends on |
|---|-------|----------|--------|-----------|
| 0.1 | Restore depguard enforcement (F-02) | High | XS | — |
| 0.2 | Fix `serveErr` race (F-01) | Medium | S | 0.1 |
| 0.3 | Hookserver fail-closed on full channel (F-03) | Medium | S | 0.1, 0.2 |
| 0.4 | Add fuzz harnesses for 5 parsers (F-04) | Low | M | 0.1 |
| 0.5 | CI smoke job for `--demo` (F-07) | Low | XS | 0.1 |
| 0.6 | Capture Keychain stderr (F-05) | Low | XS | 0.1 |
| 0.7 | Shape-miss counter on `jsonl.Source` (F-06) | Info | XS | 0.1 |
| 0.8 | Narrow TUI gocyclo/gocognit exclusion (F-08) | Info | S | 0.1 |
| 0.9 | (deferred) Deepen TUI shallow modules (F-09) | Info | L | Phase 9 |

### Task 0.1: Restore depguard enforcement (F-02)

**Why first:** every later task should be merged through a working layering check. Folds in PR-36 (stale `vladpb` doc comments) from the production-readiness analysis.

**Files:**
- Modify: `.golangci.yml` (lines 56-83)
- Modify: `internal/ports/sessionsource.go:40` (doc comment)
- Modify: `internal/adapters/jsonl/encode.go:15-22` (doc comment)
- Modify: `internal/adapters/jsonl/jsonl.go:7` (doc comment)
- Modify: `CLAUDE.md` (remove "Known issue" paragraph)

- [ ] **Step 0.1.1: Inventory the stale references**

  Run: `grep -rn "vladpb" .golangci.yml internal/`
  Expected: hits in `.golangci.yml` (4×), `internal/ports/sessionsource.go` (1×), `internal/adapters/jsonl/encode.go` (≥4×), `internal/adapters/jsonl/jsonl.go` (1×). Test fixtures (`*_test.go`) keep `vladpb` strings as data — do not change.

- [ ] **Step 0.1.2: Edit `.golangci.yml`** — replace every `pkg: "github.com/vladpb/clyde/...` with `pkg: "github.com/Systemartis/clyde/...` (4 occurrences) and update `local-prefixes:` value at the bottom. Update lines 5-6's "Module path used below" comment.

- [ ] **Step 0.1.3: Update doc comments in source** — same swap in the three `internal/...` files. Leave `*_test.go` fixture data untouched.

- [ ] **Step 0.1.4: Verify lint still passes** — `golangci-lint run ./...` → zero issues.

- [ ] **Step 0.1.5: Tripwire — verify the layering rule actually fires now**

  In a throwaway commit (DO NOT push): add `import "github.com/Systemartis/clyde/internal/adapters/tui"` to `internal/domain/session/session.go` and reference one export. Run `golangci-lint run ./internal/domain/...` → expect depguard error `"domain must not depend on adapters"`. Discard with `git checkout --`.

- [ ] **Step 0.1.6: Update the known-issue note in `CLAUDE.md`** — remove the "**Known issue:**" paragraph. Replace with: "depguard rules enforce the hexagonal contract — see `.golangci.yml` for the deny list."

- [ ] **Step 0.1.7: Commit**

  ```bash
  git add .golangci.yml internal/ports/sessionsource.go internal/adapters/jsonl/encode.go internal/adapters/jsonl/jsonl.go CLAUDE.md
  git commit -m "chore(lint): restore depguard enforcement after module rename"
  ```

### Task 0.2: Fix `serveErr` race in hookserver Start (F-01)

**Files:** `internal/adapters/hookserver/server.go` (Start, ~lines 161-181), `server_test.go`

- [ ] **Step 0.2.1: Write a failing race-detector test**

  ```go
  func TestServer_StartReturnsServeError(t *testing.T) {
      t.Parallel()
      s, err := New()
      if err != nil { t.Fatal(err) }
      if err := s.listener.Close(); err != nil { t.Fatal(err) }

      ctx, cancel := context.WithCancel(context.Background())
      errCh := make(chan error, 1)
      go func() { errCh <- s.Start(ctx) }()
      time.Sleep(50 * time.Millisecond)
      cancel()

      select {
      case got := <-errCh:
          if got == nil { t.Fatal("expected serve error, got nil") }
      case <-time.After(2 * time.Second):
          t.Fatal("Start did not return")
      }
  }
  ```

- [ ] **Step 0.2.2: Run with race detector to verify it fails** — `go test -race -run TestServer_StartReturnsServeError ./internal/adapters/hookserver/`.

- [ ] **Step 0.2.3: Refactor `Start` to use a buffered error channel**

  ```go
  func (s *Server) Start(ctx context.Context) error {
      serveErrCh := make(chan error, 1)
      go func() {
          if err := s.srv.Serve(s.listener); err != nil && err != http.ErrServerClosed {
              serveErrCh <- err
              return
          }
          serveErrCh <- nil
      }()

      <-ctx.Done()

      shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
      defer cancel()

      s.once.Do(func() {
          _ = s.srv.Shutdown(shutdownCtx)
          close(s.events)
      })

      return <-serveErrCh
  }
  ```

- [ ] **Step 0.2.4: Verify** — `go test -race -count=20 -run TestServer_StartReturnsServeError ./internal/adapters/hookserver/` PASS, no race.

- [ ] **Step 0.2.5: Run the full hookserver suite** — `go test -race ./internal/adapters/hookserver/...` all PASS.

- [ ] **Step 0.2.6: Commit** — `fix(hookserver): close serveErr race on Start`.

### Task 0.3: Hookserver fail-closed on full channel (F-03)

**Files:** `internal/adapters/hookserver/server.go` (handleHook, ~lines 235-241), `server_test.go`, `SECURITY.md`.

- [ ] **Step 0.3.1: Write a failing test** for deny-on-full behavior (fill the 8-deep buffer, send 9th, expect `decision: "block"`).

- [ ] **Step 0.3.2: Confirm it fails** — `expected block decision when channel full, got "approve"`.

- [ ] **Step 0.3.3: Flip the `select`** — replace `default: writeAllow(w)` with `default: writeDeny(w, "clyde busy — re-run the tool to retry")`. Update package-level comment.

- [ ] **Step 0.3.4: Verify the test passes**.

- [ ] **Step 0.3.5: Update SECURITY.md** — append to the "Localhost hook server" section: "When the TUI cannot drain hook events fast enough (8-deep buffer), additional hook calls are denied rather than auto-approved."

- [ ] **Step 0.3.6: Commit** — `fix(hookserver): deny instead of auto-allow on full event channel`.

### Task 0.4: Fuzz harnesses for 5 parsers (F-04)

Each fuzz test follows the same shape; write one, copy.

- [ ] **Step 0.4.1: `FuzzDecodeLineWithMsgID` for JSONL**

  ```go
  func FuzzDecodeLineWithMsgID(f *testing.F) {
      seedDir := filepath.Join("testdata")
      entries, _ := os.ReadDir(seedDir)
      for _, e := range entries {
          if filepath.Ext(e.Name()) != ".jsonl" { continue }
          data, _ := os.ReadFile(filepath.Join(seedDir, e.Name()))
          for _, line := range bytes.Split(data, []byte("\n")) {
              if len(line) > 0 { f.Add(line) }
          }
      }
      f.Add([]byte(`{"type":"user","uuid":"x"}`))
      f.Add([]byte(`{"type":"assistant","message":{"id":"x"}}`))
      f.Fuzz(func(t *testing.T, raw []byte) { _, _, _ = decodeLineWithMsgID(raw) })
  }
  ```

  Run: `go test -fuzz=FuzzDecodeLineWithMsgID -fuzztime=30s ./internal/adapters/jsonl/`.

- [ ] **Step 0.4.2: `FuzzParseClaudeSessionIDs`** for processscan.

- [ ] **Step 0.4.3: `FuzzParseStatus` + `FuzzParseDiff`** for git.

- [ ] **Step 0.4.4: `FuzzDecodeEnvelope`** for anthropicapi + `FuzzReadSettings` for claudesettings (TempDir per iteration).

- [ ] **Step 0.4.5: Final full test run** — `go test -race -cover ./...`.

- [ ] **Step 0.4.6: (Optional) 90s fuzz job in CI**

  ```yaml
  fuzz:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
      - uses: actions/setup-go@v6
        with: { go-version: '1.26', cache: true }
      - run: go test -fuzz=FuzzDecodeLineWithMsgID -fuzztime=30s ./internal/adapters/jsonl/
      - run: go test -fuzz=FuzzParseClaudeSessionIDs -fuzztime=30s ./internal/adapters/processscan/
      - run: go test -fuzz=FuzzParseStatus -fuzztime=30s ./internal/adapters/git/
  ```

- [ ] **Step 0.4.7: Commit each fuzz file as a separate small commit**.

### Task 0.5: CI smoke job for `--demo` (F-07)

- [ ] **Step 0.5.1: Append to `.github/workflows/ci.yml`**

  ```yaml
  smoke:
    name: smoke (--demo)
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
      - uses: actions/setup-go@v6
        with: { go-version: '1.26', cache: true }
      - run: go build -o /tmp/clyde ./cmd/clyde
      - run: |
          set +e
          timeout --signal=TERM 3 /tmp/clyde --demo --layout=stack < /dev/null
          code=$?
          if [ $code -ne 124 ] && [ $code -ne 0 ]; then
            echo "::error::clyde --demo exited with $code"; exit 1
          fi
  ```

- [ ] **Step 0.5.2: Verify in a draft PR**.
- [ ] **Step 0.5.3: Commit** — `ci: add --demo smoke job to catch wiring regressions`.

### Task 0.6: Capture Keychain stderr (F-05)

**Files:** `internal/adapters/anthropicapi/credentials.go` (loadFromKeychain), `credentials_test.go`.

- [ ] **Step 0.6.1: Write failing test** that asserts the returned error message includes captured stderr.

- [ ] **Step 0.6.2: Replace `cmd.Stderr = io.Discard` with capped buffer**

  ```go
  var stderr bytes.Buffer
  cmd.Stderr = &stderr
  out, err := cmd.Output()
  if err != nil {
      return Credentials{}, fmt.Errorf("%w: %s", ErrCredentialsNotFound, snippet(stderr.Bytes()))
  }
  ```

  `snippet()` already exists in `oauth.go`.

- [ ] **Step 0.6.3: Tests pass** — `go test -race ./internal/adapters/anthropicapi/...`.
- [ ] **Step 0.6.4: Commit**.

### Task 0.7: Shape-miss counter on `jsonl.Source` (F-06)

**Files:** `internal/adapters/jsonl/jsonl.go` (3× `_ = json.Unmarshal` sites), `events_test.go`.

- [ ] **Step 0.7.1: Add atomic counters**

  ```go
  type Source struct {
      // ... existing
      shapeMissesUser, shapeMissesAssistant, shapeMissesContent atomic.Int64
  }
  ```

- [ ] **Step 0.7.2: Increment at the three sites** — drop the `_ =` and `//nolint:errcheck`.

- [ ] **Step 0.7.3: Add `ShapeMisses()` accessor** returning `{User, Assistant, Content int64}`.

- [ ] **Step 0.7.4: Unit test** — decode malformed message, assert counter increments.

- [ ] **Step 0.7.5: Commit**.

### Task 0.8: Narrow TUI gocyclo/gocognit exclusion (F-08)

- [ ] **Step 0.8.1: Inventory** — `golangci-lint run --no-config --enable-only=gocyclo,gocognit ./internal/adapters/tui/...`. Note offending files (likely `keys.go`, `mouse.go`, `update.go`, `panel_viewer.go`).

- [ ] **Step 0.8.2: Replace wholesale exclusion with explicit list** in `.golangci.yml`:

  ```yaml
  - path: internal/adapters/tui/(keys|mouse|update|panel_viewer)\.go$
    linters: [gocyclo, gocognit]
  ```

- [ ] **Step 0.8.3: Verify other tui files now get checked** — `golangci-lint run ./internal/adapters/tui/...` → zero issues.

- [ ] **Step 0.8.4: Commit**.

**Phase 0 exit criterion:** all 8 tasks merged; `golangci-lint run ./...` clean; tripwire test still rejects `domain → adapters` import.

---

## Phase 1: Day-1 must-fixes

Smallest, fastest, highest leverage. Each task is a separate PR. **Critical gates before any tag.**

### Task 1.1: Add `--version` flag (PR-01)

**Files:** create `internal/version/version.go`, modify `cmd/clyde/main.go`, test `internal/version/version_test.go`.

- [ ] **Step 1.1.1: Failing test**

  ```go
  package version
  import "testing"
  func TestInfo_FallsBackToBuildInfo(t *testing.T) {
      t.Parallel()
      info := Info()
      if info.Version == "" { t.Error("Version must be non-empty") }
      if info.GoVersion == "" { t.Error("GoVersion must be populated from runtime/debug") }
  }
  ```

- [ ] **Step 1.1.2: Run, observe FAIL**.

- [ ] **Step 1.1.3: Implement** `internal/version/version.go`:

  ```go
  // Package version exposes build metadata. Values are populated either by
  // ldflags at release-build time (preferred — see .goreleaser.yml) or by
  // runtime/debug.ReadBuildInfo() as a fallback for `go install` users.
  package version

  import (
      "runtime"
      "runtime/debug"
  )

  var (
      version = ""
      commit  = ""
      date    = ""
  )

  type BuildInfo struct {
      Version, Commit, Date, GoVersion string
  }

  func Info() BuildInfo {
      info := BuildInfo{Version: version, Commit: commit, Date: date, GoVersion: runtime.Version()}
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

- [ ] **Step 1.1.5: Verify** — `go run ./cmd/clyde --version` prints `clyde dev` + Go version.

- [ ] **Step 1.1.6: Commit** — `feat(cli): add --version flag (PR-01)`.

### Task 1.2: Update LICENSE copyright (PR-03)

- [ ] **Step 1.2.1: Confirm with the original author** (Vlad) that the org-copyright change is acceptable. Get written approval (PR comment).
- [ ] **Step 1.2.2: Update line 3** of `LICENSE` to `Copyright (c) 2026 Systemartis SRL`. Optionally add `// Original work by Vlad Popescu-Bejat` for dual attribution.
- [ ] **Step 1.2.3: Commit** — `docs(license): update copyright to Systemartis (PR-03)`.

### Task 1.3: Stop printing token-bearing URL to stderr (PR-07)

**Files:** `cmd/clyde/main.go` (lines 149-151), optionally a `WriteHookURLFile` helper, integration test.

- [ ] **Step 1.3.1: Failing integration test** — captures stderr from `cmd/clyde`, asserts no `?t=[a-f0-9]{64}` regex match.

- [ ] **Step 1.3.2: Replace stderr URL print with file write**

  ```go
  hookURLPath := filepath.Join(xdgCacheHome(), "clyde", "hook-url")
  _ = os.MkdirAll(filepath.Dir(hookURLPath), 0o700)
  _ = os.WriteFile(hookURLPath, []byte(hs.URL()+"\n"), 0o600)
  fmt.Fprintf(os.Stderr, "clyde: hook server on port %d (url written to %s, mode 0600)\n",
      hs.Port(), hookURLPath)
  ```

- [ ] **Step 1.3.3: Document in README** — "On first run, clyde writes the hook callback URL to `~/.cache/clyde/hook-url` (mode 0600). Add it to your Claude Code `~/.claude/settings.json` `hooks` block."

- [ ] **Step 1.3.4: Commit** — `fix(hookserver): write token-bearing URL to ~/.cache/clyde/hook-url instead of stderr (PR-07)`.

### Task 1.4: Add panic recovery to hookserver goroutine (PR-14)

**Files:** `cmd/clyde/main.go` (lines 144-148).

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
          // ...existing handling
      }
  }()
  ```

- [ ] **Step 1.4.2: Verify** — `go test -race ./...`.
- [ ] **Step 1.4.3: Commit** — `fix(main): add defer recover around hookserver goroutine (PR-14)`.

---

## Phase 2: Release infrastructure

This phase ends with `git tag v0.1.0` shipping a multi-arch release. **Use the `goreleaser` skill.** Most leverage in the entire plan.

### Task 2.1: Author `.goreleaser.yml` (PR-02 + PR-06 + PR-30)

- [ ] **Step 2.1.1: Install goreleaser locally** — `brew install goreleaser`.

- [ ] **Step 2.1.2: Create `.goreleaser.yml`** (per the `goreleaser` skill — clyde-tuned baseline):

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
      env: [CGO_ENABLED=0]
      flags: [-trimpath, -buildvcs=true]
      ldflags:
        - -s -w
        - -X github.com/Systemartis/clyde/internal/version.version={{.Version}}
        - -X github.com/Systemartis/clyde/internal/version.commit={{.Commit}}
        - -X github.com/Systemartis/clyde/internal/version.date={{.Date}}
      goos:    [linux, darwin, windows]
      goarch:  [amd64, arm64]
      ignore:
        - { goos: windows, goarch: arm64 }

  archives:
    - format: tar.gz
      name_template: >-
        {{ .ProjectName }}_{{ .Version }}_{{- title .Os }}_
        {{- if eq .Arch "amd64" }}x86_64
        {{- else }}{{ .Arch }}{{ end }}
      format_overrides:
        - { goos: windows, format: zip }
      files: [LICENSE, README.md, CHANGELOG.md]

  checksum: { name_template: 'checksums.txt', algorithm: sha256 }

  sboms:
    - artifacts: archive
      documents: ["{{ .ArtifactName }}.sbom.cdx.json"]

  # cosign keyless (Phase 4 wires this; commented for Phase 2)
  # signs:
  #   - cmd: cosign
  #     env: [COSIGN_EXPERIMENTAL=1]
  #     args: ["sign-blob", "--yes", "--output-signature", "${signature}", "--output-certificate", "${certificate}", "${artifact}"]
  #     artifacts: archive

  changelog: { disable: true }   # we hand-curate CHANGELOG.md

  release:
    github: { owner: Systemartis, name: clyde }
    draft: false
    prerelease: auto
    mode: replace
    header: |
      # clyde {{ .Tag }}
      See `CHANGELOG.md` for full release notes.

  # Homebrew brews / Scoop scoops are wired in Phase 8 — commented for Phase 2.
  ```

- [ ] **Step 2.1.3: Test goreleaser locally** — `goreleaser release --snapshot --clean`. Expect 5 binaries + archives + `checksums.txt` + 5 SBOM files. Smoke `./dist/clyde_*_darwin_arm64/clyde --version`.

- [ ] **Step 2.1.4: Author `.github/workflows/release.yml`**

  ```yaml
  name: release
  on:
    push: { tags: ["v*"] }

  permissions:
    contents: write
    id-token: write
    attestations: write

  jobs:
    release:
      runs-on: ubuntu-latest
      steps:
        - uses: actions/checkout@<full-SHA>  # pinned in Phase 3
          with: { fetch-depth: 0, persist-credentials: false }
        - uses: actions/setup-go@<full-SHA>
          with: { go-version: '1.26', cache: true }
        - uses: anchore/sbom-action/download-syft@<full-SHA>
        - uses: sigstore/cosign-installer@<full-SHA>
        - uses: goreleaser/goreleaser-action@<full-SHA>
          with: { version: latest, args: release --clean }
          env: { GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }} }
  ```

- [ ] **Step 2.1.5: Test workflow on a feature branch with `vX.Y.Z-rc1` tag**. Iterate until green.

- [ ] **Step 2.1.6: Commit** — `feat(release): add goreleaser config + release workflow (PR-02, PR-06, PR-30)`.

### Task 2.2: Bootstrap CHANGELOG.md (PR-08 + PR-12)

Use the `changelog-generator` skill.

- [ ] **Step 2.2.1: Create `CHANGELOG.md`** in Keep a Changelog format.
- [ ] **Step 2.2.2: Backfill `[0.1.0]` entry** — categorize commits since `d5e916b` into Added/Changed/Fixed/Security. Hand-edit into customer-grade voice.
- [ ] **Step 2.2.3: Add semver policy section** to README:

  ```markdown
  ## Versioning
  clyde follows [Semantic Versioning 2.0.0](https://semver.org). Pre-1.0 (0.x.y), patch
  releases are bug fixes; minor releases may include breaking changes. Once 1.0.0 ships,
  we commit to no breaking changes in minor versions. Breaking changes are clearly
  marked in `CHANGELOG.md`.
  ```

- [ ] **Step 2.2.4: Commit**.

### Task 2.3: GOVERNANCE.md, MAINTAINERS.md (PR-09)

- [ ] **Step 2.3.1: Author `GOVERNANCE.md`** — maintainer team (initial seat: Systemartis), decision process (consensus, Systemartis breaks ties), how to become a maintainer (sustained quality contributions over 6 months), escalation (security → SECURITY.md, conduct → CODE_OF_CONDUCT.md).
- [ ] **Step 2.3.2: Author `MAINTAINERS.md`** with a single named maintainer team and contact emails.
- [ ] **Step 2.3.3: Commit**.

### Task 2.4: README badges + asciinema demo (PR-10 + PR-11)

- [ ] **Step 2.4.1: Add badges row** below the title:

  ```markdown
  [![CI](https://github.com/Systemartis/clyde/actions/workflows/ci.yml/badge.svg)](https://github.com/Systemartis/clyde/actions/workflows/ci.yml)
  [![Latest release](https://img.shields.io/github/v/release/Systemartis/clyde)](https://github.com/Systemartis/clyde/releases)
  [![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
  [![Go Report Card](https://goreportcard.com/badge/github.com/Systemartis/clyde)](https://goreportcard.com/report/github.com/Systemartis/clyde)
  ```

- [ ] **Step 2.4.2: Record asciinema** of `clyde --demo` cycling layouts + a hook approval. ~30s.
- [ ] **Step 2.4.3: Embed in README** — `[![asciicast](https://asciinema.org/a/<id>.svg)](https://asciinema.org/a/<id>)`.
- [ ] **Step 2.4.4: Commit**.

### Task 2.5: Trademark disclaimer (PR-13)

- [ ] **Step 2.5.1: Add to README footer**

  ```markdown
  ---
  Claude is a trademark of Anthropic, PBC. clyde is an independent open-source project not affiliated with, endorsed by, or sponsored by Anthropic. clyde reads from local Claude Code session files only and does not modify Anthropic's products.
  ```

- [ ] **Step 2.5.2: Commit**.

### Task 2.6: Tag `v0.1.0`

Use the `clyde-release-ritual` skill. After Tasks 2.1–2.5 are merged on `main`:

- [ ] CI green
- [ ] CHANGELOG `[Unreleased]` → `[0.1.0]`
- [ ] `git tag -a v0.1.0 -m "v0.1.0"` && `git push origin v0.1.0`
- [ ] Watch `gh run watch` until release workflow completes
- [ ] Verify `gh release view v0.1.0 --json assets --jq '.assets[].name'` shows 5 binaries + checksums + SBOMs

---

## Phase 3: CI hardening

All XS effort. Bundle as one PR or ship per-item.

### Task 3.1: Generate CODEOWNERS (PR-19)

Use the `openai/security-ownership-map` skill (one-off).

- [ ] **Step 3.1.1:** Run `gitleaks` against history first (Phase 4 will do formally; this run confirms clean history).

- [ ] **Step 3.1.2:** Hand-write `.github/CODEOWNERS`:

  ```
  *                                @Systemartis/maintainers
  /.github/                        @Systemartis/maintainers
  /internal/adapters/hookserver/   @Systemartis/maintainers
  /internal/adapters/anthropicapi/ @Systemartis/maintainers
  /SECURITY.md                     @Systemartis/maintainers
  /.golangci.yml                   @Systemartis/maintainers
  /.goreleaser.yml                 @Systemartis/maintainers
  ```

- [ ] **Step 3.1.3: Commit**.

### Task 3.2: SHA-pin all GitHub Actions (PR-20)

- [ ] **Step 3.2.1:** For every `uses: <org>/<repo>@<tag>` in `.github/workflows/*.yml`, replace `<tag>` with the full 40-char SHA, keep the version as comment:

  ```yaml
  - uses: actions/checkout@b4ffde65f46336ab88eb53be808477a3936bae11  # v6.0.0
  ```

- [ ] **Step 3.2.2:** Verify SHAs match the listed version on each repo's Releases page.
- [ ] **Step 3.2.3:** Update `.github/dependabot.yml` per the `dependabot-tuning` skill — group charm/* and golang.org/x/* updates.
- [ ] **Step 3.2.4: Commit**.

### Task 3.3: Pin govulncheck (PR-21)

- [ ] **Step 3.3.1:** Replace `go install golang.org/x/vuln/cmd/govulncheck@latest` with `@v1.1.4` (or current pinned version) in `.github/workflows/ci.yml`.
- [ ] **Step 3.3.2:** Add to `.github/dependabot.yml` if gomod ecosystem doesn't catch it.
- [ ] **Step 3.3.3: Commit**.

### Task 3.4: persist-credentials, concurrency, per-job permissions (PR-27 + PR-28 + PR-29)

- [ ] **Step 3.4.1:** `with: persist-credentials: false` on every `actions/checkout` step.
- [ ] **Step 3.4.2:** Top of `ci.yml`:

  ```yaml
  concurrency:
    group: ${{ github.workflow }}-${{ github.ref }}
    cancel-in-progress: true
  ```

- [ ] **Step 3.4.3:** Per-job `permissions:` blocks restating `contents: read` (and nothing more for current jobs).
- [ ] **Step 3.4.4: Commit**.

### Task 3.5: Document or remove `FORCE_JAVASCRIPT_ACTIONS_TO_NODE24` (PR-38)

- [ ] **Step 3.5.1:** If still needed, add inline comment linking to the GitHub blog post.
- [ ] **Step 3.5.2:** If Node 24 is the default now, remove the env var.
- [ ] **Step 3.5.3: Commit**.

---

## Phase 4: Supply chain — SBOM + signing + SAST

Use the `artifact-sbom-publisher`, `trailofbits-semgrep`, and `supply-chain-risk-auditor` skills.

### Task 4.1: Author SUPPLY_CHAIN.md (PR-04 + PR-05 + PR-37)

- [ ] **Step 4.1.1:** Run the audit workflow from `supply-chain-risk-auditor` skill across `go.mod`.
- [ ] **Step 4.1.2:** Author `SUPPLY_CHAIN.md` with:
  - Direct deps table (8 columns from the skill)
  - Pseudo-version section (PR-04) — outreach plan to Charm + replacement plan for `xo/terminfo`
  - Charm concentration (PR-37) — accepted with documented reasoning
- [ ] **Step 4.1.3:** Open issues upstream — charmbracelet/ultraviolet (request tagged release), charmbracelet/x/exp/golden (request tagged release).
- [ ] **Step 4.1.4:** Replace `xo/terminfo` if a less-stale alternative exists.
- [ ] **Step 4.1.5: Commit**.

### Task 4.2: Wire SBOM generation in goreleaser (PR-16, part 1)

- [ ] **Step 4.2.1:** `.goreleaser.yml` already declares `sboms:` (Phase 2). Verify on next snapshot: `goreleaser release --snapshot --clean && ls dist/*.sbom*`.
- [ ] **Step 4.2.2:** Decide between `cyclonedx-gomod` (better Go fidelity) and `syft` (default). Per the `artifact-sbom-publisher` skill, ship **both** — gomod for the module graph, syft for the archive.
- [ ] **Step 4.2.3:** Add `cyclonedx-gomod mod` step to `release.yml` after the goreleaser step. Upload `sbom.cdx.json` alongside.
- [ ] **Step 4.2.4: Commit**.

### Task 4.3: Wire cosign keyless signing + SLSA provenance (PR-16, part 2)

Per the `artifact-sbom-publisher` skill's full workflow snippet.

- [ ] **Step 4.3.1:** Uncomment `signs:` block in `.goreleaser.yml`.
- [ ] **Step 4.3.2:** Verify `release.yml` has `id-token: write` + `attestations: write`.
- [ ] **Step 4.3.3:** Add post-goreleaser step: `actions/attest-build-provenance@v2` with `subject-path: 'dist/**/clyde*'`.
- [ ] **Step 4.3.4:** Update `SECURITY.md` with verification recipe (cosign verify-blob + slsa-verifier).
- [ ] **Step 4.3.5: Tag `v0.1.1` (or rc) to test.** Verify `cosign verify-blob` succeeds.
- [ ] **Step 4.3.6: Commit**.

### Task 4.4: Add Semgrep + gosec + CodeQL (PR-17)

Use the `trailofbits-semgrep` skill.

- [ ] **Step 4.4.1: Add Semgrep job to `ci.yml`**

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

- [ ] **Step 4.4.2: Add gosec job**

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

- [ ] **Step 4.4.3: Add CodeQL workflow** — `.github/workflows/codeql.yml`:

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

- [ ] **Step 4.4.4: Triage findings.** First run will likely produce some — file each in `analysis/SEMGREP-FINDINGS.md` per the skill. Fix or annotate.
- [ ] **Step 4.4.5: Commit**.

---

## Phase 5: Observability — slog + crash-report

Use the `logging-best-practices` skill.

### Task 5.1: Introduce structured logging (PR-15)

**Files:** create `internal/adapters/clydelog/log.go` + tests, modify `cmd/clyde/main.go`.

- [ ] **Step 5.1.1: Author `clydelog` package** per the skill template (slog JSON handler, redactor, XDG state dir at `$XDG_STATE_HOME/clyde/clyde.log`).
- [ ] **Step 5.1.2: Author redactor tests** — assert that `sk-ant-...` strings, 64-char hex hookserver tokens, and full home-dir paths are stripped from log records.
- [ ] **Step 5.1.3: Wire in `main.go`**

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

- [ ] **Step 5.1.4: Replace `fmt.Fprintf(os.Stderr, ...)` calls in adapters with `slog.Error(...)`** for runtime errors. Stderr ONLY for startup-banner messages BEFORE `tea.NewProgram(model).Run()`.
- [ ] **Step 5.1.5: Commit**.

### Task 5.2: Add `CLYDE_DEBUG` env-driven verbosity (PR-25)

- [ ] **Step 5.2.1:** In `clydelog.Open()`, switch handler level to `slog.LevelDebug` when `os.Getenv("CLYDE_DEBUG") != ""`.
- [ ] **Step 5.2.2:** Add `slog.Debug(...)` instrumentation at hot points: hookserver request, snapshot fetch, jsonl decode error, git diff.
- [ ] **Step 5.2.3:** Document in README.

### Task 5.3: Add `clyde crash-report` subcommand (PR-26)

- [ ] **Step 5.3.1: Add subcommand handling** to `main.go` (currently flag-only; need a tiny dispatcher).
- [ ] **Step 5.3.2: `crash-report`:** read last N lines from `$XDG_STATE_HOME/clyde/clyde.log`, print to stdout (already redacted by slog). User reviews and pastes.
- [ ] **Step 5.3.3:** Add to README the "no telemetry" stance + crash-report flow.

### Task 5.4: Stale / offline visual signal (PR-24)

- [ ] **Step 5.4.1:** Add `LastRefreshedAt` + `StaleSince` fields to relevant TUI view-models.
- [ ] **Step 5.4.2:** In panel renderers, append `(stale)` or grey-out when stale > 30s.
- [ ] **Step 5.4.3: Visual snapshot test** for the stale state (use `charmbracelet` skill's teatest pattern).

---

## Phase 6: Test depth

### Task 6.1: Composition-root integration test (PR-22)

**Files:** `cmd/clyde/main_integration_test.go`.

- [ ] **Step 6.1.1:** Add `--dry-run` flag to `run()` that constructs the live model but doesn't call `tea.NewProgram(...).Run()`.
- [ ] **Step 6.1.2:** Test wires the live chain on a `t.TempDir()`-backed home and asserts no nil pointers, no panics, and that the model returns from `View()`.

### Task 6.2: Keychain darwin-only tests (PR-23)

**Files:** `internal/adapters/anthropicapi/credentials_darwin_test.go` (with `//go:build darwin`).

- [ ] **Step 6.2.1:** Mock `exec.CommandContext` (via the existing test seam if present) to inject Keychain responses + errors.
- [ ] **Step 6.2.2:** Assert fallback to file path on Keychain miss.

### Task 6.3: Coverage threshold gate (PR-18)

- [ ] **Step 6.3.1:** Add to CI: `go test -coverprofile=coverage.out -covermode=atomic ./...`.
- [ ] **Step 6.3.2:** Author `scripts/check-coverage.sh` that fails the job when:
  - `internal/domain/...` < 90%
  - `internal/application/...` < 85%
  - `internal/adapters/...` < 70%
- [ ] **Step 6.3.3:** Add codecov badge to README (Task 2.4) once threshold is enforced.

---

## Phase 7 (deferred — separate plans)

Out of scope for this plan; tracked for follow-up:

- **PR-31** — Benchmark suite (`testing.B` + `benchstat`). Open `plans/YYYY-MM-DD-benchmarks.md` after Phase 6.
- **PR-32** — Mutation testing (`gremlins` or `go-mutesting`). Best done after coverage threshold gate is stable.
- **PR-33** — Property-based testing (`pgregory.net/rapid`). Pair with the F-04 fuzz harnesses.
- **PR-34** — `goleak` integration.
- **PR-35** — DCO vs CLA decision. Governance call by maintainers.

---

## Phase 8: Install capability — one-command paths

Source of decisions: `analysis/INSTALL-OPTIONS.md`. Scope: ship Tier-1 channels (brew + scoop) and Tier-2 (curl|sh + go install docs) for v0.1.0. Defer Tier-3 (eget, manual) — they work automatically. Defer companion-skill (Tier-4) until post-1.0.

### Task 8.1: Set up `Systemartis/tap` Homebrew tap repo

**Why first in this phase:** the goreleaser `brews:` block can't push without a target tap repo.

- [ ] **Step 8.1.1: Create the tap repo** — `gh repo create Systemartis/tap --public --description "Homebrew tap for Systemartis tools"`. Add empty `Formula/` directory to claim the path. Add a one-paragraph README.
- [ ] **Step 8.1.2: Provision a fine-grained PAT** scoped to `Systemartis/tap` (contents: write only). Store in clyde repo's secrets as `HOMEBREW_TAP_TOKEN`.
- [ ] **Step 8.1.3: Uncomment + configure the `brews:` block** in `.goreleaser.yml`:

  ```yaml
  brews:
    - repository:
        owner: Systemartis
        name: tap
        token: "{{ .Env.HOMEBREW_TAP_TOKEN }}"
      directory: Formula
      homepage: https://github.com/Systemartis/clyde
      description: Terminal companion for Claude Code
      license: MIT
      install: |
        bin.install "clyde"
      test: |
        assert_match version, shell_output("#{bin}/clyde --version")
      caveats: |
        Run `clyde` next to a `claude` pane. On first run, clyde writes
        the hook callback URL to ~/.cache/clyde/hook-url — add it to
        ~/.claude/settings.json's `hooks` block.
  ```

- [ ] **Step 8.1.4: Pass `HOMEBREW_TAP_TOKEN`** through to the goreleaser step in `.github/workflows/release.yml`:

  ```yaml
  - uses: goreleaser/goreleaser-action@<SHA>
    with: { version: latest, args: release --clean }
    env:
      GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
      HOMEBREW_TAP_TOKEN: ${{ secrets.HOMEBREW_TAP_TOKEN }}
  ```

- [ ] **Step 8.1.5: Test on `v0.1.0-rc2` tag** — verify the tap repo gets a `Formula/clyde.rb` PR opened by goreleaser. Merge it. Then locally:

  ```bash
  brew tap Systemartis/tap
  brew install clyde
  clyde --version    # expect: clyde v0.1.0-rc2 ...
  brew uninstall clyde && brew untap Systemartis/tap   # leave a clean machine
  ```

- [ ] **Step 8.1.6: Commit** — `feat(release): publish Homebrew tap on every tag (Phase 8)`.

### Task 8.2: Set up `Systemartis/scoop-bucket` Scoop bucket repo

- [ ] **Step 8.2.1: Create the bucket repo** — `gh repo create Systemartis/scoop-bucket --public --description "Scoop bucket for Systemartis tools"`. Add `bucket/` directory.
- [ ] **Step 8.2.2: Provision a fine-grained PAT** scoped to `Systemartis/scoop-bucket`. Store as `SCOOP_BUCKET_TOKEN`.
- [ ] **Step 8.2.3: Add `scoops:` block** to `.goreleaser.yml`:

  ```yaml
  scoops:
    - repository:
        owner: Systemartis
        name: scoop-bucket
        token: "{{ .Env.SCOOP_BUCKET_TOKEN }}"
      directory: bucket
      homepage: https://github.com/Systemartis/clyde
      description: Terminal companion for Claude Code
      license: MIT
  ```

- [ ] **Step 8.2.4: Pass `SCOOP_BUCKET_TOKEN`** through `release.yml`.
- [ ] **Step 8.2.5: Test** — on Windows (WSL won't do; need actual PowerShell or a CI Windows runner):

  ```powershell
  scoop bucket add systemartis https://github.com/Systemartis/scoop-bucket
  scoop install clyde
  clyde --version
  ```

- [ ] **Step 8.2.6: Commit** — `feat(release): publish Scoop bucket on every tag (Phase 8)`.

### Task 8.3: Author the `install.sh` curl|sh installer

**Files:** create `install.sh` at the repo root (gitignored from goreleaser archive); reference impls — bun.sh/install, astral.sh/uv/install.sh.

- [ ] **Step 8.3.1: Author `install.sh`** — must:
  1. Detect OS (`uname -s`) and arch (`uname -m`), normalize to goreleaser's archive naming.
  2. Pin a version (default: latest; override with `CLYDE_VERSION=v0.1.2`).
  3. `curl -fsSL` the archive + checksum from GitHub Releases.
  4. Verify SHA256 against the published checksum.
  5. **Verify cosign signature** if `cosign` is installed (warn but don't fail if absent — cosign is not universally installed).
  6. Extract to `/tmp`, move binary to `${CLYDE_INSTALL_DIR:-$HOME/.local/bin}/clyde`, chmod +x.
  7. Print install location + a "ensure $HOME/.local/bin is on PATH" hint.

  Skeleton:

  ```sh
  #!/bin/sh
  set -eu

  OWNER=Systemartis
  REPO=clyde
  INSTALL_DIR="${CLYDE_INSTALL_DIR:-$HOME/.local/bin}"
  VERSION="${CLYDE_VERSION:-}"

  detect_platform() {
    os="$(uname -s | tr '[:upper:]' '[:lower:]')"
    arch="$(uname -m)"
    case "$arch" in
      x86_64|amd64) arch=x86_64 ;;
      aarch64|arm64) arch=arm64 ;;
      *) echo "unsupported arch: $arch" >&2; exit 1 ;;
    esac
    case "$os" in
      linux|darwin) ;;
      *) echo "unsupported OS: $os" >&2; exit 1 ;;
    esac
    echo "${os} ${arch}"
  }

  resolve_version() {
    if [ -z "$VERSION" ]; then
      VERSION=$(curl -fsSL "https://api.github.com/repos/${OWNER}/${REPO}/releases/latest" \
        | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n1)
    fi
    [ -n "$VERSION" ] || { echo "could not resolve latest version" >&2; exit 1; }
  }

  main() {
    set -- $(detect_platform); os=$1; arch=$2
    resolve_version
    asset="${REPO}_${VERSION#v}_${os}_${arch}.tar.gz"
    url="https://github.com/${OWNER}/${REPO}/releases/download/${VERSION}/${asset}"

    tmp=$(mktemp -d)
    trap 'rm -rf "$tmp"' EXIT

    echo "downloading ${url}"
    curl -fsSL -o "${tmp}/${asset}"           "${url}"
    curl -fsSL -o "${tmp}/checksums.txt"      "https://github.com/${OWNER}/${REPO}/releases/download/${VERSION}/checksums.txt"

    # Verify SHA256
    cd "$tmp"
    grep " ${asset}$" checksums.txt | sha256sum -c -

    # Verify cosign sig if cosign is installed
    if command -v cosign >/dev/null 2>&1; then
      curl -fsSL -o "${asset}.sig" "https://github.com/${OWNER}/${REPO}/releases/download/${VERSION}/${asset}.sig"
      curl -fsSL -o "${asset}.pem" "https://github.com/${OWNER}/${REPO}/releases/download/${VERSION}/${asset}.pem"
      cosign verify-blob \
        --certificate "${asset}.pem" \
        --signature   "${asset}.sig" \
        --certificate-identity-regexp "https://github.com/${OWNER}/${REPO}/.*" \
        --certificate-oidc-issuer     "https://token.actions.githubusercontent.com" \
        "${asset}"
    else
      printf '\033[33mwarn:\033[0m cosign not installed — skipping signature verification.\n' >&2
      printf '      install cosign to enable: https://docs.sigstore.dev/cosign/installation/\n' >&2
    fi

    tar -xzf "${asset}"
    mkdir -p "$INSTALL_DIR"
    mv clyde "$INSTALL_DIR/clyde"
    chmod +x "$INSTALL_DIR/clyde"

    cat <<EOF
✓ clyde ${VERSION} installed to ${INSTALL_DIR}/clyde

Make sure ${INSTALL_DIR} is on your PATH:
  export PATH="\$HOME/.local/bin:\$PATH"

Try: clyde --version
EOF
  }

  main "$@"
  ```

- [ ] **Step 8.3.2: Hosting** — two options:

  - (a) Recommended: branded subdomain `install.systemartis.com/clyde`. Configure Cloudflare Workers / Pages or Caddy redirect to `raw.githubusercontent.com/Systemartis/clyde/main/install.sh`. Cleaner URL, brand reinforcement, easy to migrate.
  - (b) Fallback: `https://raw.githubusercontent.com/Systemartis/clyde/main/install.sh` directly. Zero infra, ugly URL.

  Document both in README. Use (a) as primary.

- [ ] **Step 8.3.3: Add `shellcheck` job to CI** for `install.sh`:

  ```yaml
  shellcheck-install:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@<SHA>
      - run: shellcheck install.sh
  ```

- [ ] **Step 8.3.4: Test the script** on a fresh Linux container + macOS (Intel + ARM):

  ```bash
  curl -fsSL https://install.systemartis.com/clyde | sh
  clyde --version
  ```

- [ ] **Step 8.3.5: Document the paranoid path** in README:

  ```bash
  curl -fsSL https://install.systemartis.com/clyde -o /tmp/install-clyde.sh
  less /tmp/install-clyde.sh    # review
  sh /tmp/install-clyde.sh
  ```

- [ ] **Step 8.3.6: Commit** — `feat(install): one-command curl|sh installer with checksum + cosign verify`.

### Task 8.4: Document `go install` path

- [ ] **Step 8.4.1: Add to README install section**

  ```markdown
  ## Install

  ### macOS / Linux
  ```sh
  brew install Systemartis/tap/clyde
  ```

  ### Windows
  ```powershell
  scoop bucket add systemartis https://github.com/Systemartis/scoop-bucket
  scoop install clyde
  ```

  ### Anywhere (curl)
  ```sh
  curl -fsSL https://install.systemartis.com/clyde | sh
  ```

  ### Go users
  ```sh
  go install github.com/Systemartis/clyde/cmd/clyde@latest
  ```

  ### Manual
  Download the archive for your platform from [Releases](https://github.com/Systemartis/clyde/releases/latest), extract, place `clyde` on your `$PATH`.
  ```

- [ ] **Step 8.4.2: Test that `go install ...@latest` works** from a clean GOPATH after Phase 1 lands (the `runtime/debug.ReadBuildInfo()` fallback handles `--version` for go-installed users).

- [ ] **Step 8.4.3: Commit** — `docs(readme): document install paths (Phase 8)`.

### Task 8.5: Smoke-test the full install matrix on `v0.1.0`

This is the last gate before announcing.

- [ ] **Step 8.5.1: Fresh macOS box** (or VM): `brew install Systemartis/tap/clyde && clyde --version` → version matches tag.
- [ ] **Step 8.5.2: Fresh Linux container** (Alpine, Ubuntu, Fedora — pick 2): `curl -fsSL https://install.systemartis.com/clyde | sh && clyde --version`.
- [ ] **Step 8.5.3: Fresh Windows box** (or GH Actions Windows runner): `scoop install clyde && clyde --version`.
- [ ] **Step 8.5.4: A Go-toolchain-only box** (e.g., golang:1.26 docker): `go install github.com/Systemartis/clyde/cmd/clyde@latest && clyde --version`.
- [ ] **Step 8.5.5: Update `.claude/skills/clyde-release-ritual/SKILL.md`** post-flight smoke section to include all four channels.

### Task 8.6 (optional, post-launch): Companion skill on skills.sh

Only after v1.0.0 stabilizes.

- [ ] **Step 8.6.1: Create** `github.com/Systemartis/clyde-skill` repo with one `SKILL.md` teaching the agent how to install and launch clyde.
- [ ] **Step 8.6.2: Submit to skills.sh** — typically auto-indexed via the `agent-skills` topic.
- [ ] **Step 8.6.3: Cross-link** from clyde's README and from the skill's SKILL.md back to clyde.

---

## Phase 9 (deferred — separate plan): TUI module deepening (F-09)

The four TUI files >900 LOC (`keys.go` 1328, `mouse.go` 1054, `panel_viewer.go` 1092, `model.go` 906) need a separate plan because each deepening is multi-day, multi-PR, and changes user-visible behavior risk.

After Phases 0-8 land:

- [ ] **Step 9.1: Open a tracking issue** titled `TUI: deepen shallow modules (keymap, viewer)` linking back to `analysis/MASTER-ANALYSIS.md` §3 F-09.
- [ ] **Step 9.2: Write a separate plan** under `plans/YYYY-MM-DD-tui-keymap-extraction.md` following the `improve-codebase-architecture` skill's three-phase process (explore → present candidates → grilling loop).

---

## Final verification

After Phases 0-8 land on `main`, before declaring "production-ready":

- [ ] `go test -race -cover ./...` → all PASS, thresholds met
- [ ] `gofmt -l .` → empty
- [ ] `go vet ./...` → clean
- [ ] `golangci-lint run ./...` → clean (depguard fires on tripwire)
- [ ] `govulncheck ./...` → clean
- [ ] `gitleaks detect --source .` → no findings
- [ ] `gh release view v0.1.0` shows: 5 binaries + checksums + 2 SBOM files (gomod + syft) + cosign sig + SLSA provenance
- [ ] `cosign verify-blob` succeeds against a published binary
- [ ] `gh attestation verify` succeeds (SLSA provenance)
- [ ] `clyde --version` prints expected metadata across all install channels
- [ ] `brew install Systemartis/tap/clyde` works on a fresh macOS + Linuxbrew machine
- [ ] `scoop install clyde` works on a fresh Windows machine
- [ ] `curl -fsSL https://install.systemartis.com/clyde | sh` works on Linux + macOS, verifies checksum + cosign sig
- [ ] `go install github.com/Systemartis/clyde/cmd/clyde@latest` works
- [ ] README shows: badges, asciinema, install paths (Phase 8), semver policy, trademark disclaimer
- [ ] CHANGELOG.md has `[0.1.0]` entry; `[Unreleased]` empty
- [ ] LICENSE shows Systemartis copyright
- [ ] CODEOWNERS, GOVERNANCE.md, MAINTAINERS.md, SUPPLY_CHAIN.md exist
- [ ] CI dashboards show: ci, codeql, semgrep, gosec, fuzz, smoke, lint, fmt, vuln, shellcheck-install all passing

When all green, run the `clyde-release-ritual` skill for `v0.1.0` (or `v1.0.0` if maintainers decide stability is established) and announce.

---

## Skill references

- [`golang-pro`](../.claude/skills/golang-pro/SKILL.md) — pre-merge checklist for every task
- [`security-review`](../.claude/skills/security-review/SKILL.md) — Phase 0 Tasks 2/3, Phase 1, Phase 4, Phase 5
- [`improve-codebase-architecture`](../.claude/skills/improve-codebase-architecture/SKILL.md) — Phase 9 (deferred)
- [`diagnose`](../.claude/skills/diagnose/SKILL.md) — apply if any task fails in CI
- [`trailofbits-semgrep`](../.claude/skills/trailofbits-semgrep/SKILL.md) — Phase 4 Task 4.4
- [`supply-chain-risk-auditor`](../.claude/skills/supply-chain-risk-auditor/SKILL.md) — Phase 4 Task 4.1
- [`secret-scanning`](../.claude/skills/secret-scanning/SKILL.md) — pre-public sweep before any tag
- [`dependabot-tuning`](../.claude/skills/dependabot-tuning/SKILL.md) — Phase 3 Task 3.2
- [`changelog-generator`](../.claude/skills/changelog-generator/SKILL.md) — Phase 2 Task 2.2
- [`logging-best-practices`](../.claude/skills/logging-best-practices/SKILL.md) — Phase 5
- [`gh-cli`](../.claude/skills/gh-cli/SKILL.md) — Phase 2 Task 2.6 + every release
- [`goreleaser`](../.claude/skills/goreleaser/SKILL.md) — Phase 2 Task 2.1, Phase 8
- [`charmbracelet`](../.claude/skills/charmbracelet/SKILL.md) — any TUI work, Phase 5 Task 5.4
- [`artifact-sbom-publisher`](../.claude/skills/artifact-sbom-publisher/SKILL.md) — Phase 4 Tasks 4.2 / 4.3
- [`clyde-release-ritual`](../.claude/skills/clyde-release-ritual/SKILL.md) — terminates this plan

## Conventions

- **Conventional Commits**, no AI co-author trailer.
- **Branch naming:** `fix/<short>` for bug fixes, `chore/<short>` for tooling, `test/<short>` for test additions, `feat/<short>` for new behavior, `docs/<short>` for docs-only.
- **One PR per task.** Phase 0 Tasks 5–8, Phase 1 Tasks 1.1–1.4, Phase 3 all tasks, and Phase 8 Tasks 8.1/8.2 can be opened in parallel where dependencies allow.
- **No `--no-verify`, no `--no-gpg-sign`** — fix hooks instead of bypassing.

---

## Supersedes

This plan replaces and consolidates:

- ~~`plans/2026-05-05-master-analysis.md`~~ → Phase 0 here
- ~~`plans/2026-05-05-production-readiness.md`~~ → Phases 1–7 here

The originals have been removed. Reach for this single document.
