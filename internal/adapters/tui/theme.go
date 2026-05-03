// Package proto provides the visual prototype of the clyde TUI.
// It runs on mock data only — no real adapters wired.
package tui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Palette holds all semantic color roles for a theme.
// Every style in the UI is derived from this palette — never hardcoded hex.
type Palette struct {
	// Surface
	Bg         color.Color // terminal surface
	Surface    color.Color // panel bg
	SurfaceAcc color.Color // focused panel interior tint

	// Borders
	BorderDim color.Color // default panel border
	BorderAcc color.Color // active/focused panel border

	// Foreground
	Text     color.Color // primary text
	TextMid  color.Color // secondary text
	TextDim  color.Color // metadata, labels
	TextFade color.Color // gutters, disabled

	// Accents
	Purple  color.Color // clyde brand, active substate
	Pink    color.Color // mascot, claude voice ONLY
	Magenta color.Color // expanded-active panel borders
	Cyan    color.Color // paths, info
	Green   color.Color // live, done, success
	Amber   color.Color // modified, warnings
	Red     color.Color // removed, errors, current file
	Orange  color.Color // grep / find tags
}

// Theme identifies a palette in the registry. The string value is what's
// persisted in config.toml so users can edit it by hand.
type Theme string

// Theme constants. Keep ThemeTokyoNight first — it's the historical default.
const (
	ThemeTokyoNight Theme = "tokyo-night"
	ThemeCatppuccin Theme = "catppuccin"
	ThemeDracula    Theme = "dracula"
	ThemeGruvbox    Theme = "gruvbox"
	ThemeNord       Theme = "nord"
	ThemeRosePine   Theme = "rose-pine"
	ThemeKanagawa   Theme = "kanagawa"
)

// themeOrder is the cycle order used by the settings overlay. Adding a
// theme means appending it here AND registering it in palettes below.
var themeOrder = []Theme{
	ThemeTokyoNight,
	ThemeCatppuccin,
	ThemeDracula,
	ThemeGruvbox,
	ThemeNord,
	ThemeRosePine,
	ThemeKanagawa,
}

// palettes is the registry of all available themes. Lookup is via PaletteFor;
// direct map access is discouraged so the fallback-to-default behavior stays
// in one place.
var palettes = map[Theme]func() Palette{
	ThemeTokyoNight: tokyoNightPalette,
	ThemeCatppuccin: catppuccinMochaPalette,
	ThemeDracula:    draculaPalette,
	ThemeGruvbox:    gruvboxDarkPalette,
	ThemeNord:       nordPalette,
	ThemeRosePine:   rosePinePalette,
	ThemeKanagawa:   kanagawaPalette,
}

// IsValid reports whether t is a registered theme.
func (t Theme) IsValid() bool {
	_, ok := palettes[t]
	return ok
}

// Display returns a short human-readable label used in the settings chip.
func (t Theme) Display() string {
	switch t {
	case ThemeTokyoNight:
		return "Tokyo Night"
	case ThemeCatppuccin:
		return "Catppuccin"
	case ThemeDracula:
		return "Dracula"
	case ThemeGruvbox:
		return "Gruvbox"
	case ThemeNord:
		return "Nord"
	case ThemeRosePine:
		return "Rosé Pine"
	case ThemeKanagawa:
		return "Kanagawa"
	}
	return "Tokyo Night"
}

// Next returns the next theme in the registry's cycle order. Wraps from the
// last theme back to the first. Used by the settings overlay's Enter handler.
func (t Theme) Next() Theme {
	for i, candidate := range themeOrder {
		if candidate == t {
			return themeOrder[(i+1)%len(themeOrder)]
		}
	}
	return themeOrder[0]
}

// PaletteFor returns the palette for the given theme. Falls back to Tokyo
// Night when the theme is unknown — keeps the UI rendering even if a user
// hand-edits an invalid value into config.toml.
func PaletteFor(t Theme) Palette {
	if fn, ok := palettes[t]; ok {
		return fn()
	}
	return tokyoNightPalette()
}

// TokyoNightPalette returns the v4 Tokyo Night palette.
// Values are EXACT — do not substitute.
func TokyoNightPalette() Palette { return tokyoNightPalette() }

// ClydeDarkPalette is an alias for TokyoNightPalette kept for API compat.
func ClydeDarkPalette() Palette { return tokyoNightPalette() }

