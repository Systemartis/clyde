// Package pricing provides a hardcoded pricing table for Claude models and
// cost/compaction helpers for the usage panel.
//
// Prices are sourced from Anthropic's public pricing page and are hardcoded
// because Anthropic does not expose a machine-readable pricing API.
//
// WARNING: These values may be stale. Verify against
// https://www.anthropic.com/pricing before making billing decisions.
// Last verified: 2026-04-30.
package pricing

import "github.com/Systemartis/clyde/internal/domain/usage"

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
	ContextLimit int64
}

// Predefined models. Prices in USD/million tokens; context in tokens.
// Sources: https://www.anthropic.com/pricing (verified 2026-04-30).
// Stale warning: update when Anthropic publishes new prices.
var (
	// Opus47 is claude-opus-4-7 — most capable, highest cost.
	Opus47 = Model{
		ID:                      "claude-opus-4-7",
		InputPerMillion:         15.00,
		OutputPerMillion:        75.00,
		CacheReadPerMillion:     1.50,
		CacheCreationPerMillion: 18.75,
		ContextLimit:            200_000,
	}

	// Sonnet46 is claude-sonnet-4-6 — balanced capability/cost.
	Sonnet46 = Model{
		ID:                      "claude-sonnet-4-6",
		InputPerMillion:         3.00,
		OutputPerMillion:        15.00,
		CacheReadPerMillion:     0.30,
		CacheCreationPerMillion: 3.75,
		ContextLimit:            200_000,
	}

	// Sonnet45 is claude-sonnet-4-5 — previous Sonnet generation.
	Sonnet45 = Model{
		ID:                      "claude-sonnet-4-5",
		InputPerMillion:         3.00,
		OutputPerMillion:        15.00,
		CacheReadPerMillion:     0.30,
		CacheCreationPerMillion: 3.75,
		ContextLimit:            200_000,
	}

	// Haiku45 is claude-haiku-4-5 — fastest, lowest cost.
	Haiku45 = Model{
		ID:                      "claude-haiku-4-5",
		InputPerMillion:         0.80,
		OutputPerMillion:        4.00,
		CacheReadPerMillion:     0.08,
		CacheCreationPerMillion: 1.00,
		ContextLimit:            200_000,
	}

	// Opus45 is claude-opus-4-5 — previous Opus generation.
	Opus45 = Model{
		ID:                      "claude-opus-4-5",
		InputPerMillion:         15.00,
		OutputPerMillion:        75.00,
		CacheReadPerMillion:     1.50,
		CacheCreationPerMillion: 18.75,
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
	Opus47.ID:   Opus47,
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
// 1M context variants: when id ends with "[1m]", the base model pricing is
// used but ContextLimit is overridden to 1_000_000 and per-million prices are
// doubled (Anthropic charges 2× for 1M context variants — best guess as of
// 2026-04-30; verify against https://www.anthropic.com/pricing).
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
		// Override context limit to 1M and double prices per Anthropic's 2× rule
		// for extended-context variants. Prices as of 2026-04-30 (best guess):
		//   Opus47 1M: $30/M input, $150/M output.
		// TODO: verify exact 1M pricing for all models once Anthropic publishes them.
		m.ID = id
		m.ContextLimit = 1_000_000
		m.InputPerMillion *= 2
		m.OutputPerMillion *= 2
		m.CacheReadPerMillion *= 2
		m.CacheCreationPerMillion *= 2
	}

	return m
}

// lookupBase returns the base Model for a given model ID (no suffix processing).
func lookupBase(id string) Model {
	if m, ok := known[id]; ok {
		return m
	}
	// Prefix-match for forward compatibility: "claude-opus-4-8" would still
	// match Opus pricing if an exact entry doesn't exist yet.
	for _, m := range known {
		if len(id) >= len(m.ID) && id[:len(m.ID)] == m.ID {
			return m
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
