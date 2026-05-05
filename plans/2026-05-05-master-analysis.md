# Master Analysis Remediation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the nine findings recorded in `analysis/MASTER-ANALYSIS.md`. Each task is a separately-mergeable PR with its own tests.

**Architecture:** Hexagonal — `domain` (pure stdlib) / `application` (use cases via ports) / `ports` (interfaces) / `adapters` (I/O — `jsonl`, `git`, `hookserver`, `anthropicapi`, etc.) / `cmd/clyde` (composition root). Layering is enforced by `golangci-lint`'s `depguard` rules in `.golangci.yml`. **Step 1 of this plan restores that enforcement** — run it first; everything else assumes the lint guard is functional.

**Tech Stack:** Go 1.26 · `charm.land/bubbletea/v2` · `golangci-lint v2.11.4` (depguard) · `govulncheck` · standard `testing` + `teatest/v2` · CI on GitHub Actions (`.github/workflows/ci.yml`).

---

## Reading order before starting

1. `analysis/MASTER-ANALYSIS.md` — the findings (F-01..F-09) and their evidence.
2. `CLAUDE.md` — the architectural orientation, including the known-issue note about depguard.
3. `SECURITY.md` — the trust boundary (Step 3 + Step 4 sit on this).
4. `CONTRIBUTING.md` — TDD discipline + commit message conventions (Conventional Commits, no AI co-author trailer).
5. `.claude/skills/golang-pro/SKILL.md` — the pre-merge checklist for every task below.

If you change behavior in `internal/adapters/hookserver`, also re-read its package doc — the design decisions (loopback bind, token, body cap, channel buffer) are documented in the source.

---

## Tasks at a glance

| # | Title | Severity | Effort | Depends on |
|---|-------|----------|--------|-----------|
| 1 | Restore depguard enforcement (F-02) | High | XS | — |
| 2 | Fix `serveErr` race (F-01) | Medium | S | 1 |
| 3 | Hookserver fail-closed on full channel (F-03) | Medium | S | 1, 2 |
| 4 | Add fuzz harnesses for 5 parsers (F-04) | Low | M | 1 |
| 5 | CI smoke job for `--demo` (F-07) | Low | XS | 1 |
| 6 | Capture Keychain stderr (F-05) | Low | XS | 1 |
| 7 | Shape-miss counter on `jsonl.Source` (F-06) | Info | XS | 1 |
| 8 | Narrow TUI gocyclo/gocognit exclusion (F-08) | Info | S | 1 |
| 9 | (deferred) Deepen TUI shallow modules (F-09) | Info | L | separate plan |

Tasks 5–8 can be parallelized after 1 lands. Tasks 2 and 3 are sequential because they edit the same file.

---

## Task 1: Restore depguard enforcement (F-02)

**Why first:** every later task should be merged through a working layering check.

**Files:**
- Modify: `.golangci.yml` (lines 56-83)
- Modify: `internal/ports/sessionsource.go:40` (doc comment)
- Modify: `internal/adapters/jsonl/encode.go:15-22` (doc comment)
- Modify: `internal/adapters/jsonl/jsonl.go:7` (doc comment)
- Modify: `CLAUDE.md` (remove "Known issue" paragraph after fix lands)
- Test: hand-rolled "tripwire" branch (not committed) to verify lint reject

- [ ] **Step 1.1: Inventory the stale references**

Run: `grep -rn "vladpb" .golangci.yml internal/`
Expected: hits in `.golangci.yml` (4×), `internal/ports/sessionsource.go` (1× comment), `internal/adapters/jsonl/encode.go` (≥4× comment), `internal/adapters/jsonl/jsonl.go` (1× comment). Test fixtures (`*_test.go`) keep `vladpb` strings as data — do not change them in this task.

- [ ] **Step 1.2: Edit `.golangci.yml`**

Replace every `pkg: "github.com/vladpb/clyde/...` with `pkg: "github.com/Systemartis/clyde/...` (4 occurrences) and update the `local-prefixes:` value at the bottom from `github.com/vladpb/clyde` to `github.com/Systemartis/clyde`.

Also update lines 5-6's "Module path used below" comment to read `github.com/Systemartis/clyde`.

- [ ] **Step 1.3: Update doc comments in source**

