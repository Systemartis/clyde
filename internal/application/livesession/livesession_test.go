package livesession_test

import (
	"context"
	"testing"
	"time"

	"github.com/Systemartis/clyde/internal/application/livesession"
	"github.com/Systemartis/clyde/internal/domain/event"
	"github.com/Systemartis/clyde/internal/domain/project"
	"github.com/Systemartis/clyde/internal/domain/session"
	usagePkg "github.com/Systemartis/clyde/internal/domain/usage"
	"github.com/Systemartis/clyde/internal/ports"
)

// ─── Test doubles ─────────────────────────────────────────────────────────────

// fakeSessionSource is an in-memory fake that satisfies ports.SessionSource.
type fakeSessionSource struct {
	sessions []session.Summary
	events   map[session.ID][]event.Event
	sessErr  error
	evtErr   error
}

func (f *fakeSessionSource) Sessions(_ context.Context, _ string) ([]session.Summary, error) {
	if f.sessErr != nil {
		return nil, f.sessErr
	}
	return f.sessions, nil
}

func (f *fakeSessionSource) Events(_ context.Context, id session.ID) ([]event.Event, error) {
	if f.evtErr != nil {
		return nil, f.evtErr
	}
	if f.events == nil {
		return nil, nil
	}
	return f.events[id], nil
}

// compile-time check
var _ ports.SessionSource = (*fakeSessionSource)(nil)

// fakeGlobalSessionSource is an in-memory fake that satisfies ports.GlobalSessionSource.
type fakeGlobalSessionSource struct {
	refs    []ports.GlobalSessionRef
	refsErr error
}

func (f *fakeGlobalSessionSource) AllProjectSessions(_ context.Context, maxResults int) ([]ports.GlobalSessionRef, error) {
	if f.refsErr != nil {
		return nil, f.refsErr
	}
	refs := f.refs
	if maxResults > 0 && len(refs) > maxResults {
		refs = refs[:maxResults]
	}
	return refs, nil
}

// compile-time check
var _ ports.GlobalSessionSource = (*fakeGlobalSessionSource)(nil)

// fakeClock always returns a fixed instant.
type fakeClock struct {
	now time.Time
}

func (c fakeClock) Now() time.Time { return c.now }

// compile-time check
var _ ports.Clock = fakeClock{}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func mustTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic("mustTime: " + err.Error())
	}
	return t
}

func mkUserEvt(id string, ts time.Time, sid string, payload event.UserPayload) event.Event {
	return event.NewEvent(id, ts, event.KindUser, sid, "", payload)
}

func mkAssistantEvt(id string, ts time.Time, sid string) event.Event {
	return event.NewEvent(id, ts, event.KindAssistant, sid, "", event.AssistantPayload{})
}

func mkOpaqueEvt(id string, ts time.Time, sid string) event.Event {
	return event.NewEvent(id, ts, event.KindOpaque, sid, "", event.OpaquePayload{Raw: []byte(`{}`)})
}

func newProject(cwd string) project.Project {
	return project.New(cwd)
}

// ─── Tests ────────────────────────────────────────────────────────────────────

