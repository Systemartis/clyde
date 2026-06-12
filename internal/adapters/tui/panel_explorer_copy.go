package tui

import (
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"
)

// copyToastDuration is how long a "✓ copied: …" toast stays visible in
// the status bar before a clearCopyToastMsg removes it.
const copyToastDuration = 1500 * time.Millisecond

// explorerDoubleClickWindow is the maximum gap between two left-clicks on
// the SAME explorer row that still counts as a double-click. Past this
// window the second click is treated as a fresh single-click (open).
const explorerDoubleClickWindow = 400 * time.Millisecond

// copyToastLabelMax caps the rendered path inside the toast so the
// status bar doesn't wrap when copying a deeply-nested file. The
// clipboard payload itself is always the full path.
const copyToastLabelMax = 60

// clearCopyToastMsg is dispatched ~copyToastDuration after a toast was
// set. The handler clears the toast only if its expiry has elapsed,
// which lets a newer yank during the window keep its own timer.
type clearCopyToastMsg struct{}

// clearCopyToastCmd schedules a clearCopyToastMsg after the given delay.
func clearCopyToastCmd(after time.Duration) tea.Cmd {
	return tea.Tick(after, func(time.Time) tea.Msg {
		return clearCopyToastMsg{}
	})
}

// copyExplorerHighlight copies the highlighted explorer item to the
// system clipboard via OSC 52 and queues a status-bar toast. When
// basenameOnly is true it copies just the file name; otherwise the
// absolute path (or relative path in demo mode / when cwd is unknown).
//
// Used by the y / Y keybinds in active mode and by double-click in the
// mouse handler. No-op when nothing is highlighted.
func (m Model) copyExplorerHighlight(basenameOnly bool) (Model, tea.Cmd) {
	payload, label, ok := m.explorerCopyTarget(basenameOnly)
	if !ok {
		return m, nil
	}
	m.copyToast = "✓ copied: " + label
	m.copyToastExpires = time.Now().Add(copyToastDuration)
	return m, tea.Batch(tea.SetClipboard(payload), clearCopyToastCmd(copyToastDuration))
}

// explorerCopyTarget resolves what to put on the clipboard for the
// currently highlighted row. ok=false when no row is selected (empty
// tree, mod cursor pointing past the slice, etc.) so callers can no-op.
//
// payload: full string written to the clipboard.
// label:   display string for the toast (truncated when long).
func (m Model) explorerCopyTarget(basenameOnly bool) (payload, label string, ok bool) {
	rel, ok := m.explorerHighlightedRelPath()
	if !ok {
		return "", "", false
	}
	if basenameOnly {
		b := filepath.Base(rel)
		return b, b, true
	}
	abs := m.absExplorerPath(rel)
	return abs, truncateCopyLabel(abs), true
}

// explorerHighlightedRelPath returns the relative path of whichever
// section currently owns the explorer cursor (modified-files window or
// the tree). Returns ok=false when the cursor would point at nothing —
// empty tree or out-of-range mod index.
func (m Model) explorerHighlightedRelPath() (string, bool) {
	if m.explorer.section == SectionMod {
		idx := m.explorer.modHighlight
		if idx < 0 || idx >= len(m.data.ModifiedFiles) {
			return "", false
		}
		return m.data.ModifiedFiles[idx].Path, true
	}
	node := m.explorer.HighlightedNode()
	if node == nil || node.Path == "" {
		return "", false
	}
	return node.Path, true
}

// absExplorerPath turns a relative tree/mod-section path into an
// absolute filesystem path using the live project's cwd. Demo mode and
// missing cwd both fall through to the relative path unchanged — there
// is no usable absolute form in those cases.
func (m Model) absExplorerPath(rel string) string {
	if rel == "" {
		return rel
	}
	if filepath.IsAbs(rel) {
		return rel
	}
	if m.demoMode {
		return rel
	}
	cwd := m.liveView.Project.CWD()
	if cwd == "" {
		return rel
	}
	return filepath.Join(cwd, rel)
}

// truncateCopyLabel shortens long absolute paths for the toast. Keeps
// the suffix (filename + a few parent dirs) since that is what the user
// actually needs to confirm.
func truncateCopyLabel(p string) string {
	runes := []rune(p)
	if len(runes) <= copyToastLabelMax {
		return p
	}
	return "…" + string(runes[len(runes)-copyToastLabelMax+1:])
}

// isExplorerDoubleClick reports whether a click on `path` should be
// treated as the second half of a double-click (i.e. follows a click on
// the same path within explorerDoubleClickWindow). Callers reset the
// last-click timestamp afterwards to prevent triple-clicks from chain-
// firing copies.
func (m Model) isExplorerDoubleClick(path string) bool {
	if m.lastExplorerClickAt.IsZero() || path == "" {
		return false
	}
	if path != m.lastExplorerClickPath {
		return false
	}
	return time.Since(m.lastExplorerClickAt) <= explorerDoubleClickWindow
}

// recordExplorerClick stamps the latest clicked-path + timestamp so the
// next click can decide whether it's a double-click.
func (m Model) recordExplorerClick(path string) Model {
	m.lastExplorerClickPath = path
	m.lastExplorerClickAt = time.Now()
	return m
}

// resetExplorerClick clears the click stamp — used after a double-click
// fires so a third click doesn't immediately count as another double.
func (m Model) resetExplorerClick() Model {
	m.lastExplorerClickPath = ""
	m.lastExplorerClickAt = time.Time{}
	return m
}
