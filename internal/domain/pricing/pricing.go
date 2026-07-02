// Package pricing provides a hardcoded pricing table for Claude models and
// cost/compaction helpers for the usage panel.
//
// Prices are sourced from Anthropic's public pricing page and are hardcoded
// because Anthropic does not expose a machine-readable pricing API.
//
// WARNING: These values may be stale. Verify against
// https://www.anthropic.com/pricing before making billing decisions.
// Last verified: 2026-07-02.
package pricing

import (
	"strings"

	"github.com/Systemartis/clyde/internal/domain/usage"
)

// Model holds the pricing and context metadata for a Claude model.
type Model struct {
	// ID is the canonical model identifier as it appears in the JSONL
	// "message.model" field, e.g. "claude-opus-4-7".
	ID string

	// InputPerMillion is the cost in USD per one million input tokens.
	InputPerMillion float64

	// OutputPerMillion is the cost in USD per one million output tokens.
	OutputPerMillion float64

	// CacheReadPerMillion is the cost in USD per one million cache-read tokens.
	CacheReadPerMillion float64

	// CacheCreationPerMillion is the cost in USD per one million
	// cache-creation tokens.
	CacheCreationPerMillion float64

	// ContextLimit is the maximum context window in tokens for this model.
	// Used to compute CompactionPercent.
	//
	// Note: this reflects the window Claude Code runs the model with by
	// default (200K for most models), not the API maximum. Sessions running
	// under the 1M window are identified by the "[1m]" model-ID suffix and
	// resolved via Lookup, which overrides ContextLimit to 1M.
	ContextLimit int64

	// OneMPremium is true for legacy models whose 1M long-context beta was
	// billed at a premium (2× input / 1.5× output above the 200K threshold).
	// Models from the 4.6 era onward serve the 1M window at standard
	// per-token pricing, so their [1m] variant only changes ContextLimit.
	OneMPremium bool
}

// Predefined models. Prices in USD/million tokens; context in tokens.
// Sources: https://www.anthropic.com/pricing (verified 2026-07-02).
// Cache rates follow Anthropic's standard multipliers: reads = 0.1× input,
// 5-minute writes = 1.25× input.
// Stale warning: update when Anthropic publishes new prices.
var (
	// Fable5 is claude-fable-5 — Anthropic's top tier, above Opus.
	// The 1M window is Fable's default and only size, at standard pricing.
	Fable5 = Model{
		ID:                      "claude-fable-5",
		InputPerMillion:         10.00,
		OutputPerMillion:        50.00,
		CacheReadPerMillion:     1.00,
		CacheCreationPerMillion: 12.50,
		ContextLimit:            1_000_000,
	}

	// Opus48 is claude-opus-4-8 — current Opus generation.
	Opus48 = Model{
		ID:                      "claude-opus-4-8",
		InputPerMillion:         5.00,
		OutputPerMillion:        25.00,
		CacheReadPerMillion:     0.50,
		CacheCreationPerMillion: 6.25,
		ContextLimit:            200_000,
	}

	// Opus47 is claude-opus-4-7 — previous Opus generation.
	// Opus dropped to $5/$25 with the 4.5 release.
	Opus47 = Model{
		ID:                      "claude-opus-4-7",
		InputPerMillion:         5.00,
		OutputPerMillion:        25.00,
		CacheReadPerMillion:     0.50,
		CacheCreationPerMillion: 6.25,
		ContextLimit:            200_000,
	}

	// Sonnet5 is claude-sonnet-5 — current Sonnet generation.
	// Sticker price ($3/$15); the 2026 introductory discount ($2/$10
	// through 2026-08-31) is intentionally not modeled.
	Sonnet5 = Model{
		ID:                      "claude-sonnet-5",
		InputPerMillion:         3.00,
		OutputPerMillion:        15.00,
		CacheReadPerMillion:     0.30,
		CacheCreationPerMillion: 3.75,
		ContextLimit:            200_000,
	}

	// Sonnet46 is claude-sonnet-4-6 — previous Sonnet generation.
	Sonnet46 = Model{
		ID:                      "claude-sonnet-4-6",
		InputPerMillion:         3.00,
		OutputPerMillion:        15.00,
		CacheReadPerMillion:     0.30,
		CacheCreationPerMillion: 3.75,
		ContextLimit:            200_000,
	}

	// Sonnet45 is claude-sonnet-4-5 — older Sonnet generation. Its 1M
	// window was a long-context beta billed at a premium (see OneMPremium).
	Sonnet45 = Model{
		ID:                      "claude-sonnet-4-5",
		InputPerMillion:         3.00,
		OutputPerMillion:        15.00,
		CacheReadPerMillion:     0.30,
		CacheCreationPerMillion: 3.75,
		ContextLimit:            200_000,
		OneMPremium:             true,
	}

	// Haiku45 is claude-haiku-4-5 — fastest, lowest cost.
	Haiku45 = Model{
		ID:                      "claude-haiku-4-5",
		InputPerMillion:         1.00,
		OutputPerMillion:        5.00,
		CacheReadPerMillion:     0.10,
		CacheCreationPerMillion: 1.25,
		ContextLimit:            200_000,
	}

	// Opus45 is claude-opus-4-5 — the release that cut Opus to $5/$25.
	Opus45 = Model{
		ID:                      "claude-opus-4-5",
		InputPerMillion:         5.00,
		OutputPerMillion:        25.00,
		CacheReadPerMillion:     0.50,
		CacheCreationPerMillion: 6.25,
		ContextLimit:            200_000,
	}

	// unknownModel is used as a fallback for unrecognized model IDs.
	// Prices are set to a common mid-tier estimate to avoid showing $0.
	unknownModel = Model{
		ID:                      "unknown",
		InputPerMillion:         3.00,
		OutputPerMillion:        15.00,
		CacheReadPerMillion:     0.30,
		CacheCreationPerMillion: 3.75,
		ContextLimit:            200_000,
	}
)

