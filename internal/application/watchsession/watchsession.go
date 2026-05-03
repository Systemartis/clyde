// Package watchsession implements the WatchSession use case.
//
// WatchSession composes a SessionSource and a Clock to derive a view of the
// most recent activity in the focused Session for the current Project. It:
//
//  1. Asks SessionSource.Sessions to discover all Sessions for the given cwd.
//  2. Selects the most-recently-active Session (highest LastActivity).
//  3. Calls SessionSource.Events to retrieve that Session's Events.
//  4. Sorts Events ascending by Timestamp (chronological order).
//  5. Filters Events with isVisible (ADR-007): removes opaque, meta, and tool-result-only events.
//  6. Takes the last min(N, len) visible Events (default N=5).
//  7. Records Clock.Now() as the snapshot time.
//  8. Returns a SessionView with the focused session ID, events, and snapshot time.
//
// No I/O is performed by the use case itself — all data access is delegated
// to the SessionSource port.
package watchsession

import (
	"context"
	"sort"
	"time"

	"github.com/clyde-tui/clyde/internal/domain/event"
	"github.com/clyde-tui/clyde/internal/domain/session"
	"github.com/clyde-tui/clyde/internal/ports"
)

// defaultN is the maximum number of Events returned in SessionView.Events.
// Sessions with fewer than defaultN Events return all of them.
const defaultN = 5

// SessionView is the output of WatchSession.Run. It is a plain data value
// safe to pass across layer boundaries — no Tea types, no I/O handles.
type SessionView struct {
	// FocusedSession is the ID of the Session whose Events are shown.
	// Zero value (empty string) means no Sessions exist for the Project.
	FocusedSession session.ID

	// Events holds the last N Events of the focused Session in strictly
	// ascending chronological order (oldest first). May be empty.
	Events []event.Event

	// Now is the UTC instant captured from Clock at the time Run was called.
	Now time.Time

	// EmptyReason is a human-readable explanation for why Events is empty.
	// Set when there are no Sessions ("no sessions") or when the focused
	// Session has no Events. Empty string when Events is non-empty.
	EmptyReason string
}

// WatchSession is the use case that surfaces the most-recently-active
// Session's last N Events for a Project. Construct via New.
type WatchSession struct {
	source ports.SessionSource
	clock  ports.Clock
	n      int
}

// New constructs a WatchSession with the given SessionSource and Clock.
// The default window size is 5 (last 5 Events shown). Use WithN to override.
func New(source ports.SessionSource, clock ports.Clock) WatchSession {
	return WatchSession{
		source: source,
		clock:  clock,
		n:      defaultN,
	}
}

// Run executes the use case for the Project at the given absolute working
// directory path. It discovers Sessions, focuses the most-recently-active
// one, retrieves its Events, and returns a SessionView.
//
// Returns an empty SessionView (no error) when no Sessions exist.
func (w WatchSession) Run(ctx context.Context, projectCWD string) (SessionView, error) {
	now := w.clock.Now()

	summaries, err := w.source.Sessions(ctx, projectCWD)
	if err != nil {
		return SessionView{Now: now}, err
	}
	if len(summaries) == 0 {
		return SessionView{Now: now, EmptyReason: "no sessions"}, nil
	}

	// Select the Session with the greatest LastActivity timestamp.
	focused := summaries[0]
	for _, s := range summaries[1:] {
		if s.LastActivity.After(focused.LastActivity) {
			focused = s
		}
	}

	evts, err := w.source.Events(ctx, focused.ID)
	if err != nil {
		return SessionView{Now: now, FocusedSession: focused.ID}, err
	}

	// Sort ascending by Timestamp (chronological order).
	sort.Slice(evts, func(i, j int) bool {
		return evts[i].Timestamp.Before(evts[j].Timestamp)
	})

	// Filter: remove events that should not reach the focused-pane view (ADR-007).
	// This runs BEFORE the last-N truncation so N counts visible events only.
	visible := evts[:0]
	for _, ev := range evts {
		if isVisible(ev) {
			visible = append(visible, ev)
		}
	}
	evts = visible

	// Take last min(n, len) visible Events.
	if len(evts) > w.n {
		evts = evts[len(evts)-w.n:]
	}

	emptyReason := ""
	if len(evts) == 0 {
		emptyReason = "session has no events"
	}

	return SessionView{
		FocusedSession: focused.ID,
		Events:         evts,
		Now:            now,
		EmptyReason:    emptyReason,
	}, nil
}

// isVisible returns true for events that should reach the focused-pane view.
//
// Hides:
//   - Opaque events (KindOpaque): unrecognized JSONL event types that the TUI
//     cannot meaningfully display. ADR-007 rule 1.
//   - User events with IsMeta = true: skill prompt injections and system
//     scaffolding messages, not human input. ADR-007 rule 2.
//   - User events with IsToolResultOnly = true: turn plumbing carrying only
//     tool_result blocks, not human text. ADR-007 rule 3.
func isVisible(ev event.Event) bool {
	if ev.Kind == event.KindOpaque {
		return false
	}
	if up, ok := ev.Payload.(event.UserPayload); ok {
		if up.IsMeta || up.IsToolResultOnly {
			return false
		}
	}
	return true
}
