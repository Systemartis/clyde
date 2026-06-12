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

	if isSessionLive(oldID, running, oldLastActivity, freshest, now) {
		t.Error("old argv-detected session must NOT be live when a newer sibling exists")
	}
	if !isSessionLive(newID, running, newLastActivity, freshest, now) {
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

	if !isSessionLive(id, map[session.ID]bool{id: true}, last, freshest, now) {
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

	if !isSessionLive(id, nil, last, freshest, now) {
		t.Error("recent activity must keep the session live regardless of sibling state")
	}
}

// TestIsSessionLive_StaleNoProcess confirms that a long-idle session
// with no process probe + no recent activity reads as not live.
func TestIsSessionLive_StaleNoProcess(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	id := session.ID("eeeeeeee-eeee-4eee-eeee-eeeeeeeeeeee")
	last := now.Add(-10 * time.Minute)

	if isSessionLive(id, map[session.ID]bool{}, last, last, now) {
		t.Error("stale session with no process probe must NOT be live")
	}
}
