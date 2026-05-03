# Domain Spec: Session

## Entity: Session

A Session is an ordered sequence of Events belonging to a single Claude Code conversation, scoped to exactly one Project.

### Properties

- **ID** (type ID string): Unique identifier for the Session.
- **LastActivity** (time.Time): Timestamp of the most recent Event in this Session (used for multi-session ordering).

### Invariants

- Sessions are identified by their ID.
- Every Session belongs to exactly one Project (identified by its working directory).
- A Session with no Events is valid (empty Session).

## Use Case Interactions

**WatchSession** uses Sessions to discover the most-recently-active Session for a given Project and retrieves its Events in chronological order.

## Out of Scope for V1

- Multi-session navigation UI (deferred to `sessions-list-panel`)
- Session creation, deletion, or mutation
- Session metadata beyond LastActivity (deferred to future changes)
