# Clyde — Master Analysis

**Date:** 2026-05-05
**Branch:** `chore/master-analysis`
**Method:** apply candidate skills from skills.sh (development / golang / architecture / security filters), rank by what each one surfaced *that the others didn't*, codify the winners as project skills, file findings against the codebase.

---

## TL;DR

- Clyde is in good shape: hexagonal layering is real, tests/lint/govulncheck run on every PR, the hookserver implements the right primitives (loopback bind, 256-bit token, constant-time compare, body cap, timeouts).
- **Two real bugs surfaced:** a data race on `Server.serveErr` (concurrent write in goroutine + read after `<-ctx.Done()`), and `.golangci.yml` depguard rules that silently no-op because their `pkg:` patterns reference the old module path `github.com/vladpb/clyde`. Both are mechanical fixes.
- **One trust-boundary nuance:** the hookserver auto-allows when its event channel is full. A local malicious process can spam hooks while the user is idle to flip the system into auto-approve. Worth a deny-on-full reversal or a metrics counter.
- **Architecture pressure points:** four TUI files >900 LOC (`keys.go` 1328, `mouse.go` 1054, `panel_viewer.go` 1092, `model.go` 906). Lint complexity rules are excluded for the whole `tui` package, masking growth.
- **Test coverage gap:** zero fuzz tests despite five distinct external-input parsers (JSONL, ps argv, git diff, OAuth response, settings.json). Easy wins.
- **Top 4 skills installed** as project skills under `.claude/skills/`. Three skills from skills.sh (web/Python-focused or frontend-focused) didn't survive the ranking.

Full ranking and findings below.

---

## 1. Method

### 1.1 Skill candidate list (from skills.sh)

| Skill | Source | Filter |
|-------|--------|--------|
| golang-pro | jeffallan/claude-skills | golang |
| security-review | zackkorman/skills | security |
| code-review-security | hieutrtr/ai1-skills | security |
| security-review | sickn33/antigravity-awesome-skills | security |
| improve-codebase-architecture | mattpocock/skills | architecture |
| tdd | mattpocock/skills | development |
| diagnose | mattpocock/skills | development |
| zoom-out | mattpocock/skills | development |
| grill-with-docs | mattpocock/skills | development |
| audit | pbakaus/impeccable | development |

### 1.2 Application loop

For each skill: (a) fetch the SKILL.md body from skills.sh, (b) run its checklist against clyde's actual tree, (c) record findings — only those that the skill *uniquely* surfaced (not duplicated by another higher-ranked skill).

### 1.3 Constraint

The Go toolchain isn't installed on the analysis machine, so no `go vet` / `go test -race` / `golangci-lint run` / `govulncheck` results in this report. Findings come from static reading. Anything labeled "verify locally" is a recommendation to run a tool I couldn't.

---

## 2. Skill ranking

| Rank | Skill | Stars | Unique findings on clyde |
|------|-------|-------|--------------------------|
| 1 | **golang-pro** | ★★★★★ | Race on `hookserver.serveErr`; missing fuzz on 5 parsers; `cmd.Stderr = io.Discard` masks Keychain errors; gocyclo/gocognit excluded for whole `tui` package; 3× `_ = json.Unmarshal` in `jsonl/jsonl.go` are best-effort but warrant doc |
| 2 | **security-review (zackkorman)** | ★★★★★ | Hookserver channel-full fail-open behavior; depguard rules silently broken (module-path drift); operational token in stderr at startup is documented but worth restating |
| 3 | **improve-codebase-architecture** | ★★★★ | TUI shallow modules (`keys.go` 1328-LOC switch, edit/view seam in `panel_viewer.go`); `git.Source` + `claudesettings.Reader` cited as positive deep modules to copy |
| 4 | **diagnose** | ★★★ | Codifies methodology already practiced; explicit catalog of test seams (`now`, `runPS`, `httpClient`, `Path`, `baseDir`) saves new contributors a hunt |
| 5 | tdd (mattpocock) | ★★★ | "Vertical slicing" warning is helpful; otherwise overlaps with what `CONTRIBUTING.md` already says |
| 6 | zoom-out | ★★ | Overlaps with `openspec/specs/` and the new `CLAUDE.md`; useful only for first-touch onboarding |
| 7 | grill-with-docs | ★★ | Same — overlap with openspec workflow |
| — | code-review-security (hieutrtr) | ★ | Python/React-specific; eval/pickle/dangerouslySetInnerHTML — none apply |
| — | security-review (sickn33) | ★ | Next.js/Supabase/Solana focus; CSRF/XSS/RLS — none apply to a TUI |
| — | audit (pbakaus/impeccable) | ☆ | Frontend a11y/perf/theming — doesn't apply to a Go terminal app |

