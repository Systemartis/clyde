package tui

import (
	"github.com/clyde-tui/clyde/internal/application/livesession"
)

// DiffSource is the minimal interface for fetching git diff hunks.
// The concrete implementation is git.Source from the adapters/git package.
// Defined here (TUI layer) so model.go can hold a DiffSource without
// importing the adapter package directly.
type DiffSource interface {
	// Diff returns parsed unified-diff hunks for the given file in cwd.
	// If file is empty, all changed files are included.
	// Returns (nil, nil) when cwd is not a git repo or git is unavailable.
	Diff(cwd, file string) ([]livesession.DiffHunk, error)
}

// liveSessionMsg carries a completed LiveSessionView snapshot to the Bubble Tea
// Update loop. It is produced by the snapshotCmd tea.Cmd and consumed in Update.
type liveSessionMsg struct {
	view      livesession.View
	err       error
	isGitRepo bool // true when cwd is inside a git repository
}
