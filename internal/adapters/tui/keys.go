package tui

import (
	tea "charm.land/bubbletea/v2"
)

// handleKey processes a key event.
func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if quit, model, cmd := m.handleQuitKey(msg); quit {
		return model, cmd
	}
	// Boot splash dismisses on any non-quit key — we already let the quit
	// chord (above) tear down the program, so anything else here is the
	// user saying "I get it, show me the UI". Swallow the keystroke so
	// it doesn't leak through to a panel that just appeared.
	if m.boot.Active {
		m.boot = m.boot.Dismiss()
		return m, nil
	}
	if m.settingsOpen {
		m = m.handleSettingsKey(msg)
		return m, nil
	}
	// Editor input modes (insert / :command / explorer search prompt) own
	// the alphabet, so the global `?` and `h` chords would otherwise eat
	// keystrokes the user is trying to type. Ditto: explorer search.
	editorTyping := m.viewerActive && (m.viewerMode == ViewerEdit || m.viewerCmdActive)
	explorerSearching := m.activePanelID == PanelExplorer && m.explorer.search.Active

	// Some terminals deliver '?' as the rune directly; others surface it as
	// Shift+'/'. Accept both so the settings overlay opens reliably.
	if !editorTyping && !explorerSearching {
		if msg.Code == '?' || (msg.Code == '/' && msg.Mod == tea.ModShift) {
			return m.openSettings(), nil
		}
	}
	// h toggles the per-panel help overlay (cheat-sheet inside each
	// panel) — independent of mode, like `?`. Active mode handles `h`
	// later as a panel-specific binding only when it is one (it isn't
	// for any panel right now), so falling through here is safe.
	if msg.Code == 'h' && msg.Mod == 0 && !m.viewerActive && !explorerSearching {
		m.helpOpen = !m.helpOpen
		return m, nil
	}
	// Backspace closes help when it's open. Esc handles the same case
	// inside handleEscapeKey (the priority chain there picks help
	// first). Routing both gives users two natural exits — `h` to
	// toggle, Esc/⌫ to dismiss — instead of a single magic key.
	if m.helpOpen && msg.Code == tea.KeyBackspace {
		m.helpOpen = false
		return m, nil
	}
	// Global panel-jump shortcuts. Work in passive AND active mode so
	// users can leap between panels without first dropping out of an
	// expanded panel — same ergonomics as the old ⌃-prefixed jumps,
	// without the modifier. Skipped while the viewer is open (the
	// viewer captures keys for navigation), a hook is pending, or the
	// explorer's search overlay is open (we need the alphabet to feed
	// the query, not jump around).
	if !m.viewerActive && !m.hookNotif.Active && msg.Mod == 0 && !explorerSearching {
		if handled, model := m.handlePanelJumpKey(msg.Code); handled {
			// In tabs mode only honor jumps to panels that live in the tab
			// strip. Jumping to a non-tab panel (explorer/servers/bash/cache)
			// would render that panel full-screen while the strip still
			// highlights a tab — a desynced, misleading UI state. Ignore the
			// jump and fall through (it becomes a harmless no-op).
			if m.effectiveMode() != LayoutTabs || m.isTabStripPanel(model.focused) {
				return model, nil
			}
		}
	}
	if handled, model := m.handleHookPendingKey(msg); handled {
		return model, nil
	}
	if msg.Code == tea.KeyEscape {
		// Search overlay claims Esc first — closing the prompt instead of
		// dropping out of explorer's active mode lets the user iterate
		// queries without losing their place.
		if explorerSearching {
			m.explorer.search = endSearch(m.explorer.search)
			return m, nil
		}
		// In-viewer find prompt and :command prompt take Esc next so
		// they cancel the prompt instead of closing the whole viewer.
		// Without these branches, Esc-from-find would slam the user back
		// to the dashboard and lose any matches / replacement they were
		// about to apply.
		if m.viewerActive && m.viewerFindActive {
			return m.cancelFind(), nil
		}
		if m.viewerActive && m.viewerCmdActive {
			return m.cancelCommand(), nil
		}
		return m.handleEscapeKey(), nil
	}
	if msg.Mod == tea.ModCtrl {
		// Viewer + explorer-active claim Ctrl+d/u/f/b for vim half/full-page
		// scroll before the global handleCtrl picks up the same letter codes
		// for layout commands. Restricted to active mode by design — vim
		// nav inside a panel is the "I'm in this panel, scroll its content"
		// gesture, which only makes sense once the user has explicitly
		// committed to that panel via Enter.
		if m.viewerActive {
			return m.handleViewerKey(msg)
		}
		if m.activePanelID == PanelExplorer {
			model, cmd := m.handleExplorerActiveKey(msg)
			return model, cmd
		}
		m = m.handleCtrl(msg.Code)
		return m, nil
	}
	// Viewer claims keys whenever it's active, regardless of which panel
	// the user clicked last. Without this, a click on the explorer + then
	// opening a file leaves activePanelID == PanelExplorer; subsequent
	// keystrokes flow into the explorer's active-mode handler instead of
	// the viewer (so 'i', 'f', etc. silently no-op until the user clicks
	// the viewer or drops out of explorer's active mode). The viewer is
	// always the foreground UI when viewerActive, so prioritize it here.
	if m.viewerActive {
		return m.handleViewerKey(msg)
	}
	if m.isActiveMode() {
		return m.handleActiveModeKey(msg)
	}
	return m.handleModeKey(msg)
}

// handleQuitKey handles the global quit chord (q or Ctrl+C). Returns
// quit=true when the key matched, with the model and command to dispatch.
func (m Model) handleQuitKey(msg tea.KeyPressMsg) (bool, tea.Model, tea.Cmd) {
	if msg.Code != 'q' && (msg.Code != 'c' || msg.Mod != tea.ModCtrl) {
		return false, m, nil
	}
	// Quit is a global chord, but the user can be composing a `:q` command,
	// editing a buffer, or filling the explorer search prompt — in any of
	// those, plain 'q' must reach the local handler instead of nuking the
	// whole app.
	if msg.Code == 'q' {
		if m.viewerActive && (m.viewerMode == ViewerEdit || m.viewerCmdActive) {
			return false, m, nil
		}
		if m.activePanelID == PanelExplorer && m.explorer.search.Active {
			return false, m, nil
		}
	}
	// Ctrl+C is the modern-editor "copy" shortcut. Yield the chord to the
	// editor whenever it's open in edit mode (with or without an active
	// selection) so the user can copy without losing the entire app.
	// They can still quit by pressing 'q' from the dashboard.
	if msg.Code == 'c' && msg.Mod == tea.ModCtrl && m.viewerActive && m.viewerMode == ViewerEdit {
		return false, m, nil
	}
	// If a hook is pending when quitting, auto-deny so claude is not left hanging.
	m.respondHook(false, "clyde exiting")
	m.quitting = true
	return true, m, tea.Quit
}