### 2.1 What "stars" mean

5★ = found a real defect or codified a real ongoing risk that the existing docs/lint don't catch.
4★ = surfaced concrete refactoring leverage with named files.
3★ = codified a methodology already in use; prevents drift.
2★ = overlap with existing workflow; useful for new contributors only.
1★ = misfit (wrong language/stack).
☆ = inapplicable.

### 2.2 What's installed

The top four (★★★ and above where the skill is differentiated) are installed under `.claude/skills/` as **clyde-tuned** versions — each rewritten to reference real files, real seams, and the real depguard contract in this repo. See `.claude/skills/README.md`.

---

## 3. Findings

Severity scale: **Critical / High / Medium / Low / Info.**
"Trust boundary" refers to the model in `SECURITY.md` §13.

### F-01 [Medium] Data race on `hookserver.Server.serveErr`

**Where:** `internal/adapters/hookserver/server.go:161-181`

```go
func (s *Server) Start(ctx context.Context) error {
    var serveErr error
    go func() {
        if err := s.srv.Serve(s.listener); err != nil && err != http.ErrServerClosed {
            serveErr = err          // (1) write in spawned goroutine
        }
    }()
    <-ctx.Done()
    // ...
    s.once.Do(func() {
        _ = s.srv.Shutdown(shutdownCtx)
        close(s.events)
    })
    return serveErr                  // (2) read in spawning goroutine
}
```

The shutdown path (`s.srv.Shutdown`) waits for `Serve` to return — but `Serve` writes `serveErr` *before* it returns. Reading `serveErr` after `Shutdown` is *probably* safe by accident, but `go test -race` will flag it (the happens-before edge isn't established by `Shutdown` returning; `Serve` could write the error after `Shutdown`'s return path observes the listener as closed). Verify locally.

**Fix:** lift `serveErr` into a `chan error` of size 1, or use `errgroup.Group`. Either way, race-clean.

**Trust impact:** none — this is a correctness bug, not a security one.

---

### F-02 [High] depguard rules silently no-op because of module-path drift

**Where:** `.golangci.yml:42-83`

```yaml
- pkg: "github.com/vladpb/clyde/internal/adapters"   # ← stale
  desc: "domain must not depend on adapters"
- pkg: "github.com/vladpb/clyde/internal/application"
  desc: "domain must not depend on application"
# ... and same for application-via-ports and ports-pure rules
```

Module path is now `github.com/Systemartis/clyde` (`go.mod` line 1; commit `70d5596`). The `pkg:` patterns are literal prefix matches — they don't trigger on `Systemartis` imports, so layering rules for cross-package imports currently no-op. Only the third-party denials (bubbletea, lipgloss, net/http, os, fsnotify) still fire.

A motivated PR could land a `domain → application` import (or worse, `domain → adapters/tui`) and lint would not catch it. The hexagonal contract relies on this lint check; the contract is currently unenforced.

Also affected: stale `vladpb` strings live in `internal/jsonl/encode.go`, `internal/jsonl/jsonl.go` (doc comments), and 5 test fixtures (`-Users-vladpb-...` — purely cosmetic in tests but weakens grep-discoverability of the encoded-cwd format).

**Fix:** replace `vladpb` → `Systemartis` in `.golangci.yml` (4 lines), `internal/ports/sessionsource.go:40` (comment), `internal/adapters/jsonl/encode.go:15-22` (comment), `internal/adapters/jsonl/jsonl.go:7` (comment). Optionally update test fixtures for consistency.

**Verify after fix:** run `golangci-lint run ./...` and intentionally introduce a `import "github.com/Systemartis/clyde/internal/adapters/tui"` from `internal/domain/session/session.go`; lint must reject.

---

### F-03 [Medium] Hook server fail-open when event channel is full

**Where:** `internal/adapters/hookserver/server.go:235-241`

```go
select {
case s.events <- evt:
default:
    // Channel full or no consumer — auto-allow so claude is not blocked.
    writeAllow(w)
    return
}
```

Channel buffer is 8 (`server.go:116`). If the TUI is paused, slow, or wedged, a process that can talk to the loopback listener (i.e. presented the random token) will fill the buffer and from that point every subsequent hook auto-allows. The token is loopback-only and per-process, so the realistic attacker is a non-clyde process running as the same user that obtained the token (the threat model considers this out of scope per `SECURITY.md` §17). However, a benign cause — TUI hung on a long-running render — also flips the system to auto-approve, which is a usability footgun.

