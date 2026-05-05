package tui

import (
	"errors"
	"fmt"
	"time"

	"github.com/Systemartis/clyde/internal/application/livesession"
	"github.com/Systemartis/clyde/internal/domain/pricing"
	"github.com/Systemartis/clyde/internal/domain/usage"
	"github.com/Systemartis/clyde/internal/ports"
)

// shouldShowCost returns true when the dollar cost row is meaningful for
// this user. API-key users (no detected plan tier) pay per token, so the
// cost figure is their headline metric. Pro/Max subscribers pay a flat
// subscription and the per-token cost is misleading — they care about
// plan-quota %, not $.
//
// A transient plan-usage offline state does NOT flip a subscriber back to
// seeing $: the user is still on a subscription, the API endpoint is just
// temporarily unreachable.
func shouldShowCost(d MockData) bool {
	return d.PlanTier == ""
}

// deriveUsageFields populates the usage-panel fields of MockData from a
// LiveSession View. Uses the pre-accumulated TotalUsage, CurrentModel, and
// AssistantTurns fields on the View (set by Snapshot).
//
// Fields populated:
//   - Tokens200k:    total tokens (input+output+cache) for display
//   - TokenPct:      0..100 compaction percent based on pricing.CompactionPercent
//   - Cost142:       formatted USD cost string, e.g. "$1.42"
//   - Turns:         count of KindAssistant events as a string, e.g. "38"
//   - Model:         short model name, e.g. "opus 4.7 (1M)"
//   - UsageSession:  per-type breakdown row for this session (with CurrentCtx)
//   - Usage5h:       per-type breakdown row for last 5h (with ResetsIn)
//   - UsageWeek:     per-type breakdown row for last 7d (with ResetsIn)
//   - BurnRate:      derived burn-rate string, e.g. "~3.2k tok/min · $0.18/hr"
//
// Issues fields (Errors, Warnings, Tests) are deferred to Phase H.
func deriveUsageFields(v livesession.View, d MockData) MockData {
	if len(v.Events) == 0 {
		return d
	}

	u := v.TotalUsage
	m := pricing.Lookup(v.CurrentModel)

	// Total tokens for the "tokens" row: input + output + cache_creation.
	// cache_read is intentionally excluded — it accumulates the same cached
	// context once per turn, inflating the sum to impossibly high values.
	d.Tokens200k = int(pricing.TotalTokens(u))

	// Compaction percent uses the LATEST turn's context size, not the running sum.
	// The running sum's CacheRead inflates beyond any real context limit.
	d.TokenPct = int(pricing.CompactionPercent(v.LatestUsage, m) * 100)

	// Cost string formatted as "$X.XX" (e.g. "$1.42").
	d.Cost142 = fmt.Sprintf("$%.2f", pricing.Cost(u, m))

	// Turns = count of assistant events.
	d.Turns = fmt.Sprintf("%d", v.AssistantTurns)

	// Short model name: include (1M) suffix when the session is on the 1M variant.
	d.Model = shortModelName(v.CurrentModel)

	// Multi-window usage breakdown rows.
	// Session row: include CurrentCtx (LatestUsage context vs model limit).
	// Percent = TokenPct (compaction risk).
	sessionRow := buildUsageWindowRow("session ctx", v.TotalUsage, m, 0)
	if !sessionRow.Empty {
		ctxLabel := contextWindowLabel(d.Model)
		// CurrentCtx shows raw tokens vs the model limit. The percent is
		// already rendered as the row's headline value — appending "(N%)"
		// here is the duplicate the user complained about.
		sessionRow.CurrentCtx = fmt.Sprintf("%s / %s",
			formatTokenCount(pricing.ContextTokens(v.LatestUsage)),
			ctxLabel,
		)
		sessionRow.Percent = d.TokenPct
		sessionRow.IsPlanQuota = false // model-context fill, not a plan quota
	}
	d.UsageSession = sessionRow

	// 5h row: include ResetsIn countdown. Default Percent is the
	// time-elapsed approximation; applyPlanUsage overlays the REAL
	// plan-quota % when the API client is available.
	fiveHRow := buildUsageWindowRow("5h session", v.Usage5h, m, v.Sessions5hCount)
	if !fiveHRow.Empty {
		fiveHRow.ResetsIn = formatResetsIn(v.Reset5hAt, v.LastUpdate, false)
		fiveHRow.Percent = windowElapsedPercent(v.Reset5hAt, v.LastUpdate, 5*time.Hour)
		fiveHRow.IsPlanQuota = false // fallback — overlay sets true
	}
	d.Usage5h = fiveHRow

	// Weekly row: same default; overlay sets the real %.
	weekRow := buildUsageWindowRow("weekly · all models", v.UsageWeek, m, v.SessionsWeekCount)
	if !weekRow.Empty {
		weekRow.ResetsIn = formatResetsIn(v.ResetWeekAt, v.LastUpdate, true)
		weekRow.Percent = windowElapsedPercent(v.ResetWeekAt, v.LastUpdate, 7*24*time.Hour)
		weekRow.IsPlanQuota = false
	}
	d.UsageWeek = weekRow

	// Burn rate: derive from session duration (first to last event) and total tokens.
	d.BurnRate = deriveBurnRate(v)

	return d
}

