package main

import (
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// darkTheme — стандартная тема Fyne, всегда в тёмном варианте, независимо
// от настройки Windows «Режим приложения по умолчанию».
type darkTheme struct{}

var _ fyne.Theme = darkTheme{}

func (darkTheme) Color(n fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	return theme.DefaultTheme().Color(n, theme.VariantDark)
}

func (darkTheme) Font(s fyne.TextStyle) fyne.Resource     { return theme.DefaultTheme().Font(s) }
func (darkTheme) Icon(n fyne.ThemeIconName) fyne.Resource { return theme.DefaultTheme().Icon(n) }
func (darkTheme) Size(n fyne.ThemeSizeName) float32       { return theme.DefaultTheme().Size(n) }

// useDarkTheme включает тёмную тему, а после появления окна — тёмный
// заголовок и значки точного размера (decorateWindow, только Windows):
// попытки каждые 100 мс до 10 с, пока всё не удержится.
func useDarkTheme(a fyne.App, title string) {
	a.Settings().SetTheme(darkTheme{})
	go func() {
		for i := 0; i < 100; i++ {
			if decorateWindow(title) {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()
}
