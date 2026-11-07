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
	"github.com/lirprocs/shorthand/internal/crypto/kdf"
	"github.com/lirprocs/shorthand/internal/crypto/suite"
	"github.com/lirprocs/shorthand/internal/imageio"
	"github.com/lirprocs/shorthand/internal/precheck"
	"github.com/lirprocs/shorthand/internal/stego"
)

// icon.png — кадр 256×256 из Samples/ico.ico (значок для Fyne; мелкие значки
// окна рисуются в icon.go).
//
//go:embed icon.png
var iconPNG []byte

const windowTitle = "Shorthand — стеганография"

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

// pathLabel — одна строка, длинный текст обрезается, окно от него не растёт.
func pathLabel(text string) *widget.Label {
	l := widget.NewLabel(text)
	l.Wrapping = fyne.TextTruncate
	return l
}

// showPath — «имя файла — папка»: при обрезке длинного пути пропадает конец
// папки, а не имя файла.
func showPath(prefix, p string) string {
	return prefix + filepath.Base(p) + "  —  " + filepath.Dir(p)
}

// fixedLabel — подпись с переносом по словам в области высотой lines строк:
// размер окна не зависит от длины сообщения.
func fixedLabel(lines int) (*widget.Label, fyne.CanvasObject) {
	l := widget.NewLabel("")
	l.Wrapping = fyne.TextWrapWord
	s := container.NewVScroll(l)
	line := fyne.MeasureText("Ag", theme.TextSize(), fyne.TextStyle{}).Height
	s.SetMinSize(fyne.NewSize(0, float32(lines)*line+2*theme.InnerPadding()+2))
	return l, s
}

func isImage(p string) bool {
	_, err := imageio.FormatOf(p)
	return err == nil
}

func main() {
	a := app.New()
	useDarkTheme(a, windowTitle)
	w := a.NewWindow(windowTitle)
	w.SetIcon(fyne.NewStaticResource("icon.png", iconPNG))

	emb := newEmbedTab()
	ext := newExtractTab()
	tabs := container.NewAppTabs(
		container.NewTabItem("Встраивание", emb.content),
		container.NewTabItem("Извлечение", ext.content),
	)
	w.SetContent(tabs)

	// файлы, брошенные на окно, — на текущую вкладку
	w.SetOnDropped(func(_ fyne.Position, uris []fyne.URI) {
		for _, u := range uris {
			if u.Scheme() != "file" {
				continue
			}
			p := filepath.FromSlash(u.Path())
			if tabs.SelectedIndex() == 0 {
				emb.drop(p)
			} else {
				ext.drop(p)
			}
		}
	})

	// окно — ровно по содержимому: без полос прокрутки и пустого места;
	// если его растянуть, растут поля текста
	sz := tabs.MinSize()
	w.Resize(fyne.NewSize(max(sz.Width, 640), sz.Height))
	w.ShowAndRun()
}

// ─── Встраивание ────────────────────────────────────────────────────────────

type embedTab struct {
	content fyne.CanvasObject

	inPath, outPath string
	outChosen       bool // путь сохранения выбран вручную
	cover           *stego.Cover
	textBuf         string // текст из файла, если он слишком велик для поля ввода

	// этап 1 «на потом»: свойства исходного файла и проверка контейнера
	meta     *imageio.Meta
	check    *precheck.Report
	prepared bool // контейнер уменьшен вдвое кнопкой «Подготовить»

	fileLabel, outLabel *widget.Label
	info, status        *widget.Label
	textEntry           *widget.Entry
	modeSel, levelSel   *widget.Select
	prepareBtn          *widget.Button
	keepTime            *widget.Check
}

