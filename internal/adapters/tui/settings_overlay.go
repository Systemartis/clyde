package tui

import (
	"bytes"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/BurntSushi/toml"
)

// SettingsScope chooses which layer of the config the overlay edits.
type SettingsScope int

const (
	// ScopeProject — edits go to baseCfg.Projects["<cwd>"].Panels (default
	// when a cwd is known). Effective for ONLY this project.
	ScopeProject SettingsScope = iota
	// ScopeGlobal — edits go to baseCfg.Panels (the top-level [panels]).
	// Used when the user wants a setting to apply to every project that
	// does not explicitly override it.
	ScopeGlobal
)

// SettingsPanelToggle holds the per-panel toggle state for the settings overlay.
//
// Enabled mirrors the visibility flag (with project-scope override semantics).
// StartsCollapsed mirrors the global DefaultCollapsed flag — it is always
// edited at the global layer regardless of the current scope, since we don't
// support per-project default-collapsed overrides.
type SettingsPanelToggle struct {
	PanelID         PanelID
	Label           string
	Enabled         bool
	StartsCollapsed bool
	IsOverride      bool // true when the project layer differs from the global layer
}

// settingsOverlayFromConfig builds the toggle slice for the given scope.
//
// In project scope, each toggle reflects the EFFECTIVE value (project
// override OR global fallback) and IsOverride flags rows that diverge from
// the global layer. In global scope, each toggle reflects the raw global
// value with IsOverride always false.
func settingsOverlayFromConfig(base Config, scope SettingsScope, cwd string) []SettingsPanelToggle {
	src := base
	if scope == ScopeProject {
		src = base.EffectiveFor(cwd)
	}
	rows := []struct {
		id        PanelID
		label     string
		val       bool
		base      bool
		collapsed bool
	}{
		{PanelNow, "now panel", src.Panels.Now.Enabled, base.Panels.Now.Enabled, base.Panels.Now.DefaultCollapsed},
		{PanelCalls, "activity panel", src.Panels.Calls.Enabled, base.Panels.Calls.Enabled, base.Panels.Calls.DefaultCollapsed},
		{PanelDiff, "diff panel", src.Panels.Diff.Enabled, base.Panels.Diff.Enabled, base.Panels.Diff.DefaultCollapsed},
		{PanelUsage, "usage panel", src.Panels.Usage.Enabled, base.Panels.Usage.Enabled, base.Panels.Usage.DefaultCollapsed},
		{PanelExplorer, "explorer panel", src.Panels.Explorer.Enabled, base.Panels.Explorer.Enabled, base.Panels.Explorer.DefaultCollapsed},
		{PanelServers, "servers panel", src.Panels.Servers.Enabled, base.Panels.Servers.Enabled, base.Panels.Servers.DefaultCollapsed},
		{PanelBash, "bash audit panel", src.Panels.Bash.Enabled, base.Panels.Bash.Enabled, base.Panels.Bash.DefaultCollapsed},
		{PanelCache, "cache efficiency panel", src.Panels.Cache.Enabled, base.Panels.Cache.Enabled, base.Panels.Cache.DefaultCollapsed},
	}
	out := make([]SettingsPanelToggle, len(rows))
	for i, r := range rows {
		out[i] = SettingsPanelToggle{
			PanelID:         r.id,
			Label:           r.label,
			Enabled:         r.val,
			StartsCollapsed: r.collapsed,
			IsOverride:      scope == ScopeProject && cwd != "" && r.val != r.base,
		}
	}
	return out
}

