# Delta Spec: event-rendering

## Modified Capabilities

### domain/event

`UserPayload` gains one new field and `AssistantPayload` gains one new field. Both fields are domain-level value objects: they live with the data so every future consumer (logs, web UI, secondary TUI panels) receives them without re-derivation.

**UserPayload** adds:

- `Summary string` — the first ~80 runes of the typed user prompt text. Newlines and carriage returns MUST be replaced with a single space before truncation. Multiple consecutive whitespace runs SHOULD be collapsed to a single space. MAY be empty when the User Event carries no human-typed text.

**AssistantPayload** adds:

- `Summary string` — derived by the priority chain in §Summary Derivation below. MUST follow the stated priority strictly and MUST NOT be empty for visible Events (the fallback chain guarantees a non-empty value for every renderable assistant turn).

Both payloads remain immutable once the Session is decoded. `Summary` is populated at decode time, not at render time.

### application/watch-session

`WatchSession.Run` gains a filtering step between the existing sort step and the last-N selection step. The filter MUST be applied BEFORE counting toward N — N counts only visible Events.

**Filter rules (all three MUST be applied, in any order):**

1. Events with `Kind = KindOpaque` MUST be excluded.
2. User Events flagged as meta (`isMeta: true`) MUST be excluded.
3. User Events whose content consists exclusively of `tool_result` blocks MUST be excluded.

After filtering, the N-most-recent visible Events are returned in ascending chronological order, as today.

**Scenario: Unknown Kind Preserved (V1 invariant — superseded)**

The V1 scenario "Unknown Kind Preserved" stated that opaque Events MUST be included. That invariant is superseded by filter rule 1 above. Opaque Events are still decoded and stored; they are excluded only from the `SessionView.Events` slice returned by `Run`.

### adapters/tui/behavior

Row format changes from `HH:MM:SSZ <kind>` to `HH:MM:SSZ <kind>  <Summary>`. The `(opaque)` branch in `View()` is removed. Because filter rule 1 guarantees no opaque Events reach the TUI, no defensive guard is needed.

For User Events: `<kind>` renders as `user`; `<Summary>` is `UserPayload.Summary`.
For Assistant Events: `<kind>` renders as `assistant`; `<Summary>` is `AssistantPayload.Summary`.

Empty-state rendering: when `SessionView.Events` is empty (no Sessions exist, or all Events are filtered), the TUI MUST render a single friendly line such as `"No active conversation."`.

---

## Scenarios

### Filtering

**Scenario: Opaque events filtered from view**

Given a Session containing mixed Events including one or more Events with `Kind = KindOpaque`

When `WatchSession.Run` is invoked

Then the returned `SessionView.Events` MUST NOT contain any Event with `Kind = KindOpaque`

And the remaining visible Events MUST be returned in ascending chronological order

---

**Scenario: Meta user events filtered from view**

Given a Session containing User Events where one or more have `isMeta = true`

When `WatchSession.Run` is invoked

Then those meta User Events MUST NOT appear in `SessionView.Events`

---

**Scenario: Tool-result user events filtered from view**

Given a Session containing User Events whose content consists entirely of `tool_result` blocks (no text, no other block type)

When `WatchSession.Run` is invoked

Then those tool-result-only User Events MUST NOT appear in `SessionView.Events`

---

**Scenario: Typed user prompt visible**

Given a Session containing a User Event with typed text content `"Hello"`

When `WatchSession.Run` is invoked

Then `SessionView.Events` MUST include that User Event

And `UserPayload.Summary` MUST equal `"Hello"`

---

**Scenario: N counts visible events only**

Given a Session with 5 typed user prompts and 10 tool-result User Events (all other things equal, default N = 5)

When `WatchSession.Run` is invoked

Then `SessionView.Events` MUST contain exactly 5 Events

And all 5 MUST be typed user prompts (no tool-result Events)

---

### Summary Derivation

**Scenario: Assistant text response visible**

Given a Session containing an Assistant Event whose content is a single `text` block with value `"Let me check the file for you."`

When `WatchSession.Run` is invoked

Then `SessionView.Events` MUST include that Assistant Event

And `AssistantPayload.Summary` MUST equal `"Let me check the file for you."` (≤ 80 runes, no truncation needed)

---

**Scenario: Assistant tool_use response renders tool summary**

Given a Session containing an Assistant Event with a single `tool_use` block of name `"Read"` and `input.file_path = "/some/file.go"`

When `WatchSession.Run` is invoked

Then `AssistantPayload.Summary` MUST equal `"Tool: Read /some/file.go"`

---

**Scenario: Assistant with multiple tool_use uses first**

Given a Session containing an Assistant Event with two `tool_use` blocks: first `Edit /a.go`, then `Read /b.go`

When `WatchSession.Run` is invoked

Then `AssistantPayload.Summary` MUST be derived from the FIRST `tool_use` block

And `AssistantPayload.Summary` MUST equal `"Tool: Edit /a.go"`

---

**Scenario: Assistant thinking-only event**

Given a Session containing an Assistant Event whose content is a single `thinking` block (no `text`, no `tool_use`)

When `WatchSession.Run` is invoked

Then `AssistantPayload.Summary` MUST equal the literal string `"(thinking)"`

---

