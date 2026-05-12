package tui

import (
	"testing"
	"time"

	"github.com/Systemartis/clyde/internal/application/livesession"
	"github.com/Systemartis/clyde/internal/domain/event"
	"github.com/Systemartis/clyde/internal/domain/usage"
	"github.com/Systemartis/clyde/internal/ports"
)

// ─── nextResetRow: bar fill must be TIME elapsed, not plan-quota % ───────────

// TestNextResetRow_BarShowsTimeElapsedNotQuota is the regression test for the
// inaccurate "next reset" bar: the row's headline is a countdown ("1h 31m"),
// so its bar fill must be the % of TIME elapsed in the window — not the
// plan-quota utilization that applyPlanUsageToMock overlaid onto Percent.
func TestNextResetRow_BarShowsTimeElapsedNotQuota(t *testing.T) {
	t.Parallel()
	d := MockData{
		Usage5h: UsageWindowRow{
			Label:           "5h session",
			Percent:         49, // plan-quota % (overlay applied)
			IsPlanQuota:     true,
			ResetsIn:        "1h 31m",
			ResetElapsedPct: 70, // 3h29m elapsed of 5h
		},
	}
	row, ok := nextResetRow(d)
	if !ok {
		t.Fatal("nextResetRow ok = false, want true")
	}
	if row.Percent != 70 {
		t.Errorf("reset row Percent = %d, want 70 (time elapsed) — got the plan-quota %% instead", row.Percent)
	}
	if row.ResetsIn != "1h 31m" {
		t.Errorf("reset row ResetsIn = %q, want %q", row.ResetsIn, "1h 31m")
	}
}

// TestNextResetRow_Bound5hOnly verifies the reset row stays bound to the 5h
// window even when the weekly reset is closer in wall-clock time. The 5h
// cadence is what the user tracks moment-to-moment; the weekly countdown
// already lives on its own row.
func TestNextResetRow_Bound5hOnly(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	d := MockData{
		Usage5h: UsageWindowRow{
			Label:           "5h session",
			ResetsIn:        "4h 30m",
			ResetAt:         now.Add(4*time.Hour + 30*time.Minute),
			ResetElapsedPct: 10,
		},
		UsageWeek: UsageWindowRow{
			Label:           "weekly · all models",
			ResetsIn:        "2h",
			ResetAt:         now.Add(2 * time.Hour),
			ResetElapsedPct: 99,
		},
	}
	row, ok := nextResetRow(d)
	if !ok {
		t.Fatal("nextResetRow ok = false, want true")
	}
	if row.ResetsIn != "4h 30m" {
		t.Errorf("reset row ResetsIn = %q, want %q (bound to 5h window)", row.ResetsIn, "4h 30m")
	}
	if row.Percent != 10 {
		t.Errorf("reset row Percent = %d, want 10 (5h window elapsed)", row.Percent)
	}
}

// TestNextResetRow_FallsBackTo5hWithoutTimestamps covers the demo-mode path:
// rows authored with a ResetsIn string but no ResetAt timestamp keep the old
// "5h first" preference.
func TestNextResetRow_FallsBackTo5hWithoutTimestamps(t *testing.T) {
	t.Parallel()
	d := MockData{
		Usage5h:   UsageWindowRow{Label: "5h session", ResetsIn: "1h 31m", ResetElapsedPct: 70},
		UsageWeek: UsageWindowRow{Label: "weekly · all models", ResetsIn: "2d 7h", ResetElapsedPct: 67},
	}
	row, ok := nextResetRow(d)
	if !ok {
		t.Fatal("nextResetRow ok = false, want true")
	}
	if row.ResetsIn != "1h 31m" {
		t.Errorf("reset row ResetsIn = %q, want %q (5h preferred without timestamps)", row.ResetsIn, "1h 31m")
	}
	if row.Percent != 70 {
		t.Errorf("reset row Percent = %d, want 70", row.Percent)
	}
}

// TestNextResetRow_WeeklyOnly_NoBar verifies the reset row does NOT fall
// back to the weekly window when the 5h row has no countdown — when 5h data
// is missing the bar simply doesn't render.
func TestNextResetRow_WeeklyOnly_NoBar(t *testing.T) {
	t.Parallel()
	d := MockData{
		Usage5h:   UsageWindowRow{Label: "5h session", Empty: true},
		UsageWeek: UsageWindowRow{Label: "weekly · all models", ResetsIn: "2d 7h", ResetElapsedPct: 67},
	}
	if _, ok := nextResetRow(d); ok {
		t.Error("nextResetRow ok = true, want false (no weekly fallback — bar is 5h-only)")
	}
}

// TestNextResetRow_NoneAvailable verifies ok=false when no countdown exists.
func TestNextResetRow_NoneAvailable(t *testing.T) {
	t.Parallel()
	d := MockData{
		Usage5h:   UsageWindowRow{Label: "5h session", Empty: true},
		UsageWeek: UsageWindowRow{Label: "weekly · all models"}, // no ResetsIn
	}
	if _, ok := nextResetRow(d); ok {
		t.Error("nextResetRow ok = true, want false when no reset countdown is available")
	}
}

// ─── applyPlanUsageToMock: overlay must keep elapsed-time fields accurate ────

