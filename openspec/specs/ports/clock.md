# Port Spec: Clock

## Interface: Clock

Clock is the port that provides the current time to the application and adapter layers.

### Method: Now

```
Now() time.Time
```

Returns the current time as a UTC timestamp.

**Behavior:**

- MUST return the current instant in UTC (time.UTC location).
- Successive calls MUST NOT return a time earlier than a preceding call (monotonic requirement for display purposes).

## Invariants

- **UTC only**: All returned timestamps MUST be in UTC.
- **Monotonic**: Time MUST never regress between successive calls within a single program execution.
- **No system calls in domain/application**: The domain and application layers MUST NOT call any system time facility directly; all current-time access MUST be delegated to `Clock`.

## Out of Scope for V1

- Custom time sources or time travel (though easy to implement for testing)
- Timezone handling beyond UTC
