package systemclock_test

import (
	"testing"
	"time"

	"github.com/clyde-tui/clyde/internal/adapters/systemclock"
	"github.com/clyde-tui/clyde/internal/ports"
)

// compile-time assertion: *Clock satisfies ports.Clock.
var _ ports.Clock = (*systemclock.Clock)(nil)

// TestClockNowIsUTC asserts that Now() always returns a UTC timestamp.
func TestClockNowIsUTC(t *testing.T) {
	t.Parallel()

	c := systemclock.New()
	got := c.Now()

	if got.Location() != time.UTC {
		t.Errorf("Now() location = %v; want UTC", got.Location())
	}
}

// TestClockNowIsMonotonic asserts that two successive Now() calls are
// non-decreasing (first ≤ second).
func TestClockNowIsMonotonic(t *testing.T) {
	t.Parallel()

	c := systemclock.New()
	before := c.Now()
	after := c.Now()

	if after.Before(before) {
		t.Errorf("Now() regressed: first=%v second=%v", before, after)
	}
}

// TestClockNowIsCloseToSystemTime asserts that Now() is within a small window
// of the actual system clock — guards against accidental epoch or fixed-time bugs.
func TestClockNowIsCloseToSystemTime(t *testing.T) {
	t.Parallel()

	const tolerance = 5 * time.Second

	systemBefore := time.Now().UTC()
	c := systemclock.New()
	got := c.Now()
	systemAfter := time.Now().UTC()

	if got.Before(systemBefore.Add(-tolerance)) || got.After(systemAfter.Add(tolerance)) {
		t.Errorf("Now() = %v is outside expected window [%v, %v]",
			got, systemBefore.Add(-tolerance), systemAfter.Add(tolerance))
	}
}
