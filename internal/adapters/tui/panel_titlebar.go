package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/Systemartis/clyde/internal/application/livesession"
	"github.com/Systemartis/clyde/internal/domain/pricing"
)

// renderTitleBar renders the slim top bar:
//
//	clyde · claude-code · ~/projects/clyde · opus 4.7        1h 24m  47k tok  $1.42
//
// Tabs moved to the bottom status bar in v0.6 — the title bar is the
// stable identity strip (who am I, what am I editing, which model) and
// the status bar carries everything that changes as the user navigates
// (sessions, hints, version). Keeping the two roles separate stopped
// the title bar from getting crowded once a few session tabs landed in
// it.
//
// In live mode (demoMode=false), duration/tokens/cost are computed
// from liveView. In demo mode, the MockData fields are used as-is.
func renderTitleBar(s Styles, _ Palette, d MockData, _ FrameState, width int, demoMode bool, liveView livesession.View, now time.Time) string {
	brand := s.TitleBrand.Render("clyde")
	sep := s.TitleMeta.Render(" · ")
	path := s.TitlePath.Render(d.ProjectPath) + s.TitleProjct.Render(d.ProjectName)

	modelStr := d.Model
	if modelStr == "" {
		modelStr = "unknown"
	}
	model := s.TitleBrand.Render(modelStr)

	var left string
	if !demoMode && liveView.LLMSource != "" {
		src := s.TitleMeta.Render(liveView.LLMSource)
		left = brand + sep + src + sep + path + sep + model
	} else {
		left = brand + sep + path + sep + model
	}

	// Subagent count used to live here as "N subagent(s)" but it
	// duplicated the activity panel's "N agents active" badge and made
	// the (already busy) title bar feel cramped. The activity panel is
	// always the more accurate place to read it from.

	var dur, toks, cost string
	if !demoMode && len(liveView.Events) > 0 {
		// Duration: now - earliest event timestamp in the focused session.
		elapsed := now.Sub(liveView.Events[0].Timestamp)
		if elapsed < 0 {
			elapsed = 0
		}
		dur = s.TitleMeta.Render(formatTitleDuration(elapsed))

		// Tokens: total from TotalUsage.
		totalTok := pricing.TotalTokens(liveView.TotalUsage)
		toks = s.TitleValue.Render(formatTitleTokens(totalTok)) + s.TitleMeta.Render(" tok")

		// Cost: computed from TotalUsage + model pricing.
		m := pricing.Lookup(liveView.CurrentModel)
		costVal := pricing.Cost(liveView.TotalUsage, m)
		cost = s.TitleValue.Render(fmt.Sprintf("$%.2f", costVal))
	} else {
		// Demo mode — use MockData fields (the mock values from V3MockData).
		dur = s.TitleMeta.Render(d.Duration)
		toks = s.TitleValue.Render(d.Tokens) + s.TitleMeta.Render(" tok")
		cost = s.TitleValue.Render(d.Cost)
	}

	// v22+ pricing dual-mode: subscribers (Pro/Max) do not pay per token, so
	// the title-bar $ figure is misleading for them. Hide it; show duration +
	// tokens only.
	rightParts := []string{dur, toks}
	if shouldShowCost(d) {
		rightParts = append(rightParts, cost)
	}
	right := strings.Join(rightParts, s.TitleMeta.Render("  "))

	// Fill the gap between left and right
	leftW := ansiWidth(left)
	rightW := ansiWidth(right)
	gap := width - leftW - rightW - 2 // 2 for leading/trailing space
	if gap < 1 {
		gap = 1
	}
	spacer := strings.Repeat(" ", gap)

	line := " " + left + spacer + right + " "

	// Dashed bottom border separator
	dashes := strings.Repeat("─", width)
	border := s.TitleMeta.Render(dashes)

	return fmt.Sprintf("%s\n%s", line, border)
}

// renderSessionTabs returns the title-bar tab strip
// "[Σ all] [● fix auth ⚠] [docs] [● old]". Two visual axes are orthogonal:
//
//   - Live (claude code still has the session open): leading "● " bullet.
//   - Active (the user's currently-focused tab in clyde): bright text
//     style instead of dim.
//
// A user-focused but closed session reads as bright "[ resume ]"; a live
// session the user is NOT looking at still shows the bullet so the eye
// catches it. The compaction warning ⚠ is independent of both.
//
// The Σ aggregate tab is always rendered first when present and never
// carries the bullet (it represents the cwd, not a single session).
func renderSessionTabs(s Styles, tabs []SessionTab) string {
	if len(tabs) == 0 {
		return ""
	}
	var parts []string
	for _, t := range tabs {
		parts = append(parts, renderSessionTab(s, t))
	}
	return strings.Join(parts, " ")
}

func renderSessionTab(s Styles, t SessionTab) string {
	label := t.Label
	if t.Warning && !t.IsAggregate {
		label += " ⚠"
	}
	bullet := ""
	if t.Live && !t.IsAggregate {
		bullet = "● "
	}
	body := "[" + bullet + label + "]"
	if t.Active {
		return s.TitleValue.Render(body)
	}
	return s.TitleMeta.Render(body)
}

// formatTitleDuration formats an elapsed duration for the title bar:
// "14s", "1h 24m", "2m 3s", etc. — compact human-readable form.
func formatTitleDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	switch {
	case h > 0 && m > 0:
		return fmt.Sprintf("%dh %dm", h, m)
	case h > 0:
		return fmt.Sprintf("%dh", h)
	case m > 0 && s > 0:
		return fmt.Sprintf("%dm %ds", m, s)
	case m > 0:
		return fmt.Sprintf("%dm", m)
	default:
		return fmt.Sprintf("%ds", s)
	}
}

// formatTitleTokens formats a token count as a compact string:
// <1000 → "N", <10000 → "N.Nk", <1M → "NNk", ≥1M → "N.NM".
func formatTitleTokens(n int64) string {
	switch {
	case n < 1_000:
		return fmt.Sprintf("%d", n)
	case n < 10_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	case n < 1_000_000:
		return fmt.Sprintf("%dk", n/1_000)
	default:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
}
