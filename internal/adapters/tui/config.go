package tui

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// LayoutMode is a named layout mode.
type LayoutMode string

// Layout mode constants.
const (
	LayoutStack    LayoutMode = "stack"
	LayoutTabs     LayoutMode = "tabs"
	LayoutMultiCol LayoutMode = "multi-col"
)

// IsValid reports whether m is a recognized LayoutMode. Used when loading
// config to fall back to the default if an unknown string slipped in via a
// hand-edited TOML file (consistent with Theme / NotificationStyle /
// MascotPersona validation).
func (m LayoutMode) IsValid() bool {
	switch m {
	case LayoutStack, LayoutTabs, LayoutMultiCol:
		return true
	}
	return false
}

// NotificationStyle picks the visual treatment for hook prompts and the
// compaction-imminent warning.
type NotificationStyle string

// Notification style constants.
//
//	NotificationFullscreen — animated centered overlay (default). Covers
//	                          the panel grid until dismissed; pulses to
//	                          grab attention for hook prompts.
//	NotificationBanner     — compact 3-row banner above the status bar
//	                          (the V22 behavior, kept for users who find
//	                          fullscreen too aggressive).
//	NotificationOff        — no UI surfacing. Hook events still block on
//	                          the server side; the user must rely on the
//	                          claude CLI's own prompt or use y/n hotkeys.
const (
	NotificationFullscreen NotificationStyle = "fullscreen"
	NotificationBanner     NotificationStyle = "banner"
	NotificationOff        NotificationStyle = "off"
)

// IsValid reports whether s is a recognized NotificationStyle. Used when
// loading config to fall back to the default if an unknown string slipped
// in via a hand-edited TOML file.
func (s NotificationStyle) IsValid() bool {
	switch s {
	case NotificationFullscreen, NotificationBanner, NotificationOff:
		return true
	}
	return false
}

// Next returns the next style in the cycle order: fullscreen → banner →
// off → fullscreen. Drives the Enter handler on the settings overlay.
func (s NotificationStyle) Next() NotificationStyle {
	switch s {
	case NotificationFullscreen:
		return NotificationBanner
	case NotificationBanner:
		return NotificationOff
	default:
		return NotificationFullscreen
	}
}

// Display returns a short human-readable label used in the settings chip.
func (s NotificationStyle) Display() string {
	switch s {
	case NotificationFullscreen:
		return "Fullscreen"
	case NotificationBanner:
		return "Banner"
	case NotificationOff:
		return "Off"
	}
	return "Fullscreen"
}

// MascotPersona picks which character drives the now-panel mascot.
//
// Two named characters ride the same FSM:
//
//	Meowl — the v23 cat. Default. /\_/\ ears, ( o.o ) face.
//	Bowl  — the legacy v9 rabbit. (\_/) ears, (o.o) face.
//
// "Off" hides the mascot block entirely. The TOML serialization uses the
// new names ("meowl" / "bowl"); the legacy "kitten" / "bunny" values are
// accepted on read via Normalize() so a user upgrading from an earlier
// dev build doesn't see their persona reset to default.
type MascotPersona string

// MascotPersona constants.
const (
	MascotPersonaMeowl MascotPersona = "meowl"
	MascotPersonaBowl  MascotPersona = "bowl"
	MascotPersonaOff   MascotPersona = "off"

	// Legacy aliases — accepted on read, normalized to the new values.
	mascotPersonaLegacyKitten MascotPersona = "kitten"
	mascotPersonaLegacyBunny  MascotPersona = "bunny"
)

// Normalize collapses legacy persona names into their new equivalents.
// Called from LoadConfig so files written before the v23 rename keep
// working with the same character.
func (p MascotPersona) Normalize() MascotPersona {
	switch p {
	case mascotPersonaLegacyKitten:
		return MascotPersonaMeowl
	case mascotPersonaLegacyBunny:
		return MascotPersonaBowl
	}
	return p
}

// IsValid reports whether p is a recognized MascotPersona — including
// legacy aliases, since the LoadConfig path treats those as valid input
// before normalizing.
func (p MascotPersona) IsValid() bool {
	switch p.Normalize() {
	case MascotPersonaMeowl, MascotPersonaBowl, MascotPersonaOff:
		return true
	}
	return false
}

// Next returns the next persona in the cycle: meowl → bowl → off → meowl.
func (p MascotPersona) Next() MascotPersona {
	switch p.Normalize() {
	case MascotPersonaMeowl:
		return MascotPersonaBowl
	case MascotPersonaBowl:
		return MascotPersonaOff
	default:
		return MascotPersonaMeowl
	}
}

