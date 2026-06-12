package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// hookURLFilePath resolves where the hook server's token-bearing callback
// URL is written: $XDG_CACHE_HOME/clyde/hook-url, falling back to
// ~/.cache/clyde/hook-url. Shared by the live-mode startup (writeHookURLFile)
// and `clyde setup` so the printed snippet can never drift from reality.
func hookURLFilePath() (string, error) {
	cacheHome := os.Getenv("XDG_CACHE_HOME")
	if cacheHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("user home: %w", err)
		}
		cacheHome = filepath.Join(home, ".cache")
	}
	return filepath.Join(cacheHome, "clyde", "hook-url"), nil
}

// runSetup implements `clyde setup`: print, on plain stdout (outside the
// TUI), the hook-notification wiring instructions. Live mode prints its
// startup hints to stderr where the alt-screen TUI immediately covers
// them — this subcommand is the readable path.
func runSetup(w io.Writer) int {
	path, err := hookURLFilePath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "clyde: cannot resolve hook-url path: %v\n", err)
		return 1
	}

	_, _ = fmt.Fprintf(w, `clyde hook notifications — setup

While clyde runs in live mode it writes its callback URL (with a per-run
auth token) to:

    %s

Merge this into ~/.claude/settings.json (the command reads hook-url at
call time, so it survives clyde restarts and port changes):

{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "*",
        "hooks": [
          {
            "type": "command",
            "command": "curl -fsS -m 290 -X POST -H 'Content-Type: application/json' -d @- \"$(cat \"${XDG_CACHE_HOME:-$HOME/.cache}\"/clyde/hook-url)\""
          }
        ]
      }
    ]
  }
}

In the clyde notification: y approves, n denies, Esc denies and dismisses.
If clyde is not running the hook command fails and Claude Code falls back
to its own permission prompt. Full details: README.md → "Hook notifications".
`, path)
	return 0
}
