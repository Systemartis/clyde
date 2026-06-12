package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// ExplorerSearchMatch is a single hit inside the explorer's search overlay.
// Carries enough info to render a row (display text) and to act on Enter
// (full path + whether it came from the modified-files window).
type ExplorerSearchMatch struct {
	Path    string
	Display string
	IsMod   bool
}

// ExplorerSearch holds the search-overlay state for the explorer panel.
// Zero value = inactive (no overlay rendered, original tree visible).
//
// When Active is true the explorer hides its normal tree+modified split
// and shows a flat list of matches scoped to the current Query. Up/Down
// navigate Idx through Matches; Enter opens Matches[Idx]. The cursor index
// auto-clamps to the match count so a query that narrows from 5 → 1 doesn't
// land on a stale row.
type ExplorerSearch struct {
	Active  bool
	Query   string
	Matches []ExplorerSearchMatch
	Idx     int
}

// rebuildExplorerSearch recomputes Matches against the current explorer rows
// and modified-files list. Empty-query rebuilds yield zero matches — the
// overlay still renders ("Search: _") so the user sees they're in search
// mode without spamming the panel with the entire file list.
//
// The match rule is case-insensitive substring on the path. We chose substring
// over fuzzy because (a) substring is predictable — typing "auth" doesn't
// surprise you with results that contain those letters in unrelated order,
// and (b) the substring algorithm is six lines vs a few hundred for fzf-style
// scoring. If users ask for fuzzy later it slots in here.
func rebuildExplorerSearch(s ExplorerSearch, d MockData) ExplorerSearch {
	if !s.Active {
		s.Matches = nil
		s.Idx = 0
		return s
	}
	query := strings.ToLower(strings.TrimSpace(s.Query))
	if query == "" {
		s.Matches = nil
		s.Idx = 0
		return s
	}

	matches := make([]ExplorerSearchMatch, 0, 8)
	seen := make(map[string]bool)
	// Modified files first — they're more likely to be what the user wants
	// (recently-touched paths) so they earn the first slot in the list, and
	// they win the de-dup race against tree entries pointing at the same
	// path (the modified flag is the more informative signal).
	for _, mf := range d.ModifiedFiles {
		if strings.Contains(strings.ToLower(mf.Path), query) {
			matches = append(matches, ExplorerSearchMatch{
				Path:    mf.Path,
				Display: mf.Path,
				IsMod:   true,
			})
			seen[mf.Path] = true
		}
	}
	for _, node := range d.Tree {
		if node.IsDir {
			// Skip directories: opening a dir from search is ambiguous
			// (toggle? cd?) and a flat path list is the expected mental
			// model for "find a file."
			continue
		}
		path := node.FullPath
		if path == "" {
			path = strings.TrimSpace(node.Indent) + node.Name
		}
		if seen[path] {
			continue
		}
		if strings.Contains(strings.ToLower(path), query) {
			matches = append(matches, ExplorerSearchMatch{
				Path:    path,
				Display: path,
				IsMod:   false,
			})
		}
	}
	s.Matches = matches
	if s.Idx >= len(matches) {
		s.Idx = len(matches) - 1
	}
	if s.Idx < 0 {
		s.Idx = 0
	}
	return s
}

// beginSearch flips the overlay on with an empty query. Idempotent —
// re-entering search mode while already active doesn't wipe the existing
// query, only resets the cursor.
func beginSearch(s ExplorerSearch) ExplorerSearch {
	s.Active = true
	s.Idx = 0
	return s
}

// endSearch turns the overlay off and clears state. Esc and the empty-query
// backspace both call this.
func endSearch(s ExplorerSearch) ExplorerSearch {
	return ExplorerSearch{}
}

// appendSearchRune appends a printable rune to the query. Tabs and other
// control characters are filtered out at the caller (handleExplorerSearchKey).
func appendSearchRune(s ExplorerSearch, r rune) ExplorerSearch {
	s.Query += string(r)
	return s
}

