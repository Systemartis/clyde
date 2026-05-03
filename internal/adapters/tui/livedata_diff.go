package tui

import (
	"fmt"
	"path/filepath"

	"github.com/clyde-tui/clyde/internal/application/livesession"
)

// deriveDiffLabel builds the "filename · +N −M" label for the diff panel header.
// When DiffFile is set it is used; otherwise "all files" is shown.
func deriveDiffLabel(v livesession.View) string {
	if len(v.DiffHunks) == 0 {
		return ""
	}
	adds, dels := 0, 0
	for _, h := range v.DiffHunks {
		for _, l := range h.Lines {
			switch l.Type {
			case '+':
				adds++
			case '-':
				dels++
			}
		}
	}
	name := filepath.Base(v.DiffFile)
	if name == "" || name == "." {
		name = "all files"
	}
	return fmt.Sprintf("%s · +%d −%d", name, adds, dels)
}

// convertDiffHunks converts livesession.DiffHunk values to the MockData
// []DiffLine format that buildDiffLines renders.
//
// Each hunk becomes one DiffHunkKind header line followed by zero or more
// DiffAddKind/DiffRemKind/DiffCtxKind body lines.
func convertDiffHunks(hunks []livesession.DiffHunk) []DiffLine {
	var out []DiffLine
	for _, h := range hunks {
		out = append(out, DiffLine{
			Kind: DiffHunkKind,
			Text: h.Header,
		})
		newLine := h.NewStart
		oldLine := h.OldStart
		for _, l := range h.Lines {
			switch l.Type {
			case '+':
				out = append(out, DiffLine{
					Kind:   DiffAddKind,
					LineNo: fmt.Sprintf("%d", newLine),
					Text:   l.Text,
				})
				newLine++
			case '-':
				out = append(out, DiffLine{
					Kind:   DiffRemKind,
					LineNo: fmt.Sprintf("%d", oldLine),
					Text:   l.Text,
				})
				oldLine++
			default: // ' ' context
				out = append(out, DiffLine{
					Kind:   DiffCtxKind,
					LineNo: fmt.Sprintf("%d", newLine),
					Text:   l.Text,
				})
				newLine++
				oldLine++
			}
		}
	}
	return out
}
