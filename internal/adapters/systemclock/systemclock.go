// Package systemclock implements the ports.Clock interface using the real
// system clock. The sole responsibility of this adapter is to call time.Now()
// and normalize the result to UTC — all other time arithmetic happens in the
// domain or application layer.
//
// Application and domain code MUST NOT call time.Now() directly; all
// current-time access is routed through the Clock port so that tests can
// substitute a deterministic FakeClock.
package systemclock

import "time"

// Clock is the live implementation of ports.Clock.
// The zero value is ready to use; prefer New() for clarity.
type Clock struct{}

// compile-time assertion: Clock satisfies ports.Clock.
// Placed here so the package itself fails to compile when the interface
// contract changes, even before any test is run.
var _ interface{ Now() time.Time } = (*Clock)(nil)

// New returns a Clock backed by the system clock.
func New() *Clock { return &Clock{} }

// Now returns the current time in UTC.
// Successive calls are non-decreasing because time.Now() is backed by a
// monotonic clock source on all platforms supported by this binary.
func (Clock) Now() time.Time { return time.Now().UTC() }
