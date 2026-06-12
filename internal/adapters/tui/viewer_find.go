package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// promptFieldRender produces the body of a single text-input field for
// the find/replace overlay. Three branches:
//
//   - empty + focused: faded placeholder + cursor (so the user sees the
//     hint until they start typing)
//   - empty + unfocused: faded placeholder, no cursor
//   - non-empty: the typed value styled normally; cursor only when
//     focused
//
// Keeps the rendering inline-friendly so callers can stitch a glyph
// (`/` or `→`) before the returned string.
func promptFieldRender(value, placeholder string, focused bool, cursor string, valueStyle, fadeStyle lipgloss.Style) string {
	if value == "" {
		body := fadeStyle.Render(placeholder)
		if focused {
			return cursor + body
		}
		return body
	}
	body := valueStyle.Render(value)
	if focused {
		return body + cursor
	}
	return body
}

// findMatch is a single hit inside the viewer's find results.
type findMatch struct {
	Line int // 0-based source line
	Col  int // 0-based byte offset where the match starts
	End  int // 0-based byte offset just past the match (exclusive)
}

// findInBuffer scans content for case-insensitive substring matches of
// query. Empty query returns no matches. Pure function on (content, query)
// so the find logic is easy to test independently of the Model.
//
// Iteration is byte-based (not rune-based) because the cursor / scroll
// math elsewhere in the viewer also operates on byte offsets — keeping
// them consistent avoids off-by-one weirdness on multi-byte runes.
func findInBuffer(content, query string) []findMatch {
	if query == "" {
		return nil
	}
	q := strings.ToLower(query)
	var matches []findMatch
	for lineIdx, line := range strings.Split(content, "\n") {
		lower := strings.ToLower(line)
		off := 0
		for off < len(lower) {
			i := strings.Index(lower[off:], q)
			if i < 0 {
				break
			}
			start := off + i
			matches = append(matches, findMatch{
				Line: lineIdx,
				Col:  start,
				End:  start + len(q),
			})
			off = start + 1 // step by 1 so overlapping matches still count
		}
	}
	return matches
}

// beginFind opens the find prompt and clears any prior search state.
func (m Model) beginFind() Model {
	m.viewerFindActive = true
	m.viewerFindQuery = ""
	m.viewerFindReplace = ""
	m.viewerFindFocusReplace = false
	m.viewerFindMatches = nil
	m.viewerFindIdx = 0
	m.viewerStatus = ""
	return m
}

// cancelFind closes the prompt without applying. Existing matches are
// dropped so they don't accidentally highlight after the user has
// dismissed the search.
func (m Model) cancelFind() Model {
	m.viewerFindActive = false
	m.viewerFindQuery = ""
	m.viewerFindReplace = ""
	m.viewerFindFocusReplace = false
	m.viewerFindMatches = nil
	m.viewerFindIdx = 0
	return m
}

// runFind closes the prompt, runs the query against the current viewer
// content, and either jumps to the first hit (find-only flow) or applies
// the replacement (when the replace field is non-empty). Replacement uses
// the same applySubstitute helper :%s/old/new/g uses, so behavior stays
// consistent between the two entry paths.
func (m Model) runFind() Model {
	q := strings.TrimSpace(m.viewerFindQuery)
	repl := m.viewerFindReplace
	m.viewerFindActive = false
	m.viewerFindFocusReplace = false
	if q == "" {
		m.viewerFindMatches = nil
		return m
	}

	// Replace path — applies the substitution across the entire buffer
	// the same way :%s/old/new/g does, then leaves the user back in
	// view mode with the dirty marker on (so they can :w / ⌃s).
	if repl != "" {
		spec := substituteSpec{find: q, replace: repl, global: true, all: true}
		return m.runSubstitute(spec)
	}

	content := m.currentViewerContent()
	matches := findInBuffer(content, q)
	m.viewerFindMatches = matches
	m.viewerFindIdx = 0
	if len(matches) == 0 {
		m.viewerStatus = "no matches for: " + q
		return m
	}
	m.viewerStatus = ""
	return m.scrollToCurrentFindMatch()
}

// nextFindMatch / prevFindMatch step n / N through the saved matches.
// Both wrap at the ends — small lists deserve wrap, and "I'm at the last
// match" is signaled by the index display rather than a clamp.
func (m Model) nextFindMatch() Model {
	if len(m.viewerFindMatches) == 0 {
		return m
	}
	m.viewerFindIdx = (m.viewerFindIdx + 1) % len(m.viewerFindMatches)
	return m.scrollToCurrentFindMatch()
}

func (m Model) prevFindMatch() Model {
	if len(m.viewerFindMatches) == 0 {
		return m
	}
	m.viewerFindIdx = (m.viewerFindIdx - 1 + len(m.viewerFindMatches)) % len(m.viewerFindMatches)
	return m.scrollToCurrentFindMatch()
}

// scrollToCurrentFindMatch nudges the viewport so the focused match's
// line is on-screen. We can't compute panel height here (View() owns
// dimensions), so we just set the YOffset to the match line and let the
// renderer's own clamp keep it valid.
func (m Model) scrollToCurrentFindMatch() Model {
	if len(m.viewerFindMatches) == 0 {
		return m
	}
	match := m.viewerFindMatches[m.viewerFindIdx]
	m.viewport.vp.SetYOffset(match.Line)
	return m
}

// currentViewerContent returns the source string used by find. Edit
// mode pulls from the live buffer so unsaved changes are searchable;
// view mode uses the cached content.
func (m Model) currentViewerContent() string {
	if m.viewerMode == ViewerEdit && len(m.viewerEdit.Lines) > 0 {
		return m.viewerEdit.String()
	}
	return m.viewerCachedContent
}

// handleViewerFindKey owns keystrokes while the find prompt is open.
// Two text fields: Find (top) and Replace (bottom). Tab cycles which
// one captures input. Enter runs find OR substitute depending on
// whether the replace field is non-empty. Esc cancels the whole prompt.
//
// Backspace on an empty active field while the OTHER field is
// non-empty just toggles back to find — keeps the user from
// accidentally closing the prompt mid-typing.
func (m Model) handleViewerFindKey(msg tea.KeyPressMsg) Model {
	switch msg.Code {
	case tea.KeyEsc:
		return m.cancelFind()
	case tea.KeyEnter:
		return m.runFind()
	case tea.KeyTab:
		m.viewerFindFocusReplace = !m.viewerFindFocusReplace
		return m
	case tea.KeyBackspace:
		if m.viewerFindFocusReplace {
			if m.viewerFindReplace == "" {
				m.viewerFindFocusReplace = false
				return m
			}
			rs := []rune(m.viewerFindReplace)
			m.viewerFindReplace = string(rs[:len(rs)-1])
			return m
		}
		if m.viewerFindQuery == "" {
			return m.cancelFind()
		}
		rs := []rune(m.viewerFindQuery)
		m.viewerFindQuery = string(rs[:len(rs)-1])
		return m
	}
	if msg.Text != "" && (msg.Mod == 0 || msg.Mod == tea.ModShift) {
		if m.viewerFindFocusReplace {
			m.viewerFindReplace += msg.Text
		} else {
			m.viewerFindQuery += msg.Text
		}
		return m
	}
	return m
}
