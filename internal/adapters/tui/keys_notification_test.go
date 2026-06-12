package tui

import "testing"

// TestToggleDemoNotification_FiresOnce verifies the Ctrl+N preview
// trigger activates the synthetic hook on its first call, and refuses to
// re-fire on subsequent calls. Once-per-session is the contract — the
// user has to restart clyde to re-test, which keeps the binding from
// being spammed and the chrome from flickering on a held chord.
func TestToggleDemoNotification_FiresOnce(t *testing.T) {
	t.Parallel()

	m := NewModel()
	if m.hookNotif.Active {
		t.Fatal("fresh model should have no active hook")
	}
	if m.demoNotificationFired {
		t.Fatal("fresh model should have demoNotificationFired=false")
	}

	m = m.toggleDemoNotification()
	if !m.hookNotif.Active {
		t.Fatal("first call should activate a synthetic hook")
	}
	if !m.demoNotificationFired {
		t.Error("first call should latch the one-shot flag")
	}
	if m.notifAck {
		t.Error("first call must clear notifAck so the overlay can paint")
	}

	// Simulate user dismissing via Esc.
	m.notifAck = true
	m.hookNotif = HookNotification{}

	// Second call must not re-fire even after the first event was
	// cleared from state.
	m = m.toggleDemoNotification()
	if m.hookNotif.Active {
		t.Error("second call should NOT re-fire the synthetic hook")
	}
}

// TestHandleCtrl_NIsOneShot verifies the Ctrl+N dispatcher honors the
// one-shot semantics — covers the wiring, not just the helper.
func TestHandleCtrl_NIsOneShot(t *testing.T) {
	t.Parallel()

	m := NewModel()
	m = m.handleCtrl('n')
	if !m.hookNotif.Active {
		t.Error("Ctrl+N (lowercase 'n') did not activate a synthetic hook")
	}

	// Reset visible state, simulate dismiss + retry.
	m.notifAck = true
	m.hookNotif = HookNotification{}

	m = m.handleCtrl('N')
	if m.hookNotif.Active {
		t.Error("Ctrl+Shift+N after one-shot fire should be a no-op")
	}
}