// Display returns a short human-readable label used in the settings chip.
func (p MascotPersona) Display() string {
	switch p.Normalize() {
	case MascotPersonaMeowl:
		return "Meowl"
	case MascotPersonaBowl:
		return "Bowl"
	case MascotPersonaOff:
		return "Off"
	}
	return "Meowl"
}

// PanelConfig holds per-panel display settings.
//
// Height is the user's persisted manual resize for this panel. When
// RememberLayout is enabled at the Config level, +/- resize and the
// space-toggle write back to Height + DefaultCollapsed so the next
// session restores the same layout. When RememberLayout is off, Height
// is ignored and the panel falls back to its computed default.
type PanelConfig struct {
	Enabled          bool `toml:"enabled"`
	DefaultCollapsed bool `toml:"default_collapsed"`
	Position         int  `toml:"position"`
	Height           int  `toml:"height,omitempty"`
}

// LayoutConfig holds the layout section of the config.
type LayoutConfig struct {
	DefaultMode         LayoutMode `toml:"default_mode"`
	AutoSwitchThreshold int        `toml:"auto_switch_threshold"`
	CycleHotkey         string     `toml:"cycle_hotkey"`
}

// PanelsConfig holds all panel configs.
type PanelsConfig struct {
	Now      PanelConfig `toml:"now"`
	Calls    PanelConfig `toml:"calls"` // v13: replaces tasks
	Tasks    PanelConfig `toml:"tasks"` // legacy alias — copied onto Calls in LoadConfig when [panels.calls] is absent
	Diff     PanelConfig `toml:"diff"`
	Usage    PanelConfig `toml:"usage"`
	Explorer PanelConfig `toml:"explorer"`
	Servers  PanelConfig `toml:"servers"`
	Bash     PanelConfig `toml:"bash"`  // v22+: session-wide bash command ledger
	Cache    PanelConfig `toml:"cache"` // v22+: prompt-cache efficiency dashboard
}

// ProjectOverride is a per-project settings layer applied on top of the
// global config when the user is launched in that working directory.
//
// Panels carries per-panel visibility overrides (enabled flag).
// PanelLayouts carries per-panel layout overrides (manual Height and
// DefaultCollapsed) so RememberLayout-driven persistence is scoped to
// the project the user actually resized panels in.
type ProjectOverride struct {
	Panels       map[string]bool        `toml:"panels,omitempty"`
	PanelLayouts map[string]PanelLayout `toml:"panel_layouts,omitempty"`
}

// PanelLayout is the per-project subset of PanelConfig that participates
// in RememberLayout persistence. Height==0 means "no manual override";
// DefaultCollapsed defaults to false and is omitted from the TOML when
// it matches that zero value.
type PanelLayout struct {
	Height           int  `toml:"height,omitempty"`
	DefaultCollapsed bool `toml:"default_collapsed,omitempty"`
}

// Config is the root config struct for the clyde TUI.
//
// Resolution order at runtime is:
//  1. Built-in defaults (DefaultConfig).
//  2. Global user file (~/.config/clyde/config.toml).
//  3. Per-project override under [projects."<absolute cwd>"] (V22+).
//
// The global file may also contain a [projects."<cwd>"] section; the project
// layer is applied last via EffectiveFor.
type Config struct {
	Layout   LayoutConfig               `toml:"layout"`
	Panels   PanelsConfig               `toml:"panels"`
	Projects map[string]ProjectOverride `toml:"projects,omitempty"`

	// AutoSwitchToAllOnNewSession controls what happens when a brand-new
	// claude code session appears in the same cwd while clyde is running.
	// When true (default), clyde flips the focused tab to Σ all so the
	// user sees both sessions at a glance and can pick. When false, the
	// user's currently-focused tab is preserved — useful if they're
	// concentrating on one session and don't want their cursor stolen.
	AutoSwitchToAllOnNewSession bool `toml:"auto_switch_to_all_on_new_session"`

	// RememberLayout controls whether runtime panel-layout changes
	// (collapse toggles via space, manual height resize via +/-) are
	// written back to the config so the next session restores the same
	// layout. When false, runtime changes stay in-session and panels
	// fall back to PanelConfig.DefaultCollapsed on next launch.
	RememberLayout bool `toml:"remember_layout"`

	// NotificationStyle chooses the visual treatment for live hook
	// prompts and compaction warnings. Defaults to fullscreen so the
	// signal is impossible to miss; the user can downgrade to banner
	// or disable entirely from the settings overlay.
	NotificationStyle NotificationStyle `toml:"notification_style"`

	// NotifyCostThresholdUSD fires a quota notification when the
	// current session's accumulated cost crosses this dollar amount.
	// Zero (default) disables cost-based alerts — plan-quota %
	// notifications still fire on their own thresholds.
	NotifyCostThresholdUSD float64 `toml:"notify_cost_threshold_usd"`

	// Theme picks the active color palette. See theme.go for the registry
	// of supported themes; defaults to ThemeTokyoNight. Cycled live from
	// the settings overlay; the choice persists in config.toml so the
	// next launch starts in the same theme.
	Theme Theme `toml:"theme"`

	// MascotPersona picks which character drives the now-panel mascot. The
	// default ("meowl") is the cat shipped in v23+; "bowl" reverts to the
	// v9–v22 rabbit; "off" hides the mascot block entirely for users who'd
	// rather see the now panel as a flat status line. The legacy "kitten" /
	// "bunny" values are still accepted on read and normalized to
	// "meowl" / "bowl" (see MascotPersona.Normalize).
	MascotPersona MascotPersona `toml:"mascot_persona"`

	// BootScreenEnabled toggles the animated splash screen shown for
	// ~1.5s on launch. Enabled by default; users who launch clyde a lot
	// in CI scripts or back-to-back can flip it off from the settings
	// overlay.
	BootScreenEnabled bool `toml:"boot_screen_enabled"`
}