// known is the registry of all models keyed by their ID.
// Add new models here when Anthropic releases them.
var known = map[string]Model{
	Fable5.ID:   Fable5,
	Opus48.ID:   Opus48,
	Opus47.ID:   Opus47,
	Sonnet5.ID:  Sonnet5,
	Sonnet46.ID: Sonnet46,
	Sonnet45.ID: Sonnet45,
	Haiku45.ID:  Haiku45,
	Opus45.ID:   Opus45,
}

// oneMContextSuffix is the model ID suffix for 1M context-window variants,
// e.g. "claude-opus-4-7[1m]". Anthropic charges a higher rate for these.
const oneMContextSuffix = "[1m]"

// Lookup returns the Model for the given model ID.
// If the ID is not found in the pricing table, it returns a generic fallback
// model with mid-tier pricing rather than a zero-cost model.
//
// 1M context variants: when id ends with "[1m]", ContextLimit is overridden
// to 1_000_000. Models from the 4.6 era onward serve the 1M window at
// standard per-token pricing (no long-context premium), so prices stay at
// the base rate; only legacy OneMPremium models (Sonnet 4.5's long-context
// beta) apply the 2× input / 1.5× output premium, approximated flat since
// per-request tiering isn't visible in the JSONL.
func Lookup(id string) Model {
	// Check for 1M context suffix and strip it for the base pricing lookup.
	is1M := false
	baseID := id
	if len(id) > len(oneMContextSuffix) &&
		id[len(id)-len(oneMContextSuffix):] == oneMContextSuffix {
		is1M = true
		baseID = id[:len(id)-len(oneMContextSuffix)]
	}

	m := lookupBase(baseID)

	if is1M {
		m.ID = id
		m.ContextLimit = 1_000_000
		if m.OneMPremium {
			// Legacy long-context beta premium (2× input, 1.5× output).
			// Cache rates scale with the input rate.
			m.InputPerMillion *= 2
			m.OutputPerMillion *= 1.5
			m.CacheReadPerMillion *= 2
			m.CacheCreationPerMillion *= 2
		}
	}

	return m
}

