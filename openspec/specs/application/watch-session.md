# Application Spec: WatchSession

## Use Case: WatchSession

WatchSession composes a `SessionSource` port and a `Clock` port to derive a view of the most recent activity in the focused Session for the current Project.

### Intent

Display the last N Events (default N = 5) from the most-recently-active Session in a Project, together with the current timestamp, enabling the TUI to render a live feed of recent activity.

### Type: SessionView

```
SessionView {
  FocusedSession session.ID
  Events         []event.Event
  Now            time.Time
  EmptyReason    string
}
```

### Constructor

**New**(source ports.SessionSource, clock ports.Clock) WatchSession

Wires the WatchSession use case with a SessionSource and Clock port.

### Method

**Run**(ctx context.Context, cwd string) (SessionView, error)

Executes the use case for a given Project.

**Behavior:**

1. Discover all Sessions for the Project via `source.Sessions(ctx, cwd)`.
2. If no Sessions exist, return empty SessionView (or with EmptyReason set).
3. Select the Session with the most-recent LastActivity timestamp.
4. Retrieve all Events for the focused Session via `source.Events(ctx, focusedSession.ID)`.
5. Sort Events in ascending chronological order (by timestamp).
6. Take the last N Events (default N = 5); if fewer than N exist, return all.
7. Capture current time via `clock.Now()`.
8. Populate and return SessionView.

**Errors:**

- Return an error if SessionSource operations fail.
- MUST NOT panic; all errors are propagated to the caller.

## Scenarios

### Scenario: Chronological Order

**Given** a Session with Events at timestamps T1 < T2 < T3

**When** WatchSession retrieves Events for that Session

**Then** Events MUST be returned in ascending timestamp order (T1, T2, T3)

### Scenario: Surface Last N Events

**Given** a Session with more than N Events (N = 5)

**When** WatchSession is executed

**Then** ONLY the last N Events (by timestamp) SHALL be returned

**And** Events earlier than the Nth-from-last MUST NOT appear

### Scenario: Unknown Kind Preserved

**Given** a Session with Events of unknown kinds

**When** WatchSession retrieves Events

**Then** Events with unknown kinds MUST be included as opaque Events

**And** the use case MUST NOT error or panic

### Scenario: No Sessions Exist

**Given** a Project with no Sessions

**When** WatchSession is executed

**Then** the result MUST be an empty Event list

**And** the use case MUST NOT return an error

### Scenario: Multi-Session Focus Selection

**Given** a Project with multiple Sessions with different LastActivity timestamps

**When** WatchSession is executed

**Then** the Session with the greatest LastActivity SHALL be selected

**And** Events from all other Sessions MUST NOT appear

### Scenario: Fewer Than N Events

**Given** a Session with fewer than N Events

**When** WatchSession is executed

**Then** ALL Events in that Session SHALL be returned

**And** the result MUST NOT be padded or filled

## Invariants

- WatchSession is stateless; successive calls with the same cwd MAY return different results if Sessions or Events have changed externally.
- WatchSession is read-only; it MUST NOT modify any underlying data.
- All time values in the result are UTC (inherited from Clock port invariant).

## Out of Scope for V1

- Session caching or in-memory state
- Event streaming or real-time updates
- Multi-pane navigation (deferred to future panels)
