// Package ports — LSPSource port definition.
//
// LSPSource discovers language servers available on the developer's machine.
// V1 detects by scanning $PATH for known LSP binary names; V2+ may extend
// to inspect editor configurations or project-local toolchains.
package ports

// LSPInfo describes a single language server detected on the host.
type LSPInfo struct {
	// Name is the human-readable display label, e.g. "gopls", "rust-analyzer".
	Name string

	// Binary is the executable name as resolved on PATH, e.g. "/usr/local/bin/gopls".
	// Empty when the source did not preserve the resolved path.
	Binary string

	// Active is true when the binary is available (resolved on PATH).
	// Note: this is "installed and runnable", NOT "claude code is using it".
	// See ClaudeEnabled for that distinction.
	Active bool

	// ClaudeEnabled is true when Claude Code's settings.json enabledPlugins
	// map contains the plugin entry for this LSP (e.g.
	// "gopls-lsp@claude-plugins-official"). When false, the binary may be
	// installed locally but Claude Code is not actually using it — the TUI
	// flags this state with an amber dot so the user notices the gap.
	ClaudeEnabled bool
}

// LSPSource is the port through which the application layer retrieves the
// list of detected language servers.
//
// Implementations MUST:
//   - Return an empty slice (not an error) when none are found.
//   - Return entries sorted in a stable order (alphabetical by Name).
//   - Complete in well under a second on a normal machine.
type LSPSource interface {
	LSPs() ([]LSPInfo, error)
}
