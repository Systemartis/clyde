package livesession

import (
	"testing"
	"time"

	"github.com/Systemartis/clyde/internal/domain/session"
)

// TestIsSessionLive_SlashNewGhost is the regression for the bug where
// claude `/new` left the original session showing as live forever.
// claude `/new` keeps the same OS process but switches the session
// being written, so processscan still reports the original argv-detected
// ID. The newer-sibling guard demotes the old ID once a fresher session
// shows up in the same cwd.
func TestIsSessionLive_SlashNewGhost(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	oldID := session.ID("aaaaaaaa-aaaa-4aaa-aaaa-aaaaaaaaaaaa")
	newID := session.ID("bbbbbbbb-bbbb-4bbb-bbbb-bbbbbbbbbbbb")

	// Old session: argv-detected (process still launched with this ID),
	// activity 2 minutes ago — well past liveActivityWindow.
	oldLastActivity := now.Add(-2 * time.Minute)
	// New session: fresh activity in the last second.
	newLastActivity := now.Add(-1 * time.Second)
	freshest := newLastActivity

	running := map[session.ID]bool{oldID: true}
	// /new ghost: the fresh JSONL's session ID has no running process —
	// the same PID kept its old argv after the user typed /new.
	freshestIsArgv := false

	if isSessionLive(oldID, running, oldLastActivity, freshest, freshestIsArgv, now) {
		t.Error("old argv-detected session must NOT be live when a newer sibling exists without its own process")
	}
	if !isSessionLive(newID, running, newLastActivity, freshest, freshestIsArgv, now) {
		t.Error("new session with fresh activity must be live")
	}
}

// TestIsSessionLive_ResumedIdle confirms that the argv boost still
// works for the canonical /resume case: claude was resumed and is
// idle, with no other session competing in the cwd. Without the boost
// the user would lose the tab once mtime fell out of the activity
// window.
func TestIsSessionLive_ResumedIdle(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	id := session.ID("cccccccc-cccc-4ccc-cccc-cccccccccccc")
	last := now.Add(-3 * time.Minute) // past liveActivityWindow
	freshest := last                  // only session in the cwd

	if !isSessionLive(id, map[session.ID]bool{id: true}, last, freshest, true, now) {
		t.Error("argv-detected sole-session-in-cwd must remain live (resume case)")
	}
}

// TestIsSessionLive_RecentActivityWinsAlone covers the trivial-fast-path:
// JSONL mtime in the activity window keeps the session live regardless
// of process or sibling state.
func TestIsSessionLive_RecentActivityWinsAlone(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	id := session.ID("dddddddd-dddd-4ddd-dddd-dddddddddddd")
	last := now.Add(-5 * time.Second)
	// Pretend a much-newer sibling exists; recent activity should still
	// win the live test because the activity-window branch fires first.
	freshest := now.Add(-1 * time.Second)

	if !isSessionLive(id, nil, last, freshest, false, now) {
		t.Error("recent activity must keep the session live regardless of sibling state")
	}
}

// TestIsSessionLive_MultiInstanceSiblings is the regression for the bug
// where two separate `claude` processes running in the same cwd produced
// only one live tab. Each process has its own argv-detected session ID;
// the freshest-in-cwd guard previously demoted the idle sibling even
// though it had a distinct PID still alive — the guard was only meant
// to catch /new ghosts, where one process orphans its old session ID
// without a matching process for the freshest JSONL.
//
// The differentiator: when the freshest session itself appears in the
// running set, every argv-detected sibling is a separate process and
// should keep its tab. The /new case is the inverse — freshest is
// NOT in the running set.
func TestIsSessionLive_MultiInstanceSiblings(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	idleID := session.ID("11111111-1111-4111-1111-111111111111")
	activeID := session.ID("22222222-2222-4222-2222-222222222222")

	idleLast := now.Add(-5 * time.Minute)   // idle but process alive
	activeLast := now.Add(-1 * time.Second) // fresh activity
	freshest := activeLast

	running := map[session.ID]bool{idleID: true, activeID: true}
	// activeID is in the running set at the freshest timestamp.
	freshestIsArgv := true

	if !isSessionLive(idleID, running, idleLast, freshest, freshestIsArgv, now) {
		t.Error("idle sibling with its own running process must stay live")
	}
	if !isSessionLive(activeID, running, activeLast, freshest, freshestIsArgv, now) {
		t.Error("active session must be live")
	}
}

// TestIsSessionLive_StaleNoProcess confirms that a long-idle session
// with no process probe + no recent activity reads as not live.
func TestIsSessionLive_StaleNoProcess(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	id := session.ID("eeeeeeee-eeee-4eee-eeee-eeeeeeeeeeee")
	last := now.Add(-10 * time.Minute)

	if isSessionLive(id, map[session.ID]bool{}, last, last, false, now) {
		t.Error("stale session with no process probe must NOT be live")
	}
}
