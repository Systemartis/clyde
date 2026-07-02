# Observability pillar — activity / usage / bash / cache + hook notification.
#
# Every keypress here is a real binding (see keys.go):
#   a/u/b/c = panel jumps · Enter = active mode · ↑↓ = scroll
#   ⌃n = one-shot demo hook notification · y = allow
#
# Regenerate:  vhs demo/observability.tape   (from the repo root)

Output demo/observability.gif

Set Shell           "zsh"
Set FontFamily      "JetBrains Mono"
Set FontSize        16
Set Width           1400
Set Height          800
Set Padding         10
Set Theme           "TokyoNight"
Set TypingSpeed     80ms

Hide
Type "export XDG_CONFIG_HOME=$(mktemp -d) XDG_CACHE_HOME=$(mktemp -d); clear"
Enter
Type "clyde --demo"
Enter
Sleep 4s
Show

# Jump to the activity panel and enter active mode — the agent tree with
# per-subagent tool calls is the heart of clyde.
Type "a"
Sleep 800ms
Enter
Sleep 1500ms
Down
Sleep 400ms
Down
Sleep 400ms
Down
Sleep 1200ms
Backspace
Sleep 600ms

# Usage: plan quota (5h / weekly) + context fill + burn rate.
Type "u"
Sleep 2500ms

# Bash ledger: every command claude ran, with duration + failures.
Type "b"
Sleep 2s

# Cache efficiency: prompt-cache hit ratio trend.
Type "c"
Sleep 2s

# A claude permission request arrives (demo preview) — approve it with y.
Ctrl+n
Sleep 2500ms
Type "y"
Sleep 1500ms

Hide
Type "q"
