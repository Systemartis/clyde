package tui

import tea "charm.land/bubbletea/v2"

// ViewerMode selects the viewer's interaction model.
//
// Today only ViewerView (read-only) is implemented. ViewerEdit is reserved
// for a future bubbles/textarea-backed edit experience: 'i' would enter
// insert from view mode, ':w' would save, ':q' would quit. The field
// exists now so the dispatcher and the hint bar can branch on it without
// requiring a refactor when edit lands.
type ViewerMode int

const (
	// ViewerView is read-only navigation with vim bindings.
	ViewerView ViewerMode = iota
	// ViewerEdit is text editing — placeholder, not wired yet.
	ViewerEdit
)

// handleViewerVimKey is the vim-style key dispatcher for read-only viewer
// navigation. Bindings are deliberately the safe vim subset — moving the
// scroll position around without pretending to have a file cursor:
//
//	h / j / k / l    ←  ↓  ↑  →   (single line / 4-column horizontal step)
//	gg / G            top / bottom of file
//	0                 reset horizontal scroll to column 0
//	⌃d / ⌃u           half-page down / up
//	⌃f / ⌃b           full-page down / up
//
// Returns the updated model. When a `g` is pending, the next key either
// finishes a `gg` chord or clears the flag. Holding the gg-pending state
// at the model level keeps the viewer state value-typed.
func (m Model) handleViewerVimKey(msg tea.KeyPressMsg) Model {
	const horizStep = 4

	// gg-pending state machine: only a follow-up `g` completes the chord.
	// Everything else clears the flag and falls through to its own binding,
	// so an accidental `g` doesn't leave the viewer in a sticky state.
	gPending := m.vimGPending
	m.vimGPending = false

	if gPending && msg.Code == 'g' && msg.Mod == 0 {
		m.viewport.vp.GotoTop()
		return m
	}

	switch msg.Code {
	case 'h':
		if msg.Mod != 0 {
			return m
		}
		m.viewport.xOffset -= horizStep
		if m.viewport.xOffset < 0 {
			m.viewport.xOffset = 0
		}
	case 'l':
		if msg.Mod != 0 {
			return m
		}
		m.viewport.xOffset += horizStep
	case 'j':
		if msg.Mod != 0 {
			return m
		}
		m.viewport.vp.ScrollDown(1)
	case 'k':
		if msg.Mod != 0 {
			return m
		}
		m.viewport.vp.ScrollUp(1)
	case 'g':
		// Kitty keyboard protocol: Shift+g arrives as Code='g' + Mod=Shift,
		// not as the uppercase rune. Both branches must reach GotoBottom.
		if msg.Mod == tea.ModShift {
			m.viewport.vp.GotoBottom()
			return m
		}
		// First plain `g` arms the chord. A second `g` (handled at the top of
		// this function on the next keypress) jumps to top.
		if msg.Mod == 0 {
			m.vimGPending = true
		}
	case 'G':
		m.viewport.vp.GotoBottom()
	case '0':
		m.viewport.xOffset = 0
	case 'd':
		if msg.Mod == tea.ModCtrl {
			m.viewport.vp.HalfPageDown()
		}
	case 'u':
		if msg.Mod == tea.ModCtrl {
			m.viewport.vp.HalfPageUp()
		}
	case 'f':
		if msg.Mod == tea.ModCtrl {
			m.viewport.vp.PageDown()
		}
	case 'b':
		if msg.Mod == tea.ModCtrl {
			m.viewport.vp.PageUp()
		}
	}
	return m
}

// vimHintLine is the cheat-sheet rendered at the bottom of the viewer body
// while in ViewerView mode. Compact form chosen to fit even narrow panels;
// the help overlay (`h` key) has the long version. The "esc close" hint
// already lives in the panel chrome (top-bar meta), so we don't repeat it
// here.
func vimHintLine() string {
	return "j/k ↓↑  i edit  / find  : cmd  ⌃s save  f fullscreen"
}

// editHintLine is the bottom-row hint for ViewerEdit mode. The keys are
// modern-editor instead of vim-pure: ←↑↓→ moves, type to insert, esc
// returns to view, ⌃s saves. Word motion via ⌃←/⌃→ or ⌥←/⌥→ on Mac,
// undo/redo via ⌃z/⌃y, shift+arrow extends selection, ⌃c copies.
//
// Select-all is bound to ⌘/Super+A (⌃a is line-start), so the hint advertises
// ⌘a — advertising ⌃a would send users to line-start instead of select-all.
func editHintLine() string {
	return "shift+arrow select  ⌃c copy  ⌘a all  ⌃z undo  ⌃s save  esc view"
}
