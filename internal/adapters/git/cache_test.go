package git

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestStatus_CachesWithinTTL — repeated Status calls inside the cache
// window must NOT re-shell-out. This is the perf-audit fix for `git
// status` spawned every snapshot tick (1Hz) on a quiet repo.
func TestStatus_CachesWithinTTL(t *testing.T) {
	t.Parallel()

	calls := 0
	clock := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	canned := []byte(" M file1.go\n?? file2.go\n")

	s := &Source{
		now: func() time.Time { return clock },
		ttl: 3 * time.Second,
		runner: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			calls++
			return canned, nil
		},
	}

	if _, err := s.Status("/repo"); err != nil {
		t.Fatalf("first: %v", err)
	}
	if calls != 1 {
		t.Fatalf("first call: spawned %d, want 1", calls)
	}

	clock = clock.Add(2 * time.Second)
	if _, err := s.Status("/repo"); err != nil {
		t.Fatalf("second: %v", err)
	}
	if calls != 1 {
		t.Errorf("within TTL: spawned %d, want 1 (cached)", calls)
	}

	// Past TTL → new spawn.
	clock = clock.Add(2 * time.Second) // total +4s
	if _, err := s.Status("/repo"); err != nil {
		t.Fatalf("third: %v", err)
	}
	if calls != 2 {
		t.Errorf("past TTL: spawned %d, want 2", calls)
	}
}

// TestStatus_CachePerCwd — calls for different cwds do not collide.
// Important because the same Source is shared across the codebase and
// the user can switch projects (different cwd) without bouncing the
// process.
func TestStatus_CachePerCwd(t *testing.T) {
	t.Parallel()

	calls := map[string]int{}
	s := &Source{
		now: time.Now,
		ttl: time.Hour, // never expires for this test
		runner: func(_ context.Context, dir string, _ ...string) ([]byte, error) {
			calls[dir]++
			return []byte{}, nil
		},
	}

	_, _ = s.Status("/repo-a")
	_, _ = s.Status("/repo-b")
	_, _ = s.Status("/repo-a") // cache hit
	_, _ = s.Status("/repo-b") // cache hit

	if calls["/repo-a"] != 1 || calls["/repo-b"] != 1 {
		t.Errorf("per-cwd cache leaked: calls = %v, want 1 each", calls)
	}
}

// TestStatus_CachesEmptyOnNonGitRepo — when git fails (not a repo), the
// empty result is cached so subsequent ticks don't keep retrying. This
// matters because the user is likely in a non-git project for long
// stretches and we should not spawn a `git status` every second to
// confirm "still not a git repo".
func TestStatus_CachesEmptyOnNonGitRepo(t *testing.T) {
	t.Parallel()

	calls := 0
	s := &Source{
		now: time.Now,
		ttl: time.Hour,
		runner: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			calls++
			return nil, errors.New("not a git repo")
		},
	}

	_, _ = s.Status("/not-a-repo")
	_, _ = s.Status("/not-a-repo")
	_, _ = s.Status("/not-a-repo")

	if calls != 1 {
		t.Errorf("non-git repo: spawned %d, want 1 (negative result must cache)", calls)
	}
}

// TestDiff_CachesWithinTTL — same contract as Status. Diff is the
// HEAVIEST per-tick offender (TWO spawns per call: unstaged + staged).
func TestDiff_CachesWithinTTL(t *testing.T) {
	t.Parallel()

	calls := 0
	clock := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	canned := []byte(`diff --git a/x.go b/x.go
@@ -1,1 +1,1 @@
-old
+new
`)

	s := &Source{
		now: func() time.Time { return clock },
		ttl: 3 * time.Second,
		runner: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			calls++
			return canned, nil
		},
	}

	if _, err := s.Diff("/repo", "x.go"); err != nil {
		t.Fatalf("first: %v", err)
	}
	if calls != 2 { // unstaged + staged
		t.Fatalf("first call: spawned %d, want 2 (unstaged + staged)", calls)
	}

	clock = clock.Add(1 * time.Second)
	if _, err := s.Diff("/repo", "x.go"); err != nil {
		t.Fatalf("second: %v", err)
	}
	if calls != 2 {
		t.Errorf("within TTL: spawned %d, want 2 (cached)", calls)
	}
}

// TestDiff_CachePerFile — different files don't collide.
func TestDiff_CachePerFile(t *testing.T) {
	t.Parallel()

	calls := 0
	s := &Source{
		now: time.Now,
		ttl: time.Hour,
		runner: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			calls++
			return []byte{}, nil
		},
	}

	_, _ = s.Diff("/repo", "a.go")
	_, _ = s.Diff("/repo", "b.go")
	_, _ = s.Diff("/repo", "a.go") // cache hit
	_, _ = s.Diff("/repo", "b.go") // cache hit

	// Each file → 2 spawns first time, 0 on cache hit. Total: 4.
	if calls != 4 {
		t.Errorf("per-file cache leaked: spawned %d, want 4", calls)
	}
}
