// Package session_test contains table-driven tests for the Session domain types.
package session_test

import (
	"sort"
	"testing"
	"time"

	"github.com/Systemartis/clyde/internal/domain/session"
)

// TestSessionIDIsString verifies that session.ID is a string-based type and
// that its underlying string value is directly accessible via type conversion.
func TestSessionIDIsString(t *testing.T) {
	t.Parallel()

	id := session.ID("test-session-uuid-abc-123")
	if string(id) != "test-session-uuid-abc-123" {
		t.Errorf("session.ID string value = %q, want %q", string(id), "test-session-uuid-abc-123")
	}
}

// TestSummaryOrderByLastActivity verifies that a slice of Summary values can be
// sorted descending by LastActivity (most-recently-active session first).
// This is the ordering required by SessionSource.Sessions and WatchSession.
func TestSummaryOrderByLastActivity(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 4, 30, 10, 0, 0, 0, time.UTC)
	t1 := base
	t2 := base.Add(time.Hour)
	t3 := base.Add(2 * time.Hour)

	cases := []struct {
		name  string
		input []session.Summary
		want  []session.ID // expected IDs in descending order after sort
	}{
		{
			name: "three summaries already unsorted",
			input: []session.Summary{
				{ID: "sess-a", LastActivity: t1},
				{ID: "sess-b", LastActivity: t3},
				{ID: "sess-c", LastActivity: t2},
			},
			want: []session.ID{"sess-b", "sess-c", "sess-a"},
		},
		{
			name: "three summaries already sorted descending",
			input: []session.Summary{
				{ID: "sess-x", LastActivity: t3},
				{ID: "sess-y", LastActivity: t2},
				{ID: "sess-z", LastActivity: t1},
			},
			want: []session.ID{"sess-x", "sess-y", "sess-z"},
		},
		{
			name: "single summary — trivially sorted",
			input: []session.Summary{
				{ID: "sess-only", LastActivity: t2},
			},
			want: []session.ID{"sess-only"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			summaries := make([]session.Summary, len(tc.input))
			copy(summaries, tc.input)

			// Sort descending by LastActivity (most recent first).
			sort.Slice(summaries, func(i, j int) bool {
				return summaries[i].LastActivity.After(summaries[j].LastActivity)
			})

			if len(summaries) != len(tc.want) {
				t.Fatalf("sorted len = %d, want %d", len(summaries), len(tc.want))
			}
			for i, gotSummary := range summaries {
				if gotSummary.ID != tc.want[i] {
					t.Errorf("position %d: ID = %q, want %q", i, gotSummary.ID, tc.want[i])
				}
			}
		})
	}
}