// openSettings primes the settings overlay state and returns the updated
// model.
//
// Always lands on Global scope: most users tweak global preferences and
// the per-project override is a niche feature reachable by Tab. Defaulting
// to Project felt like a layer-cake — the "real" panel state already
// rides along via RememberLayout, so the project tab is reserved for
// cases where the user genuinely wants to diverge from the global layer.
func (m Model) openSettings() Model {
	m.settingsOpen = true
	// Land at the top of the overlay (Notifications) instead of the
	// first panel toggle. Starting in the middle of a list nobody
	// intends to edit on every open was disorienting — the overlay
	// should announce its top section first.
	m.settingsCursor = settingsCursorMin()
	m.settingsScope = ScopeGlobal
	m.settingsToggles = settingsOverlayFromConfig(m.baseCfg, m.settingsScope, m.liveProject.CWD())
	m.settingsRememberLayout = m.baseCfg.RememberLayout
	m.settingsNotificationStyle = m.baseCfg.NotificationStyle
	if !m.settingsNotificationStyle.IsValid() {
		m.settingsNotificationStyle = NotificationFullscreen
	}
	m.settingsCostThreshold = m.baseCfg.NotifyCostThresholdUSD
	m.settingsTheme = m.baseCfg.Theme
	if !m.settingsTheme.IsValid() {
		m.settingsTheme = ThemeTokyoNight
	}
	m.settingsMascotPersona = m.baseCfg.MascotPersona
	if !m.settingsMascotPersona.IsValid() {
		m.settingsMascotPersona = MascotPersonaMeowl
	}
	m.settingsBootScreenEnabled = m.baseCfg.BootScreenEnabled
	return m
}

// handleHookPendingKey routes y/n/Esc to a pending hook event banner.
func (m Model) handleHookPendingKey(msg tea.KeyPressMsg) (bool, Model) {
	if !m.hookNotif.Active {
		return false, m
	}
	switch msg.Code {
	case 'y', 'Y':
		m.respondHook(true, "")
		// Dismiss the overlay unconditionally. In the live path respondHook
		// already cleared hookNotif, but the SYNTHETIC preview (Ctrl+N) has no
		// ResponseCh, so respondHook is a no-op there and hookNotif stays
		// Active — without notifAck the overlay would be stuck until Esc.
		// Setting notifAck here mirrors the 'n'/Esc branches so all three
		// responses dismiss the banner in both the live and synthetic cases.
		m.notifAck = true
		return true, m
	case 'n', 'N':
		m.respondHook(false, "denied by user")
		m.notifAck = true
		return true, m
	case tea.KeyEscape:
		m.respondHook(false, "dismissed")
		m.notifAck = true
		return true, m
	}
	return false, m
}

// handleEscapeKey handles Esc when no hook is pending. Priority chain:
// help overlay → active mode → viewer → notification dismiss. Closing
// the help overlay first matches the user's mental model (Esc is the
// universal "back out of the thing I just opened").
func (m Model) handleEscapeKey() Model {
	if m.helpOpen {
		m.helpOpen = false
		return m
	}
	// Edit mode wins the Esc race: drop back to view, don't close the
	// viewer outright. The user has unsaved changes either way and
	// closing on Esc-from-edit would force them to reopen + retype.
	if m.viewerActive && m.viewerMode == ViewerEdit {
		return m.exitEditMode()
	}
	if m.isActiveMode() {
		return m.transitionToPassive()
	}
	if m.viewerActive {
		// Dirty-confirm: if there are unsaved changes, the first Esc
		// surfaces a hint but keeps the viewer open. A subsequent Esc
		// confirms the discard. Saves the user from accidentally
		// nuking edits with one stray keystroke.
		if m.viewerDirty && m.viewerStatus != viewerDiscardPrompt {
			m.viewerStatus = viewerDiscardPrompt
			return m
		}
		return m.closeViewer()
	}
	m.notifAck = true
	return m
}

// handleActiveModeKey routes keys when a panel is in Expanded-Active state.
// Most panels: ↑/↓ scrolls a viewport, Enter is a no-op, +/- resizes.
// The explorer is special-cased: in active mode the user is "inside" the
// tree, so ↑/↓ moves the highlight, Enter opens the highlighted file, and
// Backspace collapses the current directory. Esc / Tab exit active mode.
//
// Returns (Model, tea.Cmd) so panel-specific actions that emit commands
// — e.g. y/Y in the explorer dispatching tea.SetClipboard — can flow back
// up through Update without a side-channel.
func (m Model) handleActiveModeKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	pid := m.activePanelID
	if pid == PanelExplorer {
		m, cmd := m.handleExplorerActiveKey(msg)
		return m, cmd
	}
	switch msg.Code {
	case tea.KeyUp:
		m.panelVPs[pid].ScrollUp(1)
	case tea.KeyDown:
		m.panelVPs[pid].ScrollDown(1)
	case '+', '=':
		if m.effectiveMode() == LayoutStack {
			m.resizeFocusedPanel(+1)
		}
	case '-':
		if m.effectiveMode() == LayoutStack {
			m.resizeFocusedPanel(-1)
		}
	case tea.KeyBackspace:
		m = m.transitionToPassive()
	case tea.KeyTab:
		m = m.transitionToPassive()
		m = m.advanceFocus(tabDelta(msg.Mod))
	case ' ':
		if m.effectiveMode() == LayoutStack {
			m.collapse[pid].Collapse()
			m.persistLayoutIfEnabled()
		}
		m.activePanelID = PanelNone
	case tea.KeyEnter:
		// Enter → panel-specific action (no-op for most panels)
	}
	return m, nil
}

