package tui

// v1_bugs_test.go contains regression tests for the six V1 bugs fixed in this batch:
//   Bug 1: Explorer not scrollable + not resizable in active mode
//   Bug 2: 1M context variants (covered in pricing_test.go)
//   Bug 3: Live diff still mocked (applyLiveView always clears mock in live mode)
//   Bug 4: Text viewer shows real files in live mode
//   Bug 5: Usage scope — session / 5h / week all visible
//   Bug 6: Title bar wired to real data

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Systemartis/clyde/internal/application/livesession"
	"github.com/Systemartis/clyde/internal/domain/event"
	"github.com/Systemartis/clyde/internal/domain/usage"
)

// ─── Bug 1: Explorer scrollable / resizable in active mode ────────────────────

// TestExplorerActiveModeUsesViewport verifies that when the explorer is in
// Expanded-Active state, the rendered panel uses a double border (wrapPanelActive)
// indicating the viewport is being used for scrolling.
func TestExplorerActiveModeUsesViewport(t *testing.T) {
	t.Parallel()

	m := NewModel()
	m.width = 130
	m.height = 50
	m.bp = BreakpointMedium
	m.layoutMode = LayoutStack
	m.focused = PanelExplorer
	m.collapse[PanelExplorer] = NewPanelCollapseState(false, 20)
	m.panelHeights[PanelExplorer] = 20
	m.activePanelID = PanelExplorer
	m = m.syncPanelViewport(PanelExplorer)
	m.panelVPs[PanelExplorer].SetHeight(18)

	leftW := (130 * 40) / 100
	if leftW > 50 {
		leftW = 50
	}

	rendered := stripANSI(m.renderExpandedPanel(PanelExplorer, leftW, 20, m.focused == PanelExplorer))

	// Active mode must use double border (╔...╗)
	hasDoubleBorder := strings.Contains(rendered, "╔") && strings.Contains(rendered, "╗")
	if !hasDoubleBorder {
		t.Error("explorer in active mode must use double border ╔...╗")
	}
}

// TestExplorerPassiveModeDoesNotUseViewport verifies that the explorer in
// Expanded-Passive state renders with the normal rounded border (not double border).
func TestExplorerPassiveModeDoesNotUseViewport(t *testing.T) {
	t.Parallel()

	m := NewModel()
	m.width = 130
	m.height = 50
	m.bp = BreakpointMedium
	m.layoutMode = LayoutStack
	m.focused = PanelExplorer
	m.collapse[PanelExplorer] = NewPanelCollapseState(false, 20)
	// NOT in active mode
	m.activePanelID = PanelNone

	leftW := (130 * 40) / 100
	if leftW > 50 {
		leftW = 50
	}

	rendered := stripANSI(m.renderExpandedPanel(PanelExplorer, leftW, 20, true))

	// Passive mode must NOT have double border
	hasDoubleBorder := strings.Contains(rendered, "╔") && strings.Contains(rendered, "╗")
	if hasDoubleBorder {
		t.Error("explorer in passive mode must NOT use double border — only active mode uses it")
	}
}

// TestExplorerUpDownInPassiveAdvancesFocus verifies that ↑/↓ in the passive
// mode moves panel focus (consistent with other panels), NOT tree highlight.
func TestExplorerUpDownInPassiveAdvancesFocus(t *testing.T) {
	t.Parallel()

	m := NewModel()
	m.width = 130
	m.height = 50
	m.bp = BreakpointMedium
	m.layoutMode = LayoutStack
	m.focused = PanelExplorer
	// Explorer is collapsed — passive mode
	m.collapse[PanelExplorer] = NewPanelCollapseState(true, 20)
	m.activePanelID = PanelNone

	initialHL := m.explorer.highlighted

	// Press ↓ — should move focus, not change explorer tree highlight
	msg := tea.KeyPressMsg{Code: tea.KeyDown}
	m2, _ := m.handleModeKey(msg)

	if m2.explorer.highlighted != initialHL {
		t.Errorf("↓ in collapsed explorer should not change tree highlight; got %d want %d",
			m2.explorer.highlighted, initialHL)
	}
	// Focus should have moved to the next panel
	if m2.focused == PanelExplorer {
		t.Error("↓ in collapsed explorer should move focus away from explorer")
	}
}

// ─── Bug 3: Diff panel wires real data in live mode ───────────────────────────

