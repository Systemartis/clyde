package tui

import (
	"fmt"
	"strings"
)

// statusBarHeight returns the rendered height of the status bar block,
// which depends on helpOpen: normally 2 rows (dashes + content), but
// the help-mode footer expands to a 4-row grouped command palette
// because cramming every global keybind into one line was unreadable.
//
// Layout calls (e.g. buildPanelBounds, gridH calculations) must use
// this — hardcoded 2 will leak rows of help footer onto the panels
// behind it.
func statusBarHeight(helpOpen bool) int {
	if helpOpen {
		return 4 // dashes + 3 grouped lines
	}
	return 2
}

// renderStatusBar renders the bottom bar. Three states:
//
// Normal:    h commands · [Σ all] [● fix-auth ⚠] [docs]                v0.6.0
//
// Help on (3-row grouped block):
//
//	nav    ↑↓ focus · ←→ columns · tab next
//	panel  ↵ expand · ⌫ collapse · ] [ session
//	modal  esc close help · ? settings · q quit                        v0.6.0
//
// Each row groups a category of cross-panel commands. The per-panel
// keys (e/a/d/u/s/b/c, y/Y, etc.) live inside each panel's help body,
// so the footer never has to fight a panel for the same key's home.
//
// toast (e.g. "✓ copied: foo/bar.go") always wins the left side until
// it expires, regardless of helpOpen.
func renderStatusBar(s Styles, width int, _ bool, toast string, tabs []SessionTab, helpOpen bool, appVersion string) string {
	sep := s.StatusSep.Render(" · ")
	key := func(k string) string { return s.StatusKey.Render(k) }
	txt := func(t string) string { return s.StatusBar.Render(t) }
	dashes := s.StatusSep.Render(strings.Repeat("─", width))

	// Toast wins everything else. One-line footer.
	if toast != "" {
		return dashes + "\n" + buildBarRow(width, s.StatusKey.Render(toast), s.StatusVer.Render(appVersion))
	}

	if !helpOpen {
		var left string
		hHint := key("h") + txt(" commands")
		if len(tabs) > 1 {
			left = hHint + sep + renderSessionTabs(s, tabs)
		} else {
			left = hHint
		}
		right := s.StatusVer.Render(appVersion)
		return dashes + "\n" + buildBarRow(width, left, right)
	}

	// Help mode: 3 grouped rows. Category labels left-pad each row at
	// a fixed width so the keys line up vertically — easier to scan
	// than a single long bullet-separated line.
	const labelW = 8 // " nav   ", " panel ", " modal "
	labelStyle := s.StatusBar
	categoryRow := func(label, body string) string {
		padded := labelStyle.Render(padRight(label, labelW))
		content := padded + body
		bodyW := ansiWidth(content)
		gap := width - bodyW - 1
		if gap < 0 {
			gap = 0
		}
		return " " + content + strings.Repeat(" ", gap)
	}

	rowNav := categoryRow("nav",
		key("↑↓")+txt(" focus")+sep+
			key("←→")+txt(" columns")+sep+
			key("tab")+txt(" next"))

	rowPanel := categoryRow("panel",
		key("↵")+txt(" expand")+sep+
			key("⌫")+txt(" collapse")+sep+
			key("] [")+txt(" session"))

	// Last row carries the version too. h and esc both close the help
	// overlay (and Backspace, but listing three keys is overkill — h
	// is the toggle the user already knows, esc is the universal
	// "back out"). Bullet separator between hints and version keeps
	// the row visually consistent with the in-row separators above.
	modalLeft := key("h") + txt("/") + key("esc") + txt(" close help") + sep +
		key("?") + txt(" settings") + sep +
		key("q") + txt(" quit") + sep +
		s.StatusVer.Render(appVersion)
	rowModal := categoryRow("modal", modalLeft)

	return dashes + "\n" + rowNav + "\n" + rowPanel + "\n" + rowModal
}

// buildBarRow builds a single status-bar row with left text, version on
// right, and a flexible spacer between. Always returns a string padded
// to width.
func buildBarRow(width int, left, right string) string {
	leftW := ansiWidth(left)
	rightW := ansiWidth(right)
	gapW := width - leftW - rightW - 2
	if gapW < 1 {
		gapW = 1
	}
	return " " + left + strings.Repeat(" ", gapW) + right + " "
}

// renderTabStatusBar renders the status bar for Mode B (tabs) with tab-specific hints.
//
// The jump hint reflects the real number of tabs (only 1..totalTabs jump
// to anything); ⌃l cycles the layout (dispatched by handleCtrl) and `?`
// opens settings, which also carries the layout chip. The version comes
// from the same injected string as the stack-mode footer so the two never
// disagree.
func renderTabStatusBar(s Styles, width int, activeTab int, totalTabs int, appVersion string) string {
	sep := s.StatusSep.Render(" · ")

	key := func(k string) string { return s.StatusKey.Render(k) }
	txt := func(t string) string { return s.StatusBar.Render(t) }

	jumpKey := "1"
	if totalTabs > 1 {
		jumpKey = fmt.Sprintf("1-%d", totalTabs)
	}
	left := key(jumpKey) + txt(" jump") +
		sep +
		key("tab") + txt(" next") +
		sep +
		key("⌃l") + txt(" mode") +
		sep +
		key("?") + txt(" settings")

	right := s.StatusVer.Render(appVersion)

	_ = activeTab

	dashes := s.StatusSep.Render(strings.Repeat("─", width))
	leftW := ansiWidth(left)
	rightW := ansiWidth(right)
	gapW := width - leftW - rightW - 2
	if gapW < 1 {
		gapW = 1
	}
	bar := " " + left + strings.Repeat(" ", gapW) + right + " "
	return dashes + "\n" + bar
}
