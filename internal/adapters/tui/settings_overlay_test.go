package tui

import "testing"

// TestStartupCapable_ExcludesNowAndServers confirms the Startup section's
// "starts collapsed" toggle list filters out the always-visible panels.
// RememberLayout already persists their actual state when on; the toggle
// would just confuse a user who tried to flip it.
func TestStartupCapable_ExcludesNowAndServers(t *testing.T) {
	t.Parallel()

	toggles := settingsOverlayFromConfig(DefaultConfig(), ScopeGlobal, "")
	indices := startupCapableToggles(toggles)

	for _, idx := range indices {
		switch toggles[idx].PanelID {
		case PanelNow, PanelServers:
			t.Errorf("startupCapableToggles must not include %s", toggles[idx].Label)
		}
	}

	// Sanity: at least one panel makes the cut.
	if len(indices) == 0 {
		t.Fatal("startupCapableToggles returned empty — at least the activity panel should remain")
	}
}

// TestSettingsCursorMax_ProjectHidesStartup verifies that on Project scope
// the cursor stops at the last panel-visibility row instead of paging into
// the (now hidden) Startup section.
func TestSettingsCursorMax_ProjectHidesStartup(t *testing.T) {
	t.Parallel()

	toggles := settingsOverlayFromConfig(DefaultConfig(), ScopeGlobal, "")

	maxGlobal := settingsCursorMaxForScope(ScopeGlobal, toggles)
	maxProject := settingsCursorMaxForScope(ScopeProject, toggles)

	if maxProject >= maxGlobal {
		t.Errorf("project max %d should be strictly less than global max %d", maxProject, maxGlobal)
	}
	if want := len(toggles) - 1; maxProject != want {
		t.Errorf("project max = %d, want %d (last panel-visibility row)", maxProject, want)
	}
}

// TestOpenSettings_DefaultsToGlobal locks in the new "always Global"
// landing scope. RememberLayout is the actual collapse-state mechanism;
// the project tab is opt-in.
func TestOpenSettings_DefaultsToGlobal(t *testing.T) {
	t.Parallel()

	m := NewModel()
	m = m.openSettings()

	if m.settingsScope != ScopeGlobal {
		t.Errorf("openSettings landed on scope %v, want %v", m.settingsScope, ScopeGlobal)
	}
}

// TestSettingsKey_DownClampsInProjectScope verifies the cursor doesn't run
// off into the hidden Startup rows when on Project scope.
func TestSettingsKey_DownClampsInProjectScope(t *testing.T) {
	t.Parallel()

	m := NewModel()
	m.settingsOpen = true
	m.settingsScope = ScopeProject
	m.settingsToggles = settingsOverlayFromConfig(DefaultConfig(), ScopeProject, "/tmp/proj")
	m.settingsCursor = len(m.settingsToggles) - 1 // last visible (panel) row

	m = m.handleSettingsKey(keyPressDown())
	if m.settingsCursor != len(m.settingsToggles)-1 {
		t.Errorf("cursor advanced past last panel row in Project scope: got %d", m.settingsCursor)
	}
}
