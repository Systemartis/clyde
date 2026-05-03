package tui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
)

// renderUsageExpanded renders the usage panel in expanded state.
// When activeMode is true the panel is in Expanded-Active state:
// content is shown through the viewport (enables real scrolling) and
// a pink double border + mode badge replace the normal chrome.
func renderUsageExpanded(s Styles, p Palette, d MockData, progTokens, progReset progress.Model, vp viewport.Model, width, height int, focused, activeMode bool) string {
	if activeMode {
		// In active mode, pipe content through the viewport and wrap with active border.
		inner := width - 4
		vp.SetWidth(inner)
		vp.SetHeight(height - 2)
		content := vp.View()
		return wrapPanelActive(s, content, "usage", width, height)
	}
	// Passive / normal: render full content directly.
	return renderUsage(s, p, d, progTokens, progReset, width, height, focused)
}

// renderUsage renders the usage panel: 4 progress bars (session ctx, 5h usage,
// weekly usage, next reset) followed by cost/turns/model/burn and issues.
func renderUsage(s Styles, p Palette, d MockData, progTokens, progReset progress.Model, width, height int, focused bool) string {
	inner := width - 4 // border + 1-char pad each side
	content := buildUsageBody(s, p, d, progTokens, progReset, inner)
	return wrapPanel(s, content, "usage", "", width, height, focused)
}

// buildUsageBody produces the inner panel content shared by the passive and
// viewport (active-mode) renderers. inner is the visible width.
func buildUsageBody(s Styles, p Palette, d MockData, progTokens, progReset progress.Model, inner int) string {
	var sb strings.Builder

	barW := inner
	if barW < 2 {
		barW = 2
	}
	progTokens.SetWidth(barW)
	progReset.SetWidth(barW)

	// Header row: plan tier on the left, "(plan offline)" badge on the
	// right when the API is unreachable. Only rendered when there's
	// something to say. API users (IsAPIUser) read as "api" so the user
	// can see at a glance which auth mode they're in.
	tierLabel := planTierLabel(d)
	if tierLabel != "" || d.PlanUsageOffline {
		left := ""
		right := ""
		if tierLabel != "" {
			left = s.UsageLabel.Render("plan") + "  " + s.UsageModel.Render(tierLabel)
		}
		if d.PlanUsageOffline {
			right = s.UsageLabel.Render("(plan offline)")
		}
		sb.WriteString(rowSpread(left, right, inner))
		sb.WriteByte('\n')
	}

	// ── 4 progress bars: session ctx · 5h session · weekly · next reset ──
	// When the Σ aggregate tab is active and there are 2+ live sessions in
	// the cwd, replace the single session-ctx bar with a per-session
	// leaderboard. Each session gets its own mini-bar sorted by ContextPct
	// desc; the "is anything about to compact?" signal stays one glance away.
	if len(d.SessionLeaderboard) > 0 {
		sb.WriteString(renderSessionLeaderboard(s, p, d.SessionLeaderboard, inner))
		sb.WriteByte('\n')
	} else if !d.UsageSession.Empty {
		sb.WriteString(renderUsageBar(s, progTokens, d.UsageSession, sessionRowSubInfo(d.UsageSession), inner))
		sb.WriteByte('\n')
	}
	if !d.Usage5h.Empty {
		sb.WriteString(renderUsageBar(s, progTokens, d.Usage5h, windowRowSubInfo(d.Usage5h), inner))
		sb.WriteByte('\n')
	}
	if !d.UsageWeek.Empty {
		sb.WriteString(renderUsageBar(s, progTokens, d.UsageWeek, windowRowSubInfo(d.UsageWeek), inner))
		sb.WriteByte('\n')
	}

	// "next reset" — uses the SOONEST upcoming reset (5h vs weekly).
	// Cyan→Purple gradient, visually distinct from the token-usage bars.
	if reset, ok := nextResetRow(d); ok {
		sb.WriteString(renderUsageBar(s, progReset, reset, "", inner))
		sb.WriteByte('\n')
	}

	// dotted divider
	sb.WriteString(s.UsageDivider.Render(strings.Repeat("·", clamp(inner, 1, inner))))
	sb.WriteByte('\n')

	// turns / model rows — always shown.
	// cost / burn rate rows — only shown for API-key users. Subscribers (Pro/Max)
	// pay a flat subscription so per-token $ is misleading; the plan-quota bars
	// already convey the meaningful constraint for them.
	if shouldShowCost(d) {
		sb.WriteString(rowSpread(s.UsageLabel.Render("cost"), s.UsageValue.Render(d.Cost142), inner))
		sb.WriteByte('\n')
	}
	sb.WriteString(rowSpread(s.UsageLabel.Render("turns"), s.UsageValue.Render(d.Turns), inner))
	sb.WriteByte('\n')
	sb.WriteString(rowSpread(s.UsageLabel.Render("model"), s.UsageModel.Render(d.Model), inner))
	sb.WriteByte('\n')

	if shouldShowCost(d) && d.BurnRate != "" {
		burn := s.UsageLabel.Render("burn") + "  " + s.UsageValue.Render(d.BurnRate)
		sb.WriteString(burn)
		sb.WriteByte('\n')
	}

	// dotted divider before issues
	sb.WriteString(s.UsageDivider.Render(strings.Repeat("·", clamp(inner, 1, inner))))
	sb.WriteByte('\n')

	// issues rows — dot + label left, value right
	issueRow := func(dot, label, value string) string {
		left := dot + " " + label
		right := value
		leftW := ansiWidth(left)
		rightW := ansiWidth(right)
		gapW := inner - leftW - rightW
		if gapW < 1 {
			gapW = 1
		}
		return left + strings.Repeat(" ", gapW) + right
	}

	errDot := s.TextFade.Render("●")
	sb.WriteString(issueRow(errDot, s.IssueName.Render("errors"), s.IssueValue.Render(d.Errors)))
	sb.WriteByte('\n')

	warnDot := s.Amber.Render("●")
	sb.WriteString(issueRow(warnDot, s.IssueName.Render("warnings"), s.IssueValue.Render(d.Warnings)))
	sb.WriteByte('\n')

	testDot := s.StatusGreen.Render("●")
	sb.WriteString(issueRow(testDot, s.IssueName.Render("tests"), s.IssueValue.Render(d.Tests)))

	return sb.String()
}

