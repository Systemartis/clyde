package tui

import (
	"os"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/clyde-tui/clyde/internal/domain/event"
)

// ─── I.1: Bunny event triggers ───────────────────────────────────────────────

// makeToolResultEvent creates a KindUser event with IsToolResultOnly=true.
// When hasError is true, ToolResultError is also set.
func makeToolResultEvent(id string, ts time.Time, hasError bool) event.Event {
	up := event.UserPayload{
		IsToolResultOnly: true,
		ToolResultError:  hasError,
	}
	return event.NewEvent(id, ts, event.KindUser, "sess1", "", up)
}

// makeAssistantEvent creates a KindAssistant event with the given summary.
func makeAssistantEvent(id string, ts time.Time, summary string) event.Event {
	ap := event.AssistantPayload{Summary: summary}
	return event.NewEvent(id, ts, event.KindAssistant, "sess1", "", ap)
}

// TestNewToolResultEvents_OnlyNewAfterPrevID verifies newToolResultEvents returns
// only events that come after prevID.
func TestNewToolResultEvents_OnlyNewAfterPrevID(t *testing.T) {
	now := time.Now().UTC()
	ev1 := makeToolResultEvent("id1", now.Add(-5*time.Second), false)
	ev2 := makeToolResultEvent("id2", now.Add(-4*time.Second), false)
	ev3 := makeToolResultEvent("id3", now.Add(-3*time.Second), true)

	evts := []event.Event{ev1, ev2, ev3}

	// prevID = "id1": should return id2 and id3
	got := newToolResultEvents(evts, "id1")
	if len(got) != 2 {
		t.Fatalf("expected 2 events after id1, got %d", len(got))
	}
	if got[0].ID != "id2" || got[1].ID != "id3" {
		t.Errorf("unexpected IDs: %v %v", got[0].ID, got[1].ID)
	}
}

// TestNewToolResultEvents_EmptyWhenPrevIsLast verifies no events are returned
// when prevID is the last event's ID.
func TestNewToolResultEvents_EmptyWhenPrevIsLast(t *testing.T) {
	now := time.Now().UTC()
	ev1 := makeToolResultEvent("id1", now, false)
	evts := []event.Event{ev1}

	got := newToolResultEvents(evts, "id1")
	if len(got) != 0 {
		t.Errorf("expected 0 events, got %d", len(got))
	}
}

// TestNewToolResultEvents_AllWhenNoPrevID verifies all tool_result events are
// returned when prevID is empty (first snapshot).
func TestNewToolResultEvents_AllWhenNoPrevID(t *testing.T) {
	now := time.Now().UTC()
	evts := []event.Event{
		makeToolResultEvent("id1", now, false),
		makeToolResultEvent("id2", now.Add(time.Second), true),
	}
	got := newToolResultEvents(evts, "")
	if len(got) != 2 {
		t.Fatalf("expected 2 events with no prevID, got %d", len(got))
	}
}

// TestIsLongThinking_ReturnsTrueWhenThinkingExceedsThreshold verifies that a
// thinking block running longer than threshold triggers true.
func TestIsLongThinking_ReturnsTrueWhenThinkingExceedsThreshold(t *testing.T) {
	ts := time.Now().UTC().Add(-15 * time.Second) // 15 seconds ago
	evts := []event.Event{makeAssistantEvent("id1", ts, "(thinking)")}

	got := isLongThinking(evts, time.Now().UTC(), 10*time.Second)
	if !got {
		t.Error("expected isLongThinking=true when thinking > 10s threshold")
	}
}

// TestIsLongThinking_ReturnsFalseForShortThinking verifies short thinking
// blocks do not trigger the flag.
func TestIsLongThinking_ReturnsFalseForShortThinking(t *testing.T) {
	ts := time.Now().UTC().Add(-3 * time.Second) // only 3 seconds ago
	evts := []event.Event{makeAssistantEvent("id1", ts, "(thinking)")}

	got := isLongThinking(evts, time.Now().UTC(), 10*time.Second)
	if got {
		t.Error("expected isLongThinking=false when thinking < 10s threshold")
	}
}

// TestIsLongThinking_ReturnsFalseForNonThinking verifies non-thinking events
// do not trigger the flag.
func TestIsLongThinking_ReturnsFalseForNonThinking(t *testing.T) {
	ts := time.Now().UTC().Add(-20 * time.Second)
	evts := []event.Event{makeAssistantEvent("id1", ts, "Tool: Read /foo")}

	got := isLongThinking(evts, time.Now().UTC(), 10*time.Second)
	if got {
		t.Error("expected isLongThinking=false for tool_use event")
	}
}

