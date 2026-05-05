package pricing_test

import (
	"math"
	"testing"

	"github.com/Systemartis/clyde/internal/domain/pricing"
	"github.com/Systemartis/clyde/internal/domain/usage"
)

// approxEqual returns true when a and b differ by less than epsilon.
func approxEqual(a, b, epsilon float64) bool {
	return math.Abs(a-b) < epsilon
}

// TestCost_KnownValues verifies that Cost produces the expected dollar amount
// for a usage with known token counts against the Opus 4.7 model.
//
// Calculation:
//
//	input:          1_000 tokens × $15.00/M  = $0.015000
//	output:           500 tokens × $75.00/M  = $0.037500
//	cache_read:       200 tokens × $ 1.50/M  = $0.000300
//	cache_creation:   100 tokens × $18.75/M  = $0.001875
//	total = $0.054675
func TestCost_KnownValues(t *testing.T) {
	t.Parallel()

	u := usage.Usage{
		Input:         1_000,
		Output:        500,
		CacheRead:     200,
		CacheCreation: 100,
	}

	got := pricing.Cost(u, pricing.Opus47)
	want := 0.054675
	if !approxEqual(got, want, 1e-9) {
		t.Errorf("Cost() = %f; want %f", got, want)
	}
}

// TestCost_ZeroUsage verifies that zero usage always produces $0.00.
func TestCost_ZeroUsage(t *testing.T) {
	t.Parallel()

	got := pricing.Cost(usage.Zero(), pricing.Opus47)
	if got != 0 {
		t.Errorf("Cost(Zero(), Opus47) = %f; want 0", got)
	}
}

// TestCost_Haiku45 spot-checks pricing for the Haiku 4.5 model.
//
// Calculation:
//
//	input:  10_000 tokens × $0.80/M = $0.008000
//	output:  5_000 tokens × $4.00/M = $0.020000
//	total = $0.028000
func TestCost_Haiku45(t *testing.T) {
	t.Parallel()

	u := usage.Usage{Input: 10_000, Output: 5_000}
	got := pricing.Cost(u, pricing.Haiku45)
	want := 0.028
	if !approxEqual(got, want, 1e-9) {
		t.Errorf("Cost(haiku) = %f; want %f", got, want)
	}
}

// TestCompactionPercent_Normal verifies percent calculation within [0,1].
func TestCompactionPercent_Normal(t *testing.T) {
	t.Parallel()

	// 50k input tokens out of 200k limit = 25%.
	u := usage.Usage{Input: 50_000}
	got := pricing.CompactionPercent(u, pricing.Opus47)
	want := 0.25
	if !approxEqual(got, want, 1e-9) {
		t.Errorf("CompactionPercent() = %f; want %f", got, want)
	}
}

// TestCompactionPercent_Clamped verifies the result is clamped to 1.0 when
// token count exceeds context limit.
func TestCompactionPercent_Clamped(t *testing.T) {
	t.Parallel()

	// 300k input tokens — well above 200k limit.
	u := usage.Usage{Input: 300_000}
	got := pricing.CompactionPercent(u, pricing.Opus47)
	if got != 1.0 {
		t.Errorf("CompactionPercent() = %f; want 1.0 (clamped)", got)
	}
}

// TestCompactionPercent_ZeroLimit verifies no divide-by-zero when ContextLimit is 0.
func TestCompactionPercent_ZeroLimit(t *testing.T) {
	t.Parallel()

	zeroModel := pricing.Model{ID: "zero", ContextLimit: 0}
	u := usage.Usage{Input: 5_000}
	got := pricing.CompactionPercent(u, zeroModel)
	if got != 0 {
		t.Errorf("CompactionPercent(zero model) = %f; want 0", got)
	}
}

// TestCompactionPercent_IncludesCacheRead verifies that cache-read tokens
// count toward the compaction percentage (they are part of the context sent to the model).
// CompactionPercent is designed to be called with the LATEST event's usage snapshot.
func TestCompactionPercent_IncludesCacheRead(t *testing.T) {
	t.Parallel()

	// 100k input + 100k cache_read = 200k / 200k = 100%.
	// This represents a single turn where 100k input + 100k from cache = 200k total context.
	u := usage.Usage{Input: 100_000, CacheRead: 100_000}
	got := pricing.CompactionPercent(u, pricing.Opus47)
	if got != 1.0 {
		t.Errorf("CompactionPercent() = %f; want 1.0", got)
	}
}

