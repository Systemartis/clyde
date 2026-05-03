package git

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// maxDiffLines caps the number of rendered lines consumed from git diff output
// to keep the TUI render fast on large diffs.
const maxDiffLines = 200

// Hunk is one contiguous changed region from a unified diff.
type Hunk struct {
	// Header is the raw @@ line, e.g. "@@ -42,7 +42,18 @@ authenticate(token)".
	Header string

	// OldStart is the starting line number in the old file.
	OldStart int
	// OldCount is the number of lines from the old file shown in the hunk.
	OldCount int
	// NewStart is the starting line number in the new file.
	NewStart int
	// NewCount is the number of lines from the new file shown in the hunk.
	NewCount int

	// Lines are the individual diff lines (context, add, remove).
	Lines []HunkLine
}

// HunkLine is a single line within a Hunk.
type HunkLine struct {
	// Type is ' ' for context, '+' for added, '-' for removed.
	Type rune
	// Text is the line content without the leading +/- marker.
	Text string
}

// Diff shells out to `git diff -U3 [<file>]` (and `git diff --cached -U3`) in
// the given working directory and returns parsed Hunks.
//
// If file is non-empty, the diff is scoped to that single file. If empty, all
// changed files are included. At most maxDiffLines individual HunkLines are
// returned across all hunks to cap render time on very large diffs.
//
// Successive calls with the same (cwd, file) within cacheTTL return the
// cached result, so the snapshot loop's 1Hz polling does not fork two
// `git diff` subprocesses every second on a quiet repo.
//
// Returns (nil, nil) when cwd is not a git repository or git is unavailable.
func (s *Source) Diff(cwd, file string) ([]Hunk, error) {
	key := diffCacheKey{cwd: cwd, file: file}

	s.mu.Lock()
	if entry, ok := s.diffCache[key]; ok && s.nowFn().Sub(entry.fetchedAt) < s.cacheTTL() {
		s.mu.Unlock()
		return entry.result, entry.err
	}
	s.mu.Unlock()

	args := buildDiffArgs(file, false)
	unstagedOut, err := s.gitRunner()(context.Background(), cwd, args...)
	if err != nil {
		// Not a repo or git unavailable — degrade gracefully. Cache
		// the empty result so a non-git cwd doesn't trigger a spawn
		// every tick.
		s.cacheDiff(key, nil, nil)
		return nil, nil
	}

	// Also fetch staged diff so pending staged edits show up.
	stagedArgs := buildDiffArgs(file, true)
	stagedOut, _ := s.gitRunner()(context.Background(), cwd, stagedArgs...)

	combined := append(unstagedOut, stagedOut...) //nolint:gocritic
	if len(combined) == 0 {
		s.cacheDiff(key, nil, nil)
		return nil, nil
	}

	parsed := parseDiff(combined)
	s.cacheDiff(key, parsed, nil)
	return parsed, nil
}

func (s *Source) cacheDiff(key diffCacheKey, result []Hunk, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.diffCache == nil {
		s.diffCache = make(map[diffCacheKey]diffCacheEntry)
	}
	s.diffCache[key] = diffCacheEntry{result: result, err: err, fetchedAt: s.nowFn()}
}

// buildDiffArgs constructs the git diff argument list.
func buildDiffArgs(file string, cached bool) []string {
	args := []string{"diff", "-U3"}
	if cached {
		args = append(args, "--cached")
	}
	if file != "" {
		args = append(args, "--", file)
	}
	return args
}

// ActiveFile scans events from the livesession view (passed as summary strings)
// and returns the file_path argument from the most recent Edit/Write/MultiEdit
// tool call. Returns "" when no such event is found.
//
// summaries is a slice of ap.Summary values from KindAssistant events, in
// chronological order (oldest first). The last matching entry wins.
func ActiveFile(summaries []string) string {
	active := ""
	for _, s := range summaries {
		tool, arg := splitToolSummary(s)
		switch strings.ToLower(tool) {
		case "edit", "write", "multiedit":
			if arg != "" {
				active = arg
			}
		}
	}
	return active
}

