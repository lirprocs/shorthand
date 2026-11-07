// Shorthand — графический интерфейс стеганографической программы (формат v3).
//
// Сборка под Windows:
//
//	go build -ldflags "-H=windowsgui" -o shorthand.exe ./cmd/shorthand
package main

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/sqweek/dialog"

	"github.com/lirprocs/shorthand/internal/alphabet"
	"github.com/lirprocs/shorthand/internal/crypto/suite"
	"github.com/lirprocs/shorthand/internal/imageio"
	"github.com/lirprocs/shorthand/internal/stego"
)

//go:embed icon.png
var iconPNG []byte

var (
	modeNames  = []string{suite.KuznyechikMGM.String(), suite.AsconAEAD128.String(), suite.MagmaMGM.String()}
	levelNames = []string{"Обычная (наибольшая скрытность)", "Повышенная", "Максимальная"}
)

func modeByName(s string) suite.Mode {
	for i, n := range modeNames {
		if n == s {
			return suite.All[i]
		}
	}
	return suite.KuznyechikMGM
}

func levelByName(s string) stego.Level {
	for i, n := range levelNames {
		if n == s {
			return stego.Level(i)
		}
	}
	return stego.LevelNormal
}

func monoLabel() *widget.Label {
	l := widget.NewLabel("")
	l.Wrapping = fyne.TextWrapWord
	return l
}

const windowTitle = "Shorthand — стеганография"

func main() {
	a := app.New()
	useDarkTheme(a, windowTitle)
	w := a.NewWindow(windowTitle)
	w.SetIcon(fyne.NewStaticResource("icon.png", iconPNG))

	w.SetContent(container.NewAppTabs(
		container.NewTabItem("Встраивание", embedTab(w)),
		container.NewTabItem("Извлечение", extractTab(w)),
	))
	w.Resize(fyne.NewSize(640, 720))
	w.ShowAndRun()
}

// ─── Встраивание ────────────────────────────────────────────────────────────

