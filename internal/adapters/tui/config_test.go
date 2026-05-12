package tui

import (
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

// TestDefaultConfig verifies that DefaultConfig returns sensible defaults.
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Layout.DefaultMode != LayoutStack {
		t.Errorf("default mode = %q, want %q", cfg.Layout.DefaultMode, LayoutStack)
	}
	if cfg.Layout.AutoSwitchThreshold != 80 {
		t.Errorf("auto_switch_threshold = %d, want 80", cfg.Layout.AutoSwitchThreshold)
	}
	if !cfg.Panels.Now.Enabled {
		t.Error("panels.now should be enabled by default")
	}
	if cfg.Panels.Now.DefaultCollapsed {
		t.Error("panels.now should NOT be collapsed by default (it's the primary panel)")
	}
	if !cfg.Panels.Tasks.DefaultCollapsed {
		t.Error("panels.tasks should be collapsed by default")
	}
}

// TestResolveModeAutoSwitch verifies narrow widths force stack mode.
// v6 note: narrow threshold is 80 cols (was 100 in v5); multi-col still requires 160+.
func TestResolveModeAutoSwitch(t *testing.T) {
	cases := []struct {
		preferred LayoutMode
		width     int
		want      LayoutMode
	}{
		{LayoutTabs, 70, LayoutStack},         // below threshold → force stack
		{LayoutTabs, 80, LayoutTabs},          // at threshold → use preferred
		{LayoutTabs, 130, LayoutTabs},         // medium → use preferred
		{LayoutMultiCol, 70, LayoutStack},     // narrow + multi-col → force stack
		{LayoutMultiCol, 120, LayoutTabs},     // multi-col but < 160 → fall back to tabs
		{LayoutMultiCol, 130, LayoutTabs},     // multi-col still < 160 → fall back to tabs
		{LayoutMultiCol, 160, LayoutMultiCol}, // wide enough → multi-col
		{LayoutStack, 70, LayoutStack},        // narrow + stack → stay stack
		{LayoutStack, 150, LayoutStack},       // wide + stack → stay stack (user chose it)
	}
	for _, tc := range cases {
		got := ResolveMode(tc.preferred, 80, tc.width)
		if got != tc.want {
			t.Errorf("ResolveMode(%q, 80, %d) = %q, want %q",
				tc.preferred, tc.width, got, tc.want)
		}
	}
}

// TestEffectiveFor_NoOverride verifies the unmodified config is returned
// when the cwd has no [projects."..."] entry.
func TestEffectiveFor_NoOverride(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	got := cfg.EffectiveFor("/some/random/path")
	if got.Panels.Diff.Enabled != cfg.Panels.Diff.Enabled {
		t.Errorf("EffectiveFor with no override should not mutate panels")
	}
}

// TestEffectiveFor_PanelOverride verifies a project override replaces the
// global Enabled flag for the specified panels and leaves others untouched.
func TestEffectiveFor_PanelOverride(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	cfg.Projects = map[string]ProjectOverride{
		"/proj/a": {
			Panels: map[string]bool{"diff": true, "now": false},
		},
	}
	got := cfg.EffectiveFor("/proj/a")
	if !got.Panels.Diff.Enabled {
		t.Errorf("expected Diff.Enabled=true after override, got false")
	}
	if got.Panels.Now.Enabled {
		t.Errorf("expected Now.Enabled=false after override, got true")
	}
	if got.Panels.Calls.Enabled != cfg.Panels.Calls.Enabled {
		t.Errorf("Calls should be unchanged when not in override")
	}
}

// TestEffectiveFor_TasksAliasMapsToCalls verifies that the legacy 'tasks'
// key in the override still routes to the Calls slot.
func TestEffectiveFor_TasksAliasMapsToCalls(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	cfg.Panels.Calls.Enabled = true
	cfg.Projects = map[string]ProjectOverride{
		"/proj": {Panels: map[string]bool{"tasks": false}},
	}
	got := cfg.EffectiveFor("/proj")
	if got.Panels.Calls.Enabled {
		t.Errorf("legacy tasks=false should disable Calls slot")
	}
}

// TestEffectiveFor_EmptyCwd verifies a blank cwd returns the config
// unchanged even when projects are configured.
func TestEffectiveFor_EmptyCwd(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	cfg.Projects = map[string]ProjectOverride{
		"/anywhere": {Panels: map[string]bool{"diff": true}},
	}
	got := cfg.EffectiveFor("")
	if got.Panels.Diff.Enabled {
		t.Errorf("empty cwd should not match any project override")
	}
}

// TestConfig_TOMLRoundTrip verifies the project section serializes and
// deserializes cleanly, including paths with quoted special characters.
func TestConfig_TOMLRoundTrip(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	cfg.Projects = map[string]ProjectOverride{
		"/Users/vladpb/work/clyde": {
			Panels: map[string]bool{"diff": true, "bash": true},
		},
	}
	var buf strings.Builder
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		t.Fatalf("encode failed: %v", err)
	}
	var got Config
	if _, err := toml.Decode(buf.String(), &got); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if got.Projects["/Users/vladpb/work/clyde"].Panels["diff"] != true {
		t.Errorf("round-trip lost the diff override")
	}
}

