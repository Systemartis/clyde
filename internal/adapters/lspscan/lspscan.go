// Package lspscan — LSPSource adapter that detects language servers on PATH.
//
// V1 strategy: hold a curated list of well-known LSP binary names → display
// label, and check exec.LookPath for each. Detected entries appear with
// Active=true. The list is conservative — extending it is a one-line change
// in knownLSPs.
package lspscan

import (
	"os/exec"
	"sort"
	"strings"
	"sync"

	"github.com/Systemartis/clyde/internal/adapters/claudesettings"
	"github.com/Systemartis/clyde/internal/ports"
)

// Source is the concrete LSPSource. Construct with New (for production)
// or NewWith (when sharing a settings Reader across adapters).
//
// The settings Reader supplies the enabledPlugins map from
// `~/.claude/settings.json`. It is stateful (caches parsed contents
// between calls) so adapters that share a Reader instance also share
// the cache — see cmd/clyde/main.go for the shared wiring.
type Source struct {
	settings *claudesettings.Reader

	// binMu guards the one-shot binary scan cache. The set of installed
	// LSP binaries does not meaningfully change during a clyde session
	// (a user installing a new LSP would also need to restart Claude
	// Code to pick it up). Caching for the process lifetime removes
	// ~40 exec.LookPath calls from the per-second snapshot tick.
	binMu      sync.Mutex
	binCache   []scannedLSP
	binScanned bool

	// lookPath is the test seam for the binary scan. nil → exec.LookPath.
	lookPath func(name string) (string, error)
}

// scannedLSP is the cached, ClaudeEnabled-agnostic record for a detected
// language server. ClaudeEnabled is recomputed on every LSPs() call from
// the current settings.json — that field can change without a clyde
// restart, but the binary's existence on PATH cannot.
type scannedLSP struct {
	binary string // lookup key for enabledPlugins (e.g. "pyright")
	name   string // display label (e.g. "pyright")
	path   string // absolute path from exec.LookPath
}

// New constructs a Source with its own private settings Reader at the
// default path. Use NewWith to share a Reader across adapters (so their
// settings.json caches coalesce — see cmd/clyde/main.go).
func New() *Source { return NewWith(claudesettings.New()) }

// NewWith constructs a Source backed by the given settings Reader.
// A nil reader is treated as "use a fresh default-path Reader".
func NewWith(r *claudesettings.Reader) *Source {
	if r == nil {
		r = claudesettings.New()
	}
	return &Source{settings: r}
}

// knownLSP pairs a binary name with the display label clyde shows.
type knownLSP struct {
	binary string
	name   string
}

// knownLSPs is the curated list of language servers clyde recognizes. Order
// here is irrelevant — the result is sorted alphabetically by display name.
//
// Adding a new entry is a one-line change. Keep entries grouped by language
// for readability.
var knownLSPs = []knownLSP{
	// Go
	{"gopls", "gopls"},
	// Rust
	{"rust-analyzer", "rust-analyzer"},
	// TypeScript / JavaScript
	{"typescript-language-server", "ts-ls"},
	{"tsserver", "tsserver"},
	{"deno", "deno-lsp"},
	{"biome", "biome"},
	{"vscode-eslint-language-server", "eslint-ls"},
	// Python
	{"pyright-langserver", "pyright"},
	{"pyright", "pyright"},
	{"pylsp", "pylsp"},
	{"ruff-lsp", "ruff-lsp"},
	{"ruff", "ruff"},
	// C / C++
	{"clangd", "clangd"},
	// Lua / shell / web
	{"lua-language-server", "lua-ls"},
	{"bash-language-server", "bash-ls"},
	{"vscode-json-language-server", "json-ls"},
	{"vscode-html-language-server", "html-ls"},
	{"vscode-css-language-server", "css-ls"},
	{"yaml-language-server", "yaml-ls"},
	// Other systems languages
	{"haskell-language-server-wrapper", "hls"},
	{"ocamllsp", "ocamllsp"},
	{"sourcekit-lsp", "sourcekit-lsp"},
	{"zls", "zls"},
	// Web frameworks
	{"vue-language-server", "vue-ls"},
	{"svelte-language-server", "svelte-ls"},
	// Backend ecosystems
	{"elixir-ls", "elixir-ls"},
	{"nextls", "nextls"},
	{"ruby-lsp", "ruby-lsp"},
	{"solargraph", "solargraph"},
	{"jdtls", "jdtls"},
	{"kotlin-language-server", "kotlin-ls"},
	{"erlang_ls", "erlang-ls"},
	{"intelephense", "intelephense"},
	// Infra / config
	{"terraform-ls", "terraform-ls"},
	{"ansible-language-server", "ansible-ls"},
	{"helm-ls", "helm-ls"},
	{"docker-langserver", "docker-ls"},
	// Markdown
	{"marksman", "marksman"},
	// Nix
	{"nil", "nil"},
	{"rnix-lsp", "rnix-lsp"},
}