Same `vladpb` → `Systemartis` swap in:
- `internal/ports/sessionsource.go:40`
- `internal/adapters/jsonl/encode.go:15-22`
- `internal/adapters/jsonl/jsonl.go:7`

Leave `*_test.go` fixture data untouched — those simulate user paths and the encoded form is real.

- [ ] **Step 1.4: Verify lint still passes on a clean tree**

Run: `golangci-lint run ./...`
Expected: zero issues.

- [ ] **Step 1.5: Tripwire — verify the layering rule actually fires now**

In a throwaway commit (DO NOT push):
1. Add `import "github.com/Systemartis/clyde/internal/adapters/tui"` to `internal/domain/session/session.go` and reference one of the imported package's exports.
2. Run: `golangci-lint run ./internal/domain/...`
3. Expected: depguard error with description `"domain must not depend on adapters"`.
4. `git checkout -- internal/domain/session/session.go` to discard the tripwire.

If lint did NOT reject, the path patterns are still wrong — debug before merging.

- [ ] **Step 1.6: Update the known-issue note in `CLAUDE.md`**

Remove the paragraph titled "**Known issue:**" under "Layering enforcement (and a known gotcha)" and the surrounding sentence that currently reads "The hexagonal contract is still real and CONTRIBUTING.md treats it as enforced — write code as if depguard were catching violations". Replace with a single sentence: "depguard rules enforce the hexagonal contract — see `.golangci.yml` for the deny list."

- [ ] **Step 1.7: Commit**

```bash
git add .golangci.yml internal/ports/sessionsource.go internal/adapters/jsonl/encode.go internal/adapters/jsonl/jsonl.go CLAUDE.md
git commit -m "$(cat <<'EOF'
chore(lint): restore depguard enforcement after module rename

The .golangci.yml depguard rules referenced github.com/vladpb/clyde
since the migration to github.com/Systemartis/clyde, so the
domain→adapters / application→adapters / ports→adapters denials
silently no-opped. Fixes F-02 from analysis/MASTER-ANALYSIS.md.

Verified by tripwire: introducing a domain→adapters import now
fails lint with the expected depguard message.
EOF
)"
```

---

## Task 2: Fix `serveErr` race in hookserver Start (F-01)

**Files:**
- Modify: `internal/adapters/hookserver/server.go` (the `Start` method, ~lines 161-181)
- Test: `internal/adapters/hookserver/server_test.go` (add a race-detector test)

- [ ] **Step 2.1: Write a failing test that exposes the race**

Add to `internal/adapters/hookserver/server_test.go`:

```go
func TestServer_StartReturnsServeError(t *testing.T) {
    t.Parallel()
    s, err := New()
    if err != nil { t.Fatal(err) }

    // Close the underlying listener BEFORE Start runs. Serve will
    // immediately return a non-nil error other than ErrServerClosed.
    if err := s.listener.Close(); err != nil { t.Fatal(err) }

    ctx, cancel := context.WithCancel(context.Background())
    errCh := make(chan error, 1)
    go func() { errCh <- s.Start(ctx) }()

    // Give Serve a tick to fail.
    time.Sleep(50 * time.Millisecond)
    cancel()

    select {
    case got := <-errCh:
        if got == nil {
            t.Fatal("expected serve error, got nil")
        }
    case <-time.After(2 * time.Second):
        t.Fatal("Start did not return")
    }
}
```

- [ ] **Step 2.2: Run with race detector to verify it fails**

Run: `go test -race -run TestServer_StartReturnsServeError ./internal/adapters/hookserver/`
Expected (current code): either DATA RACE warning OR test fails because `serveErr` is observed as nil despite the close.

- [ ] **Step 2.3: Refactor `Start` to use a buffered error channel**

Replace the `var serveErr error` + goroutine pattern with:

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

The send into the buffered channel happens-before the receive in the spawning goroutine — that's the happens-before edge the previous code lacked.

- [ ] **Step 2.4: Verify the test now passes under race detector**

Run: `go test -race -run TestServer_StartReturnsServeError ./internal/adapters/hookserver/ -count=20`
Expected: PASS, no race warnings.

- [ ] **Step 2.5: Run the full hookserver suite**

Run: `go test -race ./internal/adapters/hookserver/...`
Expected: all PASS.

