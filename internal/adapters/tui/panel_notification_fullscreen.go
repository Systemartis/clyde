package tui

import (
	"image/color"
	"math"
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
)

// renderFullscreenNotification renders the centered animated notification
// overlay used in fullscreen mode. The returned block fills exactly
// width × height cells so callers can drop it into the body region between
// the title bar and the status bar.
//
// Animation is derived from FrameState.Tick:
//   - the heavy chrome (┏━┓ / ┃ / ┗━┛) carries a per-character gradient
//     sampled along the perimeter; tick offsets the sample point so the
//     gradient appears to flow around the card
//   - the lead icon flips between ◆ and ◇
//
// Animation is purely a function of (frame, palette) — no extra model
// state, no extra commands. Same input always renders the same frame, so
// golden tests stay deterministic if we ever add them.
func renderFullscreenNotification(s Styles, p Palette, frame FrameState, decision notificationDecision, width, height int) string {
	// Caller controls width × height — never expand past it or the title
	// bar / status bar would shift. If the body is too small for the
	// minimum-readable card, render an empty block of the requested size
	// rather than overflowing.
	if width < 12 || height < 4 {
		return strings.Repeat(" ", maxInt(width, 0)) + strings.Repeat("\n"+strings.Repeat(" ", maxInt(width, 0)), maxInt(height-1, 0))
	}

	icon := pulseIcon(frame.Tick)

	cardW, cardH := fullscreenCardDims(width, height)
	card := buildFullscreenCard(s, p, frame.Tick, icon, decision, cardW, cardH)

	return centerBlock(card, width, height)
}

// maxInt returns the larger of a and b. Tiny helper used by the
// graceful-degradation path; lipgloss has no equivalent at our v2 pin.
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// fullscreenCardDims computes the card size given the available body area.
// The card sits with breathing room on all sides so the rest of the screen
// dims into the background. Width grows up to a hard cap so the message
// doesn't span an ultra-wide terminal and become hard to scan.
//
// Both axes hard-cap at the available body so the card never drives the
// title bar or status bar out of view.
func fullscreenCardDims(bodyW, bodyH int) (int, int) {
	const maxW = 78
	const targetH = 9

	cardW := bodyW - 8
	if cardW > maxW {
		cardW = maxW
	}
	if cardW > bodyW {
		cardW = bodyW
	}
	if cardW < 12 {
		cardW = bodyW
	}

	cardH := targetH
	if cardH > bodyH {
		cardH = bodyH
	}
	if cardH < 4 {
		cardH = bodyH
	}
	return cardW, cardH
}

// pulseIcon flips between filled / outline diamond glyphs each ~0.4s.
func pulseIcon(tick uint64) string {
	if (tick/8)%2 == 0 {
		return "◆"
	}
	return "◇"
}

// gradientPeriodTicks controls how fast the perimeter gradient scrolls.
// One full cycle every gradientPeriodTicks frames at 50ms each. 60 ticks
// → ~3 s per lap which reads as "alive" without strobing.
const gradientPeriodTicks = 60.0

// gradientFG returns a lipgloss style whose foreground is the gradient
// sampled at perimeter position basePos (0..1) shifted by tick. Bold so
// the heavy box characters render with enough weight to read as a chunky
// chrome instead of thin lines.
func gradientFG(p Palette, basePos float64, tick uint64) lipgloss.Style {
	stops := gradientStops(p)
	t := math.Mod(basePos+float64(tick)/gradientPeriodTicks+1.0, 1.0)
	c := sampleGradient(stops, t)
	return lipgloss.NewStyle().Foreground(c).Bold(true)
}

// sampleGradient returns the color at fractional position t (0..1) along
// the supplied stop list, interpolating linearly between adjacent stops.
// More resolution than picking the nearest stop — the result reads as a
// smooth flow rather than a 4-step staircase.
func sampleGradient(stops []color.Color, t float64) color.Color {
	if len(stops) == 0 {
		return color.RGBA{}
	}
	if len(stops) == 1 {
		return stops[0]
	}
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	pos := t * float64(len(stops)-1)
	i := int(pos)
	if i >= len(stops)-1 {
		return stops[len(stops)-1]
	}
	frac := pos - float64(i)
	return lerpColor(stops[i], stops[i+1], frac)
}