// renderSettingsOverlay renders the settings modal dialog centered on the screen.
//
// Layout (V22+):
//
//	╭ settings · esc close ─────────────────╮
//	│ scope: [Global] [Project ▸]    tab swap│
//	│ ↑/↓ navigate · enter toggle            │
//	│                                        │
//	│ [✓] now panel                          │
//	│ [✓] activity panel                     │
//	│ [ ] diff panel              (override) │
//	│ [✓] usage panel                        │
//	│ [✓] explorer panel                     │
//	│ [✓] servers panel                      │
//	╰────────────────────────────────────────╯
//
// settingsViewParams bundles the inputs renderSettingsOverlay needs.
type settingsViewParams struct {
	toggles         []SettingsPanelToggle
	cursor          int
	scope           SettingsScope
	hasProjectScope bool
	cwd             string
	// layoutMode is the currently-active layout mode. Shown in the
	// Layout section of the settings overlay; cycled by Enter when
	// cursor == settingsLayoutCursor.
	layoutMode LayoutMode
	// rememberLayout mirrors Config.RememberLayout. Toggled by Enter
	// when cursor == settingsRememberLayoutCursor.
	rememberLayout bool
	// notificationStyle mirrors Config.NotificationStyle. Cycled by
	// Enter when cursor == settingsNotificationCursor.
	notificationStyle NotificationStyle
	// costThreshold mirrors Config.NotifyCostThresholdUSD. Cycled by
	// Enter when cursor == settingsCostThresholdCursor.
	costThreshold float64
	// theme mirrors Config.Theme. Cycled by Enter when cursor ==
	// settingsThemeCursor; the overlay applies the new palette live.
	theme Theme
	// mascotPersona mirrors Config.MascotPersona.
	mascotPersona MascotPersona
	// bootScreenEnabled mirrors Config.BootScreenEnabled.
	bootScreenEnabled bool
}

// Sentinel cursor values for non-toggle rows in the settings overlay.
//
// Cursor space layout (top-down navigation, MUST match render order):
//
//	settingsThemeCursor          (-7) → Appearance · theme chip
//	settingsMascotPersonaCursor  (-6) → Appearance · mascot persona
//	settingsBootScreenCursor     (-5) → Appearance · boot splash
//	settingsNotificationCursor   (-4) → Notifications · style chip
//	settingsCostThresholdCursor  (-3) → Notifications · cost threshold
//	settingsRememberLayoutCursor (-2) → Behavior · remember layout
//	settingsLayoutCursor         (-1) → Layout · mode chip
//	0..N-1                            → Panels · enabled toggle
//	N..N+M-1                          → Startup · starts-collapsed (filtered)
const (
	settingsLayoutCursor         = -1
	settingsRememberLayoutCursor = -2
	settingsCostThresholdCursor  = -3
	settingsNotificationCursor   = -4
	settingsBootScreenCursor     = -5
	settingsMascotPersonaCursor  = -6
	settingsThemeCursor          = -7
)

// settingsCursorMin returns the smallest valid cursor value (the topmost
// row of the overlay).
func settingsCursorMin() int { return settingsThemeCursor }

// startupCapableToggles returns the indices into toggles whose panel
// supports a "starts collapsed" preference. Now and servers are always
// visible by design — surfacing a starts-collapsed knob for them is
// misleading, and RememberLayout already persists their actual state.
//
// Keeping this as an index slice (instead of filtering the full toggle
// list) lets the Startup-section cursor space stay contiguous while
// still mapping back to a stable position in toggles for the Enter
// handler.
func startupCapableToggles(toggles []SettingsPanelToggle) []int {
	idxs := make([]int, 0, len(toggles))
	for i, t := range toggles {
		switch t.PanelID {
		case PanelNow, PanelServers:
			continue
		}
		idxs = append(idxs, i)
	}
	return idxs
}

// startupCursorOffset returns the cursor index for the k-th VISIBLE
// startup row (zero-based, indexed into the startupCapableToggles slice).
// Keeping the math in one place lets render and keys.go agree on the
// index space without re-deriving N each time.
func startupCursorOffset(toggles []SettingsPanelToggle, k int) int {
	return len(toggles) + k
}

// startupCursorMax returns the highest valid cursor value for the given
// scope. Project scope hides the Startup section so navigation caps at
// the last panel-visibility row.
func startupCursorMax(toggles []SettingsPanelToggle) int {
	starts := startupCapableToggles(toggles)
	if len(starts) == 0 {
		if len(toggles) == 0 {
			return settingsLayoutCursor
		}
		return len(toggles) - 1
	}
	return startupCursorOffset(toggles, len(starts)-1)
}

// settingsCursorMaxForScope returns the highest valid cursor in the
// given scope. Project scope hides the Startup section, so the cursor
// must clamp at the last panel row to prevent landing on an invisible
// position.
func settingsCursorMaxForScope(scope SettingsScope, toggles []SettingsPanelToggle) int {
	if scope == ScopeProject {
		if len(toggles) == 0 {
			return settingsLayoutCursor
		}
		return len(toggles) - 1
	}
	return startupCursorMax(toggles)
}

