package git

import (
	"github.com/clyde-tui/clyde/internal/application/livesession"
)

// LiveSessionAdapter wraps Source and implements livesession.GitSource.
type LiveSessionAdapter struct {
	src *Source
}

// NewLiveSessionAdapter returns a LiveSessionAdapter that satisfies
// livesession.GitSource. Creates its own Source — for production code
// that needs to share a cache between LiveSessionAdapter and DiffAdapter,
// use NewLiveSessionAdapterFor.
func NewLiveSessionAdapter() *LiveSessionAdapter {
	return &LiveSessionAdapter{src: &Source{}}
}

// NewLiveSessionAdapterFor wraps an existing Source so the LiveSession
// and Diff adapters can share the same status/diff cache.
func NewLiveSessionAdapterFor(src *Source) *LiveSessionAdapter {
	if src == nil {
		src = &Source{}
	}
	return &LiveSessionAdapter{src: src}
}

// StatusView implements livesession.GitSource.
// It translates git.FileStatus values to livesession.GitFileStatus so the
// application layer has no dependency on the adapter package.
func (a *LiveSessionAdapter) StatusView(cwd string) ([]livesession.GitFileStatus, error) {
	raw, err := a.src.Status(cwd)
	if err != nil {
		return nil, err
	}
	out := make([]livesession.GitFileStatus, len(raw))
	for i, s := range raw {
		out[i] = livesession.GitFileStatus{
			Path:   s.Path,
			Status: s.Status,
			Staged: s.Staged,
		}
	}
	return out, nil
}

// compile-time check: LiveSessionAdapter must satisfy livesession.GitSource.
var _ livesession.GitSource = (*LiveSessionAdapter)(nil)

// DiffAdapter wraps Source and converts git.Hunk values to livesession.DiffHunk
// so the proto TUI layer's DiffSource interface can be satisfied without the
// proto package importing the adapters/git package directly.
type DiffAdapter struct {
	src *Source
}

// NewDiffAdapter returns a DiffAdapter ready to use. Creates its own
// Source — for production code that needs to share a cache between
// LiveSessionAdapter and DiffAdapter, use NewDiffAdapterFor.
func NewDiffAdapter() *DiffAdapter {
	return &DiffAdapter{src: &Source{}}
}

// NewDiffAdapterFor wraps an existing Source so the LiveSession and
// Diff adapters can share the same status/diff cache.
func NewDiffAdapterFor(src *Source) *DiffAdapter {
	if src == nil {
		src = &Source{}
	}
	return &DiffAdapter{src: src}
}

// Diff implements proto.DiffSource.
// It calls git.Source.Diff and translates the result to livesession.DiffHunk
// values so the adapters/git package is not imported by the proto package.
func (a *DiffAdapter) Diff(cwd, file string) ([]livesession.DiffHunk, error) {
	raw, err := a.src.Diff(cwd, file)
	if err != nil || len(raw) == 0 {
		return nil, err //nolint:nilerr
	}
	out := make([]livesession.DiffHunk, len(raw))
	for i, h := range raw {
		lines := make([]livesession.DiffHunkLine, len(h.Lines))
		for j, l := range h.Lines {
			lines[j] = livesession.DiffHunkLine{Type: l.Type, Text: l.Text}
		}
		out[i] = livesession.DiffHunk{
			Header:   h.Header,
			OldStart: h.OldStart,
			OldCount: h.OldCount,
			NewStart: h.NewStart,
			NewCount: h.NewCount,
			Lines:    lines,
		}
	}
	return out, nil
}