// buildUsageWindowRow builds a UsageWindowRow with per-token-type breakdown for
// the given time window label, usage/model pair, and session count.
// sessionCount=0 means "not tracked" (shown for the session row or fallback).
func buildUsageWindowRow(label string, u usage.Usage, m pricing.Model, sessionCount int) UsageWindowRow {
	total := pricing.TotalTokens(u)
	if total == 0 {
		return UsageWindowRow{Label: label, Empty: true}
	}
	return UsageWindowRow{
		Label:        label,
		Input:        formatTokenCount(u.Input),
		Output:       formatTokenCount(u.Output),
		Cache:        formatTokenCount(u.CacheRead),
		Cost:         fmt.Sprintf("$%.2f", pricing.Cost(u, m)),
		TotalUsed:    formatTokenCount(total),
		SessionCount: sessionCount,
	}
}

// formatResetsIn computes a human-readable countdown from now until resetAt.
//
// Granularity decays as remaining shrinks:
//
//	weekly (useDays=true): "4d 14h" → "3d" → "5h" → "45m" → ""
//	5h    (useDays=false):                   "3h 57m" → "23m" → ""
//
// When remaining is below a minute we return "" rather than "0m" / "0h" — the
// countdown has effectively expired and the empty string lets the caller drop
// the row entirely instead of pinning a useless zero on screen.
func formatResetsIn(resetAt, now time.Time, useDays bool) string {
	if resetAt.IsZero() {
		return ""
	}
	remaining := resetAt.Sub(now)
	if remaining <= 0 {
		return ""
	}
	totalHours := int(remaining.Hours())
	totalMinutes := int(remaining.Minutes())
	if useDays {
		d := totalHours / 24
		h := totalHours % 24
		if d > 0 && h > 0 {
			return fmt.Sprintf("%dd %dh", d, h)
		}
		if d > 0 {
			return fmt.Sprintf("%dd", d)
		}
		if h > 0 {
			return fmt.Sprintf("%dh", h)
		}
		// < 1 hour remaining — show minutes so the user doesn't see "0h".
		if totalMinutes > 0 {
			return fmt.Sprintf("%dm", totalMinutes)
		}
		return ""
	}
	// 5h window: show h:mm, falling through to plain minutes when sub-hour.
	h := totalHours
	m := totalMinutes % 60
	if h > 0 && m > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	if h > 0 {
		return fmt.Sprintf("%dh", h)
	}
	if m > 0 {
		return fmt.Sprintf("%dm", m)
	}
	return ""
}