- [ ] **Step 2.6: Commit**

```bash
git add internal/adapters/hookserver/server.go internal/adapters/hookserver/server_test.go
git commit -m "fix(hookserver): close serveErr race on Start

The previous code had a goroutine writing to a stack-allocated error
variable that the spawning goroutine read after ctx.Done() returned.
The happens-before edge wasn't established by Shutdown returning, so
go test -race could (and did, intermittently) report a write/read
race. Lift the value into a buffered channel — the channel send
happens-before the receive.

Fixes F-01 from analysis/MASTER-ANALYSIS.md."
```

---

## Task 3: Hookserver fail-closed on full channel (F-03)

**Files:**
- Modify: `internal/adapters/hookserver/server.go` (the `handleHook` method, ~lines 235-241)
- Test: `internal/adapters/hookserver/server_test.go`
- Optional: `SECURITY.md` (clarify that channel-full is now deny-on-full)

- [ ] **Step 3.1: Write a failing test for deny-on-full behavior**

Add to `internal/adapters/hookserver/server_test.go`:

```go
func TestServer_DeniesWhenChannelFull(t *testing.T) {
    t.Parallel()
    s, err := New()
    if err != nil { t.Fatal(err) }
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    go func() { _ = s.Start(ctx) }()

    // Fill the events buffer (cap 8) — never drain.
    body := []byte(`{"hook_type":"PreToolUse","tool_name":"Bash","tool_input":{},"cwd":"/"}`)
    for i := 0; i < 8; i++ {
        // first 8 will enqueue and block on ResponseCh; we never drain.
        go func() {
            req, _ := http.NewRequest(http.MethodPost, s.URL(), bytes.NewReader(body))
            _, _ = http.DefaultClient.Do(req)
        }()
    }

    // 9th request — channel full.
    time.Sleep(100 * time.Millisecond)
    req, _ := http.NewRequest(http.MethodPost, s.URL(), bytes.NewReader(body))
    resp, err := http.DefaultClient.Do(req)
    if err != nil { t.Fatal(err) }
    defer resp.Body.Close()

    var decoded struct{ Decision, Reason string }
    if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
        t.Fatal(err)
    }
    if decoded.Decision != "block" {
        t.Fatalf("expected block decision when channel full, got %q", decoded.Decision)
    }
}
```

- [ ] **Step 3.2: Run to confirm it fails**

Run: `go test -race -run TestServer_DeniesWhenChannelFull ./internal/adapters/hookserver/`
Expected: FAIL with `expected block decision when channel full, got "approve"`.

- [ ] **Step 3.3: Flip the fail-open to fail-closed**

In `handleHook`, change the `select` block from:

```go
select {
case s.events <- evt:
default:
    writeAllow(w)  // ← fail-open
    return
}
```

to:

```go
select {
case s.events <- evt:
default:
    writeDeny(w, "clyde busy — re-run the tool to retry")
    return
}
```

Update the package-level comment that refers to "auto-allow" to describe the new behavior.

- [ ] **Step 3.4: Verify the test passes**

Run: `go test -race -run TestServer_DeniesWhenChannelFull ./internal/adapters/hookserver/`
Expected: PASS.

- [ ] **Step 3.5: Run the full suite**

Run: `go test -race ./internal/adapters/hookserver/...`
Expected: all PASS.

- [ ] **Step 3.6: Update SECURITY.md**

In the "Localhost hook server" section (line 22), append: "When the TUI cannot drain hook events fast enough (8-deep buffer), additional hook calls are denied rather than auto-approved. Users see a re-prompt rather than silent approval."

- [ ] **Step 3.7: Commit**

```bash
git add internal/adapters/hookserver/server.go internal/adapters/hookserver/server_test.go SECURITY.md
git commit -m "fix(hookserver): deny instead of auto-allow on full event channel

A full events channel (8-deep buffer) used to flip the server into
auto-approve mode for every subsequent hook. That gives a non-clyde
process — or a hung TUI — silent approval power. Switch to deny: the
user sees a re-prompt in claude rather than an unattended approval.

Fixes F-03 from analysis/MASTER-ANALYSIS.md."
```

---

## Task 4: Add fuzz harnesses for 5 parsers (F-04)

