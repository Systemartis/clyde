package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// newTabsModel builds a model whose effective layout is Mode B (tabs).
// Default AutoSwitchThreshold is 80, preferred layout is LayoutTabs, so any
// width >= 80 resolves to LayoutTabs.
func newTabsModel() Model {
	m := NewModelWithConfig(DefaultConfig(), LayoutTabs)
	m.width = 130
	m.height = 40
	m.bp = DetectBreakpoint(130)
	// Make sure nothing steals the key dispatch in the gate tests.
	m.hookNotif = HookNotification{}
	m.viewerActive = false
	return m
}

// --- Finding #1/#2/#3: tabs status bar (version + honest hints) ---

func TestRenderTabStatusBar_HintsAndInjectedVersion(t *testing.T) {
	t.Parallel()
	m := NewModel()
	bar := stripANSI(renderTabStatusBar(m.styles, 130, 0, 2, "v1.2.3"))

	// #2: every advertised hint must be live. ⌃l is dispatched by handleCtrl
	// (cycleLayoutMode) and `?` opens settings — both belong in the footer.
	if !strings.Contains(bar, "⌃l mode") {
		t.Errorf("tabs footer should advertise the live `⌃l mode` chord; got %q", bar)
	}
	if !strings.Contains(bar, "settings") {
		t.Errorf("tabs footer should advertise `? settings`; got %q", bar)
	}
	// #3: jump hint reflects the real tab count (2 tabs → "1-2 jump").
	if !strings.Contains(bar, "1-2 jump") {
		t.Errorf("jump hint should reflect 2 tabs (`1-2 jump`); got %q", bar)
	}
	// #1: version comes from the injected string, not a hardcode.
	if !strings.Contains(bar, "v1.2.3") {
		t.Errorf("tabs footer should render the injected version; got %q", bar)
	}
	if strings.Contains(bar, "v0.5.0-proto") || strings.Contains(bar, "v0.6.0-proto") {
		t.Errorf("tabs footer must not hardcode a proto version; got %q", bar)
	}
}

func TestRenderTabStatusBar_SingleTabJumpHint(t *testing.T) {
	t.Parallel()
	m := NewModel()
	bar := stripANSI(renderTabStatusBar(m.styles, 130, 0, 1, "v1"))
	if !strings.Contains(bar, "1 jump") {
		t.Errorf("single-tab footer should show `1 jump`; got %q", bar)
	}
	if strings.Contains(bar, "1-1") {
		t.Errorf("single-tab footer should not show a range; got %q", bar)
	}
}

// --- Finding #4: the unreachable "now" tab is dropped and jumps line up ---

func TestActiveTabs_ExcludesNow(t *testing.T) {
	t.Parallel()
	m := NewModel()
	for _, tab := range m.activeTabs(m.data) {
		if tab.id == PanelNow {
			t.Fatalf("PanelNow must not be a tab — it is non-selectable and unreachable")
		}
		if !panelIsSelectable(tab.id) {
			t.Errorf("tab %v is not selectable; tabs must be reachable", tab.id)
		}
	}
}

func TestHandleTabJump_MapsToVisibleTabs(t *testing.T) {
	t.Parallel()
	m := newTabsModel()
	tabs := m.activeTabs(m.data)
	if len(tabs) < 2 {
		t.Fatalf("need >=2 tabs for this test; got %d", len(tabs))
	}

	// '1' focuses the first visible tab (previously a silent no-op → PanelNow).
	m1 := m.handleTabJump('1')
	if m1.focused != tabs[0].id {
		t.Errorf("`1` should focus first tab %v; got %v", tabs[0].id, m1.focused)
	}
	if !panelIsSelectable(m1.focused) {
		t.Errorf("`1` landed on a non-selectable panel %v (silently rejected)", m1.focused)
	}

	// The last tab's numeric key reaches the last tab.
	n := len(tabs)
	mn := m.handleTabJump(rune('0' + n))
	if mn.focused != tabs[n-1].id {
		t.Errorf("`%d` should focus last tab %v; got %v", n, tabs[n-1].id, mn.focused)
	}
}