// TestCompactionPercent_RunningSum verifies the documented anti-pattern:
// a running sum of cache_read across many turns would produce an inflated context size
// that CompactionPercent correctly clamps to 1.0 rather than crashing.
// In practice callers MUST pass the latest event's usage, not a running sum.
func TestCompactionPercent_RunningSum(t *testing.T) {
	t.Parallel()

	// Simulates 100 turns each reading 750k tokens from cache: sum = 75M.
	// This is the anti-pattern; the correct approach is to pass a single turn's usage.
	// CompactionPercent clamps to 1.0 regardless of how large the sum gets.
	u := usage.Usage{Input: 500, CacheRead: 75_000_000}
	got := pricing.CompactionPercent(u, pricing.Opus47)
	if got != 1.0 {
		t.Errorf("CompactionPercent(inflated running sum) = %f; want 1.0 (clamped)", got)
	}
}

// TestLookup_Known verifies that known model IDs return the correct pricing.
func TestLookup_Known(t *testing.T) {
	t.Parallel()

	m := pricing.Lookup("claude-opus-4-7")
	if m.ID != pricing.Opus47.ID {
		t.Errorf("Lookup(opus-4-7).ID = %q; want %q", m.ID, pricing.Opus47.ID)
	}
	if m.InputPerMillion != pricing.Opus47.InputPerMillion {
		t.Errorf("Lookup(opus-4-7).InputPerMillion = %f; want %f",
			m.InputPerMillion, pricing.Opus47.InputPerMillion)
	}
}

// TestLookup_Unknown verifies that an unknown model ID returns a non-zero fallback.
func TestLookup_Unknown(t *testing.T) {
	t.Parallel()

	m := pricing.Lookup("claude-unknown-99")
	// Should not return a zero-cost model — that would hide pricing info.
	if m.InputPerMillion == 0 && m.OutputPerMillion == 0 {
		t.Error("Lookup(unknown) returned zero-cost model; want a non-zero fallback")
	}
}

// TestLookup_AllModels verifies every predefined model is retrievable by its ID.
func TestLookup_AllModels(t *testing.T) {
	t.Parallel()

	models := []pricing.Model{
		pricing.Opus47,
		pricing.Sonnet46,
		pricing.Sonnet45,
		pricing.Haiku45,
		pricing.Opus45,
	}
	for _, want := range models {
		got := pricing.Lookup(want.ID)
		if got.ID != want.ID {
			t.Errorf("Lookup(%q).ID = %q; want %q", want.ID, got.ID, want.ID)
		}
	}
}

// TestLookup_1MContextSuffix verifies that model IDs with the [1m] suffix
// return a 1_000_000 context limit and doubled per-million prices.
func TestLookup_1MContextSuffix(t *testing.T) {
	t.Parallel()

	m := pricing.Lookup("claude-opus-4-7[1m]")
	if m.ContextLimit != 1_000_000 {
		t.Errorf("Lookup(opus-4-7[1m]).ContextLimit = %d; want 1_000_000", m.ContextLimit)
	}
	// Prices should be 2× the base Opus47 prices.
	wantInput := pricing.Opus47.InputPerMillion * 2
	if !approxEqual(m.InputPerMillion, wantInput, 1e-9) {
		t.Errorf("Lookup(opus-4-7[1m]).InputPerMillion = %f; want %f (2× base)", m.InputPerMillion, wantInput)
	}
	wantOutput := pricing.Opus47.OutputPerMillion * 2
	if !approxEqual(m.OutputPerMillion, wantOutput, 1e-9) {
		t.Errorf("Lookup(opus-4-7[1m]).OutputPerMillion = %f; want %f (2× base)", m.OutputPerMillion, wantOutput)
	}
}

// TestLookup_1MContextSuffix_CompactionThreshold verifies that compaction
// percent is computed against 1_000_000 for the 1M variant, not 200_000.
func TestLookup_1MContextSuffix_CompactionThreshold(t *testing.T) {
	t.Parallel()

	m := pricing.Lookup("claude-opus-4-7[1m]")
	// 750k input tokens should be 75% of 1M, not over 100% of 200k.
	u := usage.Usage{Input: 750_000}
	got := pricing.CompactionPercent(u, m)
	want := 0.75
	if !approxEqual(got, want, 1e-9) {
		t.Errorf("CompactionPercent(750k, 1M variant) = %f; want %f", got, want)
	}
}

// TestLookup_1MContextSuffix_BaseIDPreserved verifies ID field contains the
// full [1m] suffix so callers can identify the variant.
func TestLookup_1MContextSuffix_BaseIDPreserved(t *testing.T) {
	t.Parallel()

	m := pricing.Lookup("claude-opus-4-7[1m]")
	if m.ID != "claude-opus-4-7[1m]" {
		t.Errorf("Lookup(opus-4-7[1m]).ID = %q; want %q", m.ID, "claude-opus-4-7[1m]")
	}
}

