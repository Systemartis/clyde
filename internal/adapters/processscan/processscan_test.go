package processscan

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Systemartis/clyde/internal/domain/session"
)

// TestRunningClaudeSessionIDs_CachesWithinTTL verifies the throttle:
// repeated calls within processCacheTTL of a successful fetch must NOT
// re-shell-out — they return the cached slice. This is the perf-audit
// fix for the ~1Hz `ps` spawning that was eating idle CPU.
func TestRunningClaudeSessionIDs_CachesWithinTTL(t *testing.T) {
	t.Parallel()

	canned := []byte(`/opt/homebrew/bin/claude --session-id aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee
`)
	calls := 0
	clock := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)

	s := &Source{
		now:      func() time.Time { return clock },
		cacheTTL: 5 * time.Second,
		runPS: func(_ context.Context) ([]byte, error) {
			calls++
			return canned, nil
		},
	}

	// First call: fetches.
	if _, err := s.RunningClaudeSessionIDs(context.Background()); err != nil {
		t.Fatalf("first call: unexpected err %v", err)
	}
	if calls != 1 {
		t.Fatalf("first call: runPS called %d times, want 1", calls)
	}

	// Second call within TTL: served from cache, no fetch.
	clock = clock.Add(2 * time.Second)
	if _, err := s.RunningClaudeSessionIDs(context.Background()); err != nil {
		t.Fatalf("second call: unexpected err %v", err)
	}
	if calls != 1 {
		t.Fatalf("second call within TTL: runPS called %d times, want 1 (cached)", calls)
	}

	// Third call past TTL: re-fetches.
	clock = clock.Add(4 * time.Second) // +6s total since first
	if _, err := s.RunningClaudeSessionIDs(context.Background()); err != nil {
		t.Fatalf("third call: unexpected err %v", err)
	}
	if calls != 2 {
		t.Fatalf("third call past TTL: runPS called %d times, want 2 (re-fetch)", calls)
	}
}

// TestRunningClaudeSessionIDs_FetchErrorBypassesCache verifies that a
// failed fetch does not poison the cache — the next call retries
// immediately rather than serving stale results.
func TestRunningClaudeSessionIDs_FetchErrorBypassesCache(t *testing.T) {
	t.Parallel()

	calls := 0
	clock := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	boom := errors.New("ps unavailable")

	s := &Source{
		now:      func() time.Time { return clock },
		cacheTTL: 5 * time.Second,
		runPS: func(_ context.Context) ([]byte, error) {
			calls++
			return nil, boom
		},
	}

	if _, err := s.RunningClaudeSessionIDs(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("first call: err = %v, want %v", err, boom)
	}
	if _, err := s.RunningClaudeSessionIDs(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("second call: err = %v, want %v", err, boom)
	}
	if calls != 2 {
		t.Fatalf("runPS called %d times, want 2 (errors must not be cached)", calls)
	}
}

// TestParseClaudeSessionIDsFromMacOSPS feeds canned macOS ps output
// (the form produced by `ps -axo command=`) and verifies the parser
// extracts every session ID from `claude --session-id <UUID>` lines
// and ignores everything else.
func TestParseClaudeSessionIDsFromMacOSPS(t *testing.T) {
	output := []byte(`/sbin/launchd
/usr/sbin/syslogd
/opt/homebrew/bin/claude --session-id a95177d2-6c3c-4110-a1a7-9a5f811e9f0d --settings {"hooks":[]}
/opt/homebrew/bin/claude --session-id 5b1bd2df-b161-446e-908d-cc61903aae1d resume
/opt/homebrew/bin/claude --teammate-mode auto
/Applications/Claude.app/Contents/MacOS/Claude
/Users/vladpb/work/Personal/clyde/clyde --demo
zsh
`)
	got := parseClaudeSessionIDs(output)
	want := []session.ID{
		"a95177d2-6c3c-4110-a1a7-9a5f811e9f0d",
		"5b1bd2df-b161-446e-908d-cc61903aae1d",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseClaudeSessionIDs = %v, want %v", got, want)
	}
}

// TestParseClaudeSessionIDsDeduplicates verifies a session ID that
// somehow appears on multiple ps lines (orphan process, fork) is
// reported once.
func TestParseClaudeSessionIDsDeduplicates(t *testing.T) {
	output := []byte(`/opt/homebrew/bin/claude --session-id aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee
/opt/homebrew/bin/claude --session-id aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee --foo
`)
	got := parseClaudeSessionIDs(output)
	if len(got) != 1 || got[0] != "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee" {
		t.Errorf("parseClaudeSessionIDs deduplication failed, got %v", got)
	}
}

// TestParseClaudeSessionIDsEmpty handles a clean process list with no
// claude processes.
func TestParseClaudeSessionIDsEmpty(t *testing.T) {
	output := []byte(`/sbin/launchd
/usr/sbin/syslogd
zsh
`)
	got := parseClaudeSessionIDs(output)
	if len(got) != 0 {
		t.Errorf("parseClaudeSessionIDs on no-claude input = %v, want empty", got)
	}
}

// TestParseClaudeSessionIDsIgnoresNonClaude verifies that a stray
// `--session-id` flag in some unrelated tool's argv doesn't trick the
// parser. Only lines that mention `claude` AND have the flag count.
func TestParseClaudeSessionIDsIgnoresNonClaude(t *testing.T) {
	output := []byte(`some-other-tool --session-id deadbeef-1234-5678-90ab-cdef01234567
`)
	got := parseClaudeSessionIDs(output)
	if len(got) != 0 {
		t.Errorf("parseClaudeSessionIDs on non-claude input = %v, want empty", got)
	}
}
