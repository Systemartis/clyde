# Workspace pillar — explorer / fuzzy search / viewer / editor.
#
# Every keypress here is a real binding (see keys.go):
#   e = jump to explorer · Enter = active mode / open
#   / = fuzzy file search · f = fullscreen viewer
#   / (in viewer) = find, n = next match · i = insert mode · : = ex command
#
# Regenerate:  vhs demo/workspace.tape   (from the repo root)

Output demo/workspace.gif

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

# Jump into the explorer and activate it.
Type "e"
Sleep 600ms
Enter
Sleep 1s

# Fuzzy-search the project for "auth" and open the source file.
Type "/"
Sleep 500ms
Type "auth"
Sleep 1200ms
Down
Sleep 600ms
Enter
Sleep 2s

# Fullscreen the viewer — syntax-highlighted, vim navigation.
Type "f"
Sleep 2s

# Find "token" and hop between matches.
Type "/"
Sleep 400ms
Type "token"
Enter
Sleep 1200ms
Type "n"
Sleep 800ms
Type "n"
Sleep 1s

# Drop into insert mode and type — a real editor, undo/redo included.
Type "i"
Sleep 800ms
Type "// reviewed with clyde"
Sleep 1500ms

# Vim-style bail out: Esc to view mode, :q! to discard.
Escape
Sleep 800ms
Type ":"
Sleep 400ms
Type "q!"
Sleep 400ms
Enter
Sleep 1500ms

Hide
Type "q"