// handleExplorerActiveKey is the explorer-specific active-mode handler.
// Tree highlight follows ↑/↓ (with viewport auto-scrolling to keep the
// cursor visible), Enter opens the file or toggles a directory, ←
// collapses the current directory (vim-style), Tab leaves to the next
// panel, Backspace and Esc both return to passive (handled at handleKey
// level for Esc).
//
// y / Y yank the highlighted item to the system clipboard via OSC 52
// (vim-style). Lowercase y → absolute path; uppercase Y → basename only.
// Returns the SetClipboard + clear-toast tick as a tea.Cmd.
func (m Model) handleExplorerActiveKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.explorer.search.Active {
		return m.handleExplorerSearchKey(msg)
	}
	// Vim navigation chords (top/bottom + page-jumps). gg uses vimGPending
	// the same way the viewer does; the chord is shared by virtue of only
	// one panel-or-viewer context being active at a time.
	gPending := m.vimGPending
	m.vimGPending = false
	if msg.Mod == tea.ModCtrl {
		switch msg.Code {
		case 'd':
			return m.explorerHalfPage(+1), nil
		case 'u':
			return m.explorerHalfPage(-1), nil
		case 'f':
			return m.explorerFullPage(+1), nil
		case 'b':
			return m.explorerFullPage(-1), nil
		}
		return m, nil
	}
	if gPending && msg.Code == 'g' && msg.Mod == 0 {
		return m.explorerJumpEdge(-1), nil
	}
	switch msg.Code {
	case 'g':
		// Kitty keyboard protocol delivers Shift+g as Code='g' + Mod=Shift,
		// while older terminals fold shift into the uppercase rune. Handle
		// both so G works regardless of the user's terminal mode.
		if msg.Mod == tea.ModShift {
			return m.explorerJumpEdge(+1), nil
		}
		if msg.Mod == 0 {
			m.vimGPending = true
			return m, nil
		}
	case 'G':
		return m.explorerJumpEdge(+1), nil
	case '/':
		// Open search overlay. The explorer renderer paints a "Search:"
		// input line + flat match list; rebuildExplorerSearch is called
		// on every keystroke to keep matches in sync with the query.
		m.explorer.search = beginSearch(m.explorer.search)
		return m, nil
	case tea.KeyUp:
		m.explorer.MoveUp(len(m.data.ModifiedFiles))
		m = m.syncPanelViewport(PanelExplorer)
		m = m.scrollExplorerToHighlight()
	case tea.KeyDown:
		m.explorer.MoveDown(len(m.data.ModifiedFiles))
		m = m.syncPanelViewport(PanelExplorer)
		m = m.scrollExplorerToHighlight()
	case tea.KeyEnter:
		return m.explorerActivate(), nil
	case tea.KeyLeft:
		// Pure section switcher: jump to modified-files (top). Enter
		// already toggles directories, so the older "← collapse dir"
		// binding was redundant AND blocked section switching when the
		// cursor sat on a directory — net negative.
		if len(m.data.ModifiedFiles) > 0 {
			m.explorer.section = SectionMod
			m.explorer.modHighlight = 0
		}
	case tea.KeyRight:
		// Symmetric to ←: jump to tree (top row). No dir-expand
		// behavior — Enter handles that.
		if len(m.explorer.rows) > 0 {
			m.explorer.section = SectionTree
			m.explorer.highlighted = 0
		}
	case tea.KeyBackspace:
		// Backspace exits active mode (matches every other panel). Use ←
		// when you want to collapse the current directory.
		m = m.transitionToPassive()
	case tea.KeyTab:
		m = m.transitionToPassive()
		m = m.advanceFocus(tabDelta(msg.Mod))
	case 'y':
		// With the kitty keyboard protocol (which Bubbletea v2 enables by
		// default) Shift+y arrives as Code='y' + Mod=ModShift, not as the
		// uppercase rune. Without this branch the shift form silently fell
		// through to the full-path copy and "Y" never copied a basename.
		if msg.Mod == tea.ModShift {
			return m.copyExplorerHighlight(true)
		}
		return m.copyExplorerHighlight(false)
	case 'Y':
		// Fallback: terminals without kitty keyboard protocol fold shift
		// into the uppercase rune directly. Keep this case so behavior is
		// consistent across both reporting modes.
		return m.copyExplorerHighlight(true)
	}
	return m, nil
}

// explorerJumpEdge moves the highlight to the top (-1) or bottom (+1) of the
// section the cursor currently lives in. gg / G in vim parlance. We never
// cross the section boundary on a jump — that would be too easy to fire
// accidentally; the user can ←/→ to switch sections.
func (m Model) explorerJumpEdge(dir int) Model {
	if m.explorer.section == SectionMod {
		if dir < 0 {
			m.explorer.modHighlight = 0
		} else {
			m.explorer.modHighlight = len(m.data.ModifiedFiles) - 1
			if m.explorer.modHighlight < 0 {
				m.explorer.modHighlight = 0
			}
		}
		return m.syncPanelViewport(PanelExplorer)
	}
	if dir < 0 {
		m.explorer.highlighted = 0
	} else {
		m.explorer.highlighted = len(m.explorer.rows) - 1
		if m.explorer.highlighted < 0 {
			m.explorer.highlighted = 0
		}
	}
	m = m.syncPanelViewport(PanelExplorer)
	return m.scrollExplorerToHighlight()
}

// explorerHalfPage advances the cursor by half the visible tree budget. dir
// is +1 (down) or -1 (up). For the modified-files section we use modAreaH;
// for tree we use treeAreaH. Clamps without wrapping at both ends.
func (m Model) explorerHalfPage(dir int) Model {
	return m.explorerPageMove(dir, 2)
}

// explorerFullPage advances the cursor by the full visible budget.
func (m Model) explorerFullPage(dir int) Model {
	return m.explorerPageMove(dir, 1)
}

// explorerPageMove is the shared body of explorerHalfPage and explorerFullPage.
// divisor=1 → full page; divisor=2 → half page. Section-aware so the cursor
// stays inside the region it started in.
func (m Model) explorerPageMove(dir, divisor int) Model {
	if m.explorer.section == SectionMod {
		areaH := m.explorerModAreaH()
		step := areaH / divisor
		if step < 1 {
			step = 1
		}
		next := m.explorer.modHighlight + dir*step
		if next < 0 {
			next = 0
		}
		if next > len(m.data.ModifiedFiles)-1 {
			next = len(m.data.ModifiedFiles) - 1
		}
		if next < 0 {
			next = 0
		}
		m.explorer.modHighlight = next
		return m.syncPanelViewport(PanelExplorer)
	}
	areaH := m.explorerTreeAreaH()
	step := areaH / divisor
	if step < 1 {
		step = 1
	}
	next := m.explorer.highlighted + dir*step
	if next < 0 {
		next = 0
	}
	if next > len(m.explorer.rows)-1 {
		next = len(m.explorer.rows) - 1
	}
	if next < 0 {
		next = 0
	}
	m.explorer.highlighted = next
	m = m.syncPanelViewport(PanelExplorer)
	return m.scrollExplorerToHighlight()
}

// handleExplorerSearchKey is the active-mode key handler that takes over
// while the explorer's search overlay is open. It owns the alphabet so the
// user can build up a query without those keys triggering the global panel
// jumps (e/a/d/u/s/b/c) — those are paused until the overlay closes.
//
// Layering:
//   - Esc: close overlay, restore tree.
//   - Backspace: shorten query; on empty query, close overlay (matches the
//     "back out of the prompt" gesture every shell does).
//   - ↑/↓: navigate matches (wraps both ends — small lists deserve wrap).
//   - Enter: open the highlighted match in the viewer.
//   - Tab: leave to next panel (still useful even while searching).
//   - Printable rune: append to query, rebuild matches.
func (m Model) handleExplorerSearchKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.Code {
	case tea.KeyEsc:
		m.explorer.search = endSearch(m.explorer.search)
		return m, nil
	case tea.KeyBackspace:
		m.explorer.search = backspaceSearch(m.explorer.search)
		m.explorer.search = rebuildExplorerSearch(m.explorer.search, m.data)
		return m, nil
	case tea.KeyUp:
		m.explorer.search = moveSearchUp(m.explorer.search)
		return m, nil
	case tea.KeyDown:
		m.explorer.search = moveSearchDown(m.explorer.search)
		return m, nil
	case tea.KeyEnter:
		return m.openSearchMatch(), nil
	case tea.KeyTab:
		m.explorer.search = endSearch(m.explorer.search)
		m = m.transitionToPassive()
		m = m.advanceFocus(tabDelta(msg.Mod))
		return m, nil
	}
	// Printable rune (Code is the rune, no modifier other than Shift).
	if msg.Code >= 0x20 && msg.Code < 0x7f && (msg.Mod == 0 || msg.Mod == tea.ModShift) {
		m.explorer.search = appendSearchRune(m.explorer.search, msg.Code)
		m.explorer.search = rebuildExplorerSearch(m.explorer.search, m.data)
		return m, nil
	}
	return m, nil
}

