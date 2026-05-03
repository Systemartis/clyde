package tui

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update", false, "update golden files")

// stripANSI removes ANSI escape sequences from a string for golden file comparison.
// This allows the golden file to be human-readable plain text.
func stripANSI(s string) string {
	var out strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			// Skip until 'm'
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			if j < len(s) {
				i = j + 1
			} else {
				i = j
			}
			continue
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String()
}

// checkGolden compares plain (ANSI-stripped) content against a golden file.
// If -update is passed, the golden file is written/updated.
func checkGolden(t *testing.T, goldenPath, plain string) {
	t.Helper()

	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(goldenPath, []byte(plain), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("golden file updated: %s", goldenPath)
		return
	}

	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (run with -update to create): %v", err)
	}

	if plain != string(golden) {
		wantLines := strings.Split(string(golden), "\n")
		gotLines := strings.Split(plain, "\n")
		maxLines := len(wantLines)
		if len(gotLines) > maxLines {
			maxLines = len(gotLines)
		}
		for i := 0; i < maxLines; i++ {
			var w, g string
			if i < len(wantLines) {
				w = wantLines[i]
			}
			if i < len(gotLines) {
				g = gotLines[i]
			}
			if w != g {
				t.Errorf("line %d mismatch:\n  want: %q\n  got:  %q", i+1, w, g)
				if t.Failed() && i > 5 {
					t.Log("... (stopping at first few mismatches)")
					break
				}
			}
		}
	}
}

// TestProtoView_StackNarrow70x40 renders the stack layout at 70×40 (narrow)
// and compares it to the golden file.
func TestProtoView_StackNarrow70x40(t *testing.T) {
	m := NewModel() // defaults to Stack mode
	m.width = 70
	m.height = 40
	m.bp = DetectBreakpoint(70)
	// Ensure default collapse state is properly initialized
	m.focused = PanelNow
	m.collapse[PanelNow].Expand()

	plain := stripANSI(m.View().Content)
	checkGolden(t, filepath.Join("testdata", "stack-narrow-70x40.golden"), plain)
}

// TestProtoView_StackMedium90x40 renders the 2-column layout at 90×40 (medium —
// explorer+servers left, now/tasks/diff/usage right). v7: images panel removed.
func TestProtoView_StackMedium90x40(t *testing.T) {
	m := NewModel()
	m.width = 90
	m.height = 40
	m.bp = DetectBreakpoint(90)
	m.focused = PanelNow
	m.collapse[PanelNow].Expand()

	plain := stripANSI(m.View().Content)
	checkGolden(t, filepath.Join("testdata", "stack-medium-90x40.golden"), plain)
}

// TestProtoView_StackMedium110x40 renders the 2-column layout at 110×40 (medium —
// explorer+servers left, now/tasks/diff/usage right).
func TestProtoView_StackMedium110x40(t *testing.T) {
	m := NewModel()
	m.width = 110
	m.height = 40
	m.bp = DetectBreakpoint(110)
	m.focused = PanelNow
	m.collapse[PanelNow].Expand()

	plain := stripANSI(m.View().Content)
	checkGolden(t, filepath.Join("testdata", "stack-medium-110x40.golden"), plain)
}

// TestProtoView_StackMedium130x40 renders the 2-column layout at 130×40 —
// the canonical medium viewport showing the new 2-col layout + new mascot + cleaner title bar.
func TestProtoView_StackMedium130x40(t *testing.T) {
	m := NewModel()
	m.width = 130
	m.height = 40
	m.bp = DetectBreakpoint(130)
	m.focused = PanelNow
	m.collapse[PanelNow].Expand()

	plain := stripANSI(m.View().Content)
	checkGolden(t, filepath.Join("testdata", "stack-medium-130x40.golden"), plain)
}

// TestProtoView_MultiCol180x50 renders the multi-column layout at 180×50
// (the design-target size, three-column dashboard).
func TestProtoView_MultiCol180x50(t *testing.T) {
	m := NewModelWithConfig(DefaultConfig(), LayoutMultiCol)
	m.width = 180
	m.height = 50
	m.bp = DetectBreakpoint(180)

	plain := stripANSI(m.View().Content)
	checkGolden(t, filepath.Join("testdata", "multicol-180x50.golden"), plain)
}

// TestProtoView_ViewerText130x40 renders the viewer mode (text file) at 130×40.
// Explorer+servers on left; auth.ts file content in viewer on right.
func TestProtoView_ViewerText130x40(t *testing.T) {
	m := NewModel()
	m.width = 130
	m.height = 40
	m.bp = DetectBreakpoint(130)
	m.focused = PanelExplorer
	// Open auth.ts in the viewer
	m.viewerActive = true
	m.viewerFile = "src/api/auth.ts"
	m.collapse[PanelNow].Expand()

	plain := stripANSI(m.View().Content)
	checkGolden(t, filepath.Join("testdata", "viewer-text-130x40.golden"), plain)
}