**Fix options:**

- **Deny-on-full** (most secure): `writeDeny(w, "clyde busy")`. Costs the user a re-prompt; never auto-approves something they didn't see.
- **Block briefly with timeout**: `select { case s.events <- evt: case <-time.After(2*time.Second): writeDeny(w, "clyde busy") }`. Compromise.
- **Metrics-only**: keep current behavior, but increment a counter the user can see in the settings overlay so they notice.

Recommend **deny-on-full** (option 1).

---

### F-04 [Low] No fuzz coverage on five external-input parsers

**Where:**
- `internal/adapters/jsonl/decodeLineWithMsgID` (line 480)
- `internal/adapters/jsonl/decodeFile` (scanner loop)
- `internal/adapters/processscan/parseClaudeSessionIDs` (line 139)
- `internal/adapters/git/parseStatus` (line 211) and `parseDiff`
- `internal/adapters/anthropicapi/decodeEnvelope` (line 113)

Each accepts bytes from outside the trust boundary and returns domain values. Existing tests are exemplar-driven; no `func FuzzX(f *testing.F)` exists in the tree. `bufio.Scanner` is configured with explicit buffer caps (good), but the field-level decoders haven't been fuzzed.

**Fix:** seed each fuzzer from existing testdata and run `go test -fuzz=. -fuzztime=30s` per parser. Seed corpora live next to the test (`internal/adapters/jsonl/testdata/`).

---

### F-05 [Low] `cmd.Stderr = io.Discard` swallows Keychain prompts

**Where:** `internal/adapters/anthropicapi/credentials.go:117`

```go
cmd.Stderr = io.Discard
```

Justified by the comment ("the parent process's stderr feeds into bubbletea-rendered terminal"), but in practice this also discards "user-not-authenticated" prompts and Keychain access denials. The user sees no usage data and no signal as to why. Today the fall-through to file fallback masks most of these, but a future change that requires write access to Keychain would silently fail.

**Fix:** capture stderr to a `bytes.Buffer`, log a single-line, redacted summary to the same channel as the hook server URL line in `main.go` only when the Keychain path errors *and* the file path also errors. Don't render mid-frame.

---

### F-06 [Info] Three best-effort `json.Unmarshal` ignored errors

**Where:** `internal/adapters/jsonl/jsonl.go:526, 544, 556`

```go
_ = json.Unmarshal(message, &uMsg) //nolint:errcheck // best-effort
```

Acceptable today — old JSONL records lack `message`, decode fails, we return zero-valued domain. But "best-effort" hides shape changes from us. A new Anthropic API field, a malformed line, or a partial write (the upstream Claude Code CLI is append-only but not fsync-on-each-line) would all silently produce a Summary-less event.

**Fix:** keep the `_ =` but increment a typed counter (e.g. `s.shapeMisses` on the `Source`) and surface it in a `--diag` panel. No correctness impact; just an observability win.

---

### F-07 [Low] No CI verification that `--demo` mode renders

**Where:** `.github/workflows/ci.yml`

CI runs `go test -race -cover ./...`. Tests cover the model, but a smoke job that runs `go run ./cmd/clyde --demo` for 2s with `tea.WithoutRenderer` would catch wiring regressions in `cmd/clyde/main.go` that compile-OK but blow up at runtime.

