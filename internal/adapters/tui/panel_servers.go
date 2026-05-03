package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
)

// renderServersExpanded renders the servers panel with active-mode support.
// In active mode the panel reads through a viewport so the user can scroll
// the full MCP + LSP list, regardless of how many of each are configured.
func renderServersExpanded(s Styles, p Palette, d MockData, vp viewport.Model, width, height int, focused, activeMode bool) string {
	if activeMode {
		inner := width - 4
		vp.SetWidth(inner)
		vp.SetHeight(height - 2)
		content := vp.View()
		return wrapPanelActive(s, content, "servers", width, height)
	}
	return renderServers(s, p, d, width, height, focused)
}

// buildServersViewportContent builds the full servers content (no truncation)
// for active-mode scrolling. Mirrors renderServers' section layout.
func buildServersViewportContent(s Styles, d MockData, inner int) string {
	var sb strings.Builder
	mcpOn := countOn(d.MCPs)
	lspOn := countOn(d.LSPs)
	if len(d.MCPs) > 0 {
		sb.WriteString(s.SrvSubHeader.Render(fmt.Sprintf("mcps · %d / %d", mcpOn, len(d.MCPs))))
		sb.WriteByte('\n')
		writeServerRows(&sb, s, d.MCPs, len(d.MCPs), inner, true)
	}
	if len(d.LSPs) > 0 {
		if len(d.MCPs) > 0 {
			sb.WriteString(s.SrvSubHeader.Render(strings.Repeat("·", clamp(inner, 1, 16))))
			sb.WriteByte('\n')
		}
		sb.WriteString(s.SrvSubHeader.Render(fmt.Sprintf("lsps · %d / %d", lspOn, len(d.LSPs))))
		sb.WriteByte('\n')
		writeServerRows(&sb, s, d.LSPs, len(d.LSPs), inner, false)
	}
	return strings.TrimRight(sb.String(), "\n")
}

// renderServers renders the servers panel with MCP + LSP sub-sections.
//
// The panel allocates its vertical budget proportionally between the two
// sections so a setup with many of one and few of the other still shows both,
// with "+ N more" trailers when items overflow the available space.
func renderServers(s Styles, _ Palette, d MockData, width, height int, focused bool) string {
	inner := width - 4 // border + 1-char pad each side
	innerH := height - 2
	if innerH < 4 {
		innerH = 4
	}

	mcpShow, lspShow := allocateServerBudget(len(d.MCPs), len(d.LSPs), innerH)

	var sb strings.Builder
	metaStr := fmt.Sprintf("%d / %d", d.ServersOn, d.ServersTotal)

	mcpOn := countOn(d.MCPs)
	lspOn := countOn(d.LSPs)

	// MCPs section.
	if len(d.MCPs) > 0 {
		header := fmt.Sprintf("mcps · %d / %d", mcpOn, len(d.MCPs))
		sb.WriteString(s.SrvSubHeader.Render(header))
		sb.WriteByte('\n')
		writeServerRows(&sb, s, d.MCPs, mcpShow, inner, true)
		if more := len(d.MCPs) - mcpShow; more > 0 {
			sb.WriteString(s.TextFade.Render(fmt.Sprintf("    + %d more", more)))
			sb.WriteByte('\n')
		}
	}

	// Separator + LSP section.
	if len(d.LSPs) > 0 {
		if len(d.MCPs) > 0 {
			sb.WriteString(s.SrvSubHeader.Render(strings.Repeat("·", clamp(inner, 1, 16))))
			sb.WriteByte('\n')
		}
		lspHeader := fmt.Sprintf("lsps · %d / %d", lspOn, len(d.LSPs))
		sb.WriteString(s.SrvSubHeader.Render(lspHeader))
		sb.WriteByte('\n')
		writeServerRows(&sb, s, d.LSPs, lspShow, inner, false)
		if more := len(d.LSPs) - lspShow; more > 0 {
			sb.WriteString(s.TextFade.Render(fmt.Sprintf("    + %d more", more)))
			sb.WriteByte('\n')
		}
	}

	content := strings.TrimRight(sb.String(), "\n")
	return wrapPanel(s, content, "servers", metaStr, width, height, focused)
}

