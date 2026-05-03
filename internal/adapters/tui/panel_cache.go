package tui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
)

// renderCacheExpanded renders the cache-efficiency panel with active-mode
// support. The body has four rows: hit ratio + bar, breakdown, biggest
// miss, sparkline trend.
func renderCacheExpanded(s Styles, p Palette, d MockData, vp viewport.Model, width, height int, focused, activeMode bool) string {
	if activeMode {
		inner := width - 4
		vp.SetWidth(inner)
		vp.SetHeight(height - 2)
		content := vp.View()
		return wrapPanelActive(s, content, "cache", width, height)
	}
	return renderCache(s, p, d, width, height, focused)
}

// renderCache renders the passive-state cache-efficiency panel.
func renderCache(s Styles, p Palette, d MockData, width, height int, focused bool) string {
	inner := width - 4
	if d.Cache.TurnCount == 0 {
		empty := s.TextFade.Render("  no turns observed yet")
		return wrapPanel(s, empty, "cache", "", width, height, focused)
	}
	body := buildCacheBody(s, p, d, inner)
	meta := fmt.Sprintf("%d turns", d.Cache.TurnCount)
	return wrapPanel(s, body, "cache", meta, width, height, focused)
}

// renderCacheCollapsed renders the collapsed one-liner.
func renderCacheCollapsed(s Styles, d MockData, width int, focused bool) string {
	if d.Cache.TurnCount == 0 {
		return wrapPanelCollapsed(s, "cache", "no turns yet", "", width, focused)
	}
	summary := fmt.Sprintf("%d%% hit · %d turns", roundPct(d.Cache.HitRatio*100), d.Cache.TurnCount)
	return wrapPanelCollapsed(s, "cache", summary, "", width, focused)
}

// buildCacheBody assembles the four content rows.
func buildCacheBody(s Styles, p Palette, d MockData, inner int) string {
	hitPct := roundPct(d.Cache.HitRatio * 100)
	headline := fmt.Sprintf("%d%% hit ratio", hitPct)
	// Color the headline by the ratio's health: high = cyan, low = pink.
	headlineStyled := lipgloss.NewStyle().Foreground(cacheRatioColor(p, d.Cache.HitRatio)).Bold(true).Render(headline)

	// Headline row: ratio + horizontal bar visualization
	barW := inner - ansiWidth(headline) - 2
	if barW < 8 {
		barW = 8
	}
	bar := buildCacheBar(p, d.Cache.HitRatio, barW)
	row1 := headlineStyled + " " + bar

	// Breakdown row
	row2 := s.TextFade.Render(fmt.Sprintf(
		"  %s from cache · %s recomputed",
		formatTokenCount(d.Cache.FromCache),
		formatTokenCount(d.Cache.Recomputed),
	))

	// Biggest miss row
	var row3 string
	if d.Cache.BiggestMissTokens > 0 && d.Cache.BiggestMissAt != "" {
		row3 = s.TextFade.Render(fmt.Sprintf(
			"  biggest miss: %s  +%s",
			d.Cache.BiggestMissAt,
			formatTokenCount(d.Cache.BiggestMissTokens),
		))
	}

	// Sparkline row
	row4 := ""
	if len(d.Cache.Trend) > 0 {
		spark := buildCacheSparkline(p, d.Cache.Trend)
		row4 = s.TextFade.Render("  trend: ") + spark
	}

	rows := []string{row1, "", row2}
	if row3 != "" {
		rows = append(rows, row3)
	}
	if row4 != "" {
		rows = append(rows, row4)
	}
	return strings.Join(rows, "\n")
}

// gradientStops returns the 4-stop gradient used by the cache panel —
// turquoise → blue → purple → pink — matching the visual language of
// the token-usage bars. Order is GOOD → BAD: cyan = healthy (high cache
// hit ratio), pink = unhealthy (lots of cache misses). The bar / sparkline
// pick a stop from this slice based on the inverted ratio so a healthy
// 99% reads as cyan and a struggling 20% reads as pink.
func gradientStops(p Palette) []color.Color {
	return []color.Color{p.Cyan, p.BorderAcc, p.Purple, p.Pink}
}

// cacheRatioColor returns the gradient stop for a cache hit ratio.
// High ratio (close to 1.0) → Cyan (healthy). Low ratio → Pink (poor).
func cacheRatioColor(p Palette, ratio float64) color.Color {
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	// Invert so high ratio maps to the start of the stops slice (cyan).
	pos := 1 - ratio
	stops := gradientStops(p)
	idx := int(pos * float64(len(stops)))
	if idx >= len(stops) {
		idx = len(stops) - 1
	}
	return stops[idx]
}

// buildCacheBar renders a horizontal bar whose ENTIRE fill takes the color
// matching the current ratio: a 99% bar reads as solid cyan, a 30% bar
// as solid pink. The whole-fill colorway reads as 'how healthy is the
// cache right now?' at a glance — the position-based gradient (token-bar
// style) didn't make sense here because cache 'fullness' is a quality
// signal, not a consumption signal.
func buildCacheBar(p Palette, ratio float64, width int) string {
	if width < 1 {
		return ""
	}
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	filled := int(ratio*float64(width) + 0.5)
	if filled > width {
		filled = width
	}
	fill := lipgloss.NewStyle().Foreground(cacheRatioColor(p, ratio))
	track := lipgloss.NewStyle().Foreground(p.TextFade)
	return fill.Render(strings.Repeat("█", filled)) + track.Render(strings.Repeat("░", width-filled))
}

// sparkChars is the eight-step block ramp used by buildCacheSparkline.
var sparkChars = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// buildCacheSparkline renders the per-turn ratio history as a single-row
// block-character sparkline. Cell HEIGHT comes from the value; cell COLOR
// comes from cacheRatioColor (high ratio = cyan/blue, low = pink) so the
// trend reads as 'good cells stay cool, bad cells flare warm'.
func buildCacheSparkline(p Palette, trend []float64) string {
	if len(trend) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, v := range trend {
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		idx := int(v*float64(len(sparkChars)-1) + 0.5)
		st := lipgloss.NewStyle().Foreground(cacheRatioColor(p, v))
		sb.WriteString(st.Render(string(sparkChars[idx])))
	}
	return sb.String()
}

// buildCacheViewportContent — the cache panel is small and self-contained,
// so the viewport content just mirrors the passive body. Kept for symmetry
// with other active-mode panels.
func buildCacheViewportContent(s Styles, p Palette, d MockData, inner int) string {
	if d.Cache.TurnCount == 0 {
		return s.TextFade.Render("  no turns observed yet")
	}
	return buildCacheBody(s, p, d, inner)
}
