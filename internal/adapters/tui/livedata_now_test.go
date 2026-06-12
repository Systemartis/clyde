package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/Systemartis/clyde/internal/application/livesession"
	"github.com/Systemartis/clyde/internal/domain/event"
	"github.com/Systemartis/clyde/internal/domain/usage"
)

// TestNowStatus_ThinkingHasNoDuplicatedFields is the regression for the
// "thinking is written 3 times" complaint: the panel border header
// (ModeText), body line 1 (Op), and body line 2 (Meta) all displayed
// identical "thinking · 47 t/s" strings. The fix moves Meta to a
// distinct value (elapsed) so the panel stops shouting itself.
func TestNowStatus_ThinkingHasNoDuplicatedFields(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	thinkingStarted := now.Add(-7 * time.Second)

	v := livesession.View{
		Events: []event.Event{
			event.NewEvent("ev1", thinkingStarted, event.KindAssistant, "sid", "", event.AssistantPayload{
				Summary: "(thinking)",
				Usage:   usage.Usage{Output: 320},
			}),
		},
		LastUpdate: now,
	}

	got := deriveNowStatus(v)

	if got.Op == "" {
		t.Fatal("Op must be set for thinking state")
	}
	if got.Meta == "" {
		t.Fatal("Meta must be set for thinking state")
	}
	if got.ModeText == "" {
		t.Fatal("ModeText must be set for thinking state")
	}
	if got.Meta == got.ModeText {
		t.Errorf("Meta == ModeText (%q) — body line 2 duplicates the header", got.Meta)
	}
}

// TestNowStatus_WritingHasNoDuplicatedFields mirrors the thinking case for
// the "writing response" turn.
func TestNowStatus_WritingHasNoDuplicatedFields(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	writingStarted := now.Add(-3 * time.Second)

	v := livesession.View{
		Events: []event.Event{
			event.NewEvent("ev1", writingStarted, event.KindAssistant, "sid", "", event.AssistantPayload{
				Summary: "drafting reply",
				Usage:   usage.Usage{Output: 120},
			}),
		},
		LastUpdate: now,
	}

	got := deriveNowStatus(v)

	if got.Meta == "" {
		t.Fatal("Meta must be set for writing state")
	}
	if got.ModeText == "" {
		t.Fatal("ModeText must be set for writing state")
	}
	if got.Meta == got.ModeText {
		t.Errorf("Meta == ModeText (%q) — body line 2 duplicates the header", got.Meta)
	}
}

// TestUsageSession_CtxLineDropsRedundantPercent verifies that the session
// ctx sub-line stops echoing the percentage that's already shown in the
// row's headline value. Prior format: "47k / 200k (23%)" — three places
// shouting "23%". New format: "47k / 200k".
func TestUsageSession_CtxLineDropsRedundantPercent(t *testing.T) {
	t.Parallel()

	v := livesession.View{
		Events: []event.Event{
			event.NewEvent("ev1", time.Now().UTC(), event.KindAssistant, "sid", "", event.AssistantPayload{
				Usage: usage.Usage{Input: 50_000, Output: 10_000, CacheRead: 30_000},
				Model: "claude-opus-4-7",
			}),
		},
		TotalUsage:   usage.Usage{Input: 50_000, Output: 10_000, CacheRead: 30_000},
		LatestUsage:  usage.Usage{Input: 50_000, Output: 10_000, CacheRead: 30_000},
		CurrentModel: "claude-opus-4-7",
	}

	d := deriveUsageFields(v, MockData{Model: "opus 4.7"})

	if d.UsageSession.CurrentCtx == "" {
		t.Fatal("CurrentCtx empty")
	}
	if containsAny(d.UsageSession.CurrentCtx, "(", ")", "%") {
		t.Errorf("CurrentCtx %q still encodes the percent — must show only tokens / limit", d.UsageSession.CurrentCtx)
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
	}
	return false
}

// TestNowStatus_StaleWritingTransitionsToAwaiting locks the freshness
// window: an assistant text event older than assistantFreshnessWindow
// should NOT keep the status pinned to "writing response". The user
// reported the panel staying on "writing response 22s" long after
// claude finished. After the freshness cutoff we should call it
// "awaiting" so the panel reflects reality.
func TestNowStatus_StaleWritingTransitionsToAwaiting(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	wroteAt := now.Add(-15 * time.Second) // safely past freshness, before idle

	v := livesession.View{
		Events: []event.Event{
			event.NewEvent("ev1", wroteAt, event.KindAssistant, "sid", "", event.AssistantPayload{
				Summary: "drafting reply",
				Usage:   usage.Usage{Output: 800},
			}),
		},
		LastUpdate: now,
	}

	got := deriveNowStatus(v)

	if strings.Contains(strings.ToLower(got.Op), "writing") {
		t.Errorf("Op = %q — stale text event should not still claim 'writing'", got.Op)
	}
	if strings.Contains(strings.ToLower(got.ModeText), "writing") {
		t.Errorf("ModeText = %q — stale text event should not still claim 'writing'", got.ModeText)
	}
}

// TestNowStatus_FreshWritingStaysWriting guards the other side of the
// freshness window: a text event seen seconds ago must still read as
// "writing" because claude is plausibly still streaming.
func TestNowStatus_FreshWritingStaysWriting(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	wroteAt := now.Add(-1 * time.Second)

	v := livesession.View{
		Events: []event.Event{
			event.NewEvent("ev1", wroteAt, event.KindAssistant, "sid", "", event.AssistantPayload{
				Summary: "drafting reply",
				Usage:   usage.Usage{Output: 800},
			}),
		},
		LastUpdate: now,
	}

	got := deriveNowStatus(v)
	if !strings.Contains(strings.ToLower(got.ModeText), "writing") {
		t.Errorf("ModeText = %q — fresh text event should read as 'writing'", got.ModeText)
	}
}

// TestNowStatus_IdleCounterResetsFromThreshold verifies that idle Meta
// shows duration since idle began (age - threshold), not raw event age.
// User reported the counter staying at the writing-elapsed value when
// transitioning into idle — they expected a fresh state-relative
// counter.
func TestNowStatus_IdleCounterResetsFromThreshold(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	// 90 seconds since last event = 60 seconds into idle (threshold 30s).
	lastEvt := now.Add(-90 * time.Second)

	v := livesession.View{
		Events: []event.Event{
			event.NewEvent("ev1", lastEvt, event.KindAssistant, "sid", "", event.AssistantPayload{
				Summary: "drafting reply",
				Usage:   usage.Usage{Output: 800},
			}),
		},
		LastUpdate: now,
	}

	got := deriveNowStatus(v)
	if got.Op != "idle" {
		t.Fatalf("expected idle Op, got %q", got.Op)
	}
	// 90s - 30s threshold = 60s = "1m"
	if !strings.Contains(got.Meta, "1m") {
		t.Errorf("idle Meta = %q — want a counter starting from threshold (~1m), not raw age", got.Meta)
	}
	// Raw 90s would format as "1m 30s"; we don't want to see the 30s.
	if strings.Contains(got.Meta, "30") {
		t.Errorf("idle Meta = %q — appears to encode raw age (90s) instead of idle-since-threshold (60s)", got.Meta)
	}
}
