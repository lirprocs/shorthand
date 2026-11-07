package main

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"

	"github.com/lirprocs/shorthand/internal/imageio"
)

func writeTestPNG(t *testing.T, p string) {
	img := image.NewNRGBA(image.Rect(0, 0, 64, 48))
	for i := range img.Pix {
		img.Pix[i] = uint8(i * 7)
	}
	img.Set(0, 0, color.NRGBA{1, 2, 3, 255})
	if err := imageio.Save(p, img, imageio.PNG); err != nil {
		t.Fatal(err)
	}
}

// Брошенные на окно файлы: изображение — в контейнер, .txt — в текст,
// прочее — подсказка; путь сохранения по умолчанию — рядом, enc_<имя>.
func TestDrop(t *testing.T) {
	test.NewApp()
	dir := t.TempDir()
	img := filepath.Join(dir, "фото.png")
	writeTestPNG(t, img)
	txt := filepath.Join(dir, "сообщение.txt")
	os.WriteFile(txt, []byte("\xEF\xBB\xBFПривет из файла"), 0o644)

	emb := newEmbedTab()
	emb.drop(img)
	if emb.cover == nil || emb.inPath != img {
		t.Fatal("изображение не принято")
	}
	// без приставки «enc_»: то же имя с номером (этап 1, ветка later/stage1)
	if want := filepath.Join(dir, "фото_1.png"); emb.outPath != want || emb.outChosen {
		t.Fatalf("путь сохранения %q, ожидался %q", emb.outPath, want)
	}
	if emb.meta == nil || emb.check == nil {
		t.Fatal("нет проверки контейнера")
	}
	if !strings.HasPrefix(emb.fileLabel.Text, "фото.png") {
		t.Fatalf("подпись файла: %q", emb.fileLabel.Text)
	}
	emb.drop(txt)
	if emb.textEntry.Text != "Привет из файла" {
		t.Fatalf("текст: %q", emb.textEntry.Text)
	}
	emb.drop(filepath.Join(dir, "архив.zip"))
	if !strings.Contains(emb.status.Text, "Перетащите") {
		t.Fatalf("подсказка: %q", emb.status.Text)
	}

	ext := newExtractTab()
	ext.drop(img)
	if ext.cover == nil {
		t.Fatal("вкладка извлечения не приняла изображение")
	}
	ext.drop(txt)
	if !strings.Contains(ext.status.Text, "PNG или BMP") {
		t.Fatalf("подсказка: %q", ext.status.Text)
	}
}

// Размер окна не должен зависеть от длины путей и сообщений (иначе окно
// растёт и появляется пустое место).
func TestLayoutStable(t *testing.T) {
	test.NewApp()
	emb, ext := newEmbedTab(), newExtractTab()
	tabs := container.NewAppTabs(
		container.NewTabItem("Встраивание", emb.content),
		container.NewTabItem("Извлечение", ext.content),
	)
	before := tabs.MinSize()
	long := strings.Repeat("очень_длинный_путь\\", 30) + "файл.png"
	emb.fileLabel.SetText(showPath("", long))
	emb.outLabel.SetText(showPath("Сохранить в: ", long))
	emb.info.SetText(strings.Repeat("Длинная сводка о размере сообщения. ", 20))
	emb.status.SetText(strings.Repeat("Длинное сообщение об итоге. ", 30))
	ext.fileLabel.SetText(showPath("", long))
	ext.status.SetText(strings.Repeat("Длинное сообщение об ошибке. ", 30))
	if after := tabs.MinSize(); after != before {
		t.Fatalf("минимальный размер изменился: %v → %v", before, after)
	}
	// кнопка «Подготовить» появляется в строке файла — высота та же
	emb.prepareBtn.Show()
	if after := tabs.MinSize(); after.Height != before.Height {
		t.Fatalf("высота изменилась при показе «Подготовить»: %v → %v", before, after)
	}
}

// Проверка после записи ловит испорченный файл и удаляет его.
func TestVerifySaved(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.png")
	writeTestPNG(t, p) // сообщения в нём нет
	err := verifySaved(p, "пароль", nil, make([]byte, 32), "текст")
	if err == nil {
		t.Fatal("пустое изображение прошло проверку")
	}
	if _, statErr := os.Stat(p); !os.IsNotExist(statErr) {
		t.Fatal("файл после неудачной проверки не удалён")
	}
}

func TestIconRender(t *testing.T) {
	for _, s := range []int{16, 20, 24, 32, 48} {
		ic := renderIcon(s)
		if ic.Rect.Dx() != s || ic.Rect.Dy() != s {
			t.Fatal("размер значка")
		}
		// угол прозрачный, центр корпуса замка — зелёный
		if ic.NRGBAAt(0, 0).A != 0 {
			t.Fatalf("%d: угол не прозрачный", s)
		}
		c := ic.NRGBAAt(s/2, s*150/256)
		if c.G < 200 || c.R > 60 || c.A < 250 {
			t.Fatalf("%d: центр замка %v", s, c)
		}
	}
}