// TestApplyPlanUsage_RecomputesResetElapsed verifies that when the real plan
// quota overlay rewrites Percent (quota %) it ALSO records the reset timestamp
// and the window time-elapsed %, so the reset row stays accurate.
func TestApplyPlanUsage_RecomputesResetElapsed(t *testing.T) {
	t.Parallel()
	fetched := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	pu := ports.PlanUsage{
		FiveHour: ports.PlanWindow{
			Present:     true,
			Utilization: 49,
			ResetsAt:    fetched.Add(90 * time.Minute), // 210m elapsed of 300m = 70%
		},
		SevenDay: ports.PlanWindow{
			Present:     true,
			Utilization: 79,
			ResetsAt:    fetched.Add(48 * time.Hour), // 120h elapsed of 168h = 71%
		},
		FetchedAt: fetched,
	}
	d := MockData{
		Usage5h:   UsageWindowRow{Label: "5h session", TotalUsed: "186k"},
		UsageWeek: UsageWindowRow{Label: "weekly · all models", TotalUsed: "1.5M"},
	}
	got := applyPlanUsageToMock(d, pu, nil, fetched)

	if got.Usage5h.Percent != 49 {
		t.Errorf("Usage5h.Percent = %d, want 49 (quota %%)", got.Usage5h.Percent)
	}
	if got.Usage5h.ResetElapsedPct != 70 {
		t.Errorf("Usage5h.ResetElapsedPct = %d, want 70", got.Usage5h.ResetElapsedPct)
	}
	if !got.Usage5h.ResetAt.Equal(pu.FiveHour.ResetsAt) {
		t.Errorf("Usage5h.ResetAt = %v, want %v", got.Usage5h.ResetAt, pu.FiveHour.ResetsAt)
	}
	if got.UsageWeek.ResetElapsedPct != 71 {
		t.Errorf("UsageWeek.ResetElapsedPct = %d, want 71", got.UsageWeek.ResetElapsedPct)
	}
	if !got.UsageWeek.ResetAt.Equal(pu.SevenDay.ResetsAt) {
		t.Errorf("UsageWeek.ResetAt = %v, want %v", got.UsageWeek.ResetAt, pu.SevenDay.ResetsAt)
	}
}

// ─── deriveUsageFields: fallback path must also populate elapsed fields ──────

// TestDeriveUsageFields_SetsResetElapsed verifies the JSONL fallback path
// (no plan API) stamps ResetAt + ResetElapsedPct on the 5h and weekly rows.
func TestDeriveUsageFields_SetsResetElapsed(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	u := usage.Usage{Input: 10_000, Output: 2_000}
	v := livesession.View{
		Events: []event.Event{
			event.NewEvent("ev1", now, event.KindAssistant, "sid", "", event.AssistantPayload{
				Usage: u,
				Model: "claude-opus-4-7",
			}),
		},
		TotalUsage:     u,
		LatestUsage:    u,
		CurrentModel:   "claude-opus-4-7",
		AssistantTurns: 1,
		LastUpdate:     now,
		Usage5h:        u,
		UsageWeek:      u,
		Reset5hAt:      now.Add(90 * time.Minute), // 70% elapsed
		ResetWeekAt:    now.Add(48 * time.Hour),   // 71% elapsed
	}

	d := deriveUsageFields(v, MockData{Model: "opus 4.7"})

	if d.Usage5h.ResetElapsedPct != 70 {
		t.Errorf("Usage5h.ResetElapsedPct = %d, want 70", d.Usage5h.ResetElapsedPct)
	}
	if !d.Usage5h.ResetAt.Equal(v.Reset5hAt) {
		t.Errorf("Usage5h.ResetAt = %v, want %v", d.Usage5h.ResetAt, v.Reset5hAt)
	}
	if d.UsageWeek.ResetElapsedPct != 71 {
		t.Errorf("UsageWeek.ResetElapsedPct = %d, want 71", d.UsageWeek.ResetElapsedPct)
	}
	if !d.UsageWeek.ResetAt.Equal(v.ResetWeekAt) {
		t.Errorf("UsageWeek.ResetAt = %v, want %v", d.UsageWeek.ResetAt, v.ResetWeekAt)
	}
}

// TestApplyPlanUsage_CountdownTicksBetweenFetches is the regression test for
// the frozen countdown: plan usage refreshes every ~5 minutes, but the
// overlay computed ResetsIn/ResetElapsedPct against FetchedAt — so between
// fetches the countdown and bar stood still while the dashboard refreshed
// every second. ResetsAt is absolute; the overlay must use the CURRENT
// clock instant passed by the caller.
func TestApplyPlanUsage_CountdownTicksBetweenFetches(t *testing.T) {
	t.Parallel()
	fetched := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	now := fetched.Add(30 * time.Minute) // 30m after the last fetch
	pu := ports.PlanUsage{
		FiveHour: ports.PlanWindow{
			Present:     true,
			Utilization: 49,
			ResetsAt:    fetched.Add(90 * time.Minute), // 60m from `now`
		},
		FetchedAt: fetched,
	}
	d := MockData{Usage5h: UsageWindowRow{Label: "5h session", TotalUsed: "186k"}}

	got := applyPlanUsageToMock(d, pu, nil, now)

	if got.Usage5h.ResetsIn != "1h" {
		t.Errorf("ResetsIn = %q, want %q (60m remaining at the CURRENT instant, not 90m at fetch time)",
			got.Usage5h.ResetsIn, "1h")
	}
	// 60m remaining of 300m → 240m elapsed → 80%.
	if got.Usage5h.ResetElapsedPct != 80 {
		t.Errorf("ResetElapsedPct = %d, want 80 (computed against now, not FetchedAt)", got.Usage5h.ResetElapsedPct)
	}
}
