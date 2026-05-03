// Package processscan is the ports.ProcessSource adapter that detects
// running `claude` CLI processes by parsing their argv for the
// `--session-id <UUID>` flag.
//
// Why scan processes at all
// -------------------------
// The TUI wants to mark a session as "live" so it earns a tab in the
// title-bar strip. A session whose JSONL was just appended to is
// trivially live (mtime within the activity window). But a session
// the user resumed via `claude --resume` and then left idle has a
// stale mtime — the `claude` process is alive, waiting for input,
// yet the file looks dead. Without a process probe, that session
// drops out of the tab strip after ~90s and the user has no way to
// switch back to it short of re-running `/resume`.
//
// Cwd filtering — by construction, not by code here
// -------------------------------------------------
// This adapter does NOT know or care what cwd a `claude` process was
// started in. The caller (livesession.applySessionStats) intersects
// the returned set of session IDs with the per-cwd session list it
// already has from SessionSource.Sessions(cwd). A process whose
// session JSONL lives under a different project's encoded dir simply
// won't be in that per-cwd list, so even if its ID appears in our
// scan it has no effect on the current view. Keeping cwd resolution
// out of this package keeps it portable + trivial to test.
package processscan

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/clyde-tui/clyde/internal/domain/session"
)

// processCacheTTL bounds how often we shell out to `ps -axo command=`.
// The TUI snapshot loop ticks at 1Hz, but the set of running `claude`
// processes does not meaningfully change second-to-second — at 5s
// granularity the user perceives no lag in the live-tab indicator
// while we shed ~80% of the fork/exec cost on the hot path.
const processCacheTTL = 5 * time.Second

// psStdoutCap caps memory consumption when reading `ps` output. A
// pathological argv on the box (a process with multi-MB env-style
// flags) shouldn't be able to inflate Clyde's RSS.
const psStdoutCap = 8 << 20 // 8 MiB

// sessionIDPattern matches a UUID v4 string that follows `--session-id`
// (with one or more spaces) in a process command line. Anchored to the
// flag so we only pick up real session IDs, not random UUIDs that may
// appear elsewhere in argv.
var sessionIDPattern = regexp.MustCompile(`--session-id\s+([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})`)

// Source is the ports.ProcessSource implementation. Holds a small
// time-windowed cache so the snapshot loop's 1Hz polling doesn't fork
// a fresh `ps` every tick — see processCacheTTL.
type Source struct {
	mu        sync.Mutex
	lastFetch time.Time
	lastIDs   []session.ID

	// Test seams. Production wires these via New().
	now      func() time.Time
	cacheTTL time.Duration
	runPS    func(ctx context.Context) ([]byte, error)
}

// New constructs a Source. The returned value is safe to share across
// goroutines.
func New() *Source {
	return &Source{
		now:      time.Now,
		cacheTTL: processCacheTTL,
		runPS:    runPSCommand,
	}
}

// runPSCommand shells out to `ps -axo command=` and returns its stdout,
// capped at psStdoutCap to prevent a process with a pathological argv
// from inflating our memory footprint.
func runPSCommand(ctx context.Context) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "ps", "-axo", "command=")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	out, readErr := io.ReadAll(io.LimitReader(stdout, psStdoutCap))
	waitErr := cmd.Wait()
	if readErr != nil {
		return nil, readErr
	}
	if waitErr != nil {
		return nil, waitErr
	}
	return out, nil
}

// RunningClaudeSessionIDs scans the OS process list and returns every
// session.ID that appears in a live `claude` command line. Empty slice
// + nil error means "no claude processes running".
//
// Calls within processCacheTTL of the last successful scan return the
// cached result without forking a new process. The cached slice is
// shared across callers — treat the returned slice as read-only.
//
// Implementation: `ps -axo command=` is POSIX-portable on macOS and
// Linux and prints one row per process containing the full argv with
// no header. We grep for "claude" + "--session-id" first as a cheap
// pre-filter, then run the regex only on candidates. Duplicate IDs
// (same session reported twice — shouldn't happen but just in case)
// are de-duplicated.
func (s *Source) RunningClaudeSessionIDs(ctx context.Context) ([]session.ID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.lastFetch.IsZero() && s.now().Sub(s.lastFetch) < s.cacheTTL {
		return s.lastIDs, nil
	}
	out, err := s.runPS(ctx)
	if err != nil {
		return nil, err
	}
	s.lastIDs = parseClaudeSessionIDs(out)
	s.lastFetch = s.now()
	return s.lastIDs, nil
}

// parseClaudeSessionIDs is the pure-function core of
// RunningClaudeSessionIDs, factored out so unit tests can feed in
// canned `ps` output without shelling out.
func parseClaudeSessionIDs(psOutput []byte) []session.ID {
	var ids []session.ID
	seen := make(map[string]bool)
	sc := bufio.NewScanner(bytes.NewReader(psOutput))
	// Some ps lines can be very long (full argv with many env-style
	// flags). Bump the buffer so we don't drop them.
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		// Cheap filter: must reference both `claude` (the binary) and
		// the flag. Avoids regex cost on the 99% of processes that
		// have nothing to do with claude code.
		if !strings.Contains(line, "claude") || !strings.Contains(line, "--session-id") {
			continue
		}
		// Skip our own clyde process — its argv contains "claude"
		// (path / package) but not the --session-id flag, so the
		// previous check already excludes us. Defense in depth: also
		// skip lines whose argv starts with the clyde binary in case
		// someone bundles the strings together.
		if strings.Contains(line, "/clyde") || strings.HasSuffix(line, " clyde") {
			continue
		}
		match := sessionIDPattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		id := match[1]
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, session.ID(id))
	}
	return ids
}
