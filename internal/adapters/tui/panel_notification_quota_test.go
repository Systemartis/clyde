package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/Systemartis/clyde/internal/ports"
)

// TestEvaluatePlanWindow_FiresOnceAtWarn verifies the warn threshold
// fires exactly once on the first crossing and does not re-fire on the
// next poll while utilization stays above the line.
func TestEvaluatePlanWindow_FiresOnceAtWarn(t *testing.T) {
	t.Parallel()

	fired := map[quotaFireKey]bool{}
	now := time.Now().UTC()
	w := ports.PlanWindow{Present: true, Utilization: 91, ResetsAt: now.Add(20 * time.Minute)}

	got, key := evaluatePlanWindow("5h", "5h plan window", w, fired, now, false)
	if got == nil {
		t.Fatal("expected warn fire on 91% utilization")
	}
	if got.Severity != QuotaSeverityWarn {
		t.Errorf("severity = %v, want warn", got.Severity)
	}
	if !strings.Contains(got.Headline, "91") {
		t.Errorf("headline missing utilization: %q", got.Headline)
	}
	fired[key] = true

	got2, _ := evaluatePlanWindow("5h", "5h plan window", w, fired, now, false)
	if got2 != nil {
		t.Error("warn must not re-fire while latched")
	}
}

// TestEvaluatePlanWindow_DangerOverridesWarn ensures a session that
// jumps from 80% straight to 96% surfaces the danger alert, not the
// stale warn-tier copy.
func TestEvaluatePlanWindow_DangerOverridesWarn(t *testing.T) {
	t.Parallel()

	fired := map[quotaFireKey]bool{}
	now := time.Now().UTC()
	w := ports.PlanWindow{Present: true, Utilization: 96, ResetsAt: now.Add(5 * time.Minute)}

	got, _ := evaluatePlanWindow("5h", "5h plan window", w, fired, now, false)
	if got == nil {
		t.Fatal("expected danger fire on 96% utilization")
	}
	if got.Severity != QuotaSeverityDanger {
		t.Errorf("severity = %v, want danger", got.Severity)
	}
}

// TestEvaluatePlanWindow_HysteresisRearm verifies that a drop below
// threshold-hysteresis clears the latch and a subsequent re-crossing
// fires again.
func TestEvaluatePlanWindow_HysteresisRearm(t *testing.T) {
	t.Parallel()

	fired := map[quotaFireKey]bool{}
	now := time.Now().UTC()

	hot := ports.PlanWindow{Present: true, Utilization: 92, ResetsAt: now.Add(time.Hour)}
	cool := ports.PlanWindow{Present: true, Utilization: 80, ResetsAt: now.Add(time.Hour)}

	if got, key := evaluatePlanWindow("5h", "5h plan window", hot, fired, now, false); got == nil {
		t.Fatal("first 92% must fire")
	} else {
		fired[key] = true
	}

	if got, _ := evaluatePlanWindow("5h", "5h plan window", cool, fired, now, false); got != nil {
		t.Error("80% should not fire — too low to cross")
	}

	// At this point the hysteresis (90 - 5 = 85) should have rearmed
	// the warn latch.
	if fired[quotaFireKey("5h:warn")] {
		t.Error("warn latch should rearm once utilization drops below 85%")
	}

	if got, _ := evaluatePlanWindow("5h", "5h plan window", hot, fired, now, false); got == nil {
		t.Error("re-crossing 92% after a cool-off must fire again")
	}
}

// TestEvaluatePlanWindow_MissingWindowDoesNothing covers the "Present
// = false" path: no alert at all, and no spurious latch.
func TestEvaluatePlanWindow_MissingWindowDoesNothing(t *testing.T) {
	t.Parallel()

	fired := map[quotaFireKey]bool{}
	got, _ := evaluatePlanWindow("5h", "5h plan window", ports.PlanWindow{Present: false}, fired, time.Now(), false)
	if got != nil {
		t.Error("missing window must not fire")
	}
	if len(fired) != 0 {
		t.Errorf("missing window must not touch latch map, got %v", fired)
	}
}

// TestEvaluateCostThreshold_OneShot verifies the cost trigger fires
// exactly once per session — cost only goes up, so no rearm.
func TestEvaluateCostThreshold_OneShot(t *testing.T) {
	t.Parallel()

	fired := map[quotaFireKey]bool{}

	if got := evaluateCostThreshold(4.99, 5.00, fired); got != nil {
		t.Error("under threshold must not fire")
	}

	got := evaluateCostThreshold(5.10, 5.00, fired)
	if got == nil {
		t.Fatal("crossing $5.00 must fire")
	}
	if !strings.Contains(got.Headline, "5.10") {
		t.Errorf("headline missing actual spend: %q", got.Headline)
	}
	fired[quotaCostFireKey] = true

	if got := evaluateCostThreshold(7.00, 5.00, fired); got != nil {
		t.Error("cost trigger must not re-fire after the latch")
	}
}

// TestResolveNotification_QuotaPriority confirms that quota loses to
// hook + compaction but wins over nothing — and that fullscreen mode
// is honored when style says fullscreen.
func TestResolveNotification_QuotaPriority(t *testing.T) {
	t.Parallel()

	q := QuotaNotification{Active: true, Headline: "5h · 92%", Severity: QuotaSeverityWarn}
	hook := HookNotification{Active: true, Tool: "Bash", KeyArg: "ls"}

	d := resolveNotification(NotificationFullscreen, false, hook, CompactionOK, q)
	if !d.Active || !d.Hook.Active {
		t.Error("hook should win when hook is active alongside quota")
	}

	d = resolveNotification(NotificationFullscreen, false, HookNotification{}, CompactionDanger, q)
	if !d.Active || d.Compaction != CompactionDanger {
		t.Error("compaction should win when no hook is in flight")
	}

	d = resolveNotification(NotificationFullscreen, false, HookNotification{}, CompactionOK, q)
	if !d.Active || !d.Quota.Active {
		t.Error("quota should activate when nothing else is in flight")
	}
	if !d.Fullscreen {
		t.Error("fullscreen style must propagate to quota-only decisions")
	}

	d = resolveNotification(NotificationOff, false, HookNotification{}, CompactionOK, q)
	if d.Active {
		t.Error("off style must mute even quota notifications")
	}
}
