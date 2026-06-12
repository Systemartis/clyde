// Package git implements a thin adapter that shells out to the git binary to
// retrieve repository status information.
//
// It is intentionally minimal: no in-process libgit2, no CGo.  We just parse
// the stable porcelain output of `git status --porcelain` and use
// `git rev-parse --abbrev-ref HEAD` for the branch name.
//
// Errors from git (e.g. not a git repo) are returned to the caller so they
// can degrade gracefully rather than crashing the UI.
//
// Caching
// -------
// Source coalesces repeated Status and Diff calls within a short TTL so the
// 1Hz TUI snapshot loop doesn't fork+exec `git status` and two `git diff`
// processes every second on a quiet repo. See gitTTL and the cache fields
// below.
package git

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// gitTTL bounds how long cached Status and Diff results are considered
// fresh. The TUI polls at 1Hz; 3s caches three out of every four calls
// while keeping the perceived UI lag for actual git changes under one
// human-noticeable threshold.
const gitTTL = 3 * time.Second

// FileStatus represents one entry from `git status --porcelain`.
type FileStatus struct {
	// Path is the file path relative to the repo root.
	Path string

	// Status is the single-character code: 'M', 'A', 'D', 'R', '?', etc.
	// For untracked files both XY columns are '?'; we normalise that to '?'.
	Status rune

	// Staged is true when the status character comes from the index (X) column.
	Staged bool
}

// Source shells out to git for status and diff information.
//
// Method receivers are pointer-typed so the cache state can be shared
// across the wrapping adapters (LiveSessionAdapter, DiffAdapter) — both
// need to see the same cached results in production. Construct one
// Source per process and share it.
//
// The zero value is usable directly (lazy default ttl/now/runGit) — useful
// for tests that don't care about caching.
type Source struct {
	mu          sync.Mutex
	statusCache map[string]statusCacheEntry
	diffCache   map[diffCacheKey]diffCacheEntry

	// Test seams. nil → package defaults.
	now    func() time.Time
	ttl    time.Duration
	runner func(ctx context.Context, dir string, args ...string) ([]byte, error)
}

// statusCacheEntry holds the parsed result of one Status call together
// with its observation timestamp. fileStatuses is sorted by Path and
// safe to share with concurrent callers (treat as read-only).
type statusCacheEntry struct {
	result    []FileStatus
	err       error
	fetchedAt time.Time
}

// diffCacheKey uniquely identifies a Diff call.
type diffCacheKey struct {
	cwd  string
	file string
}

// diffCacheEntry holds the result of one Diff call.
type diffCacheEntry struct {
	result    []Hunk
	err       error
	fetchedAt time.Time
}

// nowFn returns the timestamp source — defaulting to time.Now when the
// test seam is unset.
func (s *Source) nowFn() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

// cacheTTL returns the configured cache window — defaulting to gitTTL.
func (s *Source) cacheTTL() time.Duration {
	if s.ttl > 0 {
		return s.ttl
	}
	return gitTTL
}

// gitRunner returns the runner that actually shells out to git —
// defaulting to runGitCommand. Test code can pin a stub.
func (s *Source) gitRunner() func(ctx context.Context, dir string, args ...string) ([]byte, error) {
	if s.runner != nil {
		return s.runner
	}
	return runGitCommand
}

// Status runs `git status --porcelain` in cwd and returns the list of changed
// files.  Returns an empty slice (no error) when the directory is not a git
// repo or when git is not installed.
//
// Successive calls within cacheTTL return the cached result. A newly-staged
// or newly-modified file may not appear immediately — the lag is bounded
// by gitTTL (3s by default).
func (s *Source) Status(cwd string) ([]FileStatus, error) {
	s.mu.Lock()
	if entry, ok := s.statusCache[cwd]; ok && s.nowFn().Sub(entry.fetchedAt) < s.cacheTTL() {
		s.mu.Unlock()
		return entry.result, entry.err
	}
	s.mu.Unlock()

	out, err := s.gitRunner()(context.Background(), cwd, "status", "--porcelain")
	if err != nil {
		// Not a repo or git not available — degrade gracefully. Cache
		// the empty result so we don't keep retrying every tick when
		// the user is in a non-git cwd.
		s.cacheStatus(cwd, nil, nil)
		return nil, nil
	}
	parsed := parseStatus(out)
	s.cacheStatus(cwd, parsed, nil)
	return parsed, nil
}