// TestSnapshot_NoSessions: project with no sessions returns empty view, no error.
func TestSnapshot_NoSessions(t *testing.T) {
	t.Parallel()
	src := &fakeSessionSource{}
	clk := fakeClock{now: mustTime("2026-04-30T12:00:00Z")}
	ls := livesession.New(src, clk)

	view, err := ls.Snapshot(context.Background(), newProject("/some/project"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if view.FocusedID != "" {
		t.Errorf("FocusedID = %q, want empty", view.FocusedID)
	}
	if len(view.Sessions) != 0 {
		t.Errorf("Sessions len = %d, want 0", len(view.Sessions))
	}
	if len(view.Events) != 0 {
		t.Errorf("Events len = %d, want 0", len(view.Events))
	}
	if view.EmptyReason == "" {
		t.Error("EmptyReason should be set when no sessions exist")
	}
}

// TestSnapshot_FocusesMostRecent: given two sessions, the most recently active
// one (by LastActivity) MUST be selected as FocusedID.
func TestSnapshot_FocusesMostRecent(t *testing.T) {
	t.Parallel()

	sidOld := session.ID("old-session")
	sidNew := session.ID("new-session")
	tOld := mustTime("2026-04-29T10:00:00Z")
	tNew := mustTime("2026-04-30T10:00:00Z")

	src := &fakeSessionSource{
		sessions: []session.Summary{
			{ID: sidOld, LastActivity: tOld},
			{ID: sidNew, LastActivity: tNew},
		},
		events: map[session.ID][]event.Event{
			sidOld: {mkUserEvt("u-old", tOld, string(sidOld), event.UserPayload{Summary: "old"})},
			sidNew: {mkUserEvt("u-new", tNew, string(sidNew), event.UserPayload{Summary: "new"})},
		},
	}
	clk := fakeClock{now: mustTime("2026-04-30T12:00:00Z")}
	ls := livesession.New(src, clk)

	view, err := ls.Snapshot(context.Background(), newProject("/multi/project"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if view.FocusedID != sidNew {
		t.Errorf("FocusedID = %v, want %v", view.FocusedID, sidNew)
	}
	// Sessions must be sorted descending by LastActivity.
	if len(view.Sessions) != 2 {
		t.Fatalf("Sessions len = %d, want 2", len(view.Sessions))
	}
	if view.Sessions[0].ID != sidNew {
		t.Errorf("Sessions[0].ID = %v, want %v (most recent first)", view.Sessions[0].ID, sidNew)
	}
}

// TestSnapshot_EventsChronological: events of the focused session MUST be sorted
// ascending by Timestamp regardless of source order.
func TestSnapshot_EventsChronological(t *testing.T) {
	t.Parallel()

	sid := session.ID("chrono-session")
	t1 := mustTime("2026-04-30T10:00:00Z")
	t2 := mustTime("2026-04-30T10:01:00Z")
	t3 := mustTime("2026-04-30T10:02:00Z")

	// Source returns events in reverse order to test sorting.
	src := &fakeSessionSource{
		sessions: []session.Summary{{ID: sid, LastActivity: t3}},
		events: map[session.ID][]event.Event{
			sid: {
				mkAssistantEvt("a1", t3, string(sid)),
				mkUserEvt("u1", t2, string(sid), event.UserPayload{Summary: "hello"}),
				mkUserEvt("u0", t1, string(sid), event.UserPayload{Summary: "first"}),
			},
		},
	}
	clk := fakeClock{now: mustTime("2026-04-30T12:00:00Z")}
	ls := livesession.New(src, clk)

	view, err := ls.Snapshot(context.Background(), newProject("/chrono/project"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(view.Events) != 3 {
		t.Fatalf("Events len = %d, want 3", len(view.Events))
	}
	for i, wantTS := range []time.Time{t1, t2, t3} {
		if !view.Events[i].Timestamp.Equal(wantTS) {
			t.Errorf("Events[%d].Timestamp = %v, want %v", i, view.Events[i].Timestamp, wantTS)
		}
	}
}

// TestSnapshot_FiltersOpaque: opaque events MUST NOT appear in the view.
func TestSnapshot_FiltersOpaque(t *testing.T) {
	t.Parallel()

	sid := session.ID("filter-opaque")
	t1 := mustTime("2026-04-30T10:00:00Z")
	t2 := mustTime("2026-04-30T10:01:00Z")
	t3 := mustTime("2026-04-30T10:02:00Z")

	src := &fakeSessionSource{
		sessions: []session.Summary{{ID: sid, LastActivity: t3}},
		events: map[session.ID][]event.Event{
			sid: {
				mkUserEvt("u1", t1, string(sid), event.UserPayload{Summary: "visible"}),
				mkOpaqueEvt("op1", t2, string(sid)),
				mkAssistantEvt("a1", t3, string(sid)),
			},
		},
	}
	clk := fakeClock{now: mustTime("2026-04-30T12:00:00Z")}
	ls := livesession.New(src, clk)

	view, err := ls.Snapshot(context.Background(), newProject("/filter/opaque"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(view.Events) != 2 {
		t.Fatalf("Events len = %d, want 2 (user + assistant, opaque filtered)", len(view.Events))
	}
	for _, ev := range view.Events {
		if ev.Kind == event.KindOpaque {
			t.Error("KindOpaque event must not appear in the view")
		}
	}
}

// TestSnapshot_FiltersMetaAndToolResultOnly: meta and tool-result-only user events
// MUST be excluded.
func TestSnapshot_FiltersMetaAndToolResultOnly(t *testing.T) {
	t.Parallel()

	sid := session.ID("filter-user-variants")
	base := mustTime("2026-04-30T10:00:00Z")

	src := &fakeSessionSource{
		sessions: []session.Summary{{ID: sid, LastActivity: base.Add(3 * time.Minute)}},
		events: map[session.ID][]event.Event{
			sid: {
				mkUserEvt("u-meta", base.Add(0), string(sid), event.UserPayload{IsMeta: true}),
				mkUserEvt("u-toolresult", base.Add(time.Minute), string(sid), event.UserPayload{IsToolResultOnly: true}),
				mkUserEvt("u-visible", base.Add(2*time.Minute), string(sid), event.UserPayload{Summary: "real prompt"}),
				mkAssistantEvt("a1", base.Add(3*time.Minute), string(sid)),
			},
		},
	}
	clk := fakeClock{now: mustTime("2026-04-30T12:00:00Z")}
	ls := livesession.New(src, clk)

	view, err := ls.Snapshot(context.Background(), newProject("/filter/user"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(view.Events) != 2 {
		t.Fatalf("Events len = %d, want 2 (typed user + assistant)", len(view.Events))
	}
	if view.Events[0].ID != "u-visible" {
		t.Errorf("Events[0].ID = %q, want u-visible", view.Events[0].ID)
	}
	if view.Events[1].ID != "a1" {
		t.Errorf("Events[1].ID = %q, want a1", view.Events[1].ID)
	}
}

// TestSnapshot_ReturnsAllVisible: LiveSession does NOT apply a top-N truncation —
// all visible events are returned.
func TestSnapshot_ReturnsAllVisible(t *testing.T) {
	t.Parallel()

	sid := session.ID("big-session")
	base := mustTime("2026-04-30T10:00:00Z")

	const eventCount = 20
	evts := make([]event.Event, eventCount)
	for i := 0; i < eventCount; i++ {
		ts := base.Add(time.Duration(i) * time.Minute)
		evts[i] = mkAssistantEvt("a"+string(rune('0'+i%10)), ts, string(sid))
	}

	src := &fakeSessionSource{
		sessions: []session.Summary{{ID: sid, LastActivity: evts[eventCount-1].Timestamp}},
		events:   map[session.ID][]event.Event{sid: evts},
	}
	clk := fakeClock{now: mustTime("2026-04-30T12:00:00Z")}
	ls := livesession.New(src, clk)

	view, err := ls.Snapshot(context.Background(), newProject("/big/project"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// ALL 20 events must be returned (no top-N truncation like WatchSession).
	if len(view.Events) != eventCount {
		t.Errorf("Events len = %d, want %d (no top-N truncation)", len(view.Events), eventCount)
	}
}

// TestSnapshot_LastUpdateFromClock: LastUpdate MUST equal the clock instant.
func TestSnapshot_LastUpdateFromClock(t *testing.T) {
	t.Parallel()

	fixedNow := mustTime("2026-04-30T14:00:00Z")
	src := &fakeSessionSource{}
	clk := fakeClock{now: fixedNow}
	ls := livesession.New(src, clk)

	view, err := ls.Snapshot(context.Background(), newProject("/clock/project"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !view.LastUpdate.Equal(fixedNow) {
		t.Errorf("LastUpdate = %v, want %v", view.LastUpdate, fixedNow)
	}
}

// TestSnapshot_ProjectPreserved: the returned view MUST carry the same project as input.
func TestSnapshot_ProjectPreserved(t *testing.T) {
	t.Parallel()

	cwd := "/projects/my-app"
	src := &fakeSessionSource{}
	clk := fakeClock{now: mustTime("2026-04-30T12:00:00Z")}
	ls := livesession.New(src, clk)

	p := newProject(cwd)
	view, err := ls.Snapshot(context.Background(), p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if view.Project.CWD() != cwd {
		t.Errorf("view.Project.CWD() = %q, want %q", view.Project.CWD(), cwd)
	}
}

// TestSnapshot_MultiWindowUsage_5hAndWeek verifies that Usage5h and UsageWeek
// aggregate session usage for the correct time windows.
//
// Setup:
//   - Session A (focused): active 1 hour ago, 1000 input tokens
//   - Session B:           active 3 hours ago, 2000 input tokens  → within 5h AND week
//   - Session C:           active 6 hours ago, 4000 input tokens  → within week only
//   - Session D:           active 8 days ago,  8000 input tokens  → outside both windows
func TestSnapshot_MultiWindowUsage_5hAndWeek(t *testing.T) {
	t.Parallel()

	now := mustTime("2026-04-30T12:00:00Z")
	sidA := session.ID("sess-a") // focused, 1h ago
	sidB := session.ID("sess-b") // 3h ago
	sidC := session.ID("sess-c") // 6h ago
	sidD := session.ID("sess-d") // 8d ago

	tA := now.Add(-1 * time.Hour)
	tB := now.Add(-3 * time.Hour)
	tC := now.Add(-6 * time.Hour)
	tD := now.Add(-8 * 24 * time.Hour)

	mkAssistantWithUsage := func(id string, ts time.Time, sid string, inputTok int64) event.Event {
		ap := event.AssistantPayload{
			Usage: usagePkg.Usage{Input: inputTok},
		}
		return event.NewEvent(id, ts, event.KindAssistant, sid, "", ap)
	}

	src := &fakeSessionSource{
		sessions: []session.Summary{
			{ID: sidA, LastActivity: tA},
			{ID: sidB, LastActivity: tB},
			{ID: sidC, LastActivity: tC},
			{ID: sidD, LastActivity: tD},
		},
		events: map[session.ID][]event.Event{
			sidA: {mkAssistantWithUsage("evA", tA, string(sidA), 1_000)},
			sidB: {mkAssistantWithUsage("evB", tB, string(sidB), 2_000)},
			sidC: {mkAssistantWithUsage("evC", tC, string(sidC), 4_000)},
			sidD: {mkAssistantWithUsage("evD", tD, string(sidD), 8_000)},
		},
	}
	clk := fakeClock{now: now}
	ls := livesession.New(src, clk)

	view, err := ls.Snapshot(context.Background(), newProject("/test/project"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Usage5h: sessions A (1h) + B (3h) = 1000 + 2000 = 3000 input tokens.
	want5h := int64(3_000)
	got5h := view.Usage5h.Input
	if got5h != want5h {
		t.Errorf("Usage5h.Input = %d; want %d (sessions A+B within 5h window)", got5h, want5h)
	}

	// UsageWeek: sessions A (1h) + B (3h) + C (6h) = 1000 + 2000 + 4000 = 7000.
	// Session D (8 days ago) must be excluded.
	wantWeek := int64(7_000)
	gotWeek := view.UsageWeek.Input
	if gotWeek != wantWeek {
		t.Errorf("UsageWeek.Input = %d; want %d (sessions A+B+C within 7d window)", gotWeek, wantWeek)
	}
}

// TestApplyUsageStats_1MContextDetection verifies that a session with
// Has1hCache=true events produces a CurrentModel with the "[1m]" suffix.
//
// Regression test for Bug 1: the JSONL message.model field is "claude-opus-4-7"
// with no context-size suffix.  The [1m] suffix is synthesized by livesession
// when it detects the 1-hour prompt-cache tier in the usage data.
func TestApplyUsageStats_1MContextDetection(t *testing.T) {
	t.Parallel()

	sid := session.ID("1m-session")
	ts := mustTime("2026-04-30T10:00:00Z")

	// Build an assistant event that reports 1h cache usage (Max plan / 1M context).
	evt1M := event.NewEvent("ev-1m", ts, event.KindAssistant, string(sid), "",
		event.AssistantPayload{
			Model:      "claude-opus-4-7",
			Has1hCache: true,
			Usage:      usagePkg.Usage{Input: 10, Output: 5},
		},
	)

	src := &fakeSessionSource{
		sessions: []session.Summary{{ID: sid, LastActivity: ts}},
		events:   map[session.ID][]event.Event{sid: {evt1M}},
	}
	clk := fakeClock{now: ts.Add(time.Minute)}
	ls := livesession.New(src, clk)

	view, err := ls.Snapshot(context.Background(), newProject("/some/project"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	const wantModel = "claude-opus-4-7[1m]"
	if view.CurrentModel != wantModel {
		t.Errorf("CurrentModel = %q; want %q (1h-cache should synthesize [1m] suffix)",
			view.CurrentModel, wantModel)
	}
}

// TestApplyUsageStats_StandardPlan_NoSuffix verifies that a session without
// 1h-cache usage keeps the model ID unchanged (no spurious [1m] suffix).
func TestApplyUsageStats_StandardPlan_NoSuffix(t *testing.T) {
	t.Parallel()

	sid := session.ID("std-session")
	ts := mustTime("2026-04-30T10:00:00Z")

	evtStd := event.NewEvent("ev-std", ts, event.KindAssistant, string(sid), "",
		event.AssistantPayload{
			Model:      "claude-opus-4-7",
			Has1hCache: false, // standard plan: no 1h cache
			Usage:      usagePkg.Usage{Input: 10, Output: 5},
		},
	)

	src := &fakeSessionSource{
		sessions: []session.Summary{{ID: sid, LastActivity: ts}},
		events:   map[session.ID][]event.Event{sid: {evtStd}},
	}
	clk := fakeClock{now: ts.Add(time.Minute)}
	ls := livesession.New(src, clk)

	view, err := ls.Snapshot(context.Background(), newProject("/some/project"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	const wantModel = "claude-opus-4-7"
	if view.CurrentModel != wantModel {
		t.Errorf("CurrentModel = %q; want %q (no 1h-cache → no [1m] suffix)", view.CurrentModel, wantModel)
	}
}

// TestApplyUsageStats_LatestUsage verifies that View.LatestUsage reflects the most
// recent assistant event's usage (not the running sum), so it can be used for
// accurate compaction percentage calculation.
func TestApplyUsageStats_LatestUsage(t *testing.T) {
	t.Parallel()

	sid := session.ID("latest-usage-session")
	t1 := mustTime("2026-04-30T10:00:00Z")
	t2 := t1.Add(time.Minute)

	// Two assistant events: early one has modest usage, later one has large cache_read.
	// LatestUsage should reflect the SECOND event only, not the sum.
	evt1 := event.NewEvent("ev-a", t1, event.KindAssistant, string(sid), "",
		event.AssistantPayload{
			Model: "claude-opus-4-7",
			Usage: usagePkg.Usage{Input: 100, Output: 200, CacheRead: 1_000, CacheCreation: 50},
		},
	)
	evt2 := event.NewEvent("ev-b", t2, event.KindAssistant, string(sid), "",
		event.AssistantPayload{
			Model: "claude-opus-4-7",
			Usage: usagePkg.Usage{Input: 10, Output: 300, CacheRead: 750_000, CacheCreation: 500},
		},
	)

	src := &fakeSessionSource{
		sessions: []session.Summary{{ID: sid, LastActivity: t2}},
		events:   map[session.ID][]event.Event{sid: {evt1, evt2}},
	}
	clk := fakeClock{now: t2.Add(time.Minute)}
	ls := livesession.New(src, clk)

	view, err := ls.Snapshot(context.Background(), newProject("/some/project"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// TotalUsage must be the running sum (both events).
	wantTotal := usagePkg.Usage{
		Input:         110,
		Output:        500,
		CacheRead:     751_000,
		CacheCreation: 550,
	}
	if view.TotalUsage != wantTotal {
		t.Errorf("TotalUsage = %+v; want %+v", view.TotalUsage, wantTotal)
	}

	// LatestUsage must be only the second event's usage snapshot.
	wantLatest := usagePkg.Usage{Input: 10, Output: 300, CacheRead: 750_000, CacheCreation: 500}
	if view.LatestUsage != wantLatest {
		t.Errorf("LatestUsage = %+v; want %+v (should be the last event only, not running sum)",
			view.LatestUsage, wantLatest)
	}

	// Sanity check: LatestUsage.CacheRead should be much larger than TotalUsage.Input,
	// confirming that CacheRead from the running sum would be wildly inflated.
	if view.TotalUsage.CacheRead == view.LatestUsage.CacheRead {
		t.Logf("note: TotalUsage.CacheRead (%d) == LatestUsage.CacheRead (%d) by coincidence"+
			" (only one large cache-read event in test)", view.TotalUsage.CacheRead, view.LatestUsage.CacheRead)
	}
}

// TestSnapshot_CrossProjectMultiWindowUsage verifies that when a GlobalSessionSource
// is wired, Usage5h and UsageWeek include sessions from OTHER projects — not just
// the current project.
//
// Setup:
//   - Project "clyde": focused session sidA (1h ago, 1000 input tokens)
//   - Project "bordergen": session sidB (2h ago, 2000 input tokens)  → 5h + week
//   - Project "nexus": session sidC (4h ago, 4000 input tokens)      → 5h + week
//   - Project "diary2": session sidD (6h ago, 8000 input tokens)     → week only
//   - Project "old": session sidE (8d ago, 16000 input tokens)       → excluded
//
// Expected:
//   - Usage5h.Input = 1000 + 2000 + 4000 = 7000
//   - UsageWeek.Input = 1000 + 2000 + 4000 + 8000 = 15000
//   - Sessions5hCount = 3
//   - SessionsWeekCount = 4
func TestSnapshot_CrossProjectMultiWindowUsage(t *testing.T) {
	t.Parallel()

	now := mustTime("2026-04-30T12:00:00Z")

	sidA := session.ID("sess-clyde-a")     // focused: 1h ago
	sidB := session.ID("sess-bordergen-b") // other project: 2h ago
	sidC := session.ID("sess-nexus-c")     // other project: 4h ago
	sidD := session.ID("sess-diary2-d")    // other project: 6h ago
	sidE := session.ID("sess-old-e")       // other project: 8d ago

	tA := now.Add(-1 * time.Hour)
	tB := now.Add(-2 * time.Hour)
	tC := now.Add(-4 * time.Hour)
	tD := now.Add(-6 * time.Hour)
	tE := now.Add(-8 * 24 * time.Hour)

	mkAssistantWithUsage := func(id string, ts time.Time, sid string, inputTok int64) event.Event {
		ap := event.AssistantPayload{
			Usage: usagePkg.Usage{Input: inputTok},
		}
		return event.NewEvent(id, ts, event.KindAssistant, sid, "", ap)
	}

	// The per-project SessionSource only knows about project "clyde"'s sessions.
	src := &fakeSessionSource{
		sessions: []session.Summary{
			{ID: sidA, LastActivity: tA},
		},
		events: map[session.ID][]event.Event{
			sidA: {mkAssistantWithUsage("evA", tA, string(sidA), 1_000)},
			sidB: {mkAssistantWithUsage("evB", tB, string(sidB), 2_000)},
			sidC: {mkAssistantWithUsage("evC", tC, string(sidC), 4_000)},
			sidD: {mkAssistantWithUsage("evD", tD, string(sidD), 8_000)},
			sidE: {mkAssistantWithUsage("evE", tE, string(sidE), 16_000)},
		},
	}

	// The GlobalSessionSource returns sessions from ALL projects ordered by recency.
	globalSrc := &fakeGlobalSessionSource{
		refs: []ports.GlobalSessionRef{
			{ProjectEncodedDir: "-Users-vladpb-work-Personal-clyde", SessionID: sidA, LastActivity: tA, Path: "/fake/clyde/sess-clyde-a.jsonl"},
			{ProjectEncodedDir: "-Users-vladpb-work-Personal-bordergen", SessionID: sidB, LastActivity: tB, Path: "/fake/bordergen/sess-bordergen-b.jsonl"},
			{ProjectEncodedDir: "-Users-vladpb-work-Personal-nexus", SessionID: sidC, LastActivity: tC, Path: "/fake/nexus/sess-nexus-c.jsonl"},
			{ProjectEncodedDir: "-Users-vladpb-work-Personal-diary2", SessionID: sidD, LastActivity: tD, Path: "/fake/diary2/sess-diary2-d.jsonl"},
			{ProjectEncodedDir: "-Users-vladpb-work-Personal-old", SessionID: sidE, LastActivity: tE, Path: "/fake/old/sess-old-e.jsonl"},
		},
	}

	clk := fakeClock{now: now}
	ls := livesession.New(src, clk).WithGlobalSessions(globalSrc)

	view, err := ls.Snapshot(context.Background(), newProject("/Users/vladpb/work/Personal/clyde"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Usage5h: sessions A (1h) + B (2h) + C (4h) = 1000 + 2000 + 4000 = 7000.
	const want5h = int64(7_000)
	got5h := view.Usage5h.Input
	if got5h != want5h {
		t.Errorf("Usage5h.Input = %d; want %d (sessions A+B+C from 3 projects within 5h)",
			got5h, want5h)
	}

	// UsageWeek: sessions A+B+C+D = 1000+2000+4000+8000 = 15000. E (8d) excluded.
	const wantWeek = int64(15_000)
	gotWeek := view.UsageWeek.Input
	if gotWeek != wantWeek {
		t.Errorf("UsageWeek.Input = %d; want %d (sessions A+B+C+D from 4 projects within 7d)",
			gotWeek, wantWeek)
	}

	// Session counts.
	if view.Sessions5hCount != 3 {
		t.Errorf("Sessions5hCount = %d; want 3", view.Sessions5hCount)
	}
	if view.SessionsWeekCount != 4 {
		t.Errorf("SessionsWeekCount = %d; want 4", view.SessionsWeekCount)
	}
}

// TestSnapshot_CrossProjectUsage_FocusedSessionNotDoubleCounteed verifies that
// the focused session's usage is NOT read twice when it also appears in the
// GlobalSessionSource refs.
func TestSnapshot_CrossProjectUsage_FocusedSessionNotDoubleCounteed(t *testing.T) {
	t.Parallel()

	now := mustTime("2026-04-30T12:00:00Z")

	sidA := session.ID("sess-focused")
	tA := now.Add(-1 * time.Hour)

	mkAssistantWithUsage := func(id string, ts time.Time, sid string, inputTok int64) event.Event {
		ap := event.AssistantPayload{
			Usage: usagePkg.Usage{Input: inputTok},
		}
		return event.NewEvent(id, ts, event.KindAssistant, sid, "", ap)
	}

	callCount := 0
	src := &countingSessionSource{
		fakeSessionSource: fakeSessionSource{
			sessions: []session.Summary{{ID: sidA, LastActivity: tA}},
			events: map[session.ID][]event.Event{
				sidA: {mkAssistantWithUsage("evA", tA, string(sidA), 1_000)},
			},
		},
		onEvents: func(id session.ID) { callCount++ },
	}

	// Global source includes the focused session + one other.
	sidB := session.ID("sess-other")
	tB := now.Add(-2 * time.Hour)
	globalSrc := &fakeGlobalSessionSource{
		refs: []ports.GlobalSessionRef{
			{SessionID: sidA, LastActivity: tA}, // focused — must NOT be re-read
			{SessionID: sidB, LastActivity: tB},
		},
	}
	src.events[sidB] = []event.Event{mkAssistantWithUsage("evB", tB, string(sidB), 2_000)}

	clk := fakeClock{now: now}
	ls := livesession.New(src, clk).WithGlobalSessions(globalSrc)

	view, err := ls.Snapshot(context.Background(), newProject("/projects/focused"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The focused session is read once for its events (the main Snapshot read).
	// The global aggregation must NOT read it again (it reuses base.TotalUsage).
	// So Events() is called exactly once for sidA.
	// sidB is read once for aggregation.
	// Total: 2 calls for Events().
	if callCount != 2 {
		t.Errorf("Events() called %d times; want 2 (once for focused, once for sidB)", callCount)
	}

	// Usage5h: sidA (reused) + sidB = 1000 + 2000 = 3000.
	if view.Usage5h.Input != 3_000 {
		t.Errorf("Usage5h.Input = %d; want 3000", view.Usage5h.Input)
	}
}

// countingSessionSource wraps fakeSessionSource and calls onEvents for each Events() call.
type countingSessionSource struct {
	fakeSessionSource
	onEvents func(id session.ID)
}

func (c *countingSessionSource) Events(ctx context.Context, id session.ID) ([]event.Event, error) {
	c.onEvents(id)
	return c.fakeSessionSource.Events(ctx, id)
}
