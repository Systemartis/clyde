// Package session defines the Session domain types.
//
// A Session is an ordered sequence of Events belonging to a single Claude Code
// conversation, scoped to exactly one Project. Sessions are identified by an
// opaque string ID that corresponds to the JSONL filename (without extension)
// in the ~/.claude/projects/<encoded-cwd>/ directory.
//
// V1 exposes only the types needed by WatchSession and the JSONL adapter:
// an ID type and a Summary (lightweight descriptor used for session discovery
// and ordering). The full Session entity (with its Event slice) is not
// materialized here — the application layer works with Summary + []event.Event
// returned directly by the SessionSource port.
package session

import "time"

// ID is the session identifier — a string newtype wrapping the opaque session
// UUID assigned by Claude Code. Using a distinct type prevents accidentally
// mixing session IDs with other string identifiers (e.g. event IDs).
type ID string

// Summary is a lightweight descriptor of a Session returned by SessionSource.
// It carries only the fields needed to select the most-recently-active session;
// the full Event list is fetched separately via SessionSource.Events.
type Summary struct {
	// ID is the unique identifier for this session.
	ID ID

	// LastActivity is the UTC timestamp of the session's most recent activity,
	// used to select the focused session and sort the session list.
	LastActivity time.Time
}