// DefaultConfig returns hardcoded defaults matching the spec.
//
// V22 defaults: Diff is OFF (the standalone diff panel is opt-in; hunks
// surface inline under Edit calls in the activity panel). Every other panel
// is on by default.
func DefaultConfig() Config {
	return Config{
		Layout: LayoutConfig{
			DefaultMode:         LayoutStack,
			AutoSwitchThreshold: 80,
			CycleHotkey:         "ctrl+l",
		},
		Panels: PanelsConfig{
			Now:      PanelConfig{Enabled: true, DefaultCollapsed: false, Position: 1},
			Calls:    PanelConfig{Enabled: true, DefaultCollapsed: true, Position: 2},
			Tasks:    PanelConfig{Enabled: true, DefaultCollapsed: true, Position: 2}, // alias for calls
			Diff:     PanelConfig{Enabled: false, DefaultCollapsed: true, Position: 3},
			Usage:    PanelConfig{Enabled: true, DefaultCollapsed: true, Position: 4},
			Explorer: PanelConfig{Enabled: true, DefaultCollapsed: true, Position: 5},
			Servers:  PanelConfig{Enabled: true, DefaultCollapsed: true, Position: 6},
			Bash:     PanelConfig{Enabled: false, DefaultCollapsed: true, Position: 7},
			Cache:    PanelConfig{Enabled: false, DefaultCollapsed: true, Position: 8},
		},
		AutoSwitchToAllOnNewSession: true,
		NotificationStyle:           NotificationFullscreen,
		NotifyCostThresholdUSD:      20.0,
		Theme:                       ThemeTokyoNight,
		MascotPersona:               MascotPersonaMeowl,
		BootScreenEnabled:           true,
	}
}

// costThresholdSteps is the cycle order for the cost-threshold chip in
// the settings overlay. 0 means "off"; the rest are tasteful preset
// dollar amounts covering API-key user habits from a quick-experiment
// session ($5) to a full deep-research budget ($500). A user with a
// non-step value in their TOML (say $30) still works — Next() picks
// the smallest step strictly greater, then wraps to 0 after $500.
var costThresholdSteps = []float64{0, 5, 10, 20, 50, 100, 200, 500}

// nextCostThreshold returns the next preset above current. After the
// largest preset it wraps back to 0 (off). Off → 1 starts the cycle.
func nextCostThreshold(current float64) float64 {
	for _, v := range costThresholdSteps {
		if v > current {
			return v
		}
	}
	return 0
}

// formatCostThreshold renders the chip label for a cost threshold:
// 0 reads as "off", positive values as "$N" without trailing zeros.
func formatCostThreshold(v float64) string {
	if v <= 0 {
		return "off"
	}
	if v == float64(int(v)) {
		return fmt.Sprintf("$%d", int(v))
	}
	return fmt.Sprintf("$%.2f", v)
}