// familyAlias maps a model-family prefix to the closest known Model.
// It resolves IDs that have no exact and no date-suffixed match — a newer
// minor version ("claude-opus-4-9"), a newer major version ("claude-sonnet-6"),
// or a sibling tier ("claude-mythos-5", which shares Fable 5's pricing) — to
// an EXISTING table entry for the same family instead of the generic mid-tier
// "unknown" fallback. Each alias points at the newest known model in the tier.
var familyAlias = []struct {
	prefix string
	model  Model
}{
	{"claude-fable-", Fable5},   // top tier
	{"claude-mythos-", Fable5},  // Mythos shares Fable pricing/capabilities
	{"claude-opus-", Opus48},    // newest known Opus tier
	{"claude-sonnet-", Sonnet5}, // newest known Sonnet tier
	{"claude-haiku-", Haiku45},  // newest known Haiku tier
}

// lookupBase returns the base Model for a given model ID (no suffix processing).
//
// Resolution order:
//  1. Exact match in the known table.
//  2. Date-suffixed known version, e.g. "claude-opus-4-7-20260115" → Opus47
//     (a known ID followed by "-...").
//  3. Family fallback: an unknown version of a known tier resolves to that
//     tier's newest known entry, e.g. "claude-opus-4-8" or "claude-sonnet-5".
//  4. Generic mid-tier "unknown" fallback for genuinely unrecognized families.
func lookupBase(id string) Model {
	if m, ok := known[id]; ok {
		return m
	}
	// Date-suffixed KNOWN versions: a known ID that is a "-"-delimited prefix
	// of the incoming ID (the boundary avoids "claude-opus-4-5" matching a
	// hypothetical "claude-opus-4-50").
	for _, m := range known {
		if strings.HasPrefix(id, m.ID+"-") {
			return m
		}
	}
	// Family fallback: newer minor/major versions of a known tier, plus new
	// tiers, map to the closest existing family entry.
	for _, fa := range familyAlias {
		if strings.HasPrefix(id, fa.prefix) {
			return fa.model
		}
	}
	return unknownModel
}

// Cost computes the total USD cost for the given usage on the given model.
// Returns 0 when m.ContextLimit == 0 (zero model) to avoid divide-by-zero.
func Cost(u usage.Usage, m Model) float64 {
	const perMillion = 1_000_000.0
	return float64(u.Input)*m.InputPerMillion/perMillion +
		float64(u.Output)*m.OutputPerMillion/perMillion +
		float64(u.CacheRead)*m.CacheReadPerMillion/perMillion +
		float64(u.CacheCreation)*m.CacheCreationPerMillion/perMillion
}

// TotalTokens returns the "billed activity" token count for display:
// input + output + cache_creation.
//
// cache_read_input_tokens is intentionally excluded from this total because it
// is a per-turn value that records how many cached tokens were read on each
// individual API call. The same cached content is read again on every turn that
// uses it, so summing cache_read across turns counts the same tokens many times
// (e.g. a 5h session can accumulate 65M "cache read" tokens while the actual
// context is only ~750k). Including cache_read in the display total produces
// impossibly high numbers.
//
// Cost() still applies the correct 0.1× rate to cache_read tokens for billing
// purposes — that is unaffected by this change.
func TotalTokens(u usage.Usage) int64 {
	return u.Input + u.Output + u.CacheCreation
}

// ContextTokens returns the full context size of a single API call:
// input + cache_read + cache_creation.
//
// This is the correct value to use for compaction percentage calculation because
// it represents the total number of tokens that were sent to the model in a
// single turn (cached or not). Use this with the LATEST assistant event's usage,
// not a running sum — the running sum inflates beyond any meaningful context limit.
func ContextTokens(u usage.Usage) int64 {
	return u.Input + u.CacheRead + u.CacheCreation
}

// CompactionPercent returns a value in [0, 1] representing how full the model's
// context window is for the given single-turn usage snapshot.
//
// u should be the LATEST assistant event's usage (not a running sum across all
// turns). The context fill is: input + cache_read + cache_creation — all tokens
// sent to the model in that one API call.
// Returns 0 when m.ContextLimit == 0 to avoid divide-by-zero.
func CompactionPercent(u usage.Usage, m Model) float64 {
	if m.ContextLimit == 0 {
		return 0
	}
	used := ContextTokens(u)
	pct := float64(used) / float64(m.ContextLimit)
	if pct > 1.0 {
		pct = 1.0
	}
	if pct < 0 {
		pct = 0
	}
	return pct
}
