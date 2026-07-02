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
// Calculation (Opus tier — $5/$25 since Opus 4.5):
//
//	input:          1_000 tokens × $5.00/M  = $0.005000
//	output:           500 tokens × $25.00/M = $0.012500
//	cache_read:       200 tokens × $0.50/M  = $0.000100
//	cache_creation:   100 tokens × $6.25/M  = $0.000625
//	total = $0.018225
func TestCost_KnownValues(t *testing.T) {
	t.Parallel()

	u := usage.Usage{
		Input:         1_000,
		Output:        500,
		CacheRead:     200,
		CacheCreation: 100,
	}

	got := pricing.Cost(u, pricing.Opus47)
	want := 0.018225
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
// Calculation ($1/$5 per MTok):
//
//	input:  10_000 tokens × $1.00/M = $0.010000
//	output:  5_000 tokens × $5.00/M = $0.025000
//	total = $0.035000
func TestCost_Haiku45(t *testing.T) {
	t.Parallel()

	u := usage.Usage{Input: 10_000, Output: 5_000}
	got := pricing.Cost(u, pricing.Haiku45)
	want := 0.035
	if !approxEqual(got, want, 1e-9) {
		t.Errorf("Cost(haiku) = %f; want %f", got, want)
	}
}

// TestCost_Fable5 spot-checks pricing for the Fable 5 top tier.
//
// Calculation ($10/$50 per MTok):
//
//	input:  10_000 tokens × $10.00/M = $0.100000
//	output:  5_000 tokens × $50.00/M = $0.250000
//	total = $0.350000
func TestCost_Fable5(t *testing.T) {
	t.Parallel()

	u := usage.Usage{Input: 10_000, Output: 5_000}
	got := pricing.Cost(u, pricing.Fable5)
	want := 0.35
	if !approxEqual(got, want, 1e-9) {
		t.Errorf("Cost(fable) = %f; want %f", got, want)
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

// TestLookup_CurrentGenerationRates verifies the table matches Anthropic's
// published per-MTok rates (verified 2026-07-02): Opus tier $5/$25 since
// Opus 4.5, Sonnet tier $3/$15, Haiku 4.5 $1/$5, Fable 5 $10/$50.
func TestLookup_CurrentGenerationRates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		id      string
		wantIn  float64
		wantOut float64
	}{
		{"claude-fable-5", 10.00, 50.00},
		{"claude-opus-4-8", 5.00, 25.00},
		{"claude-opus-4-7", 5.00, 25.00},
		{"claude-opus-4-5", 5.00, 25.00},
		{"claude-sonnet-5", 3.00, 15.00},
		{"claude-sonnet-4-6", 3.00, 15.00},
		{"claude-haiku-4-5", 1.00, 5.00},
	}
	for _, tt := range tests {
		m := pricing.Lookup(tt.id)
		if !approxEqual(m.InputPerMillion, tt.wantIn, 1e-9) || !approxEqual(m.OutputPerMillion, tt.wantOut, 1e-9) {
			t.Errorf("Lookup(%q) = $%.2f/$%.2f per M; want $%.2f/$%.2f",
				tt.id, m.InputPerMillion, m.OutputPerMillion, tt.wantIn, tt.wantOut)
		}
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
		pricing.Fable5,
		pricing.Opus48,
		pricing.Opus47,
		pricing.Sonnet5,
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
// return a 1_000_000 context limit at UNCHANGED per-token rates for models
// where the 1M window is standard pricing (all Opus 4.6+ era models — see
// the migration guide: "1M context window at standard API pricing with no
// long-context premium").
func TestLookup_1MContextSuffix(t *testing.T) {
	t.Parallel()

	m := pricing.Lookup("claude-opus-4-7[1m]")
	if m.ContextLimit != 1_000_000 {
		t.Errorf("Lookup(opus-4-7[1m]).ContextLimit = %d; want 1_000_000", m.ContextLimit)
	}
	if !approxEqual(m.InputPerMillion, pricing.Opus47.InputPerMillion, 1e-9) {
		t.Errorf("Lookup(opus-4-7[1m]).InputPerMillion = %f; want %f (standard pricing, no 1M premium)",
			m.InputPerMillion, pricing.Opus47.InputPerMillion)
	}
	if !approxEqual(m.OutputPerMillion, pricing.Opus47.OutputPerMillion, 1e-9) {
		t.Errorf("Lookup(opus-4-7[1m]).OutputPerMillion = %f; want %f (standard pricing, no 1M premium)",
			m.OutputPerMillion, pricing.Opus47.OutputPerMillion)
	}
}

// TestLookup_1MPremium_Sonnet45 verifies that the legacy long-context beta
// premium still applies to Sonnet 4.5, the one table model whose 1M window
// was billed at a premium (2× input, 1.5× output above the 200K threshold —
// approximated flat here since per-request tiering isn't visible in JSONL).
func TestLookup_1MPremium_Sonnet45(t *testing.T) {
	t.Parallel()

	m := pricing.Lookup("claude-sonnet-4-5[1m]")
	if m.ContextLimit != 1_000_000 {
		t.Errorf("Lookup(sonnet-4-5[1m]).ContextLimit = %d; want 1_000_000", m.ContextLimit)
	}
	wantIn := pricing.Sonnet45.InputPerMillion * 2
	wantOut := pricing.Sonnet45.OutputPerMillion * 1.5
	if !approxEqual(m.InputPerMillion, wantIn, 1e-9) {
		t.Errorf("Lookup(sonnet-4-5[1m]).InputPerMillion = %f; want %f (2× long-context premium)",
			m.InputPerMillion, wantIn)
	}
	if !approxEqual(m.OutputPerMillion, wantOut, 1e-9) {
		t.Errorf("Lookup(sonnet-4-5[1m]).OutputPerMillion = %f; want %f (1.5× long-context premium)",
			m.OutputPerMillion, wantOut)
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
	if base.InputPerMillion != 5.00 {
		t.Errorf("Lookup(%q).InputPerMillion = %f; want 5.00", baseID, base.InputPerMillion)
	}

	// "claude-opus-4-7[1m]" is the synthesized ID produced by livesession when
	// ephemeral_1h cache tokens are detected (Max plan / 1M context indicator).
	oneM := pricing.Lookup("claude-opus-4-7[1m]")
	if oneM.ContextLimit != 1_000_000 {
		t.Errorf("Lookup(opus-4-7[1m]).ContextLimit = %d; want 1_000_000", oneM.ContextLimit)
	}
	if !approxEqual(oneM.InputPerMillion, 5.00, 1e-9) {
		t.Errorf("Lookup(opus-4-7[1m]).InputPerMillion = %f; want 5.00 (standard pricing at 1M)", oneM.InputPerMillion)
	}
}

// TestLookup_NewMinorVersion_FamilyFallback verifies that model IDs for newer
// versions of a KNOWN family (which have no exact entry in the pricing table)
// resolve to that family's closest known pricing instead of falling through to
// the generic mid-tier "unknown" fallback.
//
// Regression for the reversed prefix-match bug: the old lookup only matched
// when a known ID was a literal prefix of the incoming ID, so date-suffixed
// KNOWN versions resolved but NEWER minor/major versions silently landed on
// unknown ($3/$15) mid-tier pricing.
func TestLookup_NewMinorVersion_FamilyFallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		id      string
		wantIn  float64
		wantOut float64
	}{
		{"opus-4-9 hypothetical → opus tier", "claude-opus-4-9", pricing.Opus48.InputPerMillion, pricing.Opus48.OutputPerMillion},
		{"opus-4-8 dated → opus tier", "claude-opus-4-8-20260115", pricing.Opus48.InputPerMillion, pricing.Opus48.OutputPerMillion},
		{"sonnet-6 hypothetical → sonnet tier", "claude-sonnet-6", pricing.Sonnet5.InputPerMillion, pricing.Sonnet5.OutputPerMillion},
		{"haiku-4-5 dated → haiku tier", "claude-haiku-4-5-20251001", pricing.Haiku45.InputPerMillion, pricing.Haiku45.OutputPerMillion},
		{"fable-5 dated → fable tier", "claude-fable-5-20260601", pricing.Fable5.InputPerMillion, pricing.Fable5.OutputPerMillion},
		// Mythos 5 shares Fable 5's capabilities and pricing.
		{"mythos-5 → fable tier", "claude-mythos-5", pricing.Fable5.InputPerMillion, pricing.Fable5.OutputPerMillion},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := pricing.Lookup(tt.id)
			if m.InputPerMillion != tt.wantIn || m.OutputPerMillion != tt.wantOut {
				t.Errorf("Lookup(%q) = $%.2f/$%.2f per M; want $%.2f/$%.2f (family tier, not unknown mid-tier)",
					tt.id, m.InputPerMillion, m.OutputPerMillion, tt.wantIn, tt.wantOut)
			}
		})
	}
}

