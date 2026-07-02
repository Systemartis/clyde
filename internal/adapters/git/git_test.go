package git_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Systemartis/clyde/internal/adapters/git"
)

// initRepo creates a fresh git repo in dir, sets dummy user config, and commits
// an initial empty commit so HEAD is valid.
func initRepo(t *testing.T, dir string) {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	run("commit", "--allow-empty", "-m", "init")
}

// writeFile creates a file with given content inside dir.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("writeFile %s: %v", name, err)
	}
}

// ─── Status ───────────────────────────────────────────────────────────────────

// TestStatus_EmptyRepo: a clean repo returns no file statuses.
func TestStatus_EmptyRepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initRepo(t, dir)

	var s git.Source
	statuses, err := s.Status(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(statuses) != 0 {
		t.Errorf("want 0 statuses, got %d", len(statuses))
	}
}

// TestStatus_ModifiedFile: a modified tracked file appears as 'M'.
func TestStatus_ModifiedFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initRepo(t, dir)

	// Stage and commit a file first.
	writeFile(t, dir, "hello.go", "package main\n")
	run := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("add", "hello.go")
	run("commit", "-m", "add hello.go")

	// Now modify it without staging.
	writeFile(t, dir, "hello.go", "package main\n// changed\n")

	var s git.Source
	statuses, err := s.Status(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("want 1 status, got %d", len(statuses))
	}
	st := statuses[0]
	if st.Path != "hello.go" {
		t.Errorf("path = %q, want hello.go", st.Path)
	}
	if st.Status != 'M' {
		t.Errorf("status = %c, want M", st.Status)
	}
	if st.Staged {
		t.Error("staged = true, want false (worktree change)")
	}
}

// TestStatus_UntrackedFile: an untracked file appears with status '?'.
func TestStatus_UntrackedFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initRepo(t, dir)
	writeFile(t, dir, "new.go", "package main\n")

	var s git.Source
	statuses, err := s.Status(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("want 1 status, got %d", len(statuses))
	}
	if statuses[0].Status != '?' {
		t.Errorf("status = %c, want ?", statuses[0].Status)
	}
}

// TestStatus_StagedFile: a staged new file has Staged=true and status 'A'.
func TestStatus_StagedFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initRepo(t, dir)
	writeFile(t, dir, "staged.go", "package main\n")

	run := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("add", "staged.go")

	var s git.Source
	statuses, err := s.Status(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("want 1 status, got %d", len(statuses))
	}
	st := statuses[0]
	if st.Status != 'A' {
		t.Errorf("status = %c, want A", st.Status)
	}
	if !st.Staged {
		t.Error("staged = false, want true")
	}
}

// TestStatus_NotARepo: non-git directory returns empty slice, no error.
func TestStatus_NotARepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir() // no git init

	var s git.Source
	statuses, err := s.Status(dir)
	if err != nil {
		t.Fatalf("expected nil error for non-repo, got: %v", err)
	}
	if len(statuses) != 0 {
		t.Errorf("expected empty slice for non-repo, got %d statuses", len(statuses))
	}
}

// TestStatus_NonASCIIAndSpacePaths verifies (end-to-end, via real git) that
// non-ASCII and space-containing filenames come back verbatim rather than
// C-quoted (e.g. "caf\303\251.txt"). This exercises the `-z` porcelain path.
func TestStatus_NonASCIIAndSpacePaths(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	initRepo(t, dir)
	writeFile(t, dir, "café.txt", "x\n")
	writeFile(t, dir, "with space.txt", "y\n")

	var s git.Source
	statuses, err := s.Status(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := map[string]bool{"café.txt": false, "with space.txt": false}
	for _, st := range statuses {
		if _, ok := want[st.Path]; ok {
			want[st.Path] = true
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("status missing verbatim path %q (got %+v)", name, statuses)
		}
	}
}

// TestStatus_StagedRename verifies a staged rename yields a single entry with
// the NEW path (the trailing NUL-separated old-path field is discarded).
func TestStatus_StagedRename(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	initRepo(t, dir)
	writeFile(t, dir, "orig.txt", "hello\n")
	run := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(context.Background(), "git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("add", "orig.txt")
	run("commit", "-m", "add orig")
	run("mv", "orig.txt", "renamed.txt")

	var s git.Source
	statuses, err := s.Status(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("want 1 status for a rename, got %d (%+v)", len(statuses), statuses)
	}
	st := statuses[0]
	if st.Path != "renamed.txt" {
		t.Errorf("path = %q, want renamed.txt (old-path field should be discarded)", st.Path)
	}
	if st.Status != 'R' {
		t.Errorf("status = %c, want R", st.Status)
	}
	if !st.Staged {
		t.Error("staged = false, want true")
	}
}

// ─── Branch ───────────────────────────────────────────────────────────────────

// TestBranch_MainBranch: fresh repo on default branch returns a branch name.
func TestBranch_MainBranch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initRepo(t, dir)

	var s git.Source
	branch, dirty, err := s.Branch(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if branch == "" {
		t.Error("branch should be non-empty on a fresh repo")
	}
	if dirty {
		t.Error("dirty should be false on a clean repo")
	}
}

// TestBranch_DirtyRepo: uncommitted changes make dirty=true.
func TestBranch_DirtyRepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initRepo(t, dir)
	writeFile(t, dir, "untracked.go", "package main\n")

	var s git.Source
	_, dirty, err := s.Branch(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !dirty {
		t.Error("dirty should be true when untracked files exist")
	}
}

// TestBranch_NotARepo: non-git directory returns empty branch, no error.
func TestBranch_NotARepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	var s git.Source
	branch, dirty, err := s.Branch(dir)
	if err != nil {
		t.Fatalf("expected nil error for non-repo, got: %v", err)
	}
	if branch != "" {
		t.Errorf("expected empty branch for non-repo, got %q", branch)
	}
	if dirty {
		t.Error("expected dirty=false for non-repo")
	}
}
