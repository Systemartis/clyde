# Domain Spec: Event

## Entity: Event

An Event is an atomic record of activity within a Session.

### Properties

- **id** (string): Globally unique identifier (UUID).
- **timestamp** (time.Time): UTC instant when the Event was recorded.
- **kind** (Kind): Category of activity (e.g., user turn, assistant turn, system action).
- **sessionId** (string): Session identifier for grouping Events.
- **parentId** (string, nullable): Identifier of the preceding Event in the conversation DAG (absent for root Events).
- **payload** (Payload): Typed or opaque event data.

### Event Kinds

Events are categorized by `Kind`:

- **KindUser** ("user"): User input Event. May carry UserPayload (minimal).
- **KindAssistant** ("assistant"): Assistant response Event. Carries AssistantPayload with Usage data.
- **Unknown kinds**: MUST be preserved as opaque Events carrying id, timestamp, kind, sessionId, parentId, and raw payload bytes.

### Payload (Sealed Interface)

`Payload` is a sealed interface to enforce a fixed set of concrete types:

- **UserPayload {}**: Empty payload for user Events.
- **AssistantPayload { Usage usage.Usage }**: Carries token usage for assistant Events.
- **OpaquePayload { Raw []byte }**: Preserves raw payload for unknown kinds.

All Payload types implement an unexported marker method `isPayload()` to seal the interface.

### Invariants

- Events are immutable once created.
- Every Event MUST carry a valid timestamp and kind.
- Unknown kinds MUST NOT be dropped; they are preserved as OpaquePayload.
- Events within a Session are ordered chronologically by timestamp.

## Constructor

**NewEvent**(id string, ts time.Time, kind Kind, sessionID, parentID string, payload Payload) Event

Constructs a new Event with all envelope fields populated.

## Out of Scope for V1

- Event mutation or deletion
- Event streaming via file-watcher (deferred to `live-tail-fsnotify`)
- Detailed payload parsing for Turn, ToolCall, Subagent, Todo Events (deferred to respective panels)