func tokyoNightPalette() Palette {
	return Palette{
		Bg:         lipgloss.Color("#15151f"),
		Surface:    lipgloss.Color("#1a1b26"),
		SurfaceAcc: lipgloss.Color("#1f2335"),
		BorderDim:  lipgloss.Color("#232540"),
		BorderAcc:  lipgloss.Color("#7c5cff"),
		Text:       lipgloss.Color("#c0caf5"),
		TextMid:    lipgloss.Color("#a9b1d6"),
		// Brightened in v22+: original Tokyo Night TextDim/TextFade were
		// hard to read against the dark surface, especially for metadata
		// rows (file stats, durations, hint text). These shift up ~25%
		// lightness while staying clearly subordinate to Text/TextMid.
		TextDim:  lipgloss.Color("#8a93b8"),
		TextFade: lipgloss.Color("#606b8e"),
		Purple:   lipgloss.Color("#bb9af7"),
		Pink:     lipgloss.Color("#ff75a0"),
		Magenta:  lipgloss.Color("#ff5fdb"),
		Cyan:     lipgloss.Color("#7dcfff"),
		Green:    lipgloss.Color("#9ece6a"),
		Amber:    lipgloss.Color("#e0af68"),
		Red:      lipgloss.Color("#f7768e"),
		Orange:   lipgloss.Color("#ff9e64"),
	}
}

// catppuccinMochaPalette returns the Catppuccin Mocha palette.
// https://catppuccin.com — Mauve/Pink warm pastels on a deep blue base.
func catppuccinMochaPalette() Palette {
	return Palette{
		Bg:         lipgloss.Color("#11111b"), // Crust
		Surface:    lipgloss.Color("#1e1e2e"), // Base
		SurfaceAcc: lipgloss.Color("#313244"), // Surface0
		BorderDim:  lipgloss.Color("#313244"), // Surface0
		BorderAcc:  lipgloss.Color("#cba6f7"), // Mauve
		Text:       lipgloss.Color("#cdd6f4"),
		TextMid:    lipgloss.Color("#bac2de"), // Subtext1
		TextDim:    lipgloss.Color("#a6adc8"), // Subtext0
		TextFade:   lipgloss.Color("#7f849c"), // Overlay1
		Purple:     lipgloss.Color("#cba6f7"), // Mauve
		Pink:       lipgloss.Color("#f5c2e7"), // Pink
		Magenta:    lipgloss.Color("#f38ba8"), // Red (used as accent)
		Cyan:       lipgloss.Color("#89dceb"), // Sky
		Green:      lipgloss.Color("#a6e3a1"),
		Amber:      lipgloss.Color("#f9e2af"), // Yellow
		Red:        lipgloss.Color("#f38ba8"),
		Orange:     lipgloss.Color("#fab387"), // Peach
	}
}

// draculaPalette returns the Dracula palette.
// https://draculatheme.com — high-contrast purple/pink on slate.
func draculaPalette() Palette {
	return Palette{
		Bg:         lipgloss.Color("#1e1f29"),
		Surface:    lipgloss.Color("#282a36"),
		SurfaceAcc: lipgloss.Color("#343746"),
		BorderDim:  lipgloss.Color("#44475a"),
		BorderAcc:  lipgloss.Color("#bd93f9"),
		Text:       lipgloss.Color("#f8f8f2"),
		TextMid:    lipgloss.Color("#dcdcd6"),
		TextDim:    lipgloss.Color("#9ea0b0"),
		TextFade:   lipgloss.Color("#6272a4"),
		Purple:     lipgloss.Color("#bd93f9"),
		Pink:       lipgloss.Color("#ff79c6"),
		Magenta:    lipgloss.Color("#ff79c6"),
		Cyan:       lipgloss.Color("#8be9fd"),
		Green:      lipgloss.Color("#50fa7b"),
		Amber:      lipgloss.Color("#f1fa8c"),
		Red:        lipgloss.Color("#ff5555"),
		Orange:     lipgloss.Color("#ffb86c"),
	}
}

// gruvboxDarkPalette returns the Gruvbox Dark palette.
// https://github.com/morhetz/gruvbox — warm earthy retro.
func gruvboxDarkPalette() Palette {
	return Palette{
		Bg:         lipgloss.Color("#1d2021"),
		Surface:    lipgloss.Color("#282828"),
		SurfaceAcc: lipgloss.Color("#32302f"),
		BorderDim:  lipgloss.Color("#3c3836"),
		BorderAcc:  lipgloss.Color("#d3869b"),
		Text:       lipgloss.Color("#ebdbb2"),
		TextMid:    lipgloss.Color("#d5c4a1"),
		TextDim:    lipgloss.Color("#bdae93"),
		TextFade:   lipgloss.Color("#928374"),
		Purple:     lipgloss.Color("#d3869b"),
		Pink:       lipgloss.Color("#fb4934"),
		Magenta:    lipgloss.Color("#d3869b"),
		Cyan:       lipgloss.Color("#8ec07c"),
		Green:      lipgloss.Color("#b8bb26"),
		Amber:      lipgloss.Color("#fabd2f"),
		Red:        lipgloss.Color("#fb4934"),
		Orange:     lipgloss.Color("#fe8019"),
	}
}

