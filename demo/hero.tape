# Hero GIF — top of README.
#
# Boot splash → dashboard → expand the two headline panels. The splash
# auto-plays (~2s), the dashboard settles, then the tape jumps to the
# activity panel and the usage panel and expands each (a/u are the real
# panel-jump keys; Enter expands) so the hero shows the app moving instead
# of idling on collapsed rows.
#
# Regenerate:  vhs demo/hero.tape   (from the repo root)

Output demo/hero.gif

Set Shell           "zsh"
Set FontFamily      "JetBrains Mono"
Set FontSize        16
Set Width           1400
Set Height          800
Set Padding         10
Set Theme           "TokyoNight"
Set TypingSpeed     40ms

# Isolate config + cache so recording never touches the maintainer's real
# ~/.config/clyde or ~/.cache/clyde (settings persist on exit!).
Hide
Type "export XDG_CONFIG_HOME=$(mktemp -d) XDG_CACHE_HOME=$(mktemp -d); clear"
Enter
Type "clyde --demo"
Enter
Show

# Boot splash: kitten fades in, waves, CLYDE wordmark types in (~2s).
Sleep 2500ms

# Let the dashboard settle for a beat.
Sleep 2s

# Jump to the activity panel and expand it — the agent tree unfolds.
Type "a"
Sleep 400ms
Enter
Sleep 2500ms

# Jump to the usage panel and expand it — plan bars + reset countdowns.
Type "u"
Sleep 400ms
Enter
Sleep 3500ms

Hide
Type "q"
