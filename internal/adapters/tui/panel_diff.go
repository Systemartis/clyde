package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
)

// renderDiffExpanded renders the diff panel in expanded state.
// When activeMode is true, content flows through the viewport (real scrolling)
// with a pink double border. Otherwise renders directly.
func renderDiffExpanded(s Styles, p Palette, d MockData, vp viewport.Model, width, height int, focused, activeMode bool) string {
	if activeMode {
		inner := width - 4
		vp.SetWidth(inner)
		vp.SetHeight(height - 2)
		content := vp.View()
		return wrapPanelActive(s, content, "live diff", width, height)
	}
	return renderDiff(s, p, d, width, height, focused)
}

// renderDiff renders the live diff panel.
func renderDiff(s Styles, _ Palette, d MockData, width, height int, focused bool) string {
	inner := width - 4 // border + 1-char padding each side
	var content string
	if len(d.DiffLines) == 0 && d.DiffFile == "" {
		// Empty state: show a centered informational message.
		msg := diffEmptyStateMsg(d)
		content = centerInArea(s.TextFade.Render(msg), inner, height-2)
	} else {
		content = buildDiffLines(s, d.DiffLines, inner)
	}
	return wrapPanel(s, content, "live diff", d.DiffFile, width, height, focused)
}

// diffEmptyStateMsg returns the appropriate empty-state message for the diff panel.
// When IsGitRepo is false: "not a git repository".
// When true but no changes: "no changes".
func diffEmptyStateMsg(d MockData) string {
	if !d.IsGitRepo {
		return "not a git repository"
	}
	return "no changes"
}

// centerInArea pads text to appear vertically centered within height rows,
// each row at the given width. Returns a single string with newlines.
func centerInArea(text string, width, height int) string {
	if height <= 0 {
		return text
	}
	topPad := height / 2
	lines := make([]string, 0, height)
	for range topPad {
		lines = append(lines, strings.Repeat(" ", width))
	}
	lines = append(lines, centerText(text, width))
	return strings.Join(lines, "\n")
}

// centerText centers s within the given width using space padding.
func centerText(s string, width int) string {
	visible := ansiWidth(s)
	if visible >= width {
		return s
	}
	pad := (width - visible) / 2
	return strings.Repeat(" ", pad) + s
}

// buildDiffViewportContent builds the full diff content string for viewport.SetContent().
// Rendered at a fixed inner width; the viewport handles clipping and scroll.
func buildDiffViewportContent(s Styles, d MockData) string {
	return buildDiffLines(s, d.DiffLines, 76)
}

// buildDiffLines renders all diff lines at the given inner width as a single string.
// Shared by renderDiff and buildDiffViewportContent to avoid duplication.
func buildDiffLines(s Styles, lines []DiffLine, inner int) string {
	var sb strings.Builder
	for _, dl := range lines {
		var line string
		switch dl.Kind {
		case DiffHunkKind:
			line = s.DiffHunk.Render(truncate(dl.Text, inner))
		case DiffCtxKind:
			num := s.DiffLineNum.Render(fmt.Sprintf("%3s", dl.LineNo))
			text := s.DiffCtx.Render(truncate(dl.Text, inner-5))
			line = num + " " + text
		case DiffAddKind:
			num := s.DiffLineNum.Render(fmt.Sprintf("%3s", dl.LineNo))
			text := s.DiffAdd.Render(truncate(dl.Text, inner-5))
			line = num + " " + text
		case DiffRemKind:
			num := s.DiffLineNum.Render(fmt.Sprintf("%3s", dl.LineNo))
			text := s.DiffRem.Render(truncate(dl.Text, inner-5))
			line = num + " " + text
		}
		sb.WriteString(line)
		sb.WriteByte('\n')
	}
	return strings.TrimRight(sb.String(), "\n")
}

// renderDiffCollapsed renders the collapsed one-liner for the diff panel.
func renderDiffCollapsed(s Styles, d MockData, width int, focused bool) string {
	summary := d.DiffFile
	return wrapPanelCollapsed(s, "diff", summary, "", width, focused)
}
