// Package fsexplorer implements a filesystem tree walker that populates the
// explorer panel with real directory structure.
//
// Walk respects .gitignore at the repo root (simple line-match impl, no globs
// except the trailing-slash convention for directories).  Depth is capped at 5
// levels and entry count at 500 to prevent large repos from hanging the UI.
package fsexplorer

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	maxDepth   = 5
	maxEntries = 500

	// walkTTL bounds how long a Walk result is considered fresh. The TUI
	// snapshot loop polls at 1Hz; without coalescing we recurse the cwd
	// (depth 5, 500 entries) every second on a project the user isn't
	// even glancing at. Within walkTTL we ALSO check the cwd root mtime
	// and the .gitignore mtime, so changes that affect the visible tree
	// are picked up at the next tick — TTL is the upper bound, not the
	// only invalidator.
	walkTTL = 5 * time.Second
)

// Node is a single entry in the file tree.
type Node struct {
	// Name is the base filename or directory name.
	Name string

	// IsDir is true for directory nodes.
	IsDir bool

	// Path is the full path relative to the walk root (always uses "/").
	Path string

	// Children is non-nil for directory nodes.
	Children []*Node
}

// Source is the filesystem tree walker. The zero value is usable and
// includes the per-cwd Walk cache + .gitignore parse cache.
type Source struct {
	mu          sync.Mutex
	walkCache   map[string]walkCacheEntry
	ignoreCache map[string]ignoreCacheEntry

	// Test seams.
	now func() time.Time
	ttl time.Duration
}

// walkCacheEntry stores a parsed tree along with the cwd-root mtime
// observed at fetch time. A change in cwd-root mtime (a new top-level
// file appeared, or the user added an entry to .gitignore) invalidates
// the cache before the TTL expires.
type walkCacheEntry struct {
	root      *Node
	cwdMtime  time.Time
	ignMtime  time.Time
	fetchedAt time.Time
}

// ignoreCacheEntry stores a parsed gitIgnore for a particular file
// fingerprint. A change in mtime or size invalidates the parse.
type ignoreCacheEntry struct {
	parsed gitIgnore
	mtime  time.Time
	size   int64
}

func (s *Source) nowFn() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func (s *Source) cacheTTL() time.Duration {
	if s.ttl > 0 {
		return s.ttl
	}
	return walkTTL
}

// Walk traverses cwd up to maxDepth levels deep and returns the root node.
// Returns a non-nil root (possibly with no children) even on partial errors.
// Errors inside sub-directories are silently skipped to keep the UI alive.
//
// Successive calls within walkTTL return the cached result, provided neither
// the cwd root mtime nor the .gitignore mtime has advanced. This eliminates
// ~500 syscalls per second on a typical repo when the TUI is idle.
func (s *Source) Walk(cwd string) (*Node, error) {
	cwdInfo, _ := os.Stat(cwd)
	var cwdMtime time.Time
	if cwdInfo != nil {
		cwdMtime = cwdInfo.ModTime()
	}
	ignInfo, _ := os.Stat(filepath.Join(cwd, ".gitignore"))
	var ignMtime time.Time
	if ignInfo != nil {
		ignMtime = ignInfo.ModTime()
	}

	s.mu.Lock()
	if entry, ok := s.walkCache[cwd]; ok {
		fresh := s.nowFn().Sub(entry.fetchedAt) < s.cacheTTL()
		cwdUnchanged := entry.cwdMtime.Equal(cwdMtime)
		ignUnchanged := entry.ignMtime.Equal(ignMtime)
		if fresh && cwdUnchanged && ignUnchanged {
			s.mu.Unlock()
			return entry.root, nil
		}
	}
	s.mu.Unlock()

	root := &Node{
		Name:  filepath.Base(cwd),
		IsDir: true,
		Path:  ".",
	}

	ignorer := s.loadGitignoreCached(cwd)
	counter := &counter{}

	walkDir(cwd, root, cwd, ignorer, counter, 0)
	sortNodeIDEStyle(root)

	s.mu.Lock()
	if s.walkCache == nil {
		s.walkCache = make(map[string]walkCacheEntry)
	}
	s.walkCache[cwd] = walkCacheEntry{
		root:      root,
		cwdMtime:  cwdMtime,
		ignMtime:  ignMtime,
		fetchedAt: s.nowFn(),
	}
	s.mu.Unlock()

	return root, nil
}

// loadGitignoreCached returns the parsed .gitignore for root, served
// from the cache when the file's (mtime, size) is unchanged. A missing
// .gitignore returns the empty matcher (nothing is ignored beyond the
// hard-coded `.git`).
func (s *Source) loadGitignoreCached(root string) gitIgnore {
	path := filepath.Join(root, ".gitignore")
	info, err := os.Stat(path)
	if err != nil {
		// No .gitignore — purge any prior cache for this root.
		s.mu.Lock()
		delete(s.ignoreCache, root)
		s.mu.Unlock()
		return gitIgnore{}
	}

	s.mu.Lock()
	if entry, ok := s.ignoreCache[root]; ok && entry.mtime.Equal(info.ModTime()) && entry.size == info.Size() {
		s.mu.Unlock()
		return entry.parsed
	}
	s.mu.Unlock()

	parsed := loadGitignore(root)

	s.mu.Lock()
	if s.ignoreCache == nil {
		s.ignoreCache = make(map[string]ignoreCacheEntry)
	}
	s.ignoreCache[root] = ignoreCacheEntry{parsed: parsed, mtime: info.ModTime(), size: info.Size()}
	s.mu.Unlock()
	return parsed
}

