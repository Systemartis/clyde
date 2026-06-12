# clyde still screenshots — renders assets/screen-*.png
#
# Re-record from the repo root:   vhs assets/tapes/screens.tape
# The scratch GIF is deleted after the run; only the Screenshot frames are kept.

Output assets/screens-scratch.gif

Set Shell "bash"
Set FontSize 13
Set Width 1500
Set Height 860
Set Padding 16
Set CursorBlink false

Set Theme { "name": "Tokyo Night", "background": "#1a1b26", "foreground": "#c0caf5", "cursor": "#c0caf5", "selection": "#283457", "black": "#15161e", "red": "#f7768e", "green": "#9ece6a", "yellow": "#e0af68", "blue": "#7aa2f7", "magenta": "#bb9af7", "cyan": "#7dcfff", "white": "#a9b1d6", "brightBlack": "#414868", "brightRed": "#f7768e", "brightGreen": "#9ece6a", "brightYellow": "#e0af68", "brightBlue": "#7aa2f7", "brightMagenta": "#bb9af7", "brightCyan": "#7dcfff", "brightWhite": "#c0caf5" }

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

# 1 — full dashboard.
Sleep 3s
Screenshot assets/screen-dashboard.png

# 2 — help overlay.
Type "h"
Sleep 1.5s
Screenshot assets/screen-help.png
Escape
Sleep 1s

# 3 — active mode (scrollable panel, pink double border).
Enter
Sleep 1s
Down
Sleep 200ms
Down
Sleep 1s
Screenshot assets/screen-active.png
Escape
Sleep 1s
