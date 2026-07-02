package tui

import (
	"os"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/Systemartis/clyde/internal/adapters/hookserver"
)

// ── Finding #1: closing settings must not bake a transient layout override ──

// TestSettingsEsc_DoesNotBakeTransientLayoutOverride verifies that opening and
// closing the settings overlay (Esc) WITHOUT touching the layout chip does not
// persist a runtime-only layout mode (e.g. a --layout CLI override or a
// width-fallback) into config.toml.
func TestSettingsEsc_DoesNotBakeTransientLayoutOverride(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	m := NewModel()
	m.baseCfg = DefaultConfig() // persisted default_mode == stack
	m.cfg = m.baseCfg
	// Simulate a --layout tabs override: the runtime mode diverges from the
	// persisted config default.
	m.layoutMode = LayoutTabs
	m = m.openSettings()

	// Close without ever moving the cursor onto the layout chip.
	m = m.handleSettingsKey(tea.KeyPressMsg{Code: tea.KeyEscape})

	if m.baseCfg.Layout.DefaultMode != LayoutStack {
		t.Errorf("in-memory default_mode = %q, want stack (transient override must not persist)", m.baseCfg.Layout.DefaultMode)
	}
	loaded := LoadConfig()
	if loaded.Layout.DefaultMode != LayoutStack {
		t.Errorf("on-disk default_mode = %q, want stack", loaded.Layout.DefaultMode)
	}
}

// TestSettingsEsc_PersistsUserLayoutChange verifies that a layout change the
// user actually makes via the overlay (Enter on the layout chip) IS persisted.
func TestSettingsEsc_PersistsUserLayoutChange(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	m := NewModel()
	m.baseCfg = DefaultConfig()
	m.cfg = m.baseCfg
	m.layoutMode = LayoutStack
	m = m.openSettings()

	// Move onto the layout chip and cycle it (stack -> tabs).
	m.settingsCursor = settingsLayoutCursor
	m = m.handleSettingsKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.layoutMode != LayoutTabs {
		t.Fatalf("layoutMode = %q after cycling, want tabs", m.layoutMode)
	}

	m = m.handleSettingsKey(tea.KeyPressMsg{Code: tea.KeyEscape})

	if m.baseCfg.Layout.DefaultMode != LayoutTabs {
		t.Errorf("in-memory default_mode = %q, want tabs (user change must persist)", m.baseCfg.Layout.DefaultMode)
	}
	loaded := LoadConfig()
	if loaded.Layout.DefaultMode != LayoutTabs {
		t.Errorf("on-disk default_mode = %q, want tabs", loaded.Layout.DefaultMode)
	}
}

// ── Finding #2: invalid layout default_mode must sanitize to the default ──

func TestLoadConfig_SanitizesInvalidLayoutMode(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	dir := tmp + "/.config/clyde"
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/config.toml", []byte("[layout]\ndefault_mode = \"bogus\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := LoadConfig()
	if cfg.Layout.DefaultMode != LayoutStack {
		t.Errorf("invalid default_mode = %q, want sanitized to stack", cfg.Layout.DefaultMode)
	}
	if !cfg.Layout.DefaultMode.IsValid() {
		t.Errorf("sanitized default_mode should be valid, got %q", cfg.Layout.DefaultMode)
	}
}

// ── Finding #4: legacy [panels.tasks] must map onto the calls slot ──

