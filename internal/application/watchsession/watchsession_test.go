package watchsession_test

import (
	"context"
	"testing"
	"time"

	"github.com/Systemartis/clyde/internal/domain/event"
	"github.com/Systemartis/clyde/internal/domain/session"
	"github.com/Systemartis/clyde/internal/ports"

	"github.com/Systemartis/clyde/internal/application/watchsession"
)

// stubSessionSource is a test-only in-memory fake for ports.SessionSource.
// It returns canned responses from its Sessions and Events fields.
type stubSessionSource struct {
	sessions []session.Summary
	events   map[session.ID][]event.Event
	err      error // non-nil means both Sessions and Events return this error
}

func (s *stubSessionSource) Sessions(_ context.Context, _ string) ([]session.Summary, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.sessions, nil
}

func (s *stubSessionSource) Events(_ context.Context, id session.ID) ([]event.Event, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.events == nil {
		return nil, nil
	}
	return s.events[id], nil
}

// compile-time check: stubSessionSource satisfies ports.SessionSource.
var _ ports.SessionSource = (*stubSessionSource)(nil)

// stubClock is a test-only clock that always returns a fixed time.
type stubClock struct {
	now time.Time
}

func (c stubClock) Now() time.Time { return c.now }

// compile-time check: stubClock satisfies ports.Clock.
var _ ports.Clock = stubClock{}

// mustTime parses a time string for test fixtures, panicking on error.
func mustTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic("mustTime: " + err.Error())
	}
	return t
}

// mkEvent is a convenience constructor for test Events.
func mkEvent(id string, ts time.Time, kind event.Kind, sid string) event.Event {
	return event.NewEvent(id, ts, kind, sid, "", event.UserPayload{})
}

