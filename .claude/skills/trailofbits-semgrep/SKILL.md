---
name: trailofbits-semgrep
description: Use when adding/modifying code that touches the hookserver, OAuth credentials, exec.Command shellouts, JSONL parsing, or any new HTTP server/client. Runs Semgrep with curated security rulesets (Trail of Bits + 0xdea + Decurity) over Go, with mandatory `--metrics=off`. Adapted from skills.sh/trailofbits/skills/semgrep for clyde's hexagonal layout and trust model.
---

# trailofbits-semgrep (clyde-tuned)

Semgrep is the single highest-leverage SAST for clyde's surface. Run on every PR that touches `internal/adapters/hookserver/`, `internal/adapters/anthropicapi/`, `internal/adapters/git/`, `internal/adapters/processscan/`, or any new file under `cmd/clyde/`.

## Setup once

```bash
brew install semgrep                 # or: pipx install semgrep
semgrep --version                    # verify ≥1.50
```

## Run pattern

Always pass `--metrics=off` (Trail of Bits convention; prevents telemetry to r2c during audit):

```bash
# OSS rules + Go-specific
semgrep scan \
  --config p/golang \
  --config p/security-audit \
  --config p/owasp-top-ten \
  --metrics=off \
  --sarif --output=semgrep.sarif \
  internal/ cmd/

# Trail of Bits rulesets (require auth-free public repo)
semgrep scan \
  --config "https://semgrep.dev/p/trailofbits" \
  --metrics=off \
  internal/ cmd/
```

If Semgrep Pro is available, add `--pro` for cross-file taint tracking — the Pro engine catches taint that flows from `r.URL.Query().Get("t")` through the hookserver auth path, or from JSONL bytes through the decoder into a domain value.

## clyde-specific patterns to look for

The skills.sh source skill is generic. These are the clyde-specific patterns the rulesets should fire on:

| Pattern | File hot spots |
|---------|----------------|
| `exec.Command` with non-literal first arg | `git/git.go:179`, `processscan/processscan.go:88`, `anthropicapi/credentials.go:110` |
| `http.Server` without `ReadTimeout`/`WriteTimeout` | `hookserver/server.go` |
| `bytes.Buffer` / `io.ReadAll` without `LimitReader` | every adapter that reads external input |
| Token comparison without `subtle.ConstantTimeCompare` | already correct in `hookserver/server.go:277`; regression check |
| Hardcoded URL or token in source | catch any future regression |
| `os.WriteFile` for sensitive data without `0o600` | `anthropicapi/credentials.go:209` is correct; regression check |

## Triage rules

For each Semgrep finding:

1. **Is it in `internal/adapters/`?** Likely real. Treat at finding-severity.
2. **Is it in a `_test.go`?** Likely false positive. Add a comment or `// nosem` only if Trail of Bits would.
3. **Is it in `internal/domain/`?** Should NOT be — domain has no I/O. If a finding fires, the layering rule was broken (see F-02).
4. **Is it in `cmd/clyde/`?** Likely real but small surface; verify against composition-root behavior.

## Output expectation

File findings to `analysis/SEMGREP-FINDINGS.md` (created on first run) with:

```
### <rule_id>
- **Severity:** ERROR / WARNING / INFO
- **File:** path:line
- **What:** one paragraph from the rule message
- **Triage:** real / FP — and why
- **Fix:** code sketch or "no action — see <reason>"
```

Suppress `// nosem: <rule_id> — <reason>` only with a real reason; never blanket-suppress.

## Sources
- [trailofbits/skills/semgrep on skills.sh](https://skills.sh/trailofbits/skills/semgrep)
- Semgrep registry: <https://semgrep.dev/r>
- Trail of Bits rules: <https://github.com/trailofbits/semgrep-rules>
