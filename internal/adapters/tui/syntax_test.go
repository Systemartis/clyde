package tui

import (
	"strings"
	"testing"
)

// TestHighlightCode_LexerSelection verifies the right lexer fires for the
// languages the user explicitly requested (md, html, js, ts, tsx, jsx, css,
// go, rs, cpp, c, py). For each, a recognisable keyword or token must come
// out wrapped in ANSI codes.
func TestHighlightCode_LexerSelection(t *testing.T) {
	t.Parallel()
	p := TokyoNightPalette()
	cases := []struct {
		name    string
		path    string
		content string
		// substr is the visible (ANSI-stripped) text we expect to find — confirms
		// we did not lose tokens during line splitting.
		substr string
	}{
		{"go", "main.go", "package main\nfunc main() {}\n", "func main"},
		{"ts", "app.ts", "const x: number = 1\n", "const x"},
		{"tsx", "App.tsx", "const X = () => <div/>\n", "const X"},
		{"jsx", "App.jsx", "const X = () => <div/>\n", "const X"},
		{"js", "lib.js", "function add(a, b) { return a + b }\n", "function add"},
		{"py", "main.py", "def main():\n    return 1\n", "def main"},
		{"rs", "lib.rs", "fn main() {}\n", "fn main"},
		{"c", "main.c", "int main(void) { return 0; }\n", "int main"},
		{"cpp", "main.cpp", "int main() { return 0; }\n", "int main"},
		{"css", "site.css", ".x { color: red; }\n", "color"},
		{"html", "index.html", "<html><body>hi</body></html>\n", "<html>"},
		{"md", "README.md", "# Title\n\nbody\n", "Title"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			lines, ok := highlightCode(tc.content, tc.path, p)
			if !ok {
				t.Fatalf("highlightCode returned ok=false for %s", tc.path)
			}
			joined := strings.Join(lines, "\n")
			if !strings.Contains(stripANSI(joined), tc.substr) {
				t.Errorf("missing visible substr %q in highlighted output (visible: %q)",
					tc.substr, stripANSI(joined))
			}
			// At least one line must carry ANSI styling — otherwise we picked
			// a lexer that recognised nothing styleable, which counts as
			// regression for any of the requested languages.
			if !strings.Contains(joined, "\x1b[") {
				t.Errorf("no ANSI codes in highlighted output for %s — palette mapping not engaging", tc.path)
			}
		})
	}
}

// TestHighlightCode_LineCountMatchesContent verifies highlighter output has
// one entry per source line, regardless of whether the file ends with a
// newline. Without this guarantee the viewer's line-number prefix loop
// would drift out of sync with the source.
func TestHighlightCode_LineCountMatchesContent(t *testing.T) {
	t.Parallel()
	p := TokyoNightPalette()
	cases := []struct {
		name    string
		content string
		want    int
	}{
		{"trailing-nl", "package main\nfunc f() {}\n", 3},
		{"no-trailing-nl", "package main\nfunc f() {}", 2},
		{"single-line", "package main", 1},
		{"empty", "", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			lines, ok := highlightCode(tc.content, "x.go", p)
			if !ok {
				t.Fatalf("ok=false")
			}
			if len(lines) != tc.want {
				t.Errorf("got %d lines, want %d (lines=%q)", len(lines), tc.want, lines)
			}
		})
	}
}

// TestHasLexerFor verifies the cheap is-supported check matches the requested
// language list.
func TestHasLexerFor(t *testing.T) {
	t.Parallel()
	supported := []string{
		"f.go", "f.ts", "f.tsx", "f.jsx", "f.js", "f.py",
		"f.rs", "f.c", "f.cpp", "f.css", "f.html", "f.md",
	}
	for _, p := range supported {
		if !hasLexerFor(p) {
			t.Errorf("hasLexerFor(%q) = false, want true", p)
		}
	}
	// Negative: a plain text file with no extension should NOT register
	// (we don't want chroma to guess from short content).
	if hasLexerFor("notes") {
		t.Error(`hasLexerFor("notes") = true, want false (no extension)`)
	}
}

// TestHighlightCode_MultilineStringDoesNotLeakStyles verifies that when a
// token spans multiple lines (e.g. a triple-quoted Python string or a Go
// raw string literal), each emitted line is independently styled and ANSI
// reset codes do not bleed into the line-number column.
//
// Concretely: every output line should be either fully un-styled or wrapped
// in matching open/close SGR pairs. We assert that no line ends mid-escape.
func TestHighlightCode_MultilineStringDoesNotLeakStyles(t *testing.T) {
	t.Parallel()
	p := TokyoNightPalette()
	content := "x = \"\"\"first\nsecond\nthird\"\"\"\n"
	lines, ok := highlightCode(content, "f.py", p)
	if !ok {
		t.Fatalf("ok=false")
	}
	for i, line := range lines {
		if strings.HasSuffix(line, "\x1b") || strings.HasSuffix(line, "\x1b[") {
			t.Errorf("line %d ends mid-escape: %q", i, line)
		}
	}
}