// openSearchMatch opens the currently-highlighted search result in the
// viewer, then closes the overlay. Restores the tree underneath so the
// user can re-open search later from the same context. No-op when the
// match list is empty (silently ignores Enter so the user can type more).
func (m Model) openSearchMatch() Model {
	match := currentSearchMatch(m.explorer.search)
	if match == nil {
		return m
	}
	m.explorer.search = endSearch(m.explorer.search)
	m = m.loadViewerFile(match.Path)
	m = m.loadViewerDiff()
	return m
}

// scrollExplorerToHighlight nudges the explorer viewport so the highlighted
// row stays visible after a MoveUp/MoveDown. Without this the highlight
// would fall off the top or bottom of the visible window once it crossed
// the panel edge.
func (m Model) scrollExplorerToHighlight() Model {
	rows := buildVisibleRows(m.data, m.explorer.collapsed)
	if len(rows) == 0 {
		return m
	}
	vp := m.panelVPs[PanelExplorer]
	vpHeight := vp.Height()
	if vpHeight <= 0 {
		vpHeight = 1
	}
	yOff := vp.YOffset()
	hl := m.explorer.highlighted
	if hl < yOff {
		yOff = hl
	} else if hl >= yOff+vpHeight {
		yOff = hl - vpHeight + 1
	}
	if yOff < 0 {
		yOff = 0
	}
	maxOff := len(rows) - vpHeight
	if maxOff < 0 {
		maxOff = 0
	}
	if yOff > maxOff {
		yOff = maxOff
	}
	vp.SetYOffset(yOff)
	m.panelVPs[PanelExplorer] = vp
	return m
}

// handleModeKey handles non-ctrl, non-quit key events in passive/collapsed state.
//
// v22+: tree navigation moved out of passive mode. Passive explorer behaves
// like every other panel — ↑/↓ moves focus between panels (column-aware in
// 2-col). To navigate the tree the user enters active mode with Enter; that
// is handled by handleExplorerActiveKey via handleActiveModeKey.
func (m Model) handleModeKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.viewerActive {
		return m.handleViewerKey(msg)
	}
	return m.handleDefaultKey(msg), nil
}

// handleViewerKey routes keys when the file viewer is active.
//
// Arrow keys + Tab keep the always-on muscle-memory bindings; the rest of
// the rune namespace flows into handleViewerVimKey for the vim-style
// navigation set (h/j/k/l, gg/G, 0, ⌃d/⌃u, ⌃f/⌃b). Read-only today —
// edit mode is the next layer (tracked via Model.viewerMode).
func (m Model) handleViewerKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	// Find prompt and command-mode prompt own every key first when active.
	if m.viewerFindActive {
		return m.handleViewerFindKey(msg), nil
	}
	if m.viewerCmdActive {
		return m.handleViewerCommandKey(msg)
	}
	// Edit mode owns the entire keyboard (typing inserts characters), so
	// dispatch there before any of the view-mode chords would steal a
	// keystroke. Esc inside edit goes back to view; that branch is
	// handled inside handleViewerEditKey.
	if m.viewerMode == ViewerEdit {
		return m.handleViewerEditKey(msg)
	}
	// `:` opens command mode for vim-style :w / :q / :wq / :q!
	if msg.Code == ':' && msg.Mod == 0 {
		m.vimGPending = false
		return m.beginCommand(), nil
	}
	// `/` opens the find prompt; n / N step through matches afterwards.
	if msg.Code == '/' && msg.Mod == 0 {
		m.vimGPending = false
		return m.beginFind(), nil
	}
	if msg.Code == 'n' && msg.Mod == 0 {
		m.vimGPending = false
		return m.nextFindMatch(), nil
	}
	if msg.Code == 'N' && msg.Mod == 0 || (msg.Code == 'n' && msg.Mod == tea.ModShift) {
		m.vimGPending = false
		return m.prevFindMatch(), nil
	}

	const horizStep = 4
	// `f` toggles the fullscreen takeover. Caught at this layer so it
	// works whether the viewer was opened in passive or active panel
	// state; doesn't conflict with vim's `f<char>` find-char chord
	// because we don't (and likely won't) implement that.
	if msg.Code == 'f' && msg.Mod == 0 {
		m.vimGPending = false
		m.viewerFullscreen = !m.viewerFullscreen
		return m, nil
	}
	// `i` enters insert mode. Vim's lowercase `i` inserts AT the cursor
	// position; we don't yet have a per-character cursor in view mode
	// (h/j/k/l move the scroll, not a cursor) so we initialize the
	// edit cursor at the top-left of whatever's currently scrolled into
	// view. Future iteration: track a real cursor in view mode.
	if msg.Code == 'i' && msg.Mod == 0 {
		m.vimGPending = false
		return m.enterEditMode(), nil
	}
	// Ctrl+S / Cmd+S saves the buffer to disk. Vim's :w also works;
	// this is the modern-editor keyboard shortcut for users who don't
	// want to drop into command mode just to write.
	if (msg.Mod.Contains(tea.ModCtrl) || msg.Mod.Contains(tea.ModSuper) || msg.Mod.Contains(tea.ModMeta)) && msg.Code == 's' {
		m.vimGPending = false
		return m.saveViewerBuffer(), nil
	}
	switch msg.Code {
	case tea.KeyUp:
		m.vimGPending = false
		m.viewport.vp.ScrollUp(1)
		return m, nil
	case tea.KeyDown:
		m.vimGPending = false
		m.viewport.vp.ScrollDown(1)
		return m, nil
	case tea.KeyLeft:
		m.vimGPending = false
		m.viewport.xOffset -= horizStep
		if m.viewport.xOffset < 0 {
			m.viewport.xOffset = 0
		}
		return m, nil
	case tea.KeyRight:
		m.vimGPending = false
		m.viewport.xOffset += horizStep
		return m, nil
	case tea.KeyTab:
		m.vimGPending = false
		return m.advanceFocus(tabDelta(msg.Mod)), nil
	}
	return m.handleViewerVimKey(msg), nil
}