**Fix:** add a `smoke` job that runs `timeout 5 go run ./cmd/clyde --demo --layout=stack < /dev/null || code=$?; [ $code -eq 124 ]` (124 = SIGTERM from timeout, expected because the TUI doesn't exit on its own).

Optional; nice-to-have.

---

### F-08 [Info] TUI complexity lint exclusion is unbounded

**Where:** `.golangci.yml:122-126`

```yaml
- path: internal/adapters/tui/
  linters:
    - gocyclo
    - gocognit
```

Justified historically (event dispatch is irreducibly branchy). But `keys.go` is 1328 LOC of switch cases; `mouse.go` is 1054. The exclusion is wholesale, so growth from 1000 → 1500 LOC in any of these files passes lint silently. The natural next growth is hooks/diff/notifications, which add more `(panel, key)` pairs.

**Fix:** narrow the exclusion to specific files (`keys.go`, `mouse.go`, `update.go`) and let the linter catch new files in the same package that grow shallow. Or implement the deepening from F-09 below and remove the exclusion.

---

### F-09 [Info] TUI shallow-module candidates (architecture)

| File | LOC | Friction |
|------|-----|----------|
| `internal/adapters/tui/keys.go` | 1328 | Single switch over `(focusedPanel, key)`. Adding a binding = bottom-of-file edit. |
| `internal/adapters/tui/panel_viewer.go` | 1092 | View + edit modes share a file; the seam is implicit. |
| `internal/adapters/tui/mouse.go` | 1054 | Mirror of `keys.go` for pointer events. |
| `internal/adapters/tui/model.go` | 906 | Composition root that has accreted state. |

**Sketch (deletion-test passes):**

- Extract a `keymap` package: `(focusedPanel, KeyEvent) → Intent`. `model.Update` dispatches on `Intent`. New keybindings touch `keymap` only. Same shape applies to `mouse.go`.
- Extract `Viewer` interface from `panel_viewer.go` with two adapters (read-only, editable). Edit-mode logic stops leaking into render code.

These are not blocking; flag them in `analysis/ARCHITECTURE-FINDINGS.md` for a future phase.

---

## 4. Positive findings (what to copy)

These are deep modules that already do the right thing. Reference them when designing new adapters.

- **`internal/adapters/git/Source`** — single shared cache across `Status`, `Branch`, `Diff` callers. Test seams (`now`, `ttl`, `runner`) keep the production path lean.
- **`internal/adapters/claudesettings/Reader`** — single `(mtime, size)` cache shared by `mcpconfig` and `lspscan`. Two real consumers = real seam, the "two adapters" rule applies.
- **`internal/adapters/anthropicapi/Client`** — credential cache + 401-refresh-retry path is encapsulated. `snippet()` redacts long opaque strings before they hit error messages — copy this for any new HTTP error path.
- **`internal/adapters/hookserver`** — most security primitives are already correct: loopback bind, 256-bit token, `subtle.ConstantTimeCompare`, `MaxBytesReader(64KB)`, Read/Write/Idle timeouts on `*http.Server`. F-01 and F-03 are deltas; the foundation is solid.

---

## 5. Recommendations summary

| ID | Severity | Effort | Recommendation |
|----|----------|--------|----------------|
| F-02 | High | XS | Replace `vladpb` → `Systemartis` in `.golangci.yml` and 4 doc comments |
| F-01 | Medium | S | Lift `serveErr` into a buffered channel or use `errgroup` |
| F-03 | Medium | S | Switch hookserver fail-open → fail-closed (deny on channel full) |
| F-04 | Low | M | Add `Fuzz*` for JSONL, ps, git, OAuth, settings parsers |
| F-05 | Low | XS | Capture Keychain stderr; log redacted summary on dual-failure path |
| F-06 | Info | XS | Add a typed shape-miss counter to `jsonl.Source` |
| F-07 | Low | XS | Add `--demo` smoke job to CI |
| F-08 | Info | S | Narrow gocyclo/gocognit exclusion to specific files |
| F-09 | Info | L | Deepen TUI shallow modules (`keymap`, `Viewer`) |

The accompanying superpowers plan (`plans/2026-05-05-systemartis-launch.md`) sequences these by dependency.

---

## 6. Skills not installed (and why)

- **`code-review-security` (hieutrtr/ai1-skills)** — language-specific to Python/React. Closest Go-applicable item is "raw SQL" and clyde has no SQL. Skip.
- **`security-review` (sickn33/antigravity-awesome-skills)** — Next.js/Supabase/Solana checklist. Useful for those stacks, irrelevant to a TUI. Skip.
- **`audit` (pbakaus/impeccable)** — frontend a11y/perf/theming/anti-patterns. Doesn't apply to a terminal app. Skip.
- **`tdd` (mattpocock)** — overlaps with `CONTRIBUTING.md`'s strict-TDD section. The "vertical slicing" anti-pattern note is the only differentiated content; lifted into `golang-pro/SKILL.md` instead of a separate file.
- **`zoom-out` (mattpocock)** — overlaps with `openspec/specs/` and the project `CLAUDE.md`. Useful only on first-touch.
- **`grill-with-docs` (mattpocock)** — overlap with the openspec discuss-phase flow.

These are not bad skills. They just don't add information once the top 4 are in play.

---

## 7. References

- `SECURITY.md` — trust boundary
- `CONTRIBUTING.md` — TDD + branch / commit conventions
- `CLAUDE.md` — architectural orientation for agents
- `openspec/specs/` — Given/When/Then specs (gitignored locally; archived contract)
- skills.sh sources cited above