// TestMascotTrigger_HappyOnSuccessfulToolResult verifies that a new successful
// tool_result event queues eventHappy.
func TestMascotTrigger_HappyOnSuccessfulToolResult(t *testing.T) {
	m := NewModel()
	m.demoMode = false // enable live trigger path

	now := time.Now().UTC()
	// One earlier event that was already seen
	prev := makeAssistantEvent("prev", now.Add(-2*time.Second), "Tool: Read /foo")
	// New successful tool_result
	newResult := makeToolResultEvent("new1", now.Add(-time.Second), false)

	evts := []event.Event{prev, newResult}
	m.prevLatestEventID = "prev" // simulate having seen prev already

	fsm := m.mascotTriggerForNewEvent(evts)

	// FSM should have eventHappy pending
	if fsm.pendingEvent == nil {
		t.Fatal("expected pendingEvent to be set after successful tool_result")
	}
	if *fsm.pendingEvent != eventHappy {
		t.Errorf("pendingEvent = %v, want eventHappy (%v)", *fsm.pendingEvent, eventHappy)
	}
}

// TestMascotTrigger_SurprisedOnErrorToolResult verifies that a new tool_result
// with ToolResultError=true queues eventSurprised.
func TestMascotTrigger_SurprisedOnErrorToolResult(t *testing.T) {
	m := NewModel()
	m.demoMode = false

	now := time.Now().UTC()
	prev := makeAssistantEvent("prev", now.Add(-2*time.Second), "Tool: Bash echo")
	errResult := makeToolResultEvent("err1", now.Add(-time.Second), true)

	evts := []event.Event{prev, errResult}
	m.prevLatestEventID = "prev"

	fsm := m.mascotTriggerForNewEvent(evts)

	if fsm.pendingEvent == nil {
		t.Fatal("expected pendingEvent to be set after error tool_result")
	}
	if *fsm.pendingEvent != eventSurprised {
		t.Errorf("pendingEvent = %v, want eventSurprised (%v)", *fsm.pendingEvent, eventSurprised)
	}
}

// TestMascotTrigger_SleepOnIdle verifies that old events trigger eventSleep.
func TestMascotTrigger_SleepOnIdle(t *testing.T) {
	m := NewModel()
	m.demoMode = false

	// Last event was 60 seconds ago (well past nowIdleThreshold of 30s)
	oldTime := time.Now().UTC().Add(-60 * time.Second)
	ev := makeAssistantEvent("id1", oldTime, "Tool: Read /foo")

	fsm := m.mascotTriggerForNewEvent([]event.Event{ev})

	if fsm.pendingEvent == nil {
		t.Fatal("expected pendingEvent to be set for idle")
	}
	if *fsm.pendingEvent != eventSleep {
		t.Errorf("pendingEvent = %v, want eventSleep (%v)", *fsm.pendingEvent, eventSleep)
	}
}

// ─── I.2: Compaction state ────────────────────────────────────────────────────

// TestDeriveCompactionState verifies all threshold boundaries.
func TestDeriveCompactionState(t *testing.T) {
	cases := []struct {
		pct  int
		want CompactionState
	}{
		{0, CompactionOK},
		{50, CompactionOK},
		{74, CompactionOK},
		{75, CompactionWarn},
		{80, CompactionWarn},
		{89, CompactionWarn},
		{90, CompactionDanger},
		{95, CompactionDanger},
		{100, CompactionDanger},
	}
	for _, tc := range cases {
		got := deriveCompactionState(tc.pct)
		if got != tc.want {
			t.Errorf("deriveCompactionState(%d) = %d, want %d", tc.pct, got, tc.want)
		}
	}
}

// TestRenderUsageCollapsed_CompactionBadge verifies that the collapsed usage
// summary contains the correct badge text for each compaction state.
func TestRenderUsageCollapsed_CompactionBadge(t *testing.T) {
	s := NewStyles(TokyoNightPalette())
	d := MockData{TokenPct: 92, Cost142: "$2.00"}

	cases := []struct {
		state CompactionState
		want  string
	}{
		{CompactionOK, ""},
		{CompactionWarn, "⚠ compaction"},
		{CompactionDanger, "⛔ compact!"},
	}
	for _, tc := range cases {
		rendered := renderUsageCollapsed(s, d, tc.state, 80, false)
		plain := stripANSI(rendered)
		if tc.want == "" {
			// Should NOT contain warning symbols
			for _, sym := range []string{"⚠", "⛔"} {
				if containsStr(plain, sym) {
					t.Errorf("CompactionOK: rendered contains %q unexpectedly", sym)
				}
			}
		} else if !containsStr(plain, tc.want) {
			t.Errorf("state %d: want %q in rendered, got:\n%s", tc.state, tc.want, plain)
		}
	}
}

// containsStr is a simple string-contains helper to avoid importing strings in test.
func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}()
}

