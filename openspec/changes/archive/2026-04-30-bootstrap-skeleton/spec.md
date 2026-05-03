# Spec: bootstrap-skeleton

## Domain Entities

### Session

A Session is an ordered sequence of Events belonging to a single Claude Code conversation, scoped to exactly one Project. Each Session has a unique identifier and is associated with a Project by that Project's working directory. Events within a Session MUST be ordered chronologically by their timestamp. A Session with no Events is valid (empty Session).

### Event

An Event is an atomic record of activity within a Session. Every Event MUST carry:

- **id** — a globally unique identifier (UUID).
- **timestamp** — the moment the Event was recorded, expressed as a UTC instant.
- **kind** — the category of activity (e.g., user turn, assistant turn, system hook). Unknown kinds MUST be preserved as an opaque variant rather than rejected.
- **sessionId** — the identifier of the Session this Event belongs to.
- **parentId** — the identifier of the preceding Event in the conversation DAG (absent for the root Event).

Events of kind `user` and `assistant` are first-class for the V1 slice. All other kinds MUST be preserved as opaque Events. An opaque Event carries its id, timestamp, kind, sessionId, and parentId but no parsed payload.

### Usage

Usage is an immutable value object that holds token counts accumulated during assistant Events. A Usage value carries four counters: **input**, **output**, **cacheRead**, and **cacheCreation** — all non-negative integers.

Usage MUST satisfy the following algebraic laws:

| Law | Statement |
|-----|-----------|
| Identity | `a.Add(Zero)` equals `a` |
| Commutativity | `a.Add(b)` equals `b.Add(a)` |
| Associativity | `a.Add(b).Add(c)` equals `a.Add(b.Add(c))` |

`Add` MUST return a new Usage value; it MUST NOT mutate the receiver.

### Project

A Project is identified by its absolute working directory path. All Sessions discovered for a Project MUST share that Project's working directory as their origin context.

---

## Use Case: WatchSession

### Behavior

WatchSession composes a `SessionSource` and a `Clock` to derive a view of the most recent activity in the focused Session for the current Project. It asks the `SessionSource` to discover Sessions, selects the most-recently-active one, retrieves its Events in chronological order, and returns the last N Events (default N = 5) together with the current time from the `Clock`. The use case does not perform any I/O itself; all data access is delegated to the `SessionSource` port.

### Scenarios

**Scenario: Reading events yields chronological order**

- GIVEN a Project with a Session containing Events at timestamps T1 < T2 < T3
- WHEN WatchSession retrieves Events for that Session
- THEN Events MUST be returned in ascending timestamp order (T1, T2, T3)

**Scenario: Surface most recent N Events**

- GIVEN a Session containing more than N Events (default N = 5)
- WHEN WatchSession is executed for that Session
- THEN ONLY the last N Events (by timestamp) SHALL be included in the result
- AND Events earlier than the Nth-from-last MUST NOT appear in the result

**Scenario: Unknown Event kind is preserved, not dropped**

- GIVEN a Session whose Events include one or more Events with an unrecognized kind
- WHEN WatchSession retrieves those Events
- THEN Events with unknown kinds MUST be included in the result as opaque Events
- AND the use case MUST NOT error or panic due to the unknown kind

**Scenario: No Sessions exist for the Project**

- GIVEN a Project for which no Sessions have been recorded
- WHEN WatchSession is executed
- THEN the result MUST be an empty Event list
- AND the use case MUST NOT return an error

**Scenario: Multiple Sessions — surface most-recently-active**

- GIVEN a Project with multiple Sessions, each having at least one Event
- WHEN WatchSession is executed
- THEN the Session whose latest Event has the greatest timestamp SHALL be selected as the focused Session
- AND Events from all other Sessions MUST NOT appear in the result

**Scenario: Session has fewer than N Events**

- GIVEN a Session containing fewer than N Events
- WHEN WatchSession is executed
- THEN ALL Events in that Session SHALL be returned
- AND the result MUST not be padded or filled with placeholder Events

---

## Usage Invariants

Usage MUST be an immutable value object. `Add` returns a new Usage.