func TestWatchSession(t *testing.T) {
	t.Parallel()

	fixedNow := mustTime("2026-04-30T12:00:00Z")

	t.Run("chronological_order", func(t *testing.T) {
		t.Parallel()
		// Spec: GIVEN a Project with a Session containing Events at T1<T2<T3
		// WHEN WatchSession retrieves Events
		// THEN Events MUST be returned in ascending timestamp order (T1, T2, T3).
		t1 := mustTime("2026-04-30T10:00:00Z")
		t2 := mustTime("2026-04-30T10:01:00Z")
		t3 := mustTime("2026-04-30T10:02:00Z")

		sid := session.ID("session-abc")
		evts := []event.Event{
			mkEvent("e1", t1, event.KindUser, "session-abc"),
			mkEvent("e2", t2, event.KindAssistant, "session-abc"),
			mkEvent("e3", t3, event.KindUser, "session-abc"),
		}

		src := &stubSessionSource{
			sessions: []session.Summary{{ID: sid, LastActivity: t3}},
			events:   map[session.ID][]event.Event{sid: evts},
		}
		clk := stubClock{now: fixedNow}
		ws := watchsession.New(src, clk)

		view, err := ws.Run(context.Background(), "/some/project")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(view.Events) != 3 {
			t.Fatalf("expected 3 events, got %d", len(view.Events))
		}
		for i, want := range []time.Time{t1, t2, t3} {
			if !view.Events[i].Timestamp.Equal(want) {
				t.Errorf("events[%d] timestamp = %v, want %v", i, view.Events[i].Timestamp, want)
			}
		}
	})

	t.Run("last_N_truncation", func(t *testing.T) {
		t.Parallel()
		// Spec: GIVEN a Session with more than N Events (default N=5)
		// WHEN WatchSession is executed
		// THEN ONLY the last N Events SHALL be included.
		sid := session.ID("session-big")
		base := mustTime("2026-04-30T09:00:00Z")
		var evts []event.Event
		for i := 0; i < 8; i++ {
			ts := base.Add(time.Duration(i) * time.Minute)
			evts = append(evts, mkEvent("e"+string(rune('0'+i)), ts, event.KindUser, "session-big"))
		}

		src := &stubSessionSource{
			sessions: []session.Summary{{ID: sid, LastActivity: evts[7].Timestamp}},
			events:   map[session.ID][]event.Event{sid: evts},
		}
		clk := stubClock{now: fixedNow}
		ws := watchsession.New(src, clk)

		view, err := ws.Run(context.Background(), "/some/project")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(view.Events) != 5 {
			t.Fatalf("expected 5 events (last N), got %d", len(view.Events))
		}
		// The last 5 of 8 are indices 3..7
		for i, want := range evts[3:] {
			if !view.Events[i].Timestamp.Equal(want.Timestamp) {
				t.Errorf("events[%d] timestamp = %v, want %v", i, view.Events[i].Timestamp, want.Timestamp)
			}
		}
	})

	t.Run("opaque_kind_preserved", func(t *testing.T) {
		// Superseded by event-rendering: opaque events are now filtered at the
		// WatchSession use case (ADR-007). See spec at openspec/specs/application/watch-session.md.
		// Replaced by TestWatchSession_FiltersOpaque.
		t.Skip("superseded by ADR-007 filter rule — opaque events are now filtered from view")
		t.Parallel()
		sid := session.ID("session-opaque")
		ts := mustTime("2026-04-30T10:00:00Z")
		opaqueEvt := event.NewEvent("eo1", ts, event.Kind("ai-title"), "session-opaque", "", event.OpaquePayload{Raw: []byte(`{"type":"ai-title"}`)})

		src := &stubSessionSource{
			sessions: []session.Summary{{ID: sid, LastActivity: ts}},
			events:   map[session.ID][]event.Event{sid: {opaqueEvt}},
		}
		clk := stubClock{now: fixedNow}
		ws := watchsession.New(src, clk)

		view, err := ws.Run(context.Background(), "/some/project")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(view.Events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(view.Events))
		}
		if view.Events[0].Kind != event.Kind("ai-title") {
			t.Errorf("expected kind ai-title, got %v", view.Events[0].Kind)
		}
	})

	t.Run("no_sessions", func(t *testing.T) {
		t.Parallel()
		// Spec: GIVEN a Project with no Sessions
		// WHEN WatchSession is executed
		// THEN result MUST be empty Event list, no error.
		src := &stubSessionSource{
			sessions: nil,
			events:   nil,
		}
		clk := stubClock{now: fixedNow}
		ws := watchsession.New(src, clk)

		view, err := ws.Run(context.Background(), "/empty/project")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(view.Events) != 0 {
			t.Errorf("expected 0 events, got %d", len(view.Events))
		}
		if view.FocusedSession != "" {
			t.Errorf("expected empty FocusedSession, got %v", view.FocusedSession)
		}
	})

	t.Run("multi_session_focus", func(t *testing.T) {
		t.Parallel()
		// Spec: GIVEN multiple Sessions with different latest Events
		// WHEN WatchSession is executed
		// THEN the Session whose latest Event has greatest timestamp is selected.
		// AND Events from all other Sessions MUST NOT appear.
		sidA := session.ID("session-a")
		sidB := session.ID("session-b")
		tA := mustTime("2026-04-30T10:00:00Z")
		tB := mustTime("2026-04-30T11:00:00Z") // B is more recent

		evtsA := []event.Event{mkEvent("eA1", tA, event.KindUser, "session-a")}
		evtsB := []event.Event{mkEvent("eB1", tB, event.KindAssistant, "session-b")}

		src := &stubSessionSource{
			// summaries returned with A first, but B has later LastActivity
			sessions: []session.Summary{
				{ID: sidA, LastActivity: tA},
				{ID: sidB, LastActivity: tB},
			},
			events: map[session.ID][]event.Event{
				sidA: evtsA,
				sidB: evtsB,
			},
		}
		clk := stubClock{now: fixedNow}
		ws := watchsession.New(src, clk)

		view, err := ws.Run(context.Background(), "/multi/project")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if view.FocusedSession != sidB {
			t.Errorf("expected focused session %v, got %v", sidB, view.FocusedSession)
		}
		if len(view.Events) != 1 {
			t.Fatalf("expected 1 event from session B, got %d", len(view.Events))
		}
		if view.Events[0].ID != "eB1" {
			t.Errorf("expected event eB1, got %v", view.Events[0].ID)
		}
	})

	t.Run("fewer_than_N", func(t *testing.T) {
		t.Parallel()
		// Spec: GIVEN a Session with fewer than N Events
		// WHEN WatchSession is executed
		// THEN ALL Events SHALL be returned (no padding).
		sid := session.ID("session-small")
		t1 := mustTime("2026-04-30T10:00:00Z")
		t2 := mustTime("2026-04-30T10:01:00Z")
		t3 := mustTime("2026-04-30T10:02:00Z")

		evts := []event.Event{
			mkEvent("e1", t1, event.KindUser, "session-small"),
			mkEvent("e2", t2, event.KindAssistant, "session-small"),
			mkEvent("e3", t3, event.KindUser, "session-small"),
		}

		src := &stubSessionSource{
			sessions: []session.Summary{{ID: sid, LastActivity: t3}},
			events:   map[session.ID][]event.Event{sid: evts},
		}
		clk := stubClock{now: fixedNow}
		ws := watchsession.New(src, clk)

		view, err := ws.Run(context.Background(), "/small/project")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// 3 < N(5), so all 3 must be returned
		if len(view.Events) != 3 {
			t.Fatalf("expected 3 events (fewer-than-N passthrough), got %d", len(view.Events))
		}
	})
}

