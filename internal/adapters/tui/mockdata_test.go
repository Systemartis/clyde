package tui

import (
	"strings"
	"testing"
)

// TestDemoMockContent_OpenableEntriesResolve verifies that every file a user
// can open in demo mode — the "modified" list, tree leaves, and search results
// (which draw from the same paths) — resolves to real mock content instead of
// the "(no mock content …)" placeholder.
func TestDemoMockContent_OpenableEntriesResolve(t *testing.T) {
	t.Parallel()
	d := V3MockData()

	var paths []string
	for _, mf := range d.ModifiedFiles {
		paths = append(paths, mf.Path)
	}
	for _, n := range d.Tree {
		if n.IsDir || n.FullPath == "" {
			continue // directories toggle expansion; they don't open in the viewer
		}
		paths = append(paths, n.FullPath)
	}

	for _, p := range paths {
		if isImageFile(p) {
			continue // images render via the ASCII placeholder, not mockFileContent
		}
		if _, ok := mockFileContent[normalizePath(p)]; !ok {
			t.Errorf("openable demo path %q has no mock content entry", p)
		}
	}
}

// TestDemoMockContent_ModifiedFilesRenderContent renders each modified file in
// demo mode and asserts the viewer never shows the missing-content placeholder.
func TestDemoMockContent_ModifiedFilesRenderContent(t *testing.T) {
	t.Parallel()
	d := V3MockData()
	for _, mf := range d.ModifiedFiles {
		m := NewModel() // demoMode defaults to true
		m = m.loadViewerFile(mf.Path)
		out := stripANSI(m.renderViewerPanel(120, 30))
		if strings.Contains(out, "no mock content") {
			t.Errorf("modified file %q shows missing-content placeholder", mf.Path)
		}
	}
}