// applyPlanUsageToMock overlays the REAL plan-quota numbers from the
// Anthropic /api/oauth/usage endpoint on top of MockData rows that were
// previously populated with the time-elapsed fallback.
//
// Behavior:
//   - When err == ErrPlanUsageUnavailable: user is on API (no subscription
//     credentials). Sets IsAPIUser=true so the panel shows "api" and the
//     cost-threshold notification can fire. Not flagged as offline —
//     this is the expected steady state for API users, not an error.
//   - When err is otherwise non-nil, sets PlanUsageOffline=true and leaves
//     the fallback Percent values intact (graceful degradation).
//   - When err is nil and the FiveHour / SevenDay windows are Present,
//     overrides Percent with the real plan-quota %, sets IsPlanQuota=true,
//     and rewrites ResetsIn from the API timestamp.
//   - PlanTier is taken from the credentials file (stamped on PlanUsage)
//     so the title bar can display "Max 5x" alongside the model name.
func applyPlanUsageToMock(d MockData, pu ports.PlanUsage, err error) MockData {
	switch {
	case errors.Is(err, ports.ErrPlanUsageUnavailable):
		d.IsAPIUser = true
		d.PlanUsageOffline = false
	case err != nil:
		d.PlanUsageOffline = true
	default:
		d.PlanUsageOffline = false
		d.IsAPIUser = false
	}
	if pu.Tier != "" {
		d.PlanTier = pu.Tier
	}
	now := time.Now().UTC()
	if !pu.FetchedAt.IsZero() {
		now = pu.FetchedAt
	}

	if pu.FiveHour.Present && !d.Usage5h.Empty {
		d.Usage5h.Percent = roundPct(pu.FiveHour.Utilization)
		d.Usage5h.IsPlanQuota = true
		if !pu.FiveHour.ResetsAt.IsZero() {
			d.Usage5h.ResetsIn = formatResetsIn(pu.FiveHour.ResetsAt, now, false)
		}
	}
	if pu.SevenDay.Present && !d.UsageWeek.Empty {
		d.UsageWeek.Percent = roundPct(pu.SevenDay.Utilization)
		d.UsageWeek.IsPlanQuota = true
		if !pu.SevenDay.ResetsAt.IsZero() {
			d.UsageWeek.ResetsIn = formatResetsIn(pu.SevenDay.ResetsAt, now, true)
		}
	}
	return d
}

// roundPct rounds a float utilization to an int and pins negatives to 0.
// Values above 100 are passed through (extra-usage credits) so the panel's
// headline can display "157%"; the bar fill clamps to 100 at render time.
func roundPct(v float64) int {
	if v < 0 {
		return 0
	}
	return int(v + 0.5)
}

// windowElapsedPercent returns the 0-100 percentage of TIME elapsed in a rolling
// window of the given size, given the reset time and current time. 0 means just
// reset; 100 means about to reset. Returns 0 when resetAt is zero.
func windowElapsedPercent(resetAt, now time.Time, windowSize time.Duration) int {
	if resetAt.IsZero() || windowSize <= 0 {
		return 0
	}
	remaining := resetAt.Sub(now)
	if remaining <= 0 {
		return 100
	}
	if remaining >= windowSize {
		return 0
	}
	elapsed := windowSize - remaining
	pct := int(elapsed * 100 / windowSize)
	if pct < 0 {
		return 0
	}
	if pct > 100 {
		return 100
	}
	return pct
}

// deriveBurnRate computes a human-readable burn-rate string from a LiveSession View.
// Returns "" when there are fewer than 2 events or the session is too short to
// produce a meaningful rate (< 10 seconds).
func deriveBurnRate(v livesession.View) string {
	if len(v.Events) < 2 {
		return ""
	}
	first := v.Events[0].Timestamp
	last := v.Events[len(v.Events)-1].Timestamp
	elapsed := last.Sub(first)
	if elapsed < 10*time.Second {
		return ""
	}

	totalTok := pricing.TotalTokens(v.TotalUsage)
	if totalTok == 0 {
		return ""
	}

	m := pricing.Lookup(v.CurrentModel)
	cost := pricing.Cost(v.TotalUsage, m)

	tokPerMin := float64(totalTok) / elapsed.Minutes()
	costPerHr := cost / elapsed.Hours()

	var tokStr string
	switch {
	case tokPerMin < 1_000:
		tokStr = fmt.Sprintf("%.0f tok/min", tokPerMin)
	default:
		tokStr = fmt.Sprintf("%.1fk tok/min", tokPerMin/1_000)
	}

	return fmt.Sprintf("~%s · $%.2f/hr", tokStr, costPerHr)
}
