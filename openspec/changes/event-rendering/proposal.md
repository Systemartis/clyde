# Proposal: event-rendering

## Intent

Focused-pane today shows raw kinds (`user`/`assistant`/`(opaque)`) with no content. Most rows are meta types, skill-prompt injections, or tool-result returns — not conversation. The user can't tell what was asked, answered, or which tool ran. V1.5 filters noise at the use case and gives every visible event a one-line summary derived in the domain.

## Scope

### In Scope
- Filter noise at the `watchsession` use case (filtered events never reach the TUI).
- Add `Summary string` to `UserPayload` and `AssistantPayload`.
- Derive `Summary` during JSONL decode.
- Render `<time>  <role>  <Summary>`; remove the `(opaque)` branch.

### Out of Scope
- Decoding meta types; "show tool results" toggle (deferred); configurable length; dynamic terminal-width truncation; ANSI/color; pagination; multi-pane; session switching; cost/token UI.

## Capabilities

### New Capabilities
- None.

### Modified Capabilities
- `domain/event`: `UserPayload` and `AssistantPayload` gain `Summary string`.
- `application/watch-session`: filter rules drop opaques, `isMeta:true` users, and tool-result-only users.
- `tui/behavior`: row format becomes `<time>  <role>  <Summary>`; opaque branch removed.

## Approach

Filter at the use case so the adapter receives a clean stream and stays presentational. Make `Summary` a domain field so the rule lives with the data — future consumers (logs, web UI) get it free. JSONL adapter parses raw bytes during decode and populates the field. TUI prints what it receives.

## Filter rules (locked)

Hidden:
1. All `KindOpaque` (attachment, file-history-snapshot, last-prompt, ai-title, permission-mode, system, progress, queue-operation).
2. User events with `isMeta: true` (skill prompts).
3. User events whose only content is `tool_result` block(s). **HIDE for V1.5.** Future toggle deferred.

Shown: typed user prompts (string OR text-block-no-tool_result) and all assistant events.

## Summary derivation rules (locked)

**User**: first ~80 runes of typed text, single-line (newlines/tabs → space, trimmed).

**Assistant** (first match wins):
1. `tool_use` block → `Tool: <name> <key-arg>` (table below; first tool_use if multiple).
2. `text` block → first ~80 runes, single-line.
3. `thinking`-only → literal `(thinking)` (thinking text is encrypted in JSONL).
4. Otherwise → empty string.

## Per-tool key-arg map (locked)

| Tool | Format |
|------|--------|
| `Read` / `Write` / `Edit` / `MultiEdit` | `Tool: <Name> <file_path>` |
| `Bash` | `Tool: Bash <description>` (fallback: first 60 chars of `command`) |
| `Grep` / `rg` | `Tool: Grep <pattern>` |
| `Glob` | `Tool: Glob <pattern>` |
| `Task` / `Agent` | `Tool: Task <description>` |
| `TodoWrite` | `Tool: TodoWrite (<n> items)` |
| Default | `Tool: <Name>` |

Truncation target ~80 runes. Domain `Summary` is the source of truth; TUI may visually trim further.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/domain/event/event.go` | Modified | Add `Summary string` to `UserPayload`, `AssistantPayload`. |
| `internal/application/watchsession` | Modified | Apply filter rules before emission. |
| `internal/adapters/jsonl` | Modified | Extract `Summary` during decode. |
| `internal/adapters/tui/model.go` | Modified | Render `<time>  <role>  <Summary>`; drop opaque branch. |

## Domain concepts touched

`Summary` is a string value-object on existing payloads. No new aggregate, no new ubiquitous-language term.

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Filter hides wanted events | Med | Raw events stay on disk; future toggle re-exposes without backfill. |
| Unknown tool shape misses summary | Med | Default branch returns `Tool: <Name>` — always informative. |
| Truncation cuts mid-rune | Low | Rune-count truncation; append `…` on overflow. |
| Test fixtures break (additive field) | Med | Zero value is valid; update fixtures during apply. |

## Rollback Plan

Trivial. Delete the change folder, restore prior `tui/model.go` rendering, drop the filter call, delete the `Summary` field. No data migration, no flag, no client coordination.

## Dependencies

None external. Internal: existing `event.Kind`, `KindOpaque`, and JSONL raw-byte access.

## Success Criteria

- [ ] Zero `(opaque)` rows in any sampled session.
- [ ] Every visible row has a non-empty `Summary` (or `(thinking)`).
- [ ] Typed user prompts render; tool-results and skill-injections are absent.
- [ ] Assistant tool calls render `Tool: <Name> <key-arg>` per the locked map.
- [ ] Strict TDD: new tests cover each filter rule and each summary branch.

## Open questions for /sdd-spec and /sdd-design

- Row layout for assistant token count: second row, or appended? **Design decides.**
- `Summary` placement: directly on each payload, or on a shared embedded struct? **Design decides.** Spec only locks: both payload types expose `Summary string`.
