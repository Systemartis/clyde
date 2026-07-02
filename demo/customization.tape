# Customization pillar — settings overlay: live theme cycling + personas.
#
# Every keypress here is a real binding (see keys.go handleSettingsKey):
#   ? = open settings · ↑↓ = navigate · Enter = cycle value · Esc = save+close
# The cursor opens on the theme row; Enter re-themes the app LIVE.
#
# Regenerate:  vhs demo/customization.tape   (from the repo root)

Output demo/customization.gif

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

# Open the settings overlay — cursor lands on the theme row.
Type "?"
Sleep 2s

# Cycle themes live: Tokyo Night → Catppuccin → Dracula → Gruvbox.
Enter
Sleep 1500ms
Enter
Sleep 1500ms
Enter
Sleep 1500ms

# Swap the mascot persona (meowl → bowl).
Down
Sleep 600ms
Enter
Sleep 1500ms

# Save + close; the dashboard wears the new theme.
Escape
Sleep 3s

Hide
Type "q"