// LoadConfig reads the TOML config at ~/.config/clyde/config.toml and
// merges it over the defaults. If the file doesn't exist, defaults are returned.
func LoadConfig() Config {
	cfg := DefaultConfig()
	home, err := os.UserHomeDir()
	if err != nil {
		return cfg
	}
	path := home + "/.config/clyde/config.toml"
	// G304: path is built from the current user's home directory + a hard-
	// coded suffix. The user's own config file is what we're trying to load.
	data, err := os.ReadFile(path) //nolint:gosec // see comment
	if err != nil {
		return cfg
	}
	// Merge on top of defaults — TOML only overwrites keys that are present.
	md, err := toml.Decode(string(data), &cfg)
	if err != nil {
		return cfg
	}
	// Legacy [panels.tasks] → calls: v13 renamed the panel, but hand-written
	// configs from before the rename only carry [panels.tasks]. Honor it by
	// copying onto the calls slot when the user did NOT also set the modern
	// [panels.calls] (which always wins). Files that clyde itself wrote carry
	// both sections, so this only fires for genuine legacy input.
	if md.IsDefined("panels", "tasks") && !md.IsDefined("panels", "calls") {
		cfg.Panels.Calls = cfg.Panels.Tasks
	}
	if !cfg.Layout.DefaultMode.IsValid() {
		cfg.Layout.DefaultMode = LayoutStack
	}
	if !cfg.NotificationStyle.IsValid() {
		cfg.NotificationStyle = NotificationFullscreen
	}
	if !cfg.Theme.IsValid() {
		cfg.Theme = ThemeTokyoNight
	}
	if !cfg.MascotPersona.IsValid() {
		cfg.MascotPersona = MascotPersonaMeowl
	}
	// Normalize legacy "kitten" / "bunny" values written by older dev
	// builds so the user's choice carries forward across the rename.
	cfg.MascotPersona = cfg.MascotPersona.Normalize()
	return cfg
}

// NormalizeCwd resolves a path to its canonical absolute form, expanding
// symlinks where possible. Used as the per-project lookup key so symlinked
// or relative paths still match their config section.
func NormalizeCwd(cwd string) string {
	if cwd == "" {
		return ""
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return cwd
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return abs
	}
	return resolved
}

// EffectiveFor returns a copy of the config with the per-project override
// for cwd applied on top of the global panel settings. cwd is normalized
// before lookup so symlinked or relative paths resolve to the same key.
func (cfg Config) EffectiveFor(cwd string) Config {
	if cwd == "" || len(cfg.Projects) == 0 {
		return cfg
	}
	key := NormalizeCwd(cwd)
	override, ok := cfg.Projects[key]
	if !ok {
		// Try the unnormalized form too — users editing the file by hand
		// often paste their plain $HOME path without realizing it could
		// differ from the resolved one.
		override, ok = cfg.Projects[cwd]
	}
	if !ok {
		return cfg
	}
	out := cfg
	out.Panels = applyPanelOverride(out.Panels, override.Panels)
	out.Panels = applyPanelLayoutOverride(out.Panels, override.PanelLayouts)
	return out
}

// applyPanelOverride returns a copy of base with Enabled fields replaced
// according to the per-panel keys in over (any subset; missing keys keep
// their global value).
func applyPanelOverride(base PanelsConfig, over map[string]bool) PanelsConfig {
	for name, enabled := range over {
		if pc := panelSlotByName(&base, name); pc != nil {
			pc.Enabled = enabled
		}
	}
	return base
}

// applyPanelLayoutOverride returns a copy of base with Height and
// DefaultCollapsed replaced for each panel listed in over. Missing keys
// keep their global value. Height==0 in an override is treated as "clear
// the manual override" and falls back to the base height.
func applyPanelLayoutOverride(base PanelsConfig, over map[string]PanelLayout) PanelsConfig {
	for name, layout := range over {
		pc := panelSlotByName(&base, name)
		if pc == nil {
			continue
		}
		pc.DefaultCollapsed = layout.DefaultCollapsed
		if layout.Height > 0 {
			pc.Height = layout.Height
		}
	}
	return base
}

// panelSlotByName returns a pointer to the PanelConfig slot in cfg
// matching the TOML panel name, or nil if the name is unknown.
// "tasks" is an alias for "calls" so legacy configs keep working.
func panelSlotByName(cfg *PanelsConfig, name string) *PanelConfig {
	switch name {
	case "now":
		return &cfg.Now
	case "calls", "tasks":
		return &cfg.Calls
	case "diff":
		return &cfg.Diff
	case "usage":
		return &cfg.Usage
	case "explorer":
		return &cfg.Explorer
	case "servers":
		return &cfg.Servers
	case "bash":
		return &cfg.Bash
	case "cache":
		return &cfg.Cache
	}
	return nil
}

// ResolveMode returns the effective LayoutMode for a given terminal width,
// respecting the auto-switch threshold and the user's preferred mode.
// Multi-col requires at least 160 cols; below that it falls back to tabs.
func ResolveMode(preferred LayoutMode, threshold, width int) LayoutMode {
	if width < threshold {
		return LayoutStack
	}
	if preferred == LayoutMultiCol && width < 160 {
		return LayoutTabs
	}
	return preferred
}
