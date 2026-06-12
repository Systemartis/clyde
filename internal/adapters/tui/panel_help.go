package tui

import (
	"fmt"
	"strings"
)

// helpEntry is one keybinding row shown inside a panel when the help
// overlay is open. Key is the literal glyph(s); Label describes the
// action in plain English.
type helpEntry struct {
	Key   string
	Label string
}

// helpEntriesForPanel returns the keybind cheat-sheet rows shown inside
// a given panel when m.helpOpen is true. Each panel's first row is its
// jump-here shortcut so the user can learn how to come back to that
// panel from anywhere — that single key replaces the older global block
// where every panel-jump key was duplicated on every panel.
//
// Keep entries short — a help block must fit comfortably inside the
// panel's body. Truncation at render time is unforgiving and would hide
// less-frequent bindings on narrow terminals.
func helpEntriesForPanel(pid PanelID) []helpEntry {
	switch pid {
	case PanelExplorer:
		return []helpEntry{
			{"e", "jump here"},
			{"↑ ↓", "move cursor (crosses sections)"},
			{"← →", "jump modified ↔ tree"},
			{"↵", "open file / toggle dir"},
			{"/", "search files in cwd"},
			{"gg / G", "top / bottom of section"},
			{"⌃d / ⌃u", "half-page down / up"},
			{"⌃f / ⌃b", "full-page down / up"},
			{"f", "fullscreen (after open)"},
			{"y", "copy full path"},
			{"Y", "copy basename only"},
			{"click", "select / open"},
			{"dbl-click", "copy full path"},
			{"⌫", "exit active mode"},
		}
	case PanelCalls:
		return []helpEntry{
			{"a", "jump here"},
			{"↑ ↓", "scroll"},
			{"⌫", "exit active mode"},
		}
	case PanelDiff:
		return []helpEntry{
			{"d", "jump here"},
			{"↑ ↓", "scroll lines"},
			{"← →", "scroll horizontal (4 cols)"},
			{"⌫", "exit active mode"},
		}
	case PanelUsage:
		return []helpEntry{
			{"u", "jump here"},
			{"↑ ↓", "scroll"},
			{"⌫", "exit active mode"},
		}
	case PanelServers:
		return []helpEntry{
			{"s", "jump here"},
			{"↑ ↓", "scroll"},
			{"⌫", "exit active mode"},
		}
	case PanelBash:
		return []helpEntry{
			{"b", "jump here"},
			{"↑ ↓", "scroll"},
			{"⌫", "exit active mode"},
		}
	case PanelCache:
		return []helpEntry{
			{"c", "jump here"},
			{"↑ ↓", "scroll"},
			{"⌫", "exit active mode"},
		}
	case PanelNow:
		return []helpEntry{
			{"click", "poke the mascot"},
			{"esc", "dismiss notification"},
			{"y / n", "answer pending hook"},
		}
	}
	return nil
}

// renderHelpBody builds the help cheat-sheet for a panel. inner is the
// usable inner width (panel width minus border + padding).
//
// Only panel-specific entries are shown — the truly global keys (tab,
// ⌃l, ?, h, q) live in the bottom status bar and would just be noise
// repeated on every panel.
func renderHelpBody(s Styles, pid PanelID, inner int) string {
	if inner < 12 {
		inner = 12
	}
	entries := helpEntriesForPanel(pid)
	if len(entries) == 0 {
		return s.HintText.Render("  (no panel-specific commands)")
	}
	var sb strings.Builder
	writeHelpEntries(&sb, s, entries, inner)
	return strings.TrimRight(sb.String(), "\n")
}

func writeHelpEntries(sb *strings.Builder, s Styles, entries []helpEntry, inner int) {
	keyW := 0
	for _, e := range entries {
		if w := visualWidth(e.Key); w > keyW {
			keyW = w
		}
	}
	if keyW > inner/3 {
		keyW = inner / 3
	}
	for _, e := range entries {
		key := s.HintKey.Render(e.Key)
		labelW := inner - keyW - 2
		if labelW < 4 {
			labelW = 4
		}
		label := truncate(e.Label, labelW)
		pad := keyW - visualWidth(e.Key)
		if pad < 0 {
			pad = 0
		}
		sb.WriteString(fmt.Sprintf("%s%s  %s\n", key, strings.Repeat(" ", pad), s.HintText.Render(label)))
	}
}

// visualWidth is a small wrapper for runeWidth that tolerates ANSI-styled
// strings — keys here are plain text so it's a length-of-runes call.
func visualWidth(s string) int {
	return len([]rune(s))
}

// renderPanelHelp wraps the help body in the standard panel chrome so
// the swap is visually consistent with the panels' normal expanded
// rendering. focused controls border highlighting; activeMode renders
// the pink "active" border.
func renderPanelHelp(s Styles, pid PanelID, width, height int, focused, activeMode bool) string {
	inner := width - 4
	body := renderHelpBody(s, pid, inner)
	label := panelLabelForHelp(pid)
	if activeMode {
		return wrapPanelActive(s, body, label, width, height)
	}
	return wrapPanel(s, body, label, "h help", width, height, focused)
}

// panelLabelForHelp maps PanelID to the human-readable string used in the
// help-mode panel chrome. Keeps each panel labeled with its normal name
// so the user knows what they're looking at the help of.
func panelLabelForHelp(pid PanelID) string {
	switch pid {
	case PanelNow:
		return "now"
	case PanelCalls:
		return "activity"
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
	return "panel"
}
