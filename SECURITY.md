# Security policy

## Reporting a vulnerability

If you've found a security issue in Clyde, please **do not** open a public GitHub issue. Email the maintainers instead:

- **Contact:** contact@systemartis.com
- Use the subject line: `[clyde-security] short summary`
- Include: affected version, reproduction steps, expected vs. actual behavior, and your assessment of impact.

You will receive an acknowledgment **as soon as possible**. We aim to provide a fix or mitigation plan within **14 days** for high/critical issues, longer for lower-severity items. Once a patch is shipped we'll credit you in the release notes (unless you'd rather stay anonymous).

## Trust model

Clyde runs as your unprivileged user account on a single machine. The trust boundary is your local UNIX session:

- **In scope:** credential leaks via logs/errors, command injection into shellouts (`git`, `ps`, `security`), TLS misconfiguration, path traversal, OOM via malformed inputs, the localhost hook server's request handling.
- **Out of scope:** attacks from a process already running as your user (you've already lost), attackers with root, attackers with arbitrary disk access, vulnerabilities in transitive dependencies that have no upstream advisory.

## Localhost hook server

Clyde binds `127.0.0.1:<random-port>` to receive hook callbacks from `claude`. The listener is loopback-only AND requires a per-process random auth token printed to stderr at startup.

When the TUI cannot drain hook events fast enough to keep the internal buffer below capacity, additional hook calls are **denied** rather than auto-approved. Users see a re-prompt in `claude` rather than a silent approval — this prevents a hung TUI or another local process from harvesting approvals the user never granted.

## Dependencies

We run `govulncheck ./...` in CI on every PR and rely on a minimal set of well-maintained Go dependencies (`charm.land/*`, `BurntSushi/toml`, `alecthomas/chroma`, `aymanbagabas/go-udiff`). Direct deps are pinned in `go.mod`.