**Files (one per sub-task):**
- Test: `internal/adapters/jsonl/fuzz_test.go`
- Test: `internal/adapters/processscan/fuzz_test.go`
- Test: `internal/adapters/git/fuzz_test.go`
- Test: `internal/adapters/anthropicapi/fuzz_test.go`
- Test: `internal/adapters/claudesettings/fuzz_test.go`
- Modify: `.github/workflows/ci.yml` (optional 1-min fuzz job)

Each fuzz test follows the same shape — write one, copy the structure for the others.

- [ ] **Step 4.1: Write `FuzzDecodeLineWithMsgID` for JSONL**

In `internal/adapters/jsonl/fuzz_test.go`:

```go
package jsonl

import (
    "os"
    "path/filepath"
    "testing"
)

func FuzzDecodeLineWithMsgID(f *testing.F) {
    // Seed from any committed JSONL fixture lines.
    seedDir := filepath.Join("testdata")
    entries, _ := os.ReadDir(seedDir)
    for _, e := range entries {
        if filepath.Ext(e.Name()) != ".jsonl" { continue }
        data, _ := os.ReadFile(filepath.Join(seedDir, e.Name()))
        for _, line := range bytes.Split(data, []byte("\n")) {
            if len(line) > 0 { f.Add(line) }
        }
    }
    // Empties + minimal envelopes.
    f.Add([]byte(`{"type":"user","uuid":"x"}`))
    f.Add([]byte(`{"type":"assistant","message":{"id":"x"}}`))

    f.Fuzz(func(t *testing.T, raw []byte) {
        // Must not panic on any input.
        _, _, _ = decodeLineWithMsgID(raw)
    })
}
```

- [ ] **Step 4.2: Run the JSONL fuzzer for 30s**

Run: `go test -fuzz=FuzzDecodeLineWithMsgID -fuzztime=30s ./internal/adapters/jsonl/`
Expected: zero crashes. If a crash drops a corpus file in `testdata/fuzz/`, commit it as a regression seed and add a `t.Skipf` only after fixing the underlying decoder bug.

- [ ] **Step 4.3: Repeat the pattern for `processscan/parseClaudeSessionIDs`**

```go
func FuzzParseClaudeSessionIDs(f *testing.F) {
    f.Add([]byte("claude --session-id 12345678-1234-1234-1234-123456789012\n"))
    f.Add([]byte(""))
    f.Add([]byte("noise without session"))
    f.Fuzz(func(t *testing.T, raw []byte) {
        _ = parseClaudeSessionIDs(raw)
    })
}
```

Run: `go test -fuzz=FuzzParseClaudeSessionIDs -fuzztime=30s ./internal/adapters/processscan/`

- [ ] **Step 4.4: Same for `git.parseStatus` and `git.parseDiff`**

```go
func FuzzParseStatus(f *testing.F) {
    f.Add([]byte(" M file.go\n"))
    f.Add([]byte("?? new.go\n"))
    f.Fuzz(func(t *testing.T, raw []byte) { _ = parseStatus(raw) })
}

func FuzzParseDiff(f *testing.F) {
    f.Add([]byte("@@ -1,3 +1,3 @@\n line\n-old\n+new\n"))
    f.Fuzz(func(t *testing.T, raw []byte) { _ = parseDiff(raw) })
}
```

Run: `go test -fuzz=. -fuzztime=30s ./internal/adapters/git/`

- [ ] **Step 4.5: Same for `anthropicapi.decodeEnvelope` and `claudesettings.Read`**

The settings reader needs a TempDir + file write per fuzz iteration; copy the test pattern in `claudesettings_test.go` for the per-iteration setup.

- [ ] **Step 4.6: Final full test run**

Run: `go test -race -cover ./...`
Expected: PASS, no regressions.

- [ ] **Step 4.7: (Optional) Add a 60s fuzz job to CI**

In `.github/workflows/ci.yml`, append:

```yaml
  fuzz:
    name: fuzz
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
      - uses: actions/setup-go@v6
        with:
          go-version: '1.26'
          cache: true
      - name: jsonl fuzz
        run: go test -fuzz=FuzzDecodeLineWithMsgID -fuzztime=30s ./internal/adapters/jsonl/
      - name: processscan fuzz
        run: go test -fuzz=FuzzParseClaudeSessionIDs -fuzztime=30s ./internal/adapters/processscan/
      - name: git fuzz
        run: go test -fuzz=FuzzParseStatus -fuzztime=30s ./internal/adapters/git/
```