func newEmbedTab() *embedTab {
	t := &embedTab{
		fileLabel: pathLabel("Изображение не выбрано — нажмите кнопку или перетащите PNG/BMP на окно"),
		outLabel:  pathLabel("Куда сохранить — выберется после выбора изображения"),
	}
	var infoBox, statusBox fyne.CanvasObject
	t.info, infoBox = fixedLabel(4)
	t.status, statusBox = fixedLabel(3)

	t.prepareBtn = widget.NewButton("Подготовить: уменьшить вдвое", func() { t.prepare() })
	t.prepareBtn.Hide()
	t.keepTime = widget.NewCheck("Сохранить дату изменения исходного файла", nil)
	t.keepTime.SetChecked(true)

	t.textEntry = widget.NewMultiLineEntry()
	t.textEntry.SetPlaceHolder("Введите текст (или перетащите .txt на окно)")
	t.textEntry.Wrapping = fyne.TextWrapWord
	t.textEntry.SetMinRowsVisible(6)

	aadEntry := widget.NewEntry()
	aadEntry.SetPlaceHolder("Введите имитозащищаемые данные (не обязательно)")

	pass := widget.NewPasswordEntry()
	pass.SetPlaceHolder("Придумайте пароль")

	t.modeSel = widget.NewSelect(modeNames, nil)
	t.modeSel.SetSelected(modeNames[0])
	t.levelSel = widget.NewSelect(levelNames, nil)
	t.levelSel.SetSelected(levelNames[0])

	t.textEntry.OnChanged = func(string) { t.textBuf = ""; t.refresh() }
	t.modeSel.OnChanged = func(string) { t.refresh() }
	t.levelSel.OnChanged = func(string) { t.refresh() }

	selectFile := widget.NewButton("Выбрать файл", func() {
		p, err := dialog.File().Filter("Изображения PNG и BMP", "png", "bmp").Load()
		if err == nil {
			t.loadImage(p)
		}
	})
	loadText := widget.NewButton("Загрузить текст из файла", func() {
		p, err := dialog.File().Filter("Текстовые файлы", "txt").Load()
		if err == nil {
			t.loadText(p)
		}
	})
	saveAs := widget.NewButton("Куда сохранить…", func() {
		b := dialog.File().Filter("Изображения PNG и BMP", "png", "bmp").Title("Сохранить изображение с сообщением")
		if t.outPath != "" {
			b = b.SetStartDir(filepath.Dir(t.outPath)).SetStartFile(filepath.Base(t.outPath))
		}
		p, err := b.Save()
		if err != nil {
			return // отмена
		}
		if !isImage(p) { // расширение не указано или другое — как у исходного
			ext := ".png"
			if t.inPath != "" {
				ext = strings.ToLower(filepath.Ext(t.inPath))
			}
			p += ext
		}
		t.outPath, t.outChosen = p, true
		t.outLabel.SetText(showPath("Сохранить в: ", p))
	})

	var start *widget.Button
	start = widget.NewButtonWithIcon("Старт", theme.MediaPlayIcon(), func() {
		text := t.currentText()
		switch {
		case t.cover == nil:
			t.status.SetText("Выберите изображение (PNG или BMP)")
			return
		case text == "":
			t.status.SetText("Введите текст")
			return
		case pass.Text == "":
			t.status.SetText("Задайте пароль")
			return
		case samePath(t.outPath, t.inPath):
			t.status.SetText("Нельзя сохранять поверх исходного изображения — выберите другое имя («Куда сохранить…»)")
			return
		}
		outPath := t.outPath
		if !t.outChosen {
			outPath = freeName(outPath) // за время работы файл с таким именем мог появиться
		}
		format, err := imageio.FormatOf(outPath)
		if err != nil {
			t.status.SetText(err.Error())
			return
		}
		cover, meta, keepTime := t.cover, t.meta, t.keepTime.Checked
		password, aad := pass.Text, []byte(aadEntry.Text)
		opt := stego.Options{
			Mode: modeByName(t.modeSel.Selected), Level: levelByName(t.levelSel.Selected),
			Password: password, AAD: aad,
		}
		lost := alphabet.Normalize(text) != strings.ReplaceAll(text, "\r\n", "\n")
		start.Disable()
		t.status.SetText("Выполняется…")
		go func() {
			defer start.Enable()
			t0 := time.Now()
			// Argon2id — один раз: и для встраивания, и для проверки после записи
			opt.Master = kdf.Master(password)
			out, rep, err := stego.EmbedCover(cover, text, opt)
			if err == nil {
				err = imageio.SaveLike(outPath, out.Image(), format, meta, keepTime)
			}
			if err == nil {
				err = verifySaved(outPath, password, aad, opt.Master, alphabet.Normalize(text))
			}
			if err != nil {
				t.status.SetText("Ошибка: " + err.Error())
				return
			}
			msg := fmt.Sprintf("Готово за %.1f с: сохранено и проверено %s. Изменено элементов: %d (%.3f%%), шифр %s, устойчивость — %s.",
				time.Since(t0).Seconds(), filepath.Base(outPath), rep.Changes+rep.HeaderChanges,
				100*float64(rep.Changes+rep.HeaderChanges)/float64(rep.Elements), rep.Mode, rep.Level)
			if lost {
				msg += " Прописные буквы будут извлечены строчными."
			}
			t.status.SetText(msg)
			if !t.outChosen {
				t.setDefaultOut() // следующий раз — новое имя, без перезаписи
			}
		}()
	})

	// кнопка «Подготовить» — в строке с именем файла: её появление не меняет
	// высоту окна
	top := container.NewVBox(selectFile, container.NewBorder(nil, nil, nil, t.prepareBtn, t.fileLabel))
	bottom := container.NewVBox(
		aadEntry,
		loadText,
		pass,
		container.NewGridWithColumns(2,
			container.NewVBox(widget.NewLabel("Шифр"), t.modeSel),
			container.NewVBox(widget.NewLabel("Устойчивость к повреждениям"), t.levelSel),
		),
		infoBox,
		saveAs, t.outLabel,
		t.keepTime,
		start,
		statusBox,
	)
	t.content = container.NewBorder(top, bottom, nil, nil, t.textEntry)
	return t
}