// nordPalette returns the Nord palette.
// https://www.nordtheme.com — cool arctic blues with muted accents.
func nordPalette() Palette {
	return Palette{
		Bg:         lipgloss.Color("#2e3440"), // Polar Night 0
		Surface:    lipgloss.Color("#3b4252"), // Polar Night 1
		SurfaceAcc: lipgloss.Color("#434c5e"), // Polar Night 2
		BorderDim:  lipgloss.Color("#4c566a"), // Polar Night 3
		BorderAcc:  lipgloss.Color("#88c0d0"), // Frost 1
		Text:       lipgloss.Color("#eceff4"), // Snow Storm 2
		TextMid:    lipgloss.Color("#d8dee9"), // Snow Storm 0
		TextDim:    lipgloss.Color("#a3acb9"),
		TextFade:   lipgloss.Color("#6f7a90"),
		Purple:     lipgloss.Color("#b48ead"), // Aurora purple
		Pink:       lipgloss.Color("#d08770"), // Aurora orange (warm-mascot)
		Magenta:    lipgloss.Color("#b48ead"),
		Cyan:       lipgloss.Color("#88c0d0"),
		Green:      lipgloss.Color("#a3be8c"),
		Amber:      lipgloss.Color("#ebcb8b"),
		Red:        lipgloss.Color("#bf616a"),
		Orange:     lipgloss.Color("#d08770"),
	}
}

// rosePinePalette returns the Rosé Pine main palette.
// https://rosepinetheme.com — soho-vibes bedroom-purples.
func rosePinePalette() Palette {
	return Palette{
		Bg:         lipgloss.Color("#191724"),
		Surface:    lipgloss.Color("#1f1d2e"),
		SurfaceAcc: lipgloss.Color("#26233a"),
		BorderDim:  lipgloss.Color("#403d52"),
		BorderAcc:  lipgloss.Color("#c4a7e7"), // Iris
		Text:       lipgloss.Color("#e0def4"),
		TextMid:    lipgloss.Color("#cdcbe0"),
		TextDim:    lipgloss.Color("#908caa"),
		TextFade:   lipgloss.Color("#6e6a86"),
		Purple:     lipgloss.Color("#c4a7e7"), // Iris
		Pink:       lipgloss.Color("#ebbcba"), // Rose
		Magenta:    lipgloss.Color("#eb6f92"), // Love
		Cyan:       lipgloss.Color("#9ccfd8"), // Foam
		Green:      lipgloss.Color("#a6e3a1"),
		Amber:      lipgloss.Color("#f6c177"), // Gold
		Red:        lipgloss.Color("#eb6f92"), // Love
		Orange:     lipgloss.Color("#ea9a97"),
	}
}

// kanagawaPalette returns the Kanagawa Wave palette.
// https://github.com/rebelot/kanagawa.nvim — Hokusai-inspired warm nights.
func kanagawaPalette() Palette {
	return Palette{
		Bg:         lipgloss.Color("#16161d"), // sumiInk0
		Surface:    lipgloss.Color("#1f1f28"), // sumiInk2
		SurfaceAcc: lipgloss.Color("#2a2a37"), // sumiInk3
		BorderDim:  lipgloss.Color("#363646"), // sumiInk4
		BorderAcc:  lipgloss.Color("#957fb8"), // oniViolet
		Text:       lipgloss.Color("#dcd7ba"), // fujiWhite
		TextMid:    lipgloss.Color("#c8c093"), // oldWhite
		TextDim:    lipgloss.Color("#a6a69c"),
		TextFade:   lipgloss.Color("#727169"), // fujiGray
		Purple:     lipgloss.Color("#957fb8"), // oniViolet
		Pink:       lipgloss.Color("#d27e99"), // sakuraPink
		Magenta:    lipgloss.Color("#b8b4d0"),
		Cyan:       lipgloss.Color("#7fb4ca"), // springBlue
		Green:      lipgloss.Color("#98bb6c"), // springGreen
		Amber:      lipgloss.Color("#e6c384"), // carpYellow
		Red:        lipgloss.Color("#e46876"), // waveRed
		Orange:     lipgloss.Color("#ffa066"), // surimiOrange
	}
}