// mkUserEvent builds a user event with explicit payload fields for filter tests.
func mkUserEvent(id string, ts time.Time, sid string, payload event.UserPayload) event.Event {
	return event.NewEvent(id, ts, event.KindUser, sid, "", payload)
}

// mkAssistantEvent builds an assistant event for filter tests.
func mkAssistantEvent(id string, ts time.Time, sid string) event.Event {
	return event.NewEvent(id, ts, event.KindAssistant, sid, "", event.AssistantPayload{})
}

// mkOpaqueEvent builds an opaque event for filter tests.
func mkOpaqueEvent(id string, ts time.Time, sid string) event.Event {
	return event.NewEvent(id, ts, event.KindOpaque, sid, "", event.OpaquePayload{Raw: []byte(`{"type":"opaque"}`)})
}

// TestWatchSession_FiltersOpaque: session with user + opaque + assistant events.
// Run() MUST return only user + assistant (opaque filtered out), order preserved.
// Spec: "Opaque events filtered from view" (ADR-007).
func TestWatchSession_FiltersOpaque(t *testing.T) {
	t.Parallel()

	fixedNow := mustTime("2026-04-30T12:00:00Z")
	sid := session.ID("session-filter-opaque")
	t1 := mustTime("2026-04-30T10:00:00Z")
	t2 := mustTime("2026-04-30T10:01:00Z")
	t3 := mustTime("2026-04-30T10:02:00Z")

	userEvt := mkUserEvent("u1", t1, string(sid), event.UserPayload{Summary: "Hello"})
	opaqueEvt := mkOpaqueEvent("op1", t2, string(sid))
	assistantEvt := mkAssistantEvent("a1", t3, string(sid))

	src := &stubSessionSource{
		sessions: []session.Summary{{ID: sid, LastActivity: t3}},
		events:   map[session.ID][]event.Event{sid: {userEvt, opaqueEvt, assistantEvt}},
	}
	clk := stubClock{now: fixedNow}
	ws := watchsession.New(src, clk)

	view, err := ws.Run(context.Background(), "/filter/opaque")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(view.Events) != 2 {
		t.Fatalf("expected 2 events (user + assistant), got %d", len(view.Events))
	}
	if view.Events[0].Kind != event.KindUser {
		t.Errorf("events[0] kind = %v, want KindUser", view.Events[0].Kind)
	}
	if view.Events[1].Kind != event.KindAssistant {
		t.Errorf("events[1] kind = %v, want KindAssistant", view.Events[1].Kind)
	}
}

