# Multi-LLM Adapter Plan

## Status: V20 groundwork complete — Gemini/Codex/Kimi adapters are V21+

## Problem

Clyde was originally hardcoded to Claude Code's JSONL session format.
As AI-coding CLIs proliferate (Gemini CLI, Codex CLI, Kimi CLI, etc.),
the architecture needs a clean port-level abstraction so adapters can be
swapped at startup without touching any application or domain code.

## Architecture

Hexagonal: the application layer (`livesession`) depends only on **ports**,
never on concrete adapters. A `--source` flag at the CLI selects the adapter.

```
cmd/clyde/main.go
  --source=claude  →  internal/adapters/jsonl.Source   (implemented)
  --source=gemini  →  internal/adapters/gemini.Source  (V21+)
  --source=codex   →  internal/adapters/codex.Source   (V21+)
  --source=kimi    →  internal/adapters/kimi.Source    (V21+)
```

## Port Definition

`internal/ports/llmsource.go` — `LLMSource` interface:

```go
type LLMSource interface {
    Name() string
    Sessions(ctx context.Context, projectCWD string) ([]session.Summary, error)
    Events(ctx context.Context, id session.ID) ([]event.Event, error)
    AllProjectSessions(ctx context.Context, maxResults int) ([]GlobalSessionRef, error)
    PlanLimits(ctx context.Context) *PlanLimits
}
```

`jsonl.Source` already satisfies `LLMSource` (it satisfies `SessionSource` +
`GlobalSessionSource`; `Name()` and `PlanLimits()` were added in V20).

## Title Bar

When `--source` is not `claude` (or when source name != "claude-code") the title bar
shows the source slug between brand and path:

```
clyde · claude-code · ~/path · model          ...
clyde · gemini-cli  · ~/path · gemini-pro     ...
```

## Adding a New Adapter (V21+ steps)

1. Create `internal/adapters/<name>/source.go`.
2. Implement `ports.LLMSource`:
   - `Name()` returns the slug (e.g. `"gemini-cli"`).
   - `Sessions()` reads the CLI's session index (format is CLI-specific).
   - `Events()` decodes one session's events into `[]event.Event`.
   - `AllProjectSessions()` walks the CLI's project root.
   - `PlanLimits()` returns plan data if the CLI exposes it, else `nil`.
3. Map `session.Summary`, `event.Event` from the CLI's native format to the
   domain types. The domain types are stable — do NOT modify them for adapter
   convenience.
4. Add a case to the `--source` switch in `cmd/clyde/main.go`.
5. Wire into `livesession.LiveSession` the same way as the jsonl adapter.
6. Add adapter tests under `internal/adapters/<name>/`.

## Event Domain Contract

Every adapter must emit these domain event kinds:
- `event.KindUser` — user prompt turns
- `event.KindAssistant` — model response turns (with `usage.Usage` populated)
- `event.KindOpaque` — anything unrecognized (never drop)

Token counting (`usage.Usage`) must match Anthropic semantics:
- `Input` = raw input tokens
- `Output` = output tokens
- `CacheRead` = tokens served from prompt cache (per-turn, NOT cumulative)
- `CacheCreation` = tokens written to prompt cache

For non-Anthropic CLIs that do not have cache fields, set `CacheRead = 0` and
`CacheCreation = 0`. The pricing/compaction logic degrades gracefully.

## Reset Countdown Computation

The 5h and 7d reset times are computed inside `livesession.applyMultiWindow*`
from the **earliest** session timestamp in each window:

```
Reset5hAt   = earliest_session_in_5h_window.LastActivity + 5h
ResetWeekAt = earliest_session_in_7d_window.LastActivity + 7d
```

This matches Anthropic's rolling-window semantics. Future adapters that have
explicit reset timestamps from their API can override this by setting `Reset5hAt`
and `ResetWeekAt` directly.

## Prior Art

- [CodexBar](https://github.com/steipete/codexbar): macOS menu bar app with 5h/weekly meters
  and reset countdowns for 19+ providers. Key insight: two separate meters (top = 5h,
  bottom hairline = weekly) with explicit "resets in Xh Ym" labels.
- [tokscale](https://github.com/junhoyeo/tokscale): multi-CLI tracker (20+ tools) with
  per-day/week/month breakdowns. Key insight: unified "clients" abstraction — our
  `LLMSource` serves the same purpose.