// lerpColor linearly interpolates between two colors in straight RGB.
// RGBA() returns 16-bit pre-multiplied components; we drop the low byte
// before lerping so the result lands in the 0..255 space lipgloss expects.
func lerpColor(a, b color.Color, t float64) color.Color {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	r := uint8((1-t)*float64(ar>>8) + t*float64(br>>8))
	g := uint8((1-t)*float64(ag>>8) + t*float64(bg>>8))
	bl := uint8((1-t)*float64(ab>>8) + t*float64(bb>>8))
	return color.RGBA{R: r, G: g, B: bl, A: 0xff}
}

// buildFullscreenCard renders the bordered notification card. The body
// depends on what triggered the notification:
//   - hook event: tool name, key arg, cwd, and the y/n/esc chips
//   - compaction: a single line warning + the dismiss chip
//
// Chrome uses heavy box-drawing characters (┏━┓ / ┃ / ┗━┛) so the border
// reads as a deliberate frame instead of an afterthought. Each chrome
// character is colored by sampling the palette gradient at its position
// along the perimeter; tick scrolls the sample point so the gradient
// appears to flow continuously around the card.
func buildFullscreenCard(s Styles, p Palette, tick uint64, icon string, decision notificationDecision, w, h int) string {
	dim := lipgloss.NewStyle().Foreground(p.TextDim)
	bodyText := lipgloss.NewStyle().Foreground(p.Text)
	titleStyle := lipgloss.NewStyle().Foreground(p.Text).Bold(true)

	chrome := newCardChrome(p, tick, w, h)
	rows := make([]string, 0, h)

	rows = append(rows, chrome.topBorder())
	rows = append(rows, chrome.blankRow(0))

	rowIdx := 2

	title := titleStyle.Render(icon + "  " + fullscreenTitle(decision) + "  " + icon)
	rows = append(rows, chrome.centeredRow(rowIdx, title))
	rowIdx++
	rows = append(rows, chrome.blankRow(rowIdx))
	rowIdx++

	for _, line := range fullscreenBodyLines(s, p, bodyText, dim, decision, w-6) {
		rows = append(rows, chrome.contentRow(rowIdx, "  "+line))
		rowIdx++
	}

	rows = append(rows, chrome.blankRow(rowIdx))
	rowIdx++

	rows = append(rows, chrome.centeredRow(rowIdx, fullscreenChips(s, decision)))
	rowIdx++

	for rowIdx < h-1 {
		rows = append(rows, chrome.blankRow(rowIdx))
		rowIdx++
	}
	if len(rows) > h-1 {
		rows = rows[:h-1]
	}

	rows = append(rows, chrome.bottomBorder())
	return strings.Join(rows, "\n")
}

// cardChrome is the gradient-aware border builder for the fullscreen
// card. Encapsulates the geometry and tick offset so each border-row
// helper stays terse.
type cardChrome struct {
	palette Palette
	tick    uint64
	width   int // total card width including borders
	height  int // total card height including borders
	innerW  int // card inner width (width - 2)
	perim   int // perimeter character count
}

// newCardChrome builds a chrome for the given card dimensions. width and
// height include the border so a w=80, h=10 card renders 80×10 cells.
func newCardChrome(p Palette, tick uint64, w, h int) cardChrome {
	return cardChrome{
		palette: p,
		tick:    tick,
		width:   w,
		height:  h,
		innerW:  w - 2,
		perim:   2*w + 2*h - 4, // shared corners counted once
	}
}

// perimPosTop maps a top-row column to its perimeter coordinate (0..1).
// Top runs left → right starting at the top-left corner.
func (c cardChrome) perimPosTop(col int) float64 {
	if c.perim <= 0 {
		return 0
	}
	return float64(col) / float64(c.perim)
}