// allocateServerBudget splits the panel's vertical budget between the MCPs
// and LSPs sections. Returns how many of each to render. Reserves rows for
// the section sub-headers and an optional dotted separator between them.
//
// Strategy:
//   - When both sections fit entirely, return their counts.
//   - Otherwise allocate proportionally to size, reflowing any unused slack
//     back into the section that still has items waiting.
func allocateServerBudget(mcpCount, lspCount, innerH int) (mcpShow, lspShow int) {
	chromeRows := 0
	if mcpCount > 0 {
		chromeRows++ // mcp header
	}
	if lspCount > 0 {
		chromeRows++ // lsp header
	}
	if mcpCount > 0 && lspCount > 0 {
		chromeRows++ // dotted separator
	}
	// Reserve 1 trailer line per overflowing section (worst case).
	maxItems := innerH - chromeRows - 2
	if maxItems < 0 {
		maxItems = 0
	}
	total := mcpCount + lspCount
	if total <= maxItems {
		return mcpCount, lspCount
	}
	// Proportional split rounded down.
	mcpShow = mcpCount * maxItems / total
	lspShow = maxItems - mcpShow
	// Reflow excess from the section that has fewer items than its share.
	if lspShow > lspCount {
		excess := lspShow - lspCount
		lspShow = lspCount
		mcpShow += excess
		if mcpShow > mcpCount {
			mcpShow = mcpCount
		}
	}
	if mcpShow > mcpCount {
		excess := mcpShow - mcpCount
		mcpShow = mcpCount
		lspShow += excess
		if lspShow > lspCount {
			lspShow = lspCount
		}
	}
	return mcpShow, lspShow
}

// countOn returns the number of servers with Status==ServerOn.
func countOn(servers []Server) int {
	on := 0
	for _, srv := range servers {
		if srv.Status == ServerOn {
			on++
		}
	}
	return on
}

// writeServerRows writes up to `show` server rows from `servers` into sb.
// withCount appends the right-aligned tool count when the server has one
// (used for MCPs only — LSPs do not currently report tool counts).
func writeServerRows(sb *strings.Builder, s Styles, servers []Server, show, inner int, withCount bool) {
	if show > len(servers) {
		show = len(servers)
	}
	for i := 0; i < show; i++ {
		srv := servers[i]
		dot := serverDot(s, srv.Status)
		nameW := inner - 4
		if withCount {
			nameW = inner - 8
		}
		var name string
		if srv.Status == ServerOff {
			name = s.SrvOff.Render(truncate(srv.Name, nameW))
		} else {
			name = s.SrvName.Render(truncate(srv.Name, nameW))
		}
		line := dot + " " + name
		if withCount && srv.Count != "" {
			ct := s.SrvCount.Render(srv.Count)
			lineW := ansiWidth(line)
			ctW := ansiWidth(ct)
			gapW := inner - lineW - ctW
			if gapW < 1 {
				gapW = 1
			}
			sb.WriteString(line + strings.Repeat(" ", gapW) + ct)
		} else {
			sb.WriteString(padLine(line, inner))
		}
		sb.WriteByte('\n')
	}
}

// renderServersCollapsed renders the collapsed one-liner for the servers panel.
func renderServersCollapsed(s Styles, d MockData, width int, focused bool) string {
	summary := fmt.Sprintf("%d/%d active", d.ServersOn, d.ServersTotal)
	return wrapPanelCollapsed(s, "servers", summary, "", width, focused)
}

// serverDot returns the colored status dot for a server.
func serverDot(s Styles, status ServerStatus) string {
	switch status {
	case ServerOn:
		return s.StatusGreen.Render("●")
	case ServerBusy:
		return s.Amber.Render("●")
	default:
		return s.TextFade.Render("●")
	}
}