func (t *embedTab) currentText() string {
	if t.textBuf != "" {
		return t.textBuf
	}
	return t.textEntry.Text
}

// refresh — сводка: сколько займёт сообщение.
func (t *embedTab) refresh() {
	if t.cover == nil {
		t.info.SetText("")
		return
	}
	text := t.currentText()
	opt := stego.Options{Mode: modeByName(t.modeSel.Selected), Level: levelByName(t.levelSel.Selected)}
	rep, _, err := stego.Plan(t.cover, text, opt)
	var warn []string
	if t.check != nil {
		warn = append(warn, t.check.Warnings...)
	}
	if t.meta != nil {
		warn = append(warn, t.meta.Changes()...)
	}
	if t.prepared {
		warn = append([]string{fmt.Sprintf("изображение уменьшено до %d×%d", t.cover.W, t.cover.H)}, warn...)
	}
	warnText := ""
	if len(warn) > 0 {
		warnText = " Внимание: " + strings.Join(warn, "; ") + "."
	}
	switch {
	case text == "":
		t.info.SetText(fmt.Sprintf("Изображение %d×%d, %s, пригодных элементов: %d.",
			t.cover.W, t.cover.H, map[int]string{1: "оттенки серого", 3: "цветное"}[t.cover.C], stego.UsableCount(t.cover)) + warnText)
	case err != nil:
		t.info.SetText(err.Error() + warnText)
	default:
		risk, rate := precheck.RiskOf(rep, t.check)
		// стрелку «→» шрифт Fyne не содержит — только слова
		t.info.SetText(fmt.Sprintf("Риск обнаружения: %s (%.3f бита на элемент). Текст: %d симв., после сжатия %d байт (%s), "+
			"с имитовставкой и кодом Рида–Соломона — %d байт; занято %.2f%% пригодных элементов.",
			risk, rate, rep.Chars, rep.Compressed, rep.Method, rep.Coded, 100*float64(rep.Used)/float64(rep.Elements)) + warnText)
	}
}

