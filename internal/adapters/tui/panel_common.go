package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// PanelState describes which visual state a panel is rendered in.
type PanelState int

const (
	// PanelStateNormal — unfocused, collapsed or expanded.
	PanelStateNormal PanelState = iota
	// PanelStatePassive — focused, expanded-passive (purple border).
	PanelStatePassive
	// PanelStateActive — focused, expanded-active (pink double border + mode badge).
	PanelStateActive
)

// modeBadgeText is the active-mode instruction badge shown on the top-right border.
const modeBadgeText = " ▲▼ scroll · +/− resize · esc back "

// modeBadgeShort is the truncated badge used when the panel is too narrow to
// fit the full instruction set without overflowing the top border.
const modeBadgeShort = " ▲▼ · esc "

// panelLabel returns the short label rendered in a panel's top border.
// Single source of truth — used by both the renderer (for the badge
// fallback math) and the click handler (to compute the badge x-range
// for clickable "esc back").
func panelLabel(pid PanelID) string {
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
	return ""
}

// activeBadgeRunes returns how many runes the active-mode badge occupies
// on the top border for a panel of the given total width. Mirrors the
// short-vs-long fallback logic in wrapPanelState exactly so the click
// handler agrees with the renderer on where the badge sits.
func activeBadgeRunes(pid PanelID, width int) int {
	innerW := width - 2
	labelW := len([]rune(" " + panelLabel(pid) + " "))
	if labelW+len([]rune(modeBadgeText))+2 > innerW {
		return len([]rune(modeBadgeShort))
	}
	return len([]rune(modeBadgeText))
}

// borderCharStyleForState returns a plain foreground-only style for border characters
// based on the panel's rendering state.
func borderCharStyleForState(s Styles, state PanelState) lipgloss.Style {
	switch state {
	case PanelStateActive:
		fg := s.PanelActive.GetBorderTopForeground()
		return lipgloss.NewStyle().Foreground(fg)
	case PanelStatePassive:
		fg := s.PanelFocus.GetBorderTopForeground()
		return lipgloss.NewStyle().Foreground(fg)
	default:
		fg := s.Panel.GetBorderTopForeground()
		return lipgloss.NewStyle().Foreground(fg)
	}
}

// borderCharStyle returns a plain foreground-only style for border characters.
// Kept for backward compatibility with collapsed-panel callers.
func borderCharStyle(s Styles, focused bool) lipgloss.Style {
	state := PanelStateNormal
	if focused {
		state = PanelStatePassive
	}
	return borderCharStyleForState(s, state)
}

// wrapPanel wraps rendered content in a hand-built rounded box with a label
// riding the top-left border and optional meta on the top-right.
//
// Uses rounded corners: ╭ ╮ ╰ ╯ (matching lipgloss.RoundedBorder())
// width/height are OUTER dimensions (including the two border chars on each axis).
// 1 char horizontal padding is applied inside (left + right inner pad).
func wrapPanel(s Styles, content, label, meta string, width, height int, focused bool) string {
	state := PanelStateNormal
	if focused {
		state = PanelStatePassive
	}
	return wrapPanelState(s, content, label, meta, width, height, state)
}

// wrapPanelActive wraps rendered content with the Expanded-Active visual cues:
// pink double border, pink label, and the mode badge replacing the meta text.
func wrapPanelActive(s Styles, content, label string, width, height int) string {
	return wrapPanelState(s, content, label, "", width, height, PanelStateActive)
}

// wrapPanelActiveBadge is wrapPanelActive with a caller-provided badge text
// instead of the default modeBadgeText. Used for sub-states inside an active
// panel where the standard "▲▼ scroll · +/− resize · esc back" hint would be
// misleading — e.g. the explorer's search overlay where ▲▼ moves through
// matches and +/− does nothing.
func wrapPanelActiveBadge(s Styles, content, label, badge string, width, height int) string {
	return wrapPanelState(s, content, label, badge, width, height, PanelStateActive)
}