// TestProjectOverride_PanelLayoutsRoundTrip verifies the panel_layouts
// per-project section serializes and deserializes through TOML cleanly.
func TestProjectOverride_PanelLayoutsRoundTrip(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	cfg.Projects = map[string]ProjectOverride{
		"/proj/a": {
			PanelLayouts: map[string]PanelLayout{
				"usage": {Height: 22, DefaultCollapsed: false},
				"diff":  {DefaultCollapsed: true},
			},
		},
	}
	var buf strings.Builder
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		t.Fatalf("encode failed: %v", err)
	}
	var got Config
	if _, err := toml.Decode(buf.String(), &got); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	got2 := got.Projects["/proj/a"].PanelLayouts["usage"]
	if got2.Height != 22 {
		t.Errorf("usage height round-trip = %d, want 22", got2.Height)
	}
	if !got.Projects["/proj/a"].PanelLayouts["diff"].DefaultCollapsed {
		t.Errorf("diff DefaultCollapsed round-trip lost")
	}
}

// TestEffectiveFor_PanelLayoutOverride verifies that a per-project
// PanelLayouts entry overlays Height and DefaultCollapsed on top of the
// global panels section.
func TestEffectiveFor_PanelLayoutOverride(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	cfg.Panels.Usage.Height = 14 // global default
	cfg.Projects = map[string]ProjectOverride{
		"/proj/a": {
			PanelLayouts: map[string]PanelLayout{
				"usage": {Height: 22, DefaultCollapsed: false},
				"now":   {DefaultCollapsed: true},
			},
		},
	}
	got := cfg.EffectiveFor("/proj/a")
	if got.Panels.Usage.Height != 22 {
		t.Errorf("usage height = %d, want 22 (project override)", got.Panels.Usage.Height)
	}
	if !got.Panels.Now.DefaultCollapsed {
		t.Errorf("now DefaultCollapsed should be true under project override")
	}
}

// TestProjectLayout_CollapsedStateRoundTrip verifies that a user's
// collapsed/expanded choice in one project persists across launches:
// save → EffectiveFor for that cwd → resulting Panels mirror the choice.
func TestProjectLayout_CollapsedStateRoundTrip(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	cfg.Panels.Diff.DefaultCollapsed = false // global says expanded
	collapse := make([]PanelCollapseState, panelCount)
	collapse[PanelDiff] = NewPanelCollapseState(true, 10) // user collapsed in /proj/a
	heights := map[PanelID]int{}
	setProjectPanelLayouts(&cfg, "/proj/a", collapse, heights)

	effective := cfg.EffectiveFor("/proj/a")
	if !effective.Panels.Diff.DefaultCollapsed {
		t.Errorf("/proj/a: Diff.DefaultCollapsed = false, want true (persisted)")
	}

	// A second project without an override still sees the global default.
	effectiveB := cfg.EffectiveFor("/proj/b")
	if effectiveB.Panels.Diff.DefaultCollapsed {
		t.Errorf("/proj/b: Diff.DefaultCollapsed = true, want false (global default)")
	}
}

// TestEffectiveFor_PanelLayoutLeavesOtherProjectsAlone verifies that
// applying the override for cwd A does not mutate the global config seen
// by cwd B.
func TestEffectiveFor_PanelLayoutLeavesOtherProjectsAlone(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	cfg.Panels.Usage.Height = 14
	cfg.Projects = map[string]ProjectOverride{
		"/proj/a": {
			PanelLayouts: map[string]PanelLayout{
				"usage": {Height: 22},
			},
		},
	}
	gotB := cfg.EffectiveFor("/proj/b")
	if gotB.Panels.Usage.Height != 14 {
		t.Errorf("project B usage height = %d, want 14 (global default)", gotB.Panels.Usage.Height)
	}
}

// TestSetProjectPanelLayouts_WritesFullSnapshot verifies that any
// interaction in a project persists the complete collapsed/expanded
// state for all panels, not just the one the user touched. This lets the
// user assume "the whole layout would remain the same" across launches.
func TestSetProjectPanelLayouts_WritesFullSnapshot(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	collapse := make([]PanelCollapseState, panelCount)
	// PanelDiff is collapsed by the user; the rest are expanded.
	collapse[PanelDiff] = NewPanelCollapseState(true, 10)
	heights := map[PanelID]int{
		PanelUsage: 22, // user manually resized usage only
	}
	setProjectPanelLayouts(&cfg, "/proj/a", collapse, heights)
	override := cfg.Projects[NormalizeCwd("/proj/a")]
	if got := len(override.PanelLayouts); got != int(panelCount) {
		t.Errorf("PanelLayouts entries = %d, want %d (all panels)", got, panelCount)
	}
	if override.PanelLayouts["usage"].Height != 22 {
		t.Errorf("usage layout height = %d, want 22", override.PanelLayouts["usage"].Height)
	}
	if !override.PanelLayouts["diff"].DefaultCollapsed {
		t.Errorf("diff DefaultCollapsed should be true")
	}
	if override.PanelLayouts["now"].DefaultCollapsed {
		t.Errorf("now DefaultCollapsed should be false (expanded)")
	}
}
