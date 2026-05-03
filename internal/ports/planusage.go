// Package ports — PlanUsageSource port definition.
//
// PlanUsageSource is the abstraction for retrieving the user's subscription
// plan-quota usage from a backend. The numbers it returns are the SAME ones
// shown on https://claude.ai/settings/usage — i.e. plan-quota percentages,
// NOT token counts derivable from local JSONL session files.
//
// Adapter locations:
//   - anthropic-api  → internal/adapters/anthropicapi (calls
//     /api/oauth/usage with the OAuth token Claude Code stores locally)
package ports

import (
	"context"
	"time"
)

// PlanUsageSource fetches plan-quota usage from the active LLM provider.
//
// Contract:
//   - Fetch returns a fully-populated PlanUsage on success.
//   - Fetch returns ErrPlanUsageUnavailable when credentials are missing or
//     the user is not on a metered plan; callers SHOULD degrade gracefully
//     (e.g. hide the plan-quota bars).
//   - Fetch returns ErrPlanUsageAuth when credentials exist but are
//     invalid / expired beyond refresh; callers SHOULD prompt the user to
//     re-auth (e.g. run `claude` again).
//   - Network errors are returned wrapped — callers MAY display a stale
//     last-good value if they cache.
//   - Implementations MUST be safe for concurrent calls.
type PlanUsageSource interface {
	Fetch(ctx context.Context) (PlanUsage, error)
}

// PlanUsage is the snapshot of the user's plan-quota state at FetchedAt.
type PlanUsage struct {
	// FiveHour is the rolling 5-hour session window.
	FiveHour PlanWindow

	// SevenDay is the rolling 7-day weekly window across all models.
	SevenDay PlanWindow

	// Tier is the human-readable plan tier name, derived from the
	// credentials file's rateLimitTier (e.g. "max_5x" → "Max 5x").
	// Empty when unknown.
	Tier string

	// FetchedAt is the time the PlanUsage was retrieved.
	FetchedAt time.Time
}

// PlanWindow describes one rolling-window quota.
type PlanWindow struct {
	// Utilization is the 0–100 percentage of plan quota consumed in this window.
	// Values may exceed 100 when the user is over-limit and on extra-usage credits.
	Utilization float64

	// ResetsAt is the wall-clock time when the window resets.
	// Zero when unknown — callers SHOULD treat it as "no countdown".
	ResetsAt time.Time

	// Present is false when the backend response did not include this window
	// (e.g. an A/B-test variant renamed the key). Callers SHOULD hide the
	// bar entirely rather than drawing a 0% value.
	Present bool
}