// TestLookup_NewMinorVersion_1M verifies the [1m] variant of a newer minor
// version resolves to the FAMILY price (standard 1M pricing), not the
// mid-tier unknown price.
func TestLookup_NewMinorVersion_1M(t *testing.T) {
	t.Parallel()

	m := pricing.Lookup("claude-opus-4-8[1m]")
	if m.ContextLimit != 1_000_000 {
		t.Errorf("Lookup(opus-4-8[1m]).ContextLimit = %d; want 1_000_000", m.ContextLimit)
	}
	if !approxEqual(m.InputPerMillion, pricing.Opus48.InputPerMillion, 1e-9) {
		t.Errorf("Lookup(opus-4-8[1m]).InputPerMillion = %f; want %f (opus tier at standard 1M pricing)",
			m.InputPerMillion, pricing.Opus48.InputPerMillion)
	}
}

// TestLookup_UnknownFamily_Graceful verifies that a genuinely unknown model
// family still falls back to the non-zero mid-tier estimate rather than $0.
func TestLookup_UnknownFamily_Graceful(t *testing.T) {
	t.Parallel()

	for _, id := range []string{"gpt-5", "claude-mystery-9", "gemini-2-ultra"} {
		m := pricing.Lookup(id)
		if m.InputPerMillion == 0 && m.OutputPerMillion == 0 {
			t.Errorf("Lookup(%q) returned zero-cost model; want non-zero mid-tier fallback", id)
		}
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