// handleViewerEditKey is the active key handler while the viewer is in
// ViewerEdit mode. Edits flow into m.viewerEdit; arrow keys move the cursor;
// Esc returns to ViewerView. Save (Ctrl+S) and undo / redo (Ctrl+Z /
// Ctrl+Y / Ctrl+Shift+Z) work here too so the user doesn't have to drop
// back to view mode just to invoke them.
//
// Every mutating branch first calls pushHistory so undo can step back to
// pre-edit state. Cursor moves don't snapshot — they're not destructive
// and history would fill up with cursor-tracking entries.
func (m Model) handleViewerEditKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	// Undo / redo. Ctrl+Z (or Cmd+Z on Mac) is the universal undo;
	// Ctrl+Y / Cmd+Y the universal redo (Windows / Linux);
	// Ctrl+Shift+Z / Cmd+Shift+Z the macOS-style redo. Accept all so
	// the user's muscle memory works whatever they're used to. Mod
	// checks use Contains so capslock / numlock state bits don't break
	// the chord recognition.
	cmdLike := msg.Mod.Contains(tea.ModCtrl) || msg.Mod.Contains(tea.ModSuper) || msg.Mod.Contains(tea.ModMeta)
	if cmdLike && (msg.Code == 'z' || msg.Code == 'Z') {
		if msg.Mod.Contains(tea.ModShift) {
			return m.popRedo(), nil
		}
		return m.popUndo(), nil
	}
	if cmdLike && msg.Code == 'y' {
		return m.popRedo(), nil
	}
	if cmdLike && msg.Code == 's' {
		return m.saveViewerBuffer(), nil
	}
	// Selection-related cmd-likes:
	//   ⌃c / ⌘c     copy selection (or no-op if none active)
	//   ⌃a / ⌘a     select all (Mac/Linux/Win convention)
	// Ctrl+C is normally the global quit chord, but when the editor has
	// an active selection we steal it for copy. handleQuitKey checks
	// the same condition before firing.
	if cmdLike && msg.Code == 'c' && !msg.Mod.Contains(tea.ModShift) {
		m, cmd := m.copySelectionToClipboard()
		return m, cmd
	}
	if cmdLike && msg.Mod.Contains(tea.ModShift) && (msg.Code == 'c' || msg.Code == 'C') {
		// ⌃⇧c (Linux universal copy) — same as ⌃c above.
		m, cmd := m.copySelectionToClipboard()
		return m, cmd
	}
	// Emacs / readline navigation. Alt+b/f for word motion is the
	// default Option+arrow encoding in iTerm2 / Ghostty / Mac Terminal
	// (when "Left option key acts as: Esc+" is on) — many users have
	// these in muscle memory regardless of platform. Ctrl+A / Ctrl+E
	// line edges round out the set; both work when the user's terminal
	// strips Cmd+arrow into Home/End sequences and they fall back to
	// the textbook Unix shortcuts.
	if msg.Mod.Contains(tea.ModAlt) && (msg.Code == 'b' || msg.Code == 'B') {
		m = applySelectionExtension(m, msg)
		m.viewerEdit = moveCursorWordBack(m.viewerEdit)
		return m, nil
	}
	if msg.Mod.Contains(tea.ModAlt) && (msg.Code == 'f' || msg.Code == 'F') {
		m = applySelectionExtension(m, msg)
		m.viewerEdit = moveCursorWordForward(m.viewerEdit)
		return m, nil
	}
	if cmdLike && msg.Code == 'a' && !msg.Mod.Contains(tea.ModAlt) {
		// ⌃a / ⌘a → select all. Vim users would expect line-start, so
		// we ALSO accept Ctrl+A as line-start when no Cmd/Super is in
		// the mod set — keeps Linux Emacs muscle memory while giving
		// Mac users the modern editor shortcut they're used to.
		if msg.Mod.Contains(tea.ModSuper) || msg.Mod.Contains(tea.ModMeta) {
			m = m.selectAllBuffer()
			return m, nil
		}
		m = applySelectionExtension(m, msg)
		m.viewerEdit = moveCursorLineStart(m.viewerEdit)
		return m, nil
	}
	if msg.Mod.Contains(tea.ModCtrl) && msg.Code == 'e' {
		m = applySelectionExtension(m, msg)
		m.viewerEdit = moveCursorLineEnd(m.viewerEdit)
		return m, nil
	}

	switch msg.Code {
	case tea.KeyEsc:
		// Drop back to view. exitEditMode resyncs the cached content
		// from the buffer + rebuilds the syntax-highlight cache so
		// the user's edits surface with chroma colors immediately.
		return m.exitEditMode(), nil
	case tea.KeyEnter:
		m = m.clearSelection()
		m = m.pushHistory()
		m.viewerEdit = insertNewline(m.viewerEdit)
		m.viewerDirty = true
		return m, nil
	case tea.KeyBackspace:
		m = m.clearSelection()
		m = m.pushHistory()
		m.viewerEdit = deleteBackward(m.viewerEdit)
		m.viewerDirty = true
		return m, nil
	case tea.KeyDelete:
		m = m.clearSelection()
		m = m.pushHistory()
		m.viewerEdit = deleteForward(m.viewerEdit)
		m.viewerDirty = true
		return m, nil
	case tea.KeyUp:
		m = applySelectionExtension(m, msg)
		m.viewerEdit = moveCursorK(m.viewerEdit)
		return m, nil
	case tea.KeyDown:
		m = applySelectionExtension(m, msg)
		m.viewerEdit = moveCursorJ(m.viewerEdit)
		return m, nil
	case tea.KeyLeft:
		// Modifier-aware nav matches the conventions of every desktop
		// platform users come from:
		//   ⌃← (Linux/Win) and ⌥← (Mac Option) → previous word
		//   ⌘← (Mac Command) → start of line
		//   plain ← → previous char
		// We use Mod.Contains so capslock / numlock state bits don't
		// silently disable word motion. Shift held alongside any of
		// these starts / extends a selection.
		m = applySelectionExtension(m, msg)
		switch {
		case msg.Mod.Contains(tea.ModSuper) || msg.Mod.Contains(tea.ModMeta):
			m.viewerEdit = moveCursorLineStart(m.viewerEdit)
		case msg.Mod.Contains(tea.ModCtrl) || msg.Mod.Contains(tea.ModAlt):
			m.viewerEdit = moveCursorWordBack(m.viewerEdit)
		default:
			m.viewerEdit = moveCursorH(m.viewerEdit)
		}
		return m, nil
	case tea.KeyRight:
		m = applySelectionExtension(m, msg)
		switch {
		case msg.Mod.Contains(tea.ModSuper) || msg.Mod.Contains(tea.ModMeta):
			m.viewerEdit = moveCursorLineEnd(m.viewerEdit)
		case msg.Mod.Contains(tea.ModCtrl) || msg.Mod.Contains(tea.ModAlt):
			m.viewerEdit = moveCursorWordForward(m.viewerEdit)
		default:
			m.viewerEdit = moveCursorL(m.viewerEdit)
		}
		return m, nil
	case tea.KeyHome:
		m = applySelectionExtension(m, msg)
		m.viewerEdit = moveCursorLineStart(m.viewerEdit)
		return m, nil
	case tea.KeyEnd:
		m = applySelectionExtension(m, msg)
		m.viewerEdit = moveCursorLineEnd(m.viewerEdit)
		return m, nil
	case tea.KeyTab:
		// Tabs in edit mode insert spaces — matching the viewer's display
		// model where tab characters are pre-expanded for width
		// consistency. Inserting a literal '\t' would create a width
		// disagreement again the moment the buffer is re-rendered.
		m = m.clearSelection()
		m = m.pushHistory()
		for range viewerTabWidth {
			m.viewerEdit = insertRune(m.viewerEdit, ' ')
		}
		m.viewerDirty = true
		return m, nil
	}
	// Printable input flows through msg.Text — bubbletea v2 populates
	// this with the actual characters the user typed, handling Shift,
	// dead keys, IME composition, and non-latin layouts in one place.
	// Using msg.Code alone loses the case (Shift+a arrives as Code='a'
	// + Mod=ModShift under the kitty keyboard protocol, so insertRune
	// would write a lowercase 'a' even though the user wanted 'A').
	//
	// Code-only fallback covers TTYs that don't populate Text and the
	// internal test harness, where KeyPressMsg is constructed with
	// only Code set.
	if msg.Mod == 0 || msg.Mod == tea.ModShift {
		text := msg.Text
		if text == "" && msg.Code >= 0x20 && msg.Code != 0x7f {
			text = string(msg.Code)
		}
		if text != "" {
			m = m.clearSelection()
			m = m.pushHistory()
			for _, r := range text {
				m.viewerEdit = insertRune(m.viewerEdit, r)
			}
			m.viewerDirty = true
			return m, nil
		}
	}
	return m, nil
}

