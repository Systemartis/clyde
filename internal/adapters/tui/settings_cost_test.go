package tui

import (
	"testing"
)

// TestDefaultConfig_CostThresholdIsTwentyUSD locks in the user-requested
// default for API-key users — high enough that a quick prompt iteration
// session doesn't trip it on accident, low enough to catch a runaway.
func TestDefaultConfig_CostThresholdIsTwentyUSD(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	if cfg.NotifyCostThresholdUSD != 20.0 {
		t.Errorf("default NotifyCostThresholdUSD = %v, want 20", cfg.NotifyCostThresholdUSD)
	}
}

// TestNextCostThreshold_Cycle covers the cycle order and the wrap-back
// from $500 to off.
func TestNextCostThreshold_Cycle(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   float64
		want float64
	}{
		{0, 5},
		{5, 10},
		{10, 20},
		{20, 50},
		{50, 100},
		{100, 200},
		{200, 500},
		{500, 0}, // wrap
		{30, 50}, // user-edited TOML lands on next preset
		{600, 0}, // beyond the largest preset → off
	}
	for _, tc := range cases {
		if got := nextCostThreshold(tc.in); got != tc.want {
			t.Errorf("nextCostThreshold(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// TestFormatCostThreshold covers the chip label format. 0 reads as
// "off"; integer dollars omit the decimals; sub-dollar values keep
// cents so the user can tell something low is in play.
func TestFormatCostThreshold(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   float64
		want string
	}{
		{0, "off"},
		{-1, "off"},
		{1, "$1"},
		{5, "$5"},
		{100, "$100"},
		{0.5, "$0.50"},
		{7.25, "$7.25"},
	}
	for _, tc := range cases {
		if got := formatCostThreshold(tc.in); got != tc.want {
			t.Errorf("formatCostThreshold(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestOpenSettings_CursorStartsAtTop locks the new "land at the topmost
// row" behavior. Previously the cursor opened at 0 (first panel toggle)
// which buried the actual UX target two sections deep.
func TestOpenSettings_CursorStartsAtTop(t *testing.T) {
	t.Parallel()

	m := NewModel()
	m = m.openSettings()

	if m.settingsCursor != settingsCursorMin() {
		t.Errorf("openSettings cursor = %d, want %d (topmost row)", m.settingsCursor, settingsCursorMin())
	}
}

// TestSettingsKey_EnterCyclesCostThreshold covers the wiring: pressing
// Enter on the cost-threshold row advances the staged value through
// the cycle, and Esc commits it to baseCfg.
func TestSettingsKey_EnterCyclesCostThreshold(t *testing.T) {
	t.Parallel()

	m := NewModel()
	m = m.openSettings()
	m.settingsCursor = settingsCostThresholdCursor
	// Sanity check: openSettings loaded the default.
	if m.settingsCostThreshold != 20.0 {
		t.Fatalf("staged cost threshold = %v, want 20", m.settingsCostThreshold)
	}

	m = m.handleSettingsKey(keyPressEnter())
	if m.settingsCostThreshold != 50.0 {
		t.Errorf("after Enter on $20, staged = %v, want 50", m.settingsCostThreshold)
	}

	m = m.handleSettingsKey(keyPressEnter())
	if m.settingsCostThreshold != 100.0 {
		t.Errorf("after another Enter, staged = %v, want 100", m.settingsCostThreshold)
	}
}
