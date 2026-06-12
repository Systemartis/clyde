package tui

import (
	"context"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/Systemartis/clyde/internal/adapters/hookserver"
	"github.com/Systemartis/clyde/internal/application/livesession"
)

// snapshotCmd returns a tea.Cmd that loads the next live snapshot.
//
// When the user has selected a specific session tab (sessionTabIndex >= 0)
// we call SnapshotForSession with that session ID. When the Σ all tab is
// active (sessionTabIndex == -1) we route to SnapshotAggregated so the
// activity / bash / diff panels show a cwd-wide merged stream, distinct
// from any individual session tab.
func (m Model) snapshotCmd() tea.Cmd {
	ls := m.liveSession
	p := m.liveProject
	ds := m.diffSrc
	now := time.Now().UTC()
	aggregate := m.sessionTabIndex == -1
	focusedID := resolveSessionFocus(m.liveView, m.sessionTabIndex, now)
	return func() tea.Msg {
		var (
			view livesession.View
			err  error
		)
		if aggregate {
			view, err = ls.SnapshotAggregated(context.Background(), p)
		} else {
			view, err = ls.SnapshotForSession(context.Background(), p, focusedID)
		}
		// Fetch git diff hunks for the active file in the same goroutine
		// so the UI never blocks on the event loop.
		if err == nil && ds != nil {
			hunks, _ := ds.Diff(view.Project.CWD(), view.DiffFile)
			view.DiffHunks = hunks
		}
		isGit := isGitRepo(view.Project.CWD())
		return liveSessionMsg{view: view, err: err, isGitRepo: isGit}
	}
}

// isGitRepo reports whether cwd (or any ancestor up to the filesystem root)
// contains a .git directory. This is a fast filesystem probe used to distinguish
// "clean working tree" from "not a git repository" in the diff panel.
func isGitRepo(cwd string) bool {
	if cwd == "" {
		return false
	}
	dir := cwd
	for {
		info, err := os.Stat(filepath.Join(dir, ".git"))
		if err == nil && info.IsDir() {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root — no .git found.
			return false
		}
		dir = parent
	}
}

// refreshCmd returns a tea.Cmd that waits liveRefreshInterval then fires a new snapshot.
func (m Model) refreshCmd() tea.Cmd {
	return tea.Tick(liveRefreshInterval, func(_ time.Time) tea.Msg {
		return refreshLiveMsg{}
	})
}

// planUsageCmd fires a single Fetch against the plan-usage source. Returns
// nil when no source is wired (demo mode) so the message pump stays clean.
func (m Model) planUsageCmd() tea.Cmd {
	src := m.planUsageSrc
	if src == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		u, err := src.Fetch(ctx)
		return planUsageMsg{usage: u, err: err}
	}
}

// planUsageRefreshCmd schedules the next plan-usage tick.
func (m Model) planUsageRefreshCmd() tea.Cmd {
	if m.planUsageSrc == nil {
		return nil
	}
	return tea.Tick(planUsageRefreshInterval, func(_ time.Time) tea.Msg {
		return refreshPlanUsageMsg{}
	})
}

// hookWatchCmd returns a tea.Cmd that blocks until the next HookEvent arrives
// on the server's Events channel, then wraps it in a hookEventMsg. Safe to
// call repeatedly — each invocation waits for exactly one event.
func hookWatchCmd(events <-chan hookserver.HookEvent) tea.Cmd {
	return func() tea.Msg {
		evt, ok := <-events
		if !ok {
			// Channel closed (server shutting down) — no message.
			return nil
		}
		return hookEventMsg{evt: evt}
	}
}