// handleDefaultKey handles keys in the default (non-explorer, non-viewer) context.
// Split into navigation keys and action keys to keep cyclomatic complexity low.
func (m Model) handleDefaultKey(msg tea.KeyPressMsg) Model {
	// Navigation keys — always available regardless of collapse state
	switch msg.Code {
	case tea.KeyTab:
		if m.effectiveMode() == LayoutTabs {
			// Tabs mode cycles within the tab strip only.
			return m.advanceTabFocus(tabDelta(msg.Mod))
		}
		return m.advanceFocus(tabDelta(msg.Mod))
	case tea.KeyUp:
		return m.handleArrowUp()
	case tea.KeyDown:
		return m.handleArrowDown()
	case tea.KeyLeft:
		return m.handleArrowLeft()
	case tea.KeyRight:
		return m.handleArrowRight()
	}
	// Action keys — panel state dependent or mode-specific
	return m.handleActionKey(msg)
}

// handleActionKey handles non-navigation keys in the passive/collapsed context.
func (m Model) handleActionKey(msg tea.KeyPressMsg) Model {
	switch msg.Code {
	case ']':
		return m.cycleSession(1)
	case '[':
		return m.cycleSession(-1)
	// Panel-jump shortcuts handled globally in handleKey before the
	// active-mode dispatch — falling through to handleActionKey is the
	// passive-mode-only path, which we no longer use for these keys.
	case ' ':
		// Space: in passive-expanded → collapse; in collapsed → expand to passive.
		if m.effectiveMode() == LayoutStack {
			m.collapse[m.focused].Toggle()
			m.persistLayoutIfEnabled()
		}
	case tea.KeyEnter:
		m = m.handleEnter()
	case tea.KeyBackspace:
		m = m.handleBackspace()
	case '+', '=':
		// +/- resize is only available in Expanded-Active mode (v11 three-state).
		// In passive mode, these are no-ops so the user must Enter first.
	case '-':
		// no-op in passive mode — see above
	default:
		if m.effectiveMode() == LayoutTabs {
			m = m.handleTabJump(msg.Code)
		}
	}
	return m
}

// tabDelta returns +1 for Tab and -1 for Shift+Tab.
func tabDelta(mod tea.KeyMod) int {
	if mod == tea.ModShift {
		return -1
	}
	return 1
}

// explorerActivate handles Enter on the highlighted explorer row. The
// behavior depends on which section currently owns the cursor:
//   - SectionMod → open the highlighted modified file in the viewer.
//   - SectionTree → toggle directory (folder) or open file (leaf), the
//     existing tree behavior.
func (m Model) explorerActivate() Model {
	if m.explorer.section == SectionMod {
		idx := m.explorer.modHighlight
		if idx < 0 || idx >= len(m.data.ModifiedFiles) {
			return m
		}
		mf := m.data.ModifiedFiles[idx]
		m = m.loadViewerFile(mf.Path)
		m = m.loadViewerDiff()
		return m
	}
	node := m.explorer.HighlightedNode()
	if node == nil {
		return m
	}
	if node.IsDir {
		m.explorer.ToggleDir(node.DirKey)
		m.explorer.RefreshRows(m.data)
		m = m.syncPanelViewport(PanelExplorer)
		return m
	}
	m = m.loadViewerFile(m.explorer.HighlightedPath())
	m = m.loadViewerDiff()
	return m
}

// handleTabJump processes numeric tab jumps (1..N) in tabs mode. The index
// maps directly onto the visible tab strip (m.activeTabs), so the jump keys
// stay in lockstep with what's shown and with renderTabStatusBar's "1-N
// jump" hint — no dead keys, no unreachable tabs.
func (m Model) handleTabJump(code rune) Model {
	tabs := m.activeTabs(m.data)
	idx := int(code - '1')
	if idx >= 0 && idx < len(tabs) {
		m = m.setFocus(tabs[idx].id)
	}
	return m
}

// handlePanelJumpKey routes plain-letter panel-jump shortcuts.
// Returns handled=true when the rune matched a panel; the model is
// updated with the new focus. Caller is responsible for filtering out
// modes where these shortcuts shouldn't fire (viewer, hook prompts,
// settings overlay).
//
// Letters chosen to be mnemonic and ⌃-free:
//
//	e → explorer    a → activity (calls)    d → diff
//	u → usage       s → servers             b → bash    c → cache
func (m Model) handlePanelJumpKey(code rune) (bool, Model) {
	switch code {
	case 'e':
		return true, m.setFocus(PanelExplorer)
	case 'a':
		return true, m.setFocus(PanelCalls)
	case 'd':
		if m.panelEnabled(PanelDiff) {
			return true, m.setFocus(PanelDiff)
		}
	case 'u':
		return true, m.setFocus(PanelUsage)
	case 's':
		if m.panelEnabled(PanelServers) {
			return true, m.setFocus(PanelServers)
		}
	case 'b':
		if m.panelEnabled(PanelBash) {
			return true, m.setFocus(PanelBash)
		}
	case 'c':
		if m.panelEnabled(PanelCache) {
			return true, m.setFocus(PanelCache)
		}
	}
	return false, m
}

// handleCtrl handles all ctrl+<key> combinations.
//
// Used to host ⌃l (cycle layout) and ⌃0 (fold-all-except-focused). Both
// retired in v0.6: layout mode lives in the settings overlay now (a
// "layout" category alongside per-panel toggles), and ⌃0 had no
// matching footer hint or help entry — a stealth binding that only
// confused users who stumbled on it. Per-panel jumps are plain letters
// via handlePanelJumpKey.
func (m Model) handleCtrl(code rune) Model {
	switch code {
	case 'n', 'N':
		return m.toggleDemoNotification()
	case 'l', 'L':
		// "⌃l mode" — advertised in the status bar and FullHelp.
		return m.cycleLayoutMode()
	case 'e', 'E':
		return m.setFocus(PanelExplorer)
	case 'a', 'A':
		return m.setFocus(PanelCalls)
	case 'd', 'D':
		return m.setFocus(PanelDiff)
	case '0':
		return m.collapseOthers()
	}
	return m
}