// renderUsageBar renders a 3-line progress block:
//
//	<label>                              <value>
//	████████░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░
//	                              <subInfo>
//
// subInfo may be empty, in which case the trailing line is omitted.
// progBar is rendered statically with its own gradient (set by the caller).
func renderUsageBar(s Styles, progBar progress.Model, row UsageWindowRow, subInfo string, inner int) string {
	var sb strings.Builder

	// Top row: label left, headline value right.
	value := headlineValue(row)
	sb.WriteString(rowSpread(s.UsageLabel.Render(row.Label), s.UsageValue.Render(value), inner))
	sb.WriteByte('\n')

	// Progress bar — static render via ViewAs for golden-test stability.
	pct := clamp(row.Percent, 0, 100)
	sb.WriteString(progBar.ViewAs(float64(pct) / 100.0))

	// Optional sub-info line right-aligned.
	if subInfo != "" {
		sb.WriteByte('\n')
		line := s.UsageLabel.Render(subInfo)
		lineW := ansiWidth(line)
		padW := inner - lineW
		if padW < 0 {
			padW = 0
		}
		sb.WriteString(strings.Repeat(" ", padW) + line)
	}
	return sb.String()
}

// headlineValue returns the value displayed on the top row (right of label).
//
// Priority:
//  1. percentage (always known) — keeps the eye on "how full is the bar".
//     Values above 100% are shown verbatim (extra-usage credits territory).
//  2. resets-in countdown (for the dedicated reset row, where Percent is
//     window-elapsed but the user wants the absolute time).
func headlineValue(row UsageWindowRow) string {
	// The dedicated "next reset" synthetic row sets ResetsIn but keeps an
	// empty percent context — show the countdown as the headline.
	if row.Label == "next reset" && row.ResetsIn != "" {
		return row.ResetsIn
	}
	pct := row.Percent
	if pct < 0 {
		pct = 0
	}
	return fmt.Sprintf("%d%%", pct)
}

