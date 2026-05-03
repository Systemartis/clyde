package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestExplorerSearch_Integration_TypeAndOpen drives the search overlay
// through the public Update path: enter explorer active mode, hit '/',
// type a query, ↓ to second match, Enter — the viewer should open at
// the second match's path.
func TestExplorerSearch_Integration_TypeAndOpen(t *testing.T) {
	t.Parallel()
	m := NewModel()
	// Replace mock data with our deterministic fixture so the assertions
	// don't depend on the v3 mock content.
	m.data = fixtureMockData()
	m = m.setFocus(PanelExplorer)
	m.activePanelID = PanelExplorer

	send := func(code rune, mod tea.KeyMod) {
		t.Helper()
		next, _ := m.Update(tea.KeyPressMsg{Code: code, Mod: mod})
		m = next.(Model)
	}

	send('/', 0)
	if !m.explorer.search.Active {
		t.Fatal("search should be active after '/'")
	}
	for _, r := range []rune{'.', 't', 's'} {
		send(r, 0)
	}
	if got := m.explorer.search.Query; got != ".ts" {
		t.Fatalf("query = %q, want %q", got, ".ts")
	}
	if len(m.explorer.search.Matches) != 2 {
		t.Fatalf("matches = %d, want 2 (full: %v)", len(m.explorer.search.Matches), m.explorer.search.Matches)
	}
	// ↓ to second match.
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m = next.(Model)
	if m.explorer.search.Idx != 1 {
		t.Fatalf("idx = %d, want 1 after ↓", m.explorer.search.Idx)
	}
	// Enter — opens viewer at the second match (user.ts).
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if !m.viewerActive {
		t.Error("viewer should be active after Enter")
	}
	if m.viewerFile != "src/api/user.ts" {
		t.Errorf("viewerFile = %q, want src/api/user.ts", m.viewerFile)
	}
	if m.explorer.search.Active {
		t.Error("search overlay should close after Enter")
	}
}

// TestExplorerSearch_EscBadgeClick_FirstClosesSearch verifies a click on
// the right side of the explorer panel's top border (where "esc back" is
// rendered) acts like pressing Esc — first click closes the search overlay
// and keeps the panel in active mode; a second click drops out of active.
func TestExplorerSearch_EscBadgeClick_FirstClosesSearch(t *testing.T) {
	t.Parallel()
	m := NewModel()
	m.width = 90
	m.height = 40
	m.bp = BreakpointMedium
	m.layoutMode = LayoutStack
	// Open the search overlay from explorer active mode.
	m = m.setFocus(PanelExplorer)
	m.activePanelID = PanelExplorer
	m.explorer.search = beginSearch(m.explorer.search)
	if !m.explorer.search.Active {
		t.Fatal("setup: search should be active")
	}
	// Click on the explorer's top-border, right side (esc-back badge).
	_, xMax := m.explorerPanelXBounds()
	panelTop := m.explorerPanelTopRow()
	clickX := xMax - 3 // safely inside the badge meta region
	next, _ := m.Update(tea.MouseClickMsg{X: clickX, Y: panelTop, Button: tea.MouseLeft})
	m = next.(Model)
	if m.explorer.search.Active {
		t.Error("first esc-badge click should close the search overlay")
	}
	if m.activePanelID != PanelExplorer {
		t.Errorf("first click should leave panel in active mode, got activePanelID=%d", m.activePanelID)
	}

	// Second click on the same badge area drops out of active mode.
	next, _ = m.Update(tea.MouseClickMsg{X: clickX, Y: panelTop, Button: tea.MouseLeft})
	m = next.(Model)
	if m.activePanelID == PanelExplorer {
		t.Error("second esc-badge click should exit active mode")
	}
}