// TestApplyLiveView_DiffAlwaysUpdatesInLiveMode verifies that applyLiveView
// replaces DiffLines with real data (even empty) when not in demo mode.
// This ensures the mock diff (auth.ts) is cleared when real data flows in.
func TestApplyLiveView_DiffAlwaysUpdatesInLiveMode(t *testing.T) {
	t.Parallel()

	m := NewModel()
	m.demoMode = false // live mode

	// Set a live view with NO diff hunks (clean working tree).
	m.liveView = livesession.View{
		Events:    []event.Event{makeDummyAssistantEvent(t, "ev1")},
		DiffFile:  "",
		DiffHunks: nil, // no diff
	}

	// Pre-populate mock DiffLines with something non-nil (simulating stale mock data).
	m.data.DiffLines = []DiffLine{
		{Kind: DiffHunkKind, Text: "@@ mock diff @@"},
	}
	m.data.DiffFile = "auth.ts · +28 −6"

	m = m.applyLiveView()

	// In live mode, DiffLines must be cleared when there are no real hunks.
	if len(m.data.DiffLines) != 0 {
		t.Errorf("DiffLines must be empty when DiffHunks is nil in live mode; got %d lines", len(m.data.DiffLines))
	}
	if m.data.DiffFile != "" {
		t.Errorf("DiffFile must be empty when there are no hunks; got %q", m.data.DiffFile)
	}
}

// TestApplyLiveView_DiffPreservesInDemoMode verifies that demo mode keeps the
// mock DiffLines unchanged.
func TestApplyLiveView_DiffPreservesInDemoMode(t *testing.T) {
	t.Parallel()

	m := NewModel()
	m.demoMode = true // demo mode — should NOT touch DiffLines

	// Set a live view with no diff hunks.
	m.liveView = livesession.View{
		Events: []event.Event{makeDummyAssistantEvent(t, "ev1")},
	}

	// Pre-populate with mock diff data.
	m.data.DiffLines = []DiffLine{
		{Kind: DiffHunkKind, Text: "@@ mock diff @@"},
	}
	origLines := len(m.data.DiffLines)

	m = m.applyLiveView()

	// Demo mode must preserve mock DiffLines.
	if len(m.data.DiffLines) != origLines {
		t.Errorf("demo mode must not clear DiffLines; got %d want %d", len(m.data.DiffLines), origLines)
	}
}

// ─── Bug 4: Text viewer reads real files ──────────────────────────────────────

// TestReadFileForViewer_SmallFile verifies that readFileForViewer returns the
// correct content for a small existing file.
func TestReadFileForViewer_SmallFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	content := "hello\nworld\n"
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	got, err := readFileForViewer(path, "")
	if err != nil {
		t.Fatalf("readFileForViewer: %v", err)
	}
	if got != content {
		t.Errorf("got %q; want %q", got, content)
	}
}

// TestReadFileForViewer_RelativePath verifies relative paths are resolved
// against the cwd parameter.
func TestReadFileForViewer_RelativePath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	content := "relative file content"
	if err := os.WriteFile(filepath.Join(dir, "rel.go"), []byte(content), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	got, err := readFileForViewer("rel.go", dir)
	if err != nil {
		t.Fatalf("readFileForViewer (relative): %v", err)
	}
	if got != content {
		t.Errorf("got %q; want %q", got, content)
	}
}

// TestReadFileForViewer_TooLarge verifies that oversized files return the
// "file too large" message instead of content.
func TestReadFileForViewer_TooLarge(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Write a file that exceeds maxViewerBytes.
	large := make([]byte, maxViewerBytes+1)
	path := filepath.Join(dir, "large.bin")
	if err := os.WriteFile(path, large, 0o644); err != nil {
		t.Fatalf("write large file: %v", err)
	}

	got, err := readFileForViewer(path, "")
	if err != nil {
		t.Fatalf("readFileForViewer (large): %v", err)
	}
	if !strings.Contains(got, "file too large") {
		t.Errorf("expected 'file too large' message; got %q", got[:min(60, len(got))])
	}
}

// TestReadFileForViewer_NotFound verifies that a missing file returns an error.
func TestReadFileForViewer_NotFound(t *testing.T) {
	t.Parallel()

	_, err := readFileForViewer("/nonexistent/path/file.go", "")
	if err == nil {
		t.Error("expected error for non-existent file; got nil")
	}
}

// ─── Bug 5: Usage panel shows session / 5h / week ────────────────────────────

