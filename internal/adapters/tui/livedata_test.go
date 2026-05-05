package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/Systemartis/clyde/internal/application/livesession"
	"github.com/Systemartis/clyde/internal/domain/event"
	"github.com/Systemartis/clyde/internal/domain/usage"
)

// ─── TestUsageDisplaySeparatesSessionAndContext ───────────────────────────────

// TestUsageDisplaySeparatesSessionAndContext verifies that the usage panel
// labels clearly distinguish:
//   - "total used" = session running sum (TotalUsage)
//   - "current ctx" = latest turn's context vs model limit (LatestUsage)
//
// This is the regression test for the display ambiguity reported in v20:
// "3394.8k = 3.4M; 76% would be 760k; they don't match."
func TestUsageDisplaySeparatesSessionAndContext(t *testing.T) {
	t.Parallel()

	// Set up a view that has an inflated TotalUsage (many turns of cache read)
	// but a modest LatestUsage (single turn context).
	totalUsage := usage.Usage{
		Input:         100_000,
		Output:        20_000,
		CacheRead:     3_200_000, // massive inflated sum across many turns
		CacheCreation: 50_000,
	}
	// LatestUsage is what the LAST API call actually sent to the model.
	latestUsage := usage.Usage{
		Input:         400_000,
		Output:        20_000,
		CacheRead:     340_000, // last-turn cache read only
		CacheCreation: 50_000,
	}

	v := livesession.View{
		Events: []event.Event{
			event.NewEvent("ev1", time.Now().UTC(), event.KindAssistant, "sid", "", event.AssistantPayload{
				Usage: totalUsage,
				Model: "claude-opus-4-7",
			}),
		},
		TotalUsage:     totalUsage,
		LatestUsage:    latestUsage,
		CurrentModel:   "claude-opus-4-7",
		AssistantTurns: 1,
	}

	// Start from empty MockData and derive fields.
	d := deriveUsageFields(v, MockData{Model: "opus 4.7"})

	// 1. UsageSession row must be labeled "session ctx".
	if d.UsageSession.Label != "session ctx" {
		t.Errorf("UsageSession.Label = %q, want %q", d.UsageSession.Label, "session ctx")
	}

	// 2. TotalUsed must reflect TotalUsage (input+output+cache_creation, no cache_read).
	if d.UsageSession.TotalUsed == "" {
		t.Error("UsageSession.TotalUsed is empty — must be populated")
	}

	// 3. CurrentCtx must be non-empty and represent LatestUsage context.
	if d.UsageSession.CurrentCtx == "" {
		t.Error("UsageSession.CurrentCtx is empty — must show latest-turn context vs limit")
	}

	// 4. The two values should be different: TotalUsed is NOT "current ctx".
	// If they were the same it would be the old ambiguous single-number display.
	if d.UsageSession.TotalUsed == d.UsageSession.CurrentCtx {
		t.Errorf("TotalUsed (%q) == CurrentCtx (%q) — they should differ",
			d.UsageSession.TotalUsed, d.UsageSession.CurrentCtx)
	}

	// 5. CurrentCtx must contain "/" (e.g. "760k / 200k (38%)").
	if !strings.Contains(d.UsageSession.CurrentCtx, "/") {
		t.Errorf("CurrentCtx %q missing '/' separator — expected 'Nk / 200k (N%%)'", d.UsageSession.CurrentCtx)
	}
}

// ─── TestReset5hCountdown ─────────────────────────────────────────────────────

// TestReset5hCountdown verifies that formatResetsIn produces correctly formatted
// countdown strings for the 5h and 7d rolling windows.
func TestReset5hCountdown(t *testing.T) {
	t.Parallel()

	// Anchor time for all sub-tests.
	now := time.Date(2026, 4, 30, 14, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		resetAt  time.Time
		useDays  bool
		wantLike string // expected format pattern
		wantNot  string // must NOT appear
	}{
		{
			name:     "5h window 3h57m remaining",
			resetAt:  now.Add(3*time.Hour + 57*time.Minute),
			useDays:  false,
			wantLike: "3h 57m",
		},
		{
			name:     "5h window only minutes left",
			resetAt:  now.Add(23 * time.Minute),
			useDays:  false,
			wantLike: "23m",
		},
		{
			name:     "5h window only hours left (exact)",
			resetAt:  now.Add(2 * time.Hour),
			useDays:  false,
			wantLike: "2h",
		},
		{
			name:     "7d window 4d14h remaining",
			resetAt:  now.Add(4*24*time.Hour + 14*time.Hour),
			useDays:  true,
			wantLike: "4d 14h",
		},
		{
			name:     "7d window only days left (exact)",
			resetAt:  now.Add(3 * 24 * time.Hour),
			useDays:  true,
			wantLike: "3d",
		},
		{
			name:     "already expired returns empty",
			resetAt:  now.Add(-1 * time.Minute),
			useDays:  false,
			wantLike: "",
		},
		{
			name:     "zero time returns empty",
			resetAt:  time.Time{},
			useDays:  false,
			wantLike: "",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := formatResetsIn(tc.resetAt, now, tc.useDays)
			if got != tc.wantLike {
				t.Errorf("formatResetsIn = %q, want %q", got, tc.wantLike)
			}
		})
	}
}