func embedTab(w fyne.Window) fyne.CanvasObject {
	var (
		inPath  string
		outDir  string
		cover   *stego.Cover
		inFmt   imageio.Format
		textBuf string // текст из файла, если он слишком велик для поля ввода
	)
	fileLabel := monoLabel()
	infoLabel := monoLabel()
	dirLabel := monoLabel()
	status := monoLabel()

	textEntry := widget.NewMultiLineEntry()
	textEntry.SetPlaceHolder("Введите текст")
	textEntry.Wrapping = fyne.TextWrapWord
	textScroll := container.NewVScroll(textEntry)
	textScroll.SetMinSize(fyne.NewSize(200, 180))

	aadEntry := widget.NewMultiLineEntry()
	aadEntry.SetPlaceHolder("Введите имитозащищаемые данные (Не обязательно)")

	pass := widget.NewPasswordEntry()
	pass.SetPlaceHolder("Придумайте пароль")

	modeSel := widget.NewSelect(modeNames, nil)
	modeSel.SetSelected(modeNames[0])
	levelSel := widget.NewSelect(levelNames, nil)
	levelSel.SetSelected(levelNames[0])

	currentText := func() string {
		if textBuf != "" {
			return textBuf
		}
		return textEntry.Text
	}

	// сводка: сколько займёт сообщение
	refresh := func() {
		if cover == nil {
			infoLabel.SetText("")
			return
		}
		t := currentText()
		opt := stego.Options{Mode: modeByName(modeSel.Selected), Level: levelByName(levelSel.Selected)}
		rep, _, err := stego.Plan(cover, t, opt)
		switch {
		case t == "":
			infoLabel.SetText(fmt.Sprintf("Изображение %d×%d, %s, пригодных элементов: %d.",
				cover.W, cover.H, map[int]string{1: "оттенки серого", 3: "цветное"}[cover.C], stego.UsableCount(cover)))
		case err != nil:
			infoLabel.SetText(err.Error())
		default:
			// стрелку «→» шрифт Fyne не содержит — только слова
			infoLabel.SetText(fmt.Sprintf("Текст: %d симв., после сжатия %d байт (%s), с имитовставкой и кодом Рида–Соломона — %d байт. "+
				"Занято %.2f%% пригодных элементов, на бит сообщения — %d элементов.",
				rep.Chars, rep.Compressed, rep.Method, rep.Coded, 100*float64(rep.Used)/float64(rep.Elements), rep.W))
		}
	}
	textEntry.OnChanged = func(string) { textBuf = ""; refresh() }
	modeSel.OnChanged = func(string) { refresh() }
	levelSel.OnChanged = func(string) { refresh() }

	selectFile := widget.NewButton("Выбрать файл", func() {
		p, err := dialog.File().Filter("Изображения PNG и BMP", "png", "bmp").Load()
		if err != nil {
			return // отмена
		}
		img, f, err := imageio.Load(p)
		if err != nil {
			status.SetText(err.Error())
			return
		}
		inPath, inFmt, cover = p, f, stego.FromImage(img)
		fileLabel.SetText(p)
		status.SetText("")
		refresh()
	})

	loadText := widget.NewButton("Загрузить текст из файла", func() {
		p, err := dialog.File().Filter("Текстовые файлы", "txt").Load()
		if err != nil {
			return
		}
		b, err := os.ReadFile(p)
		if err != nil {
			status.SetText("Не удалось прочитать файл: " + err.Error())
			return
		}
		b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF}) // метка порядка байтов UTF-8
		if !utf8.Valid(b) {
			status.SetText("Файл не в кодировке UTF-8 — сохраните его в UTF-8 и повторите")
			return
		}
		if len(b) <= 200_000 {
			textEntry.SetText(string(b))
			textBuf = ""
		} else {
			textEntry.SetText(fmt.Sprintf("[текст из файла %s, %d символов — слишком велик для показа]", filepath.Base(p), utf8.RuneCount(b)))
			textBuf = string(b)
		}
		status.SetText("Текст загружен из файла")
		refresh()
	})

	selectDir := widget.NewButton("Выбрать директорию сохранения", func() {
		d, err := dialog.Directory().Title("Выберите директорию").Browse()
		if err != nil {
			return
		}
		outDir = d
		dirLabel.SetText(d)
	})

	var start *widget.Button
	start = widget.NewButtonWithIcon("Старт", theme.MediaPlayIcon(), func() {
		text := currentText()
		switch {
		case cover == nil:
			status.SetText("Пожалуйста, выберите изображение (PNG или BMP)")
			return
		case text == "":
			status.SetText("Пожалуйста, введите текст")
			return
		case pass.Text == "":
			status.SetText("Пожалуйста, задайте пароль")
			return
		case outDir == "":
			status.SetText("Пожалуйста, выберите директорию сохранения")
			return
		}
		outPath := freeName(filepath.Join(outDir, "enc_"+filepath.Base(inPath)))
		opt := stego.Options{
			Mode: modeByName(modeSel.Selected), Level: levelByName(levelSel.Selected),
			Password: pass.Text, AAD: []byte(aadEntry.Text),
		}
		lost := alphabet.Normalize(text) != strings.ReplaceAll(text, "\r\n", "\n")
		start.Disable()
		status.SetText("Выполняется…")
		go func() {
			defer start.Enable()
			t0 := time.Now()
			out, rep, err := stego.EmbedCover(cover, text, opt)
			if err == nil {
				err = imageio.Save(outPath, out.Image(), inFmt)
			}
			if err != nil {
				status.SetText("Ошибка: " + err.Error())
				return
			}
			msg := fmt.Sprintf("Готово за %.1f с: %s\nИзменено элементов: %d (%.3f%%), шифр %s, устойчивость — %s.",
				time.Since(t0).Seconds(), outPath, rep.Changes+rep.HeaderChanges,
				100*float64(rep.Changes+rep.HeaderChanges)/float64(rep.Elements), rep.Mode, rep.Level)
			if lost {
				msg += "\nПрописные буквы будут извлечены строчными."
			}
			status.SetText(msg)
		}()
	})

	return container.NewVScroll(container.NewVBox(
		selectFile, fileLabel,
		textScroll,
		aadEntry,
		loadText,
		pass,
		container.NewGridWithColumns(2,
			container.NewVBox(widget.NewLabel("Шифр"), modeSel),
			container.NewVBox(widget.NewLabel("Устойчивость к повреждениям"), levelSel),
		),
		infoLabel,
		selectDir, dirLabel,
		start,
		status,
	))
}