// collapseOthers collapses every collapsible panel except the focused one —
// the "⌃0 collapse others" chord from the keymap. The focused panel is
// explicitly expanded so the gesture always ends with exactly one panel
// open, even when it started collapsed.
func (m Model) collapseOthers() Model {
	for pid := PanelID(0); pid < panelCount; pid++ {
		if pid == m.focused {
			continue
		}
		m.collapse[pid].Collapse()
	}
	m.collapse[m.focused].Expand()
	m.persistLayoutIfEnabled()
	return m
}

// toggleDemoNotification fires the synthetic hook notification ONCE per
// session so the user can preview the active NotificationStyle without
// waiting for a real claude hook event. Subsequent Ctrl+N presses
// no-op — testing is a one-shot, not a toggle.
//
// Refuses to overwrite a real pending hook (one with hookPendingCh !=
// nil) — replacing it would orphan the response channel and leave
// hookserver hanging.
//
// Dismiss flow: Esc routes through handleEscapeKey → notifAck=true so
// the overlay disappears. The fired flag stays set; restart clyde to
// preview again.
func (m Model) toggleDemoNotification() Model {
	if m.demoNotificationFired {
		return m
	}
	if m.hookNotif.Active && m.hookPendingCh != nil {
		// Real pending hook is in flight — leave it alone, and don't
		// burn the one-shot on a no-op.
		return m
	}
	m.hookNotif = HookNotification{
		Active: true,
		Tool:   "Bash",
		KeyArg: "rm -rf /tmp/scratch",
		Cwd:    m.liveProject.CWD(),
	}
	m.notifAck = false
	m.demoNotificationFired = true
	return m
}

// cycleSession advances the active session-tab index by delta (+1 forward,
// -1 backward). The tab list runs from -1 (Σ aggregate) through
// len(tabs)-2 (last per-session tab); cycle wraps at both ends.
//
// No-op when the title bar isn't currently showing a session tab strip
// (single-session cwd) — the keybind silently does nothing rather than
// surprising the user with state that has no visible effect.
func (m Model) cycleSession(delta int) Model {
	tabs := m.data.Sessions
	if len(tabs) < 2 {
		return m
	}
	// tabs[0] is always the Σ aggregate; tabs[1..] are per-session entries.
	// sessionTabIndex == -1 selects Σ; 0..numSessions-1 selects a session.
	numSessions := len(tabs) - 1
	span := numSessions + 1 // +1 for Σ
	cur := m.sessionTabIndex
	if cur < -1 || cur >= numSessions {
		cur = 0
	}
	// Shift into [0, span) for modular arithmetic, step, shift back.
	pos := (cur + 1 + delta + span) % span
	return m.selectSession(pos - 1)
}

// selectSession switches the active session tab to the given index
// (-1 = Σ aggregate, 0..N-1 = per-session). Updates active flags on
// the tab strip + the leaderboard so the next render reflects the
// switch immediately, and resets the live-event debounce so the new
// session's events surface on the next refresh.
//
// Used by both keyboard cycling (cycleSession) and mouse clicks on
// the footer tab strip (handleSessionTabClick) so the two entry
// points stay in lockstep.
func (m Model) selectSession(idx int) Model {
	if len(m.data.Sessions) < 2 {
		return m
	}
	numSessions := len(m.data.Sessions) - 1
	if idx < -1 || idx >= numSessions {
		return m
	}
	m.sessionTabIndex = idx
	for i := range m.data.Sessions {
		if i == 0 {
			m.data.Sessions[i].Active = m.sessionTabIndex == -1
			continue
		}
		m.data.Sessions[i].Active = m.sessionTabIndex == i-1
	}
	if m.sessionTabIndex == -1 && !m.demoMode {
		m.data.SessionLeaderboard = sessionLeaderboardFromView(m.liveView, m.liveView.LastUpdate)
	} else {
		m.data.SessionLeaderboard = nil
	}
	m.data.SessionTabIndex = m.sessionTabIndex
	m.prevLatestEventID = ""
	m.prevEventCount = 0
	m.activePanelID = PanelNone
	return m
}

// handleEnter implements the three-state interaction model for Enter key.
//
// Semantics (v11):
//   - Collapsed → expand to Expanded-Passive.
//   - Expanded-Passive → transition to Expanded-Active.
//   - Expanded-Active is handled by handleActiveModeKey, not here.
//
// For explorer, Enter is handled before handleDefaultKey (in handleExplorerKey)
// so this branch only fires for non-explorer panels.
//
// Special case (v22+): in 2-col medium, explorer + servers are ALWAYS
// rendered expanded regardless of their collapse spring state. The user
// shouldn't need to "uncollapse" something that's already on screen, so for
// these panels at this breakpoint we skip the collapsed → expanded-passive
// step and go straight to active mode. (Previously the first Enter was a
// silent no-op while the spring transitioned, then the second Enter actually
// activated — a confusing fresh-run bug.)
func (m Model) handleEnter() Model {
	if m.effectiveMode() != LayoutStack {
		// Multi-col / tabs pin every panel expanded, so Enter's only job
		// is the Expanded-Passive → Expanded-Active promotion. Without
		// this there is no keyboard route to active mode (= no keyboard
		// scrolling) outside stack layout.
		if m.focused != PanelNone && m.focused != PanelNow {
			m = m.transitionToActive()
		}
		return m
	}
	pid := m.focused
	if m.bp == BreakpointMedium && (pid == PanelExplorer || pid == PanelServers) {
		wasCollapsed := m.collapse[pid].IsCollapsed()
		m.collapse[pid].Expand() // keep collapse state in sync with what's on screen
		if wasCollapsed {
			m.persistLayoutIfEnabled()
		}
		m = m.transitionToActive()
		return m
	}
	if m.collapse[pid].IsCollapsed() {
		// Collapsed → Expanded-Passive
		m.collapse[pid].Expand()
		m.persistLayoutIfEnabled()
		return m
	}
	// Expanded-Passive → Expanded-Active
	m = m.transitionToActive()
	return m
}

// handleBackspace collapses the focused panel; if already collapsed, moves to previous panel.
// In Expanded-Passive, Backspace → Collapsed.
// Expanded-Active Backspace is handled by handleActiveModeKey (→ Expanded-Passive).
func (m Model) handleBackspace() Model {
	if m.effectiveMode() != LayoutStack {
		return m
	}
	if !m.collapse[m.focused].IsCollapsed() {
		m.collapse[m.focused].Collapse()
		m.persistLayoutIfEnabled()
	} else {
		m = m.advanceFocus(-1)
	}
	return m
}

