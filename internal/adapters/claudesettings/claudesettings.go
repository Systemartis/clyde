// Package claudesettings reads and caches Claude Code's settings file
// (`~/.claude/settings.json`). Multiple TUI adapters need data from this
// file (lspscan reads enabledPlugins to flag LSPs; mcpconfig reads it to
// list MCP plugins). Without coordination, both would re-read and re-parse
// the file on every snapshot tick.
//
// A single Reader serves all callers. The cached parse is invalidated only
// when the file's (mtime, size) fingerprint changes — which happens when
// the user edits plugins in Claude Code, not on a per-second cadence.
package claudesettings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Settings is the parsed subset of `~/.claude/settings.json` that adapters
// in this codebase consume. Fields not listed here are ignored.
type Settings struct {
	EnabledPlugins map[string]bool
}

// Reader returns parsed Settings, caching the result across calls. Safe
// for concurrent use.
type Reader struct {
	// Path overrides the default `~/.claude/settings.json` location.
	// Tests pin this to a fixture; production leaves it empty.
	Path string

	mu        sync.Mutex
	cache     Settings
	haveCache bool
	lastStat  fingerprint
}

// fingerprint identifies a particular state of the settings file. We
// compare (mtime, size) rather than hashing — fast, cheap, and
// good-enough: a user edit always changes one of those.
type fingerprint struct {
	mtime time.Time
	size  int64
}

// New constructs a Reader pointing at the default settings path.
func New() *Reader { return &Reader{} }

// NewAt constructs a Reader pointing at a specific path.
func NewAt(path string) *Reader { return &Reader{Path: path} }

// Read returns the parsed settings. Returns an empty Settings (not an
// error) when the file is missing, unreadable, or malformed — adapters
// must degrade gracefully.
//
// Reads are cache-served when the file's (mtime, size) match the last
// observed state. The Stat call is cheap; the avoided ReadFile +
// json.Unmarshal would otherwise run on every TUI snapshot tick.
func (r *Reader) Read() Settings {
	r.mu.Lock()
	defer r.mu.Unlock()

	path := r.resolvePath()
	if path == "" {
		return Settings{}
	}

	info, err := os.Stat(path)
	if err != nil {
		// Missing or unreadable — clear the cache so a later
		// re-creation of the file doesn't serve stale results.
		r.cache = Settings{}
		r.haveCache = false
		r.lastStat = fingerprint{}
		return Settings{}
	}

	fp := fingerprint{mtime: info.ModTime(), size: info.Size()}
	if r.haveCache && r.lastStat == fp {
		return r.cache
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Settings{}
	}

	var raw struct {
		EnabledPlugins map[string]bool `json:"enabledPlugins"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Settings{}
	}

	r.cache = Settings{EnabledPlugins: raw.EnabledPlugins}
	r.lastStat = fp
	r.haveCache = true
	return r.cache
}

func (r *Reader) resolvePath() string {
	if r.Path != "" {
		return r.Path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "settings.json")
}
