package tui

import "os"

// TerminalGraphics describes the image rendering capability of the current terminal.
type TerminalGraphics int

const (
	// GraphicsNone — no inline image support; use ASCII fallback.
	GraphicsNone TerminalGraphics = iota
	// GraphicsKitty — Kitty graphics protocol (APC _G sequences).
	GraphicsKitty
	// GraphicsGhostty — Ghostty terminal (also supports Kitty graphics).
	GraphicsGhostty
)

// DetectTerminalGraphics checks the $TERM and $TERM_PROGRAM environment
// variables to determine whether Kitty graphics protocol is available.
//
// Supported values:
//
//	TERM=xterm-kitty            → Kitty
//	TERM=ghostty                → Ghostty (Kitty-compatible)
//	TERM=xterm-ghostty          → Ghostty
//	TERM_PROGRAM=ghostty        → Ghostty
func DetectTerminalGraphics() TerminalGraphics {
	term := os.Getenv("TERM")
	termProg := os.Getenv("TERM_PROGRAM")

	switch term {
	case "xterm-kitty":
		return GraphicsKitty
	case "ghostty", "xterm-ghostty":
		return GraphicsGhostty
	}

	if termProg == "ghostty" {
		return GraphicsGhostty
	}

	return GraphicsNone
}

// SupportsKittyGraphics returns true if the terminal supports the Kitty
// graphics protocol (either native Kitty or Ghostty in Kitty-compat mode).
func SupportsKittyGraphics() bool {
	g := DetectTerminalGraphics()
	return g == GraphicsKitty || g == GraphicsGhostty
}

// NOTE: renderKittyImage (APC sequence emitter) is intentionally deferred to v7.
// Emitting raw APC sequences inside lipgloss-composed strings causes width
// miscalculation and border corruption because lipgloss uses ANSI escape
// scanning that stops at ESC+\ but doesn't account for the APC payload width.
//
// v7 plan: render the image OUTSIDE the lipgloss composition pass by writing
// the APC bytes directly to the terminal before the View() string is printed,
// using absolute cursor positioning (CSI row;col H) to place the image in the
// correct panel area. This requires knowing the panel's screen coordinates,
// which needs a layout-to-screen-coordinate mapping pass first.
//
// For v6, ASCII fallback is always used. See renderASCIIImagePlaceholder().
