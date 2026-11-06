package stego

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"math"
	"math/rand"
	"testing"

	"golang.org/x/image/bmp"

	"github.com/lirprocs/shorthand/internal/alphabet"
	"github.com/lirprocs/shorthand/internal/crypto/suite"
)

// testImage — «похожее на фото» изображение: плавные градиенты, текстура и
// ровная область (небо) в верхней четверти.
func testImage(w, h int, seed int64, gray bool) image.Image {
	rng := rand.New(rand.NewSource(seed))
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var v [3]float64
			for c := 0; c < 3; c++ {
				base := 120 + 50*math.Sin(float64(x)/17+float64(c)) + 40*math.Cos(float64(y)/23)
				if y > h/4 {
					base += rng.NormFloat64() * 12
				}
				v[c] = math.Max(0, math.Min(255, base))
			}
			if gray {
				v[1], v[2] = v[0], v[0]
			}
			img.Set(x, y, color.NRGBA{uint8(v[0]), uint8(v[1]), uint8(v[2]), 255})
		}
	}
	return img
}

var testMaster = bytes.Repeat([]byte{0x5a}, 32)

const longText = "Встреча переносится на четверг, 15:30. Адрес тот же: улица Садовая, дом 12, офис 4. " +
	"Пропуск закажу заранее, документы возьми с собой. Если что-то пойдёт не так — пиши, телефон всегда при мне. " +
	"P.S. Код от домофона 2580, не забудь 😀"

func TestRoundTripModesLevels(t *testing.T) {
	img := testImage(320, 240, 1, false)
	cv := FromImage(img)
	for _, mode := range suite.All {
		for _, lvl := range []Level{LevelNormal, LevelHigh, LevelMax} {
			for _, text := range []string{"Привет!", longText} {
				opt := Options{Mode: mode, Level: lvl, Master: testMaster, AAD: []byte("метка")}
				out, rep, err := EmbedCover(cv, text, opt)
				if err != nil {
					t.Fatalf("%v/%v: %v", mode, lvl, err)
				}
				got, err := ExtractCover(out, "", []byte("метка"), testMaster)
				if err != nil {
					t.Fatalf("%v/%v %q: %v", mode, lvl, text, err)
				}
				if got.Text != alphabet.Normalize(text) || got.Mode != mode || got.Level != lvl {
					t.Fatalf("%v/%v: получено %q", mode, lvl, got.Text)
				}
				if got.Corrected != 0 {
					t.Fatalf("без повреждений исправлено %d байт", got.Corrected)
				}
				// изменены только младшие биты, не больше чем на 1
				diff := 0
				for i := range cv.Pix {
					d := int(out.Pix[i]) - int(cv.Pix[i])
					if d < -1 || d > 1 {
						t.Fatal("изменение больше ±1")
					}
					if d != 0 {
						diff++
					}
				}
				if diff != rep.Changes+rep.HeaderChanges {
					t.Fatalf("изменено %d, в отчёте %d+%d", diff, rep.Changes, rep.HeaderChanges)
				}
			}
		}
	}
}

func TestWrongPasswordAndAAD(t *testing.T) {
	cv := FromImage(testImage(200, 150, 2, false))
	out, _, err := EmbedCover(cv, "секрет", Options{Mode: suite.AsconAEAD128, Master: testMaster, AAD: []byte("A")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ExtractCover(out, "", []byte("A"), bytes.Repeat([]byte{1}, 32)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("чужой ключ: %v", err)
	}
	if _, err := ExtractCover(out, "", []byte("B"), testMaster); !errors.Is(err, ErrAuth) {
		t.Fatalf("чужие доп. данные: %v", err)
	}
	if _, err := ExtractCover(cv, "", nil, testMaster); !errors.Is(err, ErrNotFound) {
		t.Fatalf("пустое изображение: %v", err)
	}
}

// Настоящий пароль через Argon2id и сохранение в PNG/BMP с повторным чтением.
func TestPasswordAndFileFormats(t *testing.T) {
	for _, gray := range []bool{false, true} {
		img := testImage(160, 120, 3, gray)
		out, rep, err := Embed(img, "Проверка через файл", Options{Mode: suite.MagmaMGM, Password: "длинный пароль 123"})
		if err != nil {
			t.Fatal(err)
		}
		if gray != (FromImage(img).C == 1) {
			t.Fatal("серое изображение не распознано")
		}
		for name, enc := range map[string]func(*bytes.Buffer, image.Image) error{
			"png": func(b *bytes.Buffer, m image.Image) error { return png.Encode(b, m) },
			"bmp": func(b *bytes.Buffer, m image.Image) error { return bmp.Encode(b, m) },
		} {
			var buf bytes.Buffer
			if err := enc(&buf, out); err != nil {
				t.Fatal(err)
			}
			var back image.Image
			if name == "png" {
				back, err = png.Decode(&buf)
			} else {
				back, err = bmp.Decode(&buf)
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := Extract(back, "длинный пароль 123", nil)
			if err != nil || got.Text != "проверка через файл" {
				t.Fatalf("gray=%v %s: %v %+v (w=%d)", gray, name, err, got, rep.W)
			}
		}
	}
}

// Полностью прозрачные пиксели не используются и не меняются.
func TestTransparentPixels(t *testing.T) {
	img := testImage(200, 160, 4, false).(*image.NRGBA)
	for y := 0; y < 80; y++ {
		for x := 0; x < 200; x++ {
			img.Pix[img.PixOffset(x, y)+3] = 0
		}
	}
	cv := FromImage(img)
	out, _, err := EmbedCover(cv, longText, Options{Mode: suite.KuznyechikMGM, Master: testMaster})
	if err != nil {
		t.Fatal(err)
	}
	for i := range cv.Pix {
		if !cv.Usable(i) && out.Pix[i] != cv.Pix[i] {
			t.Fatal("изменён прозрачный пиксель")
		}
	}
	if got, err := ExtractCover(out, "", nil, testMaster); err != nil || got.Text != alphabet.Normalize(longText) {
		t.Fatalf("%v", err)
	}
}

func TestCapacity(t *testing.T) {
	cv := FromImage(testImage(40, 30, 5, false))
	_, _, err := EmbedCover(cv, longText+longText+longText, Options{Mode: suite.KuznyechikMGM, Level: LevelMax, Master: testMaster})
	var ce *CapacityError
	if !errors.As(err, &ce) {
		t.Fatalf("ожидалась ошибка ёмкости, получено %v", err)
	}
}
