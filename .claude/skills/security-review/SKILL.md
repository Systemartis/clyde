---
name: security-review
description: Use when adding/changing any code that touches secrets, the localhost hook server, exec/subprocess calls, file paths read from disk, or HTTP request handling. Walks the trust boundary clyde defines in SECURITY.md and produces a structured finding list with remediation. Adapted from skills.sh/zackkorman/skills/security-review and clyde's own threat model.
---

# security-review (clyde-tuned)

The trust boundary, copied from `SECURITY.md`: clyde runs as your unprivileged user account on a single machine. We defend against accidental credential leakage and against a non-clyde process on the same box trying to spoof Claude Code into approving an attacker-controlled tool call. We do **not** defend against root, full disk read, or another process already running as you.

Apply this checklist for any PR that:

- Touches `internal/adapters/hookserver/*`
- Touches `internal/adapters/anthropicapi/*` (OAuth credentials)
- Adds an `exec.Command` or `exec.CommandContext` call
- Reads a file path that's user-controlled (cwd, project name, settings.json)
- Adds a new HTTP client/server, or new outbound URL
- Adds or upgrades a direct dependency

## Phase 1 — Recon

- What new attack surface does this change introduce? (write it down in the PR description)
- Does it cross the trust boundary in `SECURITY.md`?
- Is there an existing pattern in the repo that handles the same risk? (constant-time compare, bounded read, atomic file write — copy them)

## Phase 2 — Dependency audit

- Run `govulncheck ./...` locally. CI runs it on every PR.
- New direct dependency? Justify in the PR (why stdlib was insufficient). Check the dep's last commit, open issue count, and whether it's in `SECURITY.md`'s allowlist.
- `go mod tidy` after the change.

## Phase 3 — Secret hygiene

- Grep your diff for tokens, OAuth client secrets, API keys: `grep -E '(sk_live|sk-ant|AKIA|BEGIN PRIVATE KEY|password|secret)' <files>`.
- OAuth tokens go in Keychain (preferred) or `~/.claude/.credentials.json` with mode `0o600` and atomic-rename. Never log the access token. `snippet()` in `oauth.go` redacts long opaque strings — re-use it for any new HTTP error path.
- Token files: write to `path + ".tmp"` then `os.Rename` (see `SaveCredentials`). Naked `os.WriteFile` on the real path leaves a half-written file on a crash, which breaks both clyde and the upstream Claude Code CLI.
- The hookserver token printed to stderr at startup is intentional — but never log it during runtime.

## Phase 4 — Code analysis

### Shellouts (`exec.CommandContext`)

- Args MUST be passed as separate strings, never as `"git " + cmd` or `sh -c`.
- The first arg is a fixed binary name (`git`, `ps`, `security`); never a user-controlled string.
- User-controlled values only appear after `--` for `git diff <file>` (see `buildDiffArgs`).
- `cmd.Stderr = io.Discard` hides errors from the operator — only acceptable when a non-zero exit is *expected* and benign (Keychain-miss path). Document the reason in a comment.
- Output is read with `io.LimitReader` or a bufio buffer with explicit cap. `processscan` caps `ps` output at 8 MiB; copy that.

### File I/O

- Resolve paths via `filepath.Join(baseDir, filepath.Clean(name))`. Reject names containing `..` or absolute paths if the input is user-supplied.
- The JSONL session ID looks like a UUID — validate it matches that shape before using it as a path component (`processscan/sessionIDPattern` shows the regex).
- Mode `0o600` for credentials, `0o700` for credential dirs.

### HTTP server (hookserver)

- Loopback bind only (`127.0.0.1:0`). Never bind to `0.0.0.0` or a fixed port.
- Bearer token: random ≥256 bits, compared with `subtle.ConstantTimeCompare`.
- Request body: `http.MaxBytesReader(w, r.Body, cap)` before `json.NewDecoder(r.Body).Decode`.
- Read/Write/Idle timeouts set on the `*http.Server` struct.
- HTTP method whitelist (POST only).
- Channel-full behavior: be explicit about whether full-buffer = deny or = auto-allow. Today we auto-allow (`server.go:238-241`); a malicious local process can exploit this by spamming hooks while the user is idle. Worth revisiting.

### HTTP client (outbound)

- `http.Client` MUST have `Timeout` set (`anthropicapi.NewClient` uses 10s).
- Context-rooted requests: `http.NewRequestWithContext`.
- Read response with `io.LimitReader(resp.Body, 1<<20)` (1 MiB).
- Errors that include the response body MUST scrub long opaque strings — copy `snippet()` from `oauth.go`.

### Goroutines

- Every `go func()` MUST be tied to a `context.Context` with a clear cancellation path. The hookserver `Start` is the reference implementation.
- Do NOT capture an error variable from a goroutine and read it from the spawning goroutine without sync — this is a data race even if `go vet -race` happens to miss it. (See report finding #2.)

## Phase 5 — Reporting

If you found something, file it in `analysis/SECURITY-FINDINGS.md` with:

- **Severity** (Critical / High / Medium / Low / Info)
- **Where** (file:line)
- **What** (one sentence)
- **Why it matters** (which trust-boundary clause it touches)
- **Fix** (concrete: add `MaxBytesReader`, lift `serveErr` into a channel, etc.)

If the finding is a real vulnerability and the project is public, follow `SECURITY.md` private-disclosure flow first. Don't open a public PR with an exploit-shaped commit message.

## Quick triage matrix

| Change | Run security-review? |
|--------|----------------------|
| New `exec.Command*` | YES |
| New `http.Server` route or middleware | YES |
| New `http.Client` outbound | YES |
| Reads `~/.claude/.credentials.json` or any token | YES |
| Reads a path built from user input (cwd, project name, env) | YES |
| New direct dependency | YES |
| Pure domain logic | NO |
| TUI rendering only | NO |
