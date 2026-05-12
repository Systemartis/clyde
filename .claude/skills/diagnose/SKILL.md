---
name: diagnose
description: Use when investigating any bug, flaky test, or unexpected runtime behavior, before proposing a fix. Builds a deterministic feedback loop first, then iterates hypotheses against it. Adapted from skills.sh/mattpocock/skills/diagnose for clyde's TUI + adapter layout.
---

# diagnose (clyde-tuned)

Rule: build a fast, deterministic, agent-runnable pass/fail signal **before** hypothesizing. The loop is the product. Optimize it for speed, signal clarity, and determinism.

## Phase 1 — Build the feedback loop

Pick the cheapest loop that captures the failure:

1. **Failing unit test** — preferred when the bug lives in `domain` or `application`. Table-driven, name the case, run with `go test -race -run <name> ./<pkg>`.
2. **Failing teatest snapshot** — preferred for any TUI behavior. `internal/adapters/tui/*_test.go` has many examples; copy the shape.
3. **Adapter integration test** — for `jsonl`, `git`, `processscan`, `hookserver`. Use `t.TempDir()` with fixture files (already the pattern).
4. **`teatest/v2` golden** — when the bug is "the screen looks wrong." Re-record only after fixing.
5. **httptest.Server** — for `anthropicapi` flows; mock the OAuth and usage endpoints separately.
6. **A canned `ps` output fixture** — for `processscan`; never call real `ps` from a test.
7. **A captured JSONL line in `testdata/`** — for `jsonl` decoder bugs. Anonymize before committing.
8. **Diff harness** — when the bug is "old behavior was different," capture both behaviors as fixtures and assert the delta.
9. **Property/fuzz test** — for parsers (JSONL, ps, git diff, OAuth response). Seed from existing testdata.

A loop that takes >5 seconds isn't a loop. Cut scope until it's <2s.

## Phase 2 — Reproduce

The loop must reproduce the *exact* failure the user reported. Not a similar one. If you can't reproduce, don't hypothesize — keep narrowing the loop. Common narrowing: shrink input, pin time (clyde adapters expose `now func() time.Time` test seams — use them), pin randomness, pin process state.

## Phase 3 — Hypothesize

Write 3–5 falsifiable hypotheses, ranked by likelihood × ease-of-test. Each hypothesis MUST predict a specific outcome if true. Examples for clyde:

- "The dedup map is keyed by message.id but the JSONL has empty IDs on this branch — the test assertion will see N+1 events instead of N."
- "The hookserver auto-allows when the channel is full — running 10 concurrent hooks against a hung TUI will yield 8 approvals."
- "The git diff cache returns stale results across cwd switches because the key is `(cwd, file)` and we never invalidate."

Don't add probes for the wrong hypothesis. Pick one, test, falsify, move on.

## Phase 4 — Instrument

- `t.Log` with structured key=value spans so a failed test message is parseable.
- For race-y bugs, run `go test -race -count=100 -run <name>`. If it fails 1/100, you have a real race; if it fails 100/100, you have a logic bug.
- For TUI flake, capture the full state trail with `tea.NewModel(model).Run()` instrumentation rather than guessing from a single snapshot.
- Don't sprinkle `fmt.Println` in production code — adapters print to stderr and corrupt the bubbletea-rendered terminal. If you need to instrument, use `t.Log` from the test or a dedicated `os.Getenv("CLYDE_TRACE")` gate.

## Phase 5 — Fix + regression test

- Write the regression test at the architectural seam closest to the bug. Domain bug → domain test. Adapter bug → adapter test. End-to-end bug → teatest.
- The regression test MUST fail before the fix. If it passes pre-fix, the test isn't actually exercising the bug.
- Apply the minimal fix. Resist the urge to refactor adjacent code in the same commit.

## Phase 6 — Cleanup + post-mortem

- Remove debug prints, env-var gates, and any `t.Log` that was scaffolding.
- Commit message: explain the *why* (root cause), not the *what* (the diff already shows that).
- If the bug exposed a class of issues (e.g. all caches with non-cwd-aware keys), capture the class in `analysis/ARCHITECTURE-FINDINGS.md` for the next iteration.

## clyde-specific seams to instrument

| Layer | Seam | How to inject |
|-------|------|---------------|
| `git.Source` | `now`, `runner` | construct with non-zero `now` and a stub `runner` — see `git_test.go` |
| `processscan.Source` | `now`, `runPS` | inject canned `ps` output |
| `anthropicapi.Client` | `httpClient`, `now` | `NewClientWithDeps` with `httptest.Server` |
| `claudesettings.Reader` | `Path` | point at a temp file |
| `jsonl.Source` | `baseDir` | point at `t.TempDir()` |
| TUI | demo mode | `tui.NewModelWithConfig(cfg, mode)` returns deterministic mock data |

## Don't

- Don't propose a fix until the loop reproduces.
- Don't add logging to "see what's happening" instead of forming hypotheses.
- Don't fix the bug in the test (e.g. by relaxing an assertion). Fix the code.
- Don't ship a fix without a regression test, even if the bug is "obviously" one-off.
