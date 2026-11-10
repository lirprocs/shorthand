package main

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// Синий акцент заменён зелёным логотипа, остальные цвета — стандартные тёмные.
func TestThemeAccent(t *testing.T) {
	th := darkTheme{}
	for _, n := range []fyne.ThemeColorName{theme.ColorNamePrimary, theme.ColorNameHyperlink} {
		if got := th.Color(n, theme.VariantLight); got != accent {
			t.Errorf("%s = %v, want %v", n, got, accent)
		}
	}
	if got := th.Color(theme.ColorNameFocus, theme.VariantLight); got != accentFocus {
		t.Errorf("focus = %v", got)
	}
	if got := th.Color(theme.ColorNameSelection, theme.VariantLight); got != accentSelection {
		t.Errorf("selection = %v", got)
	}
	for _, n := range []fyne.ThemeColorName{theme.ColorNameBackground, theme.ColorNameForeground, theme.ColorNameButton, theme.ColorNameInputBackground} {
		if got, want := th.Color(n, theme.VariantLight), theme.DefaultTheme().Color(n, theme.VariantDark); got != want {
			t.Errorf("%s = %v, want default dark %v", n, got, want)
		}
	}
}
