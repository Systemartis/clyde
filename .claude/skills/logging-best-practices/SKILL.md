---
name: logging-best-practices
description: Use when introducing structured logging, when fixing TUI corruption from stderr writes, or when adding crash-report flow. Wide-events / canonical-log-line model with `log/slog`, JSON-only, two levels (info/error), strict PII redaction. Adapted from skills.sh/boristane/agent-skills/logging-best-practices for clyde's TUI + privacy stance.
---

# logging-best-practices (clyde-tuned)

Today, clyde has zero structured logging. `fmt.Fprintf(os.Stderr, ...)` corrupts the bubbletea-rendered terminal mid-run. The fix is `slog` JSON to a file at `$XDG_STATE_HOME/clyde/clyde.log`, never stderr after `program.Run()`.

## Architecture

```
                        startup phase             run phase
                       (stderr is OK)         (stderr corrupts TUI)
                              │                      │
   fmt.Fprintf(os.Stderr) ────┤                      ├──── log to clyde.log
                              │                      │
   slog JSON file ────────────┴──────────────────────┘
   redactor wrapper           always
   (token, paths, secrets)
```

## Setup

Create `internal/adapters/clydelog/log.go`:

```go
package clydelog

import (
    "log/slog"
    "os"
    "path/filepath"
)

// Open opens the per-user log file at $XDG_STATE_HOME/clyde/clyde.log
// (falls back to ~/.cache/clyde/clyde.log on platforms without XDG).
// The returned *os.File MUST be closed at process shutdown.
func Open() (*os.File, *slog.Logger, error) {
    dir := xdgStateHome()
    if err := os.MkdirAll(dir, 0o700); err != nil {
        return nil, nil, err
    }
    path := filepath.Join(dir, "clyde.log")
    f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
    if err != nil {
        return nil, nil, err
    }
    h := slog.NewJSONHandler(f, &slog.HandlerOptions{
        Level:     slog.LevelInfo,
        AddSource: false,
        ReplaceAttr: redactingReplacer,
    })
    return f, slog.New(h), nil
}
```

## Two levels — info, error

The `boristane` skill argues for two levels. clyde matches:

- `INFO` — wide events: session start, tool call, hook approval, snapshot tick (debug-mode only). One per high-level operation.
- `ERROR` — adapter failure, panic recovery, auth failure.

No `DEBUG` / `WARN` / `TRACE`. If you need verbosity, gate INFO emission with `os.Getenv("CLYDE_DEBUG")`.

## Wide events — one per operation

Each Claude session lifecycle becomes a single rich event when it ends:

```go
log.Info("session_end",
    "session_id", sess.ID,
    "duration_ms", elapsed.Milliseconds(),
    "tool_calls", sess.ToolCallCount,
    "tokens_in", sess.UsageIn,
    "tokens_out", sess.UsageOut,
    "model", sess.Model,
    "version", buildinfo.Version,
    "commit", buildinfo.Commit,
)
```

Hook server events:

```go
log.Info("hook_request",
    "tool", req.Tool,
    "cwd_short", shortCwd(req.Cwd),  // never log full path
    "decision", decision,
    "latency_ms", elapsed.Milliseconds(),
)
```

## Redaction — non-negotiable

clyde reads `~/.claude/.credentials.json` (OAuth tokens). The redactor wrapper MUST strip:

- Anything matching `(?i)token|secret|key|auth|credential|bearer` as a key name
- Anything matching `sk-ant-[A-Za-z0-9_-]{32,}` as a value
- Full home-dir paths replaced with `~` (e.g., `/Users/maxim/foo` → `~/foo`)
- The 64-char hex hookserver token

```go
func redactingReplacer(groups []string, a slog.Attr) slog.Attr {
    if isSensitiveKey(a.Key) {
        return slog.String(a.Key, "<redacted>")
    }
    if s, ok := a.Value.Any().(string); ok {
        return slog.String(a.Key, redactString(s))
    }
    return a
}
```

Test the redactor explicitly. The skills.sh source skill is right that PII discipline must be a tested invariant, not a convention.

## Panic recovery

Wrap every long-lived goroutine (PR-14):

```go
go func() {
    defer func() {
        if r := recover(); r != nil {
            log.Error("panic_recovered",
                "where", "hookserver.Start",
                "panic", fmt.Sprint(r),
                "stack", string(debug.Stack()),
            )
        }
    }()
    if err := hs.Start(hookCtx); err != nil { /* ... */ }
}()
```

The TUI's main goroutine: defer recover at the top of `run()` in `cmd/clyde/main.go`, AFTER `tea.Program.ReleaseTerminal()` is callable.

## crash-report subcommand (PR-26)

The privacy-respecting alternative to telemetry:

```go
// clyde crash-report
// Reads the last N events from $XDG_STATE_HOME/clyde/clyde.log
// (already redacted), prints to stdout. User reviews and pastes into
// a GitHub issue. No network calls. Ever.
```

Document this in README:

> clyde does not collect telemetry. clyde does not phone home. clyde writes one local JSON-lines log at `$XDG_STATE_HOME/clyde/clyde.log` for your own debugging. To report a bug, run `clyde crash-report` and review the output before pasting.

## Sources
- [boristane/agent-skills/logging-best-practices on skills.sh](https://skills.sh/boristane/agent-skills/logging-best-practices)
- [pkg.go.dev/log/slog](https://pkg.go.dev/log/slog)
- Stripe canonical log lines: <https://brandur.org/canonical-log-lines>
- XDG Base Directory Spec: <https://specifications.freedesktop.org/basedir-spec/>