// backspaceSearch deletes the last rune from the query. Returns endSearch's
// zero value when the query was already empty — that's the user's "back out"
// gesture. The caller is responsible for distinguishing "shorten the query"
// from "exit search," which it does by inspecting s.Query before the call.
func backspaceSearch(s ExplorerSearch) ExplorerSearch {
	if s.Query == "" {
		return endSearch(s)
	}
	runes := []rune(s.Query)
	s.Query = string(runes[:len(runes)-1])
	return s
}

// moveSearchUp / moveSearchDown move the match cursor with bottom/top
// wrap-around — different from the main explorer's clamp behavior, because
// a small match list is short enough that wrap is the more useful gesture
// (you saw the bottom, now jump back up to retry).
func moveSearchUp(s ExplorerSearch) ExplorerSearch {
	if len(s.Matches) == 0 {
		return s
	}
	if s.Idx <= 0 {
		s.Idx = len(s.Matches) - 1
		return s
	}
	s.Idx--
	return s
}

func moveSearchDown(s ExplorerSearch) ExplorerSearch {
	if len(s.Matches) == 0 {
		return s
	}
	if s.Idx >= len(s.Matches)-1 {
		s.Idx = 0
		return s
	}
	s.Idx++
	return s
}

// renderExplorerSearchOverlay paints the search overlay UI: a "Search:" prompt
// with the current query and a flat list of matches below. The highlighted
// match (Idx) gets a contrast background; modified-file matches earn a "M"
// mark in their gutter so the user knows which results are uncommitted edits
// vs. plain tree files.
//
// Layout — fits in (inner × innerH) cells:
//
//	row 1   "Search: <query>_"
//	row 2   "<count> match[es]"        (or "no matches" when empty + non-empty query)
//	row 3   ──── separator ────
//	row 4+  list of matches (scrolls when overflowing)
//	row N   hint:  ↵ open  ⌫ back  ↑↓ nav  esc close
func renderExplorerSearchOverlay(s Styles, p Palette, es ExplorerSearch, inner, width, height int, activeMode, focused bool) string {
	innerH := height - 2
	if innerH < 4 {
		innerH = 4
	}
	cursor := lipgloss.NewStyle().Foreground(p.Pink).Render("_")
	header := s.SectionHeader.Render("search ") +
		lipgloss.NewStyle().Foreground(p.Text).Render(es.Query) + cursor
	var status string
	switch {
	case strings.TrimSpace(es.Query) == "":
		status = lipgloss.NewStyle().Foreground(p.TextDim).
			Render("type to find files (modified first)")
	case len(es.Matches) == 0:
		status = lipgloss.NewStyle().Foreground(p.TextDim).
			Render("no matches")
	default:
		status = lipgloss.NewStyle().Foreground(p.TextDim).
			Render(fmt.Sprintf("%d match%s", len(es.Matches), pluralES(len(es.Matches))))
	}
	dotLine := s.SectionHeader.Render(strings.Repeat("·", clamp(inner, 1, 20)))

	// "esc close" intentionally omitted — esc is the universal back-out and
	// already covered by the panel's top-bar meta. Repeating it here would
	// just steal a precious row of match-list real estate.
	hintLine := lipgloss.NewStyle().Foreground(p.TextFade).
		Render("↵ open  ⌫ back  ↑↓ nav")

	listBudget := innerH - 4 // header + status + dotLine + hint
	if listBudget < 1 {
		listBudget = 1
	}

	hlBg := lipgloss.NewStyle().
		Background(lipgloss.Color("#2D2A3E")).
		Foreground(p.Text)
	modMark := lipgloss.NewStyle().Foreground(p.Pink).Bold(true)
	pathStyle := lipgloss.NewStyle().Foreground(p.TextMid)
	dimPath := lipgloss.NewStyle().Foreground(p.TextDim)

	rows := make([]string, 0, listBudget)
	scroll := 0
	if es.Idx >= listBudget {
		scroll = es.Idx - listBudget + 1
	}
	end := scroll + listBudget
	if end > len(es.Matches) {
		end = len(es.Matches)
	}
	// Reserve 2 cells for the M/space mark prefix; the rest is path budget.
	// Long paths get LEFT-truncated with an ellipsis ("…api/auth.ts") so
	// the filename + extension always survive on the right — that's the
	// part the user is scanning for. Right-truncation would chop the
	// extension off and make matches indistinguishable.
	pathBudget := inner - 2
	if pathBudget < 4 {
		pathBudget = 4
	}
	for i := scroll; i < end; i++ {
		match := es.Matches[i]
		mark := "  "
		if match.IsMod {
			mark = modMark.Render("M ")
		}
		display := truncatePathLeft(match.Display, pathBudget)
		var body string
		if i == es.Idx {
			body = pathStyle.Render(display)
		} else {
			body = dimPath.Render(display)
		}
		row := mark + body
		if focused && i == es.Idx {
			row = hlBg.Render(padRight(row, inner))
		} else {
			row = padRight(row, inner)
		}
		rows = append(rows, row)
	}
	for len(rows) < listBudget {
		rows = append(rows, "")
	}

	body := strings.Join(append([]string{header, status, dotLine}, append(rows, hintLine)...), "\n")
	// Meta on the panel border communicates what esc does AT THIS LAYER.
	// Inside search, esc closes the overlay first (returning the user to
	// active mode); a second esc then drops out of active focus. The
	// default active-mode badge ("▲▼ scroll · +/− resize · esc back")
	// would mislead — ▲▼ here moves through matches, not the panel scroll,
	// and +/− does nothing in search. Replace it with a clean "esc back".
	if activeMode {
		return wrapPanelActiveBadge(s, body, "explorer", "esc back", width, height)
	}
	return wrapPanel(s, body, "explorer", "esc back", width, height, focused)
}