// wrapPanelState is the core rendering function for expanded panels.
// It switches border style, label color, and meta badge based on state.
func wrapPanelState(s Styles, content, label, meta string, width, height int, state PanelState) string {
	innerW := width - 2 // subtract left+right border
	innerH := height - 2
	if innerW < 4 {
		innerW = 4
	}
	if innerH < 1 {
		innerH = 1
	}

	// 1-char interior padding on left and right → visible content width
	contentW := innerW - 2
	if contentW < 1 {
		contentW = 1
	}

	// Pad/clip content lines to contentW × innerH, then add 1-char side padding.
	// clipToWidth handles the SGR-aware truncation AND the ansi/lipgloss
	// width-disagreement edge case: chroma's syntax-highlighted output can
	// produce styled lines where ansi.StringWidth and lipgloss.Width report
	// different visible widths for the same input, which leaks past a naïve
	// truncate-then-pad and pushes neighboring panels rightwards.
	lines := strings.Split(content, "\n")
	for i, l := range lines {
		lines[i] = padLine(clipToWidth(l, contentW), contentW)
	}
	for len(lines) < innerH {
		lines = append(lines, strings.Repeat(" ", contentW))
	}
	lines = lines[:innerH]

	// Select border characters based on state
	var topLeft, topRight, botLeft, botRight, hBar, vBar string
	switch state {
	case PanelStateActive:
		// Double border for unmistakable active mode
		topLeft, topRight = "╔", "╗"
		botLeft, botRight = "╚", "╝"
		hBar = "═"
		vBar = "║"
	default:
		// Rounded corners for normal/passive
		topLeft, topRight = "╭", "╮"
		botLeft, botRight = "╰", "╯"
		hBar = "─"
		vBar = "│"
	}

	// Label string (plain, for width measurement)
	labelStr := " " + label + " "

	// Meta / badge string (plain, for width measurement). The active-mode
	// badge falls back to a short form when the long version would push the
	// label off the top border.
	var metaPlain string
	switch state {
	case PanelStateActive:
		// A non-empty meta argument overrides the default badge — used by
		// sub-states like the explorer's search overlay where the generic
		// "▲▼ scroll · +/− resize · esc back" hint is misleading.
		if meta != "" {
			metaPlain = " " + meta + " "
		} else {
			metaPlain = modeBadgeText
			labelW := len([]rune(" " + label + " "))
			if labelW+len([]rune(metaPlain))+2 > innerW {
				metaPlain = modeBadgeShort
			}
		}
	default:
		if meta != "" {
			metaPlain = " " + meta + " "
		}
	}

	// Styled label
	var labelRend string
	switch state {
	case PanelStateActive:
		labelRend = s.PanelLabelActive.Render(labelStr)
	case PanelStatePassive:
		labelRend = s.PanelLabelFocus.Render(labelStr)
	default:
		labelRend = s.PanelLabel.Render(labelStr)
	}

	// Styled meta / badge
	var metaRend string
	switch state {
	case PanelStateActive:
		metaRend = s.PanelModeBadge.Render(metaPlain)
	default:
		metaRend = s.PanelMeta.Render(metaPlain)
	}

	// Dashes for the top border fill
	labelW := len([]rune(labelStr))
	metaW := len([]rune(metaPlain))
	dashCount := innerW - labelW - metaW
	if dashCount < 0 {
		dashCount = 0
	}
	dashes := strings.Repeat(hBar, dashCount)
	bottomDashes := strings.Repeat(hBar, innerW)

	bs := borderCharStyleForState(s, state)

	topLine := bs.Render(topLeft) + labelRend + bs.Render(dashes) + metaRend + bs.Render(topRight)
	botLine := bs.Render(botLeft) + bs.Render(bottomDashes) + bs.Render(botRight)

	// Interior padding character
	pad := " "

	var out strings.Builder
	out.WriteString(topLine)
	out.WriteByte('\n')
	for _, l := range lines {
		out.WriteString(bs.Render(vBar) + pad + l + pad + bs.Render(vBar))
		out.WriteByte('\n')
	}
	out.WriteString(botLine)
	return out.String()
}

// wrapPanelCollapsed renders the one-liner collapsed state for a panel.
// Border = ╭ ▸ label: summary ──────────────────────────── meta ╮
//
//	╰────────────────────────────────────────────────────╯
func wrapPanelCollapsed(s Styles, label, summary, meta string, width int, focused bool) string {
	bs := borderCharStyle(s, focused)

	var labelRend, chevRend string
	chevStr := "▸"
	if focused {
		labelRend = s.PanelLabelFocus.Render(" " + label + ": ")
		chevRend = s.PanelLabelFocus.Render(chevStr)
	} else {
		labelRend = s.PanelCollapsedLabel.Render(" " + label + ": ")
		chevRend = s.PanelCollapsedLabel.Render(chevStr)
	}
	summaryRend := s.PanelMeta.Render(summary)
	metaRend := s.PanelMeta.Render("")
	if meta != "" {
		metaRend = s.PanelMeta.Render(" " + meta + " ")
	}

	// Calculate fills. innerW must be ≥ 0 — strings.Repeat panics on a
	// negative count, and a 0-width terminal (CI pty without TIOCSWINSZ)
	// would otherwise produce innerW = -2 and crash View().
	innerW := width - 2
	if innerW < 0 {
		innerW = 0
	}
	usedW := 1 + // space before chevron
		ansiWidth(chevStr) +
		ansiWidth(" "+label+": ") +
		ansiWidth(summary) +
		ansiWidth(metaRend)
	dashCount := innerW - usedW
	if dashCount < 0 {
		dashCount = 0
	}
	dashes := bs.Render(strings.Repeat("─", dashCount))
	bottomDashes := bs.Render(strings.Repeat("─", innerW))

	topLine := bs.Render("╭") + " " + chevRend + labelRend + summaryRend + dashes + metaRend + bs.Render("╮")
	botLine := bs.Render("╰") + bottomDashes + bs.Render("╯")

	return topLine + "\n" + botLine
}

// NOTE: progressBar and taskProgressBar have been replaced by bubbles/v2/progress.
// Callers now use progress.Model.ViewAs(percent) for static (golden-safe) rendering
// and progress.Model.SetPercent(p) + Update for animated runtime rendering.