// TestProtoView_ViewerImageFallback130x40 renders the viewer mode (image, ASCII fallback) at 130×40.
func TestProtoView_ViewerImageFallback130x40(t *testing.T) {
	m := NewModel()
	m.width = 130
	m.height = 40
	m.bp = DetectBreakpoint(130)
	m.focused = PanelExplorer
	// Open logo.png in the viewer
	m.viewerActive = true
	m.viewerFile = "public/logo.png"
	m.collapse[PanelNow].Expand()

	plain := stripANSI(m.View().Content)
	checkGolden(t, filepath.Join("testdata", "viewer-image-fallback-130x40.golden"), plain)
}

// TestProtoView_StackMedium130x40Active renders the 2-column layout at 130×40
// with the tasks panel in Expanded-Active state — shows pink double border + mode badge.
func TestProtoView_StackMedium130x40Active(t *testing.T) {
	m := NewModel()
	m.width = 130
	m.height = 40
	m.bp = DetectBreakpoint(130)
	m.focused = PanelCalls
	// Use a pre-settled expanded spring (startCollapsed=false starts at expandedH).
	// This avoids waiting for spring animation to converge during the golden render.
	m.collapse[PanelCalls] = NewPanelCollapseState(false, 18)
	m.panelHeights[PanelCalls] = 18 // force expanded height
	// Put tasks panel in Expanded-Active state
	m.activePanelID = PanelCalls
	// Wire viewport content for scrolling
	m = m.syncPanelViewport(PanelCalls)
	m.panelVPs[PanelCalls].SetHeight(14) // approximate viewport height for the panel

	plain := stripANSI(m.View().Content)
	checkGolden(t, filepath.Join("testdata", "stack-medium-130x40-active.golden"), plain)
}

// TestProtoView_UsageActive130x40 renders the 2-column layout at 130×40
// with the usage panel in Expanded-Active state — verifies usage content is
// readable in active mode (v14 fix: correct inner width passed to build function).
func TestProtoView_UsageActive130x40(t *testing.T) {
	m := NewModel()
	m.width = 130
	m.height = 40
	m.bp = DetectBreakpoint(130)
	m.focused = PanelUsage
	// Settle PanelNow at fixed height 6 (renderNow always uses panelH=6)
	m.collapse[PanelNow] = NewPanelCollapseState(false, 6)
	// Collapse calls and diff so usage is visible in the 40-row layout
	m.collapse[PanelCalls] = NewPanelCollapseState(true, 18)
	m.collapse[PanelDiff] = NewPanelCollapseState(true, 10)
	// Usage panel expanded and active
	m.collapse[PanelUsage] = NewPanelCollapseState(false, 14)
	m.panelHeights[PanelUsage] = 14
	m.activePanelID = PanelUsage
	// Wire viewport content for scrolling
	m = m.syncPanelViewport(PanelUsage)
	m.panelVPs[PanelUsage].SetHeight(10)

	plain := stripANSI(m.View().Content)
	checkGolden(t, filepath.Join("testdata", "usage-active-130x40.golden"), plain)
}

// TestProtoView_180x50 is kept for backward compat with the old golden.
// It now generates the multicol-180x50 golden.
func TestProtoView_180x50(t *testing.T) {
	m := NewModelWithConfig(DefaultConfig(), LayoutMultiCol)
	m.width = 180
	m.height = 50
	m.bp = DetectBreakpoint(180)

	plain := stripANSI(m.View().Content)

	goldenPath := filepath.Join("testdata", "view-180x50.golden")

	if *updateGolden {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(goldenPath, []byte(plain), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("golden file updated: %s", goldenPath)
		return
	}

	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (run with -update to create): %v", err)
	}

	if plain != string(golden) {
		wantLines := strings.Split(string(golden), "\n")
		gotLines := strings.Split(plain, "\n")
		maxLines := len(wantLines)
		if len(gotLines) > maxLines {
			maxLines = len(gotLines)
		}
		for i := 0; i < maxLines; i++ {
			var w, g string
			if i < len(wantLines) {
				w = wantLines[i]
			}
			if i < len(gotLines) {
				g = gotLines[i]
			}
			if w != g {
				t.Errorf("line %d mismatch:\n  want: %q\n  got:  %q", i+1, w, g)
				if t.Failed() && i > 5 {
					t.Log("... (stopping at first few mismatches)")
					break
				}
			}
		}
	}
}