// TestWatchSession_FiltersMetaUser: session with IsMeta=true user event and normal user event.
// Run() MUST return only the non-meta user event.
// Spec: "Meta user events filtered from view" (ADR-007).
func TestWatchSession_FiltersMetaUser(t *testing.T) {
	t.Parallel()

	fixedNow := mustTime("2026-04-30T12:00:00Z")
	sid := session.ID("session-filter-meta")
	t1 := mustTime("2026-04-30T10:00:00Z")
	t2 := mustTime("2026-04-30T10:01:00Z")

	metaEvt := mkUserEvent("u-meta", t1, string(sid), event.UserPayload{IsMeta: true})
	typedEvt := mkUserEvent("u-typed", t2, string(sid), event.UserPayload{Summary: "Hello world"})

	src := &stubSessionSource{
		sessions: []session.Summary{{ID: sid, LastActivity: t2}},
		events:   map[session.ID][]event.Event{sid: {metaEvt, typedEvt}},
	}
	clk := stubClock{now: fixedNow}
	ws := watchsession.New(src, clk)

	view, err := ws.Run(context.Background(), "/filter/meta")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(view.Events) != 1 {
		t.Fatalf("expected 1 event (typed user only), got %d", len(view.Events))
	}
	if view.Events[0].ID != "u-typed" {
		t.Errorf("expected event id u-typed, got %v", view.Events[0].ID)
	}
}

// TestWatchSession_FiltersToolResultOnly: session with IsToolResultOnly=true user event and typed user event.
// Run() MUST return only the typed (non-tool-result-only) user event.
// Spec: "Tool-result user events filtered from view" (ADR-007).
func TestWatchSession_FiltersToolResultOnly(t *testing.T) {
	t.Parallel()

	fixedNow := mustTime("2026-04-30T12:00:00Z")
	sid := session.ID("session-filter-toolresult")
	t1 := mustTime("2026-04-30T10:00:00Z")
	t2 := mustTime("2026-04-30T10:01:00Z")

	toolResultEvt := mkUserEvent("u-toolresult", t1, string(sid), event.UserPayload{IsToolResultOnly: true})
	typedEvt := mkUserEvent("u-typed", t2, string(sid), event.UserPayload{Summary: "Summary text"})

	src := &stubSessionSource{
		sessions: []session.Summary{{ID: sid, LastActivity: t2}},
		events:   map[session.ID][]event.Event{sid: {toolResultEvt, typedEvt}},
	}
	clk := stubClock{now: fixedNow}
	ws := watchsession.New(src, clk)

	view, err := ws.Run(context.Background(), "/filter/toolresult")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(view.Events) != 1 {
		t.Fatalf("expected 1 event (typed user only), got %d", len(view.Events))
	}
	if view.Events[0].ID != "u-typed" {
		t.Errorf("expected event id u-typed, got %v", view.Events[0].ID)
	}
}