// LSPs returns all known language servers found on PATH. Result is sorted
// alphabetically by display name and deduplicated by name (so detecting both
// pyright-langserver and pyright reports a single "pyright" entry).
//
// The PATH scan is cached for the process lifetime — see scanBinaries.
// ClaudeEnabled is recomputed per call from the current settings.json so
// toggling a plugin in Claude Code is reflected without a clyde restart.
//
// ClaudeEnabled is set by cross-referencing Claude Code's settings.json
// enabledPlugins map: a plugin keyed "<binary>-lsp@*" with value true means
// Claude Code has the LSP wired into its plugin system. When this is false
// while the binary is on PATH, the TUI flags it with an amber dot — the
// user has the LSP installed but Claude isn't actually using it.
func (s *Source) LSPs() ([]ports.LSPInfo, error) {
	scanned := s.scanBinaries()
	enabled := s.readClaudeEnabledLSPs()

	out := make([]ports.LSPInfo, len(scanned))
	for i, l := range scanned {
		out[i] = ports.LSPInfo{
			Name:          l.name,
			Binary:        l.path,
			Active:        true,
			ClaudeEnabled: enabled[l.binary] || enabled[l.name],
		}
	}
	return out, nil
}

// scanBinaries walks knownLSPs once, calling exec.LookPath for each
// binary, and caches the result for the lifetime of the Source. The
// snapshot loop calls LSPs() at 1Hz; without this cache that's ~40
// PATH walks per second to ask the same question.
//
// Concurrent callers race the scan exactly once via binMu — subsequent
// callers see the populated cache.
func (s *Source) scanBinaries() []scannedLSP {
	s.binMu.Lock()
	defer s.binMu.Unlock()
	if s.binScanned {
		return s.binCache
	}
	lookPath := s.lookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	seen := map[string]bool{}
	var found []scannedLSP
	for _, l := range knownLSPs {
		if seen[l.name] {
			continue
		}
		path, err := lookPath(l.binary)
		if err != nil {
			continue
		}
		seen[l.name] = true
		found = append(found, scannedLSP{binary: l.binary, name: l.name, path: path})
	}
	sort.Slice(found, func(i, j int) bool { return found[i].name < found[j].name })
	s.binCache = found
	s.binScanned = true
	return s.binCache
}

// readClaudeEnabledLSPs returns a set of binary names Claude Code has
// enabled as LSP plugins. Plugin keys follow the pattern
// "<name>-lsp@<source>" — we strip the suffix to get the bare binary name.
//
// Returns an empty map (not an error) when the settings file is missing,
// unreadable, malformed, or has no enabledPlugins key. The TUI degrades
// gracefully — every detected LSP just defaults to ClaudeEnabled=false,
// rendering as amber if on PATH.
//
// Reads go through the shared claudesettings.Reader so the parse is
// served from cache when the file hasn't changed since the last read.
func (s *Source) readClaudeEnabledLSPs() map[string]bool {
	plugins := s.settings.Read().EnabledPlugins
	if len(plugins) == 0 {
		return nil
	}
	out := make(map[string]bool, len(plugins))
	for key, on := range plugins {
		if !on {
			continue
		}
		// Strip "@<source>" suffix to get "<name>-lsp", then drop the trailing
		// "-lsp" if present. Plugin keys without "-lsp" before "@" are not
		// LSP plugins (e.g. "engram@engram", "superpowers@claude-plugins-official").
		base := key
		if i := strings.Index(base, "@"); i >= 0 {
			base = base[:i]
		}
		if !strings.HasSuffix(base, "-lsp") {
			continue
		}
		bare := strings.TrimSuffix(base, "-lsp")
		out[bare] = true
	}
	return out
}