// perimPosRight maps a side row index to its perimeter coordinate. The
// right edge runs top → bottom right after the top-right corner.
func (c cardChrome) perimPosRight(row int) float64 {
	if c.perim <= 0 {
		return 0
	}
	return float64(c.width-1+row) / float64(c.perim)
}

// perimPosBottom maps a bottom-row column to its perimeter coordinate.
// Bottom runs right → left starting at the bottom-right corner.
func (c cardChrome) perimPosBottom(col int) float64 {
	if c.perim <= 0 {
		return 0
	}
	p := c.width - 1 + c.height - 1 + (c.width - 1 - col)
	return float64(p) / float64(c.perim)
}

// perimPosLeft maps a side row index to its perimeter coordinate. The
// left edge runs bottom → top starting at the bottom-left corner, so
// rows further from the bottom land later in the perimeter walk.
func (c cardChrome) perimPosLeft(row int) float64 {
	if c.perim <= 0 {
		return 0
	}
	p := c.width - 1 + c.height - 1 + c.width - 1 + (c.height - 1 - row)
	return float64(p) / float64(c.perim)
}

// topBorder renders the heavy top edge with corner glyphs.
func (c cardChrome) topBorder() string {
	var sb strings.Builder
	sb.WriteString(gradientFG(c.palette, c.perimPosTop(0), c.tick).Render("┏"))
	for i := 1; i < c.width-1; i++ {
		sb.WriteString(gradientFG(c.palette, c.perimPosTop(i), c.tick).Render("━"))
	}
	sb.WriteString(gradientFG(c.palette, c.perimPosTop(c.width-1), c.tick).Render("┓"))
	return sb.String()
}

// bottomBorder mirrors topBorder with ┗━┛.
func (c cardChrome) bottomBorder() string {
	var sb strings.Builder
	sb.WriteString(gradientFG(c.palette, c.perimPosBottom(0), c.tick).Render("┗"))
	for i := 1; i < c.width-1; i++ {
		sb.WriteString(gradientFG(c.palette, c.perimPosBottom(i), c.tick).Render("━"))
	}
	sb.WriteString(gradientFG(c.palette, c.perimPosBottom(c.width-1), c.tick).Render("┛"))
	return sb.String()
}

// sideChars returns the styled left and right ┃ characters for the
// given body row (0-indexed from the top border).
func (c cardChrome) sideChars(row int) (string, string) {
	left := gradientFG(c.palette, c.perimPosLeft(row), c.tick).Render("┃")
	right := gradientFG(c.palette, c.perimPosRight(row), c.tick).Render("┃")
	return left, right
}

// blankRow produces a body row with no content, just chrome on each side.
func (c cardChrome) blankRow(row int) string {
	left, right := c.sideChars(row)
	return left + strings.Repeat(" ", c.innerW) + right
}

// contentRow produces a body row containing the given (left-padded)
// content. padLine guarantees the content fills the inner width exactly.
func (c cardChrome) contentRow(row int, content string) string {
	left, right := c.sideChars(row)
	return left + padLine(content, c.innerW) + right
}

// centeredRow produces a body row with content centered between borders.
func (c cardChrome) centeredRow(row int, content string) string {
	left, right := c.sideChars(row)
	return left + centerLine(content, c.innerW) + right
}

// fullscreenTitle picks the headline for the card based on which
// notification source is active.
func fullscreenTitle(d notificationDecision) string {
	if d.Hook.Active {
		return "claude needs your attention"
	}
	if d.Compaction == CompactionDanger {
		return "compaction imminent"
	}
	if d.Quota.Active {
		if d.Quota.Severity == QuotaSeverityDanger {
			return "quota nearly exhausted"
		}
		return "quota heads-up"
	}
	return "notification"
}