// renderExplorerSearchButton paints the always-visible "⌕ press / to search"
// row at the top of the explorer panel body. Becomes a clickable affordance
// via mouse.go::explorerSearchButtonAtPos — clicking the row triggers the
// same code path as pressing /.
//
// Visual budget is the panel's inner width; on narrow widths the hint text
// drops to keep just the glyph + the slash.
func renderExplorerSearchButton(p Palette, inner int, focused bool) string {
	glyph := "⌕"
	full := glyph + "  press / to search"
	short := glyph + "  /"
	text := full
	if inner < len([]rune(full))+2 {
		text = short
	}
	style := lipgloss.NewStyle().Foreground(p.TextDim)
	if focused {
		style = lipgloss.NewStyle().Foreground(p.TextMid)
	}
	return style.Render(text)
}

// pluralES returns "es" for n != 1, "" otherwise — used by the match-count
// status line so we don't render "1 matches".
func pluralES(n int) string {
	if n == 1 {
		return ""
	}
	return "es"
}

// truncatePathLeft trims a path from the left so the filename + extension on
// the right always remain visible, prefixing an ellipsis when truncation
// happens. Rune-aware via []rune so multi-byte filenames don't get
// mid-codepoint cuts.
//
//	"internal/adapters/tui/panel_explorer.go" with budget 25 →
//	"…ui/panel_explorer.go"
//
// Paths shorter than budget pass through untouched.
func truncatePathLeft(path string, budget int) string {
	if budget <= 1 {
		return path
	}
	rs := []rune(path)
	if len(rs) <= budget {
		return path
	}
	// Reserve 1 cell for the ellipsis.
	keep := budget - 1
	return "…" + string(rs[len(rs)-keep:])
}

// currentSearchMatch returns the currently-highlighted match, or nil when
// there are no matches. Caller's Enter handler uses this to find the path
// to open in the viewer.
func currentSearchMatch(s ExplorerSearch) *ExplorerSearchMatch {
	if !s.Active || len(s.Matches) == 0 {
		return nil
	}
	if s.Idx < 0 || s.Idx >= len(s.Matches) {
		return nil
	}
	m := s.Matches[s.Idx]
	return &m
}