// renderSettingsOverlay renders a full-width app-style settings card with
// sections (Layout, Panels, About) and clear visual hierarchy. Returned
// content is a newline-separated block sized at width × height; the
// caller drops it between the title and status bars to produce the full
// settings page.
func renderSettingsOverlay(s Styles, p Palette, params settingsViewParams, width, height int) string {
	if width < 40 {
		width = 40
	}
	if height < 12 {
		height = 12
	}
	inner := width - 2    // border on each side
	contentW := inner - 4 // 2-cell padding on each side

	border := lipgloss.NewStyle().Foreground(p.BorderAcc)
	titleStyle := lipgloss.NewStyle().Foreground(p.Purple).Bold(true)
	dim := lipgloss.NewStyle().Foreground(p.TextDim)
	fade := lipgloss.NewStyle().Foreground(p.TextFade)
	sectionStyle := lipgloss.NewStyle().Foreground(p.Magenta).Bold(true)
	enabled := lipgloss.NewStyle().Foreground(p.Green)
	disabled := lipgloss.NewStyle().Foreground(p.TextFade)
	cursorMark := lipgloss.NewStyle().Foreground(p.Magenta).Bold(true)
	nameStyle := lipgloss.NewStyle().Foreground(p.TextMid)
	nameFocus := lipgloss.NewStyle().Foreground(p.Text).Bold(true)
	chipActive := lipgloss.NewStyle().Foreground(p.Magenta).Bold(true)
	chipIdle := lipgloss.NewStyle().Foreground(p.TextDim)

	body := make([]string, 0, height)

	// Top border with title.
	topLabel := " " + titleStyle.Render("settings") + dim.Render(" · esc close ")
	topFill := strings.Repeat("─", max0(inner-ansiWidth(topLabel)))
	body = append(body, border.Render("╭")+topLabel+border.Render(topFill)+border.Render("╮"))

	// Helper: emit a single content row inside the card, padded to inner width.
	row := func(content string) string {
		return border.Render("│") + padLine("  "+content+"  ", inner) + border.Render("│")
	}
	blank := border.Render("│") + strings.Repeat(" ", inner) + border.Render("│")

	body = append(body, blank)

	// — Scope row —
	var scopeRow string
	if params.hasProjectScope {
		var globalChip, projChip string
		if params.scope == ScopeGlobal {
			globalChip = chipActive.Render("[Global ▸]")
			projChip = chipIdle.Render("[Project]")
		} else {
			globalChip = chipIdle.Render("[Global]")
			projChip = chipActive.Render("[Project ▸]")
		}
		scopeRow = "scope:  " + globalChip + "  " + projChip + dim.Render("       tab swap")
	} else {
		scopeRow = "scope:  " + chipActive.Render("[Global ▸]") + dim.Render("       (project scope: cwd unknown)")
	}
	body = append(body, row(scopeRow))

	if params.scope == ScopeProject && params.cwd != "" {
		body = append(body, row(dim.Render("project: "+params.cwd)))
	}

	body = append(body, blank)

	// — Section: Appearance —
	// Rendered FIRST so the visual top-down order matches the cursor
	// sentinel sequence (-8 → -7 → -6 → -5 → ...).
	body = append(body, row(sectionDivider(sectionStyle, dim, "Appearance", contentW)))
	body = append(body, blank)
	{
		labelW := chipLabelWidth("theme", "mascot", "boot splash")
		body = append(body, row(chipRow(
			cursorMark, nameFocus, nameStyle, chipActive,
			"theme", labelW,
			"["+params.theme.Display()+" ▸]",
			params.cursor == settingsThemeCursor,
		)))
		body = append(body, row(chipRow(
			cursorMark, nameFocus, nameStyle, chipActive,
			"mascot", labelW,
			"["+params.mascotPersona.Display()+" ▸]",
			params.cursor == settingsMascotPersonaCursor,
		)))
		body = append(body, row(chipRow(
			cursorMark, nameFocus, nameStyle, chipActive,
			"boot splash", labelW,
			boolChip(params.bootScreenEnabled),
			params.cursor == settingsBootScreenCursor,
		)))
	}
	body = append(body, blank)

	// — Section: Notifications —
	body = append(body, row(sectionDivider(sectionStyle, dim, "Notifications", contentW)))
	body = append(body, blank)
	{
		labelW := chipLabelWidth("notification style", "cost alert threshold")
		body = append(body, row(chipRow(
			cursorMark, nameFocus, nameStyle, chipActive,
			"notification style", labelW,
			"["+params.notificationStyle.Display()+" ▸]",
			params.cursor == settingsNotificationCursor,
		)))
		body = append(body, row(chipRow(
			cursorMark, nameFocus, nameStyle, chipActive,
			"cost alert threshold", labelW,
			"["+formatCostThreshold(params.costThreshold)+" ▸]",
			params.cursor == settingsCostThresholdCursor,
		)))
	}
	body = append(body, blank)

	// — Section: Behavior —
	body = append(body, row(sectionDivider(sectionStyle, dim, "Behavior", contentW)))
	body = append(body, blank)
	{
		var name string
		if params.cursor == settingsRememberLayoutCursor {
			name = cursorMark.Render("▸ ") + nameFocus.Render("remember panel layout")
		} else {
			name = "  " + nameStyle.Render("remember panel layout")
		}
		var box string
		if params.rememberLayout {
			box = enabled.Render("[✓]")
		} else {
			box = disabled.Render("[ ]")
		}
		hint := dim.Render("  persists collapse state + heights across sessions")
		body = append(body, row(box+"  "+name+hint))
	}
	body = append(body, blank)

	// — Section: Layout —
	body = append(body, row(sectionDivider(sectionStyle, dim, "Layout", contentW)))
	body = append(body, blank)
	{
		var name string
		if params.cursor == settingsLayoutCursor {
			name = cursorMark.Render("▸ ") + nameFocus.Render("layout mode")
		} else {
			name = "  " + nameStyle.Render("layout mode")
		}
		modeChip := chipActive.Render("[" + string(params.layoutMode) + " ▸]")
		body = append(body, row(name+"   "+modeChip))
	}
	body = append(body, blank)

	// — Section: Panels —
	body = append(body, row(sectionDivider(sectionStyle, dim, "Panels", contentW)))
	body = append(body, blank)
	for i, t := range params.toggles {
		var box string
		if t.Enabled {
			box = enabled.Render("[✓]")
		} else {
			box = disabled.Render("[ ]")
		}
		var name string
		if i == params.cursor {
			name = cursorMark.Render("▸ ") + nameFocus.Render(t.Label)
		} else {
			name = "  " + nameStyle.Render(t.Label)
		}
		hint := ""
		if t.IsOverride {
			hint = "  " + dim.Render("(override)")
		}
		body = append(body, row(box+"  "+name+hint))
	}
	body = append(body, blank)

	// — Section: Startup — global "starts collapsed" preference per panel.
	// Always edits the global layer; project scope only overrides Enabled
	// so the section is hidden there to keep the project view focused on
	// what it actually controls. Now and servers are filtered out because
	// they're always-visible by design, and RememberLayout makes the
	// starts-collapsed knob redundant for everything else anyway — kept
	// only for users who explicitly turn RememberLayout off.
	if params.scope != ScopeProject {
		body = append(body, row(sectionDivider(sectionStyle, dim, "Startup", contentW)))
		body = append(body, blank)
		body = append(body, row(dim.Render("which panels start collapsed")))
		body = append(body, blank)
		for k, idx := range startupCapableToggles(params.toggles) {
			t := params.toggles[idx]
			var box string
			if t.StartsCollapsed {
				box = enabled.Render("[✓]")
			} else {
				box = disabled.Render("[ ]")
			}
			var name string
			if startupCursorOffset(params.toggles, k) == params.cursor {
				name = cursorMark.Render("▸ ") + nameFocus.Render(t.Label+" starts collapsed")
			} else {
				name = "  " + nameStyle.Render(t.Label+" starts collapsed")
			}
			body = append(body, row(box+"  "+name))
		}
		body = append(body, blank)
	}

	// — Section: About —
	body = append(body, row(sectionDivider(sectionStyle, dim, "About", contentW)))
	body = append(body, blank)
	body = append(body, row(dim.Render("clyde · settings live in ~/.config/clyde/config.toml")))
	body = append(body, row(dim.Render("project overrides go under [projects.\"<cwd>\"]")))
	body = append(body, blank)

	// Pad to total height (minus bottom border row).
	for len(body) < height-1 {
		body = append(body, blank)
	}
	if len(body) > height-1 {
		body = body[:height-1]
	}

	// Bottom border with hint.
	hintLabel := " " + dim.Render("↑/↓ navigate · enter toggle ") + fade.Render("· tab scope ")
	botFill := strings.Repeat("─", max0(inner-ansiWidth(hintLabel)))
	body = append(body, border.Render("╰")+border.Render(botFill)+hintLabel+border.Render("╯"))

	return strings.Join(body, "\n")
}

