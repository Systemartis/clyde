package fsexplorer_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/clyde-tui/clyde/internal/adapters/fsexplorer"
)

// mkDir creates a directory inside root (creates parent dirs as needed).
func mkDir(t *testing.T, root, rel string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, rel), 0o755); err != nil {
		t.Fatalf("mkDir %s: %v", rel, err)
	}
}

// mkFile creates a file with given content at root/rel.
func mkFile(t *testing.T, root, rel, content string) {
	t.Helper()
	abs := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkFile mkdir: %v", err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatalf("mkFile write %s: %v", rel, err)
	}
}

// findNode searches a tree for a node with the given name (BFS).
func findNode(root *fsexplorer.Node, name string) *fsexplorer.Node {
	if root == nil {
		return nil
	}
	if root.Name == name {
		return root
	}
	for _, c := range root.Children {
		if n := findNode(c, name); n != nil {
			return n
		}
	}
	return nil
}

// ─── Tests ────────────────────────────────────────────────────────────────────

// TestWalk_BasicStructure: root node is returned with correct Name and IsDir.
func TestWalk_BasicStructure(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	var s fsexplorer.Source
	root, err := s.Walk(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if root == nil {
		t.Fatal("root should not be nil")
	}
	if !root.IsDir {
		t.Error("root should be a directory")
	}
}

// TestWalk_FilesAndDirs: a simple tree is discovered correctly.
func TestWalk_FilesAndDirs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mkDir(t, dir, "src")
	mkFile(t, dir, "src/main.go", "package main")
	mkFile(t, dir, "src/util.go", "package main")
	mkFile(t, dir, "README.md", "# readme")

	var s fsexplorer.Source
	root, err := s.Walk(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have src dir and README.md at root level.
	if root == nil {
		t.Fatal("root is nil")
	}

	srcNode := findNode(root, "src")
	if srcNode == nil {
		t.Error("expected 'src' directory node")
	} else if !srcNode.IsDir {
		t.Error("src should be a directory")
	}

	mainNode := findNode(root, "main.go")
	if mainNode == nil {
		t.Error("expected 'main.go' node")
	} else if mainNode.IsDir {
		t.Error("main.go should not be a directory")
	}

	readmeNode := findNode(root, "README.md")
	if readmeNode == nil {
		t.Error("expected README.md node")
	}
}

// TestWalk_GitAndDotfiles: .git is always excluded; other dotfiles are visible.
//
// Rationale: users often want to inspect .github, .vscode, .env, etc.
// Only .git internals are excluded by default; all other dot-entries are shown.
// To hide specific dotfiles, add them to .gitignore.
func TestWalk_GitAndDotfiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mkDir(t, dir, ".git")
	mkFile(t, dir, ".git/HEAD", "ref: refs/heads/main")
	mkFile(t, dir, ".env", "SECRET=yes")
	mkDir(t, dir, ".github")
	mkFile(t, dir, ".github/workflows.yml", "# ci")
	mkFile(t, dir, "visible.go", "package main")

	var s fsexplorer.Source
	root, err := s.Walk(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// .git must always be excluded — it's noise, not project files.
	if findNode(root, ".git") != nil {
		t.Error(".git should be excluded")
	}

	// .env and .github are dotfiles but should be visible (not auto-hidden).
	if findNode(root, ".env") == nil {
		t.Error(".env should be visible (dotfiles other than .git are shown)")
	}
	if findNode(root, ".github") == nil {
		t.Error(".github should be visible (dotfiles other than .git are shown)")
	}
	if findNode(root, "visible.go") == nil {
		t.Error("visible.go should be present")
	}
}

// TestWalk_DotfileExcludedViaGitignore: dotfiles listed in .gitignore are hidden.
func TestWalk_DotfileExcludedViaGitignore(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mkFile(t, dir, ".gitignore", ".env\n.DS_Store\n")
	mkFile(t, dir, ".env", "SECRET=yes")
	mkFile(t, dir, ".DS_Store", "mac junk")
	mkFile(t, dir, ".github/workflows.yml", "# ci")
	mkFile(t, dir, "main.go", "package main")

	var s fsexplorer.Source
	root, err := s.Walk(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// .env and .DS_Store are in .gitignore — must be excluded.
	if findNode(root, ".env") != nil {
		t.Error(".env should be excluded via .gitignore")
	}
	if findNode(root, ".DS_Store") != nil {
		t.Error(".DS_Store should be excluded via .gitignore")
	}

	// .github is not in .gitignore — must be visible.
	if findNode(root, ".github") == nil {
		t.Error(".github should be visible (not in .gitignore)")
	}
	if findNode(root, "main.go") == nil {
		t.Error("main.go should be present")
	}
}

// TestWalk_GitignoreRespected: files matching .gitignore patterns are excluded.
func TestWalk_GitignoreRespected(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mkFile(t, dir, ".gitignore", "node_modules\n*.log\nbuild/\n")
	mkDir(t, dir, "node_modules")
	mkFile(t, dir, "node_modules/pkg.js", "// ignored")
	mkFile(t, dir, "debug.log", "log content")
	mkDir(t, dir, "build")
	mkFile(t, dir, "build/app.js", "// ignored")
	mkFile(t, dir, "src/main.go", "package main")

	var s fsexplorer.Source
	root, err := s.Walk(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if findNode(root, "node_modules") != nil {
		t.Error("node_modules should be gitignored")
	}
	if findNode(root, "debug.log") != nil {
		t.Error("debug.log should be gitignored (*.log pattern)")
	}
	if findNode(root, "build") != nil {
		t.Error("build/ directory should be gitignored")
	}
	if findNode(root, "main.go") == nil {
		t.Error("src/main.go should be visible")
	}
}

// TestWalk_PathRelative: node.Path is relative to walk root.
func TestWalk_PathRelative(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mkDir(t, dir, "pkg")
	mkFile(t, dir, "pkg/util.go", "package pkg")

	var s fsexplorer.Source
	root, err := s.Walk(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	util := findNode(root, "util.go")
	if util == nil {
		t.Fatal("util.go not found")
	}
	if util.Path != "pkg/util.go" {
		t.Errorf("Path = %q, want pkg/util.go", util.Path)
	}
}

// TestWalk_EmptyDir: an empty directory returns root with no children, no error.
func TestWalk_EmptyDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	var s fsexplorer.Source
	root, err := s.Walk(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if root == nil {
		t.Fatal("root should not be nil for empty dir")
	}
	if len(root.Children) != 0 {
		t.Errorf("children = %d, want 0", len(root.Children))
	}
}

// TestWalk_DeepTreeCapped: depth > 5 levels is not traversed.
func TestWalk_DeepTreeCapped(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Create a/b/c/d/e/f/g.go — 6 levels deep (exceeds maxDepth=5).
	mkFile(t, dir, "a/b/c/d/e/f/g.go", "package deep")

	var s fsexplorer.Source
	root, err := s.Walk(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// g.go at depth 6 should be excluded.
	if findNode(root, "g.go") != nil {
		t.Error("g.go at depth 6 should be excluded by maxDepth cap")
	}
}

// TestWalk_IDEStyleOrdering verifies the explorer returns directories before
// files at every level, with case-insensitive alphabetical order within each
// group. Matches the conventional layout in VS Code, IntelliJ, etc.
//
// Without this, ReadDir's case-sensitive ASCII order interleaves dotfiles,
// uppercase entries, and mixed-case directories in a way the user can't scan.
func TestWalk_IDEStyleOrdering(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Mirror the user's exact reported situation.
	mkDir(t, dir, ".atl")
	mkDir(t, dir, ".github")
	mkFile(t, dir, ".gitignore", "")
	mkFile(t, dir, ".golangci.yml", "")
	mkFile(t, dir, "README.md", "")
	mkFile(t, dir, "Screenshot.png", "")
	mkDir(t, dir, "cmd")
	mkDir(t, dir, "docs")
	mkFile(t, dir, "go.mod", "")

	var s fsexplorer.Source
	root, err := s.Walk(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Expected order: directories first (case-insensitive alpha), then files
	// (case-insensitive alpha). Mixing case should not split the groups —
	// e.g. README.md and go.mod cluster together regardless of case.
	want := []string{
		// dirs (case-insensitive alpha):
		".atl", ".github", "cmd", "docs",
		// files (case-insensitive alpha):
		".gitignore", ".golangci.yml", "go.mod", "README.md", "Screenshot.png",
	}

	if len(root.Children) != len(want) {
		t.Fatalf("got %d children, want %d", len(root.Children), len(want))
	}
	for i, name := range want {
		if root.Children[i].Name != name {
			t.Errorf("Children[%d] = %q, want %q (full order: %v)",
				i, root.Children[i].Name, name, namesOf(root.Children))
		}
	}
}

// TestWalk_OrderingRecursive verifies the dirs-first ordering applies at every
// level, not just the top. A subdirectory with mixed entries must also list
// dirs first then files.
func TestWalk_OrderingRecursive(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mkDir(t, dir, "pkg/zsub")
	mkDir(t, dir, "pkg/asub")
	mkFile(t, dir, "pkg/Z.go", "")
	mkFile(t, dir, "pkg/a.go", "")

	var s fsexplorer.Source
	root, err := s.Walk(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pkg := findNode(root, "pkg")
	if pkg == nil {
		t.Fatal("pkg dir missing")
	}

	want := []string{"asub", "zsub", "a.go", "Z.go"}
	if len(pkg.Children) != len(want) {
		t.Fatalf("got %d children, want %d (got %v)",
			len(pkg.Children), len(want), namesOf(pkg.Children))
	}
	for i, name := range want {
		if pkg.Children[i].Name != name {
			t.Errorf("pkg.Children[%d] = %q, want %q", i, pkg.Children[i].Name, name)
		}
	}
}

// namesOf returns the Name fields of nodes in order — handy for error messages.
func namesOf(nodes []*fsexplorer.Node) []string {
	out := make([]string, len(nodes))
	for i, n := range nodes {
		out[i] = n.Name
	}
	return out
}

// TestWalk_AllTopLevelDirsVisible is a regression test for the issue where only
// 3 of 7 top-level directories appeared in the explorer.
//
// Root cause: walkDir used a single shared counter and recursed into each
// subdirectory immediately after creating its node. If the first few dirs had
// many children the counter was exhausted before subsequent siblings were added.
//
// Fix: two-pass traversal — first create all sibling nodes at each level (no
// cap at depth 0), then recurse. This ensures every immediate child of the cwd
// is always returned regardless of how large sub-trees are.
func TestWalk_AllTopLevelDirsVisible(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Mirror the user's situation: 7 top-level entries, each sub-dir contains
	// enough files to exhaust the entry cap on its own.
	topLevel := []string{
		".DS_Store_fake", // file, not gitignored
		"BorderGen",
		"clyde",
		"cocopilot",
		"diary",
		"diary2",
		"Nexus",
	}

	// Create each top-level entry; populate sub-dirs with many files so the
	// old counter-based walk would run out of budget early.
	for _, name := range topLevel {
		if name == ".DS_Store_fake" {
			mkFile(t, dir, name, "mac artifact")
			continue
		}
		// Each dir gets 80 files — enough that even one dir exhausts the old
		// per-subtree budget (sub-budget was implicitly shared across all dirs).
		for i := range 80 {
			mkFile(t, dir, name+"/"+fmt.Sprintf("file%03d.go", i), "package p")
		}
	}

	var s fsexplorer.Source
	root, err := s.Walk(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Every top-level name must appear as a direct child of root.
	for _, name := range topLevel {
		if findNode(root, name) == nil {
			t.Errorf("top-level entry %q missing from walk; only %d children found",
				name, len(root.Children))
		}
	}
}