// TestDeriveUsageFields_MultiWindowDisplayStrings verifies that deriveUsageFields
// populates UsageSession, Usage5hDisplay, and UsageWeekDisplay from the view.
func TestDeriveUsageFields_MultiWindowDisplayStrings(t *testing.T) {
	t.Parallel()

	v := livesession.View{
		Events:         []event.Event{makeDummyAssistantEvent(t, "ev1")},
		TotalUsage:     usage.Usage{Input: 47_200},
		Usage5h:        usage.Usage{Input: 89_400},
		UsageWeek:      usage.Usage{Input: 412_000},
		CurrentModel:   "claude-opus-4-7",
		AssistantTurns: 1,
	}

	d := V3MockData()
	d = deriveUsageFields(v, d)

	if d.UsageSession.Empty {
		t.Error("UsageSession must not be empty after deriveUsageFields")
	}
	if d.Usage5h.Empty {
		t.Error("Usage5h must not be empty after deriveUsageFields")
	}
	if d.UsageWeek.Empty {
		t.Error("UsageWeek must not be empty after deriveUsageFields")
	}
}

// TestUsagePanelRenders_MultiWindowRows verifies that all three usage window
// rows appear in the rendered usage panel.
func TestUsagePanelRenders_MultiWindowRows(t *testing.T) {
	t.Parallel()

	m := NewModel()
	// Ensure mock data has multi-window values (V3MockData already sets them).
	panel := stripANSI(renderUsage(m.styles, m.palette, m.data, m.progTokens, m.progReset, 50, 30, false))

	for _, label := range []string{"session ctx", "5h session", "weekly · all models"} {
		if !strings.Contains(panel, label) {
			t.Errorf("usage panel must show %q row; panel content:\n%s", label, panel)
		}
	}
}

// ─── Bug 6: Title bar wired to real data ─────────────────────────────────────

// TestRenderTitleBar_DemoMode verifies that demo mode uses MockData fields.
func TestRenderTitleBar_DemoMode(t *testing.T) {
	t.Parallel()

	m := NewModel() // demoMode=true
	m.width = 120

	rendered := stripANSI(m.View().Content)

	// Title bar is the first line — check it contains the mock duration and tokens.
	lines := strings.Split(rendered, "\n")
	if len(lines) < 1 {
		t.Fatal("no output from View")
	}
	titleLine := lines[0]

	if !strings.Contains(titleLine, "1h 24m") {
		t.Errorf("demo mode title bar should contain mock duration '1h 24m'; got %q", titleLine)
	}
	if !strings.Contains(titleLine, "47k") {
		t.Errorf("demo mode title bar should contain mock tokens '47k'; got %q", titleLine)
	}
	// v22+: with Max 5x in mock, $ is hidden in the title bar (subscribers).
	// Cost visibility is covered by TestRenderTitleBar_HidesCostForSubscriber and
	// TestRenderTitleBar_ShowsCostForAPIKey below.
	if strings.Contains(titleLine, "$1.42") {
		t.Errorf("demo mode title bar should NOT show $ for subscriber mock; got %q", titleLine)
	}
}

// TestRenderTitleBar_HidesCostForSubscriber verifies the $ is hidden when the
// user is on a Pro/Max plan (PlanTier set).
func TestRenderTitleBar_HidesCostForSubscriber(t *testing.T) {
	t.Parallel()

	d := MockData{
		ProjectPath: "~/x/", ProjectName: "p", Model: "opus 4.7",
		Duration: "1h", Tokens: "47k", Cost: "$1.42",
		PlanTier: "Max 5x",
	}
	s := NewStyles(TokyoNightPalette())
	got := stripANSI(renderTitleBar(s, TokyoNightPalette(), d, FrameState{}, 200, true, livesession.View{}, time.Now()))
	if strings.Contains(got, "$1.42") {
		t.Errorf("subscriber title bar should NOT show $; got %q", got)
	}
}

// TestRenderTitleBar_ShowsCostForAPIKey verifies the $ stays visible when no
// plan tier is detected (API-key user).
func TestRenderTitleBar_ShowsCostForAPIKey(t *testing.T) {
	t.Parallel()

	d := MockData{
		ProjectPath: "~/x/", ProjectName: "p", Model: "opus 4.7",
		Duration: "1h", Tokens: "47k", Cost: "$1.42",
		PlanTier: "",
	}
	s := NewStyles(TokyoNightPalette())
	got := stripANSI(renderTitleBar(s, TokyoNightPalette(), d, FrameState{}, 200, true, livesession.View{}, time.Now()))
	if !strings.Contains(got, "$1.42") {
		t.Errorf("API-key title bar should show $; got %q", got)
	}
}