CI cost: ~90s extra. Skip this sub-step if CI minutes are precious.

- [ ] **Step 4.8: Commit each fuzz file as a separate small commit**

```bash
git add internal/adapters/jsonl/fuzz_test.go
git commit -m "test(jsonl): add fuzz harness for line decoder (F-04)"
# repeat per package
```

Final optional commit for CI:

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add 90s fuzz job covering 3 hot parsers (F-04)"
```

---

## Task 5: CI smoke job for `--demo` (F-07)

**Files:**
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 5.1: Add a smoke job that runs the demo binary briefly**

Append to `.github/workflows/ci.yml`:

```yaml
  smoke:
    name: smoke (--demo)
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
      - uses: actions/setup-go@v6
        with:
          go-version: '1.26'
          cache: true
      - name: build
        run: go build -o /tmp/clyde ./cmd/clyde
      - name: run demo for 3s
        run: |
          # Demo mode does no live reads. We expect the binary to start a
          # bubbletea program; SIGTERM (124) is the success signal here.
          set +e
          timeout --signal=TERM 3 /tmp/clyde --demo --layout=stack < /dev/null
          code=$?
          # 124 = killed by timeout; 0 = clean exit (also fine for unit-shutdown).
          if [ $code -ne 124 ] && [ $code -ne 0 ]; then
            echo "::error::clyde --demo exited with $code"
            exit 1
          fi
```

- [ ] **Step 5.2: Verify in a draft PR**

Push to a feature branch, open the PR, confirm the smoke job goes green.

- [ ] **Step 5.3: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add --demo smoke job to catch wiring regressions (F-07)"
```

---

## Task 6: Capture Keychain stderr (F-05)

**Files:**
- Modify: `internal/adapters/anthropicapi/credentials.go` (the `loadFromKeychain` function)
- Test: `internal/adapters/anthropicapi/credentials_test.go`

- [ ] **Step 6.1: Write a failing test asserting stderr is captured**

Add a test that uses a stub `exec.CommandContext` (via the existing test seam if present, or by invoking a non-existent binary path) and asserts the returned error message includes the captured stderr snippet.

- [ ] **Step 6.2: Replace `cmd.Stderr = io.Discard` with a capped buffer**

```go
var stderr bytes.Buffer
cmd.Stderr = &stderr
out, err := cmd.Output()
if err != nil {
    return Credentials{}, fmt.Errorf("%w: %s", ErrCredentialsNotFound, snippet(stderr.Bytes()))
}
```

`snippet()` already exists in `oauth.go` and redacts long opaque strings — re-use it.

- [ ] **Step 6.3: Run tests**

Run: `go test -race ./internal/adapters/anthropicapi/...`
Expected: PASS.

- [ ] **Step 6.4: Commit**

```bash
git add internal/adapters/anthropicapi/credentials.go internal/adapters/anthropicapi/credentials_test.go
git commit -m "fix(anthropicapi): capture Keychain stderr instead of discarding it (F-05)"
```

---

## Task 7: Shape-miss counter on `jsonl.Source` (F-06)

**Files:**
- Modify: `internal/adapters/jsonl/jsonl.go` (the three `_ = json.Unmarshal` sites)
- Test: `internal/adapters/jsonl/events_test.go`

- [ ] **Step 7.1: Add atomic counter fields to `Source`**

```go
type Source struct {
    // ... existing fields
    shapeMissesUser      atomic.Int64
    shapeMissesAssistant atomic.Int64
    shapeMissesContent   atomic.Int64
}
```

- [ ] **Step 7.2: Increment counters at the three sites**

```go
if err := json.Unmarshal(message, &uMsg); err != nil {
    s.shapeMissesUser.Add(1)
}
```

Replace the `_ = ` form. The `//nolint:errcheck` comments can come off.

- [ ] **Step 7.3: Add a `ShapeMisses()` accessor**

```go
type ShapeMisses struct{ User, Assistant, Content int64 }

func (s *Source) ShapeMisses() ShapeMisses {
    return ShapeMisses{
        User:      s.shapeMissesUser.Load(),
        Assistant: s.shapeMissesAssistant.Load(),
        Content:   s.shapeMissesContent.Load(),
    }
}
```

