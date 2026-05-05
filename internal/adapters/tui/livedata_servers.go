package tui

import (
	"fmt"
	"strings"

	"github.com/Systemartis/clyde/internal/application/livesession"
)

// cleanMCPName trims the "@source" suffix Claude Code uses to qualify MCP
// plugins (e.g. "engram@engram", "superpowers@claude-plugins-official"). The
// suffix is the registry source, not useful in the servers panel chrome —
// callers usually only care about the plugin's friendly name.
func cleanMCPName(raw string) string {
	if i := strings.Index(raw, "@"); i >= 0 {
		return raw[:i]
	}
	return raw
}

// deriveServersFields populates the servers panel fields of MockData from the
// live view's MCPs and LSPs lists.
//
// Fields populated:
//   - MCPs:         list of Server entries (Status=On when Enabled, Off otherwise)
//   - LSPs:         list of Server entries from detected language servers
//   - ServersOn:    count of enabled MCPs + active LSPs
//   - ServersTotal: total count of MCPs + LSPs
//
// When both v.MCPs and v.LSPs are empty (adapters not wired or no detection),
// the existing MockData server entries are preserved so demo mode keeps working.
func deriveServersFields(v livesession.View, d MockData) MockData {
	if len(v.MCPs) == 0 && len(v.LSPs) == 0 {
		// No live data — keep existing values (demo mode or no adapter).
		return d
	}

	mcps := make([]Server, 0, len(v.MCPs))
	on := 0
	for _, mcp := range v.MCPs {
		status := ServerOff
		if mcp.Enabled {
			status = ServerOn
			on++
		}
		count := ""
		if mcp.ToolCount > 0 {
			count = fmt.Sprintf("%d", mcp.ToolCount)
		}
		mcps = append(mcps, Server{
			Status: status,
			Name:   cleanMCPName(mcp.Name),
			Count:  count,
		})
	}

	lsps := make([]Server, 0, len(v.LSPs))
	for _, lsp := range v.LSPs {
		// Three-state mapping reflects the user's actual situation:
		//   on PATH + claude wired   → green (live, in use)
		//   on PATH + claude unwired → amber ("you have it, claude doesn't know")
		//   missing                  → off (gray)
		// The amber case is the actionable one — the user can turn the LSP
		// on in claude code's plugin manager and it'll go green.
		var status ServerStatus
		switch {
		case lsp.Active && lsp.ClaudeEnabled:
			status = ServerOn
			on++
		case lsp.Active:
			status = ServerBusy
		default:
			status = ServerOff
		}
		lsps = append(lsps, Server{
			Status: status,
			Name:   lsp.Name,
		})
	}

	d.MCPs = mcps
	d.LSPs = lsps
	d.ServersOn = on
	d.ServersTotal = len(mcps) + len(lsps)

	return d
}
