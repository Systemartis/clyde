package tui

import (
	"fmt"
	"time"

	"github.com/clyde-tui/clyde/internal/domain/pricing"
	"github.com/clyde-tui/clyde/internal/domain/usage"
	"github.com/clyde-tui/clyde/internal/ports"
)

// QuotaNotification carries a derived plan-quota or cost alert.
//
// Active is set when one of the configured thresholds has just been
// crossed upward; the renderers branch on Severity to pick the right
// chrome and copy.
//
// resolveNotification suppresses Active while notifAck is set, so
// dismiss flow matches hook + compaction.
type QuotaNotification struct {
	Active   bool
	Headline string
	Detail   string
	Severity QuotaSeverity
}

// QuotaSeverity tints the chrome and chooses the title copy.
type QuotaSeverity int

const (
	// QuotaSeverityWarn is the 90% crossing — heads-up.
	QuotaSeverityWarn QuotaSeverity = iota
	// QuotaSeverityDanger is the 95% crossing — final warning.
	QuotaSeverityDanger
)

// Quota threshold + hysteresis constants. The 5% hysteresis stops the
// notification from re-firing when utilization sits right at a
// threshold and oscillates by a tenth of a percent between polls.
const (
	quotaWarnPct       = 90.0
	quotaDangerPct     = 95.0
	quotaHysteresisPct = 5.0
)

// quotaFireKey identifies a single threshold instance for the
// fire-once-per-crossing tracker. Composition: "<window>:<level>" or
// the singleton "cost".
type quotaFireKey string

const quotaCostFireKey quotaFireKey = "cost"

// evaluateQuotaThresholds updates m.quotaNotif when a quota or cost
// threshold has just been crossed. Latches per-threshold keys in
// m.quotaFired so we don't re-fire on every poll; clears the latch
// when utilization drops far enough below the threshold to count as
// a fresh crossing.
//
// When a notification is raised the function clears notifAck so the
// new event paints over any previous dismiss.
func (m Model) evaluateQuotaThresholds() Model {
	if m.quotaFired == nil {
		m.quotaFired = map[quotaFireKey]bool{}
	}

	// If a quota notification is already on screen and undismissed,
	// don't replace it with another — let the user respond first.
	if m.quotaNotif.Active && !m.notifAck {
		return m
	}

	now := time.Now().UTC()
	if !m.planUsage.FetchedAt.IsZero() {
		now = m.planUsage.FetchedAt
	}

	// Plan windows: 5h then weekly. Higher severity considered first
	// so a single tick promotes warn → danger if both are newly crossed.
	if fired, key := evaluatePlanWindow("5h", "5h plan window", m.planUsage.FiveHour, m.quotaFired, now, false); fired != nil {
		m.quotaFired[key] = true
		m.quotaNotif = *fired
		m.notifAck = false
		return m
	}
	if fired, key := evaluatePlanWindow("weekly", "weekly plan window", m.planUsage.SevenDay, m.quotaFired, now, true); fired != nil {
		m.quotaFired[key] = true
		m.quotaNotif = *fired
		m.notifAck = false
		return m
	}

	// Session cost (opt-in via NotifyCostThresholdUSD; live mode only;
	// API users only). Subscribers pay a flat rate, so per-token spend
	// is mostly cosmetic for them — surfacing a "$X spent" alert there
	// is misleading. IsAPIUser stays false until the plan-usage probe
	// has confirmed there are no subscription credentials, so we never
	// fire on the unknown / pre-probe state either.
	if m.cfg.NotifyCostThresholdUSD > 0 && !m.demoMode && m.data.IsAPIUser {
		cost := computeSessionCost(m.liveView.CurrentModel, m.liveView.TotalUsage)
		if fired := evaluateCostThreshold(cost, m.cfg.NotifyCostThresholdUSD, m.quotaFired); fired != nil {
			m.quotaFired[quotaCostFireKey] = true
			m.quotaNotif = *fired
			m.notifAck = false
			return m
		}
	}
	return m
}

// evaluatePlanWindow checks one PlanWindow against warn + danger
// thresholds, returning the QuotaNotification (and its latch key)
// when a fresh upward crossing is detected. Resets latches when
// utilization drops below the hysteresis low-water mark so a later
// re-crossing fires again.
func evaluatePlanWindow(slug, label string, w ports.PlanWindow, fired map[quotaFireKey]bool, now time.Time, useDays bool) (*QuotaNotification, quotaFireKey) {
	if !w.Present {
		return nil, ""
	}
	dangerKey := quotaFireKey(slug + ":danger")
	warnKey := quotaFireKey(slug + ":warn")

	rearm(fired, dangerKey, w.Utilization, quotaDangerPct-quotaHysteresisPct)
	rearm(fired, warnKey, w.Utilization, quotaWarnPct-quotaHysteresisPct)

	if w.Utilization >= quotaDangerPct && !fired[dangerKey] {
		return &QuotaNotification{
			Active:   true,
			Headline: fmt.Sprintf("%s · %.0f%% used", label, w.Utilization),
			Detail:   resetsDetail(w, now, useDays),
			Severity: QuotaSeverityDanger,
		}, dangerKey
	}
	if w.Utilization >= quotaWarnPct && !fired[warnKey] {
		return &QuotaNotification{
			Active:   true,
			Headline: fmt.Sprintf("%s · %.0f%% used", label, w.Utilization),
			Detail:   resetsDetail(w, now, useDays),
			Severity: QuotaSeverityWarn,
		}, warnKey
	}
	return nil, ""
}

// evaluateCostThreshold fires once when session cost first crosses
// the configured dollar threshold. Cost only goes up within a session,
// so there's no hysteresis — once fired, never re-fires (restart
// clyde to rearm).
func evaluateCostThreshold(currentCost, threshold float64, fired map[quotaFireKey]bool) *QuotaNotification {
	if currentCost < threshold {
		return nil
	}
	if fired[quotaCostFireKey] {
		return nil
	}
	return &QuotaNotification{
		Active:   true,
		Headline: fmt.Sprintf("session cost · $%.2f spent", currentCost),
		Detail:   fmt.Sprintf("crossed your $%.2f threshold", threshold),
		Severity: QuotaSeverityWarn,
	}
}

// rearm clears the fired flag when utilization has fallen far enough
// below the threshold for a new crossing to count.
func rearm(fired map[quotaFireKey]bool, key quotaFireKey, util, lowWaterPct float64) {
	if util < lowWaterPct {
		delete(fired, key)
	}
}

// resetsDetail builds the secondary "resets in X" line when we have a
// reset timestamp; falls back to the headroom hint otherwise.
func resetsDetail(w ports.PlanWindow, now time.Time, useDays bool) string {
	if w.ResetsAt.IsZero() {
		headroom := 100 - w.Utilization
		if headroom < 0 {
			headroom = 0
		}
		return fmt.Sprintf("%.0f%% headroom left", headroom)
	}
	in := formatResetsIn(w.ResetsAt, now, useDays)
	if in == "" {
		return "resets shortly"
	}
	return "resets in " + in
}

// computeSessionCost returns the live session's accumulated cost in
// USD using the pricing table's lookup for the current model. Pure
// function so tests can drive it without standing up a Model.
func computeSessionCost(currentModel string, total usage.Usage) float64 {
	if pricing.TotalTokens(total) == 0 {
		return 0
	}
	return pricing.Cost(total, pricing.Lookup(currentModel))
}