// sessionRowSubInfo returns the "47k / 200k (23%)" sub-line for the session row.
func sessionRowSubInfo(row UsageWindowRow) string {
	if row.CurrentCtx != "" {
		return row.CurrentCtx
	}
	return ""
}

// windowRowSubInfo returns the "186k tokens · resets 3h 57m" sub-line for the
// 5h / weekly usage rows.
func windowRowSubInfo(row UsageWindowRow) string {
	parts := make([]string, 0, 3)
	if row.TotalUsed != "" {
		tok := row.TotalUsed + " tokens"
		if row.SessionCount > 0 {
			tok = fmt.Sprintf("%s · %d sessions", tok, row.SessionCount)
		}
		parts = append(parts, tok)
	}
	if row.ResetsIn != "" {
		parts = append(parts, "resets "+row.ResetsIn)
	}
	return strings.Join(parts, " · ")
}

// nextResetRow builds the synthetic "next reset" row (4th progress bar).
// Picks the SOONEST upcoming reset between 5h and weekly. Returns ok=false
// when no reset countdown is available.
func nextResetRow(d MockData) (UsageWindowRow, bool) {
	have5h := !d.Usage5h.Empty && d.Usage5h.ResetsIn != ""
	haveWk := !d.UsageWeek.Empty && d.UsageWeek.ResetsIn != ""
	if !have5h && !haveWk {
		return UsageWindowRow{}, false
	}
	// 5h is almost always sooner than weekly when both are present.
	// Use 5h when present, fall back to weekly otherwise.
	src := d.Usage5h
	if !have5h {
		src = d.UsageWeek
	}
	return UsageWindowRow{
		Label:    "next reset",
		Percent:  src.Percent,
		ResetsIn: src.ResetsIn,
	}, true
}

// buildUsageViewportContent builds the full usage content as a plain string
// suitable for viewport.SetContent() in active mode.
// inner is the visible content width (panel width - 4 for borders + padding).
func buildUsageViewportContent(s Styles, p Palette, d MockData, progTokens, progReset progress.Model, inner int) string {
	body := buildUsageBody(s, p, d, progTokens, progReset, inner)
	return strings.TrimRight(body, "\n")
}

// renderSessionLeaderboard produces the per-session mini-bar block shown
// in place of the single session-ctx bar when the Σ aggregate tab is
// active. Layout per row:
//
//	<label>      ████████░░░░░░░░  82%
//
// Color is computed from contextFillColor (high pct = pink, low = cyan)
// so a session about to compact reads as warm-pink at a glance.
//
// A header line "session ctx · most loaded ·──" makes it explicit that
// these are NOT a sum: the user sees per-session percentages, not a
// nonsense aggregate.
func renderSessionLeaderboard(s Styles, p Palette, leaders []SessionTab, inner int) string {
	if len(leaders) == 0 {
		return ""
	}
	var sb strings.Builder

	// Header — same pattern as the regular session-ctx top row but with a
	// "most loaded" hint on the right so the user can't mistake the
	// stacked bars for a single value.
	sb.WriteString(rowSpread(
		s.UsageLabel.Render("session ctx"),
		s.UsageLabel.Render("most loaded"),
		inner,
	))
	sb.WriteByte('\n')

	// Per-row width budget: label (max 12) + space + bar + space + pct (4 chars).
	const labelW = 12
	const pctW = 4
	for _, t := range leaders {
		labelTxt := truncateLabel(t.Label, labelW)
		labelPad := labelW - runeWidth(labelTxt)
		if labelPad < 0 {
			labelPad = 0
		}
		barW := inner - labelW - pctW - 2 // 2 single-spaces between cols
		if barW < 4 {
			barW = 4
		}
		bar := buildContextBar(p, t.ContextPct, barW)
		pctStr := fmt.Sprintf("%3d%%", t.ContextPct)
		// Pct color matches the bar color for an at-a-glance match.
		pctStyled := lipgloss.NewStyle().Foreground(contextFillColor(p, t.ContextPct)).Render(pctStr)

		labelStyle := s.UsageLabel
		if t.Warning {
			// Warning sessions get a bold/foreground bump so they pop in the list.
			labelStyle = lipgloss.NewStyle().Foreground(contextFillColor(p, t.ContextPct)).Bold(true)
		}
		sb.WriteString(labelStyle.Render(labelTxt))
		sb.WriteString(strings.Repeat(" ", labelPad))
		sb.WriteString(" ")
		sb.WriteString(bar)
		sb.WriteString(" ")
		sb.WriteString(pctStyled)
		sb.WriteByte('\n')
	}
	// Trim trailing newline — caller adds its own separator.
	out := sb.String()
	return strings.TrimRight(out, "\n")
}