| Invariant | Formula | Must hold |
|-----------|---------|-----------|
| Identity (left) | `Zero.Add(a) = a` | Always |
| Identity (right) | `a.Add(Zero) = a` | Always |
| Commutativity | `a.Add(b) = b.Add(a)` | Always |
| Associativity | `a.Add(b).Add(c) = a.Add(b.Add(c))` | Always |

**Scenario: Add is non-mutating**

- GIVEN a Usage value `a`
- WHEN `a.Add(b)` is called
- THEN a new Usage value SHALL be returned
- AND `a` MUST remain unchanged

**Scenario: Zero is the identity element**

- GIVEN `Usage.Zero()`
- WHEN `Zero.Add(a)` is called for any Usage `a`
- THEN the result MUST equal `a` in all four counter fields

**Scenario: Counters accumulate correctly**

- GIVEN `a` with input=3, output=4, cacheRead=1, cacheCreation=2
- AND `b` with input=1, output=2, cacheRead=3, cacheCreation=4
- WHEN `a.Add(b)` is called
- THEN the result MUST have input=4, output=6, cacheRead=4, cacheCreation=6

---

## Port Contracts

### SessionSource

`SessionSource` is the port through which the application layer discovers and reads Sessions for a Project.

The `SessionSource` contract:

1. **Discovery** — Given a Project, `SessionSource` MUST return all Sessions associated with that Project. If no Sessions exist, it MUST return an empty collection without error.

2. **Event streaming** — Given a Session identifier, `SessionSource` MUST return all Events for that Session in strictly ascending chronological order (by Event timestamp). If the Session is empty, it MUST return an empty collection without error.

3. **Unknown kinds** — `SessionSource` MUST NOT drop or transform Events with unrecognized kinds. It MUST surface them as opaque Events carrying at minimum: id, timestamp, kind, sessionId, and parentId.

4. **No side effects** — `SessionSource` reads are observational only. `SessionSource` MUST NOT modify the underlying data when called.

### Clock

`Clock` is the port that provides the current time to the application layer.

The `Clock` contract:

1. `Clock` MUST return the current time as a UTC timestamp.
2. Successive calls to `Clock` MUST NOT return a time earlier than a preceding call (monotonic requirement for display purposes).
3. The domain and application layers MUST NOT call any system time facility directly; all current-time access MUST go through `Clock`.

---

## TUI Behavior

### Scenarios

**Scenario: TUI displays focused Session's last N Events on startup**

- GIVEN a Project with at least one Session containing Events
- WHEN the TUI is launched
- THEN the TUI MUST display the last N Events (default N = 5) of the most-recently-active Session
- AND each Event MUST show its timestamp and kind

**Scenario: TUI displays Event timestamp and kind**

- GIVEN an Event with a known timestamp and kind
- WHEN the TUI renders that Event
- THEN the rendered line MUST include the Event's timestamp
- AND the rendered line MUST include the Event's kind identifier

**Scenario: TUI shows empty state when no Sessions exist**

- GIVEN a Project with no Sessions
- WHEN the TUI is launched
- THEN the TUI MUST render without error
- AND the TUI SHOULD display an empty or placeholder state

**Scenario: Quit via q key**

- GIVEN the TUI is running
- WHEN the user presses `q`
- THEN the TUI MUST exit cleanly with no error

**Scenario: Quit via ctrl+c**

- GIVEN the TUI is running
- WHEN the user presses `ctrl+c`
- THEN the TUI MUST exit cleanly with no error

---

## Out of Scope

The following capabilities are explicitly deferred to named follow-up changes:

| Capability | Deferred change |
|---|---|
| Sessions list panel and multi-session focus switching | `sessions-list-panel` |
| Token and cost panel | `usage-and-cost-panel` |
| Todos panel (`TodoSource` port, Todo entity) | `todos-panel` |
| Recent file changes panel | `file-changes-panel` |
| Sub-agent tree panel (queue-operation Event parsing) | `subagent-tree-panel` |
| Compaction warning | `compaction-warning` |
| Live Event streaming via file-watcher | `live-tail-fsnotify` |
| Multi-pane layout | folded into `sessions-list-panel` |
| Turn, ToolCall, Subagent, Todo domain entities | respective panels |
| Multi-project view or daemon split | not yet planned |
| Mouse support, image rendering, focus reporting | not yet planned |