// fullscreenBodyLines returns the body content as a slice of plain lines,
// each guaranteed to fit within maxW visual columns.
func fullscreenBodyLines(_ Styles, p Palette, text, dim lipgloss.Style, d notificationDecision, maxW int) []string {
	if d.Hook.Active {
		verb, target, where := hookPhrase(d.Hook)
		return []string{
			truncateAnsi(text.Render(verb)+" "+lipgloss.NewStyle().Foreground(p.Magenta).Bold(true).Render(target), maxW),
			truncateAnsi(dim.Render("in ")+lipgloss.NewStyle().Foreground(p.Cyan).Render(where), maxW),
		}
	}
	if d.Compaction == CompactionDanger {
		return []string{
			truncateAnsi(text.Render("the context window is above 90% — claude is about to compact this session"), maxW),
		}
	}
	if d.Quota.Active {
		lines := []string{truncateAnsi(text.Render(d.Quota.Headline), maxW)}
		if d.Quota.Detail != "" {
			lines = append(lines, truncateAnsi(dim.Render(d.Quota.Detail), maxW))
		}
		return lines
	}
	return []string{}
}

// hookPhrase returns the (verb, target, cwd) trio used in the body of the
// fullscreen card. Mirrors the banner's verb selection so the user sees
// the same vocabulary regardless of style.
func hookPhrase(h HookNotification) (string, string, string) {
	switch strings.ToLower(h.Tool) {
	case "bash":
		return "wants to run", truncate(h.KeyArg, 60), cwdDisplay(h.Cwd)
	case "edit", "multiedit", "write":
		return "wants to edit", filepath.Base(h.KeyArg), cwdDisplay(h.Cwd)
	case "read":
		return "wants to read", filepath.Base(h.KeyArg), cwdDisplay(h.Cwd)
	default:
		target := h.Tool
		if h.KeyArg != "" {
			target = h.Tool + " " + truncate(h.KeyArg, 50)
		}
		return "wants to call", target, cwdDisplay(h.Cwd)
	}
}

// fullscreenChips renders the action row. Hook notifications expose
// y / n / esc; compaction is informational so only esc is offered.
func fullscreenChips(s Styles, d notificationDecision) string {
	chip := func(label string) string {
		return "[" + s.NotifText.Render(label) + "]"
	}
	if d.Hook.Active {
		return chip("y allow") + "   " + chip("n deny") + "   " + chip("esc dismiss")
	}
	return chip("esc dismiss")
}

// centerLine left/right pads s so its rendered width sits centered within
// width. Falls back to padLine when the content is wider than the budget.
func centerLine(s string, width int) string {
	w := ansiWidth(s)
	if w >= width {
		return padLine(s, width)
	}
	left := (width - w) / 2
	right := width - w - left
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", right)
}

// centerBlock pads card with blank lines top/bottom and spaces left/right
// so it sits centered inside (totalW × totalH).
func centerBlock(card string, totalW, totalH int) string {
	lines := strings.Split(card, "\n")
	cardW := 0
	for _, ln := range lines {
		if w := ansiWidth(ln); w > cardW {
			cardW = w
		}
	}
	leftPad := 0
	if cardW < totalW {
		leftPad = (totalW - cardW) / 2
	}
	leftStr := strings.Repeat(" ", leftPad)

	out := make([]string, 0, totalH)
	topPad := 0
	if len(lines) < totalH {
		topPad = (totalH - len(lines)) / 2
	}
	blank := strings.Repeat(" ", totalW)
	for i := 0; i < topPad; i++ {
		out = append(out, blank)
	}
	for _, ln := range lines {
		padded := leftStr + ln
		if w := ansiWidth(padded); w < totalW {
			padded += strings.Repeat(" ", totalW-w)
		}
		out = append(out, padded)
	}
	for len(out) < totalH {
		out = append(out, blank)
	}
	if len(out) > totalH {
		out = out[:totalH]
	}
	return strings.Join(out, "\n")
}

// truncateAnsi shrinks an already-styled string to fit width visual
// columns. Because lipgloss markup wraps the content with ANSI escapes,
// we measure the printed width and slice the underlying runes if the
// styled string is too long. Falls through unchanged when it already
// fits.
func truncateAnsi(s string, width int) string {
	if ansiWidth(s) <= width {
		return s
	}
	return truncate(s, width)
}
