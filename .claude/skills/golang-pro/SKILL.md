---
name: golang-pro
description: Use when reviewing, writing, or refactoring Go code in this repo. Enforces gofmt + golangci-lint, race-tested table-driven tests, fuzzing on parsers/decoders, explicit error handling, context propagation, and small/composable interfaces. Adapted from skills.sh/jeffallan/claude-skills/golang-pro to clyde's hexagonal layout.
---

# golang-pro (clyde-tuned)

Use this skill any time you touch a `.go` file in this repo. The base skill is upstream — these tunings reflect what *actually* matters in clyde given its hexagonal layout, TUI boundaries, and the failure modes already observed.

## Core workflow

1. **Architecture first.** Identify which layer your change belongs to before writing code (`internal/domain` → `internal/application` → `internal/ports` → `internal/adapters/*` → `cmd/clyde`). Domain stays stdlib-only. Application depends on ports, not adapters. Adapters do I/O.
2. **Design the interface.** Ports are small and behavior-named, not data-named (`SessionSource`, `Clock`, not `JSONLReader`). One adapter is a hypothetical seam; two adapters is a real one — keep ports minimal until two adapters exist.
3. **Implement idiomatically.** Wrap errors with `fmt.Errorf("%w", err)`. Pass `context.Context` through every blocking call (HTTP, exec, file scan over a hot loop). Avoid `panic` outside of `domain` invariant guards.
4. **Lint & validate.**
   - `gofmt -l .` (must produce empty output)
   - `go vet ./...`
   - `golangci-lint run ./...` (depguard MUST pass — see "Layering" below)
   - `go test -race ./...`
5. **Optimize last.** Add a benchmark before any micro-optimization. Profile with `pprof` only after a concrete latency complaint.

## Mandatory in this repo

- **Race detector on every test run.** CI runs `go test -race -cover ./...` (`.github/workflows/ci.yml`). Local runs without `-race` do not satisfy "tests pass."
- **Table-driven tests with subtests.** `t.Run(tc.name, ...)`, `t.Parallel()` where state is per-test. Already the norm — match it.
- **`govulncheck ./...` clean** for any new dependency. CI fails on vulns.
- **Explicit `_ = err` is a code smell.** Allowed only with a `//nolint:errcheck // <reason>` comment naming a real reason (`best-effort` is the bar to clear, see `jsonl/jsonl.go:526`).
- **Constant-time comparison** for any token / secret check (`crypto/subtle.ConstantTimeCompare`). Already used by `hookserver.authorized`; do not regress.
- **Bounded reads on external input.** Every `io.Reader` exposed to JSONL, HTTP, or `exec` output MUST be wrapped in `io.LimitReader` or `http.MaxBytesReader`. The hookserver caps at 64KB, ps output at 8MB, JSONL scanner at 4MB — match the pattern.

## Prohibited

- `panic` in `internal/application` or `internal/adapters`. The single allowed `panic` lives in `domain/project/project.go` as an invariant guard.
- Goroutines without lifecycle management. `Server.Start` already does the right thing — copy that shape (ctx-rooted goroutine, `sync.Once` for cleanup).
- New direct dependencies. `SECURITY.md` enumerates the small set we trust. Adding a dep needs a comment in the PR explaining why a stdlib path was rejected.
- Wide interfaces. If a port grows past ~5 methods, split it.

## Layering — the depguard contract

`.golangci.yml` blocks (or *should* block — see analysis report) these import edges:

| From | Cannot import |
|------|--------------|
| `internal/domain/**` | UI libs, `net/http`, `os`, fsnotify, any `internal/adapters/**` or `internal/application/**` |
| `internal/application/**` | `internal/adapters/**`, UI libs |
| `internal/ports/**` | `internal/adapters/**` |

**Known issue (2026-05-05):** the `pkg:` patterns in `.golangci.yml` use the obsolete module path `github.com/vladpb/clyde`. Module is now `github.com/Systemartis/clyde`, so cross-package depguard rules currently no-op. Fix the module path in `.golangci.yml` before relying on lint to catch layer violations.

## Fuzzing

Any code that decodes external bytes (JSONL parser, hook request body, ps output, git output, OAuth response bodies) is a fuzz candidate. None exist today. When you write or touch one of:

- `internal/adapters/jsonl/decodeLineWithMsgID`
- `internal/adapters/jsonl/parseStatus`-style line parsers
- `internal/adapters/processscan/parseClaudeSessionIDs`
- `internal/adapters/git/parseDiff` and `parseStatus`
- `internal/adapters/anthropicapi/decodeEnvelope`

…add a `func FuzzX(f *testing.F)` next to it. Seed from existing testdata.

## Pre-merge checklist

- [ ] `gofmt -l .` produces no output
- [ ] `go vet ./...` clean
- [ ] `golangci-lint run ./...` clean
- [ ] `go test -race -cover ./...` clean
- [ ] No new direct dep without justification
- [ ] Layering preserved (domain pure, app via ports, ports interface-only)
- [ ] Bounded reads on any new external-input path
- [ ] Goroutines tied to a context with deterministic shutdown