func (s *Source) cacheStatus(cwd string, result []FileStatus, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.statusCache == nil {
		s.statusCache = make(map[string]statusCacheEntry)
	}
	s.statusCache[cwd] = statusCacheEntry{result: result, err: err, fetchedAt: s.nowFn()}
}

// Branch runs `git rev-parse --abbrev-ref HEAD` in cwd.
// dirty is true when the working tree has uncommitted changes.
// Returns ("", false, nil) when cwd is not a git repo.
func (s *Source) Branch(cwd string) (branch string, dirty bool, err error) {
	out, err := s.gitRunner()(context.Background(), cwd, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", false, nil //nolint:nilerr
	}
	branch = strings.TrimSpace(string(out))

	// Check dirty by looking at status output.
	statuses, sErr := s.Status(cwd)
	if sErr == nil && len(statuses) > 0 {
		dirty = true
	}
	return branch, dirty, nil
}

// ─── helpers ──────────────────────────────────────────────────────────────────

// runGitCommand executes git with the given args inside dir and returns
// stdout. Stderr is captured and surfaced via the error message capped
// at 256 bytes (so a verbose GIT_TRACE setting can't flood logs with
// per-call diagnostic spew).
func runGitCommand(ctx context.Context, dir string, args ...string) ([]byte, error) {
	// G204: args are caller-controlled but always come from this package's
	// own functions (Status, Diff, etc.) with hard-coded subcommand
	// strings + caller-provided commit refs. The binary name "git" is a
	// constant. Treating this as user-supplied input would be wrong.
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // see comment
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %v: %w (%s)", args, err, capStderr(stderr.Bytes()))
	}
	return stdout.Bytes(), nil
}

// capStderr returns a short, redacted summary of stderr suitable for an
// error message. Caps at 256 bytes and drops any control characters
// (ANSI escapes from a colorized GIT_TRACE) so error messages remain
// log-safe even when the user has unusual git env vars.
func capStderr(b []byte) string {
	const maxLen = 256
	if len(b) > maxLen {
		b = b[:maxLen]
	}
	out := make([]byte, 0, len(b))
	for _, c := range b {
		if c == '\t' || c == '\n' || (c >= 0x20 && c < 0x7f) {
			out = append(out, c)
		}
	}
	return strings.TrimSpace(string(out))
}

// parseStatus parses the output of `git status --porcelain` line by line.
// Porcelain v1 format: XY <path> (or XY <orig> -> <path> for renames).
// We only care about the XY codes and the final path.
func parseStatus(raw []byte) []FileStatus {
	var out []FileStatus
	sc := bufio.NewScanner(bytes.NewReader(raw))
	for sc.Scan() {
		line := sc.Text()
		if len(line) < 3 {
			continue
		}
		x := rune(line[0]) // index status
		y := rune(line[1]) // worktree status
		// Path starts at column 3 (after "XY ").
		path := strings.TrimSpace(line[3:])

		// Renames: "old -> new" — keep only the new path.
		if strings.Contains(path, " -> ") {
			parts := strings.SplitN(path, " -> ", 2)
			if len(parts) == 2 {
				path = parts[1]
			}
		}

		// Untracked files: XY == "??"
		if x == '?' && y == '?' {
			out = append(out, FileStatus{Path: path, Status: '?', Staged: false})
			continue
		}

		// Index change (staged).
		if x != ' ' && x != '?' {
			out = append(out, FileStatus{Path: path, Status: x, Staged: true})
			continue
		}

		// Worktree change (unstaged).
		if y != ' ' && y != '?' {
			out = append(out, FileStatus{Path: path, Status: y, Staged: false})
		}
	}
	return out
}