// chipLabelWidth returns the visual column width of the longest label
// in the supplied set. Used to pad sister rows so their value chips
// align in the same column — without this, "notification style" and
// "cost alert threshold" would push their chips to different x
// positions and the section reads jagged.
func chipLabelWidth(labels ...string) int {
	max := 0
	for _, l := range labels {
		if len(l) > max {
			max = len(l)
		}
	}
	return max
}

// boolChip returns the cycling chip text for a boolean toggle that's part
// of the Appearance section (theme/persona/animated/boot). Picks "[On ▸]" /
// "[Off ▸]" so the trailing arrow matches the other chips in the section.
func boolChip(b bool) string {
	if b {
		return "[On ▸]"
	}
	return "[Off ▸]"
}

// chipRow assembles a single chip-style row: cursor caret + label
// (padded to labelW) + chip. Pulled into a helper so adjacent rows
// stay aligned without each call site duplicating the spacing math.
func chipRow(cursorMark, focus, idle, chip lipgloss.Style, label string, labelW int, chipText string, focused bool) string {
	pad := labelW - len(label)
	if pad < 0 {
		pad = 0
	}
	var name string
	if focused {
		name = cursorMark.Render("▸ ") + focus.Render(label)
	} else {
		name = "  " + idle.Render(label)
	}
	return name + strings.Repeat(" ", pad) + "   " + chip.Render(chipText)
}

