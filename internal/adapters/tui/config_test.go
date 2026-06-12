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
