package tui

import (
	"github.com/clyde-tui/clyde/internal/adapters/hookserver"
	"github.com/clyde-tui/clyde/internal/ports"
)

// planUsageMsg carries the result of a PlanUsageSource.Fetch.
// err is non-nil when the fetch failed; the model decides whether to keep
// the previous (stale) value or surface a "(plan offline)" badge.
type planUsageMsg struct {
	usage ports.PlanUsage
	err   error
}

// refreshPlanUsageMsg is the internal tick that triggers the next plan-usage fetch.
type refreshPlanUsageMsg struct{}

// refreshLiveMsg is the internal tick message that triggers a new LiveSession snapshot.
type refreshLiveMsg struct{}

// hookEventMsg carries a HookEvent from the server goroutine into the Bubble Tea
// Update loop. The event must be answered via ResponseCh.
type hookEventMsg struct {
	evt hookserver.HookEvent
}
