package ports

// MCPEntry is the raw MCP plugin entry read from the settings source.
// It carries just enough information for the livesession view to build
// a displayable MCP list without knowing about the settings file format.
type MCPEntry struct {
	// Name is the plugin identifier as it appears in the settings file,
	// e.g. "engram@engram" or "claude-code-setup@claude-plugins-official".
	Name string

	// Enabled is true when the plugin is explicitly enabled.
	Enabled bool

	// ToolCount is the number of tools provided by this server.
	// 0 when live inspection is unavailable (V1).
	ToolCount int
}

// MCPSource is the port through which the application layer retrieves the list
// of configured MCP servers. Implementations MUST:
//
//   - Return entries sorted in a stable order (e.g. alphabetical by Name).
//   - Return an empty slice (not an error) when the source is unavailable.
//   - Never block for more than a few milliseconds (no live socket ping in V1).
type MCPSource interface {
	// MCPs returns all configured MCP entries from the underlying settings source.
	MCPs() ([]MCPEntry, error)
}