// TestWatchSession_FilterPreservesOrder: 6 events alternating visible/invisible.
// Run() MUST return the 3 visible events in ascending timestamp order.
// Spec: "Order preserved" (ADR-007).
func TestWatchSession_FilterPreservesOrder(t *testing.T) {
	t.Parallel()

	fixedNow := mustTime("2026-04-30T12:00:00Z")
	sid := session.ID("session-filter-order")
	base := mustTime("2026-04-30T10:00:00Z")

	// Alternate: visible, invisible, visible, invisible, visible, invisible
	evts := []event.Event{
		mkUserEvent("u1", base.Add(0*time.Minute), string(sid), event.UserPayload{Summary: "First"}),
		mkOpaqueEvent("op1", base.Add(1*time.Minute), string(sid)),
		mkAssistantEvent("a1", base.Add(2*time.Minute), string(sid)),
		mkUserEvent("u-meta", base.Add(3*time.Minute), string(sid), event.UserPayload{IsMeta: true}),
		mkUserEvent("u2", base.Add(4*time.Minute), string(sid), event.UserPayload{Summary: "Second"}),
		mkUserEvent("u-tool", base.Add(5*time.Minute), string(sid), event.UserPayload{IsToolResultOnly: true}),
	}

	src := &stubSessionSource{
		sessions: []session.Summary{{ID: sid, LastActivity: evts[5].Timestamp}},
		events:   map[session.ID][]event.Event{sid: evts},
	}
	clk := stubClock{now: fixedNow}
	ws := watchsession.New(src, clk)

	view, err := ws.Run(context.Background(), "/filter/order")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(view.Events) != 3 {
		t.Fatalf("expected 3 visible events, got %d", len(view.Events))
	}
	wantIDs := []string{"u1", "a1", "u2"}
	for i, wantID := range wantIDs {
		if view.Events[i].ID != wantID {
			t.Errorf("events[%d].ID = %v, want %v", i, view.Events[i].ID, wantID)
		}
	}
}

// TestWatchSession_FilterDoesNotAffectN: 10 visible + 5 hidden interleaved.
// Run() MUST return last 5 of the visible events (not last 5 of unfiltered).
// Spec: "N counts visible events only" (ADR-007).
func TestWatchSession_FilterDoesNotAffectN(t *testing.T) {
	t.Parallel()

	fixedNow := mustTime("2026-04-30T12:00:00Z")
	sid := session.ID("session-filter-n")
	base := mustTime("2026-04-30T10:00:00Z")

	// Build 15 events: 10 visible (typed user) interleaved with 5 invisible (opaque)
	// Layout: V V O V V O V V O V V O V V O
	// Visible IDs (in order): v0..v9
	// Opaque IDs: op0..op4
	var evts []event.Event
	visibleIdx := 0
	opaqueIdx := 0
	for i := 0; i < 15; i++ {
		ts := base.Add(time.Duration(i) * time.Minute)
		if i%3 == 2 {
			evts = append(evts, mkOpaqueEvent("op"+string(rune('0'+opaqueIdx)), ts, string(sid)))
			opaqueIdx++
		} else {
			evts = append(evts, mkUserEvent("v"+string(rune('0'+visibleIdx)), ts, string(sid), event.UserPayload{Summary: "visible"}))
			visibleIdx++
		}
	}
	// Visible events: v0..v9 at indices 0,1,3,4,6,7,9,10,12,13
	// Last 5 visible: v5,v6,v7,v8,v9 → IDs: v5,v6,v7,v8,v9

	src := &stubSessionSource{
		sessions: []session.Summary{{ID: sid, LastActivity: evts[14].Timestamp}},
		events:   map[session.ID][]event.Event{sid: evts},
	}
	clk := stubClock{now: fixedNow}
	ws := watchsession.New(src, clk)

	view, err := ws.Run(context.Background(), "/filter/n-count")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(view.Events) != 5 {
		t.Fatalf("expected 5 events (last 5 visible), got %d", len(view.Events))
	}
	// All returned events MUST be visible (KindUser with no filter flags)
	for i, ev := range view.Events {
		if ev.Kind != event.KindUser {
			t.Errorf("events[%d] kind = %v, want KindUser (all returned must be visible)", i, ev.Kind)
		}
		up, ok := ev.Payload.(event.UserPayload)
		if !ok {
			t.Errorf("events[%d] payload is not UserPayload", i)
			continue
		}
		if up.IsMeta || up.IsToolResultOnly {
			t.Errorf("events[%d] is a filtered event (IsMeta=%v IsToolResultOnly=%v) but was returned", i, up.IsMeta, up.IsToolResultOnly)
		}
	}
}