// prepare уменьшает контейнер вдвое: следы JPEG исчезают.
func (t *embedTab) prepare() {
	if t.cover == nil {
		return
	}
	t.cover = precheck.Downscale2(t.cover)
	t.prepared = true
	t.check = precheck.Check(t.cover)
	if !t.check.CanDownscale {
		t.prepareBtn.Hide()
	}
	t.status.SetText(fmt.Sprintf("Изображение уменьшено до %d×%d; исходный файл не изменён.", t.cover.W, t.cover.H))
	t.refresh()
}

// verifySaved — проверка после записи: файл открывается заново и сообщение
// извлекается тем же паролем. Если не совпало — файл удаляется.
func verifySaved(path, password string, aad, master []byte, want string) error {
	img, _, err := imageio.Load(path)
	if err == nil {
		var res *stego.Extracted
		res, err = stego.ExtractCover(stego.FromImage(img), password, aad, master)
		if err == nil && res.Text != want {
			err = errors.New("извлечён другой текст")
		}
	}
	if err != nil {
		os.Remove(path)
		return fmt.Errorf("проверка после сохранения не прошла (%v) — файл удалён, попробуйте ещё раз или другое изображение", err)
	}
	return nil
}

func (t *embedTab) loadImage(p string) {
	img, _, err := imageio.Load(p)
	if err != nil {
		t.status.SetText(err.Error())
		return
	}
	meta, err := imageio.Inspect(p)
	if err != nil {
		t.status.SetText(err.Error())
		return
	}
	cover := stego.FromImage(img)
	t.inPath, t.cover, t.meta, t.prepared = p, cover, meta, false
	t.check = precheck.Check(cover) // доли секунды
	if t.check.CanDownscale {
		t.prepareBtn.Show()
	} else {
		t.prepareBtn.Hide()
	}
	t.fileLabel.SetText(showPath("", p))
	if !t.outChosen || samePath(t.outPath, t.inPath) {
		t.setDefaultOut()
	}
	t.status.SetText("")
	t.refresh()
}

// setDefaultOut — по умолчанию сохранять рядом с исходным под тем же именем
// с номером (<имя>_1.png): приставка вроде «enc_» сама выдала бы, что с
// файлом что-то делали.
func (t *embedTab) setDefaultOut() {
	t.outPath = freeName(t.inPath)
	t.outChosen = false
	t.outLabel.SetText(showPath("Сохранить в: ", t.outPath))
}

func (t *embedTab) loadText(p string) {
	b, err := os.ReadFile(p)
	if err != nil {
		t.status.SetText("Не удалось прочитать файл: " + err.Error())
		return
	}
	b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF}) // метка порядка байтов UTF-8
	if !utf8.Valid(b) {
		t.status.SetText("Файл не в кодировке UTF-8 — сохраните его в UTF-8 и повторите")
		return
	}
	if len(b) <= 200_000 {
		t.textEntry.SetText(string(b))
		t.textBuf = ""
	} else {
		t.textEntry.SetText(fmt.Sprintf("[текст из файла %s, %d символов — слишком велик для показа]", filepath.Base(p), utf8.RuneCount(b)))
		t.textBuf = string(b)
	}
	t.status.SetText("Текст загружен из файла " + filepath.Base(p))
	t.refresh()
}

func (t *embedTab) drop(p string) {
	switch {
	case isImage(p):
		t.loadImage(p)
	case strings.EqualFold(filepath.Ext(p), ".txt"):
		t.loadText(p)
	default:
		t.status.SetText("Перетащите изображение PNG/BMP или текст .txt")
	}
}

func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	aa, err1 := filepath.Abs(a)
	bb, err2 := filepath.Abs(b)
	return err1 == nil && err2 == nil && strings.EqualFold(aa, bb)
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

type extractTab struct {
	content   fyne.CanvasObject
	inPath    string
	cover     *stego.Cover
	result    string
	fileLabel *widget.Label
	status    *widget.Label
}

