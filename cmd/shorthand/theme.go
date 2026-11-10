package main

import (
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// darkTheme — стандартная тема Fyne, всегда в тёмном варианте, независимо
// от настройки Windows «Режим приложения по умолчанию». Синий акцент Fyne
// заменён зелёным логотипа (как в icon.png): активная вкладка, рамка поля в
// фокусе, выделение текста, ссылки. Остальные цвета — стандартные.
type darkTheme struct{}

var _ fyne.Theme = darkTheme{}

var (
	accent          = color.NRGBA{0x1d, 0xe6, 0x12, 0xff} // зелёный логотипа
	accentFocus     = color.NRGBA{0x1d, 0xe6, 0x12, 0x55}
	accentSelection = color.NRGBA{0x1d, 0xe6, 0x12, 0x40}
)

func (darkTheme) Color(n fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	switch n {
	case theme.ColorNamePrimary, theme.ColorNameHyperlink:
		return accent
	case theme.ColorNameFocus:
		return accentFocus
	case theme.ColorNameSelection:
		return accentSelection
	}
	return theme.DefaultTheme().Color(n, theme.VariantDark)
}

func (darkTheme) Font(s fyne.TextStyle) fyne.Resource     { return theme.DefaultTheme().Font(s) }
func (darkTheme) Icon(n fyne.ThemeIconName) fyne.Resource { return theme.DefaultTheme().Icon(n) }
func (darkTheme) Size(n fyne.ThemeSizeName) float32       { return theme.DefaultTheme().Size(n) }

// useDarkTheme включает тёмную тему, а после появления окна — тёмный
// заголовок и значки точного размера (decorateWindow, только Windows):
// попытки каждые 100 мс до 10 с, пока всё не удержится.
//
// Fyne красит заголовок по настройке Windows «Режим приложения по умолчанию»
// не только при создании окна, но и при каждой смене настроек (тема Windows,
// масштаб, файл настроек Fyne). Поэтому после каждой такой смены заголовок
// проверяется ещё 3 с и при необходимости перекрашивается обратно. Проверка
// не прекращается на первом успехе: слушатель программы вызывается раньше
// слушателя Fyne, и в этот момент заголовок ещё тёмный.
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
	a.Settings().AddListener(func(fyne.Settings) {
		go func() {
			for i := 0; i < 30; i++ {
				time.Sleep(100 * time.Millisecond)
				decorateWindow(title)
			}
		}()
	})
}
