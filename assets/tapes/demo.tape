# clyde hero demo — renders assets/demo.gif
#
# Re-record from the repo root:   make demo-media   (or: vhs assets/tapes/demo.tape)
# Runs against `clyde --demo` (deterministic mock data, no live reads).

Output assets/demo.gif

Set Shell "bash"
Set FontSize 13
Set Width 1500
Set Height 860
Set Padding 16
Set CursorBlink false

# Tokyo Night — matches clyde's in-app palette.
Set Theme { "name": "Tokyo Night", "background": "#1a1b26", "foreground": "#c0caf5", "cursor": "#c0caf5", "selection": "#283457", "black": "#15161e", "red": "#f7768e", "green": "#9ece6a", "yellow": "#e0af68", "blue": "#7aa2f7", "magenta": "#bb9af7", "cyan": "#7dcfff", "white": "#a9b1d6", "brightBlack": "#414868", "brightRed": "#f7768e", "brightGreen": "#9ece6a", "brightYellow": "#e0af68", "brightBlue": "#7aa2f7", "brightMagenta": "#bb9af7", "brightCyan": "#7dcfff", "brightWhite": "#c0caf5" }

# Build + launch off-screen so the GIF opens directly on the dashboard.
# HOME is isolated so the recording never picks up a local config.toml.
Hide
Type "mkdir -p /tmp/clyde-vhs-home && go build -o /tmp/clyde-vhs ./cmd/clyde && clear && HOME=/tmp/clyde-vhs-home /tmp/clyde-vhs --demo"
Enter
Sleep 6s
# Default layout is stack; cycle stack → tabs → multi-col for the showcase.
Ctrl+L
Sleep 500ms
Ctrl+L
Sleep 1s
Show

# Dashboard glance — 3-column layout with boot settled.
Sleep 4s

# Walk focus across panels.
Tab
Sleep 1s
Tab
Sleep 1s

# Help overlay.
Type "h"
Sleep 2.5s
Escape
Sleep 1s

# Enter active mode on the focused panel and scroll.
Enter
Sleep 1s
Down
Sleep 250ms
Down
Sleep 250ms
Down
Sleep 250ms
Down
Sleep 250ms
Down
Sleep 1.5s
Escape

# Hold the final dashboard frame.
Sleep 3s