// --- Finding #5: help open must not underflow the tabs panel height ---

func TestRenderTabs_HeightStableWhenHelpOpen(t *testing.T) {
	t.Parallel()
	m := newTabsModel()

	closed := strings.Count(m.renderTabs(), "\n") + 1
	m.helpOpen = true
	open := strings.Count(m.renderTabs(), "\n") + 1

	if closed != open {
		t.Errorf("tabs render height changed when help opened: closed=%d open=%d", closed, open)
	}
	if open != m.height {
		t.Errorf("tabs render should fill the terminal height %d; got %d with help open", m.height, open)
	}
}

// --- Finding #6: tabs navigation stays on the tab strip ---

func TestAdvanceTabFocus_StaysOnStrip(t *testing.T) {
	t.Parallel()
	m := newTabsModel()
	for i := 0; i < 8; i++ {
		m = m.advanceTabFocus(1)
		if !m.isTabStripPanel(m.focused) {
			t.Fatalf("advanceTabFocus landed off-strip on %v after %d steps", m.focused, i+1)
		}
	}
	for i := 0; i < 8; i++ {
		m = m.advanceTabFocus(-1)
		if !m.isTabStripPanel(m.focused) {
			t.Fatalf("advanceTabFocus(-1) landed off-strip on %v", m.focused)
		}
	}
}

func TestArrowNav_TabsMode_StaysOnStrip(t *testing.T) {
	t.Parallel()
	m := newTabsModel()
	for i := 0; i < 6; i++ {
		m = m.handleArrowRight()
		if !m.isTabStripPanel(m.focused) {
			t.Fatalf("→ in tabs mode focused non-tab panel %v", m.focused)
		}
	}
}

func TestLetterJump_TabsMode_IgnoresNonTabPanels(t *testing.T) {
	t.Parallel()
	m := newTabsModel()
	m = m.setFocus(PanelCalls) // a real tab
	before := m.focused

	// 'e' targets the explorer, which is NOT a tab in tabs mode. It must be
	// ignored so the strip highlight and shown panel stay in sync.
	next, _ := m.handleKey(tea.KeyPressMsg{Code: 'e'})
	nm := next.(Model)
	if nm.focused == PanelExplorer {
		t.Errorf("`e` in tabs mode must not focus the (non-tab) explorer; focus=%v", nm.focused)
	}
	if nm.focused != before {
		t.Errorf("`e` in tabs mode should leave focus unchanged (%v); got %v", before, nm.focused)
	}

	// 'u' targets usage, which IS a tab, so it should be honored.
	next2, _ := m.handleKey(tea.KeyPressMsg{Code: 'u'})
	nm2 := next2.(Model)
	if nm2.focused != PanelUsage {
		t.Errorf("`u` in tabs mode should focus the usage tab; got %v", nm2.focused)
	}
}

// --- Finding #7: mouse routing in tabs mode ---

func TestTabsBounds_SinglePanel(t *testing.T) {
	t.Parallel()
	m := newTabsModel()
	m = m.setFocus(PanelUsage)

	bounds := m.buildPanelBounds()
	if len(bounds) != 1 {
		t.Fatalf("tabs mode should expose a single clickable panel; got %d bounds", len(bounds))
	}
	b := bounds[0]
	if b.pid != PanelUsage {
		t.Errorf("tabs bound should be the shown panel PanelUsage; got %v", b.pid)
	}
	// The panel starts BELOW the 1-row tab strip (row 3, after the 2-row title).
	if b.yMin != 3 {
		t.Errorf("tabs panel should start at row 3 (title 2 + strip 1); got yMin=%d", b.yMin)
	}

	// A body click in the middle of the visible panel maps to the shown
	// panel, not a phantom stacked panel.
	cx := (b.xMin + b.xMax) / 2
	cy := (b.yMin + b.yMax) / 2
	pid, ok := m.panelAtPos(cx, cy)
	if !ok || pid != PanelUsage {
		t.Errorf("body click at (%d,%d) should map to PanelUsage; got %v ok=%v", cx, cy, pid, ok)
	}
}

