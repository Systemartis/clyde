package livesession

import (
	"testing"
	"time"
)

func mustT(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return ts
}

// TestNextResetAfter verifies the block-tiling reset computation: windows
// anchor at the first message (truncated to the hour, matching Anthropic's
// behavior) and tile forward until the reset lands strictly after now.
func TestNextResetAfter(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		anchor string
		now    string
		window time.Duration
		want   string
	}{
		{
			name:   "mid-first-block: anchor truncates to the hour",
			anchor: "2026-04-30T09:23:00Z",
			now:    "2026-04-30T12:00:00Z",
			window: 5 * time.Hour,
			want:   "2026-04-30T14:00:00Z",
		},
		{
			name:   "third block: tiles past elapsed blocks",
			anchor: "2026-04-30T02:00:00Z",
			now:    "2026-04-30T12:30:00Z",
			window: 5 * time.Hour,
			want:   "2026-04-30T17:00:00Z",
		},
		{
			name:   "exactly on a block boundary starts the next block",
			anchor: "2026-04-30T02:00:00Z",
			now:    "2026-04-30T07:00:00Z",
			window: 5 * time.Hour,
			want:   "2026-04-30T12:00:00Z",
		},
		{
			name:   "weekly window",
			anchor: "2026-04-28T09:45:00Z",
			now:    "2026-04-30T12:00:00Z",
			window: 7 * 24 * time.Hour,
			want:   "2026-05-05T09:00:00Z",
		},
		{
			name:   "now before anchor (clock skew) returns first reset",
			anchor: "2026-04-30T13:10:00Z",
			now:    "2026-04-30T12:00:00Z",
			window: 5 * time.Hour,
			want:   "2026-04-30T18:00:00Z",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := nextResetAfter(mustT(t, tc.anchor), mustT(t, tc.now), tc.window)
			if !got.Equal(mustT(t, tc.want)) {
				t.Errorf("nextResetAfter(%s, %s, %v) = %v, want %s", tc.anchor, tc.now, tc.window, got, tc.want)
			}
		})
	}
}

// TestNextResetAfter_ZeroAnchor verifies the zero-value passthrough.
func TestNextResetAfter_ZeroAnchor(t *testing.T) {
	t.Parallel()
	if got := nextResetAfter(time.Time{}, mustT(t, "2026-04-30T12:00:00Z"), 5*time.Hour); !got.IsZero() {
		t.Errorf("zero anchor must produce zero reset, got %v", got)
	}
}