The TUI doesn't need to surface this immediately — just have the data available for a future diag panel.

- [ ] **Step 7.4: Add a unit test**

Decode a malformed-message JSONL line, assert `ShapeMisses()` reflects the increment.

- [ ] **Step 7.5: Commit**

```bash
git add internal/adapters/jsonl/jsonl.go internal/adapters/jsonl/events_test.go
git commit -m "feat(jsonl): count shape-miss decode failures for observability (F-06)"
```

---

## Task 8: Narrow TUI gocyclo/gocognit exclusion (F-08)

**Files:**
- Modify: `.golangci.yml` (the exclusion block at lines 122-126)

- [ ] **Step 8.1: Inventory which TUI files exceed thresholds today**

Run: `golangci-lint run --no-config --enable-only=gocyclo,gocognit ./internal/adapters/tui/...`

Note the offending files. Likely candidates: `keys.go`, `mouse.go`, `update.go`, `panel_viewer.go`.

- [ ] **Step 8.2: Replace the wholesale path exclusion with explicit file list**

In `.golangci.yml`, change:

```yaml
- path: internal/adapters/tui/
  linters:
    - gocyclo
    - gocognit
```

to:

```yaml
- path: internal/adapters/tui/(keys|mouse|update|panel_viewer)\.go$
  linters:
    - gocyclo
    - gocognit
```

(Adjust the alternation to match the inventory.)

- [ ] **Step 8.3: Run lint to verify other tui files now get checked**

Run: `golangci-lint run ./internal/adapters/tui/...`
Expected: zero issues. If a previously-excluded file now fails, refactor or add it to the alternation, but only after pausing to consider whether the file is shallow per F-09.

- [ ] **Step 8.4: Commit**

```bash
git add .golangci.yml
git commit -m "chore(lint): narrow TUI gocyclo/gocognit exclusion to known-shallow files (F-08)"
```

---

## Task 9 (DEFERRED): Deepen TUI shallow modules (F-09)

**Out of scope for this plan.** The four TUI files >900 LOC (`keys.go`, `mouse.go`, `panel_viewer.go`, `model.go`) need a separate plan because each deepening is multi-day, multi-PR, and changes user-visible behavior risk.

Instead, after Tasks 1-8 land:

- [ ] **Step 9.1: Open a tracking issue** titled `TUI: deepen shallow modules (keymap, viewer)` linking back to `analysis/MASTER-ANALYSIS.md` §3 F-09.
- [ ] **Step 9.2: Write a separate plan** under `plans/YYYY-MM-DD-tui-keymap-extraction.md` that follows the `improve-codebase-architecture/SKILL.md` three-phase process (explore → present candidates → grilling loop).

---

## Final verification

After Tasks 1-8 land on `main`:

- [ ] `go test -race -cover ./...` → all PASS
- [ ] `gofmt -l .` → empty
- [ ] `go vet ./...` → clean
- [ ] `golangci-lint run ./...` → clean
- [ ] `govulncheck ./...` → clean
- [ ] tripwire from Task 1.5 still rejects when reintroduced
- [ ] `analysis/MASTER-ANALYSIS.md` Recommendations table can mark F-01..F-08 as resolved

When all green, archive `analysis/MASTER-ANALYSIS.md` (move to `analysis/archive/2026-05-05-master-analysis.md`) and link to it from `CHANGELOG.md` if one exists.

---

## Skill references

- `.claude/skills/golang-pro/SKILL.md` — pre-merge checklist, layering rules, fuzz expectations
- `.claude/skills/security-review/SKILL.md` — apply for Tasks 2, 3, 6
- `.claude/skills/diagnose/SKILL.md` — apply if any task fails in CI
- `.claude/skills/improve-codebase-architecture/SKILL.md` — Task 9 (deferred)

## Conventions

- **Conventional Commits**, no AI co-author trailer (per global preference recorded in `openspec/config.yaml`).
- **Branch naming:** `fix/<short>` for bug fixes, `chore/<short>` for tooling, `test/<short>` for test additions, `feat/<short>` for new behavior.
- **One PR per task.** Tasks 5, 6, 7, 8 can be opened in parallel after Task 1 merges.
