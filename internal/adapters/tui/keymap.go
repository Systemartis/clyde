package tui

import (
	"charm.land/bubbles/v2/key"
)

// KeyMap is the central keybinding registry for the clyde prototype.
// Driven by bubbles/v2/key so the help widget gets bindings for free.
type KeyMap struct {
	Quit          key.Binding
	Tab           key.Binding
	ShiftTab      key.Binding
	Space         key.Binding
	Up            key.Binding
	Down          key.Binding
	Left          key.Binding
	Right         key.Binding
	Enter         key.Binding
	Backspace     key.Binding
	Esc           key.Binding
	CollapseAll   key.Binding
	CycleMode     key.Binding
	FocusExplorer key.Binding
	FocusCalls    key.Binding // ⌃a — activity/calls panel (replaces ⌃t tasks in v13)
	FocusDiff     key.Binding
	// Resize bindings — grow/shrink the focused expanded panel
	PanelGrow   key.Binding // + or = → increase panel height by 1
	PanelShrink key.Binding // - → decrease panel height by 1
	// Explorer bindings
	ExplorerFilter key.Binding // / — filter (v7 placeholder)

	// Numeric panel jumps for Tab mode (1-9)
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
		CollapseAll: key.NewBinding(
			key.WithKeys("ctrl+0"),
			key.WithHelp("⌃0", "collapse others"),
		),
		CycleMode: key.NewBinding(
			key.WithKeys("ctrl+l"),
			key.WithHelp("⌃l", "cycle layout"),
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
		ExplorerFilter: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "filter (v7)"),
		),
		Jump1: key.NewBinding(key.WithKeys("1")),
		Jump2: key.NewBinding(key.WithKeys("2")),
		Jump3: key.NewBinding(key.WithKeys("3")),
		Jump4: key.NewBinding(key.WithKeys("4")),
	}
}

// ShortHelp implements help.KeyMap for the bubbles/v2/help widget.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Tab, k.Up, k.Enter, k.Space, k.CycleMode, k.Quit}
}

// FullHelp implements help.KeyMap.
// Explorer note: when explorer is focused+expanded, ↑/↓ moves tree highlight
// instead of scrolling the viewport (special-cased in handleExplorerKey).
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Tab, k.ShiftTab, k.Up, k.Down, k.Left, k.Right},
		{k.Enter, k.Backspace, k.Space, k.CollapseAll, k.Esc},
		{k.PanelGrow, k.PanelShrink},
		{k.FocusExplorer, k.FocusCalls, k.FocusDiff},
		{k.CycleMode, k.Quit},
	}
}
