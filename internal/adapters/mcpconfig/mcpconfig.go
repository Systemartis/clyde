// Package mcpconfig reads MCP (Model Context Protocol) server configuration
// from the Claude Code settings file (~/.claude/settings.json).
//
// V1 scope:
//   - Lists plugins from the enabledPlugins map as MCP entries.
//   - Status is "enabled" for true entries, "disabled" for false entries.
//   - Tool count is 0 (live socket ping is deferred to V2).
//
// LSP detection is explicitly out of scope for V1 — callers receive an empty
// LSP list. This is a known limitation documented here.
//
// Read failure (file missing, permission denied, malformed JSON) is treated as
// an empty configuration — no error is propagated to the caller.
//
// The Source type satisfies ports.MCPSource — pass it wherever that interface
// is expected.
package mcpconfig

import (
	"sort"
	"strings"

	"github.com/clyde-tui/clyde/internal/adapters/claudesettings"
	"github.com/clyde-tui/clyde/internal/ports"
)

// Source reads MCP configuration from the Claude Code settings file.
//
// Backed by a claudesettings.Reader so the parse is served from cache
// when the file hasn't changed since the last call. Production code
// should share the Reader with lspscan so the per-tick reads coalesce.
//
// Source satisfies the ports.MCPSource interface.
type Source struct {
	settings *claudesettings.Reader
}

// New constructs a Source with its own private settings Reader at the
// default path. Use NewWith to share a Reader across adapters.
func New() Source { return NewWith(claudesettings.New()) }

// NewWith constructs a Source backed by the given settings Reader.
// A nil reader is treated as "use a fresh default-path Reader".
func NewWith(r *claudesettings.Reader) Source {
	if r == nil {
		r = claudesettings.New()
	}
	return Source{settings: r}
}

// MCPs implements ports.MCPSource.
//
// It returns the list of configured MCP entries from the settings file,
// sorted alphabetically by name for stable output.
//
// An empty slice (no error) is returned when:
//   - The settings file does not exist.
//   - The file cannot be read or parsed.
//   - No enabledPlugins key is present.
func (s Source) MCPs() ([]ports.MCPEntry, error) {
	plugins := s.reader().Read().EnabledPlugins
	if len(plugins) == 0 {
		return nil, nil
	}

	// Build a stable-sorted list (map iteration order is non-deterministic).
	keys := make([]string, 0, len(plugins))
	for k := range plugins {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	mcps := make([]ports.MCPEntry, 0, len(keys))
	for _, k := range keys {
		// Skip LSP plugins — they belong to the LSPs section. Plugin keys
		// follow the pattern "<name>-<kind>@<source>"; LSP entries end with
		// "-lsp" before the "@". The lspscan adapter cross-references the
		// same settings file and surfaces these as language servers.
		if isLSPPluginKey(k) {
			continue
		}
		mcps = append(mcps, ports.MCPEntry{
			Name:      k,
			Enabled:   plugins[k],
			ToolCount: 0,
		})
	}
	return mcps, nil
}

// reader returns the configured settings Reader, falling back to a
// default-path Reader for the zero-value Source — preserves the
// "Source{} is usable" contract for any caller that bypasses the
// constructor.
func (s Source) reader() *claudesettings.Reader {
	if s.settings != nil {
		return s.settings
	}
	return claudesettings.New()
}

// isLSPPluginKey reports whether a Claude Code plugin key is for a language
// server (so the MCPs adapter should ignore it; lspscan owns these).
func isLSPPluginKey(key string) bool {
	base := key
	if i := strings.Index(base, "@"); i >= 0 {
		base = base[:i]
	}
	return strings.HasSuffix(base, "-lsp")
}

// compile-time check: Source must satisfy ports.MCPSource.
var _ ports.MCPSource = Source{}
