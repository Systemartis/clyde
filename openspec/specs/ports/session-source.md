# Port Spec: SessionSource

## Interface: SessionSource

SessionSource is the contract through which the application layer discovers and reads Sessions for a Project.

### Method: Sessions

```
Sessions(ctx context.Context, cwd string) ([]session.Summary, error)
```

Discovers all Sessions associated with a given working directory.

**Behavior:**

- GIVEN a Project cwd, MUST return all Sessions for that Project.
- If no Sessions exist, MUST return an empty slice without error.
- MUST NOT drop or filter Sessions.
- MUST return Sessions in descending order by LastActivity (most recent first).

**Errors:**

- MUST NOT error if the directory does not exist (return empty slice instead).
- MAY error if the directory exists but cannot be read (permissions, I/O failure).

### Method: Events

```
Events(ctx context.Context, id session.ID) ([]event.Event, error)
```

Retrieves all Events for a given Session.

**Behavior:**

- GIVEN a Session ID, MUST return all Events belonging to that Session.
- MUST return Events in strictly ascending chronological order (by timestamp).
- MUST NOT drop or transform Events with unrecognized kinds.
- For unknown kinds, MUST preserve as opaque Events carrying id, timestamp, kind, sessionId, parentId, and raw payload bytes.
- If the Session is empty, MUST return an empty slice without error.

**Errors:**

- MAY error if the Session does not exist or cannot be read.
- MUST NOT panic due to malformed event data; unknown kinds are preserved as opaque.

## Invariants

- **Idempotency**: Multiple calls to `Sessions` or `Events` with the same arguments MUST return the same data (if no external changes).
- **No side effects**: `SessionSource` reads are observational only. No data MUST be modified as a result of calling these methods.
- **Consistency**: Events for a Session MUST always be returned in the same chronological order across successive calls.

## Out of Scope for V1

- Event streaming or real-time updates (deferred to `live-tail-fsnotify`)
- Writing or mutating Sessions (SessionSource is read-only)