// sectionDivider renders a section label with a trailing dotted divider that
// fills the remaining width. Used to break long forms into named groups.
func sectionDivider(label, dot lipgloss.Style, name string, contentW int) string {
	rendered := label.Render(name)
	used := ansiWidth(rendered) + 1 // +1 for the space separator
	dashes := contentW - used
	if dashes < 0 {
		dashes = 0
	}
	return rendered + " " + dot.Render(strings.Repeat("·", dashes))
}

// renderSettingsFullScreen returns a full-screen settings page: title bar
// at top, settings card filling the body, status bar at bottom. The card
// is the full terminal width (with a small horizontal margin) and tall
// enough to make the screen feel like a dedicated settings app rather
// than a tiny dialog floating over greyed-out panels.
func renderSettingsFullScreen(s Styles, p Palette, params settingsViewParams, titleBar, statusBar string, totalW, totalH int) string {
	titleH := strings.Count(titleBar, "\n") + 1
	statusH := strings.Count(statusBar, "\n") + 1
	bodyH := totalH - titleH - statusH
	if bodyH < 12 {
		bodyH = 12
	}

	margin := 4
	if totalW < 50 {
		margin = 1
	}
	cardW := totalW - 2*margin
	if cardW < 40 {
		cardW = totalW
		margin = 0
	}

	overlay := renderSettingsOverlay(s, p, params, cardW, bodyH)
	overlayLines := strings.Split(overlay, "\n")

	leftPad := strings.Repeat(" ", margin)
	body := make([]string, 0, bodyH)
	for _, line := range overlayLines {
		body = append(body, leftPad+line)
	}
	for len(body) < bodyH {
		body = append(body, strings.Repeat(" ", totalW))
	}
	if len(body) > bodyH {
		body = body[:bodyH]
	}

	return strings.Join([]string{titleBar, strings.Join(body, "\n"), statusBar}, "\n")
}

// max0 returns max(a, 0).
func max0(a int) int {
	if a < 0 {
		return 0
	}
	return a
}

// applySettings updates baseCfg according to the toggles + scope and returns
// the new baseCfg + the effective cfg (= baseCfg.EffectiveFor(cwd)). Callers
// store both on the model so renders pick up the change immediately.
//
// Enabled flags follow scope semantics (global vs per-project override).
// StartsCollapsed always writes to the global layer regardless of scope —
// per-project default-collapsed overrides are not supported (would require
// extending ProjectOverride from map[string]bool).
func applySettings(base Config, toggles []SettingsPanelToggle, scope SettingsScope, cwd string) (Config, Config) {
	switch scope {
	case ScopeGlobal:
		for _, t := range toggles {
			setGlobalPanelEnabled(&base, t.PanelID, t.Enabled)
		}
	case ScopeProject:
		if cwd != "" {
			setProjectPanelToggles(&base, toggles, cwd)
		}
	}
	for _, t := range toggles {
		setGlobalPanelStartsCollapsed(&base, t.PanelID, t.StartsCollapsed)
	}
	return base, base.EffectiveFor(cwd)
}