// freeName — путь, не занятый существующим файлом (name, name_1, name_2…).
func freeName(p string) string {
	if _, err := os.Stat(p); errors.Is(err, os.ErrNotExist) {
		return p
	}
	ext := filepath.Ext(p)
	base := strings.TrimSuffix(p, ext)
	for i := 1; ; i++ {
		q := fmt.Sprintf("%s_%d%s", base, i, ext)
		if _, err := os.Stat(q); errors.Is(err, os.ErrNotExist) {
			return q
		}
	}
}

// ─── Извлечение ─────────────────────────────────────────────────────────────

func extractTab(w fyne.Window) fyne.CanvasObject {
	var cover *stego.Cover
	var result string
	fileLabel := monoLabel()
	status := monoLabel()

	aadEntry := widget.NewMultiLineEntry()
	aadEntry.SetPlaceHolder("Введите имитозащищаемые данные (Не обязательно)")
	pass := widget.NewPasswordEntry()
	pass.SetPlaceHolder("Введите пароль")

	output := widget.NewMultiLineEntry()
	output.Wrapping = fyne.TextWrapWord
	output.SetPlaceHolder("Здесь появится извлечённый текст")
	outScroll := container.NewVScroll(output)
	outScroll.SetMinSize(fyne.NewSize(200, 260))

	selectFile := widget.NewButton("Выбрать файл", func() {
		p, err := dialog.File().Filter("Изображения PNG и BMP", "png", "bmp").Load()
		if err != nil {
			return
		}
		img, _, err := imageio.Load(p)
		if err != nil {
			status.SetText(err.Error())
			return
		}
		cover = stego.FromImage(img)
		fileLabel.SetText(p)
		status.SetText("")
	})

	var start *widget.Button
	start = widget.NewButtonWithIcon("Старт", theme.MediaPlayIcon(), func() {
		switch {
		case cover == nil:
			status.SetText("Пожалуйста, выберите файл")
			return
		case pass.Text == "":
			status.SetText("Пожалуйста, введите пароль")
			return
		}
		password, aad := pass.Text, []byte(aadEntry.Text)
		start.Disable()
		status.SetText("Выполняется…")
		go func() {
			defer start.Enable()
			res, err := stego.ExtractCover(cover, password, aad, nil)
			if err != nil {
				msg := "Ошибка: " + err.Error()
				if res != nil && errors.Is(err, stego.ErrDamaged) {
					msg += fmt.Sprintf(" (шифр %s, устойчивость — %s)", res.Mode, res.Level)
				}
				status.SetText(msg)
				return
			}
			result = res.Text
			if utf8.RuneCountInString(result) <= 200_000 {
				output.SetText(result)
			} else {
				output.SetText("[текст слишком велик для показа — сохраните его в файл]")
			}
			msg := fmt.Sprintf("Готово: %d симв., шифр %s, устойчивость — %s.", utf8.RuneCountInString(result), res.Mode, res.Level)
			if res.Corrected > 0 {
				msg += fmt.Sprintf(" Исправлено повреждённых байтов: %d.", res.Corrected)
			}
			status.SetText(msg)
		}()
	})

	save := widget.NewButton("Сохранить текст в файл", func() {
		if result == "" {
			status.SetText("Сначала извлеките текст")
			return
		}
		p, err := dialog.File().Filter("Текстовые файлы", "txt").Title("Сохранить текст").Save()
		if err != nil {
			return
		}
		if filepath.Ext(p) == "" {
			p += ".txt"
		}
		if err := os.WriteFile(p, []byte(result), 0o644); err != nil {
			status.SetText("Не удалось сохранить: " + err.Error())
			return
		}
		status.SetText("Текст сохранён: " + p)
	})

	return container.NewVScroll(container.NewVBox(
		selectFile, fileLabel,
		aadEntry,
		pass,
		start,
		outScroll,
		save,
		status,
	))
}