func TestTabsStripClick_SwitchesTab(t *testing.T) {
	t.Parallel()
	m := newTabsModel()
	tabs := m.activeTabs(m.data)
	lastIdx := len(tabs) - 1

	// Find a column on the strip row (y=2) that maps to the last tab.
	clickX := -1
	for x := 0; x < m.width; x++ {
		if idx, ok := m.tabStripAtPos(x, 2); ok && idx == lastIdx {
			clickX = x
			break
		}
	}
	if clickX < 0 {
		t.Fatalf("could not locate the last tab in the strip via tabStripAtPos")
	}

	res, _ := m.handleMouseClick(tea.MouseClickMsg{X: clickX, Y: 2, Button: tea.MouseLeft})
	nm := res.(Model)
	if nm.focused != tabs[lastIdx].id {
		t.Errorf("clicking the last strip tab should focus %v; got %v", tabs[lastIdx].id, nm.focused)
	}
}

// --- Finding #8: header-click collapse of an active panel clears active ---

func TestHeaderClickCollapse_ExitsActiveMode(t *testing.T) {
	t.Parallel()
	m := NewModel()
	m.width = 70
	m.height = 40
	m.bp = DetectBreakpoint(70) // narrow single-stack

	m = m.setFocus(PanelCalls)
	m = m.transitionToActive()
	if m.activePanelID != PanelCalls {
		t.Fatalf("precondition failed: PanelCalls should be active, got %v", m.activePanelID)
	}

	var b panelBounds
	found := false
	for _, bb := range m.buildPanelBounds() {
		if bb.pid == PanelCalls {
			b = bb
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no bounds for PanelCalls in narrow stack")
	}

	// Click the top-left header cell (left of the right-aligned active badge).
	res, _ := m.handleMouseClick(tea.MouseClickMsg{X: b.xMin, Y: b.yMin, Button: tea.MouseLeft})
	nm := res.(Model)

	if nm.isActiveMode() {
		t.Errorf("collapsing an active panel via header click must exit active mode; activePanelID=%v", nm.activePanelID)
	}
	if !nm.collapse[PanelCalls].IsCollapsed() {
		t.Errorf("panel should be collapsed after the header click")
	}
}

// --- Finding #9: KeyMap advertises exactly the live bindings ---

// TestKeyMap_AdvertisesLiveBindings pins the registry to the dispatched key
// set: the ⌃ chords handled by handleCtrl (⌃l/⌃0/⌃e/⌃a/⌃d), the overlays
// (h/?), session cycling ([/]), and the plain-letter panel jumps all appear
// in the help surfaces. If a chord is retired from handleCtrl, remove its
// binding here too — the registry must never advertise a dead key.
func TestKeyMap_AdvertisesLiveBindings(t *testing.T) {
	t.Parallel()
	k := DefaultKeyMap()

	var bindings []interface{ Keys() []string }
	for _, b := range k.ShortHelp() {
		bindings = append(bindings, b)
	}
	for _, row := range k.FullHelp() {
		for _, b := range row {
			bindings = append(bindings, b)
		}
	}

	seen := map[string]bool{}
	for _, b := range bindings {
		for _, kb := range b.Keys() {
			seen[kb] = true
		}
	}

	// Every dispatched control must be advertised.
	for _, want := range []string{
		"ctrl+l", "ctrl+0", "ctrl+e", "ctrl+a", "ctrl+d", // handleCtrl chords
		"h", "?", // overlays
		"[", "]", // session tabs
		"e", // panel jumps (representative letter)
	} {
		if !seen[want] {
			t.Errorf("help should advertise the live binding %q", want)
		}
	}
}

// --- Finding #10: ⌃n is documented on the now-panel help ---

func TestNowPanelHelp_DocumentsCtrlN(t *testing.T) {
	t.Parallel()
	found := false
	for _, e := range helpEntriesForPanel(PanelNow) {
		if strings.Contains(e.Key, "⌃n") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("now-panel help must document ⌃n (demo notification preview)")
	}
}
