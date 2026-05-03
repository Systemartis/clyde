package ports

import "time"

// Clock is the port that provides the current time to the application layer.
//
// The domain and application layers MUST NOT call time.Now() directly —
// all current-time access goes through Clock. This keeps time deterministic
// in tests and decouples the application from the system clock.
//
// Contract:
//   - Clock MUST return the current time as a UTC timestamp.
//   - Successive calls MUST NOT return a time earlier than a preceding call
//     (monotonic requirement for display purposes).
type Clock interface {
	// Now returns the current time in UTC.
	Now() time.Time
}
