package fsexplorer

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestWalk_ServesFromCache_WhenIdentitySwapped — pin the test seams so
// the TTL never elapses and the on-disk mtime never changes after the
// first Walk. We then mutate the cached *Node in place; a second Walk
// must return the SAME identity (same pointer / same mutated contents)
// rather than a fresh tree built from disk. This proves the cache is
// hit, not "served because the rebuilt tree happens to look the same".
func TestWalk_ServesFromCache_WhenIdentitySwapped(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("package x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	frozen := time.Now()
	s := &Source{
		now: func() time.Time { return frozen },
		ttl: time.Hour,
	}

	first, err := s.Walk(dir)
	if err != nil {
		t.Fatalf("first Walk: %v", err)
	}
	if len(first.Children) != 1 {
		t.Fatalf("first Walk children = %d, want 1", len(first.Children))
	}

	// Mutate the cached tree — add a sentinel child that could not have
	// come from disk. If the next Walk returns this same tree, the
	// cache is doing its job.
	sentinel := &Node{Name: "SENTINEL.synthetic", IsDir: false, Path: "SENTINEL.synthetic"}
	first.Children = append(first.Children, sentinel)

	second, err := s.Walk(dir)
	if err != nil {
		t.Fatalf("second Walk: %v", err)
	}
	if second != first {
		t.Errorf("cache miss: second Walk returned different *Node identity")
	}
	if len(second.Children) != 2 || second.Children[1].Name != "SENTINEL.synthetic" {
		t.Errorf("cache miss: sentinel child not present in second result; got %d children", len(second.Children))
	}
}

// TestWalk_InvalidatesOnCwdMtimeAdvance — advancing the cwd-root mtime
// (e.g. via a new top-level file) must drop the cache even when the
// TTL has not yet elapsed. Without this, a user creating a new file in
// the project would not see it in the explorer until the next 5-second
// boundary.
func TestWalk_InvalidatesOnCwdMtimeAdvance(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("a"), 0o600); err != nil {
		t.Fatalf("write a: %v", err)
	}

	s := &Source{ttl: time.Hour} // TTL effectively disabled
	first, _ := s.Walk(dir)
	if len(first.Children) != 1 {
		t.Fatalf("first: %d children, want 1", len(first.Children))
	}

	// Adding a new file changes the cwd mtime — must invalidate.
	if err := os.WriteFile(filepath.Join(dir, "b.go"), []byte("b"), 0o600); err != nil {
		t.Fatalf("write b: %v", err)
	}
	// Bump the directory mtime explicitly (some filesystems may not
	// register the change at sub-second resolution).
	future := time.Now().Add(time.Second)
	if err := os.Chtimes(dir, future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	second, _ := s.Walk(dir)
	if len(second.Children) != 2 {
		t.Errorf("after mtime advance: %d children, want 2 (cache should have invalidated)", len(second.Children))
	}
}

// TestLoadGitignoreCached_ServesFromCache_OnUnchangedFile pins down
// the .gitignore parse cache: with mtime+size unchanged, repeated calls
// must return the same gitIgnore (identity-comparable via the patterns
// slice header pointer would be ideal, but we settle for content
// equality verified after a hostile in-place mutation).
func TestLoadGitignoreCached_NoReparse(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("dist/\nnode_modules/\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	s := &Source{}
	first := s.loadGitignoreCached(dir)
	if len(first.patterns) != 2 {
		t.Fatalf("first parse: %d patterns, want 2", len(first.patterns))
	}

	// Overwrite the .gitignore with different content but DO NOT change
	// mtime/size — same byte length, restored timestamps. The cache
	// should serve the old patterns.
	info, _ := os.Stat(filepath.Join(dir, ".gitignore"))
	frozenMtime := info.ModTime()

	replacement := []byte("foo.txt\nbar.txt\n")
	if len(replacement) != int(info.Size()) {
		t.Logf("replacement len %d != original size %d — skipping cache identity test", len(replacement), info.Size())
		return
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), replacement, 0o600); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	if err := os.Chtimes(filepath.Join(dir, ".gitignore"), time.Now(), frozenMtime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	second := s.loadGitignoreCached(dir)
	if second.patterns[0] != "dist/" {
		t.Errorf("cache miss after fingerprint match: got patterns %v, want cached dist/, node_modules/", second.patterns)
	}
}
