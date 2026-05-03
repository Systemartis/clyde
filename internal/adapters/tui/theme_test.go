package tui

import (
	"image/color"
	"testing"

	"charm.land/lipgloss/v2"
)

// TestTokyoNightPaletteValues verifies exact Tokyo Night v4 hex values.
func TestTokyoNightPaletteValues(t *testing.T) {
	p := TokyoNightPalette()

	cases := []struct {
		name string
		got  color.Color
		want color.Color
	}{
		{"Bg", p.Bg, lipgloss.Color("#15151f")},
		{"Surface", p.Surface, lipgloss.Color("#1a1b26")},
		{"SurfaceAcc", p.SurfaceAcc, lipgloss.Color("#1f2335")},
		{"BorderDim", p.BorderDim, lipgloss.Color("#232540")},
		{"BorderAcc", p.BorderAcc, lipgloss.Color("#7c5cff")},
		{"Text", p.Text, lipgloss.Color("#c0caf5")},
		{"TextMid", p.TextMid, lipgloss.Color("#a9b1d6")},
		// v22+: brightened from #565f89 / #3b4261 for legibility.
		{"TextDim", p.TextDim, lipgloss.Color("#8a93b8")},
		{"TextFade", p.TextFade, lipgloss.Color("#606b8e")},
		{"Purple", p.Purple, lipgloss.Color("#bb9af7")},
		{"Pink", p.Pink, lipgloss.Color("#ff75a0")},
		{"Cyan", p.Cyan, lipgloss.Color("#7dcfff")},
		{"Green", p.Green, lipgloss.Color("#9ece6a")},
		{"Amber", p.Amber, lipgloss.Color("#e0af68")},
		{"Red", p.Red, lipgloss.Color("#f7768e")},
		{"Orange", p.Orange, lipgloss.Color("#ff9e64")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gr, gg, gb, ga := tc.got.RGBA()
			wr, wg, wb, wa := tc.want.RGBA()
			if gr != wr || gg != wg || gb != wb || ga != wa {
				t.Errorf("Palette.%s RGBA mismatch: got (%d,%d,%d,%d) want (%d,%d,%d,%d)",
					tc.name, gr, gg, gb, ga, wr, wg, wb, wa)
			}
		})
	}
}

// TestClydeDarkPaletteAlias verifies that ClydeDarkPalette returns the same
// values as TokyoNightPalette.
func TestClydeDarkPaletteAlias(t *testing.T) {
	a := ClydeDarkPalette()
	b := TokyoNightPalette()

	check := func(name string, ca, cb color.Color) {
		t.Helper()
		ar, ag, ab, aa := ca.RGBA()
		br, bg, bb, ba := cb.RGBA()
		if ar != br || ag != bg || ab != bb || aa != ba {
			t.Errorf("%s: ClydeDarkPalette != TokyoNightPalette", name)
		}
	}
	check("Bg", a.Bg, b.Bg)
	check("Purple", a.Purple, b.Purple)
	check("Pink", a.Pink, b.Pink)
	check("Green", a.Green, b.Green)
}

// TestThemeRegistryComplete verifies every theme in themeOrder has a palette
// function registered, and PaletteFor returns a non-nil palette for each.
func TestThemeRegistryComplete(t *testing.T) {
	for _, theme := range themeOrder {
		t.Run(string(theme), func(t *testing.T) {
			if !theme.IsValid() {
				t.Fatalf("theme %q is in themeOrder but not registered", theme)
			}
			p := PaletteFor(theme)
			// Sanity: palette must have at least Bg + Text set.
			if p.Bg == nil {
				t.Errorf("theme %q has nil Bg", theme)
			}
			if p.Text == nil {
				t.Errorf("theme %q has nil Text", theme)
			}
			if theme.Display() == "" {
				t.Errorf("theme %q has empty Display label", theme)
			}
		})
	}
}

// TestThemeNextWraps verifies the Next() cycle visits every theme exactly
// once before wrapping back to the first.
func TestThemeNextWraps(t *testing.T) {
	visited := map[Theme]bool{}
	current := themeOrder[0]
	for range themeOrder {
		if visited[current] {
			t.Fatalf("theme %q visited twice — Next() cycle is broken", current)
		}
		visited[current] = true
		current = current.Next()
	}
	if current != themeOrder[0] {
		t.Errorf("after a full cycle, expected to land on %q, got %q", themeOrder[0], current)
	}
}

// TestThemeNextOnUnknownThemeFallsBack verifies that calling Next() on an
// unknown theme returns the first registered theme rather than spinning.
func TestThemeNextOnUnknownThemeFallsBack(t *testing.T) {
	got := Theme("not-a-real-theme").Next()
	if got != themeOrder[0] {
		t.Errorf("Next() on unknown theme = %q, want %q", got, themeOrder[0])
	}
}

// TestPaletteForUnknownThemeFallsBack verifies that PaletteFor returns Tokyo
// Night for an unknown theme rather than panicking.
func TestPaletteForUnknownThemeFallsBack(t *testing.T) {
	got := PaletteFor("not-a-real-theme")
	want := TokyoNightPalette()
	gr, gg, gb, _ := got.Bg.RGBA()
	wr, wg, wb, _ := want.Bg.RGBA()
	if gr != wr || gg != wg || gb != wb {
		t.Errorf("PaletteFor(unknown).Bg = (%d,%d,%d), want Tokyo Night Bg", gr, gg, gb)
	}
}