// DiffSummary builds the "filename · +N −M" label from a slice of Hunks.
// Returns "" when hunks is nil/empty.
func DiffSummary(file string, hunks []Hunk) string {
	if len(hunks) == 0 {
		return ""
	}
	adds, dels := 0, 0
	for _, h := range hunks {
		for _, l := range h.Lines {
			switch l.Type {
			case '+':
				adds++
			case '-':
				dels++
			}
		}
	}
	name := filepath.Base(file)
	if name == "." || name == "" {
		name = "all files"
	}
	return fmt.Sprintf("%s · +%d −%d", name, adds, dels)
}

// ─── diff parser ──────────────────────────────────────────────────────────────

// parseDiff parses unified diff output (as emitted by git diff -U3) into Hunks.
// Lines beyond maxDiffLines are silently dropped.
func parseDiff(raw []byte) []Hunk {
	var hunks []Hunk
	var cur *Hunk
	totalLines := 0

	sc := bufio.NewScanner(bytes.NewReader(raw))
	for sc.Scan() {
		line := sc.Text()

		// Hunk header line.
		if strings.HasPrefix(line, "@@ ") {
			if cur != nil {
				hunks = append(hunks, *cur)
			}
			cur = parseHunkHeader(line)
			continue
		}

		// Diff header lines (--- a/... +++ b/... diff --git ...) — skip.
		if cur == nil {
			continue
		}

		// Cap total body lines to avoid stalling the render on huge diffs.
		if totalLines >= maxDiffLines {
			continue
		}

		var hl HunkLine
		if len(line) == 0 {
			hl = HunkLine{Type: ' ', Text: ""}
		} else {
			switch line[0] {
			case '+':
				hl = HunkLine{Type: '+', Text: line[1:]}
			case '-':
				hl = HunkLine{Type: '-', Text: line[1:]}
			case ' ':
				hl = HunkLine{Type: ' ', Text: line[1:]}
			default:
				// No-diff-marker line (e.g. "\ No newline at end of file") — skip.
				continue
			}
		}

		cur.Lines = append(cur.Lines, hl)
		totalLines++
	}

	if cur != nil {
		hunks = append(hunks, *cur)
	}

	return hunks
}

// parseHunkHeader parses a line like "@@ -42,7 +46,18 @@ authenticate(token)"
// into a Hunk with Header, OldStart, OldCount, NewStart, NewCount set.
// Lines is nil; the caller appends body lines.
func parseHunkHeader(line string) *Hunk {
	h := &Hunk{Header: line}

	// Find the @@ ... @@ bracket.
	// Format: "@@ -<old_start>[,<old_count>] +<new_start>[,<new_count>] @@[ context]"
	end := strings.Index(line[3:], " @@")
	if end < 0 {
		return h
	}
	inner := line[3 : 3+end] // "-42,7 +46,18"

	parts := strings.Fields(inner)
	for _, p := range parts {
		if strings.HasPrefix(p, "-") {
			h.OldStart, h.OldCount = parseRange(p[1:])
		} else if strings.HasPrefix(p, "+") {
			h.NewStart, h.NewCount = parseRange(p[1:])
		}
	}
	return h
}

// parseRange splits "42,7" into (42, 7). Single number "42" maps to (42, 1).
func parseRange(s string) (start, count int) {
	parts := strings.SplitN(s, ",", 2)
	start, _ = strconv.Atoi(parts[0])
	if len(parts) == 2 {
		count, _ = strconv.Atoi(parts[1])
	} else {
		count = 1
	}
	return start, count
}

// splitToolSummary splits a summary string of the form "Tool: Edit /path" into
// ("Edit", "/path"). Returns ("", "") for non-tool summaries.
func splitToolSummary(summary string) (tool, arg string) {
	const prefix = "Tool: "
	if !strings.HasPrefix(summary, prefix) {
		return "", ""
	}
	rest := summary[len(prefix):]
	idx := strings.IndexByte(rest, ' ')
	if idx < 0 {
		return rest, ""
	}
	return rest[:idx], rest[idx+1:]
}