// TestExplorerSearch_Integration_EscClosesOverlay verifies Esc inside the
// overlay closes search but keeps the explorer in active mode.
func TestExplorerSearch_Integration_EscClosesOverlay(t *testing.T) {
	t.Parallel()
	m := NewModel()
	m.data = fixtureMockData()
	m = m.setFocus(PanelExplorer)
	m.activePanelID = PanelExplorer

	next, _ := m.Update(tea.KeyPressMsg{Code: '/'})
	m = next.(Model)
	if !m.explorer.search.Active {
		t.Fatal("search should be active")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = next.(Model)
	if m.explorer.search.Active {
		t.Error("search should close on Esc")
	}
	if m.activePanelID != PanelExplorer {
		t.Errorf("activePanelID = %v, want PanelExplorer (Esc should not exit active mode while in search)", m.activePanelID)
	}
}

// fixtureMockData builds a small MockData with a known mix of modified files
// and tree nodes, used across the search tests.
func fixtureMockData() MockData {
	return MockData{
		ModifiedFiles: []ModifiedFile{
			{Path: "src/api/auth.ts", Mark: "M"},
			{Path: "internal/server/router.go", Mark: "M"},
		},
		Tree: []TreeNode{
			{Name: "src", IsDir: true, Indent: ""},
			{Name: "auth.ts", IsDir: false, Indent: "  ", FullPath: "src/api/auth.ts"},
			{Name: "user.ts", IsDir: false, Indent: "  ", FullPath: "src/api/user.ts"},
			{Name: "router.go", IsDir: false, Indent: "  ", FullPath: "internal/server/router.go"},
			{Name: "README.md", IsDir: false, Indent: "", FullPath: "README.md"},
		},
	}
}

// TestSearchInactive_NoMatches verifies the zero-value search state produces
// no matches when rebuild is called.
func TestSearchInactive_NoMatches(t *testing.T) {
	t.Parallel()
	s := ExplorerSearch{}
	out := rebuildExplorerSearch(s, fixtureMockData())
	if out.Active {
		t.Error("active should remain false")
	}
	if len(out.Matches) != 0 {
		t.Errorf("matches = %d, want 0", len(out.Matches))
	}
}

// TestSearchEmptyQuery_NoMatches verifies an active search with an empty
// query returns zero matches (we don't dump the entire tree on the user).
func TestSearchEmptyQuery_NoMatches(t *testing.T) {
	t.Parallel()
	s := beginSearch(ExplorerSearch{})
	out := rebuildExplorerSearch(s, fixtureMockData())
	if !out.Active {
		t.Error("expected active after beginSearch")
	}
	if len(out.Matches) != 0 {
		t.Errorf("matches = %d, want 0 (empty query should not match all)", len(out.Matches))
	}
}

// TestSearchSubstring_ModifiedDedup verifies that when a path appears in both
// the modified-files window and the tree, it shows once with IsMod=true.
// The modified flag is the more informative signal so it wins the dedup race.
func TestSearchSubstring_ModifiedDedup(t *testing.T) {
	t.Parallel()
	s := beginSearch(ExplorerSearch{})
	s = appendSearchRune(s, 'A')
	s = appendSearchRune(s, 'U')
	s = appendSearchRune(s, 'T')
	s = appendSearchRune(s, 'H')
	out := rebuildExplorerSearch(s, fixtureMockData())
	if len(out.Matches) != 1 {
		t.Fatalf("matches = %d, want 1 (deduped — full list: %v)", len(out.Matches), out.Matches)
	}
	if !out.Matches[0].IsMod || out.Matches[0].Path != "src/api/auth.ts" {
		t.Errorf("match = %+v, want IsMod && src/api/auth.ts", out.Matches[0])
	}
}

// TestSearchSubstring_TreeAndModMixed verifies a query that matches both a
// modified file AND a separate tree-only file returns both, modified first.
func TestSearchSubstring_TreeAndModMixed(t *testing.T) {
	t.Parallel()
	s := beginSearch(ExplorerSearch{Query: ".ts"})
	out := rebuildExplorerSearch(s, fixtureMockData())
	// Expected unique paths (modified first):
	//   src/api/auth.ts  (modified, deduped against tree's auth.ts)
	//   src/api/user.ts  (tree-only)
	if len(out.Matches) != 2 {
		t.Fatalf("matches = %d, want 2 (full list: %v)", len(out.Matches), out.Matches)
	}
	if !out.Matches[0].IsMod || out.Matches[0].Path != "src/api/auth.ts" {
		t.Errorf("first match = %+v, want IsMod && src/api/auth.ts", out.Matches[0])
	}
	if out.Matches[1].IsMod || out.Matches[1].Path != "src/api/user.ts" {
		t.Errorf("second match = %+v, want !IsMod && src/api/user.ts", out.Matches[1])
	}
}

// TestSearchSubstring_NoSpuriousFuzzy verifies a query that doesn't appear as
// a literal substring returns no matches — i.e. we are not silently doing
// fuzzy / out-of-order character matching.
func TestSearchSubstring_NoSpuriousFuzzy(t *testing.T) {
	t.Parallel()
	s := beginSearch(ExplorerSearch{})
	s = appendSearchRune(s, 'a')
	s = appendSearchRune(s, 'r')
	s = appendSearchRune(s, 'h')
	out := rebuildExplorerSearch(s, fixtureMockData())
	if len(out.Matches) != 0 {
		t.Errorf("matches = %d, want 0 (got: %v)", len(out.Matches), out.Matches)
	}
}

// TestSearchNavigation_WrapsBothEnds verifies up at index 0 wraps to the last
// match and down at last wraps to 0.
func TestSearchNavigation_WrapsBothEnds(t *testing.T) {
	t.Parallel()
	s := beginSearch(ExplorerSearch{Query: ".ts"})
	s = rebuildExplorerSearch(s, fixtureMockData())
	if len(s.Matches) != 2 {
		t.Fatalf("expected 2 matches for '.ts', got %d", len(s.Matches))
	}
	if s.Idx != 0 {
		t.Fatalf("initial idx = %d, want 0", s.Idx)
	}
	s = moveSearchUp(s)
	if s.Idx != 1 {
		t.Errorf("after up from 0, idx = %d, want 1 (wrap)", s.Idx)
	}
	s = moveSearchDown(s)
	if s.Idx != 0 {
		t.Errorf("after down from last, idx = %d, want 0 (wrap)", s.Idx)
	}
}

// TestSearchBackspace_EmptyQueryEnds verifies the empty-query backspace
// gesture exits search mode entirely.
func TestSearchBackspace_EmptyQueryEnds(t *testing.T) {
	t.Parallel()
	s := beginSearch(ExplorerSearch{})
	if !s.Active {
		t.Fatal("expected active after beginSearch")
	}
	s = backspaceSearch(s)
	if s.Active {
		t.Error("backspace on empty query should exit search mode")
	}
}

// TestSearchBackspace_ShortensQuery verifies a non-empty backspace just
// trims one rune.
func TestSearchBackspace_ShortensQuery(t *testing.T) {
	t.Parallel()
	s := beginSearch(ExplorerSearch{})
	s = appendSearchRune(s, 'a')
	s = appendSearchRune(s, 'b')
	s = appendSearchRune(s, 'c')
	s = backspaceSearch(s)
	if s.Query != "ab" {
		t.Errorf("query = %q, want %q", s.Query, "ab")
	}
	if !s.Active {
		t.Error("active should remain true while query non-empty")
	}
}

// TestTruncatePathLeft verifies the search-results path truncation preserves
// the filename + extension and pads with an ellipsis on the left.
func TestTruncatePathLeft(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		input  string
		budget int
		want   string
	}{
		{"short-passthrough", "main.go", 20, "main.go"},
		{"exact-passthrough", "abcdef", 6, "abcdef"},
		{"truncates-from-left", "internal/adapters/tui/panel_explorer.go", 25, "…rs/tui/panel_explorer.go"},
		{"unicode-safe", "αβγδ/ε.go", 7, "…δ/ε.go"},
		{"tiny-budget-no-op", "anything", 1, "anything"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := truncatePathLeft(tc.input, tc.budget)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestSearchClamp_OnReBuild verifies that when a query narrows the match list,
// a stale Idx is clamped to a valid position.
func TestSearchClamp_OnReBuild(t *testing.T) {
	t.Parallel()
	s := beginSearch(ExplorerSearch{Query: "ts"})
	s = rebuildExplorerSearch(s, fixtureMockData())
	s = moveSearchDown(s) // idx -> 1
	if s.Idx != 1 {
		t.Fatalf("setup: idx = %d, want 1", s.Idx)
	}
	// Narrow query to "auth" — only 2 modified-files-first matches.
	// Wait, "auth" still has 2 matches in our fixture (mod auth.ts +
	// tree auth.ts). Use a more restrictive query: "router" → 2 matches.
	// Use "user.ts" → 1 match.
	s.Query = "user.ts"
	s = rebuildExplorerSearch(s, fixtureMockData())
	if len(s.Matches) != 1 {
		t.Fatalf("expected 1 match for 'user.ts', got %d", len(s.Matches))
	}
	if s.Idx != 0 {
		t.Errorf("idx = %d, want 0 (clamped)", s.Idx)
	}
}

// TestSearchCurrentMatch returns the idx-th match, nil when none.
func TestSearchCurrentMatch(t *testing.T) {
	t.Parallel()
	s := beginSearch(ExplorerSearch{Query: "auth"})
	s = rebuildExplorerSearch(s, fixtureMockData())
	got := currentSearchMatch(s)
	if got == nil {
		t.Fatal("expected a current match")
	}
	if got.Path != "src/api/auth.ts" {
		t.Errorf("path = %q, want src/api/auth.ts", got.Path)
	}

	// No matches → nil
	empty := beginSearch(ExplorerSearch{Query: "zzzzz"})
	empty = rebuildExplorerSearch(empty, fixtureMockData())
	if currentSearchMatch(empty) != nil {
		t.Error("expected nil when no matches")
	}

	// Inactive → nil
	if currentSearchMatch(ExplorerSearch{}) != nil {
		t.Error("expected nil when inactive")
	}
}
