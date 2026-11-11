package imageio

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

func sample(gray bool) image.Image {
	if gray {
		g := image.NewGray(image.Rect(0, 0, 9, 7))
		for i := range g.Pix {
			g.Pix[i] = uint8(i*37 + 5)
		}
		return g
	}
	m := image.NewNRGBA(image.Rect(0, 0, 9, 7))
	for y := 0; y < 7; y++ {
		for x := 0; x < 9; x++ {
			m.SetNRGBA(x, y, color.NRGBA{uint8(x * 28), uint8(y * 36), uint8(x*y + 1), 255})
		}
	}
	return m
}

// Запись и чтение не должны менять ни одного значения: в младших битах лежит
// сообщение.
func TestSaveLoadLossless(t *testing.T) {
	dir := t.TempDir()
	for _, gray := range []bool{false, true} {
		for _, name := range []string{"a.png", "b.BMP", "в папке с пробелом.png"} {
			if gray && filepath.Ext(name) == ".BMP" {
				continue // серый BMP читается как палитровый; его разбирает stego.FromImage
			}
			src := sample(gray)
			path := filepath.Join(dir, name)
			f, err := FormatOf(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := Save(path, src, f); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			got, gf, err := Load(path)
			if err != nil || gf != f {
				t.Fatalf("%s: %v, формат %q", name, err, gf)
			}
			b := src.Bounds()
			for y := b.Min.Y; y < b.Max.Y; y++ {
				for x := b.Min.X; x < b.Max.X; x++ {
					r1, g1, b1, a1 := src.At(x, y).RGBA()
					r2, g2, b2, a2 := got.At(x, y).RGBA()
					if r1 != r2 || g1 != g2 || b1 != b2 || a1 != a2 {
						t.Fatalf("%s: пиксель (%d, %d) изменился", name, x, y)
					}
				}
			}
			if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
				t.Fatalf("%s: остался временный файл", name)
			}
		}
	}
}

func TestFormatOf(t *testing.T) {
	for _, p := range []string{"x.jpg", "x.jpeg", "x", "x.png.txt", "x.gif"} {
		if _, err := FormatOf(p); err == nil {
			t.Fatalf("%s: формат должен отвергаться", p)
		}
	}
	for p, want := range map[string]Format{"x.PNG": PNG, "dir.bmp/y.Bmp": BMP} {
		if f, err := FormatOf(p); err != nil || f != want {
			t.Fatalf("%s: %q, %v", p, f, err)
		}
	}
}

// Неудачная запись не должна ни портить существующий файл, ни оставлять
// временный.
func TestSaveFailureKeepsOldFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "old.png")
	if err := os.WriteFile(path, []byte("прежнее содержимое"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, sample(false), Format("tiff")); err == nil {
		t.Fatal("неизвестный формат должен давать ошибку")
	}
	if b, _ := os.ReadFile(path); string(b) != "прежнее содержимое" {
		t.Fatalf("существующий файл изменён: %q", b)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("остался временный файл")
	}
}

func TestLoadErrors(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := Load(filepath.Join(dir, "нет такого.png")); err == nil {
		t.Fatal("ожидалась ошибка открытия")
	}
	bad := filepath.Join(dir, "bad.png")
	os.WriteFile(bad, []byte("это не PNG"), 0o644)
	if _, _, err := Load(bad); err == nil {
		t.Fatal("ожидалась ошибка разбора")
	}
}
