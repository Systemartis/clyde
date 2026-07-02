package tui

import (
	"charm.land/bubbles/v2/key"
)

// KeyMap is the central keybinding registry for clyde. It documents the
// bindings the dashboard actually dispatches — every entry here must have a
// live handler (handleKey/handleCtrl/handlePanelJumpKey); dead declarations
// mislead the help surfaces.
type KeyMap struct {
	Quit      key.Binding
	Tab       key.Binding
	ShiftTab  key.Binding
	Space     key.Binding
	Up        key.Binding
	Down      key.Binding
	Left      key.Binding
	Right     key.Binding
	Enter     key.Binding
	Backspace key.Binding
	Esc       key.Binding

	// ⌃-prefixed chords dispatched by handleCtrl.
	CycleMode     key.Binding // ⌃l — cycle layout mode
	CollapseAll   key.Binding // ⌃0 — collapse every panel except the focused one
	FocusExplorer key.Binding // ⌃e — focus explorer
	FocusCalls    key.Binding // ⌃a — focus activity/calls
	FocusDiff     key.Binding // ⌃d — focus diff

	// Resize bindings — grow/shrink the focused expanded panel
	PanelGrow   key.Binding // + or = → increase panel height by 1
	PanelShrink key.Binding // - → decrease panel height by 1

	// Session cycling across the footer session-tab strip ([ / ]).
	SessionPrev key.Binding // [ — previous session tab
	SessionNext key.Binding // ] — next session tab

	// PanelJump documents the plain-letter panel jumps dispatched by
	// handlePanelJumpKey (e/a/d/u/s/b/c).
	PanelJump key.Binding

	// Overlays.
	Help     key.Binding // h — per-panel help cheat-sheet
	Settings key.Binding // ? — settings overlay (also cycles layout)

	// Numeric tab jumps for tabs mode (1..N).
	Jump1 key.Binding
	Jump2 key.Binding
	Jump3 key.Binding
	Jump4 key.Binding
}

// DefaultKeyMap returns the default key bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next panel"),
		),
		ShiftTab: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "prev panel"),
		),
		Space: key.NewBinding(
			key.WithKeys(" "),
			key.WithHelp("space", "toggle collapse"),
		),
		Up: key.NewBinding(
			key.WithKeys("up"),
			key.WithHelp("↑", "focus up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down"),
			key.WithHelp("↓", "focus down"),
		),
		Left: key.NewBinding(
			key.WithKeys("left"),
			key.WithHelp("←", "focus left / prev tab"),
		),
		Right: key.NewBinding(
			key.WithKeys("right"),
			key.WithHelp("→", "focus right / next tab"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("↵", "expand / primary action"),
		),
		Backspace: key.NewBinding(
			key.WithKeys("backspace"),
			key.WithHelp("⌫", "collapse panel"),
		),
		Esc: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "dismiss notification"),
		),
		CycleMode: key.NewBinding(
			key.WithKeys("ctrl+l"),
			key.WithHelp("⌃l", "cycle layout"),
		),
		CollapseAll: key.NewBinding(
			key.WithKeys("ctrl+0"),
			key.WithHelp("⌃0", "collapse others"),
		),
		FocusExplorer: key.NewBinding(
			key.WithKeys("ctrl+e"),
			key.WithHelp("⌃e", "explorer"),
		),
		FocusCalls: key.NewBinding(
			key.WithKeys("ctrl+a"),
			key.WithHelp("⌃a", "calls"),
		),
		FocusDiff: key.NewBinding(
			key.WithKeys("ctrl+d"),
			key.WithHelp("⌃d", "diff"),
		),
		PanelGrow: key.NewBinding(
			key.WithKeys("+", "="),
			key.WithHelp("+", "grow panel"),
		),
		PanelShrink: key.NewBinding(
			key.WithKeys("-"),
			key.WithHelp("-", "shrink panel"),
		),
		SessionPrev: key.NewBinding(
			key.WithKeys("["),
			key.WithHelp("[", "prev session"),
		),
		SessionNext: key.NewBinding(
			key.WithKeys("]"),
			key.WithHelp("]", "next session"),
		),
		PanelJump: key.NewBinding(
			key.WithKeys("e", "a", "d", "u", "s", "b", "c"),
			key.WithHelp("e a d u s b c", "jump to panel"),
		),
		Help: key.NewBinding(
			key.WithKeys("h"),
			key.WithHelp("h", "help"),
		),
		Settings: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "settings"),
		),
		Jump1: key.NewBinding(key.WithKeys("1")),
		Jump2: key.NewBinding(key.WithKeys("2")),
		Jump3: key.NewBinding(key.WithKeys("3")),
		Jump4: key.NewBinding(key.WithKeys("4")),
	}
}

// ShortHelp implements help.KeyMap for the bubbles/v2/help widget. Lists
// only keys that are actually dispatched today.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Tab, k.Up, k.Enter, k.Space, k.Help, k.Quit}
}

// FullHelp implements help.KeyMap.
// Explorer note: when explorer is focused+expanded, ↑/↓ moves tree highlight
// instead of scrolling the viewport (special-cased in handleExplorerKey).
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Tab, k.ShiftTab, k.Up, k.Down, k.Left, k.Right},
		{k.Enter, k.Backspace, k.Space, k.CollapseAll, k.Esc},
		{k.PanelGrow, k.PanelShrink, k.SessionPrev, k.SessionNext},
		{k.PanelJump, k.FocusExplorer, k.FocusCalls, k.FocusDiff},
		{k.CycleMode, k.Help, k.Settings, k.Quit},
	}
}
