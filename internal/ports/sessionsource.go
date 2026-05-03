// Package ports defines the port interfaces for the hexagonal architecture.
//
// Ports are the "driven" side of the hexagon — they describe what the
// application layer needs from the outside world (data sources, clocks, etc.)
// without naming any specific technology. Concrete implementations live in
// internal/adapters/*.
package ports

import (
	"context"
	"time"

	"github.com/clyde-tui/clyde/internal/domain/event"
	"github.com/clyde-tui/clyde/internal/domain/session"
)

// SessionSource is the port through which the application layer discovers and
// reads Sessions for a Project. Implementations MUST:
//
//   - Surface all Sessions for the given project CWD, ordered by latest
//     activity descending. Empty directory → empty slice, no error.
//   - Return Events for a Session in strictly ascending chronological order.
//   - Preserve Events with unknown kinds as opaque Events — never drop them.
//   - NOT modify any underlying storage (reads are observational only).
type SessionSource interface {
	// Sessions returns all Sessions belonging to the given Project working
	// directory, ordered by latest activity descending. May be empty.
	Sessions(ctx context.Context, projectCWD string) ([]session.Summary, error)

	// Events returns all Events for the given Session in strictly ascending
	// chronological order. May be empty.
	Events(ctx context.Context, id session.ID) ([]event.Event, error)
}

// GlobalSessionRef is a reference to a session in any project directory.
// It carries enough metadata to filter by time window without reading the full
// JSONL file, plus the absolute path for on-demand event loading.
type GlobalSessionRef struct {
	// ProjectEncodedDir is the encoded project directory name under ~/.claude/projects/,
	// e.g. "-Users-vladpb-work-Personal-clyde".
	ProjectEncodedDir string

	// SessionID is the session identifier (the JSONL filename without extension).
	SessionID session.ID

	// LastActivity is the file mtime — a cheap proxy for the session's last event.
	LastActivity time.Time

	// Path is the absolute path to the JSONL file.
	Path string
}

// GlobalSessionSource is an optional extension port that enumerates sessions
// across ALL project directories under ~/.claude/projects/.
//
// Implementations MUST:
//   - Walk every subdirectory of the projects root, list *.jsonl files.
//   - Return refs ordered by LastActivity descending (most recent first).
//   - Return empty slice (no error) when the projects root is missing.
//   - Cap results at a reasonable N to bound snapshot latency.
type GlobalSessionSource interface {
	// AllProjectSessions returns refs to all sessions across all projects,
	// ordered by LastActivity descending. At most maxResults refs are returned.
	// Pass maxResults=0 for no cap (not recommended in production).
	AllProjectSessions(ctx context.Context, maxResults int) ([]GlobalSessionRef, error)
}
