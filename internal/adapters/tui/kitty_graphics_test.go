package tui

import (
	"os"
	"testing"
)

// TestDetectTerminalGraphics_None verifies that unknown TERM values return GraphicsNone.
func TestDetectTerminalGraphics_None(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("TERM_PROGRAM", "")

	got := DetectTerminalGraphics()
	if got != GraphicsNone {
		t.Errorf("DetectTerminalGraphics() = %d, want GraphicsNone (%d)", got, GraphicsNone)
	}
}

// TestDetectTerminalGraphics_Kitty verifies xterm-kitty returns GraphicsKitty.
func TestDetectTerminalGraphics_Kitty(t *testing.T) {
	t.Setenv("TERM", "xterm-kitty")
	t.Setenv("TERM_PROGRAM", "")

	got := DetectTerminalGraphics()
	if got != GraphicsKitty {
		t.Errorf("DetectTerminalGraphics() = %d, want GraphicsKitty (%d)", got, GraphicsKitty)
	}
}

// TestDetectTerminalGraphics_Ghostty verifies TERM=ghostty returns GraphicsGhostty.
func TestDetectTerminalGraphics_Ghostty(t *testing.T) {
	t.Setenv("TERM", "ghostty")
	t.Setenv("TERM_PROGRAM", "")

	got := DetectTerminalGraphics()
	if got != GraphicsGhostty {
		t.Errorf("DetectTerminalGraphics() = %d, want GraphicsGhostty (%d)", got, GraphicsGhostty)
	}
}

// TestDetectTerminalGraphics_GhosttyXterm verifies TERM=xterm-ghostty returns GraphicsGhostty.
func TestDetectTerminalGraphics_GhosttyXterm(t *testing.T) {
	t.Setenv("TERM", "xterm-ghostty")
	t.Setenv("TERM_PROGRAM", "")

	got := DetectTerminalGraphics()
	if got != GraphicsGhostty {
		t.Errorf("DetectTerminalGraphics() = %d, want GraphicsGhostty (%d)", got, GraphicsGhostty)
	}
}

// TestDetectTerminalGraphics_GhosttyProgram verifies TERM_PROGRAM=ghostty returns GraphicsGhostty.
func TestDetectTerminalGraphics_GhosttyProgram(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("TERM_PROGRAM", "ghostty")

	got := DetectTerminalGraphics()
	if got != GraphicsGhostty {
		t.Errorf("DetectTerminalGraphics() = %d, want GraphicsGhostty (%d)", got, GraphicsGhostty)
	}
}

// TestSupportsKittyGraphics_FalseForUnknown verifies unknown TERM → not supported.
func TestSupportsKittyGraphics_FalseForUnknown(t *testing.T) {
	// Unset TERM to get a clean environment (t.Setenv restores after test)
	orig := os.Getenv("TERM")
	t.Setenv("TERM", "dumb")
	t.Setenv("TERM_PROGRAM", "")
	defer t.Setenv("TERM", orig)

	if SupportsKittyGraphics() {
		t.Error("SupportsKittyGraphics() should return false for TERM=dumb")
	}
}

// TestSupportsKittyGraphics_TrueForKitty verifies Kitty TERM → supported.
func TestSupportsKittyGraphics_TrueForKitty(t *testing.T) {
	t.Setenv("TERM", "xterm-kitty")
	t.Setenv("TERM_PROGRAM", "")

	if !SupportsKittyGraphics() {
		t.Error("SupportsKittyGraphics() should return true for TERM=xterm-kitty")
	}
}
