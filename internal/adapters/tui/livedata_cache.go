package tui

import (
	"github.com/clyde-tui/clyde/internal/application/livesession"
)

// deriveCacheStats maps the live session's cache stats into the TUI view.
// When aggregate is true, uses CwdCacheStats (combined across every recent
// session in the cwd) — that's the right number to show on the Σ tab,
// where a single-session figure would be misleading.
func deriveCacheStats(v livesession.View, d MockData, aggregate bool) MockData {
	stats := v.CacheStats
	if aggregate {
		stats = v.CwdCacheStats
	}
	if stats.TurnCount == 0 {
		// In live mode we want to clear stale mock data even when the new
		// session has no cache observations yet — same fix pattern as
		// activity / bash. An empty CacheStatsView is the correct
		// rendering (the panel falls back to "no turns yet").
		d.Cache = CacheStatsView{}
		return d
	}
	d.Cache = CacheStatsView{
		HitRatio:          stats.HitRatio,
		FromCache:         stats.FromCache,
		Recomputed:        stats.Recomputed,
		BiggestMissTokens: stats.BiggestMissTokens,
		BiggestMissAt:     formatTimestamp(stats.BiggestMissAt),
		Trend:             append([]float64(nil), stats.Trend...),
		TurnCount:         stats.TurnCount,
	}
	return d
}
