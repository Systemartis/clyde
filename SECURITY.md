# Security policy

## Reporting a vulnerability

If you've found a security issue in Clyde, please **do not** open a public GitHub issue. Email the maintainer instead:

- **Contact:** vlad.bejat@gmail.com
- Use the subject line: `[clyde-security] short summary`
- Include: affected version, reproduction steps, expected vs. actual behavior, and your assessment of impact.

You will receive an acknowledgment **as soon as possible**. I aim to provide a fix or mitigation plan within **14 days** for high/critical issues, longer for lower-severity items. Once a patch is shipped I'll credit you in the release notes (unless you'd rather stay anonymous).

## Trust model

Clyde runs as your unprivileged user account on a single machine. The trust boundary is your local UNIX session:

- **In scope:** credential leaks via logs/errors, command injection into shellouts (`git`, `ps`, `security`), TLS misconfiguration, path traversal, OOM via malformed inputs, the localhost hook server's request handling.
- **Out of scope:** attacks from a process already running as your user (you've already lost), attackers with root, attackers with arbitrary disk access, vulnerabilities in transitive dependencies that have no upstream advisory.

## Localhost hook server

Clyde binds `127.0.0.1:<random-port>` to receive hook callbacks from `claude`. The listener is loopback-only AND requires a per-process random auth token printed to stderr at startup.

## Dependencies

We run `govulncheck ./...` in CI on every PR and rely on a minimal set of well-maintained Go dependencies (`charm.land/*`, `BurntSushi/toml`, `alecthomas/chroma`, `aymanbagabas/go-udiff`). Direct deps are pinned in `go.mod`.