// TestRenderTitleBar_LiveMode verifies that live mode computes duration, tokens,
// and cost from the live view.
func TestRenderTitleBar_LiveMode(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	start := now.Add(-90 * time.Minute) // 1h 30m ago

	ev := makeDummyAssistantEventWithTime(t, "ev1", start, usage.Usage{Input: 47_000})

	v := livesession.View{
		Events:         []event.Event{ev},
		TotalUsage:     usage.Usage{Input: 47_000},
		CurrentModel:   "claude-opus-4-7",
		AssistantTurns: 1,
		LastUpdate:     now,
	}

	// Render the title bar directly
	s := NewStyles(TokyoNightPalette())
	d := V3MockData()
	rendered := stripANSI(renderTitleBar(s, TokyoNightPalette(), d, InitFrameState(), 120, false, v, now))

	// Duration should be ~1h 30m (live computation from event timestamp)
	if !strings.Contains(rendered, "1h 30m") {
		t.Errorf("live mode title bar should contain '1h 30m'; got %q", rendered)
	}
	// Tokens should be computed from TotalUsage.
	if !strings.Contains(rendered, "47k") {
		t.Errorf("live mode title bar should contain '47k'; got %q", rendered)
	}
}

// TestFormatTitleDuration verifies the duration formatting helper.
func TestFormatTitleDuration(t *testing.T) {
	t.Parallel()

	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "30s"},
		{90 * time.Second, "1m 30s"},
		{60 * time.Second, "1m"},
		{90 * time.Minute, "1h 30m"},
		{60 * time.Minute, "1h"},
	}
	for _, tc := range tests {
		got := formatTitleDuration(tc.d)
		if got != tc.want {
			t.Errorf("formatTitleDuration(%v) = %q; want %q", tc.d, got, tc.want)
		}
	}
}

// TestFormatTitleTokens verifies the token count formatting helper.
func TestFormatTitleTokens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		n    int64
		want string
	}{
		{500, "500"},
		{1_500, "1.5k"},
		{47_200, "47k"},
		{100_000, "100k"},
	}
	for _, tc := range tests {
		got := formatTitleTokens(tc.n)
		if got != tc.want {
			t.Errorf("formatTitleTokens(%d) = %q; want %q", tc.n, got, tc.want)
		}
	}
}

// ─── V1 Issue 2: Diff empty state ────────────────────────────────────────────

// TestDiffEmptyState_NotAGitRepo verifies that when IsGitRepo is false the diff
// panel renders "not a git repository" rather than a blank panel.
func TestDiffEmptyState_NotAGitRepo(t *testing.T) {
	t.Parallel()

	m := NewModel()
	// Simulate a cwd that is not inside a git repo.
	m.data.DiffLines = nil
	m.data.DiffFile = ""
	m.data.IsGitRepo = false

	panel := stripANSI(renderDiff(m.styles, m.palette, m.data, 60, 15, false))
	if !strings.Contains(panel, "not a git repository") {
		t.Errorf("diff panel must show 'not a git repository' when IsGitRepo=false; got:\n%s", panel)
	}
}

// TestDiffEmptyState_CleanWorkingTree verifies that when IsGitRepo is true but
// there are no diff lines the diff panel renders "no changes".
func TestDiffEmptyState_CleanWorkingTree(t *testing.T) {
	t.Parallel()

	m := NewModel()
	// Inside a git repo but clean working tree — no hunks.
	m.data.DiffLines = nil
	m.data.DiffFile = ""
	m.data.IsGitRepo = true

	panel := stripANSI(renderDiff(m.styles, m.palette, m.data, 60, 15, false))
	if !strings.Contains(panel, "no changes") {
		t.Errorf("diff panel must show 'no changes' when IsGitRepo=true and no hunks; got:\n%s", panel)
	}
}