// ─── I.4: Settings overlay ────────────────────────────────────────────────────

// TestSettingsOverlayFromConfig verifies toggles are built from config.
// v22+: the diff panel is opt-in (off by default) since hunks render inline
// in the activity panel. Every other panel is enabled.
func TestSettingsOverlayFromConfig(t *testing.T) {
	cfg := DefaultConfig()
	toggles := settingsOverlayFromConfig(cfg, ScopeGlobal, "")

	if len(toggles) != int(panelCount) {
		t.Errorf("expected %d toggles, got %d", panelCount, len(toggles))
	}
	for _, tg := range toggles {
		if tg.PanelID == PanelDiff || tg.PanelID == PanelBash || tg.PanelID == PanelCache {
			if tg.Enabled {
				t.Errorf("toggle %q should be off by default in v22+ (opt-in)", tg.Label)
			}
			continue
		}
		if !tg.Enabled {
			t.Errorf("toggle %q should be enabled by default", tg.Label)
		}
	}
}

// TestSaveSettingsToConfig_WritesFile verifies that saveSettingsToConfig creates
// a TOML file and can be read back via LoadConfig.
func TestSaveSettingsToConfig_WritesFile(t *testing.T) {
	// Override home via a temp dir so we don't pollute the real config.
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	cfg := DefaultConfig()
	toggles := settingsOverlayFromConfig(cfg, ScopeGlobal, "")

	// Disable the diff panel.
	for i := range toggles {
		if toggles[i].PanelID == PanelDiff {
			toggles[i].Enabled = false
		}
	}

	saveSettingsToConfig(cfg, toggles, ScopeGlobal, "")

	// Verify file was created.
	path := tmpDir + "/.config/clyde/config.toml"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("config file not created at %s", path)
	}

	// Read back via LoadConfig and verify diff panel is disabled.
	loaded := LoadConfig()
	if loaded.Panels.Diff.Enabled {
		t.Error("diff panel should be disabled after saving toggles")
	}
	if !loaded.Panels.Now.Enabled {
		t.Error("now panel should still be enabled")
	}
}

// TestSettingsKey_ToggleAndClose verifies that the settings overlay toggles and
// closes on Esc.
func TestSettingsKey_ToggleAndClose(t *testing.T) {
	m := NewModel()

	// Open settings.
	m.settingsOpen = true
	m.settingsCursor = 0
	m.settingsToggles = settingsOverlayFromConfig(m.cfg, ScopeGlobal, "")

	initialEnabled := m.settingsToggles[0].Enabled

	// Press Enter to toggle the first item.
	m = m.handleSettingsKey(keyPressEnter())
	if m.settingsToggles[0].Enabled == initialEnabled {
		t.Errorf("toggle[0].Enabled should have flipped after Enter")
	}

	// Navigate down.
	m = m.handleSettingsKey(keyPressDown())
	if m.settingsCursor != 1 {
		t.Errorf("settingsCursor should be 1 after ↓, got %d", m.settingsCursor)
	}

	// Navigate up.
	m = m.handleSettingsKey(keyPressUp())
	if m.settingsCursor != 0 {
		t.Errorf("settingsCursor should be 0 after ↑, got %d", m.settingsCursor)
	}
}

// TestSettingsKey_Clamp verifies the cursor clamps at both ends of the
// extended cursor space:
//
//	settingsNotificationCursor   (-3) → Notifications · style
//	settingsRememberLayoutCursor (-2) → Behavior · remember layout
//	settingsLayoutCursor         (-1) → Layout · mode
//	0..N-1                            → Panels · enabled
//	N..2N-1                           → Startup · starts collapsed
func TestSettingsKey_Clamp(t *testing.T) {
	m := NewModel()
	m.settingsOpen = true
	m.settingsCursor = settingsCursorMin()
	m.settingsToggles = settingsOverlayFromConfig(m.cfg, ScopeGlobal, "")

	// Press up at the top — cursor should stay at the topmost row.
	m = m.handleSettingsKey(keyPressUp())
	if m.settingsCursor != settingsCursorMin() {
		t.Errorf("cursor should clamp at topmost row (%d), got %d", settingsCursorMin(), m.settingsCursor)
	}

	// Move to the last startup toggle.
	maxCursor := startupCursorMax(m.settingsToggles)
	m.settingsCursor = maxCursor
	m = m.handleSettingsKey(keyPressDown())
	if m.settingsCursor != maxCursor {
		t.Errorf("cursor should clamp at max (%d), got %d", maxCursor, m.settingsCursor)
	}
}

// ─── test helpers ─────────────────────────────────────────────────────────────

func keyPressEnter() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyEnter}
}

func keyPressDown() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyDown}
}

func keyPressUp() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyUp}
}