// setGlobalPanelStartsCollapsed writes the DefaultCollapsed flag for the
// given panel into the top-level [panels] section. Used by applySettings
// to land the Startup-section toggles regardless of selected scope.
func setGlobalPanelStartsCollapsed(cfg *Config, pid PanelID, collapsed bool) {
	if pc := panelConfigPtr(cfg, pid); pc != nil {
		pc.DefaultCollapsed = collapsed
	}
}

func setGlobalPanelEnabled(cfg *Config, pid PanelID, enabled bool) {
	switch pid {
	case PanelNow:
		cfg.Panels.Now.Enabled = enabled
	case PanelCalls:
		cfg.Panels.Calls.Enabled = enabled
	case PanelDiff:
		cfg.Panels.Diff.Enabled = enabled
	case PanelUsage:
		cfg.Panels.Usage.Enabled = enabled
	case PanelExplorer:
		cfg.Panels.Explorer.Enabled = enabled
	case PanelServers:
		cfg.Panels.Servers.Enabled = enabled
	case PanelBash:
		cfg.Panels.Bash.Enabled = enabled
	case PanelCache:
		cfg.Panels.Cache.Enabled = enabled
	}
}

// setProjectPanelToggles writes the toggle states into the per-project
// override map. Only entries whose effective value differs from the global
// layer are recorded; matching entries are removed from the override so the
// project section stays minimal.
func setProjectPanelToggles(cfg *Config, toggles []SettingsPanelToggle, cwd string) {
	if cfg.Projects == nil {
		cfg.Projects = map[string]ProjectOverride{}
	}
	key := NormalizeCwd(cwd)
	override := cfg.Projects[key]
	if override.Panels == nil {
		override.Panels = map[string]bool{}
	}
	for _, t := range toggles {
		name := panelOverrideKey(t.PanelID)
		if name == "" {
			continue
		}
		globalVal := globalPanelEnabled(*cfg, t.PanelID)
		if t.Enabled == globalVal {
			delete(override.Panels, name)
		} else {
			override.Panels[name] = t.Enabled
		}
	}
	if len(override.Panels) == 0 {
		delete(cfg.Projects, key)
	} else {
		cfg.Projects[key] = override
	}
}

func panelOverrideKey(pid PanelID) string {
	switch pid {
	case PanelNow:
		return "now"
	case PanelCalls:
		return "calls"
	case PanelDiff:
		return "diff"
	case PanelUsage:
		return "usage"
	case PanelExplorer:
		return "explorer"
	case PanelServers:
		return "servers"
	case PanelBash:
		return "bash"
	case PanelCache:
		return "cache"
	}
	return ""
}

func globalPanelEnabled(cfg Config, pid PanelID) bool {
	switch pid {
	case PanelNow:
		return cfg.Panels.Now.Enabled
	case PanelCalls:
		return cfg.Panels.Calls.Enabled
	case PanelDiff:
		return cfg.Panels.Diff.Enabled
	case PanelUsage:
		return cfg.Panels.Usage.Enabled
	case PanelExplorer:
		return cfg.Panels.Explorer.Enabled
	case PanelServers:
		return cfg.Panels.Servers.Enabled
	case PanelBash:
		return cfg.Panels.Bash.Enabled
	case PanelCache:
		return cfg.Panels.Cache.Enabled
	}
	return true
}

// saveSettingsToConfig persists the current panel-visibility toggles to
// ~/.config/clyde/config.toml under the chosen scope.
func saveSettingsToConfig(base Config, toggles []SettingsPanelToggle, scope SettingsScope, cwd string) {
	updated, _ := applySettings(base, toggles, scope, cwd)
	writeConfigFile(updated)
}

func writeConfigFile(cfg Config) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	dir := home + "/.config/clyde"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	path := dir + "/config.toml"

	// Encode to a buffer first so a marshal error doesn't leave a
	// half-written file on disk. Then atomic-replace via tmp + rename
	// so a process crash mid-write can't corrupt the user's config.
	// 0o600 — config has no secrets but no reason for world-readable.
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o600); err != nil {
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return
	}
}