func newExtractTab() *extractTab {
	t := &extractTab{
		fileLabel: pathLabel("Изображение не выбрано — нажмите кнопку или перетащите PNG/BMP на окно"),
	}
	var statusBox fyne.CanvasObject
	t.status, statusBox = fixedLabel(3)

	aadEntry := widget.NewEntry()
	aadEntry.SetPlaceHolder("Введите имитозащищаемые данные (не обязательно)")
	pass := widget.NewPasswordEntry()
	pass.SetPlaceHolder("Введите пароль")

	output := widget.NewMultiLineEntry()
	output.Wrapping = fyne.TextWrapWord
	output.SetPlaceHolder("Здесь появится извлечённый текст")
	output.SetMinRowsVisible(6)

	selectFile := widget.NewButton("Выбрать файл", func() {
		p, err := dialog.File().Filter("Изображения PNG и BMP", "png", "bmp").Load()
		if err == nil {
			t.loadImage(p)
		}
	})

	var start *widget.Button
	start = widget.NewButtonWithIcon("Старт", theme.MediaPlayIcon(), func() {
		switch {
		case t.cover == nil:
			t.status.SetText("Выберите изображение")
			return
		case pass.Text == "":
			t.status.SetText("Введите пароль")
			return
		}
		cover, password, aad := t.cover, pass.Text, []byte(aadEntry.Text)
		start.Disable()
		t.status.SetText("Выполняется…")
		go func() {
			defer start.Enable()
			res, err := stego.ExtractCover(cover, password, aad, nil)
			if err != nil {
				msg := "Ошибка: " + err.Error()
				if res != nil && errors.Is(err, stego.ErrDamaged) {
					msg += fmt.Sprintf(" (шифр %s, устойчивость — %s)", res.Mode, res.Level)
				}
				t.status.SetText(msg)
				return
			}
			t.result = res.Text
			if utf8.RuneCountInString(t.result) <= 200_000 {
				output.SetText(t.result)
			} else {
				output.SetText("[текст слишком велик для показа — сохраните его в файл]")
			}
			msg := fmt.Sprintf("Готово: %d симв., шифр %s, устойчивость — %s.", utf8.RuneCountInString(t.result), res.Mode, res.Level)
			if res.Corrected > 0 {
				msg += fmt.Sprintf(" Исправлено повреждённых байтов: %d.", res.Corrected)
			}
			t.status.SetText(msg)
		}()
	})

	save := widget.NewButton("Сохранить текст в файл", func() {
		if t.result == "" {
			t.status.SetText("Сначала извлеките текст")
			return
		}
		b := dialog.File().Filter("Текстовые файлы", "txt").Title("Сохранить текст")
		if t.inPath != "" {
			name := strings.TrimSuffix(filepath.Base(t.inPath), filepath.Ext(t.inPath)) + ".txt"
			b = b.SetStartDir(filepath.Dir(t.inPath)).SetStartFile(name)
		}
		p, err := b.Save()
		if err != nil {
			return
		}
		if filepath.Ext(p) == "" {
			p += ".txt"
		}
		if err := os.WriteFile(p, []byte(t.result), 0o644); err != nil {
			t.status.SetText("Не удалось сохранить: " + err.Error())
			return
		}
		t.status.SetText("Текст сохранён: " + p)
	})

	top := container.NewVBox(selectFile, t.fileLabel, aadEntry, pass, start)
	bottom := container.NewVBox(save, statusBox)
	t.content = container.NewBorder(top, bottom, nil, nil, output)
	return t
}

func (t *extractTab) loadImage(p string) {
	img, _, err := imageio.Load(p)
	if err != nil {
		t.status.SetText(err.Error())
		return
	}
	t.inPath, t.cover = p, stego.FromImage(img)
	t.fileLabel.SetText(showPath("", p))
	t.status.SetText("")
}

func (t *extractTab) drop(p string) {
	if isImage(p) {
		t.loadImage(p)
	} else {
		t.status.SetText("Перетащите изображение PNG или BMP")
	}
}
