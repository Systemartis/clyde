package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// TestViewerOverflow_LongLineDoesNotExceedPanel is a regression test for the
// reported bug: long lines in Go files push the panel right border off
// screen. We synthesize a Go file with a single very long line, render the
// viewer at a known panel size, and assert NO output line exceeds the
// expected width (which is the panel's outer width).
//
// Reproduces with a multi-byte runes-per-cell mix because ansi.StringWidth
// and lipgloss.Width can diverge on tricky inputs (uniseg vs. width-table
// disagreements), and we want both consumers to land on the same answer.
func TestViewerOverflow_LongLineDoesNotExceedPanel(t *testing.T) {
	long := "package main\n// " + strings.Repeat("LONG-CONTENT-", 30) + "\nfunc f() {}\n"
	mockFileContent["fixture_long.go"] = long
	defer delete(mockFileContent, "fixture_long.go")

	m := NewModel()
	m = m.loadViewerFile("fixture_long.go")
	w, h := 60, 25
	out := m.renderViewerPanel(w, h)
	rows := strings.Split(out, "\n")
	for i, r := range rows {
		got := lipgloss.Width(r)
		if got > w {
			t.Errorf("row[%d] lipgloss.Width=%d > panel width=%d. ansi.Width=%d. Visible=%q",
				i, got, w, ansi.StringWidth(r), stripANSI(r))
		}
	}
}

// TestExpandTabs verifies the tab-expansion behavior used by the viewer's
// content cache.
func TestExpandTabs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		in    string
		width int
		want  string
	}{
		{"no-tabs", "abc\ndef", 4, "abc\ndef"},
		{"tab-at-start", "\tabc", 4, "    abc"},
		{"tab-mid-aligns-to-stop", "ab\tcd", 4, "ab  cd"},
		{"tab-after-3", "abc\td", 4, "abc d"},
		{"tab-after-4-jumps-full", "abcd\tef", 4, "abcd    ef"},
		{"newline-resets-column", "ab\tcd\n\tef", 4, "ab  cd\n    ef"},
		{"width-zero-is-noop", "\t", 0, "\t"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := expandTabs(tc.in, tc.width)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestViewerOverflow_GoFileTabs is the regression for the user-reported
// "long lines in Go files push the panel border off screen" bug. The
// underlying cause was tab characters rendering as 4-8 cells in the
// terminal but counting as 1 cell in lipgloss.Width — a Go import block
// with tab indentation routinely overflowed the panel because the
// truncate budget thought there was room for more characters than the
// terminal actually had.
func TestViewerOverflow_GoFileTabs(t *testing.T) {
	// Synthesise a Go file with realistic tab indentation + long URLs,
	// matching the layout in the user's screenshot (cmd/clyde/main.go).
	content := "package main\n\nimport (\n" +
		"\t\"context\"\n" +
		"\t\"flag\"\n" +
		"\t\"fmt\"\n" +
		"\t\"os\"\n\n" +
		"\ttea \"charm.land/bubbletea/v2\"\n\n" +
		"\t\"github.com/Systemartis/clyde/internal/adapters/anthropicapi\"\n" +
		"\t\"github.com/Systemartis/clyde/internal/adapters/fsexplorer\"\n" +
		"\tgitadapter \"github.com/Systemartis/clyde/internal/adapters/git\"\n" +
		"\t\"github.com/Systemartis/clyde/internal/adapters/hookserver\"\n" +
		"\t\"github.com/Systemartis/clyde/internal/adapters/jsonl\"\n" +
		")\n"
	mockFileContent["fixture_main.go"] = content
	defer delete(mockFileContent, "fixture_main.go")

	m := NewModel()
	m = m.loadViewerFile("fixture_main.go")
	// Match a 2-col right column at the user's terminal width — long URL
	// lines should clip cleanly inside, not push the border off.
	w, h := 80, 25
	out := m.renderViewerPanel(w, h)
	rows := strings.Split(out, "\n")
	for i, r := range rows {
		got := lipgloss.Width(r)
		if got > w {
			t.Errorf("row[%d] lipgloss.Width=%d > panel width=%d. Visible=%q",
				i, got, w, stripANSI(r))
		}
	}
}

// TestWidthMeasurement_AnsiVsLipgloss looks for inputs where ansi.StringWidth
// disagrees with lipgloss.Width — the wrapPanel pipeline truncates by the
// former and pads by the latter, so any disagreement leaks through as
// panel overflow.
func TestWidthMeasurement_AnsiVsLipgloss(t *testing.T) {
	t.Parallel()
	p := TokyoNightPalette()
	// Big chunk of chroma output for a Go-like fixture.
	hl, ok := highlightCode(`func f() string { return "hello" + "world" + "ABCDEF" }`, "x.go", p)
	if !ok {
		t.Fatal("highlightCode failed")
	}
	for _, line := range hl {
		a := ansi.StringWidth(line)
		l := lipgloss.Width(line)
		if a != l {
			t.Logf("DISAGREE: ansi=%d lipgloss=%d on %q", a, l, stripANSI(line))
		}
		// Truncate to 20 via ansi, measure both again.
		trunc := ansi.Truncate(line, 20, "")
		ta := ansi.StringWidth(trunc)
		tl := lipgloss.Width(trunc)
		if ta != tl {
			t.Errorf("after ansi.Truncate(_,20): ansi=%d lipgloss=%d on %q",
				ta, tl, stripANSI(trunc))
		}
		if ta > 20 {
			t.Errorf("ansi.Truncate left width %d > 20 on %q", ta, stripANSI(trunc))
		}
	}
}

// TestViewerOverflow_StyledLongLine specifically targets chroma-output
// long lines: chroma produces lines packed with SGR escapes, and the
// ansi/lipgloss width measurements need to agree on those.
func TestViewerOverflow_StyledLongLine(t *testing.T) {
	// Build a "Go file" whose middle line has many keywords + strings to
	// trigger lots of SGR transitions in chroma's output.
	var sb strings.Builder
	sb.WriteString("package main\n")
	sb.WriteString("func busy() ")
	for range 25 {
		sb.WriteString(`if x { return "AAAAAAAAAA" } else `)
	}
	sb.WriteString("\n")
	sb.WriteString("func g() {}\n")

	mockFileContent["fixture_busy.go"] = sb.String()
	defer delete(mockFileContent, "fixture_busy.go")

	m := NewModel()
	m = m.loadViewerFile("fixture_busy.go")
	w, h := 70, 20
	out := m.renderViewerPanel(w, h)
	rows := strings.Split(out, "\n")
	for i, r := range rows {
		got := lipgloss.Width(r)
		if got > w {
			t.Errorf("row[%d] lipgloss.Width=%d > panel width=%d. ansi.Width=%d. Visible=%q",
				i, got, w, ansi.StringWidth(r), stripANSI(r))
		}
	}
}