// TestLookup_BaseModelUnchanged verifies that looking up the 1M variant does
// not modify the base Opus47 model in the pricing table.
func TestLookup_BaseModelUnchanged(t *testing.T) {
	t.Parallel()

	// Look up 1M variant first.
	_ = pricing.Lookup("claude-opus-4-7[1m]")
	// Base model must still have original prices.
	base := pricing.Lookup("claude-opus-4-7")
	if base.ContextLimit != 200_000 {
		t.Errorf("Opus47 ContextLimit mutated: got %d; want 200_000", base.ContextLimit)
	}
	if !approxEqual(base.InputPerMillion, pricing.Opus47.InputPerMillion, 1e-9) {
		t.Errorf("Opus47 InputPerMillion mutated: got %f", base.InputPerMillion)
	}
}

// TestLookup_RealJSONLModelStrings verifies that the model strings actually
// observed in Claude Code JSONL files are correctly resolved.
//
// Regression test for Bug 1: the JSONL message.model field is "claude-opus-4-7"
// (no suffix) for the base model, and the synthesized 1M variant uses the
// "[1m]" suffix that livesession appends when it detects 1h-cache usage.
func TestLookup_RealJSONLModelStrings(t *testing.T) {
	t.Parallel()

	// "claude-opus-4-7" is the EXACT string written to the JSONL message.model
	// field by Claude Code (verified from ~/.claude/projects/*/*.jsonl).
	baseID := "claude-opus-4-7"
	base := pricing.Lookup(baseID)
	if base.ContextLimit != 200_000 {
		t.Errorf("Lookup(%q).ContextLimit = %d; want 200_000", baseID, base.ContextLimit)
	}
	if base.InputPerMillion != 15.00 {
		t.Errorf("Lookup(%q).InputPerMillion = %f; want 15.00", baseID, base.InputPerMillion)
	}

	// "claude-opus-4-7[1m]" is the synthesized ID produced by livesession when
	// ephemeral_1h cache tokens are detected (Max plan / 1M context indicator).
	oneM := pricing.Lookup("claude-opus-4-7[1m]")
	if oneM.ContextLimit != 1_000_000 {
		t.Errorf("Lookup(opus-4-7[1m]).ContextLimit = %d; want 1_000_000", oneM.ContextLimit)
	}
	if !approxEqual(oneM.InputPerMillion, 30.00, 1e-9) {
		t.Errorf("Lookup(opus-4-7[1m]).InputPerMillion = %f; want 30.00 (2× base)", oneM.InputPerMillion)
	}
}

// TestTotalTokens verifies the billed-activity token sum: input + output + cache_creation.
// cache_read is intentionally excluded because it accumulates the same cached
// context once per turn, producing impossibly high sums across a long session.
func TestTotalTokens(t *testing.T) {
	t.Parallel()

	u := usage.Usage{Input: 10, Output: 20, CacheRead: 5, CacheCreation: 3}
	got := pricing.TotalTokens(u)
	// 10 + 20 + 3 = 33 (cache_read=5 is excluded from the display total).
	want := int64(33)
	if got != want {
		t.Errorf("TotalTokens() = %d; want %d", got, want)
	}
}

// TestContextTokens verifies the full per-turn context size: input + cache_read + cache_creation.
// This is used for compaction percentage and reflects all tokens sent to the model in one call.
func TestContextTokens(t *testing.T) {
	t.Parallel()

	u := usage.Usage{Input: 10, Output: 20, CacheRead: 5, CacheCreation: 3}
	got := pricing.ContextTokens(u)
	// 10 + 5 + 3 = 18 (output excluded — output doesn't fill the context window going in).
	want := int64(18)
	if got != want {
		t.Errorf("ContextTokens() = %d; want %d", got, want)
	}
}

// TestTotalTokens_NoCacheRead verifies that a session with zero cache_read tokens
// is unaffected by the cache_read exclusion.
func TestTotalTokens_NoCacheRead(t *testing.T) {
	t.Parallel()

	u := usage.Usage{Input: 100, Output: 200, CacheRead: 0, CacheCreation: 50}
	got := pricing.TotalTokens(u)
	want := int64(350) // 100 + 200 + 50
	if got != want {
		t.Errorf("TotalTokens(no cache read) = %d; want %d", got, want)
	}
}
