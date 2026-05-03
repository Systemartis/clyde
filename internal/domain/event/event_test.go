// Package event_test contains table-driven tests for the Event domain type.
package event_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/clyde-tui/clyde/internal/domain/event"
	"github.com/clyde-tui/clyde/internal/domain/usage"
)

// --- Kind enum tests --------------------------------------------------------

func TestKindString(t *testing.T) {
	t.Parallel()

	cases := []struct {
		kind event.Kind
		want string
	}{
		{event.KindUser, "user"},
		{event.KindAssistant, "assistant"},
		{event.KindOpaque, "opaque"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(string(tc.kind), func(t *testing.T) {
			t.Parallel()
			if got := string(tc.kind); got != tc.want {
				t.Errorf("Kind(%q) = %q, want %q", tc.kind, got, tc.want)
			}
		})
	}
}

// --- OpaquePayload tests ----------------------------------------------------

func TestOpaquePayloadPreservesRaw(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"type":"ai-title","title":"session start"}`)
	p := event.OpaquePayload{Raw: raw}

	if string(p.Raw) != string(raw) {
		t.Errorf("OpaquePayload.Raw = %q, want %q", p.Raw, raw)
	}
}

// --- Event construction tests -----------------------------------------------

func TestNewEvent(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 4, 30, 10, 0, 0, 0, time.UTC)
	sessionID := "sess-abc-123"
	parentID := "parent-uuid-456"

	cases := []struct {
		name      string
		id        string
		ts        time.Time
		kind      event.Kind
		sessionID string
		parentID  string
		payload   event.Payload
		wantKind  event.Kind
	}{
		{
			name:      "user event with parent",
			id:        "evt-001",
			ts:        ts,
			kind:      event.KindUser,
			sessionID: sessionID,
			parentID:  parentID,
			payload:   event.UserPayload{},
			wantKind:  event.KindUser,
		},
		{
			name:      "assistant event with usage",
			id:        "evt-002",
			ts:        ts.Add(time.Minute),
			kind:      event.KindAssistant,
			sessionID: sessionID,
			parentID:  "evt-001",
			payload:   event.AssistantPayload{Usage: usage.Usage{Input: 10, Output: 200}},
			wantKind:  event.KindAssistant,
		},
		{
			name:      "root event — empty parentID",
			id:        "evt-root",
			ts:        ts,
			kind:      event.KindUser,
			sessionID: sessionID,
			parentID:  "",
			payload:   event.UserPayload{},
			wantKind:  event.KindUser,
		},
		{
			name:      "opaque event preserves kind",
			id:        "evt-opaque",
			ts:        ts,
			kind:      event.Kind("ai-title"),
			sessionID: sessionID,
			parentID:  parentID,
			payload:   event.OpaquePayload{Raw: []byte(`{"type":"ai-title"}`)},
			wantKind:  event.Kind("ai-title"),
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ev := event.NewEvent(tc.id, tc.ts, tc.kind, tc.sessionID, tc.parentID, tc.payload)

			if ev.ID != tc.id {
				t.Errorf("ID = %q, want %q", ev.ID, tc.id)
			}
			if !ev.Timestamp.Equal(tc.ts) {
				t.Errorf("Timestamp = %v, want %v", ev.Timestamp, tc.ts)
			}
			if ev.Kind != tc.wantKind {
				t.Errorf("Kind = %q, want %q", ev.Kind, tc.wantKind)
			}
			if ev.SessionID != tc.sessionID {
				t.Errorf("SessionID = %q, want %q", ev.SessionID, tc.sessionID)
			}
			if ev.ParentID != tc.parentID {
				t.Errorf("ParentID = %q, want %q", ev.ParentID, tc.parentID)
			}
			// OpaquePayload contains []byte and UserPayload contains []string —
			// neither is comparable with ==. Switch on concrete types.
			switch want := tc.payload.(type) {
			case event.OpaquePayload:
				got, ok := ev.Payload.(event.OpaquePayload)
				if !ok {
					t.Errorf("Payload type = %T, want OpaquePayload", ev.Payload)
				} else if !bytes.Equal(got.Raw, want.Raw) {
					t.Errorf("OpaquePayload.Raw = %q, want %q", got.Raw, want.Raw)
				}
			case event.UserPayload:
				got, ok := ev.Payload.(event.UserPayload)
				if !ok {
					t.Errorf("Payload type = %T, want UserPayload", ev.Payload)
				} else if got.IsMeta != want.IsMeta ||
					got.IsToolResultOnly != want.IsToolResultOnly ||
					got.Summary != want.Summary {
					// Compare non-slice fields directly; slice fields default to nil.
					t.Errorf("UserPayload = %+v, want %+v", got, want)
				}
			case event.AssistantPayload:
				got, ok := ev.Payload.(event.AssistantPayload)
				if !ok {
					t.Errorf("Payload type = %T, want AssistantPayload", ev.Payload)
				} else if got.Usage != want.Usage || got.Summary != want.Summary || got.Model != want.Model {
					t.Errorf("AssistantPayload = %+v, want %+v", got, want)
				}
			}
		})
	}
}

// TestAssistantPayloadCarriesUsage verifies that AssistantPayload exposes Usage.
func TestAssistantPayloadCarriesUsage(t *testing.T) {
	t.Parallel()

	u := usage.Usage{Input: 3, Output: 4, CacheRead: 1, CacheCreation: 2}
	p := event.AssistantPayload{Usage: u}

	if p.Usage != u {
		t.Errorf("AssistantPayload.Usage = %+v, want %+v", p.Usage, u)
	}
}
