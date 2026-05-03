package tui

import (
	"testing"
)

// TestShouldShowCost_APIKeyUser verifies that an API-key user (no plan tier)
// sees the cost row — for them the dollar figure is the only meaningful metric.
func TestShouldShowCost_APIKeyUser(t *testing.T) {
	t.Parallel()
	d := MockData{PlanTier: ""}
	if !shouldShowCost(d) {
		t.Error("shouldShowCost(API-key) = false, want true (cost is the headline metric for API users)")
	}
}

// TestShouldShowCost_HidesForProMaxSubscriber verifies that a Pro/Max user's
// cost row is hidden — the dollar figure is misleading because they pay a flat
// subscription, not per-token.
func TestShouldShowCost_HidesForProMaxSubscriber(t *testing.T) {
	t.Parallel()
	cases := []string{"Max 5x", "Max 20x", "Pro", "max_5x"}
	for _, tier := range cases {
		d := MockData{PlanTier: tier}
		if shouldShowCost(d) {
			t.Errorf("shouldShowCost(%q) = true, want false — subscribers do not pay per token", tier)
		}
	}
}

// TestShouldShowCost_HiddenWhenPlanOfflineForSubscriber verifies that
// transient plan-usage outages do not flip a subscriber back to seeing $.
// They are still on a subscription; the $ figure is still misleading.
func TestShouldShowCost_HiddenWhenPlanOfflineForSubscriber(t *testing.T) {
	t.Parallel()
	d := MockData{PlanTier: "Max 5x", PlanUsageOffline: true}
	if shouldShowCost(d) {
		t.Error("shouldShowCost(subscriber, plan offline) = true, want false")
	}
}