// sortNodeIDEStyle sorts a node's children in IDE-conventional order:
// directories first, then files; case-insensitive alphabetical within each
// group. Recurses into subdirectories so the ordering applies at every level.
//
// ReadDir returns entries in case-sensitive ASCII order ('.git' before 'Z.go'
// before 'a.go'), which interleaves dirs and files in a way that's hard to
// scan. IDEs (VS Code, IntelliJ, finder) all use this dirs-first convention.
func sortNodeIDEStyle(n *Node) {
	if n == nil || len(n.Children) == 0 {
		return
	}
	sort.SliceStable(n.Children, func(i, j int) bool {
		a, b := n.Children[i], n.Children[j]
		if a.IsDir != b.IsDir {
			return a.IsDir
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	for _, c := range n.Children {
		sortNodeIDEStyle(c)
	}
}

// ─── internal helpers ─────────────────────────────────────────────────────────

// counter tracks total entries created to enforce the maxEntries cap.
type counter struct{ n int }

func (c *counter) inc() bool {
	c.n++
	return c.n <= maxEntries
}

// walkDir recursively populates parent.Children for the directory at absDir.
//
// Two-pass design: pass 1 creates all nodes for the current directory level
// (ensuring all sibling entries are added before recursion consumes budget).
// Pass 2 recurses into directories using the shared counter for the sub-tree.
//
// At depth 0 (top-level cwd) the entry cap is never applied so that all
// immediate children of the watched directory are always visible — the cap only
// limits how deeply sub-trees are expanded.
func walkDir(absDir string, parent *Node, root string, ign gitIgnore, cnt *counter, depth int) {
	if depth >= maxDepth {
		return
	}

	entries, err := os.ReadDir(absDir)
	if err != nil {
		return
	}

	// Pass 1: create a node for every accepted entry at this depth level.
	// At depth 0 we skip the counter so all top-level entries are always shown.
	for _, e := range entries {
		name := e.Name()

		// Skip version-control internals and common noise directories regardless
		// of depth.  We deliberately do NOT skip all dot-prefixed entries so that
		// dot-directories (.github, .vscode, etc.) are visible to the user.
		// .git is the only directory that is ALWAYS noise in every repo; others
		// are filtered by .gitignore patterns (see loadGitignore).
		if name == ".git" {
			continue
		}

		// Compute relative path from walk root for gitignore matching.
		abs := filepath.Join(absDir, name)
		rel, _ := filepath.Rel(root, abs)
		rel = filepath.ToSlash(rel)

		if ign.match(rel, e.IsDir()) {
			continue
		}

		// Apply entry cap only for depth > 0 (not the immediate cwd children).
		if depth > 0 && !cnt.inc() {
			// Entry cap reached — stop adding children for this sub-tree.
			return
		}

		node := &Node{
			Name:  name,
			IsDir: e.IsDir(),
			Path:  rel,
		}
		parent.Children = append(parent.Children, node)
	}

	// Pass 2: recurse into directories now that all siblings at this level exist.
	for _, child := range parent.Children {
		if !child.IsDir {
			continue
		}
		walkDir(filepath.Join(absDir, child.Name), child, root, ign, cnt, depth+1)
	}
}

// ─── .gitignore parser ────────────────────────────────────────────────────────

// gitIgnore holds parsed patterns from a .gitignore file.
type gitIgnore struct {
	patterns []string
}

// loadGitignore reads <root>/.gitignore and returns a gitIgnore instance.
// Returns an empty gitIgnore on any read error.
func loadGitignore(root string) gitIgnore {
	// G304: root is the cwd we were launched in; the suffix is the literal
	// string ".gitignore". Reading the project's own gitignore is the
	// entire purpose of this function.
	f, err := os.Open(filepath.Join(root, ".gitignore")) //nolint:gosec // see comment
	if err != nil {
		return gitIgnore{}
	}
	defer func() { _ = f.Close() }()

	var patterns []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return gitIgnore{patterns: patterns}
}

// match reports whether the given relative path should be ignored.
// isDir is true when the path refers to a directory.
//
// Implementation handles these .gitignore conventions:
//   - Trailing "/" → directory-only pattern.
//   - Leading "/" → anchored to root (we strip it and match as a prefix).
//   - "*" in pattern → glob match via filepath.Match against the base name.
//   - Plain name → match against base name (any depth).
//   - Path with "/" in middle → match against full relative path.
func (g gitIgnore) match(rel string, isDir bool) bool {
	base := filepath.Base(rel)
	for _, pat := range g.patterns {
		if matchPattern(pat, rel, base, isDir) {
			return true
		}
	}
	return false
}

// matchPattern returns true when pat matches rel/base/isDir.
func matchPattern(pat, rel, base string, isDir bool) bool {
	// Directory-only pattern.
	dirOnly := strings.HasSuffix(pat, "/")
	if dirOnly {
		if !isDir {
			return false
		}
		pat = strings.TrimSuffix(pat, "/")
	}

	// Anchored pattern (leading slash).
	anchored := strings.HasPrefix(pat, "/")
	if anchored {
		pat = strings.TrimPrefix(pat, "/")
	}

	// If pattern contains an inner slash it's a path-based pattern.
	if strings.Contains(pat, "/") {
		// Match against full relative path.
		ok, _ := filepath.Match(pat, rel)
		if !ok {
			// Try matching a prefix segment.
			ok = strings.HasPrefix(rel+"/", pat+"/")
		}
		return ok
	}

	if anchored {
		// Anchored without inner slash → match against first path segment.
		seg := rel
		if idx := strings.Index(rel, "/"); idx != -1 {
			seg = rel[:idx]
		}
		ok, _ := filepath.Match(pat, seg)
		return ok
	}

	// Default: match against the base name (any depth).
	ok, _ := filepath.Match(pat, base)
	return ok
}