// handleArrowUp moves focus to the previous panel in passive/collapsed state.
//
// v11 three-state model: ↑/↓ ALWAYS navigate focus in passive state (collapsed or
// expanded-passive). Scrolling only happens in Expanded-Active state, routed by
// handleActiveModeKey. This fixes the v10 bug where ↑/↓ scrolled expanded panels.
//
// Explorer is a special case: handleExplorerKey intercepts before we get here.
func (m Model) handleArrowUp() Model {
	switch m.effectiveMode() {
	case LayoutStack:
		// In 2-col medium, navigate within the current column so ↑/↓ doesn't
		// accidentally jump from the left column (explorer/servers) to the
		// right column (now/calls/usage). In narrow/wide stack, all panels
		// share one column already so plain advanceFocus is correct.
		if m.bp == BreakpointMedium {
			m = m.advanceFocusInColumn2Col(-1)
		} else {
			m = m.advanceFocus(-1)
		}
	case LayoutMultiCol:
		m = m.advanceFocusInColumn(-1)
	}
	return m
}

// handleArrowDown moves focus to the next panel in passive/collapsed state.
//
// v11 three-state model: ↑/↓ ALWAYS navigate focus in passive state (collapsed or
// expanded-passive). Scrolling only happens in Expanded-Active state, routed by
// handleActiveModeKey.
func (m Model) handleArrowDown() Model {
	switch m.effectiveMode() {
	case LayoutStack:
		if m.bp == BreakpointMedium {
			m = m.advanceFocusInColumn2Col(1)
		} else {
			m = m.advanceFocus(1)
		}
	case LayoutMultiCol:
		m = m.advanceFocusInColumn(1)
	}
	return m
}

// handleArrowLeft switches tab (tabs mode) or moves focus across columns (multi-col / 2-col).
func (m Model) handleArrowLeft() Model {
	switch m.effectiveMode() {
	case LayoutTabs:
		m = m.advanceTabFocus(-1)
	case LayoutMultiCol:
		m = m.focusPrevColumn()
	case LayoutStack:
		// In 2-col medium layout, ← moves from right column to left column.
		if m.bp == BreakpointMedium {
			m = m.moveColumn2ColLeft()
		}
	}
	return m
}

// handleArrowRight switches tab (tabs mode) or moves focus across columns (multi-col / 2-col).
func (m Model) handleArrowRight() Model {
	switch m.effectiveMode() {
	case LayoutTabs:
		m = m.advanceTabFocus(1)
	case LayoutMultiCol:
		m = m.focusNextColumn()
	case LayoutStack:
		// In 2-col medium layout, → moves from left column to right column.
		if m.bp == BreakpointMedium {
			m = m.moveColumn2ColRight()
		}
	}
	return m
}

// handleSettingsKey processes key events when the settings overlay is open.
//
//	↑/↓    navigate rows (layout row sits above the panel toggles)
//	Enter  on layout → cycle mode; on a toggle → flip enabled
//	Tab    switch scope (Global ↔ Project) when a cwd is available
//	Esc    save the current scope's edits and close the overlay
func (m Model) handleSettingsKey(msg tea.KeyPressMsg) Model {
	cwd := m.liveProject.CWD()
	switch msg.Code {
	case tea.KeyEscape:
		// Apply edits to baseCfg, recompute effective cfg, save to file.
		// Remember-layout + notification style + cost threshold all live
		// globally — persist them all.
		//
		// Layout mode is deliberately NOT persisted here from m.layoutMode:
		// that field also carries transient runtime state (a --layout CLI
		// override, or a width-driven fallback) that the user never asked to
		// make permanent. Baking it in on every open+close would silently
		// rewrite config.toml's default_mode. Instead the layout chip's Enter
		// handler writes baseCfg.Layout.DefaultMode only when the user
		// actually cycles the mode (see settingsLayoutCursor below).
		m.baseCfg.RememberLayout = m.settingsRememberLayout
		m.baseCfg.NotificationStyle = m.settingsNotificationStyle
		m.baseCfg.NotifyCostThresholdUSD = m.settingsCostThreshold
		m.baseCfg.Theme = m.settingsTheme
		m.baseCfg.MascotPersona = m.settingsMascotPersona
		m.baseCfg.BootScreenEnabled = m.settingsBootScreenEnabled
		newBase, newEff := applySettings(m.baseCfg, m.settingsToggles, m.settingsScope, cwd)
		m.baseCfg = newBase
		m.cfg = newEff
		writeConfigFile(newBase)
		m.settingsOpen = false
	case tea.KeyUp:
		if m.settingsCursor > settingsCursorMin() {
			m.settingsCursor--
		}
	case tea.KeyDown:
		if m.settingsCursor < settingsCursorMaxForScope(m.settingsScope, m.settingsToggles) {
			m.settingsCursor++
		}
	case tea.KeyTab:
		newBase, _ := applySettings(m.baseCfg, m.settingsToggles, m.settingsScope, cwd)
		m.baseCfg = newBase
		if cwd == "" {
			break
		}
		if m.settingsScope == ScopeProject {
			m.settingsScope = ScopeGlobal
		} else {
			m.settingsScope = ScopeProject
		}
		m.settingsToggles = settingsOverlayFromConfig(m.baseCfg, m.settingsScope, cwd)
		m.settingsCursor = 0
	case tea.KeyEnter:
		switch {
		case m.settingsCursor == settingsThemeCursor:
			m.settingsTheme = m.settingsTheme.Next()
			m = m.applyTheme(m.settingsTheme)
		case m.settingsCursor == settingsMascotPersonaCursor:
			m.settingsMascotPersona = m.settingsMascotPersona.Next()
		case m.settingsCursor == settingsBootScreenCursor:
			m.settingsBootScreenEnabled = !m.settingsBootScreenEnabled
		case m.settingsCursor == settingsNotificationCursor:
			m.settingsNotificationStyle = m.settingsNotificationStyle.Next()
		case m.settingsCursor == settingsCostThresholdCursor:
			m.settingsCostThreshold = nextCostThreshold(m.settingsCostThreshold)
		case m.settingsCursor == settingsRememberLayoutCursor:
			m.settingsRememberLayout = !m.settingsRememberLayout
		case m.settingsCursor == settingsLayoutCursor:
			m = m.cycleLayoutMode()
			// The user explicitly changed the layout via the overlay, so this
			// is a real preference — persist it into baseCfg now. Esc no
			// longer copies m.layoutMode blindly (it can hold a transient CLI
			// override), so this is the only place a user-driven layout choice
			// reaches config.toml.
			m.baseCfg.Layout.DefaultMode = m.layoutMode
		case m.settingsCursor >= 0 && m.settingsCursor < len(m.settingsToggles):
			m.settingsToggles[m.settingsCursor].Enabled = !m.settingsToggles[m.settingsCursor].Enabled
		case m.settingsCursor >= len(m.settingsToggles) && m.settingsCursor <= startupCursorMax(m.settingsToggles):
			starts := startupCapableToggles(m.settingsToggles)
			k := m.settingsCursor - len(m.settingsToggles)
			if k >= 0 && k < len(starts) {
				idx := starts[k]
				m.settingsToggles[idx].StartsCollapsed = !m.settingsToggles[idx].StartsCollapsed
			}
		}
	}
	return m
}