**Scenario: Assistant mixed text and tool_use — tool takes priority**

Given a Session containing an Assistant Event with both a `text` block and a `tool_use` block

When `WatchSession.Run` is invoked

Then `AssistantPayload.Summary` MUST be derived from the `tool_use` block, not the `text` block

---

**Scenario: Per-tool summary — Bash without description**

Given a Session containing an Assistant Event with a `tool_use` of name `"Bash"`, `input.command = "go test ./..."`, and no `input.description` (or empty description)

When `WatchSession.Run` is invoked

Then `AssistantPayload.Summary` MUST equal `"Tool: Bash 'go test ./...'"` (command wrapped in single quotes; truncated to first 60 runes of command before appending closing quote if the command exceeds 60 runes)

---

**Scenario: Per-tool summary — Bash with description**

Given a Session containing an Assistant Event with a `tool_use` of name `"Bash"` and `input.description = "Run tests"`

When `WatchSession.Run` is invoked

Then `AssistantPayload.Summary` MUST equal `"Tool: Bash Run tests"`

---

**Scenario: Per-tool summary — unknown tool**

Given a Session containing an Assistant Event with a `tool_use` of name `"ImaginaryTool"` (not in the known-tool table)

When `WatchSession.Run` is invoked

Then `AssistantPayload.Summary` MUST equal `"Tool: ImaginaryTool"`

---

**Per-tool key-arg format table (locked):**

| Tool name(s) | Summary format |
|---|---|
| `Read`, `Write`, `Edit`, `MultiEdit` | `Tool: <Name> <file_path>` |
| `Bash` | `Tool: Bash <description>` if description non-empty; else `Tool: Bash '<command[:60]>'` |
| `Grep`, `rg` | `Tool: Grep <pattern>` |
| `Glob` | `Tool: Glob <pattern>` |
| `Task`, `Agent` | `Tool: Task <description>` |
| `TodoWrite` | `Tool: TodoWrite (<n> items)` |
| Default (any other name) | `Tool: <Name>` |

---

### Truncation Invariants

**Rule: Rune-count limit**

`Summary` values MUST be at most 80 runes in length. Truncation MUST operate on rune counts, not byte counts, to handle multibyte UTF-8 characters correctly.

**Rule: Ellipsis on overflow**

When the source text exceeds 80 runes, the `Summary` MUST be truncated to 77 runes and the Unicode ellipsis character `…` (U+2026, 1 rune) appended, yielding a total of 78 runes. (Using a single `…` rune rather than three ASCII dots `...` keeps the count predictable and looks cleaner in a terminal.)

**Rule: Newline collapse**

All newline sequences (`\n`, `\r\n`, `\r`) MUST be replaced with a single space before truncation.

**Rule: Whitespace normalisation**

Multiple consecutive whitespace characters (space, tab, collapsed newlines) SHOULD be collapsed to a single space before truncation to avoid leading spaces or double-space artefacts.

**Scenario: Truncation at rune boundary**

Given a User Event with typed text content that is 120 runes long (ASCII for simplicity)

When `Summary` is derived

Then `Summary` MUST be exactly 78 runes: the first 77 runes of the collapsed text followed by `…`

---

**Scenario: Truncation at multibyte rune boundary**

Given a User Event with typed text content containing multibyte UTF-8 characters (e.g. Japanese or emoji) and a total rune count exceeding 80

When `Summary` is derived

Then `Summary` MUST be truncated on a rune boundary (no partial UTF-8 sequences)

And `Summary` MUST end with `…`

---

### TUI Rendering

**Scenario: Focused pane shows snippets, not opaque markers**

Given a Session whose last 5 visible Events include 3 User Events and 2 Assistant Events

When the TUI renders

Then each row MUST show timestamp, kind label, and `Summary` text

And no row MUST contain the literal string `"(opaque)"`

---

**Scenario: Row format**

Given any visible Event with timestamp `T`, kind `"user"`, and `Summary = "Hello world"`

When the TUI renders that Event

Then the rendered line MUST match the format `HH:MM:SSZ user  Hello world`

---

**Scenario: Empty Session**

Given a Project with no Sessions, or a Project whose Sessions contain only Events that are all filtered out

When the TUI renders

Then the TUI MUST display a single friendly line (e.g. `"No active conversation."`)

And the TUI MUST NOT panic or display an error trace

---

## Out of Scope (referenced from proposal)

The following items are explicitly deferred from this change:

- **Decoding other opaque kinds**: `system`, `attachment`, `progress`, `queue-operation`, `ai-title`, `permission-mode`, and `last-prompt` Events remain decoded as `OpaquePayload` and are filtered from the view. No summary derivation for these kinds.
- **Tool-result user event toggle**: hiding tool-result User Events is the V1.5 default. A "show tool results" toggle is deferred.
- **Configurable summary length**: hard-coded at 80 runes. Dynamic terminal-width truncation is deferred.
- **ANSI / color rendering**: no colour added in this change.
- **Multi-pane layout and session switching**: not in scope.
- **Pagination and scrolling**: not in scope.
- **Cost / token display changes**: not in scope.
- **Grapheme-cluster-aware truncation**: rune-count is sufficient for V1.5; grapheme cluster awareness (e.g. emoji with ZWJ sequences) is deferred.
