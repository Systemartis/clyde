# Hero GIF — top of README.
#
# Boot splash → settled dashboard. The splash auto-plays (~2s) and the
# dashboard holds long enough for the mascot to animate and the reader to
# take in the panel grid. No keys pressed — the hero sells the first-run
# experience exactly as a new user sees it.
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

# Hold the settled dashboard: mascot animates, panels breathe.
Sleep 6500ms

Hide
Type "q"