func TestLoadConfig_LegacyTasksMapsToCalls(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	dir := tmp + "/.config/clyde"
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	// Legacy config: only [panels.tasks], no [panels.calls].
	body := "[panels.tasks]\nenabled = false\ndefault_collapsed = false\nposition = 9\n"
	if err := os.WriteFile(dir+"/config.toml", []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := LoadConfig()
	if cfg.Panels.Calls.Enabled {
		t.Error("legacy [panels.tasks].enabled=false should map onto the calls slot")
	}
	if cfg.Panels.Calls.Position != 9 {
		t.Errorf("legacy tasks position should map to calls, got %d", cfg.Panels.Calls.Position)
	}
}

func TestLoadConfig_ExplicitCallsWinsOverLegacyTasks(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	dir := tmp + "/.config/clyde"
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	// Both present: explicit [panels.calls] must win, legacy tasks ignored.
	body := "[panels.calls]\nenabled = false\n[panels.tasks]\nenabled = true\n"
	if err := os.WriteFile(dir+"/config.toml", []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := LoadConfig()
	if cfg.Panels.Calls.Enabled {
		t.Error("explicit [panels.calls].enabled=false must win over legacy [panels.tasks]")
	}
}

// ── Finding #3: writeConfigFile read-merge-write preserves concurrent edits ──

// TestWriteConfigFile_MergesConcurrentFieldChange simulates two clyde instances
// that both loaded the same config, each changing a DIFFERENT field. The
// second writer must not clobber the first writer's untouched-by-it field.
func TestWriteConfigFile_MergesConcurrentFieldChange(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	// Seed disk with defaults.
	writeConfigFile(DefaultConfig())

	// Instance B (concurrent) changes the theme on disk.
	b := DefaultConfig()
	b.Theme = ThemeGruvbox
	writeConfigFile(b)

	// Instance A predates B's change: its in-memory theme is still the default.
	// A changes only the notification style, then writes.
	a := DefaultConfig()
	a.NotificationStyle = NotificationBanner
	writeConfigFile(a)

	loaded := LoadConfig()
	if loaded.NotificationStyle != NotificationBanner {
		t.Errorf("A's notification change lost: got %q, want banner", loaded.NotificationStyle)
	}
	if loaded.Theme != ThemeGruvbox {
		t.Errorf("B's concurrent theme change clobbered by A: got %q, want gruvbox", loaded.Theme)
	}
}

// ── Finding #6: all three hook responses dismiss the notification overlay ──

// TestSyntheticNotification_YDismisses covers the Ctrl+N demo-notification
// preview (no ResponseCh). Pressing 'y' must dismiss the overlay, matching
// the 'n'/'esc' behavior.
func TestSyntheticNotification_YDismisses(t *testing.T) {
	m := NewModel()
	m.cfg.NotificationStyle = NotificationFullscreen

	m = m.toggleDemoNotification()
	if !m.hookNotif.Active {
		t.Fatal("synthetic notification should be active after toggleDemoNotification")
	}
	if m.hookPendingCh != nil {
		t.Fatal("synthetic notification must have a nil ResponseCh")
	}

	handled, m2 := m.handleHookPendingKey(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if !handled {
		t.Fatal("'y' should be handled while a notification is pending")
	}
	m = m2

	d := resolveNotification(m.cfg.NotificationStyle, m.notifAck, m.hookNotif, m.compaction, m.quotaNotif)
	if d.Active {
		t.Error("synthetic notification should be dismissed after pressing 'y'")
	}
}

// TestLiveNotification_YRepliesOnceAndClears re-asserts the live path stays
// correct: 'y' sends exactly one Allow=true and clears hookNotif.
func TestLiveNotification_YRepliesOnceAndClears(t *testing.T) {
	m := NewModel()
	respCh := make(chan hookserver.HookResponse, 1)
	m.hookNotif = HookNotification{Active: true, Tool: "Edit", KeyArg: "main.go"}
	m.hookPendingCh = respCh

	handled, m2 := m.handleHookPendingKey(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if !handled {
		t.Fatal("'y' should be handled")
	}
	m = m2

	if m.hookNotif.Active {
		t.Error("hookNotif should be cleared after live 'y'")
	}
	if m.hookPendingCh != nil {
		t.Error("hookPendingCh should be nil after live 'y'")
	}
	select {
	case resp := <-respCh:
		if !resp.Allow {
			t.Error("live 'y' should send Allow=true")
		}
	default:
		t.Fatal("live 'y' should have sent exactly one response")
	}
	// No second response should be queued.
	select {
	case <-respCh:
		t.Error("live 'y' must send exactly ONE response, not two")
	default:
	}
}

// ── Finding #7: a superseding hook event must auto-deny the prior request ──

func TestHookEvent_SupersededOldRequestAutoDenied(t *testing.T) {
	m := NewModel()

	ch1 := make(chan hookserver.HookResponse, 1)
	next, _ := m.Update(hookEventMsg{evt: hookserver.HookEvent{
		Type:       "PreToolUse",
		Tool:       "Bash",
		Args:       map[string]any{"command": "first"},
		ResponseCh: ch1,
	}})
	m = next.(Model)
	if m.hookPendingCh != ch1 {
		t.Fatal("first event should be the pending request")
	}

	ch2 := make(chan hookserver.HookResponse, 1)
	next, _ = m.Update(hookEventMsg{evt: hookserver.HookEvent{
		Type:       "PreToolUse",
		Tool:       "Edit",
		Args:       map[string]any{"file_path": "x.go"},
		ResponseCh: ch2,
	}})
	m = next.(Model)
	if m.hookPendingCh != ch2 {
		t.Fatal("second event should now be the pending request")
	}

	select {
	case resp := <-ch1:
		if resp.Allow {
			t.Error("superseded first request must be denied (Allow=false)")
		}
		if resp.Reason != "superseded by newer request" {
			t.Errorf("superseded reason = %q, want %q", resp.Reason, "superseded by newer request")
		}
	default:
		t.Error("superseded first request was left unanswered — the claude CLI would hang until write-timeout")
	}
}

// TestHookEvent_SingleEventLeavesRequestPending covers the normal single-event
// flow: the request stays pending (unanswered) until the user responds.
func TestHookEvent_SingleEventLeavesRequestPending(t *testing.T) {
	m := NewModel()

	ch := make(chan hookserver.HookResponse, 1)
	next, _ := m.Update(hookEventMsg{evt: hookserver.HookEvent{
		Type:       "PreToolUse",
		Tool:       "Bash",
		Args:       map[string]any{"command": "go build"},
		ResponseCh: ch,
	}})
	m = next.(Model)

	if m.hookPendingCh != ch {
		t.Fatal("single event should be the pending request")
	}
	select {
	case <-ch:
		t.Error("a single hook event must NOT auto-answer its own channel")
	default:
	}
}