// truncateLabel cuts label to maxW visual columns, appending an ellipsis
// when truncation occurred.
func truncateLabel(label string, maxW int) string {
	if runeWidth(label) <= maxW {
		return label
	}
	runes := []rune(label)
	// Trim until we fit the ellipsis too.
	for len(runes) > 0 && runeWidth(string(runes))+1 > maxW {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…"
}

// runeWidth returns the visual column width of s, counting each rune as 1.
// Good enough for ASCII labels — wide CJK chars aren't used in tab labels.
func runeWidth(s string) int {
	return len([]rune(s))
}

// contextFillColor maps a 0-100 ctx fill percent onto the gradient stops
// (cyan → border-acc → purple → pink). High percent = pink (danger);
// low = cyan (healthy). Mirrors the cache panel's logic but inverted —
// for cache, high is good; for ctx, high means compaction is imminent.
func contextFillColor(p Palette, pct int) color.Color {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	stops := gradientStops(p)
	// pct 0 → idx 0 (cyan); pct 100 → idx len-1 (pink).
	idx := int(float64(pct) / 100.0 * float64(len(stops)))
	if idx >= len(stops) {
		idx = len(stops) - 1
	}
	return stops[idx]
}

// buildContextBar renders a horizontal bar whose entire fill takes the
// color matching the current ctx %. Same shape as buildCacheBar, opposite
// semantics: cache cares about hit RATIO (good when high), ctx cares about
// FILL (bad when high).
func buildContextBar(p Palette, pct int, width int) string {
	if width < 1 {
		return ""
	}
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := int(float64(pct)/100.0*float64(width) + 0.5)
	if filled > width {
		filled = width
	}
	fill := lipgloss.NewStyle().Foreground(contextFillColor(p, pct))
	track := lipgloss.NewStyle().Foreground(p.TextFade)
	return fill.Render(strings.Repeat("█", filled)) + track.Render(strings.Repeat("░", width-filled))
}

// renderUsageCollapsed renders the collapsed one-liner for the usage panel.
// When compaction is non-zero an inline warning badge is appended.
func renderUsageCollapsed(s Styles, d MockData, compaction CompactionState, width int, focused bool) string {
	ctx := contextWindowLabel(d.Model)
	// API-key users see cost; subscribers see plan tier (or just turns) — the $
	// figure is misleading for them.
	var summary string
	switch {
	case shouldShowCost(d):
		summary = fmt.Sprintf("%d%% · %s · %s", d.TokenPct, ctx, d.Cost142)
	case d.PlanTier != "":
		summary = fmt.Sprintf("%d%% · %s · %s", d.TokenPct, ctx, d.PlanTier)
	default:
		summary = fmt.Sprintf("%d%% · %s", d.TokenPct, ctx)
	}
	switch compaction {
	case CompactionDanger:
		summary += " ⛔ compact!"
	case CompactionWarn:
		summary += " ⚠ compaction"
	}
	return wrapPanelCollapsed(s, "usage", summary, "", width, focused)
}

// planTierLabel returns the user-visible plan tier label. Subscription
// users get the human-readable tier (e.g. "Max 5x", "Pro"). API users
// — detected by IsAPIUser, which only flips on after a successful
// "no subscription credentials" probe — read as "api". Unknown /
// pre-fetch state returns empty so the row doesn't render.
func planTierLabel(d MockData) string {
	if d.PlanTier != "" {
		return d.PlanTier
	}
	if d.IsAPIUser {
		return "api"
	}
	return ""
}

// contextWindowLabel returns the context window size label derived from the
// model display name: "1M" for 1M-context variants, "200k" otherwise.
func contextWindowLabel(modelDisplayName string) string {
	if strings.Contains(modelDisplayName, "(1M)") {
		return "1M"
	}
	return "200k"
}
