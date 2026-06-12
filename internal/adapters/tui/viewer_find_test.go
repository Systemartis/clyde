package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestFindInBuffer_BasicAndCaseInsensitive(t *testing.T) {
	t.Parallel()
	matches := findInBuffer("Hello World\nhello there", "hello")
	if len(matches) != 2 {
		t.Fatalf("got %d matches, want 2 (case-insensitive)", len(matches))
	}
	if matches[0].Line != 0 || matches[0].Col != 0 || matches[0].End != 5 {
		t.Errorf("match[0] = %+v, want (0, 0, 5)", matches[0])
	}
	if matches[1].Line != 1 || matches[1].Col != 0 || matches[1].End != 5 {
		t.Errorf("match[1] = %+v, want (1, 0, 5)", matches[1])
	}
}

func TestFindInBuffer_OverlappingAndEmpty(t *testing.T) {
	t.Parallel()
	if got := findInBuffer("abc", ""); len(got) != 0 {
		t.Errorf("empty query should return nothing, got %d", len(got))
	}
	// "aa" in "aaaa" should yield 3 overlapping matches at offsets 0, 1, 2.
	matches := findInBuffer("aaaa", "aa")
	if len(matches) != 3 {
		t.Errorf("got %d, want 3 overlapping matches", len(matches))
	}
}

func TestFind_PromptAndNavigation(t *testing.T) {
	t.Parallel()
	m, _ := editTestModel(t, "alpha\nbeta gamma alpha\ndelta\n")
	// Open prompt with `/`.
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: '/'})
	if !m.viewerFindActive {
		t.Fatal("expected viewerFindActive after /")
	}
	for _, r := range []rune{'a', 'l', 'p', 'h', 'a'} {
		m = mustUpdate(t, m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.viewerFindActive {
		t.Error("Enter should close the prompt")
	}
	if got := len(m.viewerFindMatches); got != 2 {
		t.Fatalf("got %d matches, want 2", got)
	}
	if m.viewerFindIdx != 0 {
		t.Errorf("idx = %d, want 0 after Enter", m.viewerFindIdx)
	}
	// n advances; N steps back.
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'n'})
	if m.viewerFindIdx != 1 {
		t.Errorf("after n: idx = %d, want 1", m.viewerFindIdx)
	}
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'N'})
	if m.viewerFindIdx != 0 {
		t.Errorf("after N: idx = %d, want 0", m.viewerFindIdx)
	}
}

func TestFind_NoMatchesReportsStatus(t *testing.T) {
	t.Parallel()
	m, _ := editTestModel(t, "hello world\n")
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: '/'})
	for _, r := range "zzzz" {
		m = mustUpdate(t, m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(m.viewerFindMatches) != 0 {
		t.Error("expected no matches")
	}
	if m.viewerStatus == "" {
		t.Error("expected a status message for no-matches case")
	}
}

func TestFind_TabTogglesReplaceField(t *testing.T) {
	t.Parallel()
	m, _ := editTestModel(t, "x")
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: '/'})
	if m.viewerFindFocusReplace {
		t.Fatal("setup: focus should start on find field")
	}
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'a', Text: "a"})
	if m.viewerFindQuery != "a" || m.viewerFindReplace != "" {
		t.Errorf("after typing into find: query=%q replace=%q", m.viewerFindQuery, m.viewerFindReplace)
	}
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyTab})
	if !m.viewerFindFocusReplace {
		t.Error("Tab should switch focus to replace field")
	}
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: 'b', Text: "b"})
	if m.viewerFindQuery != "a" || m.viewerFindReplace != "b" {
		t.Errorf("after typing into replace: query=%q replace=%q", m.viewerFindQuery, m.viewerFindReplace)
	}
}

func TestFind_EnterWithReplaceRunsSubstitute(t *testing.T) {
	t.Parallel()
	m, _ := editTestModel(t, "foo bar foo\n")
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: '/'})
	for _, r := range "foo" {
		m = mustUpdate(t, m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyTab})
	for _, r := range "X" {
		m = mustUpdate(t, m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := m.viewerEdit.String(); got != "X bar X\n" {
		t.Errorf("buffer = %q, want %q", got, "X bar X\n")
	}
	if !m.viewerDirty {
		t.Error("substitute should mark dirty")
	}
}

func TestFind_EnterWithoutReplaceJustFinds(t *testing.T) {
	t.Parallel()
	m, _ := editTestModel(t, "alpha alpha alpha")
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: '/'})
	for _, r := range "alpha" {
		m = mustUpdate(t, m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(m.viewerFindMatches) != 3 {
		t.Errorf("got %d matches, want 3", len(m.viewerFindMatches))
	}
	if m.viewerEdit.String() != "alpha alpha alpha" {
		t.Error("buffer should not change on find-only")
	}
	if m.viewerDirty {
		t.Error("find-only should not mark dirty")
	}
}

func TestFind_EscCancelsPrompt(t *testing.T) {
	t.Parallel()
	m, _ := editTestModel(t, "x")
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: '/'})
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.viewerFindActive {
		t.Error("Esc should cancel find prompt")
	}
}

// TestFind_EscDoesNotCloseViewer is the regression for the reported bug
// where Esc-from-find closed the entire viewer instead of just the
// prompt. handleKey's Esc branch was firing handleEscapeKey directly
// without checking viewerFindActive first.
func TestFind_EscDoesNotCloseViewer(t *testing.T) {
	t.Parallel()
	m, _ := editTestModel(t, "x")
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: '/'})
	if !m.viewerActive || !m.viewerFindActive {
		t.Fatal("setup")
	}
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.viewerFindActive {
		t.Error("Esc should cancel find prompt")
	}
	if !m.viewerActive {
		t.Error("Esc-from-find should NOT close the viewer")
	}
}

// TestFind_EscFromCommandPromptKeepsViewer mirrors the find Esc test for
// the :command prompt — same dispatch-order bug class, same fix.
func TestFind_EscFromCommandPromptKeepsViewer(t *testing.T) {
	t.Parallel()
	m, _ := editTestModel(t, "x")
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: ':'})
	if !m.viewerCmdActive {
		t.Fatal("setup")
	}
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.viewerCmdActive {
		t.Error("Esc should cancel :command prompt")
	}
	if !m.viewerActive {
		t.Error("Esc-from-:command should NOT close the viewer")
	}
}

// TestFind_PlaceholdersRenderedWhenEmpty verifies the placeholder hints
// surface in the prompt body so the user knows what each row does
// before typing. Strips ANSI for visible-text comparison.
func TestFind_PlaceholdersRenderedWhenEmpty(t *testing.T) {
	t.Parallel()
	m, _ := editTestModel(t, "x")
	m = mustUpdate(t, m, tea.KeyPressMsg{Code: '/'})
	out := stripANSI(m.renderViewerPanel(120, 25))
	if !strings.Contains(out, "find text") {
		t.Errorf("expected 'find text' placeholder, output: %q", out[max(0, len(out)-300):])
	}
	if !strings.Contains(out, "replace with") {
		t.Errorf("expected 'replace with' placeholder, output: %q", out[max(0, len(out)-300):])
	}
}
