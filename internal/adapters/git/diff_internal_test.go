package git

import (
	"strings"
	"testing"
)

// countHunkLines counts add/remove lines across all hunks.
func countHunkLines(hunks []Hunk) (adds, dels int) {
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
	return
}

// TestParseDiff_MultiFileDoesNotLeakHeaders is the regression for the HIGH
// finding: with more than one `diff --git` section, the second (and later)
// file's `--- a/f` / `+++ b/f` header lines were misclassified as removed/added
// CONTENT because cur was never reset at a section boundary. The `cur == nil`
// guard only skipped headers before the FIRST hunk.
func TestParseDiff_MultiFileDoesNotLeakHeaders(t *testing.T) {
	t.Parallel()

	raw := []byte(`diff --git a/a.go b/a.go
index 1111111..2222222 100644
--- a/a.go
+++ b/a.go
@@ -1,2 +1,2 @@
 package a
-old a
+new a
diff --git a/b.go b/b.go
index 3333333..4444444 100644
--- a/b.go
+++ b/b.go
@@ -1,2 +1,2 @@
 package b
-old b
+new b
`)

	hunks := parseDiff(raw)

	if len(hunks) != 2 {
		t.Fatalf("want 2 hunks, got %d", len(hunks))
	}

	adds, dels := countHunkLines(hunks)
	if adds != 2 || dels != 2 {
		t.Errorf("counts = +%d -%d, want +2 -2 (header lines leaked into body)", adds, dels)
	}

	// No HunkLine may carry a file-header path — those are the leaked
	// `--- a/b.go` / `+++ b/b.go` lines the old parser produced.
	for hi, h := range hunks {
		for _, l := range h.Lines {
			if strings.Contains(l.Text, "a/b.go") || strings.Contains(l.Text, "b/b.go") ||
				strings.Contains(l.Text, "a/a.go") || strings.Contains(l.Text, "b/a.go") {
				t.Errorf("hunk[%d] leaked a file-header line as content: %q", hi, l.Text)
			}
		}
	}
}

// TestParseDiff_StagedPlusUnstagedConcat mirrors Diff(), which concatenates
// unstaged and staged `git diff` output. The same file appears twice, each in
// its own `diff --git` section; both must parse into independent hunks without
// the second section's headers leaking into the first hunk.
func TestParseDiff_StagedPlusUnstagedConcat(t *testing.T) {
	t.Parallel()

	unstaged := `diff --git a/foo.go b/foo.go
index aaaaaaa..bbbbbbb 100644
--- a/foo.go
+++ b/foo.go
@@ -1,1 +1,1 @@
-unstaged old
+unstaged new
`
	staged := `diff --git a/foo.go b/foo.go
index ccccccc..ddddddd 100644
--- a/foo.go
+++ b/foo.go
@@ -5,1 +5,1 @@
-staged old
+staged new
`
	hunks := parseDiff([]byte(unstaged + staged))

	if len(hunks) != 2 {
		t.Fatalf("want 2 hunks (unstaged + staged), got %d", len(hunks))
	}
	adds, dels := countHunkLines(hunks)
	if adds != 2 || dels != 2 {
		t.Errorf("counts = +%d -%d, want +2 -2", adds, dels)
	}
	for hi, h := range hunks {
		for _, l := range h.Lines {
			if strings.Contains(l.Text, "foo.go") {
				t.Errorf("hunk[%d] leaked a header line as content: %q", hi, l.Text)
			}
		}
	}
}

// TestParseDiff_ContentLinesStartingWithDashesOrPluses guards the positional
// (not textual) header detection: content lines that legitimately begin with
// `--` or `++` sit INSIDE a hunk body and must be preserved verbatim, never
// mistaken for `---`/`+++` file headers.
func TestParseDiff_ContentLinesStartingWithDashesOrPluses(t *testing.T) {
	t.Parallel()

	raw := []byte(`diff --git a/note.md b/note.md
index 1111111..2222222 100644
--- a/note.md
+++ b/note.md
@@ -1,3 +1,3 @@
 keep
--- removed dashes
+++ added pluses
`)

	hunks := parseDiff(raw)
	if len(hunks) != 1 {
		t.Fatalf("want 1 hunk, got %d", len(hunks))
	}

	var gotRemoved, gotAdded string
	for _, l := range hunks[0].Lines {
		switch l.Type {
		case '-':
			gotRemoved = l.Text
		case '+':
			gotAdded = l.Text
		}
	}
	if gotRemoved != "-- removed dashes" {
		t.Errorf("removed line = %q, want %q", gotRemoved, "-- removed dashes")
	}
	if gotAdded != "++ added pluses" {
		t.Errorf("added line = %q, want %q", gotAdded, "++ added pluses")
	}
}

// TestParseDiff_BinaryFileSectionProducesNoHunk ensures a binary file section
// (no @@ hunk, just `Binary files ... differ`) that follows a text file does
// not corrupt or extend the previous hunk.
func TestParseDiff_BinaryFileSectionProducesNoHunk(t *testing.T) {
	t.Parallel()

	raw := []byte(`diff --git a/a.txt b/a.txt
index 1111111..2222222 100644
--- a/a.txt
+++ b/a.txt
@@ -1,1 +1,1 @@
-old
+new
diff --git a/img.png b/img.png
index 3333333..4444444 100644
Binary files a/img.png and b/img.png differ
`)

	hunks := parseDiff(raw)
	if len(hunks) != 1 {
		t.Fatalf("want 1 hunk (binary section yields none), got %d", len(hunks))
	}
	adds, dels := countHunkLines(hunks)
	if adds != 1 || dels != 1 {
		t.Errorf("counts = +%d -%d, want +1 -1", adds, dels)
	}
}