// TestDiffEmptyState_WithHunks verifies that the normal diff content is shown
// (not the empty-state message) when DiffLines is non-empty.
func TestDiffEmptyState_WithHunks(t *testing.T) {
	t.Parallel()

	m := NewModel()
	// V3MockData has non-empty DiffLines.
	m.data = V3MockData()

	panel := stripANSI(renderDiff(m.styles, m.palette, m.data, 80, 20, false))
	if strings.Contains(panel, "not a git repository") {
		t.Error("diff panel must not show 'not a git repository' when DiffLines is non-empty")
	}
	if strings.Contains(panel, "no changes") {
		t.Error("diff panel must not show 'no changes' when DiffLines is non-empty")
	}
	// Should contain actual diff content (hunk header).
	if !strings.Contains(panel, "@@") {
		t.Error("diff panel must show hunk headers when DiffLines is non-empty")
	}
}

// ─── V22 pricing dual-mode: cost row gated on plan tier ──────────────────────

// TestUsagePanel_CostHiddenForSubscriber verifies that Pro/Max subscribers do
// not see the per-token cost row — they pay a flat subscription, not per token.
// (V22 replaced the V1 "cost (api eq)" disclaimer with hiding the row entirely.)
func TestUsagePanel_CostHiddenForSubscriber(t *testing.T) {
	t.Parallel()

	m := NewModel()
	m.data.PlanTier = "Max 5x"
	m.data.Cost142 = "$1.42"
	panel := stripANSI(renderUsage(m.styles, m.palette, m.data, m.progTokens, m.progReset, 60, 30, false))
	if strings.Contains(panel, "$1.42") {
		t.Errorf("usage panel must NOT show $ cost for subscriber; rendered:\n%s", panel)
	}
}

// TestUsagePanel_CostShownForAPIKey verifies that API-key users (no plan tier)
// still see the per-token cost row — for them it is the only meaningful metric.
func TestUsagePanel_CostShownForAPIKey(t *testing.T) {
	t.Parallel()

	m := NewModel()
	m.data.PlanTier = ""
	m.data.Cost142 = "$1.42"
	panel := stripANSI(renderUsage(m.styles, m.palette, m.data, m.progTokens, m.progReset, 60, 30, false))
	if !strings.Contains(panel, "$1.42") {
		t.Errorf("usage panel must show $ cost for API-key user; rendered:\n%s", panel)
	}
}

// TestUsageCollapsed_CostHiddenForSubscriber verifies the collapsed one-liner
// shows the plan tier instead of $ for subscribers.
func TestUsageCollapsed_CostHiddenForSubscriber(t *testing.T) {
	t.Parallel()

	m := NewModel()
	m.data.PlanTier = "Max 5x"
	m.data.Cost142 = "$1.42"
	panel := stripANSI(renderUsageCollapsed(m.styles, m.data, CompactionOK, 80, false))
	if strings.Contains(panel, "$1.42") {
		t.Errorf("collapsed usage must NOT show $ for subscriber; rendered:\n%s", panel)
	}
	if !strings.Contains(panel, "Max 5x") {
		t.Errorf("collapsed usage should show plan tier for subscriber; rendered:\n%s", panel)
	}
}

// TestUsageCollapsed_CostShownForAPIKey verifies the collapsed one-liner shows
// $ when the user is on an API key.
func TestUsageCollapsed_CostShownForAPIKey(t *testing.T) {
	t.Parallel()

	m := NewModel()
	m.data.PlanTier = ""
	m.data.Cost142 = "$1.42"
	panel := stripANSI(renderUsageCollapsed(m.styles, m.data, CompactionOK, 80, false))
	if !strings.Contains(panel, "$1.42") {
		t.Errorf("collapsed usage must show $ for API-key user; rendered:\n%s", panel)
	}
}

// ─── helpers ──────────────────────────────────────────────────────────────────

// makeDummyAssistantEvent creates a minimal KindAssistant event for tests.
func makeDummyAssistantEvent(t *testing.T, id string) event.Event {
	t.Helper()
	ap := event.AssistantPayload{Summary: "Tool: Read /some/file"}
	return event.NewEvent(id, time.Now().UTC(), event.KindAssistant, "sess1", "", ap)
}

// makeDummyAssistantEventWithTime creates a KindAssistant event at a specific time
// with specified token usage.
func makeDummyAssistantEventWithTime(t *testing.T, id string, ts time.Time, u usage.Usage) event.Event {
	t.Helper()
	ap := event.AssistantPayload{
		Summary: "Tool: Read /some/file",
		Usage:   u,
	}
	return event.NewEvent(id, ts, event.KindAssistant, "sess1", "", ap)
}

// min returns the smaller of a and b.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
