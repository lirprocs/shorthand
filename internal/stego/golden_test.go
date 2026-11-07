package stego

// Этап 1 «на потом» (ветка later/stage1): образцовые файлы формата v3.
//
// testdata/golden_*.png — изображения с сообщением, сделанные этой версией.
// Тест проверяет, что (1) они по-прежнему извлекаются и (2) встраивание тех
// же данных с тем же nonce даёт те же пиксели. Если формат меняется
// намеренно — новая версия формата и пересоздание образцов:
//
//	UPDATE_GOLDEN=1 go test -run TestGolden ./internal/stego

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/lirprocs/shorthand/internal/alphabet"
	"github.com/lirprocs/shorthand/internal/crypto/suite"
)

// goldenCover — контейнер только из целочисленной арифметики (без sin и
// плавающей точки): одинаков на любой платформе.
func goldenCover(w, h int) *Cover {
	c := &Cover{W: w, H: h, C: 3, Pix: make([]uint8, w*h*3)}
	s := uint32(2463534242)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			for ch := 0; ch < 3; ch++ {
				s ^= s << 13
				s ^= s >> 17
				s ^= s << 5
				v := 60 + (x*97)/w + (y*61)/h + ch*20 + int(s%23)
				c.Pix[(y*w+x)*3+ch] = uint8(v)
			}
		}
	}
	return c
}

var goldenCases = []struct {
	name     string
	mode     suite.Mode
	level    Level
	password string
	aad      string
	text     string
}{
	{"kuznyechik_normal", suite.KuznyechikMGM, LevelNormal, "пароль образца 1", "", "Образец формата v3: Кузнечик-MGM, обычная устойчивость."},
	{"ascon_high", suite.AsconAEAD128, LevelHigh, "пароль образца 2", "метка", "Образец: Ascon-AEAD128, повышенная устойчивость. 😀"},
	{"magma_max", suite.MagmaMGM, LevelMax, "пароль образца 3", "", "Образец: Магма-MGM, максимальная устойчивость."},
}

func TestGolden(t *testing.T) {
	update := os.Getenv("UPDATE_GOLDEN") != ""
	for _, gc := range goldenCases {
		t.Run(gc.name, func(t *testing.T) {
			cv := goldenCover(160, 120)
			nonce := bytes.Repeat([]byte{0x21}, 16)
			out, _, err := EmbedCover(cv, gc.text, Options{
				Mode: gc.mode, Level: gc.level, Password: gc.password, AAD: []byte(gc.aad), Nonce: nonce,
			})
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join("testdata", "golden_"+gc.name+".png")
			if update {
				os.MkdirAll("testdata", 0o755)
				var buf bytes.Buffer
				if err := png.Encode(&buf, out.Image()); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			f, err := os.Open(path)
			if err != nil {
				t.Fatalf("нет образца %s (UPDATE_GOLDEN=1 создаёт): %v", path, err)
			}
			img, err := png.Decode(f)
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
			stored := FromImage(img)
			res, err := ExtractCover(stored, gc.password, []byte(gc.aad), nil)
			if err != nil {
				t.Fatalf("образец не извлекается: %v", err)
			}
			if res.Text != alphabet.Normalize(gc.text) || res.Mode != gc.mode || res.Level != gc.level {
				t.Fatalf("извлечено %q (%v, %v)", res.Text, res.Mode, res.Level)
			}
			if !bytes.Equal(stored.Pix, out.Pix) {
				t.Fatal("встраивание тех же данных дало другие пиксели: формат или алгоритм изменился")
			}
		})
	}
	_ = image.Point{}
}
